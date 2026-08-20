package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Message represents a single turn in the AI conversation history.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall describes a tool requested by the AI.
type ToolCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// AgentResponse defines the structured output expected from the AI.
type AgentResponse struct {
	ToolCall    *ToolCall `json:"tool_call,omitempty"`
	Script      string    `json:"script,omitempty"`
	Explanation string    `json:"explanation,omitempty"`
	IsSafe      bool      `json:"is_safe"`
}

type DiscoveryQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Required bool   `json:"required"`
}

type DiscoveryPlan struct {
	Questions []DiscoveryQuestion `json:"questions"`
}

type DiscoveryAction struct {
	ToolCall *ToolCall `json:"tool_call,omitempty"`
	Answer   string    `json:"answer,omitempty"`
	Done     bool      `json:"done"`
}

type DiscoveryAnswer struct {
	Status string `json:"status"`
	Value  string `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Config holds the application configuration.
type Config struct {
	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string
	UseMock       bool
}

// Client communicates with the configured AI service.
type Client struct {
	config     Config
	httpClient *http.Client
	// systemPrompt Message
}

const chatTransportAttempts = 2

// ChatCompletionsRequest is the request payload for chat completions.
type ChatCompletionsRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// ChatCompletionsResponse is the response payload from chat completions.
type ChatCompletionsResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// client should not handle history!!!
// think in terms of separation of concerns. each function should care about what it wants to do not about history of previous function calls

// SystemPrompt returns the system instructions for the AI.
func SystemPrompt() Message {
	return Message{
		Role: "system",
		Content: `You are a PowerShell command generator.

		Generate safe, minimal, valid PowerShell scripts that satisfy the user's request.

		Rules

		1. Output
		- Return only valid JSON matching the required schema:
		{
		  "tool_call": { "name": "tool_name", "args": { "arg_name": "value" } },
		  "script": "powershell_script",
		  "explanation": "explanation_of_action",
		  "is_safe": true
		}
		- Use "tool_call" ONLY when you need more information from the environment.
		- Use "tool_call" ONLY when you need more information from the environment.
		- If the user message contains "FINALIZATION MODE", tool_call MUST be null;
		  return a script or explain why no safe script can be produced.
		- Use "script" ONLY when you are ready to execute a PowerShell command.
		- Never use both "tool_call" and "script" in the same response.
		- Do not use markdown fences.
		- Do not include commentary outside JSON.

		2. Script Style
		- Prefer minimal, idiomatic PowerShell.
		- Prefer read-only operations unless modification is explicitly requested.
		- Avoid aliases unless they improve clarity.
		- Avoid interactive commands.
		- Avoid background, watcher, or streaming operations unless explicitly requested.
		- Avoid unsupported or conflicting parameters.
		- Avoid unnecessary variables or pipelines.
		- Prefer deterministic output.

		3. Safety
		- Set is_safe=false for:
		  - destructive or dangerous actions
		  - privilege escalation
		  - persistence or evasion
		  - credential access or exfiltration
		  - ransomware-like behavior
		  - ambiguous high-risk requests
		  - unauthorized network execution
		- Never bypass execution policy.
		- Never disable security tooling.

		4. File & Path Handling
		- Never guess paths.
		- Resolve relative language explicitly: "parent directory" means the
		  Parent Directory value in ENVIRONMENT, not the current directory.
		- Treat a path reported as missing as authoritative; do not retry the
		  identical path. Inspect its parent or use a corrected candidate.
		- If a required path is not explicitly listed in ENVIRONMENT, first use:
		  - list_dir
		  - exists
		- Only generate a script after confirming paths.
		- If a tool is used, leave script empty until discovery is complete.
		- Internal tools are not PowerShell commands. Never put exists, list_dir, or read_file in the script field.
		- Internal tools are not PowerShell commands. Never put tool names in the script field.
		- For a directory listing, use Get-ChildItem -Path 'path'. Do not combine switches with commas, such as -File,Directory.

		5. Repair Behavior
		- If a previous execution failed:
		  - minimally modify the prior script
		  - preserve original intent
		  - fix only the reported error when possible

		Available Tools
		1. list_dir({"path":"string"}) - Lists contents of a directory.
		2. find_entry({"path":"string","name":"string","type":"file|directory|any"}) - Finds one exact-name child entry.
		3. find_file({"path":"string","extension":"string","limit":number}) - Finds a bounded number of matching files directly in a directory.
		4. exists({"path":"string"}) - Checks if a path exists.
		5. read_file({"path":"string"}) - Reads the first 1000 characters of a file.
		6. search_entries({"path":"string","query":"string","type":"file|directory|any","limit":number})
		   - Finds direct child names containing query, ignoring case.
		7. user_downloads({}) - Returns the current user's standard Downloads folder.
		   Use this only when the user explicitly says their standard/personal
		   Downloads location. A generic named "downloads folder" must be found
		   from the supplied working-directory environment instead.
		- Treat every name supplied by the user as an untrusted search hint. The
		  filesystem tool result is the only source of truth for paths. Use
		  search_entries for partial names or likely spelling mistakes; do not
		  construct a path from a user-provided name unless a tool confirmed it.

		Workflow
		1. Use the smallest number of focused tools possible. The application
		  enforces a maximum of three discovery calls.
		2. Once sufficient information is available, return:
		   - script
		   - explanation
		   - is_safe
		3. If the request cannot be completed safely, set is_safe=false.
		`,
	}
}

// FinalSystemPrompt is used after discovery. It deliberately exposes no
// workflow that permits another tool call.
func FinalSystemPrompt() Message {
	return Message{
		Role: "system",
		Content: `You are the final PowerShell script generator.

FINAL SCRIPT MODE
- Do not request or call any tool.
- Return only valid JSON in exactly this shape:
  {"tool_call":null,"script":"powershell script","explanation":"short description","is_safe":true}
- All four keys are required. is_safe must be the JSON boolean true for every
  safe script; do not omit it.
- Set tool_call to null.
- Use the confirmed environment and tool results in the user message.
- For a request that can be completed safely, script MUST contain a minimal
  executable PowerShell script. Never return an empty script for a safe
  request.
- If the request cannot be completed safely, set is_safe=false and explain
  why. Do not return a placeholder, an empty script, or a tool call.
- Opening a confirmed image with Start-Process is a benign, non-destructive
  action. It may be marked safe when the image path was confirmed by tools.
- Opening a local folder or image is safe. A script may resolve the current
  user's Downloads folder dynamically from the USERPROFILE environment value and
  check whether it and an image exist before opening anything; that is not
  guessing a path and is safe.
- If a required folder or image is absent, the script must terminate with
  throw (not Write-Host or Write-Warning) so the application can retry with
  the actual failure.
- Set is_safe=false for unsafe or insufficiently confirmed actions.`,
	}
}

func DiscoveryPlanPrompt() Message {
	return Message{Role: "system", Content: `You are a filesystem discovery planner.
Return only JSON: {"questions":[{"id":"string","question":"string","required":true}]}.
Identify the minimum facts required to complete the user's task. Always return
at least one question when the task refers to an unknown file, folder, or path.
Do not call tools, generate PowerShell, guess paths, or include explanations
outside JSON.`}
}

func DiscoveryActionPrompt() Message {
	return Message{Role: "system", Content: `You are a filesystem discovery assistant.
Return only JSON matching:
{"tool_call":{"name":"tool_name","args":{}},"answer":"","done":false}
or {"tool_call":null,"answer":"...","done":true}.
For the current question, either choose exactly one focused tool call or answer
from confirmed facts. Never generate PowerShell. Use user_downloads with {} only
for an explicit personal/standard Downloads request. For a generic named folder,
use the working-directory context to inspect its parent or ancestors. Never use
list_dir when exists, find_entry, find_file, or search_entries can answer the
question. User-supplied names are hints only: use search_entries for partial or
possibly misspelled names, and treat the filesystem result as authoritative.`}
}

func DiscoveryAnswerPrompt() Message {
	return Message{Role: "system", Content: `You interpret one filesystem tool result.
Return only JSON: {"status":"confirmed|missing|unknown","value":"","reason":""}.
Do not guess. Use confirmed values only; if the result is insufficient, use unknown.`}
}

// Validate checks that the client configuration is complete.
func (cfg Config) Validate() error {
	if cfg.UseMock {
		return nil
	}
	if cfg.OpenAIAPIKey == "" {
		return errors.New("missing openai api key")
	}

	if cfg.OpenAIBaseURL == "" {
		return errors.New("missing openai base url")
	}

	if cfg.OpenAIModel == "" {
		return errors.New("missing openai model")
	}
	return nil
}

// NewClient creates a client from the supplied configuration.
func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 240 * time.Second,
		},
	}, nil
}

// ParseAgentJsonOutput parses an AI response into an AgentResponse.
func ParseAgentJsonOutput(input string) (*AgentResponse, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```") {
		// Small models sometimes wrap otherwise valid JSON in a Markdown
		// code fence despite the system prompt asking for raw JSON.
		lines := strings.Split(input, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if last := len(lines) - 1; strings.TrimSpace(lines[last]) == "```" {
				lines = lines[:last]
			}
			input = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	if input == "" {
		return nil, errors.New("AI returned an empty response body")
	}

	var agentResp AgentResponse

	if err := json.Unmarshal(
		[]byte(input),
		&agentResp,
	); err != nil {
		return nil, fmt.Errorf("invalid AI JSON: %w", err)
	}
	return &agentResp, nil
}

// OpenAIChat sends messages and returns the generated content.
func (c *Client) OpenAIChat(ctx context.Context, message []Message) (string, error) {
	result, err := c.chat(ctx, message)
	if err != nil {
		return "", err
	}
	return result.Choices[0].Message.Content, nil
}

// BuildMessages prepends the system prompt to user messages.
func BuildMessages(userInput []Message) []Message {
	return BuildMessagesWithSystem(SystemPrompt(), userInput)
}

func BuildMessagesWithSystem(system Message, userInput []Message) []Message {
	var result []Message
	result = append(result, system)
	result = append(result, userInput...)
	return result
}

// MakeHTTPRequest sends a chat completion request.
func (c *Client) MakeHTTPRequest(body ChatCompletionsRequest) (*http.Response, error) {
	return c.makeHTTPRequest(context.Background(), body)
}

func (c *Client) makeHTTPRequest(ctx context.Context, body ChatCompletionsRequest) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint, err := url.JoinPath(
		c.config.OpenAIBaseURL,
		"chat/completions",
	)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < chatTransportAttempts; attempt++ {
		// A request body cannot be reused after a failed HTTP attempt, so create a
		// fresh request for the retry. Chat-completion requests are safe to retry:
		// they have no side effects outside the local model server.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.config.OpenAIAPIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
		if attempt+1 < chatTransportAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return nil, fmt.Errorf("chat completion request failed after %d attempts: %w", chatTransportAttempts, lastErr)
}

// Chat sends messages and decodes the completion response.
func (c *Client) Chat(input []Message) (*ChatCompletionsResponse, error) {
	return c.chat(context.Background(), input)
}

func (c *Client) chat(ctx context.Context, input []Message) (*ChatCompletionsResponse, error) {
	return c.chatWithSystem(ctx, SystemPrompt(), input)
}

func (c *Client) chatWithSystem(ctx context.Context, system Message, input []Message) (*ChatCompletionsResponse, error) {
	messages := BuildMessagesWithSystem(system, input)

	body := ChatCompletionsRequest{
		Model:    c.config.OpenAIModel,
		Messages: messages,
	}

	resp, err := c.makeHTTPRequest(ctx, body)
	if err != nil {
		return &ChatCompletionsResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &ChatCompletionsResponse{}, fmt.Errorf("api returned %d: %s", resp.StatusCode, string(body))
	}

	var result ChatCompletionsResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &ChatCompletionsResponse{}, err
	}

	if len(result.Choices) == 0 {
		return &ChatCompletionsResponse{}, fmt.Errorf("empty response")
	}
	if strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return &ChatCompletionsResponse{}, fmt.Errorf("empty completion content")
	}
	return &result, nil
}

// GetShellScript returns a shell script generated from the supplied messages.
func (c *Client) GetShellScript(input []Message) (*AgentResponse, string, error) {
	if c.config.UseMock {
		if len(input) == 0 {
			return &AgentResponse{}, "", errors.New("cannot generate a mock response without messages")
		}
		return &AgentResponse{
			Script:      fmt.Sprintf("echo 'Mock: %s'", input[len(input)-1].Content),
			Explanation: "Mock response",
			IsSafe:      true,
		}, "", nil
	}

	result, err := c.Chat(input)
	if err != nil {
		return &AgentResponse{}, "", err
	}

	output, err := ParseAgentJsonOutput(result.Choices[0].Message.Content)
	if err != nil {
		return &AgentResponse{}, "", err
	}

	return output, "", nil
}

// GetFinalScript generates a script after discovery is complete. Unlike
// GetShellScript, this call uses a system prompt that forbids tool calls.
func (c *Client) GetFinalScript(input []Message) (*AgentResponse, string, error) {
	if c.config.UseMock {
		if len(input) == 0 {
			return &AgentResponse{}, "", errors.New("cannot generate a mock response without messages")
		}
		return &AgentResponse{
			Script:      fmt.Sprintf("echo 'Mock final: %s'", input[len(input)-1].Content),
			Explanation: "Mock final response",
			IsSafe:      true,
		}, "", nil
	}

	result, err := c.chatWithSystem(context.Background(), FinalSystemPrompt(), input)
	if err != nil {
		return &AgentResponse{}, "", err
	}
	output, err := ParseAgentJsonOutput(result.Choices[0].Message.Content)
	if err != nil {
		return &AgentResponse{}, "", err
	}
	return output, "", nil
}

func (c *Client) GetDiscoveryPlan(input []Message) (*DiscoveryPlan, error) {
	result, err := c.chatWithSystem(context.Background(), DiscoveryPlanPrompt(), input)
	if err != nil {
		return nil, err
	}
	var plan DiscoveryPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Choices[0].Message.Content)), &plan); err != nil {
		return nil, fmt.Errorf("invalid discovery plan: %w", err)
	}
	if len(plan.Questions) == 0 {
		return nil, errors.New("discovery plan contained no questions")
	}
	for _, question := range plan.Questions {
		if question.ID == "" || question.Question == "" {
			return nil, errors.New("discovery plan contained an incomplete question")
		}
	}
	return &plan, nil
}

func (c *Client) GetDiscoveryAction(question string, state string) (*DiscoveryAction, error) {
	input := []Message{{Role: "user", Content: fmt.Sprintf("QUESTION:\n%s\n\nCONFIRMED STATE:\n%s", question, state)}}
	result, err := c.chatWithSystem(context.Background(), DiscoveryActionPrompt(), input)
	if err != nil {
		return nil, err
	}
	var action DiscoveryAction
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Choices[0].Message.Content)), &action); err != nil {
		return nil, fmt.Errorf("invalid discovery action: %w", err)
	}
	return &action, nil
}

func (c *Client) GetDiscoveryAnswer(question, toolResult, state string) (*DiscoveryAnswer, error) {
	input := []Message{{Role: "user", Content: fmt.Sprintf("QUESTION:\n%s\n\nCONFIRMED STATE:\n%s\n\nTOOL RESULT:\n%s", question, state, toolResult)}}
	result, err := c.chatWithSystem(context.Background(), DiscoveryAnswerPrompt(), input)
	if err != nil {
		return nil, err
	}
	var answer DiscoveryAnswer
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Choices[0].Message.Content)), &answer); err != nil {
		return nil, fmt.Errorf("invalid discovery answer: %w", err)
	}
	return &answer, nil
}

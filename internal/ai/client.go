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
	"time"
)

// Message represents a single turn in the AI conversation history.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

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

// Config holds the application configuration.
type Config struct {
	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string
	UseMock       bool
}

type Client struct {
	config     Config
	httpClient *http.Client
	// systemPrompt Message
}

type ChatCompletionsRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatCompletionsResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// client should not handle history!!!
// think in terms of separation of concerns. each function should care about what it wants to do not about history of previous function calls

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
		- If a required path is not explicitly listed in ENVIRONMENT, first use:
		  - list_dir
		  - exists
		- Only generate a script after confirming paths.
		- If a tool is used, leave script empty until discovery is complete.

		5. Repair Behavior
		- If a previous execution failed:
		  - minimally modify the prior script
		  - preserve original intent
		  - fix only the reported error when possible

		Available Tools
		1. list_dir({"path":"string"}) - Lists contents of a directory.
		2. exists({"path":"string"}) - Checks if a path exists.
		3. read_file({"path":"string"}) - Reads the first 1000 characters of a file.

		Workflow
		1. Use tools if more information is required.
		2. Once sufficient information is available, return:
		   - script
		   - explanation
		   - is_safe
		3. If the request cannot be completed safely, set is_safe=false.
		`,
	}
}

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

func NewClient(cfg Config) (*Client, error) {

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 240 * time.Second},
	}, nil
}

func ParseAgentJsonOutput(input string) (*AgentResponse, error) {
	var agentResp AgentResponse

	if err := json.Unmarshal(
		[]byte(input),
		&agentResp,
	); err != nil {
		return nil, err
	}
	return &agentResp, nil

}

func (c *Client) OpenAIChat(ctx context.Context, message []Message) (string, error) {
	result, err := c.chat(ctx, message)
	if err != nil {
		return "", err
	}
	return result.Choices[0].Message.Content, nil
}

func BuildMessages(userInput []Message) []Message {
	var result []Message
	result = append(result, SystemPrompt())
	result = append(result, userInput...)
	return result
}

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

	// Setting up request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.config.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	// Making HTTP Request
	resp, err := c.httpClient.Do(req)
	return resp, err

}

func (c *Client) Chat(input []Message) (*ChatCompletionsResponse, error) {
	return c.chat(context.Background(), input)
}

func (c *Client) chat(ctx context.Context, input []Message) (*ChatCompletionsResponse, error) {

	messages := BuildMessages(input)

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
	return &result, nil
}

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

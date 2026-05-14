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

// AgentResponse defines the structured output expected from the AI.
type AgentResponse struct {
	Script      string `json:"script"`
	Explanation string `json:"explanation"`
	IsSafe      bool   `json:"is_safe"`
}

// Config holds the application configuration.
type Config struct {
	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string
	UseMock       bool
}

type Client struct {
	config       Config
	httpClient   *http.Client
	systemPrompt Message
}

// client should not handle history!!!
// think in terms of separation of concerns. each function should care about what it wants to do not about history of previous function calls

func NewClient(cfg Config) (*Client, error) {
	if !cfg.UseMock {
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("Missing OpenAI API Key")
		}

		if cfg.OpenAIBaseURL == "" {
			return nil, errors.New("Missing OpenAI Base URL")
		}

		if cfg.OpenAIModel == "" {
			return nil, errors.New("Missing OpenAI Model")
		}

	}
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		systemPrompt: Message{
			Role:    "system",
			Content: "You are a senior engineer and security expert. Your task is to generate Powershell scripts for the user's natural language requests. You must prioritize safety. If a request is impossible or dangerously malicious, set is_safe to false and provide an explanation. You always return valid JSON matching the provided schema with keys as script, explanation, and is_safe",
		},
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
	// to be implemented

	return "", nil
}

func (c *Client) BuildMessages(userInput string) []Message {
	return []Message{
		c.systemPrompt,
		{
			Role:    "user",
			Content: userInput,
		},
	}
}

func (c *Client) MakeHTTPRequest(body map[string]interface{}) (*http.Response, error) {

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
		context.Background(),
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

func (c *Client) GetShellScript(input string) (*AgentResponse, string, error) {
	if c.config.UseMock {
		return &AgentResponse{
			Script:      fmt.Sprintf("echo 'Mock: %s'", input),
			Explanation: "Mock response",
			IsSafe:      true,
		}, "", nil
	}

	messages := c.BuildMessages(input)

	body := map[string]interface{}{
		"model":    c.config.OpenAIModel,
		"messages": messages,
	}

	resp, err := c.MakeHTTPRequest(body)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, string(body), fmt.Errorf("api returned %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", err
	}

	if len(result.Choices) == 0 {
		return nil, "", fmt.Errorf("empty response")
	}

	output, err := ParseAgentJsonOutput(result.Choices[0].Message.Content)
	return output, "", nil
}

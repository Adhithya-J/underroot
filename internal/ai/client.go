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
	config     Config
	httpClient *http.Client
	history    []Message
}

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
			Timeout: 30 * time.Second,
		},
		history: []Message{
			{
				Role:    "system",
				Content: "You are a senior engineer and security expert. Your task is to generate Powershell scripts for the user's natural language requests. You must prioritize safety. If a request is impossible or dangerously malicious, set is_safe to false and provide an explanation. You always return valid JSON matching the provided schema with keys as script, explanation, and is_safe",
			},
		},
	}, nil
}

<<<<<<< HEAD
func (c *Client) GetShellScript(input string) (*AgentResponse, string, error) {
=======
func (c *Client) GetShellScript(ctx context.Context, input string) (*AgentResponse, error) {
>>>>>>> main
	if c.config.UseMock {
		return &AgentResponse{
			Script:      fmt.Sprintf("echo 'Mock: %s'", input),
			Explanation: "Mock response",
			IsSafe:      true,
		}, "", nil
	}

	c.history = append(c.history, Message{
		Role:    "user",
		Content: input,
	})

	body := map[string]interface{}{
		"model":    c.config.OpenAIModel,
		"messages": c.history,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}

<<<<<<< HEAD
	req, err := http.NewRequest(
		"POST",
		c.config.OpenAIBaseURL+"v1/chat/completions",
=======
	endpoint, err := url.JoinPath(
		c.config.OpenAIBaseURL,
		"chat/completions",
	)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
>>>>>>> main
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.config.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
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

	c.history = append(c.history, Message{
		Role:    "assistant",
		Content: result.Choices[0].Message.Content,
	})

	var agentResp AgentResponse

	if err := json.Unmarshal(
		[]byte(result.Choices[0].Message.Content),
		&agentResp,
	); err != nil {
		return nil, "", err
	}

<<<<<<< HEAD
	return &agentResp, "", nil
=======
	// fmt.Print("\n\nHistory:\n")
	// fmt.Println(c.history)
	// fmt.Print("\nHistory:\n")
	return &agentResp, nil
>>>>>>> main
}

package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	Model         string
	UseMock       bool
}

type Client struct {
	config  Config
	history []Message
}

func NewClient(cfg Config) (*Client, error) {
	if !cfg.UseMock {
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("Missing OpenAI API Key")
		}

		if cfg.OpenAIBaseURL == "" {
			return nil, errors.New("Missing OpenAI Base URL")
		}

		if cfg.Model == "" {
			return nil, errors.New("Missing OpenAI Model")
		}

	}
	return &Client{
		config: cfg,
		history: []Message{
			{
				Role:    "system",
				Content: "You are a senior engineer and security expert. Your task is to generate Powershell scripts for the user's natural language requests. You must prioritize safety. If a request is impossible or dangerously malicious, set is_safe to false and provide an explanation. You always return valid JSON matching the provided schema.",
			},
		},
	}, nil
}

func (c *Client) GetShellScript(input string) (*AgentResponse, error) {
	if c.config.UseMock {
		return &AgentResponse{
			Script:      fmt.Sprintf("echo 'Mock: %s'", input),
			Explanation: "Mock response",
			IsSafe:      true,
		}, nil
	}

	c.history = append(c.history, Message{
		Role:    "user",
		Content: input,
	})

	body := map[string]interface{}{
		"model":    c.config.Model,
		"messages": c.history,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"POST",
		c.config.OpenAIBaseURL+"/chat/completions",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.config.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api returned %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	var agentResp AgentResponse

	if err := json.Unmarshal(
		[]byte(result.Choices[0].Message.Content),
		&agentResp,
	); err != nil {
		return nil, err
	}

	return &agentResp, nil
}

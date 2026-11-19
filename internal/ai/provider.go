package ai

import (
	"errors"
	"fmt"
	"strings"
)

// Provider is the model-facing surface the agent depends on. Keeping the
// concrete transport behind an interface lets the application swap in another
// backend (Anthropic, Ollama, a local mock) without changing agent logic.
type Provider interface {
	// GetDiscoveryPlan asks the model which facts are still missing.
	GetDiscoveryPlan(input []Message) (*DiscoveryPlan, error)
	// GetDiscoveryAction asks the model to act on one discovery question.
	GetDiscoveryAction(question, state string) (*DiscoveryAction, error)
	// GetFinalScript asks the model for the final, structured script output.
	GetFinalScript(input []Message) (*AgentResponse, error)
}

// NewProvider builds the configured provider. UseMock returns a deterministic
// in-process provider so every model-facing path can be tested offline.
func NewProvider(cfg Config) (Provider, error) {
	if cfg.UseMock {
		return NewMockProvider(), nil
	}
	return NewClient(cfg)
}

// MockProvider is a fully in-process Provider. Unlike the old UseMock branch,
// it covers discovery as well as final generation, so no code path reaches the
// network by accident during tests or offline use.
type MockProvider struct {
	Plan   *DiscoveryPlan
	Action *DiscoveryAction
	Final  *AgentResponse

	PlanFunc   func(input []Message) (*DiscoveryPlan, error)
	ActionFunc func(question, state string) (*DiscoveryAction, error)
	FinalFunc  func(input []Message) (*AgentResponse, error)
}

var _ Provider = (*MockProvider)(nil)

// NewMockProvider returns a provider whose defaults need no network access.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// GetDiscoveryPlan returns the configured plan, an empty plan by default.
func (m *MockProvider) GetDiscoveryPlan(input []Message) (*DiscoveryPlan, error) {
	if m.PlanFunc != nil {
		return m.PlanFunc(input)
	}
	if m.Plan != nil {
		return m.Plan, nil
	}
	return &DiscoveryPlan{}, nil
}

// GetDiscoveryAction returns the configured action, a completed mock answer by
// default.
func (m *MockProvider) GetDiscoveryAction(question, state string) (*DiscoveryAction, error) {
	if m.ActionFunc != nil {
		return m.ActionFunc(question, state)
	}
	if m.Action != nil {
		return m.Action, nil
	}
	return &DiscoveryAction{Answer: "mock discovery answer", Done: true}, nil
}

// GetFinalScript returns a deterministic mock script that echoes the task.
func (m *MockProvider) GetFinalScript(input []Message) (*AgentResponse, error) {
	if m.FinalFunc != nil {
		return m.FinalFunc(input)
	}
	if m.Final != nil {
		return m.Final, nil
	}
	if len(input) == 0 {
		return nil, errors.New("cannot generate a mock response without messages")
	}
	return &AgentResponse{
		Script:      fmt.Sprintf("echo 'Mock final: %s'", sanitizeMockEcho(input[len(input)-1].Content)),
		Explanation: "Mock final response",
		IsSafe:      true,
	}, nil
}

// sanitizeMockEcho escapes single quotes for a PowerShell single-quoted string.
func sanitizeMockEcho(text string) string {
	return strings.ReplaceAll(text, "'", "''")
}

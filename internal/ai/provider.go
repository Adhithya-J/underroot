package ai

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

// MockProvider is a fully in-process Provider. Its defaults delegate to
// ScenarioEngine, which recognizes common filesystem requests and returns
// plausible discovery calls and PowerShell. The override fields remain for
// focused tests.
type MockProvider struct {
	Engine *ScenarioEngine

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
	return &MockProvider{Engine: NewScenarioEngine()}
}

func (m *MockProvider) engine() *ScenarioEngine {
	if m.Engine == nil {
		m.Engine = NewScenarioEngine()
	}
	return m.Engine
}

// GetDiscoveryPlan returns the configured plan or a scenario-derived plan.
func (m *MockProvider) GetDiscoveryPlan(input []Message) (*DiscoveryPlan, error) {
	if m.PlanFunc != nil {
		return m.PlanFunc(input)
	}
	if m.Plan != nil {
		return m.Plan, nil
	}
	return m.engine().GetDiscoveryPlan(input)
}

// GetDiscoveryAction returns the configured action or a scenario-derived one.
func (m *MockProvider) GetDiscoveryAction(question, state string) (*DiscoveryAction, error) {
	if m.ActionFunc != nil {
		return m.ActionFunc(question, state)
	}
	if m.Action != nil {
		return m.Action, nil
	}
	return m.engine().GetDiscoveryAction(question, state)
}

// GetFinalScript returns the configured response or a scenario-derived script.
func (m *MockProvider) GetFinalScript(input []Message) (*AgentResponse, error) {
	if m.FinalFunc != nil {
		return m.FinalFunc(input)
	}
	if m.Final != nil {
		return m.Final, nil
	}
	return m.engine().GetFinalScript(input)
}

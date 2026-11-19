package agent

import (
	"strings"
	"testing"

	"github.com/Adhithya-J/underroot.git/internal/ai"
)

func TestClassifyResponseFinalOutput(t *testing.T) {
	step := ClassifyResponse(&ai.AgentResponse{Script: "Get-ChildItem", IsSafe: true})
	if step.Type != NextStepFinalOutput {
		t.Fatalf("expected final output, got %s", step.Type)
	}
}

func TestClassifyResponseHandoff(t *testing.T) {
	resp := &ai.AgentResponse{ToolCall: &ai.ToolCall{
		Name: "handoff",
		Args: map[string]interface{}{"agent": "windows-expert", "reason": "needs registry knowledge"},
	}}
	step := ClassifyResponse(resp)
	if step.Type != NextStepHandoff {
		t.Fatalf("expected handoff, got %s", step.Type)
	}
	if step.HandoffTo != "windows-expert" || step.Reason != "needs registry knowledge" {
		t.Fatalf("unexpected handoff details: %+v", step)
	}
}

func TestClassifyResponseRunAgain(t *testing.T) {
	resp := &ai.AgentResponse{ToolCall: &ai.ToolCall{
		Name: "list_dir",
		Args: map[string]interface{}{"path": `C:\temp`},
	}}
	step := ClassifyResponse(resp)
	if step.Type != NextStepRunAgain || len(step.ToolCalls) != 1 || step.ToolCalls[0].Name != "list_dir" {
		t.Fatalf("expected run_again with the tool call, got %+v", step)
	}
}

func TestRunInterruptsWhenToolApprovalIsDenied(t *testing.T) {
	provider := ai.NewMockProvider()
	provider.Plan = &ai.DiscoveryPlan{Questions: []ai.DiscoveryQuestion{
		{ID: "q1", Question: "Which folder?", Required: true},
	}}
	provider.Action = &ai.DiscoveryAction{ToolCall: &ai.ToolCall{
		Name: "exists",
		Args: map[string]interface{}{"path": `C:\temp`},
	}}

	runner := NewAgent(provider)
	runner.ToolApproval = func(*ai.ToolCall) (bool, error) {
		return false, nil
	}

	result, errType, err := runner.Run(Context{Task: "inspect the temp folder"})
	if err != nil || errType != "" {
		t.Fatalf("expected a clean interruption, got type=%q err=%v", errType, err)
	}
	if result == nil || result.NextStep.Type != NextStepInterruption {
		t.Fatalf("expected an interruption step, got %+v", result)
	}
	if result.NextStep.Interrupted == nil || result.NextStep.Interrupted.Name != "exists" {
		t.Fatalf("expected the interrupted tool call, got %+v", result.NextStep)
	}
	if !strings.Contains(result.NextStep.Reason, "denied") {
		t.Fatalf("expected a denial reason, got %q", result.NextStep.Reason)
	}
}

func TestRunHandoffContinuesToFinalScript(t *testing.T) {
	provider := ai.NewMockProvider()
	provider.Plan = &ai.DiscoveryPlan{Questions: []ai.DiscoveryQuestion{
		{ID: "q1", Question: "Who should handle this?", Required: true},
	}}
	provider.Action = &ai.DiscoveryAction{ToolCall: &ai.ToolCall{
		Name: "handoff",
		Args: map[string]interface{}{"agent": "filesystem-agent", "reason": "path discovery"},
	}}

	runner := NewAgent(provider)
	result, errType, err := runner.Run(Context{Task: "open the logs folder"})
	if err != nil || errType != "" {
		t.Fatalf("expected a successful run, got type=%q err=%v", errType, err)
	}
	if result == nil || result.NextStep.Type != NextStepFinalOutput {
		t.Fatalf("expected final output after handoff, got %+v", result)
	}
}

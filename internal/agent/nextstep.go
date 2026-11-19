package agent

import "github.com/Adhithya-J/underroot.git/internal/ai"

// NextStepType is the action the runtime should take after one model turn.
type NextStepType string

const (
	// NextStepFinalOutput means the model produced a final response.
	NextStepFinalOutput NextStepType = "final_output"
	// NextStepHandoff means control moved to another agent role.
	NextStepHandoff NextStepType = "handoff"
	// NextStepRunAgain means tool calls are present and should be executed.
	NextStepRunAgain NextStepType = "run_again"
	// NextStepInterruption means a tool needs human approval before continuing.
	NextStepInterruption NextStepType = "interruption"
)

// NextStep describes why the agent loop continued or stopped.
type NextStep struct {
	Type        NextStepType   `json:"type"`
	ToolCalls   []ai.ToolCall  `json:"tool_calls,omitempty"`
	HandoffTo   string         `json:"handoff_to,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Interrupted *ai.ToolCall   `json:"interrupted,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ClassifyResponse converts a structured agent response into a next-step state.
func ClassifyResponse(resp *ai.AgentResponse) NextStep {
	if resp == nil || resp.ToolCall == nil {
		return NextStep{Type: NextStepFinalOutput}
	}
	return ClassifyToolCall(resp.ToolCall)
}

// ClassifyToolCall converts one tool call into a next-step state. A handoff tool
// swaps agent roles; every other tool call requires another loop iteration.
func ClassifyToolCall(call *ai.ToolCall) NextStep {
	if call == nil {
		return NextStep{Type: NextStepFinalOutput}
	}
	if call.Name == "handoff" {
		target, _ := call.Args["agent"].(string)
		reason, _ := call.Args["reason"].(string)
		return NextStep{
			Type:      NextStepHandoff,
			ToolCalls: []ai.ToolCall{*call},
			HandoffTo: target,
			Reason:    reason,
		}
	}
	return NextStep{
		Type:      NextStepRunAgain,
		ToolCalls: []ai.ToolCall{*call},
	}
}

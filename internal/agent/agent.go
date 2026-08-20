package agent

import (
	"fmt"

	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/ui"
	"github.com/Adhithya-J/underroot.git/internal/validator"
)

// Agent orchestrates AI responses and tool execution.
type Agent struct {
	aiClient *ai.Client
}

// there are few types of errors - validation error (misspellings), safety error(blacklisted keywords), ai generation error (malformed script) and execution error (logic, permissions).
// The agent should know which type of error it is dealing with and take action appropriately. Also the structure would be different
// For the initial version, let's build a basic version of error type (unified)

// Also should entire error history be included?
// A naive approach would be to use the entire error history
// But I think when we run into an execution error, we can be sure that the script has passed the safety and validation error and those errors can be removed from error history

// RunResult contains the validated result of an agent run.
type RunResult struct {
	Script      string
	Explanation string
	Success     bool
}

// NewAgent creates an agent using the provided AI client.
func NewAgent(aiClient *ai.Client) *Agent {
	return &Agent{
		aiClient: aiClient,
	}
}

// ToolFunc defines a callable tool implementation.
type ToolFunc func(map[string]any) (string, error)

// ExecuteTool executes the tool call returned by the AI.
func (a *Agent) ExecuteTool(tc *ai.ToolCall) (string, error) {
	path, ok := tc.Args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing 'path' argument")
	}
	switch tc.Name {
	case "list_dir":
		return executor.List_Dir(path) // Or executor directly if bypassing validator
	case "read_file":
		return executor.Read_File(path)
	case "exists":
		return executor.Exists(path)
	default:
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// Run generates, validates, and returns a safe shell script.
func (a *Agent) Run(initialInput []ai.Message) (*RunResult, string, error) {
	currentPrompt := initialInput
	var resp *ai.AgentResponse
	var err error
	for i := 0; i < 5; i++ {
		resp, _, err = a.aiClient.GetShellScript(currentPrompt)
		if err != nil {
			// A malformed response is an internal LLM formatting failure. Retry
			// the same request with a correction hint instead of immediately
			// surfacing it as a user-visible agent failure.
			currentPrompt[len(currentPrompt)-1].Content += fmt.Sprintf(
				"\n\nINTERNAL RESPONSE FORMAT ERROR:\n%s\nReturn only valid JSON matching the required schema. Do not use Markdown fences.",
				err,
			)
			continue
		}
		if resp.ToolCall != nil {
			ui.PrintGray("Tool Call: " + resp.ToolCall.Name)
			result, err := a.ExecuteTool(resp.ToolCall)
			if err != nil {
				result = "Error : " + err.Error()
			}
			// Keep the result in the existing request, but explicitly close the
			// discovery phase. Small models otherwise tend to repeat the same
			// tool call because the original task remains the most salient text.
			currentPrompt[len(currentPrompt)-1].Content += fmt.Sprintf(`

TOOL RESULT (%s)
%s

DISCOVERY COMPLETE. Do not call any tool again. Return only the final
PowerShell command in the script field, with tool_call set to null.`, resp.ToolCall.Name, result)

		}

		if resp.ToolCall == nil {
			break
		}

	}

	// Do not allow the last tool request to fall through as an empty or stale
	// script. Return a retryable generation error instead.
	if resp != nil && resp.ToolCall != nil {
		return nil, "AIGenerationFailed", fmt.Errorf("AI exhausted tool-call retries without generating a script")
	}

	if err != nil || resp == nil {
		if err == nil {
			err = fmt.Errorf("no response generated")
		}
		return nil, "AIGenerationFailed", fmt.Errorf("AI generation failed after internal retries: %v", err)
	}

	if resp.Script == "" {
		errMsg := fmt.Errorf("AI generation returned an empty script")
		return nil, "AIGeneratedEmptyString", errMsg
	}

	// AI based Safety
	if !resp.IsSafe {
		errMsg := fmt.Errorf("AI marked script unsafe: %s", resp.Explanation)
		return nil, "AIMarkedUnsafe", errMsg

	}

	// Static rule based check
	if err := validator.Validate(resp.Script); err != nil {
		errMsg := fmt.Errorf("validation failed: %s", err)
		return nil, "RuleBasedValidationFailed", errMsg
	}

	return &RunResult{
		Script:      resp.Script,
		Explanation: resp.Explanation,
		Success:     true,
	}, "", nil
}

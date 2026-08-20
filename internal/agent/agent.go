package agent

import (
	"fmt"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/ui"
	"github.com/Adhithya-J/underroot.git/internal/validator"
)

// Agent orchestrates AI responses and tool execution.
type Agent struct {
	aiClient *ai.Client
}

const maxDiscoveryToolCalls = 3

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
	stringArg := func(name string) (string, error) {
		value, ok := tc.Args[name].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("missing '%s' argument", name)
		}
		return value, nil
	}
	path, err := stringArg("path")
	if err != nil {
		return "", err
	}
	switch tc.Name {
	case "list_dir":
		return executor.List_Dir(path) // Or executor directly if bypassing validator
	case "find_entry":
		name, err := stringArg("name")
		if err != nil {
			return "", err
		}
		entryType, _ := tc.Args["type"].(string)
		if entryType == "" {
			entryType = "any"
		}
		return executor.Find_Entry(path, name, entryType)
	case "find_file":
		extension, err := stringArg("extension")
		if err != nil {
			return "", err
		}
		limit := 20
		if value, ok := tc.Args["limit"].(float64); ok {
			limit = int(value)
		}
		return executor.Find_File(path, extension, limit)
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
	if len(initialInput) == 0 {
		return nil, "AIGenerationFailed", fmt.Errorf("empty agent input")
	}
	plan, err := a.aiClient.GetDiscoveryPlan(initialInput)
	if err != nil {
		return nil, "AIGenerationFailed", err
	}
	if len(plan.Questions) > maxDiscoveryToolCalls {
		plan.Questions = plan.Questions[:maxDiscoveryToolCalls]
	}

	state := "No facts confirmed."
	for _, question := range plan.Questions {
		for attempt := 0; attempt < 2; attempt++ {
			action, actionErr := a.aiClient.GetDiscoveryAction(question.Question, state)
			if actionErr != nil {
				return nil, "AIGenerationFailed", actionErr
			}
			if action.ToolCall == nil {
				state = fmt.Sprintf("%s\n- %s [confirmed]: %s", state, question.ID, action.Answer)
				break
			}

			ui.PrintGray("Tool Call: " + action.ToolCall.Name)
			result, toolErr := a.ExecuteTool(action.ToolCall)
			if toolErr != nil {
				result = "TOOL_ERROR: " + toolErr.Error()
			}
			// The tool result is already authoritative. Sending it directly to the
			// final generator avoids a redundant model request for every discovery
			// call, which is particularly important for small local models.
			state = fmt.Sprintf("%s\n- %s [tool result]: %s", state, question.ID, result)
			break
		}
	}

	task := initialInput[len(initialInput)-1].Content
	if marker := strings.Index(task, "\n\tENVIRONMENT"); marker >= 0 {
		task = task[:marker]
	}
	if marker := strings.Index(task, "\nENVIRONMENT"); marker >= 0 {
		task = task[:marker]
	}
	finalInput := []ai.Message{{Role: "user", Content: fmt.Sprintf("TASK:\n%s\n\nORIGINAL REQUEST CONTEXT:\n%s\n\nDISCOVERY STATE:\n%s\n\nGenerate the final script only from confirmed facts. Treat unknown facts as unavailable.", task, initialInput[len(initialInput)-1].Content, state)}}
	resp, _, err := a.aiClient.GetFinalScript(finalInput)
	if err != nil {
		return nil, "AIGenerationFailed", err
	}
	if resp.ToolCall != nil {
		return nil, "AIGenerationFailed", fmt.Errorf("final script generator returned a tool call")
	}

	// AI based Safety
	if !resp.IsSafe {
		errMsg := fmt.Errorf("AI marked script unsafe: %s", resp.Explanation)
		return nil, "AIMarkedUnsafe", errMsg

	}
	if strings.TrimSpace(resp.Script) == "" {
		detail := strings.TrimSpace(resp.Explanation)
		if detail == "" {
			detail = "no explanation was provided"
		}
		return nil, "AIGeneratedEmptyString", fmt.Errorf("AI returned no executable script (%s)", detail)
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

package agent

import (
	"encoding/json"
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
	switch tc.Name {
	case "user_downloads":
		return executor.UserDownloads()
	case "list_dir":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		return executor.List_Dir(path) // Or executor directly if bypassing validator
	case "find_entry":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		name, err := stringArg("name")
		if err != nil {
			return "", err
		}
		entryType, _ := tc.Args["type"].(string)
		if entryType == "" {
			entryType = "any"
		}
		return executor.Find_Entry(path, name, entryType)
	case "search_entries":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		query, err := stringArg("query")
		if err != nil {
			return "", err
		}
		entryType, _ := tc.Args["type"].(string)
		if entryType == "" {
			entryType = "any"
		}
		limit := 20
		if value, ok := tc.Args["limit"].(float64); ok {
			limit = int(value)
		}
		return executor.Search_Entries(path, query, entryType, limit)
	case "find_file":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
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
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		return executor.Read_File(path)
	case "exists":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		return executor.Exists(path)
	default:
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// SummarizeToolResult keeps small outputs (<= 3 items) verbatim, but summarizes
// exploratory or large tool outputs to prevent context bloat.
func SummarizeToolResult(toolName, rawResult string) string {
	rawResult = strings.TrimSpace(rawResult)
	if strings.HasPrefix(rawResult, "TOOL_ERROR:") {
		return rawResult
	}

	switch toolName {
	case "list_dir":
		var res struct {
			Path      string `json:"path"`
			Entries   []struct {
				Name      string `json:"name"`
				Directory bool   `json:"directory"`
			} `json:"entries"`
			Shown     int  `json:"shown"`
			Truncated bool `json:"truncated"`
		}
		if err := json.Unmarshal([]byte(rawResult), &res); err != nil {
			return rawResult
		}
		// If only a few entries (<= 3), no need to summarize
		if len(res.Entries) <= 3 && !res.Truncated {
			return rawResult
		}
		// Exploratory or large output: summarize
		var sample []string
		for i := 0; i < len(res.Entries) && i < 3; i++ {
			kind := "file"
			if res.Entries[i].Directory {
				kind = "dir"
			}
			sample = append(sample, fmt.Sprintf("%s (%s)", res.Entries[i].Name, kind))
		}
		summary := fmt.Sprintf("Directory %s contains %d items (first 3: %s)",
			res.Path, res.Shown, strings.Join(sample, ", "))
		if res.Truncated {
			summary += " [truncated; more items exist]"
		}
		return summary

	case "search_entries":
		var res struct {
			Path    string `json:"path"`
			Query   string `json:"query"`
			Entries []struct {
				Name      string `json:"name"`
				Directory bool   `json:"directory"`
				FullName  string `json:"full_name"`
			} `json:"entries"`
			Shown int `json:"shown"`
		}
		if err := json.Unmarshal([]byte(rawResult), &res); err != nil {
			return rawResult
		}
		if len(res.Entries) <= 3 {
			return rawResult
		}
		var sample []string
		for i := 0; i < 3; i++ {
			sample = append(sample, res.Entries[i].FullName)
		}
		return fmt.Sprintf("Search in %s for '%s' returned %d matches. Top 3: [%s]",
			res.Path, res.Query, res.Shown, strings.Join(sample, ", "))

	case "find_file":
		var res struct {
			Path      string `json:"path"`
			Extension string `json:"extension"`
			Files     []struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
			} `json:"files"`
			Shown int `json:"shown"`
		}
		if err := json.Unmarshal([]byte(rawResult), &res); err != nil {
			return rawResult
		}
		if len(res.Files) <= 3 {
			return rawResult
		}
		var sample []string
		for i := 0; i < 3; i++ {
			sample = append(sample, res.Files[i].FullName)
		}
		return fmt.Sprintf("Found %d matching %s files in %s. First 3: [%s]",
			res.Shown, res.Extension, res.Path, strings.Join(sample, ", "))

	case "read_file":
		lines := strings.Split(rawResult, "\n")
		if len(rawResult) > 300 || len(lines) > 10 {
			preview := rawResult
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			return fmt.Sprintf("File content (%d lines, %d bytes): %s", len(lines), len(rawResult), preview)
		}
		return rawResult

	default:
		// exists, find_entry, user_downloads are already single-entry/concise facts
		return rawResult
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

	type confirmedFact struct {
		ID      string
		Summary string
	}
	var facts []confirmedFact

	requestCtx := initialInput[len(initialInput)-1].Content
	for _, question := range plan.Questions {
		for attempt := 0; attempt < 2; attempt++ {
			var factsSb strings.Builder
			if len(facts) == 0 {
				factsSb.WriteString("No facts confirmed yet.")
			} else {
				for _, f := range facts {
					factsSb.WriteString(fmt.Sprintf("- %s: %s\n", f.ID, f.Summary))
				}
			}
			state := fmt.Sprintf("REQUEST CONTEXT:\n%s\n\nCONFIRMED FACTS:\n%s", requestCtx, factsSb.String())

			action, actionErr := a.aiClient.GetDiscoveryAction(question.Question, state)
			if actionErr != nil {
				return nil, "AIGenerationFailed", actionErr
			}
			if action.ToolCall == nil {
				facts = append(facts, confirmedFact{
					ID:      question.ID,
					Summary: action.Answer,
				})
				break
			}

			ui.PrintGray("Tool Call: " + action.ToolCall.Name)
			result, toolErr := a.ExecuteTool(action.ToolCall)
			if toolErr != nil {
				result = "TOOL_ERROR: " + toolErr.Error()
			} else {
				result = SummarizeToolResult(action.ToolCall.Name, result)
			}
			facts = append(facts, confirmedFact{
				ID:      question.ID,
				Summary: result,
			})
			break
		}
	}

	var finalSb strings.Builder
	finalSb.WriteString(requestCtx)
	finalSb.WriteString("\n\nCONFIRMED DISCOVERY FACTS:\n")
	if len(facts) == 0 {
		finalSb.WriteString("No additional filesystem facts required.\n")
	} else {
		for _, f := range facts {
			finalSb.WriteString(fmt.Sprintf("- %s: %s\n", f.ID, f.Summary))
		}
	}
	finalSb.WriteString("\nGenerate the final script only from confirmed facts. Treat unknown paths as unavailable.")

	finalInput := []ai.Message{{
		Role:    "user",
		Content: finalSb.String(),
	}}
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

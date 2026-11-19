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
	aiClient ai.Provider
	// ToolApproval lets the host pause a run when a tool needs human approval.
	// Returning false produces a NextStepInterruption instead of executing.
	ToolApproval func(call *ai.ToolCall) (bool, error)
}

const maxDiscoveryToolCalls = 3

// RunResult contains the validated result of an agent run.
type RunResult struct {
	Script      string
	Explanation string
	NextStep    NextStep
}

// NewAgent creates an agent using the provided AI provider.
func NewAgent(aiProvider ai.Provider) *Agent {
	return &Agent{
		aiClient: aiProvider,
	}
}

// ExecuteTool executes the tool call returned by the AI.
func (a *Agent) ExecuteTool(tc *ai.ToolCall) (string, error) {
	stringArg := func(name string) (string, error) {
		value, ok := tc.Args[name].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("missing '%s' argument", name)
		}
		return value, nil
	}
	optionalType := func() string {
		if t, ok := tc.Args["type"].(string); ok && t != "" {
			return t
		}
		return "any"
	}
	optionalLimit := func() int {
		if limit, ok := tc.Args["limit"].(float64); ok && limit > 0 {
			return int(limit)
		}
		return 20
	}

	switch tc.Name {
	case "user_downloads":
		return executor.UserDownloads()
	case "list_dir":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		return executor.List_Dir(path)
	case "find_entry":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		name, err := stringArg("name")
		if err != nil {
			return "", err
		}
		return executor.Find_Entry(path, name, optionalType())
	case "search_entries":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		query, err := stringArg("query")
		if err != nil {
			return "", err
		}
		return executor.Search_Entries(path, query, optionalType(), optionalLimit())
	case "find_file":
		path, err := stringArg("path")
		if err != nil {
			return "", err
		}
		extension, err := stringArg("extension")
		if err != nil {
			return "", err
		}
		return executor.Find_File(path, extension, optionalLimit())
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
			Path    string `json:"path"`
			Entries []struct {
				Name      string `json:"name"`
				Directory bool   `json:"directory"`
			} `json:"entries"`
			Shown     int  `json:"shown"`
			Truncated bool `json:"truncated"`
		}
		if err := json.Unmarshal([]byte(rawResult), &res); err != nil {
			return rawResult
		}
		if len(res.Entries) <= 3 && !res.Truncated {
			return rawResult
		}
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
				FullName string `json:"full_name"`
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
		for i := 0; i < 3 && i < len(res.Entries); i++ {
			sample = append(sample, res.Entries[i].FullName)
		}
		return fmt.Sprintf("Search in %s for '%s' returned %d matches. Top 3: [%s]",
			res.Path, res.Query, res.Shown, strings.Join(sample, ", "))

	case "find_file":
		var res struct {
			Path      string `json:"path"`
			Extension string `json:"extension"`
			Files     []struct {
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
		for i := 0; i < 3 && i < len(res.Files); i++ {
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

// KnownFolder is a verified standard folder supplied by the application.
type KnownFolder struct {
	Name string
	Path string
}

// Context contains the structured environment and conversation state for an agent run.
type Context struct {
	Task           string
	WorkingDir     string
	ParentDir      string
	FolderName     string
	History        string
	FailureHistory string
	ActiveError    string
	Objectives     string
	KnownFolders   []KnownFolder
}

type confirmedFact struct {
	ID      string
	Summary string
}

func formatFacts(facts []confirmedFact, emptyMsg string) string {
	if len(facts) == 0 {
		return emptyMsg
	}
	var sb strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&sb, "- %s: %s\n", f.ID, f.Summary)
	}
	return sb.String()
}

// FormatKnownFolders renders verified known folders as trusted prompt facts.
func FormatKnownFolders(folders []KnownFolder) string {
	var sb strings.Builder
	for _, folder := range folders {
		if strings.TrimSpace(folder.Path) == "" {
			continue
		}
		fmt.Fprintf(&sb, "- %s: %s\n", folder.Name, folder.Path)
	}
	text := strings.TrimRight(sb.String(), "\n")
	if text == "" {
		return "None. Use filesystem discovery tools for path questions."
	}
	return text
}

func matchKnownFolders(task string, folders []KnownFolder) []KnownFolder {
	var matches []KnownFolder
	for _, folder := range folders {
		if strings.TrimSpace(folder.Path) == "" {
			continue
		}
		if mentionsWord(task, folder.Name) {
			matches = append(matches, folder)
		}
	}
	return matches
}

func questionMentionsKnownFolder(question string, folders []KnownFolder) bool {
	for _, folder := range folders {
		if mentionsWord(question, folder.Name) {
			return true
		}
	}
	return false
}

func knownFolderFactID(name string) string {
	return "known_folder_" + strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "_"))
}

func mentionsWord(text, word string) bool {
	lowerText := strings.ToLower(text)
	lowerWord := strings.ToLower(strings.TrimSpace(word))
	if lowerWord == "" {
		return false
	}

	offset := 0
	for {
		index := strings.Index(lowerText[offset:], lowerWord)
		if index < 0 {
			return false
		}
		index += offset

		beforeOK := index == 0 || !isWordByte(lowerText[index-1])
		end := index + len(lowerWord)
		afterOK := end == len(lowerText) || !isWordByte(lowerText[end])
		if beforeOK && afterOK {
			return true
		}
		offset = end
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// Run generates, validates, and returns a safe shell script.
func (a *Agent) Run(ctx Context) (*RunResult, string, error) {
	if strings.TrimSpace(ctx.Task) == "" {
		return nil, "AIGenerationFailed", fmt.Errorf("empty agent task")
	}

	knownFolders := FormatKnownFolders(ctx.KnownFolders)

	// 1. Discovery Plan: planner only needs the active task and directory environment
	planContent := fmt.Sprintf("TASK:\n%s\n\nENVIRONMENT:\nFolder: %s\nWorking Directory: %s\nParent Directory: %s\nVerified Known Folders:\n%s",
		ctx.Task, ctx.FolderName, ctx.WorkingDir, ctx.ParentDir, knownFolders)
	if ctx.Objectives != "" {
		planContent += "\n\n" + ctx.Objectives
	}
	planInput := []ai.Message{{
		Role:    "user",
		Content: planContent,
	}}
	plan, err := a.aiClient.GetDiscoveryPlan(planInput)
	if err != nil {
		return nil, "AIGenerationFailed", err
	}
	if len(plan.Questions) > maxDiscoveryToolCalls {
		plan.Questions = plan.Questions[:maxDiscoveryToolCalls]
	}

	// Verified known folders named in the task are already answered facts. They
	// are injected before discovery so no tool call is needed to resolve them.
	matchedFolders := matchKnownFolders(ctx.Task, ctx.KnownFolders)
	facts := make([]confirmedFact, 0, len(matchedFolders))
	for _, folder := range matchedFolders {
		facts = append(facts, confirmedFact{
			ID:      knownFolderFactID(folder.Name),
			Summary: folder.Path,
		})
	}

	for _, question := range plan.Questions {
		if questionMentionsKnownFolder(question.Question, matchedFolders) {
			continue
		}

		state := fmt.Sprintf("TASK:\n%s\nWorking Directory: %s\nParent Directory: %s\n\nVERIFIED KNOWN FOLDERS:\n%s\n\nCONFIRMED FACTS:\n%s",
			ctx.Task, ctx.WorkingDir, ctx.ParentDir, knownFolders, formatFacts(facts, "No facts confirmed yet."))
		if ctx.Objectives != "" {
			state += "\n\n" + ctx.Objectives
		}

		action, actionErr := a.aiClient.GetDiscoveryAction(question.Question, state)
		if actionErr != nil {
			return nil, "AIGenerationFailed", actionErr
		}
		if action.ToolCall == nil {
			facts = append(facts, confirmedFact{
				ID:      question.ID,
				Summary: action.Answer,
			})
			continue
		}

		step := ClassifyToolCall(action.ToolCall)
		if step.Type == NextStepHandoff {
			target := strings.TrimSpace(step.HandoffTo)
			if target == "" {
				target = "default agent"
			}
			facts = append(facts, confirmedFact{
				ID:      question.ID,
				Summary: fmt.Sprintf("Handoff accepted: %s (%s)", target, step.Reason),
			})
			ui.PrintGray("Handoff: " + target)
			continue
		}

		if a.ToolApproval != nil {
			approved, approvalErr := a.ToolApproval(action.ToolCall)
			if approvalErr != nil || !approved {
				reason := "tool approval was denied"
				if approvalErr != nil {
					reason = approvalErr.Error()
				}
				return &RunResult{NextStep: NextStep{
					Type:        NextStepInterruption,
					ToolCalls:   []ai.ToolCall{*action.ToolCall},
					Interrupted: action.ToolCall,
					Reason:      reason,
				}}, "", nil
			}
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
	}

	// 2. Final Script Generation: receives the task, environment, confirmed facts, and active retry error (if retrying)
	var finalSb strings.Builder
	fmt.Fprintf(&finalSb, "TASK:\n%s\n\nENVIRONMENT:\nFolder: %s\nWorking Directory: %s\nParent Directory: %s\nVerified Known Folders:\n%s\n",
		ctx.Task, ctx.FolderName, ctx.WorkingDir, ctx.ParentDir, knownFolders)

	if ctx.History != "" && ctx.History != "None" {
		fmt.Fprintf(&finalSb, "\nCONVERSATION HISTORY:\n%s\n", ctx.History)
	}

	if ctx.FailureHistory != "" && ctx.FailureHistory != "None" {
		fmt.Fprintf(&finalSb, "\nPREVIOUS FAILED REQUESTS (evidence of approaches that did not work; do not repeat them):\n%s\n", ctx.FailureHistory)
	}

	if ctx.ActiveError != "" {
		fmt.Fprintf(&finalSb, "\nCURRENT RETRY ERROR:\n%s\n", ctx.ActiveError)
	}

	finalSb.WriteString("\nCONFIRMED DISCOVERY FACTS:\n")
	finalSb.WriteString(formatFacts(facts, "No additional filesystem facts required.\n"))
	finalSb.WriteString("\nGenerate the final script only from confirmed facts. Treat unknown paths as unavailable.")

	if ctx.Objectives != "" {
		fmt.Fprintf(&finalSb, "\n\n%s", ctx.Objectives)
	}

	finalInput := []ai.Message{{
		Role:    "user",
		Content: finalSb.String(),
	}}
	resp, err := a.aiClient.GetFinalScript(finalInput)
	if err != nil {
		return nil, "AIGenerationFailed", err
	}
	if resp.ToolCall != nil {
		step := ClassifyResponse(resp)
		return &RunResult{NextStep: step}, "AIGenerationFailed", fmt.Errorf("final script generator returned a tool call")
	}

	// AI based Safety
	if !resp.IsSafe {
		return nil, "AIMarkedUnsafe", fmt.Errorf("AI marked script unsafe: %s", resp.Explanation)
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
		return nil, "RuleBasedValidationFailed", fmt.Errorf("validation failed: %s", err)
	}

	// PowerShell AST based check. Static validation still applies if the parser
	// cannot run, so an analysis failure is reported but not treated as unsafe.
	if executor.ASTSupported() {
		analysis, analysisErr := executor.AnalyzeScript(resp.Script)
		if analysisErr != nil {
			ui.PrintGray("AST analysis skipped: " + analysisErr.Error())
		} else if err := validator.ValidateAnalysis(analysis); err != nil {
			return nil, "RuleBasedValidationFailed", fmt.Errorf("AST validation failed: %s", err)
		}
	}

	return &RunResult{
		Script:      resp.Script,
		Explanation: resp.Explanation,
		NextStep:    NextStep{Type: NextStepFinalOutput},
	}, "", nil
}

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/config"
	"github.com/Adhithya-J/underroot.git/internal/knownfolders"
	"github.com/Adhithya-J/underroot.git/internal/objectives"
	"github.com/Adhithya-J/underroot.git/internal/ui"
)

type ActiveError struct {
	ErrorType string
	ErrorMsg  string
	Script    string
}

type Interaction struct {
	UserInput   string
	FinalScript string
}

// Failure records a request that exhausted every retry attempt.
type Failure struct {
	UserInput string
	Attempts  int
	ErrorType string
	ErrorMsg  string
	Script    string
	Summary   string
}

// Session maintains conversation and environment state across user queries.
type Session struct {
	Model      string
	FolderName string
	ParentDir  string
	CurrentDir string

	CurrentInput string

	RetryCount int

	ActiveError *ActiveError
	History     []Interaction
	Failures    []Failure

	KnownFolders []knownfolders.Folder
	Permissions  *Permissions
	Objectives   *objectives.Recorder
}

// to be used later
type App struct {
	Session *Session
	Agent   *agent.Agent
	Scanner *bufio.Reader
}

func (s *Session) AddInteraction(interaction Interaction) {
	s.History = append(s.History, interaction)
}

// AddFailure records a failed request with a durable summary for later prompts.
func (s *Session) AddFailure(failure Failure) {
	failure.Summary = SummarizeFailure(failure)
	s.Failures = append(s.Failures, failure)
}

// SetKnownFolders replaces the session's trusted known-folder facts.
func (s *Session) SetKnownFolders(manifest *knownfolders.Manifest) {
	if manifest == nil {
		s.KnownFolders = nil
		return
	}
	s.KnownFolders = manifest.Existing()
}

// KnownFolderFacts returns only paths verified as present for the agent.
func (s *Session) KnownFolderFacts() []agent.KnownFolder {
	facts := make([]agent.KnownFolder, 0, len(s.KnownFolders))
	for _, folder := range s.KnownFolders {
		if folder.Exists && strings.TrimSpace(folder.Path) != "" {
			facts = append(facts, agent.KnownFolder{Name: folder.Name, Path: folder.Path})
		}
	}
	return facts
}

// ObjectiveRecitation renders the persisted todo.md objective ledger.
func (s *Session) ObjectiveRecitation() string {
	if s.Objectives == nil {
		return ""
	}
	return s.Objectives.Recitation()
}

func (s *Session) SetActiveError(errorType, errorMsg, script string) {
	s.ActiveError = &ActiveError{
		ErrorType: errorType,
		ErrorMsg:  truncateText(errorMsg, 300),
		Script:    script,
	}
}

func (s *Session) ClearActiveError() {
	s.ActiveError = nil
}

func (s *Session) ResetRequestState() {
	s.ClearActiveError()
	s.RetryCount = 0
}

func UpdateWorkingDir(session *Session) {
	cwd, err := os.Getwd()
	if err != nil {
		session.FolderName = "unknown"
		session.ParentDir = "unknown"
		session.CurrentDir = "unknown"
		return
	}
	session.CurrentDir = cwd
	session.FolderName = filepath.Base(cwd)
	session.ParentDir = filepath.Dir(cwd)
}

func EstimateTokens(txt string) int {
	return utf8.RuneCountInString(txt) / 4
}

// EstimatePromptTokens estimates message text sent for an agent context.
func EstimatePromptTokens(ctx agent.Context) int {
	return EstimateTokens(
		ctx.Task + ctx.WorkingDir + ctx.ParentDir + ctx.FolderName +
			ctx.History + ctx.ActiveError + ctx.FailureHistory + ctx.Objectives +
			agent.FormatKnownFolders(ctx.KnownFolders),
	)
}

func (s *Session) TotalTokens() int {
	total := EstimateTokens(ai.FinalSystemPrompt().Content)
	for i := range s.History {
		item := &s.History[i]
		total += EstimateTokens(item.UserInput + item.FinalScript)
	}
	for i := range s.Failures {
		total += EstimateTokens(s.Failures[i].Summary)
	}
	if s.Objectives != nil {
		total += EstimateTokens(s.Objectives.Recitation())
	}
	return total
}

func GetSessionHistory(session *Session) string {
	if len(session.History) == 0 {
		return "None"
	}
	start := 0
	const maxHistoryTurns = 5
	if len(session.History) > maxHistoryTurns {
		start = len(session.History) - maxHistoryTurns
	}
	var sb strings.Builder
	for i, item := range session.History[start:] {
		fmt.Fprintf(&sb, "[%d] User: %s\n    Script: %s\n", start+i+1, item.UserInput, item.FinalScript)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// GetFailureHistory renders recent failed requests so the model can see what
// already failed instead of repeating the same approach.
func GetFailureHistory(session *Session) string {
	if len(session.Failures) == 0 {
		return "None"
	}
	start := 0
	const maxFailureTurns = 3
	if len(session.Failures) > maxFailureTurns {
		start = len(session.Failures) - maxFailureTurns
	}

	var sb strings.Builder
	for i, failure := range session.Failures[start:] {
		fmt.Fprintf(&sb, "[F%d] %s\n", start+i+1, failure.Summary)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// SummarizeFailure builds the durable evidence recorded after all retries fail.
func SummarizeFailure(failure Failure) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Request: %s\n", truncateText(failure.UserInput, 200))
	fmt.Fprintf(&sb, "Attempts: %d/%d\n", failure.Attempts, maxRetries)
	fmt.Fprintf(&sb, "Final error [%s]: %s\n", failure.ErrorType, truncateText(failure.ErrorMsg, 300))
	if trimmed := strings.TrimSpace(failure.Script); trimmed != "" {
		fmt.Fprintf(&sb, "Failed script: %s\n", truncateText(trimmed, 300))
	}
	fmt.Fprintf(&sb, "Next approach: %s", errorGuidance(failure.ErrorType))
	return sb.String()
}

func errorGuidance(errorType string) string {
	switch errorType {
	case "AIGenerationFailed":
		return "Return valid JSON matching the schema and avoid unsupported fields."
	case "AIGeneratedEmptyString":
		return "Return an executable script in the script field, never an empty string."
	case "AIMarkedUnsafe":
		return "Generate a compliant, non-destructive script or explain why none is safe."
	case "RuleBasedValidationFailed":
		return "Replace blocked commands with a safe alternative and keep the script static."
	case "DryRunFailed":
		return "Fix PowerShell syntax or unsupported parameters before execution."
	case "ExecutionFailed":
		return "Use confirmed paths and handle missing items explicitly with throw."
	case "PermissionDenied":
		return "A session permission rule blocked this action. Choose a different command that satisfies the policy, or ask the user to change allowed_tools/disallowed_tools."
	case "ToolApprovalRequired":
		return "The requested tool needs human approval. Continue with read-only facts already confirmed, or produce a final script that does not require the interrupted tool."
	default:
		return "Analyze the error and try a different, safer approach."
	}
}

func truncateText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}

func GetActiveError(session *Session) string {
	if session.ActiveError == nil {
		return ""
	}
	fixHint := errorGuidance(session.ActiveError.ErrorType)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Error Type: %s\nError Message: %s\nSuggested Fix: %s",
		session.ActiveError.ErrorType, session.ActiveError.ErrorMsg, fixHint)
	if session.ActiveError.Script != "" {
		fmt.Fprintf(&sb, "\nFailed Script: %s", session.ActiveError.Script)
	}
	return sb.String()
}

// BuildContext constructs the typed context required for an agent run.
func BuildContext(session *Session) agent.Context {
	return agent.Context{
		Task:           session.CurrentInput,
		WorkingDir:     session.CurrentDir,
		ParentDir:      session.ParentDir,
		FolderName:     session.FolderName,
		History:        GetSessionHistory(session),
		FailureHistory: GetFailureHistory(session),
		ActiveError:    GetActiveError(session),
		Objectives:     session.ObjectiveRecitation(),
		KnownFolders:   session.KnownFolderFacts(),
	}
}

func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration failed: %v", err)
	}

	provider, err := ai.NewProvider(cfg.AI)
	if err != nil {
		log.Fatalf("AI provider initialization failed: %v", err)
	}

	recorder, recErr := objectives.Default()
	if recErr != nil {
		recorder = nil
	}

	model := cfg.AI.OpenAIModel
	if cfg.AI.UseMock {
		model = "mock"
	}

	return &App{
		Session: &Session{
			Model:       model,
			Permissions: NewPermissions(),
			Objectives:  recorder,
		},
		Agent:   agent.NewAgent(provider),
		Scanner: bufio.NewReader(os.Stdin),
	}
}

// LoadKnownFolders loads the verified folder manifest into the session. When
// force is true it always re-resolves paths (used by the /init command).
func (a *App) LoadKnownFolders(force bool) {
	var (
		manifest *knownfolders.Manifest
		err      error
	)
	if force {
		manifest, err = knownfolders.Initialize()
	} else {
		manifest, err = knownfolders.LoadOrInitialize()
	}
	if err != nil {
		ui.PrintGray("Known folders unavailable: " + err.Error() + ". Run /init to retry.")
		return
	}

	a.Session.SetKnownFolders(manifest)
	ui.PrintGray(fmt.Sprintf("Known folders ready: %d verified", len(manifest.Existing())))
	if force {
		for _, folder := range manifest.Folders {
			ui.PrintKnownFolder(folder.Name, folder.Path, folder.Exists)
		}
	}
}

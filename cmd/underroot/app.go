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
	"github.com/Adhithya-J/underroot.git/internal/knownfolders"
	"github.com/Adhithya-J/underroot.git/internal/ui"
	"github.com/joho/godotenv"
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
			ctx.History + ctx.ActiveError + ctx.FailureHistory +
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
		KnownFolders:   session.KnownFolderFacts(),
	}
}

func GetClient() *ai.Client {
	if err := godotenv.Load(); err != nil {
		fmt.Println(".env file not found")
	}

	cfg := ai.Config{
		OpenAIBaseURL: os.Getenv("OpenAIBaseURL"),
		OpenAIAPIKey:  os.Getenv("OpenAIAPIKey"),
		OpenAIModel:   os.Getenv("OpenAIModel"),
		UseMock:       false,
	}

	client, err := ai.NewClient(cfg)
	if err != nil {
		log.Fatalf("OpenAI Client initialization failed: %v", err)
	}
	return client
}

func NewApp() *App {
	client := GetClient()

	return &App{
		Session: &Session{
			Model:       os.Getenv("OpenAIModel"),
			Permissions: NewPermissions(),
		},
		Agent:   agent.NewAgent(client),
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

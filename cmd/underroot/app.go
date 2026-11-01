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

func (s *Session) SetActiveError(errorType, errorMsg, script string) {
	cleanMsg := strings.TrimSpace(errorMsg)
	if len(cleanMsg) > 300 {
		cleanMsg = cleanMsg[:300] + "..."
	}
	s.ActiveError = &ActiveError{
		ErrorType: errorType,
		ErrorMsg:  cleanMsg,
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
	return EstimateTokens(ctx.Task + ctx.WorkingDir + ctx.ParentDir + ctx.FolderName + ctx.History + ctx.ActiveError)
}

func (s *Session) TotalTokens() int {
	total := EstimateTokens(ai.FinalSystemPrompt().Content)
	for i := range s.History {
		item := &s.History[i]
		total += EstimateTokens(item.UserInput + item.FinalScript)
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
		sb.WriteString(fmt.Sprintf("[%d] User: %s\n    Script: %s\n", start+i+1, item.UserInput, item.FinalScript))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func GetActiveError(session *Session) string {
	if session.ActiveError == nil {
		return ""
	}
	var fixHint string
	switch session.ActiveError.ErrorType {
	case "AIGenerationFailed":
		fixHint = "Return valid JSON matching the schema."
	case "AIGeneratedEmptyString":
		fixHint = "Ensure response generation always returns an executable script."
	case "AIMarkedUnsafe":
		fixHint = "Generate compliant and safe output."
	case "RuleBasedValidationFailed":
		fixHint = "Fix output so it satisfies validation rules."
	case "DryRunFailed", "ExecutionFailed":
		fixHint = "Fix PowerShell script syntax/runtime errors."
	default:
		fixHint = "Analyze and fix the error."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Error Type: %s\nError Message: %s\nSuggested Fix: %s",
		session.ActiveError.ErrorType, session.ActiveError.ErrorMsg, fixHint))
	if session.ActiveError.Script != "" {
		sb.WriteString(fmt.Sprintf("\nFailed Script: %s", session.ActiveError.Script))
	}
	return sb.String()
}

// BuildContext constructs the typed context required for an agent run.
func BuildContext(session *Session) agent.Context {
	return agent.Context{
		Task:        session.CurrentInput,
		WorkingDir:  session.CurrentDir,
		ParentDir:   session.ParentDir,
		FolderName:  session.FolderName,
		History:     GetSessionHistory(session),
		ActiveError: GetActiveError(session),
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
			Model: os.Getenv("OpenAIModel"),
		},
		Agent:   agent.NewAgent(client),
		Scanner: bufio.NewReader(os.Stdin),
	}
}

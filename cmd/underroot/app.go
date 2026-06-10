package main

import (
	"bufio"
	"encoding/json"
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

type AgentError struct {
	ErrorType string `json:"error_type"`
	ErrorMsg  string `json:"error_msg"`
	Script    string `json:"script"`
	Output    string `json:"output"`
}

type Interaction struct {
	UserInput   string `json:"user_input"`
	Explanation string `json:"explanation"`
	Script      string `json:"script"`
	Output      string `json:"output"`
	// this should also have a flag to identify requests that have failed after 'n' attempts
	// if it failed, summary of the 'n' attempts should be added for better context!
}

// Introduce session state
type Session struct {
	Model        string
	FolderName   string
	ParentDir    string
	CurrentDir   string
	DirContent   string
	CurrentInput string

	LastScript      string
	LastExplanation string
	LastOutput      string

	RetryCount int

	ErrorHistory []AgentError
	History      []Interaction

	// you might also want to manage session level execution permission within the this
	// apart from a global execution permissions
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

func (s *Session) AddErrorHistory(error AgentError) {
	s.ErrorHistory = append(s.ErrorHistory, error)
}

func (s *Session) SetLastResponse(script string, explanation string) {
	s.LastScript = script
	s.LastExplanation = explanation
}

func (s *Session) ResetRequestState() {
	s.ErrorHistory = nil
	s.RetryCount = 0
}

func (s *Session) SetOutput(psout string) {
	s.LastOutput = psout
}

func UpdateWorkingDir(session *Session) {
	cwd, err := os.Getwd()
	if err != nil {
		// ui.PrintError("Failed to get cwd", err)
		session.FolderName = "unknown"
		session.ParentDir = "unknown"
		session.CurrentDir = "unknown"
		return
	}
	session.CurrentDir = cwd
	session.FolderName = filepath.Base(cwd)
	session.ParentDir = filepath.Dir(cwd)

	files, err := os.ReadDir(cwd)
	if err != nil {
		session.DirContent = "Error reading directory content"
		return
	}
	const max_files = 10
	var content []string
	for _, f := range files[:max_files] {
		prefix := "[File]"
		if f.IsDir() {
			prefix = "[Dir]"

		}
		content = append(content, fmt.Sprintf("%s %s", prefix, f.Name()))
	}
	session.DirContent = strings.Join(content, ", ")
}

func EstimateTokens(txt string) int {
	return utf8.RuneCountInString(txt) / 4
}

func (s *Session) TotalTokens() int {
	// add system prompt
	// for now adding 250 tokens for that
	systemPromptTokens := 250 // to be updated with actual count
	total := EstimateTokens(s.CurrentInput) + systemPromptTokens
	for _, item := range s.History {
		total += EstimateTokens(item.UserInput + item.Script + item.Explanation + item.Output)
	}
	return total

}

func toJson(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(out)
}

func GetCurrentDirInfo(session *Session) string {
	return fmt.Sprintf("Current Folder Name: %s\nCurrent Working Directory: %s\nCurrent Directory Contents: %s\n", session.FolderName, session.CurrentDir, session.DirContent)
}

func GetSessionHistory(session *Session) string {
	// convo history should not include explaination when being fed into prompt
	// tempSessionHistory := ""

	return toJson(session.History)
}

func GetLatestError(session *Session) string {
	// this should build error resoultion strategy (for now fixed) for llm based on error type
	n := len(session.ErrorHistory)

	// add gating so json parsing does not fail

	// current error is last error in error history
	if n <= 0 {
		return "Current Error: None"
	}

	var errorCorrection string
	currentError := session.ErrorHistory[n-1]
	jsonCEOut := toJson(currentError)
	switch currentError.ErrorType {
	case "AIGenerationFailed":
		errorCorrection = "Regenerate output with stricter validation."
	case "AIGeneratedEmptyString":
		errorCorrection = "Ensure response generation always returns content."
	case "AIMarkedUnsafe":
		errorCorrection = "Generate compliant and safe output."
	case "RuleBasedValidationFailed":
		errorCorrection = "Fix output so it satisfies validation rules."
	case "ExectionFailed":
		errorCorrection = "Fix execution/runtime issues."
	default:
		errorCorrection = "Analyze and fix the error."
	}

	return fmt.Sprintf("Current Error: %s\nSuggested Approach to solve it: %s\n", jsonCEOut, errorCorrection)
}

func GetPastErrorHistory(session *Session) string {
	n := len(session.ErrorHistory)

	// add gating so json parsing does not fail

	if n <= 1 {
		return "Previous Errors: None"
	}

	jsonEHOut := toJson(session.ErrorHistory[:n-1])

	return fmt.Sprintf("History of previous Errors: %s", string(jsonEHOut))
}

func BuildPrompt(session *Session) ([]ai.Message, error) { // message 0 should be system so we can make sure it is not lost. message n is last user request

	var prompt []ai.Message

	// iterate though the jsonEHOut and keep the latest error and mark it as current error
	// inject appropriate error fixing strategy based on error type for that
	currentErrorString := GetLatestError(session) // latest error with suggest fix

	// the rest of the errors (if present) can be added as it is
	historyOfErrorsString := GetPastErrorHistory(session) // past errors excluding latest one

	// current dir, parent dir, current folder name, directory contents are added to prompt
	// add to last user message
	currentDirInfo := GetCurrentDirInfo(session)

	// session history includes previous conversations - which inclues user input, explaination of generated script, script, ps output
	sessionHistory := GetSessionHistory(session)

	prompt = append(prompt, ai.Message{
		Role: "user",
		Content: fmt.Sprintf(`TASK
	%s\n\tENVIRONMENT
	%s\n\tCURRENT ERROR
	%s\n\tPREVIOUS ERROR PATTERNS
	%s\n\tCONVERSATION HISTORY
	%s`, session.CurrentInput, currentDirInfo, currentErrorString, historyOfErrorsString, sessionHistory),
	})
	return prompt, nil
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

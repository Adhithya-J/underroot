package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
		return
	}
	session.FolderName = filepath.Base(cwd)
	session.ParentDir = filepath.Dir(cwd)

}

func EstimateTokens(txt string) int {
	return utf8.RuneCountInString(txt) / 4
}

func (s *Session) TotalTokens() int {
	total := EstimateTokens(s.CurrentInput)
	for _, item := range s.History {
		total += EstimateTokens(item.UserInput + item.Script + item.Explanation + item.Output)
	}
	return total

}

func BuildPrompt(session *Session) (string, error) {
	// convo history should not include explaination when being fed into prompt
	jsonHOut, jsonHErr := json.Marshal(session.History)
	if jsonHErr != nil {
		jsonHOut = nil
	}

	n := len(session.ErrorHistory)

	// add gating so json parsing does not fail
	var (
		jsonCEOut []byte = nil
		jsonCEErr error
		jsonEHOut []byte = nil
		jsonEHErr error
	)

	// current error is last error in error history
	if n > 0 {
		currentError := session.ErrorHistory[n-1]
		jsonCEOut, jsonCEErr = json.Marshal(currentError)
		if jsonCEErr != nil {
			jsonCEOut = nil
		}
	}

	// add gating so json parsing does not fail
	if n > 1 {
		jsonEHOut, jsonEHErr = json.Marshal(session.ErrorHistory[:n-1])
		if jsonEHErr != nil {
			jsonEHOut = nil
		}
	}

	// iterate though the jsonEHOut and keep the latest error and mark it as current error
	// inject appropriate error fixing strategy based on error type for that
	// the rest of the errors (if present) can be added as it is

	prompt := fmt.Sprintf("User request: %s\nCurrent Error: %s\nPrevious Errors: %s\nPrevious Conversation History: %s", session.CurrentInput, string(jsonCEOut), string(jsonEHOut), string(jsonHOut))

	if jsonHErr != nil {
		return prompt, jsonHErr
	}

	if jsonCEErr != nil {
		return prompt, jsonCEErr
	}

	if jsonEHErr != nil {
		return prompt, jsonEHErr
	}

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

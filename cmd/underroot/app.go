package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
	return len(txt) / 4
}

func (s *Session) TotalTokens() int {
	total := EstimateTokens(s.CurrentInput)
	for _, item := range s.History {
		total += EstimateTokens(item.UserInput + item.Script + item.Explanation + item.Output)
	}
	return total

}

func BuildPrompt(session *Session) (string, error) {
	jsonOut, jsonErr := json.Marshal(session.ErrorHistory)
	if jsonErr != nil {
		jsonOut = nil
	}
	return fmt.Sprintf("User request: %s\nPrevious Errors: %s", session.CurrentInput, string(jsonOut)), jsonErr
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

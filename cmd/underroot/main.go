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
	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/ui"

	"github.com/joho/godotenv"
)

const (
	maxRetries = 3
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

	History []Interaction
}

// to be used later
type App struct {
	Session *Session
	Agent   *agent.Agent
}

func (s *Session) AddInteraction(interaction Interaction) {
	s.History = append(s.History, interaction)

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

func BuildPrompt(txt string, errors []AgentError) (string, error) {
	jsonOut, jsonErr := json.Marshal(errors)
	if jsonErr != nil {
		jsonOut = nil
	}
	return fmt.Sprintf("User request: %s\nPrevious Errors: %s", txt, string(jsonOut)), jsonErr
}

func main() {

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
	a := agent.NewAgent(client)

	scanner := bufio.NewReader(os.Stdin)
	session := &Session{
		Model: cfg.OpenAIModel,
	}

	ui.PrintBanner()

	for {
		// print cwd and model-name
		UpdateWorkingDir(session)
		ui.PrintPromptBar(session.ParentDir, session.FolderName, session.Model)

		txt, err := ui.ReadInput(scanner)

		if err != nil {
			ui.PrintError("User input read failed", err)
			continue
		}

		if txt == "" {
			continue
		}

		if ui.ShouldExit(txt) {
			break
		}

		var errorHistory []AgentError
		for i := 0; i < maxRetries; i++ {
			session.RetryCount = i
			ui.PrintRetry(session.RetryCount, maxRetries)

			input, err := BuildPrompt(txt, errorHistory)
			if err != nil {
				ui.PrintError("Error parsing json", err)
			}
			ui.PrintGray("Generating response...")
			response, runErr := a.Run(input)
			if runErr != nil {
				ui.PrintError("Agent run failed", runErr)
				errorHistory = append(errorHistory, AgentError{
					ErrorType: "",
					ErrorMsg:  runErr.Error(),
					Script:    "",
					Output:    "",
				})
				continue
			}
			ui.PrintLine()
			session.LastScript = response.Script
			session.LastExplanation = response.Explanation
			ui.PrintScript(session.LastScript)
			ui.PrintExplanation(session.LastExplanation)
			ui.PrintLine()

			permitted, err := ui.AskForApproval(scanner)
			if err != nil {
				ui.PrintError("Skipping execution....\nEncountering error", err)
				break
			}
			if permitted {
				ui.PrintGray("Executing Script....")
				psOut, err := executor.ExecuteScript(response.Script)
				if err != nil {
					ui.PrintError("Execution Error", err)
					errorHistory = append(errorHistory, AgentError{
						ErrorType: "",
						ErrorMsg:  err.Error(),
						Script:    response.Script,
						Output:    psOut,
					})
					continue
				}
				session.LastOutput = psOut

				interaction := Interaction{
					UserInput:   txt,
					Explanation: response.Explanation,
					Script:      response.Script,
					Output:      psOut,
				}
				session.AddInteraction(interaction)

				ui.PrintOutput(session.LastOutput)
				fmt.Println("Success!")
				fmt.Printf("History size: %d\n", len(session.History))
				ui.PrintLine()

				break
			} else {
				ui.PrintGray("Skipping execution")
				ui.PrintLine()
				break

			}
		}
	}

	ui.PrintLine()

}

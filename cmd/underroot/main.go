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

	ui.PrintBanner()

	for {
		// print cwd and model-name
		cwd, err := os.Getwd()
		if err != nil {
			ui.PrintError("Failed to get cwd", err)
			cwd = "unknown"
		}
		folderName := filepath.Base(cwd)
		parentPath := filepath.Dir(cwd)
		ui.PrintPromptBar(parentPath, folderName, cfg.OpenAIModel)

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
			ui.PrintRetry(i, maxRetries)
			jsonOut, jsonErr := json.Marshal(errorHistory)
			if jsonErr != nil {
				ui.PrintError("Error parsing json", jsonErr)
				continue
			}

			input := fmt.Sprintf("User request: %s\nPrevious Errors: %s", txt, string(jsonOut))
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
			ui.PrintScript(response.Script)
			ui.PrintExplanation(response.Explanation)
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
				ui.PrintOutput(psOut)
				fmt.Println("Success!")
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

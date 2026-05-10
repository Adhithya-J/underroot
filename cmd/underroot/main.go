package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/ui"

	"github.com/joho/godotenv"
)

const (
	Gray       = "\033[90m"
	Reset      = "\033[0m"
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
		}
		ui.PrintPromptBar(cwd, cfg.OpenAIModel)

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
			jsonOut, jsonErr := json.Marshal(errorHistory)
			if jsonErr != nil {
				ui.PrintError("Error parsing json", jsonErr)
				continue
			}

			input := fmt.Sprintf("User request: %s\nPrevious Errors: %s", txt, string(jsonOut))
			Response, runErr := a.Run(input)
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
			ui.PrintSeparator()
			fmt.Print(Response.Script + "\n")
			fmt.Print(Response.Explanation + "\n")
			ui.PrintSeparator()

			permitted, err := ui.AskForApproval(scanner)
			if err != nil {
				ui.PrintError("Skipping execution....\nEncountering error", err)
			}
			if permitted {
				fmt.Printf("\x1b[90m Executing Script....\033[0m\n")
				psOut, err := executor.ExecuteScript(Response.Script)
				if err != nil {
					fmt.Println(err)
					errorHistory = append(errorHistory, AgentError{
						ErrorType: "",
						ErrorMsg:  err.Error(),
						Script:    Response.Script,
						Output:    psOut,
					})
					continue
				}
				fmt.Printf("Output:\n%s\n", psOut)
				fmt.Println("Success!")
				break
			} else {
				fmt.Println("Skipping execution")
				break

			}
		}
	}

	ui.PrintSeparator()

}

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
			log.Printf("failed to get cwd: %v", err)
		}
		ui.PrintPromptBar(cwd, cfg.OpenAIModel)

		txt, err := ui.ReadInput(scanner)

		if err != nil {
			panic(err)
		}

		if ui.ShouldExit(txt) {
			break
		}
		var ErrorHistory []AgentError
		for i := 0; i < maxRetries; i++ {
			jsonOut, jsonErr := json.Marshal(ErrorHistory)
			if jsonErr != nil {
				fmt.Println("Error parsing json")
				continue
			}

			input := txt + string(jsonOut)
			Response, runErr := a.Run(input)
			if runErr != nil {
				log.Printf("Agent run failed: %v", runErr)
				ErrorHistory = append(ErrorHistory, AgentError{
					ErrorType: "",
					ErrorMsg:  err.Error(),
					Script:    "",
					Output:    "",
				})
				continue
			}
			ui.PrintSeparator()
			fmt.Print(Response.Script)
			fmt.Print(Response.Explanation)
			ui.PrintSeparator()

			permitted := ui.AskForApproval(scanner)
			if permitted {
				fmt.Printf("\x1b[90m Executing Script....\033[0m\n")
				psOut, err := executor.ExecuteScript(Response.Script)
				if err != nil {
					fmt.Println(err)
					ErrorHistory = append(ErrorHistory, AgentError{
						ErrorType: "",
						ErrorMsg:  err.Error(),
						Script:    Response.Script,
						Output:    psOut,
					})
					continue
				}
				fmt.Printf("Output:\n%s\n", psOut)
				fmt.Println("Success!")
			} else {
				fmt.Println("Skipping execution")

			}
		}
	}

	ui.PrintSeparator()

}

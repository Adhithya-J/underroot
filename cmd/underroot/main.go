package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/ui"

	"github.com/joho/godotenv"
)

const (
	Gray  = "\033[90m"
	Reset = "\033[0m"
)

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

		input := txt
		runErr := a.Run(input)
		if runErr != nil {
			log.Printf("Agent run failed: %v", runErr)
			continue
		}
		ui.PrintSeparator()

	}

}

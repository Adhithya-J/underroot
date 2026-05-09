package main

import (
	"log"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/ai"
)

func main() {
	cfg := ai.Config{
		OpenAIBaseURL: "http://localhost:8080/",
		OpenAIAPIKey:  "None",
		Model:         "not-needed",
		UseMock:       false,
	}

	client, err := ai.NewClient(cfg)
	if err != nil {
		log.Fatalf("OpenAI Client initialization failed: %v", err)
	}
	a := agent.NewAgent(client)

	input := "echo 'Hello from Underroot', Invoke-Expression"
	runErr := a.Run(input)
	if err != nil {
		log.Fatalf("Agent run failed: %v", runErr)
	}
}

package main

import (
	"log"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/ai"
)

func main() {
	cfg := ai.Config{
		UseMock: true,
	}

	client := ai.NewClient(cfg)
	a := agent.NewAgent(client)

	input := "echo 'Hello from Underroot', Invoke-Expression"
	err := a.Run(input)
	if err != nil {
		log.Fatalf("Agent run failed: %v", err)
	}
}

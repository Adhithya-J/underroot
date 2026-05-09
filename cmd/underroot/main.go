package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

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
	fmt.Println("--------------------")
	fmt.Println("\tUnderroot")
	fmt.Println("--------------------")
	fmt.Println("\x1b[90mHit enter or type quit or exit to close the application\033[0m")
	fmt.Println("--------------------")
	for {
		scanner := bufio.NewReader(os.Stdin)
		fmt.Print("> ")
		txt, err := scanner.ReadString('\n') //single quotes!
		if err != nil {
			panic(err)
		}
		txt = strings.TrimSpace(txt)
		lowerTxt := strings.ToLower(txt)

		if lowerTxt == "" || lowerTxt == "q" || lowerTxt == "quit" || lowerTxt == "exit" {
			break
		}
		input := txt
		runErr := a.Run(input)
		if runErr != nil {
			log.Fatalf("Agent run failed: %v", runErr)
		}
		fmt.Println("--------------------")

	}

}

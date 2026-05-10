package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/ai"

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

	printBanner()

	for {
		// print cwd and model-name
		cwd, err := os.Getwd()
		if err != nil {
			log.Printf("failed to get cwd: %v", err)
		}
		printPromptBar(cwd, cfg.OpenAIModel)

		txt, err := readInput(scanner)

		if err != nil {
			panic(err)
		}

		if shouldExit(txt) {
			break
		}

		input := txt
		runErr := a.Run(input)
		if runErr != nil {
			log.Printf("Agent run failed: %v", runErr)
			continue
		}
		printSeparator()

	}

}

func readInput(reader *bufio.Reader) (string, error) {
	fmt.Print("> ")
	txt, err := reader.ReadString('\n') //single quotes!
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(txt), err

}

func printBanner() {
	printSeparator()
	fmt.Println("\tUnderroot")
	printSeparator()
	fmt.Println(Gray + "Hit enter or type quit or exit to close the application" + Reset)
	printSeparator()

}

func printSeparator() {
	fmt.Println(Gray + "--------------------" + Reset)
}

func printPromptBar(cwd string, model string) {
	fmt.Printf(Gray+"%s | %s\n"+Reset, cwd, model)
}

func shouldExit(input string) bool {
	result := false
	txt := strings.TrimSpace(input)
	lowerTxt := strings.ToLower(txt)

	if lowerTxt == "" || lowerTxt == "q" || lowerTxt == "quit" || lowerTxt == "exit" {
		result = true
	}
	return result

}

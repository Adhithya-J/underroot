package ui

import (
	"bufio"
	"fmt"
	"strings"
)

const (
	Gray  = "\033[90m"
	Reset = "\033[0m"
)

func ReadInput(reader *bufio.Reader) (string, error) {
	fmt.Print("> ")
	txt, err := reader.ReadString('\n') //single quotes!
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(txt), err

}

func PrintBanner() {
	PrintSeparator()
	fmt.Println("\tUnderroot")
	PrintSeparator()
	fmt.Println(Gray + "Hit enter or type quit or exit to close the application" + Reset)
	PrintSeparator()

}

func PrintSeparator() {
	fmt.Println(Gray + "--------------------" + Reset)
}

func PrintPromptBar(cwd string, model string) {
	fmt.Printf(Gray+"%s | %s\n"+Reset, cwd, model)
}

func ShouldExit(input string) bool {
	result := false
	txt := strings.TrimSpace(input)
	lowerTxt := strings.ToLower(txt)

	if lowerTxt == "" || lowerTxt == "q" || lowerTxt == "quit" || lowerTxt == "exit" {
		result = true
	}
	return result

}

package ui

import (
	"bufio"
	"fmt"
	"strings"
)

const (
	Gray  = "\033[90m"
	Reset = "\033[0m"
	Blue  = "\033[34m"
	Red   = "\033[31m"
)

func ReadInput(reader *bufio.Reader) (string, error) {
	fmt.Print("> ")
	txt, err := reader.ReadString('\n') // rune
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(txt), err

}

func AskForApproval(reader *bufio.Reader) (bool, error) {
	fmt.Printf("%sDo you want to execute the script (Y/n): %s", Blue, Reset)
	txt, err := reader.ReadString('\n') //single quotes!
	if err != nil {
		return false, err
	}
	txt = strings.TrimSpace(txt)
	lowerTxt := strings.ToLower(txt)
	return lowerTxt == "y" || lowerTxt == "", nil
}

func PrintBanner() {
	PrintSeparator()
	fmt.Println("\tUnderroot")
	PrintSeparator()
	fmt.Printf("%sHit enter or type quit or exit to close the application%s\n", Gray, Reset)
	PrintSeparator()

}

func PrintSeparator() {
	fmt.Printf("%s--------------------%s\n", Gray, Reset)
}

func PrintRetry(i int, maxRetries int) {
	fmt.Printf("%s[Retry %d/%d]%s\n", Gray, i+1, maxRetries, Reset)
}

func PrintError(txt string, err error) {
	fmt.Printf("%s[Execution Error]%s %s\t %s\n", Red, Reset, err, txt)

}

func PrintPromptBar(cwd string, model string) {
	fmt.Printf("%s%s | %s%s\n", Gray, cwd, model, Reset)
}

func ShouldExit(input string) bool {
	txt := strings.TrimSpace(input)
	lowerTxt := strings.ToLower(txt)
	return lowerTxt == "q" || lowerTxt == "quit" || lowerTxt == "exit"

}

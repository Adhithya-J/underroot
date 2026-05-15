package ui

import (
	"bufio"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	Reset = "\033[0m"

	Gray   = "\033[38;5;245m"
	Blue   = "\033[38;5;75m"
	Green  = "\033[38;5;82m"
	Red    = "\033[38;5;196m"
	Yellow = "\033[38;5;220m"

	Bold = "\033[1m"
)

func terminalWidth() int {
	w, _, err := term.GetSize(0)
	if err != nil {
		return 50
	}
	return w
}

func ReadInput(reader *bufio.Reader) (string, error) {
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
	PrintLine()
	fmt.Printf("%s%sUNDERROOT%s%s - AI Shell Agent%s\n", Bold, Blue, Reset, Gray, Reset)
	PrintLine()
	PrintGray("Press Enter to continue")
	PrintGray("Type 'exit' or 'quit' to close")
	PrintLine()

}

func PrintLine() {
	width := terminalWidth()
	fmt.Printf("%s%s%s\n", Gray, strings.Repeat("─", width-2), Reset)
}

func PrintRetry(i int, maxRetries int) {
	fmt.Printf("%s[Retry %d/%d]%s\n", Gray, i+1, maxRetries, Reset)
}

func PrintScript(script string) {
	fmt.Printf("%s[Script]%s %s\n", Green, Reset, script)
}

func PrintGray(input string) {
	fmt.Printf("%s%s%s\n", Gray, input, Reset)
}

func PrintError(txt string, err error) {
	fmt.Printf("%s[Execution Error]%s %s\t %s\n", Red, Reset, err, txt)

}

func PrintExplanation(explanation string) {
	fmt.Printf("\n%sExplanation: %s%s%s\n", Blue, Gray, explanation, Reset)

}

func PrintOutput(output string) {

	termW := terminalWidth()

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	maxWidth := 0
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		w := utf8.RuneCountInString(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	contentWidth := maxWidth + 2
	if termW > contentWidth {
		contentWidth = maxWidth - 2
	}
	// borderWidth := max(termW-2, maxWidth+2)

	if contentWidth < 10 {
		contentWidth = 10
	}

	PrintLine()
	PrintGray("Output")

	border := strings.Repeat("─", contentWidth)
	fmt.Printf("%s┌%s┐%s\n", Blue, border, Reset)

	for _, line := range lines {
		lineWidth := utf8.RuneCountInString(line)
		padding := contentWidth - lineWidth - 1

		if padding < 0 {
			padding = 0
		}

		fmt.Printf("%s│%s %s%s%s│%s\n", Blue, Reset, line, strings.Repeat(" ", padding), Blue, Reset)
	}
	fmt.Printf("%s└%s┘%s\n", Blue, border, Reset)
}

func PrintPromptBar(parent string, folder string, model string, tokens int) {
	fmt.Printf("%s%s%s · %s%s%s\n", Blue, folder, Gray, Green, model, Reset)
	fmt.Printf("%s%s · %d/4096 tokens%s\n", Gray, parent, tokens, Reset)
	PrintLine()
	fmt.Print("❯ ")

}

func ShouldExit(input string) bool {
	txt := strings.TrimSpace(input)
	lowerTxt := strings.ToLower(txt)
	return lowerTxt == "q" || lowerTxt == "quit" || lowerTxt == "exit"

}

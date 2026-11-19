package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	// Reset resets terminal formatting.
	Reset = "\033[0m"

	// Gray colors terminal text gray.
	Gray = "\033[38;5;245m"
	// Blue colors terminal text blue.
	Blue = "\033[38;5;75m"
	// Green colors terminal text green.
	Green = "\033[38;5;82m"
	// Red colors terminal text red.
	Red = "\033[38;5;196m"
	// Yellow colors terminal text yellow.
	Yellow = "\033[38;5;220m"

	// Bold applies bold terminal formatting.
	Bold = "\033[1m"
)

func terminalWidth() int {
	w, _, err := term.GetSize(0)
	if err != nil {
		return 50
	}
	return w
}

// ReadInput reads and trims one line of user input.
func ReadInput(reader *bufio.Reader) (string, error) {
	txt, err := reader.ReadString('\n') // rune
	if err != nil && err != io.EOF {
		return "", err
	}
	if strings.TrimSpace(txt) == "" && err == io.EOF {
		return "", io.EOF
	}
	return strings.TrimSpace(txt), nil
}

// Approval is the user's response to an execution approval prompt.
type Approval int

const (
	// ApprovalSkip means do not execute the script.
	ApprovalSkip Approval = iota
	// ApprovalOnce means execute this script once.
	ApprovalOnce
	// ApprovalAlways means execute and remember this script for the session.
	ApprovalAlways
	// ApprovalNever means block and remember this script for the session.
	ApprovalNever
)

// AskForApproval reads whether the user approves script execution.
func AskForApproval(reader *bufio.Reader) (Approval, error) {
	fmt.Printf("%sDo you want to execute the script? (Y/n/a=always/d=never): %s", Blue, Reset)
	txt, err := reader.ReadString('\n') // single quotes!
	if err != nil && err != io.EOF {
		return ApprovalSkip, err
	}

	switch strings.ToLower(strings.TrimSpace(txt)) {
	case "", "y", "yes":
		return ApprovalOnce, nil
	case "a", "always":
		return ApprovalAlways, nil
	case "d", "never":
		return ApprovalNever, nil
	default:
		return ApprovalSkip, nil
	}
}

// PrintBanner prints the application welcome banner.
func PrintBanner() {
	PrintLine()
	fmt.Printf("%s%sUNDERROOT%s%s - AI Shell Agent%s\n", Bold, Blue, Reset, Gray, Reset)
	PrintLine()
	PrintGray("Press Enter to continue")
	PrintGray("Type /init to resolve standard folders, /help for commands")
	PrintGray("Type 'exit' or 'quit' to close")
	PrintLine()
}

// PrintLine prints a separator line sized to the terminal.
func PrintLine() {
	width := terminalWidth()
	fmt.Printf("%s%s%s\n", Gray, strings.Repeat("─", width-2), Reset)
}

// PrintRetry prints the current retry count.
func PrintRetry(i, maxRetries int) {
	fmt.Printf("%s[Retry %d/%d]%s\n", Gray, i+1, maxRetries, Reset)
}

// PrintScript prints a generated script.
func PrintScript(script string) {
	fmt.Printf("%s[Script]%s %s\n", Green, Reset, script)
}

// PrintGray prints text using the gray terminal color.
func PrintGray(input string) {
	fmt.Printf("%s%s%s\n", Gray, input, Reset)
}

// PrintError prints an execution error and its associated text.
func PrintError(txt string, err error) {
	fmt.Printf("%s[Execution Error]%s %s\t %s\n", Red, Reset, err, txt)
}

// PrintExplanation prints the explanation for a generated script.
func PrintExplanation(explanation string) {
	fmt.Printf("\n%sExplanation: %s%s%s\n", Blue, Gray, explanation, Reset)
}

// PrintOutput prints command output in a bordered block.
func PrintOutput(output string) {
	termW := terminalWidth()

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	maxWidth := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		w := utf8.RuneCountInString(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	contentWidth := maxWidth + 2
	if termW > 2 && contentWidth > termW-2 {
		contentWidth = termW - 2
	}
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

// PrintPromptBar prints the current directory, model, and token count.
func PrintPromptBar(parent, folder, model string, tokens int) {
	fmt.Printf("%s%s%s · %s%s%s\n", Blue, folder, Gray, Green, model, Reset)
	fmt.Printf("%s%s · %d/4096 tokens%s\n", Gray, parent, tokens, Reset)
	PrintLine()
	fmt.Print("❯ ")
}

// PrintKnownFolder prints one resolved known folder and its status.
func PrintKnownFolder(name, path string, exists bool) {
	status := Green + "available" + Reset
	if !exists {
		status = Yellow + "unavailable" + Reset
	}
	if strings.TrimSpace(path) == "" {
		path = "(not resolved)"
	}
	fmt.Printf("%s- %s:%s %s (%s)\n", Gray, name, Reset, path, status)
}

// PrintPermissions shows the session permission mode and remembered decisions.
func PrintPermissions(mode string, allowed, denied int, allowedRules, disallowedRules []string) {
	fmt.Printf("%sSession permissions%s mode=%s%s%s allowed=%d denied=%d\n",
		Blue, Reset, Green, mode, Reset, allowed, denied)
	printRules("allowed_tools", allowedRules)
	printRules("disallowed_tools", disallowedRules)
}

func printRules(label string, rules []string) {
	if len(rules) == 0 {
		fmt.Printf("%s  %s:%s (none)\n", Gray, label, Reset)
		return
	}
	fmt.Printf("%s  %s:%s\n", Gray, label, Reset)
	for _, rule := range rules {
		fmt.Printf("%s    - %s%s\n", Gray, rule, Reset)
	}
}

// PrintTodo shows the active objective recitation.
func PrintTodo(recitation string) {
	PrintGray("Objectives:")
	PrintGray(recitation)
}

// PrintFailures shows durable failure evidence from the session.
func PrintFailures(failures string) {
	PrintGray("Durable failure evidence:")
	PrintGray(failures)
}

// PrintHelp lists the interactive shell commands.
func PrintHelp() {
	PrintGray("Commands:")
	PrintGray("  /init                  Resolve and save your standard folders")
	PrintGray("  /permissions [mode]    Show or set ask|auto|deny")
	PrintGray("  /permissions allow P   Auto-approve commands matching P")
	PrintGray("  /permissions disallow P Block commands matching P (overrides allow)")
	PrintGray("  /permissions rules     Show allowed_tools and disallowed_tools")
	PrintGray("  /permissions reset     Forget remembered decisions and rules")
	PrintGray("  /todo                  Show the active objective ledger")
	PrintGray("  /failures              Show durable failure evidence")
	PrintGray("  /help                  Show this help")
	PrintGray("  /exit                  Close Underroot")
}

// ShouldExit reports whether input requests that the application exit.
func ShouldExit(input string) bool {
	txt := strings.TrimSpace(input)
	lowerTxt := strings.ToLower(txt)
	return lowerTxt == "q" || lowerTxt == "quit" || lowerTxt == "exit"
}

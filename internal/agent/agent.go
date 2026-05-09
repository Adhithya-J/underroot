package agent

import (
	"fmt"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/powershell"
	"github.com/Adhithya-J/underroot.git/internal/validator"
)

const maxRetries = 3

type Agent struct {
	aiClient *ai.Client
}

// there are few types of errors - validation error (misspellings), safety error(blacklisted keywords), ai generation error (malformed script) and execution error (logic, permissions).
// The agent should know which type of error it is dealing with and take action appropriately. Also the structure would be different
// For the initial version, let's build a basic version of error type (unified)

// Also should entire error history be included?
// A naive approach would be to use the entire error history
// But I think when we run into an execution error, we can be sure that the script has passed the safety and validation error and those errors can be removed from error history

type AgentError struct {
	errorType string
	errorMsg  string
	script    string
	output    string
}

func NewAgent(aiClient *ai.Client) *Agent {
	return &Agent{
		aiClient: aiClient,
	}
}

func (a *Agent) Run(input string) error {

	var ErrorHistory []AgentError

	var history strings.Builder

	history.WriteString("User Request\n")
	history.WriteString(input)
	history.WriteString("\n")

	// currentPrompt := input

	for i := 0; i < maxRetries; i++ {

		fmt.Printf("\n--- Attempt %d/%d ---\n", i+1, maxRetries)

		currentPrompt := history.String()

		resp, err := a.aiClient.GetShellScript(currentPrompt)

		if err != nil {
			errMsg := fmt.Sprintf("AI Generation failed: %v\n", err)
			fmt.Println(errMsg)
			// history.WriteString(errMsg)

			ErrorHistory = append(ErrorHistory, AgentError{
				errorType: "ai generation failure",
				errorMsg:  errMsg,
				script:    "",
				output:    "",
			})
			continue
		}

		// history.WriteString("\nScript\n")
		// history.WriteString(resp.Script)
		// history.WriteString("\n")

		// AI based Safety
		if !resp.IsSafe {
			errMsg := fmt.Sprintf("AI marked script unsafe: %s", resp.Explanation)
			// history.WriteString(errMsg)
			fmt.Println(errMsg)
			ErrorHistory = append(ErrorHistory, AgentError{
				errorType: "AI safety validation failed",
				errorMsg:  errMsg,
				script:    resp.Script,
				output:    "",
			})
			continue

		}

		// Static rule based check
		if err := validator.Validate(resp.Script); err != nil {
			errMsg := fmt.Sprintf("Validation failed: %s", err)
			// history.WriteString(errMsg)
			fmt.Println(errMsg)
			ErrorHistory = append(ErrorHistory, AgentError{
				errorType: "rule based safety validation failed",
				errorMsg:  errMsg,
				script:    resp.Script,
				output:    "",
			})
			continue

		}

		fmt.Printf("Script: %s\n", resp.Script)
		fmt.Printf("Explanation: %s\n", resp.Explanation)
		fmt.Printf("Executing Script....")

		if err = powershell.ExecuteScript(resp.Script); err != nil {

			errMsg := fmt.Sprintf("Execution failed: %s", err)

			// history.WriteString("\nExecution Error\n")
			// history.WriteString(errMsg)
			// history.WriteString("\n")
			// history.WriteString("Please fix the script based on the above error\n")

			fmt.Printf("Execution failed: %v\n", errMsg)
			ErrorHistory = append(ErrorHistory, AgentError{
				errorType: "script execution failed",
				errorMsg:  errMsg,
				script:    resp.Script,
				output:    "",
			})

			continue
			// Feed the error back to the AI for the next iteration

		}

		fmt.Println("Success!")
		return nil

	}

	return fmt.Errorf("failed after %d attempts", maxRetries)
}

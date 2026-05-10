package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/executor"
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
	ErrorType string `json:"error_type"`
	ErrorMsg  string `json:"error_msg"`
	Script    string `json:"script"`
	Output    string `json:"output"`
}

type RunResult struct {
	Script      string
	Explanation string
	Success     bool
}

func NewAgent(aiClient *ai.Client) *Agent {
	return &Agent{
		aiClient: aiClient,
	}
}

func (a *Agent) Run(input string) (*RunResult, error) {

	resp, _, err := a.aiClient.GetShellScript(input)

	if err != nil || resp.Script == "" {
		errMsg := fmt.Errorf("AI Generation failed: %v", err)
		return nil, errMsg
	}

	if resp.Script == "" {
		errMsg := fmt.Errorf("AI Generation failed: %v", err)
		return nil, errMsg

	}

	// AI based Safety
	if !resp.IsSafe {
		errMsg := fmt.Errorf("AI marked script unsafe: %s", resp.Explanation)
		return nil, errMsg

	}

	// Static rule based check
	if err := validator.Validate(resp.Script); err != nil {
		errMsg := fmt.Errorf("Validation failed: %s", err)
		return nil, errMsg
	}

	return &RunResult{
		Script:      resp.Script,
		Explanation: resp.Explanation,
		Success:     true,
	}, nil

}


func (a *Agent) TempRun(input string) {
	var ErrorHistory []AgentError

	for i := 0; i < maxRetries; i++ {

		fmt.Printf("\n\x1b[90m--- Attempt %d/%d ---\n\033[0m", i+1, maxRetries)

		jsonOut, jsonErr := json.Marshal(ErrorHistory)
		if jsonErr != nil {
			fmt.Println("Error parsing json")
		}

		ErrorHistoryStr := string(jsonOut)

		currentPrompt := ErrorHistoryStr + input

		resp, body, err := a.aiClient.GetShellScript(currentPrompt)

		if err != nil || resp.Script == "" {
			errMsg := fmt.Sprintf("AI Generation failed: %v", err)
			fmt.Println(errMsg)

			ErrorHistory = append(ErrorHistory, AgentError{
				ErrorType: "ai generation failure",
				ErrorMsg:  errMsg + body,
				Script:    "",
				Output:    "",
			})
			continue
		}

		if resp.Script == "" {
			errMsg := fmt.Sprintf("AI Generation failed: %v", err)
			fmt.Println(errMsg)

			ErrorHistory = append(ErrorHistory, AgentError{
				ErrorType: "ai generation failured with empty string",
				ErrorMsg:  errMsg + body,
				Script:    resp.Script,
				Output:    "",
			})
			continue
		}

		// AI based Safety
		if !resp.IsSafe {
			errMsg := fmt.Sprintf("AI marked script unsafe: %s", resp.Explanation)
			fmt.Println(errMsg)
			ErrorHistory = append(ErrorHistory, AgentError{
				ErrorType: "AI safety validation failed",
				ErrorMsg:  errMsg,
				Script:    resp.Script,
				Output:    "",
			})
			continue

		}

		// Static rule based check
		if err := validator.Validate(resp.Script); err != nil {
			errMsg := fmt.Sprintf("Validation failed: %s", err)
			fmt.Println(errMsg)
			ErrorHistory = append(ErrorHistory, AgentError{
				ErrorType: "rule based safety validation failed",
				ErrorMsg:  errMsg,
				Script:    resp.Script,
				Output:    "",
			})
			continue

		}

		fmt.Printf("Script: \033[32m %s \n\033[0m", resp.Script)
		fmt.Printf("Explanation: %s\n", resp.Explanation)

		if permitted {
			
	return fmt.Errorf("\033[31mfailed after %d attempts\033[0m", maxRetries)
}

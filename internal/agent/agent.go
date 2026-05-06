package agent

import (
	"fmt"
	"github.com/Adhithya-J/underroot.git/internal/ai"
	"github.com/Adhithya-J/underroot.git/internal/powershell"
)

const maxRetries = 3

type Agent struct {
	aiClient *ai.Client
}

func NewAgent(aiClient *ai.Client) *Agent {
	return &Agent{
		aiClient: aiClient,
	}
}

func (a *Agent) Run(input string) error {
	currentPrompt := input
	
	for i := 0; i < maxRetries; i++ {
		fmt.Printf("\n--- Attempt %d ---\n", i+1)
		
		resp, err := a.aiClient.GetShellScript(currentPrompt)
		if err != nil {
			return fmt.Errorf("failed to get shell script: %w", err)
		}

		if !resp.IsSafe {
			return fmt.Errorf("script is not safe: %s", resp.Explanation)
		}

		fmt.Printf("Executing: %s\n", resp.Script)
		fmt.Printf("Explanation: %s\n", resp.Explanation)

		err = powershell.ExecuteScript(resp.Script)
		if err == nil {
			fmt.Println("Success!")
			return nil
		}

		fmt.Printf("Execution failed: %v\n", err)
		// Feed the error back to the AI for the next iteration
		currentPrompt = fmt.Sprintf("The previous script failed with the following error. Please fix it:\n%v", err)
	}

	return fmt.Errorf("failed after %d attempts", maxRetries)
}

package executor

import (
	"fmt"
	"os/exec"
)

// ExecuteScript runs a PowerShell command and prints its output.
func ExecuteScript(input string) (string, error) {
	cmd := exec.Command("powershell", "-Command", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("powershell execution failed: %w", err)
	}
	return string(out), nil
}

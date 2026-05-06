package powershell

import (
	"fmt"
	"os/exec"
)

// ExecuteScript runs a PowerShell command and prints its output.
func ExecuteScript(input string) error {
	cmd := exec.Command("powershell", "-Command", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell execution failed: %w, output: %s", err, string(out))
	}
	fmt.Printf("Output:\n%s\n", out)
	return nil
}

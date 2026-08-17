package executor

import (
	"fmt"
	"os/exec"
	"strings"
)

func quotePowerShellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

// ExecuteScript runs a PowerShell command and prints its output.
func ExecuteScript(input string) (string, error) {
	// #nosec G204 -- executing the validated PowerShell script is this package's purpose.
	cmd := exec.Command("powershell", "-Command", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("powershell execution failed: %w", err)
	}
	return string(out), nil
}

// in the long term, see how you can add parsing for powershell, maybe based on AST

// List_Dir lists the contents of a directory.
func List_Dir(path string) (string, error) {
	script := fmt.Sprintf("Get-ChildItem -Path %s | Select-Object Name, Mode, Length | ConvertTo-Json", quotePowerShellPath(path))
	return ExecuteScript(script)
}

// Read_File reads the first 1000 characters of a file.
func Read_File(path string) (string, error) {
	script := fmt.Sprintf("Get-Content -Path %s | Select-Object -First 1000", quotePowerShellPath(path))
	return ExecuteScript(script)
}

// Exists reports whether a path exists.
func Exists(path string) (string, error) {
	script := fmt.Sprintf("Test-Path -Path %s", quotePowerShellPath(path))
	return ExecuteScript(script)
}

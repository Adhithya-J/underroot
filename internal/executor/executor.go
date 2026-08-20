package executor

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	maxDirectoryEntries = 20
	maxToolResultBytes  = 2048
)

func quotePowerShellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

// ExecuteScript runs a PowerShell command and prints its output.
func ExecuteScript(input string) (string, error) {
	// #nosec G204 -- executing the validated PowerShell script is this package's purpose.
	// Profiles can print warnings or other text before command output, which
	// corrupts structured tool responses such as Exists' JSON.
	cmd := exec.Command("powershell", "-NoProfile", "-Command", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("powershell execution failed: %w", err)
	}
	return string(out), nil
}

// in the long term, see how you can add parsing for powershell, maybe based on AST

// List_Dir lists the contents of a directory.
func List_Dir(path string) (string, error) {
	// Ask PowerShell for only one extra item so the result can report whether
	// it was truncated without ever materializing a whole large directory.
	script := fmt.Sprintf(`$items = @(Get-ChildItem -LiteralPath %s -ErrorAction Stop | Select-Object -First %d)
$entries = @($items | Select-Object -First %d | ForEach-Object {
  [pscustomobject]@{ name = $_.Name; directory = $_.PSIsContainer; length = $_.Length }
})
[pscustomobject]@{ path = %s; entries = $entries; shown = $entries.Count; truncated = ($items.Count -gt %d) } | ConvertTo-Json -Compress -Depth 3`,
		quotePowerShellPath(path), maxDirectoryEntries+1, maxDirectoryEntries,
		quotePowerShellPath(path), maxDirectoryEntries)

	result, err := ExecuteScript(script)
	if err != nil {
		return result, err
	}
	if len(result) <= maxToolResultBytes {
		return result, nil
	}

	// Keep oversized tool output valid and small. The model can retry with a
	// narrower path or filter instead of receiving a malformed JSON fragment.
	return fmt.Sprintf(`{"path":%s,"entries":[],"shown":0,"truncated":true,"message":"directory result exceeded the tool output budget; use a narrower query"}`,
		quoteJSON(path)), nil
}

// Read_File reads the first 1000 characters of a file.
func Read_File(path string) (string, error) {
	script := fmt.Sprintf("Get-Content -Path %s | Select-Object -First 1000", quotePowerShellPath(path))
	return ExecuteScript(script)
}

// Exists reports whether a path exists.
func Exists(path string) (string, error) {
	// Return structured data so the model can distinguish a missing path from
	// a tool failure. Test-Path itself returns False for a missing path.
	script := fmt.Sprintf(
		"$p = %s; [pscustomobject]@{ path = $p; exists = Test-Path -LiteralPath $p -PathType Any } | ConvertTo-Json -Compress",
		quotePowerShellPath(path),
	)
	return ExecuteScript(script)
}

func quoteJSON(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", `\r`, "\n", `\n`).Replace(value) + `"`
}

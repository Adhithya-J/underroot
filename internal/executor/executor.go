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

// DryRunScript parses the script and executes it with PowerShell's WhatIf
// preference enabled. WhatIf is honored by cmdlets that support the common
// WhatIf parameter; it is not a guarantee for arbitrary native commands.
func DryRunScript(input string) (string, error) {
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$WhatIfPreference = $true
$source = %s
[scriptblock]::Create($source) | Out-Null
& ([scriptblock]::Create($source))`, quotePowerShellPath(input))
	return ExecuteScript(script)
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
	return capToolResult(result, fmt.Sprintf(`{"path":%s,"entries":[],"shown":0,"truncated":true,"message":"directory result exceeded the tool output budget; use a narrower query"}`,
		quoteJSON(path))), nil
}

// Find_Entry finds one exact-name child entry of the supplied directory.
func Find_Entry(path, name, entryType string) (string, error) {
	if entryType != "" && entryType != "file" && entryType != "directory" && entryType != "any" {
		return "", fmt.Errorf("type must be file, directory, or any")
	}

	typeFilter := "$true"
	if entryType == "file" {
		typeFilter = "(-not $_.PSIsContainer)"
	} else if entryType == "directory" {
		typeFilter = "$_.PSIsContainer"
	}

	script := fmt.Sprintf(`$item = @(Get-ChildItem -LiteralPath %s -ErrorAction Stop | Where-Object { $_.Name -ceq %s -and %s } | Select-Object -First 1)
[pscustomobject]@{ path = %s; name = %s; type = %s; found = ($item.Count -gt 0); full_name = if ($item.Count -gt 0) { $item[0].FullName } else { $null } } | ConvertTo-Json -Compress`,
		quotePowerShellPath(path), quotePowerShellPath(name), typeFilter,
		quotePowerShellPath(path), quotePowerShellPath(name), quotePowerShellPath(entryType))
	result, err := ExecuteScript(script)
	if err != nil {
		return result, err
	}
	return capToolResult(result, fmt.Sprintf(`{"path":%s,"name":%s,"found":false,"message":"result exceeded the tool output budget"}`,
		quoteJSON(path), quoteJSON(name))), nil
}

// Search_Entries returns a bounded set of direct children whose names contain
// query, ignoring case. It is intended to resolve a user-provided name hint;
// callers must use the returned full_name rather than assume the hint is exact.
func Search_Entries(path, query, entryType string, limit int) (string, error) {
	if entryType != "" && entryType != "file" && entryType != "directory" && entryType != "any" {
		return "", fmt.Errorf("type must be file, directory, or any")
	}
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}
	if limit <= 0 || limit > maxDirectoryEntries {
		limit = maxDirectoryEntries
	}

	typeFilter := "$true"
	if entryType == "file" {
		typeFilter = "(-not $_.PSIsContainer)"
	} else if entryType == "directory" {
		typeFilter = "$_.PSIsContainer"
	}

	script := fmt.Sprintf(`$query = %s
$items = @(Get-ChildItem -LiteralPath %s -ErrorAction Stop | Where-Object { %s -and $_.Name.IndexOf($query, [System.StringComparison]::OrdinalIgnoreCase) -ge 0 } | Select-Object -First %d)
$entries = @($items | ForEach-Object { [pscustomobject]@{ name = $_.Name; directory = $_.PSIsContainer; full_name = $_.FullName } })
[pscustomobject]@{ path = %s; query = $query; entries = $entries; shown = $entries.Count } | ConvertTo-Json -Compress -Depth 3`,
		quotePowerShellPath(query), quotePowerShellPath(path), typeFilter, limit, quotePowerShellPath(path))
	result, err := ExecuteScript(script)
	if err != nil {
		return result, err
	}
	return capToolResult(result, fmt.Sprintf(`{"path":%s,"query":%s,"entries":[],"shown":0,"truncated":true,"message":"result exceeded the tool output budget"}`,
		quoteJSON(path), quoteJSON(query))), nil
}

// Find_File finds a bounded number of files matching an extension directly
// under path. It deliberately does not recurse unless that becomes explicit
// in a future tool contract.
func Find_File(path, extension string, limit int) (string, error) {
	if limit <= 0 || limit > maxDirectoryEntries {
		limit = maxDirectoryEntries
	}
	if extension == "" {
		return "", fmt.Errorf("extension is required")
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	script := fmt.Sprintf(`$items = @(Get-ChildItem -LiteralPath %s -File -ErrorAction Stop | Where-Object { $_.Extension -ieq %s } | Select-Object -First %d)
$files = @($items | ForEach-Object { [pscustomobject]@{ name = $_.Name; full_name = $_.FullName; length = $_.Length } })
[pscustomobject]@{ path = %s; extension = %s; files = $files; shown = $files.Count } | ConvertTo-Json -Compress -Depth 3`,
		quotePowerShellPath(path), quotePowerShellPath(extension), limit,
		quotePowerShellPath(path), quotePowerShellPath(extension))
	result, err := ExecuteScript(script)
	if err != nil {
		return result, err
	}
	return capToolResult(result, fmt.Sprintf(`{"path":%s,"files":[],"shown":0,"truncated":true,"message":"result exceeded the tool output budget"}`,
		quoteJSON(path))), nil
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

// UserDownloads returns the current Windows user's Downloads directory. It is
// resolved from USERPROFILE at execution time rather than guessed from the
// agent's working directory.
func UserDownloads() (string, error) {
	script := `$profile = $env:USERPROFILE
if ([string]::IsNullOrWhiteSpace($profile)) { throw 'USERPROFILE is not available' }
$downloads = Join-Path -Path $profile -ChildPath 'Downloads'
[pscustomobject]@{ path = $downloads; exists = (Test-Path -LiteralPath $downloads -PathType Container) } | ConvertTo-Json -Compress`
	return ExecuteScript(script)
}

func quoteJSON(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", `\r`, "\n", `\n`).Replace(value) + `"`
}

func capToolResult(result, fallback string) string {
	if len(result) <= maxToolResultBytes {
		return result
	}
	return fallback
}

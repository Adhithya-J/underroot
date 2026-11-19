package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ErrASTUnsupported indicates that the platform cannot run PowerShell's AST
// parser. Callers should fall back to static validation.
var ErrASTUnsupported = errors.New("PowerShell AST analysis is only available on Windows")

// ASTCommand is one command invocation discovered in a PowerShell script.
type ASTCommand struct {
	Name       string   `json:"name"`
	Parameters []string `json:"parameters"`
	Arguments  []string `json:"arguments"`
	Dynamic    bool     `json:"dynamic"`
}

// ASTMember is one .NET member invocation discovered in a PowerShell script.
type ASTMember struct {
	Target string `json:"target"`
	Member string `json:"member"`
}

// ScriptAnalysis is the structured result of parsing a script with PowerShell's
// language parser. It never executes the supplied script.
type ScriptAnalysis struct {
	ParseErrors []string     `json:"parse_errors"`
	Commands    []ASTCommand `json:"commands"`
	Members     []ASTMember  `json:"members"`
}

const astBody = `$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$parseErrors)
$commands = @()
if ($null -ne $ast) {
    $commands = @($ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.CommandAst] }, $true) | ForEach-Object {
        $elements = @($_.CommandElements)
        $parameters = @()
        $arguments = @()
        $dynamic = $false
        if ($elements.Count -gt 0) {
            $parameters = @($elements | Where-Object { $_ -is [System.Management.Automation.Language.CommandParameterAst] } | ForEach-Object { $_.ParameterName })
            $arguments = @($elements | Select-Object -Skip 1 | Where-Object { $_ -isnot [System.Management.Automation.Language.CommandParameterAst] } | ForEach-Object { $_.Extent.Text })
            $dynamic = $elements[0] -isnot [System.Management.Automation.Language.StringConstantExpressionAst]
        }
        [pscustomobject]@{
            name = $_.GetCommandName()
            parameters = $parameters
            arguments = $arguments
            dynamic = $dynamic
        }
    })
}
$parseMessages = @($parseErrors | ForEach-Object { $_.Message })
$members = @()
if ($null -ne $ast) {
    $members = @($ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.InvokeMemberExpressionAst] }, $true) | ForEach-Object {
        [pscustomobject]@{
            target = $_.Expression.Extent.Text
            member = $_.Member.Extent.Text
        }
    })
}
[pscustomobject]@{
    parse_errors = $parseMessages
    commands = $commands
    members = $members
} | ConvertTo-Json -Depth 6 -Compress`

// ASTSupported reports whether PowerShell AST analysis can run on this platform.
func ASTSupported() bool {
	return runtime.GOOS == "windows"
}

// AnalyzeScript parses a PowerShell script without executing it and returns the
// command structure plus any parse errors. It requires PowerShell on Windows.
func AnalyzeScript(script string) (*ScriptAnalysis, error) {
	if !ASTSupported() {
		return nil, ErrASTUnsupported
	}
	if strings.TrimSpace(script) == "" {
		return &ScriptAnalysis{ParseErrors: []string{"script is empty"}}, nil
	}

	wrapper := fmt.Sprintf("$ErrorActionPreference = 'Stop'\n$source = %s\n%s", quotePowerShellPath(script), astBody)

	// #nosec G204 -- the script is passed to the parser as inert source text.
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", wrapper)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("PowerShell AST parser failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseScriptAnalysis(output)
}

func parseScriptAnalysis(output []byte) (*ScriptAnalysis, error) {
	output = bytes.TrimPrefix(output, []byte{0xEF, 0xBB, 0xBF})
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		return nil, errors.New("PowerShell AST parser returned no data")
	}

	var analysis ScriptAnalysis
	if err := json.Unmarshal([]byte(trimmed), &analysis); err != nil {
		return nil, fmt.Errorf("decode PowerShell AST result: %w", err)
	}
	return &analysis, nil
}

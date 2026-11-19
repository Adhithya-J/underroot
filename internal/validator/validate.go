package validator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/executor"
)

var internalTools = []string{
	"user_downloads",
	"list_dir",
	"find_entry",
	"search_entries",
	"find_file",
	"read_file",
	"exists",
}

var disallowedKeywords = []string{
	"Invoke-Expression",
	"Add-Type",
	"-EncodedCommand",
	"-WindowStyle Hidden",
	"Invoke-WebRequest",
	"Net.WebClient",
	"VirtualAlloc",
	"CreateThread",
}

// astDangerousCommands maps command names (including common aliases) to the
// reason they are blocked when found by PowerShell AST analysis.
var astDangerousCommands = map[string]string{
	"invoke-expression":             "dynamic code execution",
	"iex":                           "dynamic code execution",
	"add-type":                      "runtime code compilation",
	"invoke-webrequest":             "network download",
	"iwr":                           "network download",
	"invoke-restmethod":             "network download",
	"irm":                           "network download",
	"start-bitstransfer":            "network download",
	"invoke-command":                "remote command execution",
	"enter-pssession":               "remote session",
	"new-pssession":                 "remote session",
	"set-executionpolicy":           "execution policy modification",
	"set-mppreference":              "security tooling modification",
	"add-mppreference":              "security tooling modification",
	"disable-windowsdefender":       "security tooling modification",
	"set-windowsdefenderpreference": "security tooling modification",
}

// Validate rejects scripts containing known dangerous commands.
func Validate(script string) error {
	trimmedScript := strings.TrimSpace(script)
	if trimmedScript == "" {
		return errors.New("script is empty")
	}

	lowerScript := strings.ToLower(trimmedScript)

	// These are application-internal tool names, not PowerShell commands.
	// A small model can occasionally copy a tool call into the script field.
	for _, tool := range internalTools {
		if lowerScript == tool || strings.HasPrefix(lowerScript, tool+" ") || strings.HasPrefix(lowerScript, tool+"\t") {
			return errors.New("script contains an internal tool call; return the tool call in the tool_call field")
		}
	}

	// -File and -Directory are independent switches. A comma after -File
	// makes PowerShell parse Directory as a missing argument.
	if strings.Contains(lowerScript, "-file,directory") {
		return errors.New("invalid PowerShell switch syntax: use -File and -Directory separately")
	}

	for _, d := range disallowedKeywords {
		if strings.Contains(lowerScript, strings.ToLower(d)) {
			return errors.New("Script contains dangerous command: " + d)
		}
	}
	return nil
}

// ValidateAnalysis blocks dangerous commands discovered by PowerShell's AST
// parser. It complements the substring checks in Validate with alias-aware
// command names, parameters, and member invocations.
func ValidateAnalysis(analysis *executor.ScriptAnalysis) error {
	if analysis == nil {
		return errors.New("script analysis is missing")
	}
	if len(analysis.ParseErrors) > 0 {
		return fmt.Errorf("PowerShell parser reported errors: %s", strings.Join(analysis.ParseErrors, "; "))
	}

	for _, command := range analysis.Commands {
		name := strings.ToLower(strings.TrimSpace(command.Name))
		if name == "" {
			continue
		}
		if reason, blocked := astDangerousCommands[name]; blocked {
			return fmt.Errorf("command %q is blocked (%s); replace it with a safe alternative", command.Name, reason)
		}

		parameters := make(map[string]struct{}, len(command.Parameters))
		for _, parameter := range command.Parameters {
			parameters[strings.ToLower(strings.TrimSpace(parameter))] = struct{}{}
		}

		for _, encoded := range []string{"encodedcommand", "enc"} {
			if _, ok := parameters[encoded]; ok {
				return fmt.Errorf("command %q uses -EncodedCommand, which hides executable content", command.Name)
			}
		}

		if _, ok := parameters["windowstyle"]; ok && containsFold(command.Arguments, "hidden") {
			return fmt.Errorf("command %q hides its window, which is not allowed", command.Name)
		}

		if name == "start-process" {
			if _, ok := parameters["verb"]; ok && containsFold(command.Arguments, "runas") {
				return fmt.Errorf("Start-Process with -Verb RunAs requests privilege escalation")
			}
		}
	}

	for _, member := range analysis.Members {
		switch strings.ToLower(strings.TrimSpace(member.Member)) {
		case "downloadstring", "downloadfile", "downloaddata":
			return fmt.Errorf("member invocation %q downloads remote content, which is not allowed", member.Member)
		case "create":
			if strings.Contains(strings.ToLower(member.Target), "scriptblock") {
				return errors.New("creating a scriptblock from a string is dynamic code execution")
			}
		}
	}

	return nil
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

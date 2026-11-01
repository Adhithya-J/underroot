package validator

import (
	"errors"
	"strings"
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

package validator

import (
	"errors"
	"strings"
)

// Validate rejects scripts containing known dangerous commands.
func Validate(script string) error {
	trimmedScript := strings.TrimSpace(script)
	if trimmedScript == "" {
		return errors.New("script is empty")
	}

	// These are application-internal tool names, not PowerShell commands.
	// A small model can occasionally copy a tool call into the script field.
	for _, tool := range []string{"exists", "list_dir", "read_file"} {
		if trimmedScript == tool || strings.HasPrefix(strings.ToLower(trimmedScript), tool+" ") || strings.HasPrefix(strings.ToLower(trimmedScript), tool+"\t") {
			return errors.New("script contains an internal tool call; return the tool call in the tool_call field")
		}
	}

	// -File and -Directory are independent switches. A comma after -File
	// makes PowerShell parse Directory as a missing argument.
	if strings.Contains(strings.ToLower(script), "-file,directory") {
		return errors.New("invalid PowerShell switch syntax: use -File and -Directory separately")
	}

	disallowedKeywords := []string{
		"Invoke-Expression",
		"Add-Type",
		"-EncodedCommand",
		"-WindowStyle Hidden",
		"Invoke-WebRequest",
		"Net.WebClient",
		"VirtualAlloc",
		"CreateThread",
	}

	lowerScript := strings.ToLower(script)

	for _, d := range disallowedKeywords {
		if strings.Contains(lowerScript, strings.ToLower(d)) {
			return errors.New("Script contains dangerous command: " + d)
		}
	}
	return nil
}

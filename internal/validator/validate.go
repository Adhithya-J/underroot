package validator

import (
	"errors"
	"strings"
)

// Validate rejects scripts containing known dangerous commands.
func Validate(script string) error {
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

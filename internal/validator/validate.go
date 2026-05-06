package validator

import (
	"errors"
	"strings"
)

func Validate(script string) error {
	disallowedKeywords := []string{"Remove-Item",
		"Invoke-Expression",
		"Add-Type",
		"-EncodedCommand",
		"-WindowStyle Hidden",
		"Invoke-WebRequest",
		"Net.WebClient",
		"VirtualAlloc",
		"CreateThread"}

	lowerScript := strings.ToLower(script)

	for _, d := range disallowedKeywords {
		if strings.Contains(lowerScript, strings.ToLower(d)) {
			return errors.New("Script contains dangerous command: " + d)
		}
	}
	return nil

}

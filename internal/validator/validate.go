package validator

import (
	"errors"
	"strings"
)

func Validate(script string) error {
	disallowed_keywords := []string{"Remove-Item", "Invoke-Expression", "Add-Type", "-EncodedCommand", "-WindowStyle Hidden", "Invoke-WebRequest", "Net.WebClient", "VirtualAlloc", "CreateThread"}
	for _, d := range disallowed_keywords {
		if strings.Contains(strings.ToLower(d), strings.ToLower(script)) {
			return errors.New("Script contains dangerous command: " + d)
		}
	}
	return nil

}

// Package intent validates a user request before it reaches the model. It is
// deliberately narrower than script validation: it catches empty, malformed,
// overlong, or instruction-override input, while leaving intent interpretation
// to the agent.
package intent

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind classifies a rejected request.
type Kind string

const (
	// KindEmpty means the request had no content.
	KindEmpty Kind = "empty_request"
	// KindTooLong means the request exceeded the input budget.
	KindTooLong Kind = "request_too_long"
	// KindControlChars means the request contained non-text control bytes.
	KindControlChars Kind = "invalid_control_characters"
	// KindNotActionable means the request had no letters or digits.
	KindNotActionable Kind = "request_not_actionable"
	// KindPromptInjection means the request tried to override system rules.
	KindPromptInjection Kind = "prompt_injection"
)

const maxInputRunes = 2000

// Error is a typed user-intent validation failure.
type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func newError(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

var injectionPatterns = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"ignore the previous instructions",
	"disregard previous instructions",
	"disregard all previous instructions",
	"disregard the system prompt",
	"reveal your system prompt",
	"reveal the system prompt",
	"show your system prompt",
	"you are now",
}

// Normalize trims surrounding whitespace without changing the request body.
func Normalize(input string) string {
	return strings.TrimSpace(input)
}

// Validate rejects empty, malformed, overlong, and instruction-override input.
func Validate(input string) error {
	text := Normalize(input)
	if text == "" {
		return newError(KindEmpty, "request is empty")
	}
	if utf8.RuneCountInString(text) > maxInputRunes {
		return newError(KindTooLong, "request is too long (%d characters; maximum is %d)", utf8.RuneCountInString(text), maxInputRunes)
	}

	hasText := false
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return newError(KindControlChars, "request contains unsupported control characters")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasText = true
		}
	}
	if !hasText {
		return newError(KindNotActionable, "request does not contain a readable instruction")
	}

	lower := strings.ToLower(text)
	for _, pattern := range injectionPatterns {
		if strings.Contains(lower, pattern) {
			return newError(KindPromptInjection, "request attempts to override the assistant's instructions")
		}
	}
	return nil
}

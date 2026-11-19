package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// PermissionMode is the session fallback for scripts without an explicit decision.
type PermissionMode int

const (
	// PermissionAsk requires human approval for every script (default).
	PermissionAsk PermissionMode = iota
	// PermissionAuto auto-approves scripts that passed all safety checks.
	PermissionAuto
	// PermissionDeny blocks execution for the rest of the session.
	PermissionDeny
)

// String returns the command-line name for a permission mode.
func (m PermissionMode) String() string {
	switch m {
	case PermissionAuto:
		return "auto"
	case PermissionDeny:
		return "deny"
	default:
		return "ask"
	}
}

// ParsePermissionMode converts a user-supplied mode name.
func ParsePermissionMode(value string) (PermissionMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ask":
		return PermissionAsk, true
	case "auto":
		return PermissionAuto, true
	case "deny":
		return PermissionDeny, true
	default:
		return PermissionAsk, false
	}
}

// PermissionDecision is the result of checking a script against session permissions.
type PermissionDecision int

const (
	// DecisionAsk means the human must approve the script.
	DecisionAsk PermissionDecision = iota
	// DecisionAllow means the session pre-approves the script.
	DecisionAllow
	// DecisionDeny means the session blocks the script.
	DecisionDeny
)

// ScriptCommand is one command invocation extracted from a script.
type ScriptCommand struct {
	Name    string
	Args    string
	Dynamic bool
}

// Permissions stores session-level execution decisions. Three layers apply:
// allowed_tools auto-approve matching commands, disallowed_tools block matching
// commands and override allow rules, and the mode is the fallback.
type Permissions struct {
	mode    PermissionMode
	allowed map[string]struct{}
	denied  map[string]struct{}

	allowedRules    []toolRule
	disallowedRules []toolRule
}

type toolRule struct {
	raw         string
	commandGlob string
	argsGlob    string
}

// NewPermissions creates an empty ask-by-default permission set.
func NewPermissions() *Permissions {
	return &Permissions{
		mode:    PermissionAsk,
		allowed: make(map[string]struct{}),
		denied:  make(map[string]struct{}),
	}
}

// Mode returns the current fallback mode.
func (p *Permissions) Mode() PermissionMode {
	if p == nil {
		return PermissionAsk
	}
	return p.mode
}

// SetMode changes the fallback mode.
func (p *Permissions) SetMode(mode PermissionMode) {
	if p == nil {
		return
	}
	p.mode = mode
}

// Check resolves a script to ask, allow, or deny.
func (p *Permissions) Check(script string) PermissionDecision {
	decision, _ := p.Evaluate(script)
	return decision
}

// Evaluate resolves a script and returns a human-readable denial reason.
func (p *Permissions) Evaluate(script string) (decision PermissionDecision, reason string) {
	if p == nil {
		return DecisionAsk, ""
	}

	key := scriptKey(script)
	if _, ok := p.denied[key]; ok {
		return DecisionDeny, "this script was explicitly blocked for the session"
	}
	if _, ok := p.allowed[key]; ok {
		return DecisionAllow, ""
	}

	return p.evaluateCommands(ExtractCommands(script))
}

// CheckCommand resolves a single command name and argument string.
func (p *Permissions) CheckCommand(name, args string) (decision PermissionDecision, reason string) {
	if p == nil {
		return DecisionAsk, ""
	}
	return p.evaluateCommands([]ScriptCommand{{Name: name, Args: args}})
}

func (p *Permissions) evaluateCommands(commands []ScriptCommand) (decision PermissionDecision, reason string) {
	if len(commands) > 0 {
		for _, rule := range p.disallowedRules {
			for _, command := range commands {
				if rule.matches(command) {
					return DecisionDeny, fmt.Sprintf("command %q matches disallowed rule %q", command.Name, rule.raw)
				}
			}
		}

		for _, command := range commands {
			if command.Dynamic {
				return DecisionAsk, "dynamic command invocation requires explicit approval"
			}
		}

		if len(p.allowedRules) > 0 && p.allCommandsAllowed(commands) {
			return DecisionAllow, ""
		}
	}

	switch p.mode {
	case PermissionAuto:
		return DecisionAllow, ""
	case PermissionDeny:
		return DecisionDeny, "session permission mode is deny"
	default:
		return DecisionAsk, ""
	}
}

func (p *Permissions) allCommandsAllowed(commands []ScriptCommand) bool {
	for _, command := range commands {
		if !matchesAny(p.allowedRules, command) {
			return false
		}
	}
	return true
}

func matchesAny(rules []toolRule, command ScriptCommand) bool {
	for _, rule := range rules {
		if rule.matches(command) {
			return true
		}
	}
	return false
}

func (r toolRule) matches(command ScriptCommand) bool {
	if !globMatch(r.commandGlob, command.Name) {
		return false
	}
	if r.argsGlob == "" || r.argsGlob == "*" {
		return true
	}
	if globMatch(r.argsGlob, command.Args) {
		return true
	}
	for _, arg := range splitArgs(command.Args) {
		if globMatch(r.argsGlob, arg) {
			return true
		}
	}
	return false
}

// splitArgs splits an argument string on whitespace outside quotes and strips
// surrounding quotes from each token. It is used to scope rules to individual
// arguments such as a path.
func splitArgs(input string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		token := strings.TrimSpace(current.String())
		current.Reset()
		if token != "" {
			tokens = append(tokens, strings.Trim(token, "'\""))
		}
	}

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '`' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	flush()
	return tokens
}

// AllowTool adds an allowed_tools rule such as "PowerShell(Get-ChildItem:*)".
func (p *Permissions) AllowTool(pattern string) error {
	if p == nil {
		return nil
	}
	rule, err := parseToolRule(pattern)
	if err != nil {
		return err
	}
	p.allowedRules = append(p.allowedRules, rule)
	return nil
}

// DisallowTool adds a disallowed_tools rule, which overrides allow rules.
func (p *Permissions) DisallowTool(pattern string) error {
	if p == nil {
		return nil
	}
	rule, err := parseToolRule(pattern)
	if err != nil {
		return err
	}
	p.disallowedRules = append(p.disallowedRules, rule)
	return nil
}

// SetAllowedTools replaces every allowed_tools rule.
func (p *Permissions) SetAllowedTools(patterns []string) error {
	if p == nil {
		return nil
	}
	rules, err := parseToolRules(patterns)
	if err != nil {
		return err
	}
	p.allowedRules = rules
	return nil
}

// SetDisallowedTools replaces every disallowed_tools rule.
func (p *Permissions) SetDisallowedTools(patterns []string) error {
	if p == nil {
		return nil
	}
	rules, err := parseToolRules(patterns)
	if err != nil {
		return err
	}
	p.disallowedRules = rules
	return nil
}

func parseToolRules(patterns []string) ([]toolRule, error) {
	rules := make([]toolRule, 0, len(patterns))
	for _, pattern := range patterns {
		rule, err := parseToolRule(pattern)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// ToolRules returns the configured allow and disallow rule strings.
func (p *Permissions) ToolRules() (allowed, disallowed []string) {
	if p == nil {
		return nil, nil
	}
	allowed = make([]string, 0, len(p.allowedRules))
	for _, rule := range p.allowedRules {
		allowed = append(allowed, rule.raw)
	}
	disallowed = make([]string, 0, len(p.disallowedRules))
	for _, rule := range p.disallowedRules {
		disallowed = append(disallowed, rule.raw)
	}
	return allowed, disallowed
}

// Allow remembers a script as approved for the rest of the session.
func (p *Permissions) Allow(script string) {
	if p == nil {
		return
	}
	key := scriptKey(script)
	delete(p.denied, key)
	p.allowed[key] = struct{}{}
}

// Deny remembers a script as blocked for the rest of the session.
func (p *Permissions) Deny(script string) {
	if p == nil {
		return
	}
	key := scriptKey(script)
	delete(p.allowed, key)
	p.denied[key] = struct{}{}
}

// Reset clears remembered decisions and restores ask mode.
func (p *Permissions) Reset() {
	if p == nil {
		return
	}
	p.mode = PermissionAsk
	p.allowed = make(map[string]struct{})
	p.denied = make(map[string]struct{})
	p.allowedRules = nil
	p.disallowedRules = nil
}

// Counts returns the number of remembered allow and deny decisions.
func (p *Permissions) Counts() (allowed, denied int) {
	if p == nil {
		return 0, 0
	}
	return len(p.allowed), len(p.denied)
}

func scriptKey(script string) string {
	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:])
}

func parseToolRule(pattern string) (toolRule, error) {
	raw := strings.TrimSpace(pattern)
	if raw == "" {
		return toolRule{}, fmt.Errorf("permission rule is empty")
	}

	inner := raw
	lower := strings.ToLower(raw)
	for _, wrapper := range []string{"powershell", "pwsh", "bash", "shell"} {
		prefix := wrapper + "("
		if strings.HasPrefix(lower, prefix) && strings.HasSuffix(raw, ")") {
			inner = strings.TrimSpace(raw[len(prefix) : len(raw)-1])
			break
		}
	}

	commandGlob := inner
	argsGlob := "*"
	if index := strings.Index(inner, ":"); index >= 0 {
		commandGlob = strings.TrimSpace(inner[:index])
		argsGlob = strings.TrimSpace(inner[index+1:])
		if argsGlob == "" {
			argsGlob = "*"
		}
	}
	commandGlob = strings.TrimSpace(strings.TrimPrefix(commandGlob, "-"))
	if commandGlob == "" {
		return toolRule{}, fmt.Errorf("permission rule %q has no command", raw)
	}
	return toolRule{raw: raw, commandGlob: commandGlob, argsGlob: argsGlob}, nil
}

// ExtractCommands returns command invocations found in a PowerShell script.
// It is a deliberately small, conservative scanner rather than a full parser;
// it splits only on top-level statement and pipeline separators.
func ExtractCommands(script string) []ScriptCommand {
	cleaned := stripScriptComments(script)
	var commands []ScriptCommand

	for _, statement := range splitOutsideQuotes(cleaned, "\n;") {
		for _, segment := range splitOutsideQuotes(statement, "|") {
			command, ok := parseCommandSegment(segment)
			if ok {
				commands = append(commands, command)
			}
		}
	}
	return commands
}

func parseCommandSegment(segment string) (ScriptCommand, bool) {
	segment = strings.TrimSpace(segment)
	callOperator := strings.HasPrefix(segment, "&") || strings.HasPrefix(segment, ".")
	segment = strings.TrimLeft(segment, "&| \t")
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ScriptCommand{}, false
	}

	// Assignments are expressions, not commands. Inspect the right-hand side.
	if index := strings.Index(segment, "="); index > 0 {
		left := strings.TrimSpace(segment[:index])
		if left != "" && !strings.ContainsAny(left, " \t") {
			segment = strings.TrimSpace(segment[index+1:])
			if segment == "" {
				return ScriptCommand{}, false
			}
		}
	}

	name, rest, ok := firstToken(segment)
	if !ok {
		return ScriptCommand{}, false
	}
	name = strings.Trim(name, "'\"`")
	if name == "" {
		return ScriptCommand{}, false
	}
	if strings.HasPrefix(name, "$") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "{") {
		if callOperator {
			return ScriptCommand{Name: name, Args: strings.TrimSpace(rest), Dynamic: true}, true
		}
		return ScriptCommand{}, false
	}
	if _, keyword := scriptKeywords[strings.ToLower(name)]; keyword {
		return ScriptCommand{}, false
	}
	return ScriptCommand{Name: name, Args: strings.TrimSpace(rest)}, true
}

var scriptKeywords = map[string]struct{}{
	"begin":    {},
	"break":    {},
	"catch":    {},
	"continue": {},
	"do":       {},
	"else":     {},
	"elseif":   {},
	"end":      {},
	"exit":     {},
	"filter":   {},
	"finally":  {},
	"for":      {},
	"foreach":  {},
	"function": {},
	"if":       {},
	"param":    {},
	"process":  {},
	"return":   {},
	"switch":   {},
	"throw":    {},
	"trap":     {},
	"try":      {},
	"until":    {},
	"while":    {},
}

func firstToken(input string) (token, rest string, ok bool) {
	var quote rune
	escaped := false
	for index, r := range input {
		if escaped {
			escaped = false
			continue
		}
		if r == '`' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t':
			return input[:index], input[index:], true
		}
	}
	if strings.TrimSpace(input) == "" {
		return "", "", false
	}
	return input, "", true
}

func splitOutsideQuotes(input, separators string) []string {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '`' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if strings.ContainsRune(separators, r) {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	parts = append(parts, current.String())
	return parts
}

func stripScriptComments(input string) string {
	var result strings.Builder
	var quote rune
	escaped := false
	inBlockComment := false

	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if escaped {
			result.WriteRune(r)
			escaped = false
			continue
		}
		if r == '`' && !inBlockComment {
			result.WriteRune(r)
			escaped = true
			continue
		}
		if inBlockComment {
			if r == '#' && i+1 < len(runes) && runes[i+1] == '>' {
				inBlockComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			result.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			result.WriteRune(r)
			continue
		}
		if r == '<' && i+1 < len(runes) && runes[i+1] == '#' {
			inBlockComment = true
			i++
			continue
		}
		if r == '#' {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			if i < len(runes) {
				result.WriteRune('\n')
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// globMatch performs a case-insensitive '*' and '?' wildcard match.
func globMatch(pattern, value string) bool {
	pattern = strings.ToLower(pattern)
	value = strings.ToLower(value)
	patternRunes := []rune(pattern)
	valueRunes := []rune(value)

	p, v := 0, 0
	star, match := -1, 0
	for v < len(valueRunes) {
		if p < len(patternRunes) && (patternRunes[p] == '?' || patternRunes[p] == valueRunes[v]) {
			p++
			v++
			continue
		}
		if p < len(patternRunes) && patternRunes[p] == '*' {
			star = p
			match = v
			p++
			continue
		}
		if star >= 0 {
			p = star + 1
			match++
			v = match
			continue
		}
		return false
	}
	for p < len(patternRunes) && patternRunes[p] == '*' {
		p++
	}
	return p == len(patternRunes)
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
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

// Permissions stores session-level execution decisions.
type Permissions struct {
	mode    PermissionMode
	allowed map[string]struct{}
	denied  map[string]struct{}
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

// Check resolves a script to ask, allow, or deny. Explicit per-script decisions
// take precedence over the session mode.
func (p *Permissions) Check(script string) PermissionDecision {
	if p == nil {
		return DecisionAsk
	}
	key := scriptKey(script)
	if _, ok := p.denied[key]; ok {
		return DecisionDeny
	}
	if _, ok := p.allowed[key]; ok {
		return DecisionAllow
	}

	switch p.mode {
	case PermissionAuto:
		return DecisionAllow
	case PermissionDeny:
		return DecisionDeny
	default:
		return DecisionAsk
	}
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

// Package objectives persists a small todo.md file. The agent recites the
// active objective at the end of its prompt so long sessions do not drift from
// the user's goal, and failed objectives remain visible as evidence.
package objectives

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Adhithya-J/underroot.git/internal/knownfolders"
)

const (
	// FileName is the objective ledger written inside the config directory.
	FileName = "todo.md"
	// MaxEntries bounds the ledger so it cannot grow without limit.
	MaxEntries = 20
)

// Status is the lifecycle state of one objective.
type Status string

const (
	// StatusPending is an objective that was not completed.
	StatusPending Status = "pending"
	// StatusActive is the objective being worked on.
	StatusActive Status = "active"
	// StatusDone is a completed objective.
	StatusDone Status = "done"
	// StatusFailed is an objective that exhausted its retries.
	StatusFailed Status = "failed"
)

// Entry is one persisted objective and its latest evidence.
type Entry struct {
	Goal   string
	Status Status
	Note   string
}

// Recorder reads and writes the objective ledger.
type Recorder struct {
	path string
	mu   sync.Mutex
}

// New creates a recorder for an explicit file path.
func New(path string) *Recorder {
	return &Recorder{path: path}
}

// DefaultPath returns the user-local todo.md path.
func DefaultPath() (string, error) {
	dir, err := knownfolders.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Default creates a recorder in the user-local config directory.
func Default() (*Recorder, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return New(path), nil
}

// Path returns the ledger path.
func (r *Recorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Begin marks a new objective as active and leaves earlier failures visible.
func (r *Recorder) Begin(goal string) error {
	goal = cleanText(goal)
	if goal == "" {
		return errors.New("objective is empty")
	}
	return r.update(func(entries []Entry) []Entry {
		for i := range entries {
			if entries[i].Status == StatusActive {
				entries[i].Status = StatusPending
			}
		}
		return append(entries, Entry{Goal: goal, Status: StatusActive})
	})
}

// Complete marks the matching active objective as done.
func (r *Recorder) Complete(goal, note string) error {
	return r.finish(goal, StatusDone, note)
}

// Fail marks the matching active objective as failed with durable evidence.
func (r *Recorder) Fail(goal, note string) error {
	return r.finish(goal, StatusFailed, note)
}

func (r *Recorder) finish(goal string, status Status, note string) error {
	goal = cleanText(goal)
	if goal == "" {
		return errors.New("objective is empty")
	}
	note = cleanText(note)
	return r.update(func(entries []Entry) []Entry {
		index := latestActive(entries, goal)
		if index < 0 {
			entries = append(entries, Entry{Goal: goal, Status: status, Note: note})
			return entries
		}
		entries[index].Status = status
		entries[index].Note = note
		return entries
	})
}

// Entries returns the current ledger in chronological order.
func (r *Recorder) Entries() ([]Entry, error) {
	if r == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}

// Recitation renders the ledger for the end of the model context.
func (r *Recorder) Recitation() string {
	entries, err := r.Entries()
	if err != nil || len(entries) == 0 {
		return "No active objective recorded. Treat the TASK as the objective."
	}

	var sb strings.Builder
	sb.WriteString("RECENT OBJECTIVE HISTORY (failed actions are evidence; do not repeat them):\n")
	recentStart := 0
	if len(entries) > 5 {
		recentStart = len(entries) - 5
	}
	for _, entry := range entries[recentStart:] {
		fmt.Fprintf(&sb, "- [%s] %s", entry.Status, entry.Goal)
		if entry.Note != "" {
			fmt.Fprintf(&sb, ": %s", entry.Note)
		}
		sb.WriteString("\n")
	}

	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Status == StatusActive {
			fmt.Fprintf(&sb, "\nACTIVE OBJECTIVE (recite before acting): %s\n", entries[i].Goal)
			break
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (r *Recorder) update(mutate func([]Entry) []Entry) error {
	if r == nil {
		return errors.New("objective recorder is nil")
	}
	if strings.TrimSpace(r.path) == "" {
		return errors.New("objective path is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := r.load()
	if err != nil {
		return err
	}
	entries = trimEntries(mutate(entries))
	return r.save(entries)
}

func trimEntries(entries []Entry) []Entry {
	if len(entries) <= MaxEntries {
		return entries
	}
	return entries[len(entries)-MaxEntries:]
}

func latestActive(entries []Entry, goal string) int {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Status != StatusActive {
			continue
		}
		if goal == "" || entries[i].Goal == goal {
			return i
		}
	}
	return -1
}

func (r *Recorder) load() ([]Entry, error) {
	// #nosec G304 -- the ledger path is application-controlled.
	contents, err := os.ReadFile(filepath.Clean(r.path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read objectives: %w", err)
	}
	return parseLedger(string(contents)), nil
}

func (r *Recorder) save(entries []Entry) error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create objectives directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".todo-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary objectives file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.WriteString(renderLedger(entries)); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write objectives: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close objectives: %w", err)
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		return fmt.Errorf("replace objectives: %w", err)
	}
	return nil
}

func renderLedger(entries []Entry) string {
	var sb strings.Builder
	sb.WriteString("# Underroot todo\n\n")
	sb.WriteString("Objectives are recited at the end of the model context. Failures stay visible as evidence.\n\n")
	for _, entry := range entries {
		fmt.Fprintf(&sb, "- [%s] %s", statusMarker(entry.Status), cleanText(entry.Goal))
		if note := cleanText(entry.Note); note != "" {
			fmt.Fprintf(&sb, " — %s", note)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func parseLedger(contents string) []Entry {
	var entries []Entry
	for _, rawLine := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		rest := strings.TrimPrefix(line, "- [")
		if len(rest) < 3 {
			continue
		}
		status := statusFromMarker(rest[0])
		if status == "" {
			continue
		}
		payload := strings.TrimSpace(rest[2:])
		goal, note := payload, ""
		if index := strings.Index(payload, " — "); index >= 0 {
			goal = strings.TrimSpace(payload[:index])
			note = strings.TrimSpace(payload[index+len(" — "):])
		}
		if goal == "" {
			continue
		}
		entries = append(entries, Entry{Goal: goal, Status: status, Note: note})
	}
	return entries
}

func statusMarker(status Status) string {
	switch status {
	case StatusDone:
		return "x"
	case StatusFailed:
		return "!"
	case StatusActive:
		return "~"
	default:
		return " "
	}
}

func statusFromMarker(marker byte) Status {
	switch marker {
	case 'x', 'X':
		return StatusDone
	case '!':
		return StatusFailed
	case '~':
		return StatusActive
	case ' ':
		return StatusPending
	default:
		return ""
	}
}

func cleanText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

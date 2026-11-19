package main

import (
	"strings"
	"testing"

	"github.com/Adhithya-J/underroot.git/internal/knownfolders"
)

func TestBuildContextWithoutActiveError(t *testing.T) {
	session := &Session{
		FolderName:   "underroot",
		CurrentDir:   `C:\projects\underroot`,
		ParentDir:    `C:\projects`,
		CurrentInput: "list files",
		History: []Interaction{
			{UserInput: "show downloads", FinalScript: "Get-ChildItem -Path $env:USERPROFILE\\Downloads"},
		},
	}

	ctx := BuildContext(session)
	if ctx.Task != "list files" {
		t.Fatalf("expected task 'list files', got: %s", ctx.Task)
	}
	if ctx.ActiveError != "" {
		t.Fatalf("expected empty ActiveError, got: %s", ctx.ActiveError)
	}
	if !strings.Contains(ctx.History, "[1] User: show downloads") {
		t.Fatalf("expected formatted history, got: %s", ctx.History)
	}
	if est := EstimatePromptTokens(ctx); est <= 0 {
		t.Fatalf("expected positive estimated tokens, got: %d", est)
	}
}

func TestBuildContextWithActiveError(t *testing.T) {
	session := &Session{
		FolderName:   "underroot",
		CurrentDir:   `C:\projects\underroot`,
		ParentDir:    `C:\projects`,
		CurrentInput: "list files",
	}

	session.SetActiveError("ExecutionFailed", "Cannot find path", "Get-ChildItem -Path non_existent")

	ctx := BuildContext(session)
	if !strings.Contains(ctx.ActiveError, "ExecutionFailed") {
		t.Fatalf("expected ActiveError to contain ExecutionFailed, got: %s", ctx.ActiveError)
	}
	if !strings.Contains(ctx.ActiveError, "Failed Script: Get-ChildItem -Path non_existent") {
		t.Fatalf("expected ActiveError to contain failed script, got: %s", ctx.ActiveError)
	}

	session.ClearActiveError()
	ctxAfterClear := BuildContext(session)
	if ctxAfterClear.ActiveError != "" {
		t.Fatalf("expected empty ActiveError after clear, got: %s", ctxAfterClear.ActiveError)
	}
}

func TestFailureSummaryIsRetainedInContext(t *testing.T) {
	session := &Session{
		FolderName:   "underroot",
		CurrentDir:   `C:\projects\underroot`,
		ParentDir:    `C:\projects`,
		CurrentInput: "remove the temp folder",
	}

	session.AddFailure(Failure{
		UserInput: "remove the temp folder",
		Attempts:  maxRetries,
		ErrorType: "ExecutionFailed",
		ErrorMsg:  "Cannot find path 'C:\\temp' because it does not exist.",
		Script:    "Remove-Item -Path 'C:\\temp'",
	})

	ctx := BuildContext(session)
	if !strings.Contains(ctx.FailureHistory, "Attempts: 3/3") {
		t.Fatalf("expected attempt count in failure history, got: %s", ctx.FailureHistory)
	}
	if !strings.Contains(ctx.FailureHistory, "ExecutionFailed") {
		t.Fatalf("expected error type in failure history, got: %s", ctx.FailureHistory)
	}
	if !strings.Contains(ctx.FailureHistory, "Next approach:") {
		t.Fatalf("expected guidance in failure history, got: %s", ctx.FailureHistory)
	}
	if !strings.Contains(ctx.FailureHistory, "Remove-Item") {
		t.Fatalf("expected failed script in failure history, got: %s", ctx.FailureHistory)
	}
}

func TestBuildContextIncludesOnlyPresentKnownFolders(t *testing.T) {
	session := &Session{
		FolderName:   "underroot",
		CurrentDir:   `C:\projects\underroot`,
		ParentDir:    `C:\projects`,
		CurrentInput: "open downloads",
		KnownFolders: []knownfolders.Folder{
			{Name: "Downloads", Path: `C:\Users\test\Downloads`, Exists: true},
			{Name: "Music", Path: `C:\Users\test\Music`, Exists: false},
		},
	}

	ctx := BuildContext(session)
	if len(ctx.KnownFolders) != 1 {
		t.Fatalf("expected one verified known folder, got %+v", ctx.KnownFolders)
	}
	if ctx.KnownFolders[0].Name != "Downloads" || ctx.KnownFolders[0].Path != `C:\Users\test\Downloads` {
		t.Fatalf("unexpected known folder: %+v", ctx.KnownFolders[0])
	}
}

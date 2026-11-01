package main

import (
	"strings"
	"testing"
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

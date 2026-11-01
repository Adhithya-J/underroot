package main

import (
	"strings"
	"testing"
)

func TestBuildPromptWithoutActiveError(t *testing.T) {
	session := &Session{
		FolderName:   "underroot",
		CurrentDir:   `C:\projects\underroot`,
		ParentDir:    `C:\projects`,
		CurrentInput: "list files",
		History: []Interaction{
			{UserInput: "show downloads", FinalScript: "Get-ChildItem -Path $env:USERPROFILE\\Downloads"},
		},
	}

	messages, err := BuildPrompt(session)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	content := messages[0].Content
	if strings.Contains(content, "CURRENT RETRY ERROR") {
		t.Fatalf("prompt unexpectedly contained CURRENT RETRY ERROR: %s", content)
	}
	if strings.Contains(content, "PREVIOUS ERROR PATTERNS") {
		t.Fatalf("prompt unexpectedly contained PREVIOUS ERROR PATTERNS: %s", content)
	}
	if !strings.Contains(content, "TASK\nlist files") {
		t.Fatalf("prompt missing task: %s", content)
	}
	if !strings.Contains(content, "[1] User: show downloads") {
		t.Fatalf("prompt missing formatted history: %s", content)
	}
}

func TestBuildPromptWithActiveError(t *testing.T) {
	session := &Session{
		FolderName:   "underroot",
		CurrentDir:   `C:\projects\underroot`,
		ParentDir:    `C:\projects`,
		CurrentInput: "list files",
	}

	session.SetActiveError("ExecutionFailed", "Cannot find path", "Get-ChildItem -Path non_existent")

	messages, err := BuildPrompt(session)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	content := messages[0].Content
	if !strings.Contains(content, "CURRENT RETRY ERROR") {
		t.Fatalf("prompt missing CURRENT RETRY ERROR: %s", content)
	}
	if !strings.Contains(content, "Failed Script: Get-ChildItem -Path non_existent") {
		t.Fatalf("prompt missing failed script: %s", content)
	}

	session.ClearActiveError()
	messagesAfterClear, err := BuildPrompt(session)
	if err != nil {
		t.Fatalf("BuildPrompt after clear failed: %v", err)
	}
	if strings.Contains(messagesAfterClear[0].Content, "CURRENT RETRY ERROR") {
		t.Fatalf("prompt still contained error after clear: %s", messagesAfterClear[0].Content)
	}
}

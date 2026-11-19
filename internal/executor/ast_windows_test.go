//go:build windows

package executor

import "testing"

func TestAnalyzeScriptParsesCommands(t *testing.T) {
	analysis, err := AnalyzeScript("Get-ChildItem -Path 'C:\\tmp' -File")
	if err != nil {
		t.Fatalf("AnalyzeScript failed: %v", err)
	}
	if len(analysis.ParseErrors) != 0 {
		t.Fatalf("expected no parse errors, got %v", analysis.ParseErrors)
	}
	if len(analysis.Commands) != 1 {
		t.Fatalf("expected one command, got %+v", analysis.Commands)
	}
	command := analysis.Commands[0]
	if command.Name != "Get-ChildItem" {
		t.Fatalf("expected Get-ChildItem, got %q", command.Name)
	}
	if len(command.Parameters) != 2 {
		t.Fatalf("expected two parameters, got %+v", command.Parameters)
	}
}

func TestAnalyzeScriptFindsAliases(t *testing.T) {
	analysis, err := AnalyzeScript("iex 'Get-ChildItem'")
	if err != nil {
		t.Fatalf("AnalyzeScript failed: %v", err)
	}
	if len(analysis.ParseErrors) != 0 {
		t.Fatalf("expected no parse errors, got %v", analysis.ParseErrors)
	}
	if len(analysis.Commands) != 1 || analysis.Commands[0].Name != "iex" {
		t.Fatalf("expected the iex command, got %+v", analysis.Commands)
	}
}

func TestAnalyzeScriptReportsParseErrors(t *testing.T) {
	analysis, err := AnalyzeScript("Get-ChildItem -Path {")
	if err != nil {
		t.Fatalf("AnalyzeScript failed: %v", err)
	}
	if len(analysis.ParseErrors) == 0 {
		t.Fatal("expected parse errors for an invalid script")
	}
}

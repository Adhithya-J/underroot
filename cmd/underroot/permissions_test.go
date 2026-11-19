package main

import (
	"strings"
	"testing"
)

func TestPermissionsDefaultToAsk(t *testing.T) {
	permissions := NewPermissions()
	if permissions.Mode() != PermissionAsk {
		t.Fatalf("expected ask mode, got %s", permissions.Mode())
	}
	if decision := permissions.Check("Get-ChildItem"); decision != DecisionAsk {
		t.Fatalf("expected ask decision, got %d", decision)
	}
}

func TestPermissionsDenyOverridesAllow(t *testing.T) {
	permissions := NewPermissions()
	permissions.Allow("Get-ChildItem")
	permissions.Deny("Get-ChildItem")

	if decision := permissions.Check("Get-ChildItem"); decision != DecisionDeny {
		t.Fatalf("expected deny decision, got %d", decision)
	}
}

func TestPermissionsAutoModeApprovesUnknownScripts(t *testing.T) {
	permissions := NewPermissions()
	permissions.SetMode(PermissionAuto)

	if decision := permissions.Check("Get-ChildItem"); decision != DecisionAllow {
		t.Fatalf("expected auto approval, got %d", decision)
	}

	permissions.Deny("Get-ChildItem")
	if decision := permissions.Check("Get-ChildItem"); decision != DecisionDeny {
		t.Fatalf("expected explicit deny to override auto mode, got %d", decision)
	}
}

func TestPermissionsDenyModeBlocksEverything(t *testing.T) {
	permissions := NewPermissions()
	permissions.SetMode(PermissionDeny)

	if decision := permissions.Check("Get-ChildItem"); decision != DecisionDeny {
		t.Fatalf("expected deny mode to block, got %d", decision)
	}
}

func TestPermissionsReset(t *testing.T) {
	permissions := NewPermissions()
	permissions.SetMode(PermissionAuto)
	permissions.Allow("Get-ChildItem")
	permissions.Deny("Remove-Item")

	permissions.Reset()

	allowed, denied := permissions.Counts()
	if permissions.Mode() != PermissionAsk || allowed != 0 || denied != 0 {
		t.Fatalf("expected a clean ask-mode permission set, got mode=%s allowed=%d denied=%d",
			permissions.Mode(), allowed, denied)
	}
}

func TestParsePermissionMode(t *testing.T) {
	cases := []struct {
		value    string
		expected PermissionMode
	}{
		{"ask", PermissionAsk},
		{"AUTO", PermissionAuto},
		{"deny", PermissionDeny},
		{" deny ", PermissionDeny},
	}
	for _, testCase := range cases {
		mode, ok := ParsePermissionMode(testCase.value)
		if !ok || mode != testCase.expected {
			t.Fatalf("ParsePermissionMode(%q) = %v, %v", testCase.value, mode, ok)
		}
	}

	if _, ok := ParsePermissionMode("sometimes"); ok {
		t.Fatal("expected an unknown mode to be rejected")
	}
}

func TestAllowedToolRuleAutoApprovesMatchingCommands(t *testing.T) {
	permissions := NewPermissions()
	if err := permissions.AllowTool("PowerShell(Get-ChildItem:*)"); err != nil {
		t.Fatal(err)
	}

	if decision, _ := permissions.Evaluate(`Get-ChildItem -Path 'C:\temp'`); decision != DecisionAllow {
		t.Fatalf("expected the allowed tool rule to approve, got %d", decision)
	}
	if decision, _ := permissions.Evaluate(`Remove-Item -Path 'C:\temp'`); decision != DecisionAsk {
		t.Fatalf("expected an unmatched command to fall back to ask, got %d", decision)
	}
}

func TestDisallowedToolRuleOverridesAllowedTool(t *testing.T) {
	permissions := NewPermissions()
	if err := permissions.AllowTool("*"); err != nil {
		t.Fatal(err)
	}
	if err := permissions.DisallowTool("PowerShell(Remove-Item:*)"); err != nil {
		t.Fatal(err)
	}

	decision, reason := permissions.Evaluate(`Remove-Item -Path 'C:\temp' -Recurse`)
	if decision != DecisionDeny {
		t.Fatalf("expected the disallowed tool rule to block, got %d", decision)
	}
	if !strings.Contains(reason, "disallowed") {
		t.Fatalf("expected a denial reason, got %q", reason)
	}

	if decision, _ := permissions.Evaluate(`Get-ChildItem -Path 'C:\temp'`); decision != DecisionAllow {
		t.Fatalf("expected the allowed tool rule to still apply, got %d", decision)
	}
}

func TestAllowedToolRuleRequiresEveryPipelineCommand(t *testing.T) {
	permissions := NewPermissions()
	if err := permissions.AllowTool("Get-ChildItem"); err != nil {
		t.Fatal(err)
	}

	if decision, _ := permissions.Evaluate(`Get-ChildItem -Path 'C:\temp' | Remove-Item`); decision != DecisionAsk {
		t.Fatalf("expected a mixed pipeline to require approval, got %d", decision)
	}

	if err := permissions.DisallowTool("Remove-Item"); err != nil {
		t.Fatal(err)
	}
	if decision, _ := permissions.Evaluate(`Get-ChildItem -Path 'C:\temp' | Remove-Item`); decision != DecisionDeny {
		t.Fatalf("expected the disallowed command to block the pipeline, got %d", decision)
	}
}

func TestPermissionRulesSupportArgumentScoping(t *testing.T) {
	permissions := NewPermissions()
	if err := permissions.AllowTool("PowerShell(Get-ChildItem:C:\\temp\\*)"); err != nil {
		t.Fatal(err)
	}

	if decision, _ := permissions.Evaluate(`Get-ChildItem -Path 'C:\temp\logs'`); decision != DecisionAllow {
		t.Fatalf("expected the scoped rule to approve, got %d", decision)
	}
	if decision, _ := permissions.Evaluate(`Get-ChildItem -Path 'C:\Users'`); decision != DecisionAsk {
		t.Fatalf("expected the scoped rule to reject other paths, got %d", decision)
	}
}

func TestPermissionsResetClearsToolRules(t *testing.T) {
	permissions := NewPermissions()
	if err := permissions.AllowTool("Get-ChildItem"); err != nil {
		t.Fatal(err)
	}
	if err := permissions.DisallowTool("Remove-Item"); err != nil {
		t.Fatal(err)
	}

	permissions.Reset()

	allowedRules, disallowedRules := permissions.ToolRules()
	if len(allowedRules) != 0 || len(disallowedRules) != 0 {
		t.Fatalf("expected reset to clear rules, got %+v %+v", allowedRules, disallowedRules)
	}
}

func TestExtractCommandsHandlesStatementsPipelinesAndAssignments(t *testing.T) {
	script := `$items = Get-ChildItem -Path 'C:\temp'
$items | Where-Object { $_.Length -gt 0 } | Remove-Item
Write-Output 'done'; Get-Process`
	commands := ExtractCommands(script)

	var names []string
	for _, command := range commands {
		names = append(names, command.Name)
	}
	joined := strings.Join(names, ",")
	for _, expected := range []string{"Get-ChildItem", "Where-Object", "Remove-Item", "Write-Output", "Get-Process"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %s in extracted commands, got %s", expected, joined)
		}
	}
	if strings.Contains(joined, "$items") {
		t.Fatalf("assignments should not be treated as commands, got %s", joined)
	}
}

func TestCheckCommandUsesRules(t *testing.T) {
	permissions := NewPermissions()
	if err := permissions.DisallowTool("PowerShell(Invoke-WebRequest:*)"); err != nil {
		t.Fatal(err)
	}

	decision, _ := permissions.CheckCommand("Invoke-WebRequest", "https://example.com")
	if decision != DecisionDeny {
		t.Fatalf("expected a denied command, got %d", decision)
	}
}

func TestDynamicInvocationRequiresApproval(t *testing.T) {
	permissions := NewPermissions()
	permissions.SetMode(PermissionAuto)
	if err := permissions.AllowTool("*"); err != nil {
		t.Fatal(err)
	}

	decision, reason := permissions.Evaluate(`& $scriptBlock -Path 'C:\temp'`)
	if decision != DecisionAsk {
		t.Fatalf("expected dynamic invocation to require approval, got %d", decision)
	}
	if !strings.Contains(reason, "dynamic") {
		t.Fatalf("expected a dynamic-invocation reason, got %q", reason)
	}
}

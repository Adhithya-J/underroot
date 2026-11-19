package main

import "testing"

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

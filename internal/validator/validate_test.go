package validator

import "testing"

func TestValidateRejectsInternalToolAsScript(t *testing.T) {
	for _, script := range []string{"exists -path 'C:\\tmp'", "list_dir 'C:\\tmp'", "read_file 'C:\\tmp\\a.txt'"} {
		if err := Validate(script); err == nil {
			t.Errorf("Validate(%q) accepted an internal tool call", script)
		}
	}
}

func TestValidateRejectsCombinedDirectorySwitches(t *testing.T) {
	if err := Validate("Get-ChildItem -Path 'C:\\tmp' -File,Directory"); err == nil {
		t.Fatal("Validate accepted -File,Directory")
	}
}

func TestValidateAcceptsDirectoryListing(t *testing.T) {
	if err := Validate("Get-ChildItem -Path 'C:\\tmp'"); err != nil {
		t.Fatalf("Validate rejected a valid directory listing: %v", err)
	}
}

package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExistsReturnsStructuredMissingPath(t *testing.T) {
	result, err := Exists(`C:\path\that\should\not\exist\underroot-test`)
	if err != nil {
		t.Fatalf("Exists returned an error for a missing path: %v", err)
	}

	var response struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Exists returned invalid JSON %q: %v", result, err)
	}
	if response.Exists {
		t.Fatalf("Exists reported a missing path as existing: %q", response.Path)
	}
}

func TestSearchEntriesMatchesPartialNameIgnoringCase(t *testing.T) {
	dir := t.TempDir()
	name := "DownLoads FolDer"
	if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Search_Entries(dir, "downloads folder", "directory", 10)
	if err != nil {
		t.Fatalf("Search_Entries returned an error: %v", err)
	}

	var response struct {
		Entries []struct {
			Name      string `json:"name"`
			Directory bool   `json:"directory"`
			FullName  string `json:"full_name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Search_Entries returned invalid JSON %q: %v", result, err)
	}
	if len(response.Entries) != 1 || response.Entries[0].Name != name || !response.Entries[0].Directory {
		t.Fatalf("unexpected search result: %s", result)
	}
}

func TestPermissionDeniedClassification(t *testing.T) {
	mockErr := os.ErrPermission

	unauthErr := checkPermissionError("Get-Item : Access to the path is denied. [System.UnauthorizedAccessException]", mockErr)
	if !IsPermissionDenied(unauthErr) {
		t.Fatalf("expected ErrPermissionDenied for UnauthorizedAccessException, got: %v", unauthErr)
	}

	elevationErr := checkPermissionError("The requested operation requires elevation (Run as administrator)", mockErr)
	if !IsPermissionDenied(elevationErr) {
		t.Fatalf("expected ErrPermissionDenied for elevation message, got: %v", elevationErr)
	}

	normalErr := checkPermissionError("ItemNotFoundException: Path does not exist", mockErr)
	if IsPermissionDenied(normalErr) {
		t.Fatalf("did not expect ErrPermissionDenied for normal error, got: %v", normalErr)
	}
}

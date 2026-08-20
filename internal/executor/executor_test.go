package executor

import (
	"encoding/json"
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

package objectives

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorderRecitesActiveObjective(t *testing.T) {
	recorder := New(filepath.Join(t.TempDir(), FileName))
	if err := recorder.Begin("archive log files"); err != nil {
		t.Fatal(err)
	}

	recitation := recorder.Recitation()
	if !strings.Contains(recitation, "ACTIVE OBJECTIVE") || !strings.Contains(recitation, "archive log files") {
		t.Fatalf("expected active objective recitation, got: %s", recitation)
	}

	entries, err := recorder.Entries()
	if err != nil || len(entries) != 1 || entries[0].Status != StatusActive {
		t.Fatalf("unexpected ledger: %+v %v", entries, err)
	}
}

func TestRecorderRetainsFailuresAsEvidence(t *testing.T) {
	recorder := New(filepath.Join(t.TempDir(), FileName))
	if err := recorder.Begin("remove temp folder"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Fail("remove temp folder", "ExecutionFailed: path does not exist"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Begin("archive log files"); err != nil {
		t.Fatal(err)
	}

	recitation := recorder.Recitation()
	if !strings.Contains(recitation, "[failed] remove temp folder") {
		t.Fatalf("expected failed objective evidence, got: %s", recitation)
	}
	if !strings.Contains(recitation, "ExecutionFailed") {
		t.Fatalf("expected failure note, got: %s", recitation)
	}
	if !strings.Contains(recitation, "ACTIVE OBJECTIVE (recite before acting): archive log files") {
		t.Fatalf("expected the newest active objective last, got: %s", recitation)
	}
}

func TestRecorderMarksCompletedObjective(t *testing.T) {
	recorder := New(filepath.Join(t.TempDir(), FileName))
	if err := recorder.Begin("list files"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Complete("list files", "executed successfully"); err != nil {
		t.Fatal(err)
	}

	entries, err := recorder.Entries()
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected ledger: %+v %v", entries, err)
	}
	if entries[0].Status != StatusDone || entries[0].Note != "executed successfully" {
		t.Fatalf("expected a completed objective, got %+v", entries[0])
	}
}

func TestRecorderTrimsLedger(t *testing.T) {
	recorder := New(filepath.Join(t.TempDir(), FileName))
	for i := 0; i < MaxEntries+5; i++ {
		if err := recorder.Begin("objective"); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := recorder.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != MaxEntries {
		t.Fatalf("expected %d entries, got %d", MaxEntries, len(entries))
	}
}

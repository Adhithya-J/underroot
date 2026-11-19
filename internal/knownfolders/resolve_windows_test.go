//go:build windows

package knownfolders

import "testing"

func TestParseResolvedFoldersOrdersStandardFolders(t *testing.T) {
	output := []byte(`[
		{"name":"Music","path":"C:\\Users\\test\\Music"},
		{"name":"Desktop","path":"C:\\Users\\test\\Desktop"},
		{"name":"Downloads","path":"C:\\Users\\test\\Downloads"}
	]`)

	folders, err := parseResolvedFolders(output)
	if err != nil {
		t.Fatalf("parseResolvedFolders failed: %v", err)
	}
	if len(folders) != len(StandardFolders) {
		t.Fatalf("expected %d folders, got %d", len(StandardFolders), len(folders))
	}
	if folders[0].Name != "Desktop" || folders[0].Path != `C:\Users\test\Desktop` {
		t.Fatalf("expected Desktop first, got %+v", folders[0])
	}
	if folders[1].Name != "Downloads" || folders[1].Path != `C:\Users\test\Downloads` {
		t.Fatalf("expected Downloads second, got %+v", folders[1])
	}
	if folders[5].Name != "Music" || folders[5].Path != `C:\Users\test\Music` {
		t.Fatalf("expected Music last, got %+v", folders[5])
	}
	if folders[2].Path != "" {
		t.Fatalf("expected unresolved folders to be empty, got %+v", folders[2])
	}
}

func TestParseResolvedFoldersHandlesBOM(t *testing.T) {
	output := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`[{"name":"Desktop","path":"C:\\Desktop"}]`)...)
	folders, err := parseResolvedFolders(output)
	if err != nil {
		t.Fatalf("parseResolvedFolders failed: %v", err)
	}
	if folders[0].Path != `C:\Desktop` {
		t.Fatalf("expected the Desktop path, got %+v", folders[0])
	}
}

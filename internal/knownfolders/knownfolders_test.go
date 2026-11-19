package knownfolders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setResolver(t *testing.T, resolver func() ([]Folder, error)) {
	t.Helper()
	previous := resolveFolders
	resolveFolders = resolver
	t.Cleanup(func() {
		resolveFolders = previous
	})
}

func TestInitializeWritesAndLoadsManifest(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(EnvConfigDir, configDir)

	desktop := filepath.Join(t.TempDir(), "Desktop")
	if err := os.Mkdir(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(t.TempDir(), "Downloads")

	setResolver(t, func() ([]Folder, error) {
		return []Folder{
			{Name: "Desktop", Path: desktop},
			{Name: "Downloads", Path: downloads},
		}, nil
	})

	manifest, err := Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if len(manifest.Folders) != 2 {
		t.Fatalf("expected two folders, got %d", len(manifest.Folders))
	}
	if !manifest.Folders[0].Exists {
		t.Fatalf("expected Desktop to exist")
	}
	if manifest.Folders[1].Exists {
		t.Fatalf("expected missing Downloads to be recorded as unavailable")
	}

	path, err := ManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != configDir {
		t.Fatalf("expected manifest in %q, got %q", configDir, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for _, expected := range []string{"# Underroot known folders", "Desktop", desktop, "Downloads"} {
		if !strings.Contains(string(contents), expected) {
			t.Fatalf("manifest missing %q:\n%s", expected, contents)
		}
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	existing := loaded.Existing()
	if len(existing) != 1 || existing[0].Name != "Desktop" {
		t.Fatalf("unexpected existing folders: %+v", existing)
	}
}

func TestLoadReResolvesMovedFolder(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(EnvConfigDir, configDir)

	original := filepath.Join(t.TempDir(), "Desktop")
	if err := os.Mkdir(original, 0o755); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "Desktop")
	if err := os.Mkdir(moved, 0o755); err != nil {
		t.Fatal(err)
	}

	var current []Folder
	setResolver(t, func() ([]Folder, error) {
		return current, nil
	})

	current = []Folder{{Name: "Desktop", Path: original}}
	if _, err := Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := os.Remove(original); err != nil {
		t.Fatalf("remove original folder: %v", err)
	}
	current = []Folder{{Name: "Desktop", Path: moved}}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded.Folders) != 1 || loaded.Folders[0].Path != moved || !loaded.Folders[0].Exists {
		t.Fatalf("expected re-resolved path %q, got %+v", moved, loaded.Folders)
	}
}

func TestLoadOrInitializeCreatesManifestOnFirstRun(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(EnvConfigDir, configDir)

	desktop := filepath.Join(t.TempDir(), "Desktop")
	if err := os.Mkdir(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	setResolver(t, func() ([]Folder, error) {
		return []Folder{{Name: "Desktop", Path: desktop}}, nil
	})

	manifest, err := LoadOrInitialize()
	if err != nil {
		t.Fatalf("LoadOrInitialize failed: %v", err)
	}
	if len(manifest.Existing()) != 1 {
		t.Fatalf("expected one existing folder, got %+v", manifest.Folders)
	}

	path, err := ManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected manifest at %q: %v", path, statErr)
	}
}

func TestInitializeRecordsMissingFolders(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	setResolver(t, func() ([]Folder, error) {
		return []Folder{{Name: "Desktop", Path: filepath.Join(t.TempDir(), "missing")}}, nil
	})

	manifest, err := Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if manifest.Folders[0].Exists {
		t.Fatalf("expected missing Desktop to be recorded as unavailable")
	}
	if len(manifest.Existing()) != 0 {
		t.Fatalf("expected no existing folders, got %+v", manifest.Existing())
	}
}

func TestInitializeRejectsEmptyResolution(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	setResolver(t, func() ([]Folder, error) {
		return nil, nil
	})

	if _, err := Initialize(); err == nil {
		t.Fatal("expected Initialize to reject an empty resolution")
	}
}

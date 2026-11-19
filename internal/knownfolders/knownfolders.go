// Package knownfolders resolves and persists the current user's standard
// Windows folders so the agent can treat them as verified environment facts.
package knownfolders

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ConfigDirName is the user-local Underroot configuration directory name.
	ConfigDirName = ".underroot"
	// ManifestFileName is the known-folders manifest written inside ConfigDirName.
	ManifestFileName = "known-folders.md"
	// EnvConfigDir overrides the configuration directory. It is primarily useful
	// for tests and portable installations.
	EnvConfigDir = "UNDERROOT_CONFIG_DIR"
)

// StandardFolders lists the folder names in the order the manifest records them.
var StandardFolders = []string{
	"Desktop",
	"Downloads",
	"Documents",
	"Pictures",
	"Videos",
	"Music",
}

// Folder is one resolved known folder and its verification metadata.
type Folder struct {
	Name       string
	Path       string
	VerifiedAt time.Time
	Exists     bool
}

// Manifest is the readable known-folders manifest.
type Manifest struct {
	GeneratedAt time.Time
	Folders     []Folder
}

// resolveFolders is a variable so tests can substitute a deterministic resolver.
var resolveFolders = platformResolveFolders

// ConfigDir returns the user-local Underroot configuration directory.
func ConfigDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvConfigDir)); override != "" {
		return override, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, ConfigDirName), nil
}

// ManifestPath returns the absolute path of the known-folders manifest.
func ManifestPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ManifestFileName), nil
}

// Initialize resolves every standard folder, verifies it against the
// filesystem, and writes a fresh manifest.
func Initialize() (*Manifest, error) {
	resolved, err := resolveFolders()
	if err != nil {
		return nil, fmt.Errorf("resolve known folders: %w", err)
	}

	if len(resolved) == 0 {
		return nil, errors.New("known folder resolution returned no folders")
	}

	now := time.Now().UTC()
	resolvedAny := false
	for i := range resolved {
		resolved[i].VerifiedAt = now
		resolved[i].Exists = isDirectory(resolved[i].Path)
		if strings.TrimSpace(resolved[i].Path) != "" {
			resolvedAny = true
		}
	}
	if !resolvedAny {
		return nil, errors.New("known folder resolution returned no paths")
	}

	manifest := &Manifest{
		GeneratedAt: now,
		Folders:     resolved,
	}
	if err := Write(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// Load reads the manifest and re-verifies every path. A folder that was
// previously present but is now missing is treated as stale, which forces a
// fresh platform resolution and a manifest rewrite.
func Load() (*Manifest, error) {
	path, err := ManifestPath()
	if err != nil {
		return nil, err
	}

	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read known folders manifest: %w", err)
	}

	manifest, err := parseManifest(string(contents))
	if err != nil {
		return nil, err
	}

	changed, stale := verifyManifest(manifest)
	if stale {
		return Initialize()
	}
	if changed {
		manifest.GeneratedAt = time.Now().UTC()
		if err := Write(manifest); err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

// LoadOrInitialize loads the manifest, or creates it on first run.
func LoadOrInitialize() (*Manifest, error) {
	manifest, err := Load()
	if err == nil {
		return manifest, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Initialize()
	}
	return nil, err
}

// Write persists the manifest atomically inside the configuration directory.
func Write(manifest *Manifest) error {
	if manifest == nil {
		return errors.New("manifest is nil")
	}

	path, err := ManifestPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return fmt.Errorf("create known folders config directory: %w", mkErr)
	}

	tmp, err := os.CreateTemp(dir, ".known-folders-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary manifest: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.WriteString(renderManifest(manifest)); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary manifest: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace known folders manifest: %w", err)
	}
	return nil
}

// Existing returns only folders that were verified as present.
func (m *Manifest) Existing() []Folder {
	if m == nil {
		return nil
	}
	existing := make([]Folder, 0, len(m.Folders))
	for _, folder := range m.Folders {
		if folder.Exists && strings.TrimSpace(folder.Path) != "" {
			existing = append(existing, folder)
		}
	}
	return existing
}

func parseManifest(content string) (*Manifest, error) {
	manifest := &Manifest{}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "- Generated:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "- Generated:"))
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				manifest.GeneratedAt = parsed
			}
			continue
		}

		if !strings.HasPrefix(line, "|") {
			continue
		}

		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 4 {
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}

		name := cells[0]
		if name == "" || name == "Folder" || strings.HasPrefix(name, "---") {
			continue
		}

		verifiedAt, _ := time.Parse(time.RFC3339, cells[2])
		manifest.Folders = append(manifest.Folders, Folder{
			Name:       name,
			Path:       cells[1],
			VerifiedAt: verifiedAt,
			Exists:     strings.EqualFold(cells[3], "yes"),
		})
	}

	if len(manifest.Folders) == 0 {
		return nil, errors.New("known folders manifest contains no folder entries")
	}
	return manifest, nil
}

func renderManifest(manifest *Manifest) string {
	var sb strings.Builder
	sb.WriteString("# Underroot known folders\n\n")
	sb.WriteString("Verified local folder paths for the current user. This file stores paths ")
	sb.WriteString("and verification metadata only; it never contains file listings or file contents.\n\n")
	fmt.Fprintf(&sb, "- Generated: %s\n", manifest.GeneratedAt.UTC().Format(time.RFC3339))
	sb.WriteString("- Manifest version: 1\n\n")
	sb.WriteString("| Folder | Path | Verified At | Exists |\n")
	sb.WriteString("| --- | --- | --- | --- |\n")
	for _, folder := range manifest.Folders {
		exists := "no"
		if folder.Exists {
			exists = "yes"
		}
		fmt.Fprintf(
			&sb,
			"| %s | %s | %s | %s |\n",
			folder.Name,
			folder.Path,
			folder.VerifiedAt.UTC().Format(time.RFC3339),
			exists,
		)
	}
	return sb.String()
}

// verifyManifest refreshes existence for every entry. It reports whether any
// existence value changed and whether a previously present folder went stale.
func verifyManifest(manifest *Manifest) (changed, stale bool) {
	now := time.Now().UTC()
	for i := range manifest.Folders {
		folder := &manifest.Folders[i]
		exists := isDirectory(folder.Path)
		if folder.Exists && !exists {
			stale = true
		}
		if folder.Exists != exists {
			changed = true
		}
		folder.Exists = exists
		folder.VerifiedAt = now
	}
	return changed, stale
}

func isDirectory(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && info.IsDir()
}

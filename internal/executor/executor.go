package executor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxDirectoryEntries = 20
	maxToolResultBytes  = 2048
)

// ErrPermissionDenied indicates a command failed due to missing permissions or required elevation.
var ErrPermissionDenied = errors.New("permission denied")

// IsPermissionDenied checks if the error indicates a permission or elevation failure.
func IsPermissionDenied(err error) bool {
	return errors.Is(err, ErrPermissionDenied)
}

func checkPermissionError(output string, baseErr error) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "unauthorizedaccessexception") ||
		strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "requires elevation") ||
		strings.Contains(lower, "administrator privilege") {
		return fmt.Errorf("%w: %s", ErrPermissionDenied, strings.TrimSpace(output))
	}
	return baseErr
}

func quotePowerShellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

// ExecuteScript runs a PowerShell command and returns its output.
func ExecuteScript(input string) (string, error) {
	// #nosec G204 -- executing the validated PowerShell script is this package's purpose.
	cmd := exec.Command("powershell", "-NoProfile", "-Command", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		permErr := checkPermissionError(outStr, fmt.Errorf("powershell execution failed: %w", err))
		return outStr, permErr
	}
	return string(out), nil
}

// DryRunScript parses the script and executes it with PowerShell's WhatIf preference enabled.
func DryRunScript(input string) (string, error) {
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$WhatIfPreference = $true
$source = %s
$sb = [scriptblock]::Create($source)
& $sb`, quotePowerShellPath(input))
	return ExecuteScript(script)
}

// List_Dir lists the contents of a directory.
func List_Dir(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	truncated := len(entries) > maxDirectoryEntries
	shownEntries := entries
	if truncated {
		shownEntries = entries[:maxDirectoryEntries]
	}

	type listEntry struct {
		Name      string `json:"name"`
		Directory bool   `json:"directory"`
		Length    int64  `json:"length"`
	}

	items := make([]listEntry, 0, len(shownEntries))
	for _, e := range shownEntries {
		var length int64
		if info, infoErr := e.Info(); infoErr == nil {
			length = info.Size()
		}
		items = append(items, listEntry{
			Name:      e.Name(),
			Directory: e.IsDir(),
			Length:    length,
		})
	}

	type listDirResponse struct {
		Path      string      `json:"path"`
		Entries   []listEntry `json:"entries"`
		Shown     int         `json:"shown"`
		Truncated bool        `json:"truncated"`
	}

	data, err := json.Marshal(listDirResponse{
		Path:      path,
		Entries:   items,
		Shown:     len(items),
		Truncated: truncated,
	})
	if err != nil {
		return "", err
	}
	return capToolResult(string(data), fmt.Sprintf(`{"path":%s,"entries":[],"shown":0,"truncated":true,"message":"directory result exceeded the tool output budget; use a narrower query"}`,
		quoteJSON(path))), nil
}

// Find_Entry finds one exact-name child entry of the supplied directory.
func Find_Entry(path, name, entryType string) (string, error) {
	if entryType != "" && entryType != "file" && entryType != "directory" && entryType != "any" {
		return "", fmt.Errorf("type must be file, directory, or any")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var found bool
	var fullName *string

	for _, e := range entries {
		if e.Name() != name {
			continue
		}
		if entryType == "file" && e.IsDir() {
			continue
		}
		if entryType == "directory" && !e.IsDir() {
			continue
		}
		found = true
		fn := filepath.Join(path, e.Name())
		fullName = &fn
		break
	}

	type findEntryResponse struct {
		Path     string  `json:"path"`
		Name     string  `json:"name"`
		Type     string  `json:"type"`
		Found    bool    `json:"found"`
		FullName *string `json:"full_name"`
	}

	data, err := json.Marshal(findEntryResponse{
		Path:     path,
		Name:     name,
		Type:     entryType,
		Found:    found,
		FullName: fullName,
	})
	if err != nil {
		return "", err
	}
	return capToolResult(string(data), fmt.Sprintf(`{"path":%s,"name":%s,"found":false,"message":"result exceeded the tool output budget"}`,
		quoteJSON(path), quoteJSON(name))), nil
}

// Search_Entries returns a bounded set of direct children whose names contain query, ignoring case.
func Search_Entries(path, query, entryType string, limit int) (string, error) {
	if entryType != "" && entryType != "file" && entryType != "directory" && entryType != "any" {
		return "", fmt.Errorf("type must be file, directory, or any")
	}
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}
	if limit <= 0 || limit > maxDirectoryEntries {
		limit = maxDirectoryEntries
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	type searchEntry struct {
		Name      string `json:"name"`
		Directory bool   `json:"directory"`
		FullName  string `json:"full_name"`
	}

	matches := make([]searchEntry, 0)
	lowerQuery := strings.ToLower(query)

	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name()), lowerQuery) {
			if entryType == "file" && e.IsDir() {
				continue
			}
			if entryType == "directory" && !e.IsDir() {
				continue
			}
			matches = append(matches, searchEntry{
				Name:      e.Name(),
				Directory: e.IsDir(),
				FullName:  filepath.Join(path, e.Name()),
			})
			if len(matches) >= limit {
				break
			}
		}
	}

	type searchResponse struct {
		Path    string        `json:"path"`
		Query   string        `json:"query"`
		Entries []searchEntry `json:"entries"`
		Shown   int           `json:"shown"`
	}

	data, err := json.Marshal(searchResponse{
		Path:    path,
		Query:   query,
		Entries: matches,
		Shown:   len(matches),
	})
	if err != nil {
		return "", err
	}
	return capToolResult(string(data), fmt.Sprintf(`{"path":%s,"query":%s,"entries":[],"shown":0,"truncated":true,"message":"result exceeded the tool output budget"}`,
		quoteJSON(path), quoteJSON(query))), nil
}

// Find_File finds a bounded number of files matching an extension directly under path.
func Find_File(path, extension string, limit int) (string, error) {
	if limit <= 0 || limit > maxDirectoryEntries {
		limit = maxDirectoryEntries
	}
	if extension == "" {
		return "", fmt.Errorf("extension is required")
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	type fileEntry struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Length   int64  `json:"length"`
	}

	files := make([]fileEntry, 0)
	lowerExt := strings.ToLower(extension)

	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), lowerExt) {
			var length int64
			if info, infoErr := e.Info(); infoErr == nil {
				length = info.Size()
			}
			files = append(files, fileEntry{
				Name:     e.Name(),
				FullName: filepath.Join(path, e.Name()),
				Length:   length,
			})
			if len(files) >= limit {
				break
			}
		}
	}

	type findFileResponse struct {
		Path      string      `json:"path"`
		Extension string      `json:"extension"`
		Files     []fileEntry `json:"files"`
		Shown     int         `json:"shown"`
	}

	data, err := json.Marshal(findFileResponse{
		Path:      path,
		Extension: extension,
		Files:     files,
		Shown:     len(files),
	})
	if err != nil {
		return "", err
	}
	return capToolResult(string(data), fmt.Sprintf(`{"path":%s,"files":[],"shown":0,"truncated":true,"message":"result exceeded the tool output budget"}`,
		quoteJSON(path))), nil
}

// Read_File reads the first 1000 lines of a file.
func Read_File(path string) (string, error) {
	// #nosec G304 -- reading the requested file path is this tool's purpose.
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && len(lines) < 1000 {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\r\n"), nil
}

// Exists reports whether a path exists.
func Exists(path string) (string, error) {
	_, err := os.Stat(path)
	exists := err == nil || !errors.Is(err, os.ErrNotExist)

	type existsResponse struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	}

	data, err := json.Marshal(existsResponse{
		Path:   path,
		Exists: exists,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UserDownloads returns the current Windows user's Downloads directory.
func UserDownloads() (string, error) {
	profile := os.Getenv("USERPROFILE")
	if strings.TrimSpace(profile) == "" {
		var err error
		profile, err = os.UserHomeDir()
		if err != nil || strings.TrimSpace(profile) == "" {
			return "", errors.New("USERPROFILE is not available")
		}
	}

	downloads := filepath.Join(profile, "Downloads")
	// #nosec G703 -- path traversal check on user profile downloads directory
	info, err := os.Stat(filepath.Clean(downloads))
	exists := err == nil && info.IsDir()

	type downloadsResponse struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	}

	data, err := json.Marshal(downloadsResponse{
		Path:   downloads,
		Exists: exists,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func quoteJSON(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", `\r`, "\n", `\n`).Replace(value) + `"`
}

func capToolResult(result, fallback string) string {
	if len(result) <= maxToolResultBytes {
		return result
	}
	return fallback
}

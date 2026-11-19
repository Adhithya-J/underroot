//go:build windows

package knownfolders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// resolveScript asks Windows' Shell API (SHGetKnownFolderPath) for each
// standard folder. Paths are never constructed from model output or guesses.
const resolveScript = `$ErrorActionPreference = 'Stop'
$source = @'
using System;
using System.Runtime.InteropServices;

public static class UnderrootKnownFolders
{
    [DllImport("shell32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, PreserveSig = false)]
    private static extern void SHGetKnownFolderPath(
        ref Guid rfid, uint dwFlags, IntPtr hToken, out string ppszPath);

    public static string Resolve(string guid)
    {
        Guid id = new Guid(guid);
        string path;
        SHGetKnownFolderPath(ref id, 0, IntPtr.Zero, out path);
        return path;
    }
}
'@
Add-Type -TypeDefinition $source -Language CSharp
$folders = [ordered]@{
    Desktop   = 'B4BFCC3A-DB2C-424C-B029-7FE99A87C641'
    Downloads = '374DE290-123F-4565-9164-39C4925E467B'
    Documents = 'FDD39AD0-238F-46AF-ADB4-6C85480369C7'
    Pictures  = '33E28130-4E1E-4676-835A-98395C3BC3BB'
    Videos    = '18989B1D-99B5-455B-841C-AB7C74E4DDFC'
    Music     = '4BD8D571-6D19-48D3-BE97-422220080E43'
}
$result = foreach ($name in $folders.Keys) {
    $path = $null
    try {
        $path = [UnderrootKnownFolders]::Resolve($folders[$name])
    } catch {
        $path = $null
    }
    [pscustomobject]@{ name = $name; path = $path }
}
$result | ConvertTo-Json -Compress -Depth 3
`

func platformResolveFolders() ([]Folder, error) {
	// #nosec G204 -- the script is a constant; no user input is interpolated.
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", resolveScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run known folder resolver: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return parseResolvedFolders(output)
}

type resolvedFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func parseResolvedFolders(output []byte) ([]Folder, error) {
	output = bytes.TrimPrefix(output, []byte{0xEF, 0xBB, 0xBF})
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		return nil, fmt.Errorf("known folder resolver returned no data")
	}

	var resolved []resolvedFolder
	if err := json.Unmarshal([]byte(trimmed), &resolved); err != nil {
		return nil, fmt.Errorf("decode known folder resolver output: %w", err)
	}

	paths := make(map[string]string, len(resolved))
	for _, folder := range resolved {
		if folder.Name == "" {
			continue
		}
		paths[folder.Name] = folder.Path
	}

	folders := make([]Folder, 0, len(StandardFolders))
	for _, name := range StandardFolders {
		folders = append(folders, Folder{Name: name, Path: paths[name]})
	}
	return folders, nil
}

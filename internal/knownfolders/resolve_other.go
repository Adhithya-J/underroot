//go:build !windows

package knownfolders

import "errors"

// platformResolveFolders reports that known-folder resolution is unavailable on
// non-Windows platforms. The product targets Windows, and the manifest can still
// be read elsewhere if one was copied from a Windows machine.
func platformResolveFolders() ([]Folder, error) {
	return nil, errors.New("known folder resolution requires Windows")
}

// Package storage resolves the app-local directory used to persist save files
// and other user data. It picks the executable's directory when running a real
// build, and falls back to the working directory when running via "go run"
// (where the binary lives in a temp build dir that disappears after exit).
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const savesDirName = "saves"

// AppSaveDir returns the local saves directory, creating it on demand. Errors
// during creation are logged to stderr; callers always receive a path string
// even if the directory could not be created.
func AppSaveDir() string {
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if !isTempExeDir(exeDir) {
			dir := filepath.Join(exeDir, savesDirName)
			if err := os.MkdirAll(dir, 0755); err == nil {
				return dir
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := filepath.Join(cwd, savesDirName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[storage] failed to create saves directory %q: %v\n", dir, err)
		}
		return dir
	}
	return savesDirName
}

// AppSavePath joins filename with the resolved save directory.
func AppSavePath(filename string) string {
	return filepath.Join(AppSaveDir(), filename)
}

// isTempExeDir reports whether dir looks like a Go temp build path (used when
// the binary is launched via "go run").
func isTempExeDir(dir string) bool {
	clean := filepath.Clean(dir)
	if strings.Contains(clean, string(filepath.Separator)+"go-build") {
		return true
	}
	if strings.HasPrefix(clean, filepath.Clean(os.TempDir())+string(filepath.Separator)) {
		return true
	}
	return false
}

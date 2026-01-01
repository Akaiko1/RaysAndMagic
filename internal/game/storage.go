package game

import (
	"os"
	"path/filepath"
	"strings"
)

const savesDirName = "saves"

// getAppSaveDir returns the local saves directory next to the app executable.
func getAppSaveDir() string {
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		// When running via "go run", the executable lives in a temp build dir.
		// In that case, prefer the current working directory so saves persist.
		if !isTempExeDir(exeDir) {
			dir := filepath.Join(exeDir, savesDirName)
			if err := os.MkdirAll(dir, 0755); err == nil {
				return dir
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := filepath.Join(cwd, savesDirName)
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	return savesDirName
}

func getAppSavePath(filename string) string {
	return filepath.Join(getAppSaveDir(), filename)
}

// isTempExeDir returns true when the executable directory looks like a Go temp build path.
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

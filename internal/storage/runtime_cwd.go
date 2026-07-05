package storage

import (
	"os"
	"path/filepath"
)

// EnsureRuntimeCWD makes the process run from the directory that holds
// config.yaml + assets/. macOS .app bundles seed + chdir into the shared
// writable per-user dir (so game and editor see the same files); bare
// binaries fall back to probing next to the executable. No-op for "go run"
// from the project root.
func EnsureRuntimeCWD() {
	if SetupBundleRuntime() {
		return
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	execDir := filepath.Dir(exe)
	if runtimeDir, ok := findRuntimeCWD(execDir, os.Stat); ok {
		_ = os.Chdir(runtimeDir)
	}
}

func findRuntimeCWD(execDir string, stat func(string) (os.FileInfo, error)) (string, bool) {
	candidates := []string{
		execDir,
		filepath.Join(execDir, ".."),
		// macOS .app bundle: Resources is sibling of MacOS.
		filepath.Join(execDir, "..", "Resources"),
	}
	for _, candidate := range candidates {
		if _, err := stat(filepath.Join(candidate, "config.yaml")); err == nil {
			return filepath.Clean(candidate), true
		}
	}
	return "", false
}

package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// RenderCacheDir captures a writable path before starting a worker, using the
// same root policy as saves: bundle data, standalone executable, then cwd.
func RenderCacheDir() string { return renderCacheDir(appDataRoots()) }

func renderCacheDir(roots []string) string {
	for _, root := range roots {
		dir := filepath.Join(root, ".render-cache")
		if err := os.MkdirAll(dir, 0755); err == nil {
			return dir
		} else {
			fmt.Fprintf(os.Stderr, "[storage] failed to create render cache %q: %v\n", dir, err)
		}
	}
	return ""
}

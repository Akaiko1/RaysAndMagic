package storage

import (
	"os"
	"path/filepath"
)

// RenderCacheDir captures a disposable writable path on the game owner before
// starting a worker. Bundles use their existing per-user root; local runs keep
// generated data in the project. It is separate from saves and profile data.
func RenderCacheDir() string {
	root := dataRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return ""
		}
	}
	return filepath.Join(root, ".render-cache")
}

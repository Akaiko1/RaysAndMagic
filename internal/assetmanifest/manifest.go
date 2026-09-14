// Package assetmanifest records the asset paths owned by the current bundle.
// Storage reconciles this inventory; basename resolution uses its ownership
// when legacy or custom files collide with a currently shipped resource.
package assetmanifest

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const FileName = ".seed_manifest"

// Manifest maps asset-relative paths to their shipped SHA-256 hashes. The
// original maps-only format is a valid subset of this complete inventory.
type Manifest map[string]string

func Load(dataDir string) Manifest {
	m := Manifest{}
	if b, err := os.ReadFile(filepath.Join(dataDir, FileName)); err == nil {
		if json.Unmarshal(b, &m) != nil || m == nil {
			return Manifest{}
		}
	}
	return m
}

func (m Manifest) Save(dataDir string) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, FileName), b, 0644)
}

// ContainsRuntimePath accepts the assets/... paths used by runtime loaders.
func (m Manifest) ContainsRuntimePath(runtimePath string) bool {
	rel, err := filepath.Rel("assets", runtimePath)
	return err == nil && filepath.IsLocal(rel) && m[filepath.ToSlash(rel)] != ""
}

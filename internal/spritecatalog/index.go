// Package spritecatalog resolves source PNGs without a graphics dependency.
package spritecatalog

import (
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"ugataima/internal/assetmanifest"
)

// spriteBaseDirs are the roots indexed by basename (recursively). Each maps to
// the placeholder type used when a named sprite is missing. floor/ and sky/ are
// loaded separately via resolveNamedPNG and intentionally omitted here.
var spriteBaseDirs = []struct{ dir, typ string }{
	{"assets/sprites/mobs", "npc_mob"},
	{"assets/sprites/characters", "npc_mob"},
	{"assets/sprites/environment", "environment"},
	{"assets/sprites/interface", "interface"},
}

// isIgnoredSpriteDir reports folders excluded from the sprite index: archives
// (any case) and any name starting with "_" or "." - a convention to park
// unused/duplicate art in the tree without it shadowing live sprites.
func isIgnoredSpriteDir(name string) bool {
	return strings.EqualFold(name, "archive") ||
		strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// BuildIndex walks the sprite roots recursively (skipping ignored dirs)
// and returns basename->path and basename->placeholder-type maps. Sprites may
// therefore be grouped into arbitrary subfolders; basenames must be unique
// across the whole tree. On a seeded install a current shipped path wins over
// untracked legacy/custom duplicates; equal ownership keeps root/lexical order.
// Shared by load-time validation, SpriteManager and the package-level resolver.
func BuildIndex() (paths, dirType map[string]string) {
	shipped := assetmanifest.Load(".")
	paths = make(map[string]string)
	dirType = make(map[string]string)
	for _, root := range spriteBaseDirs {
		_ = filepath.WalkDir(root.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // missing root (e.g. tests run outside the repo) - skip
			}
			if d.IsDir() {
				if isIgnoredSpriteDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".png" {
				return nil
			}
			base := strings.TrimSuffix(d.Name(), ".png")
			if existing, dup := paths[base]; dup {
				keep := existing
				if shipped.ContainsRuntimePath(path) && !shipped.ContainsRuntimePath(existing) {
					keep = path
					paths[base], dirType[base] = path, root.typ
				}
				log.Printf("sprite index: duplicate basename %q (%q vs %q); keeping %q", base, existing, path, keep)
				return nil
			}
			paths[base] = path
			dirType[base] = root.typ
			return nil
		})
	}
	return paths, dirType
}

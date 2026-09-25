package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSpellConfigValidatesTraversalFlags(t *testing.T) {
	previous := GlobalSpells
	t.Cleanup(func() { GlobalSpells = previous })
	for _, tc := range []struct {
		name, flags, want string
	}{
		{"fly", "fly: true\n    terrain_passage: true", ""},
		{"passage", "terrain_passage: true", ""},
		{"missing_passage", "fly: true", "fly requires terrain_passage"},
		{"instant_passage", "terrain_passage: true\n    duration: 0", "terrain_passage requires a timed utility spell"},
		{"non_utility", "terrain_passage: true\n    is_utility: false", "terrain_passage requires a timed utility spell"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Avoid duplicate YAML keys so validation, rather than parsing, fails.
			utility, duration := true, 10
			flags := tc.flags
			if strings.Contains(flags, "duration: 0") {
				duration = 0
				flags = "terrain_passage: true"
			}
			if strings.Contains(flags, "is_utility: false") {
				utility = false
				flags = "terrain_passage: true"
			}
			source := fmt.Sprintf("spells:\n  test:\n    category: buff\n    is_utility: %v\n    duration: %d\n    %s\n", utility, duration, flags)
			path := filepath.Join(t.TempDir(), "spells.yaml")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSpellConfig(path)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("load error = %v, want %q", err, tc.want)
			}
		})
	}
}

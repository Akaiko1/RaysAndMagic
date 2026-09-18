package monster

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWalkableTileOverridesConfig(t *testing.T) {
	previous := MonsterConfig
	t.Cleanup(func() { MonsterConfig = previous })
	for _, tc := range []struct {
		name, field string
		want        []string
		rejected    bool
	}{
		{name: "omitted"},
		{name: "empty", field: "walkable_tile_overrides: []", want: []string{}},
		{name: "blocked_tile", field: "walkable_tile_overrides: [desert_dune]", want: []string{"desert_dune"}},
		{name: "legacy", field: "habitat_preferences: [desert_dune]", rejected: true},
		{name: "legacy_empty", field: "habitat_preferences: []", rejected: true},
		{name: "both", field: "habitat_preferences: [empty]\n    walkable_tile_overrides: [desert_dune]", rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "monsters.yaml")
			data := "monsters:\n  walker:\n    name: Walker\n    size_class: person\n    " + tc.field + "\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadMonsterConfig(path)
			if tc.rejected {
				if err == nil || !strings.Contains(err.Error(), "walkable_tile_overrides") || !strings.Contains(err.Error(), "walker") {
					t.Fatalf("legacy field needs an actionable error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Monsters["walker"].WalkableTileOverrides; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("overrides = %v, want %v", got, tc.want)
			}
		})
	}
}

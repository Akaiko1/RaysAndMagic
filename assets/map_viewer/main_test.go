package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/config"
)

// Spells page must group purely BY SCHOOL (no Battle/Utility split) and order
// each school's spells by ascending level.
func TestBuildSpellCards_BySchoolByLevel(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	cards := buildSpellCards()
	if len(cards) == 0 {
		t.Fatal("no spell cards built")
	}
	lastLevelInSection := map[string]int{}
	for _, c := range cards {
		if strings.Contains(c.section, "Battle") || strings.Contains(c.section, "Utility") {
			t.Fatalf("section %q must be a school name, not a battle/utility split", c.section)
		}
		def, ok := config.GlobalSpells.Spells[c.key]
		if !ok || def == nil {
			continue
		}
		if prev, seen := lastLevelInSection[c.section]; seen && def.Level < prev {
			t.Errorf("section %q not level-ascending: %s (lvl %d) after lvl %d", c.section, c.key, def.Level, prev)
		}
		lastLevelInSection[c.section] = def.Level
	}
}

func TestFindRuntimeCWD(t *testing.T) {
	tests := []struct {
		name         string
		execDir      string
		existingPath string
		want         string
		wantOK       bool
	}{
		{
			name:         "binary directory",
			execDir:      filepath.Join("game", "bin"),
			existingPath: filepath.Join("game", "bin", "config.yaml"),
			want:         filepath.Join("game", "bin"),
			wantOK:       true,
		},
		{
			name:         "parent directory",
			execDir:      filepath.Join("game", "bin"),
			existingPath: filepath.Join("game", "config.yaml"),
			want:         "game",
			wantOK:       true,
		},
		{
			name:         "mac app resources",
			execDir:      filepath.Join("Game.app", "Contents", "MacOS"),
			existingPath: filepath.Join("Game.app", "Contents", "Resources", "config.yaml"),
			want:         filepath.Join("Game.app", "Contents", "Resources"),
			wantOK:       true,
		},
		{
			name:    "missing config",
			execDir: filepath.Join("game", "bin"),
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := findRuntimeCWD(tc.execDir, fakeConfigStat(tc.existingPath))
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if filepath.Clean(got) != filepath.Clean(tc.want) {
				t.Fatalf("expected %s, got %s", filepath.Clean(tc.want), filepath.Clean(got))
			}
		})
	}
}

func fakeConfigStat(existingPath string) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if existingPath != "" && filepath.Clean(path) == filepath.Clean(existingPath) {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
}

// TestSpellCard_SharesMechanicsWithGame verifies the editor's spell card pulls
// its mechanics from spells.EffectLines (the same source as the in-game
// tooltip), so previously-missing fields (stun chance, buff bonuses, charm,
// zone, revive…) now appear and can't drift from the game.
func TestSpellCard_SharesMechanicsWithGame(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	rowsFor := func(key string) string {
		for _, c := range buildSpellCards() {
			if c.key == key {
				return strings.Join(c.tooltipRows, "\n")
			}
		}
		t.Fatalf("no card for %q", key)
		return ""
	}
	want := map[string]string{
		"psychic_shock": "Stun chance: 10%",
		"stone_skin":    "Party takes -6 damage per hit",
		"heroism":       "Party attacks deal +10 damage",
		"charm":         "Pacifies",
		"stun":          "Stuns every monster within 3.0 tiles",
		"raise_dead":    "Revives a fallen ally to 25% HP",
	}
	for key, sub := range want {
		if got := rowsFor(key); !strings.Contains(got, sub) {
			t.Errorf("editor %s card missing %q. rows:\n%s", key, sub, got)
		}
	}
	// Charm/Disintegrate are deals_no_damage → no damage row in the editor either.
	for _, key := range []string{"charm", "disintegrate"} {
		if got := rowsFor(key); strings.Contains(got, "Base damage") {
			t.Errorf("editor %s card shows damage but it's deals_no_damage:\n%s", key, got)
		}
	}
}

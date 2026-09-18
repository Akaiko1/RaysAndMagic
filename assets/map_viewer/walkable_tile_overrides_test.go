package main

import (
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestMobInfoWalkableTileOverrides(t *testing.T) {
	if _, err := config.LoadSpellConfig("../spells.yaml"); err != nil {
		t.Fatal(err)
	}
	monster.MustLoadMonsterConfig("../monsters.yaml")
	for _, key := range []string{"bandit", "forest_spider", "desert_dervish", "dragon"} {
		t.Run(key, func(t *testing.T) {
			def := monster.MonsterConfig.Monsters[key]
			var lines []string
			for _, line := range buildMobInfo(key, def) {
				lines = append(lines, line.text)
			}
			text := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
			if strings.Contains(text, "Habitat:") {
				t.Fatal("editor still describes movement permission as habitat")
			}
			if len(def.WalkableTileOverrides) == 0 {
				if strings.Contains(text, "Walkable tile overrides:") {
					t.Fatal("editor shows an empty override row")
				}
			} else if want := "Walkable tile overrides: " + strings.Join(def.WalkableTileOverrides, ", "); !strings.Contains(text, want) {
				t.Fatalf("editor missing %q in %q", want, text)
			}
		})
	}
}

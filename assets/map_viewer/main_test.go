package main

import (
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
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

func TestMobInfo_GoldDragonShowsOnlyMonsterFacingStun(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))

	rowsFor := func(key string) string {
		def, ok := monster.MonsterConfig.Monsters[key]
		if !ok {
			t.Fatalf("monster %q missing", key)
		}
		lines := buildMobInfo(key, def)
		rows := make([]string, 0, len(lines))
		for _, line := range lines {
			rows = append(rows, line.text)
		}
		return strings.Join(rows, "\n")
	}

	for key, wantStun := range map[string]string{
		"dragon_gold":       "Stun: 10% (4s / 2 turns)",
		"elder_dragon_gold": "Stun: 10% (4s / 2 turns)",
	} {
		got := rowsFor(key)
		for _, want := range []string{"Ranged spell: Lightning Bolt (air)", wantStun} {
			if !strings.Contains(got, want) {
				t.Errorf("editor mob %s missing %q. rows:\n%s", key, want, got)
			}
		}
		if strings.Contains(got, "Stun on hit: 20% (2s / 1 turns)") {
			t.Errorf("editor mob %s shows spell stun as monster-facing stun. rows:\n%s", key, got)
		}
	}
}

// TestSpellCard_SharesMechanicsWithGame verifies the editor's spell card pulls
// its mechanics from spells.EffectLines (the same source as the in-game
// tooltip), so previously-missing fields (stun chance, buff bonuses, charm,
// zone, revive...) now appear and can't drift from the game.
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
		"stone_skin":    "Party takes -4 to -10 damage per hit by mastery",
		"heroism":       "Party physical attacks deal +3 to +10 damage by mastery",
		"charm":         "Pacifies",
		"stun":          "Stuns every monster within 3.0 tiles",
		"raise_dead":    "Revives a fallen ally to 25% HP",
	}
	for key, sub := range want {
		if got := rowsFor(key); !strings.Contains(got, sub) {
			t.Errorf("editor %s card missing %q. rows:\n%s", key, sub, got)
		}
	}
	// Charm/Disintegrate are deals_no_damage -> no damage row in the editor either.
	for _, key := range []string{"charm", "disintegrate"} {
		if got := rowsFor(key); strings.Contains(got, "Base damage") {
			t.Errorf("editor %s card shows damage but it's deals_no_damage:\n%s", key, got)
		}
	}
}

func TestMobInfo_UsesMonsterCombatEffectLines(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))

	rowsFor := func(key string) string {
		def, ok := monster.MonsterConfig.Monsters[key]
		if !ok {
			t.Fatalf("monster %q missing", key)
		}
		lines := buildMobInfo(key, def)
		rows := make([]string, 0, len(lines))
		for _, line := range lines {
			rows = append(rows, line.text)
		}
		return strings.Join(rows, "\n")
	}

	want := map[string][]string{
		"archmage": {"Ranged spell: Fireball (fire)", "Projectile AoE: whole party on hit"},
		"dragon":   {"Ranged spell: Fire Bolt (fire)", "Dragon Breath: 33% fire attack to whole party"},
	}
	for key, subs := range want {
		got := rowsFor(key)
		for _, sub := range subs {
			if !strings.Contains(got, sub) {
				t.Errorf("editor mob %s missing %q. rows:\n%s", key, sub, got)
			}
		}
	}
}

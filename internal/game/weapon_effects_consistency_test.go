package game

// Verifies that every weapon special-effect declared in weapons.yaml is
// surfaced consistently by:
//  1. config.WeaponDefinitionConfig.EffectLines (SSoT)
//  2. weaponEffectsSummary (in-game compare-tooltip)
//
// The map-viewer card and the main in-game tooltip both call EffectLines
// directly, so structural consistency there is guaranteed by code - these
// tests focus on the join-and-render layer that could silently drift.

import (
	"strings"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func loadWeaponConfigsForTest(t *testing.T) {
	t.Helper()
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
}

// TestWeaponEffectLines_ExpectedFieldsPerWeapon - locks the canonical
// shape of EffectLines for known-effect weapons. If a future refactor
// drops a field from EffectLines, every consumer downstream loses it
// silently, so we assert per-weapon here.
func TestWeaponEffectLines_ExpectedFieldsPerWeapon(t *testing.T) {
	loadWeaponConfigsForTest(t)

	cases := []struct {
		weaponKey string
		mustHave  []string // substrings every EffectLines output must contain
	}{
		// Steel Mace - uncommon stun proc. Crit is a base attribute now,
		// rendered by each consumer separately (not via EffectLines).
		{"steel_mace", []string{"Stun Chance:"}},
		// Bow of Hellfire - dark damage type, AoE, max projectiles
		// (no bonus_vs - that's Elven Bow).
		{"bow_of_hellfire", []string{"Damage Type: Dark", "AoE radius:", "Max Airborne:"}},
		// Elven Bow - bonus vs dragon, physical so no damage-type line.
		{"elven_bow", []string{"Bonus vs Dragon"}},
		// Alien Blaster - spirit damage type + disintegrate.
		{"alien_blaster", []string{"Damage Type: Spirit", "Disintegrate Chance:"}},
	}

	for _, tc := range cases {
		def, exists := config.GetWeaponDefinition(tc.weaponKey)
		if !exists || def == nil {
			t.Errorf("weapon %q missing from weapons.yaml", tc.weaponKey)
			continue
		}
		lines := def.EffectLines()
		if len(lines) == 0 {
			t.Errorf("weapon %q has zero EffectLines - expected %v", tc.weaponKey, tc.mustHave)
			continue
		}
		joined := strings.Join(lines, "\n")
		for _, want := range tc.mustHave {
			if !strings.Contains(joined, want) {
				t.Errorf("weapon %q EffectLines missing %q:\n%s", tc.weaponKey, want, joined)
			}
		}
	}
}

// TestWeaponEffectsSummary_DelegatesToEffectLines - the compare-tooltip
// (joined comma form) MUST contain every line from EffectLines, otherwise
// the comparison panel hides effects from the main tooltip.
func TestWeaponEffectsSummary_DelegatesToEffectLines(t *testing.T) {
	loadWeaponConfigsForTest(t)

	for _, weaponKey := range []string{"steel_mace", "bow_of_hellfire", "elven_bow", "alien_blaster"} {
		def, exists := config.GetWeaponDefinition(weaponKey)
		if !exists || def == nil {
			t.Fatalf("weapon %q missing", weaponKey)
		}
		item := items.CreateWeaponFromYAML(weaponKey)
		summary := weaponEffectsSummary(item)
		for _, line := range def.EffectLines() {
			if !strings.Contains(summary, line) {
				t.Errorf("weapon %q compare-tooltip lost effect line %q:\n  summary=%q",
					weaponKey, line, summary)
			}
		}
	}
}

// TestWeaponEffectLines_OrderIsStable - map iteration over BonusVs is
// non-deterministic in Go, but EffectLines sorts keys. Two calls must
// produce byte-identical output so tests and golden files don't flake.
func TestWeaponEffectLines_OrderIsStable(t *testing.T) {
	loadWeaponConfigsForTest(t)
	def, _ := config.GetWeaponDefinition("bow_of_hellfire")
	if def == nil {
		t.Fatal("bow_of_hellfire missing")
	}
	first := strings.Join(def.EffectLines(), "|")
	for i := 0; i < 50; i++ {
		got := strings.Join(def.EffectLines(), "|")
		if got != first {
			t.Fatalf("EffectLines drifted between calls:\n  first=%q\n  got  =%q", first, got)
		}
	}
}

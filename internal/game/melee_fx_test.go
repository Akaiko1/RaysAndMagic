package game

import (
	"testing"

	"ugataima/internal/config"
)

// TestMeleeFxKind: every weapon category maps to its own swing flavor so the FX
// differ per type (sword vs axe vs mace vs dagger vs spear).
func TestMeleeFxKind(t *testing.T) {
	cases := map[string]string{
		"sword":  "slash",
		"axe":    "chop",
		"mace":   "smash",
		"dagger": "stab",
		"spear":  "lunge",
		"":       "slash",
	}
	for cat, want := range cases {
		got := meleeFxKind(&config.WeaponDefinitionConfig{Category: cat})
		if got != want {
			t.Errorf("meleeFxKind(%q) = %q, want %q", cat, got, want)
		}
	}
	if got := meleeFxKind(nil); got != "slash" {
		t.Errorf("meleeFxKind(nil) = %q, want slash", got)
	}
}

// TestSeedFromID: stable, non-negative, and varies by input so each slash's
// particle pattern differs without per-frame randomness.
func TestSeedFromID(t *testing.T) {
	if seedFromID("slash_7") != seedFromID("slash_7") {
		t.Error("seedFromID must be deterministic")
	}
	if seedFromID("slash_7") == seedFromID("slash_8") {
		t.Error("different IDs should usually yield different seeds")
	}
	if seedFromID("slash_999999") < 0 {
		t.Error("seed must be non-negative (used as a hash input)")
	}
	if seedFromID("") < 0 {
		t.Error("empty ID must not produce a negative seed")
	}
}

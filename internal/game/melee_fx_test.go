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

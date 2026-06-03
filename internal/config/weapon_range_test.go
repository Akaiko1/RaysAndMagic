package config

import (
	"math"
	"testing"
)

// TestMeleeWeaponsHaveSlashGraphics: every melee weapon must carry a graphics
// block, otherwise createMeleeAttack spawns no slash effect (no swing FX).
func TestMeleeWeaponsHaveSlashGraphics(t *testing.T) {
	cfg, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	for key, def := range cfg.Weapons {
		if def.Melee == nil {
			continue // ranged weapon, uses projectile FX
		}
		if def.Graphics == nil {
			t.Errorf("melee weapon %q has no graphics block — no slash effect would spawn", key)
		}
	}
}

// TestRangedWeaponTravelsStatedRange: a ranged weapon's projectile must actually
// travel its configured range_tiles (lifetime × speed), and the display `range`
// field must match range_tiles — so "6 tiles" really reaches 6 tiles.
func TestRangedWeaponTravelsStatedRange(t *testing.T) {
	cfg, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	const tile = 64.0
	for key, def := range cfg.Weapons {
		p := def.Physics
		if p == nil || p.RangeTiles <= 0 || p.SpeedTiles <= 0 {
			continue // melee weapon, no projectile
		}
		// Actual travel = per-frame speed × lifetime frames (tileSize cancels out).
		travel := p.GetSpeedPixels(tile) * float64(p.GetLifetimeFrames()) / tile
		if math.Abs(travel-p.RangeTiles) > 0.1 {
			t.Errorf("%s travels %.3f tiles, want range_tiles %.1f (>0.1 off)", key, travel, p.RangeTiles)
		}
		// The display `range` (used for melee/ranged dispatch and player expectation)
		// must equal the physics range so there's a single true distance.
		if def.Range > 0 && math.Abs(float64(def.Range)-p.RangeTiles) > 0.01 {
			t.Errorf("%s: display range=%d but range_tiles=%.1f (mismatch)", key, def.Range, p.RangeTiles)
		}
	}
}

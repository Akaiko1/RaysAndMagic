package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Monster bodies and projectile hitboxes are closed class tables. The shipped
// tables load complete and capped; every malformed variant fails the boot.
func TestCollisionClassTablesLoadValidation(t *testing.T) {
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range ActorSizeClassNames() {
		if body, ok := cfg.MonsterBodyTiles(class); !ok || body > MaxMonsterBodyTiles {
			t.Fatalf("shipped body %s=%v ok=%v", class, body, ok)
		}
	}
	for _, hitbox := range projectileHitboxNames {
		if cfg.ProjectileHitboxTiles(&ProjectilePhysicsConfig{Hitbox: hitbox}) <= 0 {
			t.Fatalf("shipped hitbox %s missing", hitbox)
		}
	}
	for _, tt := range []struct{ name, old, replacement string }{
		{"body over cap", "huge: 0.75      # 48 px", "huge: 0.8"},
		{"zero body", "small: 0.375    # 24 px - rat, bat, pixie, wildlife", "small: 0"},
		{"nan body", "small: 0.375    # 24 px - rat, bat, pixie, wildlife", "small: .nan"},
		{"infinite body", "small: 0.375    # 24 px - rat, bat, pixie, wildlife", "small: .inf"},
		{"nan hitbox", "standard: 0.5   # arrows, bolts, bullets", "standard: .nan"},
		{"infinite hitbox", "standard: 0.5   # arrows, bolts, bullets", "standard: .inf"},
		{"missing body", "    huge: 0.75      # 48 px\n", ""},
		{"unknown body", "    small: 0.375", "    giant: 0.5\n    small: 0.375"},
		{"zero hitbox", "standard: 0.5   # arrows, bolts, bullets", "standard: 0"},
		{"missing hitbox", "    wide: 1.0       # large shots (fireball, swarm); stays in its own lane in turn-based\n", ""},
		{"unknown hitbox", "    standard: 0.5", "    narrow: 0.3\n    standard: 0.5"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed := strings.Replace(string(data), tt.old, tt.replacement, 1)
			if changed == string(data) {
				t.Fatal("fixture setting absent")
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil {
				t.Fatal("malformed collision table accepted")
			}
		})
	}
}

// Spells and projectile weapons name a hitbox class; the retired raw size key
// and anything outside the set fail at load.
func TestProjectileHitboxAuthoring(t *testing.T) {
	removed := 0.5
	for _, tt := range []struct {
		name    string
		physics ProjectilePhysicsConfig
		valid   bool
	}{
		{"standard", ProjectilePhysicsConfig{SpeedTiles: 10, RangeTiles: 6, Hitbox: "standard"}, true},
		{"wide", ProjectilePhysicsConfig{SpeedTiles: 10, RangeTiles: 6, Hitbox: "wide"}, true},
		{"missing", ProjectilePhysicsConfig{SpeedTiles: 10, RangeTiles: 6}, false},
		{"unknown", ProjectilePhysicsConfig{SpeedTiles: 10, RangeTiles: 6, Hitbox: "huge"}, false},
		{"raw size", ProjectilePhysicsConfig{SpeedTiles: 10, RangeTiles: 6, Hitbox: "standard", RemovedCollisionSizeTiles: &removed}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			physics := tt.physics
			spell := &SpellDefinitionConfig{CooldownSeconds: 1, IsProjectile: true, Physics: &physics}
			if err := validateSpellAuthoring(&SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{"bolt": spell}}); (err == nil) != tt.valid {
				t.Fatalf("spell: err=%v want valid=%v", err, tt.valid)
			}
			weapon := &WeaponDefinitionConfig{
				Name: "Test Bow", Category: "bow", DamageType: "physical", Range: RangedWeaponMinRangeTiles,
				Physics:  &physics,
				Graphics: &WeaponGraphicsConfig{BaseSize: 1, MaxSize: 1, MinSize: 1},
			}
			if err := validateWeaponConfig(&WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"bow": weapon}}); (err == nil) != tt.valid {
				t.Fatalf("weapon: err=%v want valid=%v", err, tt.valid)
			}
		})
	}
}

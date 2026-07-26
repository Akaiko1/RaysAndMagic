package spells

import (
	"math"
	"testing"

	"ugataima/internal/config"
)

func TestCalculateSpellDamageByID(t *testing.T) {
	base, bonus, total := CalculateSpellDamageByID("fireball", 10)
	if total < base || total < bonus {
		t.Errorf("Total damage should be at least as large as base or bonus: base=%d, bonus=%d, total=%d", base, bonus, total)
	}
}

func TestCalculateHealingAmountByID(t *testing.T) {
	base, bonus, total := CalculateHealingAmountByID("heal", 10)
	if total < base || total < bonus {
		t.Errorf("Total healing should be at least as large as base or bonus: base=%d, bonus=%d, total=%d", base, bonus, total)
	}
}

// The colour comes from the spell's authored graphics block, and an unknown
// spell must report an error rather than hand back an invisible black default.
func TestGetProjectileColor(t *testing.T) {
	// The colour lives in config.yaml's spell graphics, so GlobalConfig must be
	// primed; TestMain loads only spells.yaml.
	if _, err := config.LoadConfig("../../config.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	color, err := GetProjectileColor("fireball")
	if err != nil {
		t.Fatalf("fireball colour: %v", err)
	}
	if color == [3]int{} {
		t.Error("fireball resolved to black - authored colour was lost")
	}
	for i, c := range color {
		if c < 0 || c > 255 {
			t.Errorf("channel %d = %d, outside 0-255", i, c)
		}
	}
	if _, err := GetProjectileColor("no_such_spell"); err == nil {
		t.Error("an unknown spell must not resolve to a colour")
	}
}

func TestCreateProjectileUsesPhysicsConfig(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}

	physics, err := cfg.GetSpellConfig("fireball")
	if err != nil {
		t.Fatalf("get fireball physics: %v", err)
	}

	projectile, err := NewCastingSystem(cfg).CreateProjectile("fireball", 10, 20, 0)
	if err != nil {
		t.Fatalf("create projectile: %v", err)
	}

	expectedVelocity := physics.GetSpeedPixels(cfg.GetTileSize())
	if math.Abs(projectile.VelX-expectedVelocity) > 0.0001 {
		t.Fatalf("expected VelX %.4f from physics, got %.4f", expectedVelocity, projectile.VelX)
	}
	if projectile.VelY != 0 {
		t.Fatalf("expected VelY 0 at angle 0, got %.4f", projectile.VelY)
	}
	if projectile.LifeTime != physics.GetLifetimeFrames() {
		t.Fatalf("expected lifetime %d from physics, got %d", physics.GetLifetimeFrames(), projectile.LifeTime)
	}
	if projectile.Size != 16 {
		t.Fatalf("expected fireball projectile size 16, got %d", projectile.Size)
	}
}

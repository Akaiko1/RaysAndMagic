package spells

import (
	"math"
	"testing"

	"ugataima/internal/config"
)

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
	def, err := GetSpellDefinitionByID("fireball")
	if err != nil {
		t.Fatalf("get fireball definition: %v", err)
	}
	if def.ProjectileSize <= 0 {
		t.Fatalf("fireball must author a projectile_size, got %d", def.ProjectileSize)
	}
	if projectile.Size != def.ProjectileSize {
		t.Fatalf("expected projectile size %d from the definition, got %d", def.ProjectileSize, projectile.Size)
	}
}

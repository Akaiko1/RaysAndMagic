package game

import (
	"testing"

	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// TestAoESplash_DamagesNearbyMonsters_NotFarOnes verifies the splash helper
// hits exactly the monsters within the spell's radius, applies their armor
// reduction per-target, and never re-damages the primary target.
func TestAoESplash_DamagesNearbyMonsters_NotFarOnes(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tileSize := float64(cs.game.config.GetTileSize())

	primary := &monsterPkg.Monster3D{
		Name: "Primary", X: 0, Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	near := &monsterPkg.Monster3D{
		Name: "Near", X: tileSize, Y: 0, // 1 tile away — inside 2-tile radius
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	edge := &monsterPkg.Monster3D{
		Name: "Edge", X: 0, Y: tileSize * 1.9, // just inside 2-tile radius
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	far := &monsterPkg.Monster3D{
		Name: "Far", X: tileSize * 3, Y: 0, // 3 tiles away — outside
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, near, edge, far}

	primaryHPBefore := primary.HitPoints

	cs.applyAoeSplash(primary, 20, "fire", monsterPkg.DamageFire, "Fireball", 2.0, 0)

	if primary.HitPoints != primaryHPBefore {
		t.Errorf("primary should not be re-damaged by splash, got HP %d (was %d)", primary.HitPoints, primaryHPBefore)
	}
	if near.HitPoints != 80 {
		t.Errorf("near monster expected 80 HP after 20-damage splash, got %d", near.HitPoints)
	}
	if edge.HitPoints != 80 {
		t.Errorf("edge monster expected 80 HP after 20-damage splash, got %d", edge.HitPoints)
	}
	if far.HitPoints != 100 {
		t.Errorf("far monster should NOT be hit by splash, got HP %d", far.HitPoints)
	}
}

// TestAoESplash_SkipsDeadMonsters ensures already-dead monsters aren't
// re-damaged (would produce nonsensical combat log spam and double-XP).
func TestAoESplash_SkipsDeadMonsters(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tileSize := float64(cs.game.config.GetTileSize())

	primary := &monsterPkg.Monster3D{
		Name: "Primary", X: 0, Y: 0,
		HitPoints: 100, MaxHitPoints: 100,
	}
	corpse := &monsterPkg.Monster3D{
		Name: "Corpse", X: tileSize, Y: 0,
		HitPoints: 0, MaxHitPoints: 100,
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, corpse}

	cs.applyAoeSplash(primary, 20, "fire", monsterPkg.DamageFire, "Fireball", 2.0, 0)

	if corpse.HitPoints != 0 {
		t.Errorf("dead monster should remain at 0 HP, got %d", corpse.HitPoints)
	}
}

// TestAoESplash_ZeroRadius_NoOp verifies single-target spells (no
// aoe_radius_tiles) leave bystanders alone.
func TestAoESplash_ZeroRadius_NoOp(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tileSize := float64(cs.game.config.GetTileSize())

	primary := &monsterPkg.Monster3D{Name: "Primary", X: 0, Y: 0, HitPoints: 100, MaxHitPoints: 100}
	bystander := &monsterPkg.Monster3D{Name: "Bystander", X: tileSize * 0.5, Y: 0, HitPoints: 100, MaxHitPoints: 100}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, bystander}

	cs.applyAoeSplash(primary, 50, "fire", monsterPkg.DamageFire, "Firebolt", 0, 0)

	if bystander.HitPoints != 100 {
		t.Errorf("bystander should be untouched when AoE radius is 0, got HP %d", bystander.HitPoints)
	}
}

// TestFireballYAML_AdvertisesAoE guards against silently dropping the AoE
// field from fireball's YAML — combat and tooltip both read it via
// spells.SpellDefinition.AoeRadiusTiles, so a missing field would quietly
// revert Fireball to single-target.
func TestFireballYAML_AdvertisesAoE(t *testing.T) {
	_ = newTestCombatSystemWithConfig(t) // loads spells.yaml as a side effect
	def, err := spells.GetSpellDefinitionByID("fireball")
	if err != nil {
		t.Fatalf("fireball spell missing: %v", err)
	}
	if def.AoeRadiusTiles <= 0 {
		t.Fatalf("fireball should declare aoe_radius_tiles > 0, got %.2f", def.AoeRadiusTiles)
	}
}

// TestFireboltYAML_HasNoAoE confirms fire bolt remains single-target as a
// design contrast with fireball.
func TestFireboltYAML_HasNoAoE(t *testing.T) {
	_ = newTestCombatSystemWithConfig(t)
	def, err := spells.GetSpellDefinitionByID("firebolt")
	if err != nil {
		t.Fatalf("firebolt spell missing: %v", err)
	}
	if def.AoeRadiusTiles != 0 {
		t.Fatalf("firebolt should be single-target (aoe_radius_tiles=0), got %.2f", def.AoeRadiusTiles)
	}
}

func TestTonbogiriYAML_AdvertisesTwoTileAoE(t *testing.T) {
	_ = newTestCombatSystemWithConfig(t)
	def, ok := config.GetWeaponDefinition("tonbogiri")
	if !ok || def == nil {
		t.Fatal("tonbogiri missing from weapons.yaml")
	}
	if def.AoeRadiusTiles != 2.0 {
		t.Fatalf("tonbogiri AoE radius = %.2f, want 2.0", def.AoeRadiusTiles)
	}
}

func TestMeleeWeaponAoE_UsesWeaponRadiusOnRealMeleeDamagePath(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tileSize := float64(cs.game.config.GetTileSize())

	primary := &monsterPkg.Monster3D{
		ID: "primary", Name: "Primary", X: 0, Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	near := &monsterPkg.Monster3D{
		ID: "near", Name: "Near", X: tileSize * 1.9, Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	far := &monsterPkg.Monster3D{
		ID: "far", Name: "Far", X: tileSize * 2.1, Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, near, far}

	cs.ApplyDamageToMonster(primary, 20, "Tonbogiri, the Dragonfly Spear", false)

	if primary.HitPoints != 80 {
		t.Fatalf("primary HP = %d, want 80", primary.HitPoints)
	}
	if near.HitPoints != 80 {
		t.Fatalf("near monster should take Tonbogiri splash, HP = %d, want 80", near.HitPoints)
	}
	if far.HitPoints != 100 {
		t.Fatalf("far monster should be outside Tonbogiri splash, HP = %d, want 100", far.HitPoints)
	}
}

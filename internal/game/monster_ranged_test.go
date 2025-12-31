package game

import (
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

type testTileChecker struct {
	width  int
	height int
}

func (t *testTileChecker) IsTileBlocking(tileX, tileY int) bool {
	return false
}

func (t *testTileChecker) IsTileBlockingForHabitat(tileX, tileY int, habitatPrefs []string, flying bool) bool {
	return false
}

func (t *testTileChecker) IsTileOpaque(tileX, tileY int) bool {
	return false
}

func (t *testTileChecker) GetWorldBounds() (int, int) {
	return t.width, t.height
}

func newTestCombatSystemWithConfig(t *testing.T) *CombatSystem {
	t.Helper()

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	game := &MMGame{
		config: cfg,
		camera: &FirstPersonCamera{
			X:        0,
			Y:        0,
			Angle:    0,
			FOV:      cfg.GetCameraFOV(),
			ViewDist: cfg.GetViewDistance(),
		},
		party: character.NewParty(cfg),
		world: &world.World3D{},
	}
	game.selectedChar = 0
	game.collisionSystem = collision.NewCollisionSystem(&testTileChecker{width: 100, height: 100}, float64(cfg.GetTileSize()))
	game.collisionSystem.RegisterEntity(collision.NewEntity("player", game.camera.X, game.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	cs := NewCombatSystem(game)
	game.combat = cs

	return cs
}

func TestMonsterRangedAttack_SpawnsWeaponProjectile(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	game.camera.X = 64
	game.camera.Y = 0
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	monster := &monsterPkg.Monster3D{
		Name:             "Bandit",
		X:                0,
		Y:                0,
		AttackRadius:     128,
		State:            monsterPkg.StateAttacking,
		StateTimer:       1,
		ProjectileWeapon: "throwing_knife",
		DamageMin:        5,
		DamageMax:        5,
		HitPoints:        1,
		MaxHitPoints:     1,
	}
	game.world.Monsters = []*monsterPkg.Monster3D{monster}

	beforeHP := game.party.Members[0].HitPoints
	cs.HandleMonsterInteractions()

	if len(game.arrows) != 1 {
		t.Fatalf("expected 1 arrow, got %d", len(game.arrows))
	}
	arrow := game.arrows[0]
	if arrow.Owner != ProjectileOwnerMonster {
		t.Fatalf("expected monster-owned arrow, got %v", arrow.Owner)
	}
	if arrow.BowKey != "throwing_knife" {
		t.Fatalf("expected throwing_knife, got %s", arrow.BowKey)
	}
	if arrow.SourceName != "Bandit" {
		t.Fatalf("expected source name Bandit, got %s", arrow.SourceName)
	}
	if game.party.Members[0].HitPoints != beforeHP {
		t.Fatalf("expected no immediate melee damage, HP %d -> %d", beforeHP, game.party.Members[0].HitPoints)
	}
}

func TestMonsterRangedAttack_SpawnsSpellProjectile(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	game.camera.X = 64
	game.camera.Y = 0
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	monster := &monsterPkg.Monster3D{
		Name:            "Dragon",
		X:               0,
		Y:               0,
		AttackRadius:    128,
		State:           monsterPkg.StateAttacking,
		StateTimer:      1,
		ProjectileSpell: "firebolt",
		DamageMin:       7,
		DamageMax:       7,
		HitPoints:       1,
		MaxHitPoints:    1,
	}
	game.world.Monsters = []*monsterPkg.Monster3D{monster}

	beforeHP := game.party.Members[0].HitPoints
	cs.HandleMonsterInteractions()

	if len(game.magicProjectiles) != 1 {
		t.Fatalf("expected 1 magic projectile, got %d", len(game.magicProjectiles))
	}
	mp := game.magicProjectiles[0]
	if mp.Owner != ProjectileOwnerMonster {
		t.Fatalf("expected monster-owned projectile, got %v", mp.Owner)
	}
	if mp.SpellType != "firebolt" {
		t.Fatalf("expected firebolt, got %s", mp.SpellType)
	}
	if mp.SourceName != "Dragon" {
		t.Fatalf("expected source name Dragon, got %s", mp.SourceName)
	}
	if game.party.Members[0].HitPoints != beforeHP {
		t.Fatalf("expected no immediate melee damage, HP %d -> %d", beforeHP, game.party.Members[0].HitPoints)
	}
}

func TestMonsterProjectileHitsPlayer(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	// Ensure no perfect dodge
	game.party.Members[0].Luck = 0

	mp := MagicProjectile{
		ID:         "monster_test_proj",
		X:          game.camera.X,
		Y:          game.camera.Y,
		Damage:     5,
		LifeTime:   10,
		Active:     true,
		SpellType:  "firebolt",
		Owner:      ProjectileOwnerMonster,
		SourceName: "Dragon",
	}
	game.magicProjectiles = append(game.magicProjectiles, mp)
	game.collisionSystem.RegisterEntity(collision.NewEntity(mp.ID, mp.X, mp.Y, 8, 8, collision.CollisionTypeProjectile, false))

	beforeHP := game.party.Members[0].HitPoints
	cs.CheckProjectilePlayerCollisions()

	if game.party.Members[0].HitPoints >= beforeHP {
		t.Fatalf("expected player HP to decrease, HP %d -> %d", beforeHP, game.party.Members[0].HitPoints)
	}
	if game.magicProjectiles[0].Active {
		t.Fatalf("expected projectile to deactivate after hit")
	}
}

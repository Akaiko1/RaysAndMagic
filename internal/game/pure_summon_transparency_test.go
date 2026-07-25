package game

import (
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func partyTransparencyTargets(t *testing.T) (*MMGame, *monsterPkg.Monster3D, *monsterPkg.Monster3D, float64) {
	t.Helper()
	game, _, tileSize := tbBehaviorGame(t, 24, 24)
	placePlayerAtTile(game, 10, 10, tileSize)
	game.camera.Angle = 0

	pure := monsterPkg.NewMonster3DFromConfig(game.camera.X+tileSize, game.camera.Y, "masked_huntress", game.config)
	markCardAlly(pure)
	bound := monsterPkg.NewMonster3DFromConfig(game.camera.X, game.camera.Y+tileSize, "skeleton", game.config)
	bound.Bound = true
	bound.BoundFramesRemaining = 300 * game.config.GetTPS()

	for _, target := range []*monsterPkg.Monster3D{pure, bound} {
		target.MaxHitPoints = 10000
		target.HitPoints = target.MaxHitPoints
		target.ArmorClass = 0
		target.PerfectDodge = 0
		target.Resistances = make(map[monsterPkg.DamageType]int)
	}
	game.world.Monsters = []*monsterPkg.Monster3D{pure, bound}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	return game, pure, bound, tileSize
}

func assertPureIgnoredAndBoundHit(
	t *testing.T,
	pure, bound *monsterPkg.Monster3D,
	pureHP, boundHP int,
) {
	t.Helper()
	if pure.HitPoints != pureHP || pure.WasAttacked || pure.HitTintFrames != 0 {
		t.Fatalf("pure summon reacted to a party attack: HP=%d/%d attacked=%v tint=%d",
			pure.HitPoints, pureHP, pure.WasAttacked, pure.HitTintFrames)
	}
	if bound.HitPoints >= boundHP {
		t.Fatalf("bound undead was not hit: HP=%d, before=%d", bound.HitPoints, boundHP)
	}
}

func TestPureSummonIsWalkableAndIgnoredByPartyMelee(t *testing.T) {
	game, pure, bound, _ := partyTransparencyTargets(t)
	if !game.collisionSystem.CanMoveTo("player", pure.X, pure.Y) {
		t.Fatal("the party must be able to walk through its pure summon")
	}

	pureHP, boundHP := pure.HitPoints, bound.HitPoints
	game.combat.ApplyDamageToMonster(pure, 100, "Iron Sword", false)
	game.combat.ApplyDamageToMonster(bound, 100, "Iron Sword", false)
	assertPureIgnoredAndBoundHit(t, pure, bound, pureHP, boundHP)
}

func TestPartyMeleeArcDoesNotSpendTargetOnPureSummon(t *testing.T) {
	game, pure, bound, tileSize := partyTransparencyTargets(t)
	// Front and front-left are both inside arc 3. The pure summon must not count
	// as a hit or consume an arc slot; the bound undead remains a valid target.
	bound.X = game.camera.X + tileSize
	bound.Y = game.camera.Y - tileSize
	game.collisionSystem.UpdateEntity(bound.ID, bound.X, bound.Y)

	weapon, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	pureHP, boundHP := pure.HitPoints, bound.HitPoints
	hits := game.combat.performMeleeHitDetection(
		weapon,
		100,
		&config.MeleeAttackConfig{ArcType: 3},
		false,
	)

	if hits != 1 {
		t.Fatalf("melee arc connected with %d targets, want only the bound undead", hits)
	}
	assertPureIgnoredAndBoundHit(t, pure, bound, pureHP, boundHP)
}

func TestPlayerProjectilePassesThroughPureSummonIntoBoundUndead(t *testing.T) {
	game, pure, bound, tileSize := partyTransparencyTargets(t)
	game.turnBasedMode = false
	bound.X = game.camera.X + 2*tileSize
	bound.Y = game.camera.Y
	game.collisionSystem.UpdateEntity(bound.ID, bound.X, bound.Y)

	projectile := MagicProjectile{
		ID:        "pure_summon_transparency_bolt",
		X:         pure.X,
		Y:         pure.Y,
		VelX:      8,
		Damage:    100,
		LifeTime:  30,
		Active:    true,
		SpellType: "firebolt",
		Size:      16,
		Owner:     ProjectileOwnerPlayer,
	}
	game.magicProjectiles = append(game.magicProjectiles, projectile)
	game.collisionSystem.RegisterEntity(collision.NewEntity(
		projectile.ID,
		projectile.X,
		projectile.Y,
		16,
		16,
		collision.CollisionTypeProjectile,
		false,
	))

	pureHP, boundHP := pure.HitPoints, bound.HitPoints
	game.combat.CheckProjectileMonsterCollisions()
	if !game.magicProjectiles[0].Active {
		t.Fatal("a pure summon consumed the party projectile")
	}
	if pure.HitPoints != pureHP {
		t.Fatal("a party projectile damaged its pure summon")
	}

	game.magicProjectiles[0].X = bound.X
	game.magicProjectiles[0].Y = bound.Y
	game.collisionSystem.UpdateEntity(projectile.ID, bound.X, bound.Y)
	game.combat.CheckProjectileMonsterCollisions()

	if game.magicProjectiles[0].Active {
		t.Fatal("projectile was not consumed by the bound undead behind the summon")
	}
	assertPureIgnoredAndBoundHit(t, pure, bound, pureHP, boundHP)
}

func TestPartyAreaAttacksIgnorePureSummonsButHitBoundUndead(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*CombatSystem, *monsterPkg.Monster3D)
	}{
		{
			name: "weapon or spell splash",
			apply: func(cs *CombatSystem, center *monsterPkg.Monster3D) {
				attack := cs.newPartyMonsterAttack(100, 0, "fire", 0, nil, "Test Splash", false, true, false)
				cs.applyAoeSplash(center, attack, 3)
			},
		},
		{
			name: "party nova",
			apply: func(cs *CombatSystem, _ *monsterPkg.Monster3D) {
				cs.tryCastInferno(spells.SpellDefinition{
					Name:                "Test Nova",
					School:              "fire",
					SpellPointsCost:     40,
					PartyAoeRadiusTiles: 3,
				}, cs.game.party.Members[0])
			},
		},
		{
			name: "hot steam",
			apply: func(cs *CombatSystem, _ *monsterPkg.Monster3D) {
				cs.damageSteamZoneOnce(&SteamZone{
					X:          cs.game.camera.X,
					Y:          cs.game.camera.Y,
					Radius:     3 * float64(cs.game.config.GetTileSize()),
					TickDamage: 100,
				})
			},
		},
		{
			name: "stone blossom",
			apply: func(cs *CombatSystem, _ *monsterPkg.Monster3D) {
				cs.detonateMortar(pendingMortar{
					X:           cs.game.camera.X,
					Y:           cs.game.camera.Y,
					SpellID:     "test_bloom",
					Damage:      100,
					RadiusTiles: 3,
					School:      "earth",
				})
			},
		},
		{
			name: "aoe trap",
			apply: func(cs *CombatSystem, center *monsterPkg.Monster3D) {
				cs.fireTrap(&PlacedTrap{
					Key:   "cleave_trap",
					X:     center.X,
					Y:     center.Y,
					TileX: int(center.X / float64(cs.game.config.GetTileSize())),
					TileY: int(center.Y / float64(cs.game.config.GetTileSize())),
				}, center)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			game, pure, bound, _ := partyTransparencyTargets(t)
			center := monsterPkg.NewMonster3DFromConfig(game.camera.X, game.camera.Y, "goblin", game.config)
			center.MaxHitPoints, center.HitPoints = 10000, 10000
			center.ArmorClass = 0
			center.Resistances = make(map[monsterPkg.DamageType]int)
			game.world.Monsters = append(game.world.Monsters, center)

			pureHP, boundHP := pure.HitPoints, bound.HitPoints
			tc.apply(game.combat, center)
			assertPureIgnoredAndBoundHit(t, pure, bound, pureHP, boundHP)
		})
	}
}

func TestPartyAoeControlIgnoresPureSummonButHitsBoundUndead(t *testing.T) {
	game, pure, bound, _ := partyTransparencyTargets(t)
	handled := game.combat.tryCastAoeStun("test_darkness", spells.SpellDefinition{
		Name:                "Test Darkness",
		Message:             "Test darkness",
		StunRadiusTiles:     3,
		StunDurationSeconds: 5,
		StunDurationTurns:   3,
	})
	if !handled {
		t.Fatal("AoE stun was not handled")
	}
	if pure.StunFramesRemaining != 0 || pure.StunTurnsRemaining != 0 || pure.StunDRStacks != 0 {
		t.Fatalf("pure summon received AoE control: frames=%d turns=%d DR=%d",
			pure.StunFramesRemaining, pure.StunTurnsRemaining, pure.StunDRStacks)
	}
	if bound.StunFramesRemaining <= 0 || bound.StunTurnsRemaining <= 0 {
		t.Fatalf("bound undead did not receive AoE control: frames=%d turns=%d",
			bound.StunFramesRemaining, bound.StunTurnsRemaining)
	}
}

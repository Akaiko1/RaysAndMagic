package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// The map-departure rule for the three controlled-monster kinds:
//   - card ally (pure summon): crumbles, party gets nothing.
//   - bound undead (a former enemy): crumbles, party gets its XP (no loot).
//   - pacified charm (not the party's): left behind alive, untouched.
func TestBoundAllyMapExit(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cfg := game.config

	huntress := monsterPkg.NewMonster3DFromConfig(0, 0, "masked_huntress", cfg)
	markCardAlly(huntress)
	skel := monsterPkg.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	charmed := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	game.combat.applyPacify(charmed, 120, "Charm")
	game.world.Monsters = []*monsterPkg.Monster3D{huntress, skel, charmed}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	xp0 := game.party.Members[0].Experience
	game.crumbleBoundAlliesOnDeparture(game.world)

	if len(game.world.Monsters) != 1 || game.world.Monsters[0] != charmed {
		t.Fatalf("departing world should retain only the charmed monster, got %+v", game.world.Monsters)
	}
	if game.collisionSystem.GetEntityByID(huntress.ID) != nil || game.collisionSystem.GetEntityByID(skel.ID) != nil {
		t.Error("crumbled allies must be removed from collision immediately")
	}
	wm := world.NewWorldManager(game.config)
	wm.CurrentMapKey = "origin"
	wm.LoadedMaps = map[string]*world.World3D{"origin": game.world}
	if saved := game.buildSave(wm).MapMonsters["origin"]; len(saved) != 1 || saved[0].ID != charmed.ID {
		t.Fatalf("save must not retain crumbled allies, got %+v", saved)
	}
	// Only the bound undead's XP is granted; the card ally yields nothing.
	if game.party.Members[0].Experience <= xp0 {
		t.Error("bound undead should grant its XP on departure")
	}
}

func TestFailedMapSwitchKeepsBoundAllies(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	ally := monsterPkg.NewMonster3DFromConfig(0, 0, "masked_huntress", game.config)
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	wm := world.NewWorldManager(game.config)
	wm.CurrentMapKey = "origin"
	wm.LoadedMaps = map[string]*world.World3D{"origin": game.world}
	previous := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	t.Cleanup(func() { world.GlobalWorldManager = previous })

	(&InputHandler{game: game}).switchToMap("missing")

	if len(game.world.Monsters) != 1 || game.world.Monsters[0] != ally || !ally.IsAlive() {
		t.Fatal("a failed map switch must leave card allies untouched")
	}
	if game.collisionSystem.GetEntityByID(ally.ID) == nil {
		t.Fatal("a failed map switch must keep the ally collision entity")
	}
}

// switchToMap changes game.world before crumbling the departing allies, while
// collisionSystem remains shared. The cleanup therefore must unregister a
// crumbled ally itself; the later old-world sweep cannot see a monster already
// removed from oldWorld.Monsters.
func TestMapSwitchRemovesCrumbledBoundAllyCollision(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	oldWorld := game.world
	ally := monsterPkg.NewMonster3DFromConfig(0, 0, "masked_huntress", game.config)
	markCardAlly(ally)
	oldWorld.Monsters = []*monsterPkg.Monster3D{ally}
	oldWorld.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	newWorld := newTestWorldSized(game.config, 5, 5)
	wm := world.NewWorldManager(game.config)
	wm.CurrentMapKey = "origin"
	wm.LoadedMaps = map[string]*world.World3D{"origin": oldWorld, "destination": newWorld}
	previous := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	t.Cleanup(func() { world.GlobalWorldManager = previous })

	(&InputHandler{game: game}).switchToMap("destination")

	if game.world != newWorld {
		t.Fatal("map transition did not enter the destination world")
	}
	if len(oldWorld.Monsters) != 0 {
		t.Fatalf("departing world retained crumbled ally: %+v", oldWorld.Monsters)
	}
	if entity := game.collisionSystem.GetEntityByID(ally.ID); entity != nil {
		t.Fatalf("crumbled ally collision %q survived map switch: %+v", ally.ID, entity)
	}
}

// A card ally with no enemy to hunt tags along with the party (its AI target is
// the party), rather than parking in place.
func TestCardAllyFollowsPartyWhenIdle(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, ts)
	huntress := monsterPkg.NewMonster3DFromConfig(float64(3)*ts, float64(3)*ts, "masked_huntress", game.config)
	markCardAlly(huntress)
	game.world.Monsters = []*monsterPkg.Monster3D{huntress} // no enemy on the map
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.refreshBoundAllyCache()
	if huntress.AIFoe != nil {
		t.Fatal("no enemy present - the card ally should have no foe")
	}
	tx, ty := game.combat.monsterAITargetPoint(huntress)
	if tx != game.camera.X || ty != game.camera.Y {
		t.Errorf("idle card ally should target the party (%.0f,%.0f), got (%.0f,%.0f)", game.camera.X, game.camera.Y, tx, ty)
	}
}

// Bind Undead uses the same idle-follow fallback as a card ally. It still
// switches to a hostile target as soon as one is found by the per-frame cache.
func TestBoundUndeadFollowsPartyWhenIdle(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, ts)
	skel := monsterPkg.NewMonster3DFromConfig(float64(3)*ts, float64(3)*ts, "skeleton", game.config)
	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	game.world.Monsters = []*monsterPkg.Monster3D{skel} // no enemy on the map
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.refreshBoundAllyCache()
	if skel.AIFoe != nil {
		t.Fatal("no enemy present - the bound undead should have no foe")
	}
	tx, ty := game.combat.monsterAITargetPoint(skel)
	if tx != game.camera.X || ty != game.camera.Y {
		t.Errorf("idle bound undead should target the party (%.0f,%.0f), got (%.0f,%.0f)", game.camera.X, game.camera.Y, tx, ty)
	}
}

// A charmed mob snaps out of the charm and re-aggros both on any hit and when
// the charm wears off; then, being an ordinary enemy again, it rewards the party
// when slain.
func TestCharmAggressionAndReward(t *testing.T) {
	game, gl, _ := tbBehaviorGame(t, 5, 5)
	cfg := game.config

	// Break on hit.
	m := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.combat.applyPacify(m, 120, "Charm")
	if !m.Pacified {
		t.Fatal("goblin should be charmable")
	}
	game.combat.markMonsterHit(m) // any hit source funnels through here
	if m.Pacified || !m.WasAttacked {
		t.Errorf("a hit must break the charm and re-aggro (Pacified=%v WasAttacked=%v)", m.Pacified, m.WasAttacked)
	}

	// Break on expiry.
	m2 := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	game.world.Monsters = []*monsterPkg.Monster3D{m2}
	game.combat.applyPacify(m2, 120, "Charm")
	m2.PacifiedFramesRemaining = 1
	gl.updateControlledMonsters()
	if m2.Pacified || !m2.WasAttacked {
		t.Errorf("charm expiry must re-aggro (Pacified=%v WasAttacked=%v)", m2.Pacified, m2.WasAttacked)
	}

	// A formerly-charmed enemy, once slain, rewards the party like any enemy.
	enemy := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	enemy.HitPoints = 1
	killer := monsterPkg.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	game.combat.applyBindUndead(killer, 300, "Bind Undead")
	game.world.Monsters = []*monsterPkg.Monster3D{enemy, killer}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	xp0 := game.party.Members[0].Experience
	game.combat.monsterStrikeMonster(killer, enemy)
	if enemy.IsAlive() || game.party.Members[0].Experience <= xp0 {
		t.Error("slaying an ordinary enemy must reward the party")
	}
}

func TestPartyMeleeBreaksCharmBeforeDamageResolution(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	charmed := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", game.config)
	charmed.PerfectDodge = 100 // the attempted attack still ends Charm
	game.world.Monsters = []*monsterPkg.Monster3D{charmed}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyPacify(charmed, 120, "Charm")

	game.combat.ApplyDamageToMonster(charmed, 10, "Iron Sword", false)

	if charmed.Pacified || !charmed.WasAttacked {
		t.Errorf("a party melee attack must break Charm even on dodge (Pacified=%v WasAttacked=%v)", charmed.Pacified, charmed.WasAttacked)
	}
}

func TestPartyMeleeArcBreaksCharmForEveryHitTarget(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	const tx, ty = 10, 10
	placePlayerAtTile(game, tx, ty, ts)
	game.camera.Angle = 0
	weapon, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}

	targets := []*monsterPkg.Monster3D{
		monsterPkg.NewMonster3DFromConfig(float64(tx+1)*ts+ts/2, float64(ty)*ts+ts/2, "goblin", game.config),
		monsterPkg.NewMonster3DFromConfig(float64(tx+1)*ts+ts/2, float64(ty-1)*ts+ts/2, "goblin", game.config),
		monsterPkg.NewMonster3DFromConfig(float64(tx+1)*ts+ts/2, float64(ty+1)*ts+ts/2, "goblin", game.config),
	}
	for _, target := range targets {
		target.MaxHitPoints, target.HitPoints = 500, 500
		game.combat.applyPacify(target, 120, "Charm")
	}
	game.world.Monsters = targets
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.combat.performMeleeHitDetection(weapon, 20, &config.MeleeAttackConfig{ArcType: 3}, false)

	for i, target := range targets {
		if target.Pacified || !target.WasAttacked {
			t.Errorf("arc target %d must break Charm (Pacified=%v WasAttacked=%v)", i, target.Pacified, target.WasAttacked)
		}
	}
}

func TestPartyMeleeAoEBreaksCharmForPrimaryAndSplash(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ts := float64(cs.game.config.GetTileSize())
	primary := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", cs.game.config)
	splash := monsterPkg.NewMonster3DFromConfig(ts, 0, "goblin", cs.game.config)
	for _, target := range []*monsterPkg.Monster3D{primary, splash} {
		target.MaxHitPoints, target.HitPoints = 500, 500
		cs.game.combat.applyPacify(target, 120, "Charm")
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, splash}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	cs.ApplyDamageToMonster(primary, 20, "Tonbogiri, the Dragonfly Spear", false)

	for _, target := range []*monsterPkg.Monster3D{primary, splash} {
		if target.Pacified || !target.WasAttacked {
			t.Errorf("AoE target %s must break Charm (Pacified=%v WasAttacked=%v)", target.Name, target.Pacified, target.WasAttacked)
		}
	}
}

func TestPartySpellAoEBreaksCharmForPrimaryAndSplash(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ts := float64(cs.game.config.GetTileSize())
	primary := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", cs.game.config)
	splash := monsterPkg.NewMonster3DFromConfig(ts, 0, "goblin", cs.game.config)
	for _, target := range []*monsterPkg.Monster3D{primary, splash} {
		target.MaxHitPoints, target.HitPoints = 500, 500
		cs.applyPacify(target, 120, "Charm")
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, splash}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	bolt := &MagicProjectile{ID: "test_fireball", Active: true, LifeTime: 1, Damage: 20, SpellType: "fireball"}
	cs.applyProjectileDamage(bolt, "magic_projectile", primary, bolt.ID)

	for _, target := range []*monsterPkg.Monster3D{primary, splash} {
		if target.Pacified || !target.WasAttacked {
			t.Errorf("spell AoE target %s must break Charm (Pacified=%v WasAttacked=%v)", target.Name, target.Pacified, target.WasAttacked)
		}
	}
}

func TestMobTargetsBoundAlliesButNotCharmedMonster(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	makeEnemy := func() *monsterPkg.Monster3D {
		return monsterPkg.NewMonster3DFromConfig(10*ts, 10*ts, "goblin", game.config)
	}
	makeTarget := func(key string) *monsterPkg.Monster3D {
		return monsterPkg.NewMonster3DFromConfig(11*ts, 10*ts, key, game.config)
	}

	t.Run("bound_undead", func(t *testing.T) {
		enemy, target := makeEnemy(), makeTarget("skeleton")
		game.world.Monsters = []*monsterPkg.Monster3D{enemy, target}
		game.combat.applyBindUndead(target, 120, "Bind Undead")
		game.refreshBoundAllyCache()
		if enemy.AIFoe != target {
			t.Fatal("a mob must target a nearby bound undead")
		}
	})

	t.Run("card_ally", func(t *testing.T) {
		enemy, target := makeEnemy(), makeTarget("masked_huntress")
		game.world.Monsters = []*monsterPkg.Monster3D{enemy, target}
		markCardAlly(target)
		game.refreshBoundAllyCache()
		if enemy.AIFoe != target {
			t.Fatal("a mob must target a nearby card ally")
		}
	})

	t.Run("charmed", func(t *testing.T) {
		enemy, target := makeEnemy(), makeTarget("goblin")
		game.world.Monsters = []*monsterPkg.Monster3D{enemy, target}
		game.combat.applyPacify(target, 120, "Charm")
		game.refreshBoundAllyCache()
		if enemy.AIFoe != nil {
			t.Fatal("a charmed monster is neutral and must not be a crossfire target")
		}
	})
}

// Every controlled-monster kind has its own reward rule when an enemy kills it:
// a former enemy bound by Bind Undead rewards normally, a card ally never does,
// and a charmed enemy returns to normal reward behavior on the first party hit.
func TestControlledMonsterRewardRules(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	makeKiller := func() *monsterPkg.Monster3D {
		killer := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", game.config)
		killer.DamageMin, killer.DamageMax = 999, 999
		return killer
	}
	assertReward := func(t *testing.T, target *monsterPkg.Monster3D, kill func()) {
		t.Helper()
		xp0, bags0 := game.party.Members[0].Experience, len(game.groundContainers)
		kill()
		if target.IsAlive() {
			t.Fatal("test target must die")
		}
		if game.party.Members[0].Experience <= xp0 {
			t.Fatal("expected experience reward")
		}
		if len(game.groundContainers) != bags0+1 || game.groundContainers[bags0].Gold != target.Gold {
			t.Fatalf("expected a %d-gold loot bag, got %+v", target.Gold, game.groundContainers[bags0:])
		}
	}

	t.Run("bound_undead", func(t *testing.T) {
		target := monsterPkg.NewMonster3DFromConfig(0, 0, "skeleton", game.config)
		target.HitPoints, target.Experience, target.Gold = 1, 40, 17
		killer := makeKiller()
		game.world.Monsters = []*monsterPkg.Monster3D{target, killer}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		game.combat.applyBindUndead(target, 120, "Bind Undead")
		assertReward(t, target, func() { game.combat.monsterStrikeMonster(killer, target) })
	})

	t.Run("card_ally", func(t *testing.T) {
		target := monsterPkg.NewMonster3DFromConfig(0, 0, "masked_huntress", game.config)
		target.HitPoints, target.Experience, target.Gold = 1, 40, 17
		killer := makeKiller()
		game.world.Monsters = []*monsterPkg.Monster3D{target, killer}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		markCardAlly(target)
		xp0, bags0 := game.party.Members[0].Experience, len(game.groundContainers)
		game.combat.monsterStrikeMonster(killer, target)
		if target.IsAlive() {
			t.Fatal("test target must die")
		}
		if game.party.Members[0].Experience != xp0 || len(game.groundContainers) != bags0 {
			t.Fatalf("card ally must yield no XP or bag (xp %d -> %d, bags %d -> %d)", xp0, game.party.Members[0].Experience, bags0, len(game.groundContainers))
		}
	})

	t.Run("charmed_enemy", func(t *testing.T) {
		target := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", game.config)
		target.HitPoints, target.Experience, target.Gold = 1, 40, 17
		game.world.Monsters = []*monsterPkg.Monster3D{target}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		game.combat.applyPacify(target, 120, "Charm")
		assertReward(t, target, func() { game.combat.ApplyDamageToMonster(target, 999, "Iron Sword", false) })
	})

	t.Run("charmed_boss_add", func(t *testing.T) {
		target := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", game.config)
		target.HitPoints, target.Experience, target.Gold = 1, 40, 17
		target.SummonedBy = "boss_1"
		game.world.Monsters = []*monsterPkg.Monster3D{target}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		game.combat.applyPacify(target, 120, "Charm")
		assertReward(t, target, func() { game.combat.ApplyDamageToMonster(target, 999, "Iron Sword", false) })
	})
}

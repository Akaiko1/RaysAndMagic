package game

import (
	"fmt"
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

	game.switchToMap("missing")

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

	game.switchToMap("destination")

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

// Idle-follow used to be pinned here by asserting only that monsterAITargetPoint
// returns the party. That is the INTENT, not the outcome: an ally can target the
// party and never take a step (a summon with 11-tile reach did exactly that). The
// real guard now measures the settled distance per reach class in
// ally_control_test.go.

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
	if !m2.Pacified {
		t.Fatal("TB thinking must not expire Charm")
	}
	game.startPartyTurn()
	if m2.Pacified || !m2.WasAttacked || !m2.IsEngagingPlayer {
		t.Errorf("charm expiry must re-aggro (Pacified=%v WasAttacked=%v Engaging=%v)", m2.Pacified, m2.WasAttacked, m2.IsEngagingPlayer)
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

// Every party hit path breaks Charm on each monster it touches - even a hit
// the target dodges - in both combat modes.
func TestPartyHitsBreakCharmOnEveryTarget(t *testing.T) {
	type tile struct{ dx, dy int }
	for _, tc := range []struct {
		name    string
		targets []tile // tiles from the party, which faces +X
		dodge   bool
		apply   func(t *testing.T, cs *CombatSystem, targets []*monsterPkg.Monster3D)
	}{
		{name: "melee_dodged", targets: []tile{{1, 0}}, dodge: true,
			apply: func(_ *testing.T, cs *CombatSystem, targets []*monsterPkg.Monster3D) {
				cs.ApplyDamageToMonster(targets[0], 10, "Iron Sword", false)
			}},
		{name: "melee_arc", targets: []tile{{1, 0}, {1, -1}, {1, 1}},
			apply: func(t *testing.T, cs *CombatSystem, _ []*monsterPkg.Monster3D) {
				weapon, err := items.TryCreateWeaponFromYAML("steel_axe")
				if err != nil {
					t.Fatalf("steel_axe: %v", err)
				}
				cs.performMeleeHitDetection(weapon, 20, &config.MeleeAttackConfig{ArcType: 3}, false)
			}},
		{name: "melee_aoe", targets: []tile{{1, 0}, {2, 0}},
			apply: func(t *testing.T, cs *CombatSystem, targets []*monsterPkg.Monster3D) {
				def, ok := config.GetWeaponDefinition("tonbogiri")
				if !ok || def == nil || def.AoeRadiusTiles < 1 {
					t.Fatal("tonbogiri missing or its splash cannot reach the next tile")
				}
				cs.ApplyDamageToMonster(targets[0], 20, def.Name, false)
			}},
		{name: "spell_aoe", targets: []tile{{1, 0}, {2, 0}},
			apply: func(_ *testing.T, cs *CombatSystem, targets []*monsterPkg.Monster3D) {
				bolt := &MagicProjectile{ID: "test_fireball", Active: true, LifeTime: 1, Damage: 20, SpellType: "fireball"}
				cs.applyProjectileDamage(bolt, "magic_projectile", targets[0], bolt.ID)
			}},
	} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB_%v", tc.name, tb), func(t *testing.T) {
				game, _, ts := tbBehaviorGame(t, 20, 20)
				game.turnBasedMode = tb
				const px, py = 10, 10
				placePlayerAtTile(game, px, py, ts)
				game.camera.Angle = 0
				targets := make([]*monsterPkg.Monster3D, len(tc.targets))
				for i, at := range tc.targets {
					m := monsterPkg.NewMonster3DFromConfig(float64(px+at.dx)*ts+ts/2, float64(py+at.dy)*ts+ts/2, "goblin", game.config)
					m.MaxHitPoints, m.HitPoints = 500, 500
					if tc.dodge {
						m.PerfectDodge = 100 // the attempted attack still ends Charm
					}
					targets[i] = m
				}
				game.world.Monsters = targets
				game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
				for _, m := range targets {
					game.combat.applyPacify(m, 120, "Charm")
				}

				tc.apply(t, game.combat, targets)

				for i, m := range targets {
					if m.Pacified || !m.WasAttacked {
						t.Errorf("target %d must break Charm (Pacified=%v WasAttacked=%v)", i, m.Pacified, m.WasAttacked)
					}
				}
			})
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
		game.refreshMonsterAIState()
		if enemy.AIFoe != target {
			t.Fatal("a mob must target a nearby bound undead")
		}
	})

	t.Run("card_ally", func(t *testing.T) {
		enemy, target := makeEnemy(), makeTarget("masked_huntress")
		game.world.Monsters = []*monsterPkg.Monster3D{enemy, target}
		markCardAlly(target)
		game.refreshMonsterAIState()
		if enemy.AIFoe != target {
			t.Fatal("a mob must target a nearby card ally")
		}
	})

	t.Run("charmed", func(t *testing.T) {
		enemy, target := makeEnemy(), makeTarget("goblin")
		game.world.Monsters = []*monsterPkg.Monster3D{enemy, target}
		game.combat.applyPacify(target, 120, "Charm")
		game.refreshMonsterAIState()
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
		target.PerfectDodge = 0 // reward test; the killing blow must be deterministic
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

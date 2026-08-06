package game

import (
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// ONE home for party-controlled monsters: who can create them, how they are
// classified, and how they behave. Two shapes live here on purpose:
//   - the SOURCE x PROPERTY matrix (allySourceCases) - every way the party gains
//     control, checked against the same property list;
//   - per-reach-class follow tests - the same escort rule over allies whose
//     attack ranges differ wildly (melee 1 to ranged 11), which is where a
//     follow distance derived from attack range goes wrong.
// Charm-break paths and deep kill-path rewards stay in summon_lifecycle_test.go.

// Every way the party can end up controlling a monster, built through its REAL
// entry point - never by setting Bound/Pacified/SummonedBy by hand. Hand-marked
// fixtures are why the spell-summon namespace gap went unnoticed: 68 test sites
// construct allies directly, so they all pre-satisfied the very predicate under
// test. Each row states the whole contract; the subtests below check one property
// across every row, so a new ally source has exactly one place to declare itself.
type allySourceCase struct {
	name string
	// spawn returns the controlled monster, already in the world.
	spawn func(t *testing.T, g *MMGame) *monsterPkg.Monster3D
	pure  bool // pure party summon: no rewards, transparent to party damage
	// escorts: follows the party at AllyFollowDistanceTiles when it has no foe.
	// A charmed monster never escorts - it stands down where it was charmed.
	escorts             bool
	crumblesOnMapSwitch bool
	paysXPOnCrumble     bool
}

func allySourceCases() []allySourceCase {
	return []allySourceCase{
		{
			name: "card_summon",
			spawn: func(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
				t.Helper()
				if n := g.combat.summonCardAllies("masked_huntress", 1); n != 1 {
					t.Fatalf("summonCardAllies = %d, want 1", n)
				}
				return g.world.Monsters[len(g.world.Monsters)-1]
			},
			pure: true, escorts: true, crumblesOnMapSwitch: true,
		},
		{
			name: "skill_animal_bonding",
			spawn: func(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
				t.Helper()
				druid := character.CreateCharacter("Druid", character.ClassDruid, g.config)
				druid.Skills[character.SkillAnimalBonding].Mastery = character.MasteryMaster
				g.party.Members = []*character.MMCharacter{druid}
				if !g.combat.summonAnimalBondingBear(druid) {
					t.Fatal("Animal Bonding could not place a bear")
				}
				return g.world.Monsters[len(g.world.Monsters)-1]
			},
			pure: true, escorts: true, crumblesOnMapSwitch: true,
		},
		{
			name: "spell_summon",
			spawn: func(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
				t.Helper()
				def, err := spells.GetSpellDefinitionByID(spells.SpellID("summon_ice_elemental"))
				if err != nil {
					t.Fatalf("summon_ice_elemental definition: %v", err)
				}
				if !g.combat.tryCastSummon(def, g.party.Members[0]) {
					t.Fatal("summon spell was not handled by the summon path")
				}
				return g.world.Monsters[len(g.world.Monsters)-1]
			},
			pure: true, escorts: true, crumblesOnMapSwitch: true,
		},
		{
			name: "spell_bind_undead",
			spawn: func(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
				t.Helper()
				m := spawnAllyMatrixMonster(t, g, "skeleton")
				g.combat.applyBindUndead(m, 60, "Bind Undead")
				if !m.Bound {
					t.Fatal("Bind Undead did not take the skeleton")
				}
				return m
			},
			// A former ENEMY: it fights for the party but still yields its reward
			// and is not shielded from the party's own damage.
			pure: false, escorts: true, crumblesOnMapSwitch: true, paysXPOnCrumble: true,
		},
		{
			name: "spell_charm",
			spawn: func(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
				t.Helper()
				m := spawnAllyMatrixMonster(t, g, "goblin")
				g.combat.applyPacify(m, 60, "Charm")
				if !m.Pacified {
					t.Fatal("Charm did not pacify the goblin")
				}
				return m
			},
			pure: false, escorts: false, crumblesOnMapSwitch: false,
		},
	}
}

// spawnAllyMatrixMonster places a plain monster a few tiles from the party for
// the control spells to take over.
func spawnAllyMatrixMonster(t *testing.T, g *MMGame, key string) *monsterPkg.Monster3D {
	t.Helper()
	ts := float64(g.config.GetTileSize())
	m := monsterPkg.NewMonster3DFromConfig(g.camera.X+4*ts, g.camera.Y, key, g.config)
	if m == nil {
		t.Fatalf("%s: monster config missing", key)
	}
	m.MaxHitPoints, m.HitPoints = 400, 400
	g.registerSpawnedMonster(m)
	g.refreshMonsterCollisionState(m)
	return m
}

// Classification: the flags every downstream rule reads.
func TestAllyMatrix_Classification(t *testing.T) {
	for _, tc := range allySourceCases() {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			m := tc.spawn(t, g)
			if got := isPurePartySummon(m); got != tc.pure {
				t.Errorf("isPurePartySummon = %v, want %v (owner %q)", got, tc.pure, m.SummonedBy)
			}
			if m.Bound == m.Pacified {
				t.Errorf("control state is ambiguous: Bound=%v Pacified=%v", m.Bound, m.Pacified)
			}
			if tc.pure && !m.QuestProgressIgnored {
				t.Error("a pure summon must not count toward map-clear quests")
			}
			if !m.IsPartyControlled() {
				t.Error("monster is not party-controlled")
			}
		})
	}
}

// Friendly fire: the party's own damage must pass through pure summons and land
// on former enemies. Covers the direct hub and a persistent zone (Firewall),
// which are separate guards in the code.
func TestAllyMatrix_PartyDamageTransparency(t *testing.T) {
	for _, tc := range allySourceCases() {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			m := tc.spawn(t, g)
			hp := m.HitPoints
			g.combat.ApplyDamageToMonster(m, 50, "", false)
			if tc.pure && m.HitPoints != hp {
				t.Errorf("pure summon took %d party damage", hp-m.HitPoints)
			}
			if !tc.pure && m.HitPoints >= hp {
				t.Errorf("a former enemy shrugged off party damage (HP %d -> %d)", hp, m.HitPoints)
			}

			hp = m.HitPoints
			g.steamZones = append(g.steamZones[:0], SteamZone{SpellID: "firewall", X: m.X, Y: m.Y,
				Radius:     float64(g.config.GetTileSize()),
				TickDamage: 25, FramesLeft: 60, IntervalFrames: 60})
			g.combat.applyZoneEntrySpell("firewall")
			if tc.pure && m.HitPoints != hp {
				t.Errorf("pure summon took %d zone damage", hp-m.HitPoints)
			}
		})
	}
}

// Rewards: a pure summon is worth nothing even when its config says otherwise;
// a former enemy keeps paying out. Note XP has a SECOND gate (`SummonedBy != ""`
// zeroes it for boss adds too), so this row's real teeth are gold and loot -
// those hang on isPurePartySummon alone.
func TestAllyMatrix_KillRewards(t *testing.T) {
	for _, tc := range allySourceCases() {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			m := tc.spawn(t, g)
			m.Experience = 999
			m.Gold = 250 // gold drops as a ground loot bag, not straight into the purse
			bagsBefore := len(g.groundContainers)
			xp := g.combat.awardExperienceAndGold(m)
			if tc.pure && xp != 0 {
				t.Errorf("pure summon awarded %d XP", xp)
			}
			if !tc.pure && xp == 0 {
				t.Error("a former enemy awarded no XP")
			}
			bags := len(g.groundContainers) - bagsBefore
			if tc.pure && bags != 0 {
				t.Errorf("pure summon dropped %d loot bag(s)", bags)
			}
			if !tc.pure && bags == 0 {
				t.Error("a former enemy dropped no gold bag")
			}
		})
	}
}

// Escort: with no enemy on the map every ALLY closes on the party and holds the
// shared follow distance; a charmed monster stays where it was charmed.
func TestAllyMatrix_EscortDistance(t *testing.T) {
	for _, tc := range allySourceCases() {
		t.Run(tc.name, func(t *testing.T) {
			g, ts := summonTileWorld(t)
			m := tc.spawn(t, g)
			// Park it far away, then let the real RT loop drive it home.
			m.X, m.Y = g.camera.X+14*ts, g.camera.Y
			g.refreshMonsterCollisionState(m)
			runRTFoeTicks(g, 20*g.config.GetTPS())
			d := Distance(g.camera.X, g.camera.Y, m.X, m.Y)

			if !tc.escorts {
				// A charmed monster keeps wandering, so its distance may drift either
				// way by chance. What must hold is that it never PURSUES: its AI target
				// is itself and it stays out of the combat states.
				tx, ty := g.combat.monsterAITargetPoint(m)
				if tx != m.X || ty != m.Y {
					t.Errorf("charmed target point = (%.0f,%.0f), want its own position (%.0f,%.0f)",
						tx, ty, m.X, m.Y)
				}
				switch m.State {
				case monsterPkg.StatePursuing, monsterPkg.StateAttacking, monsterPkg.StateAlert:
					t.Errorf("charmed monster entered combat state %v", m.State)
				}
				if m.IsEngagingPlayer {
					t.Error("charmed monster is engaging the party")
				}
				return
			}
			if limit := (monsterPkg.AllyFollowDistanceTiles + 0.5) * ts; d > limit {
				t.Errorf("ally settled %.1f tiles away, want within %.1f",
					d/ts, monsterPkg.AllyFollowDistanceTiles)
			}
		})
	}
}

// Departure: leaving the WORLD (a dungeon door, not an open-world seam) crumbles
// the party's allies, pays a former enemy's XP, and leaves a charmed monster be.
func TestAllyMatrix_WorldSwitchDeparture(t *testing.T) {
	for _, tc := range allySourceCases() {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			m := tc.spawn(t, g)
			xpBefore := 0
			for _, c := range g.party.Members {
				xpBefore += c.Experience
			}

			departing := g.world
			g.crumbleBoundAlliesOnDeparture(departing)

			alive := false
			for _, w := range departing.Monsters {
				if w == m {
					alive = true
				}
			}
			if alive == tc.crumblesOnMapSwitch {
				t.Errorf("survived departure = %v, want %v", alive, !tc.crumblesOnMapSwitch)
			}
			if alive && g.collisionSystem.GetEntityByID(m.ID) == nil {
				t.Error("a surviving monster lost its collision entity")
			}
			if !alive && g.collisionSystem.GetEntityByID(m.ID) != nil {
				t.Error("a crumbled ally left a ghost collision entity")
			}
			xpAfter := 0
			for _, c := range g.party.Members {
				xpAfter += c.Experience
			}
			if paid := xpAfter > xpBefore; paid != tc.paysXPOnCrumble {
				t.Errorf("XP paid on departure = %v, want %v (%d -> %d)",
					paid, tc.paysXPOnCrumble, xpBefore, xpAfter)
			}
		})
	}
}

// allyKeys covers one ally of each reach class the game can summon: the card
// huntress (ranged 6), the druid's bear (melee 1) and the Ice Elemental (ranged
// 11). Their weapon reach differs by design; their ESCORT distance must not.
var allyKeys = []string{"masked_huntress", "bear", "frost_elemental"}

// allyFollowWorld drops one ally of `key` far from the party, with no enemies on
// the map, and returns the game plus that ally.
func allyFollowWorld(t *testing.T, key string) (*MMGame, *GameLoop, *monsterPkg.Monster3D, float64) {
	t.Helper()
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 8, 10, ts)
	ally := monsterPkg.NewMonster3DFromConfig(24*ts+ts/2, 10*ts+ts/2, key, game.config)
	if ally == nil {
		t.Fatalf("%s: monster config missing", key)
	}
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	return game, gl, ally, ts
}

// Every ally, whatever its attack range, walks back to the party when it has
// nothing to hunt and settles inside the shared follow distance.
func TestAllyFollowsPartyWithinFollowDistanceRT(t *testing.T) {
	for _, key := range allyKeys {
		t.Run(key, func(t *testing.T) {
			game, _, ally, ts := allyFollowWorld(t, key)
			d0 := Distance(game.camera.X, game.camera.Y, ally.X, ally.Y)
			runRTFoeTicks(game, 20*game.config.GetTPS())
			d := Distance(game.camera.X, game.camera.Y, ally.X, ally.Y)
			if d >= d0 {
				t.Fatalf("ally did not follow the party (%.0f -> %.0fpx)", d0, d)
			}
			if limit := (monsterPkg.AllyFollowDistanceTiles + 0.5) * ts; d > limit {
				t.Errorf("ally settled %.1f tiles away, want within %.1f",
					d/ts, monsterPkg.AllyFollowDistanceTiles)
			}
		})
	}
}

func TestAllyFollowsPartyWithinFollowDistanceTB(t *testing.T) {
	for _, key := range allyKeys {
		t.Run(key, func(t *testing.T) {
			game, gl, ally, ts := allyFollowWorld(t, key)
			d0 := Distance(game.camera.X, game.camera.Y, ally.X, ally.Y)
			for i := 0; i < 30; i++ {
				runOneMonsterTurn(game, gl)
			}
			d := Distance(game.camera.X, game.camera.Y, ally.X, ally.Y)
			if d >= d0 {
				t.Fatalf("ally did not follow the party (%.0f -> %.0fpx)", d0, d)
			}
			if limit := (monsterPkg.AllyFollowDistanceTiles + 0.5) * ts; d > limit {
				t.Errorf("ally settled %.1f tiles away, want within %.1f",
					d/ts, monsterPkg.AllyFollowDistanceTiles)
			}
		})
	}
}

func wallBlockedMeleeEscort(t *testing.T) (*MMGame, *GameLoop, *monsterPkg.Monster3D) {
	t.Helper()
	game, gl, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 8, 10, ts)
	game.world.Tiles[10][10] = world.TileWall
	game.collisionSystem.UpdateTileChecker(game.world)

	ally := monsterPkg.NewMonster3DFromConfig(11.5*ts, 10.5*ts, "bear", game.config)
	if ally == nil {
		t.Fatal("bear config missing")
	}
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	if game.collisionSystem.CheckLineOfSight(ally.X, ally.Y, game.camera.X, game.camera.Y) {
		t.Fatal("test wall does not block the escort's line of sight")
	}
	return game, gl, ally
}

func TestMeleeAllyRoutesAroundWallWithinFollowDistance(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*MMGame, *GameLoop)
	}{
		{
			name: "real time",
			run: func(game *MMGame, _ *GameLoop) {
				game.turnBasedMode = false
				runRTFoeTicks(game, 4*game.config.GetTPS())
			},
		},
		{
			name: "turn based",
			run: func(game *MMGame, gl *GameLoop) {
				for i := 0; i < 6; i++ {
					runOneMonsterTurn(game, gl)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			game, gl, ally := wallBlockedMeleeEscort(t)
			startX, startY := ally.X, ally.Y
			tc.run(game, gl)

			if ally.X == startX && ally.Y == startY {
				t.Fatal("melee ally froze at follow distance behind a wall")
			}
			if !game.collisionSystem.CheckLineOfSight(ally.X, ally.Y, game.camera.X, game.camera.Y) {
				t.Errorf("melee ally moved but did not route to a visible escort position")
			}
		})
	}
}

// Hunting still wins over escorting: with an enemy on the map the ally takes it
// as its target and closes on IT, not on the party.
func TestAllyHuntsEnemyBeforeFollowing(t *testing.T) {
	for _, key := range allyKeys {
		t.Run(key, func(t *testing.T) {
			game, gl, ally, ts := allyFollowWorld(t, key)
			enemy := monsterPkg.NewMonster3DFromConfig(30*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
			enemy.MaxHitPoints, enemy.HitPoints = 4000, 4000
			game.world.Monsters = append(game.world.Monsters, enemy)
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

			game.refreshMonsterAIState()
			if ally.AIFoe != enemy {
				t.Fatalf("ally must hunt the enemy, AIFoe = %v", ally.AIFoe)
			}
			d0 := Distance(enemy.X, enemy.Y, ally.X, ally.Y)
			for i := 0; i < 12; i++ {
				runOneMonsterTurn(game, gl)
			}
			if d := Distance(enemy.X, enemy.Y, ally.X, ally.Y); d >= d0 {
				t.Errorf("ally did not close on its enemy (%.0f -> %.0fpx)", d0, d)
			}
		})
	}
}

// All three summon sources go through the one shared spawner, so an ally is
// marked and registered identically no matter who called it.
func TestEveryAllySummonPathUsesTheSharedSpawner(t *testing.T) {
	assertAlly := func(t *testing.T, g *MMGame, m *monsterPkg.Monster3D, wantOwner string) {
		t.Helper()
		if m == nil {
			t.Fatal("no ally spawned")
		}
		if !m.Bound || m.BoundFramesRemaining != 0 || !m.QuestProgressIgnored || m.SummonedBy != wantOwner {
			t.Errorf("ally marking = Bound:%v frames:%d questIgnored:%v by:%q, want true/0/true/%q",
				m.Bound, m.BoundFramesRemaining, m.QuestProgressIgnored, m.SummonedBy, wantOwner)
		}
		if !isPurePartySummon(m) {
			t.Error("ally is not a pure party summon")
		}
		// EXACTLY once: a second registerSpawnedMonster gave one summon two AI
		// passes and two save entries, and "is it in the world" could not see it.
		seen := 0
		for _, w := range g.world.Monsters {
			if w == m {
				seen++
			}
		}
		if seen != 1 {
			t.Errorf("ally appears %d times in the world roster, want exactly 1", seen)
		}
		if g.collisionSystem != nil && g.collisionSystem.GetEntityByID(m.ID) == nil {
			t.Error("ally was not registered with the collision system")
		}
	}

	t.Run("card", func(t *testing.T) {
		game, _ := summonTileWorld(t)
		cs := game.combat
		if n := cs.summonCardAllies("masked_huntress", 1); n != 1 {
			t.Fatalf("summonCardAllies = %d, want 1", n)
		}
		assertAlly(t, cs.game, cs.game.world.Monsters[len(cs.game.world.Monsters)-1], cardSummonOwner)
	})

	t.Run("spell", func(t *testing.T) {
		game, _ := summonTileWorld(t)
		cs := game.combat
		id := spells.SpellID("summon_ice_elemental")
		def, err := spells.GetSpellDefinitionByID(id)
		if err != nil {
			t.Fatalf("%s definition: %v", id, err)
		}
		caster := cs.game.party.Members[0]
		if !cs.tryCastSummon(def, caster) {
			t.Fatal("summon spell must be handled by the summon path")
		}
		ally := cs.game.world.Monsters[len(cs.game.world.Monsters)-1]
		assertAlly(t, cs.game, ally, summonSpellOwner(id))
		// The summon cap counts only this spell's own allies.
		before := len(cs.game.world.Monsters)
		if !cs.tryCastSummon(def, caster) {
			t.Fatal("a capped summon still consumes the cast")
		}
		if len(cs.game.world.Monsters) != before {
			t.Errorf("summon_max %d exceeded: %d -> %d allies",
				def.SummonMax, before, len(cs.game.world.Monsters))
		}
	})
}

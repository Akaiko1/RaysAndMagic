package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Control identity is independent of duration: racial Bind is permanent, and
// pure allies use Bound only for allegiance. Other debuffs still affect allies.
func TestStatusReviewControlIdentity(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, owner := range []string{cardSummonOwner, cardSummonOwnerPrefix + "1", animalBondingOwnerPrefix + "Druid", spellSummonOwnerPrefix + "ice_elemental", "timed_bind", "permanent_bind", "boss:necromancer"} {
			t.Run(fmt.Sprintf("%s/saved=%v", owner, saved), func(t *testing.T) {
				g, _, wm := clockSaveFixture(t)
				var m *monster.Monster3D
				want := visualBind
				switch owner {
				case "timed_bind", "boss:necromancer":
					m = monster.NewMonster3DFromConfig(400, 400, "skeleton", g.config)
					if owner == "boss:necromancer" {
						m.SummonedBy = owner
					}
					g.combat.applyBindUndead(m, 10, "Bind Undead")
					g.world.Monsters = append(g.world.Monsters, m)
				case "permanent_bind":
					m = monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
					c := g.party.Members[0]
					c.Race = "dark_elf"
					c.EnsureRacialTraits(nil)
					g.combat.racialProcRoll = func(int) bool { return true }
					if !g.combat.tryDarkElfBindInstead(c, m) {
						t.Fatal("racial Bind failed")
					}
					g.world.Monsters = append(g.world.Monsters, m)
				default:
					m = g.combat.spawnPartyAlly("wolf", owner)
					want = 0
				}
				if m == nil {
					t.Fatal("no summoned actor")
				}
				id := m.ID
				if saved {
					save := auditSaveJSON(t, g.buildSave(wm))
					g.restoreSavedMonsters(wm, &save)
					m = nil
					for _, candidate := range g.world.Monsters {
						if candidate.ID == id {
							m = candidate
							break
						}
					}
					if m == nil {
						t.Fatal("actor lost on load")
					}
				}
				if got := monsterStatusVisuals(m, false); got != want {
					t.Fatalf("control visual=%v, want %v", got, want)
				}
				g.combat.applyMonsterRoot(m, 2, 240)
				if got := monsterStatusVisuals(m, false); got != want|visualRoot {
					t.Fatal("allegiance hid an unrelated root")
				}
			})
		}
	}
}

func TestStatusReviewTurnVisualLifetime(t *testing.T) {
	for _, tc := range statusVisualCatalog {
		// Only these motifs latch a final held TB turn.
		if tc.flag&(visualRoot|visualSlow|visualWeaken) == 0 {
			continue
		}
		for _, reapply := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reapply=%v", tc.key, reapply), func(t *testing.T) {
				g, gl, _ := clockSaveFixture(t)
				m := monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				g.turnBasedMode = true
				tc.monster(m)
				for range 2 {
					g.tickMonsterTurnStatuses(m, true)
					gl.endMonsterTurn()
				}
				g.currentTurn = 1
				g.tickMonsterTurnStatuses(m, true)
				if got := monsterStatusVisuals(m, true); got != tc.flag {
					t.Fatal("final monster turn lost its cue")
				}
				if got := monsterStatusVisuals(m, false); got != 0 {
					t.Fatal("completed budget leaked outside the monster phase")
				}
				if reapply {
					tc.monster(m)
				}
				gl.endMonsterTurn()
				want := statusVisuals(0)
				if reapply {
					want = tc.flag
				}
				if got := monsterStatusVisuals(m, false); got != want {
					t.Fatal("party phase cue does not match remaining budget")
				}
				if m.RootHeld() {
					t.Fatal("completed root hold survived turn boundary")
				}
				root, slow, weak := m.TurnDebuffLatches()
				if root || slow != 0 || weak != 0 {
					t.Fatal("completed percentage latch survived turn boundary")
				}
			})
		}
	}
}

func TestStatusReviewProvidenceCleanup(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, marker := range []string{"", "bless", "heroism"} {
			for _, paid := range []bool{false, true} {
				t.Run(fmt.Sprintf("saved=%v/marker=%s/paid=%v", saved, marker, paid), func(t *testing.T) {
					g, _, wm := clockSaveFixture(t)
					g.dayNightFrames = g.dayNightPhaseStartFrame(true) - 1
					g.dayNightIsNight = false
					g.addStatBuff(TimedStatBuff{SpellID: "bless", SourceID: celestialProvidenceSourceID, Frames: 1})
					g.addCombatBuff(TimedCombatBuff{SpellID: "stone_skin", SourceID: celestialProvidenceSourceID, Frames: 1})
					g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: 1000})
					g.celestialBuffSpellID = marker
					if saved {
						save := auditSaveJSON(t, g.buildSave(wm))
						g.restoreSavedEffects(&save)
					}
					if paid {
						g.advanceDayNightToPhase(true)
						g.finishDayNightSkipImmediately()
					} else {
						g.updateDayNight()
					}
					if _, ok := g.statBuffByID("bless"); ok {
						t.Fatal("stale marker stranded stat Providence")
					}
					if _, ok := g.combatBuffByID("stone_skin"); ok {
						t.Fatal("stale marker stranded combat Providence")
					}
					if _, ok := g.combatBuffByID("heroism"); !ok {
						t.Fatal("marker cleanup removed manual cast")
					}
				})
			}
		}
	}
}

func TestCardMoveBurstObstacles(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"clear", "wall", "closed_door", "opened_door", "blocked_endpoint", "outside_radius"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, kind), func(t *testing.T) {
				g, ts := summonTileWorld(t)
				setTestWorldManager(t, nil)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 10, 10, ts)
				g.cardSlots[0].key = "gorilla_titan_card"
				d := cardDef("gorilla_titan_card")
				old := d.CardMoveAoePct
				d.CardMoveAoePct = 100
				t.Cleanup(func() { d.CardMoveAoePct = old })
				targetX := 14.5 * ts
				want := true
				switch kind {
				case "wall":
					for y := range g.world.Tiles {
						g.world.Tiles[y][12] = world.TileWall
					}
					want = false
				case "closed_door", "opened_door":
					// One closed room separator with a single door opening.
					for y := range g.world.Tiles {
						g.world.Tiles[y][12] = world.TileWall
					}
					g.world.Tiles[10][12] = world.TileEmpty
					door := collision.NewSightBlockingEntity("door", 12.5*ts, 10.5*ts, ts, ts, collision.CollisionTypeNPC, true)
					g.collisionSystem.RegisterEntity(door)
					want = kind == "opened_door"
					if want {
						g.collisionSystem.UnregisterEntity(door.ID)
					}
				case "blocked_endpoint":
					g.world.Tiles[10][14] = world.TileWall
					want = false
				case "outside_radius":
					targetX = 17.5 * ts
					want = false
				}
				m := monster.NewMonster3DFromConfig(targetX, 10.5*ts, "mummy", g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				g.cardBurstTileX, g.cardBurstTileY = 10, 10
				ih := &InputHandler{game: g}
				if tb {
					if !ih.moveTurnBasedInDirection(1, 0) {
						t.Fatal("movement failed")
					}
				} else {
					ih.movePlayer(ts, 0)
				}
				if got := m.HitPoints < m.MaxHitPoints; got != want {
					t.Fatalf("damaged=%v, want %v", got, want)
				}
				if m.WasAttacked != want {
					t.Fatalf("awakened=%v, want %v", m.WasAttacked, want)
				}
				if len(g.spellHitEffects) == 0 {
					t.Fatal("legal nearby empty ground lost rubble")
				}
				targetPainted := false
				for _, fx := range g.spellHitEffects {
					for _, p := range fx.Particles {
						if !g.attackLineClear(g.camera.X, g.camera.Y, p.X, p.Y) {
							t.Fatal("rubble spawned on blocked ground")
						}
						targetPainted = targetPainted || (p.X == targetX && p.Y == 10.5*ts)
					}
				}
				if targetPainted != want {
					t.Fatalf("target ground painted=%v, want %v", targetPainted, want)
				}
				if g.screenShake != 0 {
					t.Fatal("movement proc added camera shake")
				}
			})
		}
	}
}

func TestStatusReviewBatchedGeometry(t *testing.T) {
	for _, size := range []float64{47, 112, 800} {
		b := &statusPathBatch{}
		c := statusCanvas{w: size, h: size, clock: 42, batch: b}
		c.build(visualRoot)
		if b.used > 14 {
			t.Fatalf("root uses %d paths at %v pixels, want at most 14", b.used, size)
		}
		for _, e := range statusVisualCatalog {
			b.used = 0
			c.build(e.flag)
			// Badges ride above the head; every other motif paints geometry.
			if badge := e.flag&statusBadgeVisuals != 0; badge != (b.used == 0) {
				t.Fatalf("%s: badge=%v but %d paths", e.key, badge, b.used)
			}
		}
		c.visible = func(float64, float64) bool { return false }
		b.used = 0
		c.build(visualRoot | visualSlow | visualCharm)
		if b.used != 0 {
			t.Fatal("occluded geometry entered batch")
		}
	}
}

func TestStatusReviewSavedTurnPhases(t *testing.T) {
	for _, phase := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("phase=%d", phase), func(t *testing.T) {
			g, gl, wm := clockSaveFixture(t)
			g.turnBasedMode, g.currentTurn = phase != 2, phase%2
			m := monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
			m.ApplySlow(50, 1, 1)
			m.TickSlowTurn()
			m.ApplyWeaken(50, 1, 1)
			m.TickWeakenTurn()
			g.combat.applyMonsterRoot(m, 2, 240)
			g.world.Monsters = []*monster.Monster3D{m}
			save := auditSaveJSON(t, g.buildSave(wm))
			g.restoreSavedMonsters(wm, &save)
			m = g.world.Monsters[0]
			want := visualRoot
			if phase == 1 {
				want |= visualSlow | visualWeaken
			}
			if got := monsterStatusVisuals(m, g.turnBasedMode && g.currentTurn == 1); got != want {
				t.Fatalf("restored phase visual=%v want %v", got, want)
			}
			gl.endMonsterTurn()
			if got := monsterStatusVisuals(m, false); got != visualRoot {
				t.Fatal("saved turn lost live root or retained expired debuffs")
			}
		})
	}
}

func TestStatusReviewQuakeSpellPolicy(t *testing.T) {
	g, ts := summonTileWorld(t)
	t.Chdir("../..")
	wm := world.NewWorldManager(g.config)
	wm.MapConfigs = map[string]*config.MapConfig{"arena": {SkyTexture: "arena_panorama"}}
	wm.CurrentMapKey = "arena"
	wm.LoadedMaps = map[string]*world.World3D{"arena": g.world}
	setTestWorldManager(t, wm)
	placePlayerAtTile(g, 10, 10, ts)
	for y := range g.world.Tiles {
		g.world.Tiles[y][12] = world.TileWall
	}
	m := monster.NewMonster3DFromConfig(13.5*ts, 10.5*ts, "mummy", g.config)
	m.HitPoints, m.MaxHitPoints = 10000, 10000
	g.world.Monsters = []*monster.Monster3D{m}
	equipSpellAndPrepareCaster(t, g.combat, "earthquake", 200, 60)
	if !g.combat.CastEquippedSpell() {
		t.Fatal("outdoor Earthquake failed to cast")
	}
	if m.HitPoints == m.MaxHitPoints {
		t.Fatal("card visibility filter changed nova damage policy")
	}
	painted := false
	for _, fx := range g.spellHitEffects {
		for _, p := range fx.Particles {
			if p.Solid && p.X > 13*ts {
				painted = true
			}
		}
	}
	if !painted {
		t.Fatal("card visibility filter changed spell rubble policy")
	}
	if g.screenShake <= 0 {
		t.Fatal("Earthquake lost its own screen shake")
	}
}

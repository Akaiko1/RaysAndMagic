package game

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func deathTestGame(t *testing.T) *MMGame {
	t.Helper()
	cfg := loadTestConfig(t)
	setTestWorldManager(t, nil)
	g := newTestGame(cfg, newTestWorldSized(cfg, 7, 7))
	g.combat = NewCombatSystem(g)
	g.sprites = graphics.NewSpriteManager()
	g.reusableDeadSet = make(map[string]bool)
	g.reusableEncounterRewardsMap = make(map[*monster.EncounterRewards]int)
	t.Chdir("../..")
	return g
}

// Every finalization entry point shares the same presentation/reward split in
// both combat modes. Summons and monsters without a death sheet keep their
// reward rules; corpse rendering never keeps an actor alive or colliding.
func TestMonsterDeathLifecycle(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, path := range []string{"direct", "immediate", "indirect"} {
			for _, kind := range []string{"bandit", "no_animation", "summon"} {
				t.Run(fmt.Sprintf("tb=%v/%s/%s", tb, path, kind), func(t *testing.T) {
					g := deathTestGame(t)
					g.turnBasedMode = tb
					key := kind
					if kind == "summon" || kind == "no_animation" {
						key = "bandit"
					}
					ts := g.config.GetTileSize()
					m := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, key, g.config)
					if kind == "no_animation" {
						def, err := monster.MonsterConfig.GetMonsterByKey(key)
						if err != nil {
							t.Fatal(err)
						}
						withoutArt := *def
						withoutArt.Sprite = "test_missing_death_animation"
						m.SetupMonsterFromConfig(&withoutArt)
					}
					m.HitPoints = 0
					m.Gold = 11
					if kind == "summon" {
						m.SummonedBy = spellSummonOwnerPrefix + "test"
					}
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					gl := &GameLoop{game: g}
					switch path {
					case "direct":
						g.combat.finishMonsterKill(m)
					case "immediate":
						g.combat.finishMonsterKillImmediately(m)
					case "indirect":
						gl.finalizeIndirectKills()
					}
					corpses, bags := len(g.monsterCorpses), len(g.groundContainers)
					g.combat.finishMonsterKill(m) // same-frame duplicate cannot pay twice
					if corpses != len(g.monsterCorpses) || bags != len(g.groundContainers) {
						t.Fatal("duplicate kill produced another corpse/reward")
					}
					gl.removeDeadMonstersByID()
					if len(g.world.Monsters) != 0 || g.collisionSystem.GetEntityByID(m.ID) != nil {
						t.Fatal("dead actor still active/colliding")
					}
					wantCorpses := 1
					if kind == "no_animation" {
						wantCorpses = 0
					}
					if corpses != wantCorpses {
						t.Fatalf("corpses=%d want %d", corpses, wantCorpses)
					}
					if kind == "summon" {
						if bags != 0 {
							t.Fatal("party summon paid loot")
						}
					} else {
						if bags != 1 || g.groundContainers[0].Gold != 11 {
							t.Fatalf("reward changed: %+v", g.groundContainers)
						}
						c := &g.groundContainers[0]
						if kind == "bandit" {
							if !c.hop.active(g.frameCount) || math.Abs(c.X-m.X)+math.Abs(c.Y-m.Y) != ts {
								t.Fatal("animated monster did not hop one tile")
							}
							if g.findGroundContainerIndex(10000, nil) != -1 {
								t.Fatal("airborne loot is pickable")
							}
							g.frameCount += c.hop.duration
							if g.findGroundContainerIndex(10000, nil) != 0 {
								t.Fatal("landed loot is not pickable")
							}
						} else if c.X != m.X || c.Y != m.Y || c.hop.duration != 0 {
							t.Fatal("legacy drop moved")
						}
					}
					if corpses > 0 {
						c := &g.monsterCorpses[0]
						tps := float64(g.config.GetTPS())
						cfg := g.monsterDeathSettings()
						collapse := float64(c.frameCount-1) / float64(cfg.FPS)
						for _, sample := range []struct {
							age   float64
							frame int
							alpha float32
						}{
							{0, 0, 1}, {collapse, c.frameCount - 1, 1}, {collapse + cfg.FadeSeconds/2, c.frameCount - 1, 0.5}, {collapse + cfg.FadeSeconds, c.frameCount - 1, 0},
						} {
							g.frameCount = c.started + int64(math.Round(sample.age*tps))
							frame, alpha := g.corpseFrameAndOpacity(c)
							if frame != sample.frame || math.Abs(float64(alpha-sample.alpha)) > 0.01 {
								t.Fatalf("age %v: frame=%d alpha=%v", sample.age, frame, alpha)
							}
						}
						g.updateMonsterDeaths()
						if len(g.monsterCorpses) != 0 {
							t.Fatal("corpse retained after fade")
						}
					}
				})
			}
		}
	}
}

func TestMonsterLootLandingAndHop(t *testing.T) {
	for _, test := range []string{"one_exit", "no_exit", "solid_prop", "map_edge"} {
		t.Run(test, func(t *testing.T) {
			g := deathTestGame(t)
			ts := g.config.GetTileSize()
			m := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, "bandit", g.config)
			for y := range g.world.Tiles {
				for x := range g.world.Tiles[y] {
					g.world.Tiles[y][x] = world.TileWall
				}
			}
			g.world.Tiles[3][3] = world.TileEmpty
			wantX, wantY := m.X, m.Y
			switch test {
			case "one_exit":
				g.world.Tiles[3][4] = world.TileEmpty
				wantX = 4.5 * ts
			case "solid_prop":
				g.world.Tiles[3][4] = world.TileEmpty
				g.collisionSystem.RegisterEntity(collision.NewEntity("closed_door", 4.5*ts, 3.5*ts, ts, ts, collision.CollisionTypeNPC, true))
			case "map_edge":
				m.X, m.Y = 0.5*ts, 0.5*ts
				wantX, wantY = m.X, 1.5*ts
				g.world.Tiles[0][0] = world.TileEmpty
				g.world.Tiles[1][0] = world.TileEmpty
			}
			g.addMonsterLootDrop(m, []items.Item{{Name: "Test trophy"}}, 17)
			c := &g.groundContainers[0]
			if c.X != wantX || c.Y != wantY {
				t.Fatalf("landing %v,%v want %v,%v", c.X, c.Y, wantX, wantY)
			}
			ox, oy := g.groundContainerRenderOffset(c)
			if c.X+ox != m.X || c.Y+oy != m.Y || g.lootHopHeight(c) != 0 {
				t.Fatal("hop did not start at body")
			}
			g.frameCount = c.hop.started + c.hop.duration/2
			if g.lootHopHeight(c) <= 0 {
				t.Fatal("hop has no arc")
			}
			wm := world.NewWorldManager(g.config)
			wm.CurrentMapKey = "forest"
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
			save := g.buildSave(wm)
			if len(save.GroundContainers) != 1 || save.GroundContainers[0].X != wantX || save.GroundContainers[0].Y != wantY || save.GroundContainers[0].Gold != 17 {
				t.Fatalf("mid-hop save lost landing/reward: %+v", save.GroundContainers)
			}
			g.beginMonsterDeath(m)
			g.clearTransientCombatState()
			if len(g.monsterCorpses) != 0 || c.hop.active(g.frameCount) || c.Gold != 17 {
				t.Fatal("world swap did not settle transient visuals/preserve loot")
			}
			ox, oy = g.groundContainerRenderOffset(c)
			if ox != 0 || oy != 0 || g.lootHopHeight(c) != 0 {
				t.Fatal("settled drop retained offset")
			}
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			if len(g.groundContainers) != 1 {
				t.Fatal("reload lost loot")
			}
			restored := g.groundContainers[0]
			if restored.X != wantX || restored.Y != wantY || restored.Gold != 17 || restored.hop.duration != 0 || len(g.monsterCorpses) != 0 {
				t.Fatal("reload lost landing/reward or replayed transient visuals")
			}

		})
	}
}

// Flight x combat mode covers the shared finalizer. Save/load while falling
// settles the persistent reward, just as saving during a ground loot hop does.
func TestMonsterDeathFlight(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, flying := range []bool{false, true} {
			t.Run(fmt.Sprintf("tb=%v/flying=%v", tb, flying), func(t *testing.T) {
				g := deathTestGame(t)
				g.turnBasedMode = tb
				ts := g.config.GetTileSize()
				m := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, "bandit", g.config)
				m.Flying, m.HitPoints, m.Gold = flying, 0, 17
				g.combat.finishMonsterKill(m)
				c, bag := &g.monsterCorpses[0], &g.groundContainers[0]
				if c.flying != flying {
					t.Fatal("finalizer lost flight state")
				}
				cfg, tps := g.monsterDeathSettings(), float64(g.config.GetTPS())
				const ground, size = 600.0, 120.0
				air := monsterFlyingBottom(g.config.GetScreenHeight(), ground, size)
				for _, part := range []float64{0, 0.5, 1} {
					g.frameCount = int64(part * cfg.FallSeconds * tps)
					want := ground
					if flying {
						want = air + (ground-air)*part*part
					}
					if got := g.corpseBottom(c, ground, size); math.Abs(got-want) > 0.01 {
						t.Fatalf("fall part=%v bottom=%v want=%v", part, got, want)
					}
					if bag.hop.waiting(g.frameCount) != (flying && part < 1) {
						t.Fatal("loot visibility does not match landing")
					}
					if flying {
						if _, alpha := g.corpseFrameAndOpacity(c); alpha != 1 {
							t.Fatal("flying body faded before landing")
						}
						if g.findGroundContainerIndex(10000, nil) != -1 {
							t.Fatal("loot is pickable before landing and hop finish")
						}
					}
				}
				g.frameCount = bag.hop.started + bag.hop.duration
				if g.findGroundContainerIndex(10000, nil) != 0 {
					t.Fatal("landed loot unavailable")
				}
				fadeStart := float64(c.frameCount-1) / float64(cfg.FPS)
				if flying {
					fadeStart = math.Max(fadeStart, cfg.FallSeconds)
				}
				g.frameCount = int64((fadeStart + cfg.FadeSeconds/2) * tps)
				if _, alpha := g.corpseFrameAndOpacity(c); math.Abs(float64(alpha)-0.5) > 0.01 {
					t.Fatal("fade did not start after landing/collapse")
				}
				g.frameCount = int64(0.5 * cfg.FallSeconds * tps)
				wm := world.NewWorldManager(g.config)
				wm.CurrentMapKey = "forest"
				wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
				save := g.buildSave(wm)
				x, y := bag.X, bag.Y
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
				if len(g.monsterCorpses) != 0 || len(g.groundContainers) != 1 {
					t.Fatal("reload replayed corpse or lost reward")
				}
				got := g.groundContainers[0]
				if got.X != x || got.Y != y || got.Gold != 17 || got.hop.active(g.frameCount) {
					t.Fatal("reload did not settle reward")
				}
			})
		}
	}
}

func TestMonsterDeathAssetsAndPrewarm(t *testing.T) {
	g := deathTestGame(t)
	for _, name := range []string{"walking_r", "attacking_r", "dying_r"} {
		a := g.sprites.GetAnimation("bandit", name)
		if a == nil || len(a.Frames) != 4 {
			t.Fatalf("bandit %s missing frames", name)
		}
		for _, frame := range a.Frames {
			if frame.Bounds().Dx() != 512 || frame.Bounds().Dy() != 512 {
				t.Fatal("animation scale drift")
			}
		}
	}
	requests := mapRenderSourceRequests(mapRenderPrewarmPlan{monsterSprites: []mapMonsterPrewarmResource{{key: "bandit", spriteName: "bandit"}}})
	for _, name := range []string{"dying_r", "dying_l"} {
		found := false
		for _, req := range requests {
			if req.Name == "bandit" && req.AnimationType == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("death resource %s missing from streaming plan", name)
		}
	}
}

// Every configured definition, including aliases, must resolve its authored
// animations through the runtime sprite manager and source prewarm plan.
func TestMonsterAnimationAssets(t *testing.T) {
	g := deathTestGame(t)
	keys := monster.MonsterConfig.GetAllMonsterKeys()
	slices.Sort(keys)
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			m := monster.NewMonster3DFromConfig(224, 224, key, g.config)
			name := m.GetSpriteType()
			kinds := []string{"walking", "attacking", "dying"}
			// Passive wildlife, transport and the support idol have no attack.
			passive := m.Arboreal != nil || name == "desert_rabbit" || name == "desert_caravan" || name == "deep_jungle_idol"
			if passive {
				kinds = []string{"walking", "dying"}
			}
			fish := m.IsFish()
			if fish {
				kinds = nil // Only the special leaping animation is authored.
			}
			kinds = append(kinds, monsterSpecialAnimations(key)...)
			requests := mapRenderSourceRequests(mapRenderPrewarmPlan{monsterSprites: []mapMonsterPrewarmResource{{key: key, spriteName: name}}})
			for _, kind := range kinds {
				resolved := kind + "_r"
				a := g.sprites.GetAnimation(name, resolved)
				if a == nil {
					resolved = kind + "_l"
					a = g.sprites.GetAnimation(name, resolved)
				}
				if a == nil || len(a.Frames) != 4 {
					t.Fatalf("%s/%s: missing four-frame animation", key, kind)
				}
				for _, frame := range a.Frames {
					if frame.Bounds().Dx() != 512 || frame.Bounds().Dy() != 512 {
						t.Fatalf("%s/%s: changed logical frame size", key, resolved)
					}
				}
				found := false
				for _, req := range requests {
					found = found || (req.Name == name && req.AnimationType == resolved)
				}
				if !found {
					t.Fatalf("%s missing from source prewarm", resolved)
				}
			}
			if !passive && !fish {
				g.armMonsterAttackAnimation(m)
				if m.AttackAnimFrames != animationDurationFrames(g.config.GetTPS(), AuthoredMonsterAttackFPS, 4) {
					t.Fatal("authored attack did not receive the full animation window")
				}
			}
			m.HitPoints = 0
			before := len(g.monsterCorpses)
			g.combat.finishMonsterKill(m)
			if fish {
				if len(g.monsterCorpses) != before {
					t.Fatal("fish must disappear without a corpse animation")
				}
				return
			}
			if len(g.monsterCorpses) != before+1 || g.monsterCorpses[before].spriteName != name {
				t.Fatal("death animation not wired to this monster definition")
			}
		})
	}
}

// Invariant: final hit effects, corpse and loot share the actor's visible
// anchor, while dead actors never remain eligible combat targets.
// Case table: RT diagonal = true position; TB front/back = true position;
// TB clear left/right diagonal = pulled position; occluded diagonal = true
// position; band/transit offsets remain additive in both modes. Persistence is
// N/A for this geometry: the world-swap/save tests settle the transient effects.
func TestMonsterDeathKeepsVisualAnchor(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		tb                        bool
		dx, dy                    int
		blocked, banded, wantPull bool
	}{
		{"rt_diagonal", false, 1, 1, false, false, false},
		{"rt_stack", false, 1, 1, false, true, false},
		{"tb_front", true, 1, 0, false, false, false},
		{"tb_back", true, -1, 1, false, false, false},
		{"tb_right", true, 1, 1, false, false, true},
		{"tb_left", true, 1, -1, false, false, true},
		{"tb_blocked", true, 1, 1, true, false, false},
		{"tb_stack", true, 1, 1, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := deathTestGame(t)
			g.turnBasedMode = tc.tb
			ts := g.config.GetTileSize()
			placePlayerAtTile(g, 2, 2, ts)
			g.camera.Angle = 0
			if tc.blocked {
				g.world.Tiles[2][3] = world.TileWall
				g.world.Tiles[3][2] = world.TileWall
			}
			m := monster.NewMonster3DFromConfig((2.5+float64(tc.dx))*ts, (2.5+float64(tc.dy))*ts, "bandit", g.config)
			if tc.banded {
				m.BandStackCount = 3
				m.BandStackIndex = 1
			}
			_, _, _, pulled, ok := g.combat.pulledFrontSlot(m)
			if (ok && pulled) != tc.wantPull {
				t.Fatal("fixture geometry differs from expected pull")
			}
			x, y := g.combat.monsterVisualPos(m)
			m.HitPoints = 0
			g.combat.finishMonsterKill(m)
			c := g.monsterCorpses[0]
			if c.x != x || c.y != y {
				t.Fatal("corpse snapped from its visible combat slot")
			}
			if _, _, _, _, ok := g.combat.pulledFrontSlot(m); ok {
				t.Fatal("dead actor became targetable")
			}
			if len(g.groundContainers) == 0 || g.groundContainers[0].hop.fromX != x || g.groundContainers[0].hop.fromY != y {
				t.Fatal("loot did not leave the visible body")
			}
		})
	}
}

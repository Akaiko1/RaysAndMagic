package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestFishRegistrationRejectsInvalidState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*monster.Monster3D)
		valid bool
	}{
		{"ordinary", func(m *monster.Monster3D) { m.Disposition = ""; m.FishLeap = nil }, true},
		{"leaping", func(*monster.Monster3D) {}, true},
		{"missing", func(m *monster.Monster3D) { m.FishLeap = nil }, false},
		{"zero_duration", func(m *monster.Monster3D) { m.FishLeap.Duration = 0 }, false},
		{"nonfinite", func(m *monster.Monster3D) { m.FishLeap.ToX = math.NaN() }, false},
		{"negative_height", func(m *monster.Monster3D) { m.FishLeap.PeakHeight = -1 }, false},
		{"invalid_progress", func(m *monster.Monster3D) { m.FishLeap.Progress = 2 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, tile := fishTestGame(t, "forest")
			m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "common_carp", g.config)
			m.FishLeap = &monster.FishLeapState{FromX: m.X, FromY: m.Y, ToX: m.X + tile, ToY: m.Y, Duration: 2, PeakHeight: .7}
			tc.edit(m)
			var failure any
			func() {
				defer func() { failure = recover() }()
				g.registerSpawnedMonster(m)
			}()
			if (failure == nil) != tc.valid {
				t.Fatalf("registration failure=%v, valid=%v", failure, tc.valid)
			}
			if !tc.valid {
				if !strings.Contains(fmt.Sprint(failure), "common_carp") {
					t.Fatalf("missing actor context: %v", failure)
				}
				if len(g.world.Monsters) != 0 || len(g.fishWorlds) != 0 || g.collisionSystem.GetEntityByID(m.ID) != nil {
					t.Fatal("invalid fish partly registered before failure")
				}
			} else if m.IsFish() && len(g.fishWorlds) != 1 {
				t.Fatal("valid flight not tracked")
			}
		})
	}
}

func TestFishLandingWithoutPlayerEntity(t *testing.T) {
	for _, collision := range []string{"none", "headless", "player"} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/blocked=%v", collision, blocked), func(t *testing.T) {
				g, _, tile := fishTestGame(t, "forest")
				if collision == "none" {
					g.collisionSystem = nil
				}
				if collision == "headless" {
					g.collisionSystem.UnregisterEntity("player")
				}
				deep, _ := world.GlobalTileManager.GetTileTypeFromKey("deep_water")
				g.world.Tiles[10][12], g.world.Tiles[10][13] = deep, deep
				if blocked {
					g.world.Tiles[10][11] = world.TileWall
				}
				land := g.fishDestinations("forest", 12, 10, false)
				want := 3
				if blocked {
					want = 2
				}
				if len(land) != want {
					t.Fatalf("bank destinations=%v, want %d", land, want)
				}
				m := &monster.Monster3D{X: 12.5 * tile, Y: 10.5 * tile}
				x, y := g.fishLootLanding(m)
				if !g.world.CanMoveTo(x, y) || Distance(x, y, m.X, m.Y) != tile {
					t.Fatalf("scale did not land on adjacent reachable bank: (%g,%g)", x, y)
				}
			})
		}
	}
}

func TestFishUpdateTracksOnlyLiveWorlds(t *testing.T) {
	for _, exit := range []string{"empty", "landed", "killed", "travel"} {
		t.Run(exit, func(t *testing.T) {
			g, wm, _ := fishTestGame(t, "forest")
			old := g.world
			g.frameCount = 1 // Not a spawn-roll frame.
			if exit != "empty" {
				m := g.spawnLeapingFish("forest", "common_carp", [2]int{12, 10}, config.GlobalEcology.Fish, 1)
				if len(g.fishWorlds) != 1 {
					t.Fatal("spawn was not tracked")
				}
				switch exit {
				case "landed":
					m.AdvanceFishLeap(m.FishLeap.Duration)
				case "killed":
					g.combat.ApplyDamageToMonster(m, 100, "Iron Sword", false)
					g.gameLoop.removeDeadMonstersByID()
				case "travel":
					g.world = newTestWorldSized(g.config, 30, 30)
					wm.LoadedMaps["other"], wm.CurrentMapKey = g.world, "other"
				}
				g.updateFish()
			}
			if len(g.fishWorlds) != 0 {
				t.Fatal("finished fish kept its world active")
			}
			if len(old.Monsters) != 0 {
				t.Fatal("finished fish remained in its world")
			}
			if allocs := testing.AllocsPerRun(20, g.updateFish); allocs != 0 {
				t.Fatalf("empty fish update allocated %g times", allocs)
			}
		})
	}
}

func TestFishExcludedFromAllySelectionAndCommit(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"common_carp", "koi", "rainbow_salmon", "goblin"} {
			t.Run(fmt.Sprintf("%s/TB=%v", kind, tb), func(t *testing.T) {
				g, _, tile := fishTestGame(t, "forest")
				g.turnBasedMode = tb
				ally := monster.NewMonster3DFromConfig(11.5*tile, 10.5*tile, "goblin", g.config)
				ally.Bound = true
				target := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, kind, g.config)
				target.MaxHitPoints, target.HitPoints = 5000, 5000
				target.ArmorClass, target.PerfectDodge = 0, 0
				if target.IsFish() {
					target.FishLeap = &monster.FishLeapState{FromX: target.X, FromY: target.Y, ToX: target.X + tile, ToY: target.Y, Duration: 2, PeakHeight: .7}
				}
				g.registerSpawnedMonster(ally)
				g.registerSpawnedMonster(target)
				g.refreshMonsterAIState()
				want := kind == "goblin"
				if (ally.AIFoe == target) != want {
					t.Fatalf("ally selection=%v, want target=%v", ally.AIFoe, want)
				}
				// Even a stale planned target cannot bypass the commit-time policy.
				ally.AIFoe = target
				cadence := monsterAttackRealtime
				if tb {
					cadence = monsterAttackTurn
				}
				spent := g.combat.commitMonsterAttack(ally, monsterAttackDestination{foe: target}, cadence)
				if spent != want || (target.HitPoints < target.MaxHitPoints) != want {
					t.Fatalf("commit=%v HP=%d, want hit=%v", spent, target.HitPoints, want)
				}
			})
		}
	}
}

func TestFishLoadDiscardsFlightsInUnsavedWorlds(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%v", legacy), func(t *testing.T) {
			g, wm, tile := fishTestGame(t, "forest")
			savedWorld := g.world
			save := g.buildSave(wm)
			if legacy {
				save.MapMonsters = nil
			}
			// New maps absent from either save format retain their ordinary roster.
			extra := newTestWorldSized(g.config, 30, 30)
			wm.LoadedMaps["unsaved"] = extra
			ordinary := monster.NewMonster3DFromConfig(5.5*tile, 5.5*tile, "goblin", g.config)
			extra.Monsters = append(extra.Monsters, ordinary)
			g.world = extra
			fish := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "common_carp", g.config)
			fish.FishLeap = &monster.FishLeapState{FromX: fish.X, FromY: fish.Y, ToX: fish.X + tile, ToY: fish.Y, Duration: 2, PeakHeight: .7}
			g.registerSpawnedMonster(fish)
			g.world = savedWorld
			g.spawnLeapingFish("forest", "common_carp", [2]int{12, 10}, config.GlobalEcology.Fish, 0)
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			if len(g.fishWorlds) != 0 || len(g.world.Monsters) != 0 {
				t.Fatal("load retained transient fish")
			}
			if len(extra.Monsters) != 1 || extra.Monsters[0] != ordinary {
				t.Fatal("load retained an unsaved flight or erased an ordinary monster")
			}
			if len(g.groundContainers) != 0 {
				t.Fatal("load awarded beach loot for a discarded flight")
			}
		})
	}
}

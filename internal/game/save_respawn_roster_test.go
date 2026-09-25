package game

import (
	"fmt"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

var respawnRosterStates = []struct {
	name                     string
	charmed, pacified, bound bool
	dead                     bool
}{
	{name: "hostile"},
	{name: "charmed", charmed: true},
	{name: "pacified", pacified: true},
	{name: "bound", bound: true},
	{name: "dead charm", charmed: true, dead: true},
	{name: "dead pacified", pacified: true, dead: true},
}

// Load case table (crossed with prior roster state, map locality and format):
// Empty + missing/zero stamp -> fresh authored roster, stamped today.
// Empty + positive stamp -> cleared roster stays empty.
// Populated + missing/zero/positive stamp -> saved roster and its allies survive.
// Every row must discard prior-timeline monsters, including after resave/load.
// RT and TB share applySave; farming maps cannot be merged into open world.
func TestSaveRespawnRosterReplacesPreviousTimeline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		populated bool
		stamp     int // -1 means absent; 0 is explicitly unknown.
	}{
		{name: "empty missing stamp", stamp: -1},
		{name: "empty zero stamp"},
		{name: "empty stamped", stamp: 1},
		{name: "populated missing stamp", populated: true, stamp: -1},
		{name: "populated zero stamp", populated: true},
		{name: "populated stamped", populated: true, stamp: 1},
	} {
		for _, state := range respawnRosterStates {
			for _, active := range []bool{false, true} {
				for _, version := range []int{0, respawnDaySaveVersion} {
					t.Run(fmt.Sprintf("%s/%s/active=%v/version=%d", tc.name, state.name, active, version), func(t *testing.T) {
						g, wm := dailyTimerTestGame(t)
						w, mapKey := g.world, "timer_test"
						if !active {
							mapKey = "inactive_farm"
							w = newTestWorld(g.config)
							w.MonsterSpawns = g.world.MonsterSpawns
							wm.LoadedMaps[mapKey] = w
							wm.MapConfigs[mapKey] = &config.MapConfig{RespawnDays: 3}
						}
						if tc.populated {
							for i := 0; i < 3; i++ {
								m := monster.NewMonster3DFromConfig(64, 64, "bandit", g.config)
								m.CharmedByParty, m.Pacified = i == 1, i == 2
								w.Monsters = append(w.Monsters, m)
							}
						}
						saved := g.buildSave(wm)
						saved.RespawnDayVersion = version
						saved.MapRespawnDay = nil
						if tc.stamp >= 0 {
							saved.MapRespawnDay = map[string]int{mapKey: tc.stamp}
						}
						wantCount, wantStamp := 0, max(0, tc.stamp)
						if tc.populated {
							wantCount = 3
						} else if tc.stamp <= 0 {
							wantCount, wantStamp = len(w.MonsterSpawns), 1
						}
						wantSaved := saved.MapMonsters[mapKey]
						for load := 0; load < 2; load++ {
							foreign := monster.NewMonster3DFromConfig(64, 64, "dragon", g.config)
							foreign.CharmedByParty, foreign.Pacified, foreign.Bound = state.charmed, state.pacified, state.bound
							if state.dead {
								foreign.HitPoints = 0
							}
							w.Monsters = []*monster.Monster3D{foreign}
							w.LastRespawnDay = 999
							if active {
								w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
							}
							if err := g.applySave(wm, &saved); err != nil {
								t.Fatal(err)
							}
							if len(w.Monsters) != wantCount || w.LastRespawnDay != wantStamp {
								t.Fatalf("load %d: roster/stamp = %d/%d, want %d/%d", load, len(w.Monsters), w.LastRespawnDay, wantCount, wantStamp)
							}
							if g.collisionSystem.GetEntityByID(foreign.ID) != nil {
								t.Fatal("collision system retained a previous-timeline monster")
							}
							for i, m := range w.Monsters {
								if m.Key != "bandit" || m.ID == foreign.ID {
									t.Fatal("loaded roster retained a previous-timeline monster")
								}
								if tc.populated {
									ms := wantSaved[i]
									if m.ID != ms.ID || m.Pacified != ms.Pacified || m.CharmedByParty != (ms.CharmedByParty || ms.Pacified) {
										t.Fatal("load lost a saved monster's identity or charm state")
									}
								}
								if active && g.collisionSystem.GetEntityByID(m.ID) == nil {
									t.Fatal("restored monster is missing from collision system")
								}
							}
							saved = g.buildSave(wm)
						}
					})
				}
			}
		}
	}
}

// Gameplay case table: unknown age and expired stamps rebuild authored spawns,
// preserving only living charms/pacified monsters. A fresh stamp changes none.
func TestRespawnArrivalPreservesOnlyCurrentTimelineAllies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stamp   int
		rebuild bool
	}{
		{name: "unknown age", stamp: 0, rebuild: true},
		{name: "expired", stamp: 1, rebuild: true},
		{name: "fresh", stamp: 4},
	} {
		for _, state := range respawnRosterStates {
			t.Run(tc.name+"/"+state.name, func(t *testing.T) {
				g, _ := dailyTimerTestGame(t)
				g.calendarDay = 4
				g.world.LastRespawnDay = tc.stamp
				m := monster.NewMonster3DFromConfig(64, 64, "dragon", g.config)
				m.CharmedByParty, m.Pacified, m.Bound = state.charmed, state.pacified, state.bound
				if state.dead {
					m.HitPoints = 0
				}
				g.world.Monsters = []*monster.Monster3D{m}
				g.maybeRespawnMapMonsters()
				wantKept := !tc.rebuild || (!state.dead && (state.charmed || state.pacified))
				kept, authored := false, 0
				for _, got := range g.world.Monsters {
					if got == m {
						kept = true
					} else if got.Key == "bandit" {
						authored++
					} else {
						t.Fatal("unexpected monster in respawn roster")
					}
				}
				wantAuthored := 0
				if tc.rebuild {
					wantAuthored = len(g.world.MonsterSpawns)
				}
				if kept != wantKept || authored != wantAuthored || g.world.LastRespawnDay != 4 {
					t.Fatalf("kept/authored/stamp = %v/%d/%d, want %v/%d/4", kept, authored, g.world.LastRespawnDay, wantKept, wantAuthored)
				}
			})
		}
	}
}

package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func dailyTimerTestGame(t *testing.T) (*MMGame, *world.WorldManager) {
	t.Helper()
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	w.MonsterSpawns = []world.MonsterSpawn{{X: 0, Y: 0, MonsterKey: "bandit"}}
	wm := world.NewWorldManager(cfg)
	wm.CurrentMapKey = "timer_test"
	wm.LoadedMaps = map[string]*world.World3D{"timer_test": w}
	wm.MapConfigs = map[string]*config.MapConfig{"timer_test": {RespawnDays: 3}}
	previous := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	g := newTestGame(cfg, w)
	g.calendarDay, g.calendarWeek, g.calendarMonth = 1, 1, 1
	return g, wm
}

func TestDailyRefreshBoundaries(t *testing.T) {
	for _, turnBased := range []bool{false, true} {
		for _, paidWait := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/wait=%v", turnBased, paidWait), func(t *testing.T) {
				g, _ := dailyTimerTestGame(t)
				g.turnBasedMode = turnBased
				g.maybeRespawnMapMonsters()
				if g.world.LastRespawnDay != 1 || len(g.world.Monsters) != 1 {
					t.Fatal("first arrival must stamp and spawn on day 1")
				}
				g.world.Monsters = nil
				g.arenaTierFoughtDay = map[string]int{"easy": 0, "normal": 0, "impossible": 0}
				cases := []struct {
					night   bool
					day     int
					respawn bool
				}{
					{true, 1, false}, {false, 2, false}, {true, 2, false},
					{false, 3, false}, {true, 3, false}, {false, 4, true},
				}
				for _, tc := range cases {
					if paidWait {
						g.advanceDayNightToPhase(tc.night)
						finishDayNightSkip(t, g)
					} else {
						g.dayNightFrames = g.dayNightCycleFrames() / 4
						if !tc.night {
							g.dayNightFrames *= 3
						}
						g.updateDayNight()
					}
					if g.currentCalendarDay() != tc.day || g.dayNightIsNight != tc.night {
						t.Fatalf("phase night=%v: date/phase = %d/%v, want %d/%v", tc.night, g.currentCalendarDay(), g.dayNightIsNight, tc.day, tc.night)
					}
					for tier := range g.arenaTierFoughtDay {
						if g.arenaTierSpentToday(tier) {
							t.Fatalf("day %d night=%v: tier %s must reopen at every phase change", tc.day, tc.night, tier)
						}
						g.arenaTierFoughtDay[tier] = g.dayNightDay
					}
					g.maybeRespawnMapMonsters()
					if (len(g.world.Monsters) > 0) != tc.respawn {
						t.Fatalf("day %d night=%v: respawn=%v, want %v", tc.day, tc.night, len(g.world.Monsters) > 0, tc.respawn)
					}
				}
				if g.world.LastRespawnDay != 4 {
					t.Fatalf("spawn date = %d, want day 4", g.world.LastRespawnDay)
				}
				g.world.Monsters = nil
				g.maybeRespawnMapMonsters()
				if len(g.world.Monsters) != 0 {
					t.Fatal("re-entering on the respawn day must not spawn twice")
				}
			})
		}
	}
}

func TestDailyTimersSaveMigration(t *testing.T) {
	cases := []struct {
		name                                                             string
		version, phase, day, arena, spawn, wantDay, wantArena, wantSpawn int
	}{
		{"legacy first day", 0, 0, 0, 0, 1, 1, 0, 1},
		{"legacy dusk keeps same day", 0, 5, 0, 4, 4, 3, 4, 2},
		{"legacy night stamp", 0, 5, 3, 5, 6, 3, 5, 3},
		{"legacy dawn unlocks prior night", 0, 6, 4, 5, 2, 4, 5, 1},
		{"current dates are not converted again", respawnDaySaveVersion, 6, 4, 4, 3, 4, 4, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, wm := dailyTimerTestGame(t)
			other := newTestWorld(g.config)
			wm.LoadedMaps["other"] = other
			g.world.Monsters = []*monster.Monster3D{monster.NewMonster3DFromConfig(64, 64, "bandit", g.config)}
			saved := g.buildSave(wm)
			saved.RespawnDayVersion = tc.version
			saved.DayNightDay = tc.phase
			saved.CalendarDay = tc.day
			saved.CalendarWeek, saved.CalendarMonth = 0, 0
			saved.ArenaTierFoughtDay = map[string]int{"easy": tc.arena}
			saved.MapRespawnDay = map[string]int{"timer_test": tc.spawn, "other": tc.spawn}
			// Exercise the persisted representation, not only a migration helper.
			data, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			var source GameSave
			if err := json.Unmarshal(data, &source); err != nil {
				t.Fatal(err)
			}
			if err := g.applySave(wm, &source); err != nil {
				t.Fatal(err)
			}
			if g.currentCalendarDay() != tc.wantDay || g.arenaTierFoughtDay["easy"] != tc.wantArena {
				t.Fatalf("date/arena = %d/%d, want %d/%d", g.currentCalendarDay(), g.arenaTierFoughtDay["easy"], tc.wantDay, tc.wantArena)
			}
			for key, w := range wm.LoadedMaps {
				if w.LastRespawnDay != tc.wantSpawn {
					t.Fatalf("map %s spawn day=%d, want %d", key, w.LastRespawnDay, tc.wantSpawn)
				}
			}
			if source.ArenaTierFoughtDay["easy"] != tc.arena || source.MapRespawnDay["timer_test"] != tc.spawn || source.RespawnDayVersion != tc.version {
				t.Fatal("loading mutated the caller's snapshot")
			}
			resaved := g.buildSave(wm)
			if resaved.RespawnDayVersion != respawnDaySaveVersion {
				t.Fatal("save omitted daily timer version")
			}
			if err := g.applySave(wm, &resaved); err != nil {
				t.Fatal(err)
			}
			if g.world.LastRespawnDay != tc.wantSpawn || g.arenaTierFoughtDay["easy"] != tc.wantArena {
				t.Fatal("second load converted calendar dates twice")
			}
			if g.arenaTierSpentToday("easy") != (tc.wantArena == tc.phase) {
				t.Fatal("restored arena lock disagrees with phase")
			}
			g.world.Monsters = nil
			g.maybeRespawnMapMonsters()
			if (len(g.world.Monsters) > 0) != (tc.wantDay-tc.wantSpawn >= 3) {
				t.Fatal("restored spawn date did not enforce three calendar days")
			}
		})
	}
}

func TestDailyTimersSaveDuringWait(t *testing.T) {
	for _, night := range []bool{true, false} {
		t.Run(fmt.Sprintf("waitToNight=%v", night), func(t *testing.T) {
			g, wm := dailyTimerTestGame(t)
			g.maybeRespawnMapMonsters()
			g.arenaTierFoughtDay = map[string]int{"easy": 0}
			g.advanceDayNightToPhase(night)
			saved := g.buildSave(wm)
			want := 2
			if night {
				want = 1
			}
			if saved.CalendarDay != want || saved.MapRespawnDay["timer_test"] != 1 || saved.ArenaTierFoughtDay["easy"] != 0 {
				t.Fatalf("wait save date/stamps = %d/%v/%v", saved.CalendarDay, saved.MapRespawnDay, saved.ArenaTierFoughtDay)
			}
			if err := g.applySave(wm, &saved); err != nil {
				t.Fatal(err)
			}
			if g.arenaTierSpentToday("easy") {
				t.Fatal("save during wait failed to release the phase-based arena lock")
			}
		})
	}
}

func TestDailyTimersLoadWithoutStampDoesNotInheritPreviousSave(t *testing.T) {
	for _, version := range []int{0, respawnDaySaveVersion} {
		for _, emptyRoster := range []bool{false, true} {
			for _, explicitZero := range []bool{false, true} {
				t.Run(fmt.Sprintf("version=%d/empty=%v/zero=%v", version, emptyRoster, explicitZero), func(t *testing.T) {
					g, wm := dailyTimerTestGame(t)
					g.maybeRespawnMapMonsters()
					saved := g.buildSave(wm)
					saved.RespawnDayVersion = version
					saved.MapRespawnDay = nil
					if explicitZero {
						saved.MapRespawnDay = map[string]int{"timer_test": 0}
					}
					if emptyRoster {
						saved.MapMonsters["timer_test"] = nil
					}
					g.world.LastRespawnDay = 999 // State from another slot must not survive load.
					g.world.Monsters = []*monster.Monster3D{monster.NewMonster3DFromConfig(64, 64, "dragon", g.config)}
					if err := g.applySave(wm, &saved); err != nil {
						t.Fatal(err)
					}
					wantStamp := 0
					if emptyRoster {
						wantStamp = 1
					} // Existing legacy empty-roster recovery.
					if g.world.LastRespawnDay != wantStamp {
						t.Fatalf("loaded unstamped date=%d, want %d", g.world.LastRespawnDay, wantStamp)
					}
					g.maybeRespawnMapMonsters()
					if g.world.LastRespawnDay != 1 || len(g.world.Monsters) != 1 || g.world.Monsters[0].Key != "bandit" {
						t.Fatal("unstamped arrival did not establish a fresh calendar date and authored roster")
					}
				})
			}
		}
	}
}

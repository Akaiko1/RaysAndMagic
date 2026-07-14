package game

import (
	"testing"

	"ugataima/internal/world"
)

func TestBuildSaveCommitsPendingDayNightSkip(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	g.calendarDay, g.calendarWeek, g.calendarMonth = 1, 1, 1
	wm := &world.WorldManager{CurrentMapKey: "forest", LoadedMaps: map[string]*world.World3D{"forest": w}}
	previous := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	t.Cleanup(func() { world.GlobalWorldManager = previous })

	beforeArenaPhase := g.dayNightDay
	g.advanceDayNightToPhase(false) // day -> night -> next dawn
	if !g.dayNightSkipActive || !g.dayNightIsNight {
		t.Fatal("expected pending skip after daytime wait to dawn")
	}

	save := g.buildSave(wm)
	if g.dayNightSkipActive || len(g.dayNightSkipPhases) != 0 {
		t.Fatal("buildSave must flush the transient day/night skip")
	}
	if g.dayNightIsNight || g.dayNightDay != beforeArenaPhase+2 {
		t.Fatalf("flushed game phase = night:%v arena:%d, want day and %d", g.dayNightIsNight, g.dayNightDay, beforeArenaPhase+2)
	}
	if g.calendarDay != 2 || g.calendarWeek != 1 || g.calendarMonth != 1 {
		t.Fatalf("flushed calendar = %d/%d/%d, want 2/1/1", g.calendarDay, g.calendarWeek, g.calendarMonth)
	}
	if save.DayNightFrames != g.dayNightFrames || save.DayNightDay != g.dayNightDay || save.CalendarDay != g.calendarDay {
		t.Fatal("save did not capture the completed day/night state")
	}
}

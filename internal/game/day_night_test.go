package game

import (
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestDayNightPhaseBoundaries(t *testing.T) {
	tests := []struct {
		frac  float64
		night bool
	}{
		{0, false},
		{0.25, false},
		{0.26, true},
		{0.5, true},
		{0.75, true},
		{0.76, false},
	}
	for _, tt := range tests {
		if got := dayNightIsNightAt(tt.frac); got != tt.night {
			t.Errorf("isNight(%.2f) = %v, want %v", tt.frac, got, tt.night)
		}
	}
}

func TestDayNightFracWrapsCycle(t *testing.T) {
	g := &MMGame{config: &config.Config{}}
	cycle := g.dayNightCycleFrames()
	if cycle != 2*7*60*config.GetTargetTPS() {
		t.Fatalf("default cycle = %d frames", cycle)
	}
	g.dayNightFrames = cycle + cycle/2
	if got := g.dayNightFrac(); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("frac at wrapped midnight = %.4f, want 0.5", got)
	}
}

func TestSkyVariantName(t *testing.T) {
	if got := skyVariantName("forest_panorama", false); got != "forest_panorama_day" {
		t.Errorf("day variant = %q", got)
	}
	if got := skyVariantName("forest_panorama", true); got != "forest_panorama_night" {
		t.Errorf("night variant = %q", got)
	}
}

func TestDespawnPackMonstersNonCurrentMapFilters(t *testing.T) {
	tag := dayNightPackTag("forest", true)
	w := &world.World3D{Monsters: []*monster.Monster3D{
		{ID: "a", PackKey: tag},
		{ID: "b"},
		{ID: "c", PackKey: dayNightPackTag("forest", false)},
	}}
	g := &MMGame{} // g.world != w -> non-current path (plain filter)
	g.despawnPackMonsters(w, tag)
	if len(w.Monsters) != 2 || w.Monsters[0].ID != "b" || w.Monsters[1].ID != "c" {
		t.Fatalf("filtered monsters = %+v", w.Monsters)
	}
}

func TestDespawnPackMonstersCurrentMapQueuesIDs(t *testing.T) {
	tag := dayNightPackTag("forest", false)
	w := &world.World3D{Monsters: []*monster.Monster3D{
		{ID: "a", PackKey: tag},
		{ID: "b"},
	}}
	g := &MMGame{world: w}
	g.despawnPackMonsters(w, tag)
	if len(w.Monsters) != 2 {
		t.Fatalf("current-map despawn must defer removal to the frame-end sweep")
	}
	if len(g.deadMonsterIDs) != 1 || g.deadMonsterIDs[0] != "a" {
		t.Fatalf("queued dead IDs = %v", g.deadMonsterIDs)
	}
}

// The pack clear-gate censuses a REGION rect (the unified world holds five
// maps), so the test drives the same helper the spawner calls.
func TestWorldHasLivingMonstersInRect(t *testing.T) {
	const tile = 64.0
	w := &world.World3D{Monsters: []*monster.Monster3D{{X: 96, Y: 96, HitPoints: 0}, nil}}
	if worldHasLivingMonstersInRect(w, 0, 0, 10, 10, tile) {
		t.Fatal("dead monsters must not block a clear-gated pack")
	}
	w.Monsters = append(w.Monsters, &monster.Monster3D{X: 96, Y: 96, HitPoints: 1})
	if !worldHasLivingMonstersInRect(w, 0, 0, 10, 10, tile) {
		t.Fatal("a living monster must block a clear-gated pack")
	}
	if worldHasLivingMonstersInRect(w, 20, 20, 5, 5, tile) {
		t.Fatal("a monster outside the rect must not gate it")
	}
}

// The cliff dragons return only after the cliffs are completely cleared.
func TestDragonCliffsNightPackRequiresFullClear(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, pack := range cfg.DayNight.Packs {
		if pack.Map != "dragon_cliffs" {
			continue
		}
		if !pack.RequireMapClear {
			t.Fatalf("dragon cliffs night pack = %+v, want require_map_clear", pack)
		}
		return
	}
	t.Fatal("dragon cliffs night pack missing")
}

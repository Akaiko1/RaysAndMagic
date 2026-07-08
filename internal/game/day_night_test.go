package game

import (
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestDayNightLightScaleCurve(t *testing.T) {
	const day, night = 1.0, 0.7
	tests := []struct {
		frac float64
		want float64
	}{
		{0, day},     // noon
		{0.5, night}, // midnight
		{0.25, 0.85}, // dusk boundary = curve midpoint
		{0.75, 0.85}, // dawn boundary
		{1.0, day},   // wrap
	}
	for _, tt := range tests {
		if got := dayNightLightScale(tt.frac, day, night); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("scale(%.2f) = %.4f, want %.4f", tt.frac, got, tt.want)
		}
	}
	// Smooth monotonic descent from noon to midnight.
	prev := dayNightLightScale(0, day, night)
	for f := 0.05; f <= 0.5; f += 0.05 {
		cur := dayNightLightScale(f, day, night)
		if cur >= prev {
			t.Fatalf("scale not decreasing at frac %.2f: %.4f -> %.4f", f, prev, cur)
		}
		prev = cur
	}
}

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

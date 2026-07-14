package game

import (
	"math"
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// AoE splash must burst its hit FX where each victim is DRAWN, not at its raw
// tile. A banded stack snaps all members onto ONE tile (leader's X,Y) and the
// renderer fans them out to read as several; a splash victim that is a fanned
// follower was getting its explosion at the shared tile - visually over empty
// space, a couple tiles from where the puma is drawn.
func TestAoeSplashFX_AnchorsOnBandFannedPosition(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ts := float64(g.config.GetTileSize())

	// Two banded pumas snapped onto the SAME tile (how the band pass leaves a
	// stack): leader index 0, follower index 1 - fanned apart only at draw time.
	center := &monsterPkg.Monster3D{
		Name: "Puma", HitPoints: 200, MaxHitPoints: 200,
		X: 10 * ts, Y: 10 * ts, BandStackCount: 2, BandStackIndex: 0,
	}
	follower := &monsterPkg.Monster3D{
		Name: "Puma", HitPoints: 200, MaxHitPoints: 200,
		X: 10 * ts, Y: 10 * ts, BandStackCount: 2, BandStackIndex: 1,
	}
	g.world.Monsters = []*monsterPkg.Monster3D{center, follower}

	g.spellHitEffects = g.spellHitEffects[:0]
	cs.applyAoeSplash(center, 40, "fire", monsterPkg.DamageFire, "Archmage Staff", 3.0, 0)

	if len(g.spellHitEffects) != 1 {
		t.Fatalf("splash to one follower must spawn exactly one hit effect, got %d", len(g.spellHitEffects))
	}
	parts := g.spellHitEffects[0].Particles
	if len(parts) == 0 {
		t.Fatal("splash hit effect has no particles")
	}
	// The follower's fanned draw position - where the burst must land.
	ox, oy := bandFanOffset(follower.BandStackIndex, follower.BandStackCount, ts)
	wantX, wantY := follower.X+ox, follower.Y+oy
	if ox == 0 && oy == 0 {
		t.Fatal("test setup: follower fan offset is zero, cannot distinguish the bug")
	}
	gotX, gotY := parts[0].X, parts[0].Y
	if math.Abs(gotX-wantX) > 0.01 || math.Abs(gotY-wantY) > 0.01 {
		t.Errorf("splash FX at (%.1f,%.1f), want the follower's DRAWN pos (%.1f,%.1f); "+
			"raw-tile anchor would be (%.1f,%.1f)", gotX, gotY, wantX, wantY, follower.X, follower.Y)
	}
}

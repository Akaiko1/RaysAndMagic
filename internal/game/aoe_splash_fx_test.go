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
func TestAoeSplashFX_AnchorsOnStackPosition(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ts := float64(g.config.GetTileSize())

	// Two pumas on the SAME tile: slots 0 and 1 of the tile fan, drawn apart
	// only at draw time.
	center := &monsterPkg.Monster3D{
		Name: "Puma", HitPoints: 200, MaxHitPoints: 200,
		X: 10.5 * ts, Y: 10.5 * ts, Fan: settledTileFan(0, 2, ts),
	}
	follower := &monsterPkg.Monster3D{
		Name: "Puma", HitPoints: 200, MaxHitPoints: 200,
		X: 10.5 * ts, Y: 10.5 * ts, Fan: settledTileFan(1, 2, ts),
	}
	g.camera.X, g.camera.Y, g.camera.Angle = 8.5*ts, 10.5*ts, 0
	g.world.Monsters = []*monsterPkg.Monster3D{center, follower}
	// The second of two slots stands 0.16 tile along the world line (+X),
	// farther from the camera, so it is drawn there.
	wantX, wantY := 10.66*ts, 10.5*ts

	g.spellHitEffects = g.spellHitEffects[:0]
	attack := cs.newPartyMonsterAttack(40, 0, "fire", 0, nil, "Archmage Staff", false, true, false)
	cs.applyAoeSplash(center, attack, 3.0)

	if len(g.spellHitEffects) != 1 {
		t.Fatalf("splash to one follower must spawn exactly one hit effect, got %d", len(g.spellHitEffects))
	}
	parts := g.spellHitEffects[0].Particles
	if len(parts) == 0 {
		t.Fatal("splash hit effect has no particles")
	}
	// The follower's fanned draw position - where the burst must land.
	drawX, drawY := (&Renderer{game: g}).monsterVisualPosition(follower)
	if math.Hypot(drawX-wantX, drawY-wantY) > .01 {
		t.Fatal("sprite did not use the independent expected stack anchor")
	}
	if wantX == follower.X && wantY == follower.Y {
		t.Fatal("test setup: follower fan offset is zero, cannot distinguish the bug")
	}
	gotX, gotY := parts[0].X, parts[0].Y
	if math.Abs(gotX-wantX) > 0.01 || math.Abs(gotY-wantY) > 0.01 {
		t.Errorf("splash FX at (%.1f,%.1f), want the follower's DRAWN pos (%.1f,%.1f); "+
			"raw-tile anchor would be (%.1f,%.1f)", gotX, gotY, wantX, wantY, follower.X, follower.Y)
	}
}

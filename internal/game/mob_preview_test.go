package game

import (
	"testing"

	"ugataima/internal/monster"
)

// TestTB_SightAggroScattersBand: sight aggro must scatter a band in ANY mode.
// RT sets IsEngagingPlayer in updatePlayerEngagementWithVision; the TB
// scheduler never runs it, so without its own engagement mark a band stayed
// "calm" by flags, re-stacked every frame and chased the party as one pile -
// scattering only when a member took damage.
func TestTB_SightAggroScattersBand(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatalf("NewMobPreview: %v", err)
	}
	g := p.g
	gl := g.gameLoop
	ts := float64(cfg.GetTileSize())

	// Three banding wolves stacked on one tile, 3 tiles in front of the party
	// (inside their authored alert radius, outside melee reach). NOT passive - this is the real
	// game scenario, unlike the preview's staged mobs.
	for i := 0; i < 3; i++ {
		m := monster.NewMonster3DFromConfig(g.camera.X+3*ts, g.camera.Y, "wolf", cfg)
		p.arena.Monsters = append(p.arena.Monsters, m)
	}
	p.arena.RegisterMonstersWithCollisionSystem(g.collisionSystem)

	// Calm tick: the stack forms a band.
	gl.updateMonsterBands()
	for _, m := range p.arena.Monsters {
		if m.BandID == 0 {
			t.Fatalf("wolf %s did not band up while calm", m.ID)
		}
	}

	// One TB monster turn: participation against the party must mark engagement.
	g.turnBasedMode = true
	g.currentTurn = 1
	g.monsterTurnResolved = false
	gl.updateMonstersTurnBased()
	for _, m := range p.arena.Monsters {
		if !m.IsEngagingPlayer {
			t.Errorf("wolf %s acted in the TB turn but is not engaged", m.ID)
		}
		if m.WasAttacked {
			t.Errorf("wolf %s marked as hit by sight aggro (must stay sighted-only)", m.ID)
		}
	}

	// Band pass: the aggroed band dissolves instead of re-stacking.
	gl.updateMonsterBands()
	for _, m := range p.arena.Monsters {
		if m.BandID != 0 {
			t.Errorf("wolf %s still banded after sight aggro", m.ID)
		}
	}

	// TB tile separation: the pile spreads onto distinct tiles.
	g.separateStackedMonstersTB()
	tiles := map[[2]int]int{}
	for _, m := range p.arena.Monsters {
		tiles[[2]int{int(m.X / ts), int(m.Y / ts)}]++
	}
	for tile, n := range tiles {
		if n > 1 {
			t.Errorf("%d wolves still share tile %v after TB separation", n, tile)
		}
	}
}

// TestMobPreview_SpawnAndStep smoke-tests the editor mob sandbox: it must
// stage a single specimen for a plain mob, a whole flock for a banding mob,
// keep them calm (alert radius zeroed), and survive step cycles.
func TestMobPreview_SpawnAndStep(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)

	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatalf("NewMobPreview: %v", err)
	}

	var plain, banding string
	for key, def := range monster.MonsterConfig.Monsters {
		if def.Banding && banding == "" {
			banding = key
		}
		if !def.Banding && plain == "" {
			plain = key
		}
	}
	if plain == "" || banding == "" {
		t.Fatalf("monster roster lacks a plain/banding pair (plain=%q banding=%q)", plain, banding)
	}

	p.Select(plain)
	if got := len(p.Monsters()); got != 1 {
		t.Fatalf("plain mob %q staged %d monsters, want 1", plain, got)
	}
	for i := 0; i < 90; i++ {
		p.Step()
	}

	p.Select(banding)
	if got := len(p.Monsters()); got != mobBandSize {
		t.Fatalf("banding mob %q staged %d monsters, want %d", banding, got, mobBandSize)
	}
	for i := 0; i < 90; i++ {
		p.Step()
	}
	for _, m := range p.Monsters() {
		if !m.IsAlive() {
			t.Errorf("staged mob %q died during preview", m.Key)
		}
		// The stage sits within default detection range of the camera; an
		// engaged mob means the passive staging broke (a band would scatter
		// into a teleport frenzy, ranged mobs would reposition endlessly).
		if m.IsEngagingPlayer {
			t.Errorf("staged mob %q engaged the preview camera; preview mobs must stay calm", m.Key)
		}
	}
}

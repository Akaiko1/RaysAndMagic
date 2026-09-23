package game

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestMobPreviewCaravanRoute(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	t.Chdir("../..")
	before, err := json.Marshal(config.GlobalEcology)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.g.Shutdown)
	for _, key := range []string{"desert_caravan", "wolf", "desert_caravan"} {
		p.Select(key)
		if key == "wolf" {
			if p.g.caravanRoute() != nil {
				t.Fatal("reselection retained the caravan route")
			}
			continue
		}
		route := p.g.caravanRoute()
		if route == nil || len(route.Points) < 2 || p.g.ecology.Checkpoint != 0 {
			t.Fatal("caravan preview did not start a fresh local route")
		}
		m := p.Monsters()[0]
		visited, loops := map[int]bool{}, 0
		for i := 0; i < cfg.GetTPS()*40; i++ {
			checkpoint := p.g.ecology.Checkpoint
			p.Step()
			if checkpoint != p.g.ecology.Checkpoint {
				visited[checkpoint] = true
				if p.g.ecology.Checkpoint == 0 {
					loops++
				}
			}
			if m.IsEngagingPlayer || m.X-p.g.camera.X < float64(cfg.GetTileSize()) {
				t.Fatal("caravan left the stage or engaged the camera")
			}
		}
		if len(visited) != len(route.Points) || loops < 2 {
			t.Fatalf("caravan did not loop: visited=%v loops=%d position=%.1f,%.1f target=%.1f,%.1f", visited, loops, m.X, m.Y, m.AITargetX, m.AITargetY)
		}
	}
	after, _ := json.Marshal(config.GlobalEcology)
	if !bytes.Equal(before, after) || len(p.g.ecology.Stock) != 0 || len(p.g.world.NPCs) != 0 {
		t.Fatal("preview changed campaign routes or produced trade rewards")
	}
}

// Every authored size/disposition starts close and remains clear of the
// camera after Update. Long-running movement is checked on the live renderer.
func TestMobPreviewStageClearance(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := PrimeChampions(cfg); err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.g.Shutdown)
	keys := monster.MonsterConfig.GetAllMonsterKeys()
	slices.Sort(keys)
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			p.Select(key)
			for _, m := range p.Monsters() {
				want := (1.1 + 0.35*m.GetSizeGameMultiplier()) * float64(cfg.GetTileSize())
				if math.Abs(m.X-p.g.camera.X-want) > 1e-6 {
					t.Fatal("original close framing changed")
				}
			}
			before := p.g.uiFrameCount
			p.Step()
			for _, m := range p.Monsters() {
				if m.X-p.g.camera.X < float64(cfg.GetTileSize()) {
					t.Fatal("specimen entered camera foreground")
				}
			}
			if p.g.uiFrameCount != before+1 {
				t.Fatal("presentation clock did not advance exactly once")
			}
			if p.attackTick != 0 {
				t.Fatal("attack cadence advanced before preparation finished")
			}
		})
	}
}

// Every authored fish must render and loop its real leap in the Mobs tab.
// Save/load is N/A: the stage is a transient specimen, not a campaign spawn.
func TestMobPreviewFish(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	t.Chdir("../..")
	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.g.Shutdown)
	for _, key := range config.GlobalEcology.Fish.Species {
		t.Run(key, func(t *testing.T) {
			p.Select(key)
			m := p.Monsters()[0]
			if m.FishLeap == nil {
				t.Fatal("preview fish has no leap state")
			}
			frames := int(math.Ceil(m.FishLeap.Duration * float64(cfg.GetTPS())))
			highest, loops := 0.0, 0
			for i := 0; i < frames*2+2; i++ {
				before := m.FishLeap.Progress
				p.Step()
				if m.FishLeap.Progress < before {
					loops++
				}
				highest = math.Max(highest, m.VisualHeightTiles())
			}
			if highest <= 0 || loops < 2 || len(p.Monsters()) != 1 || len(p.g.groundContainers) != 0 {
				t.Fatalf("flight height=%v loops=%d actors=%d loot=%d", highest, loops, len(p.Monsters()), len(p.g.groundContainers))
			}
			for _, standee := range []bool{false, true} {
				if img, _ := p.g.gameLoop.renderer.specialMotionSprite(m, standee); img == nil {
					t.Fatalf("missing fish preview frame (standee=%v)", standee)
				}
			}
		})
	}
}

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
	t.Cleanup(p.g.Shutdown)
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
	t.Cleanup(p.g.Shutdown)

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

func TestMobPreview_ChampionMirroredBeforeFirstFrame(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatalf("load champions: %v", err)
	}
	if err := PrimeChampions(cfg); err != nil {
		t.Fatalf("prime champions: %v", err)
	}

	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatalf("NewMobPreview: %v", err)
	}
	t.Cleanup(p.g.Shutdown)
	p.Select("hobbit_archer")
	if len(p.Monsters()) != 1 {
		t.Fatalf("champion preview staged %d monsters, want 1", len(p.Monsters()))
	}
	m := p.Monsters()[0]
	if !m.ChampionMirrored {
		t.Fatal("champion still exposes monsters.yaml placeholders immediately after Select")
	}
	if got := m.GetTurnBasedAttackCount(); got != 2 {
		t.Errorf("champion TB attacks = %d, want 2", got)
	}
	if m.ProjectileWeapon != "blowgun" {
		t.Errorf("champion projectile weapon = %q, want impossible-tier blowgun", m.ProjectileWeapon)
	}
	if m.MaxHitPoints != config.GetChampionTier(config.ChampionDefaultTier).HP {
		t.Errorf("champion HP = %d, want impossible-tier HP", m.MaxHitPoints)
	}
}

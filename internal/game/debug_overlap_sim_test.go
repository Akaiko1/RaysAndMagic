package game

// Headless overlap/jitter simulation — a DEBUG MODULE, not a regression test.
// Loads the real forest map, steps the real monster AI + separation pass and
// reports overlap and jitter metrics:
//
//	Phase A (2 sim-minutes): party parked far away — calm monsters patrol.
//	Phase B (1 sim-minute):  party standing next to the densest monster
//	                         cluster — monsters engage and crowd in.
//
// Run with:  RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_ForestOverlap -v
// Skipped by default so the normal suite stays fast.

import (
	"fmt"
	"math"
	"os"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

type overlapStats struct {
	active    map[[2]int]int // pair -> consecutive overlapped ticks so far
	durations []int          // completed overlap episode lengths (ticks)
}

func (st *overlapStats) tick(monsters []*monster.Monster3D) int {
	if st.active == nil {
		st.active = map[[2]int]int{}
	}
	overlapping := 0
	seen := map[[2]int]bool{}
	for i := 0; i < len(monsters); i++ {
		a := monsters[i]
		if !a.IsAlive() {
			continue
		}
		aw, ah := a.GetSize()
		for j := i + 1; j < len(monsters); j++ {
			b := monsters[j]
			if !b.IsAlive() {
				continue
			}
			bw, bh := b.GetSize()
			if math.Abs(b.X-a.X) < (aw+bw)/2 && math.Abs(b.Y-a.Y) < (ah+bh)/2 {
				k := [2]int{i, j}
				st.active[k]++
				seen[k] = true
				overlapping++
			}
		}
	}
	for k, dur := range st.active {
		if !seen[k] {
			st.durations = append(st.durations, dur)
			delete(st.active, k)
		}
	}
	return overlapping
}

func (st *overlapStats) report(t *testing.T, label string, ticks int) {
	maxDur, sum := 0, 0
	for _, d := range st.durations {
		sum += d
		if d > maxDur {
			maxDur = d
		}
	}
	mean := 0.0
	if len(st.durations) > 0 {
		mean = float64(sum) / float64(len(st.durations))
	}
	stuck := 0
	for _, dur := range st.active {
		if dur > ticks/2 {
			stuck++
		}
	}
	t.Logf("[%s] overlap episodes=%d meanDur=%.0f ticks maxDur=%d ticks; still-overlapped pairs=%d (of them 'stuck' >half phase: %d)",
		label, len(st.durations), mean, maxDur, len(st.active), stuck)
}

// jitterTracker measures path-length vs net displacement per monster over a
// window: a high ratio with low net displacement = shaking in place.
type jitterTracker struct {
	lastX, lastY []float64
	winX, winY   []float64
	pathLen      []float64
}

func newJitterTracker(monsters []*monster.Monster3D) *jitterTracker {
	jt := &jitterTracker{
		lastX: make([]float64, len(monsters)), lastY: make([]float64, len(monsters)),
		winX: make([]float64, len(monsters)), winY: make([]float64, len(monsters)),
		pathLen: make([]float64, len(monsters)),
	}
	for i, m := range monsters {
		jt.lastX[i], jt.lastY[i] = m.X, m.Y
		jt.winX[i], jt.winY[i] = m.X, m.Y
	}
	return jt
}

func (jt *jitterTracker) tick(monsters []*monster.Monster3D) {
	for i, m := range monsters {
		jt.pathLen[i] += math.Hypot(m.X-jt.lastX[i], m.Y-jt.lastY[i])
		jt.lastX[i], jt.lastY[i] = m.X, m.Y
	}
}

// flush reports and resets the window; returns the worst jitter ratio.
func (jt *jitterTracker) flush(monsters []*monster.Monster3D) (worst float64, worstIdx int) {
	worstIdx = -1
	for i, m := range monsters {
		net := math.Hypot(m.X-jt.winX[i], m.Y-jt.winY[i])
		if jt.pathLen[i] > 20 { // only monsters that actually tried to move
			ratio := jt.pathLen[i] / math.Max(net, 1)
			if ratio > worst {
				worst, worstIdx = ratio, i
			}
		}
		jt.winX[i], jt.winY[i] = m.X, m.Y
		jt.pathLen[i] = 0
	}
	return worst, worstIdx
}

func TestDebugSim_ForestOverlap(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("assets/weapons.yaml"); err != nil {
		t.Fatalf("weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("assets/items.yaml"); err != nil {
		t.Fatalf("items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	defer func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM }()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	if err := wm.SwitchToMap("forest"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm
	w := wm.GetCurrentWorld()

	g := newTestGame(cfg, w)
	gl := &GameLoop{game: g}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	tps := cfg.GetTPS()
	t.Logf("forest: %d monsters, %d TPS", len(w.Monsters), tps)

	runPhase := func(label string, seconds int, camX, camY float64) {
		g.camera.X, g.camera.Y = camX, camY
		g.collisionSystem.UpdateEntity("player", camX, camY)
		ov := &overlapStats{}
		jt := newJitterTracker(w.Monsters)
		ticks := seconds * tps
		window := 2 * tps
		for tick := 0; tick < ticks; tick++ {
			g.frameCount++
			for _, m := range w.Monsters {
				if !m.IsAlive() {
					continue
				}
				m.Update(g.collisionSystem, camX, camY)
				g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			}
			gl.separateOverlappingMonsters()
			ov.tick(w.Monsters)
			jt.tick(w.Monsters)
			if (tick+1)%window == 0 {
				if worst, idx := jt.flush(w.Monsters); worst > 3 && idx >= 0 {
					m := w.Monsters[idx]
					t.Logf("[%s t=%3ds] worst jitter ratio %.1f: %s at (%.0f,%.0f) state=%v engaged=%v",
						label, (tick+1)/tps, worst, m.Name, m.X, m.Y, m.State, m.IsEngagingPlayer)
				}
			}
		}
		ov.report(t, label, ticks)
	}

	// Phase A: party parked in a far corner — calm monsters patrol for 2 min.
	runPhase("A calm", 120, 64+32, 64+32)

	// Phase B: party standing next to the densest monster cluster for 1 min.
	bestX, bestY, bestCount := 64.0, 64.0, -1
	for _, m := range w.Monsters {
		count := 0
		for _, o := range w.Monsters {
			if math.Hypot(o.X-m.X, o.Y-m.Y) < 64*6 {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestX, bestY = m.X+64*2, m.Y
		}
	}
	t.Logf("phase B: party at (%.0f,%.0f), %d monsters within 6 tiles", bestX, bestY, bestCount)
	runPhase("B party", 60, bestX, bestY)

	// Final state dump for any pair still interlocked.
	for i := 0; i < len(w.Monsters); i++ {
		a := w.Monsters[i]
		aw, ah := a.GetSize()
		for j := i + 1; j < len(w.Monsters); j++ {
			b := w.Monsters[j]
			bw, bh := b.GetSize()
			if math.Abs(b.X-a.X) < (aw+bw)/2 && math.Abs(b.Y-a.Y) < (ah+bh)/2 {
				fmt.Printf("STILL OVERLAPPED: %s(%.0f,%.0f,%v) x %s(%.0f,%.0f,%v)\n",
					a.Name, a.X, a.Y, a.State, b.Name, b.X, b.Y, b.State)
			}
		}
	}
}

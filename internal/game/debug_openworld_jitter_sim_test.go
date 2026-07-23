//go:build debug

package game

// Headless diagnostic for the two open-world rendering reports: distant
// sprites bobbing vertically while the party walks, and overlapping sprites
// flip-flopping their draw order. Walks a straight line on the REAL render
// path and measures both, per tracked sprite.
//
// Run with:
//
//	RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_OpenWorldJitter -v
import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_OpenWorldJitter(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)
	r := g.gameLoop.renderer
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	ts := cfg.GetTileSize()

	// Stand in the open desert looking north: tens of tiles of clear sand
	// with palms/statues in the distance - the long-sightline case the split
	// maps never had.
	desert := wm.OpenWorldRegionByKey("desert")
	startX := (float64(desert.OffsetX) + 25.5) * ts
	startY := (float64(desert.OffsetY) + 45.5) * ts
	angle := -math.Pi / 2 // north

	type sample struct {
		screenY, size int
		topF          float64 // float top edge the draw paths use
		depth         float64
		screenX       int
	}
	type key struct {
		typ          SpriteType
		tileX, tileY int
	}
	series := map[key][]sample{}
	prevOrder := map[key]int{}
	orderSwaps := map[[2]key]int{}

	const frames = 240
	const step = 2.0
	for f := 0; f < frames; f++ {
		g.camera.X, g.camera.Y, g.camera.Angle = startX, startY-float64(f)*step, angle
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen.Clear()
			r.RenderFirstPersonView(screen)
		})
		order := map[key]int{}
		for rank, s := range r.unifiedSprites {
			// Only tile-identified sprite kinds: NPCs/monsters carry no tile
			// identity, so their samples would collapse into one bogus series.
			if s.spriteType != SpriteTypeEnvironment && s.spriteType != SpriteTypeTree {
				continue
			}
			// A cross near another standee can contribute four globally sorted
			// arm entries. Sample one stable representative per tree/frame.
			if s.treeArmOnly && s.treeArmIndex != 0 {
				continue
			}
			k := key{s.spriteType, s.tileX, s.tileY}
			series[k] = append(series[k], sample{s.screenY, s.spriteSize, s.bottomF - s.sizeF, s.depthPerp, s.screenX})
			order[k] = rank
		}
		// Count draw-order inversions vs the previous frame for pairs whose
		// screen spans overlap (the visible flip-flop case).
		for a, ra := range order {
			pa, okA := prevOrder[a]
			if !okA {
				continue
			}
			for b, rb := range order {
				if a == b {
					continue
				}
				pb, okB := prevOrder[b]
				if !okB {
					continue
				}
				if (ra < rb) != (pa < pb) {
					sa, sb := series[a][len(series[a])-1], series[b][len(series[b])-1]
					if absInt(sa.screenX-sb.screenX) < (sa.size+sb.size)/2 {
						p := [2]key{a, b}
						if b.tileX < a.tileX || (b.tileX == a.tileX && b.tileY < a.tileY) {
							p = [2]key{b, a}
						}
						orderSwaps[p]++
					}
				}
			}
		}
		prevOrder = order
	}

	// Vertical wobble: count direction reversals of the sprite edges while
	// walking a straight line - both the INT TOP the old pipeline drew from
	// (its floor/size truncations step out of phase = the reported shake) and
	// the FLOAT TOP the draw paths use now (must be monotonic).
	type wob struct {
		k           key
		intTopRev   int
		floatTopRev int
		maxJump     int
		meanDepth   float64
		n           int
	}
	var wobs []wob
	for k, ss := range series {
		if len(ss) < 60 {
			continue
		}
		intRev, floatRev, maxJump, lastIntDir, lastFloatDir := 0, 0, 0, 0, 0
		depthSum := 0.0
		for i := 1; i < len(ss); i++ {
			d := ss[i].screenY - ss[i-1].screenY
			if d != 0 {
				dir := 1
				if d < 0 {
					dir = -1
				}
				if lastIntDir != 0 && dir != lastIntDir {
					intRev++
				}
				lastIntDir = dir
				if absInt(d) > maxJump {
					maxJump = absInt(d)
				}
			}
			if df := ss[i].topF - ss[i-1].topF; df > 1e-9 || df < -1e-9 {
				dir := 1
				if df < 0 {
					dir = -1
				}
				if lastFloatDir != 0 && dir != lastFloatDir {
					floatRev++
				}
				lastFloatDir = dir
			}
			depthSum += ss[i].depth
		}
		wobs = append(wobs, wob{k, intRev, floatRev, maxJump, depthSum / float64(len(ss)-1) / ts, len(ss)})
	}
	sort.Slice(wobs, func(i, j int) bool { return wobs[i].intTopRev > wobs[j].intTopRev })
	t.Logf("tracked %d sprites over %d frames (straight walk north, %.1f u/frame)", len(wobs), frames, step)
	totalFloatRev := 0
	for _, w := range wobs {
		totalFloatRev += w.floatTopRev
	}
	t.Logf("FLOAT top-edge reversals across ALL tracked sprites: %d (draw paths use these; must be ~0)", totalFloatRev)
	for i, w := range wobs {
		if i >= 12 {
			break
		}
		t.Logf("WOBBLE type=%v tile=(%d,%d) depth~%.1ft intTopReversals=%d floatTopReversals=%d maxJumpPx=%d frames=%d",
			w.k.typ, w.k.tileX, w.k.tileY, w.meanDepth, w.intTopRev, w.floatTopRev, w.maxJump, w.n)
	}

	type swp struct {
		p [2]key
		n int
	}
	var swaps []swp
	for p, n := range orderSwaps {
		swaps = append(swaps, swp{p, n})
	}
	sort.Slice(swaps, func(i, j int) bool { return swaps[i].n > swaps[j].n })
	t.Logf("overlapping draw-order swaps: %d pairs", len(swaps))
	for i, s := range swaps {
		if i >= 12 {
			break
		}
		t.Logf("SWAP %v(%d,%d) <-> %v(%d,%d): %d flips",
			s.p[0].typ, s.p[0].tileX, s.p[0].tileY, s.p[1].typ, s.p[1].tileX, s.p[1].tileY, s.n)
	}
	_ = fmt.Sprintf
}

package game

import (
	"math"
	"math/rand"
	"os"
	"slices"
	"testing"

	"ugataima/internal/character"

	"gopkg.in/yaml.v3"
)

// The painter sort reorders keys, not sprite records. Its order must be the
// stable sort by compareUnifiedSprites, ties and all: an order that differed
// would reshuffle overlapping scenery.
func TestSortSpritesForPaintingMatchesTheStableSort(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	r := &Renderer{}
	for round := 0; round < 40; round++ {
		sprites := make([]UnifiedSpriteRenderData, rng.Intn(400))
		for i := range sprites {
			// Few distinct values per field, so every tie-break gets work.
			sprites[i] = UnifiedSpriteRenderData{
				depthPerp:    float64(rng.Intn(6)) * 64,
				paintAhead:   float64(rng.Intn(2)) * 64,
				tileX:        rng.Intn(3),
				tileY:        rng.Intn(3),
				screenX:      rng.Intn(3),
				treeArmIndex: rng.Intn(4),
				spriteSize:   i, // tells otherwise equal records apart
			}
		}
		want := slices.Clone(sprites)
		slices.SortStableFunc(want, compareUnifiedSprites)
		got := r.sortSpritesForPainting(sprites)
		if len(got) != len(want) {
			t.Fatalf("round %d: %d sprites came back of %d", round, len(got), len(want))
		}
		for i := range want {
			if got[i].spriteSize != want[i].spriteSize {
				t.Fatalf("round %d: place %d holds sprite %d, the stable sort puts %d there", round, i, got[i].spriteSize, want[i].spriteSize)
			}
		}
	}
}

// A prop is skipped only when nothing of it can land on the screen. Points of
// a prop's reach project as projectTriangle does (lateral over depth, depth
// held at the near plane's 1). The screen's region is connected and unbounded,
// so the disc shows a point only if its rim does: the rim is walked densely,
// and half the props sit at the cull's own threshold, where a bound that is
// too tight fails first.
func TestPropOutsideViewNeverHidesAVisiblePoint(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	const rimSteps = 720
	var rimCos, rimSin [rimSteps]float64
	for k := range rimCos {
		rimSin[k], rimCos[k] = math.Sincos(2 * math.Pi * float64(k) / rimSteps)
	}
	culled, shown := 0, 0
	for i := 0; i < 20000; i++ {
		halfFovTan := 0.4 + rng.Float64()*1.2
		z, reach := rng.Float64()*900-150, rng.Float64()*220
		x := (rng.Float64()*2 - 1) * 900
		if i%2 == 0 {
			x = (max(z+reach, 1) + reach/halfFovTan) * (1 + (rng.Float64()*2-1)*0.02)
			if i%4 == 0 {
				x = -x
			}
		}
		outside := propOutsideView(x, z, reach, halfFovTan)
		// A world offset moves depth by one component and the lateral
		// coordinate by the other over halfFovTan.
		visible := math.Abs(x) <= max(z, 1)
		for k := 0; k < rimSteps && !visible; k++ {
			px, pz := x+reach*rimCos[k]/halfFovTan, max(z+reach*rimSin[k], 1)
			visible = math.Abs(px) <= pz
		}
		if outside && visible {
			t.Fatalf("a prop at x=%.1f z=%.1f reach %.1f (half fov tan %.2f) is skipped with a point on screen", x, z, reach, halfFovTan)
		}
		if outside {
			culled++
		} else {
			shown++
		}
	}
	if culled == 0 || shown == 0 {
		t.Fatalf("the sample never met both cases: %d skipped, %d kept", culled, shown)
	}
}

// Every pose of every shipped prop model stays within the reach the view test
// uses, lids and falling crates through their whole travel.
func TestPropModelReachHoldsEveryPose(t *testing.T) {
	data, err := os.ReadFile("../../assets/npcs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var catalog character.NPCConfig
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	specs := map[character.NPCPropModel]string{}
	for key, npc := range catalog.NPCs {
		if npc != nil && npc.PropModel != nil {
			specs[*npc.PropModel] = key
		}
	}
	if len(specs) == 0 {
		t.Fatal("no shipped NPC carries a prop model")
	}
	for spec, key := range specs {
		model := cachedPropModel(spec)
		if model.reach <= 0 {
			t.Fatalf("%s: reach %.3f", key, model.reach)
		}
		for step := 0; step <= 40; step++ {
			pose := propPose{model: model, scale: 1.7, co: math.Cos(0.6), si: math.Sin(0.6), progress: float64(step) / 40}
			check := func(f worldModelTriangle) {
				for _, p := range f.points {
					if d := math.Hypot(p.x, p.y); d > model.reach*pose.scale+1e-9 {
						t.Fatalf("%s at progress %.3f: a point lies %.4f out, reach is %.4f", key, pose.progress, d, model.reach*pose.scale)
					}
				}
			}
			pose.eachFace(check)
			pose.eachFlameFace(check)
		}
	}
}

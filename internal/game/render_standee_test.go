package game

import (
	"image"
	"image/draw"
	"math"
	"reflect"
	"slices"
	"testing"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestCommitPreparedStandeeKeepsLazyLoadedWinner(t *testing.T) {
	key := standeeCoreKey{name: "mob:test"}
	source := ebiten.NewImage(4, 4)
	lazyCore := ebiten.NewImage(4, 4)
	r := &Renderer{standeeCoreCache: map[standeeCoreKey]*ebiten.Image{key: lazyCore}}
	prepared := standeePreparedPixels{
		sticker: image.NewRGBA(image.Rect(0, 0, 4, 4)),
		core:    image.NewRGBA(image.Rect(0, 0, 4, 4)),
	}

	_, core := r.commitPreparedStandeePixels(key, source, prepared)
	if core != lazyCore || r.standeeCoreCache[key] != lazyCore {
		t.Fatal("background commit replaced the lazy-loaded standee core")
	}
}

func TestStandeeRenderSourceSizeBoundsDerivedTexturePayload(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantW, wantH  int
	}{
		{name: "shipped scale stays exact", width: 512, height: 512, wantW: 512, wantH: 512},
		{name: "wide oversized source halves", width: 2048, height: 1024, wantW: 1024, wantH: 512},
		{name: "four k source reaches one megapixel", width: 4096, height: 4096, wantW: 1024, wantH: 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := standeeRenderSourceSize(tt.width, tt.height)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Fatalf("bounded size = %dx%d, want %dx%d", gotW, gotH, tt.wantW, tt.wantH)
			}
			if int64(gotW)*int64(gotH) > standeeRenderSourceMaxPixels {
				t.Fatalf("bounded source still exceeds pixel cap: %dx%d", gotW, gotH)
			}
		})
	}
}

func TestMapRenderUploadFrameBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		images    int
		bytes     int64
		nextBytes int64
		wantFlush bool
	}{
		{name: "empty frame accepts oversized first image", nextBytes: mapRenderUploadFrameBytes + 1, wantFlush: false},
		{name: "under both limits", images: 1, bytes: 1024, nextBytes: 1024, wantFlush: false},
		{name: "byte limit", images: 1, bytes: mapRenderUploadFrameBytes, nextBytes: 1, wantFlush: true},
		{name: "image count limit", images: mapRenderUploadFrameImages, wantFlush: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapRenderUploadFrameFull(tt.images, tt.bytes, tt.nextBytes); got != tt.wantFlush {
				t.Fatalf("flush = %v, want %v", got, tt.wantFlush)
			}
		})
	}
}

// Ray-segment math behind standee tokens: t is the perpendicular depth (ray
// built as dir + plane-s with |dir| = 1), u the position along the token.
func TestStandeeColumnHit(t *testing.T) {
	// Camera at origin looking +X; token crosses the view axis at x=100,
	// spanning y in [-10, +10] (P0 at the bottom, D pointing +Y).
	p0x, p0y, dx, dy := 100.0, -10.0, 0.0, 20.0

	// Center column: straight-ahead ray hits the middle of the token.
	tt, u, ok := standeeColumnHit(0, 0, 1, 0, p0x, p0y, dx, dy)
	if !ok || math.Abs(tt-100) > 1e-9 || math.Abs(u-0.5) > 1e-9 {
		t.Errorf("center: got t=%.3f u=%.3f ok=%v, want t=100 u=0.5", tt, u, ok)
	}

	// Slanted ray r=(1, 0.1): crosses y=10 at x=100 -> the token's far end (u=1).
	tt, u, ok = standeeColumnHit(0, 0, 1, 0.1, p0x, p0y, dx, dy)
	if !ok || math.Abs(tt-100) > 1e-9 || math.Abs(u-1.0) > 1e-9 {
		t.Errorf("edge: got t=%.3f u=%.3f ok=%v, want t=100 u=1", tt, u, ok)
	}

	// Ray missing the segment (crosses beyond its end) -> no hit.
	if _, _, ok = standeeColumnHit(0, 0, 1, 0.2, p0x, p0y, dx, dy); ok {
		t.Errorf("ray past the token end should miss")
	}

	// Ray parallel to the token plane -> no hit (degenerate determinant).
	if _, _, ok = standeeColumnHit(0, 0, 0, 1, p0x, p0y, dx, dy); ok {
		t.Errorf("parallel ray should miss")
	}

	// Token behind the camera -> rejected by the near clip.
	if _, _, ok = standeeColumnHit(200, 0, 1, 0, p0x, p0y, dx, dy); ok {
		t.Errorf("token behind the camera should be rejected")
	}
}

func TestStandeeColumnIntersectionKeepsOffSegmentBoundary(t *testing.T) {
	// A visible edge pixel has a centre ray that hits the segment, while one
	// pixel boundary can intersect its infinite line just outside the segment.
	// The renderer needs that unbounded u to give the quad a non-zero horizontal
	// texture footprint, allowing Ebiten to select a minified mip level.
	p0x, p0y, dx, dy := 100.0, -10.0, 0.0, 20.0
	tt, u, ok := standeeColumnIntersection(0, 0, 1, 0.11, p0x, p0y, dx, dy)
	if !ok || math.Abs(tt-100) > 1e-9 || u <= 1 {
		t.Fatalf("unbounded edge: got t=%.3f u=%.3f ok=%v; want t=100 and u>1", tt, u, ok)
	}
	if _, _, ok := standeeColumnHit(0, 0, 1, 0.11, p0x, p0y, dx, dy); ok {
		t.Fatal("finite segment hit must reject the off-segment boundary")
	}
}

func TestStandeeUsesMinificationSampling(t *testing.T) {
	if standeeUsesMinificationSampling(512, 512, 512, 512) {
		t.Fatal("1:1 standee must retain the crisp path")
	}
	if !standeeUsesMinificationSampling(512, 511.9, 512, 512) {
		t.Fatal("any vertical shrink must use stable linear sampling")
	}
	if !standeeUsesMinificationSampling(511.9, 512, 512, 512) {
		t.Fatal("any horizontal shrink must use stable linear sampling")
	}
}

func TestStandeeProjectedFootprintUsesLeastMinifiedAxis(t *testing.T) {
	if got := standeeProjectedFootprint(128, 64, 256, 256); got != 2 {
		t.Fatalf("anisotropic footprint = %.2f, want 2", got)
	}
	if got := standeeProjectedFootprint(512, 512, 256, 256); got != 1 {
		t.Fatalf("magnified footprint = %.2f, want 1", got)
	}
}

func TestTreeIsBillboardLOD(t *testing.T) {
	const tileSize = 64.0
	if treeIsBillboardLOD(25*tileSize, tileSize, 25) {
		t.Fatal("tree at the boundary must retain crossed slabs")
	}
	if !treeIsBillboardLOD(25.01*tileSize, tileSize, 25) {
		t.Fatal("tree beyond the boundary must use the distant single-plane LOD")
	}
	if treeIsBillboardLOD(tileSize, tileSize, 0) {
		t.Fatal("zero threshold must disable the distant LOD")
	}
}

func TestCrossedTreeArmsInterleaveWithAdjacentStandee(t *testing.T) {
	cfg := loadTestConfig(t)
	game := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	game.camera.FOV = squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	game.camera.ViewDist = cfg.GetViewDistance()
	game.camera.Angle = 0
	tileSize := float64(cfg.GetTileSize())
	game.camera.X, game.camera.Y = tileSize/2, 10.5*tileSize
	game.renderHelper = NewRenderingHelper(game)
	r := &Renderer{game: game}

	const treeTileX, treeTileY = 5, 10
	treeX, treeY := TileCenterFromTile(treeTileX, treeTileY, tileSize)
	treeDepth := treeX - game.camera.X
	footprint := 1.5 * tileSize
	halfFOVTan := math.Tan(game.camera.FOV / 2)
	treeSize := footprint * float64(cfg.GetScreenWidth()) / (2 * halfFOVTan * treeDepth)
	treeScreenX, _, ok := game.renderHelper.projectToScreenX(treeX, treeY)
	if !ok {
		t.Fatal("setup: crossed dune center is not visible")
	}
	tree := UnifiedSpriteRenderData{
		spriteType: SpriteTypeTree,
		screenX:    treeScreenX,
		screenXF:   float64(treeScreenX),
		sizeF:      treeSize,
		depthPerp:  treeDepth,
		distance:   treeDepth,
		tileX:      treeTileX,
		tileY:      treeTileY,
	}

	// Same camera depth, slightly to the screen-right: the standee overlaps the
	// cross in projection but not at its center. A whole-tree depth key cannot
	// represent this; two arms are farther and two are nearer.
	npcX, npcY := treeX, treeY+0.75*tileSize
	npcScreenX, _, ok := game.renderHelper.projectToScreenX(npcX, npcY)
	if !ok {
		t.Fatal("setup: adjacent standee is not visible")
	}
	npcSize := tileSize * float64(cfg.GetScreenWidth()) / (2 * halfFOVTan * treeDepth)
	npc := UnifiedSpriteRenderData{
		spriteType: SpriteTypeNPC,
		screenX:    npcScreenX,
		screenXF:   float64(npcScreenX),
		sizeF:      npcSize,
		depthPerp:  treeDepth,
	}

	sprites := r.splitCrossedTreesForPainterOrder(
		[]UnifiedSpriteRenderData{tree, npc}, 0, 1,
	)
	if len(sprites) != 5 {
		t.Fatalf("adjacent standee produced %d painter entries, want four dune arms + standee", len(sprites))
	}
	slices.SortStableFunc(sprites, compareUnifiedSprites)

	npcIndex := -1
	armsBefore, armsAfter := 0, 0
	for i, s := range sprites {
		if s.spriteType == SpriteTypeNPC {
			npcIndex = i
			continue
		}
		if !s.treeArmOnly {
			t.Fatal("overlapping crossed dune remained a whole-tree painter entry")
		}
		if math.Abs(s.treeCenterDepth-treeDepth) > 1e-9 {
			t.Fatalf("arm projection depth = %.2f, want shared dune-center depth %.2f", s.treeCenterDepth, treeDepth)
		}
	}
	if npcIndex < 0 {
		t.Fatal("adjacent standee disappeared from painter entries")
	}
	for i, s := range sprites {
		if !s.treeArmOnly {
			continue
		}
		if i < npcIndex {
			armsBefore++
		} else {
			armsAfter++
		}
	}
	if armsBefore == 0 || armsAfter == 0 {
		t.Fatalf("standee was not interleaved through dune arms: before=%d after=%d", armsBefore, armsAfter)
	}

	// The split is UNCONDITIONAL: a depth-separated standee still leaves the
	// cross as four arms, because cross-vs-cross occlusion is decided by the
	// same global painter order and cannot depend on an overlap scan.
	farNPC := npc
	farNPC.depthPerp += 4 * tileSize
	always := r.splitCrossedTreesForPainterOrder(
		[]UnifiedSpriteRenderData{tree, farNPC}, 0, 1,
	)
	if len(always) != 5 {
		t.Fatalf("cross produced %d painter entries, want four arms + standee", len(always))
	}
	for _, s := range always {
		if s.spriteType == SpriteTypeTree && !s.treeArmOnly {
			t.Fatal("cross remained a whole-entry despite the unconditional split")
		}
	}
}

func TestStandeeMipBlendIsContinuousAcrossLevels(t *testing.T) {
	tests := []struct {
		name      string
		footprint float32
		wantLevel int
		wantBlend float32
	}{
		{name: "magnified", footprint: 0.5, wantLevel: 0, wantBlend: 0},
		{name: "one to one", footprint: 1, wantLevel: 0, wantBlend: 0},
		{name: "pure lower mip", footprint: float32(math.Pow(2, 0.25)), wantLevel: 0, wantBlend: 0},
		{name: "halfway to level one", footprint: float32(math.Sqrt2), wantLevel: 0, wantBlend: 0.5},
		{name: "pure upper mip", footprint: float32(math.Pow(2, 0.75)), wantLevel: 1, wantBlend: 0},
		{name: "level one", footprint: 2, wantLevel: 1, wantBlend: 0},
		{name: "halfway to level two", footprint: float32(2 * math.Sqrt2), wantLevel: 1, wantBlend: 0.5},
		{name: "clamped", footprint: 256, wantLevel: maxMipLevel, wantBlend: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, blend := mipLevelBlend(tt.footprint, maxMipLevel)
			if level != tt.wantLevel || math.Abs(float64(blend-tt.wantBlend)) > 1e-5 {
				t.Fatalf("mipLevelBlend(%g) = (%d, %.4f), want (%d, %.4f)", tt.footprint, level, blend, tt.wantLevel, tt.wantBlend)
			}
		})
	}
}

func TestStandeeMipSizesMatchEngineDepthCap(t *testing.T) {
	got := mipSizesUniform(128, 64)
	want := []image.Point{
		image.Pt(128, 64), image.Pt(64, 32), image.Pt(32, 16),
		image.Pt(16, 8), image.Pt(8, 4), image.Pt(4, 2), image.Pt(2, 1),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mipSizesUniform(128, 64) = %v, want %v", got, want)
	}
}

func TestDownsampleStandeeMipAveragesPremultipliedPixels(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Pix = []byte{
		0, 20, 40, 60, 40, 60, 80, 100,
		80, 100, 120, 140, 120, 140, 160, 180,
	}
	got := downsampleMip(src, image.Pt(1, 1))
	if got == nil {
		t.Fatal("downsampleMip returned nil")
	}
	want := []byte{60, 80, 100, 120}
	if !reflect.DeepEqual(got.Pix, want) {
		t.Fatalf("downsampled pixel = %v, want %v", got.Pix, want)
	}
}

func TestDownsampleStandeeMipCoversOddSourceEdge(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 7, 10))
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			off := src.PixOffset(x, y)
			src.Pix[off] = byte((y-src.Rect.Min.Y)*3 + x - src.Rect.Min.X + 1)
			src.Pix[off+3] = 255
		}
	}
	got := downsampleMip(src, image.Pt(1, 1))
	if got == nil {
		t.Fatal("downsampleMip returned nil")
	}
	if got.Pix[0] != 5 || got.Pix[3] != 255 {
		t.Fatalf("odd 3x3 average = %v, want [5 _ _ 255]", got.Pix)
	}
}

func TestWallMountedDepthAllowanceOnlyCoversBackingWall(t *testing.T) {
	allowance := wallMountedDepthAllowanceWorld(64, 0.02)
	if got, want := allowance, 2.64; math.Abs(got-want) > 1e-9 {
		t.Fatalf("wall-mounted allowance = %.2f, want %.2f", got, want)
	}
	if standeeColumnOccluded(100, 100, allowance) {
		t.Fatal("the backing wall must not occlude its own mounted standee")
	}
	if !standeeColumnOccluded(100, 95, allowance) {
		t.Fatal("a foreground wall 5px closer must occlude the mounted standee")
	}
}

func TestWallMountedBackingWallMatchIsPlaneSpecific(t *testing.T) {
	// A north-south backing wall at X=100: its standee runs along Y, so the
	// wall-stick yaw is pi/2. The centre ray sees that plane at depth 100.
	occlusion := standeeWallOcclusion{
		backingX:       100,
		backingY:       0,
		backingYaw:     math.Pi / 2,
		hasBackingWall: true,
	}
	if !occlusion.matchesBackingWall(0, 0, 1, 0, 100) {
		t.Fatal("the exact backing wall plane must be recognized")
	}
	if occlusion.matchesBackingWall(0, 0, 1, 0, 95) {
		t.Fatal("a different foreground wall plane must not inherit backing-wall immunity")
	}
}

// clampYawFromEdgeOn keeps monster tokens readable: the slab plane never
// comes closer than minRad to the camera's sight line (slabs have period pi).
func TestClampYawFromEdgeOn(t *testing.T) {
	const min = 0.35
	// Dead-on edge view (yaw aligned with the sight line) -> pushed out to +min.
	if got := clampYawFromEdgeOn(1.0, 1.0, min); math.Abs(got-(1.0+min)) > 1e-9 {
		t.Errorf("edge-on: got %.4f want %.4f", got, 1.0+min)
	}
	// Slightly negative side -> pushed to -min, preserving the side.
	if got := clampYawFromEdgeOn(1.0-0.1, 1.0, min); math.Abs(got-(1.0-min)) > 1e-9 {
		t.Errorf("neg side: got %.4f want %.4f", got, 1.0-min)
	}
	// Outside the dead zone -> untouched.
	if got := clampYawFromEdgeOn(2.0, 1.0, min); got != 2.0 {
		t.Errorf("outside: got %.4f want 2.0", got)
	}
	// Slab period pi: a yaw offset by ~pi from the sight line is still edge-on.
	if got := clampYawFromEdgeOn(1.0+math.Pi+0.05, 1.0, min); math.Abs(math.Mod(got-1.0, math.Pi)-min) > 1e-9 {
		t.Errorf("period: got %.4f, deviation should clamp to %.2f", got, min)
	}
}

// approachAngle drives the smooth token swivel: shortest arc, capped step.
func TestApproachAngle(t *testing.T) {
	// Within the step cap -> lands on the target exactly.
	if got := approachAngle(0, 0.1, 0.5); math.Abs(got-0.1) > 1e-9 {
		t.Errorf("small diff: got %.4f want 0.1", got)
	}
	// Beyond the cap -> advances by exactly the cap.
	if got := approachAngle(0, 2.0, 0.5); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("capped: got %.4f want 0.5", got)
	}
	// Shortest arc across the +/-pi seam: from 3.0 to -3.0 is +0.28 rad forward,
	// not -6.0 backward.
	got := approachAngle(3.0, -3.0, 10)
	want := 3.0 + (2*math.Pi - 6.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("seam: got %.4f want %.4f", got, want)
	}
	// And it must move in the negative direction when that's shorter.
	if got := approachAngle(-3.0, 3.0, 0.1); math.Abs(got-(-3.1)) > 1e-9 {
		t.Errorf("negative arc: got %.4f want -3.1", got)
	}
}

// Pressed up against a cross, its center (and at some angles a corner) is
// BEHIND the camera plane while the arms are still on screen. The arms must
// survive that as clamped spans - a failed projection here used to abort the
// split and drop the draw to the two-whole-slabs fallback, which paints the
// back arm over the front one.
func TestCrossedArmsSurvivePointBlankCamera(t *testing.T) {
	cfg := loadTestConfig(t)
	game := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	game.camera.FOV = squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	game.camera.ViewDist = cfg.GetViewDistance()
	game.renderHelper = NewRenderingHelper(game)
	r := &Renderer{game: game}
	ts := float64(cfg.GetTileSize())

	treeX, treeY := 10.5*ts, 10.5*ts
	footprint := 2.0 * ts // the standard tree: its footprint extends a full tile from center

	// Camera 0.6 tiles from the center (the collision minimum), INSIDE the
	// footprint, swept through a full turn: every pose must yield usable arms.
	game.camera.X, game.camera.Y = treeX, treeY+0.6*ts
	for deg := 0; deg < 360; deg += 15 {
		game.camera.Angle = float64(deg) * math.Pi / 180
		arms, ok := r.crossedStandeeArms(treeX, treeY, math.Pi/4, 3*math.Pi/4, footprint)
		if !ok {
			t.Fatalf("angle %d: point-blank arms degenerated to the whole-slab fallback", deg)
		}
		visible := 0
		for _, arm := range arms {
			if arm.lo <= arm.hi {
				visible++
			}
		}
		if visible == 0 {
			t.Fatalf("angle %d: no visible arm spans while standing beside the cross", deg)
		}
	}
}

// The test environment forbids GPU readbacks (ebiten panics with "ReadPixels
// cannot be called before the game starts"), which doubles as the assertion
// here: every build below only completes when it consumes the stashed CPU
// pixels, and a regression to the ReadPixels path fails the test by panicking.
// Crop correctness is asserted on the CPU pixels retainedSpriteCPU returns.
func TestRetainedSpriteCPUAvoidsReadbackForStandeeBuilds(t *testing.T) {
	opaqueRGBA := func(rect image.Rectangle) *image.RGBA {
		cpu := image.NewRGBA(rect)
		for i := 0; i < len(cpu.Pix); i += 4 {
			cpu.Pix[i], cpu.Pix[i+1], cpu.Pix[i+2], cpu.Pix[i+3] = 90, 140, 70, 255
		}
		return cpu
	}
	type cpuOrigin struct {
		name    string
		install func(*Renderer, map[*ebiten.Image]*image.RGBA)
	}
	origins := []cpuOrigin{
		{
			name: "current world-pass lazy load",
			install: func(r *Renderer, pixels map[*ebiten.Image]*image.RGBA) {
				r.lazySpriteCPUPixels = pixels
			},
		},
		{
			name: "active streaming prewarm",
			install: func(r *Renderer, pixels map[*ebiten.Image]*image.RGBA) {
				r.mapRenderResourcePrewarmActive = &mapRenderPrewarmTask{cpuImages: pixels}
			},
		},
	}

	for _, origin := range origins {
		t.Run("direct image/"+origin.name, func(t *testing.T) {
			r := &Renderer{game: &MMGame{config: &config.Config{}}}
			src := ebiten.NewImage(6, 6)
			origin.install(r, map[*ebiten.Image]*image.RGBA{src: opaqueRGBA(image.Rect(0, 0, 6, 6))})
			key := makeStandeeCoreKey("tile:retained_cpu:"+origin.name, src, false)
			core := r.standeeCoreSilhouette(key, src)
			if core == nil {
				t.Fatal("silhouette was not built from retained CPU pixels")
			}
			if r.standeeMipCache[standeeMipKey{frame: key, layer: standeeMipSticker}] == nil {
				t.Fatal("CPU-built silhouette did not commit the sticker mip chain")
			}
		})

		t.Run("static sheet frame/"+origin.name, func(t *testing.T) {
			r := &Renderer{game: &MMGame{config: &config.Config{}}}
			sheet := ebiten.NewImage(16, 4)
			frames := r.animationFrames(sheet)
			if len(frames) != SpriteSheetFrameCount {
				t.Fatalf("sheet split into %d frames", len(frames))
			}
			cpuSheet := image.NewRGBA(image.Rect(0, 0, 16, 4))
			draw.Draw(cpuSheet, image.Rect(8, 0, 12, 4), opaqueRGBA(image.Rect(0, 0, 4, 4)), image.Point{}, draw.Src)
			origin.install(r, map[*ebiten.Image]*image.RGBA{sheet: cpuSheet})

			painted := r.retainedSpriteCPU(frames[2])
			if painted == nil || painted.Bounds() != frames[2].Bounds() {
				t.Fatalf("frame 2 crop = %v, want bounds %v", painted, frames[2].Bounds())
			}
			if _, _, _, a := painted.At(9, 1).RGBA(); a == 0 {
				t.Fatal("painted frame's crop lost its pixels")
			}
			empty := r.retainedSpriteCPU(frames[0])
			if empty == nil {
				t.Fatal("frame 0 crop was not resolved")
			}
			if _, _, _, a := empty.At(1, 1).RGBA(); a != 0 {
				t.Fatal("empty frame's crop leaked another frame's art")
			}
			if core := r.standeeCoreSilhouette(makeStandeeCoreKey("tile:sheet_painted:"+origin.name, frames[2], false), frames[2]); core == nil {
				t.Fatal("sheet frame silhouette was not built from retained CPU pixels")
			}
		})

		t.Run("bounded copy/"+origin.name, func(t *testing.T) {
			r := &Renderer{game: &MMGame{config: &config.Config{}}}
			src := ebiten.NewImage(2048, 1024)
			origin.install(r, map[*ebiten.Image]*image.RGBA{src: opaqueRGBA(image.Rect(0, 0, 2048, 1024))})
			key := makeStandeeCoreKey("tile:retained_big:"+origin.name, src, false)
			bounded := r.boundedStandeeRenderSource(key, src)
			if bounded == nil || bounded == src {
				t.Fatalf("oversized source was not reduced: %v", bounded)
			}
			if got := bounded.Bounds(); got.Dx() != 1024 || got.Dy() != 512 {
				t.Fatalf("bounded size = %v, want 1024x512", got)
			}
			handoff := r.lazySpriteCPUPixels[bounded]
			if handoff == nil {
				t.Fatal("reduced pixels were not handed forward for the silhouette build")
			}
			if _, _, _, a := handoff.At(0, 0).RGBA(); a == 0 {
				t.Fatal("handed-forward pixels are empty")
			}
			if core := r.standeeCoreSilhouette(key, bounded); core == nil {
				t.Fatal("bounded silhouette was not built from handed-forward CPU pixels")
			}
		})
	}

	unavailable := []struct {
		name  string
		setup func(*Renderer)
	}{
		{name: "no retained pixels"},
		{name: "cancelled prewarm", setup: func(r *Renderer) {
			r.mapRenderResourcePrewarmActive = &mapRenderPrewarmTask{
				state:     mapRenderTaskCancelled,
				cpuImages: make(map[*ebiten.Image]*image.RGBA),
			}
		}},
	}
	for _, tt := range unavailable {
		t.Run("readback fallback/"+tt.name, func(t *testing.T) {
			r := &Renderer{game: &MMGame{config: &config.Config{}}}
			src := ebiten.NewImage(6, 6)
			if tt.setup != nil {
				tt.setup(r)
			}
			if r.retainedSpriteCPU(src) != nil {
				t.Fatal("retainedSpriteCPU fabricated unavailable pixels")
			}
		})
	}
}

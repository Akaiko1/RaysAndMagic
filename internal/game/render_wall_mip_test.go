package game

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The whole point of picking levels ourselves: whatever the sample rate on an
// axis is, the level sampled on that axis must leave at most ~2 source texels
// per screen pixel, which is what bilinear filtering actually handles. Before
// this, Ebitengine's per-call minimum pinned distant city walls to level 0 at a
// measured 12.4 and 24.7 texels per pixel horizontally.
func TestWallMipLevelLeavesAtMostTwoTexelsPerPixelHorizontally(t *testing.T) {
	grid := wallRipmapSizes(256, 256)
	maxLx := len(grid[0]) - 1
	for _, footprint := range []float64{1, 1.7, 2, 3.2, 5.4, 10, 12.4, 15.9, 16, 24.7, 29.2, 64, 255} {
		level, blend := mipLevelBlend(float32(footprint), maxLx)
		residual := footprint / math.Pow(2, float64(level))
		if residual > 2.0001 {
			t.Errorf("footprint %.1f -> level %d leaves %.2f texels/pixel, want <= 2", footprint, level, residual)
		}
		if blend > 0 && level+1 > maxLx {
			t.Errorf("footprint %.1f asks to blend past the last level %d", footprint, maxLx)
		}
	}
}

// The vertical axis has the same duty: a 256px tile drawn 35px tall (the
// head-on wall at ~20 tiles) samples ~7.3 texels per pixel vertically. The
// X-only chain shipped once and left exactly that unfiltered - the brick rows
// aliased into light/dark horizontal bands shimmering through the approach.
func TestWallMipLevelLeavesAtMostTwoTexelsPerPixelVertically(t *testing.T) {
	grid := wallRipmapSizes(256, 256)
	maxLy := len(grid) - 1
	for _, wallHeight := range []float64{256, 128, 90, 35, 16, 8, 4, 2} {
		footprint := wallSliceVerticalFootprint(256, wallHeight)
		level, blend := mipLevelBlend(float32(footprint), maxLy)
		residual := footprint / math.Pow(2, float64(level))
		if residual > 2.0001 {
			t.Errorf("wall height %.0fpx (footprint %.1f) -> level %d leaves %.2f texels/pixel, want <= 2",
				wallHeight, footprint, level, residual)
		}
		if blend > 0 && level+1 > maxLy {
			t.Errorf("wall height %.0fpx asks to blend past the last level %d", wallHeight, maxLy)
		}
	}
	if got := wallSliceVerticalFootprint(256, 0); got != 1 {
		t.Errorf("degenerate wall height footprint = %.2f, want 1", got)
	}
}

// A footprint of one texel per pixel or less must stay on level 0: close walls
// keep their authored pixel art, unblurred.
func TestWallMipLevelZeroWhenNotMinified(t *testing.T) {
	for _, footprint := range []float64{0.25, 0.9, 1} {
		if level, blend := mipLevelBlend(float32(footprint), 6); level != 0 || blend != 0 {
			t.Errorf("footprint %.2f -> level %d blend %.2f, want level 0 with no blend", footprint, level, blend)
		}
	}
}

func TestWallSliceFootprintIsTexelsPerScreenPixel(t *testing.T) {
	// A one-pixel column spanning a tenth of a 256px tile covers 25.6 texels.
	if got := wallSliceFootprint(0.2, 0.3, 256, 1); math.Abs(got-25.6) > 1e-9 {
		t.Errorf("footprint = %.4f, want 25.6", got)
	}
	// Same span across four screen pixels is four times cheaper.
	if got := wallSliceFootprint(0.2, 0.3, 256, 4); math.Abs(got-6.4) > 1e-9 {
		t.Errorf("footprint = %.4f, want 6.4", got)
	}
	// Direction of the interval must not matter (mirrored wall faces).
	if got := wallSliceFootprint(0.3, 0.2, 256, 1); math.Abs(got-25.6) > 1e-9 {
		t.Errorf("mirrored footprint = %.4f, want 25.6", got)
	}
	// A degenerate width must not divide by zero.
	if got := wallSliceFootprint(0.2, 0.3, 256, 0); math.Abs(got-25.6) > 1e-9 {
		t.Errorf("zero-width footprint = %.4f, want 25.6", got)
	}
}

// Walls need a RIPMAP, not a pyramid: the axes minify independently (grazing
// angle compresses X only; a far head-on wall compresses both). Each grid row
// halves height once more; within a row width halves down to a single texel.
func TestWallRipmapSizesHalveAxesIndependently(t *testing.T) {
	grid := wallRipmapSizes(256, 256)
	if len(grid) != wallMaxVerticalMipLevels+1 {
		t.Fatalf("rows = %d, want %d", len(grid), wallMaxVerticalMipLevels+1)
	}
	for iy, row := range grid {
		wantH := 256 >> iy
		if len(row) != 9 {
			t.Fatalf("row %d has %d levels, want 9 (256 down to width 1)", iy, len(row))
		}
		for ix, size := range row {
			want := image.Pt(256>>ix, wantH)
			if size != want {
				t.Fatalf("grid[%d][%d] = %v, want %v", iy, ix, size, want)
			}
		}
	}
	// The X-only failure mode: the head-on-distance combination (little X
	// minification, deep Y minification) must exist in the grid.
	if grid[3][0] != image.Pt(256, 32) {
		t.Errorf("grid[3][0] = %v, want (256,32) - the head-on far wall level", grid[3][0])
	}
	// And the grazing combination (deep X, no Y) likewise.
	if grid[0][5] != image.Pt(8, 256) {
		t.Errorf("grid[0][5] = %v, want (8,256) - the grazing-angle level", grid[0][5])
	}
	// Standee tokens keep the isotropic pyramid - the two must not converge.
	if uniform := mipSizesUniform(256, 256); uniform[1] == grid[0][1] {
		t.Errorf("uniform pyramid level 1 %v equals ripmap row 0 - axes no longer independent", uniform[1])
	}
	// Degenerate textures terminate instead of looping.
	if got := wallRipmapSizes(1, 1); len(got) != 1 || len(got[0]) != 1 {
		t.Errorf("wallRipmapSizes(1,1) = %v", got)
	}
}

func TestWallMipLevelRepeatsHorizontallyAndPadsVerticalEdges(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	copy(src.Pix, []byte{
		1, 2, 3, 255, 4, 5, 6, 255,
		7, 8, 9, 255, 10, 11, 12, 255,
	})
	got := tileWallMipLevel(src, 3)
	if got.Bounds() != image.Rect(0, 0, 6, 4) {
		t.Fatalf("bounds = %v, want 6x4 with one gutter row on each edge", got.Bounds())
	}
	sourceRows := []int{0, 0, 1, 1}
	for row, sourceRow := range sourceRows {
		for copyIndex := 0; copyIndex < 3; copyIndex++ {
			for x := 0; x < 2; x++ {
				srcOff := sourceRow*src.Stride + x*4
				dstOff := row*got.Stride + (copyIndex*2+x)*4
				for c := 0; c < 4; c++ {
					if got.Pix[dstOff+c] != src.Pix[srcOff+c] {
						t.Fatalf("copy %d row %d (source row %d) texel %d channel %d = %d, want %d",
							copyIndex, row, sourceRow, x, c, got.Pix[dstOff+c], src.Pix[srcOff+c])
					}
				}
			}
		}
	}
}

func TestStreamingWallRipmapBuilderUsesGutteredLevelPixels(t *testing.T) {
	source := ebiten.NewImage(4, 4)
	prepared := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			off := prepared.PixOffset(x, y)
			prepared.Pix[off], prepared.Pix[off+3] = byte(20+y), 255
		}
	}
	r := &Renderer{wallRipmaps: make(map[*ebiten.Image]*wallRipmap)}
	builder := newMapRenderWallRipmapBuilder(r, source, prepared)
	if _, done := builder.advance(16); done {
		t.Fatal("streaming builder unexpectedly finished its first level")
	}
	if builder.pendingCPU == nil {
		t.Fatal("streaming builder did not retain its partially uploaded CPU level")
	}
	if bounds := builder.pendingCPU.Bounds(); bounds != image.Rect(0, 0, 12, 6) {
		t.Fatalf("pending level bounds = %v, want 12x6 including vertical gutters", bounds)
	}
	for _, x := range []int{0, 5, 11} {
		top := builder.pendingCPU.PixOffset(x, 0)
		topContent := builder.pendingCPU.PixOffset(x, 1)
		bottomContent := builder.pendingCPU.PixOffset(x, 4)
		bottom := builder.pendingCPU.PixOffset(x, 5)
		if builder.pendingCPU.Pix[top] != builder.pendingCPU.Pix[topContent] {
			t.Fatalf("top gutter at x=%d differs from first content row", x)
		}
		if builder.pendingCPU.Pix[bottom] != builder.pendingCPU.Pix[bottomContent] {
			t.Fatalf("bottom gutter at x=%d differs from last content row", x)
		}
	}
	builder.cancel()
}

func TestWallRipmapHasOneAuthoritativeBuilder(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *Renderer, *ebiten.Image, *image.RGBA)
	}{
		{
			name: "lazy lookup observes in-flight streaming cache",
			run: func(t *testing.T, r *Renderer, source *ebiten.Image, prepared *image.RGBA) {
				builder := newMapRenderWallRipmapBuilder(r, source, prepared)
				inFlight := builder.ripmap
				reserved := r.wallRipmapBytes
				if inFlight == nil || !inFlight.building || reserved <= 0 {
					t.Fatalf("streaming builder did not reserve an in-flight cache: ripmap=%v building=%v bytes=%d",
						inFlight != nil, inFlight != nil && inFlight.building, reserved)
				}
				if got := r.wallRipmapForCPU(source, prepared); got != inFlight {
					t.Fatal("lazy lookup created a second ripmap while streaming was active")
				}
				if r.wallRipmapBytes != reserved {
					t.Fatalf("lazy lookup changed reserved bytes: got %d want %d", r.wallRipmapBytes, reserved)
				}
				if _, _, _, ok := r.wallMipSource(source); ok {
					t.Fatal("render path exposed a newly registered in-flight ripmap")
				}
				for len(inFlight.levels[0]) == 0 {
					if _, done := builder.advance(32); done {
						t.Fatal("streaming builder completed before exposing a partial level")
					}
				}
				if _, _, _, ok := r.wallMipSource(source); ok {
					t.Fatal("render path exposed a partially populated in-flight ripmap")
				}
				for advances := 0; ; advances++ {
					_, done := builder.advance(1 << 20)
					if done {
						break
					}
					if advances > 1000 {
						t.Fatal("streaming builder did not converge")
					}
				}
				if got := r.wallRipmaps[source]; got != inFlight || got.building {
					t.Fatalf("completed builder replaced its cache identity: same=%v building=%v",
						got == inFlight, got != nil && got.building)
				}
				if r.wallRipmapBytes != reserved {
					t.Fatalf("completed builder counted bytes %d, want one reservation %d", r.wallRipmapBytes, reserved)
				}
				if _, _, _, ok := r.wallMipSource(source); !ok {
					t.Fatal("render path did not expose the completed ripmap")
				}
			},
		},
		{
			name: "cancel releases reservation and permits synchronous fallback",
			run: func(t *testing.T, r *Renderer, source *ebiten.Image, prepared *image.RGBA) {
				builder := newMapRenderWallRipmapBuilder(r, source, prepared)
				builder.cancel()
				if r.wallRipmaps[source] != nil || r.wallRipmapBytes != 0 {
					t.Fatalf("cancel retained cache=%v bytes=%d", r.wallRipmaps[source] != nil, r.wallRipmapBytes)
				}
				if got := r.wallRipmapForCPU(source, prepared); got == nil || len(got.levels) == 0 || got.building {
					t.Fatal("synchronous fallback did not rebuild after cancellation")
				}
			},
		},
		{
			name: "completed cache wins repeated prewarm",
			run: func(t *testing.T, r *Renderer, source *ebiten.Image, prepared *image.RGBA) {
				ready := r.wallRipmapForCPU(source, prepared)
				bytes := r.wallRipmapBytes
				builder := newMapRenderWallRipmapBuilder(r, source, prepared)
				if !builder.done || builder.ripmap != ready {
					t.Fatal("repeated prewarm did not adopt the completed cache")
				}
				if r.wallRipmapBytes != bytes {
					t.Fatalf("repeated prewarm changed bytes: got %d want %d", r.wallRipmapBytes, bytes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := ebiten.NewImage(8, 8)
			prepared := image.NewRGBA(image.Rect(0, 0, 8, 8))
			r := &Renderer{wallRipmaps: make(map[*ebiten.Image]*wallRipmap)}
			t.Cleanup(func() {
				r.clearWallRipmaps()
				source.Deallocate()
			})
			tt.run(t, r, source, prepared)
		})
	}
}

func TestSynchronousWallRipmapBuilderUsesGutteredLevelBounds(t *testing.T) {
	source := ebiten.NewImage(4, 4)
	r := &Renderer{}
	rm := r.wallRipmapForCPU(source, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	if rm == nil {
		t.Fatal("synchronous builder did not create a ripmap")
	}
	tests := []struct {
		name string
		iy   int
		ix   int
		want image.Rectangle
	}{
		{name: "full resolution", iy: 0, ix: 0, want: image.Rect(0, 0, 12, 6)},
		{name: "both axes reduced", iy: 1, ix: 1, want: image.Rect(0, 0, 6, 4)},
		{name: "one texel terminal", iy: 2, ix: 2, want: image.Rect(0, 0, 3, 3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rm.levels[tt.iy][tt.ix].Bounds(); got != tt.want {
				t.Fatalf("level[%d][%d] bounds = %v, want %v", tt.iy, tt.ix, got, tt.want)
			}
		})
	}
	r.clearWallRipmaps()
}

func TestResetMapRenderResourceResidencyClearsWallRipmaps(t *testing.T) {
	source := ebiten.NewImage(8, 8)
	level := ebiten.NewImage(24, 8)
	r := &Renderer{
		wallRipmaps: map[*ebiten.Image]*wallRipmap{
			source: {owned: []*ebiten.Image{level}},
		},
	}

	r.resetMapRenderResourceResidency()
	if r.wallRipmaps != nil {
		t.Fatalf("wall ripmap cache survived residency reset: %d entries", len(r.wallRipmaps))
	}
}

func TestWallRipmapBudgetFallsBackBeforeAllocation(t *testing.T) {
	smallRipmapBytes := wallRipmapByteSize(8, 8)
	tests := []struct {
		name         string
		residentByte int64
	}{
		{name: "one byte over budget", residentByte: wallRipmapBudgetBytes - smallRipmapBytes + 1},
		{name: "budget exhausted", residentByte: wallRipmapBudgetBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := ebiten.NewImage(8, 8)
			defer source.Deallocate()
			r := &Renderer{wallRipmapBytes: tt.residentByte}
			rm := r.wallRipmapFor(source)
			if rm != nil && len(rm.levels) > 0 {
				t.Fatal("over-budget source built a usable ripmap")
			}
			if cached := r.wallRipmaps[source]; cached == nil {
				t.Fatal("fallback decision was not cached")
			}
			if _, _, _, ok := r.wallMipSource(source); ok {
				t.Fatal("budget fallback exposed an unusable ripmap")
			}
			r.clearWallRipmaps()
		})
	}
}

func TestWallRipmapPerTextureBudgetRejectsOversizedSource(t *testing.T) {
	if bytes := wallRipmapByteSize(1024, 1024); bytes <= wallRipmapPerTextureBudgetBytes {
		t.Fatalf("test source is not oversized: ripmap bytes = %d", bytes)
	}
	source := ebiten.NewImage(1024, 1024)
	defer source.Deallocate()
	r := &Renderer{}
	if rm := r.wallRipmapFor(source); rm != nil && len(rm.levels) > 0 {
		t.Fatal("oversized wall built a ripmap")
	}
	if cached := r.wallRipmaps[source]; cached == nil {
		t.Fatal("oversized wall fallback decision was not cached")
	}
}

// The crossover quads are drawn over the base quad with source-over blending.
// Their vertex color is premultiplied, and the draw options MUST declare that:
// DrawTriangles' default is straight alpha, under which Ebitengine multiplies
// RGB by A a second time. Shipped once as a dark band crawling along every
// wall at the level-transition zone.
func TestWallMipVerticesPremultiplyCrossoverAlpha(t *testing.T) {
	opaque := appendWallMipSliceVertices(nil, 256, 256, 256, 256, 100, 1, 50, 200, 0.25, 0.30, 0.8, 1)
	if len(opaque) != 4 {
		t.Fatalf("vertices = %d, want 4", len(opaque))
	}
	for i, v := range opaque {
		if math.Abs(float64(v.ColorR)-0.8) > 1e-6 || v.ColorA != 1 {
			t.Fatalf("opaque vertex %d color = (%.3f, a=%.3f), want (0.8, a=1)", i, v.ColorR, v.ColorA)
		}
	}
	faded := appendWallMipSliceVertices(nil, 256, 256, 256, 256, 100, 1, 50, 200, 0.25, 0.30, 0.8, 0.5)
	for i, v := range faded {
		if math.Abs(float64(v.ColorR)-0.4) > 1e-6 || math.Abs(float64(v.ColorA)-0.5) > 1e-6 {
			t.Fatalf("faded vertex %d color = (%.3f, a=%.3f), want (0.4, a=0.5)", i, v.ColorR, v.ColorA)
		}
	}
}

// Premultiplied vertices are only correct under the premultiplied color-scale
// mode, and manual level selection is only in force with the engine's own
// mipmaps off. Pin the whole option set.
func TestWallMipTriangleOptions(t *testing.T) {
	opts := wallMipTriangleOptions()
	if opts.ColorScaleMode != ebiten.ColorScaleModePremultipliedAlpha {
		t.Error("vertex colors are premultiplied; the options must say so or alpha is applied twice")
	}
	if !opts.DisableMipmaps {
		t.Error("engine mipmaps must be off - the levels are chosen per slice")
	}
	if opts.Filter != ebiten.FilterLinear {
		t.Error("wall mip sampling must stay bilinear")
	}
	if opts.Blend != ebiten.BlendSourceOver {
		t.Error("crossover quads rely on source-over blending")
	}
}

// The brick pattern must not move when a level changes ON EITHER AXIS: the
// sampled position in TILE units has to be identical at every level, and level
// (0,0) has to equal the legacy full-res formula after removing the gutter
// offset (close walls keep their look). Shipped once as masonry jumping
// sideways on every level switch:
// the half-texel alignment offset was applied in level texels, which is 64x
// bigger at level 6 than at level 0.
func TestWallMipPatternPositionIsLevelInvariant(t *testing.T) {
	const fullW, fullH, leftU, rightU = 256.0, 256.0, 0.25, 0.50
	legacy := appendWallMipSliceVertices(nil, fullW, fullW, fullH, fullH, 0, 1, 0, 100, leftU, rightU, 1, 1)
	if math.Abs(float64(legacy[0].SrcX)-(fullW+leftU*fullW+0.5)) > 1e-4 {
		t.Errorf("level 0 SrcX = %.4f, want the legacy %.4f", legacy[0].SrcX, fullW+leftU*fullW+0.5)
	}
	gutter := float64(wallMipVerticalGutterRows)
	if math.Abs(float64(legacy[0].SrcY)-(gutter+0.5)) > 1e-4 ||
		math.Abs(float64(legacy[2].SrcY)-(gutter+fullH-0.5)) > 1e-4 {
		t.Errorf("level 0 SrcY = %.4f..%.4f, want guttered %.1f..%.1f",
			legacy[0].SrcY, legacy[2].SrcY, gutter+0.5, gutter+fullH-0.5)
	}

	patternU := func(srcX float32, levelW float64) float64 {
		return float64(srcX)/levelW - 1 // strip the middle-copy offset, back to tile units
	}
	for _, levelW := range []float64{128, 64, 32, 4, 1} {
		verts := appendWallMipSliceVertices(nil, levelW, fullW, fullH, fullH, 0, 1, 0, 100, leftU, rightU, 1, 1)
		for i, want := range []float64{leftU, rightU, leftU, rightU} {
			got := patternU(verts[i].SrcX, levelW)
			if math.Abs(got-(want+0.5/fullW)) > 1e-6 {
				t.Errorf("level width %.0f vertex %d samples tile U %.6f, want %.6f",
					levelW, i, got, want+0.5/fullW)
			}
		}
	}
	for _, levelH := range []float64{128, 32, 4} {
		verts := appendWallMipSliceVertices(nil, fullW, fullW, levelH, fullH, 0, 1, 0, 100, leftU, rightU, 1, 1)
		topSource := float64(verts[0].SrcY)
		bottomSource := float64(verts[2].SrcY)
		top := (topSource - gutter) / levelH
		bottom := (bottomSource - gutter) / levelH
		if math.Abs(top-0.5/fullH) > 1e-6 || math.Abs(bottom-(1-0.5/fullH)) > 1e-6 {
			t.Errorf("level height %.0f samples tile V %.6f..%.6f, want %.6f..%.6f",
				levelH, top, bottom, 0.5/fullH, 1-0.5/fullH)
		}
		if topSource < 0.5 || topSource > gutter+0.5 ||
			bottomSource < gutter+levelH-0.5 || bottomSource > 2*gutter+levelH-0.5 {
			t.Errorf("level height %.0f edge samples %.4f..%.4f escape duplicated gutter texels",
				levelH, topSource, bottomSource)
		}
	}

	// Sampling happens in the middle of the repeated copies so a wrapping
	// interval still finds real neighbouring pixels.
	if legacy[0].SrcX <= 256 || legacy[1].SrcX >= 512 {
		t.Errorf("source X %.1f..%.1f is outside the middle tile copy", legacy[0].SrcX, legacy[1].SrcX)
	}
}

// wallMipSelect resolves both axes independently - the grazing case wants deep
// X and Y=0, the head-on far case wants X near 0 and deep Y.
func TestWallMipSelectResolvesAxesIndependently(t *testing.T) {
	rm := &wallRipmap{levels: make([][]*ebiten.Image, wallMaxVerticalMipLevels+1)}
	for iy := range rm.levels {
		rm.levels[iy] = make([]*ebiten.Image, 9)
	}
	// Grazing: 20 texels/pixel horizontally, full-height wall.
	sel := wallMipSelect(rm, 0.2, 0.2+20.0/256.0, 256, 1, 256, 512)
	if sel.lx < 3 || sel.ly != 0 {
		t.Errorf("grazing selection = (lx=%d, ly=%d), want deep X and ly=0", sel.lx, sel.ly)
	}
	// Head-on at ~20 tiles: one tile spans ~35px both ways, so BOTH axes must
	// land on the same deep level (footprint 7.31 -> lod 2.87, past the blend
	// window -> snaps to 3, same as the standee contract).
	sel = wallMipSelect(rm, 0.2, 0.2+(256.0/35.0)/256.0, 256, 1, 256, 35)
	if sel.lx != 3 || sel.ly != 3 {
		t.Errorf("head-on selection = (lx=%d, ly=%d), want (3,3)", sel.lx, sel.ly)
	}
	// Crossover targets never leave the grid.
	sel = wallMipSelect(rm, 0, 1, 256, 1, 256, 1)
	if sel.lx+1 > 8 && sel.ax > 0 {
		t.Error("X crossover escapes the grid")
	}
	if sel.ly+1 > wallMaxVerticalMipLevels && sel.ay > 0 {
		t.Error("Y crossover escapes the grid")
	}
}

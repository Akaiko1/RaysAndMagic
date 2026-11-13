package game

import (
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func testLevels(sizes ...image.Point) []*image.RGBA {
	levels := make([]*image.RGBA, len(sizes))
	for k, size := range sizes {
		levels[k] = image.NewRGBA(image.Rectangle{Max: size})
	}
	return levels
}

// The shader derives every level rectangle from the block origin and the two
// header texels, so the layout and the header must agree with that derivation.
func TestStandeeSheetLayout(t *testing.T) {
	for _, full := range []image.Point{{512, 512}, {512, 1024}, {37, 91}, {4, 2}} {
		sizes := mipSizesUniform(full.X, full.Y)
		blocks, size, ok := standeeSheetLayout(sizes)
		if !ok {
			t.Fatalf("%v: levels were not packed", full)
		}
		var used []image.Rectangle
		for k := 1; k < len(sizes); k++ {
			level := sizes[k]
			header := image.Rect(blocks[k].X, blocks[k].Y, blocks[k].X+2, blocks[k].Y+1)
			stickerRect := image.Rectangle{Min: blocks[k].Add(image.Pt(0, 1))}
			stickerRect.Max = stickerRect.Min.Add(level)
			coreRect := stickerRect.Add(image.Pt(level.X, 0))
			for _, rect := range []image.Rectangle{header, stickerRect, coreRect} {
				if !rect.In(image.Rectangle{Max: size}) {
					t.Fatalf("%v level %d: %v outside the %v sheet", full, k, rect, size)
				}
				for _, other := range used {
					if rect.Overlaps(other) {
						t.Fatalf("%v level %d: %v overlaps %v", full, k, rect, other)
					}
				}
				used = append(used, rect)
			}
			// The next level's block starts right below this sticker.
			if k+1 < len(sizes) && blocks[k+1] != image.Pt(stickerRect.Min.X, stickerRect.Max.Y) {
				t.Fatalf("%v level %d: next block at %v, shader expects %v", full, k, blocks[k+1], image.Pt(stickerRect.Min.X, stickerRect.Max.Y))
			}
		}
		for _, side := range append(sizes, image.Pt(standeeSheetMaxSide, 1), image.Pt(1, standeeSheetMaxSide)) {
			b := standeeSheetHeader(side)
			// standee_sheet.kage sheetSize, on bytes.
			got := image.Pt(int(b[0])*16+int(b[1])/16, int(b[1])%16*256+int(b[2]))
			if got != side || b[3] != 0xff {
				t.Fatalf("header of %v decodes to %v (alpha %d)", side, got, b[3])
			}
		}
		// The sheet's views are the rectangles the shader derives.
		sheet := newStandeeSheet(testLevels(sizes...), testLevels(sizes...))
		if sheet == nil {
			t.Fatalf("%v: no sheet for matching chains", full)
		}
		for k := 1; k < len(sizes); k++ {
			origin := blocks[k].Add(image.Pt(0, 1))
			if got := sheet.sticker[k].Bounds(); got.Min != origin || got.Size() != sizes[k] {
				t.Fatalf("%v level %d: sticker view %v", full, k, got)
			}
			if got := sheet.core[k].Bounds(); got.Min != origin.Add(image.Pt(sizes[k].X, 0)) || got.Size() != sizes[k] {
				t.Fatalf("%v level %d: core view %v", full, k, got)
			}
		}
		sheet.image.Deallocate()
	}

	square := mipSizesUniform(64, 64)
	rejected := map[string][2][]image.Point{
		"one level":             {square[:1], square[:1]},
		"fewer core levels":     {square, square[:3]},
		"core of another size":  {square, mipSizesUniform(64, 32)},
		"level that is no half": {{square[0], square[1], {20, 16}}, {square[0], square[1], {20, 16}}},
		"side beyond 12 bits":   {{{standeeSheetMaxSide + 1, 2}, {2048, 1}}, {{standeeSheetMaxSide + 1, 2}, {2048, 1}}},
		"sheet beyond a page":   {mipSizesUniform(4096, 16), mipSizesUniform(4096, 16)},
	}
	for name, chains := range rejected {
		if sheet := newStandeeSheet(testLevels(chains[0]...), testLevels(chains[1]...)); sheet != nil {
			t.Errorf("%s: sheet built for chains the shader cannot address", name)
		}
	}
}

// A sheet is a derived allocation of its token: it leaves with the token and
// with the world, and a kept token keeps it.
func TestStandeeSheetLeavesWithItsToken(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range pixels.Pix {
		pixels.Pix[i] = 0xff
	}
	newToken := func(r *Renderer, name string) (standeeCoreKey, *standeeSheet) {
		sprite := ebiten.NewImage(32, 32)
		key := makeStandeeCoreKey(name, sprite, true)
		r.standeeCoreSilhouetteFromCPU(key, sprite, pixels)
		sheet := r.standeeSheets[key]
		sticker := r.standeeMipCache[standeeMipKey{frame: key, layer: standeeMipSticker}]
		core := r.standeeMipCache[standeeMipKey{frame: key, layer: standeeMipCore}]
		if !sheet.holds(sticker, core, 1, 2) || !sheet.holds(sticker, sticker, len(sticker.levels)-1, len(sticker.levels)-1) {
			t.Fatalf("%s: the token's reduced levels are not views of its sheet", name)
		}
		if sheet.holds(sticker, core, 0, 0) {
			t.Fatalf("%s: level 0 must stay outside the sheet", name)
		}
		return key, sheet
	}

	cfg := loadTestConfig(t)
	r := &Renderer{game: newTestGame(cfg, newTestWorld(cfg))}
	gone, goneSheet := newToken(r, "gone")
	kept, keptSheet := newToken(r, "kept")
	r.refreshRenderResourceRegistry()
	released := map[*ebiten.Image]int{}
	r.mapRenderRegistry.release = func(img *ebiten.Image) { released[img]++ }
	r.mapRenderRegistry.retain(renderResourceID{kind: renderResourceStandee, standee: kept}, renderResourceOwner{role: "test"})
	r.mapRenderRegistry.releaseResources(map[renderResourceID]struct{}{
		{kind: renderResourceStandee, standee: gone}: {},
		{kind: renderResourceStandee, standee: kept}: {},
	})
	if released[goneSheet.image] != 1 || r.standeeSheets[gone] != nil {
		t.Fatalf("evicted token: sheet released %d times, cached %v", released[goneSheet.image], r.standeeSheets[gone] != nil)
	}
	if released[keptSheet.image] != 0 || r.standeeSheets[kept] != keptSheet {
		t.Fatalf("retained token lost its sheet: released %d times", released[keptSheet.image])
	}

	r.resetMapRenderResourceResidency()
	if len(r.standeeSheets) != 0 {
		t.Fatalf("world switch kept %d sheets", len(r.standeeSheets))
	}
}

// Reduced levels draw from the sheet, level 0 and unpackable chains from the
// level images: both must put the same pixels on screen. A token without a
// sheet entry takes the level-image path the renderer itself falls back to.
func TestStandeeSheetMatchesLevelImages(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires the live GPU harness")
	}
	cfg := loadTestConfig(t)
	const w, h = 640, 480
	cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = w, h
	g := newTestGame(cfg, newTestWorld(cfg))
	g.camera.X, g.camera.Y, g.camera.Angle = 0, 0, 0
	g.camera.FOV, g.camera.ViewDist = squareProjectionFOV(w, h), 3200
	g.renderHelper = NewRenderingHelper(g)
	g.depthBuffer, g.wallTopBuffer = make([]float64, w), make([]float64, w)
	r := &Renderer{game: g}

	// Thin bright lines and holes expose a one-texel shift in either axis.
	pixels := image.NewRGBA(image.Rect(0, 0, 128, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 128; x++ {
			switch {
			case (x/13+y/19)%4 == 0:
			case x%7 == 0 || y%11 == 0:
				pixels.SetRGBA(x, y, color.RGBA{250, 240, 30, 255})
			default:
				pixels.SetRGBA(x, y, color.RGBA{uint8(40 + x), uint8(y), 70, 190})
			}
		}
	}
	source := ebiten.NewImage(256, 320)
	defer source.Deallocate()
	sprite := source.SubImage(image.Rect(32, 48, 160, 304)).(*ebiten.Image)
	sprite.WritePixels(pixels.Pix)
	key := makeStandeeCoreKey("sheet-parity", sprite, false)
	r.standeeCoreSilhouetteFromCPU(key, sprite, pixels)
	if r.standeeSheets[key] == nil {
		t.Fatal("fixture token has no sheet")
	}
	sheetImage, levelImage := ebiten.NewImage(w, h), ebiten.NewImage(w, h)
	defer sheetImage.Deallocate()
	defer levelImage.Deallocate()
	sheetPixels, levelPixels := make([]byte, 4*w*h), make([]byte, 4*w*h)

	cases := []struct {
		name             string
		depth, size, yaw float64
		tint             [2]float32
		mirror, single   bool
		walls, panini    bool
		narrow           bool // one-column clips: the column layout
	}{
		{name: "first reduced level", depth: 300, size: 100, yaw: 0.8, tint: [2]float32{1, 1}},
		{name: "blend toward the next level", depth: 300, size: 80, yaw: 0.8, tint: [2]float32{1, 1}},
		{name: "deep level, tinted", depth: 600, size: 40, yaw: 1.2, tint: [2]float32{0.7, 0.4}},
		{name: "smallest levels", depth: 900, size: 9, yaw: 1.2, tint: [2]float32{1, 1}},
		{name: "mirrored single face", depth: 300, size: 90, yaw: 2.1, tint: [2]float32{1, 1}, mirror: true, single: true},
		{name: "stepped foreground walls", depth: 300, size: 100, yaw: 0.8, tint: [2]float32{1, 1}, walls: true},
		{name: "column layout", depth: 300, size: 100, yaw: 0.8, tint: [2]float32{0.7, 0.4}, narrow: true},
		{name: "grazing face", depth: 300, size: 100, yaw: 0.02, tint: [2]float32{1, 1}},
		{name: "Panini", depth: 300, size: 60, yaw: 0.8, tint: [2]float32{1, 1}, panini: true},
		{name: "Panini column layout", depth: 300, size: 60, yaw: 0.8, tint: [2]float32{1, 1}, panini: true, narrow: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g.camera.FOV = squareProjectionFOV(w, h)
			g.combatPreferences.WideView = tc.panini
			if tc.panini {
				g.camera.FOV = 108 * math.Pi / 180
			}
			bottom := float64(h)/2 + tc.size/2
			slab, ok := r.prepareStandeeSlab(sprite, key, tc.depth, 0, tc.yaw, tc.depth, tc.size, bottom, 1, tc.tint[0], tc.tint[1], false, tc.mirror, 0, nil, -1)
			if !ok {
				t.Fatal("fixture did not project")
			}
			if tc.single {
				slab.thin(1)
			}
			r.standeeWallOcclusion = standeeWallOcclusion{}
			for x := range g.depthBuffer {
				g.depthBuffer[x], g.wallTopBuffer[x] = g.camera.ViewDist, 0
				if tc.walls {
					g.depthBuffer[x] = tc.depth - 20
					g.wallTopBuffer[x] = float64(h/2-40+(x/9)%4*15) + 0.375
				}
			}
			var packed, levels bool
			runOnDrawFrame(func(_ *ebiten.Image) {
				previous := activeWorldRaster
				defer func() { activeWorldRaster = previous }()
				var raster paniniRasterizer
				draw := func(dst *ebiten.Image) {
					if tc.panini {
						raster.configure(dst, g.widePaniniProjection())
						activeWorldRaster = &raster
					}
					dst.Clear()
					if !tc.narrow {
						r.drawStandeeSlabColumns(dst, slab, -1, -1)
						return
					}
					for x := slab.minX; x <= slab.maxX; x++ {
						r.drawStandeeSlabColumns(dst, slab, x, x)
					}
				}
				// The source each path binds tells which one drew.
				sheet := r.standeeSheets[key]
				r.standeeSheetOpts.Images[0], r.standeeTrilinearOpts.Images[0] = nil, nil
				draw(sheetImage)
				packed = r.standeeSheetOpts.Images[0] == sheet.image && r.standeeTrilinearOpts.Images[0] == nil
				delete(r.standeeSheets, key)
				draw(levelImage)
				levels = r.standeeTrilinearOpts.Images[0] != nil
				r.standeeSheets[key] = sheet
				sheetImage.ReadPixels(sheetPixels)
				levelImage.ReadPixels(levelPixels)
			})
			if !packed || !levels {
				t.Fatalf("paths under comparison: sheet=%v level images=%v, want both", packed, levels)
			}
			var covered, differing, worst int
			for i := 0; i < len(sheetPixels); i += 4 {
				if sheetPixels[i+3] > 8 || levelPixels[i+3] > 8 {
					covered++
				}
				for c := 0; c < 4; c++ {
					if d := abs(int(sheetPixels[i+c]) - int(levelPixels[i+c])); d > 0 {
						worst = max(worst, d)
						if c == 0 || d > 1 {
							differing++
						}
					}
				}
			}
			if covered == 0 {
				t.Fatal("fixture drew nothing")
			}
			t.Logf("covered %d px, worst channel delta %d", covered, worst)
			// Same texels and weights; only float rounding of the weights differs.
			if worst > 2 {
				t.Fatalf("sheet and level images disagree by %d/255 (%d differing samples)", worst, differing)
			}
		})
	}
}

// A column-material quad is cut once per knot it crosses; any other quad stays
// with the triangle clipper, which bounds attribute error by subdivision.
func TestPaniniColumnQuadCutsAtKnots(t *testing.T) {
	dst := ebiten.NewImage(640, 480)
	defer dst.Deallocate()
	const center, focal, distance = 320.0, 200.0, 1.0
	var p paniniRasterizer
	p.configure(dst, paniniProjection{center: center, horizon: 250, focal: focal, distance: distance, scale: 1 + distance*math.Hypot(1, center/focal)})

	side := func(x, y, depth float32) ebiten.Vertex {
		return ebiten.Vertex{DstX: x, DstY: y, SrcX: 3.25, SrcY: 7.5, ColorR: 1, ColorA: 1, Custom0: depth, Custom1: depth * 0.5, Custom3: -2}
	}
	// Top-left, top-right, bottom-left, bottom-right; the left edge is outside.
	quad := []ebiten.Vertex{side(-40.5, 60, 0.01), side(500.25, 90, 0.02), side(-40.5, 400, 0.01), side(500.25, 380, 0.02)}
	indices := []uint32{0, 1, 2, 1, 3, 2}

	var cuts []float64
	cuts = append(cuts, 0)
	for _, knot := range p.knots {
		if knot > 0 && knot < 500.25 {
			cuts = append(cuts, knot)
		}
	}
	cuts = append(cuts, 500.25)
	if len(cuts) < 6 {
		t.Fatalf("fixture crosses only %d knots", len(cuts)-2)
	}
	vertices, out := projectWorldTriangles(&p, quad, indices, true)
	if len(vertices) != 2*len(cuts) || len(out) != 6*(len(cuts)-1) {
		t.Fatalf("%d cuts produced %d vertices and %d indices, want %d and %d", len(cuts), len(vertices), len(out), 2*len(cuts), 6*(len(cuts)-1))
	}
	for i, x := range cuts {
		ratio := (x + 40.5) / (500.25 + 40.5)
		for j, edge := range [2][2]float64{{60, 90}, {400, 380}} {
			wantX, wantY := p.projection.presentedPoint(x, edge[0]+(edge[1]-edge[0])*ratio)
			got := vertices[2*i+j]
			if math.Abs(float64(got.DstX)-wantX) > 1e-3 || math.Abs(float64(got.DstY)-wantY) > 1e-3 {
				t.Fatalf("cut %d edge %d at (%.3f, %.3f), want the projected edge point (%.3f, %.3f)", i, j, got.DstX, got.DstY, wantX, wantY)
			}
			if wantDepth := 0.01 + 0.01*ratio; math.Abs(float64(got.Custom0)-wantDepth) > 1e-6 || got.SrcX != 3.25 || got.Custom3 != -2 {
				t.Fatalf("cut %d edge %d carries %+v, want depth %.5f and the constant slots", i, j, got, wantDepth)
			}
		}
	}
	area := func(vertices []ebiten.Vertex, indices []uint32) (sum float64) {
		for i := 0; i+2 < len(indices); i += 3 {
			a, b, c := vertices[indices[i]], vertices[indices[i+1]], vertices[indices[i+2]]
			sum += math.Abs(float64(b.DstX-a.DstX)*float64(c.DstY-a.DstY)-float64(c.DstX-a.DstX)*float64(b.DstY-a.DstY)) / 2
		}
		return sum
	}
	cutArea := area(vertices, out)

	// The same quad in another index order is not recognized: the clipper
	// covers the same projected area, clipped to the viewport rows.
	inside := []ebiten.Vertex{side(-40.5, 60, 0.01), side(500.25, 90, 0.02), side(-40.5, 400, 0.01), side(500.25, 380, 0.02)}
	vertices, out = projectWorldTriangles(&p, inside, []uint32{2, 0, 1, 2, 1, 3}, true)
	if clipped := area(vertices, out); math.Abs(clipped-cutArea) > cutArea*1e-3 {
		t.Fatalf("triangle clipper covers %.1f px, the quad cut %.1f", clipped, cutArea)
	}

	// Attributes that vary along a side need the clipper's subdivision.
	varying := append([]ebiten.Vertex(nil), quad...)
	varying[2].SrcY = 90
	p.vertices, p.indices = p.vertices[:0], p.indices[:0]
	if p.appendColumnQuad(&varying[0], &varying[1], &varying[2], &varying[3]) || len(p.vertices) != 0 {
		t.Fatal("a quad with attributes varying along its side took the column cut")
	}
}

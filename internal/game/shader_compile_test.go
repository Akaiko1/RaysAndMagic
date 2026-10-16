package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/shaders"

	"github.com/hajimehoshi/ebiten/v2"
)

// Kage sources compile at runtime, so a Go build can't catch shader syntax
// errors - compile them here so a broken shader fails the suite, not the game.
func TestKageShadersCompile(t *testing.T) {
	for _, name := range shaders.Names() {
		if _, err := ebiten.NewShader([]byte(shaders.Source(name))); err != nil {
			t.Errorf("%s shader failed to compile: %v", name, err)
		}
	}
}

// A shader branch on a Go-side id compares against a named const, never a
// bare number. Each branch variable lists the Go const blocks whose ids it
// reads (by one member's name): the shader declares every member of those
// blocks with its Go value, the variable is compared only with those names,
// never with a number or folded into arithmetic with one, and the values it
// meets are distinct. Every integer const a shader declares at top level
// repeats a Go constant, and a const block line the test cannot read fails.
// A value inserted into or appended to a Go iota list then fails here instead
// of recolouring later ids or drawing nothing.
func TestShaderConstantsMatchGo(t *testing.T) {
	goValues, goBlocks := goIntConsts(t)
	branches := map[string]map[string][]string{
		"spell_body.kage":        {"kind": {"spellFireball"}},
		"spell_bolt.kage":        {"kind": {"spellFireball", "boltMaterialGauntlet"}},
		"weapon_body.kage":       {"material": {"meshMetal"}, "DepthStage": {"depthStageOff"}},
		"boss_relic.kage":        {"material": {"meshMetal"}},
		"weapon_ribbon.kage":     {"material": {"weaponMetal"}},
		"world_material.kage":    {"mat": {"worldIron"}},
		"world_status.kage":      {"kind": {"statusShapeDisc"}},
		"impact_material.kage":   {"kind": {"impactCloudHot"}, "marker": {"impactCloudHot"}},
		"actor_spell.kage":       {"style": {"actorSpellMindWeb"}},
		"collection_slot.kage":   {"Mode": {"collectionModeNiche"}},
		"cast_overlay.kage":      {"Style": {"castStyleDarkness"}, "style": {"castStyleDarkness"}, "Pass": {"castPassBack"}},
		"floor.kage":             {"b": {"floorBlendHard"}, "a": {"floorFrozenWater", "floorBlendBoundary", "floorBlendShore"}},
		"standee_trilinear.kage": {"surface": {"standeeLayerNearSticker"}},
		"cast_object.kage":       {"mat": {"castMatPlain"}},
		"zone_plume.kage":        {"look": {"firePlumeTongues"}},
	}
	for _, name := range shaders.Names() {
		source := shaders.Source(name)
		declared, unread := kageIntConsts(source)
		for _, line := range unread {
			t.Errorf("%s: const line %q is not an integer id the test can read", name, line)
		}
		for id, value := range declared {
			if want, ok := goValues[id]; !ok || want != value {
				t.Errorf("%s: const %s = %d does not repeat a Go constant (Go: %d, found: %v)", name, id, value, want, ok)
			}
		}
		code := regexp.MustCompile(`//[^\n]*`).ReplaceAllString(source, "")
		for branch, anchors := range branches[name] {
			family, seen := map[string]bool{}, map[int]string{}
			for _, anchor := range anchors {
				block, ok := goBlocks[anchor]
				if !ok {
					t.Fatalf("%s: no Go const %s", name, anchor)
				}
				for _, id := range block {
					family[id] = true
					if _, ok := declared[id]; !ok {
						t.Errorf("%s: %s reads %s, which the shader does not declare", name, branch, id)
					}
					if other, dup := seen[goValues[id]]; dup {
						t.Errorf("%s: %s meets %s and %s, both %d", name, branch, other, id, goValues[id])
					}
					seen[goValues[id]] = id
				}
			}
			v := regexp.QuoteMeta(branch)
			compare := `\s*(?:==|!=|<=?|>=?)\s*`
			for _, pattern := range []string{`\b` + v + compare + `([A-Za-z_]\w*|-?[\d.]+)`, `\b([A-Za-z_]\w*|[\d.]+)` + compare + v + `\b`} {
				for _, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(code, -1) {
					if !family[m[1]] {
						t.Errorf("%s compares %s with %s, outside its ids", name, branch, m[1])
					}
				}
			}
			if regexp.MustCompile(`\b` + v + `\s*[-+*/]\s*[\d.]|[\d.]\s*[-+*/]\s*` + v + `\b`).MatchString(code) {
				t.Errorf("%s folds %s into arithmetic with a bare number", name, branch)
			}
		}
	}
}

// goIntConsts evaluates the integer constants this package declares (integer
// literals and iota blocks with + - * and implicit repetition) and maps each
// name to every name of its const declaration.
func goIntConsts(t *testing.T) (map[string]int, map[string][]string) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	values, blocks := map[string]int{}, map[string][]string{}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			var last ast.Expr
			var names []string
			for i, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				if len(vs.Values) > 0 {
					last = vs.Values[0]
				}
				if v, ok := evalIntConst(last, i); ok && len(vs.Names) == 1 {
					values[vs.Names[0].Name] = v
					names = append(names, vs.Names[0].Name)
				}
			}
			for _, name := range names {
				blocks[name] = names
			}
		}
	}
	return values, blocks
}
func evalIntConst(e ast.Expr, iota int) (int, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.INT {
			return 0, false
		}
		v, err := strconv.Atoi(x.Value)
		return v, err == nil
	case *ast.Ident:
		return iota, x.Name == "iota"
	case *ast.ParenExpr:
		return evalIntConst(x.X, iota)
	case *ast.UnaryExpr:
		v, ok := evalIntConst(x.X, iota)
		return -v, ok && x.Op == token.SUB
	case *ast.BinaryExpr:
		a, okA := evalIntConst(x.X, iota)
		b, okB := evalIntConst(x.Y, iota)
		switch {
		case !okA || !okB:
		case x.Op == token.ADD:
			return a + b, true
		case x.Op == token.SUB:
			return a - b, true
		case x.Op == token.MUL:
			return a * b, true
		}
	}
	return 0, false
}

// kageIntConsts returns the integer constants a Kage source declares at top
// level, in const blocks or single const lines; a function's local const (a
// loop count) is not an id. A const block holds ids only, so any other entry
// there comes back unread. A single const line may be a tuning value.
func kageIntConsts(source string) (map[string]int, []string) {
	assignment := regexp.MustCompile(`^(?:const\s+)?([A-Za-z_]\w*)\s*=\s*(-?\d+)\s*(?://.*)?$`)
	opener := regexp.MustCompile(`^const\s*\(\s*(?://.*)?$`)
	out := map[string]int{}
	var unread []string
	inBlock := false
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case opener.MatchString(line):
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case inBlock && (trimmed == "" || strings.HasPrefix(trimmed, "//")):
			continue
		case !inBlock && !strings.HasPrefix(line, "const "):
			continue
		}
		if m := assignment.FindStringSubmatch(trimmed); m != nil {
			out[m[1]], _ = strconv.Atoi(m[2])
		} else if inBlock {
			unread = append(unread, trimmed)
		}
	}
	return out, unread
}

func TestWrapPanoramaOffsetBoundsLargeAngles(t *testing.T) {
	const width = 1774.0
	for _, offset := range []float64{-1e12 - 0.25, -width, -0.25, 0, width, 1e12 + 0.25} {
		got := wrapPanoramaOffset(offset, width)
		if got < 0 || got >= width {
			t.Fatalf("wrapPanoramaOffset(%g, %g) = %g, want [0,%g)", offset, width, got, width)
		}
	}
	if got := wrapPanoramaOffset(-0.25, width); math.Abs(got-(width-0.25)) > 1e-9 {
		t.Fatalf("negative wrap = %g, want %g", got, width-0.25)
	}
}

func TestSkyShaderWrapKeepsEveryColumnCovered(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU readback requires the live Ebitengine debug-sim loop")
	}
	t.Chdir("../..")
	source, err := decodePNG("assets/sprites/sky/highlands_panorama_night.png")
	if err != nil {
		t.Fatal(err)
	}
	panorama := ebiten.NewImageFromImage(source)
	shader, err := ebiten.NewShader([]byte(shaders.Source("sky.kage")))
	if err != nil {
		t.Fatal(err)
	}

	const dstW, dstH = 1024, 384
	srcW, srcH := float32(panorama.Bounds().Dx()), float32(panorama.Bounds().Dy())
	srcSpan := float32(dstW) * srcH / dstH
	dst := ebiten.NewImage(dstW, dstH)
	pixels := make([]byte, 4*dstW*dstH)
	indices := []uint16{0, 1, 2, 1, 3, 2}
	opts := &ebiten.DrawTrianglesShaderOptions{}
	opts.Images[0] = panorama

	// Exercise positive, negative and exact-boundary phases. The large values
	// also guard against a long unnormalised RT camera angle losing enough
	// float precision to expose the texture atlas between panorama repetitions.
	for _, center := range []float32{0, 0.25, srcW - 0.25, srcW, -srcW, 1e6 + 0.25, -1e6 - 0.25} {
		wrappedCenter := float32(wrapPanoramaOffset(float64(center), float64(srcW)))
		sx0, sx1 := wrappedCenter-srcSpan/2, wrappedCenter+srcSpan/2
		vertices := []ebiten.Vertex{
			{DstX: 0, DstY: 0, SrcX: sx0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: dstW, DstY: 0, SrcX: sx1, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: 0, DstY: dstH, SrcX: sx0, SrcY: srcH, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: dstW, DstY: dstH, SrcX: sx1, SrcY: srcH, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		}
		dst.Clear()
		dst.DrawTrianglesShader(vertices, indices, shader, opts)
		dst.ReadPixels(pixels)
		for x := 0; x < dstW; x++ {
			var alphaSum, lumaSum int
			for y := 0; y < dstH; y++ {
				i := 4 * (y*dstW + x)
				alphaSum += int(pixels[i+3])
				lumaSum += 3*int(pixels[i]) + 6*int(pixels[i+1]) + int(pixels[i+2])
			}
			if alphaSum != 255*dstH {
				t.Fatalf("phase %.2f column %d alpha average %.2f: panorama wrap sampled outside its atlas region", center, x, float64(alphaSum)/dstH)
			}
			if avg := float64(lumaSum) / (10 * dstH); math.IsNaN(avg) || avg < 5 {
				t.Fatalf("phase %.2f column %d luma average %.2f: black panorama gap", center, x, avg)
			}
		}
	}
}

// Kage's pixel unit allows differently-sized images, but imageSrcNAt keeps
// image-0 pixel coordinates rather than scaling them to image N. Exercise the
// actual GPU path so a bad mapping cannot shrink a distant standee into the
// upper-left part of its quad (which also makes a floor-anchored token float).
func TestStandeeTrilinearShaderFillsOriginalQuad(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU readback requires the live Ebitengine debug-sim loop")
	}
	shader, err := ebiten.NewShader([]byte(shaders.Source("standee_trilinear.kage")))
	if err != nil {
		t.Fatal(err)
	}
	level0 := ebiten.NewImage(8, 8)
	level1 := ebiten.NewImage(4, 4)
	level2 := ebiten.NewImage(2, 2)
	coreLevel := ebiten.NewImage(4, 4)
	level1.Fill(color.RGBA{R: 0xff, A: 0xff})
	level2.Fill(color.RGBA{B: 0xff, A: 0xff})
	coreLevel.Fill(color.RGBA{G: 0xff, A: 0xff})

	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0.5, SrcY: 0.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
		{DstX: 8, DstY: 0, SrcX: 7.5, SrcY: 0.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
		{DstX: 0, DstY: 8, SrcX: 0.5, SrcY: 7.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
		{DstX: 8, DstY: 8, SrcX: 7.5, SrcY: 7.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
	}
	dst := ebiten.NewImage(8, 8)
	opts := &ebiten.DrawTrianglesShaderOptions{}
	opts.Images[0] = level0
	opts.Images[1] = level1
	opts.Images[2] = level2
	opts.Images[3] = coreLevel
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got.A < 250 || got.R < 125 || got.R > 130 || got.B < 125 || got.B > 130 {
				t.Fatalf("pixel (%d,%d) = %#v; mip blend must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// Core shells use one tap from their prefiltered mip. Exercise its
	// differently-sized coordinate mapping across the complete destination too.
	for i := range vertices {
		vertices[i].Custom1 = 1
	}
	dst.Clear()
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got != (color.RGBA{G: 0xff, A: 0xff}) {
				t.Fatalf("core pixel (%d,%d) = %#v; selected mip must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// The far sticker blends the same adjacent mip levels as the near sticker.
	// Its level-space mapping must also cover the complete destination.
	for i := range vertices {
		vertices[i].Custom1 = 2
	}
	dst.Clear()
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got.A != 0xff || got.G != 0 || got.R < 125 || got.R > 130 || got.B < 125 || got.B > 130 {
				t.Fatalf("far-sticker pixel (%d,%d) = %#v; adjacent mip blend must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// At 1:1 or magnification the visible sticker bypasses mip sampling and
	// retains the authored base pixels.
	level0.Fill(color.RGBA{R: 0xff, G: 0xff, A: 0xff})
	for i := range vertices {
		vertices[i].Custom1 = 0
		vertices[i].Custom2 = 0
	}
	dst.Clear()
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got != (color.RGBA{R: 0xff, G: 0xff, A: 0xff}) {
				t.Fatalf("base pixel (%d,%d) = %#v; level 0 must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// Animation frames are non-zero-origin SubImages. Their level 0 must still
	// occupy the complete normalized mip image instead of retaining sheet-space
	// coordinates and being drawn outside it.
	sheet := ebiten.NewImage(12, 10)
	frame := sheet.SubImage(image.Rect(4, 2, 12, 10)).(*ebiten.Image)
	frame.Fill(color.RGBA{G: 0xff, A: 0xff})
	r := &Renderer{}
	chain := r.standeeMipChainFor(standeeMipKey{}, frame)
	for _, point := range []image.Point{image.Pt(0, 0), image.Pt(7, 7)} {
		got := color.RGBAModel.Convert(chain.levels[0].At(point.X, point.Y)).(color.RGBA)
		if got != (color.RGBA{G: 0xff, A: 0xff}) {
			t.Fatalf("normalized SubImage mip at %v = %#v; want opaque green", point, got)
		}
	}
}

func TestStandeeVolumeShaderPreservesLayersAndWallClip(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU readback requires the live Ebitengine debug-sim loop")
	}
	shader, err := ebiten.NewShader([]byte(shaders.Source("standee_volume.kage")))
	if err != nil {
		t.Fatal(err)
	}

	sticker := ebiten.NewImage(8, 8)
	stickerMip := ebiten.NewImage(8, 8)
	stickerNextMip := ebiten.NewImage(8, 8)
	core := ebiten.NewImage(8, 8)
	sticker.Fill(color.RGBA{R: 0xff, A: 0xff})
	stickerMip.Fill(color.RGBA{R: 0xff, A: 0xff})
	stickerNextMip.Fill(color.RGBA{R: 0xff, A: 0xff})
	core.Fill(color.RGBA{G: 0xff, A: 0xff})

	// Horizon 2 deliberately differs from half the destination height. At
	// near depth 8 these invariants produce a face from y=-2 through y=6.
	// Far depth 10 is slightly smaller, just like a real thick slab.
	const heightScale = 64
	const bottomScale = 32
	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 1.0 / 100, ColorB: 0, ColorA: 6, Custom0: 0.1, Custom1: 0.125, Custom2: 0, Custom3: 0},
		{DstX: 8, DstY: 0, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 1.0 / 100, ColorB: 0, ColorA: 6, Custom0: 0.1, Custom1: 0.125, Custom2: 0.1, Custom3: 0.125},
		{DstX: 0, DstY: 8, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 1.0 / 100, ColorB: 0, ColorA: 6, Custom0: 0.1, Custom1: 0.125, Custom2: 0, Custom3: 0},
		{DstX: 8, DstY: 8, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 1.0 / 100, ColorB: 0, ColorA: 6, Custom0: 0.1, Custom1: 0.125, Custom2: 0.1, Custom3: 0.125},
	}
	indices := []uint16{0, 1, 2, 1, 3, 2}
	opts := &ebiten.DrawTrianglesShaderOptions{Uniforms: map[string]any{"Horizon": float32(2)}}
	opts.Images[0] = sticker
	opts.Images[1] = stickerMip
	opts.Images[2] = stickerNextMip
	opts.Images[3] = core
	dst := ebiten.NewImage(8, 8)

	draw := func() []byte {
		dst.Clear()
		dst.DrawTrianglesShader(vertices, indices, shader, opts)
		pixels := make([]byte, 8*8*4)
		dst.ReadPixels(pixels)
		return pixels
	}
	pixel := func(pixels []byte, x, y int) color.RGBA {
		i := (y*8 + x) * 4
		return color.RGBA{R: pixels[i], G: pixels[i+1], B: pixels[i+2], A: pixels[i+3]}
	}

	pixels := draw()
	if got := pixel(pixels, 4, 7); got.A != 0 {
		t.Fatalf("face ignored the authored horizon: bottom pixel = %#v", got)
	}
	if got := pixel(pixels, 4, 4); got.R < 250 || got.G != 0 || got.A < 250 {
		t.Fatalf("near sticker = %#v, want opaque red front face", got)
	}

	// A transparent sticker must reveal the virtual wooden shell stack rather
	// than flattening the standee to a single face.
	sticker.Clear()
	stickerMip.Clear()
	stickerNextMip.Clear()
	pixels = draw()
	if got := pixel(pixels, 4, 4); got.G < 150 || got.R != 0 || got.A < 250 {
		t.Fatalf("core shell = %#v, want visible opaque green volume", got)
	}

	// Exercise the full Go-validated capacity with translucent shells: opaque
	// shells would hide a shortened shader loop after its first iteration.
	core.Fill(color.RGBA{G: 16, A: 16})
	for i := range vertices {
		vertices[i].ColorA = float32(config.MaxStandeeCoreLayers)
	}
	pixels = draw()
	wantAlpha := 255 * (1 - math.Pow(1-16.0/255, config.MaxStandeeCoreLayers))
	if got := pixel(pixels, 4, 4); math.Abs(float64(got.A)-wantAlpha) > 1 {
		t.Fatalf("volume shader must composite all %d core layers: alpha=%d, want %.1f", config.MaxStandeeCoreLayers, got.A, wantAlpha)
	}
	core.Fill(color.RGBA{G: 0xff, A: 0xff})

	// A wall at depth 7 is in front of every slab layer. Its top at y=4 keeps
	// the canopy visible above it and clips the lower half, matching the general
	// per-surface standee path.
	sticker.Fill(color.RGBA{R: 0xff, A: 0xff})
	for i := range vertices {
		vertices[i].ColorG = 1.0 / 7
		vertices[i].ColorB = 4
	}
	pixels = draw()
	if got := pixel(pixels, 4, 2); got.R < 250 || got.A < 250 {
		t.Fatalf("canopy above wall = %#v, want visible sticker", got)
	}
	if got := pixel(pixels, 4, 6); got.A != 0 {
		t.Fatalf("slab below wall top = %#v, want transparent clipped pixel", got)
	}
}

// Tall surfaces need curved-diagonal subdivision as well as horizontal strips.
// This catches a visible texture-to-picking mismatch without a golden image.
func TestDirectPaniniMaterialCoordinatesMatchPicking(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU")
	}
	source, err := os.ReadFile("../shaders/testdata/panini_coordinates.kage")
	if err != nil {
		t.Fatal(err)
	}
	var worst float64
	var problem error
	runOnDrawFrame(func(_ *ebiten.Image) {
		dst := ebiten.NewImage(1280, 720)
		defer dst.Deallocate()
		projection := paniniProjection{640, 390, 640 / math.Tan(54*math.Pi/180), 1, 1 + 1/math.Cos(54*math.Pi/180)}
		var raster paniniRasterizer
		raster.configure(dst, projection)
		shader, err := ebiten.NewShader(source)
		if err != nil {
			problem = err
			return
		}
		defer shader.Deallocate()
		vertices := []ebiten.Vertex{{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0}, {DstX: 1280, DstY: 0, SrcX: 1280, SrcY: 0}, {DstX: 0, DstY: 720, SrcX: 0, SrcY: 720}, {DstX: 1280, DstY: 720, SrcX: 1280, SrcY: 720}}
		previous := activeWorldRaster
		defer func() { activeWorldRaster = previous }()
		activeWorldRaster = &raster
		worldDrawTrianglesShader(dst, vertices, weaponQuadIndices, shader, nil)
		activeWorldRaster = previous
		px := make([]byte, 1280*720*4)
		dst.ReadPixels(px)
		worst = 0.0
		for y := 2; y < 718; y += 3 {
			for x := 2; x < 1278; x += 3 {
				sx, sy := projection.sourcePoint(float64(x)+.5, float64(y)+.5)
				i := (y*1280 + x) * 4
				for c, v := range []float64{sx, sy} {
					delta := math.Abs(float64(px[i+c])*16/255 - math.Mod(v, 16))
					delta = math.Min(delta, 16-delta)
					worst = max(worst, delta)
				}
			}
		}

	})
	if problem != nil {
		t.Fatal(problem)
	}
	if worst > .2 {
		t.Fatalf("rendered material differs from picking coordinates by %.3f pixels", worst)
	}
}

func TestPaniniDefersBlurWarmupUntilPerspectiveDraw(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live GPU draw")
	}
	cfg := setupPreviewSandboxTest(t)
	t.Chdir("../..")
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	g := p.g
	runOnDrawFrame(func(_ *ebiten.Image) {
		g.handleResize(image.Pt(1280, 720), image.Pt(1280, 720))
		dst := ebiten.NewImage(1280, 720)
		defer dst.Deallocate()
		g.combatPreferences.WideView = true
		g.turnBlurWarm = false
		g.gameLoop.drawExplorationScene(dst)
		if g.turnBlurWarm {
			t.Error("Panini consumed perspective blur warm-up")
		}
		g.combatPreferences.PaniniDisabled = true
		g.gameLoop.drawExplorationScene(dst)
		if !g.turnBlurWarm || g.blurShader == nil || g.turnBlurUniform == nil {
			t.Error("first idle perspective frame did not submit blur pipeline")
		}
	})
}

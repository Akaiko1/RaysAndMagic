package game

import (
	"image"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/world"
)

type campRestPresentation struct {
	sprite                         string
	elapsed, fadeIn, hold, fadeOut int
	presented                      bool
	surface                        *ebiten.Image
	dissolve                       *ebiten.Shader
	revealPattern, hidePattern     campDissolvePattern
	revealRadius, hideRadius       float32
}

func (g *MMGame) campSceneSprite() string {
	if wm := world.GlobalWorldManager; wm != nil {
		ts := g.config.GetTileSize()
		biome := g.biomeAtTile(TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts))
		if scene := wm.Biomes[biome].CampScene; scene != "" {
			return scene
		}
	}
	if scene := g.config.Camping.DefaultScene; scene != "" {
		return scene
	}
	return config.DefaultCampingConfig().DefaultScene
}

func (g *MMGame) beginCampRest() {
	g.cancelCampPresentation()
	c, defaults := g.config.Camping, config.DefaultCampingConfig()
	frames := func(seconds, fallback float64) int {
		if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			seconds = fallback
		}
		return max(1, int(math.Round(seconds*float64(g.config.GetTPS()))))
	}
	g.campRest = &campRestPresentation{sprite: g.campSceneSprite(),
		revealPattern: newCampDissolvePattern(c), hidePattern: newCampDissolvePattern(c),
		fadeIn:  frames(c.FadeInSeconds, defaults.FadeInSeconds),
		hold:    frames(c.HoldSeconds, defaults.HoldSeconds),
		fadeOut: frames(c.FadeOutSeconds, defaults.FadeOutSeconds)}
}

func (g *MMGame) cancelCampPresentation() {
	g.campConfirmOpen = false
	if s := g.campRest; s != nil {
		if s.surface != nil {
			s.surface.Deallocate()
		}
		if s.dissolve != nil {
			s.dissolve.Deallocate()
		}
	}
	g.campRest = nil
}

func (g *MMGame) tickCampRest() {
	s := g.campRest
	if s == nil || !s.presented || topModalLayerFor(g, false) != modalLayerCampRest {
		return
	}
	// A newly requested picture must finish streaming before its display clock
	// runs. Loading owns the screen and its own pause during that interval.
	if gl := g.gameLoop; gl != nil && gl.loading != nil && gl.loading.awaitingFrame {
		return
	}
	s.elapsed++
	if s.elapsed >= s.fadeIn+s.hold+s.fadeOut {
		g.cancelCampPresentation()
	}
}

func (s *campRestPresentation) alpha() float32 {
	t := 1.0
	if s.elapsed < s.fadeIn {
		t = float64(s.elapsed) / float64(max(1, s.fadeIn))
	} else if s.elapsed > s.fadeIn+s.hold {
		t = float64(s.fadeIn+s.hold+s.fadeOut-s.elapsed) / float64(max(1, s.fadeOut))
	}
	t = max(0, min(1, t))
	return float32(t * t * (3 - 2*t))
}

func (ui *UISystem) drawCampRest(screen *ebiten.Image) {
	g, s := ui.game, ui.game.campRest
	if s == nil {
		return
	}
	img := g.sprites.GetSprite(s.sprite)
	if img == nil {
		// Missing custom art must not trap the player in a permanent modal.
		if !g.sprites.HasSprite(s.sprite) {
			s.presented = true
		}
		return
	}
	s.presented = true // Display acknowledgement, not simulation progress.
	viewport := screen.Bounds().Intersect(image.Rect(0, 0, g.config.GetScreenWidth(), gameplayViewportBottom(g)))
	w, h := viewport.Dx(), viewport.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	if s.surface == nil || s.surface.Bounds().Size() != viewport.Size() {
		if s.surface != nil {
			s.surface.Deallocate()
		}
		s.surface = ebiten.NewImage(w, h)
		s.revealRadius = s.revealPattern.coverageRadius(w, h)
		s.hideRadius = s.hidePattern.coverageRadius(w, h)
	}
	if s.dissolve == nil {
		var err error
		s.dissolve, err = ebiten.NewShader([]byte(campDissolveShaderSrc))
		if err != nil {
			panic(err)
		}
	}
	// Cover-fit through the shared scaler. The render target clips the overflow,
	// preserving one uniform scale and leaving the party HUD untouched.
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	x, y, dw, dh := campSceneCoverGeometry(w, h, iw, ih, g.config.Camping.MaxTopCropFraction)
	graphics.DrawImageScaled(s.surface, img, x, y, dw, dh, nil)
	pixels := g.config.Camping.DissolvePixelSize
	if pixels <= 0 {
		pixels = config.DefaultCampingConfig().DissolvePixelSize
	}
	pattern, radius := &s.revealPattern, s.revealRadius
	if s.elapsed > s.fadeIn+s.hold {
		pattern, radius = &s.hidePattern, s.hideRadius
	}
	op := &ebiten.DrawRectShaderOptions{Uniforms: map[string]any{
		"Progress": s.alpha(), "PixelSize": float32(pixels),
		"Centers": pattern.centers[:], "NoiseSeed": pattern.noiseSeed, "Radius": radius,
	}}
	op.Images[0] = s.surface
	op.GeoM.Translate(float64(viewport.Min.X), float64(viewport.Min.Y))
	screen.DrawRectShader(w, h, s.dissolve, op)
}

// Patterns are sampled once per camp, separately for appearing/disappearing.
// Normalized centers survive resizing; Draw never consumes random numbers.
type campDissolvePattern struct {
	centers   [12]float32 // Six vec2 uniforms; the sixth duplicates the fifth for five clusters.
	noiseSeed float32
}

func newCampDissolvePattern(c config.CampingConfig) campDissolvePattern {
	defaults := config.DefaultCampingConfig()
	low, high := c.DissolveClustersMin, c.DissolveClustersMax
	if low <= 0 {
		low = defaults.DissolveClustersMin
	}
	if high <= 0 {
		high = defaults.DissolveClustersMax
	}
	high = min(6, max(1, high))
	low = min(high, max(1, low))
	count := low + rand.IntN(high-low+1)
	p := campDissolvePattern{noiseSeed: rand.Float32() * 1000}
	// Jittered cells separate the origins so each reads as its own growing patch.
	cells := rand.Perm(6)
	for i := 0; i < count; i++ {
		cell := cells[i]
		p.centers[i*2] = (float32(cell%3) + .2 + rand.Float32()*.6) / 3
		p.centers[i*2+1] = (float32(cell/3) + .2 + rand.Float32()*.6) / 2
	}
	for i := count; i < 6; i++ {
		p.centers[i*2], p.centers[i*2+1] = p.centers[(count-1)*2], p.centers[(count-1)*2+1]
	}
	return p
}

// Bound the farthest point from its nearest origin, including the space between
// samples. Distances use viewport height, keeping clusters round on ultrawide.
// Recomputed only when allocating/resizing the transient render target.
func (p campDissolvePattern) coverageRadius(w, h int) float32 {
	aspect := float64(w) / float64(h)
	farthest := 0.0
	const steps = 24
	for y := 0; y <= steps; y++ {
		for x := 0; x <= steps; x++ {
			nearest := math.Inf(1)
			for i := 0; i < 6; i++ {
				dx := (float64(x)/steps - float64(p.centers[i*2])) * aspect
				dy := float64(y)/steps - float64(p.centers[i*2+1])
				nearest = math.Min(nearest, math.Hypot(dx, dy))
			}
			farthest = math.Max(farthest, nearest)
		}
	}
	return float32(farthest + math.Hypot(aspect, 1)/(2*steps))
}

// Pixels change only at the advancing/receding cluster boundary. Stable local
// noise roughens that boundary without scattering unrelated pixels everywhere.
//
//ebitengine:shadersource
const campDissolveShaderSrc = `//kage:unit pixels
package main
var Progress float
var PixelSize float
var Centers [6]vec2
var Radius float
var NoiseSeed float
func Fragment(dst vec4, src vec2, color vec4) vec4 {
    size := imageSrc0Size()
    cell := floor((src - imageSrc0Origin()) / PixelSize)
    point := (cell + vec2(0.5)) * PixelSize / size.y
    nearest := 100.0
    for i := 0; i < 6; i++ {
        center := Centers[i] * size / size.y
        nearest = min(nearest, distance(point, center))
    }
    noise := fract(sin(dot(cell, vec2(12.9898, 78.233)) + NoiseSeed) * 43758.5453)
    threshold := clamp(nearest / Radius * 0.9 + (noise - 0.5) * 0.035, 0.0, 0.95)
    alpha := smoothstep(threshold, threshold + 0.05, Progress)
    return imageSrc0At(src) * alpha
}
`

// Crop the foreground before the headroom. The authored scenes share a safe
// upper margin; YAML limits its removal independently of the viewport aspect.
func campSceneCoverGeometry(w, h, iw, ih int, maxTopCrop float64) (x, y, dw, dh float64) {
	if maxTopCrop <= 0 || maxTopCrop > 1 || math.IsNaN(maxTopCrop) {
		maxTopCrop = config.DefaultCampingConfig().MaxTopCropFraction
	}
	scale := math.Max(float64(w)/float64(iw), float64(h)/float64(ih))
	dw, dh = float64(iw)*scale, float64(ih)*scale
	return math.Min(0, (float64(w)-dw)/2), -math.Max(0, math.Min((dh-float64(h))/2, dh*maxTopCrop)), dw, dh
}

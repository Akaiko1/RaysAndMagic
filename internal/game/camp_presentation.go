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
	scene                          dissolveScene
	// watcherLine is the captain's remark over this rest (watcher.go); the
	// scene holds until it is read.
	watcherLine   *WatcherLine
	watcherRead   bool
	watcherLogged bool // his words showed: the line is spent and logged
}

// dissolveScene draws one picture through the camp dissolve shader: the
// pixel-cluster reveal and hide shared by camp rest and the story plates.
type dissolveScene struct {
	surface                    *ebiten.Image
	revealPattern, hidePattern campDissolvePattern
	revealRadius, hideRadius   float32
}

// dissolveShader is compiled once and shared by every camp scene and plate.
var dissolveShader *ebiten.Shader

func newDissolveScene(c config.CampingConfig) dissolveScene {
	return dissolveScene{revealPattern: newCampDissolvePattern(c), hidePattern: newCampDissolvePattern(c)}
}

func (d *dissolveScene) release() {
	if d.surface != nil {
		d.surface.Deallocate()
	}
	d.surface = nil
}

// draw renders img into rect (screen pixels) at the given reveal progress;
// place lays the picture out inside the rect. hiding switches to the second,
// independent pattern so the picture leaves differently from how it came.
func (d *dissolveScene) draw(screen *ebiten.Image, rect image.Rectangle, img *ebiten.Image,
	place func(w, h, iw, ih int) (x, y, dw, dh float64), progress float32, hiding bool, c config.CampingConfig) {
	w, h := rect.Dx(), rect.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	if d.surface == nil || d.surface.Bounds().Size() != rect.Size() {
		if d.surface != nil {
			d.surface.Deallocate()
		}
		d.surface = ebiten.NewImage(w, h)
		d.revealRadius = d.revealPattern.coverageRadius(w, h)
		d.hideRadius = d.hidePattern.coverageRadius(w, h)
	}
	if compileShader(&dissolveShader, "camp_dissolve.kage") != nil {
		return // a broken shader skips the picture; the rest still completes
	}
	// The render target clips any overflow, preserving one uniform scale.
	d.surface.Clear()
	x, y, dw, dh := place(w, h, img.Bounds().Dx(), img.Bounds().Dy())
	graphics.DrawImageScaled(d.surface, img, x, y, dw, dh, nil)
	pixels := c.DissolvePixelSize
	if pixels <= 0 {
		pixels = config.DefaultCampingConfig().DissolvePixelSize
	}
	pattern, radius := &d.revealPattern, d.revealRadius
	if hiding {
		pattern, radius = &d.hidePattern, d.hideRadius
	}
	op := &ebiten.DrawRectShaderOptions{Uniforms: map[string]any{
		"Progress": progress, "PixelSize": float32(float64(pixels) * uiScaleOf(screen)),
		"Centers": pattern.centers[:], "NoiseSeed": pattern.noiseSeed, "Radius": radius,
	}}
	op.Images[0] = d.surface
	op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	screen.DrawRectShader(w, h, dissolveShader, op)
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

// campRestFrames converts a camping duration to frames; an unset value takes
// the shipped default.
func (g *MMGame) campRestFrames(seconds, fallback float64) int {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		seconds = fallback
	}
	return g.framesForSeconds(seconds)
}

// beginCampRest starts the camp scene; hurt is whether the party lay down
// wounded (the rest has healed it by now), for the captain's line.
func (g *MMGame) beginCampRest(hurt bool) {
	g.cancelCampPresentation()
	c, defaults := g.config.Camping, config.DefaultCampingConfig()
	g.campRest = &campRestPresentation{sprite: g.campSceneSprite(), scene: newDissolveScene(c),
		fadeIn:  g.campRestFrames(c.FadeInSeconds, defaults.FadeInSeconds),
		hold:    g.campRestFrames(c.HoldSeconds, defaults.HoldSeconds),
		fadeOut: g.campRestFrames(c.FadeOutSeconds, defaults.FadeOutSeconds)}
	g.campRest.watcherLine = g.watcherCampLine(hurt)
}

func (g *MMGame) cancelCampPresentation() {
	g.campConfirmOpen = false
	if s := g.campRest; s != nil {
		s.scene.release()
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
	if g.loadingStall() {
		return
	}
	if l := s.watcherLine; l != nil && !s.watcherLogged && s.elapsed >= s.watcherWordsAt(g.storyTiming()) {
		s.watcherLogged = true
		g.speakWatcherLine(l)
	}
	if s.campWatcherHolding() {
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
	if img == nil && g.sprites.HasSprite(s.sprite) {
		return // still streaming: the clock waits for the picture
	}
	// Display acknowledgement, not simulation progress. Missing art must not
	// trap the player in a permanent modal: the clock runs and his line shows.
	s.presented = true
	if img != nil {
		// The dissolve shader works in the screen's own pixels, so the scene art
		// is cover-fitted at native resolution, leaving the party HUD untouched.
		viewport := uiPixelRect(screen, uiBounds(screen).Intersect(image.Rect(0, 0, g.config.GetScreenWidth(), gameplayViewportBottom(g))))
		cover := func(w, h, iw, ih int) (float64, float64, float64, float64) {
			return campSceneCoverGeometry(w, h, iw, ih, g.config.Camping.MaxTopCropFraction)
		}
		s.scene.draw(screen, viewport, img, cover, s.alpha(), s.elapsed > s.fadeIn+s.hold, g.config.Camping)
	}
	if s.watcherLine != nil {
		view := image.Rect(0, 0, g.config.GetScreenWidth(), gameplayViewportBottom(g))
		ui.drawCampWatcherLine(screen, view)
		ui.onGatedClick(layoutRectOf(view), func() bool { return ui.topModalLayer() == modalLayerCampRest && g.campRest == s }, g.releaseCampWatcher)
	}
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

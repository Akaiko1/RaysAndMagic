package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"ugataima/internal/shaders"
)

// Shared dispatch for validation and rendering. The numbers select material
// branches in cast_overlay.kage, not spell IDs or gameplay behavior.
var castOverlayStyles = map[string]float32{
	"darkness": 0, "town_portal": 1,
	"fire_aura": 2, "haste": 3, "shield": 4,
	"fate": 7, "shared_life": 8, "preservation": 9,
	"regeneration": 15, "purify": 16,
	"protection_from_magic": 17, "dispel_magic": 18,
	"celestial_armor": 21, "vampiric_weapon": 22,
	"pain_reflection": 23,
}

type castOverlayRenderer struct {
	shader   *ebiten.Shader
	opts     ebiten.DrawTrianglesShaderOptions
	vertices [4]ebiten.Vertex
	phase    [1]float32
	style    [1]float32
	viewport [2]float32
	origin   [2]float32
	pass     [1]float32
	object   castObjectRenderer
}

func (a buffFxAnim) objectPhase() (phase, alpha float64) {
	phase = float64(a.age) / float64(a.totalFrames())
	alpha = smoothStep(phase/.12) * (1 - smoothStep((phase-.72)/.28))
	return phase, alpha
}

func (r *Renderer) drawCastOverlay(dst *ebiten.Image, a buffFxAnim) {
	if a.overlay == "fire_aura" || a.overlay == "vampiric_weapon" {
		r.drawCastOverlayPass(dst, a, -1)
		r.drawEnchantedSword(dst, a)
		r.drawCastOverlayPass(dst, a, 1)
		return
	}
	if a.overlay == "preservation" {
		r.drawCastObject(dst, a, castWing)
		r.drawCastOverlayPass(dst, a, 0)
		return
	}
	r.drawCastOverlayPass(dst, a, 0)
	switch a.overlay {
	case "celestial_armor":
		r.drawCastObject(dst, a, castCuirass)
	case "haste":
		r.drawCastObject(dst, a, castHourglass)
	}
}

func (r *Renderer) drawEnchantedSword(dst *ebiten.Image, a buffFxAnim) {
	if r.ensureWeaponMaterialShaders() != nil {
		return
	}
	t, alpha := a.objectPhase()
	ox, oy, w, h := r.castOverlayViewport(a)
	angle := -math.Pi/2 + .10
	roll := .25 + math.Sin(t*math.Pi)*.50
	ca, sa, cr, sr := math.Cos(angle), math.Sin(angle), math.Cos(roll), math.Sin(roll)
	pose := volumeTransform{rotation: [3][3]float64{{ca, -cr * sa, sr * sa}, {sa, cr * ca, -sr * ca}, {0, sr, cr}}, uvY: 1}
	mesh := meleeWeaponMeshes.get(weaponBodyModels[a.weapon], buildMeleeWeaponMesh)
	r.drawCameraVolume(dst, ox+w*.5+h*.024, oy+h*.17, h*.132, alpha, pose, r.weaponBodyShader, &r.weaponMaterialOpts, mesh, [3]int{255, 255, 255}, 0, true)
}

// Every part of a cast object scales about the same camera-space anchor.
func (r *Renderer) castOverlayViewport(a buffFxAnim) (x, y, w, h float64) {
	fullW, fullH := float64(r.game.worldWidth()), float64(r.game.worldHeight())
	w, h = fullW*a.screenScale(), fullH*a.screenScale()
	return (fullW - w) * .5, (fullH - h) * .46, w, h
}

func (r *Renderer) drawCastOverlayPass(dst *ebiten.Image, a buffFxAnim, pass float32) {
	x, y, w, h := r.castOverlayViewport(a)
	r.drawCastOverlayRegion(dst, a, pass, x, y, w, h)
}

func (r *Renderer) drawCastOverlayRegion(dst *ebiten.Image, a buffFxAnim, pass float32, x, y, width, height float64) {
	s := &r.castOverlay
	if s.shader == nil {
		var err error
		s.shader, err = ebiten.NewShader([]byte(shaders.Source("cast_overlay.kage")))
		if err != nil {
			panic(err)
		}
		s.opts.Uniforms = map[string]any{"Phase": s.phase[:], "Style": s.style[:], "Viewport": s.viewport[:], "Origin": s.origin[:], "Pass": s.pass[:]}
	}
	w, h := float32(width), float32(height)
	s.phase[0] = float32(a.age) / float32(a.totalFrames())
	s.pass[0] = pass
	s.style[0] = castOverlayStyles[a.overlay]
	s.viewport = [2]float32{float32(w), float32(h)}
	s.origin = [2]float32{float32(x), float32(y)}
	s.opts.Images[0] = r.ensureFireNoise()
	x0, y0, x1, y1 := float32(0), float32(0), float32(w), float32(h)
	if a.overlay == "fire_aura" || a.overlay == "vampiric_weapon" {
		// Match the flame's bounded domain instead of shading two full screens.
		x0, x1 = float32(w)*.5-float32(h)*.26, float32(w)*.5+float32(h)*.26
		y0, y1 = float32(h)*.06, float32(h)*.86
	}
	for i, p := range [4][2]float32{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		s.vertices[i] = ebiten.Vertex{DstX: p[0] + float32(x), DstY: p[1] + float32(y), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	// The cast flourish belongs to the party camera and stops at the world/HUD
	// boundary. It neither reads the screen back nor creates an intermediate image.
	dst.DrawTrianglesShader(s.vertices[:], castOverlayIndices[:], s.shader, &s.opts)
}

var castOverlayIndices = [6]uint16{0, 1, 2, 1, 3, 2}

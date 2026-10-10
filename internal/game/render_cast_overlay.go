package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/shaders"
)

// Shared dispatch for validation and rendering. The numbers select material
// branches in cast_overlay.kage, not spell IDs or gameplay behavior.
var castOverlayStyles = map[string]float32{"darkness": 0, "town_portal": 1}

type castOverlayRenderer struct {
	shader   *ebiten.Shader
	opts     ebiten.DrawTrianglesShaderOptions
	vertices [4]ebiten.Vertex
	phase    [1]float32
	style    [1]float32
	viewport [2]float32
}

func (r *Renderer) drawCastOverlay(dst *ebiten.Image, a buffFxAnim) {
	s := &r.castOverlay
	if s.shader == nil {
		var err error
		s.shader, err = ebiten.NewShader([]byte(shaders.Source("cast_overlay.kage")))
		if err != nil {
			panic(err)
		}
		s.opts.Uniforms = map[string]any{"Phase": s.phase[:], "Style": s.style[:], "Viewport": s.viewport[:]}
	}
	w, h := r.game.worldWidth(), r.game.worldHeight()
	s.phase[0] = float32(a.age) / float32(a.totalFrames())
	s.style[0] = castOverlayStyles[a.overlay]
	s.viewport = [2]float32{float32(w), float32(h)}
	s.opts.Images[0] = r.ensureFireNoise()
	for i, p := range [4][2]float32{{0, 0}, {float32(w), 0}, {0, float32(h)}, {float32(w), float32(h)}} {
		s.vertices[i] = ebiten.Vertex{DstX: p[0], DstY: p[1], ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	// The cast flourish belongs to the party camera and stops at the world/HUD
	// boundary. It neither reads the screen back nor creates an intermediate image.
	dst.DrawTrianglesShader(s.vertices[:], castOverlayIndices[:], s.shader, &s.opts)
}

var castOverlayIndices = [6]uint16{0, 1, 2, 1, 3, 2}

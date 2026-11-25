package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/monster"
)

// goldMaterial is the gold response of one drawn frame (gold_reflection.kageinc).
// The zero value draws the paint as it is.
type goldMaterial struct {
	strength float32 // authored gold_reflection, 0..1
	phase    float32 // 0..1: how far the gleam has run across the body
	centre   float32 // the body's centre column on the destination
	width    float32 // the body's width there
}

func monsterGoldStrength(key string) float32 {
	if monster.MonsterConfig == nil {
		return 0
	}
	return monster.MonsterConfig.Monsters[key].GoldReflection
}

// monsterGleam advances a gold monster's gleam for this frame and returns its
// phase in passes across the body (graphics.gold_reflection in config.yaml
// paces the walk). x, y is where the body is drawn, without
// the hit rattle. The bearing is taken from the camera as drawn, less the
// shake: the logical camera jumps a whole tile while a turn-based step is
// still gliding, and the gleam would jump with it.
func (r *Renderer) monsterGleam(m *monster.Monster3D, x, y float64) float64 {
	if monsterGoldStrength(m.Key) <= 0 {
		return 0
	}
	tile := float64(r.game.config.GetTileSize())
	// A blink or a pull across the map is not a walk.
	if step := math.Hypot(x-m.GleamX, y-m.GleamY); m.GleamSeen && step < tile {
		m.GleamTravel += step
	}
	m.GleamX, m.GleamY, m.GleamSeen = x, y, true
	pose := r.game.cameraPose()
	pose.x -= r.game.screenShakeOffsetX
	pose.y -= r.game.screenShakeOffsetY
	walked := m.GleamTravel / tile / r.game.config.Graphics.GoldReflection.TilesPerPass
	m.GleamPhase = math.Atan2(y-pose.y, x-pose.x)/math.Pi + walked + r.monsterStrikeProgress(m)
	return m.GleamPhase
}

// mobGold is the gold of the mob frame f drawn at s.
func (r *Renderer) mobGold(s UnifiedSpriteRenderData, f mobFrame) goldMaterial {
	strength := monsterGoldStrength(f.key)
	if strength <= 0 {
		return goldMaterial{}
	}
	return goldMaterial{strength: strength, phase: float32(f.gleam - math.Floor(f.gleam)),
		centre: float32(s.screenXF), width: float32(s.sizeF)}
}

func (m goldMaterial) uniform(dst []float32) {
	dst[0], dst[1], dst[2], dst[3] = m.strength, m.phase, m.centre, 0
	if m.width > 0 {
		dst[3] = 1 / m.width
	}
}

// setStandeeGold writes the slab's gold into a reused options value.
func setStandeeGold(opts *ebiten.DrawTrianglesShaderOptions, gold goldMaterial) {
	if opts.Uniforms == nil {
		opts.Uniforms = make(map[string]any)
	}
	values, ok := opts.Uniforms["GoldReflection"].([]float32)
	if !ok {
		values = make([]float32, 4)
		opts.Uniforms["GoldReflection"] = values
	}
	gold.uniform(values)
}

// goldSprite is one shaded copy of a source frame.
type goldSprite struct {
	image    *ebiten.Image
	strength float32
	mirrored bool
}

// goldSpriteFrame returns src as gold for the paths that draw a plain image
// (flat billboards, UI portraits): each source frame is shaded once per
// strength and side, with the gleam at phase zero, and evicted with its
// source. A frame the caller draws mirrored is shaded lit from its right, so
// the light stays on the left.
func (r *Renderer) goldSpriteFrame(src *ebiten.Image, strength float32, mirrored bool) *ebiten.Image {
	if src == nil || strength <= 0 || compileShader(&r.goldSpriteShader, "gold_sprite.kage") != nil {
		return src
	}
	for _, cached := range r.goldSprites[src] {
		if cached.strength == strength && cached.mirrored == mirrored {
			return cached.image
		}
	}
	lit := float32(1)
	if mirrored {
		lit = -1
	}
	size := src.Bounds().Size()
	frame := ebiten.NewImage(size.X, size.Y)
	values := make([]float32, 4)
	goldMaterial{strength: strength}.uniform(values)
	opts := &ebiten.DrawRectShaderOptions{
		Blend:    ebiten.BlendCopy,
		Uniforms: map[string]any{"GoldReflection": values, "LitSide": lit},
		Images:   [4]*ebiten.Image{src},
	}
	frame.DrawRectShader(size.X, size.Y, r.goldSpriteShader, opts)
	if r.goldSprites == nil {
		r.goldSprites = make(map[*ebiten.Image][]goldSprite)
	}
	r.goldSprites[src] = append(r.goldSprites[src], goldSprite{image: frame, strength: strength, mirrored: mirrored})
	return frame
}

func (r *Renderer) forgetGoldSprite(src *ebiten.Image) {
	for _, gold := range r.goldSprites[src] {
		r.queueRenderRelease(gold.image)
	}
	delete(r.goldSprites, src)
}

// spriteGoldStrength is the gold of the monster drawn with a sprite, for
// screens that know a monster only by its picture. The table is built once
// per monster catalog.
func (r *Renderer) spriteGoldStrength(sprite string) float32 {
	catalog := monster.MonsterConfig
	if catalog == nil {
		return 0
	}
	if r.goldBySpriteOf != catalog {
		r.goldBySprite = make(map[string]float32)
		for _, def := range catalog.Monsters {
			if def.GoldReflection > 0 {
				r.goldBySprite[def.Sprite] = max(r.goldBySprite[def.Sprite], def.GoldReflection)
			}
		}
		r.goldBySpriteOf = catalog
	}
	return r.goldBySprite[sprite]
}

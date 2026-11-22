package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// The live flying anchor and corpse use the same projection. Gravity moves the
// anchor down to the ground over FallSeconds; fading only starts after landing.
func monsterFlyingBottom(horizon, groundBottom, size float64) float64 {
	// Keep small flyers centered on the horizon, but never let a large
	// sprite's lower edge sink beneath its projected ground contact.
	return math.Min(horizon+size/2, (horizon+groundBottom)/2)
}

func (g *MMGame) corpseBottom(c *monsterCorpse, depth, groundBottom, size float64) float64 {
	age := float64(g.frameCount-c.started) / float64(g.config.GetTPS())
	t := math.Max(0, math.Min(1, age/g.monsterDeathSettings().FallSeconds))
	a := monsterVisualAnchor{sizeTiles: c.sizeTiles, heightTiles: c.arborealHeight, flying: c.flying}
	return a.bottomDuringFall(g, depth, groundBottom, size, t)
}

func (r *Renderer) collectMonsterCorpses(sprites []UnifiedSpriteRenderData, camX, camY, dirX, dirY, viewDistSq float64) []UnifiedSpriteRenderData {
	for i := range r.game.monsterCorpses {
		c := &r.game.monsterCorpses[i]
		distance, depth, ok := cullAndProject(c.x, c.y, camX, camY, dirX, dirY, 0, viewDistSq)
		if !ok {
			continue
		}
		anim := r.game.sprites.GetAnimation(c.spriteName, c.animation)
		if anim == nil || len(anim.Frames) == 0 {
			continue
		}
		frame, opacity := r.game.corpseFrameAndOpacity(c)
		if opacity <= 0 {
			continue
		}
		sx, bottom, size, visible := r.game.renderHelper.CalculateMonsterSpriteMetricsF(c.x, c.y, distance, c.sizeTiles)
		if !visible {
			continue
		}
		bottom = r.game.corpseBottom(c, depth, bottom, size)
		sprites = append(sprites, UnifiedSpriteRenderData{spriteType: SpriteTypeMonsterCorpse,
			screenX: int(sx), screenY: int(bottom - size), spriteSize: int(size), screenXF: sx, bottomF: bottom, sizeF: size,
			depthPerp: depth, distance: distance, sprite: anim.Frames[min(frame, len(anim.Frames)-1)], corpse: c})
	}
	return sprites
}

func (r *Renderer) drawMonsterCorpse(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if s.corpse == nil || s.sprite == nil || !r.spriteDepthBufferVisible(s) {
		return
	}
	c := s.corpse
	_, alpha := r.game.corpseFrameAndOpacity(c)
	b := float32(r.calculateBrightnessWithTorchLight(c.x, c.y, s.distance))
	r.drawMob(screen, s, mobFrame{sprite: s.sprite, key: c.key, standee: r.game.config.Graphics.Standee.Enabled,
		x: c.x, y: c.y, yaw: c.yaw, left: s.screenXF - s.sizeF/2, top: s.bottomF - s.sizeF,
		mirror: c.mirror, rr: b, gg: b, bb: b, fade: 1 - alpha})
}

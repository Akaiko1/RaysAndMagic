package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// The live flying anchor and corpse use the same projection. Gravity moves the
// anchor down to the ground over FallSeconds; fading only starts after landing.
func monsterFlyingBottom(screenHeight int, groundBottom, size float64) float64 {
	horizon := float64(screenHeight) / 2
	// Keep small flyers centered on the horizon, but never let a large
	// sprite's lower edge sink beneath its projected ground contact.
	return math.Min(horizon+size/2, (horizon+groundBottom)/2)
}

func (g *MMGame) corpseBottom(c *monsterCorpse, groundBottom, size float64) float64 {
	if !c.flying && c.arborealHeight <= 0 {
		return groundBottom
	}
	age := float64(g.frameCount-c.started) / float64(g.config.GetTPS())
	t := math.Max(0, math.Min(1, age/g.monsterDeathSettings().FallSeconds))
	airBottom := monsterFlyingBottom(g.config.GetScreenHeight(), groundBottom, size)
	if c.arborealHeight > 0 {
		pixelsPerTile := 2 * (groundBottom - float64(g.config.GetScreenHeight())/2)
		airBottom = arborealBottom(groundBottom, pixelsPerTile, c.arborealHeight)
	}
	return airBottom + (groundBottom-airBottom)*t*t
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
		bottom = r.game.corpseBottom(c, bottom, size)
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
	brightness := float32(r.calculateBrightnessWithTorchLight(c.x, c.y, s.distance))
	rr, gg, bb := brightness, brightness, brightness
	if c.tintR != 0 || c.tintG != 0 || c.tintB != 0 {
		rr *= c.tintR
		gg *= c.tintG
		bb *= c.tintB
	}
	if r.game.config.Graphics.Standee.Enabled {
		key := makeStandeeCoreKey(r.prefixedStandeeKeyName("mob", c.key), s.sprite, true)
		slab, ok := r.prepareStandeeSlab(s.sprite, key, c.x, c.y, c.yaw, s.depthPerp, s.sizeF, s.bottomF,
			rr, gg, bb, false, c.mirror, 0, r.standeeSurfaces[:0])
		if ok {
			// Fade a single visible face: translucent wood shells would accumulate
			// alpha and keep the body opaque until the last instant.
			slab.firstSurface = len(slab.surfaces) - 1
			slab.fade = 1 - alpha
			r.drawStandeeSlabColumns(screen, slab, -1, -1)
		}
		r.standeeSurfaces = slab.surfaces[:0]
		return
	}
	sx, sy := s.sizeF/float64(s.sprite.Bounds().Dx()), s.sizeF/float64(s.sprite.Bounds().Dy())
	left := s.screenXF - s.sizeF/2
	if c.mirror {
		sx = -sx
		left += s.sizeF
	}
	opts := r.scaledWorldSpriteOpts(sx, sy)
	opts.GeoM.Translate(left, s.bottomF-s.sizeF)
	opts.ColorScale.Scale(rr, gg, bb, alpha)
	screen.DrawImage(s.sprite, opts)
}

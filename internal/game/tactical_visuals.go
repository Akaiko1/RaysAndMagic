package game

import (
	"image/color"
	"math/bits"

	"github.com/hajimehoshi/ebiten/v2"
)

var tacticalBadgeBackground = color.RGBA{12, 18, 22, 230}

// One reticle links the ready hero badge to the designated enemy.
func drawTacticalReticle(screen *ebiten.Image, cx, cy, radius float32) {
	uiFillCircle(screen, cx, cy, radius, tacticalBadgeBackground, true)
	tint := color.RGBA{235, 193, 92, 255}
	uiStrokeCircle(screen, cx, cy, radius*0.56, 1.2, tint, true)
	arm := radius * 0.78
	uiStrokeLine(screen, cx-arm, cy, cx+arm, cy, 1, tint, true)
	uiStrokeLine(screen, cx, cy-arm, cx, cy+arm, 1, tint, true)
}

// A bound former enemy wears two linked rings: allegiance at a glance, without
// covering the actor the way a full-body overlay would.
func drawBindBadge(screen *ebiten.Image, cx, cy, radius float32) {
	uiFillCircle(screen, cx, cy, radius, tacticalBadgeBackground, true)
	tint := color.RGBA{116, 224, 208, 255}
	link := radius * 0.32
	uiStrokeCircle(screen, cx-link*0.7, cy, link, 1.4, tint, true)
	uiStrokeCircle(screen, cx+link*0.7, cy, link, 1.4, tint, true)
}

// statusBadgeRenderers draw the head-badge motifs, keyed by statusBadgeVisuals.
var statusBadgeRenderers = map[statusVisuals]func(dst *ebiten.Image, cx, cy, radius float32){
	visualBind: drawBindBadge,
}

// drawMonsterHeadBadges rides the small state markers in one row above the
// head: the Sniper's designation reticle, then each badge motif in catalog order.
func (r *Renderer) drawMonsterHeadBadges(screen *ebiten.Image, s UnifiedSpriteRenderData, screenY int, badges statusVisuals) {
	designated := r.game.designationBonus(s.monster) > 0
	badges &= statusBadgeVisuals
	if !designated && badges == 0 {
		return
	}
	// Hide the markers when their anchor is behind terrain, including a partly
	// visible monster peeking from a doorway. The normal sprite pass owns FOV.
	if s.screenX < 0 || s.screenX >= len(r.game.depthBuffer) || s.depthPerp >= r.game.depthBuffer[s.screenX] {
		return
	}
	radius := min(14, max(9, s.spriteSize/12))
	y := float32(max(radius+3, screenY-radius-5))
	pitch := float32(2*radius + 4)
	count := bits.OnesCount32(uint32(badges))
	if designated {
		count++
	}
	x := float32(s.screenX) - pitch*float32(count-1)/2
	if designated {
		drawTacticalReticle(screen, x, y, float32(radius))
		x += pitch
	}
	for _, e := range statusVisualCatalog {
		if badges&e.flag != 0 {
			statusBadgeRenderers[e.flag](screen, x, y, float32(radius))
			x += pitch
		}
	}
}

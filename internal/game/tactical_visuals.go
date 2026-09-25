package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// One reticle links the ready hero badge to the designated enemy.
func drawTacticalReticle(screen *ebiten.Image, cx, cy, radius float32) {
	vector.FillCircle(screen, cx, cy, radius, color.RGBA{12, 18, 22, 230}, true)
	tint := color.RGBA{235, 193, 92, 255}
	vector.StrokeCircle(screen, cx, cy, radius*0.56, 1.2, tint, true)
	arm := radius * 0.78
	vector.StrokeLine(screen, cx-arm, cy, cx+arm, cy, 1, tint, true)
	vector.StrokeLine(screen, cx, cy-arm, cx, cy+arm, 1, tint, true)
}

func (r *Renderer) drawDesignationMarker(screen *ebiten.Image, s UnifiedSpriteRenderData, screenY int) {
	if r.game.designationBonus(s.monster) <= 0 {
		return
	}
	// Hide the marker when its anchor is behind terrain, including a partly
	// visible monster peeking from a doorway. The normal sprite pass owns FOV.
	if s.screenX < 0 || s.screenX >= len(r.game.depthBuffer) || s.depthPerp >= r.game.depthBuffer[s.screenX] {
		return
	}
	radius := min(14, max(9, s.spriteSize/12))
	x := float32(s.screenX)
	y := float32(max(radius+3, screenY-radius-5))
	drawTacticalReticle(screen, x, y, float32(radius))
}

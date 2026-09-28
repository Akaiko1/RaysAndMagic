package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"ugataima/internal/character"
)

// Four 12px sigils fit the portrait's right gutter. They stop above the
// progression badges and before the stat text, including on compact cards.
func flowingStaffChargeRects(px, py, pw int, charges int) []layoutRect {
	rects := make([]layoutRect, max(0, min(4, charges)))
	for i := range rects {
		rects[i] = layoutRect{px + pw, py - 6 + i*12, 12, 12}
	}
	return rects
}

// Like Overwatch's reticle, this is a native-size HUD glyph: a gold staff
// wrapped in a turquoise current, with a dark backing for daylight contrast.
func drawFlowingStaffCharge(screen *ebiten.Image, box layoutRect) {
	x, y := float32(box.x+6), float32(box.y+6)
	vector.FillCircle(screen, x, y, 5.8, color.NRGBA{9, 23, 27, 242}, true)
	vector.StrokeCircle(screen, x, y, 5.1, .8, color.NRGBA{73, 160, 155, 255}, true)
	vector.StrokeLine(screen, x-2.2, y+3.6, x+1.7, y-3, 1.7, color.NRGBA{228, 190, 111, 255}, true)
	vector.FillCircle(screen, x+1.7, y-3, 1.3, color.NRGBA{203, 255, 239, 255}, true)
	vector.StrokeLine(screen, x-3.6, y-.6, x-1.2, y+.7, 1.1, color.NRGBA{86, 238, 209, 255}, true)
	vector.StrokeLine(screen, x-1.2, y+.7, x+3.6, y-1.1, 1.1, color.NRGBA{86, 238, 209, 255}, true)
}

func (ui *UISystem) drawFlowingStaffCharges(screen *ebiten.Image, member *character.MMCharacter, px, py, pw, mouseX, mouseY int) {
	charges := member.FlowingStaffCharges()
	for _, box := range flowingStaffChargeRects(px, py, pw, charges) {
		drawFlowingStaffCharge(screen, box)
		if isMouseHoveringBox(mouseX, mouseY, box.x, box.y, box.right(), box.bottom()) {
			ui.queueTooltip([]string{"Flowing Staff", fmt.Sprintf("%d charges | +%d%% staff damage", charges, character.FlowingStaffPct(member.SkillTier(character.SkillFlowingStaff))), "Guaranteed critical hit. No expiry.", "Move one tile to refill."}, mouseX+12, mouseY+8)
		}
	}
}

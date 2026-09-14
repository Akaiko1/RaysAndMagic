package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Inspection reads the completed scene's projected monsters. It never changes
// selection, consumes clicks, or exposes entities hidden behind a modal/wall.
func (ui *UISystem) queueMonsterInspection(mx, my int) {
	if ui.tooltipLines != nil || ui.topModalLayer() != modalLayerNone || ui.game.gameOver ||
		ui.game.gameLoop == nil || ui.game.gameLoop.renderer == nil || my >= gameplayViewportBottom(ui.game) {
		return
	}
	r := ui.game.gameLoop.renderer
	for i := len(r.unifiedSprites) - 1; i >= 0; i-- {
		s := r.unifiedSprites[i]
		m := s.monster
		if m == nil || !m.IsAlive() || m.IsChampion() || !r.spriteDepthBufferVisible(s) {
			continue
		}
		top := clampMonsterSpriteTopToGameplayViewport(ui.game, s.bottomF-s.sizeF, s.sizeF)
		if float64(mx) < s.screenXF-s.sizeF/2 || float64(mx) >= s.screenXF+s.sizeF/2 || float64(my) < top || float64(my) >= top+s.sizeF {
			continue
		}
		if mx >= 0 && mx < len(ui.game.depthBuffer) && s.depthPerp >= ui.game.depthBuffer[mx] {
			continue
		}
		lines := []string{m.Name, fmt.Sprintf("HP: %d/%d", m.HitPoints, m.MaxHitPoints), fmt.Sprintf("Damage: %d-%d", m.DamageMin, m.DamageMax)}
		colors := []color.Color{color.White, color.White, color.White}
		if m.TrueDamage > 0 {
			lines = append(lines, fmt.Sprintf("True damage: %d", m.TrueDamage))
			colors = append(colors, color.White)
		}
		for _, line := range ui.game.MonsterCombatEffectLines(m) {
			lines = append(lines, line.Text)
			if line.School != "" {
				colors = append(colors, SchoolColor(line.School))
			} else {
				colors = append(colors, color.White)
			}
		}
		ui.queueTooltip(lines, mx+16, my+16)
		ui.tooltipColors = colors
		return
	}
}

func (ui *UISystem) queueHoveredMonsterInspection() {
	x, y := ebiten.CursorPosition()
	ui.queueMonsterInspection(x, y)
}

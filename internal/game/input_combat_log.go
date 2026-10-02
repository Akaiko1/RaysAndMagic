package game

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func (ih *InputHandler) handleCombatLogOpenInput() bool {
	x, y, w, h := combatMessageArea(ih.game)
	clickX, clickY, ok := ih.game.leftClickPosition()
	if !ok || w == 0 || clickX < x || clickX >= x+w || clickY < y || clickY >= y+h {
		return false
	}

	ih.game.consumeLeftClick()
	now := time.Now().UnixMilli()
	if withinDoubleClickWindow(now, ih.game.lastCombatLogClick) {
		ih.game.combatLogOpen = true
		ih.game.combatLogScroll = 0
		ih.game.lastCombatLogClick = 0
	} else {
		ih.game.lastCombatLogClick = now
	}
	return true
}

func (ih *InputHandler) handleCombatLogInput() {
	g := ih.game
	if ih.keys.Consume(ebiten.KeyEscape) {
		g.combatLogOpen = false
		return
	}

	if !g.combatLogOpen {
		return
	}
	_, wheelY := ebiten.Wheel()
	switch {
	case wheelY > 0 || ih.keys.Consume(ebiten.KeyUp):
		g.combatLogScroll += 3
	case wheelY < 0 || ih.keys.Consume(ebiten.KeyDown):
		g.combatLogScroll -= 3
	}

	g.clampCombatLogScroll()
}

func (ih *InputHandler) handleCombatLogMouseInput() {
	g := ih.game
	l := makeCombatLogLayout(g)
	if g.consumeLeftClickIn(l.close.x, l.close.y, l.close.right(), l.close.bottom()) {
		g.combatLogOpen = false
		return
	}
	if g.consumeLeftClickIn(l.up.x, l.up.y, l.up.right(), l.up.bottom()) {
		g.combatLogScroll += 3
	}
	if g.consumeLeftClickIn(l.down.x, l.down.y, l.down.right(), l.down.bottom()) {
		g.combatLogScroll -= 3
	}

	g.clampCombatLogScroll()
	g.consumeLeftClick()
}

func (g *MMGame) clampCombatLogScroll() {
	maxScroll := len(g.combatLogHistory) - 1
	if maxScroll < 0 {
		maxScroll = 0
	}
	if g.combatLogScroll < 0 {
		g.combatLogScroll = 0
	}
	if g.combatLogScroll > maxScroll {
		g.combatLogScroll = maxScroll
	}
}

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

	x, y, w, h := combatLogPanelLayout(g)
	closeX, closeY := x+w-30, y+8
	if g.consumeLeftClickIn(closeX, closeY, closeX+20, closeY+20) {
		g.combatLogOpen = false
		return
	}

	_, wheelY := ebiten.Wheel()
	switch {
	case wheelY > 0 || ih.keys.Consume(ebiten.KeyUp):
		g.combatLogScroll += 3
	case wheelY < 0 || ih.keys.Consume(ebiten.KeyDown):
		g.combatLogScroll -= 3
	}

	contentY, contentH := y+54, h-88
	buttonX := x + w - 36
	if g.consumeLeftClickIn(buttonX, contentY+8, buttonX+22, contentY+30) {
		g.combatLogScroll += 3
	}
	if g.consumeLeftClickIn(buttonX, contentY+contentH-30, buttonX+22, contentY+contentH-8) {
		g.combatLogScroll -= 3
	}

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

	g.consumeLeftClick()
}

package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Identification uses the displayed alpha-aware monster picker. It owns a
// paused modal, but registers no full-world input rectangle over those pixels.
func (g *MMGame) canIdentifyMonster(target *monster.Monster3D, def spells.SpellDefinition) bool {
	if target == nil || !target.IsAlive() || def.Mechanic == nil || g.world == nil {
		return false
	}
	x, y := g.combat.logicalCameraXY()
	for _, m := range g.world.Monsters {
		if m == target {
			return Distance(x, y, m.X, m.Y) <= def.Mechanic.RadiusTiles*g.config.GetTileSize() && g.combat.attackLineClear(x, y, m.X, m.Y)
		}
	}
	return false
}

func (ih *InputHandler) handleIdentifyInput() {
	g := ih.game
	if !g.identifyCursorActive() {
		return
	}
	x, y, clicked := g.leftClickPosition()
	if !clicked {
		return
	}
	target := g.monsterAtScreen(x, y)
	g.consumeLeftClick()
	g.confirmIdentify(target)
}

func (g *MMGame) confirmIdentify(target *monster.Monster3D) {
	if !g.identifySelecting {
		return
	}
	caster, id := g.townPortalCaster, g.townPortalSpell
	idx := g.combat.findCharacterIndex(caster)
	if caster == nil || !g.canSpendCombatAction(idx) {
		return
	}
	def, err := spells.GetSpellDefinitionByID(id)
	if err != nil || !g.canIdentifyMonster(target, def) {
		g.AddCombatMessage(uitext.Text("identify.invalid"))
		return
	}
	g.identifyConfirming = target
	cast := g.commitPickedSpell(caster, id, def)
	g.identifyConfirming = nil
	if cast {
		g.bumpModalContentRev()
	}
}

func (g *MMGame) identifyCursorActive() bool {
	return g.identifySelecting && topModalLayerFor(g, false) == modalLayerTownPortal && g.appScreen == AppScreenInGame
}

func (g *MMGame) syncIdentifyCursor() {
	mode := ebiten.CursorModeVisible
	if g.identifyCursorActive() || g.allySpellCursorActive() {
		mode = ebiten.CursorModeHidden
	}
	if ebiten.CursorMode() != mode {
		ebiten.SetCursorMode(mode)
	}
}

func (ui *UISystem) drawIdentifyTargeting(screen *ebiten.Image) {
	ui.registerDisplayedModalMouse((*InputHandler).handleIdentifyInput, modalLayerTownPortal)
	w := ui.game.config.GetScreenWidth()
	label := uitext.Text("identify.choose")
	box := layoutRect{(w - min(600, w-32)) / 2, 18, min(600, w-32), 38}
	drawFilledRect(screen, box.x, box.y, box.w, box.h, beaconInk.panel)
	drawRectBorder(screen, box.x, box.y, box.w, box.h, 1, beaconInk.border)
	drawCenteredTextWithShadow(screen, label, box.x+12, box.y, box.w-24, box.h, beaconInk.glow)
}

func (ui *UISystem) drawIdentifyCursor(screen *ebiten.Image) {
	g := ui.game
	if g.allySpellCursorActive() {
		x, y := uiCursorPosition()
		def, ok := config.GetSpellDefinition(string(g.townPortalSpell))
		if ok && def.Graphics != nil {
			const size = 28
			drawImageScaled(screen, g.sprites.GetSprite(def.Graphics.TargetCursor), x-size/2, y-size/2, size, size)
		}
		return
	}
	if !g.identifyCursorActive() {
		return
	}
	x, y := uiCursorPosition()
	cx, cy := float32(x), float32(y)
	ink := color.RGBA{145, 170, 190, 255}
	def, err := spells.GetSpellDefinitionByID(g.townPortalSpell)
	if err == nil && g.canIdentifyMonster(g.monsterAtScreen(x, y), def) {
		ink = beaconInk.glow
	}
	// Lens centre is the actual click point; the handle never offsets picking.
	uiStrokeLine(screen, cx+8, cy+8, cx+19, cy+19, 7, color.RGBA{9, 13, 19, 255}, true)
	uiStrokeLine(screen, cx+8, cy+8, cx+18, cy+18, 4, ink, true)
	uiFillCircle(screen, cx, cy, 11, color.RGBA{9, 15, 22, 130}, true)
	uiStrokeCircle(screen, cx, cy, 11, 4, color.RGBA{9, 13, 19, 255}, true)
	uiStrokeCircle(screen, cx, cy, 10, 1.8, ink, true)
	uiStrokeCircle(screen, cx, cy, 7.6, .7, color.RGBA{100, 171, 205, 200}, true)
	uiStrokeLine(screen, cx-4, cy-4, cx-1, cy-6, 1.3, color.RGBA{221, 244, 252, 220}, true)
}

func (g *MMGame) allySpellCursorActive() bool {
	return g.allySpellSelecting && topModalLayerFor(g, false) == modalLayerTownPortal && g.appScreen == AppScreenInGame
}

func (g *MMGame) validAllySpellTarget(index int) bool {
	if index < 0 || index >= len(g.party.Members) {
		return false
	}
	c := g.party.Members[index]
	return c.IsLiving()
}

func (ih *InputHandler) handleAllySpellInput() {
	g := ih.game
	if !g.allySpellCursorActive() {
		return
	}
	if g.consumeRightClickMatching(func(queuedClick) bool { return true }) {
		g.cancelTownPortalPicker()
		return
	}
	x, y, clicked := g.leftClickPosition()
	if clicked {
		g.consumeLeftClick()
		g.confirmAllySpell(ih.game.partyMemberAt(x, y))
	}
}

func (g *MMGame) confirmAllySpell(index int) {
	if !g.allySpellSelecting || !g.validAllySpellTarget(index) {
		return
	}
	caster, id := g.townPortalCaster, g.townPortalSpell
	casterIndex := g.combat.findCharacterIndex(caster)
	if caster == nil || !g.canSpendCombatAction(casterIndex) {
		return
	}
	def, err := spells.GetSpellDefinitionByID(id)
	if err != nil || !def.TargetsAlly() {
		return
	}
	g.combat.spellTarget = g.party.Members[index]
	cast := g.commitPickedSpell(caster, id, def)
	g.combat.spellTarget = nil
	if cast {
		g.cancelTownPortalPicker()
		g.bumpModalContentRev()
	}
}

func (ui *UISystem) drawAllySpellTargeting(screen *ebiten.Image) {
	ui.registerDisplayedModalMouse((*InputHandler).handleAllySpellInput, modalLayerTownPortal)
	g := ui.game
	d, _ := spells.GetSpellDefinitionByID(g.townPortalSpell)
	w := g.config.GetScreenWidth()
	box := layoutRect{16, gameplayViewportBottom(g) - 44, w - 32, 32}
	drawFilledRect(screen, box.x, box.y, box.w, box.h, beaconInk.panel)
	drawRectBorder(screen, box.x, box.y, box.w, box.h, 1, beaconInk.border)
	drawCenteredTextWithShadow(screen, uitext.Text("advanced.ally_target", d.Name), box.x+8, box.y, box.w-16, box.h, SchoolColor(d.School))
}

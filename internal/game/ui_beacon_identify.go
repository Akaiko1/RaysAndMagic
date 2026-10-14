package game

import (
	"image"
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

var beaconInk = struct{ panel, card, border, glow, muted color.RGBA }{
	color.RGBA{16, 22, 30, 255}, color.RGBA{23, 33, 44, 255}, color.RGBA{78, 105, 130, 255}, color.RGBA{143, 216, 242, 255}, color.RGBA{157, 171, 190, 255},
}

func (ui *UISystem) advancedMagicPanel(screen *ebiten.Image, width, height int, title string) layoutRect {
	w, h := ui.game.config.GetScreenWidth(), ui.game.config.GetScreenHeight()
	p := layoutRect{0, 0, min(width, w-32), min(height, h-32)}
	p.x, p.y = (w-p.w)/2, (h-p.h)/2
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 170})
	drawFilledRect(screen, p.x, p.y, p.w, p.h, beaconInk.panel)
	drawRectBorder(screen, p.x, p.y, p.w, p.h, 1, beaconInk.border)
	ui.drawCornerDecor(screen, frameSilver, p.x, p.y, p.w, p.h, 15)
	drawCenteredTextWithShadow(screen, title, p.x+40, p.y+16, p.w-80, 28, beaconInk.glow)
	ui.drawPopupCloseButton(screen, p.right()-36, p.y+12, 24, ui.topModalLayer() == modalLayerTownPortal, ui.game.cancelTownPortalPicker)
	return p
}

func (ui *UISystem) advancedMagicButton(screen *ebiten.Image, r layoutRect, label string, enabled bool, action func()) {
	g := ui.game
	live := ui.topModalLayer() == modalLayerTownPortal
	mx, my := uiCursorPosition()
	ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, live && enabled && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()))
	ink := beaconInk.glow
	if !enabled {
		ink = beaconInk.muted
	}
	drawCenteredTextWithShadow(screen, label, r.x, r.y, r.w, r.h, ink)
	ui.onDisplayedInput(uiCommandClick, r, func() {
		if live && enabled && g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
			action()
		}
	})
}

func (ui *UISystem) drawBeaconPicker(screen *ebiten.Image) {
	g := ui.game
	n := g.beaconSlots()
	if n <= 0 {
		return
	}
	p := ui.advancedMagicPanel(screen, 680, 128+n*84, spellName(g.townPortalSpell))
	d, _ := spells.GetSpellDefinitionByID(g.townPortalSpell)
	drawCenteredTextWithShadow(screen, uitext.Text("beacon.cost", g.townPortalCaster.Name, g.townPortalCaster.MeditatedSPCost(d.SpellPointsCost)), p.x+24, p.y+48, p.w-48, 24, beaconInk.muted)
	rowH := (p.h - 128) / n
	_, canBind := g.currentBeacon()
	for i := 0; i < n; i++ {
		r := layoutRect{p.x + 20, p.y + 80 + i*rowH, p.w - 40, rowH - 8}
		drawFilledRect(screen, r.x, r.y, r.w, r.h, beaconInk.card)
		if i == g.beaconSelection {
			drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, beaconInk.glow)
		}
		icon := min(44, r.h-16)
		if g.sprites != nil {
			drawImageScaled(screen, g.sprites.GetSprite(spellTooltipIconName(g.townPortalSpell)), r.x+8, r.y+(r.h-icon)/2, icon, icon)
		}
		x := r.x + icon + 20
		textW := r.w - icon - 224
		drawUITextColored(screen, uitext.Text("beacon.slot", i+1), x, r.y+10, beaconInk.glow)
		label := uitext.Text("advanced.beacon_empty")
		canReturn := false
		if i < len(g.townPortalCaster.SpellState.Beacons) {
			b := g.townPortalCaster.SpellState.Beacons[i]
			if b.MapKey != "" {
				ts := float64(g.config.GetTileSize())
				label = uitext.Text("advanced.beacon_here", guildMapName(b.MapKey), TileIndex(b.X, ts), TileIndex(b.Y, ts))
				_, err := g.beaconRequest(b)
				canReturn = err == nil
			}
		}
		ui.drawWrappedTextWithOverflow(screen, label, layoutRect{x, r.y + 32, textW, r.h - 36}, 2, 16)
		ui.advancedMagicButton(screen, layoutRect{r.right() - 192, r.y + (r.h-32)/2, 88, 32}, uitext.Text("advanced.beacon_bind"), canBind, func() { g.confirmBeacon(i, true) })
		ui.advancedMagicButton(screen, layoutRect{r.right() - 96, r.y + (r.h-32)/2, 88, 32}, uitext.Text("advanced.beacon_return"), canReturn, func() { g.confirmBeacon(i, false) })
	}
	drawCenteredTextWithShadow(screen, uitext.Text("beacon.controls"), p.x+20, p.bottom()-36, p.w-40, 24, beaconInk.muted)
}

func (ih *InputHandler) handleBeaconInput() {
	g := ih.game
	n := g.beaconSlots()
	if n <= 0 {
		return
	}
	if ih.keys.Consume(ebiten.KeyArrowDown) {
		g.beaconSelection = (g.beaconSelection + 1) % n
	}
	if ih.keys.Consume(ebiten.KeyArrowUp) {
		g.beaconSelection = (g.beaconSelection + n - 1) % n
	}
	if ih.keys.Consume(ebiten.KeyB) {
		g.confirmBeacon(g.beaconSelection, true)
		return
	}
	if ih.keys.Consume(ebiten.KeyEnter) {
		g.confirmBeacon(g.beaconSelection, false)
	}
}

func (ui *UISystem) drawIdentifyMonster(screen *ebiten.Image) {
	g, m := ui.game, ui.game.identifiedMonster
	if m == nil {
		return
	}
	p := ui.advancedMagicPanel(screen, 760, 540, m.Name)
	portrait := layoutRect{p.x + 24, p.y + 72, min(220, p.w/3), p.h - 128}
	drawFilledRect(screen, portrait.x, portrait.y, portrait.w, portrait.h, beaconInk.card)
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		if sprite, _ := g.gameLoop.renderer.getMonsterSprite(m); sprite != nil {
			b := sprite.Bounds()
			scale := min(float64(portrait.w-24)/float64(b.Dx()), float64(portrait.h-24)/float64(b.Dy()))
			w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
			drawImageScaled(screen, sprite, portrait.x+(portrait.w-w)/2, portrait.y+(portrait.h-h)/2, w, h)
		}
	}
	area := layoutRect{portrait.right() + 24, portrait.y, p.right() - portrait.right() - 56, portrait.h}
	type row struct {
		text   string
		header bool
		y      int
	}
	var rows []row
	contentH := 0
	for _, line := range g.identifyMonsterLines(m) {
		if line.text == "" {
			contentH += identifySpacerH
			continue
		}
		for _, text := range wrapUIText(line.text, area.w-12) {
			rows = append(rows, row{text, line.header, contentH})
			contentH += identifyRowH
		}
	}
	offset := min(g.identifyScroll, max(0, contentH-area.h))
	clip := uiClip(screen, image.Rect(area.x, area.y, area.right(), area.bottom()))
	for _, r := range rows {
		y := area.y + r.y - offset
		if y+identifyRowH >= area.y && y < area.bottom() {
			ink := beaconInk.muted
			if r.header {
				ink = beaconInk.glow
			}
			drawUITextColored(clip, r.text, area.x, y, ink)
		}
	}
	setScroll := func(v int) { g.identifyScroll = max(0, min(v, contentH-area.h)) }
	ui.drawScrollbar(screen, "identify_monster", layoutRect{area.right() + 8, area.y, 4, area.h}, offset, contentH, ui.topModalLayer() == modalLayerTownPortal, setScroll)
	ui.onDisplayedInput(uiCommandPointer, p, func() {
		if ui.topModalLayer() != modalLayerTownPortal {
			return
		}
		_, wheel := pointerWheel()
		if wheel != 0 {
			setScroll(offset - int(wheel*40))
		}
	})
	ui.advancedMagicButton(screen, layoutRect{p.x + (p.w-112)/2, p.bottom() - 44, 112, 28}, uitext.Text("advanced.identify_close"), true, g.cancelTownPortalPicker)
}

// Identify report rows: a text row, and the gap that opens each section.
const identifyRowH, identifySpacerH = 20, 8

// identifyLine is one report line: a section header, body text, or (empty
// text) the gap before a section.
type identifyLine struct {
	text   string
	header bool
}

// identifyMonsterLines is the Identify Monster report before wrapping: vitals,
// defenses, then the editor card's sections with the live region's Elemental
// Attack.
func (g *MMGame) identifyMonsterLines(m *monster.Monster3D) []identifyLine {
	body := func(text string) identifyLine { return identifyLine{text: text} }
	section := func(title string) []identifyLine { return []identifyLine{{}, {text: title, header: true}} }
	lines := []identifyLine{body(uitext.Text("identify.vitals", m.Level, m.HitPoints, m.MaxHitPoints)), body(uitext.Text("identify.armor", m.EffectiveArmorClass(), m.PerfectDodge)), body(uitext.Text("identify.attack", m.DamageMin, m.DamageMax, m.TrueDamage))}
	lines = append(lines, section(uitext.Text("identify.defenses"))...)
	keys := make([]string, 0, len(m.Resistances))
	for school := range m.Resistances {
		keys = append(keys, string(school))
	}
	sort.Strings(keys)
	for _, school := range keys {
		lines = append(lines, body(uitext.Text("identify.resistance", formatSchoolName(school), m.Resistances[monster.DamageType(school)])))
	}
	if len(keys) == 0 {
		lines = append(lines, body(uitext.Text("identify.no_resists")))
	}
	if monster.MonsterConfig != nil {
		if template, err := monster.MonsterConfig.GetMonsterByKey(m.Key); err == nil {
			// The live monster's abilities: adventure scaling rewrites them.
			def := *template
			def.Abilities = m.Abilities
			for _, s := range []struct {
				title string
				lines []monster.EffectLine
			}{
				{uitext.Text("identify.section_attack"), def.AttackLines(g.monsterEffectContext(m))},
				{uitext.Text("identify.section_abilities"), def.AbilityLines()},
				{uitext.Text("identify.section_behavior"), def.BehaviorLines()},
			} {
				if len(s.lines) == 0 {
					continue
				}
				lines = append(lines, section(s.title)...)
				for _, line := range s.lines {
					lines = append(lines, body(line.Text))
				}
			}
		}
	}
	return lines
}

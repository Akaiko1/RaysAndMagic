package game

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func hasRareBook(c *character.MMCharacter) bool {
	return c != nil && (c.Class == character.ClassAlchemist || c.Class == character.ClassWayfarer)
}

// Both source panels are 1536x1024. The common clear field is x=192..1344,
// y=96..944: it excludes the crest, hanging props, cloth and bottom border.
// Keep one source scale; the title and the entire quick-slot unit stay inside it.
type rareBookLayout struct {
	frame, inner, header, list, detail, quick, hint layoutRect
	rows, rowHeight                                 int
	pager, actions, message                         layoutRect
}

func computeRareBookLayout(content layoutRect, alchemist bool) rareBookLayout {
	frameH := min(720, content.h, content.w*2/3)
	frameH -= frameH % 2
	frameW := frameH * 3 / 2
	frame := layoutRect{content.x + (content.w-frameW)/2, content.y + (content.h-frameH)/2, frameW, frameH}
	source := func(x, y, w, h int) layoutRect {
		return layoutRect{frame.x + x*frameW/1536, frame.y + y*frameH/1024, w * frameW / 1536, h * frameH / 1024}
	}
	inner := source(192, 96, 1152, 848)
	header := layoutRect{inner.x, inner.y, inner.w, 22}
	hint := layoutRect{inner.x, inner.bottom() - 16, inner.w, 16}
	quick := layoutRect{frame.x + (frame.w-300)/2, hint.y - 8 - 64, 300, 64}
	body := layoutRect{inner.x, header.bottom() + 8, inner.w, quick.y - quickSlotTabLabelSpace - 12 - header.bottom() - 8}
	left := body.w * 36 / 100
	l := rareBookLayout{frame: frame, inner: inner, header: header, hint: hint, quick: quick,
		list: layoutRect{body.x, body.y, left, body.h}, detail: layoutRect{body.x + left + 12, body.y, body.w - left - 12, body.h}}
	l.rowHeight = 54
	l.rows = max(1, (l.list.h-52)/l.rowHeight)
	if !alchemist {
		l.rows = 5
		l.rowHeight = min(64, (l.list.h-24)/5)
	}
	l.pager = layoutRect{l.list.x + 8, l.list.bottom() - 30, l.list.w - 16, 24}
	l.actions = layoutRect{l.detail.x + 12, l.detail.bottom() - 70, l.detail.w - 24, 30}
	l.message = layoutRect{l.actions.x, l.actions.bottom() + 8, l.actions.w, 28}
	return l
}
func (l rareBookLayout) row(i int) layoutRect {
	return layoutRect{l.list.x + 8, l.list.y + 24 + i*l.rowHeight, l.list.w - 16, l.rowHeight - 6}
}
func (l rareBookLayout) auto(i int) layoutRect {
	r := l.row(i)
	return layoutRect{r.x + 54, r.y + 28, r.w - 60, 20}
}

func (ui *UISystem) rareBookButton(screen *ebiten.Image, r layoutRect, label string, enabled bool, action func()) {
	mx, my := ebiten.CursorPosition()
	hover := isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
	ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, enabled && hover)
	drawCenteredDebugText(screen, truncateName(label, max(1, (r.w-12)/6)), r.x+3, r.y, r.w-6, r.h)
	if !enabled {
		drawFilledRect(screen, r.x+2, r.y+2, r.w-4, r.h-4, color.RGBA{0, 0, 0, 105})
	}
	ui.rareBookClick(r, enabled, action)
}
func (ui *UISystem) rareBookClick(r layoutRect, enabled bool, action func()) {
	if enabled && !ui.modalLayerOwnsInput() {
		ui.onDisplayedInput(uiCommandClick, r, func() {
			if ui.game.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
				action()
			}
		})
	}
}
func rareBookPanel(screen *ebiten.Image, r layoutRect) {
	drawFilledRect(screen, r.x, r.y, r.w, r.h, color.RGBA{13, 18, 20, 220})
	drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, color.RGBA{125, 112, 78, 200})
}
func (ui *UISystem) rareBookText(screen *ebiten.Image, text string, r layoutRect) {
	ui.drawWrappedTextWithOverflow(screen, text, r, max(1, r.h/16), 16)
}
func (ui *UISystem) rareBookItem(screen *ebiten.Image, it items.Item, r layoutRect, c *character.MMCharacter) {
	ui.drawInventoryItemIcon(screen, it, r.x, r.y, r.w, r.h, 2, true)
	mx, my := ebiten.CursorPosition()
	if isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()) {
		ui.queueItemTooltip(strings.Split(GetItemTooltip(it, c, ui.game.combat, tooltipDetailHeld()), "\n"), it, c, mx+16, my+8)
	}
}
func (ui *UISystem) rareBookRow(screen *ebiten.Image, r layoutRect, it items.Item, subtitle string, selected bool, action func()) {
	if selected {
		drawFilledRect(screen, r.x, r.y, r.w, r.h, color.RGBA{83, 75, 45, 210})
	} else {
		drawFilledRect(screen, r.x, r.y, r.w, r.h, color.RGBA{28, 35, 34, 200})
	}
	if selected {
		drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, color.RGBA{205, 173, 95, 255})
	}
	c := ui.game.party.Members[ui.game.selectedChar]
	size := min(46, r.h-4)
	ui.rareBookItem(screen, it, layoutRect{r.x + 3, r.y + (r.h-size)/2, size, size}, c)
	ui.rareBookText(screen, strings.TrimPrefix(it.Name, "Brewed "), layoutRect{r.x + 54, r.y + 5, r.w - 60, 16})
	if subtitle != "" {
		ui.rareBookText(screen, subtitle, layoutRect{r.x + 54, r.y + 26, r.w - 60, 16})
	}
	if action != nil {
		ui.rareBookClick(r, true, action)
	}
}
func (ui *UISystem) drawRareClassBook(screen *ebiten.Image, content layoutRect) bool {
	g := ui.game
	c := g.party.Members[g.selectedChar]
	if !hasRareBook(c) {
		return false
	}
	alchemist := c.Class == character.ClassAlchemist
	title, art := "Alchemist's Workbench", "alchemy_workbench"
	if !alchemist {
		title, art = "Pilgrim Techniques", "pilgrim_technique_panel"
	}
	l := computeRareBookLayout(content, alchemist)
	if g.sprites.HasSprite(art) {
		drawImageScaled(screen, g.sprites.GetSprite(art), l.frame.x, l.frame.y, l.frame.w, l.frame.h)
	}
	drawCenteredDebugText(screen, c.Name+" - "+title, l.header.x, l.header.y, l.header.w, l.header.h)
	rareBookPanel(screen, l.list)
	rareBookPanel(screen, l.detail)
	if alchemist {
		ui.drawAlchemyWorkbench(screen, c, l)
	} else {
		ui.drawPilgrimTechniques(screen, c, l)
	}
	ui.drawTabQuickSlotBar(screen, l.quick.x, l.quick.y, l.quick.w)
	hint := "Click materials to select | Scroll list | Enter: brew"
	if !alchemist {
		hint = "Drag icons to quick slots | Enter: use | V: Fold | Shift+V: Return"
	}
	drawCenteredDebugText(screen, hint, l.hint.x, l.hint.y, l.hint.w, l.hint.h)
	return true
}
func (ui *UISystem) drawAlchemyWorkbench(screen *ebiten.Image, c *character.MMCharacter, l rareBookLayout) {
	g := ui.game
	if config.GlobalAlchemy == nil || len(config.GlobalAlchemy.Recipes) == 0 {
		return
	}
	rs := config.GlobalAlchemy.Recipes
	g.selectedRare = max(0, min(len(rs)-1, g.selectedRare))
	page := g.selectedRare / l.rows
	pages := pageCount(len(rs), l.rows)
	drawDebugText(screen, "RECIPES", l.list.x+12, l.list.y+6)
	for i := page * l.rows; i < min(len(rs), (page+1)*l.rows); i++ {
		it, _ := items.TryCreateItemFromYAML(rs[i].Output)
		idx := i
		ui.rareBookRow(screen, l.row(i-page*l.rows), it, fmt.Sprintf("%d per batch", character.AlchemyYield(c.SkillTier(character.SkillAlchemy), rs[i].Family)), i == g.selectedRare, func() { g.selectedRare = idx; g.rareBookMessage = "" })
	}
	ui.rareBookButton(screen, layoutRect{l.pager.x, l.pager.y, 32, l.pager.h}, "<", page > 0, func() { g.selectedRare = (page - 1) * l.rows; g.rareBookMessage = "" })
	drawCenteredDebugText(screen, fmt.Sprintf("%d / %d", page+1, pages), l.pager.x+36, l.pager.y, l.pager.w-72, l.pager.h)
	ui.rareBookButton(screen, layoutRect{l.pager.right() - 32, l.pager.y, 32, l.pager.h}, ">", page+1 < pages, func() { g.selectedRare = (page + 1) * l.rows; g.rareBookMessage = "" })
	if g.brewAnimation != nil {
		ui.drawAlchemyBrewAnimation(screen, l.detail)
		return
	}
	ui.drawAlchemyMaterials(screen, c, l, &rs[g.selectedRare])
}
func (ui *UISystem) drawPilgrimTechniques(screen *ebiten.Image, c *character.MMCharacter, l rareBookLayout) {
	g := ui.game
	if config.GlobalTechniques == nil || len(config.GlobalTechniques.Techniques) == 0 {
		return
	}
	ds := config.GlobalTechniques.Techniques
	g.selectedRare = max(0, min(len(ds)-1, g.selectedRare))
	drawDebugText(screen, "TECHNIQUES", l.list.x+12, l.list.y+6)
	for i := range ds {
		d := &ds[i]
		idx := i
		it, _ := config.TechniqueItem(d.Key)
		r := l.row(i)
		subtitle := "Manual | No action cost"
		if c.Level < d.Level {
			subtitle = fmt.Sprintf("Unlocks at level %d", d.Level)
		}
		if d.Automatic {
			subtitle = ""
		}
		// Only the name and icon select/drag. Autocast owns the lower text row.
		selectRect := r
		if d.Automatic {
			selectRect.h = 28
		}
		ui.rareBookRow(screen, r, it, subtitle, i == g.selectedRare, nil)
		ui.rareBookClick(selectRect, true, func() { g.selectedRare = idx })
		if c.Level >= d.Level {
			ui.quickRareActionDragSource(it, selectRect)
			iconRect := layoutRect{r.x + 3, r.y + 6, 46, 46}
			ui.rareBookClick(iconRect, true, func() { g.selectedRare = idx })
			ui.quickRareActionDragSource(it, iconRect)
		}
		if d.Automatic {
			mark := "[ ] Autocast"
			if c.RareClass.Automatic[d.Key] {
				mark = "[x] Autocast"
			}
			if c.Level < d.Level {
				mark = fmt.Sprintf("Autocast at level %d", d.Level)
			}
			key := d.Key
			ui.rareBookButton(screen, l.auto(i), mark, c.Level >= d.Level, func() {
				if c.RareClass.Automatic == nil {
					c.RareClass.Automatic = map[string]bool{}
				}
				c.RareClass.Automatic[key] = !c.RareClass.Automatic[key]
			})
		}
	}
	d := &ds[g.selectedRare]
	it, _ := config.TechniqueItem(d.Key)
	x, y, w := l.detail.x+12, l.detail.y+12, l.detail.w-24
	ui.rareBookItem(screen, it, layoutRect{x, y, 64, 64}, c)
	ui.rareBookText(screen, d.Name, layoutRect{x + 76, y + 4, w - 76, 32})
	action := "1 action"
	if d.FreeStep {
		action = "No action cost"
	}
	drawDebugText(screen, fmt.Sprintf("%d SP | %s", g.techniqueSPCost(c, d), action), x+76, y+32)
	drawDebugText(screen, fmt.Sprintf("Unlock: level %d", d.Level), x+76, y+50)
	ui.rareBookText(screen, techniqueMagnitude(c, d, g.config.GetTPS()), layoutRect{x, y + 76, w, 32})
	descBottom := l.actions.y - 12
	if d.Automatic {
		descBottom -= 76
	}
	ui.rareBookText(screen, d.Description, layoutRect{x, y + 100, w, descBottom - (y + 100)})
	if d.Automatic {
		r := layoutRect{x, l.actions.y - 84, w, 70}
		drawFilledRect(screen, r.x, r.y, r.w, r.h, color.RGBA{30, 45, 45, 255})
		drawDebugText(screen, "AUTOCAST CONDITION", r.x+8, r.y+6)
		ui.rareBookText(screen, d.Trigger, layoutRect{r.x + 8, r.y + 26, r.w - 16, 40})
	}
	half := (l.actions.w - 8) / 2
	ui.rareBookButton(screen, layoutRect{l.actions.x, l.actions.y, half, 30}, "Equip quick action", c.Level >= d.Level, func() { c.Equipment[items.SlotSpell] = it })
	ui.rareBookButton(screen, layoutRect{l.actions.x + half + 8, l.actions.y, l.actions.w - half - 8, 30}, "Use technique", c.Level >= d.Level, func() { g.useTechniqueFromBook(d.Key) })
	message := "Select a technique. Hover its icon for details."
	if d.Automatic {
		message = "Autocast is independent of your quick action."
	}
	if c.Level < d.Level {
		message = fmt.Sprintf("Learned automatically at level %d.", d.Level)
	}
	ui.rareBookText(screen, message, l.message)
}
func techniqueMagnitude(c *character.MMCharacter, d *config.TechniqueDefinition, tps int) string {
	tier := c.SkillTier(character.SkillTranslocation)
	switch d.Key {
	case "fold_step":
		return fmt.Sprintf("Range %d-%d tiles | Anchor %ds", d.MinRange, config.TierValue(d.Range, tier), config.TierValue(d.Duration, tier))
	case "phase_veil":
		return fmt.Sprintf("Dodge +%d%% | %ds", config.TierValue(d.Power, tier), config.TierValue(d.Duration, tier))
	case "quickening":
		return fmt.Sprintf("RT recovery -%d%% | TB pool +%d | %ds", config.TierValue(d.Power, tier), config.TierValue(d.TBPower, tier), config.TierValue(d.Duration, tier))
	case "purify":
		return "All curable afflictions | No healing or revival"
	case "return_step":
		return fmt.Sprintf("Any distance, same map | Anchor %ds", c.RareClass.Anchor.Frames/max(1, tps))
	}
	return ""
}

func (ih *InputHandler) handleRareBookInput() bool {
	g := ih.game
	c := g.party.Members[g.selectedChar]
	if !hasRareBook(c) {
		return false
	}
	n := 0
	if c.Class == character.ClassAlchemist && config.GlobalAlchemy != nil {
		n = len(config.GlobalAlchemy.Recipes)
	} else if config.GlobalTechniques != nil {
		n = len(config.GlobalTechniques.Techniques)
	}
	if n == 0 {
		return true
	}
	g.selectedRare = max(0, min(n-1, g.selectedRare))
	if ih.keys.Consume(ebiten.KeyUp) {
		g.selectedRare = (g.selectedRare + n - 1) % n
	}
	if ih.keys.Consume(ebiten.KeyDown) {
		g.selectedRare = (g.selectedRare + 1) % n
	}
	if ih.keys.Consume(ebiten.KeyEnter) {
		if c.Class == character.ClassAlchemist {
			g.brewSelectedRecipe()
		} else {
			g.useTechniqueFromBook(config.GlobalTechniques.Techniques[g.selectedRare].Key)
		}
	}
	return true
}

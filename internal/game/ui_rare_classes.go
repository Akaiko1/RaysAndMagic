package game

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
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
	rows, rowHeight, rowGap                         int
	rowsTop                                         int  // y of the first row
	listTitle                                       bool // the list heading fits above the rows
	pager                                           layoutRect
}

// Row text: a name line over a second line (subtitle or autocast toggle).
// A row of rareBookRowRoomyH keeps the authored offsets; a shorter one centres
// the two lines as a block down to rareBookRowMinH.
const (
	rareBookLineH     = uiTextCharHeight
	rareBookToggleH   = 20
	rareBookRowRoomyH = 28 + rareBookToggleH
	rareBookRowMinH   = rareBookLineH + 2 + rareBookToggleH + 2
	rareBookListTitle = 24
)

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
	l.listTitle, l.rowsTop, l.rowGap = true, l.list.y+rareBookListTitle, 6
	l.rowHeight = 54
	l.rows = max(1, (l.list.h-52)/l.rowHeight)
	if alchemist {
		l.rowsTop += 48
		l.rows = max(1, (l.list.bottom()-32-l.rowsTop)/l.rowHeight)
	}
	if !alchemist {
		// Every technique is on one page. A page too short for the authored
		// rows tightens the gaps, then drops the list heading.
		l.rows = 1
		if config.GlobalTechniques != nil {
			l.rows = max(1, len(config.GlobalTechniques.Techniques))
		}
		l.rowHeight = min(64, (l.list.bottom()-l.rowsTop)/l.rows)
		if l.rowHeight-l.rowGap < rareBookRowRoomyH {
			l.rowGap = 4
			l.rowHeight = min(64, (l.list.bottom()-4-l.rowsTop)/l.rows)
		}
		if l.rowHeight-l.rowGap < rareBookRowMinH {
			l.listTitle, l.rowsTop = false, l.list.y+6
			l.rowHeight = min(64, (l.list.bottom()-4-l.rowsTop)/l.rows)
		}
	}
	l.pager = layoutRect{l.list.x + 8, l.list.bottom() - 30, l.list.w - 16, 24}
	return l
}
func (l rareBookLayout) row(i int) layoutRect {
	return layoutRect{l.list.x + 8, l.rowsTop + i*l.rowHeight, l.list.w - 16, l.rowHeight - l.rowGap}
}
func (l rareBookLayout) auto(i int) layoutRect {
	return rareBookRowParts(l.row(i)).second
}

type rareBookRowPart struct{ icon, name, subtitle, second layoutRect }

// rareBookRowParts splits a list row into its icon, the name line, and the
// second line (the toggle box, with the subtitle's text line inside it).
func rareBookRowParts(r layoutRect) rareBookRowPart {
	size := min(46, r.h-4)
	var p rareBookRowPart
	p.icon = layoutRect{r.x + 3, r.y + (r.h-size)/2, size, size}
	x := r.x + 54 // clear of the largest icon, whatever this row's size
	w := r.right() - 6 - x
	if r.h >= rareBookRowRoomyH {
		p.name = layoutRect{x, r.y + 5, w, rareBookLineH}
		p.subtitle = layoutRect{x, r.y + 26, w, rareBookLineH}
		p.second = layoutRect{x, r.y + 28, w, rareBookToggleH}
		return p
	}
	gap := max(2, min(4, r.h-rareBookRowMinH+2))
	p.name = layoutRect{x, r.y + (r.h-rareBookLineH-gap-rareBookToggleH)/2, w, rareBookLineH}
	p.second = layoutRect{x, p.name.bottom() + gap, w, rareBookToggleH}
	p.subtitle = layoutRect{x, p.second.y + (rareBookToggleH-rareBookLineH)/2, w, rareBookLineH}
	return p
}

// pilgrimDetailLayout stacks one technique top-down: the head (icon beside
// the name, cost and unlock lines), the magnitude, then the description; the
// autocast condition sits whole above the buttons.
type pilgrimDetailLayout struct {
	icon, name, cost, unlock, magnitude, desc, auto, trigger, actions, message layoutRect
	autoHeadY, descLines                                                       int
	roomy                                                                      bool
}

// pilgrimDetailSpacing is one scheme of the technique details: offsets and
// gaps, top to bottom. messageH 0 wraps the note under the buttons and keeps
// it only while the description still fits whole.
type pilgrimDetailSpacing struct {
	top, icon, headH, nameY, costY, unlockY    int
	headGap, textGap                           int
	descAuto, autoActions, descActions         int
	autoHeadY, autoTextY, autoMinText, autoPad int
	actionsH, messageGap, messageH, bottom     int
}

// The authored look; kept wherever the whole description fits it.
var pilgrimRoomySpacing = pilgrimDetailSpacing{top: 12, icon: 64, headH: 64, nameY: 4, costY: 32, unlockY: 50,
	headGap: 12, textGap: 8, descAuto: 4, autoActions: 14, descActions: 12,
	autoHeadY: 6, autoTextY: 26, autoMinText: 40, autoPad: 4,
	actionsH: 30, messageGap: 8, messageH: 28, bottom: 4}

// The short-page scheme: a smaller head spread over its icon, tight gaps.
func pilgrimCompactSpacing(detail layoutRect) pilgrimDetailSpacing {
	icon := max(48, min(64, detail.h/6))
	headH := max(icon, 3*rareBookLineH+4)
	pitch := (headH - rareBookLineH) / 2
	return pilgrimDetailSpacing{top: 8, icon: icon, headH: headH, costY: pitch, unlockY: 2 * pitch,
		headGap: 4, textGap: 4, descAuto: 6, autoActions: 6, descActions: 6,
		autoHeadY: 4, autoTextY: 22, autoPad: 4, actionsH: 26, messageGap: 6, bottom: 8}
}

// makePilgrimDetailLayout keeps the roomy scheme while the whole description
// fits it. On a shorter page the compact scheme takes over, and only the
// description gives way there (it clips and hover shows the rest).
func makePilgrimDetailLayout(detail layoutRect, magnitude, description, trigger, message string) pilgrimDetailLayout {
	p := layoutPilgrimDetail(detail, pilgrimRoomySpacing, magnitude, description, trigger, message)
	if p.desc.h >= p.descLines*rareBookLineH {
		p.roomy = true
		return p
	}
	return layoutPilgrimDetail(detail, pilgrimCompactSpacing(detail), magnitude, description, trigger, message)
}

func layoutPilgrimDetail(detail layoutRect, s pilgrimDetailSpacing, magnitude, description, trigger, message string) pilgrimDetailLayout {
	lines := func(text string, w int) int { return len(wrapUIText(text, w)) * rareBookLineH }
	x, w := detail.x+12, detail.w-24
	var p pilgrimDetailLayout
	p.icon = layoutRect{x, detail.y + s.top, s.icon, s.icon}
	tx := p.icon.right() + 12
	p.name = layoutRect{tx, p.icon.y + s.nameY, x + w - tx, rareBookLineH}
	p.cost = layoutRect{tx, p.icon.y + s.costY, p.name.w, rareBookLineH}
	p.unlock = layoutRect{tx, p.icon.y + s.unlockY, p.name.w, rareBookLineH}
	y := p.icon.y + s.headH + s.headGap
	if magnitude != "" {
		p.magnitude = layoutRect{x, y, w, lines(magnitude, w)}
		y = p.magnitude.bottom() + s.textGap
	}
	bottom := detail.bottom() - s.bottom
	if s.messageH > 0 && message != "" {
		p.message = layoutRect{x, bottom - s.messageH, w, s.messageH}
		bottom = p.message.y - s.messageGap
	}
	p.actions = layoutRect{x, bottom - s.actionsH, w, s.actionsH}
	above := p.actions.y - s.descActions
	if trigger != "" {
		th := max(lines(trigger, w-16), s.autoMinText)
		h := s.autoTextY + th + s.autoPad
		p.auto = layoutRect{x, p.actions.y - s.autoActions - h, w, h}
		p.trigger = layoutRect{x + 8, p.auto.y + s.autoTextY, w - 16, th}
		p.autoHeadY = p.auto.y + s.autoHeadY
		above = p.auto.y - s.descAuto
	}
	p.descLines = len(wrapUIText(description, w))
	if mh := lines(message, w); s.messageH == 0 && message != "" && above-mh-s.messageGap-y >= p.descLines*rareBookLineH {
		p.message = layoutRect{x, detail.bottom() - s.bottom - mh, w, mh}
		shift := mh + s.messageGap
		p.actions.y -= shift
		if trigger != "" {
			p.auto.y -= shift
			p.trigger.y -= shift
			p.autoHeadY -= shift
		}
		above -= shift
	}
	p.desc = layoutRect{x, y, w, max(0, above-y)}
	return p
}

func (ui *UISystem) rareBookButton(screen *ebiten.Image, r layoutRect, label string, enabled bool, action func()) {
	mx, my := uiCursorPosition()
	hover := isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
	ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, enabled && hover)
	drawCenteredUIText(screen, label, r.x+3, r.y, r.w-6, r.h)
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
	mx, my := uiCursorPosition()
	if isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()) {
		ui.queueItemTooltip(GetItemTooltipRows(it, c, ui.game.combat, tooltipDetailHeld()), it, c, mx+16, my+8)
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
	parts := rareBookRowParts(r)
	ui.rareBookItem(screen, it, parts.icon, c)
	ui.rareBookText(screen, strings.TrimPrefix(it.Name, "Brewed "), parts.name)
	if subtitle != "" {
		ui.rareBookText(screen, subtitle, parts.subtitle)
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
	drawCenteredUIText(screen, c.Name+" - "+title, l.header.x, l.header.y, l.header.w, l.header.h)
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
	drawCenteredUIText(screen, hint, l.hint.x, l.hint.y, l.hint.w, l.hint.h)
	return true
}
func (ui *UISystem) drawAlchemyWorkbench(screen *ebiten.Image, c *character.MMCharacter, l rareBookLayout) {
	g := ui.game
	if config.GlobalAlchemy == nil || len(config.GlobalAlchemy.Recipes) == 0 {
		return
	}
	rs := config.GlobalAlchemy.Recipes
	visible := g.visibleAlchemyRecipes()
	pos := g.selectVisibleAlchemyRecipe(visible, 0)
	page := max(0, pos) / l.rows
	pages := pageCount(len(visible), l.rows)
	drawUIText(screen, fmt.Sprintf("RECIPES (%d)", len(visible)), l.list.x+12, l.list.y+6)
	category := g.alchemyRecipeFilter
	if category == "" {
		category = "All"
	}
	element := g.alchemyElementFilter
	if element == "" {
		element = "All"
	}
	row := layoutRect{l.list.x + 8, l.list.y + 24, l.list.w - 16, 20}
	ui.rareBookButton(screen, row, "Type: "+category, true, func() {
		g.alchemyRecipeFilter = nextAlchemyFilter(g.alchemyRecipeFilter, alchemyRecipeFilters)
		g.rareBookMessage = ""
	})
	row.y += 24
	row.w = (row.w - 4) / 2
	ui.rareBookButton(screen, row, element, true, func() {
		g.alchemyElementFilter = nextAlchemyFilter(g.alchemyElementFilter, alchemyElementFilters)
		g.rareBookMessage = ""
	})
	row.x += row.w + 4
	ready := "Stock: All"
	if g.alchemyBrewableOnly {
		ready = "Brewable"
	}
	ui.rareBookButton(screen, row, ready, true, func() { g.alchemyBrewableOnly = !g.alchemyBrewableOnly; g.rareBookMessage = "" })
	for i := page * l.rows; i < min(len(visible), (page+1)*l.rows); i++ {
		idx := visible[i]
		it, _ := items.TryCreateItemFromYAML(rs[idx].Output)
		ui.rareBookRow(screen, l.row(i-page*l.rows), it, fmt.Sprintf("%d per batch", character.AlchemyYield(c.SkillTier(character.SkillAlchemy), rs[idx].Family)), idx == g.selectedRare, func() { g.selectedRare = idx; g.rareBookMessage = "" })
	}
	ui.rareBookButton(screen, layoutRect{l.pager.x, l.pager.y, 32, l.pager.h}, "<", page > 0, func() { g.selectedRare = visible[(page-1)*l.rows]; g.rareBookMessage = "" })
	drawCenteredUIText(screen, fmt.Sprintf("%d / %d", page+1, pages), l.pager.x+36, l.pager.y, l.pager.w-72, l.pager.h)
	ui.rareBookButton(screen, layoutRect{l.pager.right() - 32, l.pager.y, 32, l.pager.h}, ">", page+1 < pages, func() { g.selectedRare = visible[(page+1)*l.rows]; g.rareBookMessage = "" })
	if g.brewAnimation != nil {
		ui.drawAlchemyBrewAnimation(screen, l.detail)
		return
	}
	if len(visible) == 0 {
		ui.rareBookText(screen, "No matching recipes. Change the filters or gather the selected ingredients.", l.detail)
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
	if l.listTitle {
		drawUIText(screen, "TECHNIQUES", l.list.x+12, l.list.y+6)
	}
	for i := range ds {
		d := &ds[i]
		idx := i
		it, _ := config.TechniqueItem(d.Key)
		r := l.row(i)
		parts := rareBookRowParts(r)
		icon, second := parts.icon, parts.second
		subtitle := fittingUIForm(parts.subtitle.w, "Manual | No action cost", "No action cost")
		if c.Level < d.Level {
			subtitle = fittingUIForm(parts.subtitle.w, fmt.Sprintf("Unlocks at level %d", d.Level), fmt.Sprintf("Level %d", d.Level))
		}
		if d.Automatic {
			subtitle = ""
		}
		// Only the name and icon select/drag. Autocast owns the lower text row.
		selectRect := r
		if d.Automatic {
			selectRect.h = second.y - r.y
		}
		ui.rareBookRow(screen, r, it, subtitle, i == g.selectedRare, nil)
		ui.rareBookClick(selectRect, true, func() { g.selectedRare = idx })
		if c.Level >= d.Level {
			ui.quickRareActionDragSource(it, selectRect)
			ui.rareBookClick(icon, true, func() { g.selectedRare = idx })
			ui.quickRareActionDragSource(it, icon)
		}
		if d.Automatic {
			mark := "[ ] Autocast"
			if c.RareClass.Automatic[d.Key] {
				mark = "[x] Autocast"
			}
			if c.Level < d.Level {
				mark = fittingUIForm(second.w-6, fmt.Sprintf("Autocast at level %d", d.Level), fmt.Sprintf("At level %d", d.Level))
			}
			key := d.Key
			ui.rareBookButton(screen, second, mark, c.Level >= d.Level, func() {
				if c.RareClass.Automatic == nil {
					c.RareClass.Automatic = map[string]bool{}
				}
				c.RareClass.Automatic[key] = !c.RareClass.Automatic[key]
			})
		}
	}
	d := &ds[g.selectedRare]
	it, _ := config.TechniqueItem(d.Key)
	message := "Select a technique. Hover its icon for details."
	if d.Automatic {
		message = "Autocast is independent of your quick action."
	}
	if c.Level < d.Level {
		message = fmt.Sprintf("Learned automatically at level %d.", d.Level)
	}
	trigger := ""
	if d.Automatic {
		trigger = d.Trigger
	}
	p := makePilgrimDetailLayout(l.detail, techniqueMagnitude(c, d, g.config.GetTPS()), d.Description, trigger, message)
	ui.rareBookItem(screen, it, p.icon, c)
	ui.rareBookText(screen, d.Name, p.name)
	action := "1 action"
	if d.FreeStep {
		action = "No action cost"
	}
	ui.rareBookText(screen, fmt.Sprintf("%d SP | %s", g.techniqueSPCost(c, d), action), p.cost)
	ui.rareBookText(screen, fmt.Sprintf("Unlock: level %d", d.Level), p.unlock)
	ui.rareBookText(screen, techniqueMagnitude(c, d, g.config.GetTPS()), p.magnitude)
	ui.rareBookText(screen, d.Description, p.desc)
	if d.Automatic {
		drawFilledRect(screen, p.auto.x, p.auto.y, p.auto.w, p.auto.h, color.RGBA{30, 45, 45, 255})
		drawUIText(screen, "AUTOCAST CONDITION", p.auto.x+8, p.autoHeadY)
		ui.rareBookText(screen, d.Trigger, p.trigger)
	}
	half := (p.actions.w - 8) / 2
	ui.rareBookButton(screen, layoutRect{p.actions.x, p.actions.y, half, p.actions.h}, fittingUIForm(half-6, "Equip quick action", "Equip"), c.Level >= d.Level, func() { c.Equipment[items.SlotSpell] = it })
	ui.rareBookButton(screen, layoutRect{p.actions.x + half + 8, p.actions.y, p.actions.w - half - 8, p.actions.h}, fittingUIForm(p.actions.w-half-14, "Use technique", "Use"), c.Level >= d.Level, func() { g.useTechniqueFromBook(d.Key) })
	if p.message.h > 0 {
		ui.rareBookText(screen, message, p.message)
	}
}

// techniqueMagnitude states what a technique does: the hero's own tier, or
// with no hero (catalog cards) every tier as "Novice/Expert/Master/GM".
func techniqueMagnitude(c *character.MMCharacter, d *config.TechniqueDefinition, tps int) string {
	v := func(values [4]int) string {
		tier := 0
		if c != nil {
			tier = c.SkillTier(character.SkillTranslocation)
		}
		return tierValueText(c != nil, tier, values)
	}
	switch d.Key {
	case "fold_step":
		return fmt.Sprintf("Range %d-%s tiles | Anchor %ss", d.MinRange, v(d.Range), v(d.Duration))
	case "phase_veil":
		return fmt.Sprintf("%s | %ss", uitext.Text("buff.dodge", v(d.Power)), v(d.Duration))
	case "quickening":
		return fmt.Sprintf("%s | %s | %ss", uitext.Text("buff.rt_recovery", v(d.Power)), uitext.Text("buff.tb_pool", v(d.TBPower)), v(d.Duration))
	case "purify":
		return "All curable afflictions | No healing or revival"
	case "return_step":
		if c == nil {
			return "Any distance, same map"
		}
		return fmt.Sprintf("Any distance, same map | Anchor %ds", c.RareClass.Anchor.Frames/max(1, tps))
	}
	return ""
}

// tierLadder prints a per-mastery value set, or one value when all agree.
func tierLadder(values [4]int) string {
	if tierFlat(values) {
		return strconv.Itoa(values[0])
	}
	return fmt.Sprintf("%d/%d/%d/%d", values[0], values[1], values[2], values[3])
}

// tierCountText is a counted ladder: "1 round" when flat, else "1/1/2/2 rounds".
func tierCountText(values [4]int, singular, plural string) string {
	if tierFlat(values) {
		return pluralizeCount(values[0], singular, plural)
	}
	return tierLadder(values) + " " + plural
}

func tierFlat(values [4]int) bool {
	return values[0] == values[1] && values[1] == values[2] && values[2] == values[3]
}

// tierValues evaluates one mastery-scaled value at every tier through the
// same function the effect itself uses.
func tierValues(at func(tier int) int) [4]int {
	var out [4]int
	for t := range out {
		out[t] = at(t)
	}
	return out
}

// tierValueText is the hero's own tier, or with no hero (catalog cards,
// editor) every tier.
func tierValueText(hero bool, tier int, values [4]int) string {
	if hero {
		return strconv.Itoa(config.TierValue(values, tier))
	}
	return tierLadder(values)
}

// tierLabel names a value line: the hero's "Current" value, a catalog
// "Base" value when every tier agrees, else the bare label over the ladder.
func tierLabel(hero bool, label string, values [4]int) string {
	switch {
	case hero:
		return "Current " + label
	case tierFlat(values):
		return "Base " + label
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

func (ih *InputHandler) handleRareBookInput() bool {
	g := ih.game
	c := g.party.Members[g.selectedChar]
	if !hasRareBook(c) {
		return false
	}
	n := 0
	if c.Class == character.ClassAlchemist && config.GlobalAlchemy != nil {
		visible := g.visibleAlchemyRecipes()
		g.selectVisibleAlchemyRecipe(visible, 0)
		if ih.keys.Consume(ebiten.KeyUp) {
			g.selectVisibleAlchemyRecipe(visible, -1)
		}
		if ih.keys.Consume(ebiten.KeyDown) {
			g.selectVisibleAlchemyRecipe(visible, 1)
		}
		if ih.keys.Consume(ebiten.KeyEnter) && len(visible) > 0 {
			g.brewSelectedRecipe()
		}
		return true
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

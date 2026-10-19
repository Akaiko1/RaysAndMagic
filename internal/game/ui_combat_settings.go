package game

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/stats"
)

// gameplayPage is the page Settings > Gameplay shows.
type gameplayPage int

const (
	gameplayPageView gameplayPage = iota
	gameplayPageBuilds
)

var gameplayPageLabels = [...]string{gameplayPageView: "View", gameplayPageBuilds: "Builds"}

// Keyboard rows: the page switch first, then the View page's rows. The Builds
// page has one row per class after the switch.
const (
	gameplayRowPage = iota
	gameplayRowView
	gameplayRowPanini
	gameplayRowAurora
	gameplayRows
)

func (g *MMGame) gameplayRowCount() int {
	if g.gameplaySettingsPage == gameplayPageBuilds {
		return 1 + len(g.autoBuildClasses())
	}
	return gameplayRows
}

// The same warm ink and bronze used by the other Settings tabs. Gold marks
// the active choice; the quiet section edge identifies keyboard focus.
var gameplayInk = struct {
	heading, text, muted, edge, focus, selected color.RGBA
}{
	color.RGBA{230, 213, 172, 255},
	color.RGBA{210, 209, 197, 255},
	color.RGBA{156, 158, 158, 255},
	color.RGBA{68, 65, 55, 255},
	color.RGBA{177, 149, 89, 255},
	color.RGBA{43, 39, 29, 255},
}

type combatSettingsText struct {
	label string
	box   layoutRect
	tint  color.RGBA
}

// The Gameplay tab's pages are text tabs on a hairline, a rank below the
// framed settings tabs; each hit box is its label plus padding.
const (
	gameplayPagesY   = 98
	gameplayPagesH   = 26
	gameplayPagesPad = 12
)

func gameplayPageRect(px, py int, page gameplayPage) layoutRect {
	x := px + 28
	for i := gameplayPage(0); i < page; i++ {
		x += uiTextWidth(gameplayPageLabels[i]) + 2*gameplayPagesPad + 4
	}
	return layoutRect{x, py + gameplayPagesY, uiTextWidth(gameplayPageLabels[page]) + 2*gameplayPagesPad, gameplayPagesH}
}

// The View page's two sections sit below the page switch.
func gameplayViewBox(px, py, panelW int) layoutRect {
	return layoutRect{px + 28, py + 132, panelW - 56, 168}
}
func gameplayAuroraBox(px, py, panelW int) layoutRect {
	return layoutRect{px + 28, py + 308, panelW - 56, 126}
}

func combatSettingsTextLayout(px, py, panelW int) []combatSettingsText {
	view, aurora := gameplayViewBox(px, py, panelW), gameplayAuroraBox(px, py, panelW)
	texts := []combatSettingsText{
		{"Field of view", layoutRect{view.x + 16, view.y + 12, 0, 0}, gameplayInk.heading},
		{"Battlefield aurora", layoutRect{aurora.x + 16, aurora.y + 12, 0, 0}, gameplayInk.heading},
		{"Show enemy reach in turn-based combat.", layoutRect{aurora.x + 16, aurora.y + 38, 0, 0}, gameplayInk.muted},
	}
	labels := [...]string{"Battlefield", "Attack reach", "Movement reach"}
	for i, label := range labels {
		texts = append(texts, combatSettingsText{label, layoutRect{px + 80 + i*((panelW-88)/3), aurora.y + 86, 0, 0}, gameplayInk.text})
	}
	return texts
}

func viewChoiceRect(px, py, panelW int, wide bool) layoutRect {
	const gap = 16
	w := (panelW - 88 - gap) / 2
	x := px + 44
	if wide {
		x += w + gap
	}
	return layoutRect{x, gameplayViewBox(px, py, panelW).y + 32, w, 120}
}

func viewChoiceTextLayout(r layoutRect, wide bool) []combatSettingsText {
	labels := [4]string{"Classic", "Original perspective", "Small creatures stay above", "the party panel."}
	if wide {
		labels = [4]string{"Wide", "Broader perspective", "", ""}
	}
	texts := []combatSettingsText{
		{labels[0], layoutRect{r.x + 16, r.y + 16, 0, 0}, gameplayInk.heading},
		{labels[1], layoutRect{r.x + 16, r.y + 48, 0, 0}, gameplayInk.text},
		{labels[2], layoutRect{r.x + 16, r.y + 76, 0, 0}, gameplayInk.muted},
		{labels[3], layoutRect{r.x + 16, r.y + 96, 0, 0}, gameplayInk.muted},
	}
	if wide {
		return texts[:2]
	}
	return texts
}

func combatOverlayChoiceRect(px, py, panelW int, enabled bool) layoutRect {
	x := px + panelW - 204
	if enabled {
		x += 80
	}
	return layoutRect{x, gameplayAuroraBox(px, py, panelW).y + 8, 72, 32}
}

func (g *MMGame) setGameplayPage(page gameplayPage) {
	if g.gameplaySettingsPage != page {
		g.gameplaySettingsPage = page
		g.audioSettingsSelection = gameplayRowPage
	}
}

func (ui *UISystem) drawCombatSettings(screen *ebiten.Image, px, py, panelW int) {
	ui.drawGameplayPageSwitch(screen, px, py, panelW)
	if ui.game.gameplaySettingsPage == gameplayPageBuilds {
		ui.drawBuildSettings(screen, px, py)
		return
	}
	g := ui.game
	// A thin section edge follows keyboard focus without competing with the
	// selected option's bronze frame and filled radio mark.
	for row, box := range []layoutRect{gameplayViewBox(px, py, panelW), gameplayAuroraBox(px, py, panelW)} {
		edge := gameplayInk.edge
		if g.audioSettingsSelection == []int{gameplayRowView, gameplayRowAurora}[row] || row == 0 && g.audioSettingsSelection == gameplayRowPanini {
			edge = gameplayInk.focus
		}
		drawRectBorder(screen, box.x, box.y, box.w, box.h, 1, edge)
	}
	for _, text := range combatSettingsTextLayout(px, py, panelW) {
		drawUITextColored(screen, text.label, text.box.x, text.box.y, text.tint)
	}
	mx, my := uiCursorPosition()
	for _, wide := range []bool{false, true} {
		r := viewChoiceRect(px, py, panelW, wide)
		chosen := g.combatPreferences.WideView == wide
		hover := ui.audioSettingsOwnsInput() && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, chosen || hover)
		if chosen {
			drawFilledRect(screen, r.x+4, r.y+4, r.w-8, r.h-8, gameplayInk.selected)
		}
		for _, text := range viewChoiceTextLayout(r, wide) {
			drawUITextColored(screen, text.label, text.box.x, text.box.y, text.tint)
		}
		cx, cy := float32(r.right()-24), float32(r.y+24)
		ink := gameplayInk.edge
		if chosen || hover {
			ink = gameplayInk.focus
		}
		uiStrokeCircle(screen, cx, cy, 6, 1, ink, true)
		if chosen {
			uiFillCircle(screen, cx, cy, 3, gameplayInk.heading, true)
		}
		if wide {
			ui.drawPaniniChoice(screen, r)
			r.h = 72
		}
		ui.settingsClick(r, func() {
			g.audioSettingsSelection = gameplayRowView
			g.setWideView(wide)
		})
	}
	for _, enabled := range []bool{false, true} {
		label := "Off"
		if enabled {
			label = "On"
		}
		r := combatOverlayChoiceRect(px, py, panelW, enabled)
		ui.drawSettingsChoice(screen, label, r, g.combatPreferences.TurnBasedOverlay == enabled)
		ui.settingsClick(r, func() {
			g.audioSettingsSelection = gameplayRowAurora
			g.setCombatOverlay(enabled)
		})
	}
	// The legend uses floor-grid and perimeter glyphs, matching what each color
	// means in play instead of suggesting that every threatened tile is filled.
	legendY := gameplayAuroraBox(px, py, panelW).y + 82
	for i, rgb := range [][3]int{combatAuraBlue, combatAuraRed, combatAuraGreen} {
		x, y := px+44+i*((panelW-88)/3), legendY
		ink := color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
		drawRectBorder(screen, x, y, 24, 24, 1, ink)
		if i == 0 {
			drawFilledRect(screen, x+8, y, 1, 24, ink)
			drawFilledRect(screen, x+16, y, 1, 24, ink)
			drawFilledRect(screen, x, y+8, 24, 1, ink)
			drawFilledRect(screen, x, y+16, 24, 1, ink)
		} else {
			drawFilledRect(screen, x+11, y+11, 2, 2, gameplayInk.muted)
		}
	}
}

// drawGameplayPageSwitch draws the pages as text tabs: the active one in
// heading ink over a gold stroke, the rest muted until hovered. The hairline
// under them takes the focus ink while the keyboard is on this row, as the
// section edges below do.
func (ui *UISystem) drawGameplayPageSwitch(screen *ebiten.Image, px, py, panelW int) {
	g := ui.game
	rule := gameplayInk.edge
	if g.audioSettingsSelection == gameplayRowPage {
		rule = gameplayInk.focus
	}
	drawFilledRect(screen, px+28, py+gameplayPagesY+gameplayPagesH-1, panelW-56, 1, rule)
	owns := ui.audioSettingsOwnsInput()
	mx, my := uiCursorPosition()
	for i, label := range gameplayPageLabels {
		page := gameplayPage(i)
		r := gameplayPageRect(px, py, page)
		active := g.gameplaySettingsPage == page
		ink := gameplayInk.muted
		switch {
		case active:
			ink = gameplayInk.heading
		case owns && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()):
			ink = gameplayInk.text
		}
		drawCenteredTextWithShadow(screen, label, r.x, r.y, r.w, r.h-4, ink)
		if active {
			drawFilledRect(screen, r.x+gameplayPagesPad/2, r.bottom()-3, r.w-gameplayPagesPad, 2, buildsBarTones.accent)
		}
		ui.settingsClick(r, func() {
			g.setGameplayPage(page)
			g.audioSettingsSelection = gameplayRowPage
		})
	}
}

func paniniChoiceRect(card layoutRect) layoutRect {
	return layoutRect{card.x + 16, card.y + 78, card.w - 32, 34}
}

func (ui *UISystem) drawPaniniChoice(screen *ebiten.Image, card layoutRect) {
	g := ui.game
	r := paniniChoiceRect(card)
	enabled := !g.combatPreferences.PaniniDisabled
	ink := gameplayInk.text
	if !g.combatPreferences.WideView {
		ink = gameplayInk.muted
	}
	if g.audioSettingsSelection == gameplayRowPanini {
		drawRectBorder(screen, r.x-4, r.y-2, r.w+8, r.h+4, 1, gameplayInk.focus)
	}
	x, y := r.x+2, r.y+(r.h-18)/2
	drawRectBorder(screen, x, y, 18, 18, 1, ink)
	if enabled {
		uiStrokeLine(screen, float32(x+4), float32(y+9), float32(x+8), float32(y+13), 2, ink, true)
		uiStrokeLine(screen, float32(x+8), float32(y+13), float32(x+15), float32(y+5), 2, ink, true)
	}
	drawUITextColored(screen, "Panini", x+30, r.y+(r.h-uiTextCharHeight)/2, ink)
	ui.settingsClick(r, func() {
		g.audioSettingsSelection = gameplayRowPanini
		g.setPanini(!enabled)
	})
}

// The Builds page: the classes on the left (the party's, or all of them before
// a game), the selected class's builds in the middle, and the chosen build as a
// hero grows in it on the right. Offsets are from the panel origin.
const (
	buildsTop          = 134
	buildsRosterX      = 28
	buildsRosterW      = 172
	buildsRosterRowH   = 60
	buildsRosterPitch  = 66
	buildsRosterRows   = 4 // a full party
	buildsListX        = 210
	buildsListW        = 156
	buildsRowH         = 40
	buildsRowPitch     = 44
	buildsDetailX      = 378
	buildsDetailW      = 314
	buildsLabelW       = 96 // stat names before the growth bars
	buildsGearCell     = 40
	buildsGearPitch    = 46
	buildsLadderRowH   = 18
	buildsHeaderToList = 22
)

// The growth bars share one hue per role, in three steps for the three
// preview levels: the primary in gold, the secondary in bronze, the rest slate.
var buildsBarTones = struct {
	primary, secondary, other [len(autoBuildPreviewLevels)]color.RGBA
	track, rim, ghost         color.RGBA
	primaryInk, secondaryInk  color.RGBA
	accent                    color.RGBA
}{
	[3]color.RGBA{{120, 98, 58, 255}, {176, 147, 88, 255}, {228, 200, 132, 255}},
	[3]color.RGBA{{96, 82, 58, 255}, {140, 122, 86, 255}, {190, 168, 124, 255}},
	[3]color.RGBA{{58, 62, 64, 255}, {84, 89, 90, 255}, {120, 125, 125, 255}},
	color.RGBA{12, 14, 16, 255}, color.RGBA{58, 54, 44, 255}, color.RGBA{232, 226, 205, 230},
	color.RGBA{232, 204, 136, 255}, color.RGBA{204, 182, 136, 255},
	color.RGBA{202, 174, 104, 255},
}

// autoBuildStatTags are the short stat names a build's tag uses.
var autoBuildStatTags = map[string]string{"might": "Mig", "intellect": "Int", "personality": "Per", "endurance": "End", "accuracy": "Acc", "speed": "Spd", "luck": "Lck"}

func autoBuildTag(a config.AutoStatsConfig) string {
	if a.Secondary == "" {
		return autoBuildStatTags[a.Primary]
	}
	return autoBuildStatTags[a.Primary] + " > " + autoBuildStatTags[a.Secondary]
}

// autoBuildLabel is the "Build: <name>" line the hero screens show for the
// build AUTO follows, at most maxW wide at scale: the name alone when the
// line does not fit, nil when neither does. Hovering it shows
// autoBuildTooltipRows.
func autoBuildLabel(b config.AutoBuild, maxW, scale int) []coloredTextSegment {
	label := []coloredTextSegment{{uitext.Text("build.label") + " ", gameplayInk.muted}, {b.Name, gameplayInk.text}}
	for ; len(label) > 0; label = label[1:] {
		if coloredTextWidth(label)*scale <= maxW {
			return label
		}
	}
	return nil
}

// autoBuildTooltipRows names a build and its primary and secondary stats.
func autoBuildTooltipRows(b config.AutoBuild) character.CardRows {
	main := stats.Title(b.Primary)
	if b.Secondary != "" {
		main += ", " + stats.Title(b.Secondary)
	}
	return referenceTooltipRows(uitext.Text("build.card_title", b.Name), uitext.Text("build.main_stats"), main, "")
}

// autoBuildOrder says how AUTO spends points in a build, phase by phase.
func autoBuildOrder(a config.AutoStatsConfig) string {
	s := stats.Title(a.Primary) + " to 99"
	if a.Secondary != "" {
		s += " with " + stats.Title(a.Secondary) + " to 50"
	}
	var rest []string
	for _, k := range []string{a.Secondary, "endurance"} {
		if k != "" && k != a.Primary && (len(rest) == 0 || rest[0] != stats.Title(k)) {
			rest = append(rest, stats.Title(k))
		}
	}
	if len(rest) > 0 {
		s += ", then " + strings.Join(rest, " and ") + " to 99"
	}
	return s + "."
}

func buildsRosterArea(px, py int) layoutRect {
	return layoutRect{px + buildsRosterX, py + buildsTop + buildsHeaderToList, buildsRosterW, buildsRosterRows*buildsRosterPitch - (buildsRosterPitch - buildsRosterRowH)}
}
func buildsRosterRow(px, py, i, offset int) layoutRect {
	a := buildsRosterArea(px, py)
	return layoutRect{a.x, a.y + i*buildsRosterPitch - offset, buildsRosterW, buildsRosterRowH}
}
func buildsListRow(px, py, j int) layoutRect {
	return layoutRect{px + buildsListX, py + buildsTop + buildsHeaderToList + j*buildsRowPitch, buildsListW, buildsRowH}
}

// selectedBuildsClass is the class the Builds page shows.
func (g *MMGame) selectedBuildsClass(classes []character.CharacterClass) int {
	return max(0, min(g.buildsSettingsClass, len(classes)-1))
}

// selectBuildsClass shows class i and keeps it in the roster's view.
func (g *MMGame) selectBuildsClass(i int) {
	g.buildsSettingsClass = i
	top, bottom := i*buildsRosterPitch, i*buildsRosterPitch+buildsRosterRowH
	view := buildsRosterRows*buildsRosterPitch - (buildsRosterPitch - buildsRosterRowH)
	g.buildsRosterScroll = max(min(g.buildsRosterScroll, top), bottom-view)
}

// stepAutoBuild moves the selected class's build one step left or right.
func (g *MMGame) stepAutoBuild(dir int) {
	classes := g.autoBuildClasses()
	if len(classes) == 0 {
		return
	}
	class := classes[g.selectedBuildsClass(classes)]
	builds := classAutoBuilds(class, g.config)
	at := 0
	for i, b := range builds {
		if b.Key == g.autoBuildFor(class).Key {
			at = i
		}
	}
	g.setAutoBuild(class, builds[max(0, min(at+dir, len(builds)-1))].Key)
}

func (ui *UISystem) drawBuildSettings(screen *ebiten.Image, px, py int) {
	g := ui.game
	classes := g.autoBuildClasses()
	if len(classes) == 0 {
		return
	}
	sel := g.selectedBuildsClass(classes)
	owns := ui.audioSettingsOwnsInput()
	mx, my := uiCursorPosition()
	y0 := py + buildsTop

	heading := "Party classes"
	if g.appScreen != AppScreenInGame {
		heading = "Classes"
	}
	drawUITextColored(screen, heading, px+buildsRosterX, y0, gameplayInk.heading)
	area := buildsRosterArea(px, py)
	total := len(classes)*buildsRosterPitch - (buildsRosterPitch - buildsRosterRowH)
	limit := max(0, total-area.h)
	g.buildsRosterScroll = max(0, min(g.buildsRosterScroll, limit))
	offset := g.buildsRosterScroll
	if owns && limit > 0 {
		ui.onDisplayedInput(uiCommandPointer, area, func() {
			_, wheel := pointerWheel()
			wx, wy := pointerPosition()
			if wheel != 0 && isMouseHoveringBox(wx, wy, area.x, area.y, area.right(), area.bottom()) {
				g.buildsRosterScroll = max(0, min(g.buildsRosterScroll+wheelScrollPixels(wheel, 36), limit))
			}
		})
	}
	if dst, view, ok := uiListClip(screen, area); ok {
		for i, class := range classes {
			r := buildsRosterRow(px, py, i, offset)
			hit := image.Rect(r.x, r.y, r.right(), r.bottom()).Intersect(view)
			if hit.Empty() {
				continue
			}
			chosen := i == sel
			hover := owns && image.Pt(mx, my).In(hit)
			fill, edge := color.RGBA{28, 25, 21, 255}, gameplayInk.edge
			if chosen {
				fill, edge = gameplayInk.selected, gameplayInk.focus
			} else if hover {
				edge = gameplayInk.focus
			}
			drawFilledRect(dst, r.x, r.y, r.w, r.h, fill)
			drawRectBorder(dst, r.x, r.y, r.w, r.h, 1, edge)
			if chosen {
				drawFilledRect(dst, r.x, r.y, 3, r.h, buildsBarTones.accent)
			}
			var heroes []string
			if g.appScreen == AppScreenInGame && g.party != nil {
				for _, m := range g.party.Members {
					if m != nil && m.Class == class {
						heroes = append(heroes, m.Name)
					}
				}
			}
			// Class, its heroes, its build; without heroes (before a game) the
			// two remaining lines sit centered.
			classY, buildY := r.y+6, r.y+40
			if len(heroes) == 0 {
				classY, buildY = r.y+12, r.y+31
			} else {
				drawUITextColored(dst, clipUIText(strings.Join(heroes, ", "), r.w-20), r.x+12, r.y+23, gameplayInk.muted)
			}
			drawUITextColored(dst, class.String(), r.x+12, classY, gameplayInk.heading)
			build, ink := g.autoBuildFor(class), buildsBarTones.secondaryInk
			if build.Key == config.DefaultAutoBuildKey {
				ink = gameplayInk.text
			}
			drawUITextColored(dst, clipUIText(build.Name, r.w-20), r.x+12, buildY, ink)
			ui.settingsClick(layoutRectOf(hit), func() {
				g.selectBuildsClass(i)
				g.audioSettingsSelection = gameplayRowPage + 1 + i
			})
		}
	}
	if limit > 0 {
		ui.drawScrollbar(screen, "builds:classes", layoutRect{area.right() + 3, area.y, 3, area.h}, offset, total, owns, func(v int) {
			g.buildsRosterScroll = v
		})
	}

	class := classes[sel]
	previews := g.autoBuildPreviews(class)
	picked := 0
	for j, p := range previews {
		if p.build.Key == g.autoBuildFor(class).Key {
			picked = j
		}
	}
	drawUITextColored(screen, "Builds", px+buildsListX, y0, gameplayInk.heading)
	for j, p := range previews {
		r := buildsListRow(px, py, j)
		chosen := j == picked
		hover := owns && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, chosen || hover)
		if chosen {
			drawFilledRect(screen, r.x+3, r.y+3, r.w-6, r.h-6, gameplayInk.selected)
		}
		nameInk, edge, tag := gameplayInk.text, gameplayInk.edge, autoBuildTag(p.build.AutoStatsConfig)
		if chosen {
			nameInk, edge = gameplayInk.heading, gameplayInk.focus
		}
		if j == 0 {
			tag = "Class default"
		}
		drawUITextColored(screen, clipUIText(p.build.Name, r.w-40), r.x+12, r.y+5, nameInk)
		drawUITextColored(screen, clipUIText(tag, r.w-24), r.x+12, r.y+21, gameplayInk.muted)
		drawBuildCheck(screen, layoutRect{r.right() - 22, r.y + 8, 11, 11}, chosen, edge)
		key := p.build.Key
		ui.settingsClick(r, func() {
			g.audioSettingsSelection = gameplayRowPage + 1 + sel
			g.setAutoBuild(class, key)
		})
	}
	ui.drawBuildDetail(screen, px+buildsDetailX, y0, buildsDetailW, previews, picked)
}

func drawBuildCheck(screen *ebiten.Image, r layoutRect, on bool, edge color.RGBA) {
	drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, edge)
	if on {
		ink := color.RGBA{233, 215, 141, 255}
		uiStrokeLine(screen, float32(r.x+2), float32(r.y+5), float32(r.x+4), float32(r.y+8), 1.5, ink, true)
		uiStrokeLine(screen, float32(r.x+4), float32(r.y+8), float32(r.x+9), float32(r.y+2), 1.5, ink, true)
	}
}

// drawBuildGrowth is one stat's growth bar: nested tones to its value at each
// preview level on a 0..99 track, and a hairline where the class default
// stands at the last level (ghost < 0: none).
func drawBuildGrowth(screen *ebiten.Image, x, y, w int, values [len(autoBuildPreviewLevels)]int, tones [len(autoBuildPreviewLevels)]color.RGBA, ghost int) {
	drawFilledRect(screen, x, y, w, 8, buildsBarTones.track)
	drawRectBorder(screen, x-1, y-1, w+2, 10, 1, buildsBarTones.rim)
	for i := len(values) - 1; i >= 0; i-- {
		drawFilledRect(screen, x, y, w*min(values[i], MaxStatValue)/MaxStatValue, 8, tones[i])
	}
	if ghost >= 0 {
		drawFilledRect(screen, x+w*min(ghost, MaxStatValue)/MaxStatValue, y-3, 1, 14, buildsBarTones.ghost)
	}
}

func drawRightUIText(screen *ebiten.Image, text string, right, y int, ink color.Color) {
	drawUITextColored(screen, text, right-uiTextWidth(text), y, ink)
}

// drawBuildDetail shows the chosen build: how AUTO spends its points, every
// stat at the preview levels against the class default, HP and SP, and the
// gear it is made for.
func (ui *UISystem) drawBuildDetail(screen *ebiten.Image, x, y, w int, previews []autoBuildPreview, picked int) {
	g := ui.game
	p, auto := previews[picked], previews[0]
	owns := ui.audioSettingsOwnsInput()
	mx, my := uiCursorPosition()
	drawUITextColored(screen, p.build.Name, x, y, gameplayInk.heading)
	if picked == 0 {
		drawUITextColored(screen, "(class default)", x+uiTextWidth(p.build.Name)+8, y, gameplayInk.muted)
	}
	ui.drawWrappedTextWithOverflow(screen, autoBuildOrder(p.build.AutoStatsConfig), layoutRect{x, y + 18, w, 32}, 2, 16)

	hy := y + 56
	cols := [len(autoBuildPreviewLevels)]int{x + w - 64, x + w - 32, x + w}
	for k, level := range autoBuildPreviewLevels {
		drawRightUIText(screen, fmt.Sprintf("L%d", level), cols[k], hy, gameplayInk.muted)
	}
	barX := x + buildsLabelW
	barW := cols[0] - 30 - barX - 6
	if picked != 0 {
		drawFilledRect(screen, barX, hy+1, 1, 13, buildsBarTones.ghost)
		drawUITextColored(screen, fmt.Sprintf("Auto at L%d", autoBuildPreviewLevels[len(autoBuildPreviewLevels)-1]), barX+6, hy, gameplayInk.muted)
	}
	if owns && isMouseHoveringBox(mx, my, x, hy, x+w, hy+uiTextCharHeight) {
		ui.queueTooltip([]string{"A hero leveled in this build from level 1.", "Points already spent stay where they are."}, mx+12, my+8)
	}
	for i, name := range stats.Names {
		ry := hy + 20 + i*buildsLadderRowH
		tones, ink := buildsBarTones.other, gameplayInk.muted
		switch name {
		case p.build.Primary:
			tones, ink = buildsBarTones.primary, buildsBarTones.primaryInk
		case p.build.Secondary:
			tones, ink = buildsBarTones.secondary, buildsBarTones.secondaryInk
		}
		drawUITextColored(screen, clipUIText(stats.Title(name), buildsLabelW-4), x, ry, ink)
		var values [len(autoBuildPreviewLevels)]int
		for k := range autoBuildPreviewLevels {
			values[k] = p.stats[k][i]
			drawRightUIText(screen, fmt.Sprint(values[k]), cols[k], ry, gameplayInk.text)
		}
		ghost := -1
		if picked != 0 {
			ghost = auto.stats[len(autoBuildPreviewLevels)-1][i]
		}
		drawBuildGrowth(screen, barX, ry+4, barW, values, tones, ghost)
	}
	vy := hy + 24 + len(stats.Names)*buildsLadderRowH
	for j, row := range []struct {
		label  string
		values [len(autoBuildPreviewLevels)]int
	}{{"HP", p.hp}, {"SP", p.sp}} {
		ry := vy + j*buildsLadderRowH
		drawUITextColored(screen, row.label, x, ry, gameplayInk.muted)
		for k := range autoBuildPreviewLevels {
			drawRightUIText(screen, fmt.Sprint(row.values[k]), cols[k], ry, gameplayInk.text)
		}
	}
	gy := vy + 2*buildsLadderRowH + 8
	drawUITextColored(screen, "Built for", x, gy, gameplayInk.heading)
	for k, it := range p.gear {
		cx, cy := x+k*buildsGearPitch, gy+18
		drawFilledRect(screen, cx, cy, buildsGearCell, buildsGearCell, color.RGBA{24, 27, 30, 255})
		drawRectBorder(screen, cx, cy, buildsGearCell, buildsGearCell, 1, gameplayInk.edge)
		ui.drawInventoryItemIcon(screen, it, cx+2, cy+2, buildsGearCell-4, buildsGearCell-4, 1, true)
		if owns && isMouseHoveringBox(mx, my, cx, cy, cx+buildsGearCell, cy+buildsGearCell) {
			ui.queueItemTooltip(GetItemTooltipRows(it, nil, g.combat, tooltipDetailHeld()), it, nil, mx+16, my+8)
		}
	}
}

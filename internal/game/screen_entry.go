package game

import (
	"fmt"
	"image/color"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// ---------------------------------------------------------------------------
// Entry / title menu (AppScreenMainMenu)
//
// Graphics-ready by design: every drawable element first checks for a named
// sprite via sprites.HasSprite and only falls back to a procedural rectangle +
// text when the art is absent. Drop a correctly-named PNG into assets/sprites/
// and it replaces the placeholder with no code change. The sprite keys are:
//
//   screen_title_bg            full-screen background
//   title_logo                 game logo near the top
//   menu_btn_<key>             a button face (label baked in); else procedural
//   menu_btn_<key>_hover       optional hovered button face
//
// where <key> is one of: start, load, scores, achievements, quit.
// ---------------------------------------------------------------------------

// entryButton is one root-menu choice. Action runs on click/Enter.
type entryButton struct {
	key    string
	label  string
	action func(g *MMGame)
}

// entryButtonDefs is built once at startup - the action closures take *MMGame as
// a parameter (capture-free), so the slice is safe to share across frames.
var entryButtonDefs = []entryButton{
	{"start", "Start Game", func(g *MMGame) { g.enterPartyCreate() }},
	{"load", "Load Game", func(g *MMGame) { g.entryMenuMode = EntryMenuLoad; g.slotSelection = 0; g.savePage = 0 }},
	{"scores", "Top Scores", func(g *MMGame) { g.entryMenuMode = EntryMenuScores }},
	{"achievements", "Achievements", func(g *MMGame) { g.entryMenuMode = EntryMenuAchievements; g.achievementsScroll = 0 }},
	{"quit", "Quit", func(g *MMGame) { g.exitRequested = true }},
}

func entryButtons() []entryButton { return entryButtonDefs }

// updateEntryMenu handles keyboard/back navigation for the entry menu. Mouse
// interaction is handled in drawEntryMenuScreen (roster-screen convention).
func (g *MMGame) updateEntryMenu() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if g.entryMenuMode != EntryMenuRoot {
			g.entryMenuMode = EntryMenuRoot
		}
		return
	}
	if g.entryMenuMode == EntryMenuAchievements {
		_, wy := ebiten.Wheel()
		if wy < 0 {
			g.achievementsScroll++
		} else if wy > 0 && g.achievementsScroll > 0 {
			g.achievementsScroll--
		}
	}
}

// returnToMainMenu leaves the current game and shows the title screen - the
// in-game ESC menu's "Main Menu" option. It does NOT quit the app (the title
// screen's own "Quit" does). The world/party stay in memory but aren't drawn
// while on the title; Start/Load from the title replaces them.
func (g *MMGame) returnToMainMenu() {
	g.mainMenuOpen = false
	g.mainMenuMode = MenuMain
	g.entryMenuMode = EntryMenuRoot
	g.appScreen = AppScreenMainMenu
}

// enterPartyCreate switches to the party-creation screen, building the hero pool.
func (g *MMGame) enterPartyCreate() {
	g.partyCreate = newPartyCreateState(g.config)
	g.appScreen = AppScreenPartyCreate
}

func (ui *UISystem) drawEntryMenuScreen(screen *ebiten.Image) {
	g := ui.game
	w := g.config.GetScreenWidth()
	h := g.config.GetScreenHeight()

	ui.drawScreenBackdrop(screen, w, h, "screen_title_bg")

	switch g.entryMenuMode {
	case EntryMenuRoot:
		ui.drawEntryMenuRoot(screen, w, h)
	case EntryMenuLoad:
		ui.drawEntryLoadList(screen, w, h)
	case EntryMenuScores:
		ui.drawHighScoresOverlay(screen)
		ui.drawBackHint(screen, h)
	case EntryMenuAchievements:
		ui.drawAchievementsScreen(screen, w, h)
	}
}

func (ui *UISystem) drawEntryMenuRoot(screen *ebiten.Image, w, h int) {
	g := ui.game

	// Logo / title.
	logoW, logoH := 460, 140
	logoX := (w - logoW) / 2
	logoY := h/6 - logoH/2
	if logoY < 20 {
		logoY = 20
	}
	if g.sprites.HasSprite("title_logo") {
		drawImageScaled(screen, g.sprites.GetSprite("title_logo"), logoX, logoY, logoW, logoH)
	} else {
		ui.drawBigCenteredText(screen, "RAYS AND MAGIC", w/2, logoY+logoH/2-14, color.RGBA{230, 220, 180, 255})
	}

	// Vertical stack of buttons, centered.
	btns := entryButtons()
	const btnW, btnH, gap = 280, 52, 16
	totalH := len(btns)*btnH + (len(btns)-1)*gap
	startY := logoY + logoH + 40
	if startY+totalH > h-30 {
		startY = h - 30 - totalH
	}
	bx := (w - btnW) / 2
	mouseX, mouseY := ebiten.CursorPosition()
	for i, b := range btns {
		by := startY + i*(btnH+gap)
		hover := isMouseHoveringBox(mouseX, mouseY, bx, by, bx+btnW, by+btnH)
		ui.drawMenuButton(screen, b.key, b.label, bx, by, btnW, btnH, hover)
		if g.consumeLeftClickIn(bx, by, bx+btnW, by+btnH) {
			b.action(g)
			return
		}
	}
}

// drawEntryLoadList shows a page of save slots; clicking a populated slot loads
// it and enters the game. Row 0 of page 0 is the load-only Autosave. Left/Right
// (keys or the on-screen buttons) page through savePageCount pages.
func (ui *UISystem) drawEntryLoadList(screen *ebiten.Image, w, h int) {
	g := ui.game
	panelW, panelH := entryLoadPanelW, entryLoadPanelH
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	ui.drawPanel(screen, "menu_panel_wide", px, py, panelW, panelH)
	drawDebugText(screen, "Load Game", px+menuFrameInset, py+menuFrameInset-4)

	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.savePage = (g.savePage + savePageCount - 1) % savePageCount
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		g.savePage = (g.savePage + 1) % savePageCount
	}

	mouseX, mouseY := ebiten.CursorPosition()
	rowX := px + menuFrameInset
	rowW := panelW - 2*menuFrameInset
	startY := py + menuFrameInset + 22
	rowH := entryLoadRowH
	for i := 0; i < saveRowsPerPage; i++ {
		row := g.savePage*saveRowsPerPage + i
		y := startY + i*rowH
		sum := GetSaveRowSummary(row)
		hover := isMouseHoveringBox(mouseX, mouseY, rowX, y, rowX+rowW, y+rowH-8)
		bg := color.RGBA{40, 40, 70, 220}
		if !sum.Exists {
			bg = color.RGBA{30, 30, 45, 180}
		} else if hover {
			bg = color.RGBA{70, 110, 160, 230}
		}
		drawFilledRect(screen, rowX, y, rowW, rowH-8, bg)
		drawRectBorder(screen, rowX, y, rowW, rowH-8, 1, color.RGBA{90, 90, 130, 200})

		label := fmt.Sprintf("%s - (empty)", saveRowLabel(row))
		if sum.Exists {
			name := sum.Name
			if name == "" || saveRowIsAutosave(row) {
				name = "Saved game"
			}
			mode := "RT"
			if sum.TurnBased {
				mode = "TB"
			}
			t := sum.SavedAt
			if len(t) > 19 {
				t = t[:19]
			}
			label = fmt.Sprintf("%s - %s  [%s %s]", saveRowLabel(row), truncateSaveName(name, 18), mode, t)
		}
		drawDebugText(screen, label, rowX+12, y+rowH/2-12)

		if sum.Exists && g.consumeLeftClickIn(rowX, y, rowX+rowW, y+rowH-8) {
			if err := g.LoadGameFromFile(saveRowPath(row)); err != nil {
				g.AddCombatMessage("Load failed")
			} else {
				g.entryMenuMode = EntryMenuRoot
				g.appScreen = AppScreenInGame
			}
			return
		}
	}

	// Page controls: distinct Prev/Next buttons on their own row (dimmed at the
	// ends), clearly above the Back button so neither overlaps the other.
	pagerY := startY + saveRowsPerPage*rowH + 6
	const pbW, pbH = 96, 26
	drawEntryPagerBtn := func(bx int, label string, enabled bool, onClick func()) {
		fill := color.RGBA{60, 60, 100, 230}
		if !enabled {
			fill = color.RGBA{35, 35, 55, 200}
		}
		drawFilledRect(screen, bx, pagerY, pbW, pbH, fill)
		drawRectBorder(screen, bx, pagerY, pbW, pbH, 1, color.RGBA{120, 120, 180, 230})
		drawCenteredDebugText(screen, label, bx, pagerY+(pbH-12)/2, pbW, 12)
		if enabled && g.consumeLeftClickIn(bx, pagerY, bx+pbW, pagerY+pbH) {
			onClick()
		}
	}
	drawEntryPagerBtn(rowX, "< Prev", true, func() { g.savePage = (g.savePage + savePageCount - 1) % savePageCount })
	drawEntryPagerBtn(rowX+rowW-pbW, "Next >", true, func() { g.savePage = (g.savePage + 1) % savePageCount })
	drawCenteredDebugText(screen, fmt.Sprintf("Page %d/%d", g.savePage+1, savePageCount), rowX, pagerY+(pbH-12)/2, rowW, 12)

	ui.drawBackButton(screen, px+menuFrameInset, pagerY+pbH+12, func() { g.entryMenuMode = EntryMenuRoot })
}

// drawAchievementsScreen renders the data-driven (stub) achievements list. All
// entries display as locked - unlock tracking is not implemented yet.
func (ui *UISystem) drawAchievementsScreen(screen *ebiten.Image, w, h int) {
	g := ui.game
	panelW, panelH := 640, 480
	if panelW > w-40 {
		panelW = w - 40
	}
	if panelH > h-40 {
		panelH = h - 40
	}
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	ui.drawPanel(screen, "menu_panel_wide", px, py, panelW, panelH)
	drawDebugText(screen, "Achievements", px+menuFrameInset, py+menuFrameInset-4)

	defs := config.GetAchievements()
	listX := px + menuFrameInset
	listY := py + menuFrameInset + 22
	listW := panelW - 2*menuFrameInset
	rowH := 64
	backY := py + panelH - menuFrameInset - 30
	visibleRows := (backY - listY - 8) / rowH

	if len(defs) == 0 {
		drawDebugText(screen, "No achievements defined.", listX, listY+8)
		ui.drawBackButton(screen, listX, backY, func() { g.entryMenuMode = EntryMenuRoot })
		return
	}

	// Clamp scroll.
	maxScroll := len(defs) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if g.achievementsScroll > maxScroll {
		g.achievementsScroll = maxScroll
	}

	for row := 0; row < visibleRows; row++ {
		idx := g.achievementsScroll + row
		if idx >= len(defs) {
			break
		}
		def := defs[idx]
		y := listY + row*rowH
		drawFilledRect(screen, listX, y, listW, rowH-8, color.RGBA{34, 34, 54, 220})
		drawRectBorder(screen, listX, y, listW, rowH-8, 1, color.RGBA{80, 80, 120, 200})

		// Icon (sprite hook) or procedural locked badge.
		iconSize := rowH - 20
		iconX := listX + 8
		iconY := y + 6
		if def.Icon != "" && g.sprites.HasSprite(def.Icon) {
			drawImageScaled(screen, g.sprites.GetSprite(def.Icon), iconX, iconY, iconSize, iconSize)
		} else {
			drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{50, 50, 60, 255})
			drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{90, 90, 110, 255})
			drawCenteredDebugText(screen, "?", iconX, iconY, iconSize, iconSize)
		}

		textX := iconX + iconSize + 12
		drawDebugTextColored(screen, def.Name, textX, y+8, color.RGBA{220, 210, 160, 255})
		drawDebugTextColored(screen, def.Description, textX, y+26, color.RGBA{170, 170, 180, 255})
		drawDebugTextColored(screen, "[ Locked ]", listX+listW-90, y+8, color.RGBA{140, 110, 110, 255})
	}

	if maxScroll > 0 {
		drawDebugText(screen, "Scroll: mouse wheel", px+panelW-menuFrameInset-150, py+menuFrameInset-4)
	}
	ui.drawBackButton(screen, listX, backY, func() { g.entryMenuMode = EntryMenuRoot })
}

// ---------------------------------------------------------------------------
// Shared menu-screen drawing helpers (also used by the party-creation screen).
// ---------------------------------------------------------------------------

// drawScreenBackdrop fills the screen with a named background sprite when one
// exists, otherwise a dark vertical-ish gradient placeholder.
func (ui *UISystem) drawScreenBackdrop(screen *ebiten.Image, w, h int, spriteKey string) {
	if spriteKey != "" && ui.game.sprites.HasSprite(spriteKey) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(spriteKey), 0, 0, w, h)
		return
	}
	// Placeholder gradient: a few horizontal bands from deep blue to near-black.
	bands := 8
	for i := 0; i < bands; i++ {
		t := float64(i) / float64(bands)
		c := color.RGBA{
			R: uint8(18 + 10*(1-t)),
			G: uint8(18 + 14*(1-t)),
			B: uint8(40 + 26*(1-t)),
			A: 255,
		}
		drawFilledRect(screen, 0, i*h/bands, w, h/bands+1, c)
	}
}

// drawPanel draws an ornate framed panel using a 9-sliced sprite when present
// (corners kept crisp), else a procedural dark rect + border. frameKey "" forces
// the procedural look.
func (ui *UISystem) drawPanel(screen *ebiten.Image, frameKey string, x, y, w, h int) {
	if frameKey != "" && ui.game.sprites.HasSprite(frameKey) {
		drawNineSlice(screen, ui.game.sprites.GetSprite(frameKey), x, y, w, h, menuFrameSlice)
		return
	}
	drawFilledRect(screen, x, y, w, h, color.RGBA{20, 20, 40, 235})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{100, 100, 160, 255})
}

// menuFrameSlice is the corner size (source px) for 9-slicing the ornate menu
// frames so their gold corners don't stretch. menuFrameInset is how far panel
// CONTENT must sit inside the frame so it clears the decorative gold border.
const (
	menuFrameSlice = 34
	menuFrameInset = 44 // must exceed menuFrameSlice so content clears the gold corner band
)

// drawButtonHoverGlow draws a soft warm halo just OUTSIDE the button edge - a
// light highlight around it, not a wash over the face. Two fading gold rings.
func drawButtonHoverGlow(screen *ebiten.Image, x, y, w, h int) {
	drawRectBorder(screen, x-2, y-2, w+4, h+4, 1, color.RGBA{255, 226, 150, 90})
	drawRectBorder(screen, x-1, y-1, w+2, h+2, 1, color.RGBA{255, 236, 175, 170})
}

// drawMenuButton draws a button face. Resolution order: per-key art
// (menu_btn_<key>[_hover], label assumed baked in) -> generic frame (menu_btn,
// label drawn on top) -> procedural panel. Hover gets a highlight overlay when
// the art has no dedicated _hover variant.
func (ui *UISystem) drawMenuButton(screen *ebiten.Image, key, label string, x, y, w, h int, hover bool) {
	s := ui.game.sprites
	draw := func(name string) bool {
		if !s.HasSprite(name) {
			return false
		}
		drawImageScaled(screen, s.GetSprite(name), x, y, w, h)
		return true
	}

	// Per-key art: the label is baked into the image.
	if hover && draw("menu_btn_"+key+"_hover") {
		return
	}
	if draw("menu_btn_" + key) {
		if hover {
			drawButtonHoverGlow(screen, x, y, w, h)
		}
		return
	}

	// Generic frame: draw the label centered on top.
	used := false
	if hover && draw("menu_btn_hover") {
		used = true
	} else if draw("menu_btn") {
		used = true
		if hover {
			drawButtonHoverGlow(screen, x, y, w, h)
		}
	}
	if used {
		drawCenteredDebugText(screen, label, x, y, w, h)
		return
	}

	// Procedural fallback.
	bg := color.RGBA{45, 45, 78, 235}
	border := color.RGBA{110, 110, 170, 255}
	if hover {
		bg = color.RGBA{70, 110, 165, 245}
		border = color.RGBA{170, 200, 240, 255}
	}
	drawFilledRect(screen, x, y, w, h, bg)
	drawRectBorder(screen, x, y, w, h, 2, border)
	drawCenteredDebugText(screen, label, x, y, w, h)
}

// drawBigCenteredText draws text scaled up ~2x, horizontally centered at cx.
func (ui *UISystem) drawBigCenteredText(screen *ebiten.Image, text string, cx, y int, col color.Color) {
	// The debug font has no scaling; emulate emphasis by drawing the string and
	// centering it on cx. (Art replaces this via title_logo.)
	x := cx - debugTextWidth(text)/2
	drawDebugTextColored(screen, text, x, y, col)
}

// drawBackButton draws a small "Back" button at (x,y) and runs onClick when hit.
func (ui *UISystem) drawBackButton(screen *ebiten.Image, x, y int, onClick func()) {
	const w, h = 110, 30
	mouseX, mouseY := ebiten.CursorPosition()
	hover := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
	ui.drawMenuButton(screen, "back", "Back (Esc)", x, y, w, h, hover)
	if ui.game.consumeLeftClickIn(x, y, x+w, y+h) {
		onClick()
	}
}

// drawBackHint prints a small return hint at the bottom for full-bleed sub
// screens (e.g. high scores) that draw their own background.
func (ui *UISystem) drawBackHint(screen *ebiten.Image, h int) {
	ui.drawBackButton(screen, 20, h-44, func() { ui.game.entryMenuMode = EntryMenuRoot })
}

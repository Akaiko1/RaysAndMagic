package game

import (
	"image"
	"image/color"
	"math"
	"strconv"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/shaders"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Storage Clerk's collections: a full-viewport book with two tabs beside
// the party bag. The legendary showcase is a velvet display case, one niche per
// legendary; the card binder is a ring binder of 3x3 sleeve pages, one pocket
// per card. A click files a bag item; a click on a filled place takes one back
// to the bag. Every move is a stash transfer.

const (
	collectionTabShowcase = iota
	collectionTabBinder
)

const (
	collectionSideW   = 300
	collectionHeaderH = 54
	collectionBagCell = 56
	collectionBagGap  = 8
	collectionBagCols = 4

	showcaseNiche  = 92
	showcaseGap    = 14
	showcasePlaque = 18
	// The niche's velvet reaches this share of the icon past it on every side.
	showcaseMarginFrac = .2

	binderCols, binderRows = 3, 3
	binderGap              = 8
	binderPagePad          = 14
	binderGutter           = 40 // the rings between the two pages of a spread
	binderHoleInset        = 7  // ring holes from a page's inner edge
	binderRings            = 3  // rings through each page
	binderWire             = 3  // ring wire radius
	binderPlate            = 6  // half width of the ring plate
)

// binderMargin is how far a pocket's sleeve reaches past its card on each
// side, as a share of the card's width and height. Every card is measured
// inside its own icon (IconArtBounds) and fills the same share of its
// pocket, whatever its size in the icon.
var binderMargin = [2]float32{.07, .05}

type collectionLayout struct {
	panel, close layoutRect
	tabs         [2]layoutRect
	page, side   layoutRect
	// Showcase: the case under its glass, the grid of wells scrolled inside
	// it, and its scrollbar.
	glass, grid, track layoutRect
	cols               int
	// Binder: the pages of the open spread and the page turner.
	pages          []layoutRect
	pocketW        int
	pocketH        int
	turner         layoutRect
	bagGrid, pager layoutRect
	hint           layoutRect // the side column's instructions, under the pager
	bagRows        int
}

func computeCollectionLayout(screenW, viewportBottom, tab int) collectionLayout {
	hub := computeTabbedMenuLayout(screenW, viewportBottom)
	L := collectionLayout{panel: hub.panel, close: hub.close}
	tabX := hub.panel.x + 18
	tabW := min(240, (hub.close.x-12-tabX-5)/2)
	for i := range L.tabs {
		L.tabs[i] = layoutRect{tabX + i*(tabW+5), hub.panel.y + 18, tabW, tabbedMenuTabH}
	}
	content := hub.content
	L.side = layoutRect{content.right() - collectionSideW, content.y, collectionSideW, content.h}
	L.page = layoutRect{content.x, content.y, content.w - collectionSideW - 16, content.h}
	body := layoutRect{L.page.x + 12, L.page.y + collectionHeaderH, L.page.w - 24, L.page.h - collectionHeaderH - 12}
	if tab == collectionTabBinder {
		// Pockets keep the 63:88 card shape; two pages share a spread when they fit.
		body.h -= 30
		L.pocketH = (body.h - 2*binderPagePad - (binderRows-1)*binderGap) / binderRows
		L.pocketW = L.pocketH * 63 / 88
		pageW := 2*binderPagePad + binderCols*L.pocketW + (binderCols-1)*binderGap
		if maxW := (body.w - 2*binderPagePad - (binderCols-1)*binderGap) / binderCols; L.pocketW > maxW {
			L.pocketW = maxW
			L.pocketH = L.pocketW * 88 / 63
			pageW = 2*binderPagePad + binderCols*L.pocketW + (binderCols-1)*binderGap
		}
		pageH := 2*binderPagePad + binderRows*L.pocketH + (binderRows-1)*binderGap
		spread := 1
		if 2*pageW+binderGutter <= body.w {
			spread = 2
		}
		spreadW := spread*pageW + (spread-1)*binderGutter
		x := body.x + (body.w-spreadW)/2
		for i := 0; i < spread; i++ {
			L.pages = append(L.pages, layoutRect{x + i*(pageW+binderGutter), body.y, pageW, pageH})
		}
		L.turner = layoutRect{body.x + (body.w-240)/2, body.y + pageH + 10, 240, 18}
	} else {
		L.cols = max(1, (body.w-20+showcaseGap)/(showcaseNiche+showcaseGap))
		used := L.cols*showcaseNiche + (L.cols-1)*showcaseGap
		L.grid = layoutRect{body.x + (body.w-20-used)/2, body.y, used, body.h}
		L.track = layoutRect{L.page.right() - 14, body.y, 4, body.h}
		L.glass = layoutRect{L.page.x + 8, body.y - 6, L.page.w - 30, body.h + 6}
	}
	bagW := collectionBagCols*collectionBagCell + (collectionBagCols-1)*collectionBagGap
	L.bagRows = max(1, (L.side.h-130)/(collectionBagCell+collectionBagGap))
	L.bagGrid = layoutRect{L.side.x + (L.side.w-bagW)/2, L.side.y + 40, bagW, L.bagRows*(collectionBagCell+collectionBagGap) - collectionBagGap}
	L.pager = layoutRect{L.bagGrid.x, L.bagGrid.bottom() + 10, bagW, 18}
	L.hint = layoutRect{L.side.x + 12, L.pager.bottom() + 14, L.side.w - 24, L.side.bottom() - L.pager.bottom() - 14}
	return L
}

// showcasePitch is one case row: the niche, its plaque and the gap.
func showcasePitch() int { return showcaseNiche + showcasePlaque + showcaseGap }

// pocketsPerSpread is how many binder pockets one open spread shows.
func (L collectionLayout) pocketsPerSpread() int { return len(L.pages) * binderCols * binderRows }

// pocketRect is pocket i of the open spread.
func (L collectionLayout) pocketRect(i int) layoutRect {
	perPage := binderCols * binderRows
	page := L.pages[i/perPage]
	i %= perPage
	return layoutRect{page.x + binderPagePad + i%binderCols*(L.pocketW+binderGap), page.y + binderPagePad + i/binderCols*(L.pocketH+binderGap), L.pocketW, L.pocketH}
}

// openCollectionScreen shows the clerk's collections; the chest stays at the tavern.
func (g *MMGame) openCollectionScreen() bool {
	if !g.ensureStashLoaded() {
		g.AddCombatMessage("Could not open the stash.")
		return false
	}
	g.collectionScreenOpen = true
	g.setCollectionTab(collectionTabShowcase)
	return true
}

func (g *MMGame) closeCollectionScreen() { g.collectionScreenOpen = false }

func (g *MMGame) setCollectionTab(tab int) {
	g.collectionTab = tab
	g.collectionScroll, g.collectionSpread, g.collectionBagPage = 0, 0, 0
	g.resetDialogClickTracker()
}

// collectionTemplate is a key's catalog item, built once: an empty place's art
// and tooltip.
func (ui *UISystem) collectionTemplate(key string) items.Item {
	if it, ok := ui.collectionTemplates[key]; ok {
		return it
	}
	if ui.collectionTemplates == nil {
		ui.collectionTemplates = map[string]items.Item{}
	}
	it, err := items.TryCreateWeaponFromYAML(key)
	if err != nil {
		it = items.CreateItemFromYAML(key)
	}
	ui.collectionTemplates[key] = it
	return it
}

func (ui *UISystem) drawCollectionScreen(screen *ebiten.Image) {
	g := ui.game
	if g.stash == nil {
		return
	}
	viewportBottom := gameplayViewportBottom(g)
	binder := g.collectionTab == collectionTabBinder
	L := computeCollectionLayout(g.config.GetScreenWidth(), viewportBottom, g.collectionTab)
	interactive := ui.topModalLayer() == modalLayerCollection

	drawFilledRect(screen, 0, 0, uiBounds(screen).Dx(), viewportBottom, color.RGBA{5, 7, 12, 205})
	ui.drawPatternFrame(screen, "menu_panel_frame", L.panel.x, L.panel.y, L.panel.w, L.panel.h, menuPanelFrameSlice)
	ui.drawCornerDecor(screen, frameGold, L.panel.x-8, L.panel.y-8, L.panel.w+16, L.panel.h+16, decorBottomCorners)
	for i, label := range [2]string{uitext.Text("collection.tab_showcase"), uitext.Text("collection.tab_binder")} {
		tab := L.tabs[i]
		ui.drawButtonFrame(screen, tab.x, tab.y, tab.w, tab.h, g.collectionTab == i)
		drawCenteredUIText(screen, label, tab.x, tab.y, tab.w, tab.h)
		ui.onDisplayedInput(uiCommandNavigation, tab, func() {
			if interactive && g.consumeLeftClickIn(tab.x, tab.y, tab.right(), tab.bottom()) && g.collectionTab != i {
				g.setCollectionTab(i)
			}
		})
	}
	ui.drawPopupCloseButton(screen, L.close.x, L.close.y, L.close.w, interactive, g.closeCollectionScreen)
	if binder {
		ui.drawBinderPage(screen, L, interactive)
	} else {
		ui.drawShowcasePage(screen, L, interactive)
	}
	ui.drawCollectionBag(screen, L, binder, interactive)
}

// drawCollectionHeader is the page title, its count and one line of context.
func (ui *UISystem) drawCollectionHeader(screen *ebiten.Image, page layoutRect, title, progress, about string) {
	drawUITextColored(screen, title, page.x+16, page.y+10, gameplayInk.heading)
	drawUITextColored(screen, progress, page.right()-16-uiTextWidth(progress), page.y+10, gameplayInk.focus)
	drawUITextColored(screen, clipUIText(about, page.w-32), page.x+16, page.y+28, gameplayInk.muted)
}

// drawShowcasePage is the display case: velvet niches under the glass, a brass
// plaque under each, scrolled by the wheel or the scrollbar.
func (ui *UISystem) drawShowcasePage(screen *ebiten.Image, L collectionLayout, interactive bool) {
	g := ui.game
	keys := LegendaryCollectionKeys()
	rows := (len(keys) + L.cols - 1) / L.cols
	contentH := rows*showcasePitch() - showcaseGap
	g.collectionScroll = max(0, min(g.collectionScroll, contentH-L.grid.h))

	// The cabinet, and the velvet case under its glass with a brass trim.
	ui.drawCollectionSurface(screen, L.page, collectionSurface{mode: collectionModeCabinet, page: L.page})
	drawRectBorder(screen, L.page.x, L.page.y, L.page.w, L.page.h, 1, color.RGBA{120, 92, 50, 255})
	ui.drawCollectionHeader(screen, L.page, uitext.Text("collection.tab_showcase"),
		uitext.Text("collection.progress_showcase", len(g.stash.Collection), len(keys)), uitext.Text("collection.about_showcase"))

	glass := L.glass
	clipRect := image.Rect(glass.x, glass.y, glass.right(), glass.bottom())
	clip := uiClip(screen, clipRect)
	scroll := float32(g.collectionScroll)
	ui.drawCollectionSurface(clip, glass, collectionSurface{mode: collectionModeBed, page: glass, scroll: scroll})
	drawRectBorder(screen, glass.x-1, glass.y-1, glass.w+2, glass.h+2, 1, color.RGBA{150, 116, 62, 255})
	mx, my := uiCursorPosition()
	onPage := isMouseHoveringBox(mx, my, clipRect.Min.X, clipRect.Min.Y, clipRect.Max.X, clipRect.Max.Y)
	accent := rarityRGBA("legendary")
	for i, key := range keys {
		r := layoutRect{L.grid.x + i%L.cols*(showcaseNiche+showcaseGap), L.grid.y + i/L.cols*showcasePitch() - g.collectionScroll, showcaseNiche, showcaseNiche}
		if r.bottom()+showcasePlaque < L.grid.y || r.y > L.grid.bottom() {
			continue
		}
		held, filled := g.stash.Collection[key]
		art := ui.collectionTemplate(key)
		hover := onPage && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		ui.drawCollectionPlace(clip, r, art, false, filled, hover, 0, glass, accent, collectionSurface{scroll: scroll, seed: float32(i)})
		// The brass plaque under the well, polished once its piece is on show.
		plate := layoutRect{r.x + 6, r.bottom() + 3, r.w - 12, showcasePlaque - 4}
		ui.drawCollectionSurface(clip, plate, collectionSurface{mode: collectionModePlaque, filled: filled, page: plate})
		name := clipUIText(art.Name, plate.w+10)
		nx := plate.x + (plate.w-uiTextWidth(name))/2
		ink := gameplayInk.muted
		if filled {
			ink = gameplayInk.heading
		}
		drawUITextColored(clip, name, nx, plate.y, ink)
		if hover && interactive {
			ui.queueCollectionTooltip(key, held, filled, mx, my)
		}
		ui.onDisplayedInput(uiCommandClick, r, func() {
			if interactive && filled && g.consumeDoubleClickIn("showcase", i, r.x, max(r.y, L.grid.y), r.right(), min(r.bottom(), L.grid.bottom())) {
				g.takeFromCollection(key, false)
			}
		})
	}
	ui.drawScrollbar(screen, "collection_showcase", L.track, g.collectionScroll, contentH, interactive, func(v int) { g.collectionScroll = v })
	ui.onWheelOver(glass, interactive, func(dir int) {
		g.collectionScroll = max(0, min(contentH-L.grid.h, g.collectionScroll+dir*showcasePitch()/2))
	})
}

// drawBinderPage is the open binder: one or two 3x3 sleeve pages with the
// rings between them, turned by the page turner or the wheel.
func (ui *UISystem) drawBinderPage(screen *ebiten.Image, L collectionLayout, interactive bool) {
	g := ui.game
	keys := CardAlbumKeys()
	perSpread := L.pocketsPerSpread()
	spreads := pageCount(len(keys), perSpread)
	clampPage(&g.collectionSpread, spreads)
	copies := 0
	for _, page := range g.stash.Album {
		copies += len(page)
	}
	drawFilledRect(screen, L.page.x, L.page.y, L.page.w, L.page.h, color.RGBA{16, 16, 19, 240})
	drawRectBorder(screen, L.page.x, L.page.y, L.page.w, L.page.h, 1, gameplayInk.edge)
	ui.drawCollectionHeader(screen, L.page, uitext.Text("collection.tab_binder"),
		uitext.Text("collection.progress_binder", len(g.stash.Album), len(keys), copies), uitext.Text("collection.about_binder", stash.AlbumCopies))
	// The open binder: its cover, the sleeve pages with their welds and ring
	// holes, and the rings across the gutter of a spread.
	left, right := L.pages[0], L.pages[len(L.pages)-1]
	cover := layoutRect{left.x - 12, left.y - 4, right.right() - left.x + 24, left.h + 8}
	ui.drawCollectionSurface(screen, cover, collectionSurface{mode: collectionModeCover, page: cover})
	scale := float32(uiScaleOf(screen))
	seam := float32(binderPagePad) - binderGap/2.
	grid := [4]float32{seam * scale, seam * scale, float32(L.pocketW+binderGap) * scale, float32(L.pocketH+binderGap) * scale}
	for i, page := range L.pages {
		side := float32(0)
		if len(L.pages) == 2 {
			side = 1 - 2*float32(i)
		}
		ui.drawCollectionSurface(screen, page, collectionSurface{mode: collectionModeSheet, page: page, grid: grid, gap: binderGap * scale, side: side})
	}
	if len(L.pages) == 2 {
		// The ring quad runs from hole to hole, binderHoleInset into each page.
		end := binderHoleInset + binderWire + 2
		rings := layoutRect{left.right() - end, left.y, right.x - left.right() + 2*end, left.h}
		geometry := [4]float32{float32(binderWire+2) * scale, float32(rings.w-binderWire-2) * scale, binderWire * scale, binderPlate * scale}
		ui.drawCollectionSurface(screen, rings, collectionSurface{mode: collectionModeRings, page: rings, grid: geometry})
	}
	mx, my := uiCursorPosition()
	clock := float64(g.uiFrameCount) / float64(max(1, g.config.GetTPS()))
	accent := color.RGBA{190, 120, 220, 255}
	first := g.collectionSpread * perSpread
	for slot := 0; slot < perSpread; slot++ {
		n := first + slot
		if n >= len(keys) {
			break
		}
		key := keys[n]
		r := L.pocketRect(slot)
		page := g.stash.Album[key]
		count := len(page)
		var held items.Item
		if count > 0 {
			held = page[count-1]
		}
		hover := isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		// The hovered card tilts toward the pointer; the rest sway gently.
		tilt := [2]float32{float32(math.Sin(clock*.23+float64(n)*1.7)) * .6, float32(math.Cos(clock*.19+float64(n)*2.3)) * .5}
		if hover {
			tilt = [2]float32{float32(2*(mx-r.x))/float32(r.w) - 1, float32(2*(my-r.y))/float32(r.h) - 1}
		}
		ui.drawCollectionPlace(screen, r, ui.collectionTemplate(key), true, count > 0, hover, count, L.pages[slot/(binderCols*binderRows)], accent, collectionSurface{tilt: tilt})
		if count > 1 {
			badge := "x" + strconv.Itoa(count)
			drawUITextColored(screen, badge, r.right()-uiTextWidth(badge)-6, r.y+5, gameplayInk.heading)
		}
		if hover && interactive {
			ui.queueCollectionTooltip(key, held, count > 0, mx, my)
		}
		ui.onDisplayedInput(uiCommandClick, r, func() {
			if interactive && count > 0 && g.consumeDoubleClickIn("binder", n, r.x, r.y, r.right(), r.bottom()) {
				g.takeFromCollection(key, true)
			}
		})
	}
	ui.drawPager(screen, L.turner.x, L.turner.y, L.turner.w, &g.collectionSpread, spreads, interactive)
	ui.onWheelOver(L.page, interactive, func(dir int) {
		g.collectionSpread += dir
		clampPage(&g.collectionSpread, spreads)
	})
}

// queueCollectionTooltip is the place's base item card (no hero holds it, as
// in a shop): the filed item, or the catalog entry for a place still empty.
func (ui *UISystem) queueCollectionTooltip(key string, held items.Item, filled bool, mx, my int) {
	g := ui.game
	it := held
	if !filled {
		it = ui.collectionTemplate(key)
	}
	if cardKey := itemCardKey(it); cardKey != "" {
		ui.fullArtCardKey = cardKey
	}
	// A collection view: the item's inventory hints give way to the place's.
	rows := itemTooltipWithUsageRows(it, nil, g.combat, tooltipDetailHeld(), itemUsage{ReplaceDefaultHints: true})
	if len(rows) == 0 {
		return
	}
	if filled {
		rows.Add(character.CardRowHint, uitext.Text("collection.take_hint"))
	} else {
		rows.Add(character.CardRowHint, uitext.Text("collection.not_collected"))
	}
	ui.queueItemTooltip(rows, it, nil, mx+16, my+8)
}

// drawCollectionBag is the side column: the party bag's legendaries or cards,
// each filed by a click.
func (ui *UISystem) drawCollectionBag(screen *ebiten.Image, L collectionLayout, binder, interactive bool) {
	g := ui.game
	drawFilledRect(screen, L.side.x, L.side.y, L.side.w, L.side.h, color.RGBA{14, 13, 13, 225})
	drawRectBorder(screen, L.side.x, L.side.y, L.side.w, L.side.h, 1, gameplayInk.edge)
	drawCenteredUIText(screen, uitext.Text("collection.bag"), L.side.x, L.side.y+10, L.side.w, 16)
	ui.collectionBag = ui.collectionBag[:0]
	for i, it := range g.party.Inventory {
		if key := collectionBagKey(it, binder); key != "" {
			ui.collectionBag = append(ui.collectionBag, collectionBagEntry{i, key})
		}
	}
	perPage := L.bagRows * collectionBagCols
	pages := pageCount(len(ui.collectionBag), perPage)
	clampPage(&g.collectionBagPage, pages)
	mx, my := uiCursorPosition()
	empty := uitext.Text("collection.bag_empty_showcase")
	if binder {
		empty = uitext.Text("collection.bag_empty_binder")
	}
	if len(ui.collectionBag) == 0 {
		drawCenteredUIText(screen, empty, L.side.x, L.bagGrid.y+20, L.side.w, 16)
	}
	start := g.collectionBagPage * perPage
	for slot := 0; slot < perPage; slot++ {
		n := start + slot
		if n >= len(ui.collectionBag) {
			break
		}
		idx, key := ui.collectionBag[n].idx, ui.collectionBag[n].key
		it := g.party.Inventory[idx]
		x := L.bagGrid.x + slot%collectionBagCols*(collectionBagCell+collectionBagGap)
		y := L.bagGrid.y + slot/collectionBagCols*(collectionBagCell+collectionBagGap)
		cell := layoutRect{x, y, collectionBagCell, collectionBagCell}
		full := g.collectionPlaceFull(key, binder)
		ui.drawThemeFrame(screen, frameGold, x, y, collectionBagCell, collectionBagCell)
		ui.drawInventoryItemIcon(screen, it, x+2, y+2, collectionBagCell-4, collectionBagCell-4, 0, !full)
		if isMouseHoveringBox(mx, my, x, y, cell.right(), cell.bottom()) {
			drawRectBorder(screen, x-2, y-2, collectionBagCell+4, collectionBagCell+4, 2, color.RGBA{120, 200, 120, 220})
			if interactive {
				ui.stashCellTooltip(it, image.Rect(x, y, cell.right(), cell.bottom()), mx, my)
			}
		}
		ui.onDisplayedInput(uiCommandClick, cell, func() {
			if interactive && g.consumeDoubleClickIn("collection_bag", idx, cell.x, cell.y, cell.right(), cell.bottom()) {
				g.fileIntoCollection(idx, binder)
			}
		})
	}
	ui.drawPager(screen, L.pager.x, L.pager.y, L.pager.w, &g.collectionBagPage, pages, interactive)
	for i, line := range wrapUIText(collectionBagHint(binder), L.hint.w) {
		drawUITextColored(screen, line, L.hint.x, L.hint.y+i*collectionHintLineH, gameplayInk.muted)
	}
}

const collectionHintLineH = 18

// collectionBagHint is the side column's instructions for the open tab.
func collectionBagHint(binder bool) string {
	hint := uitext.Text("collection.bag_hint_showcase")
	if binder {
		hint = uitext.Text("collection.bag_hint_binder")
	}
	return hint + " " + uitext.Text("collection.take_hint")
}

// collectionBagEntry is a bag item the open tab can file, with its place key.
type collectionBagEntry struct {
	idx int
	key string
}

// Collection shader surfaces (collection_slot.kage Mode).
const (
	collectionModeNiche = iota
	collectionModePocket
	collectionModeSheet
	collectionModeRings
	collectionModePlaque
	collectionModeCover
	collectionModeBed
	collectionModeCabinet
)

// collectionDraw is the reused state of the collection surface draws.
type collectionDraw struct {
	uniforms             map[string]any
	vertices             [4]ebiten.Vertex
	opts                 ebiten.DrawTrianglesShaderOptions
	page, grid, card     [4]float32
	layout               [4]float32
	accent               [3]float32
	dstMin, tilt, margin [2]float32
}

// collectionSurface is one quad of the collection shader. icon and src (its
// texels) are set for the niche and pocket; the other surfaces have no image.
type collectionSurface struct {
	mode          int
	icon          *ebiten.Image
	src, card     [4]float32 // pocket: the card inside the icon, 0..1
	margin        [2]float32 // texels the quad runs past the icon or card
	copies        float32
	filled, hover bool
	page          layoutRect // the surface its sheen or glass spans
	grid          [4]float32 // pixels: sheet pocket grid, ring geometry
	gap, side     float32
	scroll, seed  float32    // showcase: the case's scroll (UI units), a well's phase
	tilt          [2]float32 // binder: a card's tilt toward the light, -1..1
	accent        color.RGBA
}

// drawCollectionPlace draws one showcase well or binder pocket over its icon;
// a key without an icon gets a plain frame. page is the case glass or binder
// page the place sits under; s carries the showcase's scroll and seed.
func (ui *UISystem) drawCollectionPlace(dst *ebiten.Image, r layoutRect, art items.Item, binder, filled, hover bool, copies int, page layoutRect, accent color.RGBA, s collectionSurface) {
	icon := ui.game.sprites.GetSprite(itemTooltipIconName(art))
	if icon == nil {
		drawRectBorder(dst, r.x, r.y, r.w, r.h, 1, gameplayInk.edge)
		return
	}
	b := icon.Bounds()
	s.mode, s.icon, s.filled, s.hover, s.copies, s.page, s.accent = collectionModeNiche, icon, filled, hover, float32(copies), page, accent
	if binder {
		// The quad runs over the card inside the icon plus the sleeve around it.
		card, ok := ui.game.sprites.IconArtBounds(itemTooltipIconName(art))
		if !ok {
			card = image.Rect(0, 0, b.Dx(), b.Dy())
		}
		w, h := float32(b.Dx()), float32(b.Dy())
		s.mode = collectionModePocket
		s.card = [4]float32{float32(card.Min.X) / w, float32(card.Min.Y) / h, float32(card.Max.X) / w, float32(card.Max.Y) / h}
		s.margin = [2]float32{float32(card.Dx()) * binderMargin[0], float32(card.Dy()) * binderMargin[1]}
		s.src = [4]float32{float32(b.Min.X+card.Min.X) - s.margin[0], float32(b.Min.Y+card.Min.Y) - s.margin[1],
			float32(b.Min.X+card.Max.X) + s.margin[0], float32(b.Min.Y+card.Max.Y) + s.margin[1]}
	} else {
		m := float32(b.Dx()) * showcaseMarginFrac
		s.margin = [2]float32{m, m}
		s.src = [4]float32{float32(b.Min.X) - m, float32(b.Min.Y) - m, float32(b.Max.X) + m, float32(b.Max.Y) + m}
	}
	ui.drawCollectionSurface(dst, r, s)
}

// drawCollectionSurface draws one collection shader quad over r (UI units),
// in dst's pixels.
func (ui *UISystem) drawCollectionSurface(dst *ebiten.Image, r layoutRect, s collectionSurface) {
	d := &ui.collectionDraw
	if ui.collectionShader == nil {
		shader, err := ebiten.NewShader([]byte(shaders.Source("collection_slot.kage")))
		if err != nil {
			panic(err)
		}
		ui.collectionShader = shader
		d.uniforms = map[string]any{}
	}
	if s.icon == nil {
		s.src = [4]float32{0, 0, 1, 1}
	}
	px := uiPixelRect(dst, image.Rect(r.x, r.y, r.right(), r.bottom()))
	pg := uiPixelRect(dst, image.Rect(s.page.x, s.page.y, s.page.right(), s.page.bottom()))
	dx0, dy0, dx1, dy1 := float32(px.Min.X), float32(px.Min.Y), float32(px.Max.X), float32(px.Max.Y)
	v := &d.vertices
	v[0] = ebiten.Vertex{DstX: dx0, DstY: dy0, SrcX: s.src[0], SrcY: s.src[1], ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	v[1] = ebiten.Vertex{DstX: dx1, DstY: dy0, SrcX: s.src[2], SrcY: s.src[1], ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	v[2] = ebiten.Vertex{DstX: dx0, DstY: dy1, SrcX: s.src[0], SrcY: s.src[3], ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	v[3] = ebiten.Vertex{DstX: dx1, DstY: dy1, SrcX: s.src[2], SrcY: s.src[3], ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	d.page = [4]float32{float32(pg.Min.X), float32(pg.Min.Y), float32(pg.Dx()), float32(pg.Dy())}
	d.grid, d.card, d.margin = s.grid, s.card, s.margin
	d.accent = [3]float32{float32(s.accent.R) / 255, float32(s.accent.G) / 255, float32(s.accent.B) / 255}
	u := d.uniforms
	u["Mode"] = float32(s.mode)
	u["Filled"] = boolFloat32(s.filled)
	u["Hover"] = boolFloat32(s.hover)
	u["Time"] = float32(float64(ui.game.uiFrameCount) / float64(max(1, ui.game.config.GetTPS())))
	u["Margin"] = d.margin[:]
	u["Copies"] = s.copies
	u["Side"] = s.side
	u["Gap"] = s.gap
	u["Scroll"] = s.scroll
	u["Seed"] = s.seed
	d.tilt = s.tilt
	u["Tilt"] = d.tilt[:]
	d.layout = [4]float32{binderCols, binderRows, binderHoleInset, binderRings}
	u["Layout"] = d.layout[:]
	u["Scale"] = float32(uiScaleOf(dst))
	d.dstMin = [2]float32{float32(dst.Bounds().Min.X), float32(dst.Bounds().Min.Y)}
	u["DstMin"] = d.dstMin[:]
	u["CardRect"] = d.card[:]
	u["Page"] = d.page[:]
	u["Grid"] = d.grid[:]
	u["Accent"] = d.accent[:]
	d.opts.Uniforms = u
	d.opts.Images[0] = s.icon
	dst.DrawTrianglesShader(v[:], collectionQuadIndices, ui.collectionShader, &d.opts)
}

var collectionQuadIndices = []uint16{0, 1, 2, 1, 3, 2}

func boolFloat32(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

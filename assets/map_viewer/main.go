package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"ugataima/internal/boot"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	windowWidth   = 1200
	windowHeight  = 800
	sidebarWidth  = 300
	pageBarHeight = 32 // top tab bar (Maps | Items & Spells)
)

// Top-level pages within the viewer window.
const (
	pageMaps   = 0
	pageItems  = 1 // weapons + items
	pageSpells = 2 // spells grouped by school
	pageChars  = 3 // playable characters with starting loadout
	pageSkills = 4 // all skills with detailed descriptions
	pageFX     = 5 // live preview of the game's special effects (fx_page.go)
	pageMobs   = 6 // monster stat sheets + live animated preview (mobs_page.go)
)

// pageTabDefs drives both the top tab bar and the F1..F5 hotkeys.
var pageTabDefs = []struct {
	page   int
	label  string
	hotkey string
}{
	{pageMaps, "Maps", "F1"},
	{pageItems, "Items", "F2"},
	{pageSpells, "Spells", "F3"},
	{pageChars, "Characters", "F4"},
	{pageSkills, "Skills", "F5"},
	{pageFX, "FX", "F6"},
	{pageMobs, "Mobs", "F7"},
}

type mapInfo struct {
	Key    string
	Config *config.MapConfig
	Data   *world.MapData
	Err    error
	Header []string // leading "#" comment lines, preserved across save
	EOL    string   // original line ending ("\r\n" or "\n"), preserved across save
}

type viewer struct {
	page           int
	maps           []mapInfo
	mapIndex       int
	legendLines    []legendEntry
	legendScroll   int
	sidebarTab     int
	tileDataByKey  map[string]*config.TileData
	tileManager    *world.TileManager
	monsterCfg     *monster.MonsterYAMLConfig
	brush          brush
	saveDialogOpen bool
	savePath       string
	saveError      string
	lastErr        string

	// Content page state: per-page card lists and independent scroll offsets.
	pageCards   map[int][]contentCard
	pageScroll  map[int]int
	charDetails []charDetail             // Characters page (custom full-detail renderer)
	iconCache   map[string]*ebiten.Image // key: "<kind>:<itemKey>"; nil value = "no icon on disk"
}

// contentCard, contentKind, and the cardX constants live in content_cards.go.

const (
	tabInfo = iota
	tabLegend
)

type brushKind int

const (
	brushNone brushKind = iota
	brushTile
	brushMonster
	brushNPC
	brushEraser
)

type brush struct {
	kind        brushKind
	letter      string
	tileKey     string
	monsterKey  string
	monsterName string
	npcKey      string
	npcName     string
}

type legendEntry struct {
	Text        string
	Kind        brushKind
	Letter      string
	TileKey     string
	MonsterKey  string
	MonsterName string
	NPCKey      string
	NPCName     string
	IsHeader    bool
}

type layout struct {
	mapAreaX  int
	mapAreaY  int
	mapAreaW  int
	mapAreaH  int
	toolbarX  int
	toolbarY  int
	toolbarW  int
	toolbarH  int
	sidebarX  int
	sidebarY  int
	tabHeight int
	legendX   int
	legendY   int
	legendW   int
	legendH   int
	originX   int
	originY   int
	tileSize  int
	worldW    int
	worldH    int
}

type rect struct {
	x int
	y int
	w int
	h int
}

type toolbarButton struct {
	id    string
	label string
	rect  rect
}

func main() {
	// Shared content configs + bridges, same sequence as the game.
	cfg, monsterCfg := boot.LoadGameData()

	maps, err := loadMaps(cfg)
	if err != nil {
		log.Printf("Warning: %v", err)
	}

	v := &viewer{
		page:          pageMaps,
		maps:          maps,
		mapIndex:      0,
		sidebarTab:    tabInfo,
		tileDataByKey: world.GlobalTileManager.ListTiles(),
		tileManager:   world.GlobalTileManager,
		monsterCfg:    monsterCfg,
		brush:         brush{kind: brushEraser},
		pageCards: map[int][]contentCard{
			pageItems:  buildItemsCards(),
			pageSpells: buildSpellCards(),
			pageSkills: buildSkillCards(),
		},
		pageScroll:  map[int]int{},
		charDetails: buildCharacterDetails(cfg),
		iconCache:   make(map[string]*ebiten.Image),
	}
	// Legend is biome-scoped to the current map (universal tiles/monsters
	// plus the map biome's own); rebuilt whenever the map changes.
	v.refreshLegend()
	if len(maps) == 0 {
		v.lastErr = "no maps loaded"
	}

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("RaysAndMagic Map Viewer")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetVsyncEnabled(false)
	ebiten.SetMaxTPS(120)

	if err := ebiten.RunGame(v); err != nil {
		log.Fatal(err)
	}
}

func (v *viewer) Update() error {
	if v.saveDialogOpen {
		v.handleSaveDialogInput()
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}

	// Top-page switching via F1..F5 (Maps / Items / Spells / Characters / Skills).
	for i, def := range pageTabDefs {
		if inpututil.IsKeyJustPressed(ebiten.KeyF1 + ebiten.Key(i)) {
			v.page = def.page
		}
	}

	// Page-bar click works on both pages. Consume the click here if it
	// landed on a tab so the page-specific handler below doesn't also
	// fire (e.g., a stray brush stroke on the Maps page).
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		_, my := ebiten.CursorPosition()
		if my < pageBarHeight {
			v.handlePageBarClick()
			return nil
		}
	}

	if v.page == pageFX {
		v.updateFXPage()
		return nil
	}

	if v.page == pageMobs {
		v.updateMobsPage()
		return nil
	}

	if v.page != pageMaps {
		scroll := v.pageScroll[v.page]
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 {
			scroll -= int(wheelY * 30)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
			scroll += 200
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
			scroll -= 200
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
			scroll += 40
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
			scroll -= 40
		}
		if scroll < 0 {
			scroll = 0
		}
		maxScroll := v.maxContentScroll()
		if v.page == pageChars {
			maxScroll = v.maxCharactersScroll()
		}
		if scroll > maxScroll {
			scroll = maxScroll
		}
		v.pageScroll[v.page] = scroll
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			v.handlePageBarClick()
		}
		return nil
	}

	// Maps page below.
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		if v.sidebarTab == tabInfo {
			v.sidebarTab = tabLegend
		} else {
			v.sidebarTab = tabInfo
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		v.sidebarTab = tabInfo
	}
	if inpututil.IsKeyJustPressed(ebiten.Key2) {
		v.sidebarTab = tabLegend
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyE) {
		v.brush = brush{kind: brushEraser}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		if len(v.maps) > 0 {
			v.mapIndex = (v.mapIndex + 1) % len(v.maps)
			v.refreshLegend()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		if len(v.maps) > 0 {
			v.mapIndex--
			if v.mapIndex < 0 {
				v.mapIndex = len(v.maps) - 1
			}
			v.refreshLegend()
		}
	}

	if v.sidebarTab == tabLegend {
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 {
			v.legendScroll -= int(wheelY * 14)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
			v.legendScroll += 14 * 8
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
			v.legendScroll -= 14 * 8
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
			v.legendScroll += 14
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
			v.legendScroll -= 14
		}
		maxScroll := v.maxLegendScroll()
		if v.legendScroll < 0 {
			v.legendScroll = 0
		}
		if v.legendScroll > maxScroll {
			v.legendScroll = maxScroll
		}
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		v.handleMouseClick()
	}
	return nil
}

func (v *viewer) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{15, 15, 22, 255})

	v.drawPageBar(screen)

	if v.page == pageFX {
		v.drawFXPage(screen)
		return
	}
	if v.page == pageMobs {
		v.drawMobsPage(screen)
		return
	}
	if v.page == pageChars {
		v.drawCharactersPage(screen)
		return
	}
	if v.page != pageMaps {
		v.drawContentPage(screen)
		return
	}

	// Maps page below.
	if len(v.maps) == 0 {
		msg := v.lastErr
		if msg == "" {
			msg = "no maps loaded"
		}
		ebitenutil.DebugPrintAt(screen, msg, 16, pageBarHeight+16)
		return
	}

	m := v.maps[v.mapIndex]
	if m.Err != nil {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("map %s failed to load: %v", m.Key, m.Err), 16, pageBarHeight+16)
		return
	}

	lay := v.computeLayout(m)

	drawMapPanel(screen, m, lay.mapAreaX, lay.mapAreaY, lay.mapAreaW, lay.mapAreaH, v.tileManager, v.tileDataByKey, v.tileSpriteThumbnail)
	drawToolbar(screen, lay, v.brush)
	drawSidebar(screen, m, lay.sidebarX, lay.sidebarY, sidebarWidth, lay.mapAreaH+lay.toolbarH+16, v.sidebarTab, v.legendLines, v.legendScroll, v.brush, v.tileManager, v.tileDataByKey, v.tileSpriteThumbnail)

	if !v.saveDialogOpen {
		drawNPCHoverTooltip(screen, m, lay)
	}

	if v.saveDialogOpen {
		drawSaveDialog(screen, v.savePath, v.saveError)
	}
}

// drawNPCHoverTooltip shows who an `@` marker is when the mouse is over a tile
// holding an NPC spawn — name/type/description pulled from the shared npcs.yaml
// config (no hardcoded list), so it stays in sync with the game.
func drawNPCHoverTooltip(screen *ebiten.Image, m mapInfo, lay layout) {
	if lay.tileSize <= 0 || m.Data == nil {
		return
	}
	mouseX, mouseY := ebiten.CursorPosition()
	mapW := lay.worldW * lay.tileSize
	mapH := lay.worldH * lay.tileSize
	if !pointInRect(mouseX, mouseY, lay.originX, lay.originY, mapW, mapH) {
		return
	}
	tileX := (mouseX - lay.originX) / lay.tileSize
	tileY := (mouseY - lay.originY) / lay.tileSize

	for _, npc := range m.Data.NPCSpawns {
		if npc.X != tileX || npc.Y != tileY {
			continue
		}
		lines := []string{"@  " + npc.NPCKey}
		if character.NPCConfigInstance != nil {
			if def, ok := character.NPCConfigInstance.NPCs[npc.NPCKey]; ok {
				if def.Name != "" {
					lines[0] = "@  " + def.Name
				}
				if def.Type != "" {
					lines = append(lines, "  ["+def.Type+"]")
				}
				if def.Description != "" {
					lines = append(lines, "")
					lines = append(lines, wrapTooltipLines(def.Description, 64)...)
				}
			}
		}

		const lineH = 14
		maxLineW := 0
		for _, ln := range lines {
			if w := utf8.RuneCountInString(ln) * 7; w > maxLineW {
				maxLineW = w
			}
		}
		boxW := maxLineW + 16
		boxH := len(lines)*lineH + 12
		boxX := mouseX + 16
		boxY := mouseY + 12
		if boxX+boxW > windowWidth-4 {
			boxX = mouseX - boxW - 8
		}
		if boxX < 4 {
			boxX = 4
		}
		if boxY+boxH > windowHeight-4 {
			boxY = windowHeight - boxH - 4
		}
		if boxY < 4 {
			boxY = 4
		}
		drawFilledRect(screen, boxX, boxY, boxW, boxH, color.RGBA{18, 18, 28, 240})
		drawRectBorder(screen, boxX, boxY, boxW, boxH, 1, color.RGBA{200, 180, 60, 255})
		for i, ln := range lines {
			ebitenutil.DebugPrintAt(screen, ln, boxX+8, boxY+6+i*lineH)
		}
		return
	}
}

func (v *viewer) Layout(_, _ int) (int, int) {
	return windowWidth, windowHeight
}

func (v *viewer) computeLayout(m mapInfo) layout {
	padding := 16
	toolbarH := 36
	topOffset := pageBarHeight + padding // leave room for the top page-tab bar
	mapAreaW := windowWidth - sidebarWidth - padding*3
	mapAreaH := windowHeight - topOffset - padding - toolbarH - padding
	mapAreaX := padding
	mapAreaY := topOffset
	toolbarX := mapAreaX
	toolbarY := mapAreaY + mapAreaH + padding
	toolbarW := mapAreaW
	sidebarX := mapAreaX + mapAreaW + padding
	sidebarY := topOffset
	tabHeight := 24

	legendX := sidebarX
	legendY := sidebarY + tabHeight + 12
	legendW := sidebarWidth
	legendH := mapAreaH + toolbarH + padding - (legendY - sidebarY) - 12

	originX := mapAreaX
	originY := mapAreaY
	tileSize := 0
	worldW := 0
	worldH := 0
	if m.Data != nil {
		worldW = m.Data.Width
		worldH = m.Data.Height
		if worldW > 0 && worldH > 0 {
			tileSize = mapAreaW / worldW
			if alt := mapAreaH / worldH; alt < tileSize {
				tileSize = alt
			}
			if tileSize < 2 {
				tileSize = 2
			}
			originX = mapAreaX + (mapAreaW-worldW*tileSize)/2
			originY = mapAreaY + (mapAreaH-worldH*tileSize)/2
		}
	}

	return layout{
		mapAreaX:  mapAreaX,
		mapAreaY:  mapAreaY,
		mapAreaW:  mapAreaW,
		mapAreaH:  mapAreaH,
		toolbarX:  toolbarX,
		toolbarY:  toolbarY,
		toolbarW:  toolbarW,
		toolbarH:  toolbarH,
		sidebarX:  sidebarX,
		sidebarY:  sidebarY,
		tabHeight: tabHeight,
		legendX:   legendX,
		legendY:   legendY,
		legendW:   legendW,
		legendH:   legendH,
		originX:   originX,
		originY:   originY,
		tileSize:  tileSize,
		worldW:    worldW,
		worldH:    worldH,
	}
}

func (v *viewer) handleMouseClick() {
	if len(v.maps) == 0 {
		return
	}
	m := v.maps[v.mapIndex]
	lay := v.computeLayout(m)

	mouseX, mouseY := ebiten.CursorPosition()
	tabW := sidebarWidth / 2
	if pointInRect(mouseX, mouseY, lay.sidebarX, lay.sidebarY, tabW, lay.tabHeight) {
		v.sidebarTab = tabInfo
		return
	}
	if pointInRect(mouseX, mouseY, lay.sidebarX+tabW, lay.sidebarY, sidebarWidth-tabW, lay.tabHeight) {
		v.sidebarTab = tabLegend
		return
	}

	if v.sidebarTab == tabLegend && pointInRect(mouseX, mouseY, lay.legendX, lay.legendY, lay.legendW, lay.legendH) {
		lineHeight := 14
		index := (mouseY - lay.legendY + v.legendScroll) / lineHeight
		if index >= 0 && index < len(v.legendLines) {
			entry := v.legendLines[index]
			if entry.Kind != brushNone && !entry.IsHeader {
				v.brush = brushFromEntry(entry)
			}
		}
		return
	}

	for _, btn := range toolbarButtons(lay) {
		if pointInRect(mouseX, mouseY, btn.rect.x, btn.rect.y, btn.rect.w, btn.rect.h) {
			switch btn.id {
			case "brush":
				v.sidebarTab = tabLegend
			case "eraser":
				v.brush = brush{kind: brushEraser}
			case "save":
				v.openSaveDialog()
			}
			return
		}
	}

	if v.brush.kind == brushNone || lay.tileSize <= 0 || m.Data == nil {
		return
	}

	mapW := lay.worldW * lay.tileSize
	mapH := lay.worldH * lay.tileSize
	if !pointInRect(mouseX, mouseY, lay.originX, lay.originY, mapW, mapH) {
		return
	}

	tileX := (mouseX - lay.originX) / lay.tileSize
	tileY := (mouseY - lay.originY) / lay.tileSize
	if tileX < 0 || tileX >= lay.worldW || tileY < 0 || tileY >= lay.worldH {
		return
	}

	v.applyBrush(&v.maps[v.mapIndex], tileX, tileY)
}

func (v *viewer) maxLegendScroll() int {
	lineHeight := 14
	contentHeight := lineHeight
	if len(v.maps) > 0 {
		lay := v.computeLayout(v.maps[v.mapIndex])
		contentHeight = lay.legendH
		if contentHeight < lineHeight {
			contentHeight = lineHeight
		}
	}
	totalHeight := len(v.legendLines) * lineHeight
	if totalHeight <= contentHeight {
		return 0
	}
	return totalHeight - contentHeight
}

func drawMapPanel(screen *ebiten.Image, m mapInfo, x, y, w, h int, tm *world.TileManager, tileDataByKey map[string]*config.TileData, thumb func(sprite string) *ebiten.Image) {
	drawFilledRect(screen, x, y, w, h, color.RGBA{20, 20, 35, 255})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{70, 70, 90, 255})

	if m.Data == nil || m.Config == nil {
		ebitenutil.DebugPrintAt(screen, "map data missing", x+12, y+12)
		return
	}

	worldW := m.Data.Width
	worldH := m.Data.Height
	if worldW <= 0 || worldH <= 0 {
		ebitenutil.DebugPrintAt(screen, "invalid map size", x+12, y+12)
		return
	}

	tileSize := w / worldW
	if alt := h / worldH; alt < tileSize {
		tileSize = alt
	}
	if tileSize < 2 {
		tileSize = 2
	}

	originX := x + (w-worldW*tileSize)/2
	originY := y + (h-worldH*tileSize)/2

	floorColor := effectiveFloorColor(m, tm, tileDataByKey)

	for ty := 0; ty < worldH; ty++ {
		for tx := 0; tx < worldW; tx++ {
			tile := m.Data.Tiles[ty][tx]
			drawX := originX + tx*tileSize
			drawY := originY + ty*tileSize
			// Tiles with a sprite (objects: trees, palms, houses…) draw the
			// sprite over the floor color, so the map reads like the game,
			// not just colored squares. Floors stay flat color. Skip sprites
			// when cells are too tiny to be legible (keeps it fast + clean).
			var sprite string
			if key := tm.GetTileKey(tile); key != "" {
				if data := tileDataByKey[key]; data != nil {
					sprite = data.Sprite
				}
			}
			if tileSize >= 6 && sprite != "" && thumb != nil {
				if img := thumb(sprite); img != nil {
					under := floorUnderObjectColor(m, tm, tileDataByKey, tx, ty, floorColor)
					vector.FillRect(screen, float32(drawX), float32(drawY), float32(tileSize), float32(tileSize), under, false)
					drawImageInBox(screen, img, drawX, drawY, tileSize, tileSize)
					continue
				}
			}
			cellColor := getMapTileColor(tile, floorColor, tm, tileDataByKey)
			vector.FillRect(screen, float32(drawX), float32(drawY), float32(tileSize), float32(tileSize), cellColor, false)
		}
	}

	drawOverlays(screen, m, originX, originY, tileSize)
	drawMapHeader(screen, m, x, y)
}

func drawMapHeader(screen *ebiten.Image, m mapInfo, x, y int) {
	title := "Map Viewer"
	if m.Config != nil {
		title = fmt.Sprintf("%s (%s)", m.Config.Name, m.Key)
	}
	ebitenutil.DebugPrintAt(screen, title, x+12, y+8)
	ebitenutil.DebugPrintAt(screen, "Left/Right (or A/D) to switch maps, Esc to quit", x+12, y+24)
}

func drawOverlays(screen *ebiten.Image, m mapInfo, originX, originY, tileSize int) {
	// Start position
	if m.Data.StartX >= 0 && m.Data.StartY >= 0 {
		drawTileMarkerCircle(screen, originX, originY, tileSize, m.Data.StartX, m.Data.StartY, color.RGBA{50, 200, 255, 255}, true)
	}

	// NPCs
	for _, npc := range m.Data.NPCSpawns {
		drawTileMarkerRect(screen, originX, originY, tileSize, npc.X, npc.Y, color.RGBA{255, 220, 0, 255})
		drawTileLetter(screen, originX, originY, tileSize, npc.X, npc.Y, "@")
	}

	// Monsters
	for _, spawn := range m.Data.MonsterSpawns {
		drawTileMarkerCircle(screen, originX, originY, tileSize, spawn.X, spawn.Y, color.RGBA{230, 80, 80, 255}, false)
		letter := monsterLetterForKey(spawn.MonsterKey)
		if letter != "" {
			drawTileLetter(screen, originX, originY, tileSize, spawn.X, spawn.Y, letter)
		}
	}

	// Special tiles (teleporters, etc.)
	for _, special := range m.Data.SpecialTileSpawns {
		label := strings.ToUpper(string([]rune(special.TileKey)[0]))
		drawTileLetter(screen, originX, originY, tileSize, special.X, special.Y, label)
	}
}

func drawSidebar(screen *ebiten.Image, m mapInfo, x, y, w, h int, tab int, legendLines []legendEntry, scroll int, currentBrush brush, tm *world.TileManager, tileDataByKey map[string]*config.TileData, thumb func(sprite string) *ebiten.Image) {
	drawFilledRect(screen, x, y, w, h, color.RGBA{18, 18, 26, 255})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{70, 70, 90, 255})

	tabHeight := 24
	drawSidebarTabs(screen, x, y, w, tabHeight, tab)
	row := y + tabHeight + 12

	if tab == tabLegend {
		drawLegendList(screen, x, row, w, h-(row-y)-12, legendLines, scroll, currentBrush, tileDataByKey, effectiveFloorColor(m, tm, tileDataByKey), thumb)
		return
	}

	if m.Data == nil {
		return
	}

	stats := []string{
		fmt.Sprintf("Tiles: %dx%d", m.Data.Width, m.Data.Height),
		fmt.Sprintf("Monsters: %d", len(m.Data.MonsterSpawns)),
		fmt.Sprintf("NPCs: %d", len(m.Data.NPCSpawns)),
		fmt.Sprintf("Special tiles: %d", len(m.Data.SpecialTileSpawns)),
	}
	for _, line := range stats {
		ebitenutil.DebugPrintAt(screen, line, x+12, row)
		row += 16
	}

	row += 8
	ebitenutil.DebugPrintAt(screen, "Markers:", x+12, row)
	row += 16
	ebitenutil.DebugPrintAt(screen, "Cyan: start  Red: monsters", x+12, row)
	row += 16
	ebitenutil.DebugPrintAt(screen, "Yellow: NPCs  Letters: keys", x+12, row)
	row += 24
	ebitenutil.DebugPrintAt(screen, "Brush:", x+12, row)
	row += 16
	ebitenutil.DebugPrintAt(screen, formatBrushLabel(currentBrush), x+12, row)
	row += 16
	ebitenutil.DebugPrintAt(screen, "Save uses toolbar prompt", x+12, row)
}

func drawSidebarTabs(screen *ebiten.Image, x, y, w, h int, active int) {
	tabW := w / 2
	infoColor := color.RGBA{40, 40, 55, 255}
	legendColor := color.RGBA{40, 40, 55, 255}
	if active == tabInfo {
		infoColor = color.RGBA{70, 70, 95, 255}
	} else {
		legendColor = color.RGBA{70, 70, 95, 255}
	}
	drawFilledRect(screen, x, y, tabW, h, infoColor)
	drawFilledRect(screen, x+tabW, y, w-tabW, h, legendColor)
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{70, 70, 90, 255})
	drawCenteredLabel(screen, "Info (1)", rect{x: x, y: y, w: tabW, h: h})
	drawCenteredLabel(screen, "Legend (2)", rect{x: x + tabW, y: y, w: w - tabW, h: h})
}

func drawLegendList(screen *ebiten.Image, x, y, w, h int, lines []legendEntry, scroll int, currentBrush brush, tileDataByKey map[string]*config.TileData, floorColor color.RGBA, thumb func(sprite string) *ebiten.Image) {
	lineHeight := 14
	startY := y - scroll
	for i, entry := range lines {
		drawY := startY + i*lineHeight
		if drawY < y-lineHeight {
			continue
		}
		if drawY > y+h-lineHeight {
			break
		}
		if brushMatchesEntry(currentBrush, entry) {
			drawFilledRect(screen, x+4, drawY-2, w-8, lineHeight+2, color.RGBA{70, 70, 95, 255})
		}
		// Preview showing how the tile/monster looks on the map, so the
		// letter isn't the only cue: a sprite thumbnail for tiles that have
		// one (objects), otherwise a color swatch matching the map grid.
		// Floors have no sprite, so they stay color-only (no atlas overload).
		const sw = 12
		textX := x + 10
		if !entry.IsHeader {
			sx, sy := x+8, drawY
			drawn := false
			if entry.Kind == brushTile && thumb != nil {
				if data := tileDataByKey[entry.TileKey]; data != nil && data.Sprite != "" {
					if img := thumb(data.Sprite); img != nil {
						drawImageInBox(screen, img, sx, sy, sw, sw)
						drawn = true
					}
				}
			}
			if !drawn {
				if swatch, ok := legendSwatchColor(entry, tileDataByKey, floorColor); ok {
					drawFilledRect(screen, sx, sy, sw, sw, swatch)
					drawn = true
				}
			}
			if drawn {
				drawRectBorder(screen, sx, sy, sw, sw, 1, color.RGBA{15, 15, 22, 255})
				textX = sx + sw + 6
			}
		}
		// Clip text so long names don't run off the panel.
		avail := (x + w) - textX - 8
		ebitenutil.DebugPrintAt(screen, clipText(entry.Text, avail), textX, drawY)
	}
}

// drawImageInBox draws img scaled to fit a sw×sw box at (bx,by).
func drawImageInBox(screen *ebiten.Image, img *ebiten.Image, bx, by, bw, bh int) {
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	if iw == 0 || ih == 0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(bw)/float64(iw), float64(bh)/float64(ih))
	op.GeoM.Translate(float64(bx), float64(by))
	screen.DrawImage(img, op)
}

// clipText truncates text with an ellipsis to fit availPx (≈6px per glyph in
// the debug font).
func clipText(text string, availPx int) string {
	const glyphW = 6
	maxChars := availPx / glyphW
	if maxChars < 1 {
		return ""
	}
	if len(text) <= maxChars {
		return text
	}
	if maxChars <= 1 {
		return text[:maxChars]
	}
	return text[:maxChars-1] + "…"
}

// legendSwatchColor returns the preview color for a legend entry: the map
// color for tiles, a red marker for monsters. ok=false for entries with no
// visual (eraser / unresolved).
func legendSwatchColor(entry legendEntry, tileDataByKey map[string]*config.TileData, floorColor color.RGBA) (color.RGBA, bool) {
	switch entry.Kind {
	case brushTile:
		if c, ok := tileSwatchColor(entry.TileKey, tileDataByKey[entry.TileKey], floorColor); ok {
			return c, true
		}
		return floorColor, true
	case brushMonster:
		return color.RGBA{200, 60, 60, 255}, true // monsters render as red markers on the map
	case brushNPC:
		return color.RGBA{255, 220, 0, 255}, true // NPCs render as yellow @ markers on the map
	}
	return color.RGBA{}, false
}

func drawToolbar(screen *ebiten.Image, lay layout, currentBrush brush) {
	drawFilledRect(screen, lay.toolbarX, lay.toolbarY, lay.toolbarW, lay.toolbarH, color.RGBA{18, 18, 26, 255})
	drawRectBorder(screen, lay.toolbarX, lay.toolbarY, lay.toolbarW, lay.toolbarH, 2, color.RGBA{70, 70, 90, 255})

	for _, btn := range toolbarButtons(lay) {
		bg := color.RGBA{40, 40, 55, 255}
		if btn.id == "eraser" && currentBrush.kind == brushEraser {
			bg = color.RGBA{70, 70, 95, 255}
		}
		if btn.id == "brush" && currentBrush.kind != brushEraser && currentBrush.kind != brushNone {
			bg = color.RGBA{70, 70, 95, 255}
		}
		drawFilledRect(screen, btn.rect.x, btn.rect.y, btn.rect.w, btn.rect.h, bg)
		drawRectBorder(screen, btn.rect.x, btn.rect.y, btn.rect.w, btn.rect.h, 1, color.RGBA{90, 90, 115, 255})
		drawCenteredLabel(screen, btn.label, btn.rect)
	}
}

func toolbarButtons(lay layout) []toolbarButton {
	padding := 8
	btnW := 96
	btnH := lay.toolbarH - padding*2
	if btnH < 18 {
		btnH = 18
	}
	x := lay.toolbarX + padding
	y := lay.toolbarY + padding
	labels := []toolbarButton{
		{id: "brush", label: "Brush", rect: rect{x: x, y: y, w: btnW, h: btnH}},
		{id: "eraser", label: "Eraser", rect: rect{x: x + btnW + padding, y: y, w: btnW, h: btnH}},
		{id: "save", label: "Save", rect: rect{x: x + (btnW+padding)*2, y: y, w: btnW, h: btnH}},
	}
	return labels
}

func formatBrushLabel(b brush) string {
	switch b.kind {
	case brushEraser:
		return "Eraser"
	case brushTile:
		if b.tileKey != "" {
			return fmt.Sprintf("Tile %s (%s)", b.letter, b.tileKey)
		}
		return fmt.Sprintf("Tile %s", b.letter)
	case brushMonster:
		if b.monsterKey != "" {
			return fmt.Sprintf("Monster %s (%s)", b.letter, b.monsterKey)
		}
		return fmt.Sprintf("Monster %s", b.letter)
	case brushNPC:
		if b.npcName != "" {
			return fmt.Sprintf("NPC @ (%s)", b.npcName)
		}
		return fmt.Sprintf("NPC @ (%s)", b.npcKey)
	default:
		return "None"
	}
}

func brushMatchesEntry(b brush, entry legendEntry) bool {
	if entry.Kind == brushNone || entry.IsHeader {
		return false
	}
	switch b.kind {
	case brushEraser:
		return entry.Kind == brushEraser
	case brushTile:
		return entry.Kind == brushTile && entry.Letter == b.letter && entry.TileKey == b.tileKey
	case brushMonster:
		return entry.Kind == brushMonster && entry.MonsterKey == b.monsterKey
	case brushNPC:
		return entry.Kind == brushNPC && entry.NPCKey == b.npcKey
	default:
		return false
	}
}

func (v *viewer) openSaveDialog() {
	if len(v.maps) == 0 {
		return
	}
	m := v.maps[v.mapIndex]
	defaultPath := ""
	if m.Config != nil && m.Config.File != "" {
		defaultPath = filepath.Join("assets", m.Config.File)
	} else if m.Key != "" {
		defaultPath = filepath.Join("assets", m.Key+".map")
	} else {
		defaultPath = "assets/untitled.map"
	}
	v.saveDialogOpen = true
	v.savePath = defaultPath
	v.saveError = ""
}

func (v *viewer) handleSaveDialogInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		v.saveDialogOpen = false
		v.saveError = ""
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		if err := v.saveCurrentMap(); err != nil {
			v.saveError = err.Error()
		} else {
			v.saveDialogOpen = false
			v.saveError = ""
		}
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		v.savePath = trimLastRune(v.savePath)
	}

	for _, r := range ebiten.AppendInputChars(nil) {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		v.savePath += string(r)
	}
}

func (v *viewer) saveCurrentMap() error {
	if len(v.maps) == 0 {
		return fmt.Errorf("no map loaded")
	}
	if strings.TrimSpace(v.savePath) == "" {
		return fmt.Errorf("empty path")
	}
	m := v.maps[v.mapIndex]
	gridLines, err := encodeMapLines(&m, v.tileManager)
	if err != nil {
		return err
	}
	// Preserve the original "#" comment header and line-ending style for a
	// lossless round-trip.
	lines := append(append([]string{}, m.Header...), gridLines...)
	eol := m.EOL
	if eol == "" {
		eol = "\n"
	}

	path := v.savePath
	if !strings.HasSuffix(path, ".map") {
		path += ".map"
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, eol)+eol), 0o644)
}

func drawSaveDialog(screen *ebiten.Image, path, errMsg string) {
	w := 640
	h := 140
	screenW := screen.Bounds().Dx()
	screenH := screen.Bounds().Dy()
	x := (screenW - w) / 2
	y := (screenH - h) / 2

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})
	drawFilledRect(screen, x, y, w, h, color.RGBA{25, 25, 35, 255})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{90, 90, 120, 255})
	ebitenutil.DebugPrintAt(screen, "Save map as:", x+12, y+12)
	drawFilledRect(screen, x+12, y+34, w-24, 24, color.RGBA{15, 15, 20, 255})
	drawRectBorder(screen, x+12, y+34, w-24, 24, 1, color.RGBA{80, 80, 100, 255})
	ebitenutil.DebugPrintAt(screen, path, x+16, y+38)
	ebitenutil.DebugPrintAt(screen, "Enter: save  Esc: cancel", x+12, y+70)
	if errMsg != "" {
		ebitenutil.DebugPrintAt(screen, "Error: "+errMsg, x+12, y+92)
	}
}

func brushFromEntry(entry legendEntry) brush {
	switch entry.Kind {
	case brushEraser:
		return brush{kind: brushEraser}
	case brushTile:
		return brush{kind: brushTile, letter: entry.Letter, tileKey: entry.TileKey}
	case brushMonster:
		return brush{kind: brushMonster, letter: entry.Letter, monsterKey: entry.MonsterKey, monsterName: entry.MonsterName}
	case brushNPC:
		return brush{kind: brushNPC, letter: "@", npcKey: entry.NPCKey, npcName: entry.NPCName}
	default:
		return brush{kind: brushNone}
	}
}

func pointInRect(x, y, rx, ry, rw, rh int) bool {
	return x >= rx && y >= ry && x < rx+rw && y < ry+rh
}

func trimLastRune(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= 1 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func drawCenteredLabel(screen *ebiten.Image, label string, r rect) {
	if label == "" {
		return
	}
	const charW = 7
	const charH = 13
	textW := utf8.RuneCountInString(label) * charW
	textH := charH
	x := r.x + (r.w-textW)/2
	y := r.y + (r.h-textH)/2
	if x < r.x+2 {
		x = r.x + 2
	}
	if y < r.y+2 {
		y = r.y + 2
	}
	ebitenutil.DebugPrintAt(screen, label, x, y)
}

func (v *viewer) applyBrush(m *mapInfo, tx, ty int) {
	if m == nil || m.Data == nil || v.tileManager == nil {
		return
	}

	m.Data.MonsterSpawns = removeMonsterAt(m.Data.MonsterSpawns, tx, ty)
	m.Data.NPCSpawns = removeNPCAt(m.Data.NPCSpawns, tx, ty)
	m.Data.SpecialTileSpawns = removeSpecialAt(m.Data.SpecialTileSpawns, tx, ty)

	switch v.brush.kind {
	case brushEraser:
		v.setTile(m, tx, ty, ".")
	case brushTile:
		v.setTile(m, tx, ty, v.brush.letter)
	case brushMonster:
		v.setTile(m, tx, ty, ".")
		if v.brush.monsterKey != "" {
			m.Data.MonsterSpawns = append(m.Data.MonsterSpawns, world.MonsterSpawn{
				X:          tx,
				Y:          ty,
				MonsterKey: v.brush.monsterKey,
			})
		}
	case brushNPC:
		// NPC sits on empty ground; saved as an `@` bound to the npc key.
		v.setTile(m, tx, ty, ".")
		if v.brush.npcKey != "" {
			m.Data.NPCSpawns = append(m.Data.NPCSpawns, world.NPCSpawn{
				X:      tx,
				Y:      ty,
				NPCKey: v.brush.npcKey,
			})
		}
	}
}

func (v *viewer) setTile(m *mapInfo, tx, ty int, letter string) {
	if m == nil || m.Data == nil || v.tileManager == nil {
		return
	}
	biome := ""
	if m.Config != nil {
		biome = m.Config.Biome
	}
	tileType := world.TileEmpty
	if letter != "" {
		if t, ok := v.tileManager.GetTileTypeFromLetterForBiome(letter, biome); ok {
			tileType = t
		} else if letter != "." {
			return
		}
	}

	if letter == "+" {
		if m.Data.StartX >= 0 && m.Data.StartY >= 0 && (m.Data.StartX != tx || m.Data.StartY != ty) {
			if m.Data.StartY < len(m.Data.Tiles) && m.Data.StartX < len(m.Data.Tiles[m.Data.StartY]) {
				m.Data.Tiles[m.Data.StartY][m.Data.StartX] = world.TileEmpty
			}
		}
		m.Data.StartX = tx
		m.Data.StartY = ty
	}

	if ty < 0 || ty >= len(m.Data.Tiles) || tx < 0 || tx >= len(m.Data.Tiles[ty]) {
		return
	}
	m.Data.Tiles[ty][tx] = tileType
}

func removeMonsterAt(spawns []world.MonsterSpawn, x, y int) []world.MonsterSpawn {
	if len(spawns) == 0 {
		return spawns
	}
	out := spawns[:0]
	for _, spawn := range spawns {
		if spawn.X == x && spawn.Y == y {
			continue
		}
		out = append(out, spawn)
	}
	return out
}

func removeNPCAt(spawns []world.NPCSpawn, x, y int) []world.NPCSpawn {
	if len(spawns) == 0 {
		return spawns
	}
	out := spawns[:0]
	for _, spawn := range spawns {
		if spawn.X == x && spawn.Y == y {
			continue
		}
		out = append(out, spawn)
	}
	return out
}

func removeSpecialAt(spawns []world.SpecialTileSpawn, x, y int) []world.SpecialTileSpawn {
	if len(spawns) == 0 {
		return spawns
	}
	out := spawns[:0]
	for _, spawn := range spawns {
		if spawn.X == x && spawn.Y == y {
			continue
		}
		out = append(out, spawn)
	}
	return out
}

func encodeMapLines(m *mapInfo, tm *world.TileManager) ([]string, error) {
	if m == nil || m.Data == nil {
		return nil, fmt.Errorf("no map data")
	}
	if tm == nil {
		return nil, fmt.Errorf("tile manager not initialized")
	}

	height := m.Data.Height
	width := m.Data.Width
	if height <= 0 || width <= 0 {
		return nil, fmt.Errorf("invalid map size")
	}

	grid := make([][]rune, height)
	for y := 0; y < height; y++ {
		row := make([]rune, width)
		for x := 0; x < width; x++ {
			tileType := m.Data.Tiles[y][x]
			letter := tm.GetLetterFromTileType(tileType)
			if letter == "" {
				letter = "."
			}
			row[x] = []rune(letter)[0]
		}
		grid[y] = row
	}

	monsterLetters := make(map[[2]int]string)
	if monster.MonsterConfig != nil {
		for _, spawn := range m.Data.MonsterSpawns {
			def, ok := monster.MonsterConfig.Monsters[spawn.MonsterKey]
			if !ok || def.Letter == "" {
				continue
			}
			monsterLetters[[2]int{spawn.X, spawn.Y}] = def.Letter
		}
	}

	npcByRow := make(map[int][]world.NPCSpawn)
	for _, npc := range m.Data.NPCSpawns {
		npcByRow[npc.Y] = append(npcByRow[npc.Y], npc)
	}
	for y := range npcByRow {
		sort.Slice(npcByRow[y], func(i, j int) bool { return npcByRow[y][i].X < npcByRow[y][j].X })
	}

	specialByRow := make(map[int][]world.SpecialTileSpawn)
	for _, sp := range m.Data.SpecialTileSpawns {
		specialByRow[sp.Y] = append(specialByRow[sp.Y], sp)
	}
	for y := range specialByRow {
		sort.Slice(specialByRow[y], func(i, j int) bool { return specialByRow[y][i].X < specialByRow[y][j].X })
	}

	var lines []string
	for y := 0; y < height; y++ {
		row := make([]rune, width)
		copy(row, grid[y])

		if m.Data.StartX >= 0 && m.Data.StartY == y && m.Data.StartX < width {
			row[m.Data.StartX] = '+'
		}

		for x := 0; x < width; x++ {
			if letter, ok := monsterLetters[[2]int{x, y}]; ok && letter != "" {
				row[x] = []rune(letter)[0]
			}
		}

		var defs []string
		if npcs := npcByRow[y]; len(npcs) > 0 {
			for _, npc := range npcs {
				if npc.X < 0 || npc.X >= width {
					continue
				}
				row[npc.X] = '@'
				defs = append(defs, fmt.Sprintf("[npc:%s]", npc.NPCKey))
			}
		}
		if specials := specialByRow[y]; len(specials) > 0 {
			for _, sp := range specials {
				if sp.X < 0 || sp.X >= width {
					continue
				}
				row[sp.X] = '@'
				key := sp.TileKey
				if key == "" {
					key = tm.GetTileKey(sp.TileType)
				}
				if key == "" {
					continue
				}
				defs = append(defs, fmt.Sprintf("[stile:%s]", key))
			}
		}

		line := string(row)
		if len(defs) > 0 {
			// Match the canonical format: two spaces, ">", then comma-joined defs
			// (the ">" prefixes only the first def). Loader tolerates either way.
			line += "  >" + strings.Join(defs, ", ")
		}
		lines = append(lines, line)
	}

	return lines, nil
}

func drawTileMarkerCircle(screen *ebiten.Image, originX, originY, tileSize, tx, ty int, clr color.RGBA, stroke bool) {
	if tileSize < 2 {
		return
	}
	centerX := float32(originX + tx*tileSize + tileSize/2)
	centerY := float32(originY + ty*tileSize + tileSize/2)
	radius := float32(tileSize) * 0.35
	vector.DrawFilledCircle(screen, centerX, centerY, radius, clr, true)
	if stroke {
		vector.StrokeCircle(screen, centerX, centerY, radius, 1, color.RGBA{255, 255, 255, 255}, true)
	}
}

func drawTileMarkerRect(screen *ebiten.Image, originX, originY, tileSize, tx, ty int, clr color.RGBA) {
	if tileSize < 2 {
		return
	}
	size := tileSize
	if size < 3 {
		size = 3
	}
	drawX := originX + tx*tileSize + (tileSize-size)/2
	drawY := originY + ty*tileSize + (tileSize-size)/2
	vector.FillRect(screen, float32(drawX), float32(drawY), float32(size), float32(size), clr, false)
}

func drawTileLetter(screen *ebiten.Image, originX, originY, tileSize, tx, ty int, letter string) {
	if tileSize < 6 || letter == "" {
		return
	}
	drawX := originX + tx*tileSize + 2
	drawY := originY + ty*tileSize + 1
	ebitenutil.DebugPrintAt(screen, letter, drawX, drawY)
}

// tileSwatchColor returns the schematic map color for a tile from its key +
// config, mirroring exactly how the map grid paints it. Shared by the map
// panel and the legend palette so the legend swatch matches the map. The bool
// is false when the key/data don't determine a color (caller falls back).
func tileSwatchColor(key string, data *config.TileData, floorColor color.RGBA) (color.RGBA, bool) {
	obstacle := color.RGBA{50, 50, 60, 255}
	switch key {
	case "vteleporter":
		return color.RGBA{170, 80, 200, 255}, true
	case "rteleporter":
		return color.RGBA{200, 70, 70, 255}, true
	}
	if data != nil {
		if data.RenderType == "floor_only" || data.RenderType == "flooring_object" {
			if key == "empty" {
				return floorColor, true
			}
			if data.FloorColor != [3]int{} {
				return colorFromRGB(data.FloorColor, 255), true
			}
			return floorColor, true
		}
		if data.RenderType == "environment_sprite" && data.Walkable {
			return floorColor, true
		}
		if data.Solid || !data.Walkable {
			// Use the obstacle's own wall_color so different obstacles
			// (tree vs dune vs house) are distinguishable, not a flat gray.
			if data.WallColor != [3]int{} {
				return colorFromRGB(data.WallColor, 255), true
			}
			return obstacle, true
		}
		if data.FloorColor != [3]int{} {
			return colorFromRGB(data.FloorColor, 255), true
		}
	}
	return color.RGBA{}, false
}

// floorUnderObjectColor is the ground shown under an object sprite. A tile
// that authors its own floor_color (flooring objects) keeps it; otherwise the
// ground is dynamic — the same dominant-neighbour vote the game uses for
// under-entity and inherit_floor ground — so a tree in a road patch sits on
// road, not on the biome default.
func floorUnderObjectColor(m mapInfo, tm *world.TileManager, tileDataByKey map[string]*config.TileData, tx, ty int, base color.RGBA) color.RGBA {
	tile := m.Data.Tiles[ty][tx]
	if key := tm.GetTileKey(tile); key != "" {
		if data := tileDataByKey[key]; data != nil && data.FloorColor != [3]int{} {
			return colorFromRGB(data.FloorColor, 255)
		}
	}
	// TileEmpty's authored floor_color is ignored, same as the game renderer:
	// empty ground always shows the map's default floor color.
	if t, ok := tm.DominantNeighbourFloor(m.Data.Tiles, m.Data.Width, m.Data.Height, tx, ty, nil); ok && t != world.TileEmpty {
		if data := tileDataByKey[tm.GetTileKey(t)]; data != nil && data.FloorColor != [3]int{} {
			return colorFromRGB(data.FloorColor, 255)
		}
	}
	return base
}

// effectiveFloorColor is the color the biome's '.' floor actually renders
// with on the panel: the biome '.' tile's own floor_color when it defines one
// (e.g. highlands_floor), else the map's default_floor_color. Used for cells
// and as the fill under object sprites, so both always match.
func effectiveFloorColor(m mapInfo, tm *world.TileManager, tileDataByKey map[string]*config.TileData) color.RGBA {
	def := color.RGBA{60, 180, 60, 255}
	if m.Config == nil {
		return def
	}
	def = colorFromRGB(m.Config.DefaultFloorColor, 255)
	if tm == nil {
		return def
	}
	if t, ok := tm.GetTileTypeFromLetterForBiome(".", m.Config.Biome); ok && t != world.TileEmpty {
		if data := tileDataByKey[tm.GetTileKey(t)]; data != nil && data.FloorColor != [3]int{} {
			return colorFromRGB(data.FloorColor, 255)
		}
	}
	return def
}

func getMapTileColor(tile world.TileType3D, floorColor color.RGBA, tm *world.TileManager, tileDataByKey map[string]*config.TileData) color.RGBA {
	if tm != nil {
		key := tm.GetTileKey(tile)
		if key != "" {
			if c, ok := tileSwatchColor(key, tileDataByKey[key], floorColor); ok {
				return c
			}
		}
	}

	switch tile {
	case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
		return color.RGBA{50, 50, 60, 255}
	case world.TileWater:
		return color.RGBA{40, 90, 160, 255}
	case world.TileDeepWater:
		return color.RGBA{25, 60, 120, 255}
	case world.TileVioletTeleporter:
		return color.RGBA{170, 80, 200, 255}
	case world.TileRedTeleporter:
		return color.RGBA{200, 70, 70, 255}
	case world.TileClearing:
		return color.RGBA{80, 140, 80, 255}
	default:
		return floorColor
	}
}

// readMapHeaderAndEOL reads a .map file's leading "#" comment block (the header)
// and detects its line-ending style, so saving can preserve both. Defaults to
// LF and no header if the file can't be read.
func readMapHeaderAndEOL(path string) (header []string, eol string) {
	eol = "\n"
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, eol
	}
	if strings.Contains(string(raw), "\r\n") {
		eol = "\r\n"
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			header = append(header, line)
			continue
		}
		break // header is the leading comment block only
	}
	return header, eol
}

func loadMaps(cfg *config.Config) ([]mapInfo, error) {
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		return nil, fmt.Errorf("failed to load map configs: %w", err)
	}

	keys := make([]string, 0, len(wm.MapConfigs))
	for key := range wm.MapConfigs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var maps []mapInfo
	for _, key := range keys {
		mapCfg := wm.MapConfigs[key]
		loader := world.NewMapLoaderWithBiome(cfg, mapCfg.Biome)
		mapPath := filepath.Join("assets", mapCfg.File)
		data, err := loader.LoadMap(mapPath)
		header, eol := readMapHeaderAndEOL(mapPath)
		maps = append(maps, mapInfo{
			Key:    key,
			Config: mapCfg,
			Data:   data,
			Err:    err,
			Header: header,
			EOL:    eol,
		})
	}

	return maps, nil
}

// matchesBiome reports whether a tile/monster (with the given biome scope) is
// usable in the given biome. Empty scope = universal (every biome).
func matchesBiome(scope []string, biome string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, b := range scope {
		if b == biome {
			return true
		}
	}
	return false
}

// currentBiome returns the biome of the map currently being edited.
func (v *viewer) currentBiome() string {
	if v.mapIndex >= 0 && v.mapIndex < len(v.maps) && v.maps[v.mapIndex].Config != nil {
		return v.maps[v.mapIndex].Config.Biome
	}
	return ""
}

// refreshLegend rebuilds the (biome-scoped) tile/monster palette for the
// current map. Call after any change to mapIndex.
func (v *viewer) refreshLegend() {
	v.legendLines = buildLegendEntries(v.tileManager, v.monsterCfg, v.currentBiome())
	v.legendScroll = 0
}

// legendBuildItem is a legend entry plus whether its source def is
// biome-specific (used to drop the universal fallback when a biome-specific
// def claims the same letter).
type legendBuildItem struct {
	entry    legendEntry
	specific bool
}

// emitBiomeScoped flattens per-letter build items into sorted legend entries,
// dropping universal entries for any letter that ALSO has a biome-specific one.
// This mirrors GetTileTypeFromLetterForBiome / GetMonsterByLetterForBiome,
// where a biome-specific def wins over the universal fallback for that letter —
// so the palette shows only what would actually be placed.
func emitBiomeScoped(byLetter map[string][]legendBuildItem) []legendEntry {
	letters := make([]string, 0, len(byLetter))
	for l := range byLetter {
		letters = append(letters, l)
	}
	sort.Strings(letters)
	var out []legendEntry
	for _, l := range letters {
		items := byLetter[l]
		hasSpecific := false
		for _, it := range items {
			if it.specific {
				hasSpecific = true
				break
			}
		}
		kept := make([]legendEntry, 0, len(items))
		for _, it := range items {
			if hasSpecific && !it.specific {
				continue // biome-specific def wins this letter; hide universal
			}
			kept = append(kept, it.entry)
		}
		sort.Slice(kept, func(i, j int) bool { return kept[i].Text < kept[j].Text })
		out = append(out, kept...)
	}
	return out
}

// buildLegendEntries builds the editor palette scoped to one biome: universal
// tiles/monsters plus those whose biome list contains `biome`. Other biomes'
// entries are hidden so a forest tree can't be painted into a desert map, and
// when a biome-specific def shares a letter with a universal one, only the
// biome-specific (the def that actually resolves) is shown.
func buildLegendEntries(tm *world.TileManager, mc *monster.MonsterYAMLConfig, biome string) []legendEntry {
	var entries []legendEntry
	entries = append(entries, legendEntry{Text: "Tools", IsHeader: true})
	entries = append(entries, legendEntry{Text: "Eraser", Kind: brushEraser})
	entries = append(entries, legendEntry{Text: "", IsHeader: true})

	biomeLabel := biome
	if biomeLabel == "" {
		biomeLabel = "-"
	}
	entries = append(entries, legendEntry{Text: fmt.Sprintf("Tiles - biome: %s", biomeLabel), IsHeader: true})

	tileItems := make(map[string][]legendBuildItem)
	for key, data := range tm.ListTiles() {
		letter := data.Letter
		if letter == "" {
			continue
		}
		if !matchesBiome(data.Biomes, biome) {
			continue
		}
		text := fmt.Sprintf("%s  %s (%s)", letter, key, data.Name)
		tileItems[letter] = append(tileItems[letter], legendBuildItem{
			entry: legendEntry{
				Text:    text,
				Kind:    brushTile,
				Letter:  letter,
				TileKey: key,
			},
			specific: len(data.Biomes) > 0,
		})
	}
	entries = append(entries, emitBiomeScoped(tileItems)...)

	entries = append(entries, legendEntry{Text: "", IsHeader: true})
	entries = append(entries, legendEntry{Text: "Monsters (letter -> key/name)", IsHeader: true})

	if mc != nil {
		monsterItems := make(map[string][]legendBuildItem)
		for key, def := range mc.Monsters {
			letter := def.Letter
			if letter == "" {
				continue
			}
			if !matchesBiome(def.Biomes, biome) {
				continue
			}
			text := fmt.Sprintf("%s  %s (%s)", letter, key, def.Name)
			monsterItems[letter] = append(monsterItems[letter], legendBuildItem{
				entry: legendEntry{
					Text:        text,
					Kind:        brushMonster,
					Letter:      letter,
					MonsterKey:  key,
					MonsterName: def.Name,
				},
				specific: len(def.Biomes) > 0,
			})
		}
		entries = append(entries, emitBiomeScoped(monsterItems)...)
	}

	// Special NPCs (quest givers, encounters, merchants, portals, …) — every NPC
	// from npcs.yaml is placeable. Selecting one paints an `@` bound to that NPC;
	// the eraser removes it. Not biome-scoped (any NPC can sit on any map).
	if character.NPCConfigInstance != nil && len(character.NPCConfigInstance.NPCs) > 0 {
		entries = append(entries, legendEntry{Text: "", IsHeader: true})
		entries = append(entries, legendEntry{Text: "Special NPCs (@ -> key/name)", IsHeader: true})
		npcKeys := make([]string, 0, len(character.NPCConfigInstance.NPCs))
		for key := range character.NPCConfigInstance.NPCs {
			npcKeys = append(npcKeys, key)
		}
		sort.Strings(npcKeys)
		for _, key := range npcKeys {
			data := character.NPCConfigInstance.NPCs[key]
			name := key
			if data != nil && data.Name != "" {
				name = data.Name
			}
			entries = append(entries, legendEntry{
				Text:    fmt.Sprintf("@  %s (%s)", key, name),
				Kind:    brushNPC,
				Letter:  "@",
				NPCKey:  key,
				NPCName: name,
			})
		}
	}

	entries = append(entries, legendEntry{Text: "", IsHeader: true})
	entries = append(entries, legendEntry{Text: "Notes", IsHeader: true})
	entries = append(entries, legendEntry{Text: "+ = start position", IsHeader: true})
	entries = append(entries, legendEntry{Text: "@ = NPC/special-tile placeholder in map lines", IsHeader: true})
	entries = append(entries, legendEntry{Text: "a-z = monster letters (tile underneath is empty)", IsHeader: true})

	return entries
}

func monsterLetterForKey(key string) string {
	if monster.MonsterConfig == nil {
		return ""
	}
	def, ok := monster.MonsterConfig.Monsters[key]
	if !ok {
		return ""
	}
	return def.Letter
}

func colorFromRGB(rgb [3]int, a uint8) color.RGBA {
	return color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), a}
}

func drawFilledRect(screen *ebiten.Image, x, y, w, h int, clr color.RGBA) {
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), clr, false)
}

// drawImageScaled scales src into the w×h box at (x,y). Mirrors the game's
// helper (ui_helpers.go): linear filtering when SHRINKING (mipmaps) so thin
// baked-in details/frames aren't dropped, nearest when upscaling so pixel art
// stays crisp. Used for sprite icons and portraits so the editor renders them
// exactly like the game (no "squished"/clipped look from nearest downscaling).
func drawImageScaled(dst, src *ebiten.Image, x, y, w, h int) {
	if src == nil || w <= 0 || h <= 0 {
		return
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(float64(w)/float64(sw), float64(h)/float64(sh))
	opts.GeoM.Translate(float64(x), float64(y))
	if w < sw || h < sh {
		opts.Filter = ebiten.FilterLinear
	}
	dst.DrawImage(src, opts)
}

func drawRectBorder(screen *ebiten.Image, x, y, w, h, thickness int, clr color.RGBA) {
	t := float32(thickness)
	fx := float32(x)
	fy := float32(y)
	fw := float32(w)
	fh := float32(h)
	vector.FillRect(screen, fx, fy, fw, t, clr, false)
	vector.FillRect(screen, fx, fy+fh-t, fw, t, clr, false)
	vector.FillRect(screen, fx, fy, t, fh, clr, false)
	vector.FillRect(screen, fx+fw-t, fy, t, fh, clr, false)
}

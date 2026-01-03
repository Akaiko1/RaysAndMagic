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

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	windowWidth  = 1200
	windowHeight = 800
	sidebarWidth = 300
)

type mapInfo struct {
	Key    string
	Config *config.MapConfig
	Data   *world.MapData
	Err    error
}

type viewer struct {
	maps           []mapInfo
	mapIndex       int
	legendLines    []legendEntry
	legendScroll   int
	sidebarTab     int
	tileDataByKey  map[string]*config.TileData
	tileManager    *world.TileManager
	brush          brush
	saveDialogOpen bool
	savePath       string
	saveError      string
	lastErr        string
}

const (
	tabInfo = iota
	tabLegend
)

type brushKind int

const (
	brushNone brushKind = iota
	brushTile
	brushMonster
	brushEraser
)

type brush struct {
	kind        brushKind
	letter      string
	tileKey     string
	monsterKey  string
	monsterName string
}

type legendEntry struct {
	Text        string
	Kind        brushKind
	Letter      string
	TileKey     string
	MonsterKey  string
	MonsterName string
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
	ensureRuntimeCWD()

	cfg := config.MustLoadConfig("config.yaml")

	// Initialize tile manager + configs (needed by map loader).
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		log.Printf("Warning: Failed to load tile config: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		log.Printf("Warning: Failed to load special tile config: %v", err)
	}

	monsterCfg := monster.MustLoadMonsterConfig("assets/monsters.yaml")

	maps, err := loadMaps(cfg)
	if err != nil {
		log.Printf("Warning: %v", err)
	}

	v := &viewer{
		maps:          maps,
		mapIndex:      0,
		legendLines:   buildLegendEntries(world.GlobalTileManager, monsterCfg),
		sidebarTab:    tabInfo,
		tileDataByKey: world.GlobalTileManager.ListTiles(),
		tileManager:   world.GlobalTileManager,
		brush:         brush{kind: brushEraser},
	}
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
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		if len(v.maps) > 0 {
			v.mapIndex--
			if v.mapIndex < 0 {
				v.mapIndex = len(v.maps) - 1
			}
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

	if len(v.maps) == 0 {
		msg := v.lastErr
		if msg == "" {
			msg = "no maps loaded"
		}
		ebitenutil.DebugPrintAt(screen, msg, 16, 16)
		return
	}

	m := v.maps[v.mapIndex]
	if m.Err != nil {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("map %s failed to load: %v", m.Key, m.Err), 16, 16)
		return
	}

	lay := v.computeLayout(m)

	drawMapPanel(screen, m, lay.mapAreaX, lay.mapAreaY, lay.mapAreaW, lay.mapAreaH, v.tileManager, v.tileDataByKey)
	drawToolbar(screen, lay, v.brush)
	drawSidebar(screen, m, lay.sidebarX, lay.sidebarY, sidebarWidth, lay.mapAreaH+lay.toolbarH+16, v.sidebarTab, v.legendLines, v.legendScroll, v.brush)

	if v.saveDialogOpen {
		drawSaveDialog(screen, v.savePath, v.saveError)
	}
}

func (v *viewer) Layout(_, _ int) (int, int) {
	return windowWidth, windowHeight
}

func (v *viewer) computeLayout(m mapInfo) layout {
	padding := 16
	toolbarH := 36
	mapAreaW := windowWidth - sidebarWidth - padding*3
	mapAreaH := windowHeight - padding*2 - toolbarH - padding
	mapAreaX := padding
	mapAreaY := padding
	toolbarX := mapAreaX
	toolbarY := mapAreaY + mapAreaH + padding
	toolbarW := mapAreaW
	sidebarX := mapAreaX + mapAreaW + padding
	sidebarY := padding
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

func drawMapPanel(screen *ebiten.Image, m mapInfo, x, y, w, h int, tm *world.TileManager, tileDataByKey map[string]*config.TileData) {
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

	floorColor := colorFromRGB(m.Config.DefaultFloorColor, 255)

	for ty := 0; ty < worldH; ty++ {
		for tx := 0; tx < worldW; tx++ {
			tile := m.Data.Tiles[ty][tx]
			cellColor := getMapTileColor(tile, floorColor, tm, tileDataByKey)
			drawX := originX + tx*tileSize
			drawY := originY + ty*tileSize
			vector.DrawFilledRect(screen, float32(drawX), float32(drawY), float32(tileSize), float32(tileSize), cellColor, false)
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

func drawSidebar(screen *ebiten.Image, m mapInfo, x, y, w, h int, tab int, legendLines []legendEntry, scroll int, currentBrush brush) {
	drawFilledRect(screen, x, y, w, h, color.RGBA{18, 18, 26, 255})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{70, 70, 90, 255})

	tabHeight := 24
	drawSidebarTabs(screen, x, y, w, tabHeight, tab)
	row := y + tabHeight + 12

	if tab == tabLegend {
		drawLegendList(screen, x, row, w, h-(row-y)-12, legendLines, scroll, currentBrush)
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

func drawLegendList(screen *ebiten.Image, x, y, w, h int, lines []legendEntry, scroll int, currentBrush brush) {
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
		ebitenutil.DebugPrintAt(screen, entry.Text, x+10, drawY)
	}
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
	lines, err := encodeMapLines(&m, v.tileManager)
	if err != nil {
		return err
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
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
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
					key = specialTileKeyFromType(sp.TileType)
				}
				if key == "" {
					continue
				}
				defs = append(defs, fmt.Sprintf("[stile:%s]", key))
			}
		}

		line := string(row)
		if len(defs) > 0 {
			line += "  > " + strings.Join(defs, ", ")
		}
		lines = append(lines, line)
	}

	return lines, nil
}

func specialTileKeyFromType(tileType world.TileType3D) string {
	switch tileType {
	case world.TileVioletTeleporter:
		return "vteleporter"
	case world.TileRedTeleporter:
		return "rteleporter"
	default:
		return ""
	}
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
	vector.DrawFilledRect(screen, float32(drawX), float32(drawY), float32(size), float32(size), clr, false)
}

func drawTileLetter(screen *ebiten.Image, originX, originY, tileSize, tx, ty int, letter string) {
	if tileSize < 6 || letter == "" {
		return
	}
	drawX := originX + tx*tileSize + 2
	drawY := originY + ty*tileSize + 1
	ebitenutil.DebugPrintAt(screen, letter, drawX, drawY)
}

func getMapTileColor(tile world.TileType3D, floorColor color.RGBA, tm *world.TileManager, tileDataByKey map[string]*config.TileData) color.RGBA {
	obstacle := color.RGBA{50, 50, 60, 255}
	if tm != nil {
		key := tm.GetTileKey(tile)
		if key != "" {
			if key == "vteleporter" {
				return color.RGBA{170, 80, 200, 255}
			}
			if key == "rteleporter" {
				return color.RGBA{200, 70, 70, 255}
			}
			if data, ok := tileDataByKey[key]; ok && data != nil {
				if data.RenderType == "floor_only" || data.RenderType == "flooring_object" {
					if key == "empty" {
						return floorColor
					}
					if data.FloorColor != [3]int{} {
						return colorFromRGB(data.FloorColor, 255)
					}
					return floorColor
				}
				if data.RenderType == "environment_sprite" && data.Walkable {
					return floorColor
				}
				if data.Solid || !data.Walkable {
					return obstacle
				}
				if data.FloorColor != [3]int{} {
					return colorFromRGB(data.FloorColor, 255)
				}
			}
		}
	}

	switch tile {
	case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
		return obstacle
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
		maps = append(maps, mapInfo{
			Key:    key,
			Config: mapCfg,
			Data:   data,
			Err:    err,
		})
	}

	return maps, nil
}

func buildLegendEntries(tm *world.TileManager, mc *monster.MonsterYAMLConfig) []legendEntry {
	var entries []legendEntry
	entries = append(entries, legendEntry{Text: "Tools", IsHeader: true})
	entries = append(entries, legendEntry{Text: "Eraser", Kind: brushEraser})
	entries = append(entries, legendEntry{Text: "", IsHeader: true})

	entries = append(entries, legendEntry{Text: "Tiles (letter -> key/name [biomes])", IsHeader: true})

	tileEntries := make(map[string][]legendEntry)
	for key, data := range tm.ListTiles() {
		letter := data.Letter
		if letter == "" {
			continue
		}
		biomes := "all"
		if len(data.Biomes) > 0 {
			biomes = strings.Join(data.Biomes, ",")
		}
		text := fmt.Sprintf("%s -> %s (%s) [%s]", letter, key, data.Name, biomes)
		tileEntries[letter] = append(tileEntries[letter], legendEntry{
			Text:    text,
			Kind:    brushTile,
			Letter:  letter,
			TileKey: key,
		})
	}

	letters := make([]string, 0, len(tileEntries))
	for letter := range tileEntries {
		letters = append(letters, letter)
	}
	sort.Strings(letters)
	for _, letter := range letters {
		entriesForLetter := tileEntries[letter]
		sort.Slice(entriesForLetter, func(i, j int) bool {
			return entriesForLetter[i].Text < entriesForLetter[j].Text
		})
		entries = append(entries, entriesForLetter...)
	}

	entries = append(entries, legendEntry{Text: "", IsHeader: true})
	entries = append(entries, legendEntry{Text: "Monsters (letter -> key/name)", IsHeader: true})

	if mc != nil {
		monsterEntries := make(map[string][]legendEntry)
		for key, def := range mc.Monsters {
			letter := def.Letter
			if letter == "" {
				continue
			}
			text := fmt.Sprintf("%s -> %s (%s)", letter, key, def.Name)
			monsterEntries[letter] = append(monsterEntries[letter], legendEntry{
				Text:        text,
				Kind:        brushMonster,
				Letter:      letter,
				MonsterKey:  key,
				MonsterName: def.Name,
			})
		}
		monsterLetters := make([]string, 0, len(monsterEntries))
		for letter := range monsterEntries {
			monsterLetters = append(monsterLetters, letter)
		}
		sort.Strings(monsterLetters)
		for _, letter := range monsterLetters {
			entriesForLetter := monsterEntries[letter]
			sort.Slice(entriesForLetter, func(i, j int) bool {
				return entriesForLetter[i].Text < entriesForLetter[j].Text
			})
			entries = append(entries, entriesForLetter...)
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
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), clr, false)
}

func drawRectBorder(screen *ebiten.Image, x, y, w, h, thickness int, clr color.RGBA) {
	t := float32(thickness)
	fx := float32(x)
	fy := float32(y)
	fw := float32(w)
	fh := float32(h)
	vector.DrawFilledRect(screen, fx, fy, fw, t, clr, false)
	vector.DrawFilledRect(screen, fx, fy+fh-t, fw, t, clr, false)
	vector.DrawFilledRect(screen, fx, fy, t, fh, clr, false)
	vector.DrawFilledRect(screen, fx+fw-t, fy, t, fh, clr, false)
}

func ensureRuntimeCWD() {
	if _, err := os.Stat("config.yaml"); err == nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	execDir := filepath.Dir(exe)
	_ = os.Chdir(execDir)
}

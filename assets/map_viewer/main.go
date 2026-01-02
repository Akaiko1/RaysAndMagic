package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	maps          []mapInfo
	mapIndex      int
	legendLines   []string
	legendScroll  int
	sidebarTab    int
	tileDataByKey map[string]*config.TileData
	tileManager   *world.TileManager
	lastErr       string
}

const (
	tabInfo = iota
	tabLegend
)

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
		legendLines:   buildLegendLines(world.GlobalTileManager, monsterCfg),
		sidebarTab:    tabInfo,
		tileDataByKey: world.GlobalTileManager.ListTiles(),
		tileManager:   world.GlobalTileManager,
	}
	if len(maps) == 0 {
		v.lastErr = "no maps loaded"
	}

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("RaysAndMagic Map Viewer")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(v); err != nil {
		log.Fatal(err)
	}
}

func (v *viewer) Update() error {
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

	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()

	padding := 16
	mapAreaW := screenW - sidebarWidth - padding*3
	mapAreaH := screenH - padding*2
	mapAreaX := padding
	mapAreaY := padding
	sidebarX := mapAreaX + mapAreaW + padding
	sidebarY := padding

	drawMapPanel(screen, m, mapAreaX, mapAreaY, mapAreaW, mapAreaH, v.tileManager, v.tileDataByKey)
	drawSidebar(screen, m, sidebarX, sidebarY, sidebarWidth, mapAreaH, v.sidebarTab, v.legendLines, v.legendScroll)
}

func (v *viewer) Layout(_, _ int) (int, int) {
	return windowWidth, windowHeight
}

func (v *viewer) maxLegendScroll() int {
	lineHeight := 14
	padding := 12
	tabHeight := 24
	sidebarHeight := windowHeight - padding*2
	contentHeight := sidebarHeight - tabHeight - padding
	if contentHeight < lineHeight {
		contentHeight = lineHeight
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

func drawSidebar(screen *ebiten.Image, m mapInfo, x, y, w, h int, tab int, legendLines []string, scroll int) {
	drawFilledRect(screen, x, y, w, h, color.RGBA{18, 18, 26, 255})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{70, 70, 90, 255})

	tabHeight := 24
	drawSidebarTabs(screen, x, y, w, tabHeight, tab)
	row := y + tabHeight + 12

	if tab == tabLegend {
		drawLegendList(screen, x, row, w, h-(row-y)-12, legendLines, scroll)
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
	ebitenutil.DebugPrintAt(screen, "Info (1)", x+10, y+6)
	ebitenutil.DebugPrintAt(screen, "Legend (2)", x+tabW+10, y+6)
}

func drawLegendList(screen *ebiten.Image, x, y, w, h int, lines []string, scroll int) {
	lineHeight := 14
	startY := y - scroll
	for i, line := range lines {
		drawY := startY + i*lineHeight
		if drawY < y-lineHeight {
			continue
		}
		if drawY > y+h-lineHeight {
			break
		}
		ebitenutil.DebugPrintAt(screen, line, x+10, drawY)
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

func buildLegendLines(tm *world.TileManager, mc *monster.MonsterYAMLConfig) []string {
	var lines []string
	lines = append(lines, "Tiles (letter -> key/name [biomes])")
	lines = append(lines, "---------------------------------")

	tileEntries := make(map[string][]string)
	for key, data := range tm.ListTiles() {
		letter := data.Letter
		if letter == "" {
			continue
		}
		biomes := "all"
		if len(data.Biomes) > 0 {
			biomes = strings.Join(data.Biomes, ",")
		}
		entry := fmt.Sprintf("%s (%s) [%s]", key, data.Name, biomes)
		tileEntries[letter] = append(tileEntries[letter], entry)
	}

	letters := make([]string, 0, len(tileEntries))
	for letter := range tileEntries {
		letters = append(letters, letter)
	}
	sort.Strings(letters)
	for _, letter := range letters {
		entries := tileEntries[letter]
		sort.Strings(entries)
		for _, entry := range entries {
			lines = append(lines, fmt.Sprintf("%s -> %s", letter, entry))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "Monsters (letter -> key/name)")
	lines = append(lines, "------------------------------")
	if mc != nil {
		monsterEntries := make(map[string][]string)
		for key, def := range mc.Monsters {
			letter := def.Letter
			if letter == "" {
				continue
			}
			monsterEntries[letter] = append(monsterEntries[letter], fmt.Sprintf("%s (%s)", key, def.Name))
		}
		monsterLetters := make([]string, 0, len(monsterEntries))
		for letter := range monsterEntries {
			monsterLetters = append(monsterLetters, letter)
		}
		sort.Strings(monsterLetters)
		for _, letter := range monsterLetters {
			entries := monsterEntries[letter]
			sort.Strings(entries)
			for _, entry := range entries {
				lines = append(lines, fmt.Sprintf("%s -> %s", letter, entry))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, "Notes")
	lines = append(lines, "-----")
	lines = append(lines, "+ = start position")
	lines = append(lines, "@ = NPC/special-tile placeholder in map lines")
	lines = append(lines, "a-z = monster letters (tile underneath is empty)")

	return lines
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

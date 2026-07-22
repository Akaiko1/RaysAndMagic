package main

// Open World editor page (F9) + open-world connection highlights on the Maps
// page. The page edits assets/open_world.yaml live: drag placed maps, rotate
// (R) / mirror (M) them, and every change re-runs the REAL stitcher for a
// preview + fail-fast validation, so what you see is exactly what the game
// builds. Connections stay authored in each map's own local terms, which is
// also why the Maps-page highlights need no transform.

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"sort"

	"ugataima/internal/config"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"gopkg.in/yaml.v3"
)

const openWorldConfigPath = "assets/open_world.yaml"

type openWorldPage struct {
	loaded  bool
	err     string // stitch/load error (red)
	status  string // last action feedback (green)
	unsaved bool

	preview *ebiten.Image           // 1 px per tile, from the last good stitch
	regions []world.OpenWorldRegion // from the last good stitch
	gridW   int
	gridH   int

	selected string
	dragging bool
	dragKey  string
	dragOrig config.OpenWorldPlacement
	dragMX   int
	dragMY   int
	dirty    bool // placements changed since the last stitch
}

var owPage openWorldPage

// owMapDims returns a placement's LOCAL map size from the loaded maps.
func (v *viewer) owMapDims(key string) (int, int, bool) {
	for _, m := range v.maps {
		if m.Key == key && m.Data != nil {
			return m.Data.Width, m.Data.Height, true
		}
	}
	return 0, 0, false
}

// owPlacedRect returns a placement's footprint in unified tiles (respecting
// orient), straight from the edited config - valid even mid-drag or when the
// last stitch failed.
func (v *viewer) owPlacedRect(key string) (x, y, w, h int, ok bool) {
	if v.owc == nil {
		return 0, 0, 0, 0, false
	}
	p, exists := v.owc.Placements[key]
	if !exists {
		return 0, 0, 0, 0, false
	}
	lw, lh, ok := v.owMapDims(key)
	if !ok {
		return 0, 0, 0, 0, false
	}
	switch p.Orient {
	case "rot90", "rot270":
		lw, lh = lh, lw
	}
	return p.X, p.Y, lw, lh, true
}

// owRebuild runs the real stitcher on a throwaway world manager and refreshes
// the preview. Errors land in owPage.err exactly as they would at game boot.
func (v *viewer) owRebuild() {
	owPage.dirty = false
	owPage.preview = nil
	owPage.regions = nil
	owPage.err = ""
	if v.owc == nil {
		owPage.err = "no " + openWorldConfigPath
		return
	}
	wm := world.NewWorldManager(v.cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		owPage.err = err.Error()
		return
	}
	wm.SetOpenWorldConfig(v.owc)
	if err := wm.LoadAllMaps(); err != nil {
		owPage.err = err.Error()
		return
	}
	ow := wm.OpenWorld
	if ow == nil {
		owPage.err = "stitcher produced no world"
		return
	}
	owPage.gridW, owPage.gridH = ow.Width, ow.Height
	owPage.regions = append([]world.OpenWorldRegion(nil), wm.OpenWorldRegions...)

	img := image.NewRGBA(image.Rect(0, 0, ow.Width, ow.Height))
	voidColor := color.RGBA{12, 12, 18, 255}
	for ty := 0; ty < ow.Height; ty++ {
		for tx := 0; tx < ow.Width; tx++ {
			floor := color.RGBA{60, 120, 60, 255}
			if mc := wm.MapConfigAtTile(tx, ty); mc != nil {
				floor = color.RGBA{uint8(mc.DefaultFloorColor[0]), uint8(mc.DefaultFloorColor[1]), uint8(mc.DefaultFloorColor[2]), 255}
			} else {
				img.SetRGBA(tx, ty, voidColor)
				continue
			}
			img.SetRGBA(tx, ty, getMapTileColor(ow.Tiles[ty][tx], floor, v.tileManager, v.tileDataByKey))
		}
	}
	owPage.preview = ebiten.NewImageFromImage(img)
}

// owViewport is the preview's on-screen placement: fit-to-panel scale (>=2 px
// per tile when it fits) and the panel rect left of the side panel.
func (v *viewer) owViewport() (panelX, panelY, panelW, panelH int, scale float64, viewX, viewY int) {
	panelX, panelY = 8, pageBarHeight+8
	panelW = windowWidth - sidebarWidth - 24
	panelH = windowHeight - pageBarHeight - 16
	gw, gh := owPage.gridW, owPage.gridH
	if (gw <= 0 || gh <= 0) && v.owc != nil {
		// No successful stitch yet: size the viewport around the raw placements.
		for key := range v.owc.Placements {
			if x, y, w, h, ok := v.owPlacedRect(key); ok {
				if x+w > gw {
					gw = x + w
				}
				if y+h > gh {
					gh = y + h
				}
			}
		}
	}
	if gw <= 0 || gh <= 0 {
		gw, gh = 1, 1
	}
	scale = float64(panelW-16) / float64(gw)
	if s := float64(panelH-16) / float64(gh); s < scale {
		scale = s
	}
	if scale < 1 {
		scale = 1
	}
	viewX = panelX + 8
	viewY = panelY + 8
	return
}

func (v *viewer) updateOpenWorldPage() {
	if !owPage.loaded {
		owPage.loaded = true
		if v.owc == nil {
			if owc, err := config.LoadOpenWorldConfig(openWorldConfigPath); err == nil {
				v.owc = owc
			} else {
				owPage.err = err.Error()
			}
		}
		owPage.dirty = true
	}
	if v.owc == nil {
		return
	}
	if owPage.dirty && !owPage.dragging {
		v.owRebuild()
	}

	_, _, _, _, scale, viewX, viewY := v.owViewport()
	mx, my := ebiten.CursorPosition()
	tileAt := func() (int, int) {
		return int(float64(mx-viewX) / scale), int(float64(my-viewY) / scale)
	}

	// Selection + drag start.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && my > pageBarHeight {
		tx, ty := tileAt()
		hit := ""
		for key := range v.owc.Placements {
			if x, y, w, h, ok := v.owPlacedRect(key); ok && tx >= x && tx < x+w && ty >= y && ty < y+h {
				hit = key
			}
		}
		if hit != "" {
			owPage.selected = hit
			owPage.dragging = true
			owPage.dragKey = hit
			owPage.dragOrig = v.owc.Placements[hit]
			owPage.dragMX, owPage.dragMY = mx, my
		}
	}
	if owPage.dragging {
		p := owPage.dragOrig
		p.X += int(float64(mx-owPage.dragMX) / scale)
		p.Y += int(float64(my-owPage.dragMY) / scale)
		if p.X < 0 {
			p.X = 0
		}
		if p.Y < 0 {
			p.Y = 0
		}
		if v.owc.Placements[owPage.dragKey] != p {
			v.owc.Placements[owPage.dragKey] = p
			owPage.unsaved = true
			owPage.status = ""
		}
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			owPage.dragging = false
			owPage.dirty = true
		}
		return
	}

	// Keyboard editing of the selected placement.
	if owPage.selected != "" {
		if p, ok := v.owc.Placements[owPage.selected]; ok {
			orig := p
			if inpututil.IsKeyJustPressed(ebiten.KeyR) {
				p.Orient = nextInCycle(p.Orient, []string{"", "rot90", "rot180", "rot270"})
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyM) {
				p.Orient = nextInCycle(p.Orient, []string{"", "mirror_x", "mirror_y"})
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyLeft) && p.X > 0 {
				p.X--
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
				p.X++
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyUp) && p.Y > 0 {
				p.Y--
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
				p.Y++
			}
			if p != orig {
				v.owc.Placements[owPage.selected] = p
				owPage.dirty = true
				owPage.unsaved = true
				owPage.status = ""
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if err := v.owSave(); err != nil {
			owPage.err = "save: " + err.Error()
		} else {
			owPage.unsaved = false
			owPage.status = "saved " + openWorldConfigPath
		}
	}
}

// nextInCycle advances a value through cycle (values outside it restart at
// the second element, so R after a mirror begins rotating cleanly).
func nextInCycle(cur string, cycle []string) string {
	if cur == "none" {
		cur = ""
	}
	for i, val := range cycle {
		if val == cur {
			return cycle[(i+1)%len(cycle)]
		}
	}
	return cycle[1]
}

// owSave normalizes placements (shift so the layout starts at 0,0) and writes
// the config back. Comments in the file are replaced by a generated header -
// the editor is the authoring tool for this file.
func (v *viewer) owSave() error {
	minX, minY := -1, -1
	for _, p := range v.owc.Placements {
		if minX < 0 || p.X < minX {
			minX = p.X
		}
		if minY < 0 || p.Y < minY {
			minY = p.Y
		}
	}
	if minX > 0 || minY > 0 {
		for key, p := range v.owc.Placements {
			p.X -= minX
			p.Y -= minY
			v.owc.Placements[key] = p
		}
		owPage.dirty = true
	}
	body, err := yaml.Marshal(v.owc)
	if err != nil {
		return err
	}
	header := "# Open-world stitching rules (see CLAUDE.md \"Unified Open World\").\n" +
		"# Edited by the map editor's Open World tab (F9): drag maps, R rotates,\n" +
		"# M mirrors, S saves. Placements are unified tile offsets with optional\n" +
		"# orient; connections are authored in each map's own LOCAL edge/at terms\n" +
		"# and carve straight 2-tile passes when placed edges align (auto-routed\n" +
		"# L/Z canyons otherwise). removals strip split-world travel devices.\n"
	return os.WriteFile(openWorldConfigPath, append([]byte(header), body...), 0o644)
}

func (v *viewer) drawOpenWorldPage(screen *ebiten.Image) {
	panelX, panelY, panelW, panelH, scale, viewX, viewY := v.owViewport()
	drawFilledRect(screen, panelX, panelY, panelW, panelH, color.RGBA{20, 20, 35, 255})
	drawRectBorder(screen, panelX, panelY, panelW, panelH, 2, color.RGBA{70, 70, 90, 255})

	if v.owc == nil {
		ebitenutil.DebugPrintAt(screen, "no open_world.yaml: "+owPage.err, panelX+12, panelY+12)
		return
	}

	if owPage.preview != nil {
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(scale, scale)
		opts.GeoM.Translate(float64(viewX), float64(viewY))
		if owPage.dragging || owPage.dirty {
			opts.ColorScale.Scale(0.45, 0.45, 0.45, 1) // stale while editing
		}
		screen.DrawImage(owPage.preview, opts)
	}

	// Region frames + labels from the LIVE config (correct mid-drag).
	keys := make([]string, 0, len(v.owc.Placements))
	for key := range v.owc.Placements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		x, y, w, h, ok := v.owPlacedRect(key)
		if !ok {
			continue
		}
		px := float32(viewX) + float32(float64(x)*scale)
		py := float32(viewY) + float32(float64(y)*scale)
		pw := float32(float64(w) * scale)
		ph := float32(float64(h) * scale)
		frame := color.RGBA{110, 110, 130, 255}
		if key == owPage.selected {
			frame = color.RGBA{255, 220, 80, 255}
		}
		vector.StrokeRect(screen, px, py, pw, ph, 2, frame, false)
		label := key
		if p := v.owc.Placements[key]; p.Orient != "" && p.Orient != "none" {
			label += " [" + p.Orient + "]"
		}
		ebitenutil.DebugPrintAt(screen, label, int(px)+4, int(py)+4)
	}

	// Connection openings from the last good stitch (placed via the real
	// transform math).
	for _, conn := range v.owc.Connections {
		for _, side := range []config.OpenWorldPortalSide{conn.From, conn.To} {
			r := owRegionByKey(owPage.regions, side.Map)
			if r == nil {
				continue
			}
			for _, c := range r.OpeningCells(side, conn.Width) {
				px := float32(viewX) + float32(float64(c[0])*scale)
				py := float32(viewY) + float32(float64(c[1])*scale)
				vector.FillRect(screen, px, py, float32(scale), float32(scale), color.RGBA{255, 140, 40, 230}, false)
			}
		}
	}

	v.drawOpenWorldSidebar(screen, panelX+panelW+8)
}

func owRegionByKey(regions []world.OpenWorldRegion, key string) *world.OpenWorldRegion {
	for i := range regions {
		if regions[i].MapKey == key {
			return &regions[i]
		}
	}
	return nil
}

func (v *viewer) drawOpenWorldSidebar(screen *ebiten.Image, x int) {
	y := pageBarHeight + 8
	drawFilledRect(screen, x, y, sidebarWidth-16, windowHeight-y-8, color.RGBA{25, 25, 40, 255})
	drawRectBorder(screen, x, y, sidebarWidth-16, windowHeight-y-8, 2, color.RGBA{70, 70, 90, 255})
	line := func(s string) {
		ebitenutil.DebugPrintAt(screen, s, x+10, y+8)
		y += 16
	}
	line("OPEN WORLD EDITOR")
	line(fmt.Sprintf("grid %dx%d", owPage.gridW, owPage.gridH))
	line("")
	line("click map: select")
	line("drag: move (snaps to tiles)")
	line("arrows: nudge 1 tile")
	line("R: rotate  M: mirror")
	line("S: save open_world.yaml")
	line("")
	if owPage.selected != "" {
		p := v.owc.Placements[owPage.selected]
		orient := p.Orient
		if orient == "" {
			orient = "none"
		}
		line("selected: " + owPage.selected)
		line(fmt.Sprintf("  at (%d,%d) %s", p.X, p.Y, orient))
	} else {
		line("selected: -")
	}
	line("")
	if owPage.err != "" {
		for _, l := range wrapTooltipLines("STITCH ERROR: "+owPage.err, 44) {
			line(l)
		}
	} else if owPage.dirty || owPage.dragging {
		line("editing... (stitch on release)")
	} else {
		line("stitch OK")
	}
	if owPage.unsaved {
		line("UNSAVED CHANGES (S to save)")
	} else if owPage.status != "" {
		line(owPage.status)
	}
}

// drawOpenWorldHighlights marks, on the regular Maps page, the tiles a map
// contributes to open-world passages (connection openings, in the map's own
// LOCAL coordinates - placement orientation does not apply here).
func (v *viewer) drawOpenWorldHighlights(screen *ebiten.Image, m mapInfo, lay layout) {
	if v.owc == nil || m.Data == nil {
		return
	}
	clip := screen.SubImage(image.Rect(lay.mapAreaX+2, lay.mapAreaY+2, lay.mapAreaX+lay.mapAreaW-2, lay.mapAreaY+lay.mapAreaH-2)).(*ebiten.Image)
	for _, conn := range v.owc.Connections {
		for si, side := range []config.OpenWorldPortalSide{conn.From, conn.To} {
			if side.Map != m.Key {
				continue
			}
			partner := conn.To.Map
			if si == 1 {
				partner = conn.From.Map
			}
			depth := side.Depth
			if depth == 0 {
				depth = 1
			}
			var labelX, labelY int
			for i := 0; i < conn.Width; i++ {
				for d := 0; d < depth; d++ {
					var tx, ty int
					switch side.Edge {
					case "north":
						tx, ty = side.At+i, d
					case "south":
						tx, ty = side.At+i, m.Data.Height-1-d
					case "west":
						tx, ty = d, side.At+i
					default: // east
						tx, ty = m.Data.Width-1-d, side.At+i
					}
					px := float32(lay.originX + tx*lay.tileSize)
					py := float32(lay.originY + ty*lay.tileSize)
					vector.FillRect(clip, px, py, float32(lay.tileSize), float32(lay.tileSize), color.RGBA{255, 140, 40, 110}, false)
					vector.StrokeRect(clip, px, py, float32(lay.tileSize), float32(lay.tileSize), 1, color.RGBA{255, 170, 60, 255}, false)
					if i == 0 && d == 0 {
						labelX, labelY = int(px), int(py)
					}
				}
			}
			ebitenutil.DebugPrintAt(clip, "OW> "+partner, labelX+2, labelY-14)
		}
	}
}

package game

import (
	"image"
	"image/color"
	"math"
	"strconv"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// The paper is artwork; every geographic mark comes from the live world.
// Four source pixels per tile leave room for ink edges without a screen-sized
// render target. The cached plate is rebuilt only when its terrain changes.
const atlasTilePixels = 4

var atlasInk = color.RGBA{77, 58, 36, 255}
var atlasGold = color.RGBA{225, 194, 127, 255}
var atlasPaper = color.RGBA{233, 215, 175, 255}
var atlasParty = color.RGBA{58, 219, 231, 255}
var atlasPeople = color.RGBA{221, 160, 57, 255}
var atlasPortal = color.RGBA{153, 106, 206, 255}
var atlasQuest = [3]color.RGBA{{223, 74, 55, 255}, {59, 160, 92, 255}, {58, 123, 214, 255}}

type worldAtlas struct {
	world                           *world.World3D
	tiles                           *world.TileManager
	manager                         *world.WorldManager
	terrain                         *ebiten.Image
	terrainBounds                   image.Rectangle
	hash                            uint64
	observedHash                    uint64
	terrainChecked                  bool
	region                          string
	page                            int
	zoom, panX, panY                float64
	dragging, pressed, pointerReady bool
	lastX, lastY                    int
	tileBuckets                     map[atlasTileMark]bool
	bucketMarks                     []atlasTileMark
	bucketStride                    int
	tileMarks                       []atlasTileMark
	portals                         []image.Point
}

type atlasTileMark struct {
	point image.Point
	icon  string
}

func (a *worldAtlas) release() {
	if a.terrain != nil {
		a.terrain.Deallocate()
	}
	*a = worldAtlas{}
}

func atlasRegions(w *world.World3D) []world.OpenWorldRegion {
	wm := world.GlobalWorldManager
	if wm != nil && wm.OpenWorld == w {
		return wm.OpenWorldRegions
	}
	return nil
}

func atlasHasTile(w *world.World3D, x, y int) bool {
	if x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return false
	}
	wm := world.GlobalWorldManager
	return wm == nil || wm.OpenWorld != w || wm.OpenWorldRegionAtTile(x, y) != nil
}

func atlasTerrainHash(w *world.World3D) uint64 {
	h := uint64(14695981039346656037)
	add := func(n int) { h = (h ^ uint64(n)) * 1099511628211 }
	add(w.Width)
	add(w.Height)
	for _, row := range w.Tiles {
		for _, tile := range row {
			add(int(tile))
		}
	}
	for _, r := range atlasRegions(w) {
		add(r.OffsetX)
		add(r.OffsetY)
		add(r.Width)
		add(r.Height)
		for _, c := range r.MapKey {
			add(int(c))
		}
	}
	return h
}

func atlasMix(c [3]int, ink color.RGBA, weight int) color.RGBA {
	return color.RGBA{
		uint8((c[0]*weight + int(ink.R)*(4-weight)) / 4),
		uint8((c[1]*weight + int(ink.G)*(4-weight)) / 4),
		uint8((c[2]*weight + int(ink.B)*(4-weight)) / 4), 255,
	}
}

func (ui *UISystem) atlasTerrain(bounds image.Rectangle) *ebiten.Image {
	g, a := ui.game, &ui.atlas
	w := g.world
	// The modal pauses world mutation. Inspect terrain once per opening, not
	// once per frame; world/manager replacement also releases this cache.
	if !a.terrainChecked {
		a.observedHash = atlasTerrainHash(w)
		a.terrainChecked = true
	}
	hash := a.observedHash
	if a.terrain != nil && a.tiles == world.GlobalTileManager && a.hash == hash && a.terrainBounds == bounds {
		return a.terrain
	}
	a.world, a.manager, a.tiles, a.hash = w, world.GlobalWorldManager, world.GlobalTileManager, hash
	a.terrainBounds = bounds
	a.tileMarks, a.portals = a.tileMarks[:0], a.portals[:0]
	a.bucketStride = 0
	// A separate plate for the selected bounds prevents linear filtering from
	// sampling a neighbouring region across a subimage's transparent edge.
	cpu := image.NewRGBA(image.Rect(0, 0, bounds.Dx()*atlasTilePixels, bounds.Dy()*atlasTilePixels))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			tile := w.Tiles[y][x]
			if !atlasHasTile(w, x, y) {
				continue
			}
			fc := g.floorColorForTile(x, y, [3]int{60, 110, 60})
			c := atlasMix(fc, atlasPaper, 2)
			solid, natural, water := false, false, false
			if a.tiles != nil {
				if td := a.tiles.GetTileData(tile); td != nil {
					solid = td.Solid
					natural = td.RenderType == "crossed_standee"
					if solid {
						c = atlasMix(config.TileMapColor(td), atlasInk, 2)
					}
				}
			}
			switch tile {
			case world.TileWater, world.TileDeepWater, world.TileForestStream:
				water = true
				c = color.RGBA{99, 143, 153, 255}
				if tile == world.TileDeepWater {
					c = color.RGBA{66, 105, 122, 255}
				}
			case world.TileVioletTeleporter, world.TileRedTeleporter:
				c = atlasPortal
			}
			if a.tiles != nil {
				if icon := g.atlasStyle.Tiles[a.tiles.GetTileKey(tile)]; icon != "" {
					a.tileMarks = append(a.tileMarks, atlasTileMark{image.Pt(x, y), icon})
				}
			}
			if tile == world.TileVioletTeleporter || tile == world.TileRedTeleporter {
				a.portals = append(a.portals, image.Pt(x, y))
			}
			px, py := (x-bounds.Min.X)*atlasTilePixels, (y-bounds.Min.Y)*atlasTilePixels
			for iy := range atlasTilePixels {
				for ix := range atlasTilePixels {
					pixel := c
					// Stable engraving, never random per frame. Only actual tile
					// boundaries and blockers receive ink; routes stay legible.
					if solid && (iy == 0 || ix == 0) {
						pixel = atlasMix([3]int{int(c.R), int(c.G), int(c.B)}, atlasInk, 2)
					}
					if natural && solid && iy == 2 && (ix == 1 || ix == 2) {
						pixel = atlasMix(fc, atlasPaper, 2)
					}
					if water && iy == 2 && (x+y)%3 == 0 && ix > 0 && ix < 3 {
						pixel = color.RGBA{133, 168, 169, 255}
					}
					if (iy == 0 && !atlasHasTile(w, x, y-1)) || (ix == 0 && !atlasHasTile(w, x-1, y)) ||
						(iy == 3 && !atlasHasTile(w, x, y+1)) || (ix == 3 && !atlasHasTile(w, x+1, y)) {
						pixel = atlasInk
					}
					cpu.SetRGBA(px+ix, py+iy, pixel)
				}
			}
		}
	}
	if a.terrain == nil || a.terrain.Bounds() != cpu.Bounds() {
		if a.terrain != nil {
			a.terrain.Deallocate()
		}
		a.terrain = ebiten.NewImage(cpu.Bounds().Dx(), cpu.Bounds().Dy())
	}
	a.terrain.WritePixels(cpu.Pix)
	return a.terrain
}

type atlasProjection struct {
	bounds      image.Rectangle
	x, y, scale float64
}

func fitAtlas(view layoutRect, bounds image.Rectangle) atlasProjection {
	scale := math.Min(float64(max(1, view.w-32))/float64(max(1, bounds.Dx())), float64(max(1, view.h-32))/float64(max(1, bounds.Dy())))
	return atlasProjection{bounds, float64(view.x) + (float64(view.w)-float64(bounds.Dx())*scale)/2,
		float64(view.y) + (float64(view.h)-float64(bounds.Dy())*scale)/2, scale}
}

func (p atlasProjection) point(x, y int) (float32, float32, bool) {
	return float32(p.x + (float64(x-p.bounds.Min.X)+0.5)*p.scale), float32(p.y + (float64(y-p.bounds.Min.Y)+0.5)*p.scale), image.Pt(x, y).In(p.bounds)
}

func (ui *UISystem) atlasBounds() image.Rectangle {
	w := ui.game.world
	if ui.atlas.world != w || ui.atlas.manager != world.GlobalWorldManager {
		ui.atlas.release()
		ui.atlas.world, ui.atlas.manager = w, world.GlobalWorldManager
	}
	full := image.Rect(0, 0, w.Width, w.Height)
	for _, r := range atlasRegions(w) {
		if r.MapKey == ui.atlas.region {
			return image.Rect(r.OffsetX-2, r.OffsetY-2, r.OffsetX+r.Width+2, r.OffsetY+r.Height+2).Intersect(full)
		}
	}
	ui.atlas.region = ""
	return full
}

func (ui *UISystem) handleMapOverlayInput() {
	if ui == nil || ui.game == nil || !ui.game.mapOverlayOpen {
		return
	}
	layout := ui.mapOverlayLayout()
	if ui.game.consumeLeftClickIn(layout.close.x, layout.close.y, layout.close.right(), layout.close.bottom()) {
		ui.game.mapOverlayOpen = false
		ui.dropQueuedClicks()
	}
}

func (ui *UISystem) drawMapOverlay(screen *ebiten.Image) {
	g := ui.game
	if g.world == nil {
		g.mapOverlayOpen = false
		return
	}
	l := ui.mapOverlayLayout()
	// The atlas owns this modal's hover layer. Discard tooltips from the hub
	// beneath it before its own named markers queue one.
	ui.clearQueuedTooltips()
	uiClippedLabels = uiClippedLabels[:0]
	drawFilledRect(screen, 0, 0, g.config.GetScreenWidth(), gameplayViewportBottom(g), interfacePanelFill)
	ui.drawThemeFrame(screen, frameGold, l.panel.x, l.panel.y, l.panel.w, l.panel.h)
	title := uitext.Text("atlas.title")
	titleScale := math.Min(2, float64(l.title.w)/float64(max(1, uiTextWidth(title))))
	drawScaledMetalCenteredText(screen, title, l.title.x+int(float64(uiTextWidth(title))*titleScale/2), l.title.y+l.title.h/2, titleScale, atlasGold)
	subtitle := ""
	wm := world.GlobalWorldManager
	if wm != nil {
		if cfg := wm.GetCurrentMapConfig(); cfg != nil {
			subtitle = cfg.Name
		}
	}
	if wm != nil && ui.atlas.region != "" {
		if cfg := wm.MapConfigs[ui.atlas.region]; cfg != nil {
			subtitle = cfg.Name
		}
	}
	drawUITextColored(screen, clipUIText(subtitle, l.subtitle.w), l.subtitle.x, l.subtitle.y, atlasGold)
	mx, my := uiCursorPosition()
	hover := isMouseHoveringBox(mx, my, l.close.x, l.close.y, l.close.right(), l.close.bottom())
	ui.drawAtlasButtonFace(screen, l.close, hover, false)
	ui.drawCloseButtonVisual(screen, l.close.x+4, l.close.y+6, 28, 28)
	drawCenteredTextWithShadow(screen, uitext.Text("atlas.close"), l.close.x+32, l.close.y, l.close.w-36, l.close.h, atlasGold)
	ui.onDisplayedInput(uiCommandClick, l.close, ui.handleMapOverlayInput)
	ui.atlasButton(screen, l.overview, uitext.Text("atlas.overview"), ui.atlas.region == "" && ui.atlas.zoom <= 1, func() { ui.atlas.selectRegion("") })
	if g.world.Width <= 0 || g.world.Height <= 0 {
		return
	}

	// Measured 1254px source: corner ornament ends before 112px; the plain
	// inner field begins after 32px. Nine-slice keeps brass corners square.
	drawFilledRect(screen, l.body.x, l.body.y, l.body.w, l.body.h, atlasPaper)
	if g.sprites != nil && g.sprites.HasSprite("theme_world_atlas") {
		drawNineSliceScaled(screen, g.sprites.GetSprite("theme_world_atlas"), l.body.x, l.body.y, l.body.w, l.body.h, 112, 36)
	}
	bounds := ui.atlasBounds()
	terrain := ui.atlasTerrain(bounds)
	view := layoutRect{l.body.x + 12, l.body.y + 12, l.body.w - 24, l.body.h - 24}
	p := ui.atlas.projection(view, bounds)
	canvas := uiClip(screen, image.Rect(view.x, view.y, view.right(), view.bottom()))
	ui.onDisplayedInput(uiCommandPointer, view, func() { ui.handleAtlasPointer(view, bounds) })
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.ScaleAlpha(0.91)
	uiDrawImageScaled(canvas, terrain, p.x, p.y, float64(bounds.Dx())*p.scale, float64(bounds.Dy())*p.scale, op)
	ui.drawAtlasTileIcons(canvas, p)
	ui.drawAtlasMarkers(canvas, p)
	ui.drawAtlasRegionSeals(canvas, p)
	ui.drawAtlasIndex(screen, l.index)
	ui.drawAtlasLegend(screen, l.legend)
	// The compass belongs to the page margin, never to geographic content.
	if l.body.w > 500 && l.body.h > 320 && ui.atlas.region == "" && ui.atlas.zoom <= 1 {
		drawAtlasCompass(screen, l.body.right()-52, l.body.bottom()-56)
	}
}

func atlasMarker(screen *ebiten.Image, x, y float32, radius float32, c color.RGBA, diamond bool) {
	if diamond {
		uiFillCircle(screen, x, y, radius+1, atlasInk, true)
		uiStrokeLine(screen, x, y-radius, x+radius, y, 2, c, true)
		uiStrokeLine(screen, x+radius, y, x, y+radius, 2, c, true)
		uiStrokeLine(screen, x, y+radius, x-radius, y, 2, c, true)
		uiStrokeLine(screen, x-radius, y, x, y-radius, 2, c, true)
	} else {
		uiFillCircle(screen, x, y, radius+1, atlasInk, true)
		uiFillCircle(screen, x, y, radius, c, true)
	}
}

func (ui *UISystem) drawAtlasMarkers(screen *ebiten.Image, p atlasProjection) {
	g := ui.game
	ts := float64(g.config.GetTileSize())
	iconSize := math.Max(14, math.Min(26, p.scale*2.4))
	for _, npc := range g.world.NPCs {
		if !g.npcMapMarkerVisible(npc) {
			continue
		}
		x, y, ok := p.point(int(npc.X/ts), int(npc.Y/ts))
		if ok {
			ui.drawAtlasNamedIcon(screen, ui.atlasNPCIcon(npc), npc.Name, x, y, iconSize)
		}
	}
	for _, tile := range ui.atlas.portals {
		x, y, ok := p.point(tile.X, tile.Y)
		if ok {
			ui.drawAtlasIcon(screen, g.atlasStyle.PortalIcon, x, y, iconSize)
		}
	}
	// Bosses use the current roster and position, never cached spawn markers.
	// Death, movement and a chamber reset therefore appear on the next draw.
	for _, boss := range g.world.Monsters {
		if !boss.IsBoss() || !boss.IsAlive() {
			continue
		}
		x, y, ok := p.point(int(boss.X/ts), int(boss.Y/ts))
		if ok {
			ui.drawAtlasNamedIcon(screen, g.atlasStyle.BossIcon, boss.Name, x, y, math.Max(22, iconSize))
		}
	}
	if g.questManager != nil {
		wm := world.GlobalWorldManager
		for i, q := range g.questManager.GetActiveQuests() {
			if i >= len(atlasQuest) {
				break
			}
			d := q.Definition
			if d.MarkerX == 0 && d.MarkerY == 0 {
				continue
			}
			x, y := d.MarkerX, d.MarkerY
			if wm != nil {
				key := d.MarkerMap
				if key == "" {
					key = wm.CurrentMapKey
				}
				if wm.WorldByKey(key) != g.world {
					continue
				}
				x, y = wm.ProjectTile(key, x, y)
			}
			px, py, ok := p.point(x, y)
			if ok {
				atlasMarker(screen, px, py, 7, atlasQuest[i], true)
				label := strconv.Itoa(i + 1)
				drawUIText(screen, label, int(px)-uiTextWidth(label)/2, int(py)-uiTextCharHeight/2)
			}
		}
	}
	if g.camera != nil {
		x, y, ok := p.point(int(g.camera.X/ts), int(g.camera.Y/ts))
		if ok {
			atlasMarker(screen, x, y, 5, atlasParty, false)
			uiStrokeCircle(screen, x, y, 8, 1, atlasInk, true)
			dx, dy := float32(math.Cos(g.camera.Angle)), float32(math.Sin(g.camera.Angle))
			uiStrokeLine(screen, x+dx*8, y+dy*8, x+dx*13, y+dy*13, 2, atlasInk, true)
		}
	}
}

func (ui *UISystem) atlasButton(screen *ebiten.Image, r layoutRect, label string, selected bool, action func()) {
	mx, my := uiCursorPosition()
	hover := isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
	ui.drawAtlasButtonFace(screen, r, hover, selected)
	drawCenteredTextWithShadow(screen, label, r.x+14, r.y, r.w-28, r.h, atlasGold)
	ui.onDisplayedInput(uiCommandNavigation, r, func() {
		if ui.topModalLayer() == modalLayerMap && ui.game.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
			action()
			ui.dropQueuedClicks()
		}
	})
}

func (ui *UISystem) drawAtlasIndex(screen *ebiten.Image, r layoutRect) {
	regions := atlasRegions(ui.game.world)
	if r.w == 0 || len(regions) == 0 {
		return
	}
	drawUITextColored(screen, uitext.Text("atlas.regions"), r.x, r.y+4, atlasGold)

	rows := max(1, (r.h-68)/40)
	pages := (len(regions) + rows - 1) / rows
	ui.atlas.page = min(ui.atlas.page, pages-1)
	for i := ui.atlas.page * rows; i < min(len(regions), (ui.atlas.page+1)*rows); i++ {
		region := regions[i]
		name := region.MapKey
		if cfg := world.GlobalWorldManager.MapConfigs[region.MapKey]; cfg != nil {
			name = cfg.Name
		}
		label := strconv.Itoa(i+1) + ". " + name
		row := layoutRect{r.x, r.y + 28 + (i%rows)*40, r.w, 36}
		ui.atlasButton(screen, row, label, ui.atlas.region == region.MapKey, func() { ui.atlas.selectRegion(region.MapKey) })
	}
	if pages > 1 {
		half := (r.w - 8) / 2
		ui.atlasButton(screen, layoutRect{r.x, r.bottom() - 32, half, 32}, uitext.Text("atlas.previous"), false, func() { ui.atlas.page = (ui.atlas.page + pages - 1) % pages })
		ui.atlasButton(screen, layoutRect{r.x + half + 8, r.bottom() - 32, half, 32}, uitext.Text("atlas.next"), false, func() { ui.atlas.page = (ui.atlas.page + 1) % pages })
	}
}

func (ui *UISystem) drawAtlasRegionSeals(screen *ebiten.Image, p atlasProjection) {
	regions := atlasRegions(ui.game.world)
	if ui.atlas.region != "" {
		return
	}
	for i, region := range regions {
		x, y, ok := p.point(region.OffsetX, region.OffsetY)
		if !ok {
			continue
		}
		atlasMarker(screen, x+4, y+4, 8, atlasInk, false)
		label := strconv.Itoa(i + 1)
		drawUITextColored(screen, label, int(x)+4-uiTextWidth(label)/2, int(y)+4-uiTextCharHeight/2, atlasGold)
	}
}

func (ui *UISystem) drawAtlasLegend(screen *ebiten.Image, r layoutRect) {
	x := r.x
	entries := []struct {
		label, icon string
		c           color.RGBA
		quest       bool
	}{
		{uitext.Text("atlas.party"), "", atlasParty, false}, {uitext.Text("atlas.boss"), ui.game.atlasStyle.BossIcon, atlasPeople, false},
		{uitext.Text("atlas.town"), ui.game.atlasStyle.LegendIcons["town"], atlasPeople, false}, {uitext.Text("atlas.people"), ui.game.atlasStyle.LegendIcons["people"], atlasPeople, false},
		{uitext.Text("atlas.objects"), ui.game.atlasStyle.LegendIcons["objects"], atlasPeople, false}, {uitext.Text("atlas.portal"), ui.game.atlasStyle.PortalIcon, atlasPortal, false}, {uitext.Text("atlas.quest"), "", atlasQuest[0], true},
	}
	for _, e := range entries {
		label := e.label
		width := uiTextWidth(label) + 40
		if x+width > r.right() {
			break
		}
		if e.icon != "" {
			ui.drawAtlasIcon(screen, e.icon, float32(x+8), float32(r.y+10), 22)
		} else {
			atlasMarker(screen, float32(x+7), float32(r.y+9), 4, e.c, e.quest)
		}
		drawUITextColored(screen, label, x+20, r.y+3, atlasGold)
		x += width
	}
	hint := uitext.Text("atlas.controls")
	if x+24+uiTextWidth(hint) < r.right() {
		drawUITextColored(screen, hint, r.right()-uiTextWidth(hint), r.y+3, atlasGold)
	}
}

func drawAtlasCompass(screen *ebiten.Image, x, y int) {
	cx, cy := float32(x), float32(y)
	uiStrokeCircle(screen, cx, cy, 19, 1, atlasInk, true)
	for i, label := range []string{uitext.Text("atlas.north"), uitext.Text("atlas.east"), uitext.Text("atlas.south"), uitext.Text("atlas.west")} {
		angle := float64(i)*math.Pi/2 - math.Pi/2
		dx, dy := float32(math.Cos(angle)), float32(math.Sin(angle))
		uiStrokeLine(screen, cx+dx*4, cy+dy*4, cx+dx*24, cy+dy*24, 2, atlasInk, true)
		drawUITextColored(screen, label, int(cx+dx*32)-uiTextWidth(label)/2, int(cy+dy*32)-uiTextCharHeight/2, atlasGold)
	}
}

// These limits bound presentation magnification, not world/gameplay ranges.
const atlasMaxZoom = 16

func (a *worldAtlas) selectRegion(key string) {
	a.region, a.zoom, a.panX, a.panY = key, 1, 0, 0
	a.dragging = false
}

func (a *worldAtlas) projection(view layoutRect, bounds image.Rectangle) atlasProjection {
	if a.zoom < 1 {
		a.zoom = 1
	}
	p := fitAtlas(view, bounds)
	p.scale *= a.zoom
	// A resized window must not strand the map beyond an edge.
	maxX := math.Max(0, (float64(bounds.Dx())-float64(view.w-24)/p.scale)/2)
	maxY := math.Max(0, (float64(bounds.Dy())-float64(view.h-24)/p.scale)/2)
	a.panX = math.Max(-maxX, math.Min(maxX, a.panX))
	a.panY = math.Max(-maxY, math.Min(maxY, a.panY))
	p.x = float64(view.x) + (float64(view.w)-float64(bounds.Dx())*p.scale)/2 + a.panX*p.scale
	p.y = float64(view.y) + (float64(view.h)-float64(bounds.Dy())*p.scale)/2 + a.panY*p.scale
	return p
}

func (a *worldAtlas) zoomAt(view layoutRect, bounds image.Rectangle, x, y int, wheel float64) {
	before := a.projection(view, bounds)
	a.zoom = math.Max(1, math.Min(atlasMaxZoom, a.zoom*math.Pow(1.2, math.Max(-8, math.Min(8, wheel)))))
	scale := fitAtlas(view, bounds).scale * a.zoom
	// Preserve the tile beneath the cursor, then clamp at the map edges.
	originX := float64(x) - (float64(x)-before.x)*scale/before.scale
	originY := float64(y) - (float64(y)-before.y)*scale/before.scale
	a.panX = (originX - (float64(view.x) + (float64(view.w)-float64(bounds.Dx())*scale)/2)) / scale
	a.panY = (originY - (float64(view.y) + (float64(view.h)-float64(bounds.Dy())*scale)/2)) / scale
	a.projection(view, bounds)
}

func (ui *UISystem) handleAtlasPointer(view layoutRect, bounds image.Rectangle) {
	if !ui.game.mapOverlayOpen || ui.topModalLayer() != modalLayerMap {
		return
	}
	a := &ui.atlas
	x, y := pointerPosition()
	pressed := pointerLeftPressed()
	inside := isMouseHoveringBox(x, y, view.x, view.y, view.right(), view.bottom())
	if !a.pointerReady {
		// Opening the map with a held inventory press cannot start a drag.
		a.pointerReady, a.pressed = true, pressed
	}
	_, wheel := pointerWheel()
	if inside && wheel != 0 {
		a.zoomAt(view, bounds, x, y, wheel)
	}
	if !pressed {
		a.dragging = false
	}
	if inside && pressed && !a.pressed {
		a.dragging = true
		a.lastX, a.lastY = x, y
	}
	if a.dragging && pressed {
		p := a.projection(view, bounds)
		a.panX += float64(x-a.lastX) / p.scale
		a.panY += float64(y-a.lastY) / p.scale
		a.projection(view, bounds)
	}
	a.lastX, a.lastY, a.pressed = x, y, pressed
}

func (ui *UISystem) atlasNPCIcon(npc *character.NPC) string {
	if icon := ui.game.atlasStyle.NPCs[npc.Key]; icon != "" {
		return icon
	}
	if npc.PropModel != nil {
		if icon := ui.game.atlasStyle.PropShapes[npc.PropModel.Shape]; icon != "" {
			return icon
		}
	}
	if icon := ui.game.atlasStyle.NPCTypes[npc.Type]; icon != "" {
		return icon
	}
	return ui.game.atlasStyle.RenderCategories[npcCatName[npcRenderCatOf(npc)]]
}

func (ui *UISystem) drawAtlasIcon(screen *ebiten.Image, key string, x, y float32, size float64) {
	if ui.game.sprites != nil && ui.game.sprites.HasSprite(key) {
		uiDrawImageScaled(screen, ui.game.sprites.GetSprite(key), float64(x)-size/2, float64(y)-size/2, size, size, nil)
	}
}

func (ui *UISystem) drawAtlasNamedIcon(screen *ebiten.Image, key, name string, x, y float32, size float64) {
	ui.drawAtlasIcon(screen, key, x, y, size)
	mx, my := uiCursorPosition()
	if image.Pt(mx, my).In(uiBounds(screen)) && math.Abs(float64(mx)-float64(x)) <= size/2 && math.Abs(float64(my)-float64(y)) <= size/2 {
		ui.queueTooltip([]string{name}, mx+12, my+12)
	}
}

func (ui *UISystem) drawAtlasTileIcons(screen *ebiten.Image, p atlasProjection) {
	// One miniature of each kind per screen-sized patch of authored terrain.
	// Zooming reveals individual authored placements without covering paths.
	stride := max(1, int(math.Ceil(14/p.scale)))
	size := math.Max(10, math.Min(26, p.scale*2))
	if ui.atlas.tileBuckets == nil {
		ui.atlas.tileBuckets = make(map[atlasTileMark]bool)
	}
	if ui.atlas.bucketStride != stride {
		clear(ui.atlas.tileBuckets)
		ui.atlas.bucketMarks = ui.atlas.bucketMarks[:0]
		for _, mark := range ui.atlas.tileMarks {
			bucket := atlasTileMark{image.Pt(mark.point.X/stride, mark.point.Y/stride), mark.icon}
			if !ui.atlas.tileBuckets[bucket] {
				ui.atlas.tileBuckets[bucket] = true
				ui.atlas.bucketMarks = append(ui.atlas.bucketMarks, mark)
			}
		}
		ui.atlas.bucketStride = stride
	}
	for _, mark := range ui.atlas.bucketMarks {
		tile := mark.point
		x, y, ok := p.point(tile.X, tile.Y)
		if ok {
			ui.drawAtlasIcon(screen, mark.icon, x, y, size)
		}
	}
}

func (ui *UISystem) drawAtlasButtonFace(screen *ebiten.Image, r layoutRect, hover, selected bool) {
	if ui.game.sprites == nil || !ui.game.sprites.HasSprite("theme_atlas_button") {
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, hover || selected)
		return
	}
	src := ui.game.sprites.GetSprite("theme_atlas_button")
	b := src.Bounds()
	// 2172x724 source; ornament ends at 320px. Full-height end caps retain
	// their aspect ratio. Only the plain leather and straight rails stretch.
	capPixels := b.Dy() / 2
	capWidth := min(r.w/2, r.h/2)
	sourceX := [4]int{b.Min.X, b.Min.X + capPixels, b.Max.X - capPixels, b.Max.X}
	destX := [4]int{r.x, r.x + capWidth, r.right() - capWidth, r.right()}
	op := &ebiten.DrawImageOptions{}
	if hover {
		op.ColorScale.Scale(1.25, 1.25, 1.25, 1)
	} else if selected {
		op.ColorScale.Scale(1.15, 1.15, 1.05, 1)
	}
	for i := range 3 {
		part := src.RecyclableSubImage(image.Rect(sourceX[i], b.Min.Y, sourceX[i+1], b.Max.Y))
		uiDrawImageScaled(screen, part, float64(destX[i]), float64(r.y), float64(destX[i+1]-destX[i]), float64(r.h), op)
		part.Recycle()
	}
	if selected {
		uiFillCircle(screen, float32(r.x+9), float32(r.y+r.h/2), 2.5, atlasGold, true)
	}
}

package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const pageOverlay = 9

type overlayPage struct {
	catalogs                  map[string][]string
	pendingNPC, pendingPicker string
	choicePath                []int
	documents                 map[string]*overlayDocument
	section                   string
	activeMap                 string
	selected                  int
	treeScroll, fieldScroll   int
	fieldFocus                int
	collapsed                 map[string]bool
	zoom                      float64
	panX, panY                int
	pan                       bool
	lastX, lastY              int
	tool                      string
	dragging                  bool
	anchor, end               [2]int
	original                  [4]int
	anchorSet                 bool
	beforeDrag                overlayState
	modal                     *overlayModal
	quit                      bool
}
type overlayModal struct {
	title, help, text   string
	choices             []string
	selected            []string
	multi, confirm, all bool
	scroll, cursor      int
	apply               func(string) error
}
type overlayWidget struct {
	r          rect
	text, help string
	active     bool
	action     func()
	kind       string
}

var overlayBG = color.RGBA{24, 29, 40, 255}
var overlayAccent = color.RGBA{114, 211, 182, 255}

func (v *viewer) overlayDirty() bool {
	for _, d := range v.overlay.documents {
		if d.dirty() {
			return true
		}
	}
	return false
}
func (v *viewer) overlayConfirm(title, help string, fn func()) {
	v.overlay.modal = &overlayModal{title: title, help: help, confirm: true, apply: func(string) error { fn(); return nil }}
}
func (v *viewer) overlayRequestQuit() bool {
	if v.overlay.dragging {
		if d := v.overlayDoc(); d != nil {
			v.overlayCancelGesture(d)
		}
	}
	if v.overlay.quit || !v.overlayDirty() {
		return true
	}
	if v.overlay.modal == nil {
		v.overlayConfirm("Discard unsaved overlays?", "Changes in every unsaved overlay will be lost. Cancel to return and save.", func() { v.overlay.quit = true })
	}
	return false
}
func (v *viewer) overlaySelect(kind string, index int) {
	v.overlay.section, v.overlay.selected = kind, index
	v.overlay.choicePath = nil
	v.overlay.fieldScroll = 0
	v.overlay.fieldFocus = -1
	v.overlay.tool = ""
	v.overlay.anchorSet = false
}
func (v *viewer) overlayOpenField(d *overlayDocument, index int) {
	fields := v.overlayFields(d)
	if index < 0 || index >= len(fields) || !fields[index].interactive() {
		return
	}
	f := fields[index]
	if f.multi && len(f.choices) == 0 {
		d.err = "No matching targets on this map. Add one before linking it."
		return
	}
	if f.action != nil {
		f.action()
		return
	}
	if f.kind == "toggle" {
		value := "Yes"
		if f.value == "Yes" {
			value = "No"
		}
		if err := v.overlayApplyField(d, index, value); err != nil {
			d.err = err.Error()
		}
		return
	}
	if f.apply == nil {
		d.status = f.help
		return
	}
	v.overlay.modal = &overlayModal{title: f.label, help: f.help, text: f.value, cursor: len(f.value), all: true, choices: f.choices, multi: f.multi, selected: strings.Split(f.value, ", "), apply: func(s string) error {
		return v.overlayApplyField(d, index, s)
	}}
	if len(f.choices) > 0 {
		v.overlay.modal.text = ""
		v.overlay.modal.cursor = 0
		v.overlay.modal.all = false
	}
}
func (v *viewer) overlayApplyField(d *overlayDocument, index int, value string) error {
	before := d.state.clone()
	undo, redo := d.undo, d.redo
	d.checkpoint()
	if err := v.overlayFields(d)[index].apply(value); err != nil {
		d.state, d.undo, d.redo = before, undo, redo
		return err
	}
	return nil
}
func (v *viewer) overlayWidgets(d *overlayDocument) []overlayWidget {
	var out []overlayWidget
	add := func(x, y, w int, text, help string, active bool, fn func()) {
		out = append(out, overlayWidget{r: rect{x, y, w, 28}, text: text, help: help, active: active, action: fn})
	}
	add(12, 42, 214, d.key, "Choose a map. Drafts stay open when switching maps.", false, func() {
		choices := []string{}
		for _, m := range v.maps {
			if m.Data != nil {
				choices = append(choices, m.Key)
			}
		}
		v.overlay.modal = &overlayModal{title: "Choose map", help: "Each map keeps an independent unsaved draft.", choices: choices, apply: func(key string) error {
			for i, m := range v.maps {
				if m.Key == key {
					v.overlayCancelGesture(d)
					v.mapIndex = i
					v.refreshLegend()
					v.overlaySelect("overview", 0)
					v.overlay.treeScroll = 0
					v.overlay.panX, v.overlay.panY = 0, 0
					v.overlay.zoom = 1
					return nil
				}
			}
			return fmt.Errorf("Unknown map")
		}}
	})
	add(242, 42, 86, "Save", "Save this overlay and its switch placements. Ctrl+S", d.dirty(), func() {
		if err := v.saveOverlay(d); err != nil {
			d.err = err.Error()
		}
	})
	add(334, 42, 86, "Validate", "Check references, geometry and runtime rules.", false, func() {
		if err := v.validateOverlay(d); err != nil {
			d.err = err.Error()
		} else {
			d.err = ""
			d.status = "Validation passed."
		}
	})
	add(426, 42, 70, "Undo", "Undo the last edit. Ctrl+Z", false, func() { v.overlayHistory(d, false) })
	add(502, 42, 70, "Redo", "Redo the last edit. Ctrl+Shift+Z", false, func() { v.overlayHistory(d, true) })
	add(578, 42, 90, "Reload", "Discard this draft and reload current files.", false, func() {
		v.overlayConfirm("Reload this map?", "Unsaved edits in Overlay and Maps for this map will be discarded.", func() { v.reloadOverlay(d.key) })
	})
	add(674, 42, 90, "Fit map", "Reset zoom and pan.", false, func() { v.overlay.zoom = 1; v.overlay.panX, v.overlay.panY = 0, 0 })
	add(832, 42, 118, "+ Object", "Place an interactive object, NPC or special tile.", false, func() { v.overlayObjectPicker(d) })
	add(956, 42, 108, "+ Boss", "Choose and place a boss on this map.", false, func() { v.overlayBossPicker(d) })
	add(1070, 42, 118, "Overview", "Interaction sources, conditions and targets.", false, func() { v.overlaySelect("overview", 0) })
	for i, tool := range []string{"trap", "occupation", "transfer", "lane", "new-control"} {
		label := map[string]string{"trap": "+ Trap", "occupation": "+ Hazard", "transfer": "+ Transfer", "lane": "+ Lane", "new-control": "+ Switch"}[tool]
		add(242+i*116, 80, 110, label, "Choose a tool, then drag an area on the map. Switches use one click.", v.overlay.tool == tool, func() { v.overlay.tool = tool; v.overlay.anchorSet = false })
	}
	add(242, 114, 92, "Select", "Click a zone to select; drag it to move. Escape cancels a tool.", v.overlay.tool == "", func() { v.overlay.tool = "" })
	add(340, 114, 92, "Duplicate", "Duplicate the selected zone, then drag it into place.", false, func() {
		a := &d.state.Adventure
		if v.overlay.section == "effect" && v.overlay.selected >= 0 && v.overlay.selected < len(a.Effects) {
			e := a.Effects[v.overlay.selected]
			d.checkpoint()
			e.ID = d.uniqueID(e.Kind)
			e.Name += " copy"
			a.Effects = append(a.Effects, e)
			v.overlaySelect("effect", len(a.Effects)-1)
		}
	})
	add(438, 114, 92, "Delete", "Remove the selected object. Linked zones must be unlinked first.", false, func() {
		v.overlayConfirm("Delete selected object?", "Referenced zones are protected. Deleting a control removes its operate actions but keeps the NPC.", func() { v.overlayDelete(d) })
	})
	add(536, 114, 126, "+ Jump link", "Click two walkable endpoints, two tiles apart.", v.overlay.tool == "link-new", func() { v.overlay.tool = "link-new"; v.overlay.anchorSet = false })
	add(668, 114, 150, "Boss settings", "Placement, movement, mechanics and attack patterns.", false, func() { v.overlaySelect("boss", 0) })
	if b := d.state.Adventure.Boss; b != nil && b.Arena != nil {
		add(486, 154, 326, "Movement area: "+overlayArenaSummary(b)+"  >", "Edit the violet boss movement boundary.", v.overlay.section == "arena", func() { v.overlaySelect("arena", 0) })
	}
	for i := range out {
		w := &out[i]
		if (w.text == "Undo" && len(d.undo) == 0) || (w.text == "Redo" && len(d.redo) == 0) || (w.text == "Duplicate" && v.overlay.section != "effect") || (w.text == "Delete" && !slices.Contains([]string{"boss", "object", "special", "effect", "control", "phase", "link"}, v.overlay.section)) {
			w.action = nil
		}
	}
	tree, _ := v.overlayTree(d)
	out = append(out, tree...)
	return out
}
func (v *viewer) overlayAddPhase(d *overlayDocument) {
	a := &d.state.Adventure
	if a.Boss == nil {
		v.overlayBossPicker(d)
		return
	}
	lanes := []string{}
	for _, e := range a.Effects {
		if e.Kind == "lane" {
			lanes = append(lanes, e.ID)
			break
		}
	}
	if len(lanes) == 0 {
		d.err = "Add an attack lane before adding a boss phase."
		return
	}
	threshold := 100
	if n := len(a.Boss.Phases); n > 0 {
		threshold = a.Boss.Phases[n-1].BelowPercent / 2
	}
	if threshold < 1 {
		d.err = "No lower HP threshold is available."
		return
	}
	d.checkpoint()
	a.Boss.Phases = append(a.Boss.Phases, config.AdventureBossPhase{BelowPercent: threshold, Patterns: [][]string{lanes}})
	v.overlaySelect("phase", len(a.Boss.Phases)-1)
}
func (v *viewer) overlayDelete(d *overlayDocument) {
	p := &v.overlay
	a := &d.state.Adventure
	var err error
	switch p.section {
	case "object":
		if p.selected >= 0 && p.selected < len(d.state.Data.NPCSpawns) {
			d.checkpoint()
			spawn := d.state.Data.NPCSpawns[p.selected]
			floor, _ := v.tileManager.GetTileTypeFromLetterForBiome(floorLetter, v.currentBiome())
			d.state.Data.ClearNPCGround(v.tileManager, spawn, floor)
			d.state.Data.NPCSpawns = slices.Delete(d.state.Data.NPCSpawns, p.selected, p.selected+1)
			v.overlayRemoveUnusedLocalNPC(d, spawn.NPCKey)
		}
	case "special":
		if p.selected >= 0 && p.selected < len(d.state.Data.SpecialTileSpawns) {
			d.checkpoint()
			t := d.state.Data.SpecialTileSpawns[p.selected]
			floor, _ := v.tileManager.GetTileTypeFromLetterForBiome(".", v.currentBiome())
			d.state.Data.Tiles[t.Y][t.X] = floor
			d.state.Data.SpecialTileSpawns = slices.Delete(d.state.Data.SpecialTileSpawns, p.selected, p.selected+1)
		}
	case "effect":
		if p.selected >= 0 && p.selected < len(a.Effects) {
			err = d.deleteEffect(p.selected)
		}
	case "control":
		if p.selected >= 0 && p.selected < len(a.Controls) {
			err = d.deleteControl(p.selected)
		}
	case "phase":
		if a.Boss != nil && p.selected >= 0 && p.selected < len(a.Boss.Phases) {
			d.checkpoint()
			a.Boss.Phases = slices.Delete(a.Boss.Phases, p.selected, p.selected+1)
		}
	case "link":
		if p.selected >= 0 && p.selected < len(a.JumpLinks) {
			d.checkpoint()
			a.JumpLinks = slices.Delete(a.JumpLinks, p.selected, p.selected+1)
		}
	case "boss":
		d.checkpoint()
		a.Boss = nil
	}
	if err != nil {
		d.err = err.Error()
		return
	}
	v.overlaySelect("overview", 0)
}
func (v *viewer) overlayLayout(d *overlayDocument) layout {
	const x, y, w, h = 242, 150, 578, 558
	data := d.state.Data
	fit := max(1, min((w-16)/max(1, data.Width), (h-48)/max(1, data.Height)))
	zoom := v.overlay.zoom
	if zoom < 1 {
		zoom = 1
	}
	size := max(1, int(float64(fit)*zoom))
	// Signed offsets keep cursor-anchored zoom stable even on a fitting axis.
	// Fit map explicitly restores the centered view.
	ox := x + 8 + max(0, (w-16-data.Width*size)/2) - v.overlay.panX
	oy := y + 40 + max(0, (h-48-data.Height*size)/2) - v.overlay.panY
	return layout{mapAreaX: x, mapAreaY: y, mapAreaW: w, mapAreaH: h, worldW: data.Width, worldH: data.Height, tileSize: size, originX: ox, originY: oy}
}
func overlayMapViewport(l layout) rect {
	return rect{l.mapAreaX + 2, l.mapAreaY + 40, l.mapAreaW - 4, l.mapAreaH - 42}
}
func overlayTileAt(l layout, x, y int) ([2]int, bool) {
	r := overlayMapViewport(l)
	if !pointInRect(x, y, r.x, r.y, r.w, r.h) {
		return [2]int{}, false
	}
	tx := int(math.Floor(float64(x-l.originX) / float64(l.tileSize)))
	ty := int(math.Floor(float64(y-l.originY) / float64(l.tileSize)))
	return [2]int{tx, ty}, tx >= 0 && ty >= 0 && tx < l.worldW && ty < l.worldH
}
func (v *viewer) scrollOverlayAt(d *overlayDocument, mx, my int, wheel float64) {
	if wheel == 0 {
		return
	}
	p := &v.overlay
	if pointInRect(mx, my, 8, 78, 222, 636) {
		p.treeScroll = max(0, p.treeScroll-int(wheel*32))
		_, maxScroll := v.overlayTree(d)
		p.treeScroll = min(p.treeScroll, maxScroll)
		return
	}
	if pointInRect(mx, my, 832, 78, 360, 636) {
		_, maxScroll := overlayInspectorRows(v.overlayFields(d), 0)
		p.fieldScroll = clampInt(p.fieldScroll-int(wheel*44), 0, maxScroll)
		return
	}
	before := v.overlayLayout(d)
	r := overlayMapViewport(before)
	if !pointInRect(mx, my, r.x, r.y, r.w, r.h) {
		return
	}
	fx := float64(mx-before.originX) / float64(before.tileSize)
	fy := float64(my-before.originY) / float64(before.tileSize)
	p.zoom = math.Max(1, math.Min(8, max(1, p.zoom)*math.Pow(1.15, wheel)))
	after := v.overlayLayout(d)
	// Use the rendered integer tile size, not the unrounded zoom ratio.
	p.panX += int(math.Round(float64(after.originX-mx) + fx*float64(after.tileSize)))
	p.panY += int(math.Round(float64(after.originY-my) + fy*float64(after.tileSize)))
}
func overlayBox(a, b [2]int) [4]int {
	return [4]int{min(a[0], b[0]), min(a[1], b[1]), max(a[0], b[0]), max(a[1], b[1])}
}
func (v *viewer) updateOverlayPage() {
	d := v.overlayDoc()
	if d == nil || d.state.Data == nil {
		return
	}
	p := &v.overlay
	if p.pendingPicker != "" {
		v.overlayContinuePicker(d)
		return
	}
	if p.activeMap != d.key {
		v.overlaySelect("overview", 0)
		p.activeMap = d.key
		p.treeScroll = 0
		p.panX, p.panY = 0, 0
	}
	if p.section == "" {
		v.overlaySelect("overview", 0)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		p.tool = ""
		p.anchorSet = false
		if p.dragging {
			d.state = p.beforeDrag
			p.dragging = false
		}
		return
	}
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		v.overlayCancelGesture(d)
		if err := v.saveOverlay(d); err != nil {
			d.err = err.Error()
		}
		return
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		v.overlayCancelGesture(d)
		v.overlayHistory(d, ebiten.IsKeyPressed(ebiten.KeyShift))
		return
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyY) {
		v.overlayCancelGesture(d)
		v.overlayHistory(d, true)
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		v.overlayFocusNext(d, ebiten.IsKeyPressed(ebiten.KeyShift))
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && p.fieldFocus >= 0 {
		v.overlayOpenField(d, p.fieldFocus)
		return
	}
	mx, my := ebiten.CursorPosition()
	_, wheel := ebiten.Wheel()
	v.scrollOverlayAt(d, mx, my, wheel)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for _, w := range v.overlayWidgets(d) {
			if w.action != nil && pointInRect(mx, my, w.r.x, w.r.y, w.r.w, w.r.h) {
				w.action()
				return
			}
		}
		if index := overlayInspectorHit(v.overlayFields(d), p.fieldScroll, mx, my); index >= 0 {
			p.fieldFocus = index
			v.overlayOpenField(d, index)
			return
		}
	}
	l := v.overlayLayout(d)
	viewport := overlayMapViewport(l)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) && pointInRect(mx, my, viewport.x, viewport.y, viewport.w, viewport.h) {
		p.pan = true
		p.lastX, p.lastY = mx, my
	}
	if p.pan {
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
			p.panX -= mx - p.lastX
			p.panY -= my - p.lastY
			p.lastX, p.lastY = mx, my
			l = v.overlayLayout(d)
		} else {
			p.pan = false
		}
	}
	tile, inside := overlayTileAt(l, mx, my)
	if p.dragging {
		if inside {
			p.end = tile
			if p.tool == "move" {
				dx, dy := tile[0]-p.anchor[0], tile[1]-p.anchor[1]
				r := p.original
				dx = clampInt(dx, -r[0], d.state.Data.Width-1-r[2])
				dy = clampInt(dy, -r[1], d.state.Data.Height-1-r[3])
				d.state.Adventure.Effects[p.selected].Rect = [4]int{r[0] + dx, r[1] + dy, r[2] + dx, r[3] + dy}
			}
		}
		if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			v.overlayFinishDrag(d)
		}
		return
	}
	if inside && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		v.overlayMapPress(d, tile)
	}
}
func (v *viewer) overlayMapPress(d *overlayDocument, tile [2]int) {
	p := &v.overlay
	a := &d.state.Adventure
	switch p.tool {
	case "boss-place":
		if err := v.overlayPlaceBoss(d, tile); err != nil {
			d.err = err.Error()
		}
		return
	case "object-new":
		if err := v.overlayPlaceObject(d, tile); err != nil {
			d.err = err.Error()
		}
		return
	case "object-move":
		if p.selected < 0 || p.selected >= len(d.state.Data.NPCSpawns) {
			return
		}
		if !d.freeTile(tile[0], tile[1], v.tileManager) {
			d.err = "Choose empty walkable ground."
			return
		}
		d.checkpoint()
		v.overlayMovePlacement(d, false, p.selected, tile)
		p.tool = ""
		return
	case "special-new":
		if !d.freeTile(tile[0], tile[1], v.tileManager) {
			d.err = "Choose empty walkable ground."
			return
		}
		t, ok := v.tileManager.GetTileTypeFromKey(p.pendingNPC)
		if !ok {
			return
		}
		d.checkpoint()
		d.state.Data.SpecialTileSpawns = append(d.state.Data.SpecialTileSpawns, world.SpecialTileSpawn{X: tile[0], Y: tile[1], TileKey: p.pendingNPC, TileType: t})
		d.state.Data.Tiles[tile[1]][tile[0]] = t
		v.overlaySelect("special", len(d.state.Data.SpecialTileSpawns)-1)
		return
	case "new-control", "healing-control":
		healing := p.tool == "healing-control"
		i, err := d.addControl(tile[0], tile[1], v.tileManager)
		if err != nil {
			d.err = err.Error()
			return
		}
		if healing {
			a.Controls[i].StopHealing = true
			if a.Boss != nil {
				if a.Boss.HealPercent == 0 {
					a.Boss.HealPercent = 5
				}
				if a.Boss.HealCapPercent == 0 {
					a.Boss.HealCapPercent = 25
				}
			}
		}
		_, n := d.controlNPC(a.Controls[i].ID)
		if n != nil {
			n.Biomes = []string{v.maps[v.mapIndex].Config.Biome}
		}
		v.overlaySelect("control", i)
		if healing {
			v.overlaySelect("regeneration", 0)
		}
	case "receiver":
		if p.section == "effect" && p.selected >= 0 && p.selected < len(a.Effects) {
			if err := v.overlayReceiverError(d, tile); err != nil {
				d.err = err.Error()
				return
			}
			d.checkpoint()
			a.Effects[p.selected].Destination = tile
			p.tool = ""
		}
	case "switch":
		if p.selected < 0 || p.selected >= len(a.Controls) {
			return
		}
		key, _ := d.controlNPC(a.Controls[p.selected].ID)
		if !d.freeTile(tile[0], tile[1], v.tileManager) {
			d.err = "Choose empty walkable ground."
			return
		}
		for i, n := range d.state.Data.NPCSpawns {
			if n.NPCKey == key {
				d.checkpoint()
				v.overlayMovePlacement(d, false, i, tile)
				break
			}
		}
		p.tool = ""
	case "tile-change":
		if p.selected >= 0 && p.selected < len(a.Controls) {
			d.checkpoint()
			a.Controls[p.selected].Tiles = append(a.Controls[p.selected].Tiles, config.EnvironmentTile{X: tile[0], Y: tile[1], Tile: v.tileManager.GetTileKey(d.state.Data.Tiles[tile[1]][tile[0]])})
			p.tool = ""
			_, p.fieldScroll = overlayInspectorRows(v.overlayFields(d), 0)
		}
	case "link-new", "link-redraw":
		if p.tool == "link-redraw" && (p.selected < 0 || p.selected >= len(a.JumpLinks)) {
			p.tool = ""
			p.anchorSet = false
			return
		}
		if !p.anchorSet {
			p.anchor = tile
			p.anchorSet = true
			return
		}
		j := [4]int{p.anchor[0], p.anchor[1], tile[0], tile[1]}
		if math.Abs(float64(j[2]-j[0]))+math.Abs(float64(j[3]-j[1])) != 2 || (j[0] != j[2] && j[1] != j[3]) {
			d.err = "Choose an endpoint exactly two tiles away in one row or column."
			return
		}
		d.checkpoint()
		if p.tool == "link-new" {
			a.JumpLinks = append(a.JumpLinks, j)
			v.overlaySelect("link", len(a.JumpLinks)-1)
		} else {
			a.JumpLinks[p.selected] = j
			p.tool = ""
		}
		p.anchorSet = false
	case "trap", "occupation", "transfer", "lane", "redraw", "arena":
		p.beforeDrag = d.state.clone()
		p.anchor, p.end = tile, tile
		p.dragging = true
	default:
		for i, n := range d.state.Data.NPCSpawns {
			if n.X == tile[0] && n.Y == tile[1] {
				for j, c := range a.Controls {
					key, _ := d.controlNPC(c.ID)
					if key == n.NPCKey {
						v.overlaySelect("control", j)
						return
					}
				}
				v.overlaySelect("object", i)
				return
			}
		}
		if a.Boss != nil {
			for _, n := range d.state.Data.MonsterSpawns {
				if n.MonsterKey == a.Boss.Monster && n.X == tile[0] && n.Y == tile[1] {
					v.overlaySelect("boss", 0)
					return
				}
			}
		}
		for i, n := range d.state.Data.SpecialTileSpawns {
			if n.X == tile[0] && n.Y == tile[1] {
				v.overlaySelect("special", i)
				return
			}
		}
		// Repeated clicks cycle through overlapping zones; the tree is always exact.
		hits := []int{}
		for i, e := range a.Effects {
			if e.Contains(tile[0], tile[1]) {
				hits = append(hits, i)
			}
		}
		if len(hits) > 0 {
			hit := hits[0]
			if p.section == "effect" {
				if i := slices.Index(hits, p.selected); i >= 0 {
					hit = hits[(i+1)%len(hits)]
				}
			}
			v.overlaySelect("effect", hit)
			p.original = a.Effects[hit].Rect
			p.beforeDrag = d.state.clone()
			p.anchor, p.end = tile, tile
			p.dragging = true
			p.tool = "move"
			return
		}
		for i, c := range a.Controls {
			key, _ := d.controlNPC(c.ID)
			for _, n := range d.state.Data.NPCSpawns {
				if n.NPCKey == key && n.X == tile[0] && n.Y == tile[1] {
					v.overlaySelect("control", i)
					return
				}
			}
		}
	}
}
func (v *viewer) overlayFinishDrag(d *overlayDocument) {
	p := &v.overlay
	p.dragging = false
	r := overlayBox(p.anchor, p.end)
	switch p.tool {
	case "move":
		if !overlaySame(p.beforeDrag, d.state) {
			d.checkpointState(p.beforeDrag)
		}
		p.tool = ""
	case "redraw":
		d.checkpoint()
		d.state.Adventure.Effects[p.selected].Rect = r
		p.tool = ""
	case "arena":
		if d.state.Adventure.Boss != nil {
			d.checkpoint()
			d.state.Adventure.Boss.Arena = &r
		}
		v.overlaySelect("arena", 0)
	default:
		kind := p.tool
		i := d.addEffect(kind, r)
		v.overlaySelect("effect", i)
		if kind == "transfer" {
			p.tool = "receiver"
		}
	}
}

func (v *viewer) drawOverlayPage(screen *ebiten.Image) {
	d := v.overlayDoc()
	if d == nil || d.state.Data == nil {
		game.DrawPlainText(screen, "No map available.", 20, 60)
		return
	}
	p := &v.overlay
	for _, r := range []rect{{8, 78, 222, 636}, {832, 78, 360, 636}} {
		drawFilledRect(screen, r.x, r.y, r.w, r.h, overlayBG)
	}
	m := v.maps[v.mapIndex]
	mc := *m.Config
	mc.Adventure = nil
	m.Config = &mc
	m.Data = d.state.Data
	l := v.overlayLayout(d)
	drawMapPanel(screen, m, l, v.tileManager, v.tileDataByKey, v.tileSpriteThumbnail)
	drawFilledRect(screen, l.mapAreaX+2, l.mapAreaY+2, l.mapAreaW-4, 36, color.RGBA{20, 20, 35, 255})
	game.DrawShadedText(screen, clipText(mc.Name, 224), l.mapAreaX+12, l.mapAreaY+12, overlayText)
	viewport := overlayMapViewport(l)
	clip := screen.SubImage(image.Rect(viewport.x, viewport.y, viewport.x+viewport.w, viewport.y+viewport.h)).(*ebiten.Image)
	drawBox := func(r [4]int, c color.RGBA, selected bool) {
		x, y := l.originX+r[0]*l.tileSize, l.originY+r[1]*l.tileSize
		w, h := (r[2]-r[0]+1)*l.tileSize, (r[3]-r[1]+1)*l.tileSize
		fill := c
		fill = color.RGBA{uint8(uint16(c.R) * 40 / 255), uint8(uint16(c.G) * 40 / 255), uint8(uint16(c.B) * 40 / 255), 40}
		vector.FillRect(clip, float32(x), float32(y), float32(w), float32(h), fill, false)
		thick := 1
		if selected {
			thick = 3
		}
		drawRectBorder(clip, x, y, w, h, thick, c)
	}
	for i, e := range d.state.Adventure.Effects {
		c := color.RGBA{241, 181, 89, 255}
		if e.Kind == "lane" {
			c = color.RGBA{241, 112, 122, 255}
		}
		if e.Kind == "transfer" {
			c = color.RGBA{94, 195, 240, 255}
		}
		drawBox(e.Rect, c, p.section == "effect" && p.selected == i)
		if e.Kind == "transfer" {
			sx, sy := l.originX+e.Rect[0]*l.tileSize+l.tileSize/2, l.originY+e.Rect[1]*l.tileSize+l.tileSize/2
			dx, dy := l.originX+e.Destination[0]*l.tileSize+l.tileSize/2, l.originY+e.Destination[1]*l.tileSize+l.tileSize/2
			vector.StrokeLine(clip, float32(sx), float32(sy), float32(dx), float32(dy), 1, c, false)
			drawRectBorder(clip, dx-l.tileSize/2, dy-l.tileSize/2, l.tileSize, l.tileSize, 2, c)
		}
	}
	for i, n := range d.state.Data.NPCSpawns {
		if def := d.npc(n.NPCKey); def != nil && d.state.NPCs[n.NPCKey] != nil {
			drawTileThumb(clip, l.originX, l.originY, l.tileSize, n.X, n.Y, def.Sprite, overlayAccent, "@", v.tileSpriteThumbnail)
		}
		if (p.section == "object" || p.section == "choice") && p.selected == i {
			drawBox([4]int{n.X, n.Y, n.X, n.Y}, overlayAccent, true)
		}
	}
	for _, m := range d.state.Data.MonsterSpawns {
		if b := d.state.Adventure.Boss; b != nil && m.MonsterKey == b.Monster {
			drawBox([4]int{m.X, m.Y, m.X, m.Y}, color.RGBA{187, 144, 244, 255}, p.section == "boss")
		}
	}
	for i, c := range d.state.Adventure.Controls {
		key, def := d.controlNPC(c.ID)
		for _, n := range d.state.Data.NPCSpawns {
			if n.NPCKey == key {
				if d.state.NPCs[key] != nil && def != nil {
					drawTileThumb(clip, l.originX, l.originY, l.tileSize, n.X, n.Y, def.Sprite, overlayAccent, "@", v.tileSpriteThumbnail)
				}
				drawBox([4]int{n.X, n.Y, n.X, n.Y}, overlayAccent, p.section == "control" && p.selected == i)
			}
		}
	}
	if b := d.state.Adventure.Boss; b != nil && b.Arena != nil {
		r := b.Arena
		x, y := l.originX+r[0]*l.tileSize, l.originY+r[1]*l.tileSize
		w, h := (r[2]-r[0]+1)*l.tileSize, (r[3]-r[1]+1)*l.tileSize
		thick := 1
		if p.section == "arena" {
			thick = 2
		}
		drawRectBorder(clip, x, y, w, h, thick, overlayBossColor)
		if p.section == "arena" {
			for _, corner := range [][2]int{{x, y}, {x + w - 4, y}, {x, y + h - 4}, {x + w - 4, y + h - 4}} {
				drawFilledRect(clip, corner[0], corner[1], 4, 4, overlayBossColor)
			}
		}
	}
	for i, j := range d.state.Adventure.JumpLinks {
		for _, xy := range [][2]int{{j[0], j[1]}, {j[2], j[3]}} {
			drawBox([4]int{xy[0], xy[1], xy[0], xy[1]}, color.RGBA{225, 225, 240, 255}, p.section == "link" && p.selected == i)
		}
	}
	if p.dragging && p.tool != "move" {
		drawBox(overlayBox(p.anchor, p.end), overlayAccent, true)
	}
	if p.anchorSet {
		drawBox([4]int{p.anchor[0], p.anchor[1], p.anchor[0], p.anchor[1]}, overlayAccent, true)
	}
	mx, my := ebiten.CursorPosition()
	help := "Wheel: zoom map or scroll panels. Right-drag: pan. Ctrl+S: save. Ctrl+Z: undo."
	for _, w := range v.overlayWidgets(d) {
		hover := pointInRect(mx, my, w.r.x, w.r.y, w.r.w, w.r.h)
		if hover {
			help = w.help
		}
		fg, bg := overlayText, overlayButton
		if w.action == nil {
			fg = overlayMuted
			bg = overlayBG
		}
		if w.active {
			bg = color.RGBA{43, 78, 75, 255}
		}
		if hover && w.action != nil {
			bg = color.RGBA{55, 69, 82, 255}
		}
		if w.kind == "heading" {
			game.DrawShadedText(screen, clipText(w.text, w.r.w-12), w.r.x+6, w.r.y+10, overlayMuted)
			continue
		}
		if w.kind != "tree" || w.active || hover {
			drawFilledRect(screen, w.r.x, w.r.y, w.r.w, w.r.h, bg)
		}
		if w.kind == "tree" && w.active {
			drawFilledRect(screen, w.r.x, w.r.y, 2, w.r.h, overlayAccent)
		}
		if w.kind != "tree" {
			drawRectBorder(screen, w.r.x, w.r.y, w.r.w, w.r.h, 1, overlayBorder)
		}
		game.DrawShadedText(screen, clipText(w.text, w.r.w-12), w.r.x+6, w.r.y+7, fg)
	}
	_, treeMax := v.overlayTree(d)
	if treeMax > 0 {
		h := max(24, 616*616/(treeMax+616))
		y := 90 + (616-h)*min(p.treeScroll, treeMax)/treeMax
		drawFilledRect(screen, 227, y, 2, h, overlayBorder)
	}
	help = v.drawOverlayInspector(screen, d, mx, my, help)
	status := d.status
	if d.dirty() {
		status = "UNSAVED - " + status
	}
	if d.err != "" {
		status = "ERROR: " + d.err
	}
	if p.tool != "" {
		instruction := map[string]string{"boss-place": "Click empty ground to place or move this boss.", "object-new": "Click empty ground to place this object.", "object-move": "Click empty ground to move the selected object.", "special-new": "Click empty ground to place this special tile.", "healing-control": "Click empty ground to place a regeneration switch.", "receiver": "Click a safe receiving tile outside all traps, hazards and transfer zones.", "new-control": "Click empty walkable ground to place a new switch.", "switch": "Click empty walkable ground to move this switch.", "tile-change": "Click the terrain cell this switch will change.", "link-new": "Click the launch tile, then a landing tile two cells away.", "link-redraw": "Click the launch tile, then a landing tile two cells away."}[p.tool]
		if instruction == "" {
			instruction = "Drag a rectangle on the map."
		}
		help = instruction + " Escape cancels."
	}
	if tile, ok := overlayTileAt(l, mx, my); ok {
		game.DrawPlainText(screen, fmt.Sprintf("Tile %d, %d", tile[0], tile[1]), 242, 718)
	}
	for i, line := range wrapTooltipLines(help, game.ShadedTextColumns(1170)) {
		if i >= 2 {
			break
		}
		game.DrawPlainText(screen, line, 12, 740+i*16)
	}
	for i, line := range wrapTooltipLines(status, game.ShadedTextColumns(1170)) {
		if i >= 2 {
			break
		}
		game.DrawShadedText(screen, line, 12, 770+i*14, func() color.RGBA {
			if d.err != "" {
				return color.RGBA{255, 142, 132, 255}
			}
			return overlayAccent
		}())
	}
}

// A page change or keyboard command must never leave a half-applied drag.
func (v *viewer) overlayCancelGesture(d *overlayDocument) {
	p := &v.overlay
	if p.dragging {
		d.state = p.beforeDrag
		p.dragging = false
	}
	p.tool = ""
	p.anchorSet = false
	p.pan = false
}

func (v *viewer) overlayReceiverError(d *overlayDocument, tile [2]int) error {
	data := d.state.Data
	x, y := tile[0], tile[1]
	if x < 0 || y < 0 || x >= data.Width || y >= data.Height || !v.tileManager.IsWalkable(data.Tiles[y][x]) {
		return fmt.Errorf("Receiver must be on walkable ground")
	}
	for _, e := range d.state.Adventure.Effects {
		if e.Kind != "lane" && e.Contains(x, y) {
			return fmt.Errorf("Receiver overlaps %s", e.Name)
		}
	}
	return nil
}

func (v *viewer) overlayHistory(d *overlayDocument, redo bool) {
	v.overlayCancelGesture(d)
	d.history(redo)
	if len(v.overlayFields(d)) == 0 {
		v.overlaySelect("overview", 0)
	}
}

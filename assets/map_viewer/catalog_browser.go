package main

import (
	"fmt"
	"image"
	"maps"
	"slices"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"gopkg.in/yaml.v3"
)

const catalogRowHeight = 28

// Catalog filters are presentation state. They never alter map indices or
// brush eligibility, which are shared with the runtime map loader.
type editorBrowser struct {
	mapQuery, mapBiome, mapSort         string
	legendQuery, legendKind             string
	mobQuery, mobGroup, mobSort, mobTab string
	fxQuery, fxKind                     string
	fxPaused                            bool
	focus                               string
	infoScroll                          int
	closed                              map[string]bool
	revision                            int
	mobSelection, fxSelection           catalogEntryRow
	scrollDrag                          string // pane whose scrollbar holds the left button
	scrollGrab                          int    // cursor offset inside the dragged thumb
	mobRows, fxRows                     catalogCache[catalogListKey, []catalogEntryRow]
	mobDetails                          catalogCache[mobDetailKey, []catalogDetailRow]
	mapDetails                          catalogCache[mapDetailKey, []infoLine]
}

type mapDetailKey struct {
	config                                                        *config.MapConfig
	key, lighting                                                 string
	brush                                                         brush
	width, revision                                               int
	dataPresent                                                   bool
	mapWidth, mapHeight, startX, startY, monsters, npcs, specials int
}

func (v *viewer) mapDetailRows(m mapInfo, width int) []infoLine {
	k := mapDetailKey{config: m.Config, key: m.Key, lighting: m.LightingText, brush: v.brush, width: width, revision: v.browser.revision}
	if d := m.Data; d != nil {
		k.dataPresent = true
		k.mapWidth, k.mapHeight, k.startX, k.startY = d.Width, d.Height, d.StartX, d.StartY
		k.monsters, k.npcs, k.specials = len(d.MonsterSpawns), len(d.NPCSpawns), len(d.SpecialTileSpawns)
	}
	return v.browser.mapDetails.get(k, func() []infoLine {
		rows := appendInfoHeader(buildMapInfoLines(m, v.brush), "MAP CONFIGURATION")
		rows = append(rows, catalogSourceLines(m.Config)...)
		var wrapped []infoLine
		for _, row := range rows {
			wrapped = append(wrapped, wrapCatalogInfo([]infoLine{row}, width-32-row.depth*12)...)
		}
		return wrapped
	})
}

func (v *viewer) catalogMapReady() bool {
	return len(v.maps) > 0 && v.maps[v.mapIndex].Err == nil && v.maps[v.mapIndex].Data != nil
}

type catalogControl struct {
	r            rect
	label, field string
	action       func()
}

// canvasSize is the logical canvas every page lays out in: the window size,
// never smaller than the authored 1200x800 (see Layout).
func (v *viewer) canvasSize() (int, int) {
	if v.viewportW <= 0 || v.viewportH <= 0 {
		return windowWidth, windowHeight
	}
	return v.viewportW, v.viewportH
}

func (v *viewer) catalogSidebarWidth() int {
	w, _ := v.canvasSize()
	return min(520, max(350, w*30/100))
}

func (v *viewer) catalogText(field string) *string {
	switch field {
	case "map":
		return &v.browser.mapQuery
	case "legend":
		return &v.browser.legendQuery
	case "mob":
		return &v.browser.mobQuery
	case "fx":
		return &v.browser.fxQuery
	}
	return nil
}

func (v *viewer) catalogChoose(title string, choices []string, apply func(string)) {
	v.browser.focus = ""
	v.overlay.modal = &overlayModal{title: title, help: "Type to filter; click a result to choose.", choices: choices, apply: func(s string) error { apply(s); return nil }}
}

func (v *viewer) catalogMaps() []int {
	var out []int
	for i, m := range v.maps {
		if v.browser.mapBiome != "" && m.Config != nil && m.Config.Biome != v.browser.mapBiome {
			continue
		}
		name := m.Key
		if m.Config != nil {
			name += " " + m.Config.Name + " " + m.Config.Biome
		}
		if strings.Contains(strings.ToLower(name), strings.ToLower(v.browser.mapQuery)) {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := v.maps[out[i]], v.maps[out[j]]
		if v.browser.mapSort == "Size" {
			aa, bb := -1, -1
			if a.Data != nil {
				aa = a.Data.Width * a.Data.Height
			}
			if b.Data != nil {
				bb = b.Data.Width * b.Data.Height
			}
			if aa != bb {
				return aa > bb
			}
		}
		if v.browser.mapSort == "Biome" && a.Config != nil && b.Config != nil && a.Config.Biome != b.Config.Biome {
			return a.Config.Biome < b.Config.Biome
		}
		an, bn := a.Key, b.Key
		if a.Config != nil && a.Config.Name != "" {
			an = a.Config.Name
		}
		if b.Config != nil && b.Config.Name != "" {
			bn = b.Config.Name
		}
		if an != bn {
			return an < bn
		}
		return a.Key < b.Key
	})
	return out
}

func (v *viewer) selectCatalogMap(i int) {
	if i < 0 || i >= len(v.maps) || i == v.mapIndex {
		return
	}
	v.mapIndex = i
	v.resetMapView()
	v.browser.infoScroll = 0
	v.refreshLegend()
}

func (v *viewer) stepCatalogMap(delta int) {
	indices := v.catalogMaps()
	if len(indices) == 0 {
		return
	}
	pos := -1
	for i, index := range indices {
		if index == v.mapIndex {
			pos = i
			break
		}
	}
	if pos < 0 {
		if delta < 0 {
			pos = 0
		} else {
			pos = -1
		}
	}
	v.selectCatalogMap(indices[(pos+delta+len(indices))%len(indices)])
}

func (v *viewer) catalogControls() []catalogControl {
	if v.page == pageFX {
		return v.fxCatalogControls()
	}
	if v.page == pageMobs {
		return v.mobCatalogControls()
	}
	if len(v.maps) == 0 {
		return nil
	}
	l := v.computeLayout(v.maps[v.mapIndex])
	x, y := l.mapAreaX, pageBarHeight+12
	var out []catalogControl
	add := func(w int, label, field string, action func()) {
		out = append(out, catalogControl{rect{x, y, w, 30}, label, field, action})
		x += w + 8
	}
	add(180, "Search maps", "map", nil)
	biome := v.browser.mapBiome
	if biome == "" {
		biome = "All biomes"
	}
	add(132, biome, "", func() {
		set := map[string]bool{"All biomes": true}
		for _, m := range v.maps {
			if m.Config != nil {
				set[m.Config.Biome] = true
			}
		}
		choices := slices.Sorted(maps.Keys(set))
		v.catalogChoose("Map biome", choices, func(s string) {
			if s == "All biomes" {
				s = ""
			}
			v.browser.mapBiome = s
		})
	})
	order := v.browser.mapSort
	if order == "" {
		order = "Name"
	}
	add(100, order, "", func() {
		v.catalogChoose("Sort maps", []string{"Name", "Biome", "Size"}, func(s string) { v.browser.mapSort = s })
	})
	add(44, "<", "", func() { v.stepCatalogMap(-1) })
	add(44, ">", "", func() { v.stepCatalogMap(1) })
	add(64, "Fit", "", func() { v.resetMapView() })
	add(min(240, max(90, l.mapAreaX+l.mapAreaW-x)), fmt.Sprintf("Maps (%d)", len(v.catalogMaps())), "", func() {
		var choices []string
		indices := map[string]int{}
		for _, i := range v.catalogMaps() {
			m := v.maps[i]
			label := m.Key
			if m.Config != nil {
				label += " - " + m.Config.Name
			}
			choices = append(choices, label)
			indices[label] = i
		}
		if len(choices) > 0 {
			v.catalogChoose("Choose map", choices, func(s string) { v.selectCatalogMap(indices[s]) })
		}
	})
	if v.sidebarTab == tabLegend && v.catalogMapReady() {
		x, y = l.legendX+8, l.mapAreaY+32
		add(l.legendW-16, "Search palette", "legend", nil)
		x, y = l.legendX+8, y+36
		kind := v.browser.legendKind
		if kind == "" {
			kind = "All objects"
		}
		add(l.legendW-104, kind, "", func() {
			v.catalogChoose("Palette category", []string{"All objects", "Terrain / tiles", "Monsters", "NPCs / services", "Special tiles", "Decorations", "Tools"}, func(s string) {
				if s == "All objects" {
					s = ""
				}
				v.browser.legendKind = s
				v.rebuildLegend(true)
			})
		})
		add(80, "Fold all", "", func() {
			for _, e := range v.filteredLegend() {
				if e.CollapseID != "" {
					if v.legendCollapsed == nil {
						v.legendCollapsed = map[string]bool{}
					}
					v.legendCollapsed[e.CollapseID] = true
				}
			}
			v.rebuildLegend(true)
		})
	}
	return out
}

func (v *viewer) updateCatalogControls() bool {
	controls := v.catalogControls()
	// Right-drag panning and wheel navigation keep their existing ownership,
	// even after typing in a search field.
	_, wheel := ebiten.Wheel()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || wheel != 0 {
		v.browser.focus = ""
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		v.browser.focus = ""
		for _, c := range controls {
			if pointInRect(x, y, c.r.x, c.r.y, c.r.w, c.r.h) {
				v.browser.focus = c.field
				if c.action != nil {
					c.action()
				}
				return true
			}
		}
	}
	text := v.catalogText(v.browser.focus)
	if text == nil {
		return false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		v.browser.focus = ""
		return true
	}
	before := *text
	if (ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)) && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		*text = ""
	}
	if overlayKey(ebiten.KeyBackspace) && len(*text) > 0 {
		*text = (*text)[:len(*text)-1]
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 32 && r < 127 && len(*text) < 120 {
			*text += string(r)
		}
	}
	if before != *text {
		if v.browser.focus == "legend" {
			v.rebuildLegend(true)
		}
		if v.browser.focus == "mob" {
			mobsPage.scroll = 0
		}
		if v.browser.focus == "fx" {
			fxPage.scroll = 0
		}
	}
	return true
}

func (v *viewer) drawCatalogControls(screen *ebiten.Image) {
	for _, c := range v.catalogControls() {
		bg, fg := overlayButton, overlayText
		label := c.label
		if text := v.catalogText(c.field); text != nil {
			bg = overlayInput
			fg = overlayMuted
			if *text != "" {
				label = *text
				fg = overlayText
			}
			if v.browser.focus == c.field {
				label += "|"
			}
		}
		drawFilledRect(screen, c.r.x, c.r.y, c.r.w, c.r.h, bg)
		border := overlayBorder
		if c.field != "" && v.browser.focus == c.field {
			border = overlayAccent
		}
		drawRectBorder(screen, c.r.x, c.r.y, c.r.w, c.r.h, 1, border)
		if v.page == pageMobs {
			tab := v.browser.mobTab
			if tab == "" {
				tab = "Overview"
			}
			if c.label == tab {
				drawFilledRect(screen, c.r.x, c.r.y+c.r.h-2, c.r.w, 2, overlayAccent)
			}
		}
		game.DrawShadedText(screen, clipText(label, c.r.w-18), c.r.x+9, c.r.y+8, fg)
	}
}

func (v *viewer) drawLegendHoverTooltip(screen *ebiten.Image, l layout) {
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		return
	}
	x, y := ebiten.CursorPosition()
	e := v.legendEntryAt(l, x, y)
	if e == nil || e.IsHeader {
		return
	}
	lines := []string{"PALETTE OBJECT"}
	lines = append(lines, wrapTooltipLines(e.Text, 64)...)
	switch e.Kind {
	case brushTile, brushGeneral, brushSpecialTile:
		lines = append(lines, "Key: "+e.TileKey)
		data := v.tileDataByKey[e.TileKey]
		if data == nil && v.tileManager != nil {
			data = v.tileManager.ListSpecialTiles()[e.TileKey]
		}
		if data != nil {
			lines = appendTileTooltipLines(lines, data)
		}
	case brushMonster:
		lines = append(lines, "Key: "+e.MonsterKey)
		if d, ok := v.monsterCfg.Monsters[e.MonsterKey]; ok {
			lines = append(lines, mapMonsterStatLines(d)...)
			lines = append(lines, "Full abilities and encounters: Mobs tab")
		}
	case brushNPC:
		lines = append(lines, "Key: "+e.NPCKey)
		if character.NPCConfigInstance != nil {
			if d := character.NPCConfigInstance.NPCs[e.NPCKey]; d != nil {
				lines = append(lines, "Type: "+d.Type, "Render: "+d.RenderCategory)
				lines = append(lines, wrapTooltipLines(d.Description, 64)...)
				lines = append(lines, d.AvailabilityLines()...)
				lines = append(lines, character.TrainingOfferLines(d.Training)...)
			}
		}
	}
	lines = append(lines, "", "Click: select brush   Shift: sprite preview")
	drawTooltipBox(screen, lines, x, y)
}

func legendCatalogKind(e legendEntry) string {
	switch e.Kind {
	case brushTile:
		return "Terrain / tiles"
	case brushMonster:
		return "Monsters"
	case brushNPC:
		return "NPCs / services"
	case brushSpecialTile:
		return "Special tiles"
	case brushGeneral:
		return "Decorations"
	default:
		return "Tools"
	}
}

func (v *viewer) legendSubgroup(e legendEntry) string {
	if t := v.tileDataByKey[e.TileKey]; t != nil {
		return t.RenderType + " / " + t.Type
	}
	if e.Kind == brushMonster {
		d := v.monsterCfg.Monsters[e.MonsterKey]
		if d.Boss {
			return "Bosses"
		}
		if d.Type != "" {
			return d.Type
		}
		return "Creatures"
	}
	if e.Kind == brushNPC && character.NPCConfigInstance != nil {
		if d := character.NPCConfigInstance.NPCs[e.NPCKey]; d != nil {
			s := d.Type
			if d.PropModel != nil {
				s += " / " + d.PropModel.Shape
			}
			return s
		}
	}
	return "General"
}

// Reuse the canonical palette before applying display-only grouping/search.
// Wrapped source labels are rejoined; an object has exactly one selectable row.
func (v *viewer) filteredLegend() []legendEntry {
	all := buildLegendEntries(v.tileManager, v.monsterCfg, v.currentBiome(), nil)
	var leaves []legendEntry
	for _, e := range all {
		if e.Continuation {
			if len(leaves) > 0 {
				leaves[len(leaves)-1].Text += " " + strings.TrimSpace(e.Text)
			}
			continue
		}
		if e.IsHeader || e.Kind == brushNone {
			continue
		}
		leaves = append(leaves, e)
	}
	groups := map[string]map[string][]legendEntry{}
	for _, e := range leaves {
		kind := legendCatalogKind(e)
		if v.browser.legendKind != "" && v.browser.legendKind != kind {
			continue
		}
		sub := v.legendSubgroup(e)
		if !strings.Contains(strings.ToLower(e.Text+" "+e.TileKey+" "+e.NPCKey+" "+e.MonsterKey+" "+sub), strings.ToLower(v.browser.legendQuery)) {
			continue
		}
		if groups[kind] == nil {
			groups[kind] = map[string][]legendEntry{}
		}
		groups[kind][sub] = append(groups[kind][sub], e)
	}
	var out []legendEntry
	search := v.browser.legendQuery != ""
	for _, kind := range slices.Sorted(maps.Keys(groups)) {
		count := 0
		for _, es := range groups[kind] {
			count += len(es)
		}
		id := "catalog:" + kind
		closed := v.legendCollapsed[id] && !search
		out = append(out, collapsibleHeader(fmt.Sprintf("%s (%d)", kind, count), id, closed, 0))
		if closed {
			continue
		}
		for _, sub := range slices.Sorted(maps.Keys(groups[kind])) {
			es := groups[kind][sub]
			sort.SliceStable(es, func(i, j int) bool { return es[i].Text < es[j].Text })
			// Long homogeneous groups get alphabetical ranges, not 100-row walls.
			for start := 0; start < len(es); start += 24 {
				end := min(start+24, len(es))
				label := sub
				if len(es) > 24 {
					label += fmt.Sprintf(" [%d-%d]", start+1, end)
				}
				sid := fmt.Sprintf("%s/%s/%d", id, sub, start)
				closed := v.legendCollapsed[sid] && !search
				out = append(out, collapsibleHeader(fmt.Sprintf("%s (%d)", label, end-start), sid, closed, 1))
				if !closed {
					out = append(out, es[start:end]...)
				}
			}
		}
	}
	return out
}

func catalogClip(screen *ebiten.Image, r rect) *ebiten.Image {
	return screen.SubImage(image.Rect(r.x, r.y, r.x+r.w, r.y+r.h).Intersect(screen.Bounds())).(*ebiten.Image)
}

func (v *viewer) drawCatalogSidebar(screen *ebiten.Image, m mapInfo, l layout) {
	drawFilledRect(screen, l.legendX, l.mapAreaY, l.legendW, l.legendY+l.legendH-l.mapAreaY, overlayBG)
	drawSidebarTabs(screen, l.legendX, l.mapAreaY, l.legendW, 24, v.sidebarTab)
	r := rect{l.legendX, l.legendY, l.legendW, l.legendH}
	clip := catalogClip(screen, r)
	if v.sidebarTab == tabLegend {
		drawLegendList(clip, r.x, r.y, r.w, r.h, v.legendLines, v.legendScroll, v.brush, v.tileDataByKey, effectiveFloorColor(m, v.tileManager, v.tileDataByKey), v.tileSpriteThumbnail)
		if len(v.legendLines) == 0 {
			game.DrawShadedText(clip, "No matching objects", r.x+12, r.y+12, overlayMuted)
		}
		drawCatalogScroll(screen, r, v.legendScroll, len(v.legendLines)*catalogRowHeight)
		return
	}
	rows := v.mapDetailRows(m, r.w)
	v.browser.infoScroll = min(v.browser.infoScroll, max(0, len(rows)*22-r.h))
	for i, row := range rows {
		y := r.y + i*22 - v.browser.infoScroll
		if y < r.y-22 || y >= r.y+r.h {
			continue
		}
		if row.header {
			drawFilledRect(clip, r.x+8, y, r.w-16, 22, overlayButton)
		}
		game.DrawShadedText(clip, row.text, r.x+12+row.depth*12, y+4, row.col)
	}
	drawCatalogScroll(screen, r, v.browser.infoScroll, len(rows)*22)
}

// The source inspector reads the actual typed catalog, so uncommon authored
// fields remain inspectable without maintaining another field whitelist.
func catalogSourceLines(value any) []infoLine {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return []infoLine{{text: err.Error(), col: overlayDanger}}
	}
	var out []infoLine
	var walk func(*yaml.Node, int)
	walk = func(n *yaml.Node, depth int) {
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i < len(n.Content); i += 2 {
				k, val := n.Content[i].Value, n.Content[i+1]
				if value, ok := catalogScalarValue(val); ok {
					out = append(out, infoLine{text: k + ": " + value, col: overlayText, depth: depth})
				} else {
					out = append(out, infoLine{text: k, col: overlayAccent, header: true, depth: depth})
					walk(val, depth+1)
				}
			}
		case yaml.SequenceNode:
			for i, val := range n.Content {
				if value, ok := catalogScalarValue(val); ok {
					out = append(out, infoLine{text: "- " + value, col: overlayText, depth: depth})
				} else {
					out = append(out, infoLine{text: fmt.Sprintf("Entry %d", i+1), col: overlayMuted, header: true, depth: depth})
					walk(val, depth+1)
				}
			}
		case yaml.DocumentNode:
			for _, val := range n.Content {
				walk(val, depth)
			}
		}
	}
	walk(&node, 0)
	return out
}

func catalogScalarValue(n *yaml.Node) (string, bool) {
	if n.Kind == yaml.ScalarNode {
		return n.Value, true
	}
	if n.Kind != yaml.SequenceNode {
		return "", false
	}
	var values []string
	for _, child := range n.Content {
		if child.Kind != yaml.ScalarNode {
			return "", false
		}
		values = append(values, child.Value)
	}
	return "[" + strings.Join(values, ", ") + "]", true
}

func nestedCatalogSource(value any) []infoLine {
	rows := catalogSourceLines(value)
	for i := range rows {
		rows[i].depth++
	}
	return rows
}

func wrapCatalogInfo(rows []infoLine, width int) []infoLine {
	var out []infoLine
	for _, row := range rows {
		words := strings.Fields(row.text)
		line := ""
		for _, word := range words {
			if line != "" && game.ShadedTextWidth(line+" "+word) > width {
				r := row
				r.text = line
				out = append(out, r)
				line = ""
			}
			if line != "" {
				line += " "
			}
			for _, ch := range word {
				if line != "" && game.ShadedTextWidth(line+string(ch)) > width {
					r := row
					r.text = line
					out = append(out, r)
					line = ""
				}
				line += string(ch)
			}
		}
		row.text = line
		out = append(out, row)
	}
	return out
}

// catalogScrollThumb is the thumb geometry shared by drawing and dragging.
func catalogScrollThumb(r rect, offset, total int) (y, h int, ok bool) {
	if total <= r.h {
		return 0, 0, false
	}
	h = max(24, r.h*r.h/total)
	return r.y + (r.h-h)*offset/max(1, total-r.h), h, true
}

func drawCatalogScroll(screen *ebiten.Image, r rect, offset, total int) {
	y, h, ok := catalogScrollThumb(r, offset, total)
	if !ok {
		return
	}
	drawFilledRect(screen, r.x+r.w-4, r.y, 3, r.h, overlayInput)
	drawFilledRect(screen, r.x+r.w-4, y, 3, h, overlayAccent)
}

// catalogScrollGrip is the strip at a pane's right edge that grabs its bar.
const catalogScrollGrip = 12

// dragCatalogScroll drags a pane's scrollbar by its thumb; pressing the track
// centres the thumb under the cursor first. owned means the left button
// belongs to the scrollbar this frame, so the pane must not treat it as a
// row click.
func (v *viewer) dragCatalogScroll(id string, r rect, offset, total int, in catalogListInput) (int, bool) {
	return v.dragScrollbar(id, r, offset, total, in, catalogScrollGrip)
}

// dragScrollbar is dragCatalogScroll with an explicit grab-strip width, for
// panes whose rows run close to their bar.
func (v *viewer) dragScrollbar(id string, r rect, offset, total int, in catalogListInput, grip int) (int, bool) {
	y, h, ok := catalogScrollThumb(r, offset, total)
	if !ok || !in.held {
		if v.browser.scrollDrag == id {
			v.browser.scrollDrag = ""
		}
		return offset, false
	}
	if in.click && pointInRect(in.x, in.y, r.x+r.w-grip, r.y, grip, r.h) {
		v.browser.scrollDrag, v.browser.scrollGrab = id, in.y-y
		if in.y < y || in.y >= y+h {
			v.browser.scrollGrab = h / 2
		}
	}
	if v.browser.scrollDrag != id {
		return offset, false
	}
	return catalogScrollOffset(r, h, total, in.y-v.browser.scrollGrab), true
}

// catalogScrollOffset inverts catalogScrollThumb for a thumb top at thumbY.
func catalogScrollOffset(r rect, h, total, thumbY int) int {
	thumbY = clampInt(thumbY, r.y, r.y+r.h-h)
	return (thumbY - r.y) * (total - r.h) / max(1, r.h-h)
}

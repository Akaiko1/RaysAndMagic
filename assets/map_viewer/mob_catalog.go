package main

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"

	"github.com/hajimehoshi/ebiten/v2"
)

type mobCatalogLayout struct{ list, stage, details rect }

func (v *viewer) mobCatalogLayout() mobCatalogLayout {
	w, h := v.canvasSize()
	lw := min(360, max(280, w/4))
	dw := min(520, max(360, w*30/100))
	return mobCatalogLayout{rect{16, 142, lw - 16, h - 174}, rect{lw + 16, 142, w - lw - dw - 48, h - 174}, rect{w - dw, 142, dw - 16, h - 174}}
}

func mobCatalogGroup(d monster.MonsterDefinition, mode string) []string {
	switch mode {
	case "Biome":
		if len(d.Biomes) > 0 {
			return d.Biomes
		}
		return []string{"Universal"}
	case "Type":
		if d.Type != "" {
			return []string{titleCase(d.Type)}
		}
		return []string{"Other creatures"}
	default:
		if d.Boss {
			return []string{"Bosses"}
		}
		if d.Champion != "" {
			return []string{"Champions"}
		}
		if !d.HasAttackStats() {
			return []string{"Non-combatants"}
		}
		if d.Telegraph != nil {
			return []string{"Telegraphed attacks"}
		}
		if d.ProjectileSpell != "" || d.ProjectileWeapon != "" {
			return []string{"Ranged"}
		}
		return []string{"Melee"}
	}
}

func (v *viewer) mobCatalogRows() []catalogEntryRow {
	key := catalogListKey{v.browser.mobQuery, v.browser.mobGroup, v.browser.mobSort, v.browser.revision, len(mobsPage.keys)}
	return v.browser.mobRows.get(key, v.buildMobCatalogRows)
}

func (v *viewer) buildMobCatalogRows() []catalogEntryRow {
	groups := map[string][]catalogEntryRow{}
	for i, key := range mobsPage.keys {
		d := v.monsterCfg.Monsters[key]
		search := key + " " + d.Name + " " + d.Type + " " + strings.Join(d.Biomes, " ")
		if !strings.Contains(strings.ToLower(search), strings.ToLower(v.browser.mobQuery)) {
			continue
		}
		for _, group := range mobCatalogGroup(d, v.browser.mobGroup) {
			groups[group] = append(groups[group], catalogEntryRow{fmt.Sprintf("L%d  %s", d.Level, d.Name), "", i})
		}
	}
	return v.catalogGroupRows("mobs:"+v.browser.mobGroup+":", v.browser.mobQuery, groups, func(left, right catalogEntryRow) bool {
		a, b := v.monsterCfg.Monsters[mobsPage.keys[left.index]], v.monsterCfg.Monsters[mobsPage.keys[right.index]]
		if v.browser.mobSort == "Name" {
			if a.Name != b.Name {
				return a.Name < b.Name
			}
		} else if a.Level != b.Level {
			if v.browser.mobSort == "Level descending" {
				return a.Level > b.Level
			}
			return a.Level < b.Level
		}
		return mobsPage.keys[left.index] < mobsPage.keys[right.index]
	})
}

func (v *viewer) mobCatalogControls() []catalogControl {
	l := v.mobCatalogLayout()
	group := v.browser.mobGroup
	if group == "" {
		group = "Role"
	}
	order := v.browser.mobSort
	if order == "" {
		order = "Level"
	}
	out := []catalogControl{
		{rect{16, 80, l.list.w - 104, 30}, "Search name, key, biome", "mob", nil},
		v.catalogFoldControl(rect{l.list.x + l.list.w - 96, 80, 96, 30}, v.mobCatalogRows),
		{rect{l.stage.x, 80, 150, 30}, "Group: " + group, "", func() {
			v.catalogChoose("Group monsters", []string{"Role", "Biome", "Type"}, func(s string) { v.browser.mobGroup = s; mobsPage.scroll = 0 })
		}},
		{rect{l.stage.x + 158, 80, 160, 30}, order, "", func() {
			v.catalogChoose("Sort monsters", []string{"Level", "Level descending", "Name"}, func(s string) { v.browser.mobSort = s; mobsPage.scroll = 0 })
		}},
	}
	x := l.details.x
	for _, tab := range []string{"Overview", "Abilities", "Drops", "Source"} {
		tab := tab
		tw := (l.details.w - 18) / 4
		out = append(out, catalogControl{rect{x, 80, tw, 30}, tab, "", func() { v.browser.mobTab = tab; mobsPage.infoOffset = 0 }})
		x += tw + 6
	}
	return out
}

func (v *viewer) toggleCatalogSection(id string) {
	if v.browser.closed == nil {
		v.browser.closed = map[string]bool{}
	}
	v.browser.closed[id] = !v.browser.closed[id]
	v.browser.revision++
}

func (v *viewer) mobDetailRows() []infoLine {
	if len(mobsPage.keys) == 0 {
		return nil
	}
	key := mobsPage.keys[mobsPage.selIdx]
	tab := v.browser.mobTab
	var out []infoLine
	if tab == "Source" {
		return catalogSourceLines(v.monsterCfg.Monsters[key])
	}
	section := ""
	for _, row := range mobsPage.info {
		if row.header {
			section = row.text
		}
		include := false
		switch tab {
		case "Abilities":
			include = section == "ABILITIES" || section == "CHAMPION" || section == "BEHAVIOR"
		case "Drops":
			include = section == "DROPS"
		default:
			include = section != "ABILITIES" && section != "DROPS" && section != "BEHAVIOR" && section != "CHAMPION"
		}
		if include {
			if !row.header {
				row.depth++
			}
			out = append(out, row)
		}
	}
	if tab == "Abilities" {
		for _, m := range v.maps {
			if m.Config == nil || m.Config.Adventure == nil || m.Config.Adventure.Boss == nil || m.Config.Adventure.Boss.Monster != key {
				continue
			}
			a := m.Config.Adventure
			out = appendInfoHeader(out, "ENCOUNTER: %s", m.Key)
			out = append(out, infoLine{text: "Map-specific mechanics, not base abilities.", col: overlayMuted, depth: 1})
			out = append(out, nestedCatalogSource(a.Boss)...)
			out = appendInfoHeader(out, "ENCOUNTER FIELDS: %s", m.Key)
			out = append(out, nestedCatalogSource(a.Effects)...)
			if config.GlobalBossMechanics != nil && len(config.GlobalBossMechanics.Maps[m.Key]) > 0 {
				out = appendInfoHeader(out, "CONTROL-LINKED STATS: %s", m.Key)
				out = append(out, nestedCatalogSource(config.GlobalBossMechanics.Maps[m.Key])...)
			}
		}
		if len(out) == 0 {
			out = append(out, infoLine{text: "No special abilities.", col: overlayMuted})
		}
	}
	return out
}

type catalogDetailRow struct {
	infoLine
	id string
}

type mobDetailKey struct {
	first                     *infoLine
	tab                       string
	width, selected, revision int
}

func (v *viewer) visibleMobDetails(width int) []catalogDetailRow {
	var first *infoLine
	if len(mobsPage.info) > 0 {
		first = &mobsPage.info[0]
	}
	key := mobDetailKey{first, v.browser.mobTab, width, mobsPage.selIdx, v.browser.revision}
	return v.browser.mobDetails.get(key, func() []catalogDetailRow { return v.buildVisibleMobDetails(width) })
}

func (v *viewer) buildVisibleMobDetails(width int) []catalogDetailRow {
	rows := v.mobDetailRows()
	var out []catalogDetailRow
	hiddenDepth := -1
	for i, row := range rows {
		if hiddenDepth >= 0 {
			if row.depth > hiddenDepth {
				continue
			}
			hiddenDepth = -1
		}
		id := ""
		if row.header {
			id := fmt.Sprintf("detail:%d:%s:%d", mobsPage.selIdx, v.browser.mobTab, i)
			mark := "- "
			if v.browser.closed[id] {
				mark = "+ "
				hiddenDepth = row.depth
			}
			row.text = mark + row.text
			for _, r := range wrapCatalogInfo([]infoLine{row}, width-32-row.depth*12) {
				out = append(out, catalogDetailRow{r, id})
			}
			continue
		}
		for _, r := range wrapCatalogInfo([]infoLine{row}, width-32-row.depth*12) {
			out = append(out, catalogDetailRow{r, id})
		}
	}
	return out
}

func (v *viewer) updateMobsPage() {
	v.ensureMobsPage()
	if mobsPage.preview == nil {
		return
	}
	l := v.mobCatalogLayout()
	rows := v.mobCatalogRows()
	in := readCatalogListInput()
	v.updateCatalogList("mobs:list", l.list, rows, &mobsPage.scroll, &v.browser.mobSelection, mobsPage.selIdx, in, v.selectMob)
	details := v.visibleMobDetails(l.details.w)
	capacity := max(1, l.details.h/22)
	infoWheel := 0.0
	if pointInRect(in.x, in.y, l.details.x, l.details.y, l.details.w, l.details.h) {
		infoWheel = in.wheel
	}
	mobsPage.infoOffset = scrollCatalogRows(mobsPage.infoOffset, infoWheel, len(details), capacity)
	if px, dragging := v.dragCatalogScroll("mobs:details", l.details, int(mobsPage.infoOffset)*22, len(details)*22, in); dragging {
		// Rows scroll whole; round so the last row stays reachable.
		mobsPage.infoOffset = scrollCatalogRows(float64((px+11)/22), 0, len(details), capacity)
		in.click = false
	}
	if in.click {
		if pointInRect(in.x, in.y, l.details.x, l.details.y, l.details.w, l.details.h) {
			i := (in.y-l.details.y)/22 + int(mobsPage.infoOffset)
			if i < len(details) && details[i].id != "" {
				v.toggleCatalogSection(details[i].id)
			}
		}
	}
	mobsPage.preview.Step()
}

func (v *viewer) drawMobsPage(screen *ebiten.Image) {
	w, h := v.canvasSize()
	drawFilledRect(screen, 0, pageBarHeight, w, h-pageBarHeight, overlayInput)
	game.DrawShadedText(screen, "BESTIARY", 16, 49, overlayAccent)
	v.drawCatalogControls(screen)
	if mobsPage.initErr != "" {
		game.DrawShadedText(screen, mobsPage.initErr, 16, 150, overlayDanger)
		return
	}
	if mobsPage.preview == nil {
		game.DrawShadedText(screen, "Loading preview...", 16, 150, overlayMuted)
		return
	}
	l := v.mobCatalogLayout()
	for _, r := range []rect{l.list, l.stage, l.details} {
		drawFilledRect(screen, r.x, r.y, r.w, r.h, overlayBG)
		drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, overlayBorder)
	}
	rows := v.mobCatalogRows()
	drawCatalogList(screen, l.list, rows, mobsPage.scroll, catalogSelectedRow(rows, v.browser.mobSelection, mobsPage.selIdx))
	key := mobsPage.keys[mobsPage.selIdx]
	d := v.monsterCfg.Monsters[key]
	game.DrawShadedText(screen, clipText(d.Name, l.stage.w-24), l.stage.x+12, l.stage.y+16, overlayAccent)
	game.DrawShadedText(screen, clipText(key, l.stage.w-24), l.stage.x+12, l.stage.y+40, overlayMuted)
	scene := mobsPage.preview.Scene()
	sw, sh := scene.Bounds().Dx(), scene.Bounds().Dy()
	scale := min(float64(l.stage.w-16)/float64(sw), float64(l.stage.h-190)/float64(sh))
	dw, dh := float64(sw)*scale, float64(sh)*scale
	graphics.DrawImageScaled(screen, scene, float64(l.stage.x)+(float64(l.stage.w)-dw)/2, float64(l.stage.y+76), dw, dh, nil)
	details := v.visibleMobDetails(l.details.w)
	clip := catalogClip(screen, l.details)
	offset := int(mobsPage.infoOffset)
	for i := offset; i < len(details); i++ {
		y := l.details.y + (i-offset)*22
		if y >= l.details.y+l.details.h {
			break
		}
		row := details[i]
		if row.header {
			drawFilledRect(clip, l.details.x+1, y, l.details.w-2, 22, overlayButton)
		}
		game.DrawShadedText(clip, clipText(row.text, l.details.w-32-row.depth*12), l.details.x+12+row.depth*12, y+4, row.col)
	}
	drawCatalogScroll(screen, l.details, offset*22, len(details)*22)
	tab := v.browser.mobTab
	if tab == "" {
		tab = "Overview"
	}
	game.DrawShadedText(screen, strings.ToUpper(tab)+" / click headings to fold", l.details.x, 120, overlayAccent)
	game.DrawShadedText(screen, fmt.Sprintf("%d monsters   |   Arrow keys: select   |   Wheel: scroll hovered pane", len(mobsPage.keys)), 16, h-20, overlayMuted)
}

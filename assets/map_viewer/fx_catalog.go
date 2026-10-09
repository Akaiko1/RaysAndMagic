package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/graphics"

	"github.com/hajimehoshi/ebiten/v2"
)

func (v *viewer) fxCatalogLayout() (list, stage rect) {
	w, h := v.canvasSize()
	lw := min(400, max(320, w/4))
	return rect{16, 142, lw - 16, h - 174}, rect{lw + 16, 142, w - lw - 32, h - 174}
}

func (v *viewer) fxCatalogControls() []catalogControl {
	list, stage := v.fxCatalogLayout()
	kind := v.browser.fxKind
	if kind == "" {
		kind = "All effects"
	}
	pause := "Pause"
	if v.browser.fxPaused {
		pause = "Resume"
	}
	return []catalogControl{
		{rect{16, 80, list.w - 104, 30}, "Search name or key", "fx", nil},
		v.catalogFoldControl(rect{list.x + list.w - 96, 80, 96, 30}, v.fxCatalogRows),
		{rect{stage.x, 80, 190, 30}, kind, "", func() {
			v.catalogChoose("Effect category", fxCategoryChoices(fxPage.items), func(s string) {
				if s == "All effects" {
					s = ""
				}
				v.browser.fxKind = s
				fxPage.scroll = 0
			})
		}},
		{rect{stage.x + 206, 80, 100, 30}, "Replay", "", func() {
			if fxPage.preview != nil && len(fxPage.items) > 0 {
				fxPage.preview.Select(fxPage.items[fxPage.selIdx])
				v.browser.fxPaused = false
			}
		}},
		{rect{stage.x + 314, 80, 100, 30}, pause, "", func() { v.browser.fxPaused = !v.browser.fxPaused }},
	}
}

func (v *viewer) fxCatalogRows() []catalogEntryRow {
	key := catalogListKey{query: v.browser.fxQuery, group: v.browser.fxKind, revision: v.browser.revision, count: len(fxPage.items)}
	return v.browser.fxRows.get(key, v.buildFXCatalogRows)
}

func (v *viewer) buildFXCatalogRows() []catalogEntryRow {
	groups := map[string][]catalogEntryRow{}
	for i, item := range fxPage.items {
		group := fxItemGroup(item)
		// The filter takes a whole kind ("[weapon]") or one of its groups.
		if f := v.browser.fxKind; f != "" && f != fxKindTag(item.Kind) && f != group {
			continue
		}
		if !strings.Contains(strings.ToLower(item.Label+" "+item.Key+" "+group), strings.ToLower(v.browser.fxQuery)) {
			continue
		}
		groups[group] = append(groups[group], catalogEntryRow{label: item.Label, index: i})
	}
	return v.catalogGroupRows("fx:", v.browser.fxQuery, groups, func(a, b catalogEntryRow) bool { return a.label < b.label })
}

// fxItemGroup is an item's list group: its kind tag, with weapons subdivided
// by their authored class (weapons.yaml category).
func fxItemGroup(item game.FxItem) string {
	group := fxKindTag(item.Kind)
	if item.Kind == game.FxWeapon && config.GlobalWeapons != nil {
		if def, ok := config.GlobalWeapons.Weapons[item.Key]; ok && def.Category != "" {
			group += " " + weaponClassLabel(def.Category)
		}
	}
	return group
}

// fxCategoryChoices lists every kind, each followed by the groups its items
// actually form, so the filter offers exactly what the list can show.
func fxCategoryChoices(items []game.FxItem) []string {
	subgroups := map[string]map[string]bool{}
	for _, item := range items {
		if kind, group := fxKindTag(item.Kind), fxItemGroup(item); group != kind {
			if subgroups[kind] == nil {
				subgroups[kind] = map[string]bool{}
			}
			subgroups[kind][group] = true
		}
	}
	choices := []string{"All effects"}
	for _, k := range []game.FxKind{game.FxSpell, game.FxWeapon, game.FxTrap, game.FxTile, game.FxCard, game.FxStatus} {
		kind := fxKindTag(k)
		choices = append(choices, kind)
		choices = append(choices, slices.Sorted(maps.Keys(subgroups[kind]))...)
	}
	return choices
}

// weaponClassLabel names a weapon class by the skill that wields it, as the
// game does.
func weaponClassLabel(category string) string {
	if skill, ok := character.WeaponSkillForCategory(strings.ToLower(category)); ok {
		return skill.String()
	}
	return titleCase(category)
}

func (v *viewer) updateFXPage() {
	v.ensureFXPage()
	if fxPage.preview == nil {
		return
	}
	list, _ := v.fxCatalogLayout()
	rows := v.fxCatalogRows()
	v.updateCatalogList("fx:list", list, rows, &fxPage.scroll, &v.browser.fxSelection, fxPage.selIdx, readCatalogListInput(), func(index int) {
		fxPage.selIdx = index
		fxPage.preview.Select(fxPage.items[index])
	})
	if !v.browser.fxPaused {
		fxPage.preview.Step()
	}
}

func (v *viewer) drawFXPage(screen *ebiten.Image) {
	w, h := v.canvasSize()
	drawFilledRect(screen, 0, pageBarHeight, w, h-pageBarHeight, overlayInput)
	game.DrawShadedText(screen, "EFFECTS LAB", 16, 49, overlayAccent)
	v.drawCatalogControls(screen)
	if fxPage.initErr != "" {
		game.DrawShadedText(screen, fxPage.initErr, 16, 150, overlayDanger)
		return
	}
	if fxPage.preview == nil || len(fxPage.items) == 0 {
		game.DrawShadedText(screen, "Loading preview...", 16, 150, overlayMuted)
		return
	}
	list, stage := v.fxCatalogLayout()
	for _, r := range []rect{list, stage} {
		drawFilledRect(screen, r.x, r.y, r.w, r.h, overlayBG)
		drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, overlayBorder)
	}
	rows := v.fxCatalogRows()
	drawCatalogList(screen, list, rows, fxPage.scroll, catalogSelectedRow(rows, v.browser.fxSelection, fxPage.selIdx))
	selected := fxPage.items[fxPage.selIdx]
	game.DrawShadedText(screen, clipText(selected.Label, stage.w-32), stage.x+16, stage.y+16, overlayAccent)
	game.DrawShadedText(screen, clipText(fxKindTag(selected.Kind)+" / "+selected.Key, stage.w-32), stage.x+16, stage.y+40, overlayMuted)
	scene := fxPage.preview.Scene()
	sw, sh := scene.Bounds().Dx(), scene.Bounds().Dy()
	scale := min(float64(stage.w-32)/float64(sw), float64(stage.h-104)/float64(sh))
	dw, dh := float64(sw)*scale, float64(sh)*scale
	graphics.DrawImageScaled(screen, scene, float64(stage.x)+(float64(stage.w)-dw)/2, float64(stage.y+72)+(float64(stage.h-104)-dh)/2, dw, dh, nil)
	status := "LIVE / automatic replay"
	if v.browser.fxPaused {
		status = "PAUSED"
	}
	game.DrawShadedText(screen, status, stage.x+16, stage.y+stage.h-22, overlayAccent)
	game.DrawShadedText(screen, fmt.Sprintf("%d effects  |  Up/Down: select  |  Wheel: scroll catalog", len(fxPage.items)), 16, h-20, overlayMuted)
}

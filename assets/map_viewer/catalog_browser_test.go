package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestCatalogFiltersPreserveEditingIdentity(t *testing.T) {
	v := overlayTestViewer(t)
	v.page = pageMaps
	v.sidebarTab = tabLegend
	v.refreshLegend()
	var chosen legendEntry
	for _, entry := range v.legendLines {
		if entry.Kind == brushMonster {
			chosen = entry
			break
		}
	}
	if chosen.MonsterKey == "" {
		t.Fatal("fixture has no monster palette entry")
	}
	v.brush = brushFromEntry(chosen)
	v.browser.legendQuery = "not-an-existing-content-key"
	v.rebuildLegend(true)
	if len(v.legendLines) != 0 || !v.brushAvailable(v.brush, v.currentBiome()) || !brushMatchesEntry(v.brush, chosen) {
		t.Fatal("display filter changed paint eligibility or selected brush")
	}
	v.browser.legendQuery = chosen.MonsterKey
	v.rebuildLegend(true)
	for _, entry := range v.legendLines {
		if entry.CollapseID != "" {
			v.toggleLegendCollapse(entry.CollapseID)
		}
	}
	if !hasCatalogBrush(v.legendLines, v.brush) {
		t.Fatal("search result hidden by a collapsed parent")
	}
	for _, size := range [][2]int{{1200, 800}, {1600, 1000}, {1920, 1080}} {
		v.Layout(size[0], size[1])
		l := v.computeLayout(v.maps[v.mapIndex])
		for i, entry := range v.legendLines {
			if brushMatchesEntry(v.brush, entry) {
				v.legendScroll = i * catalogRowHeight
				got := v.legendEntryAt(l, l.legendX+30, l.legendY+catalogRowHeight/2)
				if got == nil || !brushMatchesEntry(v.brush, *got) {
					t.Fatal("resized palette row selects a different brush")
				}
			}
		}
		for _, c := range v.catalogControls() {
			if c.r.x < l.mapAreaX+l.mapAreaW && c.r.x+c.r.w > l.mapAreaX && c.r.y < l.mapAreaY+l.mapAreaH && c.r.y+c.r.h > l.mapAreaY {
				t.Fatal("new catalog control intercepts a map gesture")
			}
		}
	}
	selected := v.mapIndex
	original := v.maps[selected].Data
	v.browser.mapQuery = v.maps[selected].Key
	v.browser.mapSort = "Size"
	indices := v.catalogMaps()
	if len(indices) != 1 || indices[0] != selected {
		t.Fatalf("filtered map identity changed: %v", indices)
	}
	v.stepCatalogMap(1)
	if v.mapIndex != selected || v.maps[selected].Data != original {
		t.Fatal("single-result navigation replaced the editing document")
	}
	v.browser.mapQuery = "no-map-matches"
	v.stepCatalogMap(-1)
	if v.mapIndex != selected {
		t.Fatal("empty search changed current map")
	}
}

func hasCatalogBrush(rows []legendEntry, b brush) bool {
	for _, row := range rows {
		if brushMatchesEntry(b, row) {
			return true
		}
	}
	return false
}

func TestMonsterAndFXCatalogSelectionSurvivesGrouping(t *testing.T) {
	oldMobs, oldFX := mobsPage, fxPage
	t.Cleanup(func() { mobsPage = oldMobs; fxPage = oldFX })
	v := &viewer{monsterCfg: &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{
		"z": {Name: "Zebra", Level: 2, Biomes: []string{"forest", "desert"}},
		"a": {Name: "Alpha", Level: 12, Boss: true},
		"b": {Name: "Beta", Level: 7},
	}}}
	mobsPage.keys = []string{"z", "a", "b"}
	mobsPage.selIdx = 1
	for _, mode := range []string{"Role", "Biome", "Type"} {
		for _, order := range []string{"Name", "Level", "Level descending"} {
			v.browser.mobGroup, v.browser.mobSort = mode, order
			rows := v.mobCatalogRows()
			for _, row := range rows {
				if row.index >= 0 {
					def := v.monsterCfg.Monsters[mobsPage.keys[row.index]]
					if !strings.Contains(row.label, def.Name) {
						t.Fatal("sorted row lost its original key")
					}
				}
			}
			if mobsPage.selIdx != 1 {
				t.Fatal("sorting changed selection")
			}
			for _, row := range rows {
				if row.index < 0 {
					v.toggleCatalogSection(row.group)
				}
			}
			v.browser.mobQuery = "Alpha"
			filtered := v.mobCatalogRows()
			i := catalogMoveSelection(filtered, -1, 1)
			if i < 0 || filtered[i].index != 1 {
				t.Fatal("search/navigation must reach the original selected identity")
			}
			v.browser.mobQuery = ""
		}
	}
	fxPage.items = []game.FxItem{{Kind: game.FxWeapon, Key: "blade", Label: "Z Blade"}, {Kind: game.FxSpell, Key: "fire", Label: "Fire"}, {Kind: game.FxWeapon, Key: "axe", Label: "Axe"}}
	v.browser.fxKind = fxKindTag(game.FxWeapon)
	rows := v.fxCatalogRows()
	i := catalogMoveSelection(rows, -1, 1)
	if i < 0 || rows[i].index != 2 {
		t.Fatal("FX sorted selection did not preserve source item index")
	}
	v.browser.fxQuery = "blade"
	rows = v.fxCatalogRows()
	i = catalogMoveSelection(rows, -1, 1)
	if i < 0 || rows[i].index != 0 {
		t.Fatal("FX key search selected the wrong effect")
	}
}

// Exercise the same list event handler used by both live preview pages. Source
// identity may repeat, but each visible occurrence must be reached exactly once.
func TestCatalogNavigationVisitsEveryOccurrence(t *testing.T) {
	oldMobs, oldFX := mobsPage, fxPage
	t.Cleanup(func() { mobsPage = oldMobs; fxPage = oldFX })
	for _, kind := range []string{"mobs", "fx"} {
		t.Run(kind, func(t *testing.T) {
			v := &viewer{monsterCfg: &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{
				"a": {Name: "Alpha", Biomes: []string{"desert", "forest"}},
				"b": {Name: "Beta", Biomes: []string{"desert", "forest"}},
			}}}
			mobsPage.keys = []string{"a", "b"}
			v.browser.mobGroup = "Biome"
			fxPage.items = []game.FxItem{{Kind: game.FxWeapon, Key: "a", Label: "Alpha"}, {Kind: game.FxSpell, Key: "b", Label: "Beta"}}
			build := v.mobCatalogRows
			if kind == "fx" {
				build = v.fxCatalogRows
			}
			rows := build()
			var leaves []int
			for i, r := range rows {
				if r.index >= 0 {
					leaves = append(leaves, i)
				}
			}
			r := rect{0, 0, 240, 1000}
			source, scroll := -1, 0
			var selected catalogEntryRow
			apply := func(in catalogListInput) {
				v.updateCatalogList("test", r, rows, &scroll, &selected, source, in, func(i int) { source = i })
			}
			for _, i := range leaves {
				apply(catalogListInput{delta: 1})
				if got := catalogSelectedRow(rows, selected, source); got != i {
					t.Fatalf("forward navigation reached row %d, want occurrence %d", got, i)
				}
			}
			for j := len(leaves) - 2; j >= 0; j-- {
				apply(catalogListInput{delta: -1})
				if got := catalogSelectedRow(rows, selected, source); got != leaves[j] {
					t.Fatalf("backward navigation: %d != %d", got, leaves[j])
				}
			}
			apply(catalogListInput{delta: 10})
			if got := catalogSelectedRow(rows, selected, source); got != leaves[len(leaves)-1] {
				t.Fatal("page navigation did not clamp at last leaf")
			}
			apply(catalogListInput{x: 10, y: leaves[0]*catalogRowHeight + 5, click: true})
			if got := catalogSelectedRow(rows, selected, source); got != leaves[0] {
				t.Fatal("click selected a different occurrence")
			}
			// Fold the selected group, then navigate into the remaining group.
			apply(catalogListInput{x: 10, y: 5, click: true})
			rows = build()
			for _, row := range rows {
				if row.index >= 0 && row.group == selected.group {
					t.Fatal("fold left selected group's leaves visible")
				}
			}
			apply(catalogListInput{delta: 1})
			if catalogSelectedRow(rows, selected, source) < 0 {
				t.Fatal("navigation failed after folding")
			}
			v.browser.mobQuery, v.browser.fxQuery = "no-match", "no-match"
			rows = build()
			before := source
			apply(catalogListInput{delta: 1})
			if len(rows) != 0 || source != before {
				t.Fatal("empty search changed selection")
			}
		})
	}
}

func TestMobInspectorSectionRoutingAndWrapping(t *testing.T) {
	old := mobsPage
	t.Cleanup(func() { mobsPage = old })
	for _, tc := range []struct {
		name      string
		def       monster.MonsterDefinition
		abilities []string
	}{
		{"without resists", monster.MonsterDefinition{}, []string{"Melee:"}},
		{"with resists", monster.MonsterDefinition{Resistances: map[string]int{"fire": 20}}, []string{"Melee:"}},
		{"champion", monster.MonsterDefinition{Champion: "fixture"}, []string{"CHAMPION"}},
		{"inert", monster.MonsterDefinition{Disposition: monster.DispositionFish}, []string{"No special abilities."}},
		{"behavior", monster.MonsterDefinition{Disposition: monster.DispositionFish, Flying: true}, []string{"BEHAVIOR", "flying"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Name, tc.def.Biomes, tc.def.Sprite = "Fixture", []string{"forest"}, "fixture.png"
			v := &viewer{monsterCfg: &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{"fixture": tc.def}}}
			mobsPage.keys, mobsPage.selIdx = []string{"fixture"}, 0
			mobsPage.info = buildMobInfoRuntime("fixture", tc.def, nil, 64)
			text := func(tab string) string {
				v.browser.mobTab = tab
				var b strings.Builder
				for _, row := range v.visibleMobDetails(600) {
					fmt.Fprintln(&b, row.text)
				}
				return b.String()
			}
			overview, abilities := text("Overview"), text("Abilities")
			for _, s := range []string{"Biomes: forest", "sprite fixture.png"} {
				if !strings.Contains(overview, s) || strings.Contains(abilities, s) {
					t.Fatalf("placement row %q routed incorrectly", s)
				}
			}
			if strings.Contains(overview, "CHAMPION") {
				t.Fatal("champion block duplicated in Overview")
			}
			for _, s := range tc.abilities {
				if !strings.Contains(abilities, s) {
					t.Fatalf("Abilities missing %q: %s", s, abilities)
				}
			}
		})
	}
	d := monster.MonsterDefinition{Telegraph: &monster.TelegraphedAttack{Name: "Slam", WarningSeconds: 2, WarningRounds: 1, CooldownSeconds: 6, CooldownRounds: 3}}
	v := &viewer{monsterCfg: &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{"fixture": d}}}
	mobsPage.info = buildMobInfoRuntime("fixture", d, nil, 64)
	v.browser.mobTab = "Abilities"
	const warning = "Warning: 2s / 1 rounds; cooldown: 6s / 3 rounds"
	wide := v.visibleMobDetails(800)
	found := false
	for _, row := range wide {
		if row.text == warning && row.depth == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("wide pane still pre-wraps warning at 40 characters or loses nesting")
	}
	for _, row := range v.visibleMobDetails(220) {
		if strings.Contains(row.text, "cooldown:") && row.depth != 2 {
			t.Fatal("wrapped warning continuation lost its indent")
		}
	}
}

func TestCatalogCachesInvalidateVisibleState(t *testing.T) {
	old := mobsPage
	t.Cleanup(func() { mobsPage = old })
	v := &viewer{maps: []mapInfo{{Key: "fixture", Config: &config.MapConfig{Name: "Fixture", Biome: "fixture"}, Data: &world.MapData{Width: 4, Height: 4}}}}
	m := v.maps[v.mapIndex]
	first := v.mapDetailRows(m, 360)
	if got := v.mapDetailRows(m, 360); &got[0] != &first[0] {
		t.Fatal("idle Map Info rebuilt")
	}
	// An Overlay save updates the existing config and refreshes the palette.
	m.Config.RespawnDays++
	v.refreshLegend()
	after := v.mapDetailRows(m, 360)
	if &after[0] == &first[0] {
		t.Fatal("published config left Map Info stale")
	}
	if got := v.mapDetailRows(m, 600); &got[0] == &after[0] {
		t.Fatal("resize did not reflow Map Info")
	}
	// A brush edit mutates map data in place without refreshing the palette.
	after = v.mapDetailRows(m, 360)
	m.Data.NPCSpawns = append(m.Data.NPCSpawns, world.NPCSpawn{})
	if got := v.mapDetailRows(m, 360); &got[0] == &after[0] {
		t.Fatal("placement count remained stale after brush edit")
	}
	v.brush = brush{kind: brushEraser}
	after = v.mapDetailRows(m, 360)
	var text strings.Builder
	for _, row := range after {
		fmt.Fprintln(&text, row.text)
	}
	if !strings.Contains(text.String(), formatBrushLabel(v.brush)) {
		t.Fatal("brush description remained stale")
	}
	mobsPage.keys, mobsPage.selIdx = []string{"fixture"}, 0
	v.monsterCfg = &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{"fixture": {Name: "Fixture"}}}
	mobsPage.info = buildMobInfoRuntime("fixture", v.monsterCfg.Monsters["fixture"], nil, 64)
	rows := v.mobCatalogRows()
	if next := v.mobCatalogRows(); &next[0] != &rows[0] {
		t.Fatal("idle Bestiary list rebuilt")
	}
	details := v.visibleMobDetails(360)
	if next := v.visibleMobDetails(360); &next[0] != &details[0] {
		t.Fatal("idle Bestiary details rebuilt")
	}
	v.browser.mobTab = "Source"
	if next := v.visibleMobDetails(360); &next[0] == &details[0] {
		t.Fatal("tab change retained stale details")
	}
}

func TestFailedMapKeepsOnlyVisibleRecoveryControls(t *testing.T) {
	v := &viewer{page: pageMaps, sidebarTab: tabLegend, maps: []mapInfo{{Key: "broken", Err: errors.New("fixture")}, {Key: "working", Data: &world.MapData{Width: 1, Height: 1}}}}
	if v.catalogMapReady() {
		t.Fatal("failed map accepts map gestures")
	}
	for _, c := range v.catalogControls() {
		if c.r.y >= pageBarHeight+60 {
			t.Fatal("failed map has hidden sidebar hit targets")
		}
		if c.label == ">" {
			c.action()
			if !v.catalogMapReady() {
				t.Fatal("recovery navigation did not reach the valid map")
			}
			return
		}
	}
	t.Fatal("failed map has no recovery navigation")
}

func TestMobAbilityInspectorIncludesTelegraphAndEncounter(t *testing.T) {
	old := mobsPage
	t.Cleanup(func() { mobsPage = old })
	d := monster.MonsterDefinition{Name: "Sentinel", Telegraph: &monster.TelegraphedAttack{Name: "Ground slam", Kind: "strike", School: "earth", Shape: "burst", Range: 3, WarningSeconds: 2, WarningRounds: 1, CooldownSeconds: 6, CooldownRounds: 3, DamagePercent: 150, DurationSeconds: 4, DurationRounds: 2, SustainDamage: 7, BelowHPPercent: 60}}
	v := &viewer{monsterCfg: &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{"sentinel": d}}, maps: []mapInfo{{Key: "test_arena", Config: &config.MapConfig{Adventure: &config.AdventureConfig{Boss: &config.AdventureBoss{Monster: "sentinel", EveryActions: 3, Lanes: []string{"north"}}}}}}}
	mobsPage.keys = []string{"sentinel"}
	mobsPage.selIdx = 0
	mobsPage.info = buildMobInfoRuntime("sentinel", d, nil, 64)
	v.browser.mobTab = "Abilities"
	var text strings.Builder
	for _, r := range v.visibleMobDetails(360) {
		text.WriteString(r.text)
		text.WriteByte(' ')
	}
	for _, want := range []string{"Ground slam", "Warning:", "2s / 1 rounds", "6s / 3 rounds", "150%", "sustained damage: 7", "60% HP", "ENCOUNTER: test_arena", "north"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("missing ability/encounter information %q in %s", want, text.String())
		}
	}
	v.browser.mobTab = "Source"
	rows := v.visibleMobDetails(360)
	for _, r := range rows {
		if r.header && strings.Contains(r.text, "telegraphed_attack") {
			v.toggleCatalogSection(r.id)
			break
		}
	}
	var folded strings.Builder
	for _, r := range v.visibleMobDetails(360) {
		fmt.Fprintln(&folded, r.text)
	}
	if strings.Contains(folded.String(), "Ground slam") || !strings.Contains(folded.String(), "name: Sentinel") {
		t.Fatal("folding a nested ability hid sibling fields or left its children visible")
	}
}

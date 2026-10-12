package main

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestTilePaletteGroupUsesPlacementBehavior(t *testing.T) {
	tests := []struct {
		name string
		data *config.TileData
		want string
	}{
		{name: "floor", data: &config.TileData{RenderType: config.TileRenderFloor}, want: legendGroupTerrain},
		{name: "wall", data: &config.TileData{RenderType: config.TileRenderWall}, want: legendGroupWalls},
		{name: "tree", data: &config.TileData{Type: "nature", RenderType: config.TileRenderCrossedStandee, SizeClass: config.SizeClassTree}, want: legendGroupCrossed},
		{name: "non-tree wide cross", data: &config.TileData{Type: "rock", RenderType: config.TileRenderCrossedStandee, SizeClass: config.SizeClassTree}, want: legendGroupCrossed},
		{name: "crossed prop", data: &config.TileData{Type: "prop", RenderType: config.TileRenderCrossedProp, SizeClass: "small_prop"}, want: legendGroupCrossedProps},
		{name: "narrow tree", data: &config.TileData{Type: "nature", RenderType: config.TileRenderCrossedStandee, SizeClass: "full_tile"}, want: legendGroupCrossed},
		{name: "landmark", data: &config.TileData{RenderType: config.TileRenderLandmarkStandee}, want: legendGroupLandmarks},
		{name: "wall decor", data: &config.TileData{RenderType: config.TileRenderStandee, WallMounted: true, Walkable: true}, want: legendGroupWallDecor},
		{name: "passable decor", data: &config.TileData{RenderType: config.TileRenderStandee, Walkable: true}, want: legendGroupPassableDecor},
		{name: "invalid blocker fallback", data: &config.TileData{RenderType: config.TileRenderStandee, Solid: true}, want: legendGroupOtherTiles},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tilePaletteGroup(tt.data); got != tt.want {
				t.Fatalf("tilePaletteGroup() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegendCollapseSupportsScopesAndRenderGroups(t *testing.T) {
	groups := []legendGroup{
		{id: legendGroupTerrain, label: "Ground and Terrain", entries: []legendEntry{{Text: "Floor", Kind: brushTile}}},
		{id: legendGroupCrossed, label: "Crossed: Trees and Props", entries: []legendEntry{{Text: "Oak", Kind: brushTile}, {Text: "Rock", Kind: brushTile}}},
	}

	containsText := func(lines []legendEntry, text string) bool {
		for _, line := range lines {
			if strings.Contains(line.Text, text) {
				return true
			}
		}
		return false
	}

	crossesCollapsed := appendLegendScope(nil, "Biome: Forest", "scope:biome:forest", groups, map[string]bool{legendGroupCrossed: true})
	if !containsText(crossesCollapsed, "[+] Crossed: Trees and Props") || containsText(crossesCollapsed, "Oak") || containsText(crossesCollapsed, "Rock") {
		t.Fatalf("collapsed crossed parent leaked children: %+v", crossesCollapsed)
	}
	if !containsText(crossesCollapsed, "Floor") {
		t.Fatal("collapsing crossed standees also hid the terrain group")
	}

	scopeCollapsed := appendLegendScope(nil, "Biome: Forest", "scope:biome:forest", groups, map[string]bool{"scope:biome:forest": true})
	if len(scopeCollapsed) != 1 || !containsText(scopeCollapsed, "[+] Biome: Forest") {
		t.Fatalf("collapsed biome = %+v, want one expandable header", scopeCollapsed)
	}
}

// Spells page must group purely BY SCHOOL (no Battle/Utility split) and order
// each school's spells by ascending SP cost.
func TestBuildSpellCards_BySchoolByCost(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	cards := buildSpellCards()
	if len(cards) == 0 {
		t.Fatal("no spell cards built")
	}
	lastCostInSection := map[string]int{}
	for _, c := range cards {
		if strings.Contains(c.section, "Battle") || strings.Contains(c.section, "Utility") {
			t.Fatalf("section %q must be a school name, not a battle/utility split", c.section)
		}
		def, ok := config.GlobalSpells.Spells[c.key]
		if !ok || def == nil {
			continue
		}
		if prev, seen := lastCostInSection[c.section]; seen && def.SpellPointsCost < prev {
			t.Errorf("section %q not cost-ascending: %s (%d SP) after %d SP", c.section, c.key, def.SpellPointsCost, prev)
		}
		lastCostInSection[c.section] = def.SpellPointsCost
	}
}

// mobSheetText is the editor stat sheet as one whitespace-normalized string.
func mobSheetText(key string, def monster.MonsterDefinition) string {
	var rows []string
	for _, line := range buildMobInfo(key, def) {
		rows = append(rows, line.text)
	}
	return strings.Join(strings.Fields(strings.Join(rows, " ")), " ")
}

// The sheet describes the stun the MONSTER applies: its own stun wins over the
// stun authored on the spell it casts, which only rides the projectile when the
// monster has none.
func TestMobInfoShowsOnlyMonsterFacingStun(t *testing.T) {
	previous := config.GlobalSpells
	t.Cleanup(func() { config.GlobalSpells = previous })
	config.GlobalSpells = &config.SpellSystemConfig{Spells: map[string]*config.SpellDefinitionConfig{
		"fixture_bolt": {Name: "Fixture Bolt", School: "air", StunChance: .2, StunDurationSeconds: 2, StunDurationTurns: 1},
	}}
	for _, tc := range []struct {
		name         string
		def          monster.MonsterDefinition
		want, absent []string
	}{
		{"own stun wins", monster.MonsterDefinition{ProjectileSpell: "fixture_bolt", Abilities: monster.Abilities{Stun: monster.TimedStatusAbility{Chance: .1, Seconds: 4, Turns: 2}}},
			[]string{"Ranged spell: Fixture Bolt (air)", "Stun: 10% (4s / 2 turns)", "Projectile stun: 10% on hit, 4s / 2 turn(s)"},
			[]string{"20%"}},
		{"spell stun rides the projectile", monster.MonsterDefinition{ProjectileSpell: "fixture_bolt"},
			[]string{"Ranged spell: Fixture Bolt (air)", "Projectile stun: 20% on hit, 2s / 1 turn(s)"},
			[]string{"Stun: "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Name = "Fixture"
			got := mobSheetText("fixture", tc.def)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %s", want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("spell stun shown as monster-facing (%q) in %s", absent, got)
				}
			}
		})
	}
}

// The ABILITIES section is exactly CombatEffectLines, in order and school tint,
// for every shipped monster.
func TestMobInfo_UsesMonsterCombatEffectLines(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	withAbilities := 0
	for key, def := range monster.MonsterConfig.Monsters {
		t.Run(key, func(t *testing.T) {
			rows := buildMobInfo(key, def)
			// Each formatter's rows open their own section, in order.
			for _, section := range []struct {
				header string
				lines  []monster.EffectLine
			}{
				{"ATTACK", def.AttackLines()},
				{"ABILITIES", def.AbilityLines()},
				{"BEHAVIOR", def.BehaviorLines()},
			} {
				header := -1
				for i, row := range rows {
					if row.header && row.text == section.header {
						header = i
					}
				}
				if len(section.lines) == 0 {
					if header >= 0 && section.header != "BEHAVIOR" {
						t.Fatalf("%s header without its lines", section.header)
					}
					continue
				}
				if section.header == "ABILITIES" {
					withAbilities++
				}
				if header < 0 || len(rows) < header+1+len(section.lines) {
					t.Fatalf("%s section missing or short: %d rows after header %d, want %d", section.header, len(rows), header, len(section.lines))
				}
				for i, line := range section.lines {
					var col color.Color = mobStatDefault
					if line.School != "" {
						col = game.SchoolColor(line.School)
					}
					if got := rows[header+1+i]; got.text != strings.TrimSpace(line.Text) || got.col != col {
						t.Fatalf("%s row %d = %q %v, want %q %v", section.header, i, got.text, got.col, line.Text, col)
					}
				}
			}
		})
	}
	if withAbilities == 0 {
		t.Fatal("no shipped monster has an ability")
	}
}

func TestMobInfo_ShowsEffectiveCadenceAndAuthoredAbilities(t *testing.T) {
	for _, tc := range []struct {
		name         string
		def          monster.MonsterDefinition
		want, absent []string
	}{
		// TB swings are fractional and accumulate; the sheet plays the first five
		// rounds from a fresh carry (expected sequences worked out by hand).
		{"default cadence", monster.MonsterDefinition{}, []string{"TB attacks/round: 1.00", "First 5 rounds: 1 1 1 1 1 = 5 (avg 1.00)"}, []string{"RT attack cooldown", "Enraged TB attacks"}},
		{"0.6 cooldown averages 1.67 swings", monster.MonsterDefinition{AttackCooldownMult: .6}, []string{"TB attacks/round: 1.67", "First 5 rounds: 2 1 2 2 1 = 8 (avg 1.60)", "RT attack cooldown: x0.60"}, nil},
		{"0.7 cooldown alternates", monster.MonsterDefinition{AttackCooldownMult: .7}, []string{"TB attacks/round: 1.43", "First 5 rounds: 1 2 1 2 1 = 7 (avg 1.40)"}, nil},
		{"slow cooldown keeps one swing", monster.MonsterDefinition{AttackCooldownMult: 1.8}, []string{"TB attacks/round: 1.00", "First 5 rounds: 1 1 1 1 1 = 5"}, nil},
		{"very fast cooldown is capped", monster.MonsterDefinition{AttackCooldownMult: .1}, []string{"TB attacks/round: 8.00"}, nil},
		{"enraged cadence and summons", monster.MonsterDefinition{
			Abilities: monster.Abilities{Summon: monster.SummonAbility{Monsters: []string{"goblin"}, Chance: .18, FirstGuaranteed: true}, Enrage: monster.EnrageAbility{AtHP: 50, CooldownMult: .7}},
		}, []string{"TB attacks/round: 1.00", "Enraged TB attacks/round: 1.43", "First 5 rounds: 1 2 1 2 1 = 7 (avg 1.40)", "first guaranteed, then 18%"}, nil},
		{"capped aggro rally", monster.MonsterDefinition{AttackCooldownMult: .5, RallyOnAggroTiles: 15, RallyMaxTargets: 4}, []string{"TB attacks/round: 2.00", "Aggro rally: up to 4 mobs within 15 tiles"}, nil},
		{"uncapped aggro rally", monster.MonsterDefinition{RallyOnAggroTiles: 15}, []string{"Aggro rally: every mob within 15 tiles"}, []string{"up to"}},
		// Movement uses the same rule: speed / turn_step_speed (2.0), one tile minimum.
		{"step speed walks one tile", monster.MonsterDefinition{Speed: 2}, []string{"TB moves/round: 1.00"}, nil},
		{"fast walker banks an extra tile", monster.MonsterDefinition{Speed: 2.4}, []string{"TB moves/round: 1.20", "First 5 rounds: 1 1 2 1 1 = 6 (avg 1.20)"}, nil},
		{"stationary", monster.MonsterDefinition{Speed: 0}, []string{"TB moves/round: 0 (stationary)"}, []string{"TB moves/round: 1"}},
		{"ambient stroller may idle", monster.MonsterDefinition{Speed: 0.7, Disposition: monster.DispositionCaravan}, []string{"TB moves/round: 0.35", "First 5 rounds: 0 1 0 0 1 = 2 (avg 0.40)"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Name = "Fixture"
			got := mobSheetText("fixture", tc.def)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %s", want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("unexpected %q in %s", absent, got)
				}
			}
		})
	}
}

func TestBuildMapInfoLines_ShowsMapRuntimeSettings(t *testing.T) {
	m := mapInfo{
		Key: "test_map",
		Config: &config.MapConfig{
			Name:              "Test Map",
			File:              "assets/test.map",
			Biome:             "forest",
			DefaultFloorColor: [3]int{12, 34, 56},
			CanopyShade: &config.MapCanopyShadeConfig{
				MinAmbient:   0.7,
				RadiusTiles:  4,
				StartDensity: 2,
				FullDensity:  6,
			},
		},
		Data: &world.MapData{
			Width:  30,
			Height: 20,
			StartX: 4,
			StartY: 7,
		},
	}
	m.LightingText = game.MapLightingText(m.Config)
	lines := buildMapInfoLines(m, brush{kind: brushEraser})
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, line.text)
	}
	got := strings.Join(rows, "\n")
	for _, want := range []string{
		"Test Map  (test_map)",
		"Biome: forest",
		"Size: 30x20   Start: 4,7",
		"Ambient light: 1.00",
		"Floor RGB: 12, 34, 56",
		"Canopy: 0.70 light, 4.0t radius",
		"Brush: Eraser",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("map info missing %q. rows:\n%s", want, got)
		}
	}
}

func TestHeaderBandsClearPreviousOutlinedText(t *testing.T) {
	const (
		textY                    = 100
		outlinedTextBottomOffset = 15
	)
	for _, rowAdvance := range []int{14, 16, 18} {
		bandY, bandH := headerBandForTextRowBounds(textY, rowAdvance)
		previousTextBottom := textY - rowAdvance + outlinedTextBottomOffset
		if bandY <= previousTextBottom {
			t.Errorf("row %d: band starts at %d over previous text ending at %d",
				rowAdvance, bandY, previousTextBottom)
		}
		if bandY+bandH > textY+rowAdvance {
			t.Errorf("row %d: band ends past the next row baseline", rowAdvance)
		}
	}
}

// TestContentCardSectionsAreContiguous: the page draws a header whenever the
// section changes, so every section must appear as ONE run. A skill appended
// late in the save-pinned SkillType enum (Blaster is a weapon skill sitting
// after the Misc block) used to print "Weapon Skills" twice.
func TestContentCardSectionsAreContiguous(t *testing.T) {
	// The card builders read the GLOBAL item/spell configs; under -shuffle no
	// earlier test is guaranteed to have loaded them.
	if _, err := config.LoadItemConfig(filepath.Join("..", "..", "assets", "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	pages := map[string][]contentCard{
		"items":  groupCardsBySection(buildItemsCards()),
		"spells": groupCardsBySection(buildSpellCards()),
		"skills": groupCardsBySection(buildSkillCards()),
	}
	for page, cards := range pages {
		if len(cards) == 0 {
			t.Errorf("page %s built no cards", page)
			continue
		}
		seen := map[string]bool{}
		prev := ""
		for _, card := range cards {
			if card.section == prev {
				continue
			}
			if seen[card.section] {
				t.Errorf("page %s: section %q starts a second run - its header would repeat", page, card.section)
			}
			seen[card.section] = true
			prev = card.section
		}
	}
}

// TestBlasterSkillCardStatesTrainingIsOptional keeps the editor's skill text
// honest about the equip rule it shares with the game.
func TestBlasterSkillCardStatesTrainingIsOptional(t *testing.T) {
	for _, card := range buildSkillCards() {
		if card.name != "Blaster" {
			continue
		}
		if !strings.Contains(strings.ToLower(card.description), "untrained") {
			t.Errorf("Blaster card must say it needs no training: %q", card.description)
		}
		return
	}
	t.Fatal("no Blaster skill card")
}

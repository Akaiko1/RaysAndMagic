package main

import (
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

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

func TestMobInfo_GoldDragonShowsOnlyMonsterFacingStun(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))

	rowsFor := func(key string) string {
		def, ok := monster.MonsterConfig.Monsters[key]
		if !ok {
			t.Fatalf("monster %q missing", key)
		}
		lines := buildMobInfo(key, def)
		rows := make([]string, 0, len(lines))
		for _, line := range lines {
			rows = append(rows, line.text)
		}
		return strings.Join(rows, "\n")
	}

	for key, wantStun := range map[string]string{
		"dragon_gold":       "Stun: 10% (4s / 2 turns)",
		"elder_dragon_gold": "Stun: 10% (4s / 2 turns)",
	} {
		got := rowsFor(key)
		for _, want := range []string{"Ranged spell: Lightning Bolt (air)", wantStun} {
			if !strings.Contains(got, want) {
				t.Errorf("editor mob %s missing %q. rows:\n%s", key, want, got)
			}
		}
		if strings.Contains(got, "Stun on hit: 20% (2s / 1 turns)") {
			t.Errorf("editor mob %s shows spell stun as monster-facing stun. rows:\n%s", key, got)
		}
	}
}

// TestSpellCard_SharesMechanicsWithGame verifies the editor's spell card pulls
// its mechanics from spells.EffectLines (the same source as the in-game
// tooltip), so previously-missing fields (stun chance, buff bonuses, charm,
// zone, revive...) now appear and can't drift from the game.
func TestSpellCard_SharesMechanicsWithGame(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	rowsFor := func(key string) string {
		for _, c := range buildSpellCards() {
			if c.key == key {
				return strings.Join(c.tooltipRows, "\n")
			}
		}
		t.Fatalf("no card for %q", key)
		return ""
	}
	want := map[string]string{
		"psychic_shock": "Stun chance: 10%",
		"stone_skin":    "Party takes -4 to -10 damage per hit by mastery",
		"heroism":       "Party physical attacks deal +3 to +10 damage by mastery",
		"charm":         "Pacifies",
		"stun":          "Stuns every monster within 3.0 tiles",
		"raise_dead":    "Revives a fallen ally to 25% HP",
	}
	for key, sub := range want {
		if got := rowsFor(key); !strings.Contains(got, sub) {
			t.Errorf("editor %s card missing %q. rows:\n%s", key, sub, got)
		}
	}
	// Charm/Disintegrate are deals_no_damage -> no damage row in the editor either.
	for _, key := range []string{"charm", "disintegrate"} {
		if got := rowsFor(key); strings.Contains(got, "Base damage") {
			t.Errorf("editor %s card shows damage but it's deals_no_damage:\n%s", key, got)
		}
	}
}

func TestMobInfo_UsesMonsterCombatEffectLines(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))

	rowsFor := func(key string) string {
		def, ok := monster.MonsterConfig.Monsters[key]
		if !ok {
			t.Fatalf("monster %q missing", key)
		}
		lines := buildMobInfo(key, def)
		rows := make([]string, 0, len(lines))
		for _, line := range lines {
			rows = append(rows, line.text)
		}
		return strings.Join(rows, "\n")
	}

	want := map[string][]string{
		"archmage": {"Ranged spell: Fireball (fire)", "Projectile AoE: whole party on hit"},
		"dragon":   {"Ranged spell: Fire Bolt (fire)", "Dragon Breath: 33% fire attack to whole party"},
	}
	for key, subs := range want {
		got := strings.Join(strings.Fields(rowsFor(key)), " ")
		for _, sub := range subs {
			if !strings.Contains(got, sub) {
				t.Errorf("editor mob %s missing %q. rows:\n%s", key, sub, got)
			}
		}
	}
}

func TestMobInfo_ShowsEffectiveCadenceAndAuthoredAbilities(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))

	rowsFor := func(key string) string {
		def, ok := monster.MonsterConfig.Monsters[key]
		if !ok {
			t.Fatalf("monster %q missing", key)
		}
		lines := buildMobInfo(key, def)
		rows := make([]string, 0, len(lines))
		for _, line := range lines {
			rows = append(rows, line.text)
		}
		return strings.Join(strings.Fields(strings.Join(rows, "\n")), " ")
	}

	for key, wants := range map[string][]string{
		"dragon_green": {
			"TB attacks: 2",
			"RT attack cooldown: x0.60",
		},
		"old_samurai": {
			"TB attacks: 1",
			"Enraged TB attacks: 2",
			"first guaranteed, then 18%",
		},
		"alarm_clock": {
			"TB attacks: 2",
			"Aggro rally: up to 4 mobs within 15 tiles",
		},
	} {
		got := rowsFor(key)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("editor mob %s missing %q. rows:\n%s", key, want, got)
			}
		}
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

func TestCatalogTooltipsFitDefaultWindow(t *testing.T) {
	assets := filepath.Join("..", "..", "assets")
	if _, err := config.LoadSpellConfig(filepath.Join(assets, "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig(filepath.Join(assets, "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig(filepath.Join(assets, "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadTrapConfig(filepath.Join(assets, "traps.yaml")); err != nil {
		t.Fatalf("load traps: %v", err)
	}

	var cards []contentCard
	cards = append(cards, buildItemsCards()...)
	cards = append(cards, buildSpellCards()...)
	cards = append(cards, buildSkillCards()...)
	if len(cards) == 0 {
		t.Fatal("catalog is empty")
	}
	for i := range cards {
		w, h := cardTooltipSize(&cards[i])
		if w > windowWidth-8 {
			t.Errorf("%s tooltip is %dpx wide, exceeds %dpx window", cards[i].key, w, windowWidth)
		}
		if h > windowHeight-pageBarHeight-8 {
			t.Errorf("%s tooltip is %dpx tall, exceeds available %dpx", cards[i].key, h, windowHeight-pageBarHeight-8)
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

func TestMobSheetsFitDefaultColumns(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))

	const maxVisibleRows = 69 // three 23-row columns at the default 1200x800 layout
	for key, def := range monster.MonsterConfig.Monsters {
		if rows := len(buildMobInfo(key, def)); rows > maxVisibleRows {
			t.Errorf("%s stat sheet has %d rows, exceeds %d-row default layout", key, rows, maxVisibleRows)
		}
	}
}

// TestContentCardSectionsAreContiguous: the page draws a header whenever the
// section changes, so every section must appear as ONE run. A skill appended
// late in the save-pinned SkillType enum (Blaster is a weapon skill sitting
// after the Misc block) used to print "Weapon Skills" twice.
func TestContentCardSectionsAreContiguous(t *testing.T) {
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

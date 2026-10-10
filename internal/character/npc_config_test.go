package character

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"ugataima/internal/config"
)

func TestLoadNPCConfigRejectsRemovedSizeTiles(t *testing.T) {
	previous := NPCConfigInstance
	t.Cleanup(func() { NPCConfigInstance = previous })
	path := filepath.Join(t.TempDir(), "npcs.yaml")
	data := []byte("npcs:\n  legacy:\n    name: Legacy\n    type: quest_giver\n    size_tiles: 0.75\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	err := LoadNPCConfig(path)
	if err == nil || !strings.Contains(err.Error(), "removed size_tiles") {
		t.Fatalf("LoadNPCConfig error = %v, want removed size_tiles rejection", err)
	}
}

func TestCreateNPCFromConfig_MerchantStock(t *testing.T) {
	if _, err := config.LoadItemConfig(filepath.Join("..", "..", "assets", "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadWeaponConfig(filepath.Join("..", "..", "assets", "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	npc, err := CreateNPCFromConfig("merchant_general", 0, 0)
	if err != nil {
		t.Fatalf("create npc: %v", err)
	}
	if len(npc.MerchantStock) == 0 {
		t.Fatalf("expected merchant stock to be populated")
	}

	// The shop row carries the authored row's price and quantity.
	var authored *NPCItem
	for _, row := range NPCConfigInstance.NPCs["merchant_general"].Inventory {
		if row != nil && row.Name == "Health Potion" {
			authored = row
		}
	}
	if authored == nil || authored.Cost <= 0 || authored.Quantity == 0 {
		t.Fatalf("merchant_general must author a priced, counted Health Potion row, got %+v", authored)
	}
	foundPotion := false
	for _, entry := range npc.MerchantStock {
		if entry.Item.Name == authored.Name {
			foundPotion = true
			if entry.Cost != authored.Cost || entry.Quantity != authored.Quantity {
				t.Fatalf("Health Potion stock = cost %d qty %d, want the authored %d/%d", entry.Cost, entry.Quantity, authored.Cost, authored.Quantity)
			}
		}
	}
	if !foundPotion {
		t.Fatalf("expected Health Potion in merchant stock")
	}
}

func TestCreateNPCFromConfig_MerchantSellAvailable(t *testing.T) {
	if _, err := config.LoadItemConfig(filepath.Join("..", "..", "assets", "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadWeaponConfig(filepath.Join("..", "..", "assets", "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	npc, err := CreateNPCFromConfig("desert_merchant", 0, 0)
	if err != nil {
		t.Fatalf("create npc: %v", err)
	}
	if !npc.SellAvailable {
		t.Fatalf("expected desert_merchant to allow selling")
	}
	if len(npc.MerchantStock) != 0 {
		t.Fatalf("expected desert_merchant to have no stock")
	}
}

func TestMerchantStockItemEffectiveCurrency(t *testing.T) {
	plain := &MerchantStockItem{}
	if got := plain.EffectiveCurrency(CurrencyArenaPoints); got != CurrencyArenaPoints {
		t.Fatalf("shop currency = %q, want %q", got, CurrencyArenaPoints)
	}

	override := &MerchantStockItem{CurrencyItem: "black_dragon_scale"}
	want := CurrencyItemPrefix + "black_dragon_scale"
	if got := override.EffectiveCurrency(""); got != want {
		t.Fatalf("entry currency = %q, want %q", got, want)
	}
}

// Trader catalogs author spell IDs and costs; backfillTraderSpells must fill
// name, school and description from spells.yaml without inventing a purchase
// level or mastery gate.
func TestBackfillTraderSpells(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	get := func(npcKey, spellID string) *NPCSpell {
		t.Helper()
		npc, ok := NPCConfigInstance.NPCs[npcKey]
		if !ok || npc.Spells == nil {
			t.Fatalf("%s has no spells", npcKey)
		}
		sp := npc.Spells[spellID]
		if sp == nil {
			t.Fatalf("%s should sell %s", npcKey, spellID)
		}
		return sp
	}

	// Lake trader: a Body spell entry given as just an ID is fully backfilled.
	heal := get("spell_trader_mage", "heal")
	if heal.Name == "" || heal.Cost <= 0 {
		t.Errorf("heal not backfilled: %+v", heal)
	}

	// Catalog entries carry price + identity only - there is no purchase gate to
	// backfill, so a bare ID must still resolve its school and cost.
	wb := get("city_spell_shop", "water_breathing")
	if wb.Name == "" || wb.Cost <= 0 {
		t.Errorf("water_breathing not backfilled: %+v", wb)
	}

	// Mira CASTS her water charms for gold instead of teaching them, so she
	// carries no shop stock at all - the service lives in her dialogue.
	if got := len(NPCConfigInstance.NPCs["mtrader0"].Spells); got != 0 {
		t.Errorf("Mira should sell no spells (she casts them), got %d", got)
	}
	casts := map[string]int{}
	for _, c := range NPCConfigInstance.NPCs["mtrader0"].Dialogue.Choices {
		if c != nil && c.Action == "cast_buff" {
			casts[c.Buff] = c.DurationSeconds
		}
	}
	for _, buff := range []string{"walk_on_water", "water_breathing"} {
		if casts[buff] <= 0 {
			t.Errorf("Mira's paid casts = %v, want a timed %s", casts, buff)
		}
	}

	// City sells elemental only - no Light/Dark. The row carries no school of its
	// own any more (it would only drift), so the check reads the definition the
	// row key names - the same place the shop label and the counter read.
	for id := range NPCConfigInstance.NPCs["city_spell_shop"].Spells {
		def, ok := config.GetSpellDefinition(id)
		if !ok || def == nil {
			t.Fatalf("city shop sells %q, which spells.yaml does not define", id)
		}
		schools := def.Schools
		if len(schools) == 0 {
			schools = []string{def.School}
		}
		for _, school := range schools {
			if school == "light" || school == "dark" {
				t.Errorf("city shop must not sell light/dark, found %q (%s)", id, school)
			}
		}
	}
}

// Spell rows only become a shop on a spell_trader - CreateNPCFromConfig copies
// SpellData for that type alone. Authored on any other type the rows are
// silently dropped: no shop, no complaint, and the catalog reads like it sells
// something. That is a load-time error, and every row is validated wherever it
// is written rather than only on the rows that happen to be typed right.
func TestSpellRowsBelongToSpellTraders(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	previous := NPCConfigInstance
	t.Cleanup(func() { NPCConfigInstance = previous })

	for _, tc := range []struct {
		name  string
		npc   *NPCData
		wants string
	}{
		{
			name:  "rows on a non-trader",
			npc:   &NPCData{Type: "quest_giver", Spells: map[string]*NPCSpell{"heal": {Cost: 120}}},
			wants: "only become a shop on a spell_trader",
		},
		{
			name:  "no cost",
			npc:   &NPCData{Type: "spell_trader", Spells: map[string]*NPCSpell{"heal": {}}},
			wants: "positive cost",
		},
		{
			name:  "unknown spell",
			npc:   &NPCData{Type: "spell_trader", Spells: map[string]*NPCSpell{"no_such_spell": {Cost: 10}}},
			wants: "not defined in spells.yaml",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NPCConfigInstance = &NPCConfig{NPCs: map[string]*NPCData{"hedge_witch": tc.npc}}
			err := backfillTraderSpells()
			if err == nil || !strings.Contains(err.Error(), tc.wants) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.wants)
			}
		})
	}

	// A properly typed row is filled and kept.
	NPCConfigInstance = &NPCConfig{NPCs: map[string]*NPCData{
		"hedge_witch": {Type: "spell_trader", Spells: map[string]*NPCSpell{"heal": {Cost: 120}}},
	}}
	if err := backfillTraderSpells(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	// Backfill fills identity only; the authored price is never replaced.
	if sp := NPCConfigInstance.NPCs["hedge_witch"].Spells["heal"]; sp.Name == "" || sp.Cost != 120 {
		t.Fatalf("row not backfilled with its authored cost 120: %+v", sp)
	}

	// And the type that gets the rows is the one the runtime reads them from -
	// this is the pairing that made the old check useless.
	npc, err := CreateNPCFromConfig("hedge_witch", 0, 0)
	if err != nil {
		t.Fatalf("build NPC: %v", err)
	}
	if len(npc.SpellData) != 1 {
		t.Fatalf("a spell_trader's rows did not reach SpellData: %v", npc.SpellData)
	}
}

func TestValidatePricedChoicesWalksNestedDialogue(t *testing.T) {
	previous := NPCConfigInstance
	t.Cleanup(func() { NPCConfigInstance = previous })
	NPCConfigInstance = &NPCConfig{NPCs: map[string]*NPCData{
		"nested_service": {
			Dialogue: &NPCDialogue{Choices: []*NPCDialogueChoice{
				{
					Text:   "Ask about magic",
					Action: "info",
					Choices: []*NPCDialogueChoice{
						{
							Text:            "Cast it",
							Action:          "cast_buff",
							Buff:            "walk_on_water",
							DurationSeconds: 300,
							Cost:            -100,
						},
					},
				},
			}},
		},
	}}

	if err := validatePricedChoices(); err == nil {
		t.Fatal("nested cast_buff with negative cost passed priced-choice validation")
	}
}

func TestPaidServiceResponseValidation(t *testing.T) {
	previous := NPCConfigInstance
	t.Cleanup(func() { NPCConfigInstance = previous })
	for _, action := range []string{"tavern_rest", "wait_until_night", "wait_until_dawn"} {
		for _, tc := range []struct {
			response string
			valid    bool
		}{
			{"", false}, {"  ", false}, {"You wait by the fire", true},
			{"100% sheltered (-{cost} gold).", true}, {"Cost: {price}", false},
			{"Cost: {cost", false},
		} {
			t.Run(action+"/"+tc.response, func(t *testing.T) {
				NPCConfigInstance = &NPCConfig{NPCs: map[string]*NPCData{
					"camp": {Dialogue: &NPCDialogue{Choices: []*NPCDialogueChoice{
						{Action: "info", Choices: []*NPCDialogueChoice{
							{Action: action, Cost: 10, Response: tc.response},
						}},
					}}},
				}}
				err := validatePricedChoices()
				if (err == nil) != tc.valid {
					t.Fatalf("response %q: validation error = %v", tc.response, err)
				}
			})
		}
	}
}

func TestNPCDialogueHasActionWalksNestedChoices(t *testing.T) {
	dialogue := &NPCDialogue{Choices: []*NPCDialogueChoice{
		{
			Text:   "Ask about services",
			Action: "info",
			Choices: []*NPCDialogueChoice{
				{Text: "Rest", Action: "tavern_rest"},
			},
		},
	}}

	if !dialogue.HasAction("tavern_rest") {
		t.Fatal("nested tavern_rest action was not found")
	}
	if dialogue.HasAction("start_arena_duel") {
		t.Fatal("missing action was reported as present")
	}
}

func TestCreateNPCFromConfig_EncounterMessages(t *testing.T) {
	// NPC validation checks spell traders against the LOADED spell config; a
	// shuffled-in test may have left a reduced one behind, so load the real set.
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	npc, err := CreateNPCFromConfig("shipwreck_bandit_camp", 0, 0)
	if err != nil {
		t.Fatalf("create npc: %v", err)
	}
	if npc.DialogueData == nil || npc.DialogueData.VisitedMessage == "" {
		t.Fatalf("expected visited_message to be set for shipwreck encounter")
	}
	if npc.EncounterData == nil || npc.EncounterData.StartMessage == "" {
		t.Fatalf("expected start_message to be set for shipwreck encounter")
	}
}

func TestNPCPropModelRequiresAuthoredColors(t *testing.T) {
	for _, tc := range []struct {
		name, body, trim string
		valid            bool
	}{
		{"both", "[12,23,34]", "[45,56,67]", true},
		{"black", "[0,0,0]", "[0,0,0]", true},
		{"missing body", "", "[45,56,67]", false},
		{"missing trim", "[12,23,34]", "", false},
		{"null", "null", "[45,56,67]", false},
		{"short", "[12,23]", "[45,56,67]", false},
		{"long", "[12,23,34,45]", "[45,56,67]", false},
		{"non-numeric", "[12,23,blue]", "[45,56,67]", false},
		{"outside range", "[12,23,256]", "[45,56,67]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "npcs:\n  fixture:\n    type: loot_crate\n    render_category: scenery\n    prop_model:\n      shape: crates\n      use_seconds: 1\n"
			if tc.body != "" {
				source += "      body: " + tc.body + "\n"
			}
			if tc.trim != "" {
				source += "      trim: " + tc.trim + "\n"
			}
			var cfg NPCConfig
			err := yaml.Unmarshal([]byte(source), &cfg)
			if err == nil {
				err = validateNPCPropModel("fixture", cfg.NPCs["fixture"])
			}
			if (err == nil) != tc.valid {
				t.Fatalf("authored colors accepted=%v: %v", err == nil, err)
			}
			if err != nil && !strings.Contains(err.Error(), `NPC "fixture"`) {
				t.Fatalf("color error does not identify its NPC: %v", err)
			}
		})
	}
}

package character

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	foundPotion := false
	for _, entry := range npc.MerchantStock {
		if entry.Item.Name == "Health Potion" {
			foundPotion = true
			if entry.Cost != 50 {
				t.Fatalf("expected Health Potion cost 50, got %d", entry.Cost)
			}
			if entry.Quantity != 10 {
				t.Fatalf("expected Health Potion quantity 10, got %d", entry.Quantity)
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

	// An explicit cost override is preserved (not replaced by a tier default).
	wow := get("city_spell_shop", "walk_on_water")
	if wow.Cost != 500 {
		t.Errorf("city walk_on_water cost should be its authored 500, got %d", wow.Cost)
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
	if casts["walk_on_water"] != 300 || casts["water_breathing"] != 600 {
		t.Errorf("Mira's paid casts = %v, want walk_on_water 300s and water_breathing 600s", casts)
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
	if sp := NPCConfigInstance.NPCs["hedge_witch"].Spells["heal"]; sp.Name == "" {
		t.Fatalf("row not backfilled: %+v", sp)
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

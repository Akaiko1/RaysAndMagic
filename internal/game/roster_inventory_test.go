package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

func rosterTestRequest(t *testing.T, h *displayedModalHarness, embedded bool) {
	t.Helper()
	if embedded {
		h.g.dialogActive, h.g.dialogNPC, h.g.dialogTab = true, tavernTestNPC(), 0
	} else {
		h.g.rosterScreenOpen = true
	}
	h.g.rosterSelectedActive = 0
	h.ui.Draw(h.screen)
	for _, cmd := range h.ui.displayedInput.commands {
		r := cmd.bounds
		if cmd.kind == uiCommandClick && r.h == 30 && r.x >= 400 {
			h.clicks(false, r.x+3, r.y+3, 1)
			return
		}
	}
	t.Fatal("no reserve row displayed")
}

func rosterTestAnswer(t *testing.T, h *displayedModalHarness, confirm bool) {
	t.Helper()
	h.ui.Draw(h.screen)
	var buttons []layoutRect
	for _, cmd := range h.ui.displayedInput.commands {
		if cmd.kind == uiCommandClick && cmd.bounds.h == 32 && cmd.bounds.w == 186 {
			buttons = append(buttons, cmd.bounds)
		}
	}
	if len(buttons) != 2 {
		t.Fatalf("warning has %d confirmation buttons, want 2", len(buttons))
	}
	i := 0
	if !confirm {
		i = 1
	}
	r := buttons[i]
	h.clicks(false, r.x+4, r.y+4, 1)
}

func TestRosterImportantItemsDisplayedConfirmation(t *testing.T) {
	for _, quick := range []bool{false, true} {
		for _, embedded := range []bool{false, true} {
			for _, kind := range []string{"quest", "map", "phylactery", "key", "master key", "shop currency", "stock currency", "statuette", "ordinary", "empty", "shared", "restored"} {
				for _, confirm := range []bool{false, true} {
					t.Run(fmt.Sprintf("embedded=%v/%s/confirm=%v/quick=%v", embedded, kind, confirm, quick), func(t *testing.T) {
						h := newDisplayedModalHarness(t, 800, 680)
						g := h.g
						old := character.NPCConfigInstance
						t.Cleanup(func() { character.NPCConfigInstance = old })
						character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
							"shop":   {Currency: "item:wolf_pelt"},
							"mixed":  {Inventory: []*character.NPCItem{{CurrencyItem: "spider_silk"}}},
							"statue": {Summons: []*character.NPCSummon{{Statuette: "Offering"}}},
						}}
						hero := g.party.Members[0]
						hero.Inventory = nil
						incoming := character.CreateCharacter("Reserve", character.ClassKnight, g.config)
						g.party.Reserve = []*character.MMCharacter{incoming}
						it := items.Item{Name: "Important", Type: items.ItemQuest}
						switch kind {
						case "map":
							it = items.CreateItemFromYAML("world_map")
						case "phylactery":
							it = items.CreateItemFromYAML("lich_phylactery")
						case "key":
							it.Type, it.Attributes = items.ItemTrinket, map[string]int{"door_key": 1}
						case "master key":
							it.Type, it.Attributes = items.ItemTrinket, map[string]int{"master_key": 1}
						case "shop currency":
							it = items.CreateItemFromYAML("wolf_pelt")
						case "stock currency":
							it = items.CreateItemFromYAML("spider_silk")
						case "statuette":
							it.Name, it.Type = "Offering", items.ItemTrinket
						case "ordinary":
							it = items.CreateItemFromYAML("health_potion")
						}
						if kind == "shared" {
							g.party.Inventory = []items.Item{it}
						} else if kind != "empty" {
							hero.Inventory = []items.Item{it}
						}
						if quick && len(hero.Inventory) > 0 {
							hero.Inventory = nil
							carryInQuickSlot(t, g, 0, 4, it)
						}
						if kind == "restored" {
							hero = restoreCharacterSave(buildCharacterSave(hero))
							g.party.Members[0] = hero
						}
						rosterTestRequest(t, h, embedded)
						warn := kind != "ordinary" && kind != "empty" && kind != "shared"
						if warn {
							if g.party.Members[0] != hero {
								t.Fatal("important personal items were benched without a warning")
							}
							if g.pendingRosterSwap == nil {
								t.Fatal("roster click did not open the important-items warning")
							}
							rosterTestAnswer(t, h, confirm)
						}
						want := hero
						if !warn || confirm {
							want = incoming
						}
						if g.party.Members[0] != want || g.pendingRosterSwap != nil {
							t.Fatal("confirmation/cancellation did not control the roster swap")
						}
						var owned []items.Item
						for item := range hero.PersonalItems() {
							owned = append(owned, item)
						}
						if kind != "empty" && kind != "shared" && (len(owned) != 1 || owned[0].Name != it.Name) {
							t.Fatal("roster swap lost the hero's personal goods")
						}
					})
				}
			}
		}
	}

}

func TestRosterWarningInvalidation(t *testing.T) {
	for _, change := range []string{"close roster", "close dialog", "tab", "reorder", "new item", "new quick item", "removed item", "load"} {
		t.Run(change, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 680)
			g := h.g
			hero := g.party.Members[0]
			hero.Inventory = []items.Item{{Name: "Quest token", Type: items.ItemQuest}}
			incoming := character.CreateCharacter("Reserve", character.ClassKnight, g.config)
			g.party.Reserve = []*character.MMCharacter{incoming}
			rosterTestRequest(t, h, false)
			if g.pendingRosterSwap == nil {
				t.Fatal("warning missing")
			}
			switch change {
			case "close roster":
				g.closeRosterScreen()
			case "close dialog":
				g.closeConversation()
			case "tab":
				g.switchDialogTab(1)
			case "reorder":
				g.party.Reserve[0] = character.CreateCharacter("Another", character.ClassKnight, g.config)
				rosterTestAnswer(t, h, true)
			case "new item", "new quick item":
				newKey := items.Item{Name: "New key", Attributes: map[string]int{"master_key": 1}}
				if change == "new quick item" {
					carryInQuickSlot(t, g, 0, 4, newKey)
				} else {
					hero.Inventory = append(hero.Inventory, newKey)
				}
				rosterTestAnswer(t, h, true)
				if g.pendingRosterSwap == nil || len(g.pendingRosterSwap.items) != 2 || g.party.Members[0] != hero {
					t.Fatal("new important goods were not presented before confirmation")
				}
				rosterTestAnswer(t, h, false)
			case "removed item":
				hero.Inventory = nil
				rosterTestAnswer(t, h, true)
			case "load":
				wm := world.GlobalWorldManager
				save := g.buildSave(wm)
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
				hero = g.party.Members[0]
			}
			if g.pendingRosterSwap != nil {
				t.Fatal("stale confirmation survived navigation or resolution")
			}
			if change != "removed item" && g.party.Members[0] != hero {
				t.Fatal("stale warning benched a hero")
			}
		})
	}
}

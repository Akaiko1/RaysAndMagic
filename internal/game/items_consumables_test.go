package game

import (
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Regression: a heal potion must not be usable on an Eradicated member -
// applyHealTo only checked Unconscious/Dead, unlike its siblings
// RevivablePartyIndices/HealablePartyIndices which both exclude Eradicated.
// Only the Resurrect spell may clear that condition.
func TestApplyHealTo_RefusesEradicatedMember(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.HitPoints = 0
	member.Conditions = []character.Condition{character.ConditionEradicated}
	g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML("health_potion"))
	itemIdx := len(g.party.Inventory) - 1

	if g.applyHealTo(itemIdx, 0) {
		t.Fatal("a heal potion must not revive an Eradicated member")
	}
	if member.HitPoints != 0 {
		t.Errorf("Eradicated member HP = %d, want 0 (unhealed)", member.HitPoints)
	}
	if !member.HasCondition(character.ConditionEradicated) {
		t.Error("Eradicated condition should still be set")
	}
}

// A manually parked Eradicated member can be selected for inventory inspection.
// With no eligible ally, a health potion must remain unspent rather than trying
// to self-heal or opening an empty picker.
func TestUseConsumable_ParkedEradicatedSelectionCannotSelfHeal(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.HitPoints = 0
	member.Conditions = []character.Condition{character.ConditionEradicated}
	if !g.selectPartyMemberManually(0) {
		t.Fatal("manual park on the eradicated member failed")
	}
	g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML("health_potion"))
	itemIdx := len(g.party.Inventory) - 1
	unitsBefore := g.party.GetTotalItems()

	if g.UseConsumableFromInventory(itemIdx, g.selectedChar) {
		t.Fatal("heal potion must not apply with an Eradicated member selected")
	}
	if member.HitPoints != 0 || !member.HasCondition(character.ConditionEradicated) {
		t.Fatalf("Eradicated member changed: HP=%d conditions=%v", member.HitPoints, member.Conditions)
	}
	if got := g.party.GetTotalItems(); got != unitsBefore {
		t.Fatalf("potion units = %d, want %d (refused use must not consume)", got, unitsBefore)
	}
}

func TestHealthPotion_IncapacitatedOwnerHealsEligibleAlly(t *testing.T) {
	conditions := []struct {
		name      string
		condition character.Condition
	}{
		{name: "unconscious", condition: character.ConditionUnconscious},
		{name: "dead", condition: character.ConditionDead},
		{name: "eradicated", condition: character.ConditionEradicated},
	}

	for _, tc := range conditions {
		t.Run(tc.name, func(t *testing.T) {
			g := selectionTestGame(t)
			owner := g.party.Members[0]
			owner.HitPoints = 0
			owner.AddCondition(tc.condition)
			ally := g.party.Members[1]
			ally.HitPoints = 1
			g.party.Inventory = []items.Item{items.CreateItemFromYAML("health_potion")}

			if !g.UseConsumableFromInventory(0, 0) {
				t.Fatal("health potion was not redirected to the only eligible ally")
			}
			if ally.HitPoints <= 1 {
				t.Fatalf("eligible ally was not healed: HP=%d", ally.HitPoints)
			}
			if len(g.party.Inventory) != 0 {
				t.Fatal("redirected health potion was not consumed")
			}
			if !owner.HasCondition(tc.condition) {
				t.Fatalf("owner lost %s while redirecting a health potion", tc.name)
			}
		})
	}
}

func TestTimedDraughtUsesItemDefinitionAsSourceOfTruth(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.party.Inventory = []items.Item{items.CreateItemFromYAML("flame_ward_draught")}

	if !g.UseConsumableFromInventory(0, 0) {
		t.Fatal("flame ward draught was not consumed")
	}
	buff, ok := g.combatBuffByID("flame_ward_draught")
	if !ok {
		t.Fatal("draught did not use its YAML item key as buff identity")
	}
	if buff.ResistSchool != "fire" || buff.ResistSchoolPct != 50 || buff.Frames != 60*g.config.GetTPS() {
		t.Fatalf("draught buff = %+v", buff)
	}

	saves := buildCombatBuffSaves(g.combatBuffs)
	if len(saves) != 1 {
		t.Fatalf("saved buffs = %d, want 1", len(saves))
	}
	if saves[0].ResistSchool != "" || saves[0].ResistSchoolPct != 0 || saves[0].ArmorBonus != 0 {
		t.Fatalf("save duplicated static item data: %+v", saves[0])
	}
	restored := restoreCombatBuffs(saves)
	if len(restored) != 1 || restored[0].ResistSchool != "fire" || restored[0].ResistSchoolPct != 50 {
		t.Fatalf("restored draught did not re-derive YAML data: %+v", restored)
	}

	g.setUtilityStatus("flame_ward_draught", buff.Frames)
	status := g.utilitySpellStatuses["flame_ward_draught"]
	// The draught wears its OWN bottle, not a spell's icon: pointing status_icon
	// at "fire_shield" once put the Fire Shield spell icon in the status bar for
	// a fire-resist potion. The token is the item key; resolveStatusIconSprite
	// turns it into icon_item_<key> when that sprite is present.
	if status == nil || status.Icon != "flame_ward_draught" || status.Label != "Flame Ward Draught" {
		t.Fatalf("draught status did not use its own YAML metadata: %+v", status)
	}
	for _, key := range []string{"flame_ward_draught", "storm_ward_draught", "gloom_ward_draught", "stoneskin_draught"} {
		def, ok := config.GetItemDefinition(key)
		if !ok || def.StatusIcon != key {
			t.Errorf("draught %q must carry its own status_icon, got %q", key, def.StatusIcon)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "assets", "sprites", "interface", "items", "icon_item_"+key+".png")); err != nil {
			t.Errorf("draught %q has no icon art for the status bar: %v", key, err)
		}
	}
}

func TestLegacyDraughtSaveMigratesToItemKey(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	restored := restoreCombatBuffs([]CombatBuffSave{{
		SpellID:         "draught_fire",
		Frames:          123,
		ResistSchool:    "fire",
		ResistSchoolPct: 50,
	}})
	if len(restored) != 1 {
		t.Fatalf("restored buffs = %d, want 1", len(restored))
	}
	if restored[0].SpellID != "flame_ward_draught" ||
		restored[0].ResistSchool != "fire" ||
		restored[0].ResistSchoolPct != 50 {
		t.Fatalf("legacy draught migration = %+v", restored[0])
	}
}

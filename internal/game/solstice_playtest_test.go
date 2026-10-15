package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/storage"
)

func TestSolsticePlaytestScenario(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, _ := bootOpenWorldGame(t, false)
	loadBenchContent(t)
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "solstice"); err != nil {
		t.Fatal(err)
	}
	if currentMapKey() != "solstice_approach" || g.appScreen != AppScreenInGame {
		t.Fatal("scenario did not open the approach")
	}
	if len(g.party.Members) != 4 {
		t.Fatal("missing playtest party")
	}
	for _, member := range g.party.Members {
		if member.Level != 20 || member.HitPoints != member.MaxHitPoints {
			t.Fatalf("unprepared member: %s", member.Name)
		}
	}
	for _, key := range []string{"solstice_thermal_lance", "solstice_flowstaff", "solstice_anchor_hammer", "solstice_transfer_blade", "solstice_fire_ward", "solstice_water_ward", "solstice_earth_ward", "solstice_air_ward", "grave_orchid"} {
		item, err := items.TryCreateWeaponFromYAML(key)
		if err != nil {
			item = items.CreateItemFromYAML(key)
		}
		want := item.Name
		found := false
		for _, item := range g.party.Inventory {
			found = found || item.Name == want
		}
		if !found {
			t.Errorf("missing test item %s", key)
		}
	}
}

// The new-items launcher: each Solstice reward worn by the hero it fits,
// built on the stat it scales with, the boss cards in the bag, in the desert.
func TestNewItemsPlaytestScenario(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, _ := bootOpenWorldGame(t, false)
	loadBenchContent(t)
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "new_items"); err != nil {
		t.Fatal(err)
	}
	if currentMapKey() != "desert" || g.appScreen != AppScreenInGame || g.currentLevelUpChoice() != nil {
		t.Fatal("scenario did not open the desert ready to play")
	}
	stat := func(m *character.MMCharacter, name string) int {
		return character.EffectiveCombatStats(m).ValueByName(name)
	}
	for i, want := range []struct{ weapon, stat string }{
		{"solstice_stone_script", "intellect"},
		{"solstice_sun_crucible", "intellect"},
		{"solstice_returning_gale", "accuracy"},
		{"", "intellect"}, // the retort scales flasks with Intellect
	} {
		m := g.party.Members[i]
		if m.Level != 35 || m.HitPoints != m.MaxHitPoints || m.SpellPoints != m.MaxSpellPoints {
			t.Errorf("%s is not a rested level-35 hero", m.Name)
		}
		if want.weapon != "" {
			if def, _, _ := config.GetWeaponDefinitionByName(m.Equipment[items.SlotMainHand].Name); def == nil || def != config.GlobalWeapons.Weapons[want.weapon] {
				t.Errorf("%s does not wield %s", m.Name, want.weapon)
			}
		}
		for _, other := range []string{"might", "intellect", "personality", "accuracy", "luck"} {
			if stat(m, other) > stat(m, want.stat) {
				t.Errorf("%s's %s %d beats the %s %d the gear scales with", m.Name, other, stat(m, other), want.stat, stat(m, want.stat))
			}
		}
	}
	elara := g.party.Members[3]
	if action := elara.Equipment[items.SlotQuickAction]; action.Flask != "solstice_everflow_retort" || !g.flaskReady(elara, action.Flask) {
		t.Error("the Alchemist does not ready a usable Everflow Retort")
	}
	for _, key := range []string{"solstice_aureth_card", "solstice_neressa_card", "solstice_talura_card", "solstice_vaelith_card"} {
		found := false
		for _, it := range g.party.Inventory {
			found = found || itemCardKey(it) == key
		}
		if !found {
			t.Errorf("missing card %s", key)
		}
	}
}

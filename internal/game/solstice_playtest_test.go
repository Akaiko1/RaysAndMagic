package game

import (
	"testing"
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

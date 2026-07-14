package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/world"
)

func TestPotionMerchantRestocksEverySevenSunrises(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load NPC config: %v", err)
	}
	potionShop, err := character.CreateNPCFromConfig("city_potion_shop", 96, 96)
	if err != nil {
		t.Fatalf("create potion shop: %v", err)
	}
	for _, entry := range potionShop.MerchantStock {
		entry.Quantity = 0
	}

	city := newTestWorld(cfg)
	city.NPCs = []*character.NPC{potionShop}
	g := newTestGame(cfg, city)
	previous := world.GlobalWorldManager
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "city", LoadedMaps: map[string]*world.World3D{"city": city}}
	t.Cleanup(func() { world.GlobalWorldManager = previous })

	g.calendarDay, g.calendarWeek, g.calendarMonth = 7, 1, 1
	g.refreshScheduledMerchantStocks()
	for _, entry := range potionShop.MerchantStock {
		if entry.Quantity != 0 {
			t.Fatalf("merchant restocked before the week elapsed: %s = %d", entry.Item.Name, entry.Quantity)
		}
	}

	weekChanged, _ := g.advanceCalendarAtDawn()
	if !weekChanged || g.calendarWeek != 2 {
		t.Fatalf("calendar should enter week 2 after day 7, got day=%d week=%d", g.calendarDay, g.calendarWeek)
	}
	g.refreshScheduledMerchantStocks()
	want := map[string]int{"Health Potion": 20, "Mana Potion": 20, "Revival Potion": 10}
	for _, entry := range potionShop.MerchantStock {
		if got := entry.Quantity; got != want[entry.Item.Name] {
			t.Errorf("%s stock = %d, want %d after weekly refresh", entry.Item.Name, got, want[entry.Item.Name])
		}
	}
}

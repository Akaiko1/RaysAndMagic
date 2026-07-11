package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestTownPortalRegistersConfiguredDestination(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 8, 8)
	w.StartX, w.StartY = 3, 5
	game := newTestGame(cfg, w)

	previousManager := world.GlobalWorldManager
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "japanese_castle",
		MapConfigs: map[string]*config.MapConfig{
			"japanese_castle": {
				Name:                  "Eastern Isle Castle",
				TownPortalDestination: true,
			},
		},
	}
	t.Cleanup(func() { world.GlobalWorldManager = previousManager })

	game.registerVisitedTownPortalDestination()
	got := game.sortedTownPortalDestinations()
	if len(got) != 1 || got[0] != "japanese_castle" {
		t.Fatalf("destinations = %v, want japanese_castle", got)
	}
	if label := townPortalDestinationLabel("japanese_castle"); label != "Eastern Isle Castle" {
		t.Fatalf("destination label = %q, want map name without Tavern", label)
	}

	x, y := w.GetStartingPosition()
	tileSize := float64(cfg.GetTileSize())
	if x != 3.5*tileSize || y != 5.5*tileSize {
		t.Fatalf("start position = (%v, %v), want tile (3, 5) center", x, y)
	}
}

func TestTownPortalConfiguredMaps(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("load map configs: %v", err)
	}
	for _, mapKey := range []string{"city", "japanese_castle"} {
		if mc := wm.MapConfigs[mapKey]; mc == nil || !mc.TownPortalDestination {
			t.Errorf("%s must be a Town Portal destination", mapKey)
		}
	}
}

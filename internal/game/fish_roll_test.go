package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"ugataima/internal/config"
)

// All species and both combat clocks reach the same per-tile roll. Zero and
// certain probabilities make the cadence, tile count and overlapping flights
// deterministic without replacing the production random source.
func TestFishIndependentTileRolls(t *testing.T) {
	for _, region := range []string{"forest", "sakura_garden", "highlands"} {
		for _, tb := range []bool{false, true} {
			for _, chance := range []float64{0, 1} {
				t.Run(fmt.Sprintf("%s/TB=%v/chance=%g", region, tb, chance), func(t *testing.T) {
					g, _, _ := fishTestGame(t, region)
					g.turnBasedMode = tb
					f := config.GlobalEcology.Fish
					f.SpawnChancePerTile = chance
					count := len(g.fishSources(region, f.RadiusTiles))
					if count != 2 {
						t.Fatalf("expected two connected water cells, got %d", count)
					}
					for i := 0; i < 119; i++ {
						g.frameCount++
						g.updateFish()
					}
					if len(g.world.Monsters) != 0 {
						t.Fatal("fish spawned before frame 120")
					}
					g.frameCount++
					g.updateFish()
					want := int(chance) * count
					if len(g.world.Monsters) != want {
						t.Fatalf("first roll: got %d, want %d", len(g.world.Monsters), want)
					}
					for i := 0; i < 120; i++ {
						g.frameCount++
						g.updateFish()
					}
					if len(g.world.Monsters) != 2*want {
						t.Fatalf("active fish blocked rolls: got %d, want %d", len(g.world.Monsters), 2*want)
					}
					for _, m := range g.world.Monsters {
						if m.Key != f.Species[region] {
							t.Fatalf("wrong regional species: %s", m.Key)
						}
					}
				})
			}
		}
	}
}

func TestFishLegacyCooldownIgnored(t *testing.T) {
	var state EcologyState
	if err := json.Unmarshal([]byte(`{"fish_cooldowns":{"forest":999999},"fish_roll_frames":119,"deliveries":4}`), &state); err != nil {
		t.Fatal(err)
	}
	if state.Deliveries != 4 {
		t.Fatal("legacy cooldown changed campaign state")
	}
	g, _, _ := fishTestGame(t, "forest")
	g.ecology = state
	config.GlobalEcology.Fish.SpawnChancePerTile = 1
	for i := 0; i < 120; i++ {
		g.frameCount++
		g.updateFish()
	}
	if len(g.world.Monsters) != 2 {
		t.Fatal("legacy save retained a spawn delay")
	}
}

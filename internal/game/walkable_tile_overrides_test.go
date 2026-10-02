package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Case table (made-up movers, not authored monsters): ordinary floor;
// matching/missing/wrong blocked-tile override, one or several overrides;
// flying/grounded chasm and wall; map bounds; live/snapshot movement and
// tile-only occupancy; entity collision still applies only to movement.
// These are the shared terrain entry points used by RT and TB pathfinding.
func TestWalkableTileOverrideMovement(t *testing.T) {
	g, ts := summonTileWorld(t)
	dunes := []string{"desert_dune", "large_dune"}
	undergrowth := []string{"thicket", "fern_patch"}
	for _, tc := range []struct {
		name, tile string
		overrides  []string
		flying     bool
		want       bool
	}{
		{"floor", "empty", nil, false, true},
		{"clearing", "clearing", nil, false, true},
		{"blocked_without_override", "desert_dune", nil, false, false},
		{"single_override", "desert_dune", dunes[:1], false, true},
		{"wrong_single_override", "large_dune", dunes[:1], false, false},
		{"first_of_two_overrides", "desert_dune", dunes, false, true},
		{"second_of_two_overrides", "large_dune", dunes, false, true},
		{"undergrowth_thicket", "thicket", undergrowth, false, true},
		{"undergrowth_fern", "fern_patch", undergrowth, false, true},
		{"undergrowth_wrong_override", "desert_dune", undergrowth, false, false},
		{"grounded_chasm", "dragon_cliffs_chasm_floor", nil, false, false},
		{"flying_chasm", "dragon_cliffs_chasm_floor", nil, true, true},
		{"grounded_wall", "wall", nil, false, false},
		{"flying_wall", "wall", nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tile, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.tile)
			if !ok {
				t.Fatalf("unknown tile %s", tc.tile)
			}
			w := newTestWorldSized(g.config, 3, 3)
			w.Tiles[1][1] = tile
			const moverID = "mover"
			cs := collision.NewCollisionSystem(w, ts)
			cs.RegisterEntity(collision.NewEntity(moverID, .5*ts, 1.5*ts, 16, 16, collision.CollisionTypeMonster, false))
			for _, blocked := range []bool{false, true} {
				if blocked {
					cs.RegisterEntity(collision.NewEntity("blocker", 1.5*ts, 1.5*ts, 16, 16, collision.CollisionTypeNPC, true))
				}
				snap := cs.Snapshot()
				for _, entry := range []struct {
					name string
					call func(string, float64, float64, []string, bool) bool
					move bool
				}{
					{"live_move", cs.CanMoveToWithTileOverrides, true},
					{"snapshot_move", snap.CanMoveToWithTileOverrides, true},
					{"live_tiles", cs.CanOccupyTilesWithTileOverrides, false},
					{"snapshot_tiles", snap.CanOccupyTilesWithTileOverrides, false},
				} {
					want := tc.want && !(blocked && entry.move)
					if got := entry.call(moverID, 1.5*ts, 1.5*ts, tc.overrides, tc.flying); got != want {
						t.Errorf("%s blocked=%v: got %v, want %v", entry.name, blocked, got, want)
					}
					for _, x := range []float64{-ts, 4 * ts} {
						if entry.call(moverID, x, 1.5*ts, tc.overrides, tc.flying) {
							t.Errorf("%s accepted out-of-map position", entry.name)
						}
					}
				}
			}
		})
	}
}

func TestWalkableTileOverridesAreEffective(t *testing.T) {
	summonTileWorld(t)
	for key, def := range monster.MonsterConfig.Monsters {
		seen := map[string]bool{}
		for _, override := range def.WalkableTileOverrides {
			tile, ok := world.GlobalTileManager.GetTileTypeFromKey(override)
			if !ok {
				t.Errorf("%s: unknown override %q", key, override)
				continue
			}
			if seen[override] || world.GlobalTileManager.IsWalkable(tile) || (def.Flying && world.GlobalTileManager.CanFlyOver(tile)) {
				t.Errorf("%s: redundant override %q", key, override)
			}
			seen[override] = true
		}
	}
}

// Overrides are reconstructed from YAML, never serialized. Current and legacy
// save rosters must retain the same movement permissions after reconstruction.
func TestWalkableTileOverridesSurviveRestore(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, key := range []string{"bandit", "desert_dervish", "forest_spider", "dragon", "dragon_green"} {
			t.Run(fmt.Sprintf("%s/legacy=%v", key, legacy), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				m := monster.NewMonster3DFromConfig(20.5*ts, 20.5*ts, key, g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				save := g.buildSave(wm)
				if legacy {
					save.MapMonsters = nil
				}
				g.restoreSavedMonsters(wm, &save)
				if len(g.world.Monsters) != 1 {
					t.Fatalf("restored %d monsters", len(g.world.Monsters))
				}
				restored := g.world.Monsters[0]
				w := newTestWorldSized(g.config, 1, 1)
				for tileKey := range world.GlobalTileManager.ListTiles() {
					tile, _ := world.GlobalTileManager.GetTileTypeFromKey(tileKey)
					w.Tiles[0][0] = tile
					before := w.IsTileBlockingForMonster(0, 0, m.WalkableTileOverrides, m.Flying)
					after := w.IsTileBlockingForMonster(0, 0, restored.WalkableTileOverrides, restored.Flying)
					if before != after {
						t.Errorf("load changed movement on %s", tileKey)
					}
				}
			})
		}
	}
}

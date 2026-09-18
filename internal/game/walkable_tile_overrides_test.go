package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Case table: ordinary floor; matching/missing/wrong blocked-tile override;
// flying/grounded chasm and wall; map bounds; live/snapshot movement and
// tile-only occupancy; entity collision still applies only to movement.
// These are the shared terrain entry points used by RT and TB pathfinding.
func TestWalkableTileOverrideMovement(t *testing.T) {
	g, ts := summonTileWorld(t)
	for _, tc := range []struct {
		name, monster, tile string
		want                bool
	}{
		{"floor", "bandit", "empty", true},
		{"clearing", "bandit", "clearing", true},
		{"blocked_without_override", "bandit", "desert_dune", false},
		{"dervish_dune", "desert_dervish", "desert_dune", true},
		{"dervish_wrong_dune", "desert_dervish", "large_dune", false},
		{"dragon_dune", "dragon", "desert_dune", true},
		{"dragon_large_dune", "dragon", "large_dune", true},
		{"spider_thicket", "forest_spider", "thicket", true},
		{"spider_fern", "forest_spider", "fern_patch", true},
		{"spider_wrong_override", "forest_spider", "desert_dune", false},
		{"grounded_chasm", "bandit", "dragon_cliffs_chasm_floor", false},
		{"flying_chasm", "dragon_green", "dragon_cliffs_chasm_floor", true},
		{"grounded_wall", "bandit", "wall", false},
		{"flying_wall", "dragon_green", "wall", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tile, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.tile)
			if !ok {
				t.Fatalf("unknown tile %s", tc.tile)
			}
			w := newTestWorldSized(g.config, 3, 3)
			w.Tiles[1][1] = tile
			m := monster.NewMonster3DFromConfig(.5*ts, 1.5*ts, tc.monster, g.config)
			cs := collision.NewCollisionSystem(w, ts)
			cs.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, false))
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
					if got := entry.call(m.ID, 1.5*ts, 1.5*ts, m.WalkableTileOverrides, m.Flying); got != want {
						t.Errorf("%s blocked=%v: got %v, want %v", entry.name, blocked, got, want)
					}
					for _, x := range []float64{-ts, 4 * ts} {
						if entry.call(m.ID, x, 1.5*ts, m.WalkableTileOverrides, m.Flying) {
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

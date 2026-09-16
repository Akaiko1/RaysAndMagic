package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Real stitched regions initialize the private projection index and region grid.
// Both layouts use the snapshot/roster restore, including repeated restores.
func TestAIPatrolAnchorSaveLayouts(t *testing.T) {
	cfg := loadTestConfig(t)
	tile := float64(cfg.GetTileSize())
	t.Chdir("../..")
	oldTM := world.GlobalTileManager
	oldWM := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager = oldTM; world.GlobalWorldManager = oldWM })
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}

	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, orient := range []string{"none", "rot90", "rot180", "rot270", "mirror_x", "mirror_y"} {
		base := world.NewWorldManager(cfg)
		if err := base.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
			t.Fatal(err)
		}
		base.MapConfigs = map[string]*config.MapConfig{"forest": base.MapConfigs["forest"]}
		base.SetOpenWorldConfig(&config.OpenWorldConfig{VoidTile: "oob_cliff", Placements: map[string]config.OpenWorldPlacement{"forest": {X: 20, Y: 20, Orient: orient}}})
		if err := base.LoadAllMaps(); err != nil {
			t.Fatal(err)
		}
		if x, y := base.ProjectWorldPos("forest", 0, 0); x == 0 && y == 0 {
			t.Fatal("fixture did not initialize region projection")
		}

		for _, fromMerged := range []bool{false, true} {
			for _, toMerged := range []bool{false, true} {
				for _, legacyRoster := range []bool{false, true} {
					for _, anchor := range []string{"ordinary", "zero", "outside_region", "legacy_absent"} {
						t.Run(fmt.Sprintf("%s/from=%v/to=%v/old_roster=%v/%s", orient, fromMerged, toMerged, legacyRoster, anchor), func(t *testing.T) {
							g := newTestGame(cfg, newTestWorldSized(cfg, 100, 100))
							g.combat = NewCombatSystem(g)
							var wm *world.WorldManager
							configure := func(merged bool) {
								if merged {
									copyWM := *base
									wm = &copyWM
									wm.LoadedMaps = map[string]*world.World3D{}
									wm.OpenWorld = g.world
								} else {
									wm = world.NewWorldManager(cfg)
									wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
								}
								wm.CurrentMapKey = "forest"
								world.GlobalWorldManager = wm
							}
							configure(fromMerged)
							px, py := 12.5*tile, 12.5*tile
							sx, sy := 10.5*tile, 10.5*tile
							if anchor == "zero" {
								sx, sy = 0, 0
							}
							if anchor == "outside_region" {
								sx, sy = -2.5*tile, 35.5*tile
							}
							x, y := wm.ProjectWorldPos("forest", px, py)
							m := monster.NewMonster3DFromConfig(x, y, "karasu_tengu", g.config)
							m.SpawnX, m.SpawnY = wm.ProjectWorldPos("forest", sx, sy)
							g.world.Monsters = []*monster.Monster3D{m}
							saved := g.buildSave(wm)
							if anchor == "legacy_absent" {
								saved.MapMonsters["forest"][0].SpawnPosition = nil
								sx, sy = px, py
							}
							if legacyRoster {
								saved.Monsters = saved.MapMonsters["forest"]
								saved.MapMonsters = nil
							}
							bytes, err := json.Marshal(saved)
							if err != nil {
								t.Fatal(err)
							}
							if err = json.Unmarshal(bytes, &saved); err != nil {
								t.Fatal(err)
							}
							before, _ := json.Marshal(saved)
							configure(toMerged)
							for n := 0; n < 2; n++ {
								g.restoreSavedMonsters(wm, &saved)
								if len(g.world.Monsters) != 1 {
									t.Fatalf("restored %d monsters", len(g.world.Monsters))
								}
								got := g.world.Monsters[0]
								wantX, wantY := wm.ProjectWorldPos("forest", sx, sy)
								if got.SpawnX != wantX || got.SpawnY != wantY {
									t.Fatalf("anchor (%v,%v), want (%v,%v)", got.SpawnX, got.SpawnY, wantX, wantY)
								}
								// A future save must keep the home even after more wandering.
								got.X += tile
								next := g.buildSave(wm)
								g.restoreSavedMonsters(wm, &next)
								got = g.world.Monsters[0]
								if got.SpawnX != wantX || got.SpawnY != wantY {
									t.Fatal("second save rebased the anchor")
								}
							}
							after, _ := json.Marshal(saved)
							if string(before) != string(after) {
								t.Fatal("restore mutated saved coordinates")
							}
						})
					}
				}
			}
		}
	}
}

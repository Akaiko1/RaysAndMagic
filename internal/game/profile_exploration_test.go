package game

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
	"ugataima/internal/config"
	"ugataima/internal/playerprofile"
	"ugataima/internal/world"
)

func TestProfileExplorationWalkingAndResizing(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			g.turnBasedMode = tb
			g.world = newTestWorldSized(g.config, 8, 8)
			g.collisionSystem.UpdateTileChecker(g.world)
			wm := world.GlobalWorldManager
			wm.CurrentMapKey = "forest"
			wm.LoadedMaps["forest"] = g.world
			ts := g.config.GetTileSize()
			g.setPartyPosition(2.5*ts, 2.5*ts)
			g.updatePlayerProfile(time.Now())
			if tb {
				h.loop.inputHandler.moveTurnBasedInDirection(1, 0)
			} else {
				h.loop.inputHandler.movePlayer(ts, 0)
			}
			d := &g.playerProfile.Data
			if d.VisitedTileCount("forest", 8, 8) != 2 {
				t.Fatal("start and committed walking tile not recorded")
			}
			if tb {
				h.loop.inputHandler.moveTurnBasedInDirection(-1000, 0)
			} else {
				h.loop.inputHandler.movePlayer(-1000*ts, 0)
			}
			g.updatePlayerProfile(time.Now())
			if d.VisitedTileCount("forest", 8, 8) != 2 {
				t.Fatal("blocked step or repeated frame added a tile")
			}
			path := filepath.Join(t.TempDir(), "save.json")
			if err := g.SaveGameToFile(path); err != nil {
				t.Fatal(err)
			}
			if err := g.LoadGameFromFile(path); err != nil {
				t.Fatal(err)
			}
			g.updatePlayerProfile(time.Now())
			if d.VisitedTileCount("forest", 8, 8) != 2 {
				t.Fatal("save restoration changed tile history")
			}
			scores := g.profileExplorationStats().entries
			if len(scores) != 1 || scores[0].Count != 31 {
				t.Fatalf("all 64 tiles must be the denominator: %v", scores)
			}
			// A tile-type edit does not change the denominator or erase a coordinate.
			g.world.Tiles[2][3] = world.TileWall
			g.recordProfileExploration(3.5*ts, 2.5*ts)
			if scores = g.profileExplorationStats().entries; scores[0].Count != 31 {
				t.Fatal("tile type changed exploration")
			}
			g.world.Width = 16
			if scores = g.profileExplorationStats().entries; scores[0].Count != 15 {
				t.Fatal("resizing must change area without shifting coordinates")
			}
		})
	}
}

func TestProfileExplorationUsesLocalOpenWorldCoordinates(t *testing.T) {
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, true)
	// This content boot uses repo-relative assets, unlike the small UI fixture.
	s, err := playerprofile.Open(filepath.Join(t.TempDir(), "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	g.playerProfile = s
	defer s.Close()
	g.world = wm.OpenWorld
	for _, r := range wm.OpenWorldRegions {
		t.Run(r.MapKey, func(t *testing.T) {
			tx, ty := wm.ProjectTile(r.MapKey, 2, 3)
			if actual := wm.OpenWorldRegionAtTile(tx, ty); actual == nil || actual.MapKey != r.MapKey {
				t.Fatal("invalid region fixture")
			}
			// Deliberately stale CurrentMapKey: the tile, not the last UI region, owns it.
			wm.CurrentMapKey = "wrong-region"
			x, y := TileCenterFromTile(tx, ty, cfg.GetTileSize())
			g.recordProfileExploration(x, y)
			g.recordProfileExploration(x, y)
			if !s.Data.VisitedTiles[r.MapKey]["2,3"] || len(s.Data.VisitedTiles[r.MapKey]) != 1 {
				t.Fatalf("not a unique local coordinate: %v", s.Data.VisitedTiles[r.MapKey])
			}
		})
	}
	var totalArea int64
	for _, r := range wm.OpenWorldRegions {
		totalArea += int64(r.LocalWidth) * int64(r.LocalHeight)
	}
	for _, mapped := range wm.LoadedMaps {
		totalArea += int64(mapped.Width) * int64(mapped.Height)
	}
	summary := g.profileExplorationStats()
	if summary.area != totalArea || summary.visited != int64(len(wm.OpenWorldRegions)) {
		t.Fatalf("stitched regions counted incorrectly: visited=%d area=%d want=%d", summary.visited, summary.area, totalArea)
	}
	if _, ok := s.Data.VisitedTiles["wrong-region"]; ok {
		t.Fatal("cross-region visit used stale region")
	}
}

func TestProfileExplorationSortsPercentNotAreaOrTime(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	wm := world.GlobalWorldManager
	wm.LoadedMaps = map[string]*world.World3D{"small": newTestWorldSized(g.config, 4, 4), "large": newTestWorldSized(g.config, 10, 10)}
	wm.MapConfigs = map[string]*config.MapConfig{"small": {Name: "Small"}, "large": {Name: "Large"}}
	d := &g.playerProfile.Data
	for x := 0; x < 4; x++ {
		d.VisitTile("small", x, 0)
	}
	for x := 0; x < 10; x++ {
		d.VisitTile("large", x, 0)
	}
	d.Rank("regions", "large", "Large", "", 10000)
	scores := g.profileExplorationStats().entries
	if len(scores) != 2 || scores[0].Name != "Small" || scores[0].Count != 250 || scores[1].Count != 100 {
		t.Fatalf("bad percentage ranking: %v", scores)
	}
	spec := profilePages[2].rankings[1]
	if spec.group != "exploration" || spec.value(250) != "25.0%" {
		t.Fatal("discoveries still displays time")
	}
	if profilePages[0].rankings[2].group != "regions" || !profilePages[0].rankings[2].duration {
		t.Fatal("favorite regions changed")
	}
}

func TestProfileExplorationWaitsForCommittedLoadingPose(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	oldTM := world.GlobalTileManager
	world.GlobalTileManager = nil
	t.Cleanup(func() { world.GlobalTileManager = oldTM })
	h.loop.renderer = &Renderer{game: g}
	h.loop.loading = &gameLoadingState{awaitingFrame: true}
	t.Cleanup(h.loop.closeResourceLoading)
	g.updatePlayerProfile(time.Now())
	if len(g.playerProfile.Data.VisitedTiles) != 0 {
		t.Fatal("loading pose counted as occupied gameplay tile")
	}
	h.loop.closeResourceLoading()
	g.updatePlayerProfile(time.Now())
	if len(g.playerProfile.Data.VisitedTiles) == 0 {
		t.Fatal("ready gameplay pose not recorded")
	}
}

func TestProfileOverallExplorationBadge(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
		visit bool
		want  string
		area  int64
	}{
		{"unvisited", 10, false, "0.0%", 116},
		{"weighted and unvisited regions", 10, true, "3.4%", 116},
		{"map grows", 20, true, "1.8%", 216},
		{"empty large map", 0, true, "25.0%", 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			wm := world.GlobalWorldManager
			small, large := newTestWorldSized(g.config, 4, 4), newTestWorldSized(g.config, 10, 10)
			large.Width = tc.width
			wm.LoadedMaps = map[string]*world.World3D{"small": small, "large": large}
			wm.OpenWorldRegions = nil
			wm.MapConfigs = map[string]*config.MapConfig{"small": {Name: "Small"}, "large": {Name: "Large"}}
			d := &g.playerProfile.Data
			if tc.visit {
				for x := 0; x < 4; x++ {
					d.VisitTile("small", x, 0)
					d.VisitTile("small", x, 0)
				}
				small.Tiles[0][1] = world.TileWall // All tile types stay in the denominator.
			}
			d.VisitTile("small", 8, 8)   // Historical coordinates outside today's map.
			d.VisitTile("removed", 0, 0) // Removed maps contribute neither count nor area.
			counter := profilePages[2].counters[2]
			if counter.key != "exploration" {
				t.Fatal("Discoveries lacks exploration badge")
			}
			if got := h.ui.profileCounterValue(counter, d); got != tc.want {
				t.Fatalf("badge=%s want %s", got, tc.want)
			}
			if h.ui.profileExploration.area != tc.area {
				t.Fatalf("area=%d want %d", h.ui.profileExploration.area, tc.area)
			}
			entries := h.ui.profileRankingEntries(profilePages[2].rankings[1], d)
			if tc.visit && (len(entries) != 1 || entries[0].Count != 250) {
				t.Fatalf("region breakdown changed: %+v", entries)
			}
			if !tc.visit && len(entries) != 0 {
				t.Fatal("unvisited maps appeared as explored")
			}
		})
	}
}

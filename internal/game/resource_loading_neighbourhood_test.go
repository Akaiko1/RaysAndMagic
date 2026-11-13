package game

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"ugataima/internal/world"
)

func neighbourhoodLoadingFixture(t *testing.T) *GameLoop {
	t.Helper()
	gl := loadingFixture(t)
	g, r := gl.game, gl.renderer
	g.world = newTestWorldSized(g.config, 110, 30)
	g.world.Monsters = nil
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "center", OpenWorld: g.world,
		OpenWorldRegions: []world.OpenWorldRegion{
			{MapKey: "center", OffsetX: 10, OffsetY: 10, Width: 10, Height: 10},
			{MapKey: "east", OffsetX: 20, OffsetY: 10, Width: 10, Height: 10},
			{MapKey: "west", OffsetX: 0, OffsetY: 10, Width: 10, Height: 10},
			{MapKey: "north", OffsetX: 10, OffsetY: 0, Width: 10, Height: 10},
			{MapKey: "south", OffsetX: 10, OffsetY: 20, Width: 10, Height: 10},
			{MapKey: "far", OffsetX: 100, OffsetY: 10, Width: 10, Height: 10},
		},
	}
	ts := float64(g.config.GetTileSize())
	g.setPartyPosition(15.5*ts, 15.5*ts)
	g.camera.Angle, g.camera.FOV, g.camera.ViewDist = 0, math.Pi/3, 6*ts
	r.mapRenderResourcesByMap = make(map[string]*mapRenderRegionResources)
	r.mapRenderResidentMapKeys = nil
	r.commitMapRenderResidency("center", testMapRenderResources())
	return gl
}

func TestOpenWorldLoadingCompletesSurroundingBatch(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			gl := neighbourhoodLoadingFixture(t)
			g, r := gl.game, gl.renderer
			g.turnBasedMode = tb
			if !gl.loadingBarrier() {
				t.Fatal("cold visible region did not start loading")
			}
			started := gl.loading.started
			// Complete the visible neighbours first. The last neighbour is
			// behind the party and must join this episode before play resumes.
			for _, key := range []string{"east", "north", "south", "west"} {
				if gl.loadingWorkReady() {
					t.Fatalf("released loading before nearby %s completed", key)
				}
				r.commitMapRenderResidency(key, testMapRenderResources())
			}
			r.mapRenderResourcePrewarmMapKeys = nil
			r.mapRenderResourcePrewarmPending = false
			if !gl.loadingWorkReady() || gl.loading.started != started {
				t.Fatal("nearby batch did not complete as one episode")
			}
			if _, loaded := r.mapRenderResourcesByMap["far"]; loaded {
				t.Fatal("loading expanded beyond the nearby prefetch area")
			}
			// Draw owns release after a complete frame; model that already
			// validated transition here to isolate subsequent party movement.
			gl.loading.awaitingFrame = false
			manifests := make(map[string]*mapRenderRegionResources)
			for key, resources := range r.mapRenderResourcesByMap {
				manifests[key] = resources
			}
			for _, step := range []struct {
				name string
				x, y float64
			}{
				{"same_tile_forward", 15.8, 15.5},
				{"same_tile_backward", 15.2, 15.5},
				{"same_tile_sideways", 15.5, 15.8},
				{"two_steps_east", 17.5, 15.5},
				{"two_steps_west", 13.5, 15.5},
				{"seam_before", 19.9, 15.5},
				{"seam_after", 20.1, 15.5},
			} {
				for direction := 0; direction < 8; direction++ {
					ts := float64(g.config.GetTileSize())
					g.setPartyPosition(step.x*ts, step.y*ts)
					g.camera.Angle = float64(direction) * math.Pi / 4
					g.syncOpenWorldRegion()
					if gl.loadingBarrier() {
						t.Fatalf("%s/direction=%d reopened loading", step.name, direction)
					}
					if len(r.mapRenderResourcePrewarmMapKeys) != 0 {
						t.Fatalf("%s requeued resident neighbours", step.name)
					}
					for key, resources := range manifests {
						if r.mapRenderResourcesByMap[key] != resources {
							t.Fatalf("%s replaced nearby %s resources", step.name, key)
						}
					}
				}
			}
		})
	}
}

func TestOpenWorldSpeculativeWorkAndActiveLoading(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, region := range []string{"west", "far"} {
			for _, work := range []string{"manifest", "upload", "shader"} {
				t.Run(fmt.Sprintf("active=%v/%s/%s", active, region, work), func(t *testing.T) {
					gl := neighbourhoodLoadingFixture(t)
					r := gl.renderer
					for _, key := range []string{"east", "north", "south", "west"} {
						if work != "manifest" || key != region {
							r.commitMapRenderResidency(key, testMapRenderResources())
						}
					}
					task := &mapRenderPrewarmTask{mapKey: region}
					if work == "upload" {
						r.mapRenderUploadQueue = []mapRenderUpload{{task: task}}
					} else if work == "shader" {
						r.mapRenderShaderWarmTasks = []*mapRenderPrewarmTask{task}
					}
					gl.loading.awaitingFrame = active
					wantReady := !active || region == "far"
					if gl.loadingWorkReady() != wantReady {
						t.Fatalf("readiness must respect active nearby batch: want %v", wantReady)
					}
					if !active && gl.loadingBarrier() {
						t.Fatal("speculative work started a gameplay pause")
					}
				})
			}
		}
	}
}

// Residency reaches the configured lead past the required distance: a region
// in that band loads in the background without pausing, stays through the
// hysteresis band and leaves beyond it.
func TestOpenWorldPrefetchLeadLoadsBeforeARegionIsRequired(t *testing.T) {
	// The fixture sees 6 tiles; "far" spans x 100..110. Required reaches
	// 6+4 = 10 tiles, residency 10+lead, retention 14+lead.
	for _, tc := range []struct {
		name                     string
		lead, cameraX            float64
		resident                 bool // "far" published before the step
		wantQueued, wantResident bool
		wantPause                bool
	}{
		{name: "inside the lead", lead: 20, cameraX: 75.5, wantQueued: true},
		{name: "no lead configured", lead: 0, cameraX: 75.5},
		{name: "beyond the lead", lead: 20, cameraX: 65.5},
		{name: "required and missing", lead: 20, cameraX: 91.5, wantQueued: true, wantPause: true},
		{name: "resident inside retention", lead: 20, cameraX: 67.5, resident: true, wantResident: true},
		{name: "resident beyond retention", lead: 20, cameraX: 64.5, resident: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := neighbourhoodLoadingFixture(t)
			g, r := gl.game, gl.renderer
			g.config.Graphics.Streaming.PrefetchLeadTiles = tc.lead
			if tc.resident {
				r.commitMapRenderResidency("far", testMapRenderResources())
			}
			ts := float64(g.config.GetTileSize())
			g.setPartyPosition(tc.cameraX*ts, 15.5*ts)
			g.camera.Angle = 0
			if pause := gl.loadingBarrier(); pause != tc.wantPause {
				t.Fatalf("loading pause = %v, want %v", pause, tc.wantPause)
			}
			queued := slices.Contains(r.mapRenderResourcePrewarmMapKeys, "far")
			if task := r.mapRenderResourcePrewarmActive; task != nil && task.mapKey == "far" {
				queued = true
			}
			if queued != tc.wantQueued {
				t.Fatalf("background load queued = %v, want %v", queued, tc.wantQueued)
			}
			if _, resident := r.mapRenderResourcesByMap["far"]; resident != tc.wantResident {
				t.Fatalf("resident = %v, want %v", resident, tc.wantResident)
			}
		})
	}
}

// The view polygon circumscribes its arc and reaches past the nearby radius.
// A region it sees there stays required once the pause began; released, the
// pause would lift on Draw and return on the next Update for as long as the
// party stood at that distance.
func TestOpenWorldPausedLoadingKeepsEveryVisibleRegion(t *testing.T) {
	gl := neighbourhoodLoadingFixture(t)
	g, r := gl.game, gl.renderer
	ts := float64(g.config.GetTileSize())
	// "far" begins 10.01 tiles dead ahead: outside the 10-tile nearby radius,
	// inside the polygon's corner on the view axis.
	g.setPartyPosition(89.99*ts, 15.5*ts)
	g.camera.Angle = 0
	if slices.Contains(nearbyOpenWorldMapKeys(world.GlobalWorldManager, g.camera, ts, mapRenderLoadMarginInTiles*ts), "far") {
		t.Fatal("fixture: the region is inside the nearby radius")
	}
	if !gl.loadingBarrier() {
		t.Fatal("fixture: the visible region did not pause the game")
	}
	if gl.loadingWorkReady() || !slices.Contains(r.requiredLoadingRegions(), "far") {
		t.Fatal("a paused frame dropped the visible region it paused for")
	}
	r.commitMapRenderResidency("far", testMapRenderResources())
	r.mapRenderResourcePrewarmMapKeys, r.mapRenderResourcePrewarmPending = nil, false
	if !gl.loadingWorkReady() {
		t.Fatal("the pause outlived its region")
	}
}

// Corridors and the void between maps lie outside every region footprint;
// each such tile is prepared by exactly one region, the nearest.
func TestRegionScopeOwnsEveryTileOnce(t *testing.T) {
	gl := neighbourhoodLoadingFixture(t)
	g := gl.game
	regions := world.GlobalWorldManager.OpenWorldRegions
	owners := func(tx, ty int) (keys []string) {
		for i := range regions {
			scope := mapRenderPrewarmScope{mapKey: regions[i].MapKey, region: &regions[i], regions: regions, tileSize: 64}
			if scope.containsTile(tx, ty) {
				keys = append(keys, regions[i].MapKey)
			}
		}
		return keys
	}
	for ty := 0; ty < g.world.Height; ty++ {
		for tx := 0; tx < g.world.Width; tx++ {
			if got := owners(tx, ty); len(got) != 1 {
				t.Fatalf("tile (%d,%d) is prepared by %v, want one region", tx, ty, got)
			}
		}
	}
	for _, tc := range []struct {
		tx, ty int
		want   string
	}{
		{15, 15, "center"}, // footprint
		{25, 15, "east"},   // a neighbour's footprint is never shared
		{34, 15, "east"},   // void past the east edge
		{96, 15, "far"},    // void nearer to the far region
		{15, 25, "south"},
		{3, 3, "west"}, // a corner between two footprints goes to one of them
	} {
		if got := owners(tc.tx, tc.ty); len(got) != 1 || got[0] != tc.want {
			t.Errorf("tile (%d,%d) is prepared by %v, want %s", tc.tx, tc.ty, got, tc.want)
		}
	}
}

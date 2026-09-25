package game

import (
	"fmt"
	"math"
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

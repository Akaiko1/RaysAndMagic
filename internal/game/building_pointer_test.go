package game

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"math"
	"os"
	"testing"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// Wall-backed facades must pick through their own backing plane, with the
// same stable result for hover and clicks. Foreground walls still win.
// Rotated spans cover stitched-world geometry; no hover state is persisted.
func TestBuildingEntrancePointerBackingWall(t *testing.T) {
	for _, key := range []string{"solstice_vestibule_entrance", "solstice_jungle_entrance", "pyramid_entrance", "clock_tower_entrance"} {
		for _, dir := range []string{"e", "s", "w", "n"} {
			t.Run(key+"/"+dir, func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1280, 720)
				g := h.g
				restoreNPCCatalog(t)
				if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
					t.Fatal(err)
				}
				tm := world.NewTileManager(testTileSizeClasses())
				if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
					t.Fatal(err)
				}
				oldTM := world.GlobalTileManager
				t.Cleanup(func() { world.GlobalTileManager = oldTM })
				world.GlobalTileManager = tm
				wall, _ := tm.GetTileTypeFromKey("wall")
				ts := float64(g.config.GetTileSize())
				npc, err := character.CreateNPCFromConfig(key, 13.5*ts, 10.5*ts)
				if err != nil {
					t.Fatal(err)
				}
				npc.GridSpanDir = dir
				for _, p := range g.buildingFootprintTiles(npc) {
					tx, ty := TileIndex(p[0], ts), TileIndex(p[1], ts)
					if dir == "e" || dir == "w" {
						ty--
					} else {
						tx--
					}
					g.world.Tiles[ty][tx] = wall
				}
				g.world.NPCs = []*character.NPC{npc}
				g.world.Monsters = nil
				g.menuOpen, g.dialogActive = false, false
				g.config.Graphics.Standee.Enabled = true
				g.renderHelper = NewRenderingHelper(g)
				g.worldFrame = image.Point{}
				g.camera.FOV = squareProjectionFOV(1280, 720)
				g.camera.ViewDist = 5000
				ex, ey, yaw, ok := g.buildingPose(npc)
				if !ok {
					t.Fatal("missing building pose")
				}
				// Backing plane is flush for the temple; one half-tile behind the
				// free-standing pyramid/tower center.
				bx, by := ex, ey
				if !npc.WallBacked {
					if dir == "e" || dir == "w" {
						by -= ts / 2
					} else {
						bx -= ts / 2
					}
				}
				for _, oblique := range []float64{0, 0.25} {
					for _, jitter := range []float64{-0.000001, 0, 0.000001} {
						for _, foreground := range []bool{false, true} {
							if dir == "e" || dir == "w" {
								g.camera.X, g.camera.Y = ex+oblique*ts, ey+ts*1.5
							} else {
								g.camera.X, g.camera.Y = ex+ts*1.5, ey+oblique*ts
							}
							g.camera.Angle = math.Atan2(ey-g.camera.Y, ex-g.camera.X)
							g.cameraPresentation.presented = g.cameraPose()
							g.cameraPresentation.presentedValid = true
							sx, sy, size, visible := g.renderHelper.NPCSpriteMetrics(npc, ex, ey, Distance(ex, ey, g.camera.X, g.camera.Y))
							if !visible {
								t.Fatal("invisible fixture")
							}
							y := max(1, min(g.worldHeight()/2, sy+size-1))
							dx, dy := math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
							px, py := -dy*math.Tan(g.camera.FOV/2), dx*math.Tan(g.camera.FOV/2)
							nx, ny := -math.Sin(yaw), math.Cos(yaw)
							g.depthBuffer = make([]float64, g.worldWidth())
							for x := range g.depthBuffer {
								rx, ry := standeeRayAtScreenX(float64(x)+.5, g.worldWidth(), dx, dy, px, py)
								depth := (nx*(bx-g.camera.X) + ny*(by-g.camera.Y)) / (nx*rx + ny*ry)
								if foreground {
									depth -= ts
								}
								g.depthBuffer[x] = depth + jitter
							}
							want := !foreground
							if got := g.npcScreenHitTest(npc, ex, ey, sx, y); got != want {
								t.Fatalf("hover oblique=%v jitter=%g foreground=%v: got %v want %v", oblique, jitter, foreground, got, want)
							}
							ux, uy := sx, y
							got, inRange := g.findNPCAtScreen(ux, uy)
							if (got == npc) != want || (want && !inRange) {
								t.Fatalf("click target mismatch: %v %v", got, inRange)
							}
							if want {
								g.dialogActive = false
								g.mouseLeftClicks = []queuedClick{{x: ux, y: uy, at: time.Now().UnixMilli()}}
								NewInputHandler(g).handleWorldMouseInput()
								if !g.dialogActive || g.dialogNPC != npc {
									t.Fatalf("entrance click did not open %s", key)
								}
								g.closeConversation()
							}
						}
					}
				}
			})
		}
	}
}

func TestSolsticeEntranceRenderedPicking(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires renderer")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, stitched := range []bool{false, true} {
		t.Run(fmt.Sprint(stitched), func(t *testing.T) {
			g, _, cfg := bootOpenWorldGame(t, stitched)
			for _, entry := range []struct{ mapKey, key string }{{"solstice_approach", "solstice_vestibule_entrance"}, {"deep_jungle", "solstice_jungle_entrance"}} {
				if stitched && entry.mapKey == "deep_jungle" {
					continue
				} // replaced by the open-world passage
				if err := g.transitionToMap(mapTransition{mapKey: entry.mapKey, arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				var npc *character.NPC
				for _, n := range g.world.NPCs {
					if n.Key == entry.key {
						npc = n
						break
					}
				}
				if npc == nil {
					t.Fatal("missing entrance")
				}
				g.world.Monsters = nil
				ex, ey := g.npcEffectivePos(npc)
				dx, dy := g.buildingWallOffset(npc)
				for _, offset := range []float64{0, 0.2} {
					cx, cy := ex-dx*3, ey-dy*3
					if dx == 0 {
						cx += offset * cfg.GetTileSize()
					} else {
						cy += offset * cfg.GetTileSize()
					}
					g.setPartyPosition(cx, cy)
					g.camera.Angle = math.Atan2(ey-cy, ex-cx)
					runOnDrawFrame(func(_ *ebiten.Image) {
						g.handleResize(image.Pt(1280, 720), image.Pt(1280, 720))
						dst := ebiten.NewImage(1280, 720)
						defer dst.Deallocate()
						g.gameLoop.renderer.RenderFirstPersonView(dst)
						x, y, size, visible := g.renderHelper.NPCSpriteMetrics(npc, ex, ey, Distance(cx, cy, ex, ey))
						y = max(1, min(g.worldHeight()/2, y+size-1))
						got, reachable := g.findNPCAtScreen(x, y)
						if !visible || got != npc || !reachable {
							t.Errorf("%s/offset=%v: rendered entrance cannot be picked", entry.key, offset)
						}
					})
				}
			}
		})
	}
}

func TestSolsticeEntranceWallPlacement(t *testing.T) {
	t.Chdir("../..")
	for _, stitched := range []bool{false, true} {
		g, wm, _ := bootOpenWorldGame(t, stitched)
		for _, key := range []string{"deep_jungle", "solstice_approach"} {
			if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
				t.Fatal(err)
			}
			target := "solstice_jungle_entrance"
			if key == "solstice_approach" {
				target = "solstice_vestibule_entrance"
			}
			found := false
			for _, npc := range g.world.NPCs {
				if npc.Key != target {
					continue
				}
				found = true
				dx, dy := g.buildingWallOffset(npc)
				ts := float64(g.config.GetTileSize())
				if math.Abs(math.Hypot(dx, dy)-ts/2) > 1e-6 {
					t.Fatalf("%s stitched=%v: no flush wall offset", key, stitched)
				}
				for _, tile := range g.buildingFootprintTiles(npc) {
					if !world.GlobalTileManager.IsSolid(g.world.GetTileAt(tile[0]+2*dx, tile[1]+2*dy)) {
						t.Fatal("gap behind facade")
					}
					if world.GlobalTileManager.IsSolid(g.world.GetTileAt(tile[0]-2*dx, tile[1]-2*dy)) {
						t.Fatal("blocked approach")
					}
				}
			}
			if stitched && key == "deep_jungle" {
				if found {
					t.Fatal("stitched jungle passage retained its split-map facade")
				}
			} else if !found {
				t.Fatalf("missing facade in %s (%v)", key, wm.FailedMaps)
			}
		}
	}
}

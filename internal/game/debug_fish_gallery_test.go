//go:build debug

package game

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Captures authored maps with production sprites, flight, damage and loot.
// The short gallery forces rare outcomes instead of waiting for their RNG.
func TestDebugSim_FishGallery(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	oldEcology := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = oldEcology })
	if err := config.LoadEcology("assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("RAM_FISH_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	installFakePointer(t).moveTo(0, 0)
	tile := g.config.GetTileSize()
	frameNo := 0
	for sceneIndex, scene := range []struct{ region, key, title string }{
		{"forest", "common_carp", "Forest - Common Carp"},
		{"sakura_garden", "koi", "Japanese Garden - Koi"},
		{"highlands", "rainbow_salmon", "Highlands - Rainbow Salmon"},
	} {
		if filter := os.Getenv("RAM_FISH_QA_REGIONS"); filter != "" && !strings.Contains(","+filter+",", ","+scene.region+",") {
			continue
		}
		frameNo = sceneIndex * 315
		if err := g.switchToMap(scene.region); err != nil {
			t.Fatal(err)
		}
		// Isolate the scripted fish from other combatants that can intercept
		// the test arrow. Map terrain, NPCs, physics and rendering stay authored.
		for _, m := range g.world.Monsters {
			g.collisionSystem.UnregisterEntity(m.ID)
		}
		g.world.Monsters = nil
		// Search actual connected water for a nearby, unobstructed bank viewpoint.
		found := false
		var source [2]int
		for y := 1; y < g.world.Height-1 && !found; y++ {
			for x := 1; x < g.world.Width-1 && !found; x++ {
				if !g.fishRegionContains(scene.region, x, y) || !fishWater(g.world, x, y) || len(g.fishDestinations(scene.region, x, y, true)) == 0 || len(g.fishDestinations(scene.region, x, y, false)) == 0 {
					continue
				}
				for dy := -3; dy <= 3 && !found; dy++ {
					for dx := -3; dx <= 3 && !found; dx++ {
						distance := math.Hypot(float64(dx), float64(dy))
						if distance < 2.4 || distance > 3.2 {
							continue
						}
						px, py := (float64(x+dx)+.5)*tile, (float64(y+dy)+.5)*tile
						tx, ty := (float64(x)+.5)*tile, (float64(y)+.5)*tile
						if !g.fishRegionContains(scene.region, x+dx, y+dy) || fishWater(g.world, x+dx, y+dy) || !g.world.CanMoveTo(px, py) || !g.collisionSystem.CanMoveTo("player", px, py) || !g.collisionSystem.CheckLineOfSight(px, py, tx, ty) {
							continue
						}
						// Transparent standees can still hide a tiny fish: require a clear
						// floor corridor for the gallery camera, without changing the map.
						clearView := true
						for sample := 0; sample <= 12; sample++ {
							sx := int((px + (tx-px)*float64(sample)/12) / tile)
							sy := int((py + (ty-py)*float64(sample)/12) / tile)
							for oy := -1; oy <= 1; oy++ {
								for ox := -1; ox <= 1; ox++ {
									if sx+ox < 0 || sy+oy < 0 || sx+ox >= g.world.Width || sy+oy >= g.world.Height {
										clearView = false
										continue
									}
									d := world.GlobalTileManager.GetTileData(g.world.Tiles[sy+oy][sx+ox])
									if d == nil || d.RenderType != "floor" {
										clearView = false
									}
								}
							}
						}
						for _, npc := range g.world.NPCs {
							if Distance(npc.X, npc.Y, tx, ty) < 5*tile || Distance(npc.X, npc.Y, px, py) < 4*tile {
								clearView = false
							}
						}
						if !clearView {
							continue
						}
						g.setPartyPosition(px, py)
						g.snapFacing(math.Atan2(ty-py, tx-px))
						source, found = [2]int{x, y}, true
					}
				}
			}
		}
		if !found {
			t.Fatalf("no viewable water in %s", scene.region)
		}
		t.Logf("%s water=%v party=(%.1f,%.1f)", scene.region, source, g.camera.X/tile, g.camera.Y/tile)
		g.menuOpen, g.turnBasedMode, g.currentTurn = false, true, 0
		for _, m := range g.world.Monsters {
			m.Pacified = true
		}
		w, h := g.gameLoop.Layout(1280, 720)
		captureGameplayPreviewFrame(t, g, w, h)
		for outcomeIndex, outcome := range []string{"Dive - no drop", "Stranded - one scale", "Caught in flight - one scale"} {
			firstScene, _ := strconv.Atoi(os.Getenv("RAM_FISH_QA_FIRST_SCENE"))
			if sceneIndex*3+outcomeIndex < firstScene {
				continue
			}
			frameNo = sceneIndex*315 + outcomeIndex*105
			settings := *config.GlobalEcology.Fish
			settings.SpawnChancePerTile = 0
			config.GlobalEcology.Fish = &settings
			g.groundContainers = nil
			var fish *monster.Monster3D
			screen := ebiten.NewImage(w, h)
			for f := 0; f < 105; f++ {
				shot := image.NewRGBA(image.Rect(0, 0, w, h))
				runOnDrawFrame(func(_ *ebiten.Image) {
					if f == 15 {
						roll := 1.0
						if outcome == "Stranded - one scale" {
							roll = 0
						}
						fish = g.spawnLeapingFish(scene.region, scene.key, source, config.GlobalEcology.Fish, roll)
					}
					for step := 0; step < g.config.GetTPS()/30; step++ {
						g.frameCount++
						g.updateFish()
						g.updateMonsterDeaths()
					}
					if f == 30 && outcome == "Caught in flight - one scale" && fish != nil {
						// Lead the moving fish using the capture's 30 Hz projectile clock.
						speed := 8.0
						vx := (fish.FishLeap.ToX - fish.FishLeap.FromX) / (fish.FishLeap.Duration * 30)
						vy := (fish.FishLeap.ToY - fish.FishLeap.FromY) / (fish.FishLeap.Duration * 30)
						dx, dy := fish.X-g.camera.X, fish.Y-g.camera.Y
						a, b, c := vx*vx+vy*vy-speed*speed, 2*(dx*vx+dy*vy), dx*dx+dy*dy
						ticks := (-b - math.Sqrt(b*b-4*a*c)) / (2 * a)
						angle := math.Atan2(dy+vy*ticks, dx+vx*ticks)
						id := g.GenerateProjectileID("arrow")
						g.arrows = append(g.arrows, Arrow{ID: id, Active: true, LifeTime: 90, X: g.camera.X, Y: g.camera.Y, VelX: math.Cos(angle) * speed, VelY: math.Sin(angle) * speed, Damage: 100, DamageType: "physical", BowKey: "hunting_bow", Owner: ProjectileOwnerPlayer, Attacker: g.party.Members[3]})
						g.collisionSystem.RegisterEntity(collision.NewEntity(id, g.camera.X, g.camera.Y, 8, 8, collision.CollisionTypeProjectile, false))
					}
					g.gameLoop.updateProjectilesAndImpacts()
					g.gameLoop.removeDeadMonstersByID()
					g.Draw(screen)
					ebitenutil.DebugPrintAt(screen, scene.title+"  |  "+outcome+"  |  Controlled gameplay test", 12, 92)
					screen.ReadPixels(shot.Pix)
				})
				file, err := os.Create(filepath.Join(out, fmt.Sprintf("frame_%05d.png", frameNo)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, shot)
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("PNG: %v %v", err, closeErr)
				}
				frameNo++
			}
			screen.Deallocate()
			want := 1
			if outcome == "Dive - no drop" {
				want = 0
			}
			if len(g.groundContainers) != want {
				t.Fatalf("%s %s: drops=%d", scene.key, outcome, len(g.groundContainers))
			}
		}
	}
	t.Logf("captured %d frames in %s", frameNo, out)
}

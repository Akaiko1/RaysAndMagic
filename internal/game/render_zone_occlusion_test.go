package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Exercise the production collector, painter sort and GPU draw. A column-depth
// approximation cannot satisfy both opaque coverage and transparent openings.
func TestWorldFieldOcclusionWithScenery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires the live Ebitengine test loop")
	}
	cfg := setupPreviewSandboxTest(t)
	t.Chdir("../..")
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	g, r := p.g, p.g.gameLoop.renderer
	g.handleResize(image.Pt(640, 480), image.Pt(640, 480))
	p.clearTransient()
	r.wallTorches = nil
	for _, panini := range []bool{false, true} {
		g.combatPreferences.WideView = panini
		// Offset the wide camera so even a low curtain crosses both the trunk
		// and an opening; straight-on it can fit entirely behind the trunk.
		g.camera.Y = p.homeY
		if panini {
			g.camera.Y += 24
		}
		for _, effectKind := range []string{"firewall", "trap-flames", "thief-trap", "telegraph-warning", "telegraph-active", "impassable", "spawn"} {
			for _, key := range []string{"fern_patch", "tree"} {
				t.Run(fmt.Sprintf("panini=%v/%s/%s", panini, effectKind, key), func(t *testing.T) {
					tile, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
					if !ok {
						t.Fatal("missing scenery fixture")
					}
					data := world.GlobalTileManager.GetTileData(tile)
					prop := TransparentSpriteData{tileX: 6, tileY: 8, worldX: 6.5 * 64, worldY: 8.5 * 64, tileType: tile, spriteName: data.Sprite}
					runOnDrawFrame(func(_ *ebiten.Image) {
						defer func() { g.persistentDamageZones = nil; g.bossFireTraps = nil; g.traps = nil; g.world.Monsters = nil }()
						originalTiles := make([][]world.TileType3D, len(g.world.Tiles))
						for y, row := range g.world.Tiles {
							originalTiles[y] = append([]world.TileType3D(nil), row...)
						}
						startX, startY := g.world.StartX, g.world.StartY
						defer func() { g.world.Tiles = originalTiles; g.world.StartX, g.world.StartY = startX, startY }()
						chasm, _ := world.GlobalTileManager.GetTileTypeFromKey("dragon_cliffs_chasm_floor")
						if effectKind == "impassable" && !tileShowsImpassableAura(world.GlobalTileManager.GetTileData(chasm)) {
							t.Error("missing chasm fixture")
							return
						}
						g.config.Graphics.ImpassableAura.Enabled = true
						dst := ebiten.NewImage(g.worldWidth(), g.worldHeight())
						defer dst.Deallocate()
						previous := activeWorldRaster
						defer func() { activeWorldRaster = previous }()
						if panini {
							g.gameLoop.paniniRaster.configure(dst, g.widePaniniProjection())
							activeWorldRaster = &g.gameLoop.paniniRaster
						}
						wallDepth := math.Inf(1)
						render := func(scenery bool, depth int) []byte {
							r.transparentSpritesCache, r.treeTilesCache, r.treeHits = nil, nil, nil
							if scenery {
								if key == "tree" {
									r.treeTilesCache = []TransparentSpriteData{prop}
								} else {
									r.transparentSpritesCache = []TransparentSpriteData{prop}
								}
							}
							r.propSpatial.rebuild(r.transparentSpritesCache, 64)
							r.treeSpatial.rebuild(r.treeTilesCache, 64)
							g.persistentDamageZones = nil
							g.bossFireTraps = nil
							g.traps = nil
							g.world.Monsters = nil
							g.world.StartX, g.world.StartY = -1, -1
							for y, row := range originalTiles {
								copy(g.world.Tiles[y], row)
							}
							if depth > 0 {
								tx, ty := 3+depth, 8
								switch effectKind {
								case "firewall":
									g.persistentDamageZones = []PersistentDamageZone{{SpellID: "firewall", X: p.homeX + float64(depth)*64, Y: p.homeY, Radius: 35, FramesLeft: 100, AxisY: 1}}
								case "trap-flames":
									g.bossFireTraps = []bossFireTrap{{TX: tx, TY: ty}}
								case "thief-trap":
									g.traps = []PlacedTrap{{Key: "stasis_trap", MapKey: fxStageMapKey, TileX: tx, TileY: ty}}
								case "telegraph-warning", "telegraph-active":
									m := &monster.Monster3D{HitPoints: 1, Telegraph: monster.TelegraphState{Tiles: [][2]int{{tx, ty}}}}
									if effectKind == "telegraph-warning" {
										m.Telegraph.Warning = 1
									} else {
										m.Telegraph.Remaining = 1
									}
									g.world.Monsters = []*monster.Monster3D{m}
								case "impassable":
									g.world.Tiles[ty][tx] = chasm
									r.floorColorCache[[2]int{tx, ty}] = color.RGBA{100, 100, 100, 255}
								case "spawn":
									g.world.StartX, g.world.StartY = tx, ty
								}
							}
							for x := range g.depthBuffer {
								g.depthBuffer[x] = wallDepth
								g.actorDepthBuffer[x] = math.Inf(1)
							}
							dst.Clear()
							r.drawAllSpritesSorted(dst)
							pixels := make([]byte, 4*g.worldWidth()*g.worldHeight())
							dst.ReadPixels(pixels)
							return pixels
						}
						base := render(true, 0)
						fire := render(false, 5)
						behind := render(true, 5)
						front := render(true, 2)
						wallDepth = 64
						behindWall := render(false, 5)
						if !bytes.Equal(behindWall, make([]byte, len(behindWall))) {
							t.Error("field drew through a nearer wall")
						}
						opaque, covered, leaks, frontChanges, openings := 0, 0, 0, 0, 0
						loX, hiX, loY, hiY := g.worldWidth(), 0, g.worldHeight(), 0
						for i := 0; i < len(base); i += 4 {
							if base[i+3] == 255 {
								x, y := (i/4)%g.worldWidth(), (i/4)/g.worldWidth()
								loX = min(loX, x)
								hiX = max(hiX, x)
								loY = min(loY, y)
								hiY = max(hiY, y)
								opaque++
								if fire[i+3] > 2 {
									covered++
									// Alpha can round to 255 while retaining a sub-byte
									// background contribution in the GPU blend.
									for channel := 0; channel < 4; channel++ {
										if absInt(int(base[i+channel])-int(behind[i+channel])) > 1 {
											leaks++
											break
										}
									}
								}
								if !bytes.Equal(base[i:i+4], front[i:i+4]) {
									frontChanges++
								}
							}
						}
						// The tree fixture has openings at curtain height; the fern is
						// opaque near its base, where the low curtain intersects it.
						for y := loY; y <= hiY; y++ {
							for x := loX; x <= hiX; x++ {
								i := (y*g.worldWidth() + x) * 4
								if base[i+3] == 0 && fire[i+3] > 2 && behind[i+3] > 2 {
									openings++
								}
							}
						}
						// The small device fits wholly behind this trunk; unlike the
						// wide fields, it must be fully hidden rather than peek through.
						wantOpenings := key == "tree" && effectKind != "thief-trap"
						if opaque < 10 || covered < 10 || frontChanges < 10 || (wantOpenings && openings == 0) {
							t.Errorf("fixture lacks coverage: opaque=%d covered=%d front=%d openings=%d", opaque, covered, frontChanges, openings)
						}
						t.Logf("opaque=%d covered=%d front=%d openings=%d leaks=%d", opaque, covered, frontChanges, openings, leaks)
						if leaks > 0 {
							t.Errorf("field behind scenery overwrote %d opaque pixels", leaks)
						}
					})
				})
			}
		}
	}
}

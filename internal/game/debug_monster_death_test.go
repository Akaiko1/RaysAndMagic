//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Runs the real sorted sprite pass and standee shader. Optional frame exports
// are diagnostic captures, not reconstructed art or a simulated renderer.
func TestMonsterDeathGPU(t *testing.T) {
	requireStandeeGPU(t)
	// Ground/flying x small/large x near/far x RT/TB x both renderers.
	// Live and newly dead anchors agree; airborne bodies descend to ground.
	for _, key := range []string{"bandit", "pixie", "skeleton", "dust_slime", "dragon", "fennec"} {
		for _, distance := range []float64{0.75, 2, 12} {
			for _, tb := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/distance=%g/tb=%v", key, distance, tb), func(t *testing.T) {
					testMonsterDeathGPU(t, key, distance, tb)
				})
			}
		}
	}
}

func testMonsterDeathGPU(t *testing.T, key string, distance float64, tb bool) {
	g := deathTestGame(t)
	g.turnBasedMode = tb
	const width, height = 960, 600
	g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = width, height
	g.camera.FOV = squareProjectionFOV(width, height)
	g.camera.X, g.camera.Y, g.camera.Angle = (3.5-distance)*g.config.GetTileSize(), 3.5*g.config.GetTileSize(), 0
	g.camera.ViewDist = 40 * g.config.GetTileSize()
	g.renderHelper = NewRenderingHelper(g)
	g.depthBuffer = make([]float64, width)
	g.wallTopBuffer = make([]int, width)
	for i := range g.depthBuffer {
		g.depthBuffer[i] = g.camera.ViewDist
	}
	r := &Renderer{game: g, ambientLight: 1}
	var target *ebiten.Image
	runOnDrawFrame(func(_ *ebiten.Image) { target = ebiten.NewImage(width, height) })
	defer runOnDrawFrame(func(_ *ebiten.Image) { target.Deallocate() })
	ts := g.config.GetTileSize()
	m := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, key, g.config)
	m.StandeeYaw = math.Pi / 2
	m.StandeeYawTick = 1
	_, ground, _, _ := g.renderHelper.CalculateMonsterSpriteMetricsF(m.X, m.Y, distance*ts, m.GetSizeGameMultiplier())
	liveBottom := make(map[bool]float64)
	g.world.Monsters = []*monster.Monster3D{m}
	for _, standee := range []bool{false, true} {
		g.config.Graphics.Standee.Enabled = standee
		runOnDrawFrame(func(_ *ebiten.Image) { target.Clear(); r.drawAllSpritesSorted(target) })
		for _, sprite := range r.unifiedSprites {
			if sprite.spriteType == SpriteTypeMonster {
				liveBottom[standee] = sprite.bottomF
			}
		}
		bottom, found := liveBottom[standee]
		if !found || (m.Flying && bottom >= ground) || (!m.Flying && math.Abs(bottom-ground) > 0.01) {
			t.Fatalf("standee=%v flying=%v live bottom=%v ground=%v found=%v", standee, m.Flying, bottom, ground, found)
		}
	}
	g.world.Monsters = nil
	m.HitPoints = 0
	g.beginMonsterDeath(m)
	// Force an open, lateral landing for the capture.
	g.world.Tiles[3][4] = world.TileWall
	g.world.Tiles[3][2] = world.TileWall
	g.world.Tiles[2][3] = world.TileWall
	g.addMonsterLootDrop(m, nil, 17)
	loot := g.groundContainers
	g.groundContainers = nil
	fadeStart := 0.5
	if m.Flying {
		fadeStart = g.monsterDeathSettings().FallSeconds
	}
	for _, standee := range []bool{false, true} {
		g.config.Graphics.Standee.Enabled = standee
		// At point-blank range the landed body can be below the viewport.
		// Anchor continuity is checked at every distance below; pixel fading
		// requires a fully visible body.
		if distance < 2 {
			continue
		}
		sums := []int64{}
		for _, age := range []float64{fadeStart, fadeStart + 2.5, fadeStart + 5} {
			g.frameCount = int64(age * float64(g.config.GetTPS()))
			var sum int64
			runOnDrawFrame(func(_ *ebiten.Image) {
				target.Clear()
				r.drawAllSpritesSorted(target)
				pixels := make([]byte, width*height*4)
				target.ReadPixels(pixels)
				for i := 3; i < len(pixels); i += 4 {
					sum += int64(pixels[i])
				}
			})
			sums = append(sums, sum)
		}
		if sums[0] == 0 || sums[2] != 0 || math.Abs(float64(sums[1])/float64(sums[0])-0.5) > 0.03 {
			t.Fatalf("standee=%v fade alpha sums=%v", standee, sums)
		}
	}
	// Exercise collection through the real sorted pass, with both render paths.
	for _, standee := range []bool{false, true} {
		g.config.Graphics.Standee.Enabled = standee
		g.groundContainers = loot
		previous := -1.0
		for _, age := range []float64{0, 0.5, 1} {
			g.frameCount = int64(age * float64(g.config.GetTPS()))
			runOnDrawFrame(func(_ *ebiten.Image) { target.Clear(); r.drawAllSpritesSorted(target) })
			bags, bodies := 0, 0
			for _, sprite := range r.unifiedSprites {
				switch sprite.spriteType {
				case SpriteTypeGroundContainer:
					bags++
				case SpriteTypeMonsterCorpse:
					bodies++
					if age == 0 && math.Abs(sprite.bottomF-liveBottom[standee]) > 0.01 {
						t.Fatal("death changed the live anchor before falling")
					}
					if age == 1 && math.Abs(sprite.bottomF-ground) > 0.01 {
						t.Fatal("corpse did not land at its projected ground contact")
					}
					if m.Flying && previous >= 0 && sprite.bottomF <= previous {
						t.Fatal("flying corpse did not descend in production renderer")
					}
					previous = sprite.bottomF
				}
			}
			wantBags := 1
			if m.Flying && age < 1 {
				wantBags = 0
			}
			// The lateral loot landing is outside the near camera's FOV.
			if (distance >= 2 && bags != wantBags) || bodies != 1 {
				t.Fatalf("standee=%v age=%v bags=%d want=%d bodies=%d", standee, age, bags, wantBags, bodies)
			}
		}
	}
	out := os.Getenv("RAM_DEATH_OUT")
	if out == "" {
		return
	}
	out = filepath.Join(out, fmt.Sprintf("%s_distance%g_tb%v", key, distance, tb))
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	g.config.Graphics.Standee.Enabled = true
	g.groundContainers = loot
	for frame := 0; frame <= int((fadeStart+5)*20); frame++ {
		g.frameCount = int64(float64(frame) / 20 * float64(g.config.GetTPS()))
		var shot *image.RGBA
		runOnDrawFrame(func(_ *ebiten.Image) {
			target.Fill(color.RGBA{28, 32, 27, 255})
			r.drawAllSpritesSorted(target)
			shot = image.NewRGBA(image.Rect(0, 0, width, height))
			target.ReadPixels(shot.Pix)
		})
		path := filepath.Join(out, fmt.Sprintf("frame_%03d.png", frame))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// HUD visibility must never move world actors or their attached effects. This
// compares actual GPU output across both sprite renderers and combat modes.
func TestMonsterRenderingIndependentOfPartyHUD(t *testing.T) {
	requireStandeeGPU(t)
	for _, key := range []string{"bandit", "pixie"} {
		t.Run(key, func(t *testing.T) {
			g := deathTestGame(t)
			const width, height = 960, 600
			g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = width, height
			ts := g.config.GetTileSize()
			g.camera.FOV = squareProjectionFOV(width, height)
			g.camera.X, g.camera.Y, g.camera.Angle = 3.2*ts, 3.5*ts, 0
			g.camera.ViewDist = 40 * ts
			g.renderHelper = NewRenderingHelper(g)
			g.depthBuffer = make([]float64, width)
			g.wallTopBuffer = make([]int, width)
			for i := range g.depthBuffer {
				g.depthBuffer[i] = g.camera.ViewDist
			}
			r := &Renderer{game: g, ambientLight: 1}
			var target *ebiten.Image
			runOnDrawFrame(func(_ *ebiten.Image) { target = ebiten.NewImage(width, height) })
			defer runOnDrawFrame(func(_ *ebiten.Image) { target.Deallocate() })
			m := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, key, g.config)
			m.StandeeYaw, m.StandeeYawTick = math.Pi/2, 1
			for _, tb := range []bool{false, true} {
				for _, standee := range []bool{false, true} {
					for _, state := range []string{"alive", "elemental", "dying", "landed"} {
						t.Run(fmt.Sprintf("tb=%v/standee=%v/%s", tb, standee, state), func(t *testing.T) {
							g.turnBasedMode = tb
							g.config.Graphics.Standee.Enabled = standee
							g.camera.X = 3.2 * ts
							if state == "landed" {
								g.camera.X = 2.5 * ts
							}
							g.frameCount = 1
							m.Direction = 0
							m.StandeeYaw, m.StandeeYawTick = math.Pi/2, 1
							g.monsterCorpses = nil
							g.elementalAttackEffects = nil
							m.HitPoints = m.MaxHitPoints
							g.world.Monsters = []*monster.Monster3D{m}
							if state == "elemental" {
								g.addMonsterElementalAttackFX(m, "fire")
							}
							if state == "dying" || state == "landed" {
								m.HitPoints = 0
								g.beginMonsterDeath(m)
								if state == "landed" {
									g.frameCount += int64(g.config.GetTPS())
								}
							}
							snapshots := make([][]byte, 2)
							for i, hud := range []bool{false, true} {
								g.showPartyStats = hud
								runOnDrawFrame(func(_ *ebiten.Image) {
									target.Clear()
									r.drawAllSpritesSorted(target)
									if state == "elemental" {
										target.Clear() // isolate the attached glyph from the actor
										r.drawElementalAttackFX(target)
									}
									snapshots[i] = make([]byte, width*height*4)
									target.ReadPixels(snapshots[i])
								})
							}
							var visible bool
							for i := 3; i < len(snapshots[0]); i += 4 {
								if snapshots[0][i] > 0 {
									visible = true
									break
								}
							}
							if !visible {
								t.Fatal("fixture rendered no visible pixels")
							}
							if !bytes.Equal(snapshots[0], snapshots[1]) {
								t.Fatal("party HUD visibility changed world sprite pixels")
							}
							if len(r.unifiedSprites) != 1 || r.unifiedSprites[0].bottomF <= float64(gameplayViewportBottom(g)) {
								t.Fatal("fixture does not cross the party bar boundary")
							}
						})
					}
				}
			}
		})
	}
}

//go:build debug

package game

import (
	"bytes"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
)

func TestDebugSim_SniperGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	ch := character.CreateCharacter("Mara", character.ClassSniper, g.config)
	originalHero := g.party.Members[0]
	g.party.Members[0] = ch
	g.updateTacticalClocks()
	g.tactics.stationarySeconds = 10
	g.tactics.movedTB = false
	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_Mara_implementation")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	minW, minH := MinimumWindowSize()
	for _, size := range [][2]int{{minW, minH}, {1024, 768}, {1920, 1080}, {3840, 2160}} {
		lw, lh := g.gameLoop.Layout(size[0], size[1])
		if lw > size[0] || lh > size[1] {
			t.Fatalf("preview would downsample text: %dx%d into %dx%d", lw, lh, size[0], size[1])
		}
		for _, view := range []string{"hud", "party-create", "tavern"} {
			g.appScreen = AppScreenInGame
			g.dialogActive = false
			g.party.Members[0] = originalHero
			if view == "hud" {
				g.party.Members[0] = ch
			}
			switch view {
			case "party-create":
				g.appScreen = AppScreenPartyCreate
				g.partyCreate = newPartyCreateState(g.config)
				g.partyCreate.poolScroll = 999
				for _, hero := range g.partyCreate.pool {
					if hero.char.Name == "Mara" {
						g.partyCreate.detail = hero
					}
				}
			case "tavern":
				g.dialogActive = true
				g.dialogNPC = tavernTestNPC()
				g.dialogTab = 0
				g.rosterScroll = 999
			}
			var problem error
			runOnDrawFrame(func(_ *ebiten.Image) {
				screen := ebiten.NewImage(lw, lh)
				defer screen.Deallocate()
				if view == "hud" {
					r.RenderFirstPersonView(screen)
				}
				if view == "party-create" {
					g.gameLoop.Draw(screen)
				} else {
					g.gameLoop.ui.Draw(screen)
				}
				f, err := os.Create(filepath.Join(out, fmt.Sprintf("%s-%dx%d.png", view, size[0], size[1])))
				if err != nil {
					problem = err
					return
				}
				defer f.Close()
				output := ebiten.NewImage(size[0], size[1])
				defer output.Deallocate()
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(float64(size[0])/float64(lw), float64(size[1])/float64(lh))
				op.Filter = ebiten.FilterNearest
				output.DrawImage(screen, op)
				problem = png.Encode(f, output)
			})
			if problem != nil {
				t.Fatal(problem)
			}
		}
	}
}

func TestDebugSim_OverwatchHUDAndTargetMarker(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.gameLoop.Layout(1024, 768)
	ch := character.CreateCharacter("Mara", character.ClassSniper, g.config)
	g.party.Members[0] = ch
	tile := float64(g.config.GetTileSize())
	placePlayerAtTile(g, 12, 4, tile)
	g.camera.Angle, g.viewAngleRender = 0, 0
	m := spawnMonsterAtTile(g, "wolf", 15, 4, tile)
	m.State = monster.StatePursuing
	g.updateTacticalClocks()
	g.tactics.stationarySeconds = 10
	g.tactics.movedTB = false
	g.designateTarget(ch, m)
	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_Mara_implementation")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tb := range []bool{false, true} {
		g.turnBasedMode = tb
		var baseline []byte
		for _, shake := range []float64{0, 6, 0} {
			g.screenShake = shake
			x, y := g.camera.X, g.camera.Y
			runOnDrawFrame(func(_ *ebiten.Image) {
				screen := ebiten.NewImage(1024, 768)
				defer screen.Deallocate()
				g.gameLoop.drawExplorationFrame(screen)
				pw, ph, left, top := partyPortraitLayout(g)
				panelX, panelY, _, _ := partyCardPanelRect(left, top, pw, ph)
				cx := panelX + panelPortraitX + panelPortraitW - 11
				cy := panelY + panelPortraitY + 11
				crop := image.Rect(cx-8, cy-8, cx+8, cy+8)
				pixels := make([]byte, crop.Dx()*crop.Dy()*4)
				screen.SubImage(crop).(*ebiten.Image).ReadPixels(pixels)
				if baseline == nil {
					baseline = pixels
				} else if !bytes.Equal(baseline, pixels) {
					t.Error("screen shake changed Overwatch badge pixels")
				}
				if shake == 0 {
					f, err := os.Create(filepath.Join(out, fmt.Sprintf("marked-target-tb-%v.png", tb)))
					if err != nil {
						t.Error(err)
						return
					}
					if err = png.Encode(f, screen); err != nil {
						t.Error(err)
					}
					f.Close()
				}
			})
			if g.camera.X != x || g.camera.Y != y || !g.overwatchReady(ch) {
				t.Fatal("scene draw changed logical readiness")
			}
		}
	}
	// Test the actual shared status-FX draw wire with no other status effects.
	for _, state := range []string{"active", "expired", "dead", "reserve", "wall"} {
		g.party.Members[0] = ch
		m.HitPoints = m.MaxHitPoints
		ch.DesignationFrames = 100
		g.depthBuffer = make([]float64, 100)
		for i := range g.depthBuffer {
			g.depthBuffer[i] = math.Inf(1)
		}
		switch state {
		case "expired":
			ch.DesignationFrames = 0
		case "dead":
			m.HitPoints = 0
		case "reserve":
			g.party.Members[0] = nil
		case "wall":
			g.depthBuffer[50] = 1
		}
		var drawn bool
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen := ebiten.NewImage(100, 100)
			defer screen.Deallocate()
			r.drawMonsterStatusFX(screen, UnifiedSpriteRenderData{monster: m, screenX: 50, spriteSize: 50, depthPerp: 10}, 40)
			pixels := make([]byte, 100*100*4)
			screen.ReadPixels(pixels)
			for i := 3; i < len(pixels); i += 4 {
				if pixels[i] > 0 {
					drawn = true
					break
				}
			}
		})
		if drawn != (state == "active") {
			t.Errorf("%s: visible mark=%v", state, drawn)
		}
	}
}

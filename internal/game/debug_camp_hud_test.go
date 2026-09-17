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
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestDebugSim_CampHUDGallery(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	// Every authored tile class must match production; uniform test classes
	// previously made trees and scenery the wrong size in exported previews.
	tm := world.GlobalTileManager
	for _, key := range tm.GetAllTileKeys() {
		tile, _ := tm.GetTileTypeFromKey(key)
		data := tm.GetTileData(tile)
		if want, ok := config.ResolveSizeClassTiles(g.config.Graphics.SizeClasses, data.SizeClass); ok && tm.GetSizeTiles(tile) != want {
			t.Fatalf("preview tile %s size=%v want production %v", key, tm.GetSizeTiles(tile), want)
		}
	}
	x, y := TileCenterFromTile(13, 36, g.config.GetTileSize())
	x, y = world.GlobalWorldManager.ProjectWorldPos("forest", x, y)
	g.setPartyPosition(x, y)
	g.snapFacing(0)
	g.party.Food = 12
	g.menuOpen = false
	g.party.Members[g.selectedChar].QuickSlots[0] = itemPointerForCampGallery()
	g.AddCombatMessage("The path is quiet. A good place to make camp.")
	g.utilitySpellStatuses = map[spells.SpellID]*UtilitySpellStatus{
		"bless": {SpellID: "bless", Icon: "status_bless", Label: "Bless", Duration: 240, MaxDuration: 300},
	}
	out := os.Getenv("RAM_CAMP_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	fp := installFakePointer(t)
	for _, res := range campHUDResolutions {
		w, h := g.gameLoop.Layout(res[0], res[1])
		for _, state := range []string{"", "_hover", "_confirm"} {
			fp.moveTo(0, 0)
			g.campConfirmOpen = state == "_confirm"
			if state == "_hover" {
				l, _ := inGameActionBarLayout(g)
				fp.moveTo(l.camp.x+l.camp.w/2, l.camp.y+l.camp.h/2)
			}
			shot := captureGameplayPreviewFrame(t, g, w, h)
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("camping_window_%dx%d_native_%dx%d%s.png", res[0], res[1], w, h, state)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, shot)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("capture write: %v %v", err, closeErr)
			}
		}
	}
}

// Export through the production world renderer at the native layout size.
// Only the interlude phase is selected; no scenery or rendering overrides.
func TestDebugSim_CampScenesGallery(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.threading.Shutdown()
	g.menuOpen = false
	out := os.Getenv("RAM_CAMP_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	save := func(name string, w, h int) {
		t.Helper()
		shot := captureGameplayPreviewFrame(t, g, w, h)
		f, err := os.Create(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write preview: %v %v", err, closeErr)
		}
	}
	scenes := []string{"forest", "desert", "highlands", "dragon_cliffs", "jungle", "japanese_castle", "sakura_garden", "dungeon", "water"}
	for _, res := range [][2]int{{800, 600}, {1920, 1080}, {3440, 1440}, {3840, 2160}} {
		w, h := g.gameLoop.Layout(res[0], res[1])
		for _, scene := range scenes {
			if scene != "forest" && res[0] != 1920 {
				continue
			}
			g.beginCampRest()
			g.campRest.sprite = "camp_" + scene
			if !g.sprites.HasSprite(g.campRest.sprite) {
				t.Fatalf("missing authored camp scene %s", scene)
			}
			g.campRest.elapsed = g.campRest.fadeIn + g.campRest.hold/2
			save(fmt.Sprintf("scene_%s_window_%dx%d_native_%dx%d.png", scene, res[0], res[1], w, h), w, h)
			if scene == "forest" && res[0] == 800 {
				g.campRest.elapsed = g.campRest.fadeIn / 2
				save(fmt.Sprintf("scene_forest_fade_in_native_%dx%d.png", w, h), w, h)
				g.campRest.elapsed = g.campRest.fadeIn + g.campRest.hold + g.campRest.fadeOut/2
				save(fmt.Sprintf("scene_forest_fade_out_native_%dx%d.png", w, h), w, h)
			}
		}
	}
}

// Capture the complete game frame only after the production loading/update/draw
// pipeline is ready. No render settings overrides or post-render resizing.
func captureGameplayPreviewFrame(t *testing.T, g *MMGame, w, h int) *image.RGBA {
	t.Helper()
	frame := ebiten.NewImage(w, h)
	defer frame.Deallocate()
	var shot *image.RGBA
	// Initial open-world prewarming competes with other local work. Allow the
	// real loader to finish rather than bypassing it for a faster screenshot.
	deadline := time.Now().Add(180 * time.Second)
	for shot == nil && time.Now().Before(deadline) {
		var updateErr error
		runOnDrawFrame(func(_ *ebiten.Image) {
			updateErr = g.Update()
			g.Draw(frame)
			gl := g.gameLoop
			if gl.loading != nil && !gl.loading.awaitingFrame && gl.loading.front != nil && gl.loading.bannerAlpha(time.Now()) == 0 {
				shot = image.NewRGBA(image.Rect(0, 0, w, h))
				frame.ReadPixels(shot.Pix)
			}
		})
		if updateErr != nil {
			t.Fatal(updateErr)
		}
	}
	if shot == nil {
		t.Fatalf("gameplay preview did not finish loading: screen=%v loading=%+v", g.appScreen, g.gameLoop.loading)
	}
	return shot
}

func itemPointerForCampGallery() *items.Item {
	item := items.CreateItemFromYAML("health_potion")
	return &item
}

// Hover is a white, alpha-shaped edge cue; normal and modal-blocked states
// retain the exact base image. Persistence is not involved in this draw rule.
func TestDebugSim_CampHUDSilhouetteGlow(t *testing.T) {
	requireStandeeGPU(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	g.party.Food = 12
	fp := installFakePointer(t)
	for _, res := range campHUDResolutions {
		w, h := g.gameLoop.Layout(res[0], res[1])
		l, _ := inGameActionBarLayout(g)
		region := image.Rect(l.camp.x-4, l.camp.y-4, l.camp.right()+4, l.camp.bottom()+4).Intersect(image.Rect(0, 0, w, h))
		capture := func(hover, blocked bool) *image.RGBA {
			fp.moveTo(0, 0)
			if hover {
				fp.moveTo(l.camp.x+l.camp.w/2, l.camp.y+l.camp.h/2)
			}
			g.mainMenuOpen = blocked
			shot := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
			runOnDrawFrame(func(_ *ebiten.Image) {
				screen := ebiten.NewImage(w, h)
				defer screen.Deallocate()
				g.gameLoop.ui.drawCampHUD(screen)
				screen.SubImage(region).(*ebiten.Image).ReadPixels(shot.Pix)
			})
			return shot
		}
		base := capture(false, false)
		for _, tc := range []struct {
			name           string
			hover, blocked bool
		}{{"normal", false, false}, {"hover", true, false}, {"modal", true, true}} {
			t.Run(fmt.Sprintf("%dx%d/%s", res[0], res[1], tc.name), func(t *testing.T) {
				got := capture(tc.hover, tc.blocked)
				edges := 0
				for y := 0; y < got.Bounds().Dy(); y++ {
					for x := 0; x < got.Bounds().Dx(); x++ {
						a, b := base.RGBAAt(x, y), got.RGBAAt(x, y)
						if !tc.hover || tc.blocked || a.A == 255 {
							// GPU filtering/blending can round an 8-bit channel by one.
							if math.Abs(float64(a.R)-float64(b.R)) > 1 || math.Abs(float64(a.G)-float64(b.G)) > 1 || math.Abs(float64(a.B)-float64(b.B)) > 1 || math.Abs(float64(a.A)-float64(b.A)) > 1 {
								t.Fatalf("base artwork changed at %d,%d: %v -> %v", x, y, a, b)
							}
							continue
						}
						if a.A != 0 || b.A == 0 {
							continue
						}
						edges++
						if math.Abs(float64(b.R)-float64(b.G)) > 1 || math.Abs(float64(b.G)-float64(b.B)) > 1 {
							t.Fatalf("non-white edge at %d,%d: %v", x, y, b)
						}
						near := false
						for dy := -2; dy <= 2; dy++ {
							for dx := -2; dx <= 2; dx++ {
								near = near || base.RGBAAt(x+dx, y+dy).A > 0
							}
						}
						if !near {
							t.Fatalf("glow escaped silhouette at %d,%d", x, y)
						}
					}
				}
				if tc.hover && !tc.blocked && edges == 0 {
					t.Fatal("hover has no silhouette glow")
				}
			})
		}
	}
}

// Extracting the shared silhouette compositor must not recolor or resample the
// existing world billboard glow. Compare against its previous draw contract.
func TestDebugSim_WorldSpriteGlowParity(t *testing.T) {
	requireStandeeGPU(t)
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	sprite := g.sprites.GetSprite(campHUDSprite)
	for _, size := range []int{32, 128, 192} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var a, b []byte
			runOnDrawFrame(func(_ *ebiten.Image) {
				actual := ebiten.NewImage(256, 256)
				defer actual.Deallocate()
				sx, sy := float64(size)/float64(sprite.Bounds().Dx()), float64(size)/float64(sprite.Bounds().Dy())
				r.drawSpriteEdgeGlow(actual, sprite, 20, 20, sx, sy, size)
				a, b = make([]byte, 256*256*4), make([]byte, 256*256*4)
				actual.ReadPixels(a)
				// Reuse the same target so nearest-neighbor sample ties cannot
				// differ with destination atlas placement at fractional scales.
				actual.Clear()
				offset := max(2, size/40)
				for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
					op := r.scaledWorldSpriteOpts(sx, sy)
					op.GeoM.Translate(float64(20+d[0]*offset), float64(20+d[1]*offset))
					op.ColorScale.Scale(1, 0.85, 0.45, 0.10)
					op.Blend = additiveGlowBlend
					actual.DrawImage(sprite, op)
				}
				actual.ReadPixels(b)
			})
			for i := range a {
				if math.Abs(float64(a[i])-float64(b[i])) > 1 {
					t.Fatalf("world billboard glow changed at channel %d: %d -> %d", i, b[i], a[i])
				}
			}
		})
	}
}

// Case table: common viewport sizes x visible/hidden party HUD x reveal/hold/
// dissolve. Every phase uses the real compositor. Repeat Draw must be stable;
// resizing must replace its bounded surface, and cancellation must discard it.
func TestDebugSim_CampViewportDissolve(t *testing.T) {
	requireStandeeGPU(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.beginCampRest()
	g.campRest.sprite = "camp_forest"
	for _, res := range campHUDResolutions {
		for _, hud := range []bool{true, false} {
			t.Run(fmt.Sprintf("%dx%d/HUD=%v", res[0], res[1], hud), func(t *testing.T) {
				g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = res[0], res[1]
				g.showPartyStats = hud
				s := g.campRest
				w, h, bottom := res[0], res[1], gameplayViewportBottom(g)
				capture := func(elapsed int) []byte {
					s.elapsed = elapsed
					pixels := make([]byte, w*h*4)
					runOnDrawFrame(func(_ *ebiten.Image) {
						screen := ebiten.NewImage(w, h)
						defer screen.Deallocate()
						screen.Fill(color.RGBA{19, 29, 41, 255})
						g.gameLoop.ui.drawCampRest(screen)
						screen.ReadPixels(pixels)
					})
					return pixels
				}
				base, full := capture(0), capture(s.fadeIn)
				if s.surface.Bounds().Size() != image.Pt(w, bottom) {
					t.Fatal("camp surface did not follow the resized HUD boundary")
				}
				for _, elapsed := range []int{s.fadeIn / 2, s.fadeIn + s.hold + s.fadeOut/2, s.fadeIn + s.hold + s.fadeOut} {
					part, again := capture(elapsed), capture(elapsed)
					if !bytes.Equal(part, again) {
						t.Fatal("pixel mask changes without Update")
					}
					if !bytes.Equal(part[bottom*w*4:], base[bottom*w*4:]) {
						t.Fatal("camp altered the party HUD")
					}
					if elapsed == s.fadeIn+s.hold+s.fadeOut {
						if !bytes.Equal(part, base) {
							t.Fatal("dissolve did not return to the unchanged world")
						}
						continue
					}
					visible, hidden := 0, 0
					for y := 4; y < bottom; y += 8 {
						for x := 4; x < w; x += 8 {
							i := (y*w + x) * 4
							if bytes.Equal(part[i:i+4], base[i:i+4]) {
								hidden++
							}
							if bytes.Equal(part[i:i+4], full[i:i+4]) {
								visible++
							}
						}
					}
					if visible < 10 || hidden < 10 {
						t.Fatal("transition is a flat fade, not a pixel dissolve")
					}
					// A cluster interior is locally coherent. Independent random
					// pixel noise would change state along about half these edges.
					changes, edges := 0, 0
					for y := 4; y < bottom; y += 8 {
						for x := 4; x+8 < w; x += 8 {
							a, b := (y*w+x)*4, (y*w+x+8)*4
							left := bytes.Equal(part[a:a+4], base[a:a+4])
							right := bytes.Equal(part[b:b+4], base[b:b+4])
							edges++
							if left != right {
								changes++
							}
						}
					}
					if changes*5 > edges {
						t.Fatal("dissolve scatters pixels instead of spreading in clusters")
					}

				}
				// No letterbox or dimmed strips may remain along any viewport edge.
				for _, p := range []image.Point{{0, 0}, {w - 1, 0}, {0, bottom - 1}, {w - 1, bottom - 1}, {w / 2, 0}, {0, bottom / 2}} {
					i := (p.Y*w + p.X) * 4
					if bytes.Equal(full[i:i+4], base[i:i+4]) {
						t.Fatalf("uncovered viewport edge at %v", p)
					}
				}
			})
		}
	}
	g.cancelCampPresentation()
	if g.campRest != nil {
		t.Fatal("cancel retained a camp render surface")
	}
}

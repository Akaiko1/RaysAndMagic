//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

func TestDebugSim_AchievementBannerIconMatchesPlate(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	h := newDisplayedModalHarness(t, 1024, 768)
	attachTestProfile(t, h.g)
	t.Chdir("../..")
	g := h.g
	for _, width := range []int{800, 1024, 1280, 1366, 1920, 2560} {
		for _, def := range config.GetAchievements() {
			for _, phase := range []struct {
				name  string
				frame int
			}{
				{"entering", g.bannerInFrames() / 2},
				{"holding", g.bannerInFrames() + 1},
				{"leaving", g.bannerInFrames() + g.bannerHoldFrames(bannerAchievement) + g.bannerOutFrames()/2},
			} {
				t.Run(fmt.Sprintf("%d/%s/%s", width, def.Key, phase.name), func(t *testing.T) {
					var problem string
					runOnDrawFrame(func(_ *ebiten.Image) {
						g.config.Display.ScreenWidth = width
						text := "Achievement unlocked - " + def.Name
						g.screenBannerQueue = []screenBanner{{text: text, icon: def.Icon, kind: bannerAchievement, frame: phase.frame}}
						alpha, offset := g.screenBannerAnim(phase.frame, g.bannerHoldFrames(bannerAchievement))
						geo := screenBannerLayoutWithIcon(width, text, offset, true)
						if geo.icon.h != geo.plateH || geo.icon.w != geo.icon.h || geo.icon.y != geo.plateY {
							problem = "icon must be square and match both plate edges"
							return
						}
						if geo.icon.x < bannerCornerReservePx || geo.plateX+geo.plateW > width-bannerCornerReservePx {
							problem = "icon and text group overlaps corner readouts"
							return
						}
						if delta := geo.icon.x + geo.plateX + geo.plateW - width; delta < -1 || delta > 1 {
							problem = "complete icon and text group is not centered"
							return
						}
						dst := ebiten.NewImage(width, 128)
						defer dst.Deallocate()
						h.ui.drawScreenBanner(dst)
						expected := ebiten.NewImage(width, 128)
						defer expected.Deallocate()
						style := &ebiten.DrawImageOptions{}
						style.ColorScale.ScaleAlpha(float32(alpha))
						graphics.DrawImageScaled(expected, g.sprites.GetSprite(def.Icon), float64(geo.icon.x), float64(geo.icon.y), float64(geo.plateH), float64(geo.plateH), style)
						want := make([]byte, geo.plateH*geo.plateH*4)
						expected.SubImage(image.Rect(geo.icon.x, geo.icon.y, geo.icon.x+geo.plateH, geo.icon.y+geo.plateH)).(*ebiten.Image).ReadPixels(want)
						// Shipped icons are opaque squares. Their rendered alpha bounds
						// measure height and fading without depending on texture-sampling
						// roundoff from the GPU atlas's placement of individual texels.
						got := make([]byte, geo.plateH*geo.plateH*4)
						dst.SubImage(image.Rect(geo.icon.x, geo.icon.y, geo.icon.x+geo.plateH, geo.icon.y+geo.plateH)).(*ebiten.Image).ReadPixels(got)
						for i, v := range got {
							delta := int(v) - int(want[i])
							if delta < -2 || delta > 2 {
								problem = "banner bypassed common filtered resize"
								return
							}
						}
						wantAlpha := int(math.Round(alpha * 255))
						for i := 3; i < len(got); i += 4 {
							delta := int(got[i]) - wantAlpha
							if delta < -1 || delta > 1 {
								problem = fmt.Sprintf("icon must fill the plate height: pixel %d alpha %d, want %d", i/4, got[i], wantAlpha)
								return
							}
						}

					})
					if problem != "" {
						t.Fatal(problem)
					}
				})
			}
		}
		for _, icon := range []string{"", "missing_achievement_icon"} {
			t.Run(fmt.Sprintf("%d/no-art/%s", width, icon), func(t *testing.T) {
				var equal bool
				runOnDrawFrame(func(_ *ebiten.Image) {
					g.config.Display.ScreenWidth = width
					const text = "Achievement unlocked - First Blood"
					g.screenBannerQueue = []screenBanner{{text: text, icon: icon, kind: bannerAchievement, frame: g.bannerInFrames() + 1}}
					got, want := ebiten.NewImage(width, 128), ebiten.NewImage(width, 128)
					defer got.Deallocate()
					defer want.Deallocate()
					h.ui.drawScreenBanner(got)
					h.ui.drawScreenBannerContent(want, text, bannerAchievement, 1, 0)
					a, b := make([]byte, width*128*4), make([]byte, width*128*4)
					got.ReadPixels(a)
					want.ReadPixels(b)
					equal = bytes.Equal(a, b)
				})
				if !equal {
					t.Fatal("missing artwork changed the original text-only banner")
				}
			})
		}
	}
}

func TestDebugSim_AchievementBannerGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	g, renderer := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.switchToMap("forest")
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	out := os.Getenv("RAM_BANNER_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{800, 680}, {1280, 720}, {1920, 1080}} {
		var drawErr error
		runOnDrawFrame(func(_ *ebiten.Image) {
			w, h := g.gameLoop.Layout(size[0], size[1])
			dst := ebiten.NewImage(w, h)
			defer dst.Deallocate()
			g.screenBannerQueue = []screenBanner{{text: "Achievement unlocked - First Blood", icon: "icon_achievement_first_blood", kind: bannerAchievement, frame: g.bannerInFrames() + 1}}
			renderer.RenderFirstPersonView(dst)
			g.gameLoop.ui.Draw(dst)
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("achievement-popup-%dx%d.png", size[0], size[1])))
			if err != nil {
				drawErr = err
				return
			}
			defer f.Close()
			drawErr = png.Encode(f, dst)
		})
		if drawErr != nil {
			t.Fatal(drawErr)
		}
	}
}

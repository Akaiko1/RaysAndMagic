//go:build debug

package game

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Read actual Scene pixels on the live graphics driver. Pin only world time so
// unrelated scenery cannot make a frozen Card overlay look animated.
func TestDebugSim_CardPreviewsAnimate(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	t.Chdir("../..")
	for _, it := range p.Items() {
		if it.Kind != FxCard {
			continue
		}
		t.Run(it.Key, func(t *testing.T) {
			p.Select(it)
			capture := func() []byte {
				var pixels []byte
				runOnDrawFrame(func(*ebiten.Image) {
					before := p.g.uiFrameCount
					worldTime := p.g.frameCount
					p.g.frameCount = 0
					defer func() { p.g.frameCount = worldTime }()
					scene := p.Scene()
					w, h := scene.Bounds().Dx(), scene.Bounds().Dy()
					card := scene.SubImage(image.Rect((w-220)/2, (h-300)/2, (w+220)/2, (h+300)/2)).(*ebiten.Image)
					pixels = make([]byte, 220*300*4)
					card.ReadPixels(pixels)
					if p.g.uiFrameCount != before {
						t.Error("Scene advanced presentation time")
					}
				})
				return pixels
			}
			for _, cycle := range []string{"select", "replay", "reselect"} {
				if cycle == "replay" {
					for p.tick != 0 {
						p.Step()
					}
				}
				if cycle == "reselect" {
					p.Select(it)
				}
				before := capture()
				if again := capture(); !bytes.Equal(before, again) {
					t.Errorf("%s: Draw changed a frame without Step", cycle)
				}
				for i := 0; i < 6; i++ {
					p.Step()
				}
				after := capture()
				if bytes.Equal(before, after) {
					t.Errorf("%s: %s Card preview stayed static after six Steps", cycle, it.Key)
				}
			}
		})
	}
}

func TestDebugSim_ImpassableAuraPreviewVisibleAndAnimated(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	t.Chdir("../..")
	found := false
	for _, it := range p.Items() {
		if it.Kind == FxTile && it.Label == "Impassable aura" {
			p.Select(it)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no authored aura exhibit")
	}
	var previous []byte
	for _, tick := range []int{0, 12} {
		for p.tick < tick {
			p.Step()
		}
		runOnDrawFrame(func(*ebiten.Image) {
			scene := p.Scene()
			if dir := os.Getenv("RAM_FX_PREVIEW_QA_DIR"); dir != "" && tick == 0 {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Error(err)
				} else if f, err := os.Create(filepath.Join(dir, "impassable_aura.png")); err != nil {
					t.Error(err)
				} else {
					if err := png.Encode(f, scene); err != nil {
						t.Error(err)
					}
					f.Close()
				}
			}
			enabled := make([]byte, scene.Bounds().Dx()*scene.Bounds().Dy()*4)
			scene.ReadPixels(enabled)
			p.g.config.Graphics.ImpassableAura.Enabled = false
			disabled := make([]byte, len(enabled))
			p.Scene().ReadPixels(disabled)
			p.g.config.Graphics.ImpassableAura.Enabled = true
			if bytes.Equal(enabled, disabled) {
				t.Error("Scene shows no aura despite an enabled authored exhibit")
			}
			// Compare only the aura's real draw output so scenery cannot mask a freeze.
			layer := ebiten.NewImage(scene.Bounds().Dx(), scene.Bounds().Dy())
			defer layer.Deallocate()
			p.g.gameLoop.renderer.drawImpassableTileAura(layer)
			pixels := make([]byte, len(enabled))
			layer.ReadPixels(pixels)
			visible := false
			for i := 3; i < len(pixels); i += 4 {
				if pixels[i] > 0 {
					visible = true
					break
				}
			}
			if !visible {
				t.Error("aura rendered no visible particles")
			}
			if previous != nil && bytes.Equal(previous, pixels) {
				t.Error("aura remained static across Steps")
			}
			previous = pixels
		})
	}
}

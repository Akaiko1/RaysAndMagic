//go:build debug

package game

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"ugataima/internal/storage"
)

// The same catalog/entry point as the launchers, through production Update and
// Draw. Set RAM_TEST_SCENARIO and RAM_SCENARIO_PREVIEW to capture any fixture.
func TestScenarioGameplayPreview(t *testing.T) {
	key, out := os.Getenv("RAM_TEST_SCENARIO"), os.Getenv("RAM_SCENARIO_PREVIEW")
	if key == "" || out == "" {
		t.Skip("set RAM_TEST_SCENARIO and RAM_SCENARIO_PREVIEW")
	}
	requireStandeeGPU(t)
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", key); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{1920, 1080}, {1024, 768}} {
		w, h := g.gameLoop.Layout(size[0], size[1])
		shot := captureGameplayPreviewFrame(t, g, w, h)
		name := "start.png"
		if size[0] == 1024 {
			name = "resized.png"
		}
		f, err := os.Create(filepath.Join(out, name))
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

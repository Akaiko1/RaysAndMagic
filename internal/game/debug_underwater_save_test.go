//go:build debug

package game

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func TestDebugSim_SavedUnderwaterTransition(t *testing.T) {
	requireStandeeGPU(t)
	source := os.Getenv("RAM_PREVIEW_SAVE")
	if source == "" {
		t.Skip("set RAM_PREVIEW_SAVE")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	storage.SetDataRootForTesting(root)
	defer storage.SetDataRootForTesting("")
	path := filepath.Join(root, "input.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if content := os.Getenv("RAM_PREVIEW_CONTENT"); content != "" {
		for _, name := range []string{"assets", "config.yaml"} {
			if err := os.Symlink(filepath.Join(content, name), filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
		}
		nested := filepath.Join(root, "internal", "game")
		if err := os.MkdirAll(nested, 0700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(nested)
	}
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	old := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = old })
	if err := config.LoadEcology("assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := g.LoadGameFromFile(path); err != nil {
		t.Fatal(err)
	}
	w, h := g.gameLoop.Layout(1280, 720)
	captureGameplayPreviewFrame(t, g, w, h)
	t.Log("loaded saved scene")
	runOnDrawFrame(func(_ *ebiten.Image) {
		g.sprites.SetDeferredResourceHandler(g.gameLoop.deferGameplayResource)
		defer g.sprites.SetDeferredResourceHandler(nil)
		def, err := spells.GetSpellDefinitionByID("water_breathing")
		if err != nil {
			t.Error(err)
			return
		}
		for i, caster := range g.party.Members {
			g.selectedChar = i
			if g.combat.castPlayerSpell("water_breathing", def, caster, true) {
				break
			}
		}
		if !g.waterBreathingActive {
			t.Error("buff not active")
			return
		}
		ts := float64(g.config.GetTileSize())
		best := math.Inf(1)
		var px, py float64
		for y, row := range g.world.Tiles {
			for x, tile := range row {
				if tile == world.TileDeepWater {
					dx, dy := (float64(x)+.5)*ts, (float64(y)+.5)*ts
					dist := math.Hypot(dx-g.camera.X, dy-g.camera.Y)
					if dist < best {
						best, px, py = dist, dx, dy
					}
				}
			}
		}
		if math.IsInf(best, 1) {
			t.Error("no deep water")
			return
		}
		t.Logf("enter deep water at %.0f %.0f distance %.0f", px, py, best)
		g.world.SetWaterBreathingActive(true)
		// Fish may appear while the player approaches the shore. Force this
		// transient state so the autosave regression is deterministic.
		fish := monster.NewMonster3DFromConfig(px, py, "common_carp", g.config)
		g.world.Monsters = append([]*monster.Monster3D{fish}, g.world.Monsters...)
		g.gameLoop.inputHandler.movePlayer(px-g.camera.X, py-g.camera.Y)
		if world.GlobalWorldManager.CurrentMapKey != "water" {
			t.Error("did not enter water map")
		}
	})
	if t.Failed() {
		return
	}
	t.Log("transition committed")
	captureGameplayPreviewFrame(t, g, w, h)
	t.Log("rendered underwater scene")
	if err := g.LoadGameFromFile(saveRowPath(0)); err != nil {
		t.Fatal(err)
	}
	captureGameplayPreviewFrame(t, g, w, h)
	t.Log("reloaded the underwater autosave")
}

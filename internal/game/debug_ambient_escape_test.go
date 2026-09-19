//go:build debug

package game

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/storage"
)

// Diagnose an actual save through the production loader without writing to it.
func TestDebugSim_AmbientSavedEscape(t *testing.T) {
	requireStandeeGPU(t)
	source := os.Getenv("RAM_AMBIENT_SAVE")
	if source == "" {
		t.Skip("set RAM_AMBIENT_SAVE to a private save fixture")
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
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	if err := g.LoadGameFromFile(path); err != nil {
		t.Fatal(err)
	}
	var animal *monster.Monster3D
	best := math.Inf(1)
	for _, m := range g.world.Monsters {
		if d := Distance(m.X, m.Y, g.camera.X, g.camera.Y); m.Key == "fennec" && m.IsAlive() && d < best {
			best, animal = d, m
		}
	}
	if animal == nil {
		t.Fatal("no living fennec in saved map")
	}
	changes, previous := 0, animal.AmbientFlee
	safe, walked := false, 0.0
	unsafeChanges := 0
	for i := 0; i < g.config.GetTPS()*20; i++ {
		x, y := animal.X, animal.Y
		stepLemur(g, animal, false)
		if safe {
			walked += Distance(x, y, animal.X, animal.Y)
			clearance := animal.AlertRadius + g.config.GetTileSize()
			if animal.Threat.DetourClearance > 0 {
				clearance = min(clearance, animal.Threat.DetourClearance)
			}
			if Distance(animal.X, animal.Y, g.camera.X, g.camera.Y)+1e-6 < clearance {
				t.Fatal("saved wildlife returned inside the safe clearance")
			}
		} else if Distance(animal.X, animal.Y, g.camera.X, g.camera.Y) >= animal.AlertRadius+g.config.GetTileSize() {
			safe = true
		}
		if previous != animal.AmbientFlee {
			changes++
			if Distance(animal.X, animal.Y, g.camera.X, g.camera.Y) < animal.AlertRadius+g.config.GetTileSize() {
				unsafeChanges++
			}
			previous = animal.AmbientFlee
			if changes <= 12 {
				t.Logf("tick %d flee=%v distance=%.3f tiles", i, previous, Distance(animal.X, animal.Y, g.camera.X, g.camera.Y)/g.config.GetTileSize())
			}
		}
	}
	t.Logf("flee transitions=%d, final distance=%.3f tiles", changes, Distance(animal.X, animal.Y, g.camera.X, g.camera.Y)/g.config.GetTileSize())
	if unsafeChanges > 3 {
		t.Fatal("saved wildlife repeatedly alternated escape and approach")
	}
	t.Logf("walked after escape=%.3f tiles", walked/g.config.GetTileSize())
	if !safe || walked < g.config.GetTileSize() {
		t.Fatal("saved wildlife did not continue roaming after escape")
	}
}

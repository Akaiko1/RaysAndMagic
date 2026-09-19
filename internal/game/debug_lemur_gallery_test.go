//go:build debug

package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Capture the production renderer against the authored jungle, including trees.
func TestDebugSim_LemurGallery(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	if err := g.switchToMap("deep_jungle"); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("RAM_LEMUR_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	tile := g.config.GetTileSize()
	point := func(x, y float64) (float64, float64) {
		return world.GlobalWorldManager.ProjectWorldPos("deep_jungle", x*tile, y*tile)
	}
	px, py := point(18.5, 8.5)
	tx, ty := point(21.5, 4.5)
	g.setPartyPosition(px, py)
	g.snapFacing(math.Atan2(ty-py, tx-px))
	installFakePointer(t).moveTo(0, 0)
	g.menuOpen, g.turnBasedMode, g.currentTurn = false, true, 0
	for _, m := range g.world.Monsters {
		m.Pacified = true
	}
	var lemurs []*monster.Monster3D
	for i, key := range []string{"ring_tailed_lemur", "red_ruffed_lemur"} {
		x, y := point(20+float64(i), 5.5)
		m := monster.NewMonster3DFromConfig(x, y, key, g.config)
		m.Direction = math.Atan2(ty-y, tx-x)
		m.RootFramesRemaining, m.RootTurnsRemaining = 100000, 100000
		g.addEcologyActor(g.world, m)
		lemurs = append(lemurs, m)
	}
	for _, phase := range []string{"ground", "climbing", "perched", "jumping", "descending"} {
		for i, m := range lemurs {
			x, y := point(20+float64(i), 5.5)
			m.X, m.Y, m.Arbor = x, y, monster.ArborealState{}
			if phase != "ground" {
				m.X, m.Y = point(21.2+float64(i)*.6, 4.8)
				h := m.Arboreal.HeightTiles
				if phase == "climbing" || phase == "descending" {
					h *= .5
				}
				if phase == "jumping" {
					h += .35
				}
				m.Arbor = monster.ArborealState{Phase: phase, Progress: .5, Height: h, FromX: x, FromY: y, ToX: tx, ToY: ty, GroundX: x, GroundY: y}
			}
		}
		for _, res := range [][2]int{{800, 600}, {1920, 1080}} {
			w, h := g.gameLoop.Layout(res[0], res[1])
			shot := captureGameplayPreviewFrame(t, g, w, h)
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("game_%s_%dx%d.png", phase, w, h)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, shot)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("capture: %v %v", err, closeErr)
			}
		}
	}
}

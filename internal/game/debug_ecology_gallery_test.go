//go:build debug

package game

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Export native gameplay frames through the production loader and renderer.
func TestDebugSim_EcologyGallery(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	old := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = old })
	if err := config.LoadEcology("assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := g.switchToMap("desert"); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("RAM_ECOLOGY_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	tile := g.config.GetTileSize()
	px, py := world.GlobalWorldManager.ProjectWorldPos("desert", 28.5*tile, 18.5*tile)
	g.setPartyPosition(px, py)
	g.snapFacing(0)
	fp := installFakePointer(t)
	fp.moveTo(0, 0)
	g.menuOpen = false
	g.turnBasedMode = true
	g.currentTurn = 0
	for i, key := range []string{"desert_rabbit", "fennec", "desert_caravan"} {
		m := monster.NewMonster3DFromConfig(px+4*tile, py+float64(i-1)*1.2*tile, key, g.config)
		g.addEcologyActor(g.world, m)
	}
	save := func(name string, w, h int) {
		shot := captureGameplayPreviewFrame(t, g, w, h)
		f, err := os.Create(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("capture: %v %v", err, closeErr)
		}
	}
	for _, res := range [][2]int{{800, 600}, {1920, 1080}} {
		w, h := g.gameLoop.Layout(res[0], res[1])
		save(fmt.Sprintf("wildlife_%dx%d.png", w, h), w, h)
	}
	if err := g.switchToMap("nomad_city"); err != nil {
		t.Fatal(err)
	}
	g.ecology.Unlocked = true
	g.ecology.Stock = map[string]int{}
	for i, key := range config.CaravanTradePool()[:12] {
		g.ecology.Stock[key] = i + 1
	}
	g.syncCaravanStock()
	for _, npc := range g.world.NPCs {
		if npc.Key == config.GlobalEcology.Caravan.Merchant {
			g.beginConversation(npc)
			break
		}
	}
	if g.dialogNPC == nil {
		t.Fatal("missing Safiya")
	}
	g.switchDialogTab(1)
	if g.npcDialogKindFor(g.dialogNPC) != dialogKindArenaGladiator {
		t.Fatal("trade goods unreachable")
	}
	for _, res := range [][2]int{{800, 600}, {1920, 1080}} {
		w, h := g.gameLoop.Layout(res[0], res[1])
		save(fmt.Sprintf("trade_goods_%dx%d.png", w, h), w, h)
	}
}

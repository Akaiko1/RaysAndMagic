//go:build debug

package game

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestDebugSim_DialogueAndUsageGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	out := os.Getenv("RAM_DIALOGUE_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-dialogue-usage")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	newTestCombatSystemWithConfig(t)
	h := newDisplayedModalHarness(t, 1280, 720)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	ronin, err := character.CreateNPCFromConfig("castle_oldman", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	fp := installFakePointer(t)
	minW, minH := MinimumWindowSize()
	for _, size := range [][2]int{{minW, minH}, {1280, 720}, {1920, 1080}} {
		for _, scene := range []string{"ronin-nested", "ronin-concluded", "medusa_card", "lich_phylactery", "clock_hand", "red_dragon_scale"} {
			t.Run(fmt.Sprintf("%s/%dx%d", scene, size[0], size[1]), func(t *testing.T) {
				g := h.g
				g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = size[0], size[1]
				g.closeConversation()
				g.menuOpen = false
				fp.x, fp.y = 1, 1
				switch scene {
				case "ronin-nested", "ronin-concluded":
					ronin.Visited = scene == "ronin-concluded"
					g.beginConversation(ronin)
					if !ronin.Visited {
						NewInputHandler(g).executeEncounterChoice()
					}
				default:
					g.party.Inventory = []items.Item{items.CreateItemFromYAML(scene)}
					g.menuOpen, g.currentTab = true, TabInventory
					g.selectedChar = 0
					layout := computeInventoryContentLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content)
					x, y, w, height := scaleInventorySourceRect(layout.grid.x, layout.grid.y, layout.grid.w, layout.grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
					fp.x, fp.y = x+w/2, y+height/2
				}
				var problem error
				runOnDrawFrame(func(_ *ebiten.Image) {
					screen := ebiten.NewImage(size[0], size[1])
					defer screen.Deallocate()
					h.ui.Draw(screen)
					f, err := os.Create(filepath.Join(out, fmt.Sprintf("%s-%dx%d.png", scene, size[0], size[1])))
					if err != nil {
						problem = err
						return
					}
					defer f.Close()
					problem = png.Encode(f, screen)
				})
				if problem != nil {
					t.Fatal(problem)
				}
			})
		}
	}
}

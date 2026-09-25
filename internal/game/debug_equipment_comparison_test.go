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

func TestDebugSim_EquipmentComparisonGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	out := os.Getenv("RAM_EQUIPMENT_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-equipment-comparison")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	newTestCombatSystemWithConfig(t)
	h := newDisplayedModalHarness(t, 1280, 720)
	t.Chdir("../..")
	fp := installFakePointer(t)
	minW, minH := MinimumWindowSize()
	for _, tc := range []struct {
		name, key, old string
		slot           items.EquipSlot
		weapon         bool
	}{
		{"armor-scaling", "onryo_lamellar", "leather_armor", items.SlotArmor, false},
		{"accessory", "belt_of_strength", "belt_of_speed", items.SlotBelt, false},
		{"shield-effects", "broodscale_aegis", "parma_shield", items.SlotOffHand, false},
		{"weapon-set", "gold_sword", "iron_sword", items.SlotMainHand, true},
		{"weapon-effects", "kage_kunai", "hunting_bow", items.SlotMainHand, true},
	} {
		for _, size := range [][2]int{{minW, minH}, {1280, 720}, {1920, 1080}} {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size[0], size[1]), func(t *testing.T) {
				g := h.g
				g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = size[0], size[1]
				ch := g.party.Members[0]
				comparisonTestHero(ch)
				delete(ch.Skills, character.SkillDualWielding)
				makeItem := items.CreateItemFromYAML
				if tc.weapon {
					makeItem = items.CreateWeaponFromYAML
				}
				ch.Equipment[tc.slot] = makeItem(tc.old)
				if tc.name == "weapon-set" {
					ch.Equipment[items.SlotArmor] = items.CreateItemFromYAML("golden_armor")
				}
				ch.RecalculateMaxStatsKeepingCurrent(g.config)
				g.party.Inventory = []items.Item{makeItem(tc.key)}
				g.menuOpen, g.currentTab = true, TabInventory
				g.selectedChar = 0
				layout := computeInventoryContentLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content)
				x, y, w, height := scaleInventorySourceRect(layout.grid.x, layout.grid.y, layout.grid.w, layout.grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
				fp.x, fp.y = x+w/2, y+height/2
				var problem error
				runOnDrawFrame(func(_ *ebiten.Image) {
					screen := ebiten.NewImage(size[0], size[1])
					defer screen.Deallocate()
					h.ui.Draw(screen)
					f, err := os.Create(filepath.Join(out, fmt.Sprintf("%s-%dx%d.png", tc.name, size[0], size[1])))
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
				if len(h.ui.tooltipCompareLines) == 0 {
					t.Fatal("inventory hover failed to queue comparison")
				}
			})
		}
	}
}

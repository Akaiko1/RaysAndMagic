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
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
)

func TestDebugSim_AlchemyMaterialPicker(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			previous := config.GlobalIconFrames
			config.GlobalIconFrames = nil
			t.Cleanup(func() { config.GlobalIconFrames = previous })
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			t.Chdir("../..")
			if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
				t.Fatal(err)
			}
			g.sprites = graphics.NewSpriteManager()
			g.showPartyStats = true
			g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
			g.selectedChar, g.currentTab, g.menuOpen = 0, TabSpellbook, true
			fp := installFakePointer(t)
			fp.moveTo(0, 0)
			r := &config.GlobalAlchemy.Recipes[0]
			capture := func(name string) {
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.ui.Draw(h.screen)
					path := filepath.Join(os.TempDir(), fmt.Sprintf("alchemy_materials_%d_%s.png", size[0], name))
					f, err := os.Create(path)
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err = png.Encode(f, snapshotUIImage(h.screen)); err != nil {
						t.Error(err)
					}
					t.Log(path)
				})
			}
			for _, state := range []string{"mixed", "bottom", "empty", "large_stock", "many_ingredients", "hover"} {
				g.party.Inventory = nil
				g.alchemyBatches = 1
				g.brewAnimation = nil
				selected := character.AlchemySelection{{"dawnleaf": true}, {"carp_scale": true, "koi_scale": true, "wolf_pelt": true}}
				for _, entry := range []struct {
					key string
					n   int
				}{{"dawnleaf", 12}, {"carp_scale", 1}, {"koi_scale", 1}, {"wolf_pelt", 1}} {
					it, _ := items.TryCreateItemFromYAML(entry.key)
					it.Quantity = entry.n
					g.party.AddItem(it)
				}
				if state == "empty" {
					g.party.Inventory = nil
					selected[1] = map[string]bool{}
				}
				if state == "large_stock" {
					for key := range r.Ingredients[1].Materials() {
						it, _ := items.TryCreateItemFromYAML(key)
						it.Quantity = 9999
						g.party.AddItem(it)
						selected[1][key] = true
					}
					g.alchemyBatches = 12
				}
				g.alchemy.Selections = map[string]character.AlchemySelection{r.Key: selected}
				h.ui.alchemyScroll = map[string]int{}
				if state == "hover" {
					l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, true)
					a := makeAlchemyMaterialLayout(l, r)
					fp.moveTo(a.cells[0].rect.x+15, a.cells[0].rect.y+25)
				}
				if state == "bottom" {
					h.ui.alchemyScroll[r.Key] = 100000
				}
				if state == "many_ingredients" {
					g.party.Inventory = nil
					batches := len(r.Ingredients[1].Materials())
					g.alchemyBatches = batches
					base, _ := items.TryCreateItemFromYAML("dawnleaf")
					base.Quantity = batches
					g.party.AddItem(base)
					for _, category := range config.GlobalAlchemy.Categories {
						for _, key := range category.Items {
							if n, ok := r.Ingredients[1].Materials()[key]; ok {
								it, _ := items.TryCreateItemFromYAML(key)
								it.Quantity = n
								g.party.AddItem(it)
								selected[1][key] = true
							}
						}
					}
					if !g.brewSelectedRecipe() {
						t.Fatal(g.rareBookMessage)
					}
					g.brewAnimation.Frames = g.brewAnimation.Duration / 4
				}
				capture(state)
			}
		})
	}
}

//go:build debug

package game

import (
	"fmt"
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// The two production card renderers must produce the same icon decoration.
// Cells: selected x equipped x affordable, plus trap-only level locking.
// Persistence is N/A: each draw derives state from the existing character.
func TestDebugSim_BookSelectionParity(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{config: cfg, sprites: graphics.NewSpriteManager()}}
	actor := character.CreateCharacter("Reader", character.ClassSorcerer, cfg)
	spell := spells.SpellDefinition{Name: "Test", SpellPointsCost: 10}
	trap := &config.TrapDefinitionConfig{Name: "Test", Icon: spellTooltipIconName(spells.SpellID("firebolt")), Level: 1, SPCost: 10, Element: "fire"}
	const x, y, w, h, size = 20, 20, 162, 158, 100
	ix, iy := x+(w-size)/2, y+6
	for _, selected := range []bool{false, true} {
		for _, equipped := range []bool{false, true} {
			for _, affordable := range []bool{false, true} {
				t.Run(fmt.Sprintf("selected=%v/equipped=%v/affordable=%v", selected, equipped, affordable), func(t *testing.T) {
					var equal, quiet bool
					runOnDrawFrame(func(_ *ebiten.Image) {
						a, b := ebiten.NewImage(210, 200), ebiten.NewImage(210, 200)
						defer a.Deallocate()
						defer b.Deallocate()
						actor.SpellPoints = 0
						if affordable {
							actor.SpellPoints = 100
						}
						delete(actor.Equipment, items.SlotSpell)
						if equipped {
							actor.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "firebolt"}
						}
						ui.drawSpellbookSpellCard(a, x, y, w, h, size, spells.SpellID("firebolt"), spell, actor, character.MagicSchoolFire, selected)
						if equipped {
							actor.Equipment[items.SlotSpell] = items.Item{Type: items.ItemTrap, SpellEffect: "test_trap"}
						}
						ui.drawTrapCard(b, x, y, w, h, size, "test_trap", trap, actor, selected)
						pa, pb := snapshotUIImage(a), snapshotUIImage(b)
						equal, quiet = true, true
						// Ignore icon art and text; inspect the surrounding decoration.
						for sy := iy - 5; sy < iy+size; sy++ {
							for sx := ix - 5; sx < ix+size+5; sx++ {
								if sx >= ix && sx < ix+size && sy >= iy {
									continue
								}
								if pa.RGBAAt(sx, sy) != pb.RGBAAt(sx, sy) {
									equal = false
								}
							}
						}
						for sy := y; sy < y+h; sy++ {
							if pb.RGBAAt(x, sy).A != 0 || pb.RGBAAt(x+w-1, sy).A != 0 {
								quiet = false
							}
						}
					})
					if !equal {
						t.Fatal("spell and trap icon decoration diverged")
					}
					if !quiet {
						t.Fatal("trap selection added a whole-card frame")
					}
				})
			}
		}
	}
	for _, selected := range []bool{false, true} {
		t.Run(fmt.Sprintf("locked/selected=%v", selected), func(t *testing.T) {
			var edge color.RGBA
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(210, 200)
				defer dst.Deallocate()
				trap.Level = actor.Level + 1
				actor.SpellPoints = 100
				ui.drawTrapCard(dst, x, y, w, h, size, "test_trap", trap, actor, selected)
				pixels := snapshotUIImage(dst)
				edge = pixels.RGBAAt(ix-1, iy+size/2)
				if pixels.RGBAAt(x, y).A != 0 {
					t.Error("locked entry paints over parchment outside its icon")
				}
			})
			if edge != (color.RGBA{120, 38, 28, 255}) {
				t.Fatalf("locked edge lost: %v", edge)
			}
		})
	}
}

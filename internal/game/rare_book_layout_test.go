package game

import (
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func TestRareBookDisplayedPagesAndSources(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
			g.menuOpen = true
			g.currentTab = TabSpellbook
			g.selectedChar = 0
			l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, true)
			for page := 0; page < pageCount(len(config.GlobalAlchemy.Recipes), l.rows); page++ {
				for local := 0; local < l.rows; local++ {
					idx := page*l.rows + local
					if idx >= len(config.GlobalAlchemy.Recipes) {
						break
					}
					row := l.row(local)
					h.clicks(false, row.x+65, row.y+12, 1)
					if g.selectedRare != idx {
						t.Fatalf("recipe %d selected %d", idx, g.selectedRare)
					}
					recipe := &config.GlobalAlchemy.Recipes[idx]
					a := makeAlchemyMaterialLayout(l, recipe)
					// All and None change only optional materials; the base stays pinned.
					for _, all := range []bool{true, false} {
						x := a.toolbar.right() - 24
						if all {
							x -= 58
						}
						h.clicks(false, x, a.toolbar.y+10, 1)
						for gi, group := range recipe.Ingredients {
							for key := range group.Materials() {
								if g.alchemySelection(recipe)[gi][key] != (all || len(group.Materials()) == 1) {
									t.Fatalf("%s/%s: All/None selection", recipe.Key, key)
								}
							}
						}
					}
					cell := a.cells[0]
					h.clicks(false, cell.rect.x+10, cell.rect.y+25, 1)
					if !g.alchemySelection(recipe)[cell.group][cell.key] {
						t.Fatal("material click did not select")
					}
					h.clicks(false, cell.rect.x+10, cell.rect.y+25, 1)
					if g.alchemySelection(recipe)[cell.group][cell.key] {
						t.Fatal("material click did not deselect")
					}
				}
				if page+1 < pageCount(len(config.GlobalAlchemy.Recipes), l.rows) {
					h.clicks(false, l.pager.right()-16, l.pager.y+12, 1)
					if g.selectedRare != (page+1)*l.rows {
						t.Fatal("next page did not select its first recipe")
					}
				}
			}
			h.clicks(false, l.pager.x+16, l.pager.y+12, 1)
			if g.selectedRare/l.rows != pageCount(len(config.GlobalAlchemy.Recipes), l.rows)-2 {
				t.Fatalf("previous page failed: selected=%d rows=%d pages=%d", g.selectedRare, l.rows, pageCount(len(config.GlobalAlchemy.Recipes), l.rows))
			}
		})
	}
}

func TestRareBookLayoutSeparation(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		for _, alchemy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/alchemy=%v", size, alchemy), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				content := computeTabbedMenuLayout(size[0], gameplayViewportBottom(h.g)).content
				l := computeRareBookLayout(content, alchemy)
				if l.frame.w*2 != l.frame.h*3 {
					t.Fatal("artwork aspect ratio changed")
				}
				if l.header.y < l.inner.y || l.hint.bottom() > l.inner.bottom() || l.quick.x < l.inner.x || l.quick.right() > l.inner.right() {
					t.Fatal("title or footer overlaps the source decoration")
				}
				if l.header.bottom() > l.list.y || l.list.right() >= l.detail.x || l.detail.bottom() > l.quick.y-quickSlotTabLabelSpace || l.quick.bottom() > l.hint.y || l.hint.bottom() > content.bottom() {
					t.Fatal("book sections overlap")
				}
				for i := 0; i < l.rows; i++ {
					r := l.row(i)
					bottom := l.list.bottom()
					if alchemy {
						bottom = l.pager.y
					}
					if r.bottom() > bottom || r.x < l.list.x || r.right() > l.list.right() {
						t.Fatalf("row %d outside list", i)
					}
					if !alchemy && l.auto(i).bottom() > r.bottom() {
						t.Fatalf("checkbox %d outside row", i)
					}
				}
				if alchemy {
					for _, recipe := range config.GlobalAlchemy.Recipes {
						a := makeAlchemyMaterialLayout(l, &recipe)
						if a.title.bottom() > a.base.y || a.base.bottom() > a.toolbar.y || a.toolbar.bottom() > a.viewport.y || a.viewport.h < 130 || a.viewport.bottom() > a.controls.y || a.controls.bottom() > a.message.y || a.message.bottom() > l.detail.bottom() {
							t.Fatal("materials lack readable space or overlap fixed controls")
						}
					}
				}
				if l.message.bottom() > l.detail.bottom() {
					t.Fatal("message outside details")
				}
			})
		}
	}
}

func TestRareAlchemyCraftEverySource(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	for ri := range config.GlobalAlchemy.Recipes {
		recipe := &config.GlobalAlchemy.Recipes[ri]
		key := recipe.Key
		for ai, alt := range recipe.Ingredients[1].Alternatives {
			for _, source := range append(append([]string{}, alt.Items...), "mixed") {
				t.Run(fmt.Sprintf("%s/%d/%s", key, ai, source), func(t *testing.T) {
					g.party.Inventory = nil
					herb, _ := items.TryCreateItemFromYAML(recipe.Ingredients[0].Alternatives[0].Items[0])
					herb.Quantity = recipe.Ingredients[0].Alternatives[0].Count * 2
					g.party.AddItem(herb)
					for n := 0; n < alt.Count*2; n++ {
						itemKey := source
						if source == "mixed" {
							itemKey = alt.Items[n%len(alt.Items)]
						}
						it, _ := items.TryCreateItemFromYAML(itemKey)
						it.Quantity = 1
						g.party.AddItem(it)
					}
					choices := []int{0, ai}
					if got := g.party.MaxAlchemyBatches(recipe, choices); got != 2 {
						t.Fatalf("max=%d", got)
					}
					before := append([]items.Item(nil), g.party.Inventory...)
					if _, _, err := g.party.Brew(c, recipe, choices, 3); err == nil || !reflect.DeepEqual(before, g.party.Inventory) {
						t.Fatal("insufficient batch changed inventory")
					}
					n, _, err := g.party.Brew(c, recipe, choices, 2)
					want := 2 * character.AlchemyYield(c.SkillTier(character.SkillAlchemy), recipe.Family)
					if err != nil || n != want || len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != want {
						t.Fatalf("source failed: n=%d err=%v inventory=%v", n, err, g.party.Inventory)
					}
				})
			}
		}
	}
}

func TestRareBookTechniqueGestures(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		for _, level := range []int{1, 8} {
			t.Run(fmt.Sprintf("%v/level=%d", size, level), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				c := character.CreateCharacter("Kael", character.ClassWayfarer, g.config)
				c.Level = level
				g.party.Members[0] = c
				g.selectedChar = 0
				g.menuOpen = true
				g.currentTab = TabSpellbook
				l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, false)
				fp := installFakePointer(t)
				for i, d := range config.GlobalTechniques.Techniques {
					row := l.row(i)
					h.clicks(false, row.x+65, row.y+12, 1)
					if g.selectedRare != i {
						t.Fatalf("%s selection failed", d.Key)
					}
					if d.Automatic {
						r := l.auto(i)
						h.clicks(false, r.x+10, r.y+10, 1)
						if c.RareClass.Automatic[d.Key] != (level >= d.Level) {
							t.Fatalf("%s learned/locked checkbox", d.Key)
						}
						if g.selectedRare != i {
							t.Fatal("checkbox changed selected technique")
						}
					}
					if level < d.Level {
						continue
					}
					// Grab the bottom of the icon, below the checkbox's top edge.
					fp.moveTo(row.x+20, row.y+40)
					fp.press()
					h.pointerStep()
					if g.dragSrc != dragFromRareAction {
						t.Fatalf("%s icon did not arm drag", d.Key)
					}
					_, slots := quickSlotRects(l.quick.x, l.quick.y, l.quick.w)
					fp.hold()
					fp.moveTo(slots[0].Min.X+5, slots[0].Min.Y+5)
					h.pointerStep()
					fp.release()
					h.pointerStep()
					it, _ := config.TechniqueItem(d.Key)
					if c.QuickSlots[0] == nil || c.QuickSlots[0].Name != it.Name {
						t.Fatalf("%s icon drop did not bind", d.Key)
					}
				}
			})
		}
	}
}

func TestRareFlasksHaveAbundantMaterialRoutes(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		r := config.AlchemyRecipeByKey(key)
		required := []string{"carp_scale", "koi_scale", "rainbow_salmon_scale"}
		if key == "fire_flask" {
			required = []string{"rabbit_pelt", "fennec_pelt", "lemur_fur"}
		}
		for _, source := range required {
			found := false
			for _, alt := range r.Ingredients[1].Alternatives {
				for _, item := range alt.Items {
					if item == source {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("%s lacks abundant source %s", key, source)
			}
		}
	}
}

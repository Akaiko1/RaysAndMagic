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
	for _, size := range withInterfaceFrames(t, [][2]int{{1024, 768}, {1920, 1080}}) {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			g.showPartyStats = true
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
					name := rareBookRowParts(l.row(local)).name
					h.clicks(false, name.x+10, name.y+name.h/2, 1)
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

// Every part of both rare books stays inside its section and clear of its
// neighbours at every interface frame and font. A Pilgrim row keeps both of
// its lines, the autocast condition is always whole, and the description is
// whole on any book at least the size of the 1280x720 frame's; a smaller one
// clips it and hover shows the rest. Normal on a 1920x1080 screen keeps the
// authored (roomy) scheme of both books.
func TestRareBookLayoutSeparation(t *testing.T) {
	h := newDisplayedModalHarness(t, 1920, 1080)
	whole := computeRareBookLayout(computeTabbedMenuLayout(1280, gameplayViewportBottomWithPartyHUD(720)).content, false).detail
	kael := character.CreateCharacter("Kael", character.ClassWayfarer, h.g.config)
	inside := func(r, outer layoutRect) bool {
		return r.x >= outer.x && r.y >= outer.y && r.right() <= outer.right() && r.bottom() <= outer.bottom()
	}
	overlaps := func(a, b layoutRect) bool {
		return a.w > 0 && a.h > 0 && b.w > 0 && b.h > 0 && a.x < b.right() && b.x < a.right() && a.y < b.bottom() && b.y < a.bottom()
	}
	forEachUIFont(t, func(t *testing.T) {
		for _, size := range withInterfaceFrames(t, [][2]int{{1024, 768}, {1920, 1080}}) {
			content := computeTabbedMenuLayout(size[0], gameplayViewportBottomWithPartyHUD(size[1])).content
			for _, alchemy := range []bool{false, true} {
				l := computeRareBookLayout(content, alchemy)
				where := fmt.Sprintf("%v alchemy=%v", size, alchemy)
				authored := size == [2]int{1920, 1080}
				if l.frame.w*2 != l.frame.h*3 {
					t.Fatalf("%s: artwork aspect ratio changed", where)
				}
				if l.header.y < l.inner.y || l.hint.bottom() > l.inner.bottom() || l.quick.x < l.inner.x || l.quick.right() > l.inner.right() {
					t.Fatalf("%s: title or footer overlaps the source decoration", where)
				}
				if l.header.bottom() > l.list.y || l.list.right() >= l.detail.x || l.detail.bottom() > l.quick.y-quickSlotTabLabelSpace || l.quick.bottom() > l.hint.y || l.hint.bottom() > content.bottom() {
					t.Fatalf("%s: book sections overlap", where)
				}
				rows := l.list
				if alchemy {
					rows.h = l.pager.y - rows.y
				}
				if l.listTitle && l.rowsTop < l.list.y+rareBookListTitle {
					t.Fatalf("%s: rows cover the list heading", where)
				}
				for i := 0; i < l.rows; i++ {
					r := l.row(i)
					p := rareBookRowParts(r)
					if !inside(r, rows) || r.h < rareBookRowMinH || (i > 0 && r.y < l.row(i-1).bottom()) {
						t.Fatalf("%s: row %d %+v outside list %+v or too short", where, i, r, rows)
					}
					if !inside(p.icon, r) || !inside(p.name, r) || !inside(p.second, r) || !inside(p.subtitle, r) ||
						overlaps(p.name, p.second) || overlaps(p.name, p.subtitle) || overlaps(p.icon, p.name) || overlaps(p.icon, p.second) {
						t.Fatalf("%s: row %d parts %+v leave row %+v", where, i, p, r)
					}
					if authored && (r.h < rareBookRowRoomyH || l.rowGap != 6 || !l.listTitle) {
						t.Fatalf("%s: row %d left the authored scheme (%+v, gap %d)", where, i, r, l.rowGap)
					}
				}
				if alchemy {
					for _, recipe := range config.GlobalAlchemy.Recipes {
						a := makeAlchemyMaterialLayout(l, &recipe)
						// The viewport shows at least a category heading over one whole row of cells.
						if a.title.bottom() > a.base.y || a.base.bottom() > a.toolbar.y || a.toolbar.bottom() > a.viewport.y || a.viewport.h < alchemyHeadingPitch+alchemyCellH || a.viewport.bottom() > a.controls.y || a.controls.bottom() > a.message.y || a.message.bottom() > l.detail.bottom() {
							t.Fatalf("%s: materials lack readable space or overlap fixed controls (viewport %d)", where, a.viewport.h)
						}
						if authored && a.title.h != alchemyRoomySpacing.titleH {
							t.Fatalf("%s: materials left the authored scheme", where)
						}
					}
					continue
				}
				if l.rows != len(config.GlobalTechniques.Techniques) {
					t.Fatalf("%s: %d rows for %d techniques", where, l.rows, len(config.GlobalTechniques.Techniques))
				}
				for _, d := range config.GlobalTechniques.Techniques {
					trigger := ""
					if d.Automatic {
						trigger = d.Trigger
					}
					for _, message := range []string{"Select a technique. Hover its icon for details.", "Autocast is independent of your quick action.", "Learned automatically at level 6."} {
						p := makePilgrimDetailLayout(l.detail, techniqueMagnitude(kael, &d, 60), d.Description, trigger, message)
						if authored && !p.roomy {
							t.Fatalf("%s %s: details left the authored scheme", where, d.Key)
						}
						head := layoutRect{p.icon.x, p.icon.y, p.name.right() - p.icon.x, max(p.icon.bottom(), p.unlock.bottom()) - p.icon.y}
						parts := map[string]layoutRect{"head": head, "magnitude": p.magnitude, "description": p.desc, "autocast": p.auto, "buttons": p.actions, "note": p.message}
						for a, ra := range parts {
							if ra.h > 0 && !inside(ra, l.detail) {
								t.Fatalf("%s %s: %s %+v leaves the details %+v", where, d.Key, a, ra, l.detail)
							}
							for b, rb := range parts {
								if a < b && overlaps(ra, rb) {
									t.Fatalf("%s %s: %s %+v overlaps %s %+v", where, d.Key, a, ra, b, rb)
								}
							}
						}
						if d.Automatic && (!inside(p.trigger, p.auto) || p.trigger.h < len(wrapUIText(d.Trigger, p.trigger.w))*rareBookLineH) {
							t.Fatalf("%s %s: autocast condition cut: %+v in %+v", where, d.Key, p.trigger, p.auto)
						}
						if p.desc.h < rareBookLineH {
							t.Fatalf("%s %s: no line left for the description (%+v)", where, d.Key, p.desc)
						}
						if l.detail.w >= whole.w && l.detail.h >= whole.h && p.desc.h < p.descLines*rareBookLineH {
							t.Fatalf("%s %s: description shows %d of %d lines", where, d.Key, p.desc.h/rareBookLineH, p.descLines)
						}
					}
				}
			}
		}
	})
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
	for _, size := range withInterfaceFrames(t, [][2]int{{1024, 768}, {1920, 1080}}) {
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
				g.showPartyStats = true
				l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, false)
				fp := installFakePointer(t)
				for i, d := range config.GlobalTechniques.Techniques {
					parts := rareBookRowParts(l.row(i))
					icon, name := parts.icon, parts.name
					h.clicks(false, name.x+10, name.y+name.h/2, 1)
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
					fp.moveTo(icon.x+icon.w/2, icon.bottom()-3)
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

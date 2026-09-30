package game

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

type alchemyPreviewCache struct {
	recipe       *config.AlchemyRecipe
	signature    uint64
	batches, max int
	plan         map[int]int
	err          error
}

func (ui *UISystem) alchemyPlan(r *config.AlchemyRecipe, selected character.AlchemySelection, batches int) *alchemyPreviewCache {
	h := uint64(14695981039346656037)
	mix := func(v uint64) { h ^= v; h *= 1099511628211 }
	for _, it := range ui.game.party.CarriedItems() {
		for _, b := range []byte(it.Name) {
			mix(uint64(b))
		}
		mix(0)
		mix(uint64(it.Count()))
		mix(uint64(it.Attributes["value"]))
		mix(uint64(it.Type))
	}
	for gi, group := range r.Ingredients {
		for _, a := range group.Alternatives {
			mix(uint64(a.Count))
			for _, key := range a.Items {
				mix(uint64(boolInt(selected[gi][key])))
			}
		}
	}
	p := &ui.alchemyPreview
	if p.recipe != r || p.signature != h {
		*p = alchemyPreviewCache{recipe: r, signature: h, batches: -1, max: ui.game.party.MaxAlchemySelected(r, selected)}
	}
	if p.batches != batches {
		p.batches = batches
		p.plan, p.err = ui.game.party.PlanAlchemySelected(r, selected, batches)
	}
	return p
}

type alchemyMaterialCell struct {
	key          string
	group, count int
	rect         layoutRect
}
type alchemyMaterialHeading struct {
	label string
	rect  layoutRect
}
type alchemyMaterialLayout struct {
	title, base, toolbar, viewport, controls, maximum, brew, message layoutRect
	cells                                                            []alchemyMaterialCell
	headings                                                         []alchemyMaterialHeading
	height                                                           int
}

func makeAlchemyMaterialLayout(l rareBookLayout, r *config.AlchemyRecipe) alchemyMaterialLayout {
	x, w := l.detail.x+10, l.detail.w-20
	a := alchemyMaterialLayout{
		title:    layoutRect{x, l.detail.y + 8, w, 40},
		base:     layoutRect{x, l.detail.y + 52, w, 32},
		toolbar:  layoutRect{x, l.detail.y + 90, w, 22},
		controls: layoutRect{x, l.detail.bottom() - 63, w, 26},
		brew:     layoutRect{x + w - 112, l.detail.bottom() - 63, 112, 26},
		message:  layoutRect{x, l.detail.bottom() - 33, w, 28},
	}
	a.maximum = layoutRect{a.brew.x - 46, a.controls.y, 40, 26}
	a.viewport = layoutRect{x, a.toolbar.bottom() + 4, w, max(1, a.controls.y-a.toolbar.bottom()-10)}
	cols := max(1, (w-2)/166)
	cellW := (w - 8 - (cols-1)*6) / cols
	y := 0
	for gi, group := range r.Ingredients {
		material := group.Materials()
		if len(material) == 1 {
			continue
		}
		for _, category := range config.GlobalAlchemy.Categories {
			var keys []string
			for _, key := range category.Items {
				if _, ok := material[key]; ok {
					keys = append(keys, key)
				}
			}
			if len(keys) == 0 {
				continue
			}
			a.headings = append(a.headings, alchemyMaterialHeading{category.Label, layoutRect{x + 2, a.viewport.y + y, w - 10, 18}})
			y += 20
			for i, key := range keys {
				a.cells = append(a.cells, alchemyMaterialCell{key, gi, material[key], layoutRect{x + (i%cols)*(cellW+6), a.viewport.y + y + (i/cols)*60, cellW, 54}})
			}
			y += ((len(keys)+cols-1)/cols)*60 + 4
		}
	}
	a.height = y
	return a
}

func (ui *UISystem) drawAlchemyMaterials(screen *ebiten.Image, c *character.MMCharacter, l rareBookLayout, r *config.AlchemyRecipe) {
	g := ui.game
	selected := g.alchemySelection(r)
	g.alchemyBatches = max(1, min(99, g.alchemyBatches))
	preview := ui.alchemyPlan(r, selected, g.alchemyBatches)
	a := makeAlchemyMaterialLayout(l, r)
	output, _ := items.TryCreateItemFromYAML(r.Output)
	ui.rareBookItem(screen, output, layoutRect{a.title.x, a.title.y, 38, 38}, c)
	ui.rareBookText(screen, output.Name, layoutRect{a.title.x + 54, a.title.y, a.title.w - 54, 26})
	yield := character.AlchemyYield(c.SkillTier(character.SkillAlchemy), r.Family)
	drawUIText(screen, fmt.Sprintf("%d per batch | Carried: %d", yield, g.party.CountItemsByName(output.Name)), a.title.x+54, a.title.y+25)
	carried := g.party.CarriedItems()
	used := map[string]int{}
	for index, n := range preview.plan {
		used[carried[index].Name] += n
	}
	for _, group := range r.Ingredients {
		if len(group.Materials()) != 1 {
			continue
		}
		for key, cost := range group.Materials() {
			it, _ := items.TryCreateItemFromYAML(key)
			have, need := g.party.CountItemsByName(it.Name), cost*g.alchemyBatches
			drawFilledRect(screen, a.base.x, a.base.y, a.base.w, a.base.h, color.RGBA{28, 38, 36, 245})
			ui.rareBookItem(screen, it, layoutRect{a.base.x + 3, a.base.y + 3, 26, 26}, c)
			drawUITextColored(screen, profileText("Base: "+it.Name, a.base.w-152), a.base.x+36, a.base.y+10, color.RGBA{207, 185, 128, 255})
			col := color.RGBA{163, 198, 159, 255}
			if have < need {
				col = color.RGBA{221, 122, 105, 255}
			}
			drawUITextColored(screen, fmt.Sprintf("Use %d | Bag %d", need, have), a.base.right()-112, a.base.y+10, col)
		}
	}
	checked := 0
	for _, cell := range a.cells {
		if selected[cell.group][cell.key] {
			checked++
		}
	}
	drawUITextColored(screen, fmt.Sprintf("MATERIALS (%d selected)", checked), a.toolbar.x, a.toolbar.y+6, color.RGBA{209, 186, 131, 255})
	for i, label := range []string{"All", "None"} {
		on := i == 0
		ui.rareBookButton(screen, layoutRect{a.toolbar.right() - 112 + i*58, a.toolbar.y, 54, 22}, label, true, func() {
			for gi, group := range r.Ingredients {
				if len(group.Materials()) > 1 {
					for key := range group.Materials() {
						selected[gi][key] = on
					}
				}
			}
			ui.alchemyRevision++
			g.rareBookMessage = ""
		})
	}
	if ui.alchemyScroll == nil {
		ui.alchemyScroll = map[string]int{}
	}
	limit := max(0, a.height-a.viewport.h)
	offset := max(0, min(ui.alchemyScroll[r.Key], limit))
	ui.alchemyScroll[r.Key] = offset
	// The displayed-input adapter owns scrolling, so a wheel cannot also click a
	// newly moved tile before its replacement frame has been presented.
	ui.onDisplayedInput(uiCommandPointer, a.viewport, func() {
		if ui.modalLayerOwnsInput() {
			return
		}
		mx, my := pointerPosition()
		if !isMouseHoveringBox(mx, my, a.viewport.x, a.viewport.y, a.viewport.right(), a.viewport.bottom()) {
			return
		}
		_, wheel := pointerWheel()
		delta := int(wheel * 40)
		if delta == 0 && wheel != 0 {
			if wheel > 0 {
				delta = 1
			} else {
				delta = -1
			}
		}
		next := max(0, min(ui.alchemyScroll[r.Key]-delta, limit))
		if next != ui.alchemyScroll[r.Key] {
			ui.alchemyScroll[r.Key] = next
			ui.alchemyRevision++
		}
	})
	clip := image.Rect(a.viewport.x, a.viewport.y, a.viewport.right(), a.viewport.bottom()).Intersect(uiBounds(screen))
	if !clip.Empty() {
		dst := uiClip(screen, clip)
		for _, h := range a.headings {
			y := h.rect.y - offset
			drawUITextColored(dst, h.label, h.rect.x, y+2, color.RGBA{174, 169, 141, 255})
			drawFilledRect(dst, h.rect.x, y+16, h.rect.w, 1, color.RGBA{64, 66, 53, 255})
		}
		for _, cell := range a.cells {
			cell.rect.y -= offset
			hit := image.Rect(cell.rect.x, cell.rect.y, cell.rect.right(), cell.rect.bottom()).Intersect(clip)
			if hit.Empty() {
				continue
			}
			it, _ := items.TryCreateItemFromYAML(cell.key)
			have := g.party.CountItemsByName(it.Name)
			on := selected[cell.group][cell.key]
			bg, border := color.RGBA{23, 30, 30, 250}, color.RGBA{74, 78, 66, 255}
			if on {
				bg = color.RGBA{43, 51, 36, 250}
				border = color.RGBA{184, 161, 94, 255}
			}
			drawFilledRect(dst, cell.rect.x, cell.rect.y, cell.rect.w, cell.rect.h, bg)
			drawRectBorder(dst, cell.rect.x, cell.rect.y, cell.rect.w, cell.rect.h, 1, border)
			drawUITextColored(dst, profileText(it.Name, cell.rect.w-26), cell.rect.x+5, cell.rect.y+5, color.RGBA{214, 210, 192, 255})
			box := layoutRect{cell.rect.right() - 17, cell.rect.y + 4, 12, 12}
			drawRectBorder(dst, box.x, box.y, box.w, box.h, 1, border)
			if on {
				ink := color.RGBA{234, 216, 144, 255}
				uiStrokeLine(dst, float32(box.x+2), float32(box.y+6), float32(box.x+5), float32(box.y+9), 1.5, ink, true)
				uiStrokeLine(dst, float32(box.x+5), float32(box.y+9), float32(box.x+10), float32(box.y+3), 1.5, ink, true)
			}
			icon := layoutRect{cell.rect.x + 5, cell.rect.y + 20, 30, 30}
			ui.drawInventoryItemIcon(dst, it, icon.x, icon.y, icon.w, icon.h, 1, true)
			if have == 0 {
				drawFilledRect(dst, icon.x, icon.y, icon.w, icon.h, color.RGBA{0, 0, 0, 130})
			}
			tx := cell.rect.x + 42
			drawUITextColored(dst, fmt.Sprintf("Bag %d", have), tx, cell.rect.y+22, color.RGBA{196, 198, 186, 255})
			take := "Use -"
			if preview.err == nil {
				take = fmt.Sprintf("Use %d", used[it.Name])
			}
			drawUITextColored(dst, take, cell.rect.right()-max(38, uiTextWidth(take))-5, cell.rect.y+22, color.RGBA{150, 201, 145, 255})
			drawUITextColored(dst, fmt.Sprintf("%d per batch", cell.count), tx, cell.rect.y+38, color.RGBA{156, 159, 145, 255})
			mx, my := pointerPosition()
			if image.Pt(mx, my).In(hit) {
				lines := strings.Split(GetItemTooltip(it, c, g.combat, tooltipDetailHeld()), "\n")
				lines = append(lines, fmt.Sprintf("Bag: %d | Per batch: %d", have, cell.count))
				ui.queueItemTooltip(lines, it, c, mx+16, my+8)
			}
			key, gi := cell.key, cell.group
			ui.rareBookClick(layoutRect{hit.Min.X, hit.Min.Y, hit.Dx(), hit.Dy()}, true, func() { selected[gi][key] = !selected[gi][key]; ui.alchemyRevision++; g.rareBookMessage = "" })
		}
	}
	ui.drawScrollbar(screen, "alchemy:"+r.Key, layoutRect{a.viewport.right() - 4, a.viewport.y, 3, a.viewport.h}, offset, a.height, !ui.modalLayerOwnsInput(), func(v int) {
		ui.alchemyScroll[r.Key] = v
		ui.alchemyRevision++
	})
	ui.rareBookButton(screen, layoutRect{a.controls.x, a.controls.y, 26, 26}, "-", g.alchemyBatches > 1, func() { g.alchemyBatches-- })
	ui.rareBookButton(screen, layoutRect{a.controls.x + 30, a.controls.y, 26, 26}, "+", g.alchemyBatches < preview.max, func() { g.alchemyBatches++ })
	drawCenteredUIText(screen, fmt.Sprintf("Batches %d/%d", g.alchemyBatches, preview.max), a.controls.x+60, a.controls.y, a.controls.w-218, 26)
	ui.rareBookButton(screen, a.maximum, "Max", preview.max > 0, func() { g.alchemyBatches = preview.max })
	reason, safe := g.safeToPrepare()
	ui.rareBookButton(screen, a.brew, fmt.Sprintf("Brew %d items", yield*g.alchemyBatches), safe && preview.err == nil, func() { g.brewSelectedRecipe() })
	message := g.rareBookMessage
	if message == "" {
		message = "Checked materials only. Cheapest mixture first."
	}
	if preview.err != nil {
		message = preview.err.Error()
	}
	if !safe {
		message = reason
	}
	ui.rareBookText(screen, message, a.message)
}

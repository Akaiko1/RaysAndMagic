package game

import (
	"image/color"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	tavernRosterAction   = "tavern_roster"
	tavernStashAction    = "tavern_stash"
	tavernServicesAction = "tavern_services"
	tavernRumorsAction   = "tavern_rumors"
)

type tavernTab struct {
	label  string
	action string
}

// tavernDrawsAction is the authored-action contract of the fixed tavern
// surface. Both the tabs and boot validation ask this predicate, so adding a new
// tavern service cannot make the validator accept a row the UI ignores.
func tavernDrawsAction(action string) bool {
	switch action {
	case "tavern_rest", "buy_food", "open_roster", "manage_stash":
		return true
	default:
		return false
	}
}

func tavernChoice(npc *character.NPC, action string) *character.NPCDialogueChoice {
	if npc == nil || !tavernDrawsAction(action) {
		return nil
	}
	return npc.DialogueData.TopLevelChoice(action)
}

func tavernServiceChoices(npc *character.NPC) []*character.NPCDialogueChoice {
	choices := make([]*character.NPCDialogueChoice, 0, 2)
	for _, action := range []string{"tavern_rest", "buy_food"} {
		if choice := tavernChoice(npc, action); choice != nil {
			choices = append(choices, choice)
		}
	}
	return choices
}

func tavernTabs(npc *character.NPC) []tavernTab {
	if npc == nil || npc.DialogueData == nil {
		return nil
	}
	tabs := make([]tavernTab, 0, 4)
	if tavernChoice(npc, "open_roster") != nil {
		tabs = append(tabs, tavernTab{label: "Roster", action: tavernRosterAction})
	}
	if tavernChoice(npc, "manage_stash") != nil {
		tabs = append(tabs, tavernTab{label: "Stash", action: tavernStashAction})
	}
	if len(tavernServiceChoices(npc)) > 0 {
		tabs = append(tabs, tavernTab{label: "Services", action: tavernServicesAction})
	}
	tabs = append(tabs, tavernTab{label: "Rumors", action: tavernRumorsAction})
	return tabs
}

func (g *MMGame) activeTavernTab(npc *character.NPC) (tavernTab, bool) {
	tabs := tavernTabs(npc)
	if len(tabs) == 0 {
		return tavernTab{}, false
	}
	if g.dialogTab < 0 || g.dialogTab >= len(tabs) {
		g.dialogTab = 0
	}
	return tabs[g.dialogTab], true
}

func tavernContentRect(dialogX, dialogY, dialogWidth, dialogHeight int) layoutRect {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	return layoutRect{layout.body.x + 6, layout.body.y + 6, layout.body.w - 12, layout.body.h - 12}
}

func tavernServiceCardRect(area layoutRect, index int) layoutRect {
	const (
		sidePadding = 18
		gap         = 16
	)
	width := (area.w - 2*sidePadding - gap) / 2
	return layoutRect{area.x + sidePadding + index*(width+gap), area.y + 48, width, 180}
}

func tavernServiceConfirmRect(area layoutRect) layoutRect {
	const width = 360
	return layoutRect{area.x + (area.w-width)/2, area.y + 254, width, 46}
}

func (ui *UISystem) drawTavernDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	g := ui.game
	npc := g.dialogNPC
	tabs := tavernTabs(npc)
	if len(tabs) == 0 {
		ui.drawEncounterDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
		return
	}
	if g.dialogTab < 0 || g.dialogTab >= len(tabs) {
		g.dialogTab = 0
	}
	labels := make([]string, len(tabs))
	for i := range tabs {
		labels[i] = tabs[i].label
	}
	tabsEnabled := !g.stashDragActive && !g.stashDragPickedUp && !ui.stackSplitPicker.open
	ui.drawDialogFolderTabsEnabled(screen, dialogX, dialogY, labels, tabsEnabled)

	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	drawDebugText(screen, clipDebugText(uitext.Text("dialog.tavern")+npc.Name, layout.title.w), layout.title.x, layout.title.y)
	drawDebugText(screen, clipDebugText(uitext.Text("dialog.party_gold", g.party.Gold), layout.balance.w),
		layout.balance.x, layout.balance.y)

	greeting := ""
	if npc.DialogueData != nil {
		greeting = npc.DialogueData.Greeting
	}
	ui.drawWrappedTextWithOverflow(screen, greeting, layout.greeting, 2, dialogueLineHeight)

	area := tavernContentRect(dialogX, dialogY, dialogWidth, dialogHeight)
	drawPortraitFrame(screen, area.x, area.y, area.w, area.h)

	tab := tabs[g.dialogTab]
	switch tab.action {
	case tavernRosterAction:
		ui.drawRosterManager(screen, layoutRect{area.x + 16, area.y + 18, area.w - 32, area.h - 30}, ui.topModalLayer() == modalLayerDialog)
	case tavernStashAction:
		ui.drawTavernStash(screen, area)
	case tavernServicesAction:
		ui.drawTavernServices(screen, npc, area)
	case tavernRumorsAction:
		ui.drawTavernRumor(screen, npc, area)
	}

	footer := uitext.Text("dialog.tab_or_switches_sections_esc_leaves")
	switch tab.action {
	case tavernRosterAction:
		footer = uitext.Text("dialog.select_an_active_hero_then_click_a")
	case tavernStashAction:
		footer = uitext.Text("dialog.drag_items_between_the_chest_and_your")
	case tavernServicesAction:
		footer = uitext.Text("dialog.select_a_service_then_confirm_it_up")
	case tavernRumorsAction:
		footer = uitext.Text("dialog.rumors_follow_the_story_tab_or_switches")
	}
	drawDebugText(screen, clipDebugText(footer, layout.footer[0].w), layout.footer[0].x, layout.footer[0].y)
}

func (ui *UISystem) drawTavernStash(screen *ebiten.Image, area layoutRect) {
	g := ui.game
	if g.stash == nil {
		drawCenteredDebugText(screen, "The shared stash could not be loaded.", area.x, area.y, area.w, area.h)
		return
	}
	drawDebugText(screen, "Shared across all saves.", area.x+16, area.y+10)
	ui.drawStashManager(screen, computeStashLayoutForArea(area, 46), ui.topModalLayer() == modalLayerDialog)
}

func (ui *UISystem) drawTavernServices(screen *ebiten.Image, npc *character.NPC, area layoutRect) {
	g := ui.game
	choices := tavernServiceChoices(npc)
	if g.selectedChoice < 0 || g.selectedChoice >= len(choices) {
		g.selectedChoice = 0
	}
	drawDebugTextColored(screen, "Services", area.x+16, area.y+16, color.RGBA{220, 180, 90, 255})
	mouseX, mouseY := ebiten.CursorPosition()
	for i, choice := range choices {
		card := tavernServiceCardRect(area, i)
		hovered := isMouseHoveringBox(mouseX, mouseY, card.x, card.y, card.right(), card.bottom())
		affordable := g.party.Gold >= choice.Cost
		ui.drawButtonFrame(screen, card.x, card.y, card.w, card.h, hovered || g.selectedChoice == i)
		if !affordable {
			drawFilledRect(screen, card.x+4, card.y+4, card.w-8, card.h-8, color.RGBA{25, 3, 3, 55})
		}

		title := uitext.Text("dialog.rest_the_night")
		detail := uitext.Text("dialog.restore_hp_and_spell_points_for_every")
		effect := uitext.Text("dialog.dead_party_members_remain_dead")
		if choice.Action == "buy_food" {
			title = uitext.Text("dialog.buy_rations")
			detail = uitext.Text("dialog.add_food_to_the_party_supplies", choice.Amount)
			effect = uitext.Text("dialog.current_food", g.party.Food)
		}
		drawDebugTextColored(screen, title, card.x+14, card.y+16, color.RGBA{230, 205, 135, 255})
		ui.drawWrappedTextWithOverflow(screen, detail, layoutRect{card.x + 14, card.y + 50, card.w - 28, 40}, 2, dialogueLineHeight)
		drawDebugText(screen, effect, card.x+14, card.y+104)
		drawDebugText(screen, uitext.Text("dialog.cost_gold", choice.Cost), card.x+14, card.y+132)
		if !affordable {
			drawDebugTextColored(screen, uitext.Text("dialog.not_enough_gold"), card.x+14, card.y+152, color.RGBA{225, 105, 105, 255})
		} else if g.selectedChoice == i {
			drawDebugTextColored(screen, "Selected", card.x+14, card.y+152, color.RGBA{235, 195, 95, 255})
		} else {
			drawDebugTextColored(screen, uitext.Text("dialog.click_to_select"), card.x+14, card.y+152, color.RGBA{125, 205, 135, 255})
		}
		ui.onDisplayedInput(uiCommandClick, layoutRect{card.x, card.y, (card.right()) - (card.x), (card.bottom()) - (card.y)}, func() {
			if g.consumeLeftClickIn(card.x, card.y, card.right(), card.bottom()) {
				g.selectedChoice = i
			}
		})
	}

	selected := choices[g.selectedChoice]
	confirm := tavernServiceConfirmRect(area)
	affordable := g.party.Gold >= selected.Cost
	hovered := isMouseHoveringBox(mouseX, mouseY, confirm.x, confirm.y, confirm.right(), confirm.bottom())
	ui.drawButtonFrame(screen, confirm.x, confirm.y, confirm.w, confirm.h, affordable && hovered)
	if !affordable {
		drawFilledRect(screen, confirm.x+4, confirm.y+4, confirm.w-8, confirm.h-8, color.RGBA{25, 3, 3, 55})
	}
	label := uitext.Text("dialog.confirm_rest_gold", selected.Cost)
	if selected.Action == "buy_food" {
		label = uitext.Text("dialog.confirm_rations_gold", selected.Cost)
	}
	drawCenteredDebugText(screen, clipDebugText(label, confirm.w-20), confirm.x, confirm.y, confirm.w, confirm.h)
	ui.onDisplayedInput(uiCommandClick, layoutRect{confirm.x, confirm.y, (confirm.right()) - (confirm.x), (confirm.bottom()) - (confirm.y)}, func() {
		if affordable && g.consumeLeftClickIn(confirm.x, confirm.y, confirm.right(), confirm.bottom()) {
			g.pendingTavernAction = selected
		}
	})
}

func (ui *UISystem) drawTavernRumor(screen *ebiten.Image, npc *character.NPC, area layoutRect) {
	drawDebugTextColored(screen, uitext.Text("dialog.latest_rumor"), area.x+18, area.y+18, color.RGBA{220, 180, 90, 255})
	rumor := ui.game.currentRumorText(tavernRumorSeed(npc, ui.game.tavernRegionKey(npc)))
	lines := wrapDebugText(rumor, area.w-36)
	for i, line := range lines {
		y := area.y + 56 + i*dialogueLineHeight
		if y >= area.bottom()-20 {
			break
		}
		drawDebugText(screen, line, area.x+18, y)
	}
}

func (ih *InputHandler) handleTavernInput() {
	g := ih.game
	tabs := tavernTabs(g.dialogNPC)
	if len(tabs) == 0 {
		return
	}
	if g.dialogTab < 0 || g.dialogTab >= len(tabs) {
		g.switchDialogTab(0)
	}

	if choice := g.pendingTavernAction; choice != nil {
		g.pendingTavernAction = nil
		ih.executeTavernAction(choice)
		return
	}
	if g.stashDragActive || g.stashDragPickedUp || ih.game.stackSplitInteractionActive() {
		return
	}
	if ih.keys.Consume(ebiten.KeyTab) {
		g.switchDialogTab((g.dialogTab + 1) % len(tabs))
		return
	}
	for i := 0; i < len(tabs) && i < 9; i++ {
		if ih.keys.Consume(ebiten.Key1 + ebiten.Key(i)) {
			g.switchDialogTab(i)
			return
		}
	}

	tab, ok := g.activeTavernTab(g.dialogNPC)
	if !ok || tab.action != tavernServicesAction {
		return
	}
	choices := tavernServiceChoices(g.dialogNPC)
	if len(choices) == 0 {
		return
	}
	if g.selectedChoice < 0 || g.selectedChoice >= len(choices) {
		g.selectedChoice = 0
	}
	if ih.keys.Consume(ebiten.KeyUp) {
		g.selectedChoice = (g.selectedChoice - 1 + len(choices)) % len(choices)
		return
	}
	if ih.keys.Consume(ebiten.KeyDown) {
		g.selectedChoice = (g.selectedChoice + 1) % len(choices)
		return
	}
	if ih.keys.Consume(ebiten.KeyEnter) {
		ih.executeTavernAction(choices[g.selectedChoice])
	}
}

func (ih *InputHandler) executeTavernAction(choice *character.NPCDialogueChoice) {
	if choice == nil {
		return
	}
	switch choice.Action {
	case "tavern_rest":
		ih.handleTavernRest(choice)
	case "buy_food":
		ih.handleBuyFood(choice)
	}
}

func (g *MMGame) closeRosterScreen() {
	g.rosterScreenOpen = false
	g.rosterSelectedActive = -1
}

func (g *MMGame) closeStashScreen() {
	g.stashScreenOpen = false
	g.clearStashDrag()
}

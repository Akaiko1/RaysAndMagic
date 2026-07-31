package game

import (
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func tavernTestNPC() *character.NPC {
	return &character.NPC{
		Name: "Test Tavern",
		DialogueData: &character.NPCDialogue{
			Greeting: "Welcome.",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Manage roster", Action: "open_roster"},
				{Text: "Manage stash", Action: "manage_stash"},
				{Text: "Rest", Action: "tavern_rest", Cost: 25},
				{Text: "Buy food", Action: "buy_food", Cost: 100, Amount: 5},
				{Text: "Leave", Action: "leave"},
			},
		},
	}
}

func TestTavernDialogUsesWorkingTabs(t *testing.T) {
	npc := tavernTestNPC()
	if got := npcDialogKindFor(npc); got != dialogKindTavern {
		t.Fatalf("dialog kind = %v, want tavern", got)
	}

	tabs := tavernTabs(npc)
	labels := make([]string, len(tabs))
	actions := make([]string, len(tabs))
	for i := range tabs {
		labels[i] = tabs[i].label
		actions[i] = tabs[i].action
	}
	if want := []string{"Roster", "Stash", "Services", "Rumors"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("tab labels = %v, want %v", labels, want)
	}
	if want := []string{tavernRosterAction, tavernStashAction, tavernServicesAction, tavernRumorsAction}; !reflect.DeepEqual(actions, want) {
		t.Fatalf("tab actions = %v, want %v", actions, want)
	}
}

func TestNestedTavernRestRemainsARegularDialogueChoice(t *testing.T) {
	npc := &character.NPC{
		DialogueData: &character.NPCDialogue{
			Choices: []*character.NPCDialogueChoice{{
				Text:   "Ask about lodging",
				Action: "info",
				Choices: []*character.NPCDialogueChoice{{
					Text:   "Rest",
					Action: "tavern_rest",
					Cost:   25,
				}},
			}},
		},
	}

	if !npcOffersTavernRest(npc) {
		t.Fatal("nested rest should still identify a Town Portal tavern capability")
	}
	if got := npcDialogKindFor(npc); got != dialogKindChoices {
		t.Fatalf("nested rest dialog kind = %v, want regular choices", got)
	}
}

func TestTavernUsesExpandedDialogAndTabsFit(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.dialogNPC = tavernTestNPC()
	dlg := npcDialogLayout(g)
	if dlg.w != tavernDialogWidth || dlg.h != tavernDialogHeight {
		t.Fatalf("tavern dialog = %dx%d, want %dx%d", dlg.w, dlg.h, tavernDialogWidth, tavernDialogHeight)
	}
	for i := range tavernTabs(g.dialogNPC) {
		x, _, w, _ := dialogFolderTabRect(dlg.x, dlg.y, i)
		if x < dlg.x || x+w > dlg.x+dlg.w {
			t.Fatalf("tab %d rect [%d,%d) escapes tavern dialog [%d,%d)", i, x, x+w, dlg.x, dlg.x+dlg.w)
		}
	}
}

func TestTavernServiceCardsStayInsideContent(t *testing.T) {
	area := tavernContentRect(0, 0, tavernDialogWidth, tavernDialogHeight)
	left := tavernServiceCardRect(area, 0)
	right := tavernServiceCardRect(area, 1)
	confirm := tavernServiceConfirmRect(area)
	for i, card := range []layoutRect{left, right} {
		if card.x < area.x || card.y < area.y || card.right() > area.right() || card.bottom() > area.bottom() {
			t.Fatalf("service card %d %+v escapes content %+v", i, card, area)
		}
		if card.x-area.x < 16 || area.right()-card.right() < 16 {
			t.Fatalf("service card %d lacks padding from outer frame: card=%+v area=%+v", i, card, area)
		}
	}
	if left.right() > right.x {
		t.Fatalf("service cards overlap: left=%+v right=%+v", left, right)
	}
	if confirm.y < left.bottom()+16 || confirm.y < right.bottom()+16 {
		t.Fatalf("confirm control touches service cards: left=%+v right=%+v confirm=%+v", left, right, confirm)
	}
	if confirm.x < area.x+16 || confirm.right() > area.right()-16 || confirm.bottom() > area.bottom()-16 {
		t.Fatalf("confirm control lacks padding from outer frame: confirm=%+v area=%+v", confirm, area)
	}
}

func TestEmbeddedStashLayoutStaysInsideContent(t *testing.T) {
	area := tavernContentRect(0, 0, tavernDialogWidth, tavernDialogHeight)
	layout := computeStashLayoutForArea(area, 46)
	toggle := stashToggleRect(layout)
	if toggle.Min.X < area.x || toggle.Max.X > area.right() || toggle.Min.Y < area.y || toggle.Max.Y > area.bottom() {
		t.Fatalf("stash toggle %v escapes content %+v", toggle, area)
	}
	lastChest := stashCellRect(layout.centerX, layout.chestTop, stashInvMaxShown-1)
	lastBag := stashCellRect(layout.centerX, layout.invTop, stashInvMaxShown-1)
	if lastChest.Max.Y > area.bottom() || lastBag.Max.Y > area.bottom() || layout.pagerY+20 > area.bottom() {
		t.Fatalf("embedded stash escapes content: chest=%v bag=%v pagerY=%d area=%+v", lastChest, lastBag, layout.pagerY, area)
	}
}

func TestTavernPendingActionUsesExistingServiceLogic(t *testing.T) {
	g := &MMGame{
		party: &character.Party{Gold: 250, Food: 2},
	}
	npc := tavernTestNPC()
	g.dialogActive = true
	g.dialogNPC = npc
	g.pendingTavernAction = npc.DialogueData.Choices[3]

	ih := &InputHandler{game: g}
	ih.handleTavernInput()

	if g.party.Gold != 150 || g.party.Food != 7 {
		t.Fatalf("rations result = %d gold, %d food; want 150 gold, 7 food", g.party.Gold, g.party.Food)
	}
	if !g.dialogActive || g.dialogNPC != npc {
		t.Fatal("buying rations should keep the tavern open")
	}
	if g.pendingTavernAction != nil {
		t.Fatal("resolved tavern action remained pending")
	}
}

func TestSwitchDialogTabClearsTransientTabState(t *testing.T) {
	g := &MMGame{
		dialogTab:            0,
		pendingTavernAction:  &character.NPCDialogueChoice{Action: "tavern_rest"},
		pendingBuffService:   &character.NPCDialogueChoice{Action: "cast_buff"},
		dialogLastClickedIdx: 3,
		dialogLastClickZone:  "tavern",
		rosterSelectedActive: 1,
		stashDragActive:      true,
		stashDragFrom:        2,
		stashDragItem:        items.Item{Name: "Test"},
	}

	g.switchDialogTab(2)

	if g.pendingTavernAction != nil || g.pendingBuffService != nil {
		t.Fatal("tab transition retained a pending action from the previous tab")
	}
	if g.dialogLastClickedIdx != -1 || g.dialogLastClickZone != "" {
		t.Fatal("tab transition retained the previous tab's click tracker")
	}
	if g.rosterSelectedActive != -1 || g.stashDragActive || g.stashDragFrom != -1 {
		t.Fatal("tab transition retained embedded manager state")
	}
}

func TestEmbeddedStashParticipatesInDragLifecycle(t *testing.T) {
	npc := tavernTestNPC()
	g := &MMGame{
		party:        &character.Party{},
		dialogActive: true,
		dialogNPC:    npc,
		dialogTab:    1,
	}
	if !g.stashInteractionOpen() {
		t.Fatal("active tavern Stash tab was not recognized as a stash interaction surface")
	}
	g.dialogTab = 0
	if g.stashInteractionOpen() {
		t.Fatal("Roster tab was incorrectly recognized as a stash interaction surface")
	}
}

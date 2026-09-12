package game

import (
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
)

func staleConversation(npc *character.NPC) dialogState {
	choice := &character.NPCDialogueChoice{Text: "Old service"}
	return dialogState{
		dialogActive: true, dialogNPC: npc, dialogLastClickTime: 123,
		dialogLastClickedIdx: 3, dialogLastClickZone: "old",
		dialogSelectedSpell: 2, selectedCharIdx: 2, skillTrainerPage: 3,
		selectedSpellKey: "old", selectedChoice: 2, dialogNodePath: []*character.NPCDialogueChoice{choice},
		dialogTab: 1, pendingBuffService: choice, pendingTavernAction: choice,
		merchantBuyPage: 2, merchantSellPage: 3, spellTraderPage: 4, cardCollectorInvPage: 5,
	}
}

func TestConversationLifetimeEntryPoints(t *testing.T) {
	for _, entry := range []string{"leave", "escape", "quest", "roster", "wipe", "title", "reopen_same", "reopen_other", "trainer_escape"} {
		t.Run(entry, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			setTestWorldManager(t, nil)
			npc := &character.NPC{Name: "First", Type: character.NPCTypeMerchant}
			g.dialogState = staleConversation(npc)
			ih := NewInputHandler(g)
			want := dialogState{dialogLastClickedIdx: -1}
			switch entry {
			case "leave":
				dialogActions["leave"](ih, npc, nil)
			case "escape", "trainer_escape":
				if entry == "trainer_escape" {
					want = g.dialogState
					g.skillTrainerPopup = true
				}
				ih.keys = keytracker.NewWithSource(escKeyboard())
				ih.HandleInput()
			case "quest":
				ih.handleGiveQuest("")
			case "roster":
				ih.handleOpenRoster()
				if !g.rosterScreenOpen {
					t.Fatal("roster handoff did not open its destination")
				}
			case "wipe":
				for _, member := range g.party.Members {
					member.HitPoints = 0
				}
				g.checkGameOver()
			case "title":
				g.returnToMainMenu()
			case "reopen_same", "reopen_other":
				if entry == "reopen_other" {
					npc = &character.NPC{Name: "Second", Type: character.NPCTypeMerchant}
				}
				ih.openNPCInteraction(npc)
				want.dialogActive, want.dialogNPC = true, npc
			}
			if !reflect.DeepEqual(g.dialogState, want) {
				t.Fatalf("conversation lifetime leaked state after %s: got %+v, want %+v", entry, g.dialogState, want)
			}
		})
	}
}

func TestMenuLifetimeEntryPoints(t *testing.T) {
	for _, entry := range []string{"continue", "escape", "title", "reopen", "rename_escape", "submenu_escape"} {
		t.Run(entry, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			setTestWorldManager(t, nil)
			g.menuState = menuState{mainMenuOpen: true, mainMenuSelection: 3, slotSelection: 4,
				savePage: 2, saveRenameSlot: 9, saveRenameInput: "Old slot", audioSliderDrag: 1}
			want := menuState{saveRenameSlot: -1, audioSliderDrag: -1}
			switch entry {
			case "continue":
				mainMenuOptions[0].action(g)
			case "title":
				g.returnToMainMenu()
			case "reopen":
				g.openMainMenu()
				want.mainMenuOpen = true
			default:
				if entry == "rename_escape" {
					g.mainMenuMode, g.saveRenameOpen = MenuLoadSelect, true
					want = g.menuState
					want.saveRenameOpen, want.saveRenameSlot, want.saveRenameInput = false, -1, ""
				} else if entry == "submenu_escape" {
					g.mainMenuMode = MenuLoadSelect
					want = g.menuState
					want.mainMenuMode = MenuMain
				}
				ih := NewInputHandler(g)
				ih.keys = keytracker.NewWithSource(escKeyboard())
				ih.HandleInput()
			}
			if !reflect.DeepEqual(g.menuState, want) {
				t.Fatalf("menu lifetime leaked state after %s: got %+v, want %+v", entry, g.menuState, want)
			}
		})
	}
}

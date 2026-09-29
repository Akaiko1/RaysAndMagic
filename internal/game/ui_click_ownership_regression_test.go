package game

import (
	"fmt"
	"testing"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/quests"
)

func claimOwnershipHarness(t *testing.T, gold int) *displayedModalHarness {
	t.Helper()
	h := newDisplayedModalHarness(t, 1024, 768)
	h.g.menuOpen, h.g.currentTab = true, TabQuests
	h.g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"first":  {Name: "A first", Description: "First objective.", TargetCount: 1, IsStartingQuest: true, Rewards: quests.QuestRewards{Gold: gold}},
		"third":  {Name: "C inactive", Description: "Inactive.", TargetCount: 1},
		"second": {Name: "B second", Description: "Second objective.", TargetCount: 1, IsStartingQuest: true, Rewards: quests.QuestRewards{Gold: gold}},
	}})
	h.g.questManager.InitializeStartingQuests()
	h.g.questManager.MarkCompleted("first")
	h.g.questManager.MarkCompleted("second")
	return h
}

func claimButtons(h *displayedModalHarness) []layoutRect {
	h.ui.Draw(h.screen)
	var buttons []layoutRect
	content := computeTabbedMenuLayout(h.g.config.GetScreenWidth(), gameplayViewportBottom(h.g)).content
	all := h.g.questManager.GetAllQuests()
	sortQuestJournal(all)
	layout := computeQuestContentLayout(content, nil, 0)
	copies := make([]questCardCopy, len(all))
	for i, q := range all {
		copies[i] = questCardCopyForQuest(q, layout.cardW, layout.maxDescRows)
	}
	layout = computeQuestContentLayout(content, copies, h.ui.questPage)
	for _, cmd := range h.ui.displayedInput.commands {
		if cmd.kind != uiCommandClick {
			continue
		}
		for _, row := range layout.rows {
			if cmd.bounds.x >= row.x && cmd.bounds.right() <= row.right() && cmd.bounds.y >= row.y && cmd.bounds.bottom() <= row.bottom() {
				buttons = append(buttons, cmd.bounds)
				break
			}
		}
	}
	return buttons
}

func TestJournalPhysicalClickClaimsExactlyOnce(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, updates := range []int{1, 4} {
			t.Run(fmt.Sprintf("tb=%v/updates=%d", tb, updates), func(t *testing.T) {
				h := claimOwnershipHarness(t, 10)
				h.g.turnBasedMode = tb
				fp := installFakePointer(t)
				buttons := claimButtons(h)
				if len(buttons) != 2 {
					t.Fatalf("claim buttons=%d", len(buttons))
				}
				fp.moveTo(buttons[0].x+2, buttons[0].y+2)
				fp.press()
				h.pointerStep()
				fp.hold()
				for n := 0; n < updates; n++ {
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
				}
				for n := 0; n < 3; n++ {
					h.pointerStep()
				}
				fp.release()
				h.pointerStep()
				fp.idle()
				h.pointerStep()
				if !h.g.questManager.GetQuest("first").RewardsClaimed || h.g.questManager.GetQuest("second").RewardsClaimed {
					t.Fatal("one physical gesture did not claim exactly the first quest")
				}
				buttons = claimButtons(h)
				if len(buttons) != 1 {
					t.Fatalf("remaining buttons=%d: %+v", len(buttons), buttons)
				}
				fp.moveTo(buttons[0].x+2, buttons[0].y+2)
				fp.press()
				h.pointerStep()
				if !h.g.questManager.GetQuest("second").RewardsClaimed {
					t.Fatal("fresh press on the new display was blocked")
				}
			})
		}
	}
}

func TestJournalClaimRetiresOldDisplayBatch(t *testing.T) {
	for _, gold := range []int{0, 10} {
		for _, sameButton := range []bool{false, true} {
			t.Run(fmt.Sprintf("gold=%d/same=%v", gold, sameButton), func(t *testing.T) {
				h := claimOwnershipHarness(t, gold)
				buttons := claimButtons(h)
				if len(buttons) != 2 {
					t.Fatalf("buttons=%d", len(buttons))
				}
				b := buttons[1]
				if sameButton {
					b = buttons[0]
				}
				now := time.Now().UnixMilli()
				h.g.mouseLeftClicks = []queuedClick{{x: buttons[0].x + 2, y: buttons[0].y + 2, at: now}, {x: b.x + 2, y: b.y + 2, at: now + 1}}
				h.ui.dispatchDisplayedInput()
				if !h.g.questManager.GetQuest("first").RewardsClaimed || h.g.questManager.GetQuest("second").RewardsClaimed {
					t.Fatal("claim batch acted past a changed journal layout")
				}
				if h.ui.displayedInputCurrent() || len(h.g.mouseLeftClicks) != 0 {
					t.Fatal("old quest display or queued click survived turn-in")
				}
			})
		}
	}
}

func TestQuestChangesInvalidateDisplayedActions(t *testing.T) {
	for _, surface := range []string{"journal", "npc"} {
		for _, change := range []string{"progress", "completed", "claimed", "target", "activation", "reset", "restore", "restored_progress"} {
			t.Run(surface+"/"+change, func(t *testing.T) {
				h := claimOwnershipHarness(t, 0)
				if surface == "npc" {
					h.g.menuOpen = false
					h.g.dialogActive = true
					h.g.dialogNPC = &character.NPC{Name: "Quest owner"}
				}
				fired := false
				h.ui.beginDisplayedInput()
				h.ui.onDisplayedInput(uiCommandClick, layoutRect{10, 10, 20, 20}, func() {
					if h.g.consumeLeftClickIn(10, 10, 30, 30) {
						fired = true
					}
				})
				h.ui.endDisplayedInput()
				q := h.g.questManager.GetQuest("first")
				switch change {
				case "progress":
					q.CurrentCount++
				case "completed":
					q.Completed = false
				case "claimed":
					q.RewardsClaimed = true
				case "target":
					q.DynamicTarget = 7
				case "activation":
					if err := h.g.questManager.ActivateQuest("third"); err != nil {
						t.Fatal(err)
					}
				case "reset":
					h.g.questManager.Reset()
				case "restored_progress":
					h.g.questManager.RestoreQuestProgress("first", quests.QuestStatusActive, 0, 0, false)
				case "restore":
					// Restoring a manager with the same visible values must retire closures
					// captured against the prior owner, without persisting UI identity.
					h.g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: h.g.questManager.Definitions()})
					h.g.questManager.InitializeStartingQuests()
					h.g.questManager.MarkCompleted("first")
					h.g.questManager.MarkCompleted("second")
				}
				h.g.mouseLeftClicks = []queuedClick{{x: 15, y: 15, at: time.Now().UnixMilli()}}
				h.ui.dispatchDisplayedInput()
				if fired || len(h.g.mouseLeftClicks) != 0 {
					t.Fatal("quest mutation retained an actionable old display")
				}
			})
		}
	}
}

func TestUnmatchedHubClickCannotHitLaterContent(t *testing.T) {
	for _, tab := range []MenuTab{TabQuests, TabInventory, TabSpellbook} {
		t.Run(fmt.Sprint(tab), func(t *testing.T) {
			h := claimOwnershipHarness(t, 0)
			h.g.currentTab = tab
			h.ui.beginDisplayedInput()
			h.ui.endDisplayedInput()
			h.g.mouseLeftClicks = []queuedClick{{x: 15, y: 15, at: time.Now().UnixMilli()}}
			h.ui.dispatchDisplayedInput()
			fired := false
			h.ui.beginDisplayedInput()
			h.ui.onDisplayedInput(uiCommandClick, layoutRect{10, 10, 20, 20}, func() {
				if h.g.consumeLeftClickIn(10, 10, 30, 30) {
					fired = true
				}
			})
			h.ui.endDisplayedInput()
			h.ui.dispatchDisplayedInput()
			if fired || len(h.g.mouseLeftClicks) != 0 {
				t.Fatal("background click was replayed on a later hub widget")
			}
		})
	}
}

func TestPhysicalInventoryGesturesPreserveDoubleClick(t *testing.T) {
	for _, source := range []string{"bag", "quick_slot"} {
		t.Run(source, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			// Isolate manual gestures from the separately tested automatic drinking policy.
			h.g.config.Characters.AutoDrink.ThresholdPct = 0
			h.g.menuOpen, h.g.currentTab = true, TabInventory
			potion := items.CreateItemFromYAML("health_potion")
			potion.Quantity, potion.InstanceID = 3, 700
			ch := h.g.party.Members[0]
			ch.HitPoints = 1
			h.g.party.Inventory = []items.Item{potion}
			layout := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(h.g)).content)
			x, y, w, height := scaleInventorySourceRect(layout.grid.x, layout.grid.y, layout.grid.w, layout.grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
			count := func() int { return h.g.party.Inventory[0].Count() }
			if source == "quick_slot" {
				ch.QuickSlots[0] = &potion
				h.g.party.Inventory = nil
				h.g.menuOpen, h.g.turnBasedMode = false, true
				ch.ActionsRemaining = 9
				h.g.world.Monsters = nil
				bar, visible := inGameQuickSlotBarLayout(h.g)
				if !visible {
					t.Fatal("gameplay quick bar is not visible")
				}
				_, slots := quickSlotRects(bar.x, bar.y, bar.w)
				x, y, w, height = slots[0].Min.X, slots[0].Min.Y, slots[0].Dx(), slots[0].Dy()
				count = func() int { return ch.QuickSlots[0].Count() }
			}
			fp := installFakePointer(t)
			fp.moveTo(x+w/2, y+height/2)
			fp.press()
			h.pointerStep()
			fp.hold()
			h.pointerStep()
			h.pointerStep()
			fp.release()
			h.pointerStep()
			if count() != 3 || ch.HitPoints != 1 {
				t.Fatal("single press/hold/release executed a double-click action")
			}
			fp.press()
			h.pointerStep()
			fp.hold()
			h.pointerStep()
			fp.release()
			h.pointerStep()
			fp.idle()
			h.pointerStep()
			if count() != 2 || ch.HitPoints <= 1 {
				t.Fatal("two physical clicks did not consume exactly one potion")
			}
		})
	}
}

func TestUnmatchedClickKeepsOnlyExplorationOwner(t *testing.T) {
	for _, screen := range []AppScreen{AppScreenInGame, AppScreenMainMenu, AppScreenPartyCreate} {
		for _, right := range []bool{false, true} {
			t.Run(fmt.Sprintf("screen=%d/right=%v", screen, right), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				h.g.appScreen = screen
				h.ui.beginDisplayedInput()
				h.ui.endDisplayedInput()
				click := queuedClick{x: 15, y: 15, at: time.Now().UnixMilli()}
				if right {
					h.g.mouseRightClicks = []queuedClick{click}
				} else {
					h.g.mouseLeftClicks = []queuedClick{click}
				}
				h.ui.dispatchDisplayedInput()
				want := 0
				if screen == AppScreenInGame {
					want = 1
				}
				if got := len(h.g.mouseLeftClicks) + len(h.g.mouseRightClicks); got != want {
					t.Fatalf("unmatched events=%d, want %d for this input owner", got, want)
				}
			})
		}
	}
}

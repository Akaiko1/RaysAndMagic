package game

import (
	"fmt"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
	"ugataima/internal/threading"
	"ugataima/internal/world"
)

type displayedModalHarness struct {
	t      *testing.T
	g      *MMGame
	ui     *UISystem
	loop   *GameLoop
	screen *ebiten.Image
}

func newDisplayedModalHarness(t *testing.T, width, height int) *displayedModalHarness {
	t.Helper()
	g, ui := merchantDragGame(t)
	g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = width, height
	g.appScreen = AppScreenInGame
	g.dialogActive = false
	g.combat = NewCombatSystem(g)
	g.threading = threading.NewThreadingComponents(g.config)
	t.Cleanup(g.threading.Shutdown)
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
	setTestWorldManager(t, wm)
	loop := &GameLoop{game: g, ui: ui, inputHandler: NewInputHandler(g)}
	g.gameLoop = loop
	screen := ebiten.NewImage(width, height)
	t.Cleanup(screen.Deallocate)
	return &displayedModalHarness{t, g, ui, loop, screen}
}

// Each batch starts with the real displayed layout, then uses the same Update
// entry point as a paused game, with no modal events left for later handlers.
func (h *displayedModalHarness) clicks(right bool, x, y, count int) {
	h.t.Helper()
	h.ui.Draw(h.screen)
	h.g.prevWorldClickAllowed = h.g.worldClickAllowed()
	now := time.Now().UnixMilli()
	for i := 0; i < count; i++ {
		c := queuedClick{x: x, y: y, at: now + int64(i)}
		if right {
			h.g.mouseRightClicks = append(h.g.mouseRightClicks, c)
		} else {
			h.g.mouseLeftClicks = append(h.g.mouseLeftClicks, c)
		}
	}
	if err := h.loop.Update(); err != nil {
		h.t.Fatal(err)
	}
	if len(h.g.mouseLeftClicks)+len(h.g.mouseRightClicks) != 0 {
		h.t.Fatal("modal event survived its owner")
	}
}

func (h *displayedModalHarness) pointerStep() {
	h.t.Helper()
	h.ui.Draw(h.screen)
	h.g.prevWorldClickAllowed = h.g.worldClickAllowed()
	if err := h.loop.Update(); err != nil {
		h.t.Fatal(err)
	}
}

func TestDisplayedPointerGesturesReachUpdate(t *testing.T) {
	for _, kind := range []string{"quick slot", "standalone stash", "tavern stash", "merchant"} {
		for _, outside := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/outside=%v", kind, outside), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				fp := installFakePointer(t)
				var sx, sy, dx, dy int
				switch kind {
				case "quick slot":
					g.menuOpen, g.currentTab = true, TabInventory
					menu := computeTabbedMenuLayout(1024, gameplayViewportBottom(g))
					layout := computeInventoryContentLayout(menu.content)
					x, y, w, height := scaleInventorySourceRect(layout.grid.x, layout.grid.y, layout.grid.w, layout.grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
					sx, sy = x+w/2, y+height/2
					_, slots := quickSlotRects(layout.quickSlots.x, layout.quickSlots.y, layout.quickSlots.w)
					dx, dy = slots[0].Min.X+3, slots[0].Min.Y+3
				case "standalone stash", "tavern stash":
					g.stash = &stash.Stash{}
					layout := computeStashLayout(1024, 768)
					if kind == "tavern stash" {
						g.dialogActive, g.dialogNPC, g.dialogTab = true, tavernTestNPC(), 1
						dlg := npcDialogLayout(g)
						layout = computeStashLayoutForArea(tavernContentRect(dlg.x, dlg.y, dlg.w, dlg.h), 46)
					} else {
						g.stashScreenOpen = true
					}
					src, dst := stashCellRect(layout.centerX, layout.invTop, 0), stashCellRect(layout.centerX, layout.chestTop, 0)
					sx, sy, dx, dy = src.Min.X+3, src.Min.Y+3, dst.Min.X+3, dst.Min.Y+3
				case "merchant":
					g.dialogActive = true
					dlg := npcDialogLayout(g)
					lx, rx, top, _ := merchantGridLayout(dlg.x, dlg.y)
					sx, sy, dx, dy = rx+3, top+3, lx+3, top+3
				}
				if outside {
					dx, dy = 1, 1
				}
				fp.moveTo(sx, sy)
				fp.press()
				h.pointerStep()
				if kind == "quick slot" && g.dragSrc != dragFromInventory || kind != "quick slot" && g.stashDragFrom != stashDragInvBase {
					t.Fatal("displayed source did not capture the armed pointer gesture")
				}
				fp.hold()
				fp.moveTo(dx, dy)
				h.pointerStep()
				fp.release()
				h.pointerStep()
				if g.dragSrc != dragNone || g.stashDragActive || g.stashDragDrop {
					t.Fatal("released gesture remained active")
				}
				if outside {
					if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 5 || h.ui.stackSplitPicker.open {
						t.Fatal("outside release changed inventory or opened a transaction")
					}
					return
				}
				if kind == "merchant" {
					if !h.ui.stackSplitPicker.open || h.ui.stackSplitPicker.source != stackSplitPickerMerchantSell || g.party.Inventory[0].Count() != 5 {
						t.Fatal("merchant drop did not preserve the stack pending confirmation")
					}
				} else if kind == "quick slot" {
					if it := g.party.Members[0].QuickSlots[0]; it == nil || it.Count() != 5 || len(g.party.Inventory) != 0 {
						t.Fatal("quick slot drop did not transfer the stack")
					}
				} else if g.stash.Slots[0].Count() != 5 || len(g.party.Inventory) != 0 {
					t.Fatal("stash drop did not transfer the stack")
				}
			})
		}
	}
}

func TestDisplayedHeldStatRepeatsAndStops(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	h.g.statPopupOpen, h.g.statPopupCharIdx = true, 0
	ch := h.g.party.Members[0]
	ch.FreeStatPoints = 5
	fp := installFakePointer(t)
	fp.moveTo((1024-340)/2+194, (768-320)/2+90)
	fp.press()
	h.pointerStep()
	if ch.FreeStatPoints != 4 {
		t.Fatal("initial stat press did not spend exactly one point")
	}
	fp.hold()
	for i := 0; i < statHoldInitialDelay+statHoldRepeatRate; i++ {
		h.pointerStep()
	}
	if ch.FreeStatPoints != 3 {
		t.Fatal("held stat did not repeat without queued clicks")
	}
	fp.release()
	h.pointerStep()
	fp.idle()
	for i := 0; i < statHoldRepeatRate+1; i++ {
		h.pointerStep()
	}
	if ch.FreeStatPoints != 3 || h.ui.statHoldStat != "" {
		t.Fatal("released stat hold kept spending points")
	}
}

func TestDisplayedContextOutsideClickDismisses(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	h.g.menuOpen, h.g.currentTab = true, TabInventory
	h.ui.inventoryContextOpen = true
	h.ui.inventoryContextX, h.ui.inventoryContextY = 400, 300
	h.ui.inventoryContextIndex = 0
	h.clicks(false, 1, 1, 1)
	if h.ui.inventoryContextOpen || h.g.party.Inventory[0].Count() != 5 {
		t.Fatal("outside context click failed to dismiss without discarding")
	}
}

func TestDisplayedTavernWidgetsKeepTheirOwner(t *testing.T) {
	for _, action := range []string{"roster", "services", "rumors"} {
		t.Run(action, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.dialogActive, g.dialogNPC = true, tavernTestNPC()
			g.party.Gold, g.party.Food = 250, 2
			dlg := npcDialogLayout(g)
			area := tavernContentRect(dlg.x, dlg.y, dlg.w, dlg.h)
			tab := map[string]int{"roster": 0, "services": 2, "rumors": 3}[action]
			x, y, _, _ := dialogFolderTabRect(dlg.x, dlg.y, tab)
			h.clicks(false, x+3, y+3, 1)
			if g.dialogTab != tab {
				t.Fatal("tavern tab click was lost")
			}
			switch action {
			case "roster":
				reserve := *g.party.Members[1]
				g.party.Reserve = []*character.MMCharacter{&reserve}
				active := g.party.Members[0]
				roster := layoutRect{area.x + 16, area.y + 18, area.w - 32, area.h - 30}
				h.clicks(false, roster.x+3, roster.y+40, 1)
				h.clicks(false, roster.x+(roster.w-16)/2+19, roster.y+40, 1)
				if g.party.Members[0] != &reserve || g.party.Reserve[0] != active {
					t.Fatal("displayed tavern roster did not swap selected heroes")
				}
			case "services":
				card := tavernServiceCardRect(area, 1)
				h.clicks(false, card.x+3, card.y+3, 1)
				confirm := tavernServiceConfirmRect(area)
				h.clicks(false, confirm.x+3, confirm.y+3, 1)
				if g.party.Gold != 150 || g.party.Food != 7 || g.pendingTavernAction != nil {
					t.Fatal("displayed tavern service did not execute once")
				}
			case "rumors":
				confirm := tavernServiceConfirmRect(area)
				h.clicks(false, confirm.x+3, confirm.y+3, 2)
				if g.party.Gold != 250 || g.party.Food != 2 {
					t.Fatal("hidden service consumed a click from the Rumors tab")
				}
			}
		})
	}
}

func TestDisplayedSaveEventsRespectLeftRightOrder(t *testing.T) {
	for _, rightFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("right_first=%v", rightFirst), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			if err := g.SaveGameToFile(saveRowPath(1)); err != nil {
				t.Fatal(err)
			}
			g.mainMenuOpen, g.mainMenuMode = true, MenuSaveSelect
			w, height := menuPanelSize(g.mainMenuMode)
			box, _, _ := menuRowRect((1024-w)/2, (768-height)/2, w, saveMenuListTopY, saveMenuRowPitch, 1)
			h.ui.Draw(h.screen)
			g.prevWorldClickAllowed = g.worldClickAllowed()
			now := time.Now().UnixMilli()
			leftAt, rightAt := now, now+1
			if rightFirst {
				leftAt, rightAt = rightAt, leftAt
			}
			g.mouseLeftClicks = []queuedClick{{x: box.x1 + 3, y: box.y1 + 3, at: leftAt}}
			g.mouseRightClicks = []queuedClick{{x: box.x1 + 3, y: box.y1 + 3, at: rightAt}}
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			if g.saveRenameOpen != rightFirst || !rightFirst && g.mainMenuMode != MenuMain {
				t.Fatal("events ignored timestamp order or crossed the changed modal")
			}
			if len(g.mouseLeftClicks)+len(g.mouseRightClicks) != 0 {
				t.Fatal("stale second event survived the modal transition")
			}
		})
	}
}

func TestDisplayedModalUpdateAdapters(t *testing.T) {
	cases := []struct {
		name string
		run  func(*displayedModalHarness)
	}{
		{"combat log close", func(h *displayedModalHarness) {
			h.g.combatLogOpen = true
			x, y, w, _ := combatLogPanelLayout(h.g)
			h.clicks(false, x+w-25, y+13, 1)
			if h.g.combatLogOpen {
				h.t.Fatal("displayed combat log did not close")
			}
		}},
		{"combat log arrows", func(h *displayedModalHarness) {
			h.g.combatLogOpen = true
			h.g.combatLogHistory = make([]combatLogEntry, 30)
			x, y, w, height := combatLogPanelLayout(h.g)
			h.clicks(false, x+w-30, y+65, 1)
			if h.g.combatLogScroll != 3 {
				h.t.Fatal("up arrow was lost or replayed")
			}
			h.clicks(false, x+w-30, y+54+height-88-20, 1)
			if h.g.combatLogScroll != 0 {
				h.t.Fatal("down arrow was lost or replayed")
			}
		}},
		{"ESC root", func(h *displayedModalHarness) {
			h.g.mainMenuOpen = true
			w, height := menuPanelSize(MenuMain)
			x, y := (h.g.config.GetScreenWidth()-w)/2, (h.g.config.GetScreenHeight()-height)/2
			box, _, _ := menuRowRect(x, y, w, mainMenuListTopY, mainMenuRowPitch, 0)
			h.clicks(false, box.x1+4, box.y1+4, 1)
			if h.g.mainMenuOpen {
				h.t.Fatal("Resume click was not delivered")
			}
		}},
		{"save and load rows", func(h *displayedModalHarness) {
			g := h.g
			g.mainMenuOpen, g.mainMenuMode = true, MenuSaveSelect
			g.party.Gold = 321
			w, height := menuPanelSize(g.mainMenuMode)
			x, y := (g.config.GetScreenWidth()-w)/2, (g.config.GetScreenHeight()-height)/2
			box, _, _ := menuRowRect(x, y, w, saveMenuListTopY, saveMenuRowPitch, 1)
			h.clicks(false, box.x1+4, box.y1+4, 1)
			if !GetSaveRowSummary(1).Exists || g.mainMenuMode != MenuMain {
				h.t.Fatal("Save row did not write the chosen slot")
			}
			g.party.Gold = 999
			g.mainMenuMode = MenuLoadSelect
			h.clicks(false, box.x1+4, box.y1+4, 1)
			if g.party.Gold != 321 || g.mainMenuOpen {
				h.t.Fatal("Load row did not restore the chosen save")
			}
		}},
		{"save pager and rename", func(h *displayedModalHarness) {
			g := h.g
			if err := g.SaveGameToFile(saveRowPath(1)); err != nil {
				h.t.Fatal(err)
			}
			g.mainMenuOpen, g.mainMenuMode = true, MenuSaveSelect
			w, height := menuPanelSize(g.mainMenuMode)
			x, y := (g.config.GetScreenWidth()-w)/2, (g.config.GetScreenHeight()-height)/2
			prev, next := savePagerButtonRects(x, y, w, height)
			h.clicks(false, next.x1+2, next.y1+2, 1)
			if g.savePage != 1 {
				h.t.Fatal("Next page click was lost")
			}
			h.clicks(false, prev.x1+2, prev.y1+2, 1)
			if g.savePage != 0 {
				h.t.Fatal("Prev page click was lost")
			}
			box, _, _ := menuRowRect(x, y, w, saveMenuListTopY, saveMenuRowPitch, 1)
			g.mainMenuMode = MenuLoadSelect
			h.clicks(true, box.x1+4, box.y1+4, 1)
			if g.saveRenameOpen {
				h.t.Fatal("Load opened a hidden rename dialog")
			}
			g.mainMenuMode = MenuSaveSelect
			h.clicks(true, box.x1+4, box.y1+4, 1)
			if !g.saveRenameOpen || g.saveRenameSlot != 1 {
				h.t.Fatal("Save row rename click was lost")
			}
		}},
		{"single level choice", func(h *displayedModalHarness) {
			g := h.g
			skill, _ := twoOwnedSkills(h.t, g.party.Members[0])
			g.party.Members[0].Skills[skill].Mastery = character.MasteryNovice
			g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, options: []levelUpChoiceOption{{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: skill}}}}
			g.levelUpChoiceOpen = true
			x, _, w, _, y, height := levelUpChoiceLayout(g.currentLevelUpChoice(), g.config.GetScreenWidth(), g.config.GetScreenHeight())
			h.clicks(false, x+w/2, y+height/2, 2)
			if g.currentLevelUpChoice() != nil || g.party.Members[0].Skills[skill].Mastery != character.MasteryExpert {
				h.t.Fatal("level choice was lost or applied twice")
			}
		}},
		{"multi level choice", func(h *displayedModalHarness) {
			g := h.g
			first, second := twoOwnedSkills(h.t, g.party.Members[0])
			for _, skill := range []character.SkillType{first, second} {
				g.party.Members[0].Skills[skill].Mastery = character.MasteryNovice
			}
			g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, maxSelections: 2, selected: make([]bool, 2), options: []levelUpChoiceOption{
				{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: first},
				{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: second},
			}}}
			g.levelUpChoiceOpen = true
			x, _, w, _, y, height := levelUpChoiceLayout(g.currentLevelUpChoice(), g.config.GetScreenWidth(), g.config.GetScreenHeight())
			h.clicks(false, x+w/2, y+height/2, 1)
			if g.currentLevelUpChoice().selectedCount() != 1 {
				h.t.Fatal("first choice was not toggled")
			}
			h.clicks(false, x+w/2, y+height+height/2, 1)
			h.clicks(false, x+w/2, y+2*height+height/2, 1)
			if g.currentLevelUpChoice() != nil || g.party.Members[0].Skills[first].Mastery != character.MasteryExpert || g.party.Members[0].Skills[second].Mastery != character.MasteryExpert {
				h.t.Fatal("multi choice did not commit selected options")
			}
		}},
		{"merchant buy and sell", func(h *displayedModalHarness) {
			g := h.g
			g.dialogActive = true
			g.party.Gold = 100
			g.dialogNPC.MerchantStock = []*character.MerchantStockItem{potionStock(10, 5)}
			dlg := npcDialogLayout(g)
			lx, rx, top, _ := merchantGridLayout(dlg.x, dlg.y)
			x, y, _, _ := merchantCellRect(lx, top, 0)
			h.clicks(false, x+4, y+4, 2)
			if g.party.Gold != 90 || g.dialogNPC.MerchantStock[0].Quantity != 4 {
				h.t.Fatal("merchant double-click purchase was lost")
			}
			x, y, _, _ = merchantCellRect(rx, top, 0)
			h.clicks(false, x+4, y+4, 2)
			if g.party.Gold != 100 {
				h.t.Fatal("merchant double-click sale was lost")
			}
			if err := h.loop.Update(); err != nil {
				h.t.Fatal(err)
			}
			if g.party.Gold != 100 {
				h.t.Fatal("transaction replayed without another input event")
			}
		}},
		{"card collection", func(h *displayedModalHarness) {
			g := h.g
			g.dialogActive = true
			g.dialogNPC = &character.NPC{Name: "Collector", Type: character.NPCTypeCardCollector}
			g.party.Inventory = []items.Item{items.CreateItemFromYAML("puma_card")}
			dlg := npcDialogLayout(g)
			x, y, _, _ := cardCollectorInvRect(dlg.x, dlg.y, 0)
			h.clicks(false, x+4, y+4, 2)
			if g.cardSlots[0].key != "puma_card" || len(g.party.Inventory) != 0 {
				h.t.Fatal("card collection click did not slot the card")
			}
			x, y, _, _ = cardCollectorSlotRect(dlg.x, dlg.y, 0)
			h.clicks(false, x+4, y+4, 2)
			if g.cardSlots[0].key != "" || len(g.party.Inventory) != 1 {
				h.t.Fatal("card collection click did not return the card")
			}
		}},
		{"spell trader portrait and purchase", func(h *displayedModalHarness) {
			g := h.g
			g.dialogActive = true
			g.dialogNPC = &character.NPC{Name: "Teacher", SpellData: map[string]*character.NPCSpell{"firebolt": {Name: "Fire Bolt", Cost: 10}}}
			ch := g.party.Members[1]
			ch.MagicSchools[character.MagicSchoolFire] = &character.MagicSkill{Mastery: character.MasteryNovice}
			g.party.Gold = 100
			dlg := npcDialogLayout(g)
			x, y, _, _ := spellTraderPortraitRect(dlg.x, dlg.y, 1)
			h.clicks(false, x+4, y+4, 1)
			if g.selectedCharIdx != 1 {
				h.t.Fatal("trader portrait did not select its character")
			}
			x, y, _, _ = spellTraderIconRect(dlg.x, dlg.y, 0)
			h.clicks(false, x+4, y+4, 1) // Publish selected spell before its confirming click.
			h.clicks(false, x+4, y+4, 1)
			if g.party.Gold != 90 || !ch.KnowsSpell(spells.SpellID("firebolt")) {
				h.t.Fatal("trader did not teach the selected character")
			}
		}},
		{"trainer portrait and training", func(h *displayedModalHarness) {
			g := h.g
			g.dialogActive = true
			g.dialogNPC = &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer}
			g.party.Gold = 100000
			dlg := npcDialogLayout(g)
			x, y, _, _ := skillTrainerPortraitRect(dlg.x, dlg.y, dlg.w, 0)
			h.clicks(false, x+4, y+4, 1)
			if !g.skillTrainerPopup {
				h.t.Fatal("trainer portrait did not open its popup")
			}
			options := trainerOptions(g.party.Members[0])
			if len(options) == 0 {
				h.t.Fatal("fixture has no trainable skill")
			}
			before := g.party.Gold
			px, py, _, _ := skillTrainerPopupRect(dlg.x, dlg.y, dlg.w, dlg.h)
			x, y, _, _ = skillTrainerOptionRect(px, py, 0)
			h.clicks(false, x+4, y+4, 2)
			if g.party.Gold != before-options[0].Cost {
				h.t.Fatal("trainer option did not commit one training")
			}
			h.clicks(false, dlg.x+2, dlg.y+2, 1)
			if g.skillTrainerPopup || !g.dialogActive {
				h.t.Fatal("outside click did not return to the trainer portraits")
			}
		}},
	}
	for _, size := range [][2]int{{1024, 768}, {1280, 800}} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], tc.name), func(t *testing.T) { tc.run(newDisplayedModalHarness(t, size[0], size[1])) })
		}
	}
}

func TestDisplayedConversationSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    npcDialogKind
		tab     int
		visible bool
	}{
		{"conversation", dialogKindChoices, 0, true},
		{"gated merchant conversation", dialogKindMerchant, 0, true},
		{"trader talk", dialogKindSpellTrader, 1, true},
		{"trader shop hides talk", dialogKindSpellTrader, 0, false},
		{"service talk", dialogKindBuffService, 1, true},
		{"service rows hide talk", dialogKindBuffService, 0, false},
		{"arena talk", dialogKindArenaGladiator, 0, true},
		{"arena shop hides talk", dialogKindArenaGladiator, 1, false},
		{"arena board hides talk", dialogKindArenaGladiator, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			npc := &character.NPC{Name: "Guide", DialogueData: &character.NPCDialogue{Greeting: "Welcome", Choices: []*character.NPCDialogueChoice{
				{Text: "Tell me more", Action: "info", Response: "More information"},
				{Text: "Leave", Action: "leave"},
			}}}
			switch tc.kind {
			case dialogKindMerchant:
				npc.MerchantStock = []*character.MerchantStockItem{potionStock(10, 5)}
				npc.RequiresQuest = "uncompleted-test-quest"
			case dialogKindSpellTrader:
				npc.SpellData = map[string]*character.NPCSpell{"firebolt": {Name: "Fire Bolt", Cost: 10}}
			case dialogKindBuffService:
				npc.DialogueData.Choices = append(npc.DialogueData.Choices, &character.NPCDialogueChoice{Text: "Bless", Action: "cast_buff", Buff: "bless", Cost: 10})
			case dialogKindArenaGladiator:
				loadTestArenaData(t)
				npc.ArenaBoard = true
				npc.MerchantStock = []*character.MerchantStockItem{potionStock(10, 5)}
			}
			g.dialogActive, g.dialogNPC, g.dialogTab = true, npc, tc.tab
			if got := g.npcDialogKindFor(npc); got != tc.kind && !(tc.kind == dialogKindMerchant && got == dialogKindChoices) {
				t.Fatalf("fixture kind=%v", got)
			}
			dlg := npcDialogLayout(g)
			x, y, w, height := g.dialogueChoiceRect(npc, 0, dlg.x, dlg.y, dlg.w)
			if height == 0 {
				t.Fatal("fixture has no first choice")
			}
			h.clicks(false, x+w/2, y+height/2, 2)
			if (len(g.dialogNodePath) > 0) != tc.visible {
				t.Fatalf("conversation action executed=%v, visible=%v", len(g.dialogNodePath) > 0, tc.visible)
			}
		})
	}
}

func TestDisplayedTrainerPagerOwnsItsPopup(t *testing.T) {
	for _, covered := range []bool{false, true} {
		t.Run(fmt.Sprintf("covered=%v", covered), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.dialogActive, g.skillTrainerPopup = true, true
			g.dialogNPC = &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer}
			for _, school := range character.AllMagicSchools {
				g.party.Members[0].MagicSchools[school] = &character.MagicSkill{Mastery: character.MasteryNovice}
			}
			dlg := npcDialogLayout(g)
			px, py, _, height := skillTrainerPopupRect(dlg.x, dlg.y, dlg.w, dlg.h)
			if len(trainerOptions(g.party.Members[0])) <= skillTrainerPageSize(height) {
				t.Fatal("fixture needs two trainer pages")
			}
			if covered {
				g.levelUpChoiceOpen = true
				g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, options: []levelUpChoiceOption{{label: "Choose"}}}}
			}
			h.clicks(false, px+12+396-15, py+height-40, 1)
			want := 1
			if covered {
				want = 0
			}
			if g.skillTrainerPage != want {
				t.Fatalf("trainer page=%d, want %d", g.skillTrainerPage, want)
			}
		})
	}
}

func TestDisplayedModalAdapterRejectsStaleContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*MMGame)
	}{
		{"quantity", func(g *MMGame) { g.party.Inventory[0].Quantity++ }},
		{"replacement item", func(g *MMGame) { g.party.Inventory[0].InstanceID++ }},
		{"roster", func(g *MMGame) { g.party.Members[0], g.party.Members[1] = g.party.Members[1], g.party.Members[0] }},
		{"party", func(g *MMGame) { p := *g.party; g.party = &p }},
		{"world", func(g *MMGame) { g.world = newTestWorld(g.config) }},
		{"NPC", func(g *MMGame) { npc := *g.dialogNPC; g.dialogNPC = &npc }},
		{"page", func(g *MMGame) { g.merchantSellPage++ }},
		{"resize", func(g *MMGame) { g.config.Display.ScreenWidth += 100 }},
		{"nested modal", func(g *MMGame) { g.statPopupOpen = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.dialogActive = true
			h.ui.Draw(h.screen)
			dlg := npcDialogLayout(g)
			_, rx, top, _ := merchantGridLayout(dlg.x, dlg.y)
			x, y, _, _ := merchantCellRect(rx, top, 0)
			tc.change(g)
			beforeGold, beforeCount := g.party.Gold, g.party.Inventory[0].Count()
			g.mouseLeftClicks = []queuedClick{{x: x + 4, y: y + 4, at: time.Now().UnixMilli()}, {x: x + 4, y: y + 4, at: time.Now().UnixMilli()}}
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			if g.party.Gold != beforeGold || g.party.Inventory[0].Count() != beforeCount || len(g.mouseLeftClicks) != 0 {
				t.Fatal("stale merchant clicks executed or survived")
			}
		})
	}
}

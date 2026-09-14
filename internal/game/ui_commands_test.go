package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/threading"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
)

func commandTestUI(t *testing.T) (*MMGame, *UISystem, *ebiten.Image) {
	t.Helper()
	g, _ := newThiefTestGame(t)
	g.sprites = graphics.NewSpriteManager()
	g.appScreen = AppScreenInGame
	ui := NewUISystem(g)
	return g, ui, ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
}

func TestDisplayedInputDrawIsPureAndUpdateConsumesOnce(t *testing.T) {
	for _, setup := range []struct {
		name  string
		apply func(*MMGame, *UISystem)
	}{
		{"hud", func(g *MMGame, _ *UISystem) { g.showPartyStats = true }},
		{"inventory", func(g *MMGame, _ *UISystem) { g.menuOpen = true; g.currentTab = TabInventory }},
		{"spellbook", func(g *MMGame, _ *UISystem) { g.menuOpen = true; g.currentTab = TabSpellbook }},
		{"stat", func(g *MMGame, _ *UISystem) { g.statPopupOpen = true; g.statPopupCharIdx = 0 }},
		{"roster", func(g *MMGame, _ *UISystem) { g.rosterScreenOpen = true }},
	} {
		t.Run(setup.name, func(t *testing.T) {
			g, ui, screen := commandTestUI(t)
			g.party.Members[0].FreeStatPoints = 5
			setup.apply(g, ui)
			before, _ := json.Marshal(g.party)
			// A real queued press is present for every repeated Draw. In the stat case
			// it lands on the first + button, so the purity assertion has a live target.
			x := (g.config.GetScreenWidth()-340)/2 + 194
			y := (g.config.GetScreenHeight()-320)/2 + 90
			g.mouseLeftClicks = []queuedClick{{x: x, y: y, at: 1000}}
			for i := 0; i < 3; i++ {
				ui.Draw(screen)
			}
			after, _ := json.Marshal(g.party)
			if string(before) != string(after) {
				t.Fatal("Draw changed party inventory, roster, HP/SP, progression or equipment")
			}
			if len(g.mouseLeftClicks) != 1 {
				t.Fatal("Draw consumed the queued input")
			}
			if setup.name == "stat" {
				ui.dispatchDisplayedInput()
				if got := g.party.Members[0].FreeStatPoints; got != 4 {
					t.Fatalf("Update spent %d points, want one", 5-got)
				}
				ui.dispatchDisplayedInput()
				if g.party.Members[0].FreeStatPoints != 4 {
					t.Fatal("Update replayed a consumed click")
				}
			}
		})
	}
}

func TestDisplayedInputRejectsChangedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*MMGame, *UISystem)
	}{
		{"unchanged", func(*MMGame, *UISystem) {}},
		{"inventory reorder", func(g *MMGame, _ *UISystem) {
			g.party.Inventory[0], g.party.Inventory[1] = g.party.Inventory[1], g.party.Inventory[0]
		}},
		{"stack quantity", func(g *MMGame, _ *UISystem) { g.party.Inventory[0].Quantity++ }},
		{"slot binding", func(g *MMGame, _ *UISystem) { it := g.party.Inventory[1]; g.party.Members[0].QuickSlots[0] = &it }},
		{"actor selection", func(g *MMGame, _ *UISystem) { g.selectedChar = 1 }},
		{"roster replacement", func(g *MMGame, _ *UISystem) {
			g.party.Members[0], g.party.Members[1] = g.party.Members[1], g.party.Members[0]
		}},
		{"nested modal", func(g *MMGame, _ *UISystem) { g.statPopupOpen = true }},
		{"resize", func(g *MMGame, _ *UISystem) { g.config.Display.ScreenWidth++ }},
		{"world replacement", func(g *MMGame, _ *UISystem) { g.world = newTestWorldSized(g.config, 8, 8) }},
		{"party replacement", func(g *MMGame, _ *UISystem) { cp := *g.party; g.party = &cp }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, ui, _ := commandTestUI(t)
			g.menuOpen = true
			g.currentTab = TabInventory
			health, other := items.CreateItemFromYAML("health_potion"), items.CreateItemFromYAML("health_potion")
			health.InstanceID, health.Quantity = 11, 2
			other.InstanceID, other.Quantity = 12, 2
			g.party.Inventory = []items.Item{health, other}
			g.party.Members[0].HitPoints = 1
			ui.beginDisplayedInput()
			ui.handleInventoryItemClick(0, 10, 10, 30, 30)
			ui.endDisplayedInput()
			tc.change(g, ui)
			before, _ := json.Marshal(g.party)
			g.mouseLeftClicks = []queuedClick{{x: 20, y: 20, at: 1000}, {x: 20, y: 20, at: 1050}}
			ui.dispatchDisplayedInput()
			after, _ := json.Marshal(g.party)
			if tc.name == "unchanged" {
				if g.party.Inventory[0].Count() != 1 || g.party.Members[0].HitPoints <= 1 {
					t.Fatal("current displayed potion command did not heal and consume one unit")
				}
				return
			}
			if string(before) != string(after) || len(g.mouseLeftClicks) != 0 {
				t.Fatal("stale displayed item command executed or survived")
			}
		})
	}
}

func TestDisplayedDragRejectsChangedSourceAfterAnotherDraw(t *testing.T) {
	for _, kind := range []dragSource{dragFromInventory, dragFromEquip, dragFromQuickSlot} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			g, ui, _ := commandTestUI(t)
			g.menuOpen = true
			it := items.Item{Name: "first", Type: items.ItemWeapon, InstanceID: 1}
			other := items.Item{Name: "replacement", Type: items.ItemWeapon, InstanceID: 2}
			g.party.Inventory = []items.Item{it}
			g.party.Members[0].Equipment[items.SlotMainHand] = it
			g.party.Members[0].QuickSlots[0] = &it
			g.dragSrc, g.dragItem, g.dragActive = kind, it, true
			g.dragInvIndex, g.dragEquipChar, g.dragQuickChar, g.dragQuickSlot = 0, 0, 0, 0
			g.dragEquipSlot = items.SlotMainHand
			ui.captureDisplayedDrags()
			switch kind {
			case dragFromInventory:
				g.party.Inventory[0] = other
			case dragFromEquip:
				g.party.Members[0].Equipment[items.SlotMainHand] = other
			case dragFromQuickSlot:
				g.party.Members[0].QuickSlots[0] = &other
			}
			ui.beginDisplayedInput()
			ui.quickInvDropZone(10, 10, 40, 40)
			ui.endDisplayedInput()
			before, _ := json.Marshal(g.party)
			g.dragDropAt, g.dragCurX, g.dragCurY = 1, 20, 20
			ui.dispatchDisplayedInput()
			after, _ := json.Marshal(g.party)
			if string(before) != string(after) || g.dragSrc != dragNone {
				t.Fatal("redrawn layout retargeted the carried source")
			}
		})
	}
}

func TestDisplayedCommandsRespectEventOrderAndModalChange(t *testing.T) {
	g, ui, _ := commandTestUI(t)
	order := []int{}
	ui.beginDisplayedInput()
	// Reverse widget order deliberately: event time must own dispatch order.
	for _, x := range []int{20, 10} {
		ui.onDisplayedInput(uiCommandClick, layoutRect{x, 0, 5, 5}, func() {
			if g.consumeLeftClickIn(x, 0, x+5, 5) {
				order = append(order, x)
			}
		})
	}
	ui.endDisplayedInput()
	g.mouseLeftClicks = []queuedClick{{x: 10, y: 1, at: 1000}, {x: 20, y: 1, at: 1001}}
	ui.dispatchDisplayedInput()
	if len(order) != 2 || order[0] != 10 || order[1] != 20 {
		t.Fatalf("event order = %v", order)
	}
	ui.dispatchDisplayedInput()
	if len(order) != 2 {
		t.Fatal("click was delivered twice")
	}
}

func TestDisplayedMerchantConfirmationTransactsOnlyInUpdate(t *testing.T) {
	for _, buy := range []bool{false, true} {
		t.Run(fmt.Sprintf("buy=%v", buy), func(t *testing.T) {
			g, ui := merchantDragGame(t)
			g.appScreen = AppScreenInGame
			g.party.Gold = 100
			source := stackSplitPickerMerchantSell
			item := g.party.Inventory[0]
			if buy {
				g.dialogNPC.MerchantStock = []*character.MerchantStockItem{potionStock(10, 5)}
				source = stackSplitPickerMerchantBuy
				item = g.dialogNPC.MerchantStock[0].Item
			}
			ui.openStackSplitPicker(source, 0, item)
			ui.stackSplitPicker.quantity = 2
			screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
			r := stackSplitPickerLayout(stackSplitPickerRect(screen.Bounds().Dx(), screen.Bounds().Dy())).take
			g.mouseLeftClicks = []queuedClick{{x: r.Min.X + 2, y: r.Min.Y + 2, at: 1000}}
			before, _ := json.Marshal(g.party)
			ui.Draw(screen)
			ui.Draw(screen)
			after, _ := json.Marshal(g.party)
			if string(before) != string(after) {
				t.Fatal("Draw executed a merchant transaction")
			}
			ui.dispatchDisplayedInput()
			wantGold, wantCount := 120, 3
			if buy {
				wantGold, wantCount = 80, 7
			}
			if g.party.Gold != wantGold || g.party.Inventory[0].Count() != wantCount {
				t.Fatalf("gold/count = %d/%d, want %d/%d", g.party.Gold, g.party.Inventory[0].Count(), wantGold, wantCount)
			}
			ui.dispatchDisplayedInput()
			if g.party.Gold != wantGold || g.party.Inventory[0].Count() != wantCount {
				t.Fatal("merchant transaction replayed")
			}
		})
	}
}

func TestGameLoopDispatchesDisplayedCommandBeforePause(t *testing.T) {
	g, ui, screen := commandTestUI(t)
	g.statPopupOpen = true
	g.statPopupCharIdx = 0
	g.party.Members[0].FreeStatPoints = 5
	g.threading = threading.NewThreadingComponents(g.config)
	t.Cleanup(g.threading.Shutdown)
	loop := &GameLoop{game: g, inputHandler: NewInputHandler(g), ui: ui}
	g.gameLoop = loop
	ui.Draw(screen)
	x := (g.config.GetScreenWidth()-340)/2 + 194
	y := (g.config.GetScreenHeight()-320)/2 + 90
	g.mouseLeftClicks = []queuedClick{{x: x, y: y, at: time.Now().UnixMilli()}}
	beforeFrame := g.frameCount
	if err := loop.Update(); err != nil {
		t.Fatal(err)
	}
	if g.party.Members[0].FreeStatPoints != 4 {
		t.Fatal("paused GameLoop.Update did not dispatch the displayed stat command")
	}
	if g.frameCount != beforeFrame {
		t.Fatal("UI command advanced the paused world")
	}
}

func TestDisplayedStatHoldKeepsItsActorAndLayout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*MMGame)
	}{
		{"unchanged", func(*MMGame) {}},
		{"popup actor", func(g *MMGame) { g.statPopupCharIdx = 1 }},
		{"roster replacement", func(g *MMGame) {
			g.party.Members[0], g.party.Members[1] = g.party.Members[1], g.party.Members[0]
		}},
		{"resize", func(g *MMGame) { g.config.Display.ScreenWidth += 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, ui, screen := commandTestUI(t)
			g.statPopupOpen = true
			g.statPopupCharIdx = 0
			g.party.Members[0].FreeStatPoints = 5
			g.party.Members[1].FreeStatPoints = 5
			fp := installFakePointer(t)
			fp.moveTo((g.config.GetScreenWidth()-340)/2+194, (g.config.GetScreenHeight()-320)/2+90)
			fp.hold()
			ui.Draw(screen)
			ui.dispatchDisplayedInput()
			for i := 0; i < statHoldInitialDelay+statHoldRepeatRate-1; i++ {
				ui.dispatchDisplayedInput()
			}
			tc.change(g)
			// A new Draw must not transfer the old gesture timer to new content.
			ui.Draw(screen)
			ui.dispatchDisplayedInput()
			want := 10
			if tc.name == "unchanged" {
				want--
			}
			if got := g.party.Members[0].FreeStatPoints + g.party.Members[1].FreeStatPoints; got != want {
				t.Fatalf("remaining points=%d, want %d", got, want)
			}
		})
	}
}

package game

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
	"ugataima/internal/threading"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

type displayedModalHarness struct {
	t         *testing.T
	g         *MMGame
	ui        *UISystem
	loop      *GameLoop
	screen    *ebiten.Image
	clickTime int64
}

func newDisplayedModalHarness(t *testing.T, width, height int) *displayedModalHarness {
	t.Helper()
	g, ui := merchantDragGame(t)
	// Pointer scenarios place world rewards beyond the merchant's tiny room.
	g.world = newTestWorldSized(g.config, 40, 40)
	g.collisionSystem.UpdateTileChecker(g.world)
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
	return &displayedModalHarness{t: t, g: g, ui: ui, loop: loop, screen: screen}
}

// Each batch starts with the real displayed layout, then uses the same Update
// entry point as a paused game, with no modal events left for later handlers.
func (h *displayedModalHarness) clicks(right bool, x, y, count int) {
	h.t.Helper()
	h.clickTime += 100
	h.clicksAt(right, x, y, count, h.clickTime)
	h.clickTime += int64(count)
}

// Gesture tests supply event time independently of rendering and scheduling.
func (h *displayedModalHarness) clicksAt(right bool, x, y, count int, now int64) {
	h.t.Helper()
	h.ui.Draw(h.screen)
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
	if err := h.loop.Update(); err != nil {
		h.t.Fatal(err)
	}
}

func TestDisplayedPointerGesturesReachUpdate(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		loading bool
	}{
		{"quick slot", false}, {"quick slot", true},
		{"standalone stash", false}, {"standalone stash", true},
		{"tavern stash", false}, {"tavern stash", true},
		{"merchant", false}, {"merchant", true},
	} {
		kind := tc.kind
		for _, outside := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/loading=%v/outside=%v", kind, tc.loading, outside), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				fp := installFakePointer(t)
				var sx, sy, dx, dy int
				switch kind {
				case "quick slot":
					g.menuOpen, g.currentTab = true, TabInventory
					menu := computeTabbedMenuLayout(1024, gameplayViewportBottom(g))
					layout := computeInventoryContentLayout(menu.content, 1)
					x, y, w, height := scaleInventorySourceRect(layout.grid.x, layout.grid.y, layout.grid.w, layout.grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
					sx, sy = x+w/2, y+height/2
					_, slots := quickSlotRects(layout.quickSlots.x, layout.quickSlots.y, layout.quickSlots.w)
					dx, dy = slots[0].Min.X+3, slots[0].Min.Y+3
				case "standalone stash", "tavern stash":
					g.stash = &stash.Stash{}
					layout := openStashHost(g, kind == "tavern stash")
					src, dst := stashGridCell(layout.bag, layout.bagCols, 0), stashGridCell(layout.chest, stashChestCols, 0)
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
				if tc.loading {
					h.loop.renderer = &Renderer{game: g, mapRenderResourcesByMap: map[string]*mapRenderRegionResources{currentMapKey(): {}}}
					h.loop.ensureResourceLoading()
					t.Cleanup(h.loop.closeResourceLoading)
					h.loop.loading.begin(time.Now())
					// The same retirement runs when a speculative Draw misses a
					// resource, before the next paused Update has sampled input.
					h.loop.discardLoadingInput()
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
					h.loop.closeResourceLoading()
					h.loop.renderer = nil
				}
				fp.release()
				h.pointerStep()
				if g.dragSrc != dragNone || g.stashDrag.active || g.stashDragDrop {
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

// openStashHost shows the stash in the tavern's Stash tab or the standalone
// modal and returns the grids it draws.
func openStashHost(g *MMGame, tavern bool) stashLayout {
	if !tavern {
		g.stashScreenOpen = true
		_, layout := computeStashLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight())
		return layout
	}
	g.dialogActive, g.dialogNPC, g.dialogTab = true, tavernTestNPC(), 1
	dlg := npcDialogLayout(g)
	return computeStashLayoutForArea(tavernContentRect(dlg.x, dlg.y, dlg.w, dlg.h))
}

// A double-click moves a cell across through the real press/release path in
// both hosts: a bag stack onto its chest stack or else the first free cell, a
// chest item into the bag. One click, or a full chest, moves nothing.
func TestStashDoubleClickMovesItemsAcross(t *testing.T) {
	potion := func(n int) items.Item {
		it := items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: n}
		items.EnsureInstanceID(&it)
		return it
	}
	sword := func() items.Item {
		it := items.Item{Name: "Iron Sword", Type: items.ItemWeapon}
		items.EnsureInstanceID(&it)
		return it
	}
	full := map[int]items.Item{}
	fullWant := map[int]string{}
	for i := range stash.SlotCount {
		full[i], fullWant[i] = sword(), "Iron Sword x1"
	}
	for _, tc := range []struct {
		name      string
		bag       []items.Item
		chest     map[int]items.Item
		fromChest int // -1 clicks bag cell 0
		clicks    int
		wantBag   int            // units left in the bag
		wantChest map[int]string // filled cells afterwards
	}{
		{"bag stack joins its chest stack", []items.Item{potion(3)}, map[int]items.Item{0: sword(), 5: potion(2)}, -1, 2, 0, map[int]string{0: "Iron Sword x1", 5: "Health Potion x5"}},
		{"bag item takes the first free cell", []items.Item{sword()}, map[int]items.Item{0: potion(2)}, -1, 2, 0, map[int]string{0: "Health Potion x2", 1: "Iron Sword x1"}},
		{"chest item goes to the bag", nil, map[int]items.Item{7: potion(4)}, 7, 2, 4, map[int]string{}},
		{"one click moves nothing", []items.Item{sword()}, nil, -1, 1, 1, map[int]string{}},
		{"a full chest refuses", []items.Item{potion(1)}, full, -1, 2, 1, fullWant},
	} {
		for _, tavern := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tavern=%v", tc.name, tavern), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				fp := installFakePointer(t)
				g.party.Inventory = append([]items.Item(nil), tc.bag...)
				g.stash = &stash.Stash{}
				for i, it := range tc.chest {
					g.stash.Slots[i] = it
				}
				layout := openStashHost(g, tavern)
				cell := stashGridCell(layout.bag, layout.bagCols, 0)
				if tc.fromChest >= 0 {
					cell = stashGridCell(layout.chest, stashChestCols, tc.fromChest)
				}
				fp.moveTo(cell.Min.X+cell.Dx()/2, cell.Min.Y+cell.Dy()/2)
				for range tc.clicks {
					fp.press()
					h.pointerStep()
					fp.release()
					h.pointerStep()
				}
				units := 0
				for _, it := range g.party.Inventory {
					units += it.Count()
				}
				got := map[int]string{}
				for i, it := range g.stash.Slots {
					if !stash.IsEmpty(it) {
						got[i] = fmt.Sprintf("%s x%d", it.Name, it.Count())
					}
				}
				if units != tc.wantBag || !reflect.DeepEqual(got, tc.wantChest) {
					t.Fatalf("bag holds %d units, chest %v; want %d and %v", units, got, tc.wantBag, tc.wantChest)
				}
			})
		}
	}
}

// The placement rule in the stash, through real gestures in both hosts: a
// drop onto a matching stack merges, an empty cell keeps a stack of its own,
// a whole stack trades cells with another item (a displaced item takes the
// source cell), and part of a stack (a right-click split, or one unit with
// Shift) never displaces anything. Each row starts from bag [P6 S] and chest
// {3: P2, 5: D}: P potions, S a sword, D a dagger.
func TestStashDragsKeepFreeStacks(t *testing.T) {
	potion := func(n int) items.Item {
		it := items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: n}
		items.EnsureInstanceID(&it)
		return it
	}
	gear := func(name string) items.Item {
		it := items.Item{Name: name, Type: items.ItemWeapon}
		items.EnsureInstanceID(&it)
		return it
	}
	label := func(it items.Item) string {
		if it.Stackable() {
			return fmt.Sprintf("P%d", it.Count())
		}
		return strings.Fields(it.Name)[1][:1] // Iron Sword: S
	}
	type cell struct {
		chest bool
		idx   int
	}
	unchangedBag, unchangedChest := []string{"P6", "S"}, map[int]string{3: "P2", 5: "D"}
	for _, tc := range []struct {
		name      string
		drag      [2]cell // from, to; split rows right-click the source instead
		split     bool
		shift     bool // Shift held: the drag carries one unit
		wantBag   []string
		wantChest map[int]string
	}{
		{"bag stack to an empty chest cell stays apart", [2]cell{{false, 0}, {true, 10}}, false, false, []string{"S"}, map[int]string{3: "P2", 5: "D", 10: "P6"}},
		{"bag stack onto its chest stack merges", [2]cell{{false, 0}, {true, 3}}, false, false, []string{"S"}, map[int]string{3: "P8", 5: "D"}},
		{"chest stack to an empty bag cell stays apart", [2]cell{{true, 3}, {false, 2}}, false, false, []string{"P6", "S", "P2"}, map[int]string{5: "D"}},
		{"chest stack onto its bag stack merges", [2]cell{{true, 3}, {false, 0}}, false, false, []string{"P8", "S"}, map[int]string{5: "D"}},
		{"bag split to an empty bag cell", [2]cell{{false, 0}, {false, 2}}, true, false, []string{"P3", "S", "P3"}, unchangedChest},
		{"bag split onto the chest stack", [2]cell{{false, 0}, {true, 3}}, true, false, []string{"P3", "S"}, map[int]string{3: "P5", 5: "D"}},
		{"shift-drag to an empty bag cell", [2]cell{{false, 0}, {false, 2}}, false, true, []string{"P5", "S", "P1"}, unchangedChest},
		{"shift-drag onto the chest stack", [2]cell{{false, 0}, {true, 3}}, false, true, []string{"P5", "S"}, map[int]string{3: "P3", 5: "D"}},
		{"shift-drag to an empty chest cell", [2]cell{{true, 3}, {true, 10}}, false, true, unchangedBag, map[int]string{3: "P1", 5: "D", 10: "P1"}},
		{"bag stack onto another chest item swaps", [2]cell{{false, 0}, {true, 5}}, false, false, []string{"D", "S"}, map[int]string{3: "P2", 5: "P6"}},
		{"chest stack onto another bag item swaps", [2]cell{{true, 3}, {false, 1}}, false, false, []string{"P6", "P2"}, map[int]string{3: "S", 5: "D"}},
		{"chest stack onto another chest item swaps", [2]cell{{true, 3}, {true, 5}}, false, false, unchangedBag, map[int]string{3: "D", 5: "P2"}},
		{"bag stack onto another bag item swaps", [2]cell{{false, 0}, {false, 1}}, false, false, []string{"S", "P6"}, unchangedChest},
		{"split onto another chest item moves nothing", [2]cell{{false, 0}, {true, 5}}, true, false, unchangedBag, unchangedChest},
		{"shift-drag onto another bag item moves nothing", [2]cell{{false, 0}, {false, 1}}, false, true, unchangedBag, unchangedChest},
		{"shift-drag from the chest onto a bag item moves nothing", [2]cell{{true, 3}, {false, 1}}, false, true, unchangedBag, unchangedChest},
	} {
		for _, tavern := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tavern=%v", tc.name, tavern), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				fp := installFakePointer(t)
				g.party.Inventory = []items.Item{potion(6), gear("Iron Sword")}
				g.stash = &stash.Stash{}
				g.stash.Slots[3], g.stash.Slots[5] = potion(2), gear("Iron Dagger")
				layout := openStashHost(g, tavern)
				at := func(c cell) (int, int) {
					r := stashGridCell(layout.bag, layout.bagCols, c.idx)
					if c.chest {
						r = stashGridCell(layout.chest, stashChestCols, c.idx)
					}
					return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
				}
				x0, y0 := at(tc.drag[0])
				x1, y1 := at(tc.drag[1])
				if tc.split {
					fp.moveTo(x0, y0)
					fp.rightPress()
					h.pointerStep()
					fp.idle()
					if !h.ui.stackSplitPicker.open {
						t.Fatal("right-click opened no split picker")
					}
					h.ui.stackSplitConfirm() // the default half: 3 of 6
					fp.moveTo(x1, y1)
					fp.press()
					h.pointerStep()
					fp.release()
					h.pointerStep()
				} else {
					fp.moveTo(x0, y0)
					fp.shift = tc.shift
					fp.press()
					h.pointerStep()
					fp.hold()
					fp.moveTo((x0+x1)/2, (y0+y1)/2)
					h.pointerStep()
					fp.moveTo(x1, y1)
					h.pointerStep()
					fp.release()
					h.pointerStep()
				}
				fp.idle()
				h.pointerStep()
				var bag []string
				for _, it := range g.party.Inventory {
					bag = append(bag, label(it))
				}
				chest := map[int]string{}
				for i, it := range g.stash.Slots {
					if !stash.IsEmpty(it) {
						chest[i] = label(it)
					}
				}
				if !reflect.DeepEqual(bag, tc.wantBag) || !reflect.DeepEqual(chest, tc.wantChest) {
					t.Fatalf("bag %v chest %v; want %v and %v", bag, chest, tc.wantBag, tc.wantChest)
				}
				refused := slices.Contains(g.GetCombatMessages(), uitext.Text("stack.part_needs_room"))
				if refused != strings.HasSuffix(tc.name, "moves nothing") {
					t.Fatalf("refusal message shown=%v", refused)
				}
			})
		}
	}
}

func TestSpellbookDragAcrossPresentationStalls(t *testing.T) {
	for _, scenario := range []string{"normal", "catch_up", "loading_armed", "loading_active", "loading_release", "loading_new_press", "loading_owner_changed", "loading_closed", "loading_tab_changed", "loading_resized"} {
		t.Run(scenario, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.menuOpen, g.currentTab, g.selectedChar = true, TabSpellbook, 0
			caster := g.party.Members[0]
			caster.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
			caster.LearnSpell("firebolt")
			g.selectedSchool, g.selectedSpell = 0, -1
			bl := computeBookLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content, 1)
			x, y := bl.cardPos(0)
			_, slots := quickSlotRects(bl.quick.x, bl.quick.y, bl.quick.w)
			dx, dy := slots[0].Min.X+3, slots[0].Min.Y+3
			fp := installFakePointer(t)
			fp.moveTo(x+bl.iconSize/2, y+bl.iconSize/2)
			fp.press()
			h.pointerStep()
			if g.dragSrc != dragFromSpell || g.dragSpellID != "firebolt" {
				t.Fatal("spellbook press did not capture the displayed spell")
			}
			fp.hold()
			wantDrop := true
			if scenario == "catch_up" {
				for i := 0; i < 3; i++ {
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
				}
			} else if scenario != "normal" {
				if scenario != "loading_armed" {
					fp.moveTo(dx, dy)
					h.pointerStep()
				}
				// Enter the real loading Update gate without a worker or sleeps:
				// awaitingFrame stays set until the complete presentation returns.
				h.loop.renderer = &Renderer{game: g, mapRenderResourcesByMap: map[string]*mapRenderRegionResources{currentMapKey(): {}}}
				h.loop.ensureResourceLoading()
				t.Cleanup(h.loop.closeResourceLoading)
				h.loop.loading.begin(time.Now())
				switch scenario {
				case "loading_release":
					fp.release()
					wantDrop = false
				case "loading_new_press":
					fp.press()
					wantDrop = false
				case "loading_owner_changed":
					g.selectedChar = 1
					wantDrop = false
				case "loading_closed":
					g.menuOpen = false
					wantDrop = false
				case "loading_tab_changed":
					g.currentTab = TabInventory
					wantDrop = false
				case "loading_resized":
					g.config.Display.ScreenWidth++
					wantDrop = false
				}
				for i := 0; i < 3; i++ {
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
					if scenario == "loading_release" {
						fp.idle()
					} else {
						fp.hold()
					}
				}
				h.loop.closeResourceLoading()
				h.loop.renderer = nil
				g.menuOpen, g.selectedChar, g.currentTab = true, 0, TabSpellbook
				g.config.Display.ScreenWidth = 1024
			}
			fp.moveTo(dx, dy)
			h.pointerStep()
			fp.release()
			h.pointerStep()
			got := caster.QuickSlots[0]
			if wantDrop && (got == nil || got.SpellEffect != "firebolt") {
				t.Fatal("held spell drag was lost across a presentation stall")
			}
			if !wantDrop && got != nil {
				t.Fatal("cancelled gesture replayed a drop after loading")
			}
			if g.drag.armed || g.drag.active || g.dragDropAt != 0 {
				t.Fatal("released gesture retained drag state")
			}
		})
	}
}

func TestDisplayedHeldStatRepeatsAndStops(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	h.g.statPopupOpen, h.g.statPopupCharIdx = true, 0
	ch := h.g.party.Members[0]
	ch.FreeStatPoints = 5
	fp := installFakePointer(t)
	fp.moveTo(firstStatPlusPoint(h.g))
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
				roster := tavernRosterRect(area)
				activeRow, reserveRow := rosterRowRect(roster, false, 0), rosterRowRect(roster, true, 0)
				h.clicks(false, activeRow.x+3, activeRow.y+3, 1)
				h.clicks(false, reserveRow.x+3, reserveRow.y+3, 1)
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
			if err := g.SaveGameToFile(saveRowPath(firstManualRow)); err != nil {
				t.Fatal(err)
			}
			g.mainMenuOpen, g.mainMenuMode = true, MenuSaveSelect
			r := escSaveListLayout(1024, 768).rows[firstManualRow]
			box := pagerRect{r.x, r.y, r.right(), r.bottom()}
			h.ui.Draw(h.screen)
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
			closeBtn := makeCombatLogLayout(h.g).close
			h.clicks(false, closeBtn.x+closeBtn.w/2, closeBtn.y+closeBtn.h/2, 1)
			if h.g.combatLogOpen {
				h.t.Fatal("displayed combat log did not close")
			}
		}},
		{"combat log arrows", func(h *displayedModalHarness) {
			h.g.combatLogOpen = true
			h.g.combatLogHistory = make([]combatLogEntry, 30)
			l := makeCombatLogLayout(h.g)
			h.clicks(false, l.up.x+l.up.w/2, l.up.y+l.up.h/2, 1)
			if h.g.combatLogScroll != 3 {
				h.t.Fatal("up arrow was lost or replayed")
			}
			h.clicks(false, l.down.x+l.down.w/2, l.down.y+l.down.h/2, 1)
			if h.g.combatLogScroll != 0 {
				h.t.Fatal("down arrow was lost or replayed")
			}
		}},
		{"ESC root", func(h *displayedModalHarness) {
			h.g.mainMenuOpen = true
			w, height := menuPanelSize(MenuMain, 1024, 768)
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
			r := escSaveListLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight()).rows[firstManualRow]
			box := pagerRect{r.x, r.y, r.right(), r.bottom()}
			h.clicks(false, box.x1+4, box.y1+4, 1)
			if !GetSaveRowSummary(firstManualRow).Exists || g.mainMenuMode != MenuMain {
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
			if err := g.SaveGameToFile(saveRowPath(firstManualRow)); err != nil {
				h.t.Fatal(err)
			}
			g.mainMenuOpen, g.mainMenuMode = true, MenuSaveSelect
			l := escSaveListLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight())
			h.clicks(false, l.next.x+2, l.next.y+2, 1)
			if g.savePage != 1 {
				h.t.Fatal("Next page click was lost")
			}
			h.clicks(false, l.prev.x+2, l.prev.y+2, 1)
			if g.savePage != 0 {
				h.t.Fatal("Prev page click was lost")
			}
			r := l.rows[firstManualRow]
			box := pagerRect{r.x, r.y, r.right(), r.bottom()}
			g.mainMenuMode = MenuLoadSelect
			h.clicks(true, box.x1+4, box.y1+4, 1)
			if g.saveRenameOpen {
				h.t.Fatal("Load opened a hidden rename dialog")
			}
			g.mainMenuMode = MenuSaveSelect
			h.clicks(true, box.x1+4, box.y1+4, 1)
			if !g.saveRenameOpen || g.saveRenameSlot != firstManualRow {
				h.t.Fatal("Save row rename click was lost")
			}
		}},
		{"single level choice", func(h *displayedModalHarness) {
			g := h.g
			skill, _ := twoOwnedSkills(h.t, g.party.Members[0])
			g.party.Members[0].Skills[skill].Mastery = character.MasteryNovice
			g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, options: []levelUpChoiceOption{{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: skill}}}}
			g.levelUpChoiceOpen = true
			row := makeLevelUpChoicePanel(g.currentLevelUpChoice(), g.config.GetScreenWidth(), g.config.GetScreenHeight()).target(0)
			h.clicks(false, row.x+row.w/2, row.y+row.h/2, 2)
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
			p := makeLevelUpChoicePanel(g.currentLevelUpChoice(), g.config.GetScreenWidth(), g.config.GetScreenHeight())
			click := func(i int) { r := p.target(i); h.clicks(false, r.x+r.w/2, r.y+r.h/2, 1) }
			click(0)
			if g.currentLevelUpChoice().selectedCount() != 1 {
				h.t.Fatal("first choice was not toggled")
			}
			click(1)
			click(2) // the Confirm button
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
			g.dialogNPC = &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer, Training: map[string]int{"expert": 1000, "master": 4000}}
			g.party.Gold = 100000
			dlg := npcDialogLayout(g)
			x, y, _, _ := skillTrainerPortraitRect(dlg.x, dlg.y, dlg.w, 0)
			h.clicks(false, x+4, y+4, 1)
			if !g.skillTrainerPopup {
				h.t.Fatal("trainer portrait did not open its popup")
			}
			options := trainerOptions(g.party.Members[0], g.dialogNPC)
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
			g.dialogNPC = &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer, Training: map[string]int{"expert": 1000, "master": 4000}}
			for _, school := range character.AllMagicSchools {
				g.party.Members[0].MagicSchools[school] = &character.MagicSkill{Mastery: character.MasteryNovice}
			}
			dlg := npcDialogLayout(g)
			px, py, pw, height := skillTrainerPopupRect(dlg.x, dlg.y, dlg.w, dlg.h)
			if len(trainerOptions(g.party.Members[0], g.dialogNPC)) <= skillTrainerPageSize(height) {
				t.Fatal("fixture needs two trainer pages")
			}
			if covered {
				g.levelUpChoiceOpen = true
				g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, options: []levelUpChoiceOption{{label: "Choose"}}}}
			}
			pager := makeSkillTrainerPopupLayout(px, py, pw, height).pager
			_, next := pagerButtonRects(pager.x, pager.y, pager.w)
			h.clicks(false, next.x+next.w/2, next.y+next.h/2, 1)
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

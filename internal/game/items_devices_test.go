package game

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func TestTranslocatorUse(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"ready", "recharging", "rooted", "wall", "arena boundary", "unconscious", "no action", "unsupported action", "missing spell"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, state), func(t *testing.T) {
				g, ts := summonTileWorld(t)
				setTestWorldManager(t, nil)
				g.turnBasedMode, g.currentTurn = tb, 0
				g.snapFacing(0)
				hero := g.party.Members[0]
				hero.SpellPoints = 0
				hero.ActionsRemaining = 1
				carryInQuickSlot(t, g, 0, 0, items.CreateItemFromYAML("translocator"))
				it := hero.QuickSlots[0]
				switch state {
				case "recharging":
					it.DeviceCooldownFrames = 4 * g.config.GetTPS()
				case "rooted":
					g.partyRoot = PartyRootState{Frames: 60, Turns: 1}
				case "wall":
					g.world.Tiles[10][11] = world.TileWall
				case "arena boundary":
					g.collisionSystem.GetEntityByID("player").WithMovementBounds(collision.MovementBounds{Enabled: true, MinX: 5 * ts, MinY: 5 * ts, MaxX: 10 * ts, MaxY: 15 * ts})
				case "unconscious":
					hero.HitPoints = 0
				case "no action":
					hero.ActionsRemaining, hero.RTCooldown = 0, 60
				case "unsupported action":
					it.UseAction = "unimplemented"
				case "missing spell":
					it.SpellEffect = "missing"
				}
				x, y, before := g.camera.X, g.camera.Y, it.DeviceCooldownFrames
				beforeAction := hero.RTCooldown
				g.useQuickSlot(0, 0)
				if hero.QuickSlots[0] != it || it.Count() != 1 || hero.SpellPoints != 0 {
					t.Fatal("device was consumed, replaced or charged spell points")
				}
				if state != "ready" {
					if g.camera.X != x || g.camera.Y != y || it.DeviceCooldownFrames != before || g.currentTurn != 0 || hero.RTCooldown != beforeAction {
						t.Fatal("rejected device use moved, spent cooldown or ended turn")
					}
					if len(g.combatLogHistory) == 0 {
						t.Fatal("rejected device use gave no feedback")
					}
					return
				}
				if math.Abs(g.camera.X-x-3*ts) > .01 || g.camera.Y != y || it.DeviceCooldownFrames != 10*g.config.GetTPS() {
					t.Fatalf("jump=(%v,%v), recharge=%v", g.camera.X-x, g.camera.Y-y, it.DeviceCooldownFrames)
				}
				if hero.RTCooldown <= 0 || g.canSpendCombatAction(0) {
					t.Fatal("successful jump did not spend the hero action")
				}
				if tb && (g.currentTurn != 1 || hero.ActionsRemaining != 0) {
					t.Fatal("turn-based jump failed to end the party turn")
				}
				if tb && g.turnBasedExtraMonsterAction {
					t.Fatal("opening with a jump granted an extra monster action")
				}
				landed := g.camera.X
				g.currentTurn, hero.ActionsRemaining, hero.RTCooldown = 0, 1, 0
				g.useQuickSlot(0, 0)
				if g.camera.X != landed || it.DeviceCooldownFrames != 10*g.config.GetTPS() {
					t.Fatal("second use bypassed recharge")
				}
			})
		}
	}
}

func TestTranslocatorDisplayedInventory(t *testing.T) {
	for _, personal := range []bool{false, true} {
		t.Run(fmt.Sprint(personal), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.menuOpen, g.currentTab = true, TabInventory
			g.turnBasedMode = false
			g.snapFacing(0)
			hero := g.party.Members[0]
			g.party.Inventory, hero.Inventory = nil, nil
			var owner *character.MMCharacter
			if personal {
				owner = hero
			}
			g.party.Bag(owner).Add(items.CreateItemFromYAML("translocator"))
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
			grid := l.grid
			if personal {
				grid = l.personalGrid
			}
			x, y := personalBagCell(grid, 0)
			before := g.camera.X
			h.clicks(false, x, y, 2)
			bag := g.party.Bag(owner).Items()
			if g.camera.X != before+3*g.config.GetTileSize() || len(bag) != 1 || bag[0].DeviceCooldownFrames != 10*g.config.GetTPS() {
				t.Fatal("displayed double click did not use the device in its source bag")
			}
			for i := 0; i < 10; i++ {
				if err := h.loop.Update(); err != nil {
					t.Fatal(err)
				}
			}
			if bag[0].DeviceCooldownFrames != 10*g.config.GetTPS() {
				t.Fatal("paused inventory drained gameplay recharge")
			}
		})
	}
}

func TestTranslocatorClockAndSave(t *testing.T) {
	for _, mode := range []string{"real time", "turn based", "mixed", "round boundary"} {
		t.Run(mode, func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			g.turnBasedMode = false
			g.snapFacing(0)
			g.party.Inventory = []items.Item{items.CreateItemFromYAML("translocator")}
			if !g.useDeviceFromInventory(0, 0) {
				t.Fatal("initial use failed")
			}
			g.dragSrc, g.dragInvIndex = dragFromInventory, 0
			g.resolveQuickSlotDrop(0, 0)
			if len(g.party.Inventory) != 0 || g.party.Members[0].QuickSlots[0].DeviceCooldownFrames != 10*g.config.GetTPS() {
				t.Fatal("moving to quick slot reset or duplicated device")
			}
			gl := &GameLoop{game: g}
			remaining := func() float64 {
				return float64(g.party.Members[0].QuickSlots[0].DeviceCooldownFrames) / float64(g.config.GetTPS())
			}
			if mode != "turn based" {
				seconds := 4
				if mode == "round boundary" {
					seconds = 1
				}
				for i := 0; i < seconds*g.config.GetTPS(); i++ {
					gl.updateSpecialEffects()
				}
				if remaining() != float64(10-seconds) {
					t.Fatalf("%d seconds left recharge %v", seconds, remaining())
				}
				if mode == "round boundary" {
					rows := GetItemTooltipRows(*g.party.Members[0].QuickSlots[0], g.party.Members[0], g.combat, true)
					if !strings.Contains(rows.String(), "3 rounds") {
						t.Fatalf("nine seconds should show three rounds: %v", rows)
					}
				}
			}
			save := g.buildSave(wm)
			raw, err := json.Marshal(save)
			if err != nil {
				t.Fatal(err)
			}
			var decoded GameSave
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			before := remaining()
			if err := g.applySave(wm, &decoded); err != nil {
				t.Fatal(err)
			}
			if remaining() != before {
				t.Fatal("save/load changed recharge")
			}
			if mode == "real time" {
				for i := 0; i < 6*g.config.GetTPS()-1; i++ {
					gl.updateSpecialEffects()
				}
				if remaining() <= 0 {
					t.Fatal("recharged early")
				}
				gl.updateSpecialEffects()
			} else {
				g.ToggleTurnBasedMode()
				for i := 0; i < 20; i++ {
					gl.updateSpecialEffects()
				}
				if remaining() != before {
					t.Fatal("TB entry or thinking drained recharge")
				}
				g.ToggleTurnBasedMode()
				g.ToggleTurnBasedMode()
				if remaining() != before {
					t.Fatal("mode toggle reset recharge")
				}
				rounds := 4
				if mode == "mixed" {
					rounds = 2
				} else if mode == "round boundary" {
					rounds = 3
				}
				for i := 1; i < rounds; i++ {
					g.startPartyTurn()
				}
				if remaining() <= 0 {
					t.Fatal("recharged one round early")
				}
				g.startPartyTurn()
			}
			if remaining() != 0 {
				t.Fatalf("device did not recharge: %v", remaining())
			}
		})
	}
}

func TestTranslocatorShopSave(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	if err := g.transitionToMap(mapTransition{mapKey: "solstice_approach", arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	find := func() (*character.NPC, *character.MerchantStockItem) {
		for _, npc := range g.world.NPCs {
			if npc.Name != "Tovan" {
				continue
			}
			for i := range npc.MerchantStock {
				if npc.MerchantStock[i].Item.Type == items.ItemDevice {
					return npc, npc.MerchantStock[i]
				}
			}
		}
		t.Fatal("Tovan does not stock the Translocator")
		return nil, nil
	}
	npc, stock := find()
	if stock.Cost != 25000 || stock.Quantity != 1 || stock.Item.Stackable() {
		t.Fatal("wrong device price, stock or stacking")
	}
	legacy := g.buildSave(wm)
	for i := range legacy.NPCStates {
		ns := &legacy.NPCStates[i]
		if ns.Name != "Tovan" {
			continue
		}
		var oldStock []NPCStockSave
		for _, line := range ns.Stock {
			if line.Name != "Translocator" {
				oldStock = append(oldStock, line)
			}
		}
		ns.Stock = oldStock
	}
	if err := g.applySave(wm, &legacy); err != nil {
		t.Fatal(err)
	}
	npc, stock = find()
	if !stock.InStock() {
		t.Fatal("old save hid newly authored merchant stock")
	}
	g.dialogNPC = npc
	for _, hero := range g.party.Members {
		delete(hero.Skills, character.SkillMerchant)
	}
	g.party.Gold = 25000
	if !g.buyMerchantUnits(stock, 1) || g.party.Gold != 0 || stock.InStock() {
		t.Fatal("device purchase failed")
	}
	save := g.buildSave(wm)
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	_, stock = find()
	if stock.InStock() || g.party.CountItemsByName("Translocator") != 1 {
		t.Fatal("reload duplicated or lost purchased device")
	}
}

// Definition changes and legacy cooldowns must migrate in every physical store.
func TestTranslocatorRestoreCooldown(t *testing.T) {
	for _, state := range []string{"legacy boundary", "legacy clamped", "frames clamped", "negative", "partial"} {
		t.Run(state, func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			save := g.buildSave(wm)
			it := items.CreateItemFromYAML("translocator")
			want := 9 * g.config.GetTPS()
			switch state {
			case "legacy boundary":
				it.LegacyDeviceCooldown = 9.00000000000004
			case "legacy clamped":
				it.LegacyDeviceCooldown = 30
				want = 10 * g.config.GetTPS()
			case "frames clamped":
				it.DeviceCooldownFrames = 30 * g.config.GetTPS()
				want = 10 * g.config.GetTPS()
			case "negative":
				it.DeviceCooldownFrames, want = -10, 0
			case "partial":
				it.DeviceCooldownFrames, want = 17, 17
			}
			it.Attributes["use_cooldown_seconds"] = 30
			it.UseAction, it.SpellEffect = "outdated", "outdated"
			save.Party.Inventory = []items.Item{it}
			save.Party.Members[0].Inventory = []items.Item{it}
			save.Party.Members[0].QuickSlots = []QuickSlotEntry{{Slot: 0, Item: it}}
			save.GroundContainers = []GroundContainerSave{{MapKey: save.MapKey, Items: []items.Item{it}}}
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			check := func(want int) {
				t.Helper()
				for _, got := range []items.Item{g.party.Inventory[0], g.party.Members[0].Inventory[0], *g.party.Members[0].QuickSlots[0], g.groundContainers[0].Items[0]} {
					if got.DeviceCooldownFrames != want || got.LegacyDeviceCooldown != 0 || got.UseAction != "jump" || got.SpellEffect != "jump" {
						t.Fatalf("restored device: %+v, want %d frames", got, want)
					}
				}
			}
			check(want)
			g.turnBasedMode = false
			(&GameLoop{game: g}).updateSpecialEffects()
			check(max(0, want-1))
			g.tickDeviceCooldowns(30 * g.config.GetTPS())
			// Withdrawal into an idle party wakes its timer, including bag reallocations.
			it = items.CreateItemFromYAML("translocator")
			it.DeviceCooldownFrames = 2
			g.party.Bag().Add(it)
			(&GameLoop{game: g}).updateSpecialEffects()
			if g.party.Inventory[len(g.party.Inventory)-1].DeviceCooldownFrames != 1 {
				t.Fatal("idle sweep missed an incoming device")
			}
		})
	}
}

func TestJumpUsesCallingCombatAim(t *testing.T) {
	g, ts := summonTileWorld(t)
	setTestWorldManager(t, nil)
	g.turnBasedMode = false
	g.snapFacing(0)
	cs := NewCombatSystem(g)
	x, y := g.camera.X, g.camera.Y
	defer cs.beginPartyTargetAim(&monster.Monster3D{X: x, Y: y + 10*ts})()
	def, err := spells.GetSpellDefinitionByID("jump")
	if err != nil {
		t.Fatal(err)
	}
	if cs.tryCastJump(def, g.party.Members[0]) != castCommitted || math.Abs(g.camera.X-x) > .01 || math.Abs(g.camera.Y-y-def.JumpTiles*ts) > .01 {
		t.Fatal("Jump ignored the calling combat system's explicit aim")
	}
}

func TestTranslocatorConfiguredAction(t *testing.T) {
	g, ts := summonTileWorld(t)
	setTestWorldManager(t, nil)
	g.turnBasedMode = false
	g.snapFacing(0)
	def := config.GlobalItems.Items["translocator"]
	original := *def
	jump := *config.GlobalSpells.Spells["jump"]
	jump.JumpTiles = 3
	config.GlobalSpells.Spells["renamed_jump"] = &jump
	t.Cleanup(func() { *def = original; delete(config.GlobalSpells.Spells, "renamed_jump") })
	def.UseSpell, def.UseJumpExtraTiles, def.UseCooldownSeconds = "renamed_jump", 2, 7
	g.party.Inventory = nil
	g.party.Bag().Add(items.CreateItemFromYAML("translocator"))
	x := g.camera.X
	if !g.useDeviceFromInventory(0, 0) || math.Abs(g.camera.X-x-5*ts) > .01 || g.party.Inventory[0].DeviceCooldownFrames != 7*g.config.GetTPS() {
		t.Fatal("device ignored configured spell, range or recharge")
	}
	rows := GetItemTooltipRows(g.party.Inventory[0], g.party.Members[0], g.combat, true).String()
	if !strings.Contains(rows, "Teleports the entire party 5 tiles forward") || !strings.Contains(rows, "Recharge: 7 seconds") {
		t.Fatalf("tooltip ignored configuration: %s", rows)
	}
}

package game

import (
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// tovanShop opens the shipped Tovan as the dialog merchant with an empty bag.
func tovanShop(t *testing.T) (*MMGame, *character.NPC) {
	t.Helper()
	g := newTestCombatSystemWithConfig(t).game
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	npc, err := character.CreateNPCFromConfig("solstice_tovan", 96, 96)
	if err != nil {
		t.Fatal(err)
	}
	g.dialogActive, g.dialogNPC = true, npc
	g.party.Inventory = nil
	return g, npc
}

func itemName(t *testing.T, key string) string {
	t.Helper()
	d, ok := config.GetItemDefinition(key)
	if !ok {
		t.Fatalf("no item %q", key)
	}
	return d.Name
}

// A barter takes every good of its price together or nothing: a short good
// refuses the whole trade, n units cost n prices, the affordable count is set
// by the scarcest good, and coin never moves.
func TestBarterTradesGoodsAtomically(t *testing.T) {
	const core, ceramic, root = "solstice_thermal_core", "solstice_furnace_ceramic", "bitterroot"
	for _, tc := range []struct {
		name, entry string
		have        map[string]int
		n, wantMax  int
		ok          bool
		left        map[string]int
	}{
		{"one herb trade", "Dawnleaf", map[string]int{root: 3}, 1, 1, true, map[string]int{root: 1}},
		{"short herb", "Dawnleaf", map[string]int{root: 1}, 1, 0, false, map[string]int{root: 1}},
		{"two units pay twice", "Dawnleaf", map[string]int{root: 4}, 2, 2, true, map[string]int{root: 0}},
		{"one good short", "Advanced Heat Draught", map[string]int{core: 1, ceramic: 1}, 1, 0, false, map[string]int{core: 1, ceramic: 1}},
		{"both goods", "Advanced Heat Draught", map[string]int{core: 2, ceramic: 2}, 1, 1, true, map[string]int{core: 1, ceramic: 0}},
		{"scarcest good limits", "Advanced Heat Draught", map[string]int{core: 3, ceramic: 4}, 2, 2, true, map[string]int{core: 1, ceramic: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, npc := tovanShop(t)
			i := slices.IndexFunc(npc.MerchantStock, func(m *character.MerchantStockItem) bool { return m.Item.Name == tc.entry })
			if i < 0 || len(npc.MerchantStock[i].Barter) == 0 {
				t.Fatalf("Tovan has no barter entry %q", tc.entry)
			}
			entry := npc.MerchantStock[i]
			for key, n := range tc.have {
				for range n {
					g.party.AddItem(items.CreateItemFromYAML(key))
				}
			}
			g.party.Gold = 1000
			if got := g.merchantMaxUnits(entry); got != tc.wantMax {
				t.Fatalf("affordable units = %d, want %d", got, tc.wantMax)
			}
			if got := g.buyMerchantUnits(entry, tc.n); got != tc.ok {
				t.Fatalf("trade = %v, want %v", got, tc.ok)
			}
			for key, want := range tc.left {
				if got := g.party.CountItemsByName(itemName(t, key)); got != want {
					t.Errorf("%s left = %d, want %d", key, got, want)
				}
			}
			bought := 0
			if tc.ok {
				bought = tc.n
			}
			if got := g.party.CountItemsByName(tc.entry); got != bought || g.party.Gold != 1000 || entry.Quantity != character.UnlimitedStock {
				t.Fatalf("got %d %s, gold %d, shelf %d; want %d, 1000, unlimited", got, tc.entry, g.party.Gold, entry.Quantity, bought)
			}
		})
	}
}

// Tovan pays his authored offer per orchid, flat - the Merchant skill does not
// touch it - while the same orchid fetches the ordinary value price at any
// other coin shop. The offer is also what marks (and lights) the orchid there.
func TestSpecialOfferSale(t *testing.T) {
	g, tovan := tovanShop(t)
	other, err := character.CreateNPCFromConfig("desert_merchant", 96, 96)
	if err != nil {
		t.Fatal(err)
	}
	g.party.Members[0].Skills[character.SkillMerchant] = &character.Skill{Mastery: character.MasteryGrandMaster}
	orchid := items.CreateItemFromYAML("grave_orchid")
	valuePrice := g.merchantSellPrice(orchid.Attributes["value"])
	if valuePrice == 900 {
		t.Fatal("fixture: the value price must differ from the offer")
	}
	for _, tc := range []struct {
		name      string
		npc       *character.NPC
		valueless bool // the good has no value of its own
		want      int
		special   bool
	}{
		{"Tovan", tovan, false, 900, true},
		{"Tovan buys a valueless good at its offer", tovan, true, 900, true},
		{"another coin shop", other, false, valuePrice, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g.dialogNPC = tc.npc
			g.party.Inventory = nil
			for range 3 {
				it := items.CreateItemFromYAML("grave_orchid")
				if tc.valueless {
					it.Attributes["value"] = 0
				}
				g.party.AddItem(it)
			}
			if price, special := g.merchantUnitSellPrice(g.party.Inventory[0]); price != tc.want || special != tc.special {
				t.Fatalf("offer = %d (special %v), want %d (%v)", price, special, tc.want, tc.special)
			}
			// The drag-to-sell picker opens on the same price the sale pays.
			ui := &UISystem{game: g}
			if ui.beginMerchantSellPicker(0, 0); !ui.stackSplitPicker.open {
				t.Fatal("the drag-to-sell picker refused a good the merchant pays for")
			}
			gold := g.party.Gold
			if !g.sellInventoryUnits(0, 3) || g.party.Gold != gold+3*tc.want || len(g.party.Inventory) != 0 {
				t.Fatalf("sale paid %d and left %d stacks, want %d and none", g.party.Gold-gold, len(g.party.Inventory), 3*tc.want)
			}
		})
	}
}

// Tovan's shop+talk dialog lists Talk, Shop and Exchange. The Exchange shelf
// holds the advanced draughts back until the records quest is settled - also
// when the shelf was viewed (and memoized) before the quest closed - and the
// Shop tab never shows a barter entry. Like an exchange merchant, the
// Exchange tab buys nothing for coin and shows every carried stack (personal
// bags too) it can be paid from; like a coin shop, the Shop tab buys and
// shows the shared bag only.
func TestTovanExchangeShelfFollowsQuestGate(t *testing.T) {
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, false)
	settle := func(id string) {
		t.Helper()
		_ = g.questManager.ActivateQuest(id)
		g.questManager.MarkCompleted(id)
		q := g.questManager.GetQuest(id)
		if q == nil {
			t.Fatalf("no quest %q", id)
		}
		q.RewardsClaimed = true
	}
	settle("solstice_expedition")
	npc, err := character.CreateNPCFromConfig("solstice_tovan", 96, 96)
	if err != nil {
		t.Fatal(err)
	}
	g.dialogActive, g.dialogNPC = true, npc
	g.switchDialogTab(0)
	if tabs := g.merchantServiceTabs(); !slices.Equal(tabs, []string{"Talk", "Shop", "Exchange"}) {
		t.Fatalf("tabs = %q", tabs)
	}
	if g.serviceShopTabOpen() {
		t.Fatal("the Talk tab reads as a shop tab")
	}
	shelf := func(tab int) []string {
		g.switchDialogTab(tab)
		if !g.serviceShopTabOpen() {
			t.Fatalf("tab %d is not a shop tab", tab)
		}
		var names []string
		for _, m := range g.merchantVisibleStock() {
			names = append(names, m.Item.Name)
		}
		return names
	}
	if got := shelf(2); !slices.Equal(got, []string{"Dawnleaf"}) {
		t.Fatalf("gated Exchange shelf = %q, want only Dawnleaf", got)
	}
	settle("solstice_records")
	want := []string{"Advanced Heat Draught", "Advanced Flow Draught", "Advanced Anchor Draught", "Advanced Vector Draught", "Dawnleaf"}
	if got := shelf(2); !slices.Equal(got, want) {
		t.Fatalf("open Exchange shelf = %q, want %q", got, want)
	}
	for _, name := range shelf(1) {
		if slices.Contains(want, name) {
			t.Fatalf("Shop shelf shows barter entry %q", name)
		}
	}
	g.party.Inventory = []items.Item{items.CreateItemFromYAML("grave_orchid")}
	g.party.Bag(g.party.Members[0]).Add(items.CreateItemFromYAML("bitterroot"))
	shows := func(name string) bool {
		return slices.ContainsFunc(g.merchantBagItems(), func(it items.Item) bool { return it.Name == name })
	}
	gold := g.party.Gold
	shelf(2)
	if !shows("Bitterroot") || !shows("Grave Orchid") {
		t.Fatal("the Exchange tab hid a carried stack it can be paid from")
	}
	if g.merchantSellsHere() || g.sellInventoryUnits(0, 1) || len(g.party.Inventory) != 1 || g.party.Gold != gold {
		t.Fatal("the Exchange tab bought goods for coin")
	}
	shelf(1)
	if shows("Bitterroot") || !shows("Grave Orchid") {
		t.Fatal("the Shop tab shows more than the shared bag")
	}
	if !g.merchantSellsHere() || !g.sellInventoryUnits(0, 1) || g.party.Gold != gold+900 {
		t.Fatal("the Shop tab did not buy the orchid at its offer")
	}

	// A stock gate that changes the tab set under the open dialog: the
	// selection follows its tab by name, and a tab that vanished hands the
	// dialog back to Talk. The Shop tab is gated behind an open errand here.
	for _, m := range npc.MerchantStock {
		if m.Tab == "Shop" {
			m.RequiresQuest = "solstice_schedule"
		}
	}
	npc.MerchantStock = slices.Clone(npc.MerchantStock) // gates are fixed per stock slice, as a restock builds it
	selected := func() string {
		t.Helper()
		tabs := g.merchantServiceTabs()
		if g.dialogTab < 0 || g.dialogTab >= len(tabs) {
			t.Fatalf("dialogTab %d outside %q", g.dialogTab, tabs)
		}
		return tabs[g.dialogTab]
	}
	g.merchantShopTabs() // a frame's draw refreshes the view before a tab click lands
	g.switchDialogTab(1)
	if got := selected(); got != "Exchange" || !g.merchantBarterShelfOpen() {
		t.Fatalf("with the Shop gated, tab 1 = %q", got)
	}
	settle("solstice_schedule") // the Shop tab opens left of the Exchange
	if got := selected(); got != "Exchange" || !g.merchantBarterShelfOpen() || g.merchantSellsHere() {
		t.Fatalf("an opening tab moved the selection to %q", got)
	}
	g.switchDialogTab(1)
	g.questManager.GetQuest("solstice_schedule").RewardsClaimed = false // the gate closes again
	if got := selected(); got != "Talk" || g.serviceShopTabOpen() {
		t.Fatalf("the vanished Shop tab left the selection on %q", got)
	}
}

// A delivery the errand's own giver takes is its turn-in. Tovan's commission
// rows are Accept, then Deliver while it runs: a short delivery changes
// nothing, a full one pays on the spot and leaves no Claim row, and the
// commission is offered again when it repeats. The four commissions run side
// by side. A world prop on another NPC (a culvert valve) keeps its row before
// its errand is taken.
func TestCommissionDeliveryPaysAtTheGiver(t *testing.T) {
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, false)
	for _, id := range []string{"solstice_expedition", "solstice_records"} {
		_ = g.questManager.ActivateQuest(id)
		g.questManager.MarkCompleted(id)
		g.questManager.GetQuest(id).RewardsClaimed = true
	}
	if err := g.switchToMap("solstice_approach"); err != nil {
		t.Fatal(err)
	}
	var tovan *character.NPC
	for _, n := range g.world.NPCs {
		if n.Key == "solstice_tovan" {
			tovan = n
		}
	}
	if tovan == nil {
		t.Fatal("no Tovan on his map")
	}
	ih := &InputHandler{game: g}
	rows := func() []string {
		g.dialogActive, g.dialogNPC = true, tovan
		var out []string
		for _, c := range g.visibleNPCChoices(tovan) {
			out = append(out, c.Text)
		}
		return out
	}
	pick := func(text string) {
		t.Helper()
		i := slices.Index(rows(), text)
		if i < 0 {
			t.Fatalf("no row %q in %q", text, rows())
		}
		g.selectedChoice = i
		ih.executeEncounterChoice()
	}
	const accept, deliver, quest = "Accept Fire supply commission.", "Deliver three Heat Resistant Fang.", "solstice_fire_commission"
	fang := itemName(t, "solstice_heat_resistant_fang")
	if got := rows(); !slices.Contains(got, accept) || slices.Contains(got, deliver) {
		t.Fatalf("before the commission: %q", got)
	}
	pick(accept)
	if got := rows(); slices.Contains(got, accept) || !slices.Contains(got, deliver) {
		t.Fatalf("commission under way: %q", got)
	}
	// Errands without a requires_quest chain run in parallel.
	pick("Accept Water supply commission.")
	for _, want := range []string{deliver, "Deliver three Coldwater Gland.", "Accept Earth supply commission.", "Accept Air supply commission."} {
		if got := rows(); !slices.Contains(got, want) {
			t.Fatalf("parallel commissions: no %q in %q", want, got)
		}
	}
	g.party.AddItem(items.CreateItemFromYAML("solstice_heat_resistant_fang"))
	gold, draughts := g.party.Gold, g.party.CountItemsByName("Heat Draught")
	pick(deliver)
	if q := g.questManager.GetQuest(quest); q.Completed || g.party.CountItemsByName(fang) != 1 || g.party.Gold != gold {
		t.Fatal("a short delivery changed the errand, the goods or the purse")
	}
	g.party.AddItem(items.CreateItemFromYAML("solstice_heat_resistant_fang"))
	g.party.AddItem(items.CreateItemFromYAML("solstice_heat_resistant_fang"))
	pick(deliver)
	q := g.questManager.GetQuest(quest)
	if !q.Completed || !q.RewardsClaimed || g.party.CountItemsByName(fang) != 0 ||
		g.party.Gold != gold+180 || g.party.CountItemsByName("Heat Draught") != draughts+1 {
		t.Fatalf("delivery did not pay at once: completed=%v claimed=%v fangs=%d gold=+%d", q.Completed, q.RewardsClaimed, g.party.CountItemsByName(fang), g.party.Gold-gold)
	}
	for _, row := range rows() {
		if row == deliver || row == accept || strings.HasPrefix(row, "Claim") {
			t.Fatalf("after the payout: %q", rows())
		}
	}
	if got := rows(); !slices.Contains(got, "Deliver three Coldwater Gland.") {
		t.Fatalf("paying one commission closed another: %q", got)
	}
	g.calendarDay = g.currentCalendarDay() + 3
	g.refreshRepeatableQuests("")
	if got := rows(); !slices.Contains(got, accept) {
		t.Fatalf("the commission did not come back when it repeats: %q", got)
	}

	valve, err := character.CreateNPCFromConfig("culvert_valve_1", 96, 96)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(g.visibleNPCChoices(valve), func(c *character.NPCDialogueChoice) bool { return c.Action == questPropAction }) {
		t.Fatal("a world prop lost its row before its errand is taken")
	}

	// A delivery with no activity token takes the counted interact route; on
	// its own giver it is the turn-in all the same. A partial delivery keeps
	// the giver open, and a refused payout names the journal, which still pays.
	lamp, err := character.CreateNPCFromConfig("shrine_lamp_1", 96, 96)
	if err != nil {
		t.Fatal(err)
	}
	lampRow := lamp.DialogueData.Choices[slices.IndexFunc(lamp.DialogueData.Choices, func(c *character.NPCDialogueChoice) bool { return c.Action == questPropAction })]
	const lamps, other = "shrine_lamps", "Win three bouts."
	keeper := &character.NPC{Key: "test_lamp_keeper", Name: "Lamp Keeper", DialogueData: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
		{Text: "Light the lamps.", Action: "give_quest", QuestID: lamps},
		{Text: other, Action: "give_quest", QuestID: "pit_standing"},
		lampRow,
		{Text: "Leave.", Action: "leave"},
	}}}
	keeperRows := func() []string {
		g.dialogActive, g.dialogNPC = true, keeper
		var out []string
		for _, c := range g.visibleNPCChoices(keeper) {
			out = append(out, c.Text)
		}
		return out
	}
	choose := func(text string) {
		t.Helper()
		i := slices.Index(keeperRows(), text)
		if i < 0 {
			t.Fatalf("no row %q in %q", text, keeperRows())
		}
		g.selectedChoice = i
		ih.executeEncounterChoice()
	}
	choose("Light the lamps.")
	choose(lampRow.Text)
	if got := keeperRows(); keeper.Visited || !slices.Contains(got, lampRow.Text) || !slices.Contains(got, other) {
		t.Fatalf("a partial delivery closed the giver: visited=%v rows %q", keeper.Visited, got)
	}
	choose(lampRow.Text)
	def := g.questManager.GetQuest(lamps).Definition
	pool := def.Rewards.ItemPool
	t.Cleanup(func() { def.Rewards.ItemPool = pool })
	def.Rewards.ItemPool = []string{"no_such_item"}
	choose(lampRow.Text)
	lq := g.questManager.GetQuest(lamps)
	if !lq.Completed || lq.RewardsClaimed || keeper.Visited || countCombatLog(g, questJournalClaimHint) != 1 {
		t.Fatalf("refused payout: completed=%v claimed=%v visited=%v journal hint=%d", lq.Completed, lq.RewardsClaimed, keeper.Visited, countCombatLog(g, questJournalClaimHint))
	}
	def.Rewards.ItemPool = pool
	gold = g.party.Gold
	if !g.claimQuestReward(lamps, nil) || g.party.Gold != gold+2000 {
		t.Fatalf("the journal did not pay the refused delivery: gold +%d", g.party.Gold-gold)
	}
}

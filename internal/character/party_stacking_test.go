package character

import (
	"fmt"
	"testing"

	"ugataima/internal/items"
)

func potion(n int) items.Item {
	return items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: n}
}

func trinket(n int) items.Item {
	return items.Item{Name: "Clock Hand", Type: items.ItemTrinket, Quantity: n}
}

func sword() items.Item {
	return items.Item{Name: "Iron Sword", Type: items.ItemWeapon}
}

func TestAddItemStacksFungibles(t *testing.T) {
	p := &Party{}
	p.AddItem(potion(0)) // zero quantity reads as 1
	p.AddItem(potion(1))
	p.AddItem(trinket(2))
	p.AddItem(trinket(1))
	p.AddItem(sword())
	p.AddItem(sword()) // gear never stacks

	if len(p.Inventory) != 4 {
		t.Fatalf("want 4 entries (potion stack, trinket stack, 2 swords), got %d", len(p.Inventory))
	}
	if got := p.Inventory[0].Count(); got != 2 {
		t.Errorf("potion stack: want 2, got %d", got)
	}
	if got := p.Inventory[1].Count(); got != 3 {
		t.Errorf("trinket stack: want 3, got %d", got)
	}
	if got := p.GetTotalItems(); got != 7 {
		t.Errorf("total units: want 7, got %d", got)
	}
}

func TestConsumeOneAt(t *testing.T) {
	p := &Party{}
	p.AddItem(potion(2))
	if !p.ConsumeUnitsAt(0, 1) {
		t.Fatal("consume from stack failed")
	}
	if len(p.Inventory) != 1 || p.Inventory[0].Count() != 1 {
		t.Fatalf("want stack of 1 left, got %+v", p.Inventory)
	}
	if !p.ConsumeUnitsAt(0, 1) {
		t.Fatal("consume last unit failed")
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("last unit must remove the entry, got %+v", p.Inventory)
	}
	if p.ConsumeUnitsAt(0, 1) {
		t.Fatal("consume from empty bag must fail")
	}
}

func TestTakeStackUnits(t *testing.T) {
	p := &Party{Inventory: []items.Item{{
		Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 42,
	}}}
	part, ok := p.TakeStackUnits(0, 2)
	if !ok {
		t.Fatal("partial stack take failed")
	}
	if part.Count() != 2 || part.InstanceID != 42 {
		t.Errorf("taken fragment = %+v, want quantity 2 with ID 42", part)
	}
	if len(p.Inventory) != 1 || p.Inventory[0].Count() != 3 || p.Inventory[0].InstanceID == 42 {
		t.Errorf("remaining bag = %+v, want three potions with a fresh ID", p.Inventory)
	}

	whole, ok := p.TakeStackUnits(0, 3)
	if !ok || whole.Count() != 3 || len(p.Inventory) != 0 {
		t.Errorf("whole take = %+v, bag=%+v; want all three units removed", whole, p.Inventory)
	}
	if _, ok := p.TakeStackUnits(0, 1); ok {
		t.Error("taking from an empty bag must fail")
	}
}

func TestItemCurrencyAcrossStacks(t *testing.T) {
	p := &Party{}
	p.AddItem(trinket(5))
	if got := p.CountItemsByName("Clock Hand"); got != 5 {
		t.Fatalf("count: want 5, got %d", got)
	}
	if p.RemoveItemsByName("Clock Hand", 6) {
		t.Fatal("overdraft must fail")
	}
	if got := p.CountItemsByName("Clock Hand"); got != 5 {
		t.Fatalf("failed payment must not touch the stack, got %d", got)
	}
	if !p.RemoveItemsByName("Clock Hand", 3) {
		t.Fatal("payment of 3 from a stack of 5 must succeed")
	}
	if got := p.CountItemsByName("Clock Hand"); got != 2 {
		t.Fatalf("after paying 3: want 2, got %d", got)
	}
	if !p.RemoveItemsByName("Clock Hand", 2) {
		t.Fatal("paying out the rest must succeed")
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("drained stack must vanish, got %+v", p.Inventory)
	}
}

func TestItemCurrencyPaymentPreservesStackLineage(t *testing.T) {
	p := &Party{Inventory: []items.Item{{
		Name:       "Clock Hand",
		Type:       items.ItemTrinket,
		Quantity:   5,
		InstanceID: 11,
		Lineages: []items.StackLineage{
			{ID: 11, Quantity: 3},
			{ID: 22, Quantity: 2},
		},
	}}}
	if !p.RemoveItemsByName("Clock Hand", 3) {
		t.Fatal("currency payment failed")
	}
	if len(p.Inventory) != 1 || p.Inventory[0].Count() != 2 {
		t.Fatalf("remaining currency = %+v, want two hands", p.Inventory)
	}
	if got := p.Inventory[0].StackLineageParts(); len(got) != 1 || got[0] != (items.StackLineage{ID: 22, Quantity: 2}) {
		t.Fatalf("remaining currency provenance = %+v, want #22x2", got)
	}
}

func TestMergeStacksMigratesOldSaves(t *testing.T) {
	// A pre-stacking save: duplicates as separate entries, gear interleaved.
	p := &Party{Inventory: []items.Item{
		{Name: "Health Potion", Type: items.ItemConsumable, InstanceID: 11},
		sword(),
		{Name: "Health Potion", Type: items.ItemConsumable, InstanceID: 12},
		{Name: "Clock Hand", Type: items.ItemTrinket, InstanceID: 13},
		{Name: "Health Potion", Type: items.ItemConsumable, InstanceID: 14},
	}}
	p.MergeStacks()

	if len(p.Inventory) != 3 {
		t.Fatalf("want [potion stack, sword, trinket], got %d entries: %+v", len(p.Inventory), p.Inventory)
	}
	if p.Inventory[0].Count() != 3 || p.Inventory[0].InstanceID != 11 {
		t.Errorf("potion stack: want count 3 keeping the FIRST entry's id 11, got count %d id %d",
			p.Inventory[0].Count(), p.Inventory[0].InstanceID)
	}
	if p.Inventory[1].Type != items.ItemWeapon {
		t.Errorf("sword must keep its position, got %+v", p.Inventory[1])
	}
	if p.Inventory[2].Count() != 1 {
		t.Errorf("lone trinket stays a stack of 1, got %d", p.Inventory[2].Count())
	}
}

func TestConsumeUnitsAt(t *testing.T) {
	p := &Party{}
	p.AddItem(potion(5))
	if !p.ConsumeUnitsAt(0, 3) {
		t.Fatal("partial multi-unit consume failed")
	}
	if len(p.Inventory) != 1 || p.Inventory[0].Count() != 2 {
		t.Fatalf("want stack of 2 left, got %+v", p.Inventory)
	}
	if p.ConsumeUnitsAt(0, 3) {
		t.Fatal("consuming more units than the stack holds must fail")
	}
	if p.Inventory[0].Count() != 2 {
		t.Fatalf("a refused consume mutated the stack: %+v", p.Inventory[0])
	}
	if !p.ConsumeUnitsAt(0, 2) {
		t.Fatal("consuming the exact remainder failed")
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("the emptied entry must be removed, got %+v", p.Inventory)
	}
}

// Free stacks: a bag keeps the layout the player drops. Each row starts from
// [potion x5, sword, potion x2] and moves n units of src to cell at (3 is the
// free tail); want is the resulting layout as name/count.
func TestBagMoveWithinKeepsDroppedLayout(t *testing.T) {
	type stack struct {
		name  string
		count int
	}
	p, s := stack{"Health Potion", 0}, stack{"Iron Sword", 1}
	pot := func(n int) stack { p.count = n; return p }
	for _, tc := range []struct {
		name       string
		src, at, n int
		ok         bool
		want       []stack
	}{
		{"whole stack merges onto its match", 0, 2, 5, true, []stack{s, pot(7)}},
		{"whole stack swaps with another item", 0, 1, 5, true, []stack{s, pot(5), pot(2)}},
		{"whole stack moves to the free tail", 0, 3, 5, true, []stack{s, pot(2), pot(5)}},
		{"part becomes its own stack at the tail", 0, 3, 2, true, []stack{pot(3), s, pot(2), pot(2)}},
		{"part merges onto a matching stack", 0, 2, 2, true, []stack{pot(3), s, pot(4)}},
		{"part never displaces another item", 0, 1, 2, false, []stack{pot(5), s, pot(2)}},
		{"a drop on itself does nothing", 0, 0, 2, false, []stack{pot(5), s, pot(2)}},
		{"whole stack merges onto a match before it", 2, 0, 2, true, []stack{pot(7), s}},
		{"whole stack swaps with an item before it", 2, 1, 2, true, []stack{pot(5), pot(2), s}},
		{"part merges onto a match before it", 2, 0, 1, true, []stack{pot(6), s, pot(1)}},
		{"part never displaces an item before it", 2, 1, 1, false, []stack{pot(5), s, pot(2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			party := &Party{Inventory: []items.Item{potion(5), sword(), potion(2)}}
			for i := range party.Inventory {
				items.EnsureInstanceID(&party.Inventory[i])
			}
			bag := party.Bag()
			if got := bag.MoveWithin(tc.src, tc.at, tc.n); got != tc.ok {
				t.Fatalf("MoveWithin = %v, want %v", got, tc.ok)
			}
			var got []stack
			units := 0
			for _, it := range party.Inventory {
				got = append(got, stack{it.Name, it.Count()})
				if it.Stackable() {
					units += it.Count()
					lineage := 0
					for _, part := range it.StackLineageParts() {
						lineage += part.Quantity
					}
					if lineage != it.Count() {
						t.Fatalf("%s x%d carries lineage for %d units", it.Name, it.Count(), lineage)
					}
				}
			}
			if units != 7 || len(got) != len(tc.want) {
				t.Fatalf("layout %v (%d potions), want %v", got, units, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("layout %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// An item arriving from elsewhere keeps the cell it was dropped on: onto its
// match it merges, at the tail it is its own stack, onto another item it takes
// the cell and hands back the item it displaced; a negative cell stacks
// automatically like loot.
func TestBagPlaceFollowsTheTargetCell(t *testing.T) {
	for _, tc := range []struct {
		name      string
		at        int
		count     []int // potion stacks afterwards, in order
		displaced string
	}{
		{"onto its match", 2, []int{5, 4}, ""},
		{"at the free tail", 3, []int{5, 2, 2}, ""},
		{"onto another item", 1, []int{5, 2, 2}, "Iron Sword"},
		{"automatic", -1, []int{7, 2}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			party := &Party{Inventory: []items.Item{potion(5), sword(), potion(2)}}
			displaced := party.Bag().Place(potion(2), tc.at)
			var got []int
			for _, it := range party.Inventory {
				if it.Stackable() {
					got = append(got, it.Count())
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.count) || displaced.Name != tc.displaced {
				t.Fatalf("potion stacks %v, displaced %q; want %v and %q", got, displaced.Name, tc.count, tc.displaced)
			}
		})
	}
}

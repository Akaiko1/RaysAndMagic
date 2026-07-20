package character

import (
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
	if !p.ConsumeOneAt(0) {
		t.Fatal("consume from stack failed")
	}
	if len(p.Inventory) != 1 || p.Inventory[0].Count() != 1 {
		t.Fatalf("want stack of 1 left, got %+v", p.Inventory)
	}
	if !p.ConsumeOneAt(0) {
		t.Fatal("consume last unit failed")
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("last unit must remove the entry, got %+v", p.Inventory)
	}
	if p.ConsumeOneAt(0) {
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

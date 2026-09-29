package character

import (
	"iter"

	"ugataima/internal/items"
)

// InventoryBag addresses one physical container. A nil owner is the shared bag.
// All bags use the same stacking and provenance rules and party content revision.
type InventoryBag struct {
	party *Party
	Owner *MMCharacter
}

func (p *Party) Bag(owner ...*MMCharacter) InventoryBag {
	var ch *MMCharacter
	if len(owner) > 0 {
		ch = owner[0]
	}
	return InventoryBag{p, ch}
}
func (b InventoryBag) entries() *[]items.Item {
	if b.Owner != nil {
		return &b.Owner.Inventory
	}
	return &b.party.Inventory
}
func (b InventoryBag) Items() []items.Item { return *b.entries() }
func (b InventoryBag) Add(item items.Item) {
	b.party.contentRev++
	inv := b.entries()
	if item.Stackable() {
		for i := range *inv {
			if items.SameStack((*inv)[i], item) {
				(*inv)[i].MergeStack(item)
				return
			}
		}
	}
	*inv = append(*inv, item)
}
func (b InventoryBag) Remove(index int) {
	inv := b.entries()
	if index < 0 || index >= len(*inv) {
		return
	}
	b.party.contentRev++
	*inv = append((*inv)[:index], (*inv)[index+1:]...)
}
func (b InventoryBag) Consume(index, n int) bool {
	inv := b.Items()
	if index < 0 || index >= len(inv) || n < 1 || n > inv[index].Count() {
		return false
	}
	if inv[index].Count() == n {
		b.Remove(index)
		return true
	}
	b.party.contentRev++
	return inv[index].ConsumeStackUnits(n)
}
func (b InventoryBag) Take(index, n int) (items.Item, bool) {
	inv := b.Items()
	if index < 0 || index >= len(inv) || n < 1 || n > inv[index].Count() {
		return items.Item{}, false
	}
	it := inv[index]
	if n == it.Count() {
		b.Remove(index)
		return it, true
	}
	if !it.Stackable() {
		return items.Item{}, false
	}
	part, ok := inv[index].SplitOff(n)
	if ok {
		b.party.contentRev++
	}
	return part, ok
}
func (b InventoryBag) MoveTo(dst InventoryBag, index, n int) bool {
	if b.party != dst.party || b.Owner == dst.Owner {
		return false
	}
	item, ok := b.Take(index, n)
	if ok {
		dst.Add(item)
	}
	return ok
}
func (b InventoryBag) Reorder(src, dst int) {
	inv := b.Items()
	if src < 0 || src >= len(inv) || dst < 0 || dst > len(inv) {
		return
	}
	if dst == len(inv) {
		it := inv[src]
		copy(inv[src:], inv[src+1:])
		inv[len(inv)-1] = it
	} else {
		inv[src], inv[dst] = inv[dst], inv[src]
	}
	b.party.contentRev++
}
func (b InventoryBag) MergeStacks() {
	type key struct {
		name string
		typ  items.ItemType
	}
	first := map[key]int{}
	inv := b.Items()
	kept := inv[:0]
	for _, it := range inv {
		k := key{it.Name, it.Type}
		if it.Stackable() {
			if at, ok := first[k]; ok {
				kept[at].MergeStack(it)
				continue
			}
			first[k] = len(kept)
		}
		kept = append(kept, it)
	}
	*b.entries() = kept
	b.party.contentRev++
}

// CarriedBags excludes benched and captive heroes. Shared stock is spent first
// on equal-cost operations; personal possessions travel with their owner.
func (p *Party) CarriedBags() []InventoryBag {
	bags := []InventoryBag{p.Bag()}
	for _, ch := range p.Members {
		if ch != nil {
			bags = append(bags, p.Bag(ch))
		}
	}
	return bags
}
func (p *Party) CarriedItems() []items.Item {
	var result []items.Item
	for stack := range p.carriedStacks() {
		result = append(result, *stack.item())
	}
	return result
}

// carriedStack addresses physical stock without copying it. A quick-slot
// index is stable when another slot empties; bag indices shift after removal.
type carriedStack struct {
	bag   InventoryBag
	index int
	quick bool
}

func (s carriedStack) item() *items.Item {
	if s.quick {
		return s.bag.Owner.QuickSlots[s.index]
	}
	return &s.bag.Items()[s.index]
}

func (s carriedStack) consume(n int) bool {
	if !s.quick {
		return s.bag.Consume(s.index, n)
	}
	it := s.item()
	if it == nil || n < 1 || n > it.Count() {
		return false
	}
	if n == it.Count() {
		s.bag.Owner.QuickSlots[s.index] = nil
	} else if !it.ConsumeStackUnits(n) {
		return false
	}
	s.bag.party.contentRev++
	return true
}

func physicalQuickStacks(p *Party, hero *MMCharacter) iter.Seq[carriedStack] {
	return func(yield func(carriedStack) bool) {
		if hero == nil {
			return
		}
		for i, it := range hero.QuickSlots {
			if it != nil && !it.VirtualAction() && !yield(carriedStack{InventoryBag{p, hero}, i, true}) {
				return
			}
		}
	}
}

// PersonalItems visits the hero's stored physical goods, including quick slots.
// Worn equipment and non-owning spell/technique/flask shortcuts are not stock.
func (c *MMCharacter) PersonalItems() iter.Seq[items.Item] {
	return func(yield func(items.Item) bool) {
		if c == nil {
			return
		}
		for _, it := range c.Inventory {
			if !it.VirtualAction() && !yield(it) {
				return
			}
		}
		for stack := range physicalQuickStacks(nil, c) {
			if !yield(*stack.item()) {
				return
			}
		}
	}
}

// All carried reads and indexed payments share this order: shared bag, active
// personal bags, then active quick slots. Reserve and captive stock is excluded.
func (p *Party) carriedStacks() iter.Seq[carriedStack] {
	return func(yield func(carriedStack) bool) {
		if p == nil {
			return
		}
		for _, b := range p.CarriedBags() {
			for i, it := range b.Items() {
				if !it.VirtualAction() && !yield(carriedStack{b, i, false}) {
					return
				}
			}
		}
		for _, hero := range p.Members {
			for stack := range physicalQuickStacks(p, hero) {
				if !yield(stack) {
					return
				}
			}
		}
	}
}

// ConsumeCarriedUnitsAt resolves the same index as CarriedItems. Callers making
// multiple payments consume descending indices, including now-empty quick slots.
func (p *Party) ConsumeCarriedUnitsAt(index, n int) bool {
	for stack := range p.carriedStacks() {
		if index == 0 {
			return stack.consume(n)
		}
		index--
	}
	return false
}

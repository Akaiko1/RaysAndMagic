package character

import (
	"iter"
	"slices"

	"ugataima/internal/items"
)

// InventoryBag addresses one physical container. A nil owner is the shared bag.
// All bags use the same stacking and provenance rules and party content
// revision. A bag may hold several stacks of one item: automatic adds (loot,
// purchases, withdrawals) merge into the first matching stack, while a manual
// placement keeps the cell it was dropped on.
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

// Cell is the item at cell at; the free tail past the last entry reads as an
// empty cell.
func (b InventoryBag) Cell(at int) items.Item {
	if inv := b.Items(); at >= 0 && at < len(inv) {
		return inv[at]
	}
	return items.Item{}
}

// Place lands an item from elsewhere on cell at under items.DropOnto: onto a
// matching stack it merges, at the free tail it keeps a stack of its own, onto
// any other item it takes that cell and returns the item it displaced. A
// negative at stacks automatically like Add. Callers refuse part of a stack
// onto another item (DropOnto) before taking anything.
func (b InventoryBag) Place(item items.Item, at int) (displaced items.Item) {
	if at < 0 {
		b.Add(item)
		return items.Item{}
	}
	inv := b.Items()
	switch items.DropOnto(item, b.Cell(at), true) {
	case items.DropMerge:
		inv[at].MergeStack(item)
	case items.DropPlace:
		*b.entries() = append(*b.entries(), item)
	default:
		displaced, inv[at] = inv[at], item
	}
	b.party.contentRev++
	return displaced
}

// Insert puts an item at index at, shifting the later entries: a displaced
// item returns to the cell its swap partner left.
func (b InventoryBag) Insert(at int, item items.Item) {
	inv := b.entries()
	*inv = slices.Insert(*inv, max(0, min(at, len(*inv))), item)
	b.party.contentRev++
}

// MoveWithin moves n units of the stack at src to cell at of the same bag
// under items.DropOnto: a merge onto a matching stack, a stack of its own at
// the free tail, a swap of a whole stack with another item. Part of a stack
// onto another item moves nothing.
func (b InventoryBag) MoveWithin(src, at, n int) bool {
	inv := b.Items()
	if src < 0 || src >= len(inv) || at < 0 || at == src || n < 1 || n > inv[src].Count() {
		return false
	}
	at = min(at, len(inv))
	whole := n == inv[src].Count()
	switch items.DropOnto(inv[src], b.Cell(at), whole) {
	case items.DropRefused:
		return false
	case items.DropMerge:
		if whole {
			b.party.contentRev++
			inv[at].MergeStack(inv[src])
			b.Remove(src)
			return true
		}
	default:
		if whole { // swap, or the last cell at the free tail
			b.Reorder(src, at)
			return true
		}
	}
	part, ok := b.Take(src, n)
	if ok {
		b.Place(part, at) // part of a stack leaves src in place: at is unchanged
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

// MergeStacks folds every stack into the first of its kind. It runs once on a
// save written before free stacks (see stackLayoutSaveVersion).
func (b InventoryBag) MergeStacks() {
	inv := b.Items()
	kept := inv[:0]
	for _, it := range inv {
		if at := slices.IndexFunc(kept, func(k items.Item) bool { return items.SameStack(k, it) }); at >= 0 {
			kept[at].MergeStack(it)
			continue
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

// ConsumeQuickSlotUnits spends n units of a hero's quick-slot stack, clearing
// the slot with its last unit.
func (p *Party) ConsumeQuickSlotUnits(hero *MMCharacter, slot, n int) bool {
	if hero == nil || slot < 0 || slot >= len(hero.QuickSlots) {
		return false
	}
	return carriedStack{InventoryBag{p, hero}, slot, true}.consume(n)
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

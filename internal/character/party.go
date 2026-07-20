package character

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
)

type Party struct {
	Members []*MMCharacter
	Gold    int
	Food    int
	// ArenaPoints is the arena victory currency (champion duels); spent at the
	// arena quartermaster. Persisted with the save like Gold.
	ArenaPoints int
	Inventory   []items.Item
	// Reserve holds benched heroes available at the tavern (swappable into the
	// active party). They keep all gear/XP/skills and level alongside the party.
	Reserve []*MMCharacter
	// Captive holds heroes still imprisoned (e.g. the mountain prison). They also
	// level alongside the party from the start, but aren't usable until freed -
	// clearing the prison moves them into Reserve.
	Captive []*MMCharacter
}

// FreeCaptives moves all imprisoned heroes into the reserve roster and returns
// the freed heroes (for messaging). No-op if there are none.
func (p *Party) FreeCaptives() []*MMCharacter {
	freed := p.Captive
	p.Reserve = append(p.Reserve, p.Captive...)
	p.Captive = nil
	return freed
}

// Recruit adds a hero to the reserve roster (e.g. rescued from the prison).
func (p *Party) Recruit(c *MMCharacter) {
	if c != nil {
		p.Reserve = append(p.Reserve, c)
	}
}

// SwapActiveReserve exchanges an active party member with a reserve member.
// The party stays at the same size; the benched member moves to the reserve
// slot the new active member came from. All state rides along on the pointers.
func (p *Party) SwapActiveReserve(activeIdx, reserveIdx int) bool {
	if activeIdx < 0 || activeIdx >= len(p.Members) ||
		reserveIdx < 0 || reserveIdx >= len(p.Reserve) {
		return false
	}
	p.Members[activeIdx], p.Reserve[reserveIdx] = p.Reserve[reserveIdx], p.Members[activeIdx]
	return true
}

// Fallback rosters used when config.yaml omits the lists (keeps older/minimal
// configs and tests working). The canonical roster lives in config.yaml.
var defaultStartingParty = []config.RosterEntry{
	{Name: "Gareth", Class: "knight"},
	{Name: "Lysander", Class: "sorcerer"},
	{Name: "Celestine", Class: "cleric"},
	{Name: "Silvelyn", Class: "archer"},
}

var defaultCaptives = []config.RosterEntry{
	{Name: "Auberon", Class: "paladin"},
	{Name: "Mirelle", Class: "druid"},
}

// createRosterCharacter builds a roster hero from a config entry, or nil on an
// unknown class key. A race entry shifts the class base stats additively
// (human/empty = baseline) and re-derives HP/SP.
// CreateRosterCharacter builds a hero from a config roster entry (class kit +
// race modifiers) - the SAME path NewParty uses; exported so the map editor
// renders the real shipped roster, not per-class approximations.
func CreateRosterCharacter(e config.RosterEntry, cfg *config.Config) *MMCharacter {
	return createRosterCharacter(e, cfg)
}

func createRosterCharacter(e config.RosterEntry, cfg *config.Config) *MMCharacter {
	class, ok := ClassFromKey(e.Class)
	if !ok {
		return nil
	}
	c := CreateCharacter(e.Name, class, cfg)
	if e.Race != "" {
		c.ApplyRace(e.Race, cfg)
		c.CalculateDerivedStats(cfg)
	}
	return c
}

// StartingRoster returns the three roster groups (active party, imprisoned
// captives, tavern recruits) from config, applying the same fallbacks NewParty
// uses for the active/captive lists. The party-creation screen pools all three.
func StartingRoster(cfg *config.Config) (active, captives, recruits []config.RosterEntry) {
	active = cfg.Characters.StartingParty
	captives = cfg.Characters.Captives
	if len(active) == 0 {
		active = defaultStartingParty
	}
	if len(captives) == 0 {
		captives = defaultCaptives
	}
	return active, captives, cfg.Characters.TavernRecruits
}

// LeftoverHero is a benched hero awaiting assignment to the jail or the tavern
// reserve, with whether the roster flagged it a captive.
type LeftoverHero struct {
	Char    *MMCharacter
	Captive bool
}

// PartitionLeftovers splits benched heroes into the mountain prison (jail) and
// the tavern reserve. Config-flagged captives fill the jail first, then
// non-captives top it up to jailTarget so the prison always holds the configured
// number (preserving the rescue narrative); everyone else joins the reserve.
// Input order is preserved within each group.
func PartitionLeftovers(leftovers []LeftoverHero, jailTarget int) (jail, reserve []*MMCharacter) {
	used := make([]bool, len(leftovers))
	take := func(wantCaptive bool) {
		for i, h := range leftovers {
			if len(jail) >= jailTarget {
				return
			}
			if !used[i] && (!wantCaptive || h.Captive) {
				jail = append(jail, h.Char)
				used[i] = true
			}
		}
	}
	take(true)  // captives first
	take(false) // then fill to jailTarget from the front
	for i, h := range leftovers {
		if !used[i] {
			reserve = append(reserve, h.Char)
		}
	}
	return jail, reserve
}

// newPartyBase allocates a party with the configured starting gold/food and an
// empty inventory - the shared shell for every new-game constructor.
func newPartyBase(cfg *config.Config) *Party {
	return &Party{
		Members:   make([]*MMCharacter, 0, 4),
		Gold:      cfg.Characters.StartingGold,
		Food:      cfg.Characters.StartingFood,
		Inventory: make([]items.Item, 0),
	}
}

// addStartingItems seeds the shared new-game inventory from YAML definitions.
func (p *Party) addStartingItems() {
	p.AddItem(items.CreateWeaponFromYAML("iron_spear"))
	p.AddItem(items.CreateItemFromYAML("leather_armor"))
	p.AddItem(items.CreateItemFromYAML("health_potion"))
	p.AddItem(items.CreateItemFromYAML("revival_potion"))
	p.AddItem(items.CreateItemFromYAML("magic_ring"))
	p.AddItem(items.CreateItemFromYAML("world_map"))
}

func NewParty(cfg *config.Config) *Party {
	party := newPartyBase(cfg)

	// Build the starting roster from config (data-driven). Active party + the
	// imprisoned captives that train alongside it.
	active, captives, recruits := StartingRoster(cfg)
	for _, e := range active {
		if c := createRosterCharacter(e, cfg); c != nil {
			party.AddMember(c)
		}
	}
	for _, e := range captives {
		if c := createRosterCharacter(e, cfg); c != nil {
			party.Captive = append(party.Captive, c)
		}
	}
	// Tavern recruits start benched in the reserve - available at the tavern
	// from the very first visit.
	for _, e := range recruits {
		if c := createRosterCharacter(e, cfg); c != nil {
			party.Reserve = append(party.Reserve, c)
		}
	}

	party.addStartingItems()
	return party
}

// NewPartyFromGroups builds a new-game party from already-constructed heroes
// split into active members, imprisoned captives, and tavern reserve. Used by
// the party-creation screen, where the player picks who goes where. Shares the
// same starting gold/food/inventory as NewParty. Active is capped at 4.
func NewPartyFromGroups(cfg *config.Config, active, captive, reserve []*MMCharacter) *Party {
	party := newPartyBase(cfg)
	for _, c := range active {
		if c != nil {
			party.AddMember(c)
		}
	}
	for _, c := range captive {
		if c != nil {
			party.Captive = append(party.Captive, c)
		}
	}
	for _, c := range reserve {
		if c != nil {
			party.Reserve = append(party.Reserve, c)
		}
	}
	party.addStartingItems()
	return party
}

// HasLich reports whether any living-or-dead party member has been promoted to
// a Lich. Used to gate the Mage Tower and to enrage otherwise-passive monsters.
func (p *Party) HasLich() bool {
	for _, m := range p.Members {
		if m != nil && m.IsLich() {
			return true
		}
	}
	return false
}

func (p *Party) AddMember(character *MMCharacter) {
	if len(p.Members) < 4 {
		p.Members = append(p.Members, character)
	}
}

func (p *Party) Update() {
	for _, member := range p.Members {
		member.Update()
	}
}

// UpdateWithMode updates the party with knowledge of the current game mode
func (p *Party) UpdateWithMode(turnBasedMode bool) {
	for _, member := range p.Members {
		member.UpdateWithMode(turnBasedMode)
	}
}

// AddItem adds an item to the party inventory. Stackable items (consumables,
// trinkets) merge into an existing same-name stack; everything else appends.
func (p *Party) AddItem(item items.Item) {
	if item.Stackable() {
		for i := range p.Inventory {
			if items.SameStack(p.Inventory[i], item) {
				p.Inventory[i].MergeStack(item)
				return
			}
		}
	}
	p.Inventory = append(p.Inventory, item)
}

// RemoveItem removes a whole inventory entry (the full stack) by index.
func (p *Party) RemoveItem(index int) {
	if index >= 0 && index < len(p.Inventory) {
		p.Inventory = append(p.Inventory[:index], p.Inventory[index+1:]...)
	}
}

// ConsumeOneAt removes ONE unit from the entry at index: decrements a stack,
// removes the entry when the last unit goes. Reports whether a unit was taken.
func (p *Party) ConsumeOneAt(index int) bool {
	if index < 0 || index >= len(p.Inventory) {
		return false
	}
	if p.Inventory[index].Count() > 1 {
		return p.Inventory[index].ConsumeStackUnits(1)
	}
	p.RemoveItem(index)
	return true
}

// TakeStackUnits removes quantity units from one stackable bag entry and
// returns them as a separate item. A full take preserves the usual whole-entry
// move; a partial take delegates the lineage split to items.Item.SplitOff so
// stash reconciliation remains correct across old saves.
func (p *Party) TakeStackUnits(index, quantity int) (items.Item, bool) {
	if index < 0 || index >= len(p.Inventory) || quantity < 1 {
		return items.Item{}, false
	}
	item := p.Inventory[index]
	if !item.Stackable() || quantity > item.Count() {
		return items.Item{}, false
	}
	if quantity == item.Count() {
		p.RemoveItem(index)
		return item, true
	}
	fragment, ok := p.Inventory[index].SplitOff(quantity)
	return fragment, ok
}

// MergeStacks folds duplicate stackable entries into single stacks (first
// entry keeps its place and InstanceID). Load-time migration for saves
// written before stacking existed.
func (p *Party) MergeStacks() {
	type stackKey struct {
		name string
		typ  items.ItemType
	}
	first := make(map[stackKey]int)
	kept := p.Inventory[:0]
	for _, it := range p.Inventory {
		if !it.Stackable() {
			kept = append(kept, it)
			continue
		}
		k := stackKey{it.Name, it.Type}
		if i, ok := first[k]; ok {
			kept[i].MergeStack(it)
			continue
		}
		first[k] = len(kept)
		kept = append(kept, it)
	}
	p.Inventory = kept
}

// CountItemsByName counts inventory units of the named item, stacks included
// (item-backed merchant currencies: clock hands).
func (p *Party) CountItemsByName(name string) int {
	n := 0
	for i := range p.Inventory {
		if p.Inventory[i].Name == name {
			n += p.Inventory[i].Count()
		}
	}
	return n
}

// RemoveItemsByName removes up to n units of the named item, draining stacks
// as needed; reports whether all n were found and removed (payment in an
// item-backed currency).
func (p *Party) RemoveItemsByName(name string, n int) bool {
	if p.CountItemsByName(name) < n {
		return false
	}
	kept := p.Inventory[:0]
	for _, it := range p.Inventory {
		if n > 0 && it.Name == name {
			c := it.Count()
			if c <= n {
				n -= c
				continue
			}
			// Keep the provenance of a merged stack aligned with its remaining
			// units. Currency can be stored in the shared stash just like any
			// other trinket, so direct Quantity edits would make stale-save
			// reconciliation count the wrong lineage after a partial payment.
			it.ConsumeStackUnits(n)
			n = 0
		}
		kept = append(kept, it)
	}
	p.Inventory = kept
	return true
}

// GetTotalItems returns the number of item units in the party inventory,
// stacks included.
func (p *Party) GetTotalItems() int {
	n := 0
	for i := range p.Inventory {
		n += p.Inventory[i].Count()
	}
	return n
}

// equipFromInventory validates the indices and conscious state, runs the given
// equip call, and on success removes the item from the bag and returns any
// displaced item to it. Shared core of the equip-from-inventory variants.
func (p *Party) equipFromInventory(itemIndex, characterIndex int, equip func(*MMCharacter, items.Item) (items.Item, bool, bool)) bool {
	if itemIndex < 0 || itemIndex >= len(p.Inventory) {
		return false
	}
	if characterIndex < 0 || characterIndex >= len(p.Members) {
		return false
	}

	item := p.Inventory[itemIndex]
	character := p.Members[characterIndex]

	// Disallow equipping if character is unconscious
	if character.HasCondition(ConditionUnconscious) {
		return false
	}

	previousItem, hadPreviousItem, success := equip(character, item)
	if !success {
		return false
	}
	p.RemoveItem(itemIndex)
	p.returnDisplacedToBag(previousItem, hadPreviousItem)
	return true
}

// EquipItemFromInventory attempts to equip an item from inventory to a character
func (p *Party) EquipItemFromInventory(itemIndex, characterIndex int) bool {
	return p.equipFromInventory(itemIndex, characterIndex, func(c *MMCharacter, item items.Item) (items.Item, bool, bool) {
		return c.EquipItem(item)
	})
}

// returnDisplacedToBag puts an item displaced by an equip back into the inventory,
// skipping spellbook-owned spell items (which never live in the bag).
func (p *Party) returnDisplacedToBag(item items.Item, had bool) {
	if had && item.Type != items.ItemBattleSpell && item.Type != items.ItemUtilitySpell {
		p.AddItem(item)
	}
}

// EquipItemFromInventoryToSlot equips an inventory item into a SPECIFIC slot
// (drag-drop onto an exact paperdoll slot), so a ring goes to the finger it was
// dropped on. Mirrors EquipItemFromInventory otherwise (inventory removal +
// displaced item returned to the bag).
func (p *Party) EquipItemFromInventoryToSlot(itemIndex, characterIndex int, slot items.EquipSlot) bool {
	return p.equipFromInventory(itemIndex, characterIndex, func(c *MMCharacter, item items.Item) (items.Item, bool, bool) {
		return c.EquipItemToSlot(item, slot)
	})
}

// MoveEquippedSlot moves a character's equipped item from srcSlot to dstSlot
// (swapping if dstSlot is occupied). For interchangeable slots like the two ring
// fingers - nothing leaves the paperdoll, so no inventory changes.
func (p *Party) MoveEquippedSlot(srcSlot, dstSlot items.EquipSlot, characterIndex int) bool {
	if characterIndex < 0 || characterIndex >= len(p.Members) {
		return false
	}
	return p.Members[characterIndex].MoveEquipmentSlot(srcSlot, dstSlot)
}

// UnequipItemToInventory removes an item from a character's equipment and adds it to inventory.
// Spell-slot items are never returned to inventory - the spellbook is the only owner
// of learned spells, so unequipping just clears the slot.
func (p *Party) UnequipItemToInventory(slot items.EquipSlot, characterIndex int) bool {
	if characterIndex < 0 || characterIndex >= len(p.Members) {
		return false
	}

	character := p.Members[characterIndex]

	item, success := character.UnequipItem(slot)
	if !success {
		return false
	}
	if slot != items.SlotSpell {
		p.AddItem(item)
	}
	return true
}

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
	ArenaPoints        int
	NonKeyDoorsOpened  int
	ThievesGuildMember bool
	Inventory          []items.Item
	// Reserve holds benched heroes available at the tavern (swappable into the
	// active party). They keep all gear/XP/skills and level alongside the party.
	Reserve []*MMCharacter
	// Captive holds heroes still imprisoned (e.g. the mountain prison). They also
	// level alongside the party from the start, but aren't usable until freed -
	// clearing the prison moves them into Reserve.
	Captive []*MMCharacter

	// contentRev counts inventory/roster mutations that may keep every length
	// and currency unchanged (a stack merge, a partial stack drain, a bench
	// swap). The UI's modal redraw barrier compares it between frames; the
	// field is unexported so it never enters a save file.
	contentRev uint64
}

// ContentRevision exposes the mutation counter to the UI snapshot.
func (p *Party) ContentRevision() uint64 { return p.contentRev }

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
	p.contentRev++
	p.Members[activeIdx].RareClass.Anchor = SpatialAnchor{}
	p.Reserve[reserveIdx].RareClass.Anchor = SpatialAnchor{}
	p.Members[activeIdx], p.Reserve[reserveIdx] = p.Reserve[reserveIdx], p.Members[activeIdx]
	p.GrantClassItems(config.GlobalConfig)
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
	party.GrantClassItems(cfg)
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
	party.GrantClassItems(cfg)
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

// UpdateWithMode updates the party with knowledge of the current game mode and
// reports whether any member completed an RT regeneration cadence.
func (p *Party) UpdateWithMode(turnBasedMode bool) bool {
	regenCadenceCompleted := false
	for _, member := range p.Members {
		if member.UpdateWithMode(turnBasedMode) {
			regenCadenceCompleted = true
		}
	}
	return regenCadenceCompleted
}

// AddItem adds an item to the party inventory. Stackable items (consumables,
// trinkets) merge into an existing same-name stack; everything else appends.
func (p *Party) AddItem(item items.Item)          { p.Bag().Add(item) }
func (p *Party) RemoveItem(index int)             { p.Bag().Remove(index) }
func (p *Party) ConsumeUnitsAt(index, n int) bool { return p.Bag().Consume(index, n) }
func (p *Party) TakeStackUnits(index, quantity int) (items.Item, bool) {
	if index < 0 || index >= len(p.Inventory) || !p.Inventory[index].Stackable() {
		return items.Item{}, false
	}
	return p.Bag().Take(index, quantity)
}
func (p *Party) MergeStacks() {
	p.Bag().MergeStacks()
	for _, roster := range [][]*MMCharacter{p.Members, p.Reserve, p.Captive} {
		for _, ch := range roster {
			if ch != nil {
				p.Bag(ch).MergeStacks()
			}
		}
	}
}

// CountItemsByName counts carried units available for crafting and item exchanges.
func (p *Party) CountItemsByName(name string) int {
	n := 0
	for stack := range p.carriedStacks() {
		if it := stack.item(); it.Name == name {
			n += it.Count()
		}
	}
	return n
}

// RemoveItemsByName pays from shared stock, active personal bags, then quick
// slots. Stage the whole payment before mutating; consume backwards so bag
// removals cannot retarget a later part of the same payment.
func (p *Party) RemoveItemsByName(name string, n int) bool {
	if n < 0 {
		return false
	}
	type payment struct {
		stack carriedStack
		count int
	}
	var plan []payment
	for stack := range p.carriedStacks() {
		if n == 0 {
			break
		}
		if it := stack.item(); it.Name == name {
			take := min(n, it.Count())
			plan = append(plan, payment{stack, take})
			n -= take
		}
	}
	if n != 0 {
		return false
	}
	for i := len(plan) - 1; i >= 0; i-- {
		plan[i].stack.consume(plan[i].count)
	}
	return true
}

// GetTotalItems returns the number of item units in the party inventory,
// stacks included. Test-only: character and game tests count units with it.
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
func (p *Party) equipFromInventory(itemIndex, characterIndex int, equip func(*MMCharacter, items.Item) (items.Item, bool, bool), owner ...*MMCharacter) bool {
	bag := p.Bag(owner...)
	if itemIndex < 0 || itemIndex >= len(bag.Items()) {
		return false
	}
	if characterIndex < 0 || characterIndex >= len(p.Members) {
		return false
	}

	item := bag.Items()[itemIndex]
	character := p.Members[characterIndex]

	// Disallow equipping if character is unconscious
	if character.HasCondition(ConditionUnconscious) {
		return false
	}

	previousItem, hadPreviousItem, success := equip(character, item)
	if !success {
		return false
	}
	bag.Remove(itemIndex)
	if hadPreviousItem && !previousItem.VirtualAction() {
		bag.Add(previousItem)
	}
	return true
}

// EquipItemFromInventory attempts to equip an item from inventory to a character
func (p *Party) EquipItemFromInventory(itemIndex, characterIndex int, owner ...*MMCharacter) bool {
	return p.equipFromInventory(itemIndex, characterIndex, func(c *MMCharacter, item items.Item) (items.Item, bool, bool) {
		return c.EquipItem(item)
	}, owner...)
}

// EquipItemFromInventoryToSlot equips an inventory item into a SPECIFIC slot
// (drag-drop onto an exact paperdoll slot), so a ring goes to the finger it was
// dropped on. Mirrors EquipItemFromInventory otherwise (inventory removal +
// displaced item returned to the bag).
func (p *Party) EquipItemFromInventoryToSlot(itemIndex, characterIndex int, slot items.EquipSlot, owner ...*MMCharacter) bool {
	return p.equipFromInventory(itemIndex, characterIndex, func(c *MMCharacter, item items.Item) (items.Item, bool, bool) {
		return c.EquipItemToSlot(item, slot)
	}, owner...)
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
func (p *Party) UnequipItemToInventory(slot items.EquipSlot, characterIndex int, owner ...*MMCharacter) bool {
	if characterIndex < 0 || characterIndex >= len(p.Members) {
		return false
	}

	character := p.Members[characterIndex]

	item, success := character.UnequipItem(slot)
	if !success {
		return false
	}
	if slot != items.SlotSpell {
		p.Bag(owner...).Add(item)
	}
	return true
}

// AverageLevel is the floor of the active roster's mean, shared by world gates
// and score displays. Empty and not-yet-created parties begin at level one.
func (p *Party) AverageLevel() int {
	total, count := 0, 0
	if p != nil {
		for _, member := range p.Members {
			if member != nil {
				total += member.Level
				count++
			}
		}
	}
	if count == 0 {
		return 1
	}
	return max(1, total/count)
}

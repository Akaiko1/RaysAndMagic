package character

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
)

type Party struct {
	Members   []*MMCharacter
	Gold      int
	Food      int
	Inventory []items.Item
	// Reserve holds benched heroes available at the tavern (swappable into the
	// active party). They keep all gear/XP/skills and level alongside the party.
	Reserve []*MMCharacter
	// Captive holds heroes still imprisoned (e.g. the mountain prison). They also
	// level alongside the party from the start, but aren't usable until freed —
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
// race modifiers) — the SAME path NewParty uses; exported so the map editor
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

// newPartyBase allocates a party with the configured starting gold/food and an
// empty inventory — the shared shell for every new-game constructor.
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
	// Tavern recruits start benched in the reserve — available at the tavern
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

// AddItem adds an item to the party inventory
func (p *Party) AddItem(item items.Item) {
	p.Inventory = append(p.Inventory, item)
}

// RemoveItem removes an item from the party inventory by index
func (p *Party) RemoveItem(index int) {
	if index >= 0 && index < len(p.Inventory) {
		p.Inventory = append(p.Inventory[:index], p.Inventory[index+1:]...)
	}
}

// GetTotalItems returns the number of items in the party inventory
func (p *Party) GetTotalItems() int {
	return len(p.Inventory)
}

// EquipItemFromInventory attempts to equip an item from inventory to a character
func (p *Party) EquipItemFromInventory(itemIndex, characterIndex int) bool {
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

	// Try to equip the item
	previousItem, hadPreviousItem, success := character.EquipItem(item)
	if success {
		// Successfully equipped - remove item from inventory
		p.RemoveItem(itemIndex)

		// Add the previously equipped item back to inventory (if any).
		// Spells are spellbook-owned and must never leak into inventory.
		if hadPreviousItem && previousItem.Type != items.ItemBattleSpell && previousItem.Type != items.ItemUtilitySpell {
			p.AddItem(previousItem)
		}
		return true
	}
	return false
}

// UnequipItemToInventory removes an item from a character's equipment and adds it to inventory.
// Spell-slot items are never returned to inventory — the spellbook is the only owner
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

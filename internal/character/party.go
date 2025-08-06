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
}

func NewParty(cfg *config.Config) *Party {
	party := &Party{
		Members:   make([]*MMCharacter, 0, 4),
		Gold:      cfg.Characters.StartingGold,
		Food:      cfg.Characters.StartingFood,
		Inventory: make([]items.Item, 0),
	}

	// Create starting party with classic classes
	party.AddMember(CreateCharacter("Gareth", ClassKnight, cfg))
	party.AddMember(CreateCharacter("Lysander", ClassSorcerer, cfg))
	party.AddMember(CreateCharacter("Celestine", ClassCleric, cfg))
	party.AddMember(CreateCharacter("Silvelyn", ClassArcher, cfg))

	// Add some starting items using YAML definitions
	party.AddItem(items.CreateWeaponFromYAML("iron_spear")) // Alternative weapon
	party.AddItem(items.CreateWeaponFromYAML("bow_of_hellfire")) // Legendary bow for debugging
	party.AddItem(items.Item{Name: "Leather Armor", Type: items.ItemArmor, Description: "Simple leather protection"})
	party.AddItem(items.Item{Name: "Health Potion", Type: items.ItemConsumable, Description: "Restores health"})
	party.AddItem(items.Item{Name: "Magic Ring", Type: items.ItemAccessory, Description: "Increases mana"})

	return party
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
	
	// Try to equip the item
	previousItem, hadPreviousItem, success := character.EquipItem(item)
	if success {
		// Successfully equipped - remove item from inventory
		p.RemoveItem(itemIndex)
		
		// Add the previously equipped item back to inventory (if any)
		if hadPreviousItem {
			p.AddItem(previousItem)
		}
		return true
	}
	return false
}

// UnequipItemToInventory removes an item from a character's equipment and adds it to inventory
func (p *Party) UnequipItemToInventory(slot items.EquipSlot, characterIndex int) bool {
	if characterIndex < 0 || characterIndex >= len(p.Members) {
		return false
	}
	
	character := p.Members[characterIndex]
	
	// Try to unequip the item
	if item, success := character.UnequipItem(slot); success {
		// Add the unequipped item to inventory
		p.AddItem(item)
		return true
	}
	return false
}

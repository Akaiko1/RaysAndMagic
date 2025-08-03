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

	// Add some starting items
	party.AddItem(items.Item{Name: "Iron Sword", Type: items.ItemWeapon, Description: "A basic iron sword"})
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

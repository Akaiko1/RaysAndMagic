package character

import "fmt"

// GuildServiceConfig authors the party-wide membership gate, independently of
// the building's sprite/model representation and its map placement.
type GuildServiceConfig struct {
	MembershipGold    int            `yaml:"membership_gold"`
	NonKeyDoorsOpened int            `yaml:"non_key_doors_opened"`
	Art               GuildLedgerArt `yaml:"art"`
}

// GuildLedgerArt supplies the ledger's dedicated report miniatures.
type GuildLedgerArt struct {
	Merchant string `yaml:"merchant"`
	Town     string `yaml:"town"`
	Dungeon  string `yaml:"dungeon"`
	Creature string `yaml:"creature"`
	Wildlife string `yaml:"wildlife"`
	Caravan  string `yaml:"caravan"`
}

// Sprites returns every required report miniature for validation and preload.
func (a GuildLedgerArt) Sprites() []string {
	return []string{a.Merchant, a.Town, a.Dungeon, a.Creature, a.Wildlife, a.Caravan}
}

func validateGuildService(key string, n *NPCData) error {
	if n == nil {
		return nil
	}
	if n.Type != NPCTypeThievesGuild {
		if n.Guild != nil {
			return fmt.Errorf("NPC %q: guild requires type thieves_guild", key)
		}
		return nil
	}
	if n.Guild == nil || n.Guild.MembershipGold <= 0 || n.Guild.NonKeyDoorsOpened <= 0 {
		return fmt.Errorf("NPC %q: guild requires positive membership_gold and non_key_doors_opened", key)
	}
	for _, name := range n.Guild.Art.Sprites() {
		if name == "" {
			return fmt.Errorf("NPC %q: guild.art requires all six report miniatures", key)
		}
	}
	if n.RenderCategory != "landmark" || n.HideWhenVisited || n.Dialogue == nil || n.Dialogue.Greeting == "" {
		return fmt.Errorf("NPC %q: guild requires a visible landmark and greeting", key)
	}
	if n.TownPortal || len(n.Inventory) > 0 || n.StockWeaponsRarity != "" || len(n.Spells) > 0 || len(n.Dialogue.Choices) > 0 {
		return fmt.Errorf("NPC %q: guild owns its ledger and membership-gated portal; ordinary portal, stock, spells and choices are not supported", key)
	}
	return nil
}

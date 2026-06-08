package character

import (
	"fmt"
	"os"
	"strings"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"

	"gopkg.in/yaml.v3"
)

// NPCConfig represents the structure of the npcs.yaml file
type NPCConfig struct {
	NPCs map[string]*NPCData `yaml:"npcs"`
}

// NPCData represents an NPC definition from the YAML file
type NPCData struct {
	Name             string               `yaml:"name"`
	Type             string               `yaml:"type"`
	Description      string               `yaml:"description"`
	Sprite           string               `yaml:"sprite"`
	RenderType       string               `yaml:"render_type,omitempty"`
	Transparent      bool                 `yaml:"transparent,omitempty"`
	GroundTile       string               `yaml:"ground_tile,omitempty"`
	SizeMultiplier   float64              `yaml:"size_multiplier,omitempty"`
	SellAvailable    bool                 `yaml:"sell_available,omitempty"`
	SteamWhenVisited bool                 `yaml:"steam_when_visited,omitempty"` // emit steam particles once Visited (e.g. a shut culvert valve)
	Dialogue         *NPCDialogue         `yaml:"dialogue"`
	Spells           map[string]*NPCSpell `yaml:"spells,omitempty"`
	Inventory        []*NPCItem           `yaml:"inventory,omitempty"`
	Encounter        *NPCEncounter        `yaml:"encounter,omitempty"`
	Summons          []*NPCSummon         `yaml:"summons,omitempty"`
}

// NPCSummon maps a held statuette (by item Name) to the monster a statue
// summons when that statuette is offered, plus a short label for the choice.
type NPCSummon struct {
	Statuette string `yaml:"statuette"`
	Monster   string `yaml:"monster"`
	Label     string `yaml:"label"`
}

// NPCDialogue represents the dialogue options for an NPC
type NPCDialogue struct {
	Greeting         string `yaml:"greeting"`
	Teaching         string `yaml:"teaching,omitempty"`
	InsufficientGold string `yaml:"insufficient_gold,omitempty"`
	AlreadyKnown     string `yaml:"already_known,omitempty"`
	Success          string `yaml:"success,omitempty"`
	VisitedMessage   string `yaml:"visited_message,omitempty"`
	// Quest-state bodies for quest-giver NPCs: ActiveMessage shows while the
	// linked quest is taken-but-not-done, CompletedMessage once it's done (turn-in
	// available). The offer state uses Greeting; the concluded state uses
	// VisitedMessage. See npc_dialogue.go.
	ActiveMessage    string               `yaml:"active_message,omitempty"`
	CompletedMessage string               `yaml:"completed_message,omitempty"`
	ChoicePrompt     string               `yaml:"choice_prompt,omitempty"`
	Choices          []*NPCDialogueChoice `yaml:"choices,omitempty"`
}

// NPCDialogueChoice represents a dialogue choice option
type NPCDialogueChoice struct {
	Text    string `yaml:"text"`
	Action  string `yaml:"action"`
	Map     string `yaml:"map,omitempty"`
	QuestID string `yaml:"quest_id,omitempty"` // for give_quest / turn_in_quest actions
	// SummonIndex is set at runtime (not from YAML) when statue summon choices
	// are built from the held statuettes; it indexes NPC.Summons.
	SummonIndex int `yaml:"-"`
}

// NPCEncounter represents an encounter definition
type NPCEncounter struct {
	Type           string                    `yaml:"type"`
	Monsters       []*EncounterMonster       `yaml:"monsters"`
	Rewards        *monster.EncounterRewards `yaml:"rewards"`
	FirstVisitOnly bool                      `yaml:"first_visit_only"`
	StartMessage   string                    `yaml:"start_message,omitempty"`
	// Quest integration - creates a quest when encounter is triggered
	QuestID          string `yaml:"quest_id"`          // Unique ID for the encounter quest
	QuestName        string `yaml:"quest_name"`        // Display name in quest log
	QuestDescription string `yaml:"quest_description"` // Description shown in quest tab
}

// EncounterMonster represents a monster in an encounter
type EncounterMonster struct {
	Type     string `yaml:"type"`
	CountMin int    `yaml:"count_min"`
	CountMax int    `yaml:"count_max"`
}

// NPCSpell represents a spell that an NPC can teach
type NPCSpell struct {
	Name         string             `yaml:"name"`
	School       string             `yaml:"school"`
	Level        int                `yaml:"level"`
	Cost         int                `yaml:"cost"`
	Description  string             `yaml:"description"`
	Requirements *SpellRequirements `yaml:"requirements,omitempty"`
}

// SpellRequirements represents requirements to learn a spell
type SpellRequirements struct {
	MinLevel int                      `yaml:"min_level,omitempty"`
	Schools  []SpellSchoolRequirement `yaml:"schools,omitempty"`
}

// SpellSchoolRequirement represents a required magic school level.
type SpellSchoolRequirement struct {
	School   string `yaml:"school"`
	MinLevel int    `yaml:"min_level,omitempty"`
}

// NPCItem represents an item that an NPC can sell
type NPCItem struct {
	Type     string `yaml:"type"`
	Name     string `yaml:"name"`
	Cost     int    `yaml:"cost"`
	Quantity int    `yaml:"quantity"`
}

// Global NPC configuration
var NPCConfigInstance *NPCConfig

// LoadNPCConfig loads NPC configuration from a YAML file
func LoadNPCConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read NPC config file: %w", err)
	}

	var config NPCConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("failed to parse NPC config: %w", err)
	}

	NPCConfigInstance = &config
	if err := backfillTraderSpells(); err != nil {
		return err
	}
	return nil
}

// backfillTraderSpells fills each spell_trader entry's intrinsic data (name,
// school, level, description, min-level gate) from spells.yaml keyed by the entry
// ID, so a catalog only authors the price. Cost stays per-entry (a shop property)
// and is required (fail-fast). Spells must already be loaded.
func backfillTraderSpells() error {
	if NPCConfigInstance == nil {
		return nil
	}
	for npcKey, npc := range NPCConfigInstance.NPCs {
		if npc == nil || npc.Type != "spell_trader" {
			continue
		}
		for id, sp := range npc.Spells {
			if sp == nil {
				sp = &NPCSpell{}
				npc.Spells[id] = sp
			}
			if sp.Cost <= 0 {
				return fmt.Errorf("spell_trader %q: spell %q must declare a positive cost", npcKey, id)
			}
			def, ok := config.GetSpellDefinition(id)
			if !ok || def == nil {
				continue // unknown spell ID → respect whatever the YAML gave
			}
			if sp.Name == "" {
				sp.Name = def.Name
			}
			if sp.School == "" {
				sp.School = def.School
			}
			if sp.Level == 0 {
				sp.Level = def.Level
			}
			if sp.Description == "" {
				sp.Description = def.Description
			}
			if sp.Requirements == nil {
				// Gate purchase on the spell's own level; the school-open check is
				// already enforced by canCharacterLearnNPCSpell.
				sp.Requirements = &SpellRequirements{MinLevel: def.Level}
			}
		}
	}
	return nil
}

// MustLoadNPCConfig loads NPC configuration and panics on error
func MustLoadNPCConfig(filename string) {
	if err := LoadNPCConfig(filename); err != nil {
		panic(fmt.Sprintf("Failed to load NPC config: %v", err))
	}
}

// GetNPCData returns NPC data by key
func (nc *NPCConfig) GetNPCData(key string) (*NPCData, bool) {
	data, exists := nc.NPCs[key]
	return data, exists
}

// CreateNPCFromConfig creates an NPC instance from configuration data
func CreateNPCFromConfig(key string, x, y float64) (*NPC, error) {
	if NPCConfigInstance == nil {
		return nil, fmt.Errorf("NPC config not loaded")
	}

	data, exists := NPCConfigInstance.GetNPCData(key)
	if !exists {
		return nil, fmt.Errorf("NPC data not found for key: %s", key)
	}

	npc := &NPC{
		X:                x,
		Y:                y,
		Name:             data.Name,
		Type:             data.Type,
		Description:      data.Description,
		Sprite:           data.Sprite,
		RenderType:       data.RenderType,
		Transparent:      data.Transparent,
		GroundTile:       data.GroundTile,
		SizeMultiplier:   data.SizeMultiplier,
		SellAvailable:    data.SellAvailable,
		SteamWhenVisited: data.SteamWhenVisited,
		DialogueData:     data.Dialogue,
		Summons:          data.Summons,
	}

	// Set up type-specific data
	switch data.Type {
	case "spell_trader":
		npc.SpellData = data.Spells
	case "merchant":
		npc.MerchantStock = buildMerchantStock(data.Inventory)
	case "encounter":
		npc.EncounterData = data.Encounter
	}

	return npc, nil
}

func buildMerchantStock(entries []*NPCItem) []*MerchantStockItem {
	if len(entries) == 0 {
		return nil
	}
	stock := make([]*MerchantStockItem, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Name == "" {
			continue
		}
		item, ok := createMerchantItem(entry)
		if !ok {
			continue
		}
		cost := entry.Cost
		if cost <= 0 {
			cost = item.Attributes["value"]
		}
		qty := entry.Quantity
		if qty <= 0 {
			qty = 1
		}
		stock = append(stock, &MerchantStockItem{
			Item:     item,
			Cost:     cost,
			Quantity: qty,
		})
	}
	return stock
}

func createMerchantItem(entry *NPCItem) (items.Item, bool) {
	itemType := strings.ToLower(entry.Type)
	switch itemType {
	case "weapon":
		key := items.GetWeaponKeyByName(entry.Name)
		weapon, err := items.TryCreateWeaponFromYAML(key)
		if err != nil {
			return items.Item{}, false
		}
		return weapon, true
	default:
		_, key, ok := config.GetItemDefinitionByName(entry.Name)
		if !ok {
			return items.Item{}, false
		}
		item, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			return items.Item{}, false
		}
		return item, true
	}
}

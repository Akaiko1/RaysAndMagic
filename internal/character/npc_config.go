package character

import (
	"fmt"
	"os"
	"ugataima/internal/items"
	"ugataima/internal/monster"

	"gopkg.in/yaml.v3"
)

// NPCConfig represents the structure of the npcs.yaml file
type NPCConfig struct {
	NPCs           map[string]*NPCData       `yaml:"npcs"`
	PlacementRules map[string]*PlacementRule `yaml:"placement_rules"`
}

// NPCData represents an NPC definition from the YAML file
type NPCData struct {
	Name        string               `yaml:"name"`
	Type        string               `yaml:"type"`
	Description string               `yaml:"description"`
	Sprite      string               `yaml:"sprite"`
	RenderType  string               `yaml:"render_type,omitempty"`
	Transparent bool                 `yaml:"transparent,omitempty"`
	Dialogue    *NPCDialogue         `yaml:"dialogue"`
	Spells      map[string]*NPCSpell `yaml:"spells,omitempty"`
	Inventory   []*NPCItem           `yaml:"inventory,omitempty"`
	Quests      []*NPCQuest          `yaml:"quests,omitempty"`
	Encounter   *NPCEncounter        `yaml:"encounter,omitempty"`
}

// NPCDialogue represents the dialogue options for an NPC
type NPCDialogue struct {
	Greeting         string               `yaml:"greeting"`
	Teaching         string               `yaml:"teaching,omitempty"`
	InsufficientGold string               `yaml:"insufficient_gold,omitempty"`
	AlreadyKnown     string               `yaml:"already_known,omitempty"`
	Success          string               `yaml:"success,omitempty"`
	ChoicePrompt     string               `yaml:"choice_prompt,omitempty"`
	Choices          []*NPCDialogueChoice `yaml:"choices,omitempty"`
}

// NPCDialogueChoice represents a dialogue choice option
type NPCDialogueChoice struct {
	Text   string `yaml:"text"`
	Action string `yaml:"action"`
}

// NPCEncounter represents an encounter definition
type NPCEncounter struct {
	Type           string                    `yaml:"type"`
	Monsters       []*EncounterMonster       `yaml:"monsters"`
	Rewards        *monster.EncounterRewards `yaml:"rewards"`
	FirstVisitOnly bool                      `yaml:"first_visit_only"`
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
	SpellPoints  int                `yaml:"spell_points"`
	Cost         int                `yaml:"cost"`
	Description  string             `yaml:"description"`
	Requirements *SpellRequirements `yaml:"requirements,omitempty"`
}

// SpellRequirements represents requirements to learn a spell
type SpellRequirements struct {
	MinLevel      int `yaml:"min_level,omitempty"`
	MinWaterSkill int `yaml:"min_water_skill,omitempty"`
}

// NPCItem represents an item that an NPC can sell
type NPCItem struct {
	Type     string `yaml:"type"`
	Name     string `yaml:"name"`
	Cost     int    `yaml:"cost"`
	Quantity int    `yaml:"quantity"`
}

// NPCQuest represents a quest that an NPC can give
type NPCQuest struct {
	ID               string `yaml:"id"`
	Name             string `yaml:"name"`
	Description      string `yaml:"description"`
	RewardGold       int    `yaml:"reward_gold"`
	RewardExperience int    `yaml:"reward_experience"`
}

// PlacementRule represents rules for placing NPCs in the world
type PlacementRule struct {
	PreferredTiles           []string `yaml:"preferred_tiles"`
	AvoidTiles               []string `yaml:"avoid_tiles"`
	MinDistanceFromMonsters  float64  `yaml:"min_distance_from_monsters"`
	MinDistanceFromOtherNPCs float64  `yaml:"min_distance_from_other_npcs"`
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
		X:           x,
		Y:           y,
		Name:        data.Name,
		Type:        data.Type,
		Description: data.Description,
		Sprite:      data.Sprite,
		Dialogue:    make([]string, 0),
		Inventory:   make([]items.Item, 0),
		Services:    make([]NPCService, 0),
	}

	// Set up dialogue
	if data.Dialogue != nil {
		npc.Dialogue = append(npc.Dialogue, data.Dialogue.Greeting)
	}

	// Set up services based on NPC type
	switch data.Type {
	case "spell_trader":
		npc.Services = append(npc.Services, ServiceSpellTrading)
		npc.SpellData = data.Spells
		npc.DialogueData = data.Dialogue
	case "merchant":
		npc.Services = append(npc.Services, ServiceTrading)
		// Convert NPCItems to Items (would need Item conversion logic)
	case "quest_giver":
		npc.Services = append(npc.Services, ServiceQuests)
		// Set up quest data
	case "encounter":
		npc.Services = append(npc.Services, ServiceEncounter)
		npc.DialogueData = data.Dialogue
		npc.EncounterData = data.Encounter
		npc.RenderType = data.RenderType
		npc.Transparent = data.Transparent
	}

	return npc, nil
}

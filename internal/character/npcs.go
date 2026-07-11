package character

import "ugataima/internal/items"

type NPC struct {
	X, Y             float64
	Key              string // npcs.yaml key this NPC was created from
	Name             string
	Type             string
	Description      string
	Sprite           string
	RenderCategory   string // render class (standee/animated/wall_mounted/landmark/scenery/door/invisible); required, validated at load
	PromptVerb       string // interaction-hint verb override ("enter", ...); "" = derived from render_category
	Transparent      bool
	GroundTile       string // optional tile key to paint under the NPC (e.g. a portal stream)
	SizeClass        string // shared size tier (person, etc.); wins over SizeTiles
	SizeTiles        float64
	MerchantStock    []*MerchantStockItem
	Currency         string // "" = gold; "arena_points" = arena victory currency
	ArenaBoard       bool   // carries the champions' leaderboard dialog tab
	SellAvailable    bool
	SteamWhenVisited bool
	HideWhenVisited  bool
	RejectsLich      bool // Light-aligned ward (Mage Tower) - won't speak to a party with a Lich
	SpellData        map[string]*NPCSpell
	DialogueData     *NPCDialogue
	EncounterData    *NPCEncounter
	Summons          []*NPCSummon
	Lectern          *NPCLectern // spell-teaching book (loot-crate cousin); crate loot lives in loots.yaml
	Visited          bool
}

// NPC type discriminators that carry behavior (the authored `type:` field).
// One canonical spelling each - every consumer (validation, interaction
// dispatch, render treatment, editor palette) references these, never a bare
// literal, so a rename or a new walk-up prop is a single-site change.
const (
	NPCTypeLootCrate    = "loot_crate"
	NPCTypeSpellLectern = "spell_lectern"
)

// IsWalkUpPropType reports whether a `type:` is a walk-up interactable prop
// (chest, lectern): the party walks right onto its tile and uses it. These
// share render treatment (no on-tile skip, no near-cull) and an immediate-use
// interaction, distinct from person/scenery NPCs. Single source of truth for
// that membership, shared by the game and the map editor.
func IsWalkUpPropType(npcType string) bool {
	return npcType == NPCTypeLootCrate || npcType == NPCTypeSpellLectern
}

// NPCLectern is a spell-teaching book (type "spell_lectern"): teaches Spell
// (or a random spell from Pool) to the first party member with the school
// open who doesn't know it. Not consumed when nobody can learn.
type NPCLectern struct {
	Spell string   `yaml:"spell,omitempty"`
	Pool  []string `yaml:"pool,omitempty"`
}

type MerchantStockItem struct {
	Item     items.Item
	Cost     int
	Quantity int // UnlimitedStock (negative) = never sells out
}

// UnlimitedStock marks a merchant entry that never sells out.
const UnlimitedStock = -1

// CurrencyArenaPoints is the arena victory currency (party.ArenaPoints).
const CurrencyArenaPoints = "arena_points"

// InStock reports whether the entry can still be bought.
func (m *MerchantStockItem) InStock() bool { return m.Quantity != 0 }

// Take consumes one unit (no-op for unlimited stock).
func (m *MerchantStockItem) Take() {
	if m.Quantity > 0 {
		m.Quantity--
	}
}

type WorldItem struct {
	X, Y         float64
	Item         items.Item
	Respawnable  bool
	RespawnTimer int
}

type SkillTeacher struct {
	Name       string
	Skill      interface{} // Can be SkillType or MagicSchoolID
	MaxMastery SkillMastery
	X, Y       float64
	Cost       int
}

func NewSkillTeacher(name string, skill interface{}, maxMastery SkillMastery, x, y float64) *SkillTeacher {
	return &SkillTeacher{
		Name:       name,
		Skill:      skill,
		MaxMastery: maxMastery,
		X:          x,
		Y:          y,
		Cost:       TrainingCostForMastery(maxMastery),
	}
}

func TrainingCostForMastery(mastery SkillMastery) int {
	switch mastery {
	case MasteryNovice:
		return 100
	case MasteryExpert:
		return 500
	case MasteryMaster:
		return 2000
	case MasteryGrandMaster:
		return 10000
	default:
		return 100
	}
}

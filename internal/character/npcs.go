package character

import "ugataima/internal/items"

type NPC struct {
	X, Y             float64
	Name             string
	Type             string
	Description      string
	Sprite           string
	RenderCategory   string // render class (standee/animated/wall_mounted/landmark/scenery/door/invisible); required, validated at load
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
	Visited          bool
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

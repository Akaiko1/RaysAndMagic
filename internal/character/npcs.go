package character

import "ugataima/internal/items"

type NPC struct {
	X, Y             float64
	Name             string
	Type             string
	Description      string
	Sprite           string
	RenderType       string
	WallMounted      bool // render as a flush wall standee (slides to the nearest solid neighbour)
	Transparent      bool
	GroundTile       string // optional tile key to paint under the NPC (e.g. a portal stream)
	SizeMultiplier   float64
	MerchantStock    []*MerchantStockItem
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
	Quantity int
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

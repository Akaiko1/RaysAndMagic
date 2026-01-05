package character

import "ugataima/internal/items"

type NPC struct {
	X, Y           float64
	Name           string
	Type           string
	Description    string
	Sprite         string
	RenderType     string
	Transparent    bool
	SizeMultiplier float64
	MerchantStock  []*MerchantStockItem
	SellAvailable  bool
	SpellData      map[string]*NPCSpell
	DialogueData   *NPCDialogue
	EncounterData  *NPCEncounter
	Visited        bool
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
	Skill      interface{} // Can be SkillType or MagicSchool
	MaxMastery SkillMastery
	X, Y       float64
	Cost       int
}

func NewNPC(x, y float64, name string) *NPC {
	return &NPC{
		X:             x,
		Y:             y,
		Name:          name,
		MerchantStock: make([]*MerchantStockItem, 0),
	}
}

func NewSkillTeacher(name string, skill interface{}, maxMastery SkillMastery, x, y float64) *SkillTeacher {
	return &SkillTeacher{
		Name:       name,
		Skill:      skill,
		MaxMastery: maxMastery,
		X:          x,
		Y:          y,
		Cost:       calculateTeachingCost(maxMastery),
	}
}

func calculateTeachingCost(mastery SkillMastery) int {
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

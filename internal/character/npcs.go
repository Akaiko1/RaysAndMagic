package character

import "ugataima/internal/items"

type NPC struct {
	X, Y          float64
	Name          string
	Type          string
	Description   string
	Sprite        string
	RenderType    string
	Transparent   bool
	Dialogue      []string
	QuestGiver    bool
	Merchant      bool
	Inventory     []items.Item
	Services      []NPCService
	SpellData     map[string]*NPCSpell
	DialogueData  *NPCDialogue
	EncounterData *NPCEncounter
	Visited       bool // Track if this encounter has been visited
}

type NPCService int

const (
	ServiceTraining NPCService = iota
	ServiceHealing
	ServiceRepair
	ServiceIdentify
	ServiceSpellTrading
	ServiceTrading
	ServiceQuests
	ServiceEncounter
)

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
		X:         x,
		Y:         y,
		Name:      name,
		Dialogue:  make([]string, 0),
		Inventory: make([]items.Item, 0),
		Services:  make([]NPCService, 0),
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

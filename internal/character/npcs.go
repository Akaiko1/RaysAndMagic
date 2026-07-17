package character

import "ugataima/internal/items"

type NPC struct {
	X, Y             float64
	Key              string // npcs.yaml key this NPC was created from
	Name             string
	Type             string
	Description      string
	Sprite           string
	VisitedSprite    string // optional art swap once Visited (an emptied barrel closes)
	NoSpin           bool   // pin a non-person token to a fixed pose
	GridSpanTiles    int    // >=2: render as a grid-aligned facade slab spanning this many tiles (clock tower); 0 = normal
	GridSpanDir      string // span direction from the anchor tile: "e"|"s" (the slab runs along it)
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
	NPCTypeEncounter     = "encounter"
	NPCTypeQuestGiver    = "quest_giver"
	NPCTypeMerchant      = "merchant"
	NPCTypeSpellTrader   = "spell_trader"
	NPCTypeSkillTrainer  = "skill_trainer"
	NPCTypeCardCollector = "card_collector"
	NPCTypeLootCrate     = "loot_crate"
	NPCTypeSpellLectern  = "spell_lectern"
)

// ValidNPCTypes is the closed set of authored NPC `type:` values. REQUIRED on
// every npcs.yaml entry and validated fail-fast at load: type drives behavior
// dispatch (dialog kind, walk-up interaction) AND the editor palette grouping,
// so a typo must fail loud, not fall into the default dialog branch.
var ValidNPCTypes = map[string]bool{
	NPCTypeEncounter: true, NPCTypeQuestGiver: true, NPCTypeMerchant: true,
	NPCTypeSpellTrader: true, NPCTypeSkillTrainer: true, NPCTypeCardCollector: true,
	NPCTypeLootCrate: true, NPCTypeSpellLectern: true,
}

// NPCTypeOrder is the canonical editor palette section order for NPC types:
// people you deal with first, props after, the encounter catch-all last.
var NPCTypeOrder = []string{
	NPCTypeQuestGiver, NPCTypeMerchant, NPCTypeSpellTrader, NPCTypeSkillTrainer,
	NPCTypeCardCollector, NPCTypeSpellLectern, NPCTypeLootCrate, NPCTypeEncounter,
}

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
	Quantity int    // UnlimitedStock (negative) = never sells out
	Tab      string // shop tab label ("" = the classic single grid)
}

// MerchantTabs lists the distinct shop tab labels in authored stock order;
// empty for a classic untabbed merchant.
func MerchantTabs(stock []*MerchantStockItem) []string {
	var tabs []string
	seen := map[string]bool{}
	for _, m := range stock {
		if m == nil || m.Tab == "" || seen[m.Tab] {
			continue
		}
		seen[m.Tab] = true
		tabs = append(tabs, m.Tab)
	}
	return tabs
}

// UnlimitedStock marks a merchant entry that never sells out.
const UnlimitedStock = -1

// CurrencyArenaPoints is the arena victory currency (party.ArenaPoints).
const CurrencyArenaPoints = "arena_points"

// CurrencyItemPrefix marks an item-backed merchant currency: "item:<items.yaml
// key>". The merchant trades at flat prices paid by consuming that many copies
// of the item from the party inventory (the clock tower's clock hands).
const CurrencyItemPrefix = "item:"

// CurrencyItemKey extracts the item key from an item-backed currency string;
// ok=false for gold/arena_points.
func CurrencyItemKey(currency string) (string, bool) {
	if len(currency) > len(CurrencyItemPrefix) && currency[:len(CurrencyItemPrefix)] == CurrencyItemPrefix {
		return currency[len(CurrencyItemPrefix):], true
	}
	return "", false
}

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

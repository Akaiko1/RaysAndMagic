package character

import (
	"fmt"
	"maps"
	"strings"
	"ugataima/internal/status"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// RareClassState travels with the hero, including while benched. All timers use
// simulation frames; a resolved TB round advances three seconds, never thinking time.
type RareClassState struct {
	FlowDistance float64         `json:"flow_distance,omitempty"`
	KitGranted   bool            `json:"kit_granted,omitempty"`
	Automatic    map[string]bool `json:"automatic,omitempty"`
	AutoFrames   int             `json:"auto_frames,omitempty"`
	PurifyFrames int             `json:"purify_frames,omitempty"`
	FlowFrames   int             `json:"flow_frames,omitempty"` // Legacy save field; migrated to one persistent charge on load.
	FlowCharges  int             `json:"flow_charges,omitempty"`
	Anchor       SpatialAnchor   `json:"anchor,omitempty"`
}

type SpatialAnchor struct {
	MapKey string  `json:"map_key,omitempty"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Frames int     `json:"frames,omitempty"`
}

func (r RareClassState) Clone() RareClassState {
	r.Automatic = maps.Clone(r.Automatic)
	return r
}

func (r *RareClassState) Tick(frames int) {
	r.AutoFrames = max(0, r.AutoFrames-frames)
	r.PurifyFrames = max(0, r.PurifyFrames-frames)
	r.Anchor.Frames = max(0, r.Anchor.Frames-frames)
	if r.Anchor.Frames == 0 {
		r.Anchor = SpatialAnchor{}
	}
}

func AlchemyYield(tier int, family string) int {
	switch family {
	case "protective":
		return masteryTableValue([4]int{1, 2, 3, 4}, tier)
	case "revival":
		return masteryTableValue([4]int{1, 2, 3, 4}, tier)
	default:
		return masteryTableValue([4]int{2, 3, 4, 6}, tier)
	}
}
func PharmacologyHP(tier int) int       { return masteryTableValue([4]int{20, 35, 50, 75}, tier) }
func PharmacologySP(tier int) int       { return masteryTableValue([4]int{10, 15, 20, 25}, tier) }
func FlowingStaffPct(tier int) int      { return masteryTableValue([4]int{25, 50, 75, 100}, tier) }
func FlowingStaffCapacity(tier int) int { return masteryTableValue([4]int{1, 2, 3, 4}, tier) }
func PathfindingSpeedPct(tier int) int  { return masteryTableValue([4]int{5, 10, 15, 20}, tier) }

const BombThrowingIntellectDivisor = 3

// The best capable active guide leads the shared party; guides do not stack.
func (p *Party) PathfindingTier() int {
	best := -1
	if p != nil {
		for _, c := range p.Members {
			if c != nil && c.CanAct() && c.HasSkill(SkillPathfinding) {
				best = max(best, c.SkillTier(SkillPathfinding))
			}
		}
	}
	return best
}

func (c *MMCharacter) FlowingStaffCharges() int {
	if c == nil || !c.HasSkill(SkillFlowingStaff) {
		return 0
	}
	return min(FlowingStaffCapacity(c.SkillTier(SkillFlowingStaff)), max(0, c.RareClass.FlowCharges))
}

func (c *MMCharacter) ConsumeFlowingStaffCharge() {
	if n := c.FlowingStaffCharges(); n > 0 {
		c.RareClass.FlowCharges = n - 1
	}
}

// Flowing Staff enhances only staff weapon attacks, including ranged staves.
// Damage, critical chance and tooltips share the same charge eligibility.
func (c *MMCharacter) HasChargedStaffAttack(def *config.WeaponDefinitionConfig) bool {
	return def != nil && def.Category == "staff" && c.FlowingStaffCharges() > 0
}

func rareSkillDescription(skill SkillType) string {
	switch skill {
	case SkillAlchemy:
		var yields []string
		for _, family := range []string{"common", "protective", "revival"} {
			yields = append(yields, family+": "+masteryProgression(func(tier int) int { return AlchemyYield(tier, family) }))
		}
		return "Batch yield - " + strings.Join(yields, "; ") + ". Flasks use common yields."
	case SkillPharmacology:
		return fmt.Sprintf("Party potion recovery: HP +%s%% + Intellect/3; SP +%s%%. Best capable active Alchemist; adds to Field Medicine. Revival unchanged.", masteryProgression(PharmacologyHP), masteryProgression(PharmacologySP))
	case SkillBombThrowing:
		return bombThrowingDescription()
	case SkillTranslocation:
		return translocationDescription()
	case SkillFlowingStaff:
		return fmt.Sprintf("Moving one tile refills %s staff charges. Each adds +%s%% normal damage and guarantees a critical staff attack. Charges remain until spent.", masteryProgression(FlowingStaffCapacity), masteryProgression(FlowingStaffPct))
	case SkillPathfinding:
		return fmt.Sprintf("Party RT movement speed +%s%%. Best capable active guide only.\n\nGrand Master:\nParty may attack and cast while running.", masteryProgression(PathfindingSpeedPct))
	}
	return ""
}

// Read the same definitions used by thrown projectiles. A reference without
// loaded content must not invent a second set of flask balance values.
func bombThrowingDescription() string {
	var lines []string
	keys := config.FlaskKeys()
	if len(keys) == 0 {
		return fmt.Sprintf("Flask damage + INT/%d.", BombThrowingIntellectDivisor)
	}
	var geometryOrder [][2]int
	geometryNames := map[[2]int][]string{}
	for _, key := range keys {
		d, _ := config.GetItemDefinition(key)
		f := d.Flask
		geometry := [2]int{f.RangeTiles, f.RadiusTiles}
		if len(geometryNames[geometry]) == 0 {
			geometryOrder = append(geometryOrder, geometry)
		}
		geometryNames[geometry] = append(geometryNames[geometry], d.Name)
		line := fmt.Sprintf("%s: %s + INT/%d %s damage", d.Name, masteryValues(f.Damage), BombThrowingIntellectDivisor, f.Element)
		if f.PoisonSeconds != [4]int{} {
			line += fmt.Sprintf("; poison %ss", masteryValues(f.PoisonSeconds))
		}
		if f.BurnSeconds != [4]int{} {
			line += fmt.Sprintf("; burning %ss", masteryValues(f.BurnSeconds))
		}
		lines = append(lines, line+".")
	}
	for _, geometry := range geometryOrder {
		label := strings.Join(geometryNames[geometry], ", ")
		if len(geometryOrder) == 1 {
			label = "All flasks"
		}
		lines = append(lines, fmt.Sprintf("%s: range %d tiles; radius %d tiles.", label, geometry[0], geometry[1]))
	}
	return strings.Join(lines, "\n")
}

func translocationDescription() string {
	var lines []string
	if config.GlobalTechniques != nil {
		for _, d := range config.GlobalTechniques.Techniques {
			var effects []string
			switch d.Key {
			case "fold_step":
				effects = append(effects, fmt.Sprintf("%s tiles; anchor %ss", masteryValues(d.Range), masteryValues(d.Duration)))
			case "phase_veil":
				effects = append(effects, fmt.Sprintf("dodge +%s%%; %ss", masteryValues(d.Power), masteryValues(d.Duration)))
			case "quickening":
				effects = append(effects, fmt.Sprintf("RT recovery -%s%%; TB +%s actions next turn; %ss", masteryValues(d.Power), masteryValues(d.TBPower), masteryValues(d.Duration)))
			}
			if d.SPCost != [4]int{d.SPCost[0], d.SPCost[0], d.SPCost[0], d.SPCost[0]} {
				effects = append(effects, masteryValues(d.SPCost)+" SP")
			}
			if len(effects) > 0 {
				lines = append(lines, d.Name+": "+strings.Join(effects, "; ")+".")
			}
		}
	}
	if len(lines) == 0 {
		return "Technique mastery."
	}
	return strings.Join(lines, "\n")
}

// GrantClassItems grants only active heroes' initial supplies. Benching/loading a
// hero never repeats this transaction. Missing catalogs in minimal fixtures defer it.
func (p *Party) GrantClassItems(cfg *config.Config) {
	if p == nil || cfg == nil || config.GlobalItems == nil {
		return
	}
	for _, ch := range p.Members {
		if ch == nil || ch.RareClass.KitGranted {
			continue
		}
		kit := cfg.Characters.Classes[ch.GetClassKey()].Items
		if len(kit) == 0 {
			continue
		}
		ready := make([]items.Item, 0, len(kit))
		for _, entry := range kit {
			item, err := items.TryCreateItemFromYAML(entry.Key)
			if err != nil {
				panic(fmt.Sprintf("class %s supplies: %v", ch.GetClassKey(), err))
			}
			item.Quantity = entry.Count
			ready = append(ready, item)
		}
		for _, item := range ready {
			p.AddItem(item)
		}
		ch.RareClass.KitGranted = true
	}
}

// Purifiable reports removable afflictions without treating a life state as a
// cleanse target. Timers count even when an older save lost its condition icon.
func (c *MMCharacter) Purifiable() bool {
	if c == nil || c.HasCondition(ConditionDead) || c.HasCondition(ConditionEradicated) {
		return false
	}
	if c.PoisonFramesRemaining > 0 || c.BurnFramesRemaining > 0 || c.IsStunned() {
		return true
	}
	for _, condition := range c.Conditions {
		if purifiableCondition(condition) {
			return true
		}
	}
	return false
}
func purifiableCondition(c Condition) bool {
	switch c {
	case ConditionPoisoned, ConditionDiseased, ConditionCursed, ConditionAsleep, ConditionFear, ConditionParalyzed, ConditionStone, ConditionBurning, ConditionStunned:
		return true
	}
	return false
}

// Purify clears afflictions and their clocks, preserving HP/SP and unconsciousness.
func (c *MMCharacter) Purify() bool {
	if !c.Purifiable() {
		return false
	}
	c.CurePoison()
	status.Clear(&c.BurnFramesRemaining, &c.burnTickTimer)
	c.StunFramesRemaining, c.StunTurnsRemaining, c.StunRate = 0, 0, 0
	kept := c.Conditions[:0]
	for _, condition := range c.Conditions {
		if !purifiableCondition(condition) {
			kept = append(kept, condition)
		}
	}
	c.Conditions = kept
	return true
}

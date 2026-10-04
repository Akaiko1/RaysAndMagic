package game

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// AdventureState owns opening-specific controls, cadence and grants. The normal
// world snapshot owns monsters, NPC stock, containers and return poses.
type AdventureState struct {
	Visits   map[string]*AdventureVisit `json:"visits,omitempty"`
	Records  map[string]bool            `json:"records,omitempty"`
	Occupied string                     `json:"occupied,omitempty"`
}
type AdventureVisit struct {
	Loot        map[string]AdventureLoot     `json:"loot,omitempty"`
	Opening     string                       `json:"opening"`
	Element     string                       `json:"element"`
	Level       int                          `json:"level"`
	Generation  int                          `json:"generation"`
	Controls    map[string]bool              `json:"controls,omitempty"`
	Effects     map[string]*EnvironmentState `json:"effects,omitempty"`
	BossGranted bool                         `json:"boss_granted,omitempty"`
	BossActions int                          `json:"boss_actions,omitempty"`
	BossHealed  int                          `json:"boss_healed,omitempty"`
	HealCarry   float64                      `json:"heal_carry,omitempty"`
}
type EnvironmentState struct {
	SourceDamageCarry map[string]float64 `json:"source_damage_carry,omitempty"`
	DamageCarry       float64            `json:"damage_carry,omitempty"`
	Activated         bool               `json:"activated,omitempty"`
	Disarmed          bool               `json:"disarmed,omitempty"`
	Carry             float64            `json:"carry,omitempty"`
	Warning           float64            `json:"warning,omitempty"`
	Remaining         float64            `json:"remaining,omitempty"`
	Rearm             float64            `json:"rearm,omitempty"`
	WasInside         bool               `json:"was_inside,omitempty"`
	Triggered         bool               `json:"triggered,omitempty"`
	Suppressed        bool               `json:"suppressed,omitempty"`
}

func cloneAdventureState(s AdventureState) AdventureState {
	b, _ := json.Marshal(s)
	var out AdventureState
	_ = json.Unmarshal(b, &out)
	return out
}
func (g *MMGame) adventureConfig(key string) *config.AdventureConfig {
	if wm := world.GlobalWorldManager; wm != nil {
		if m := wm.MapConfigs[key]; m != nil {
			return g.projectAdventureConfig(key, m.Adventure)
		}
	}
	return nil
}
func (g *MMGame) adventureVisit(key string) *AdventureVisit {
	if g.adventure.Visits == nil {
		g.adventure.Visits = map[string]*AdventureVisit{}
	}
	v := g.adventure.Visits[key]
	if v == nil {
		v = &AdventureVisit{}
		g.adventure.Visits[key] = v
	}
	if v.Controls == nil {
		v.Controls = map[string]bool{}
	}
	if v.Effects == nil {
		v.Effects = map[string]*EnvironmentState{}
	}
	return v
}
func (g *MMGame) openingID() string {
	phase := "day"
	if g.dayNightIsNight {
		phase = "night"
	}
	return fmt.Sprintf("%d:%s", g.currentCalendarDay(), phase)
}
func (g *MMGame) canPrepareSpell(id string) bool {
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
	if err != nil || g.party == nil || g.partyRooted() {
		return false
	}
	for _, c := range g.party.Members {
		if c == nil || !c.CanUseCombatAction() || !c.KnowsSpell(def.ID) {
			continue
		}
		cost := def.SpellPointsCost
		if g.combat != nil {
			cost = g.combat.effectiveSpellCost(c, cost)
		}
		if c.SpellPoints >= cost {
			return true
		}
	}
	return false
}
func (g *MMGame) canPrepareAdventureTechnique(key string) bool {
	d := config.Technique(key)
	if d == nil || key != "fold_step" || g.party == nil || g.partyRooted() {
		return false
	}
	for _, c := range g.party.Members {
		if c != nil && c.Class == character.ClassWayfarer && c.CanUseCombatAction() && c.HasSkill(character.SkillTranslocation) && c.Level >= d.Level && c.SpellPoints >= g.techniqueSPCost(c, d) {
			return true
		}
	}
	return false
}

func (g *MMGame) enterAdventureSchedule(source string) error {
	a := g.adventureConfig(source)
	if a == nil || a.Schedule == nil {
		return fmt.Errorf("No receiving schedule is installed.")
	}
	if currentMapKey() != source {
		return fmt.Errorf("Use the receiving threshold to enter.")
	}
	s := a.Schedule
	q := g.questManager.GetQuest(s.RequiresQuest)
	if q == nil || !q.RewardsClaimed {
		return fmt.Errorf("Restore the receiving anchor and report back first.")
	}
	if !g.canPrepareSpell(s.RequiresSpell) && !g.canPrepareAdventureTechnique(s.AlternativeTechnique) {
		return fmt.Errorf("An active hero must be able to use Jump or Fold Step with enough spell points.")
	}
	dest := s.Destination(g.currentCalendarDay(), g.dayNightIsNight)
	if g.worldByKey(dest) == nil {
		return fmt.Errorf("The receiving chamber is unavailable.")
	}
	opening := g.openingID()
	v := g.adventure.Visits[dest]
	request := mapTransition{mapKey: dest, arrival: mapArrivalEntrance}
	if v == nil || v.Opening != opening {
		w, err := world.GlobalWorldManager.FreshAdventureMap(dest)
		if err != nil {
			return err
		}
		generation := 1
		if v != nil {
			generation = v.Generation + 1
		}
		next := &AdventureVisit{Opening: opening, Element: g.adventureConfig(dest).Element, Level: g.party.AverageLevel() + 5, Generation: generation}
		for _, m := range w.Monsters {
			scaleAdventureMonster(m, next.Level)
		}
		request.adventureWorld = w
		request.adventureVisit = next
	}
	return g.transitionToMap(request)
}
func (g *MMGame) adventureScheduleText(source string) string {
	a := g.adventureConfig(source)
	if a == nil || a.Schedule == nil {
		return ""
	}
	s := a.Schedule
	day, night := g.currentCalendarDay(), g.dayNightIsNight
	current := s.Destination(day, night)
	if night {
		day++
	}
	next := s.Destination(day, !night)
	name := func(key string) string {
		if m := world.GlobalWorldManager.MapConfigs[key]; m != nil {
			return m.Name
		}
		return key
	}
	cycle := g.dayNightCycleFrames()
	remaining := 0
	if cycle > 0 {
		f := g.dayNightFrames
		boundary := cycle/4 + 1
		if night {
			boundary = 3*cycle/4 + 1
		}
		remaining = (boundary - f + cycle) % cycle
	}
	secs := int(math.Ceil(float64(remaining) / float64(max(1, g.config.GetTPS()))))
	return fmt.Sprintf("Now: %s. Next: %s in %d:%02d. The destination is selected when you enter.", name(current), name(next), secs/60, secs%60)
}
func (g *MMGame) commitAdventureVisit(key string, v *AdventureVisit) {
	if g.adventure.Visits == nil {
		g.adventure.Visits = map[string]*AdventureVisit{}
	}
	g.adventure.Visits[key] = v
	g.adventureVisit(key)
	g.prepareAdventureLoot(key, v)
	g.groundContainers = slices.DeleteFunc(g.groundContainers, func(c GroundContainer) bool { return c.MapKey == key })
	delete(g.mapReturnPoses, key)
	for id, pop := range g.alchemy.Populations {
		if len(pop.Nodes) > 0 && pop.Nodes[0].Region == key {
			delete(g.alchemy.Populations, id)
		}
	}
	if config.GlobalAlchemySpawns != nil {
		for _, p := range config.GlobalAlchemySpawns.Populations {
			if p.Map == key {
				delete(g.alchemy.Populations, p.Map+":"+p.Key)
			}
		}
	}
	g.harvestRuntime = harvestRuntime{}
	g.terrainChanges = slices.DeleteFunc(g.terrainChanges, func(c TerrainChange) bool { return c.Map == key })
}
func regularMonsterHP(level int) float64 {
	return math.Exp(4.181 + .1255*float64(min(level, 25)) + .04*float64(max(0, level-25)))
}

// Scale from the authored archetype, never from already-scaled runtime values.
func scaleAdventureMonster(m *monster.Monster3D, level int) {
	if m == nil || level < 1 || monster.MonsterConfig == nil {
		return
	}
	d, ok := monster.MonsterConfig.Monsters[m.Key]
	if !ok || d.Level < 1 {
		return
	}
	ratio := regularMonsterHP(level) / regularMonsterHP(d.Level)
	if m.IsBoss() {
		ratio = math.Exp(.101 * float64(level-d.Level))
	}
	m.AdventureScaleLevel = level
	m.Level = level
	m.MaxHitPoints = max(1, int(math.Round(float64(d.MaxHitPoints)*ratio)))
	m.HitPoints = m.MaxHitPoints
	damageRatio := math.Exp(.057 * float64(level-d.Level))
	m.DamageMin = max(1, int(math.Round(float64(d.DamageMin)*damageRatio)))
	m.DamageMax = max(m.DamageMin, int(math.Round(float64(d.DamageMax)*damageRatio)))
	m.ArmorClass = max(0, int(math.Round(float64(d.ArmorClass)*math.Sqrt(float64(level)/float64(d.Level)))))
	xp := 4 * max(100*level, 8*level*level) / 15
	if m.IsBoss() {
		xp *= 3
	} else {
		xp = xp * 3 / 2
	}
	m.Experience = xp
	if d.EnrageAtHP > 0 {
		m.EnrageAtHP = max(1, int(math.Round(float64(m.MaxHitPoints)*float64(d.EnrageAtHP)/float64(d.MaxHitPoints))))
	}
}
func (g *MMGame) restoreAdventureMonster(m *monster.Monster3D, level int) {
	scaleAdventureMonster(m, level)
	if m != nil {
		if v := g.adventure.Visits[m.HomeMap]; v != nil {
			if a := g.adventureConfig(m.HomeMap); a != nil && a.Boss != nil && a.Boss.Monster == m.Key {
				for _, c := range a.Controls {
					if v.Controls[c.ID] {
						m.ArmorClass = max(a.Boss.ArmorFloor, m.ArmorClass-c.ArmorReduction)
					}
				}
			}
		}
	}
}

func (g *MMGame) respawnAuthoredMonsters(w *world.World3D) {
	previous := w.Monsters
	w.RespawnAuthoredMonsters()
	for _, m := range w.Monsters {
		// Controls survive ordinary respawns. Retained allies already have live
		// stats; restoring them would heal or weaken them a second time.
		if !slices.Contains(previous, m) {
			level := 0
			if v := g.adventure.Visits[m.HomeMap]; v != nil {
				level = v.Level
			}
			g.restoreAdventureMonster(m, level)
		}
	}
}

package game

import (
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestScenario is a declarative fresh-game fixture. Nothing dispatches on its
// name: new scenarios need only a catalog entry and an optional shell shortcut.
type TestScenario struct {
	Level                 int              `yaml:"level"`
	Party                 []ScenarioMember `yaml:"party"`
	Map                   string           `yaml:"map"`
	X                     int              `yaml:"x"`
	Y                     int              `yaml:"y"`
	SafeRadius            float64          `yaml:"safe_radius"`
	Angle                 float64          `yaml:"angle"`
	Gold                  int              `yaml:"gold"`
	Items                 []string         `yaml:"items"`
	Quests                []string         `yaml:"quests"`
	ClearMaps             []string         `yaml:"clear_maps"`
	CompleteNPCEncounters []string         `yaml:"complete_npc_encounters"`
	RewardMapEncounters   []string         `yaml:"reward_map_encounters"`
	SpeedTarget           int              `yaml:"speed_target"`
	EnduranceTarget       int              `yaml:"endurance_target"`
	LearnSchoolSpells     bool             `yaml:"learn_school_spells"`
}
type ScenarioMember struct {
	Name      string            `yaml:"name"`
	Class     string            `yaml:"class"`
	Skills    map[string]string `yaml:"skills"`
	Equipment []string          `yaml:"equipment"`
}

func LoadTestScenarios(path string) (map[string]TestScenario, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var data struct {
		Scenarios map[string]TestScenario `yaml:"scenarios"`
	}
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("scenario catalog must contain one YAML document")
	}
	if len(data.Scenarios) == 0 {
		return nil, fmt.Errorf("empty scenario catalog")
	}
	for key, s := range data.Scenarios {
		if key == "" || strings.ContainsAny(key, "/\\.") || s.Level < 0 || s.Level > 100 || len(s.Party) > 4 || s.Gold < 0 || s.SpeedTarget < 0 || s.EnduranceTarget < 0 || s.SafeRadius < 0 || (s.SafeRadius > 0 && s.Map == "") {
			return nil, fmt.Errorf("invalid scenario %q", key)
		}
		for _, m := range s.Party {
			if _, ok := character.ClassFromKey(m.Class); !ok || m.Name == "" {
				return nil, fmt.Errorf("scenario %q: invalid member %q", key, m.Class)
			}
			for skill, tier := range m.Skills {
				if _, ok := character.SkillTypeFromKey(skill); !ok {
					return nil, fmt.Errorf("scenario %q: unknown skill %q", key, skill)
				}
				if _, ok := character.MasteryFromKey(tier); !ok {
					return nil, fmt.Errorf("scenario %q: unknown mastery %q", key, tier)
				}
			}
		}
	}
	return data.Scenarios, nil
}

func (g *MMGame) ApplyTestScenario(path, key string) error {
	catalog, err := LoadTestScenarios(path)
	if err != nil {
		return err
	}
	s, ok := catalog[key]
	if !ok {
		return fmt.Errorf("unknown test scenario %q", key)
	}
	return g.applyTestScenario(s)
}
func (g *MMGame) applyTestScenario(s TestScenario) error {
	// Resolve references before changing the live party or world.
	wm := world.GlobalWorldManager
	maps := append(append([]string(nil), s.ClearMaps...), s.RewardMapEncounters...)
	if s.Map != "" {
		maps = append(maps, s.Map)
	}
	for _, key := range maps {
		if wm == nil || wm.WorldByKey(key) == nil {
			return fmt.Errorf("scenario: unloaded map %q", key)
		}
	}
	for _, key := range s.CompleteNPCEncounters {
		d, ok := character.NPCConfigInstance.GetNPCData(key)
		if !ok || d.Encounter == nil {
			return fmt.Errorf("scenario: unknown NPC encounter %q", key)
		}
	}
	for _, key := range s.RewardMapEncounters {
		if wm.MapConfigs[key] == nil || wm.MapConfigs[key].ClearEncounter == nil {
			return fmt.Errorf("scenario: no map encounter in %q", key)
		}
	}
	for _, id := range s.Quests {
		if g.questManager == nil || g.questManager.Definitions()[id] == nil {
			return fmt.Errorf("scenario: unknown quest %q", id)
		}
	}
	var loot []items.Item
	for _, key := range s.Items {
		it, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			return err
		}
		loot = append(loot, it)
	}
	var members []*character.MMCharacter
	for _, data := range s.Party {
		class, ok := character.ClassFromKey(data.Class)
		if !ok {
			return fmt.Errorf("scenario: unknown class %q", data.Class)
		}
		m := character.CreateCharacter(data.Name, class, g.config)
		for key, tier := range data.Skills {
			skill, _ := character.SkillTypeFromKey(key)
			mastery, _ := character.MasteryFromKey(tier)
			if m.Skills[skill] == nil {
				return fmt.Errorf("scenario: %s does not start with %s", data.Name, key)
			}
			m.Skills[skill].Mastery = mastery
		}
		for _, key := range data.Equipment {
			it, err := items.TryCreateItemFromYAML(key)
			if err != nil {
				return err
			}
			if _, _, ok := m.EquipItem(it); !ok {
				return fmt.Errorf("scenario: %s cannot equip %s", data.Name, key)
			}
		}
		members = append(members, m)
	}
	if s.Map != "" {
		w := wm.WorldByKey(s.Map)
		x, y := projectTileToCurrentWorld(s.Map, s.X, s.Y)
		if x < 0 || y < 0 || x >= w.Width || y >= w.Height || w.IsTileBlockingTerrainAt(x, y) {
			return fmt.Errorf("scenario: blocked start %s (%d,%d)", s.Map, s.X, s.Y)
		}
	}
	if len(members) > 0 {
		g.party = character.NewPartyFromGroups(g.config, members, nil, nil)
	}
	g.appScreen = AppScreenInGame
	for _, key := range s.ClearMaps {
		xp, gold := g.clearMapAndTally(key)
		g.awardGold(gold)
		g.grantSharedXP(xp)
	}
	for _, key := range s.CompleteNPCEncounters {
		g.completeTestNPCEncounter(key)
	}
	for _, key := range s.RewardMapEncounters {
		g.completeTestMapEncounter(key)
	}
	for _, m := range g.party.Members {
		if m == nil {
			continue
		}
		for m.Level < s.Level {
			m.Experience += xpStepCost(m.Level)
			g.combat.checkLevelUp(m, false)
		}
		pts := m.FreeStatPoints
		pts -= raiseStat(&m.Speed, s.SpeedTarget, pts)
		pts -= raiseStat(&m.Endurance, s.EnduranceTarget, pts)
		addMainDamageStat(m, pts)
		m.FreeStatPoints = 0
		if s.LearnSchoolSpells {
			learnAllSchoolSpells(m)
		}
		m.CalculateDerivedStats(g.config)
		m.HitPoints = m.MaxHitPoints
		m.SpellPoints = m.MaxSpellPoints
	}
	g.awardGold(s.Gold)
	for _, it := range loot {
		g.party.AddItem(it)
	}
	if s.Map != "" {
		if err := g.switchToMap(s.Map); err != nil {
			return err
		}
		x, y := projectTileToCurrentWorld(s.Map, s.X, s.Y)
		px, py := TileCenterFromTile(x, y, float64(g.config.GetTileSize()))
		g.setPartyPosition(px, py)
		angle := s.Angle
		if wm.IsOpenWorldRegion(s.Map) {
			angle = wm.ProjectAngle(s.Map, angle)
		}
		g.snapFacing(angle)
		if s.SafeRadius > 0 {
			radius := s.SafeRadius * float64(g.config.GetTileSize())
			g.world.Monsters = slices.DeleteFunc(g.world.Monsters, func(m *monster.Monster3D) bool {
				if math.Hypot(m.X-px, m.Y-py) > radius {
					return false
				}
				g.collisionSystem.UnregisterEntity(m.ID)
				return true
			})
		}
	}
	for _, id := range s.Quests {
		if err := g.activateQuest(id); err != nil {
			return err
		}
	}
	g.AddCombatMessage("Test scenario ready. This is a fresh party; earned level-up choices remain available.")
	return nil
}

// TestScenarioArg also retains the old arena shortcut. Ambiguous/missing
// arguments are errors instead of quietly opening an ordinary new game.
func TestScenarioArg(args []string) (string, error) {
	result := ""
	for i := 0; i < len(args); i++ {
		value := ""
		switch {
		case args[i] == "--test-arena":
			value = "arena"
		case args[i] == "--test-scenario":
			i++
			if i >= len(args) {
				return "", fmt.Errorf("--test-scenario requires a catalog key")
			}
			value = args[i]
		case strings.HasPrefix(args[i], "--test-scenario="):
			value = strings.TrimPrefix(args[i], "--test-scenario=")
		default:
			continue
		}
		if result != "" || value == "" || strings.ContainsAny(value, "/\\.") || strings.HasPrefix(value, "-") {
			return "", fmt.Errorf("invalid or repeated test scenario argument")
		}
		result = value
	}
	return result, nil
}

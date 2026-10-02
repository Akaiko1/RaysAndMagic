package game

import (
	"bytes"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// BenchCatalog holds benchmark parties: realistic builds at fixed levels for
// balance checks and play-tests (assets/bench_parties.yaml).
type BenchCatalog struct {
	Levels  []int                 `yaml:"levels"`
	Parties map[string]BenchParty `yaml:"parties"`
}

type BenchParty struct {
	Note    string           `yaml:"note"`
	Members []BenchMember    `yaml:"members"`
	Cards   map[int][]string `yaml:"cards"`
	Items   map[int][]string `yaml:"items"`
	Gold    map[int]int      `yaml:"gold"`
}

type BenchMember struct {
	Name   string             `yaml:"name"`
	Class  string             `yaml:"class"`
	Race   string             `yaml:"race"`
	Builds map[int]BenchBuild `yaml:"levels"`
}

// BenchBuild is one member at one level. Skills and schools list only tiers
// above Novice; every open school learns all of its spells.
type BenchBuild struct {
	Skills    map[string]string `yaml:"skills"`
	Schools   map[string]string `yaml:"schools"`
	Equipment []string          `yaml:"equipment"`
	Spell     string            `yaml:"spell"`
	Quick     []string          `yaml:"quick"`
}

func LoadBenchCatalog(path string) (*BenchCatalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cat BenchCatalog
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cat); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(cat.Levels) == 0 || len(cat.Parties) == 0 {
		return nil, fmt.Errorf("%s: bench catalog needs levels and parties", path)
	}
	for key, p := range cat.Parties {
		if len(p.Members) == 0 || len(p.Members) > 4 {
			return nil, fmt.Errorf("bench party %q: 1-4 members", key)
		}
		for _, m := range p.Members {
			if _, ok := character.ClassFromKey(m.Class); !ok || m.Name == "" {
				return nil, fmt.Errorf("bench party %q: invalid member %q (%s)", key, m.Name, m.Class)
			}
			for _, level := range cat.Levels {
				if _, ok := m.Builds[level]; !ok {
					return nil, fmt.Errorf("bench party %q: %s has no level %d build", key, m.Name, level)
				}
			}
			for level := range m.Builds {
				if !slices.Contains(cat.Levels, level) {
					return nil, fmt.Errorf("bench party %q: %s builds undeclared level %d", key, m.Name, level)
				}
			}
		}
	}
	return &cat, nil
}

// buildBenchMembers creates the party through the normal progression: class
// kit, race, XP level-ups with the class auto stat chain, then the authored
// masteries, spells, gear and quick slots. Level-up choices are represented by
// the authored masteries, so their queued prompts are dropped.
func (g *MMGame) buildBenchMembers(p BenchParty, level int) ([]*character.MMCharacter, error) {
	var members []*character.MMCharacter
	for _, data := range p.Members {
		class, _ := character.ClassFromKey(data.Class)
		build := data.Builds[level]
		m := character.CreateCharacter(data.Name, class, g.config)
		if data.Race != "" && data.Race != "human" {
			if _, ok := g.config.Characters.Races[data.Race]; !ok {
				return nil, fmt.Errorf("bench: %s has unknown race %q", data.Name, data.Race)
			}
			m.ApplyRace(data.Race, g.config)
		}
		for m.Level < level {
			m.Experience += xpStepCost(m.Level)
			g.combat.checkLevelUp(m, false)
		}
		autoDistributeStatPoints(m, g.config)
		for key, tier := range build.Skills {
			skill, ok := character.SkillTypeFromKey(key)
			mastery, okTier := character.MasteryFromKey(tier)
			if !ok || !okTier || m.Skills[skill] == nil {
				return nil, fmt.Errorf("bench: %s cannot hold %s at %q", data.Name, key, tier)
			}
			m.Skills[skill].Mastery = mastery
		}
		for key, tier := range build.Schools {
			school := character.MagicSchoolID(key)
			mastery, ok := character.MasteryFromKey(tier)
			if !ok || !slices.Contains(character.AllMagicSchools, school) {
				return nil, fmt.Errorf("bench: %s has invalid school %s %q", data.Name, key, tier)
			}
			if m.MagicSchools[school] == nil {
				m.MagicSchools[school] = &character.MagicSkill{}
			}
			m.MagicSchools[school].Mastery = mastery
		}
		learnAllSchoolSpells(m)
		// The build lists everything worn; the kit keeps only its readied spell.
		for slot := range m.Equipment {
			if slot != items.SlotSpell {
				delete(m.Equipment, slot)
			}
		}
		for _, key := range build.Equipment {
			it, err := benchGear(key)
			if err != nil {
				return nil, err
			}
			if _, _, ok := m.EquipItem(it); !ok {
				return nil, fmt.Errorf("bench: %s cannot equip %s", data.Name, key)
			}
		}
		if build.Spell != "" {
			sp, err := spells.CreateSpellItem(spells.SpellID(build.Spell))
			if err != nil || !characterKnowsSpellByID(m, spells.SpellID(build.Spell)) {
				return nil, fmt.Errorf("bench: %s cannot ready spell %s", data.Name, build.Spell)
			}
			m.Equipment[items.SlotSpell] = sp
		}
		if len(build.Quick) > len(m.QuickSlots) {
			return nil, fmt.Errorf("bench: %s has %d quick slots", data.Name, len(m.QuickSlots))
		}
		for i, key := range build.Quick {
			it, err := benchQuickItem(key)
			if err != nil || !canBindQuickItem(m, &it) {
				return nil, fmt.Errorf("bench: %s cannot quick-slot %s", data.Name, key)
			}
			m.QuickSlots[i] = &it
		}
		m.CalculateDerivedStats(g.config)
		m.HitPoints, m.SpellPoints = m.MaxHitPoints, m.MaxSpellPoints
		members = append(members, m)
	}
	g.levelUpChoiceQueue = g.levelUpChoiceQueue[:0]
	g.closeLevelUpChoice()
	return members, nil
}

// applyBenchPartyState adds the party-wide part of a build after the party
// exists: the card collection, carried items and gold.
func (g *MMGame) applyBenchPartyState(p BenchParty, level int) error {
	cards := p.Cards[level]
	if len(cards) > MaxCardSlots {
		return fmt.Errorf("bench: %d cards exceed %d slots", len(cards), MaxCardSlots)
	}
	for i, key := range cards {
		it, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			return err
		}
		items.EnsureInstanceID(&it)
		if !g.setCardCollectionSlot(i, it) {
			return fmt.Errorf("bench: %s is not a collectible card", key)
		}
	}
	for _, key := range p.Items[level] {
		it, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			return err
		}
		g.party.AddItem(it)
	}
	g.awardGold(p.Gold[level])
	g.applyPartyStatBonuses()
	for _, m := range g.party.Members {
		if m != nil {
			m.HitPoints, m.SpellPoints = m.MaxHitPoints, m.MaxSpellPoints
		}
	}
	return nil
}

// benchGear creates a weapons.yaml or items.yaml piece by key.
func benchGear(key string) (items.Item, error) {
	if _, ok := config.GlobalWeapons.Weapons[key]; ok {
		return items.TryCreateWeaponFromYAML(key)
	}
	return items.TryCreateItemFromYAML(key)
}

// benchQuickItem resolves a quick-slot key the way the tabs bind it: a spell,
// trap, technique or flask shortcut, else a carried item.
func benchQuickItem(key string) (items.Item, error) {
	if sp, err := spells.CreateSpellItem(spells.SpellID(key)); err == nil {
		return sp, nil
	}
	if it, ok := config.TrapItem(key); ok {
		return it, nil
	}
	if it, ok := config.TechniqueItem(key); ok {
		return it, nil
	}
	if it, ok := config.FlaskItem(key); ok {
		return it, nil
	}
	return items.TryCreateItemFromYAML(key)
}

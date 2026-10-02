package game

import (
	"bytes"
	"fmt"
	"maps"
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
	Note    string                 `yaml:"note"`
	Members []BenchMember          `yaml:"members"`
	Cards   map[int][]string       `yaml:"cards"`
	Items   map[int]map[string]int `yaml:"items"` // carried items by key and count
	Gold    map[int]int            `yaml:"gold"`
	// A variant copies Base and lists only its changes: spells no member
	// learns, and readied actions by member name for every level.
	Base          string            `yaml:"base"`
	WithoutSpells []string          `yaml:"without_spells"`
	Spell         map[string]string `yaml:"spell"`
}

type BenchMember struct {
	Name   string             `yaml:"name"`
	Class  string             `yaml:"class"`
	Race   string             `yaml:"race"`
	Builds map[int]BenchBuild `yaml:"levels"`
	// From names another party whose member of the same name this one copies.
	From string `yaml:"from"`
}

// BenchBuild is one member at one level. Skills and schools list only tiers
// above Novice; every open school learns all of its spells. Spell is the
// readied action Space uses: a known spell, a flask, a trap or a technique.
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
		for i, m := range p.Members {
			if m.From == "" {
				continue
			}
			src, err := benchMemberFrom(cat.Parties, key, m)
			if err != nil {
				return nil, err
			}
			p.Members[i] = src
		}
	}
	for key, p := range cat.Parties {
		if p.Base == "" && (len(p.WithoutSpells) > 0 || len(p.Spell) > 0) {
			return nil, fmt.Errorf("bench party %q: without_spells and spell need a base", key)
		}
		if p.Base != "" {
			v, err := benchVariant(cat.Parties, key, p)
			if err != nil {
				return nil, err
			}
			cat.Parties[key] = v
		}
	}
	for key, p := range cat.Parties {
		if len(p.Members) == 0 || len(p.Members) > 4 {
			return nil, fmt.Errorf("bench party %q: 1-4 members", key)
		}
		for level, carried := range p.Items {
			for item, n := range carried {
				if n <= 0 {
					return nil, fmt.Errorf("bench party %q: L%d carries %d %s", key, level, n, item)
				}
			}
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

// benchMemberFrom resolves a member that names only itself and its source party.
func benchMemberFrom(parties map[string]BenchParty, key string, m BenchMember) (BenchMember, error) {
	if m.Class != "" || m.Race != "" || m.Builds != nil {
		return m, fmt.Errorf("bench party %q: %s copies %s and lists only its name", key, m.Name, m.From)
	}
	src, ok := parties[m.From]
	if !ok || src.Base != "" {
		return m, fmt.Errorf("bench party %q: %s copies from %q, which is not a party with members", key, m.Name, m.From)
	}
	for _, o := range src.Members {
		if o.Name == m.Name && o.From == "" {
			o.From = m.From
			return o, nil
		}
	}
	return m, fmt.Errorf("bench party %q: %s has no own member %s", key, m.From, m.Name)
}

// benchVariant copies the base party with the variant's changes applied.
func benchVariant(parties map[string]BenchParty, key string, v BenchParty) (BenchParty, error) {
	base, ok := parties[v.Base]
	if !ok || base.Base != "" {
		return v, fmt.Errorf("bench party %q: base %q must be a party that is not a variant", key, v.Base)
	}
	if len(v.Members) > 0 || v.Cards != nil || v.Items != nil || v.Gold != nil {
		return v, fmt.Errorf("bench party %q: a variant lists only base, without_spells and spell", key)
	}
	out := base
	out.Note, out.Base, out.WithoutSpells, out.Spell = v.Note, v.Base, v.WithoutSpells, v.Spell
	out.Members = make([]BenchMember, len(base.Members))
	named := map[string]bool{}
	for i, m := range base.Members {
		named[m.Name] = true
		builds := make(map[int]BenchBuild, len(m.Builds))
		for level, b := range m.Builds {
			if spell, ok := v.Spell[m.Name]; ok {
				b.Spell = spell
			}
			builds[level] = b
		}
		m.Builds = builds
		out.Members[i] = m
	}
	for name := range v.Spell {
		if !named[name] {
			return v, fmt.Errorf("bench party %q: no member %q in %s", key, name, v.Base)
		}
	}
	return out, nil
}

// buildBenchMembers creates the party through the normal progression: class
// kit, race, XP level-ups with the class auto stat chain, then the authored
// masteries, spells, gear and quick slots. Level-up choices are represented by
// the authored masteries, so their queued prompts are dropped.
func (g *MMGame) buildBenchMembers(p BenchParty, level int) ([]*character.MMCharacter, error) {
	var members []*character.MMCharacter
	withheld := map[string]bool{}
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
		for _, s := range m.MagicSchools {
			if s == nil {
				continue
			}
			s.KnownSpells = slices.DeleteFunc(s.KnownSpells, func(id spells.SpellID) bool {
				drop := slices.Contains(p.WithoutSpells, string(id))
				if drop {
					withheld[string(id)] = true
				}
				return drop
			})
		}
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
			action, err := benchQuickItem(build.Spell)
			if err != nil || action.Type == items.ItemConsumable || !canBindQuickItem(m, &action) {
				return nil, fmt.Errorf("bench: %s cannot ready %s", data.Name, build.Spell)
			}
			m.Equipment[items.SlotSpell] = action
		} else if kit := m.Equipment[items.SlotSpell].SpellEffect; slices.Contains(p.WithoutSpells, string(kit)) {
			return nil, fmt.Errorf("bench: %s still readies the kit's %s; ready another action", data.Name, kit)
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
	for _, id := range p.WithoutSpells {
		if !withheld[id] {
			return nil, fmt.Errorf("bench: no member would learn %s", id)
		}
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
	// The build lists everything carried: the new game's starting kit gives
	// way to it. Key items (the world map) stay.
	unlisted := func(it items.Item) bool { return it.Type != items.ItemQuest }
	g.party.Inventory = slices.DeleteFunc(g.party.Inventory, unlisted)
	for _, m := range g.party.Members {
		m.Inventory = slices.DeleteFunc(m.Inventory, unlisted)
	}
	carried := p.Items[level]
	for _, key := range slices.Sorted(maps.Keys(carried)) {
		for range carried[key] {
			it, err := items.TryCreateItemFromYAML(key)
			if err != nil {
				return err
			}
			g.party.AddItem(it)
		}
	}
	// A quick-slotted item comes out of that stock: one unit moves into the
	// slot and the rest stays in the shared bag, where every hero's automatic
	// drinking reaches it (a quick slot serves only its own hero).
	for _, m := range g.party.Members {
		for i, q := range m.QuickSlots {
			if q == nil || benchShortcut(*q) {
				continue
			}
			bag := g.party.Bag()
			at := slices.IndexFunc(bag.Items(), func(it items.Item) bool { return items.SameStack(it, *q) })
			if at < 0 {
				return fmt.Errorf("bench: %s quick-slots %s, which the party does not carry", m.Name, q.Name)
			}
			unit, _ := bag.Take(at, 1)
			m.QuickSlots[i] = &unit
		}
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

// benchShortcut: a quick-slot entry that points at a spell, technique, trap
// or flask stock instead of holding a carried item.
func benchShortcut(it items.Item) bool {
	switch it.Type {
	case items.ItemBattleSpell, items.ItemUtilitySpell, items.ItemTechnique, items.ItemTrap, items.ItemThrowable:
		return true
	}
	return false
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

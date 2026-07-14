package game

import (
	"sort"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Resist sources in the game are three data-driven families, and EVERY one must
// (1) raise the number the character sheet renders and (2) reduce real incoming
// damage by that exact number - both read the single schoolResistPct total.
// These tests enumerate each family straight from YAML, so a newly-authored
// resist spell / gear piece / card is covered automatically.

const resistTestDamage = 1000

func cappedResistPct(resist int) int {
	if resist > 100 {
		return 100
	}
	return resist
}

// expectedResistMitigation independently replays mitigateCharacterDamage's
// post-armor pipeline. The expected resistance comes from the authored source,
// not schoolResistPct, so this catches a bad source aggregation.
func expectedResistMitigation(g *MMGame, member *character.MMCharacter, base, resist int) int {
	if resist >= 100 {
		return 0
	}
	d := base * (100 - resist) / 100
	if d < 1 {
		d = 1
	}
	d -= member.DisarmTrapTier() * DisarmTrapDamageReductionPerTier
	d -= g.combatBuffInReduce()
	if d < 0 {
		d = 0
	}
	return d
}

// assertResistSource verifies the exact authored resistance total and the
// resulting damage. The physical summary panel is a third consumer of this
// total, so it is checked here too.
func assertResistSource(t *testing.T, cs *CombatSystem, member *character.MMCharacter, school string, wantResist int) {
	t.Helper()
	wantResist = cappedResistPct(wantResist)
	if got := cs.game.schoolResistPct(member, school); got != wantResist {
		t.Errorf("%s resistance = %d%%, want %d%%", school, got, wantResist)
	}
	if got, want := cs.mitigateCharacterDamage(resistTestDamage, school, member, true), expectedResistMitigation(cs.game, member, resistTestDamage, wantResist); got != want {
		t.Errorf("%s mitigation = %d, want %d at %d%% resistance", school, got, want, wantResist)
	}
	if school == "physical" {
		if got := cs.PhysicalMitigationBreakdown(member).ResistPct; got != wantResist {
			t.Errorf("physical mitigation breakdown = %d%%, want %d%%", got, wantResist)
		}
	}
}

// allResistSchools is the school column set the character sheet renders.
var allResistSchools = []string{"physical", "fire", "water", "air", "earth", "spirit", "mind", "body", "light", "dark"}

func resistSpellKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	for k := range config.GlobalSpells.Spells {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(k))
		if err != nil {
			continue
		}
		if def.ResistBuffPct > 0 || def.ResistBuffSchoolPct > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func resistGearKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	for k, def := range config.GlobalItems.Items {
		if len(def.Resistances) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func resistCardKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	for k, def := range config.GlobalItems.Items {
		if def.Type == "card" && len(def.CardResistBonus) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Every resist spell: casting it must raise the sheet total for the school(s) it
// covers (its own school for Fire Shield, every school for the all-damage Day of
// the Gods) and reduce real damage by that same total.
func TestResistSources_SpellsShowAndMitigate(t *testing.T) {
	newTestCombatSystemWithConfig(t) // populate config globals for enumeration
	keys := resistSpellKeys(t)
	if len(keys) == 0 {
		t.Fatal("no resist-granting spells found in spells.yaml - enumeration or filter broke")
	}
	for _, key := range keys {
		key := key
		t.Run(key, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			fillTestParty(t, cs.game)
			member := cs.game.party.Members[0]
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
			if err != nil {
				t.Fatalf("def %q: %v", key, err)
			}

			// An all-damage buff (resist_buff_pct) affects every school; a pure
			// per-school buff (resist_buff_school_pct) affects only its school.
			check := allResistSchools
			if def.ResistBuffPct <= 0 {
				check = []string{strings.ToLower(strings.TrimSpace(def.ResistBuffSchool))}
			}

			before := map[string]int{}
			for _, s := range check {
				before[s] = cs.game.schoolResistPct(member, s)
			}
			if !cs.tryCastPartyBuff(spells.SpellID(key), def, member) {
				t.Fatalf("resist spell %q did not activate as a party buff", key)
			}
			buff, ok := cs.game.combatBuffByID(key)
			if !ok {
				t.Fatalf("resist spell %q did not create a combat buff", key)
			}
			for _, s := range check {
				increase := buff.ResistPct
				if strings.EqualFold(buff.ResistSchool, s) {
					increase += buff.ResistSchoolPct
				}
				assertResistSource(t, cs, member, s, before[s]+increase)
			}
		})
	}
}

// Every gear piece carrying a resistances map: equipping it must raise the sheet
// total for each listed school and reduce real damage by that same total.
func TestResistSources_GearShowsAndMitigates(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	keys := resistGearKeys(t)
	if len(keys) == 0 {
		t.Fatal("no resist-granting gear found in items.yaml - enumeration or filter broke")
	}
	for _, key := range keys {
		key := key
		t.Run(key, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			fillTestParty(t, cs.game)
			member := cs.game.party.Members[0]
			it, err := items.TryCreateItemFromYAML(key)
			if err != nil {
				t.Fatalf("create %q: %v", key, err)
			}
			def, ok := config.GetItemDefinition(key)
			if !ok {
				t.Fatalf("no definition for %q", key)
			}

			before := map[string]int{}
			for s := range def.Resistances {
				before[strings.ToLower(s)] = cs.game.schoolResistPct(member, strings.ToLower(s))
			}
			member.Equipment[it.PreferredSlot(items.SlotArmor)] = it
			for s, pct := range def.Resistances {
				if pct <= 0 {
					continue
				}
				s = strings.ToLower(s)
				assertResistSource(t, cs, member, s, before[s]+pct)
			}
		})
	}
}

// Every card carrying card_resist_bonus: collecting it must raise the sheet
// total for each listed school and reduce real damage by that same total.
func TestResistSources_CardsShowAndMitigate(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	keys := resistCardKeys(t)
	if len(keys) == 0 {
		t.Fatal("no resist-granting cards found in items.yaml - enumeration or filter broke")
	}
	for _, key := range keys {
		key := key
		t.Run(key, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			fillTestParty(t, cs.game)
			member := cs.game.party.Members[0]
			it, err := items.TryCreateItemFromYAML(key)
			if err != nil {
				t.Fatalf("create %q: %v", key, err)
			}
			def, ok := config.GetItemDefinition(key)
			if !ok {
				t.Fatalf("no definition for %q", key)
			}

			before := map[string]int{}
			for s := range def.CardResistBonus {
				before[strings.ToLower(s)] = cs.game.schoolResistPct(member, strings.ToLower(s))
			}
			if !cs.game.setCardCollectionSlot(0, it) {
				t.Fatalf("card %q did not slot into the collection", key)
			}
			for s, pct := range def.CardResistBonus {
				if pct <= 0 {
					continue
				}
				s = strings.ToLower(s)
				assertResistSource(t, cs, member, s, before[s]+pct)
			}
		})
	}
}

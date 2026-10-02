package game

// Verifies the school-based damage scaling stat: Body/Mind/Spirit (self magic)
// scale with Personality; all other schools scale with Intellect. Guards both
// the formula and the shared spellScalesWithPersonality helper the tooltip uses.

import (
	"fmt"
	"sort"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

func TestOutgoingDamageBonusFiltersByDamageType(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	game.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: 600, OutBonus: 3, OutDamageType: "physical"})
	game.addCombatBuff(TimedCombatBuff{SpellID: "hour_of_power", Frames: 600, OutBonus: 5})

	if got := game.combatBuffOutBonusForDamageType("physical"); got != 8 {
		t.Errorf("physical outgoing bonus = %d, want 8", got)
	}
	if got := game.combatBuffOutBonusForDamageType("fire"); got != 5 {
		t.Errorf("fire outgoing bonus = %d, want 5", got)
	}
}

// buffMasteryValue is the authored mastery ladder: the base value at Novice, the
// *_grandmaster cap at Grandmaster, linear (rounded down) in between. A spell
// without a higher cap stays flat at its base.
func buffMasteryValue(base, grandmaster int, tier character.SkillMastery) int {
	if grandmaster <= base {
		return base
	}
	return base + (grandmaster-base)*int(tier)/int(character.MasteryGrandMaster)
}

// Every party combat buff scales its magnitudes along its authored mastery
// ladder, and its duration by SpellMasteryDurationBonusPct per tier.
func TestPartyBuffMagnitudeScalesWithMastery(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	for _, tc := range []struct {
		spellID spells.SpellID
		class   character.CharacterClass
		school  character.MagicSchoolID
	}{
		{"heroism", character.ClassCleric, character.MagicSchoolSpirit},
		{"stone_skin", character.ClassDruid, character.MagicSchoolEarth},
		{"day_of_the_gods", character.ClassCleric, character.MagicSchoolLight},
		{"hour_of_power", character.ClassCleric, character.MagicSchoolLight},
	} {
		def, err := spells.GetSpellDefinitionByID(tc.spellID)
		if err != nil {
			t.Fatalf("%s def: %v", tc.spellID, err)
		}
		if def.OutgoingDamageBonusGrandmaster <= def.OutgoingDamageBonus &&
			def.IncomingDamageReductionGrandmaster <= def.IncomingDamageReduction &&
			def.ResistBuffPctGrandmaster <= def.ResistBuffPct {
			t.Fatalf("%s has no mastery ladder to exercise", tc.spellID)
		}
		caster := character.CreateCharacter("Caster", tc.class, game.config)
		skill := &character.MagicSkill{Mastery: character.MasteryNovice}
		caster.MagicSchools[tc.school] = skill
		for tier := character.MasteryNovice; tier <= character.MasteryGrandMaster; tier++ {
			t.Run(fmt.Sprintf("%s/tier%d", tc.spellID, tier), func(t *testing.T) {
				skill.Mastery = tier
				if !cs.tryCastPartyBuff(tc.spellID, def, caster) {
					t.Fatalf("%s must cast as a party buff", tc.spellID)
				}
				buff, ok := game.combatBuffByID(string(tc.spellID))
				if !ok {
					t.Fatalf("%s buff not registered", tc.spellID)
				}
				wantOut := buffMasteryValue(def.OutgoingDamageBonus, def.OutgoingDamageBonusGrandmaster, tier)
				wantIn := buffMasteryValue(def.IncomingDamageReduction, def.IncomingDamageReductionGrandmaster, tier)
				wantResist := buffMasteryValue(def.ResistBuffPct, def.ResistBuffPctGrandmaster, tier)
				if buff.OutBonus != wantOut || buff.InReduce != wantIn || buff.ResistPct != wantResist {
					t.Errorf("out/in/resist = %d/%d/%d, want %d/%d/%d",
						buff.OutBonus, buff.InReduce, buff.ResistPct, wantOut, wantIn, wantResist)
				}
				if buff.OutDamageType != def.OutgoingDamageType {
					t.Errorf("OutDamageType = %q, want %q", buff.OutDamageType, def.OutgoingDamageType)
				}
				durationPct := 100 + int(tier)*character.SpellMasteryDurationBonusPct
				if want := def.Duration * durationPct / 100 * game.config.GetTPS(); buff.Frames != want {
					t.Errorf("duration = %d frames, want %d", buff.Frames, want)
				}
			})
		}
	}
}

func TestBlessStatBonusScalesWithMastery(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	cleric := character.CreateCharacter("Cle", character.ClassCleric, game.config)
	spirit := cleric.MagicSchools[character.MagicSchoolSpirit]
	if spirit == nil {
		t.Fatal("cleric should start with the spirit school")
	}
	def, err := spells.GetSpellDefinitionByID("bless")
	if err != nil {
		t.Fatalf("bless def: %v", err)
	}
	if def.StatBonusGrandmaster <= def.StatBonus {
		t.Fatalf("bless has no mastery ladder: %d..%d", def.StatBonus, def.StatBonusGrandmaster)
	}

	for tier := character.MasteryNovice; tier <= character.MasteryGrandMaster; tier++ {
		spirit.Mastery = tier
		want := buffMasteryValue(def.StatBonus, def.StatBonusGrandmaster, tier)
		if got := cs.CalculateSpellStatBonus("bless", cleric); got != want {
			t.Errorf("Bless stat bonus at tier %d = %d, want %d", tier, got, want)
		}
	}
}

func TestSpellDamageScalingStatBySchool(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	caster := game.party.Members[0]

	// Helper: total damage for a spell with a given Intellect/Personality.
	dmg := func(spellID string, intel, pers int) int {
		caster.Intellect = intel
		caster.Personality = pers
		_, _, total := game.combat.CalculateSpellDamage(spells.SpellID(spellID), caster)
		return total
	}

	// Every single-stat projectile formula: Body/Mind/Spirit scale with
	// Personality, every other school with Intellect.
	keys := make([]string, 0, len(config.GlobalSpells.Spells))
	for key := range config.GlobalSpells.Spells {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	checked := map[bool]int{}
	for _, key := range keys {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatalf("%s def: %v", key, err)
		}
		if f := def.DamageFormula(); f.Kind != spells.DamageProjectile || len(f.Terms) != 1 {
			continue
		}
		personality := spells.SchoolScalesWithPersonality(def.School)
		checked[personality]++
		hiPers := dmg(key, 4, 60) // low Int, high Personality
		hiInt := dmg(key, 60, 4)  // high Int, low Personality
		if personality && hiPers <= hiInt {
			t.Errorf("%s (%s) should scale with Personality: hiPers=%d should exceed hiInt=%d", key, def.School, hiPers, hiInt)
		}
		if !personality && hiInt <= hiPers {
			t.Errorf("%s (%s) should scale with Intellect: hiInt=%d should exceed hiPers=%d", key, def.School, hiInt, hiPers)
		}
	}
	if checked[true] == 0 || checked[false] == 0 {
		t.Fatalf("need projectile spells of both scaling stats, got %v", checked)
	}
}

package game

// Verifies the school-based damage scaling stat: Body/Mind/Spirit (self magic)
// scale with Personality; all other schools scale with Intellect. Guards both
// the formula and the shared spellScalesWithPersonality helper the tooltip uses.

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

func TestHeroismDamageBonusScalesWithMastery(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	cleric := character.CreateCharacter("Cle", character.ClassCleric, game.config)
	spirit := cleric.MagicSchools[character.MagicSchoolSpirit]
	if spirit == nil {
		t.Fatal("cleric should start with the spirit school")
	}
	def, err := spells.GetSpellDefinitionByID("heroism")
	if err != nil {
		t.Fatalf("heroism def: %v", err)
	}

	castAt := func(m character.SkillMastery) TimedCombatBuff {
		spirit.Mastery = m
		if !cs.tryCastPartyBuff("heroism", def, cleric) {
			t.Fatalf("heroism must cast as a party buff")
		}
		b, ok := game.combatBuffByID("heroism")
		if !ok {
			t.Fatalf("buff not registered")
		}
		return b
	}

	tests := []struct {
		name        string
		mastery     character.SkillMastery
		durationPct int
		wantBonus   int
	}{
		{name: "novice", mastery: character.MasteryNovice, durationPct: 100, wantBonus: 3},
		{name: "expert", mastery: character.MasteryExpert, durationPct: 120, wantBonus: 5},
		{name: "master", mastery: character.MasteryMaster, durationPct: 140, wantBonus: 7},
		{name: "grandmaster", mastery: character.MasteryGrandMaster, durationPct: 160, wantBonus: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buff := castAt(tt.mastery)
			if buff.OutBonus != tt.wantBonus {
				t.Errorf("Heroism bonus at %s mastery = %d, want %d",
					tt.name, buff.OutBonus, tt.wantBonus)
			}
			if buff.OutDamageType != "physical" {
				t.Errorf("Heroism OutDamageType = %q, want physical", buff.OutDamageType)
			}

			wantFrames := def.Duration * tt.durationPct / 100 * game.config.GetTPS()
			if buff.Frames != wantFrames {
				t.Errorf("duration at %s mastery = %d frames, want %d",
					tt.name, buff.Frames, wantFrames)
			}
		})
	}
}

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
	if got := game.combatBuffOutBonus(); got != 8 {
		t.Errorf("aggregate outgoing bonus = %d, want 8", got)
	}
}

func TestStoneSkinReductionScalesWithMastery(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	druid := character.CreateCharacter("Dru", character.ClassDruid, game.config)
	earth := druid.MagicSchools[character.MagicSchoolEarth]
	if earth == nil {
		t.Fatal("druid should start with the earth school")
	}
	def, err := spells.GetSpellDefinitionByID("stone_skin")
	if err != nil {
		t.Fatalf("stone_skin def: %v", err)
	}

	tests := []struct {
		name    string
		mastery character.SkillMastery
		want    int
	}{
		{name: "novice", mastery: character.MasteryNovice, want: 4},
		{name: "expert", mastery: character.MasteryExpert, want: 6},
		{name: "master", mastery: character.MasteryMaster, want: 8},
		{name: "grandmaster", mastery: character.MasteryGrandMaster, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			earth.Mastery = tt.mastery
			if !cs.tryCastPartyBuff("stone_skin", def, druid) {
				t.Fatal("stone_skin must cast as a party buff")
			}
			buff, ok := game.combatBuffByID("stone_skin")
			if !ok {
				t.Fatal("stone_skin buff not registered")
			}
			if buff.InReduce != tt.want {
				t.Errorf("Stone Skin reduction at %s = %d, want %d", tt.name, buff.InReduce, tt.want)
			}
		})
	}
}

func TestPartyBuffMagnitudeScalesWithMastery(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	caster := character.CreateCharacter("Light", character.ClassCleric, game.config)
	light := &character.MagicSkill{Mastery: character.MasteryNovice}
	caster.MagicSchools[character.MagicSchoolLight] = light

	tests := []struct {
		name       string
		spellID    spells.SpellID
		mastery    character.SkillMastery
		wantOut    int
		wantIn     int
		wantResist int
	}{
		{name: "day novice", spellID: "day_of_the_gods", mastery: character.MasteryNovice, wantResist: 10},
		{name: "day expert", spellID: "day_of_the_gods", mastery: character.MasteryExpert, wantResist: 16},
		{name: "day master", spellID: "day_of_the_gods", mastery: character.MasteryMaster, wantResist: 23},
		{name: "day grandmaster", spellID: "day_of_the_gods", mastery: character.MasteryGrandMaster, wantResist: 30},
		{name: "hour novice", spellID: "hour_of_power", mastery: character.MasteryNovice, wantOut: 5, wantIn: 1},
		{name: "hour expert", spellID: "hour_of_power", mastery: character.MasteryExpert, wantOut: 8, wantIn: 2},
		{name: "hour master", spellID: "hour_of_power", mastery: character.MasteryMaster, wantOut: 11, wantIn: 3},
		{name: "hour grandmaster", spellID: "hour_of_power", mastery: character.MasteryGrandMaster, wantOut: 15, wantIn: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			light.Mastery = tt.mastery
			def, err := spells.GetSpellDefinitionByID(tt.spellID)
			if err != nil {
				t.Fatalf("%s def: %v", tt.spellID, err)
			}
			if !cs.tryCastPartyBuff(tt.spellID, def, caster) {
				t.Fatalf("%s must cast as a party buff", tt.spellID)
			}
			buff, ok := game.combatBuffByID(string(tt.spellID))
			if !ok {
				t.Fatalf("%s buff not registered", tt.spellID)
			}
			if buff.OutBonus != tt.wantOut || buff.InReduce != tt.wantIn || buff.ResistPct != tt.wantResist {
				t.Errorf("%s at %s: out/in/resist = %d/%d/%d, want %d/%d/%d",
					tt.spellID, tt.name, buff.OutBonus, buff.InReduce, buff.ResistPct, tt.wantOut, tt.wantIn, tt.wantResist)
			}
		})
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

	tests := []struct {
		name    string
		mastery character.SkillMastery
		want    int
	}{
		{name: "novice", mastery: character.MasteryNovice, want: 5},
		{name: "expert", mastery: character.MasteryExpert, want: 6},
		{name: "master", mastery: character.MasteryMaster, want: 8},
		{name: "grandmaster", mastery: character.MasteryGrandMaster, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spirit.Mastery = tt.mastery
			if got := cs.CalculateSpellStatBonus("bless", cleric); got != tt.want {
				t.Errorf("Bless stat bonus at %s = %d, want %d", tt.name, got, tt.want)
			}
		})
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

	cases := []struct {
		spell     string
		personIty bool // expected to scale with Personality
	}{
		{"mind_blast", true},  // mind
		{"spirit_lash", true}, // spirit
		{"harm", true},        // body
		{"firebolt", false},   // fire
		{"rock_blast", false}, // earth
	}
	for _, c := range cases {
		hiPers := dmg(c.spell, 4, 60) // low Int, high Personality
		hiInt := dmg(c.spell, 60, 4)  // high Int, low Personality
		if c.personIty {
			if hiPers <= hiInt {
				t.Errorf("%s should scale with Personality: hiPers=%d should exceed hiInt=%d", c.spell, hiPers, hiInt)
			}
		} else {
			if hiInt <= hiPers {
				t.Errorf("%s should scale with Intellect: hiInt=%d should exceed hiPers=%d", c.spell, hiInt, hiPers)
			}
		}
	}

	// The tooltip label must match the formula's scaling stat.
	if got := spellDamageStatLabel("mind", false); got != "Personality" {
		t.Errorf("mind label = %q, want Personality", got)
	}
	if got := spellDamageStatLabel("fire", false); got != "Intellect" {
		t.Errorf("fire label = %q, want Intellect", got)
	}
	// A non-self school flagged scales_with_personality (e.g. ray_of_light) adds
	// a Personality term on top of Intellect — the label must name both.
	if got := spellDamageStatLabel("light", true); got != "Intellect + Personality" {
		t.Errorf("light+personality label = %q, want 'Intellect + Personality'", got)
	}
}

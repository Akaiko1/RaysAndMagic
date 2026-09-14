package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// The damage ladders score mastery by the school the caster CASTS with, not by
// the spell's primary school - the same rule the card shows.
func TestCasterSpellMasteryTierFollowsTheCaster(t *testing.T) {
	loadTestConfig(t)
	def := spells.SpellDefinition{
		ID: "dual_test", Name: "Dual Test", School: "earth", Schools: []string{"earth", "air"},
	}
	air := &character.MMCharacter{MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
		character.MagicSchoolAir: {Mastery: character.MasteryMaster},
	}}
	if got := casterSpellMasteryTier(air, def); got != int(character.MasteryMaster) {
		t.Fatalf("an Air caster scores tier %d for a [earth air] spell, want Master (%d)", got, character.MasteryMaster)
	}
	none := &character.MMCharacter{MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{}}
	if got := casterSpellMasteryTier(none, def); got != 0 {
		t.Fatalf("a caster holding neither school scores tier %d, want 0", got)
	}
}

// Resist pierce takes its BRANCH and its mastery from the same school - the one
// the caster casts with. Branching on the spell's primary while reading the
// caster's skill handed a fire-filed page the self-magic GM bonus.
func TestSpellResistPierceBranchesOnTheCastersSchool(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// A page authored primary=body (self magic) that a FIRE caster holds.
	def := spells.SpellDefinition{
		ID: "dual_pierce_test", Name: "Dual Pierce", School: "body", Schools: []string{"body", "fire"},
	}
	// Injected into the loaded catalog for this test: no shipped spell straddles
	// an elemental and a self-magic school, which is the pair that discriminates.
	config.GlobalSpells.Spells[string(def.ID)] = &config.SpellDefinitionConfig{
		Name: def.Name, School: def.School, Schools: def.Schools,
	}
	t.Cleanup(func() { delete(config.GlobalSpells.Spells, string(def.ID)) })

	fire := &character.MMCharacter{MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
		character.MagicSchoolFire: {Mastery: character.MasteryGrandMaster},
	}}
	// Elemental branch: no Elemental Mastery skill -> no pierce. The self-magic GM
	// bonus must NOT be handed out for a page filed under Fire.
	if got := cs.spellResistPierce(fire, string(def.ID)); got != 0 {
		t.Fatalf("a Fire-filed page granted %d%% self-magic pierce", got)
	}
	// A genuine self-magic holder still gets it.
	body := &character.MMCharacter{MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
		character.MagicSchoolBody: {Mastery: character.MasteryGrandMaster},
	}}
	if got := cs.spellResistPierce(body, string(def.ID)); got != SelfMagicGMResistPiercePct {
		t.Fatalf("a Body GM got %d%% pierce, want %d%%", got, SelfMagicGMResistPiercePct)
	}
}

// `school:` is OPTIONAL authoring: SchoolList falls back to it, so a page written
// with only `schools:` has an empty primary. Every mastery question must survive
// that - they used to test def.School first and silently answer "no school" for a
// GM caster, while the card printed the real tier.
func TestSchoolsOnlySpellStillScoresMastery(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	def := spells.SpellDefinition{
		ID: "schools_only_test", Name: "Schools Only", Schools: []string{"air", "earth"},
		Duration: 100, ResistBuffSchool: "",
	}
	if def.School != "" {
		t.Fatal("fixture: this test is about a spell with NO primary school")
	}
	config.GlobalSpells.Spells[string(def.ID)] = &config.SpellDefinitionConfig{
		Name: def.Name, Schools: def.Schools, Duration: def.Duration,
	}
	t.Cleanup(func() { delete(config.GlobalSpells.Spells, string(def.ID)) })

	air := &character.MMCharacter{MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
		character.MagicSchoolAir: {Mastery: character.MasteryGrandMaster},
	}}

	// Mastery bonus: +5 per tier, from the school the caster actually holds.
	wantBonus := int(character.MasteryGrandMaster) * MasterySpellEffectPerLevel
	if got := cs.spellMasteryBonus(air, def.ID); got != wantBonus {
		t.Errorf("mastery bonus = %d, want %d", got, wantBonus)
	}
	// Duration bonus: the same skill lengthens the buff.
	wantSeconds := def.Duration * (100 + int(character.MasteryGrandMaster)*SpellMasteryDurationBonusPct) / 100
	if got := cs.CalculateSpellDurationSeconds(def.ID, air); got != wantSeconds {
		t.Errorf("duration = %ds, want %ds", got, wantSeconds)
	}
	// GM true-damage split: an elemental holder converts the mastery part.
	if parts := cs.spellDamageParts(def.ID, air, 100); parts.True <= 0 {
		t.Errorf("GM Air caster got no true-damage split: %+v", parts)
	}
	// And a caster holding neither school still gets nothing.
	none := &character.MMCharacter{MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{}}
	if got := cs.spellMasteryBonus(none, def.ID); got != 0 {
		t.Errorf("a caster holding neither school got bonus %d", got)
	}
	if parts := cs.spellDamageParts(def.ID, none, 100); parts.True != 0 {
		t.Errorf("a caster holding neither school got a true split: %+v", parts)
	}
}

package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func gmSkill() *character.Skill { return &character.Skill{Mastery: character.MasteryGrandMaster} }

// TestWeaponMasteryStrike_TrueDamageAndDodge: weapon mastery yields per-tier true
// damage; only a Grandmaster makes the strike ignore Perfect Dodge.
func TestWeaponMasteryStrike_TrueDamageAndDodge(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	weaponDef := lookupWeaponConfigByName(items.CreateWeaponFromYAML("iron_sword").Name) // category: sword
	if weaponDef == nil {
		t.Fatal("iron_sword config missing")
	}
	m := cs.game.party.Members[0]

	delete(m.Skills, character.SkillSword)
	if td, ignore := cs.weaponMasteryStrike(weaponDef); td != 0 || ignore {
		t.Errorf("no skill → (%d,%v), want (0,false)", td, ignore)
	}

	m.Skills[character.SkillSword] = expertSkill() // tier 1
	if td, ignore := cs.weaponMasteryStrike(weaponDef); td != MasteryWeaponTrueDamagePerTier || ignore {
		t.Errorf("Expert → (%d,%v), want (%d,false)", td, ignore, MasteryWeaponTrueDamagePerTier)
	}

	m.Skills[character.SkillSword] = gmSkill() // tier 3
	if td, ignore := cs.weaponMasteryStrike(weaponDef); td != 3*MasteryWeaponTrueDamagePerTier || !ignore {
		t.Errorf("GM → (%d,%v), want (%d,true)", td, ignore, 3*MasteryWeaponTrueDamagePerTier)
	}
}

// TestWeaponCrit_GMBonuses: GM in the weapon's category and GM Arms Master each
// add their crit bonus.
func TestWeaponCrit_GMBonuses(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	wpn := items.CreateWeaponFromYAML("iron_sword")
	m := cs.game.party.Members[0]
	m.Luck = 0 // isolate from luck-based crit
	delete(m.Skills, character.SkillSword)
	delete(m.Skills, character.SkillArmsMaster)

	base := cs.CalculateWeaponCritChance(wpn, m)
	m.Skills[character.SkillSword] = gmSkill()
	withWeapon := cs.CalculateWeaponCritChance(wpn, m)
	if withWeapon-base != WeaponGMCritBonus {
		t.Errorf("GM weapon crit delta = %d, want %d", withWeapon-base, WeaponGMCritBonus)
	}
	m.Skills[character.SkillArmsMaster] = gmSkill()
	withArms := cs.CalculateWeaponCritChance(wpn, m)
	if withArms-withWeapon != ArmsMasterGMCritBonus {
		t.Errorf("GM ArmsMaster crit delta = %d, want %d", withArms-withWeapon, ArmsMasterGMCritBonus)
	}
}

// TestArmorMastery_AddsAC: armor-category mastery adds MasteryArmorACPerLevel AC
// per tier to a piece of that category.
func TestArmorMastery_AddsAC(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	m := cs.game.party.Members[0]
	leather := items.Item{ArmorCategory: "leather", Attributes: map[string]int{"armor_class_base": 5}}

	delete(m.Skills, character.SkillLeather)
	base := cs.CalculateArmorClassContribution(leather, m)

	m.Skills[character.SkillLeather] = expertSkill() // tier 1
	expert := cs.CalculateArmorClassContribution(leather, m)
	if expert-base != MasteryArmorACPerLevel {
		t.Errorf("Expert leather AC delta = %d, want %d", expert-base, MasteryArmorACPerLevel)
	}

	m.Skills[character.SkillLeather] = gmSkill() // tier 3
	gm := cs.CalculateArmorClassContribution(leather, m)
	if gm-base != 3*MasteryArmorACPerLevel {
		t.Errorf("GM leather AC delta = %d, want %d", gm-base, 3*MasteryArmorACPerLevel)
	}
}

// TestSpellResistPierce_GMGated: only a Grandmaster of the spell's school pierces
// resistance.
func TestSpellResistPierce_GMGated(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	m := cs.game.party.Members[0]

	delete(m.MagicSchools, character.MagicSchoolFire)
	if p := cs.spellResistPierce("fireball"); p != 0 {
		t.Fatalf("no Fire school → pierce %d, want 0", p)
	}
	m.MagicSchools[character.MagicSchoolFire] = &character.MagicSkill{Mastery: character.MasteryMaster} // tier 2, not GM
	if p := cs.spellResistPierce("fireball"); p != 0 {
		t.Errorf("Master Fire → pierce %d, want 0 (GM only)", p)
	}
	m.MagicSchools[character.MagicSchoolFire] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
	if p := cs.spellResistPierce("fireball"); p != MagicGMResistPiercePct {
		t.Errorf("GM Fire → pierce %d, want %d", p, MagicGMResistPiercePct)
	}
}

// TestSkillTooltips_CiteGMConstants: every GM capstone's tooltip cites the real
// constant, so the capstone text can't lie or drift from the mechanic.
func TestSkillTooltips_CiteGMConstants(t *testing.T) {
	checks := map[character.SkillType]int{
		character.SkillSword:        WeaponGMCritBonus,
		character.SkillLeather:      ArmorGMDodgeBonus,
		character.SkillBodybuilding: character.BodybuildingGMMaxHPPct,
		character.SkillMeditation:   MeditationGMSpellCostReductionPct,
		character.SkillLearning:     LearningGMPartyXPPct,
		character.SkillArmsMaster:   ArmsMasterGMCritBonus,
	}
	for s, want := range checks {
		if tip := masteryTooltipTextForSkill(s); !strings.Contains(tip, fmt.Sprint(want)) {
			t.Errorf("skill %v GM tooltip %q should cite constant %d", s, tip, want)
		}
	}
}

// TestEffectiveSpellCost_MeditationGM: a GM meditator pays the reduced percent.
func TestEffectiveSpellCost_MeditationGM(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	m := cs.game.party.Members[0]
	delete(m.Skills, character.SkillMeditation)
	if got := cs.effectiveSpellCost(m, 100); got != 100 {
		t.Fatalf("no Meditation → cost %d, want 100", got)
	}
	m.Skills[character.SkillMeditation] = gmSkill()
	if got := cs.effectiveSpellCost(m, 100); got != 100-MeditationGMSpellCostReductionPct {
		t.Errorf("GM Meditation → cost %d, want %d", got, 100-MeditationGMSpellCostReductionPct)
	}
}

// TestArmorGMDodge: each GM-mastered armor type worn adds ArmorGMDodgeBonus dodge.
func TestArmorGMDodge(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	m := cs.game.party.Members[0]
	if got := cs.armorGMDodgeBonus(m); got != 0 {
		t.Fatalf("bare → %d dodge, want 0", got)
	}
	// Equip a plate piece and GM Plate.
	m.Equipment[items.SlotArmor] = items.Item{ArmorCategory: "plate"}
	m.Skills[character.SkillPlate] = gmSkill()
	if got := cs.armorGMDodgeBonus(m); got != ArmorGMDodgeBonus {
		t.Errorf("GM plate worn → %d, want %d", got, ArmorGMDodgeBonus)
	}
	// Add a GM shield in the off-hand → second mastered type stacks.
	m.Equipment[items.SlotOffHand] = items.Item{ArmorCategory: "shield"}
	m.Skills[character.SkillShield] = gmSkill()
	if got := cs.armorGMDodgeBonus(m); got != 2*ArmorGMDodgeBonus {
		t.Errorf("GM plate + shield → %d, want %d", got, 2*ArmorGMDodgeBonus)
	}
}

// TestBodybuildingGM_MaxHPPercent: at GM, Bodybuilding adds the flat per-tier HP
// plus a percent of base Max HP.
func TestBodybuildingGM_MaxHPPercent(t *testing.T) {
	cfg := loadTestConfig(t)
	plain := character.CreateCharacter("Plain", character.ClassKnight, cfg)
	gm := character.CreateCharacter("Buff", character.ClassKnight, cfg)
	gm.Skills[character.SkillBodybuilding] = gmSkill()
	plain.CalculateDerivedStats(cfg)
	gm.CalculateDerivedStats(cfg)

	base := plain.MaxHitPoints
	want := 3*character.BodybuildingHPPerTier + base*character.BodybuildingGMMaxHPPct/100
	if got := gm.MaxHitPoints - base; got != want {
		t.Errorf("GM Bodybuilding HP delta = %d, want %d", got, want)
	}
}

// TestLearningGM_PartyShare: a GM-of-Learning teacher grants the party-wide XP %
// to every living member, on top of personal per-tier bonus.
func TestLearningGM_PartyShare(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.combat = NewCombatSystem(g)

	teacher := g.party.Members[0]
	student := g.party.Members[1]
	for _, m := range []*character.MMCharacter{teacher, student} {
		m.Level = 50
		m.Experience = 0
		delete(m.Skills, character.SkillLearning)
	}
	teacher.Skills[character.SkillLearning] = gmSkill() // tier 3 → +30% self, +5% party
	g.party.Members = g.party.Members[:2]
	g.party.Reserve = nil
	g.party.Captive = nil

	g.grantSharedXP(100)
	// Student has no Learning, but gets the teacher's party share.
	if want := 100 + 100*LearningGMPartyXPPct/100; student.Experience != want {
		t.Errorf("student XP = %d, want %d (teacher party share)", student.Experience, want)
	}
	// Teacher: own +30% plus +5% party.
	if want := 100 + 100*(3*LearningXPPctPerTier+LearningGMPartyXPPct)/100; teacher.Experience != want {
		t.Errorf("teacher XP = %d, want %d", teacher.Experience, want)
	}
}

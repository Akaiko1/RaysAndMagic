package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// TestSkillTooltips_NoOmission: every skill (weapon/armor/misc) describes its
// effect - none silently returns an empty tooltip.
func TestSkillTooltips_NoOmission(t *testing.T) {
	for s := character.SkillSword; s <= character.SkillArmsMaster; s++ {
		if strings.TrimSpace(masteryTooltipTextForSkill(s)) == "" {
			t.Errorf("skill %v (%d) has no tooltip text", s, int(s))
		}
	}
}

// TestSkillTooltips_UseRealConstants: numeric skill tooltips cite the same
// constants the mechanics use, so tooltip and combat can't drift.
func TestSkillTooltips_UseRealConstants(t *testing.T) {
	checks := map[character.SkillType]int{
		character.SkillSword:        MasteryWeaponTrueDamagePerTier,
		character.SkillLeather:      MasteryArmorACPerLevel,
		character.SkillBodybuilding: character.BodybuildingHPPerTier,
		character.SkillMeditation:   character.MeditationRegenPerTier,
		character.SkillLearning:     LearningXPPctPerTier,
		character.SkillArmsMaster:   ArmsMasterDamagePerTier,
		character.SkillMerchant:     MerchantPricePctPerTier,
		character.SkillDisarmTrap:   DisarmTrapDamageReductionPerTier,
	}
	for s, want := range checks {
		if tip := masteryTooltipTextForSkill(s); !strings.Contains(tip, fmt.Sprint(want)) {
			t.Errorf("skill %v tooltip %q should cite constant %d", s, tip, want)
		}
	}
}

// TestSpeedTooltip_NoTurnBasedLie: Speed grants party-wide turn-based bonus
// action slots, so its tooltip must not claim it has no turn-based effect.
func TestSpeedTooltip_NoTurnBasedLie(t *testing.T) {
	tip := statTooltipText("speed")
	if strings.Contains(strings.ToLower(tip), "no effect in turn-based") {
		t.Errorf("speed tooltip still lies about turn-based: %q", tip)
	}
	for _, want := range []int{character.SpeedBonusAction1Threshold, character.SpeedBonusAction2Threshold} {
		if !strings.Contains(tip, fmt.Sprint(want)) {
			t.Errorf("speed tooltip should cite bonus-action threshold %d: %q", want, tip)
		}
	}
}

func expertSkill() *character.Skill { return &character.Skill{Mastery: character.MasteryExpert} } // tier 1

// TestBodybuilding_AddsMaxHP: a Bodybuilding tier raises Max HP.
func TestBodybuilding_AddsMaxHP(t *testing.T) {
	cfg := loadTestConfig(t)
	base := character.CreateCharacter("Plain", character.ClassKnight, cfg)
	buff := character.CreateCharacter("Buff", character.ClassKnight, cfg)
	buff.Skills[character.SkillBodybuilding] = expertSkill()
	base.CalculateDerivedStats(cfg)
	buff.CalculateDerivedStats(cfg)
	if got := buff.MaxHitPoints - base.MaxHitPoints; got != character.BodybuildingHPPerTier {
		t.Errorf("Bodybuilding HP delta = %d, want %d", got, character.BodybuildingHPPerTier)
	}
}

// TestMeditation_SpeedsManaRegen: Meditation raises SP regenerated per tick.
func TestMeditation_SpeedsManaRegen(t *testing.T) {
	cfg := loadTestConfig(t)
	c := character.CreateCharacter("Monk", character.ClassCleric, cfg)
	delete(c.Skills, character.SkillMeditation)
	without := c.CalculateManaRegenAmount()
	c.Skills[character.SkillMeditation] = expertSkill()
	with := c.CalculateManaRegenAmount()
	if with-without != character.MeditationRegenPerTier {
		t.Errorf("Meditation regen delta = %d, want %d", with-without, character.MeditationRegenPerTier)
	}
}

// TestArmsMaster_AddsWeaponDamage: ArmsMaster adds flat damage with any weapon.
func TestArmsMaster_AddsWeaponDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	wpn := items.CreateWeaponFromYAML("iron_sword")
	plain := character.CreateCharacter("A", character.ClassKnight, cs.game.config)
	armed := character.CreateCharacter("B", character.ClassKnight, cs.game.config)
	armed.Skills[character.SkillArmsMaster] = expertSkill()
	_, _, base := cs.CalculateWeaponDamage(wpn, plain)
	_, _, boosted := cs.CalculateWeaponDamage(wpn, armed)
	if boosted-base != ArmsMasterDamagePerTier {
		t.Errorf("ArmsMaster damage delta = %d, want %d", boosted-base, ArmsMasterDamagePerTier)
	}
}

// TestDisarmTrap_ReducesIncomingDamage: the placeholder shaves incoming damage.
func TestDisarmTrap_ReducesIncomingDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	plain := character.CreateCharacter("A", character.ClassArcher, cs.game.config)
	skilled := character.CreateCharacter("B", character.ClassArcher, cs.game.config)
	skilled.Skills[character.SkillDisarmTrap] = expertSkill()
	const raw = 50 // large enough to avoid the floor-at-1
	base := cs.mitigateCharacterDamage(raw, "physical", plain, false)
	reduced := cs.mitigateCharacterDamage(raw, "physical", skilled, false)
	if base-reduced != DisarmTrapDamageReductionPerTier {
		t.Errorf("DisarmTrap reduction delta = %d, want %d", base-reduced, DisarmTrapDamageReductionPerTier)
	}
}

// TestMerchant_AdjustsPrices: the party's Merchant tier discounts buys, boosts sells.
func TestMerchant_AdjustsPrices(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	if g.merchantBuyPrice(100) != 100 || g.merchantSellPrice(100) != 100 {
		t.Fatalf("no Merchant skill should leave prices unchanged")
	}
	g.party.Members[0].Skills[character.SkillMerchant] = expertSkill() // tier 1 -> 5%
	if got := g.merchantBuyPrice(100); got != 100-MerchantPricePctPerTier {
		t.Errorf("buy price = %d, want %d", got, 100-MerchantPricePctPerTier)
	}
	if got := g.merchantSellPrice(100); got != 100+MerchantPricePctPerTier {
		t.Errorf("sell price = %d, want %d", got, 100+MerchantPricePctPerTier)
	}
}

// TestLearning_BoostsXP: Learning grants bonus % XP to that character.
func TestLearning_BoostsXP(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.combat = NewCombatSystem(g)
	m := g.party.Members[0]
	m.Level = 50 // high enough that the grant won't trigger a level-up
	m.Experience = 0
	m.Skills[character.SkillLearning] = expertSkill() // tier 1 -> +10%
	// Isolate this member: drop the rest so grantSharedXP only touches them.
	g.party.Members = g.party.Members[:1]
	g.party.Reserve = nil
	g.party.Captive = nil
	g.grantSharedXP(100)
	if m.Experience != 100+100*LearningXPPctPerTier/100 {
		t.Errorf("Learning XP = %d, want %d", m.Experience, 100+100*LearningXPPctPerTier/100)
	}
}

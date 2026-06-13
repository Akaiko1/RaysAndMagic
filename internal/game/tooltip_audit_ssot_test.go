package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// These cover the second SSoT audit round: arc/mass-hit, RT-vs-TB cooldown,
// disintegrate immunity, monster-only cards, dual stat scaling, splash-crit and
// dodge rules, projectile hitbox, the active-buff bump and the Meditation
// discount — every line that combat (or a YAML field) actually drives.

func TestTooltip_WeaponArcCooldownStun(t *testing.T) {
	g, thief := newThiefTestGame(t)
	mace, err := items.TryCreateWeaponFromYAML("steel_mace")
	if err != nil {
		t.Fatalf("steel_mace: %v", err)
	}
	full := GetItemTooltip(mace, thief, g.combat, true)

	// Swing arc is a core melee differentiator → visible without Shift.
	compact := GetItemTooltip(mace, thief, g.combat, false)
	if !strings.Contains(compact, "Swing Arc: 90°") {
		t.Errorf("mace must show its 90° swing arc:\n%s", compact)
	}
	// Cooldown distinguishes real-time seconds from the turn-based action.
	for _, want := range []string{"RT Cooldown:", "TB: 1 action"} {
		if !strings.Contains(compact, want) {
			t.Errorf("cooldown must label RT/TB (%q):\n%s", want, compact)
		}
	}
	// Stun spells out RT seconds AND TB turns (was an ambiguous "(3 turns)").
	if !strings.Contains(full, "(3s RT / 3 turns TB)") {
		t.Errorf("mace stun must read RT seconds / TB turns:\n%s", full)
	}
}

func TestTooltip_DisintegrateImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	def, err := spells.GetSpellDefinitionByID("disintegrate")
	if err != nil {
		t.Fatalf("disintegrate: %v", err)
	}
	spellTip := buildSpellTooltipUnified(def, char, cs, true)
	if !strings.Contains(spellTip, "undead and dragons immune") {
		t.Errorf("disintegrate spell must note universal immunity:\n%s", spellTip)
	}

	blaster, err := items.TryCreateWeaponFromYAML("alien_blaster")
	if err != nil {
		t.Skip("alien_blaster not defined")
	}
	weaponTip := GetItemTooltip(blaster, char, cs, true)
	if !strings.Contains(weaponTip, "undead and dragons immune") {
		t.Errorf("alien blaster disintegrate must note universal immunity:\n%s", weaponTip)
	}
}

func TestEditorCard_MonsterOnlySpellHidesPlayerFormula(t *testing.T) {
	newTestCombatSystemWithConfig(t) // loads spell config
	def, ok := config.GetSpellDefinition("alien_dark_bolt")
	if !ok || def == nil {
		t.Skip("alien_dark_bolt not defined")
	}
	if !def.MonsterOnly {
		t.Fatalf("alien_dark_bolt should be monster_only")
	}
	sd, err := spells.GetSpellDefinitionByID("alien_dark_bolt")
	if err != nil {
		t.Fatalf("alien_dark_bolt sd: %v", err)
	}
	joined := strings.Join(character.RenderCardLines(character.MonsterSpellCardSections(def, sd), true), "\n")

	if !strings.Contains(joined, "Cast by monsters only") ||
		!strings.Contains(joined, "casting monster's attack damage") {
		t.Errorf("monster card must describe monster mechanics:\n%s", joined)
	}
	// It must NOT borrow the player formula (SP cost, stat scaling, mastery).
	for _, leak := range []string{"Cost:", "Intellect /", "Mastery:", "Base ("} {
		if strings.Contains(joined, leak) {
			t.Errorf("monster card leaks player formula %q:\n%s", leak, joined)
		}
	}
}

func TestEditorCard_RayOfLightDualScaling(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	def, ok := config.GetSpellDefinition("ray_of_light")
	if !ok || def == nil {
		t.Skip("ray_of_light not defined")
	}
	sd, err := spells.GetSpellDefinitionByID("ray_of_light")
	if err != nil {
		t.Fatalf("ray_of_light sd: %v", err)
	}
	joined := strings.Join(character.RenderCardLines(character.SpellCardSections("ray_of_light", def, sd, character.ArmorPhysicalReductionDivisor), true), "\n")
	// Ray of Light scales with BOTH stats (school Intellect + the personality flag).
	if !strings.Contains(joined, "Intellect / 3") || !strings.Contains(joined, "Personality / 3") {
		t.Errorf("Ray of Light editor card must scale with BOTH Intellect and Personality:\n%s", joined)
	}
}

func TestTooltip_AoESplashCritAndDodgeRules(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	def, err := spells.GetSpellDefinitionByID("fireball")
	if err != nil {
		t.Fatalf("fireball: %v", err)
	}
	full := buildSpellTooltipUnified(def, char, cs, true)
	if !strings.Contains(full, character.SplashCritRule) {
		t.Errorf("AoE spell must explain splash inherits the crit:\n%s", full)
	}
	if !strings.Contains(full, "Perfect Dodge") {
		t.Errorf("projectile spell must mention Perfect Dodge:\n%s", full)
	}
	if !strings.Contains(full, "Hitbox:") {
		t.Errorf("projectile spell should show its hitbox size:\n%s", full)
	}
}

func TestTooltip_ActiveBuffRaisesTotalDamage(t *testing.T) {
	g, thief := newThiefTestGame(t)
	w := thief.Equipment[items.SlotMainHand]
	before := GetItemTooltip(w, thief, g.combat, true)

	// Heroism (+10 outgoing) is the OutBonus combat adds to every hit.
	g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", OutBonus: 10, Frames: 600})
	after := GetItemTooltip(w, thief, g.combat, true)

	if !strings.Contains(after, "Active party buff: +10") {
		t.Errorf("buffed weapon tooltip must surface the active buff:\n%s", after)
	}
	if before == after {
		t.Errorf("an active outgoing-damage buff must change Total Damage:\n%s", after)
	}
}

func TestTooltip_MeditationCostDiscount(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := character.CreateCharacter("Med", character.ClassSorcerer, cs.game.config)
	caster.Skills[character.SkillMeditation] = &character.Skill{Mastery: character.MasteryGrandMaster}
	cs.game.party.Members[0] = caster

	def, err := spells.GetSpellDefinitionByID("fireball")
	if err != nil {
		t.Fatalf("fireball: %v", err)
	}
	full := buildSpellTooltipUnified(def, caster, cs, true)
	for _, want := range []string{"Base Cost:", "GM Meditation: -25%"} {
		if !strings.Contains(full, want) {
			t.Errorf("GM meditator spell must break down the cost (%q):\n%s", want, full)
		}
	}
}

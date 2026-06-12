package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// The weapon tooltip's cooldown line must quote the SAME frames the RT loop
// charges (WeaponCooldownFramesFor) — including weapon-type multipliers.
func TestTooltip_WeaponCooldownMatchesCombat(t *testing.T) {
	g, thief := newThiefTestGame(t)
	weapon := thief.Equipment[items.SlotMainHand] // magic dagger
	wantFrames := g.combat.WeaponCooldownFramesFor(thief, weapon.Name)
	tip := GetItemTooltip(weapon, thief, g.combat)
	if !strings.Contains(tip, "Cooldown: "+cooldownSeconds(g.combat, wantFrames)) {
		t.Errorf("weapon tooltip must quote combat cooldown %d frames; got:\n%s", wantFrames, tip)
	}
}

// The spell tooltip's cooldown line must quote SpellCooldownFrames (per-spell
// cooldown_seconds × speed factor × staff modifier), not a generic curve.
func TestTooltip_SpellCooldownMatchesCombat(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	g.combat = NewCombatSystem(g)
	caster := character.CreateCharacter("Lys", character.ClassSorcerer, cfg)
	g.party.Members[0] = caster

	wantFrames := g.combat.SpellCooldownFrames(caster, "firebolt")
	tip := GetSpellTooltip("firebolt", caster, g.combat)
	if !strings.Contains(tip, "Cooldown: "+cooldownSeconds(g.combat, wantFrames)) {
		t.Errorf("spell tooltip must quote combat cooldown %d frames; got:\n%s", wantFrames, tip)
	}
}

// Archmage Staff carries spell_cooldown_multiplier 0.8 → its EffectLines must
// surface "Spell cooldown -20%" (combat applies it in SpellCooldownFrames).
func TestWeaponEffectLines_CooldownModifiers(t *testing.T) {
	loadTestConfig(t)
	def, _, ok := config.GetWeaponDefinitionByName("Archmage Staff")
	if !ok || def == nil {
		t.Skip("Archmage Staff not defined")
	}
	joined := strings.Join(def.EffectLines(), "\n")
	if !strings.Contains(joined, "Spell cooldown -20%") {
		t.Errorf("Archmage Staff EffectLines must state the -20%% spell cooldown; got:\n%s", joined)
	}
}

// Golden Thief Bug Carapace's defining power is its resistances — the shared
// item formatter must list them.
func TestItemEffectLines_CarapaceResistances(t *testing.T) {
	loadTestConfig(t)
	def, ok := config.GlobalItems.Items["golden_thiefbug_carapace"]
	if !ok || def == nil {
		t.Skip("carapace not defined")
	}
	joined := strings.Join(def.EffectLines(), "\n")
	if !strings.Contains(strings.ToLower(joined), "resist") {
		t.Errorf("carapace EffectLines must list resistances; got:\n%s", joined)
	}
}

// Stat descriptions must quote the REAL balance constants — hand-typed
// divisors drifted twice (Intellect/3 and Personality/4 were both invented).
func TestStatDescriptions_QuoteRealConstants(t *testing.T) {
	intDesc := character.StatDescription("intellect")
	if !strings.Contains(intDesc, fmt.Sprintf("Intellect/%d", spells.SpellIntellectDivisor)) {
		t.Errorf("intellect description must quote spell divisor %d: %s", spells.SpellIntellectDivisor, intDesc)
	}
	if !strings.Contains(intDesc, fmt.Sprintf("/%d", character.TrapStatScalingDivisor)) {
		t.Errorf("intellect description must quote trap divisor %d: %s", character.TrapStatScalingDivisor, intDesc)
	}
	persDesc := character.StatDescription("personality")
	if !strings.Contains(persDesc, fmt.Sprintf("Personality/%d", spells.HealingPersonalityDivisor)) {
		t.Errorf("personality description must quote healing divisor %d: %s", spells.HealingPersonalityDivisor, persDesc)
	}
}

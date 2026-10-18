package game

import (
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// spellControlDuration is called once per eligible victim, after target gates.
// A zero duration is a paid resisted attempt, not a refundable lack of targets.
func (cs *CombatSystem) spellControlDuration(def spells.SpellDefinition, caster *character.MMCharacter, m *monster.Monster3D, turns, frames int) (int, int) {
	rule := cs.game.config.StatusEffects.ControlSaves
	if def.Mechanic.Kind == "feeblemind" && (m.IsSilenced() || m.SilenceImmunity.Active()) {
		cs.game.logCombat(logToneNone, rule.ResistedMessage, logMonsterName(m), logSchoolWord(def.School, def.Name))
		return 0, 0
	}
	if !rule.Applies(def.Mechanic.Kind) {
		return turns, frames
	}
	school, _ := monster.ParseDamageType(string(caster.SpellSchoolFor(def))) // Spell schools are validated on load.
	chance := rule.Chance(m.Level-caster.Level, int(character.MasteryGrandMaster)-casterSpellMasteryTier(caster, def), m.Resistances[school])
	roll := cs.controlSaveRoll
	if roll == nil {
		roll = rand.Float64
	}
	if roll()*100 >= chance {
		if !rule.Diminishes(def.Mechanic.Kind) {
			return turns, frames
		}
		turns, frames = m.StunDR().Step(turns, cs.game.config.GetTPS())
		if turns > 0 {
			return turns, frames
		}
	}
	cs.game.logCombat(logToneNone, rule.ResistedMessage, logMonsterName(m), logSchoolWord(def.School, def.Name))
	return 0, 0
}

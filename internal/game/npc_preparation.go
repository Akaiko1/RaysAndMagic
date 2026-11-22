package game

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// teachPreparationSpell sells one lesson; its lines are the spell trader's.
func (ih *InputHandler) teachPreparationSpell(c *character.NPCDialogueChoice) {
	g := ih.game
	if c.Cost <= 0 || g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) {
		return
	}
	hero := g.party.Members[g.selectedChar]
	if hero == nil {
		return
	}
	name := spellName(spells.SpellID(c.Spell))
	if !canCharacterLearnNPCSpell(hero, c.Spell) {
		g.AddCombatMessage(uitext.Text("dialog.cannot_learn_matching_magic_school_is_not", hero.Name, name))
		return
	}
	if !g.canAfford(c.Cost) {
		g.AddCombatMessage(uitext.Text("dialog.service_cannot_afford", c.Cost))
		return
	}
	if !hero.LearnSpell(spells.SpellID(c.Spell)) {
		g.AddCombatMessage(uitext.Text("dialog.cannot_be_taught_right_now", name))
		return
	}
	g.spendGold(c.Cost)
	g.AddCombatMessage(uitext.Text("dialog.learned", hero.Name, name))
}

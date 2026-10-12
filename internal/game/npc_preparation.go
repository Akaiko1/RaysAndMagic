package game

import (
	"fmt"
	"ugataima/internal/character"
)

func (ih *InputHandler) teachPreparationSpell(c *character.NPCDialogueChoice) {
	g := ih.game
	if c.Cost <= 0 || g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) {
		return
	}
	hero := g.party.Members[g.selectedChar]
	if hero == nil || !canCharacterLearnNPCSpell(hero, c.Spell) {
		g.AddCombatMessage("The selected hero needs the matching magic school.")
		return
	}
	if g.party.Gold < c.Cost {
		g.AddCombatMessage(fmt.Sprintf("The lesson costs %d gold.", c.Cost))
		return
	}
	if !ih.addSpellToCharacter(hero, c.Spell) {
		g.AddCombatMessage("That spell is already known or unavailable.")
		return
	}
	g.party.Gold -= c.Cost
	g.AddCombatMessage(hero.Name + " learns " + c.Spell + ".")
}

package game

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
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
func (g *MMGame) exchangePreparation(c *character.NPCDialogueChoice) {
	e := c.Exchange
	if e == nil || len(e.Costs) == 0 || e.Gold < 0 || (e.Output == "") == (e.Gold == 0) {
		return
	}
	var out items.Item
	if e.Output != "" {
		var err error
		out, err = items.TryCreateItemFromYAML(e.Output)
		if err != nil {
			return
		}
	} else if g.party.Gold > int(^uint(0)>>1)-e.Gold {
		return
	}
	for key, n := range e.Costs {
		d, ok := config.GetItemDefinition(key)
		if !ok || n < 1 {
			return
		}
		if g.party.CountItemsByName(d.Name) < n {
			g.AddCombatMessage(fmt.Sprintf("The exchange needs %d %s.", n, d.Name))
			return
		}
	}
	for key, n := range e.Costs {
		d, _ := config.GetItemDefinition(key)
		g.party.RemoveItemsByName(d.Name, n)
	}
	if e.Gold > 0 {
		g.party.Gold += e.Gold
		g.AddCombatMessage(fmt.Sprintf("Received %d gold.", e.Gold))
	} else {
		g.party.AddItem(out)
		g.AddCombatMessage("Received " + out.Name + ".")
	}
}

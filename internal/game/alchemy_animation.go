package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Cosmetic feedback for an already committed atomic batch. Never saved and
// never owns an inventory transaction, even when dismissed or interrupted.
type alchemyBrewAnimation struct {
	Caster                  *character.MMCharacter
	Output                  items.Item
	Count, Frames, Duration int
	Color                   [3]int
	Ingredients             []items.Item
}

func (g *MMGame) startAlchemyBrewAnimation(recipe *config.AlchemyRecipe, count int, ingredients []items.Item) {
	out, err := items.TryCreateItemFromYAML(recipe.Output)
	if err != nil {
		return
	}
	d, _ := config.GetItemDefinition(recipe.Output)
	g.brewAnimation = &alchemyBrewAnimation{
		Caster: g.party.Members[g.selectedChar], Output: out, Count: count,
		Duration: 3 * g.config.GetTPS(), Color: d.BrewColor, Ingredients: ingredients,
	}
}
func (g *MMGame) tickAlchemyBrewAnimation() {
	fx := g.brewAnimation
	if fx == nil {
		return
	}
	if !g.menuOpen || g.currentTab != TabSpellbook || g.party == nil || g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) || g.party.Members[g.selectedChar] != fx.Caster {
		g.brewAnimation = nil
		return
	}
	fx.Frames++
	if fx.Frames >= fx.Duration {
		g.brewAnimation = nil
	}
}

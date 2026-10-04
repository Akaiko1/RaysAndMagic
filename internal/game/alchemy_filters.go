package game

import (
	"slices"
	"ugataima/internal/config"
)

var alchemyRecipeFilters = []string{"All", "Recovery", "Damage", "Defense", "Revival"}
var alchemyElementFilters = []string{"All", "Fire", "Water", "Earth", "Air", "Body", "Mind", "Light", "Dark"}

func alchemyRecipeCategory(d *config.ItemDefinitionConfig) string {
	if d == nil {
		return ""
	}
	switch {
	case d.Revive:
		return "Revival"
	case d.Flask != nil || d.DamageBuffPct > 0:
		return "Damage"
	case d.ResistBuffSchoolPct > 0 || d.BuffArmorClass > 0 || d.BuffDodgePct > 0:
		return "Defense"
	default:
		return "Recovery"
	}
}

func alchemyRecipeElement(d *config.ItemDefinitionConfig) string {
	if d.Flask != nil {
		return config.TitleWords(d.Flask.Element)
	}
	if d.DamageBuffSchool != "" {
		return config.TitleWords(d.DamageBuffSchool)
	}
	return config.TitleWords(d.ResistBuffSchool)
}

func (g *MMGame) visibleAlchemyRecipes() []int {
	var visible []int
	if config.GlobalAlchemy == nil {
		return visible
	}
	for i := range config.GlobalAlchemy.Recipes {
		r := &config.GlobalAlchemy.Recipes[i]
		d, ok := config.GetItemDefinition(r.Output)
		if !ok || d == nil {
			continue
		}
		if f := g.alchemyRecipeFilter; f != "" && f != "All" && f != alchemyRecipeCategory(d) {
			continue
		}
		if f := g.alchemyElementFilter; f != "" && f != "All" && f != alchemyRecipeElement(d) {
			continue
		}
		if g.alchemyBrewableOnly && (g.party == nil || g.party.MaxAlchemySelected(r, g.alchemySelection(r)) < 1) {
			continue
		}
		visible = append(visible, i)
	}
	return visible
}

// selectedRare always remains a catalog index, including during filtered
// navigation. Never reinterpret it as a filtered-list index when brewing.
func (g *MMGame) selectVisibleAlchemyRecipe(visible []int, delta int) int {
	if len(visible) == 0 {
		g.selectedRare = -1
		return -1
	}
	pos := slices.Index(visible, g.selectedRare)
	if pos < 0 {
		pos = 0
	}
	pos = (pos + delta%len(visible) + len(visible)) % len(visible)
	g.selectedRare = visible[pos]
	return pos
}

func nextAlchemyFilter(value string, choices []string) string {
	return choices[(max(0, slices.Index(choices, value))+1)%len(choices)]
}

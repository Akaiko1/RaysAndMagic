package game

import (
	"fmt"
	"maps"
	"slices"
	"ugataima/internal/character"
	"ugataima/internal/config"
)

// AlchemyState stores campaign choices independently of a temporary book view.
type AlchemyState struct {
	Serial      uint64                                `json:"serial,omitempty"`
	Choices     map[string][]int                      `json:"choices,omitempty"`
	Selections  map[string]character.AlchemySelection `json:"selections,omitempty"`
	Populations map[string]HarvestPopulationState     `json:"populations,omitempty"`
}
type HarvestPopulationState struct {
	Day   int           `json:"day"`
	Nodes []HarvestNode `json:"nodes"`
}
type HarvestNode struct {
	Relocate bool   `json:"relocate,omitempty"`
	Region   string `json:"region"`
	Quantity int    `json:"quantity,omitempty"`
	Key      string `json:"key"`
	X, Y     int
	ID       string `json:"id"`
}

func (s AlchemyState) Clone() AlchemyState {
	s.Selections = maps.Clone(s.Selections)
	for k, v := range s.Selections {
		s.Selections[k] = v.Clone()
	}
	s.Choices = maps.Clone(s.Choices)
	for k, v := range s.Choices {
		s.Choices[k] = slices.Clone(v)
	}
	s.Populations = maps.Clone(s.Populations)
	for k, v := range s.Populations {
		v.Nodes = slices.Clone(v.Nodes)
		s.Populations[k] = v
	}
	return s
}

func (g *MMGame) alchemySelection(r *config.AlchemyRecipe) character.AlchemySelection {
	if g.alchemy.Selections == nil {
		g.alchemy.Selections = map[string]character.AlchemySelection{}
	}
	selected, exists := g.alchemy.Selections[r.Key]
	if !exists || len(selected) != len(r.Ingredients) {
		selected = character.AlchemySourceSelection(r, g.alchemyChoices(r))
		g.alchemy.Selections[r.Key] = selected
	}
	for i, group := range r.Ingredients {
		allowed := group.Materials()
		if selected[i] == nil {
			selected[i] = map[string]bool{}
		}
		for key := range selected[i] {
			if _, ok := allowed[key]; !ok {
				delete(selected[i], key)
			}
		}
		// A single fixed ingredient is the required base, not an optional source.
		if len(allowed) == 1 {
			for key := range allowed {
				selected[i][key] = true
			}
		}
	}
	return selected
}
func (g *MMGame) alchemyChoices(r *config.AlchemyRecipe) []int {
	if g.alchemy.Choices == nil {
		g.alchemy.Choices = map[string][]int{}
	}
	if len(g.alchemy.Choices[r.Key]) != len(r.Ingredients) {
		g.alchemy.Choices[r.Key] = make([]int, len(r.Ingredients))
	}
	choices := g.alchemy.Choices[r.Key]
	// Saved source indices must follow the current recipe catalog. Normalize
	// once here so the displayed source, batch preview and commit always agree.
	for i, group := range r.Ingredients {
		choices[i] = max(0, min(len(group.Alternatives)-1, choices[i]))
	}
	return choices
}
func (g *MMGame) brewSelectedRecipe() bool {
	if g.brewAnimation != nil {
		return false
	}
	if config.GlobalAlchemy == nil || g.selectedRare < 0 || g.selectedRare >= len(config.GlobalAlchemy.Recipes) {
		return false
	}
	if !slices.Contains(g.visibleAlchemyRecipes(), g.selectedRare) {
		return false
	}
	if reason, safe := g.safeToPrepare(brewActivity()); !safe {
		g.AddCombatMessage(reason)
		g.rareBookMessage = reason
		return false
	}
	r := &config.GlobalAlchemy.Recipes[g.selectedRare]
	batches := max(1, min(99, g.alchemyBatches))
	n, ingredients, err := g.party.BrewSelected(g.party.Members[g.selectedChar], r, g.alchemySelection(r), batches)
	if err != nil {
		g.rareBookMessage = err.Error()
		return false
	}
	d, _ := config.GetItemDefinition(r.Output)
	g.rareBookMessage = fmt.Sprintf("Prepared %d x %s.", n, d.Name)
	g.AddCombatMessage(g.rareBookMessage)
	g.startAlchemyBrewAnimation(r, n, ingredients)
	return true
}

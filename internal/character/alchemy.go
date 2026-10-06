package character

import (
	"fmt"
	"maps"
	"sort"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// ValidateAlchemyTradeMaterials reserves item-backed merchant currencies for
// trading. Read both shop defaults and stock overrides from the same catalog
// and currency contract as purchases, including currencies added in the future.
func ValidateAlchemyTradeMaterials(recipes *config.AlchemyConfig, npcs *NPCConfig) error {
	if recipes == nil || npcs == nil {
		return fmt.Errorf("alchemy and NPC catalogs must be loaded")
	}
	reserved := ItemCurrencyMerchants(npcs)
	for _, recipe := range recipes.Recipes {
		for _, group := range recipe.Ingredients {
			for _, source := range group.Alternatives {
				for _, key := range source.Items {
					if npcKey, ok := reserved[key]; ok {
						return fmt.Errorf("recipe %q: ingredient %q is reserved for merchant %q", recipe.Key, key, npcKey)
					}
				}
			}
		}
	}
	return nil
}

// ItemCurrencyMerchants identifies items spent by shops, including per-stock
// overrides. Recipe validation and roster warnings share this currency rule.
func ItemCurrencyMerchants(npcs *NPCConfig) map[string]string {
	reserved := map[string]string{}
	if npcs == nil {
		return reserved
	}
	keys := make([]string, 0, len(npcs.NPCs))
	for key := range npcs.NPCs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, npcKey := range keys {
		npc := npcs.NPCs[npcKey]
		if npc == nil {
			continue
		}
		if key, ok := CurrencyItemKey(npc.Currency); ok {
			reserved[key] = npcKey
		}
		for _, entry := range npc.Inventory {
			if entry == nil {
				continue
			}
			stock := MerchantStockItem{CurrencyItem: entry.CurrencyItem}
			if key, ok := CurrencyItemKey(stock.EffectiveCurrency(npc.Currency)); ok {
				reserved[key] = npcKey
			}
		}
	}
	return reserved
}

// AlchemySelection stores allowed item keys per required ingredient group.
// An empty group is an explicit empty selection, never permission to use all stock.
type AlchemySelection []map[string]bool

func (s AlchemySelection) Clone() AlchemySelection {
	out := make(AlchemySelection, len(s))
	for i, group := range s {
		out[i] = maps.Clone(group)
	}
	return out
}

// AlchemySourceSelection is the adapter for legacy numeric source choices.
func AlchemySourceSelection(recipe *config.AlchemyRecipe, choices []int) AlchemySelection {
	if recipe == nil {
		return nil
	}
	out := make(AlchemySelection, len(recipe.Ingredients))
	for i, group := range recipe.Ingredients {
		out[i] = map[string]bool{}
		choice := 0
		if i < len(choices) {
			choice = choices[i]
		}
		if choice < 0 || choice >= len(group.Alternatives) {
			continue
		}
		for _, key := range group.Alternatives[choice].Items {
			out[i][key] = true
		}
	}
	return out
}

// PlanAlchemySelected reserves groups in recipe order, then finds the cheapest
// exact mixture of checked materials. Whole items contribute 1/authoredCount of
// a batch. Integer units preserve the recipe's costs without rounding or waste.
func (p *Party) PlanAlchemySelected(recipe *config.AlchemyRecipe, selected AlchemySelection, batches int) (map[int]int, error) {
	if p == nil || recipe == nil || batches < 1 || batches > 99 {
		return nil, fmt.Errorf("Invalid batch.")
	}
	if len(selected) != len(recipe.Ingredients) {
		return nil, fmt.Errorf("Choose materials for each ingredient.")
	}
	inventory := p.CarriedItems()
	plan := map[int]int{}
	order := make([]int, len(inventory))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return inventory[order[i]].Attributes["value"] < inventory[order[j]].Attributes["value"]
	})
	type allocation struct {
		previous     *allocation
		index, count int
	}
	for gi, group := range recipe.Ingredients {
		units, err := group.Units()
		if err != nil {
			return nil, err
		}
		costsByName := map[string]int{}
		for key, count := range group.Materials() {
			if !selected[gi][key] {
				continue
			}
			def, ok := config.GetItemDefinition(key)
			if !ok {
				return nil, fmt.Errorf("Unknown ingredient %s.", key)
			}
			costsByName[def.Name] = count
		}
		if len(costsByName) == 0 {
			return nil, fmt.Errorf("Select materials for %s.", group.Label)
		}
		target := units * batches
		costs := make([]int64, target+1)
		const unreachable = int64(1 << 62)
		for i := 1; i <= target; i++ {
			costs[i] = unreachable
		}
		paths := make([]*allocation, target+1)
		for _, index := range order {
			item := inventory[index]
			count, ok := costsByName[item.Name]
			if !ok || item.Type != items.ItemTrinket {
				continue
			}
			weight := units / count
			available := min(item.Count()-plan[index], target/weight)
			// Binary bundles make bounded allocation proportional to log(stock), while
			// immutable path nodes keep reconstruction correct after later DP updates.
			for bundle := 1; available > 0; bundle *= 2 {
				take := min(bundle, available)
				available -= take
				w, price := take*weight, int64(take)*int64(max(0, item.Attributes["value"]))
				for total := target; total >= w; total-- {
					if costs[total-w] == unreachable {
						continue
					}
					candidate := costs[total-w] + price
					if candidate < costs[total] {
						costs[total] = candidate
						paths[total] = &allocation{paths[total-w], index, take}
					}
				}
			}
		}
		if costs[target] == unreachable {
			return nil, fmt.Errorf("Not enough selected %s.", group.Label)
		}
		for node := paths[target]; node != nil; node = node.previous {
			plan[node.index] += node.count
		}
	}
	return plan, nil
}

// BrewSelected returns the output count and snapshots of the materials actually spent.
// The receipt follows the allocation plan, including mixed and split stacks.
func (p *Party) BrewSelected(caster *MMCharacter, recipe *config.AlchemyRecipe, selected AlchemySelection, batches int) (int, []items.Item, error) {
	if caster == nil || caster.Class != ClassAlchemist || !caster.CanUseCombatAction() || !caster.HasSkill(SkillAlchemy) {
		return 0, nil, fmt.Errorf("A capable Alchemist must prepare the batch.")
	}
	active := false
	for _, m := range p.Members {
		if m == caster {
			active = true
		}
	}
	if !active {
		return 0, nil, fmt.Errorf("The Alchemist must be in the active party.")
	}
	plan, err := p.PlanAlchemySelected(recipe, selected, batches)
	if err != nil {
		return 0, nil, err
	}
	output, err := items.TryCreateItemFromYAML(recipe.Output)
	if err != nil {
		return 0, nil, err
	}
	def, ok := config.GetItemDefinition(recipe.Output)
	if !ok || !def.CraftedOnly {
		return 0, nil, fmt.Errorf("Invalid recipe output.")
	}
	count := AlchemyYield(caster.SkillTier(SkillAlchemy), recipe.Family) * batches
	output.Quantity = count
	inventory := p.CarriedItems()
	var ingredients []items.Item
	byName := map[string]int{}
	for i, item := range inventory {
		if n := plan[i]; n > 0 {
			if at, exists := byName[item.Name]; exists {
				ingredients[at].Quantity += n
			} else {
				item.Quantity = n
				item.Attributes = maps.Clone(item.Attributes)
				// Cosmetic snapshots are not inventory or provenance records.
				item.InstanceID, item.Lineages = 0, nil
				byName[item.Name] = len(ingredients)
				ingredients = append(ingredients, item)
			}
		}
	}
	// Descending indices preserve the staged plan as whole stacks disappear.
	for i := len(inventory) - 1; i >= 0; i-- {
		if n := plan[i]; n > 0 {
			p.ConsumeCarriedUnitsAt(i, n)
		}
	}
	p.AddItem(output)
	return count, ingredients, nil
}

// MaxAlchemySelected uses the same allocation rules as the commit, including
// overlapping alternative groups. The UI batch limit is deliberately shared.
func (p *Party) MaxAlchemySelected(recipe *config.AlchemyRecipe, selected AlchemySelection) int {
	low, high := 0, 99
	for low < high {
		mid := (low + high + 1) / 2
		if _, err := p.PlanAlchemySelected(recipe, selected, mid); err == nil {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
}

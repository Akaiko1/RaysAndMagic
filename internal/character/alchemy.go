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
	reserved := map[string]string{}
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

// PlanAlchemy stages every input before changing inventory. Allocation accounts
// for overlapping alternatives across groups and consumes only the chosen source.
func (p *Party) PlanAlchemy(recipe *config.AlchemyRecipe, choices []int, batches int) (map[int]int, error) {
	if p == nil || recipe == nil || batches < 1 || batches > 99 {
		return nil, fmt.Errorf("Invalid batch.")
	}
	plan := map[int]int{}
	// A pooled source spends the cheapest eligible stock first. Keep bag order
	// only as a tie-breaker; preview and commit share this allocation.
	order := make([]int, len(p.Inventory))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return p.Inventory[order[i]].Attributes["value"] < p.Inventory[order[j]].Attributes["value"]
	})
	for groupIndex, group := range recipe.Ingredients {
		choice := 0
		if groupIndex < len(choices) {
			choice = choices[groupIndex]
		}
		if choice < 0 || choice >= len(group.Alternatives) {
			return nil, fmt.Errorf("Choose an ingredient for %s.", group.Label)
		}
		selected := group.Alternatives[choice]
		names := map[string]bool{}
		for _, key := range selected.Items {
			def, ok := config.GetItemDefinition(key)
			if !ok {
				return nil, fmt.Errorf("Unknown ingredient %s.", key)
			}
			names[def.Name] = true
		}
		need := selected.Count * batches
		for _, i := range order {
			item := p.Inventory[i]
			if item.Type != items.ItemTrinket || !names[item.Name] {
				continue
			}
			take := min(need, item.Count()-plan[i])
			if take <= 0 {
				continue
			}
			plan[i] += take
			need -= take
			if need == 0 {
				break
			}
		}
		if need > 0 {
			return nil, fmt.Errorf("Missing %s: %d more required.", group.Label, need)
		}
	}
	return plan, nil
}

// Brew returns the output count and snapshots of the materials actually spent.
// The receipt follows the allocation plan, including mixed and split stacks.
func (p *Party) Brew(caster *MMCharacter, recipe *config.AlchemyRecipe, choices []int, batches int) (int, []items.Item, error) {
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
	plan, err := p.PlanAlchemy(recipe, choices, batches)
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
	var ingredients []items.Item
	byName := map[string]int{}
	for i, item := range p.Inventory {
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
	for i := len(p.Inventory) - 1; i >= 0; i-- {
		if n := plan[i]; n > 0 {
			p.ConsumeUnitsAt(i, n)
		}
	}
	p.AddItem(output)
	return count, ingredients, nil
}

// MaxAlchemyBatches uses the same allocation rules as the commit, including
// overlapping alternative groups. The UI batch limit is deliberately shared.
func (p *Party) MaxAlchemyBatches(recipe *config.AlchemyRecipe, choices []int) int {
	low, high := 0, 99
	for low < high {
		mid := (low + high + 1) / 2
		if _, err := p.PlanAlchemy(recipe, choices, mid); err == nil {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
}

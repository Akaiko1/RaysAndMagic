package game

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
)

func TestSolsticeDamageDraughts(t *testing.T) {
	for _, school := range []string{"fire", "water", "earth", "air"} {
		for _, tier := range []struct {
			suffix  string
			percent int
		}{{"_field", 15}, {"", 30}, {"_advanced", 40}} {
			for _, brewed := range []bool{false, true} {
				if brewed && tier.suffix == "_advanced" {
					continue
				}
				key := "solstice_" + school + tier.suffix + "_ward"
				if brewed {
					key = "brewed_" + key
				}
				t.Run(key, func(t *testing.T) {
					g := newTestCombatSystemWithConfig(t).game
					d, _ := config.GetItemDefinition(key)
					if d.DamageBuffPct != tier.percent || d.DamageBuffSchool != school || d.ResistBuffSchoolPct != 0 {
						t.Fatalf("incorrect effect: %+v", d)
					}
					g.party.Inventory = []items.Item{items.CreateItemFromYAML(key)}
					if !g.UseConsumableFromInventory(0, 0) {
						t.Fatal("not consumed")
					}
					g.combatBuffs = restoreCombatBuffs(buildCombatBuffSaves(g.combatBuffs))
					if len(g.combatBuffs) != 1 || g.combatBuffs[0].Frames != 60*g.config.GetTPS() {
						t.Fatal("lost clock")
					}
					for _, target := range []string{"fire", "water", "earth", "air", "physical"} {
						want := 100
						if target == school {
							want += tier.percent
						}
						got := g.elementalDamageBuff(damagecalc.Parts{Normal: 100, True: 100}, target)
						if got.Normal != want || got.True != want {
							t.Fatalf("%s: %+v want %d", target, got, want)
						}
					}
					card := GetItemTooltip(items.CreateItemFromYAML(key), nil, g.combat, true)
					if !strings.Contains(card, fmt.Sprintf("damage +%d%%", tier.percent)) || strings.Contains(card, "resistance") {
						t.Fatal(card)
					}
				})
			}
		}
	}
}

func TestElementalDraughtSharedDamagePaths(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.turnBasedMode = tb
			for _, pct := range []int{15, 30, 40} {
				g.addCombatBuff(TimedCombatBuff{SpellID: fmt.Sprint(pct), Frames: 600, OutDamageType: "fire", OutPercent: pct})
			}
			g.addCombatBuff(TimedCombatBuff{SpellID: "flat", Frames: 600, OutBonus: 10})
			m, _ := partyDamageTargets(g, 64)
			attack := cs.newPartyWeaponAttack(100, 20, "fire", nil, "test", false, false, 0)
			if hit := cs.applyPartyMonsterAttack(m, attack); hit.Normal != 154 || hit.True != 28 {
				t.Fatalf("weapon hit %+v", hit)
			}
			mp := &MagicProjectile{SpellType: "fireball", Damage: 100, TrueDamage: 20}
			if hit := cs.applyPartyMonsterAttack(m, cs.spellProjectileAttack(mp)); hit.Normal != 154 || hit.True != 28 {
				t.Fatalf("spell hit %+v", hit)
			}
			parts, _ := cs.spellPartsWithOutgoingBuff(damagecalc.Parts{Normal: 100, True: 20}, "fire")
			if parts.Normal != 154 || parts.True != 28 {
				t.Fatal(parts)
			}
			g.combatBuffs[2].Frames = 0
			if got := g.elementalDamageBuff(damagecalc.Parts{Normal: 100}, "fire").Normal; got != 130 {
				t.Fatalf("expired strongest still applies: %d", got)
			}
		})
	}
}

func TestAlchemyFiltersKeepRecipeIdentity(t *testing.T) {
	g, _ := rareClassGame(t, character.ClassAlchemist, false)
	g.alchemyRecipeFilter, g.alchemyElementFilter = "Damage", "Fire"
	visible := g.visibleAlchemyRecipes()
	if len(visible) < 3 {
		t.Fatalf("missing recipes: %v", visible)
	}
	for _, i := range visible {
		d, _ := config.GetItemDefinition(config.GlobalAlchemy.Recipes[i].Output)
		if alchemyRecipeCategory(d) != "Damage" || alchemyRecipeElement(d) != "Fire" {
			t.Fatal(d.Name)
		}
	}
	g.selectedRare = visible[len(visible)-1]
	g.selectVisibleAlchemyRecipe(visible, 1)
	if g.selectedRare != visible[0] {
		t.Fatal("navigation lost catalog identity")
	}
	g.alchemyRecipeFilter = "Revival"
	if got := g.visibleAlchemyRecipes(); len(got) != 0 {
		t.Fatal(got)
	}
	g.selectVisibleAlchemyRecipe(nil, 0)
	if g.selectedRare != -1 || g.brewSelectedRecipe() {
		t.Fatal("empty results can brew hidden recipe")
	}
	g.alchemyRecipeFilter, g.alchemyElementFilter = "All", "All"
	if len(g.visibleAlchemyRecipes()) != len(config.GlobalAlchemy.Recipes) {
		t.Fatal("reset lost recipes")
	}
	g.party.Inventory = nil
	g.alchemyBrewableOnly = true
	if len(g.visibleAlchemyRecipes()) != 0 {
		t.Fatal("empty bag has brewable recipe")
	}
	r := config.AlchemyRecipeByKey("solstice_fire_ward")
	for _, key := range []string{"solstice_living_ember", "solstice_furnace_bloom"} {
		g.party.AddItem(items.CreateItemFromYAML(key))
	}
	got := g.visibleAlchemyRecipes()
	i := slices.IndexFunc(config.GlobalAlchemy.Recipes, func(x config.AlchemyRecipe) bool { return x.Key == r.Key })
	if !slices.Contains(got, i) {
		t.Fatal("stock change did not update filter")
	}
	g.selectedRare = i
	if !g.brewSelectedRecipe() {
		t.Fatal("filtered recipe failed to brew")
	}
	if slices.Contains(g.visibleAlchemyRecipes(), i) {
		t.Fatal("consumed recipe still brewable")
	}
}

func TestPreparationGoldExchange(t *testing.T) {
	g := newTestCombatSystemWithConfig(t).game
	c := &character.NPCDialogueChoice{Exchange: &character.NPCExchange{Gold: 900, Costs: map[string]int{"grave_orchid": 1}}}
	g.party.Inventory = nil
	before := g.party.Gold
	g.exchangePreparation(c)
	if g.party.Gold != before {
		t.Fatal("unpaid exchange")
	}
	g.party.AddItem(items.CreateItemFromYAML("grave_orchid"))
	g.exchangePreparation(c)
	if g.party.Gold != before+900 || g.party.CountItemsByName("Grave Orchid") != 0 {
		t.Fatal("incorrect exchange")
	}
	g.exchangePreparation(c)
	if g.party.Gold != before+900 {
		t.Fatal("duplicate payment")
	}
}

func TestPreparationDawnleafExchange(t *testing.T) {
	g := newTestCombatSystemWithConfig(t).game
	c := &character.NPCDialogueChoice{Exchange: &character.NPCExchange{Output: "dawnleaf", Costs: map[string]int{"bitterroot": 2}}}
	g.party.Inventory = nil
	g.party.AddItem(items.CreateItemFromYAML("bitterroot"))
	g.exchangePreparation(c)
	if g.party.CountItemsByName("Bitterroot") != 1 || g.party.CountItemsByName("Dawnleaf") != 0 {
		t.Fatal("incomplete payment consumed")
	}
	g.party.AddItem(items.CreateItemFromYAML("bitterroot"))
	g.exchangePreparation(c)
	if g.party.CountItemsByName("Bitterroot") != 0 || g.party.CountItemsByName("Dawnleaf") != 1 {
		t.Fatal("incorrect herb exchange")
	}
}

package game

import (
	"fmt"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestAutomaticConsumableSourcesAndThresholds(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, source := range []string{"bag", "quick"} {
			for _, key := range []string{"health_potion", "sake_flask", "mana_potion"} {
				for _, percent := range []int{34, 35, 36} {
					t.Run(fmt.Sprintf("tb=%v/%s/%s/%d", tb, source, key, percent), func(t *testing.T) {
						g, _, ch, _ := sniperFixture(t, tb)
						ch.MaxHitPoints, ch.MaxSpellPoints = 100, 100
						ch.HitPoints, ch.SpellPoints = 100, 100
						if key == "mana_potion" {
							ch.SpellPoints = percent
						} else {
							ch.HitPoints = percent
						}
						item := items.CreateItemFromYAML(key)
						item.Quantity = 3
						if source == "bag" {
							g.party.Inventory = []items.Item{item}
						} else {
							ch.QuickSlots[2] = &item
						}
						g.updateAutomaticConsumables()
						left := 0
						if source == "bag" {
							left = g.party.Inventory[0].Count()
						} else {
							left = ch.QuickSlots[2].Count()
						}
						want := 3
						if percent < 35 {
							want = 2
						}
						if left != want {
							t.Fatalf("units=%d want %d", left, want)
						}
						if percent < 35 && ch.AutoDrinkCooldown <= 0 {
							t.Fatal("no automatic-use interval")
						}
						if percent < 35 {
							ch.HitPoints, ch.SpellPoints = 1, 1
							g.updateAutomaticConsumables()
							if source == "bag" {
								left = g.party.Inventory[0].Count()
							} else {
								left = ch.QuickSlots[2].Count()
							}
							if left != 2 {
								t.Fatal("drank again in same interval")
							}
						}
					})
				}
			}
		}
	}
}

func TestAutomaticConsumableEligibility(t *testing.T) {
	for _, name := range []string{"dead", "unconscious", "stunned", "eradicated", "menu", "picker", "drag", "no mana capacity", "revival", "ward", "empty", "other pocket"} {
		t.Run(name, func(t *testing.T) {
			g, _, ch, _ := sniperFixture(t, false)
			ch.MaxHitPoints = 100
			ch.HitPoints = 1
			potion := items.CreateItemFromYAML("health_potion")
			potion.Quantity = 3
			g.party.Inventory = []items.Item{potion}
			switch name {
			case "dead":
				ch.Conditions = []character.Condition{character.ConditionDead}
			case "unconscious":
				ch.Conditions = []character.Condition{character.ConditionUnconscious}
			case "eradicated":
				ch.Conditions = []character.Condition{character.ConditionEradicated}
			case "stunned":
				ch.StunFramesRemaining = 120
			case "menu":
				g.menuOpen = true
			case "picker":
				g.healPickerOpen = true
			case "drag":
				g.dragPickedUp = true
			case "no mana capacity":
				ch.HitPoints = 100
				ch.MaxSpellPoints = 0
				g.party.Inventory = []items.Item{items.CreateItemFromYAML("mana_potion")}
			case "revival":
				g.party.Inventory = []items.Item{items.CreateItemFromYAML("revival_potion")}
			case "ward":
				g.party.Inventory = []items.Item{items.CreateItemFromYAML("stoneskin_draught")}
			case "empty":
				g.party.Inventory = nil
			case "other pocket":
				other := character.CreateCharacter("Other", character.ClassKnight, g.config)
				other.QuickSlots[0] = &potion
				g.party.Members = append(g.party.Members, other)
				g.party.Inventory = nil
			}
			before := g.party.GetTotalItems()
			hp := ch.HitPoints
			g.updateAutomaticConsumables()
			if g.party.GetTotalItems() != before || ch.HitPoints != hp || ch.AutoDrinkCooldown != 0 {
				t.Fatal("ineligible automatic use")
			}
		})
	}
}

func TestAutomaticConsumablePriorityAndSharedManualEffects(t *testing.T) {
	for _, source := range []string{"manual bag", "manual quick", "auto bag", "auto quick"} {
		t.Run(source, func(t *testing.T) {
			g, _, ch, _ := sniperFixture(t, false)
			ch.MaxHitPoints, ch.MaxSpellPoints = 1000, 1000
			ch.HitPoints, ch.SpellPoints = 1, 1
			ch.Skills[character.SkillFieldMedicine].Mastery = character.MasteryGrandMaster
			p := items.CreateItemFromYAML("sake_flask")
			p.Quantity = 2
			if strings.HasSuffix(source, "quick") {
				ch.QuickSlots[0] = &p
			} else {
				g.party.Inventory = []items.Item{p}
			}
			want := 1 + character.ConsumableRestore(ch, p.Attributes["heal_base"], p.Attributes["heal_endurance_divisor"], false)
			if source == "manual bag" {
				g.UseConsumableFromInventory(0, 0)
			} else if source == "manual quick" {
				g.useQuickSlot(0, 0)
			} else {
				g.updateAutomaticConsumables()
			}
			if ch.HitPoints != want || ch.SpellPoints != 1 {
				t.Fatalf("healing=%d want %d; SP=%d", ch.HitPoints, want, ch.SpellPoints)
			}
		})
	}
	g, _, ch, _ := sniperFixture(t, false)
	ch.MaxHitPoints, ch.MaxSpellPoints = 100, 100
	ch.HitPoints, ch.SpellPoints = 1, 1
	quick := items.CreateItemFromYAML("sake_flask")
	quick.Quantity = 2
	ch.QuickSlots[0] = &quick
	g.party.Inventory = []items.Item{items.CreateItemFromYAML("health_potion"), items.CreateItemFromYAML("mana_potion")}
	g.updateAutomaticConsumables()
	if ch.QuickSlots[0].Count() != 1 || len(g.party.Inventory) != 2 || ch.SpellPoints != 1 {
		t.Fatal("own pocket/HP priority violated")
	}
	ch.QuickSlots[0] = nil
	g.party.Inventory = g.party.Inventory[1:]
	ch.AutoDrinkCooldown = 0
	g.updateAutomaticConsumables()
	if ch.SpellPoints <= 1 || len(g.party.Inventory) != 0 {
		t.Fatal("no HP supplies blocked available mana potion")
	}
}

func TestAutomaticConsumableTurnClockAndModeSwitch(t *testing.T) {
	g, _, ch, _ := sniperFixture(t, true)
	ch.MaxHitPoints = 1000
	ch.HitPoints = 1
	p := items.CreateItemFromYAML("health_potion")
	p.Quantity = 5
	g.party.Inventory = []items.Item{p}
	g.updateAutomaticConsumables()
	first := ch.HitPoints
	for range 1000 {
		g.updateAutomaticConsumables()
	}
	if ch.HitPoints != first || g.party.Inventory[0].Count() != 4 {
		t.Fatal("TB wall time advanced drinking")
	}
	g.startPartyTurn()
	g.updateAutomaticConsumables()
	if ch.HitPoints <= first || g.party.Inventory[0].Count() != 3 {
		t.Fatal("next round did not permit drinking")
	}
	remaining := ch.AutoDrinkCooldown
	g.ToggleTurnBasedMode()
	g.ToggleTurnBasedMode()
	if ch.AutoDrinkCooldown != remaining {
		t.Fatal("mode switch reset drinking interval")
	}
}

func TestAutomaticConsumablesProductionLoop(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	h.g.menuOpen = false
	h.g.turnBasedMode = true
	h.g.world.Monsters = nil
	ch := h.g.party.Members[0]
	ch.HitPoints = 1
	p := items.CreateItemFromYAML("sake_flask")
	p.Quantity = 2
	ch.QuickSlots[0] = &p
	h.pointerStep()
	if ch.HitPoints <= 1 || ch.QuickSlots[0].Count() != 1 {
		t.Fatal("active game loop never dispatched automatic drinking")
	}
}

func TestAutomaticRestorativeAntivenomRequiresPoison(t *testing.T) {
	for _, poisoned := range []bool{false, true} {
		g, _, ch, _ := sniperFixture(t, false)
		ch.HitPoints = 1
		g.party.Inventory = []items.Item{items.CreateItemFromYAML("antivenom")}
		if poisoned {
			ch.ApplyPoison(1000)
		}
		g.updateAutomaticConsumables()
		if (len(g.party.Inventory) == 0) != poisoned || ch.HasCondition(character.ConditionPoisoned) {
			t.Fatal("automatic antivenom eligibility disagrees with its manual use")
		}
	}
}

func TestAutomaticConsumableRealTimeInterval(t *testing.T) {
	g, _, ch, _ := sniperFixture(t, false)
	ch.MaxHitPoints = 1000
	ch.HitPoints = 1
	p := items.CreateItemFromYAML("health_potion")
	p.Quantity = 4
	g.party.Inventory = []items.Item{p}
	g.updateAutomaticConsumables()
	interval := ch.AutoDrinkCooldown
	for range interval - 1 {
		g.updateAutomaticConsumables()
	}
	if g.party.Inventory[0].Count() != 3 {
		t.Fatal("automatic drinking bypassed interval")
	}
	g.updateAutomaticConsumables()
	if g.party.Inventory[0].Count() != 2 {
		t.Fatal("automatic drinking did not resume after interval")
	}
}

func TestAutomaticDrinkingAvailableToEveryClass(t *testing.T) {
	for _, class := range character.PlayableClasses {
		for _, tb := range []bool{false, true} {
			for _, mana := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/TB=%v/mana=%v", class.Key(), tb, mana), func(t *testing.T) {
					g, _, _, _ := sniperFixture(t, tb)
					ch := character.CreateCharacter("Drinker", class, g.config)
					g.party.Members = []*character.MMCharacter{ch}
					ch.MaxHitPoints, ch.MaxSpellPoints = 1000, 1000
					ch.HitPoints, ch.SpellPoints = 1000, 1000
					key, baseKey, divisorKey := "health_potion", "heal_base", "heal_endurance_divisor"
					if mana {
						key, baseKey, divisorKey = "mana_potion", "mana_base", "mana_personality_divisor"
						ch.SpellPoints = 1
					} else {
						ch.HitPoints = 1
					}
					potion := items.CreateItemFromYAML(key)
					potion.Quantity = 2
					g.party.Inventory = []items.Item{potion}
					bonus := 0
					if ch.HasSkill(character.SkillFieldMedicine) {
						bonus = character.FieldMedicineRestorePct(ch.SkillTier(character.SkillFieldMedicine))
					}
					stat := ch.GetEffectiveEndurance()
					if mana {
						stat = ch.GetEffectivePersonality()
					}
					base := potion.Attributes[baseKey]
					if divisor := potion.Attributes[divisorKey]; divisor > 0 {
						base += stat / divisor
					}
					want := 1 + base*(100+bonus)/100
					g.updateAutomaticConsumables()
					got := ch.HitPoints
					if mana {
						got = ch.SpellPoints
					}
					if got != want || len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 1 || ch.AutoDrinkCooldown <= 0 {
						t.Fatalf("automatic drinking: got=%d want=%d, inventory=%v", got, want, g.party.Inventory)
					}
				})
			}
		}
	}
}

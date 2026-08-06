package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

func TestDragonHoardSetLootAndCriticalChance(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}

	for _, tc := range []struct {
		monsterKey string
		chance     float64
	}{
		{"dragon_green", 0.05},
		{"dragon_gold", 0.06},
	} {
		t.Run(tc.monsterKey+" drops both pieces", func(t *testing.T) {
			entries := config.GetLootTable(tc.monsterKey, false)
			for _, want := range []struct {
				typ, key string
			}{
				{"item", "golden_armor"},
				{"weapon", "gold_sword"},
			} {
				found := false
				for _, entry := range entries {
					if entry.Type == want.typ && entry.Key == want.key {
						found = true
						if entry.Chance != tc.chance {
							t.Errorf("%s chance = %.2f, want %.2f", want.key, entry.Chance, tc.chance)
						}
					}
				}
				if !found {
					t.Errorf("missing %s %q from %s loot", want.typ, want.key, tc.monsterKey)
				}
			}
		})
	}

	set := config.GetItemSet("dragon_hoard")
	if set == nil || set.Name != "Dragon's Hoard" || set.PiecesRequired != 2 ||
		set.BonusCritChance != 75 || strings.Join(set.RequiredPieces, ",") != "golden_armor,gold_sword" {
		t.Fatalf("dragon_hoard set = %+v, want the authored two-piece +75%% crit set", set)
	}

	armor := items.CreateItemFromYAML("golden_armor")
	sword := items.CreateWeaponFromYAML("gold_sword")
	if armor.Set != "dragon_hoard" || sword.Set != "dragon_hoard" {
		t.Fatalf("set membership armor=%q sword=%q, want dragon_hoard", armor.Set, sword.Set)
	}

	ch := cs.game.party.Members[0]
	ch.Luck = 0
	delete(ch.Skills, character.SkillSword)
	delete(ch.Skills, character.SkillArmsMaster)
	ch.Equipment = map[items.EquipSlot]items.Item{}
	baseCrit := cs.CalculateWeaponCritChance(sword, ch)
	if baseCrit != 25 {
		t.Fatalf("Gold Sword base crit = %d, want 25", baseCrit)
	}

	// A duplicate sword must not replace the armor in an exact-piece set.
	ch.Equipment[items.SlotMainHand] = sword
	ch.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("gold_sword")
	if got := cs.CalculateWeaponCritChance(sword, ch); got != baseCrit {
		t.Fatalf("two Gold Swords crit = %d, want %d without Golden Armor", got, baseCrit)
	}

	ch.Equipment = map[items.EquipSlot]items.Item{
		items.SlotMainHand: sword,
		items.SlotArmor:    armor,
	}
	if got := cs.CalculateWeaponCritChance(sword, ch); got != 100 {
		t.Fatalf("completed Dragon's Hoard crit = %d, want 100", got)
	}
	if got := cs.CalculateCriticalChance(ch); got != 75 {
		t.Fatalf("Dragon's Hoard spell crit bonus = %d, want 75", got)
	}
	if _, total := cs.RollCriticalChance(0, ch); total != 75 {
		t.Fatalf("Dragon's Hoard spell crit total = %d, want 75", total)
	}
	spellTip := GetSpellTooltip(spells.SpellID("fireball"), ch, cs, true)
	for _, want := range []string{"Chance: 75%", "Set: +75%"} {
		if !strings.Contains(spellTip, want) {
			t.Errorf("Fireball tooltip missing %q:\n%s", want, spellTip)
		}
	}

	weaponTip := GetItemTooltip(sword, ch, cs, true)
	for _, want := range []string{
		"Chance: 100%", "Set: Dragon's Hoard (2 pieces)",
		"Set bonus: critical chance +75%", "Set: +75%",
	} {
		if !strings.Contains(weaponTip, want) {
			t.Errorf("Gold Sword tooltip missing %q:\n%s", want, weaponTip)
		}
	}
	armorTip := GetItemTooltip(armor, ch, cs, true)
	if !strings.Contains(armorTip, "Set bonus: critical chance +75%") {
		t.Errorf("Golden Armor tooltip missing set bonus:\n%s", armorTip)
	}
}

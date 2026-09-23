package game

import (
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

func gmReferenceChar(cfg *config.Config) *character.MMCharacter {
	c := character.CreateCharacter("Parity", character.ClassSorcerer, cfg)
	for _, s := range character.AllSkills {
		c.Skills[s] = &character.Skill{Mastery: character.MasteryGrandMaster}
	}
	for _, id := range character.AllMagicSchools {
		c.MagicSchools[id] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
	}
	c.Might, c.Intellect, c.Personality = 40, 40, 40
	c.Endurance, c.Accuracy, c.Speed, c.Luck = 40, 40, 40, 40
	return c
}

// A catalog has no bearer. Even a live combat system with active party state
// must produce exactly the same base tooltip as the standalone editor.
func TestBaseTooltipsIndependentOfParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.party.Members = []*character.MMCharacter{gmReferenceChar(cs.game.config)}
	cs.game.combatBuffs = []TimedCombatBuff{{SpellID: "test", Frames: 600, OutBonus: 99, OutDamageType: "all"}}
	for _, full := range []bool{false, true} {
		check := func(key string, it items.Item) {
			t.Helper()
			t.Run(key, func(t *testing.T) {
				base := GetItemTooltip(it, nil, nil, full)
				shop := GetItemTooltip(it, nil, cs, full)
				if base == "" || base != shop {
					t.Fatalf("base/shop mismatch:\n%s\n%s", base, shop)
				}
				for _, leak := range []string{"Active party buff:", "\nCurrent ", "Mastery - Grandmaster", "Field Medicine:"} {
					if strings.Contains(base, leak) {
						t.Errorf("base tooltip leaked %q: %s", leak, base)
					}
				}
			})
		}
		for key := range config.GlobalWeapons.Weapons {
			check("weapon/"+key, items.CreateWeaponFromYAML(key))
		}
		for key := range config.GlobalItems.Items {
			check("item/"+key, items.CreateItemFromYAML(key))
		}
		for _, key := range config.TrapKeysOrdered() {
			it, _ := config.TrapItem(key)
			check("trap/"+key, it)
		}
		for key := range config.GlobalSpells.Spells {
			it, err := spells.CreateSpellItem(spells.SpellID(key))
			if err != nil {
				t.Fatal(err)
			}
			check("spell/"+key, it)
		}
	}
}

func baseTestItem(t *testing.T, name string) items.Item {
	t.Helper()
	_, key, ok := config.GetItemDefinitionByName(name)
	if !ok {
		t.Fatalf("unknown fixture item %s", name)
	}
	return items.CreateItemFromYAML(key)
}

func TestSpellItemBaseUsesCurrentDefinition(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	it, err := spells.CreateSpellItem("fireball")
	if err != nil {
		t.Fatal(err)
	}
	it.Description = "Obsolete saved spell damage"
	for _, full := range []bool{false, true} {
		tip := GetItemTooltip(it, nil, cs, full)
		if strings.Contains(tip, it.Description) || !strings.Contains(tip, "Total Damage: 12") {
			t.Fatal(tip)
		}
	}
	// Confirm the live path still applies real party buffs.
	cs.game.combatBuffs = []TimedCombatBuff{{SpellID: "test", Frames: 600, OutBonus: 99, OutDamageType: "all"}}
	live := GetSpellTooltip("fireball", gmReferenceChar(cs.game.config), cs, true)
	if !strings.Contains(live, "Active party buff: +99") {
		t.Fatal(live)
	}
}

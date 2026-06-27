package game

// Armor mitigation tests — exercise the REAL CombatSystem path:
// CalculateTotalArmorClass → armorMitigationPct (% with diminishing returns,
// capped 75% physical / 33% elemental) → mitigateCharacterDamage pipeline
// (armor % → resist % → flat buff → floor; 100% resist = immune).

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// equipArmorPieces equips the named items.yaml entries onto the first party
// member's armor slots. Caller is responsible for ensuring the character
// has any skill required to wear them.
func equipArmorPieces(t *testing.T, char *character.MMCharacter, keys ...string) {
	t.Helper()
	if char.Equipment == nil {
		char.Equipment = make(map[items.EquipSlot]items.Item)
	}
	for _, k := range keys {
		item, err := items.TryCreateItemFromYAML(k)
		if err != nil {
			t.Fatalf("item %q missing from items.yaml: %v", k, err)
		}
		if _, _, ok := char.EquipItem(item); !ok {
			t.Fatalf("EquipItem(%s) failed (missing skill?)", item.Name)
		}
	}
}

func TestArmorMitigationPct_NoArmor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	if mit := cs.armorMitigationPct(char, true); mit != 0 {
		t.Errorf("no-armor physical mitigation: got %d%%, want 0", mit)
	}
	if mit := cs.armorMitigationPct(char, false); mit != 0 {
		t.Errorf("no-armor elemental mitigation: got %d%%, want 0", mit)
	}
}

func TestArmorMitigationPct_FormulaAndCaps(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	equipArmorPieces(t, char, "leather_armor")
	ac := cs.CalculateTotalArmorClass(char)
	if ac <= 0 {
		t.Fatalf("expected positive AC after leather armor, got %d", ac)
	}
	wantPhys := 100 * ac / (ac + ArmorMitigationK)
	if wantPhys > ArmorPhysicalMitigationCap {
		wantPhys = ArmorPhysicalMitigationCap
	}
	// Elemental is the physical curve scaled to reach 33% when physical reaches 75%.
	wantElem := wantPhys * ArmorElementalMitigationCap / ArmorPhysicalMitigationCap
	if got := cs.armorMitigationPct(char, true); got != wantPhys {
		t.Errorf("physical mit: got %d%%, want %d%% (AC %d, K %d)", got, wantPhys, ac, ArmorMitigationK)
	}
	if got := cs.armorMitigationPct(char, false); got != wantElem {
		t.Errorf("elemental mit: got %d%%, want %d%% (scaled to cap %d)", got, wantElem, ArmorElementalMitigationCap)
	}
}

func TestMitigateCharacterDamage_FloorsAt1(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	// Capped armor (≤75%) can never fully negate without 100% resist, so a tiny
	// physical hit always chips at least 1.
	equipArmorPieces(t, char, "leather_armor", "leather_helmet", "leather_pants")
	if got := cs.mitigateCharacterDamage(1, "physical", char, false); got != 1 {
		t.Errorf("incoming 1 physical damage with armor: got %d, want 1 (floor)", got)
	}
}

// Total Armor Class aggregates additively across every equipped armor slot, so
// the percentage mitigation derived from it grows with each piece.
func TestTotalArmorClass_MultiSlotIsAdditive(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	// Measure AC after each piece is added — should grow strictly monotonically.
	zeroAC := cs.CalculateTotalArmorClass(char)
	if zeroAC != 0 {
		t.Fatalf("starter has unexpected baseline AC: %d", zeroAC)
	}
	equipArmorPieces(t, char, "leather_armor")
	oneAC := cs.CalculateTotalArmorClass(char)
	equipArmorPieces(t, char, "leather_helmet")
	twoAC := cs.CalculateTotalArmorClass(char)
	equipArmorPieces(t, char, "leather_pants")
	threeAC := cs.CalculateTotalArmorClass(char)

	if !(oneAC > zeroAC && twoAC > oneAC && threeAC > twoAC) {
		t.Errorf("AC should grow with each armor piece, got %d → %d → %d → %d",
			zeroAC, oneAC, twoAC, threeAC)
	}

	// Per-piece contributions should sum to total.
	pieces := []items.EquipSlot{items.SlotArmor, items.SlotHelmet, items.SlotBoots}
	sum := 0
	for _, slot := range pieces {
		if item, ok := char.Equipment[slot]; ok {
			sum += cs.CalculateArmorClassContribution(item, char)
		}
	}
	if sum != threeAC {
		t.Errorf("per-slot contributions sum %d ≠ total AC %d", sum, threeAC)
	}
}

// This is the full runtime path, not a direct mitigation-unit test:
// monsters.yaml Minotaur -> HandleMonsterInteractions -> real party HP loss.
// Armor is loaded from items.yaml and equipped through EquipItem; Bless and
// Stone Skin are loaded from spells.yaml and cast through CastEquippedSpell.
func TestRealMonsterAttack_ArmorBlessAndStoneSkin(t *testing.T) {
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	type scenario struct {
		armorKeys []string
		bless     bool
		stoneSkin bool
	}
	tests := map[string]scenario{
		"naked":                   {},
		"one leather piece":       {armorKeys: []string{"leather_armor"}},
		"full leather":            {armorKeys: []string{"leather_armor", "leather_helmet", "leather_pants"}},
		"full chain":              {armorKeys: []string{"chain_armor", "chain_helmet", "chain_pants"}},
		"full plate":              {armorKeys: []string{"iron_armor", "iron_helmet", "iron_pants"}},
		"full plate + bless":      {armorKeys: []string{"iron_armor", "iron_helmet", "iron_pants"}, bless: true},
		"full plate + stone":      {armorKeys: []string{"iron_armor", "iron_helmet", "iron_pants"}, stoneSkin: true},
		"full plate + both buffs": {armorKeys: []string{"iron_armor", "iron_helmet", "iron_pants"}, bless: true, stoneSkin: true},
	}

	type result struct {
		damageByRaw  map[int]int
		armorClass   int
		effectiveEnd int
	}
	results := make(map[string]result, len(tests))

	cast := func(t *testing.T, cs *CombatSystem, spellID spells.SpellID) {
		t.Helper()
		spellItem, err := spells.CreateSpellItem(spellID)
		if err != nil {
			t.Fatalf("create %s from spells.yaml: %v", spellID, err)
		}
		caster := cs.game.party.Members[0]
		if _, _, ok := caster.EquipItem(spellItem); !ok {
			t.Fatalf("equip %s", spellID)
		}
		caster.SpellPoints = 1000
		cs.game.selectedChar = 0
		if !cs.CastEquippedSpell() {
			t.Fatalf("cast %s through CastEquippedSpell", spellID)
		}
	}

	run := func(t *testing.T, tc scenario) result {
		t.Helper()
		cs := newTestCombatSystemWithConfig(t)
		game := cs.game
		target := game.party.Members[0]
		game.party.Members = []*character.MMCharacter{target}

		// Remove the temporary DisarmTrap combat placeholder so this test
		// isolates armor and the two requested spell effects.
		target.Endurance = 20
		delete(target.Skills, character.SkillDisarmTrap)
		target.Skills[character.SkillLeather] = &character.Skill{Mastery: character.MasteryNovice}
		target.Skills[character.SkillChain] = &character.Skill{Mastery: character.MasteryNovice}
		target.Skills[character.SkillPlate] = &character.Skill{Mastery: character.MasteryNovice}
		for _, slot := range []items.EquipSlot{
			items.SlotArmor, items.SlotHelmet, items.SlotBoots,
			items.SlotCloak, items.SlotGauntlets, items.SlotBelt, items.SlotOffHand,
		} {
			delete(target.Equipment, slot)
		}
		equipArmorPieces(t, target, tc.armorKeys...)

		if tc.bless {
			cast(t, cs, "bless")
			if _, ok := game.statBuffByID("bless"); !ok {
				t.Fatal("Bless cast did not register its real stat buff")
			}
		}
		if tc.stoneSkin {
			cast(t, cs, "stone_skin")
			buff, ok := game.combatBuffByID("stone_skin")
			if !ok || buff.InReduce != 4 {
				t.Fatalf("Stone Skin cast registered %+v (ok=%v), want incoming reduction 4", buff, ok)
			}
		}

		// Cancel any Luck supplied by Bless so Perfect Dodge is impossible.
		// Setting base Luck to zero is insufficient because combat uses the
		// effective stat (base + buffs).
		target.Luck = -target.BuffBonuses.Luck
		if _, chance := cs.RollPerfectDodge(target); chance != 0 {
			t.Fatalf("test target still has %d%% Perfect Dodge chance", chance)
		}
		target.MaxHitPoints = 1000
		target.HitPoints = 1000

		mob := monsterPkg.NewMonster3DFromConfig(game.camera.X+1, game.camera.Y, "minotaur", game.config)
		if mob.Key != "minotaur" || mob.DamageMin != 24 || mob.DamageMax != 38 {
			t.Fatalf("unexpected Minotaur loaded from monsters.yaml: key=%q damage=%d-%d",
				mob.Key, mob.DamageMin, mob.DamageMax)
		}
		game.world.Monsters = []*monsterPkg.Monster3D{mob}

		// Exercise every authored Minotaur damage roll through the real attack
		// path. Pinning each value in the YAML range removes RNG without replacing
		// the monster's actual 24..38 damage profile with an invented test value.
		damageByRaw := make(map[int]int, mob.DamageMax-mob.DamageMin+1)
		rawMin, rawMax := mob.DamageMin, mob.DamageMax
		for raw := rawMin; raw <= rawMax; raw++ {
			mob.DamageMin, mob.DamageMax = raw, raw
			mob.State = monsterPkg.StateAttacking
			mob.StateTimer = 1
			mob.AttackCDFrames = 0 // force a fresh hit each iteration — this test measures mitigation, not cadence
			target.HitPoints = 1000

			cs.HandleMonsterInteractions()
			damage := 1000 - target.HitPoints
			if damage <= 0 {
				t.Fatalf("real Minotaur attack with raw=%d dealt no damage", raw)
			}
			damageByRaw[raw] = damage
		}
		return result{
			damageByRaw:  damageByRaw,
			armorClass:   cs.CalculateTotalArmorClass(target),
			effectiveEnd: target.GetEffectiveEndurance(),
		}
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			results[name] = run(t, tc)
		})
	}

	for name, tc := range tests {
		got := results[name]
		for raw := 24; raw <= 38; raw++ {
			// Mirror mitigateCharacterDamage: armor % (capped 75) → flat buff → floor.
			mit := 100 * got.armorClass / (got.armorClass + ArmorMitigationK)
			if mit > ArmorPhysicalMitigationCap {
				mit = ArmorPhysicalMitigationCap
			}
			want := raw * (100 - mit) / 100
			if tc.stoneSkin {
				want -= 4
			}
			if want < 1 {
				want = 1
			}
			if damage := got.damageByRaw[raw]; damage != want {
				t.Errorf("%s: raw=%d dealt %d, want %d (AC=%d stone=%v)",
					name, raw, damage, want, got.armorClass, tc.stoneSkin)
			}
		}
	}

	if !(results["naked"].armorClass < results["one leather piece"].armorClass &&
		results["one leather piece"].armorClass < results["full leather"].armorClass &&
		results["full leather"].armorClass < results["full chain"].armorClass &&
		results["full chain"].armorClass < results["full plate"].armorClass) {
		t.Errorf("armor progression is not strictly increasing: naked=%d one-leather=%d leather=%d chain=%d plate=%d",
			results["naked"].armorClass,
			results["one leather piece"].armorClass,
			results["full leather"].armorClass,
			results["full chain"].armorClass,
			results["full plate"].armorClass)
	}
	if results["full plate + bless"].effectiveEnd != results["full plate"].effectiveEnd+5 {
		t.Errorf("Bless did not add 5 effective Endurance: plate=%d blessed=%d",
			results["full plate"].effectiveEnd, results["full plate + bless"].effectiveEnd)
	}
	if results["full plate + bless"].armorClass <= results["full plate"].armorClass {
		t.Errorf("Bless Endurance did not improve endurance-scaled plate AC: plate=%d blessed=%d",
			results["full plate"].armorClass, results["full plate + bless"].armorClass)
	}
	if results["full plate + stone"].damageByRaw[31] >= results["full plate"].damageByRaw[31] {
		t.Errorf("Stone Skin did not reduce the real Minotaur hit: plate=%d stone=%d",
			results["full plate"].damageByRaw[31], results["full plate + stone"].damageByRaw[31])
	}
	if results["full plate + both buffs"].damageByRaw[31] > results["full plate + stone"].damageByRaw[31] {
		t.Errorf("Bless + Stone Skin took more damage than Stone Skin alone: both=%d stone=%d",
			results["full plate + both buffs"].damageByRaw[31], results["full plate + stone"].damageByRaw[31])
	}
}

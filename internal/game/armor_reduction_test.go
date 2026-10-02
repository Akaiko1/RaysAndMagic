package game

// Armor mitigation tests - exercise the REAL CombatSystem path:
// CalculateTotalArmorClass -> armorMitigationPct (% with diminishing returns,
// capped 75% physical / 33% elemental) -> mitigateCharacterDamage pipeline
// (armor % -> resist % -> floor -> flat reductions; 100% resist = immune).

import (
	"fmt"
	"sort"
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
	for _, tc := range []struct {
		name      string
		endurance int // 0 keeps the starter's own; leather scales with END
		capped    bool
	}{
		{name: "below_cap"},
		// END/10 leather scaling: this AC lands far past the cap knee.
		{name: "past_cap", endurance: 100 * ArmorMitigationK, capped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			char := cs.game.party.Members[0]
			if tc.endurance > 0 {
				char.Endurance = tc.endurance
			}
			equipArmorPieces(t, char, "leather_armor")
			ac := cs.CalculateTotalArmorClass(char)
			if ac <= 0 {
				t.Fatalf("expected positive AC after leather armor, got %d", ac)
			}
			raw := 100 * ac / (ac + ArmorMitigationK)
			if (raw > ArmorPhysicalMitigationCap) != tc.capped {
				t.Fatalf("fixture AC %d gives raw %d%%, want capped=%v", ac, raw, tc.capped)
			}
			wantPhys := min(raw, ArmorPhysicalMitigationCap)
			// Elemental is the physical curve scaled to reach its cap with physical.
			wantElem := wantPhys * ArmorElementalMitigationCap / ArmorPhysicalMitigationCap
			if got := cs.armorMitigationPct(char, true); got != wantPhys {
				t.Errorf("physical mit: got %d%%, want %d%% (AC %d, K %d)", got, wantPhys, ac, ArmorMitigationK)
			}
			if got := cs.armorMitigationPct(char, false); got != wantElem {
				t.Errorf("elemental mit: got %d%%, want %d%% (scaled to cap %d)", got, wantElem, ArmorElementalMitigationCap)
			}
		})
	}
}

// This input makes every pipeline step observable: moving either percentage
// step, the floor, or the flat reduction changes the result. In particular the
// 1-damage floor applies before flat reductions, so a sufficiently strong flat
// ward may still reduce a non-immune hit to zero.
func TestMitigateCharacterDamage_ArmorResistFloorThenFlat(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	equipArmorPieces(t, char, "leather_armor")
	char.Equipment[items.SlotRing1] = items.Item{Attributes: map[string]int{"resist_physical": 50}}
	cs.game.combatBuffs = []TimedCombatBuff{{InReduce: 2}}

	const raw = 3
	armorPct := cs.armorMitigationPct(char, true)
	afterArmor := raw * (100 - armorPct) / 100
	afterResist := afterArmor * 50 / 100
	if afterResist < 1 {
		afterResist = 1
	}
	want := afterResist - 2
	if want < 0 {
		want = 0
	}

	if got := cs.mitigateCharacterDamage(raw, "physical", char, false); got != want {
		t.Fatalf("pipeline damage = %d, want %d (armor=%d%%, resist=50%%, flat=2)", got, want, armorPct)
	}
	if want != 0 {
		t.Fatalf("test setup must distinguish floor-before-flat, got expected %d", want)
	}
}

func TestMitigateCharacterDamage_FloorsAt1(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	// Capped armor (<=75%) can never fully negate without 100% resist, so a tiny
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

	// Measure AC after each piece is added - should grow strictly monotonically.
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
		t.Errorf("AC should grow with each armor piece, got %d -> %d -> %d -> %d",
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
		t.Errorf("per-slot contributions sum %d != total AC %d", sum, threeAC)
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
		damageByRaw    map[int]int
		rawMin, rawMax int
		armorClass     int
		effectiveEnd   int
		stoneReduce    int // Stone Skin's per-hit reduction for this caster
		blessBonus     int // Bless's stat bonus for this caster
	}
	results := make(map[string]result, len(tests))

	cast := func(t *testing.T, cs *CombatSystem, spellID spells.SpellID) {
		t.Helper()
		spellItem, err := spells.CreateSpellItem(spellID)
		if err != nil {
			t.Fatalf("create %s from spells.yaml: %v", spellID, err)
		}
		caster := cs.game.party.Members[0]
		caster.LearnSpell(spells.SpellID(spellID))
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

		// Disarm Trap carries its own flat reduction; drop it so this test
		// isolates armor and the two spell effects.
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

		blessBonus, stoneReduce := 0, 0
		if tc.bless {
			cast(t, cs, "bless")
			if _, ok := game.statBuffByID("bless"); !ok {
				t.Fatal("Bless cast did not register its real stat buff")
			}
			blessBonus = cs.CalculateSpellStatBonus("bless", target)
		}
		if tc.stoneSkin {
			cast(t, cs, "stone_skin")
			stoneDef, err := spells.GetSpellDefinitionByID("stone_skin")
			if err != nil {
				t.Fatalf("stone_skin: %v", err)
			}
			if stoneReduce = scaledIncomingDamageReduction(stoneDef, target); stoneReduce <= 0 {
				t.Fatal("Stone Skin authors no incoming reduction")
			}
			buff, ok := game.combatBuffByID("stone_skin")
			if !ok || buff.InReduce != stoneReduce {
				t.Fatalf("Stone Skin cast registered %+v (ok=%v), want incoming reduction %d", buff, ok, stoneReduce)
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

		mob := monsterPkg.NewMonster3DFromConfig(game.camera.X+float64(game.config.GetTileSize()), game.camera.Y, "minotaur", game.config)
		// Under test is the PHYSICAL armor curve; the minotaur's authored body
		// school (lower elemental cap) is covered by the melee-school tests.
		if mob.Key != "minotaur" || mob.DamageMin < 1 || mob.DamageMax < mob.DamageMin {
			t.Fatalf("unexpected Minotaur loaded from monsters.yaml: key=%q damage=%d-%d",
				mob.Key, mob.DamageMin, mob.DamageMax)
		}
		game.world.Monsters = []*monsterPkg.Monster3D{mob}

		// Exercise every authored Minotaur damage roll through the real attack
		// path. Pinning each value in the YAML range removes RNG without replacing
		// the monster's authored damage profile with an invented test value.
		damageByRaw := make(map[int]int, mob.DamageMax-mob.DamageMin+1)
		rawMin, rawMax := mob.DamageMin, mob.DamageMax
		for raw := rawMin; raw <= rawMax; raw++ {
			mob.DamageMin, mob.DamageMax = raw, raw
			mob.State = monsterPkg.StateAttacking
			mob.StateTimer = 1
			mob.AttackCDFrames = 0 // force a fresh hit each iteration - this test measures mitigation, not cadence
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
			rawMin:       rawMin,
			rawMax:       rawMax,
			armorClass:   cs.CalculateTotalArmorClass(target),
			effectiveEnd: target.GetEffectiveEndurance(),
			stoneReduce:  stoneReduce,
			blessBonus:   blessBonus,
		}
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			results[name] = run(t, tc)
		})
	}

	for name, tc := range tests {
		got := results[name]
		for raw := got.rawMin; raw <= got.rawMax; raw++ {
			// Mirror mitigateCharacterDamage: armor % (capped 75) -> floor -> flat buff.
			mit := 100 * got.armorClass / (got.armorClass + ArmorMitigationK)
			if mit > ArmorPhysicalMitigationCap {
				mit = ArmorPhysicalMitigationCap
			}
			want := raw * (100 - mit) / 100
			if want < 1 {
				want = 1
			}
			if tc.stoneSkin {
				want -= got.stoneReduce
			}
			if want < 0 {
				want = 0
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
	blessed := results["full plate + bless"]
	if blessed.blessBonus <= 0 || blessed.effectiveEnd != results["full plate"].effectiveEnd+blessed.blessBonus {
		t.Errorf("Bless did not add its %d effective Endurance: plate=%d blessed=%d",
			blessed.blessBonus, results["full plate"].effectiveEnd, blessed.effectiveEnd)
	}
	if results["full plate + bless"].armorClass <= results["full plate"].armorClass {
		t.Errorf("Bless Endurance did not improve endurance-scaled plate AC: plate=%d blessed=%d",
			results["full plate"].armorClass, results["full plate + bless"].armorClass)
	}
	plate, stone, both := results["full plate"], results["full plate + stone"], results["full plate + both buffs"]
	for raw := plate.rawMin; raw <= plate.rawMax; raw++ {
		if stone.damageByRaw[raw] >= plate.damageByRaw[raw] {
			t.Errorf("raw=%d: Stone Skin did not reduce the real Minotaur hit: plate=%d stone=%d",
				raw, plate.damageByRaw[raw], stone.damageByRaw[raw])
		}
		if both.damageByRaw[raw] > stone.damageByRaw[raw] {
			t.Errorf("raw=%d: Bless + Stone Skin took more damage than Stone Skin alone: both=%d stone=%d",
				raw, both.damageByRaw[raw], stone.damageByRaw[raw])
		}
	}
}

// TestMonsterDamageVsArmorTiers reports, for EVERY current monster, the physical
// melee damage it deals to a level-6 / 20-Endurance character wearing a full set
// of each armor tier - leather, chain, plate. Each monster's min..max is run
// through the real percentage armor path (armorMitigationPctFromAC: physical%
// = min(75, 100*AC/(AC+K)), floored at 1). Output is a table (run with
// -run MonsterDamageVsArmorTiers -v); it also asserts AC rises leather<chain<plate,
// heavier armor never takes MORE, and the 1-damage floor holds.
func TestMonsterDamageVsArmorTiers(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	armorSlots := []items.EquipSlot{
		items.SlotArmor, items.SlotHelmet, items.SlotBoots,
		items.SlotCloak, items.SlotGauntlets, items.SlotBelt,
	}
	sets := []struct {
		name   string
		pieces map[items.EquipSlot]string
	}{
		{"leather", map[items.EquipSlot]string{items.SlotArmor: "leather_armor", items.SlotHelmet: "leather_helmet", items.SlotBoots: "leather_pants"}},
		{"chain", map[items.EquipSlot]string{items.SlotArmor: "chain_armor", items.SlotHelmet: "chain_helmet", items.SlotBoots: "chain_pants"}},
		{"plate", map[items.EquipSlot]string{items.SlotArmor: "iron_armor", items.SlotHelmet: "iron_helmet", items.SlotBoots: "iron_pants"}},
	}

	// A L6/20-END tank wearing ONLY the given set (other armor slots cleared so AC
	// is purely that set).
	newTank := func(pieces map[items.EquipSlot]string) *character.MMCharacter {
		c := character.CreateCharacter("Tank", character.ClassKnight, cs.game.config)
		delete(c.Skills, character.SkillImpenetrableDefense) // this matrix isolates armor tiers
		c.Level = 6
		c.Endurance = 20
		for _, s := range armorSlots {
			delete(c.Equipment, s)
		}
		for slot, key := range pieces {
			c.Equipment[slot] = items.CreateItemFromYAML(key)
		}
		return c
	}

	tanks := map[string]*character.MMCharacter{}
	ac := map[string]int{}
	for _, s := range sets {
		tanks[s.name] = newTank(s.pieces)
		ac[s.name] = cs.CalculateTotalArmorClass(tanks[s.name])
	}
	t.Logf("L6 Knight, 20-END (incl. class armor mastery) total AC - leather:%d  chain:%d  plate:%d (phys mitigation leather:%d%% chain:%d%% plate:%d%%)",
		ac["leather"], ac["chain"], ac["plate"],
		armorMitigationPctFromAC(ac["leather"], true), armorMitigationPctFromAC(ac["chain"], true), armorMitigationPctFromAC(ac["plate"], true))
	if !(ac["leather"] < ac["chain"] && ac["chain"] < ac["plate"]) {
		t.Errorf("AC should rise leather<chain<plate, got %d/%d/%d", ac["leather"], ac["chain"], ac["plate"])
	}

	// Build every monster, then sort by level (then name) so the table reads from
	// weakest to toughest.
	mobs := make([]*monsterPkg.Monster3D, 0, len(monsterPkg.MonsterConfig.Monsters))
	for k := range monsterPkg.MonsterConfig.Monsters {
		mobs = append(mobs, monsterPkg.NewMonster3DFromConfig(0, 0, k, cs.game.config))
	}
	sort.Slice(mobs, func(i, j int) bool {
		if mobs[i].Level != mobs[j].Level {
			return mobs[i].Level < mobs[j].Level
		}
		return mobs[i].Name < mobs[j].Name
	})

	t.Logf("%-3s %-22s %-9s %-11s %-11s %-11s", "lvl", "monster", "raw", "leather", "chain", "plate")
	rng := func(set string, m *monsterPkg.Monster3D) (int, int) {
		return cs.mitigateCharacterDamage(m.DamageMin, "physical", tanks[set], false), cs.mitigateCharacterDamage(m.DamageMax, "physical", tanks[set], false)
	}
	for _, m := range mobs {
		lMin, lMax := rng("leather", m)
		cMin, cMax := rng("chain", m)
		pMin, pMax := rng("plate", m)
		t.Logf("%-3d %-22s %-9s %-11s %-11s %-11s", m.Level, m.Name,
			fmt.Sprintf("%d-%d", m.DamageMin, m.DamageMax),
			fmt.Sprintf("%d-%d", lMin, lMax),
			fmt.Sprintf("%d-%d", cMin, cMax),
			fmt.Sprintf("%d-%d", pMin, pMax))
		// Heavier armor must never take MORE damage; floor never below 1.
		if lMax < cMax || cMax < pMax {
			t.Errorf("%s: heavier armor took more (leather %d, chain %d, plate %d)", m.Name, lMax, cMax, pMax)
		}
		// Anything that actually deals damage must chip at least 1 through armor;
		// passive 0-damage entries (e.g. the Warlord Idol) legitimately stay 0.
		if m.DamageMin >= 1 && pMin < 1 {
			t.Errorf("%s: damage floor dropped below 1 (got %d)", m.Name, pMin)
		}
	}
}

// The Golden Thief Bug Carapace grants +100% resist to all non-physical schools
// (full immunity) to its wearer, while physical damage still goes through armor.
// Exercises the resist_nonphysical attribute end-to-end (config->bridge->item->
// character) and the mitigateCharacterDamage chokepoint.
func TestGoldenCarapace_NonPhysicalImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]

	// Baseline: no resist gear -> non-physical passes through untouched.
	if got := cs.mitigateCharacterDamage(40, "fire", member, false); got != 40 {
		t.Errorf("no resist gear: fire 40 should pass, got %d", got)
	}

	member.Equipment[items.SlotArmor] = items.CreateItemFromYAML("golden_thiefbug_carapace")
	if got := member.GearResistPct("fire"); got != 100 {
		t.Fatalf("carapace must grant 100%% fire (per-element) resist, got %d", got)
	}
	for _, school := range []string{"fire", "water", "air", "earth", "body", "mind", "spirit", "dark", "light"} {
		if got := cs.mitigateCharacterDamage(40, school, member, false); got != 0 {
			t.Errorf("%s 40 should be fully resisted by the carapace, got %d", school, got)
		}
	}
	// Physical is NOT in the carapace's resistances -> still reduced by armor only (>=1).
	if got := cs.mitigateCharacterDamage(40, "physical", member, false); got < 1 {
		t.Errorf("physical must still apply via armor, got %d", got)
	}
}

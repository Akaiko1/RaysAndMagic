package game

import (
	"testing"

	"ugataima/internal/world"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// endurance_scaling_divisor is the armor AC formula, NOT a stat bonus: wearing
// armor must not raise effective Endurance, and armor pieces must not inflate
// each other's AC through it.
func TestEnduranceDivisor_ACOnly_NoStatFeedback(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0] // knight: leather skill from kit
	char.Equipment = map[items.EquipSlot]items.Item{}
	baseEnd := char.Endurance

	equipArmorPieces(t, char, "leather_armor")
	if _, _, _, eff, _, _, _ := char.GetEffectiveStats(); eff != baseEnd {
		t.Errorf("armor raised effective Endurance %d -> %d; divisor must be AC-only", baseEnd, eff)
	}

	// AC of the piece = base + effectiveEnd/div, where effectiveEnd has no
	// divisor feedback: leather scales category-wide as END/10.
	wantAC := 2 + cs.armorMasteryBonus(char, char.Equipment[items.SlotArmor]) + baseEnd/10
	if got := cs.CalculateTotalArmorClass(char); got != wantAC {
		t.Errorf("AC = %d, want %d (no divisor feedback)", got, wantAC)
	}
}

// Flat equipment stat bonuses flow into MaxHP/MaxSP, but equip changes are
// REVERSIBLE: the maxima move, current values never grow — an equip/unequip
// cycle must net exactly zero (the old +delta grant was an infinite-heal pump).
func TestEquipmentStats_DriveMaxHPandSP(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	char.Equipment = map[items.EquipSlot]items.Item{}
	char.RecalculateMaxStatsKeepingCurrent(cs.game.config)
	char.HitPoints = char.MaxHitPoints / 2 // wounded: no free healing allowed
	hpBefore, maxBefore := char.HitPoints, char.MaxHitPoints

	endurMult := cs.game.config.Characters.HitPoints.EnduranceMultiplier
	belt := items.Item{
		Name: "Test Girdle", Type: items.ItemArmor, ArmorCategory: "cloth",
		Attributes: map[string]int{"bonus_endurance": 5, "equip_slot": int(items.SlotBelt)},
	}
	if _, _, ok := char.EquipItem(belt); !ok {
		t.Fatal("equip failed")
	}
	wantGain := 5 * endurMult
	if char.MaxHitPoints != maxBefore+wantGain {
		t.Errorf("MaxHP %d -> %d, want +%d from +5 Endurance", maxBefore, char.MaxHitPoints, wantGain)
	}
	if char.HitPoints != hpBefore {
		t.Errorf("current HP must not grow on equip (pump exploit): %d -> %d", hpBefore, char.HitPoints)
	}

	// Unequip: max drops back, current capped — the full cycle nets zero.
	char.UnequipItem(items.SlotBelt)
	if char.MaxHitPoints != maxBefore || char.HitPoints != hpBefore {
		t.Errorf("after unequip = %d/%d, want %d/%d (no pump)", char.HitPoints, char.MaxHitPoints, hpBefore, maxBefore)
	}

	// bonus_intellect now raises MaxSP too.
	spMaxBefore := char.MaxSpellPoints
	ring := items.Item{
		Name: "Test Loop", Type: items.ItemAccessory,
		Attributes: map[string]int{"bonus_intellect": 4, "equip_slot": int(items.SlotRing1)},
	}
	if _, _, ok := char.EquipItem(ring); !ok {
		t.Fatal("equip ring failed")
	}
	if char.MaxSpellPoints != spMaxBefore+4 {
		t.Errorf("MaxSP %d -> %d, want +4 from bonus_intellect", spMaxBefore, char.MaxSpellPoints)
	}
}

// Bless raises MaxHP/MaxSP while active and removes the gain on expiry
// (current values capped, never refunded).
func TestBless_SwellsAndShrinksMaxima(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	char := g.party.Members[0]
	maxHPBefore, maxSPBefore := char.MaxHitPoints, char.MaxSpellPoints
	endurMult := g.config.Characters.HitPoints.EnduranceMultiplier

	cs.applyStatBuffSpell("bless", 600, character.UniformStatBonuses(10))
	if char.MaxHitPoints != maxHPBefore+10*endurMult {
		t.Errorf("blessed MaxHP = %d, want %d", char.MaxHitPoints, maxHPBefore+10*endurMult)
	}
	wantSP := maxSPBefore + 10 + 10/character.MaxSPPersonalityDivisor
	if char.MaxSpellPoints != wantSP {
		t.Errorf("blessed MaxSP = %d, want %d", char.MaxSpellPoints, wantSP)
	}

	// Expire through the registry tick (the real path).
	if b, ok := g.statBuffByID("bless"); !ok || b.Frames != 600 {
		t.Fatalf("bless not registered: %+v ok=%v", b, ok)
	}
	for i := 0; i < 600; i++ {
		g.tickStatBuffs()
	}
	if char.MaxHitPoints != maxHPBefore || char.MaxSpellPoints != maxSPBefore {
		t.Errorf("after expiry max = %d/%d, want %d/%d", char.MaxHitPoints, char.MaxSpellPoints, maxHPBefore, maxSPBefore)
	}
	if char.HitPoints > char.MaxHitPoints || char.SpellPoints > char.MaxSpellPoints {
		t.Error("current HP/SP must be capped at the shrunken max")
	}
}

// Per-stat spell buffs: a stat_bonuses map applies exactly the authored stats.
func TestPerStatSpellBuff_AppliesOnlyAuthoredStats(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	baseMight, baseLuck := char.Might, char.Luck

	cs.applyStatBuffSpell("test_might_buff", 600, character.StatBonusesFromMap(map[string]int{"might": 15}))
	might, _, _, _, _, _, luck := char.GetEffectiveStats()
	if might != baseMight+15 {
		t.Errorf("might = %d, want %d", might, baseMight+15)
	}
	if luck != baseLuck {
		t.Errorf("luck = %d, want %d (untouched)", luck, baseLuck)
	}
}

// Spending a stat point (irreversible) DOES grant the gained HP immediately.
func TestStatPointSpend_GrantsGain(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	char.HitPoints = char.MaxHitPoints / 2
	hpBefore, maxBefore := char.HitPoints, char.MaxHitPoints
	endurMult := cs.game.config.Characters.HitPoints.EnduranceMultiplier

	char.Endurance++
	char.RecalculateMaxStatsGrantingGain(cs.game.config)
	if char.MaxHitPoints != maxBefore+endurMult || char.HitPoints != hpBefore+endurMult {
		t.Errorf("spend +1 End: %d/%d, want %d/%d", char.HitPoints, char.MaxHitPoints, hpBefore+endurMult, maxBefore+endurMult)
	}
}

// A tavern swap moves the active-party buff: the incoming hero picks up Bless,
// the benched one sheds it (no frozen bench buffs, no unbuffed newcomers).
func TestTavernSwap_SyncsBuffBonuses(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	bench := character.CreateCharacter("Benchy", character.ClassKnight, g.config)
	g.party.Recruit(bench)
	benchIdx := len(g.party.Reserve) - 1

	cs.applyStatBuffSpell("bless", 600, character.UniformStatBonuses(10))
	outgoing := g.party.Members[0]
	if !g.swapRosterMember(0, benchIdx) {
		t.Fatal("swap failed")
	}
	if g.party.Members[0].BuffBonuses != g.statBonuses {
		t.Errorf("incoming hero missing the active buff: %+v", g.party.Members[0].BuffBonuses)
	}
	if !outgoing.BuffBonuses.IsZero() {
		t.Errorf("benched hero kept the buff: %+v", outgoing.BuffBonuses)
	}
}

// Different stat-buff spells STACK (with each other and with gear); recasting
// the same spell refreshes its entry instead of double-stacking.
func TestStatBuffs_StackAcrossSpellsAndGear(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	char := g.party.Members[0]
	char.Equipment = map[items.EquipSlot]items.Item{}
	baseMight := char.Might

	ring := items.Item{
		Name: "Mighty Loop", Type: items.ItemAccessory,
		Attributes: map[string]int{"bonus_might": 2, "equip_slot": int(items.SlotRing1)},
	}
	if _, _, ok := char.EquipItem(ring); !ok {
		t.Fatal("equip failed")
	}
	cs.applyStatBuffSpell("bless", 600, character.UniformStatBonuses(10))
	cs.applyStatBuffSpell("war_chant", 600, character.StatBonusesFromMap(map[string]int{"might": 5}))

	might, _, _, _, _, _, _ := char.GetEffectiveStats()
	if want := baseMight + 2 + 10 + 5; might != want {
		t.Errorf("gear+bless+chant might = %d, want %d (all three stack)", might, want)
	}

	// Recast bless: refresh, not double-stack.
	cs.applyStatBuffSpell("bless", 900, character.UniformStatBonuses(10))
	might, _, _, _, _, _, _ = char.GetEffectiveStats()
	if want := baseMight + 2 + 10 + 5; might != want {
		t.Errorf("after bless recast might = %d, want %d (refresh, no double-stack)", might, want)
	}
	if b, _ := g.statBuffByID("bless"); b.Frames != 900 {
		t.Errorf("recast should refresh duration: %d, want 900", b.Frames)
	}

	// One expires → only its share is removed.
	g.removeStatBuff("war_chant")
	might, _, _, _, _, _, _ = char.GetEffectiveStats()
	if want := baseMight + 2 + 10; might != want {
		t.Errorf("after chant dispel might = %d, want %d", might, want)
	}
}

// A save's benched heroes re-derive MaxHP/MaxSP under the CURRENT formula on
// load — stale pre-rebalance maxima must not survive on the bench.
func TestLoad_RecalcsReserveMaxima(t *testing.T) {
	cfg := loadTestConfig(t)

	wmSave := world.NewWorldManager(cfg)
	worldSave := newTestWorld(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": worldSave}
	wmSave.CurrentMapKey = "forest"
	g := newTestGame(cfg, worldSave)

	bench := character.CreateCharacter("Benchy", character.ClassSorcerer, cfg)
	g.party.Recruit(bench)
	save := g.buildSave(wmSave)

	// Simulate a pre-rebalance save: corrupt the benched hero's stored maxima.
	for i := range save.Party.Reserve {
		if save.Party.Reserve[i].Name == "Benchy" {
			save.Party.Reserve[i].MaxSpellPoints = 1
			save.Party.Reserve[i].MaxHitPoints = 1
		}
	}

	wmLoad := world.NewWorldManager(cfg)
	worldLoad := newTestWorld(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": worldLoad}
	wmLoad.CurrentMapKey = "forest"
	prev := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	t.Cleanup(func() { world.GlobalWorldManager = prev })

	loaded := newTestGame(cfg, worldLoad)
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	for _, m := range loaded.party.Reserve {
		if m.Name != "Benchy" {
			continue
		}
		if m.MaxSpellPoints == 1 || m.MaxHitPoints == 1 {
			t.Errorf("benched maxima not re-derived on load: HP %d SP %d", m.MaxHitPoints, m.MaxSpellPoints)
		}
		return
	}
	t.Fatal("Benchy missing from loaded reserve")
}

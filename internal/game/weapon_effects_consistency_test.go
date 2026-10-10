package game

// Every weapon special effect authored in weapons.yaml must reach the cards the
// game really renders: the item tooltip (shop, bearer and editor) and the
// equipment comparison's Gain/Lose rows.

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Each authored effect field maps to the row it must produce on the card.
func TestWeaponTooltipSurfacesEveryAuthoredEffect(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	cs.game.party.Members = []*character.MMCharacter{ch}
	exercised := map[string]bool{}
	for _, key := range slices.Sorted(maps.Keys(config.GlobalWeapons.Weapons)) {
		def := config.GlobalWeapons.Weapons[key]
		var want []string
		add := func(field string, on bool, lines ...string) {
			if on {
				exercised[field] = true
				want = append(want, lines...)
			}
		}
		// Stun spells out RT seconds and TB turns; an unset stun_turns stuns one turn.
		turns := max(1, def.StunTurns)
		add("stun", def.StunChance > 0, fmt.Sprintf("Stun Chance: %.0f%% (%ds RT / %d turns TB)", def.StunChance*100, turns, turns))
		add("disintegrate", def.DisintegrateChance > 0, fmt.Sprintf("Disintegrate Chance: %.0f%% (undead and dragons immune)", def.DisintegrateChance*100))
		add("spell_cooldown", def.SpellCooldownMultiplier > 0 && def.SpellCooldownMultiplier != 1,
			fmt.Sprintf("Spell cooldown %+.0f%%", (def.SpellCooldownMultiplier-1)*100))
		for _, vs := range slices.Sorted(maps.Keys(def.BonusVs)) {
			add("bonus_vs", true, fmt.Sprintf("Bonus vs %s: x%.1f", config.TitleWords(vs), def.BonusVs[vs]))
		}
		add("damage_type", def.DamageType != "" && def.DamageType != "physical", character.DamageTypeAoELine(def.DamageType, def.AoeRadiusTiles))
		add("aoe", def.AoeRadiusTiles > 0, character.DamageTypeAoELine(def.DamageType, def.AoeRadiusTiles))
		want = append(want, def.CoreEffectLines()...)
		for _, bearer := range []*character.MMCharacter{nil, ch} {
			for _, full := range []bool{false, true} {
				it := items.CreateWeaponFromYAML(key)
				ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: it}
				card := GetItemTooltipRows(it, bearer, cs, full).String()
				rows := want
				if full && def.MaxProjectiles > 0 {
					exercised["max_projectiles"] = true
					rows = append(slices.Clone(want), fmt.Sprintf("Maximum Projectiles: %d", def.MaxProjectiles))
				}
				for _, row := range rows {
					if !strings.Contains(card, row) {
						t.Errorf("%s bearer=%v full=%v: missing %q:\n%s", key, bearer != nil, full, row, card)
					}
				}
			}
		}
	}
	for _, field := range []string{"stun", "disintegrate", "spell_cooldown", "bonus_vs", "damage_type", "aoe", "max_projectiles"} {
		if !exercised[field] {
			t.Errorf("no authored weapon exercises the %s row; the case is untested", field)
		}
	}
}

// The comparison panel lists effects as Gain/Lose rows; set membership has its
// own Set activated/lost rows (TestEquipmentComparisonCompletedSets).
func TestWeaponComparisonGainsAndLosesEveryEffect(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	comparisonTestHero(ch)
	delete(ch.Skills, character.SkillDualWielding)
	keys := slices.Sorted(maps.Keys(config.GlobalWeapons.Weapons))
	effects := func(key string) []string {
		def := config.GlobalWeapons.Weapons[key]
		return append(def.SpecialEffectLines(), character.WeaponCombatLines(def)...)
	}
	plainAt := slices.IndexFunc(keys, func(key string) bool { return len(effects(key)) == 0 })
	if plainAt < 0 {
		t.Fatal("no authored weapon without special effects to compare against")
	}
	plain := items.CreateWeaponFromYAML(keys[plainAt])
	checked := 0
	for _, key := range keys {
		lines := effects(key)
		if len(lines) == 0 {
			continue
		}
		checked++
		it := items.CreateWeaponFromYAML(key)
		ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: plain}
		gain := GetItemComparisonTooltipRows(it, ch, cs).String()
		ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: it}
		lose := GetItemComparisonTooltipRows(plain, ch, cs).String()
		for _, line := range lines {
			if !strings.Contains(gain, "Gain: "+line) {
				t.Errorf("%s: comparison hides gained %q:\n%s", key, line, gain)
			}
			if !strings.Contains(lose, "Lose: "+line) {
				t.Errorf("%s: comparison hides lost %q:\n%s", key, line, lose)
			}
		}
		// The same weapon on both sides gains and loses nothing.
		if same := GetItemComparisonTooltipRows(items.CreateWeaponFromYAML(key), ch, cs).String(); strings.Contains(same, "Gain: ") || strings.Contains(same, "Lose: ") {
			t.Errorf("%s: identical weapon reports effect changes:\n%s", key, same)
		}
	}
	if checked == 0 {
		t.Fatal("no authored weapon has special effects")
	}
}

// BonusVs is a map: its rows must come out in sorted key order every call.
func TestWeaponBonusVsLinesAreSorted(t *testing.T) {
	def := &config.WeaponDefinitionConfig{BonusVs: map[string]float64{"undead": 1.5, "dragon": 2, "beast": 1.25, "giant": 3}}
	var want []string
	for _, k := range []string{"beast", "dragon", "giant", "undead"} {
		want = append(want, fmt.Sprintf("Bonus vs %s: x%.1f", config.TitleWords(k), def.BonusVs[k]))
	}
	for i := 0; i < 50; i++ {
		if got := def.CoreEffectLines(); !slices.Equal(got, want) {
			t.Fatalf("call %d: got %q, want %q", i, got, want)
		}
		if got := def.SpecialEffectLines(); !slices.Equal(got, want) {
			t.Fatalf("call %d comparison rows: got %q, want %q", i, got, want)
		}
	}
}

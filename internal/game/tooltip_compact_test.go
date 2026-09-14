package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// The compact/full contract (user-designed unified tooltip): compact hides the
// Base->Stat->Mastery decomposition and the universal RULES, but keeps the
// totals, cost/cooldown and per-item state (requirements, locks). Full reveals
// the breakdown, in builder order (decomposition BEFORE the total). The
// "[Shift] full breakdown" hint shows only in compact when detail exists.

// indexOf returns the line index of the first line containing sub, or -1.
func ttIndexOf(tip, sub string) int {
	for i, ln := range strings.Split(tip, "\n") {
		if strings.Contains(ln, sub) {
			return i
		}
	}
	return -1
}

func TestTooltipCompact_WeaponHidesBreakdownKeepsTotals(t *testing.T) {
	g, thief := newThiefTestGame(t)
	w := thief.Equipment[items.SlotMainHand] // magic dagger

	compact := GetItemTooltip(w, thief, g.combat, false)
	full := GetItemTooltip(w, thief, g.combat, true)

	// Compact keeps the decision-relevant lines.
	for _, want := range []string{"Total Damage:", "Cooldown:", "[Shift] full breakdown"} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact weapon must contain %q:\n%s", want, compact)
		}
	}
	// Compact hides the decomposition + universal RULES.
	for _, hidden := range []string{"Base:", "Normal Damage:", "Reduced by target Armor", "RULES"} {
		if strings.Contains(compact, hidden) {
			t.Errorf("compact weapon must NOT contain %q:\n%s", hidden, compact)
		}
	}
	// Full reveals them, and the hint is gone.
	for _, want := range []string{"Base:", "Normal Damage:", "Reduced by target Armor"} {
		if !strings.Contains(full, want) {
			t.Errorf("full weapon must contain %q:\n%s", want, full)
		}
	}
	if strings.Contains(full, "[Shift]") {
		t.Errorf("full view must not show the Shift hint:\n%s", full)
	}
	// Ordering: decomposition BEFORE the total (the round-1 bug).
	if base, total := ttIndexOf(full, "Base:"), ttIndexOf(full, "Total Damage:"); base < 0 || total < 0 || base >= total {
		t.Errorf("full weapon DAMAGE must read Base->...->Total (base=%d total=%d):\n%s", base, total, full)
	}
}

func TestWeaponTooltipFullBreakdownListsOnlyActiveFactors(t *testing.T) {
	type setupFunc func(*CombatSystem, *character.MMCharacter)
	fury := func(mastery character.SkillMastery) setupFunc {
		return func(_ *CombatSystem, char *character.MMCharacter) {
			char.Skills[character.SkillOrcishFury] = &character.Skill{Mastery: mastery}
		}
	}
	dualWielding := func(mastery character.SkillMastery) setupFunc {
		return func(_ *CombatSystem, char *character.MMCharacter) {
			char.Skills[character.SkillDualWielding] = &character.Skill{Mastery: mastery}
		}
	}
	tests := []struct {
		name      string
		weaponKey string
		shop      bool
		setup     setupFunc
		want      []string
		wantOnce  []string
		absent    []string
		order     []string
	}{
		{
			name: "baseline names Speed but omits inactive factors", weaponKey: "iron_sword",
			setup: func(_ *CombatSystem, char *character.MMCharacter) {
				delete(char.Skills, character.SkillOrcishFury)
				delete(char.Skills, character.SkillDualWielding)
			},
			want:   []string{"Speed ("},
			absent: []string{"Orcish Fury -", "Dual Wielding -", "Safety clamp:", "Capped at 100%"},
		},
		{
			name: "shop omits every bearer factor", weaponKey: "iron_sword", shop: true,
			absent: []string{"Attack cooldown x", "Speed (", "Orcish Fury -", "Dual Wielding -", "Safety clamp:", "Capped at 100%"},
		},
		{
			name: "shop keeps category cooldown property", weaponKey: "hunting_bow", shop: true,
			want:     []string{"Attack cooldown x1.20 (20% slower than standard)"},
			wantOnce: []string{"Attack cooldown x1.20"},
			absent:   []string{"Speed (", "Dual Wielding -", "Safety clamp:", "RT Cooldown:"},
		},
		{
			name: "shop keeps authored cooldown override", weaponKey: "suppressor_gun", shop: true,
			want:     []string{"Attack cooldown x0.40 (60% faster than standard)"},
			wantOnce: []string{"Attack cooldown x0.40"},
			absent:   []string{"Speed (", "Dual Wielding -", "Safety clamp:", "RT Cooldown:"},
		},
		{
			name: "equipped card keeps one category cooldown property", weaponKey: "hunting_bow",
			want:     []string{"Speed (", "Attack cooldown x1.20 (20% slower than standard)", "RT Cooldown:"},
			wantOnce: []string{"Attack cooldown x1.20"},
		},
		{name: "Fury Novice", weaponKey: "iron_sword", setup: fury(character.MasteryNovice), want: []string{"Orcish Fury - Novice: +3"}},
		{name: "Fury Expert", weaponKey: "iron_sword", setup: fury(character.MasteryExpert), want: []string{"Orcish Fury - Expert: +5"}},
		{name: "Fury Master", weaponKey: "iron_sword", setup: fury(character.MasteryMaster), want: []string{"Orcish Fury - Master: +7"}},
		{name: "Fury Grandmaster", weaponKey: "iron_sword", setup: fury(character.MasteryGrandMaster), want: []string{"Orcish Fury - Grandmaster: +10"}},
		{
			name: "Dual Wielding Novice has no cooldown bonus", weaponKey: "iron_sword",
			setup: dualWielding(character.MasteryNovice), absent: []string{"Dual Wielding -"},
		},
		{
			name: "Dual Wielding Expert", weaponKey: "iron_sword", setup: dualWielding(character.MasteryExpert),
			want: []string{"Dual Wielding - Expert: -10% cooldown"},
		},
		{
			name: "Dual Wielding Master", weaponKey: "iron_sword", setup: dualWielding(character.MasteryMaster),
			want: []string{"Dual Wielding - Master: -20% cooldown"},
		},
		{
			name: "Dual Wielding Grandmaster", weaponKey: "iron_sword", setup: dualWielding(character.MasteryGrandMaster),
			want: []string{"Dual Wielding - Grandmaster: -30% cooldown"},
		},
		{
			name: "cooldown safety floor is named only when active", weaponKey: "suppressor_gun",
			setup: func(_ *CombatSystem, char *character.MMCharacter) {
				char.Speed = int(AttackCooldownCapSpeed)
				char.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryGrandMaster}
			},
			want:     []string{"Attack cooldown x0.40 (60% faster than standard)", "Safety clamp: 0.1s"},
			wantOnce: []string{"Attack cooldown x0.40"},
		},
		{
			name: "critical cap is named only when active", weaponKey: "iron_sword",
			setup: func(_ *CombatSystem, char *character.MMCharacter) {
				char.Luck = 1000
			},
			want: []string{"Chance: 100%", "Capped at 100%"},
		},
		{
			name: "melee buff precedes card multiplier", weaponKey: "iron_sword",
			setup: func(cs *CombatSystem, _ *character.MMCharacter) {
				cs.game.cardSlots[0].key = "masked_serpent_dancer_card"
				cs.game.addCombatBuff(TimedCombatBuff{SpellID: "test", Frames: 60, OutBonus: 5, OutDamageType: "all"})
			},
			want:  []string{"Active party buff: +5", "Cards: +20% melee damage"},
			order: []string{"Active party buff: +5", "Cards: +20% melee damage", "Normal Damage:"},
		},
		{
			name: "ranged card multiplier precedes buff", weaponKey: "hunting_bow",
			setup: func(cs *CombatSystem, _ *character.MMCharacter) {
				cs.game.cardSlots[0].key = "masked_huntress_card"
				cs.game.addCombatBuff(TimedCombatBuff{SpellID: "test", Frames: 60, OutBonus: 5, OutDamageType: "all"})
			},
			want:  []string{"Cards: +20% ranged damage", "Active party buff: +5"},
			order: []string{"Cards: +20% ranged damage", "Active party buff: +5", "Normal Damage:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			char := cs.game.party.Members[0]
			delete(char.Skills, character.SkillOrcishFury)
			delete(char.Skills, character.SkillDualWielding)
			if tt.setup != nil {
				tt.setup(cs, char)
			}
			weapon, err := items.TryCreateWeaponFromYAML(tt.weaponKey)
			if err != nil {
				t.Fatalf("create %s: %v", tt.weaponKey, err)
			}
			bearer := char
			if tt.shop {
				bearer = nil
			}
			full := GetItemTooltip(weapon, bearer, cs, true)
			compact := GetItemTooltip(weapon, bearer, cs, false)

			for _, want := range tt.want {
				if !strings.Contains(full, want) {
					t.Errorf("full tooltip missing %q:\n%s", want, full)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(full, absent) {
					t.Errorf("full tooltip contains inactive factor %q:\n%s", absent, full)
				}
			}
			for _, wantOnce := range tt.wantOnce {
				if count := strings.Count(full, wantOnce); count != 1 {
					t.Errorf("full tooltip contains %q %d times, want exactly once:\n%s", wantOnce, count, full)
				}
			}
			for _, detail := range []string{"Attack cooldown x", "Speed (", "Orcish Fury -", "Dual Wielding -", "Safety clamp:", "Capped at 100%"} {
				if strings.Contains(compact, detail) {
					t.Errorf("compact tooltip contains detail factor %q:\n%s", detail, compact)
				}
			}
			last := -1
			for _, part := range tt.order {
				idx := ttIndexOf(full, part)
				if idx < 0 || idx <= last {
					t.Errorf("factor order %q after line %d failed (line %d):\n%s", part, last, idx, full)
					break
				}
				last = idx
			}
			if tt.shop && strings.Contains(full, "RT Cooldown:") {
				t.Errorf("shop tooltip unexpectedly contains bearer cooldown:\n%s", full)
			}
		})
	}
}

func TestTooltipCompact_ArmorRequirementAndOrder(t *testing.T) {
	g, thief := newThiefTestGame(t) // thief lacks Plate skill
	plate, err := items.TryCreateItemFromYAML("iron_armor")
	if err != nil {
		t.Fatalf("iron_armor: %v", err)
	}
	compact := GetItemTooltip(plate, thief, g.combat, false)
	full := GetItemTooltip(plate, thief, g.combat, true)

	// The equip requirement is per-item state -> visible in COMPACT.
	if !strings.Contains(compact, "Requires: Plate Skill") {
		t.Errorf("compact armor must show the equip requirement:\n%s", compact)
	}
	if !strings.Contains(compact, "Total Armor Class:") {
		t.Errorf("compact armor must show Total Armor Class:\n%s", compact)
	}
	// The AC decomposition is detail-only.
	if strings.Contains(compact, "Base Armor Class:") {
		t.Errorf("compact armor must hide the AC breakdown:\n%s", compact)
	}
	// Full: Base->...->Total order.
	if base, total := ttIndexOf(full, "Base Armor Class:"), ttIndexOf(full, "Total Armor Class:"); base < 0 || total < 0 || base >= total {
		t.Errorf("full armor DEFENSE must read Base->...->Total (base=%d total=%d):\n%s", base, total, full)
	}
}

func TestTooltipCompact_TrapKeepsCooldown(t *testing.T) {
	g, thief := newThiefTestGame(t)
	trap := thief.Equipment[items.SlotSpell] // cleave_trap (ItemTrap)

	compact := GetItemTooltip(trap, thief, g.combat, false)
	// Cooldown is a core combat stat - visible compact (like weapons/spells).
	for _, want := range []string{"Cost:", "Cooldown:", "Range:", "Total Damage:"} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact trap must contain %q:\n%s", want, compact)
		}
	}
	// Armed Lifetime + the armor-interaction RULES stay detail.
	for _, hidden := range []string{"Armed Lifetime:", "Reduced by target Armor"} {
		if strings.Contains(compact, hidden) {
			t.Errorf("compact trap must NOT contain %q:\n%s", hidden, compact)
		}
	}
}

func TestTooltipCompact_SpellHidesDecompKeepsTotalsAndCost(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	g.combat = NewCombatSystem(g)
	caster := character.CreateCharacter("Lys", character.ClassSorcerer, cfg)
	g.party.Members[0] = caster

	compact := GetSpellTooltip("fireball", caster, g.combat, false)
	full := GetSpellTooltip("fireball", caster, g.combat, true)

	for _, want := range []string{"Cost:", "Cooldown:", "Total Damage:"} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact spell must contain %q:\n%s", want, compact)
		}
	}
	if strings.Contains(compact, "Base (") {
		t.Errorf("compact spell must hide the Base(...) decomposition:\n%s", compact)
	}
	if base, total := ttIndexOf(full, "Base ("), ttIndexOf(full, "Total Damage:"); base < 0 || total < 0 || base >= total {
		t.Errorf("full spell DAMAGE must read Base->...->Total (base=%d total=%d):\n%s", base, total, full)
	}
}

func TestTooltipCompact_SimpleItemHasNoShiftHint(t *testing.T) {
	g, thief := newThiefTestGame(t)
	potion, err := items.TryCreateItemFromYAML("health_potion")
	if err != nil {
		t.Skip("health_potion not defined")
	}
	compact := GetItemTooltip(potion, thief, g.combat, false)
	// Consumables are small - fully compact, no detail tier, so NO hint.
	if strings.Contains(compact, "[Shift]") {
		t.Errorf("simple item must not advertise a full breakdown it doesn't have:\n%s", compact)
	}
}

// The map editor always renders the FULL card (reference panel): its sections
// include detail lines without any Shift.
func TestEditorCardsAlwaysFull(t *testing.T) {
	loadTestConfig(t)
	def, _, ok := config.GetWeaponDefinitionByName("Magic Dagger")
	if !ok || def == nil {
		t.Skip("magic dagger not defined")
	}
	rows := character.RenderCardLines(character.WeaponCardSections(def), true)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Reduced by target Armor") {
		t.Errorf("editor weapon card (full) must include the RULES detail:\n%s", joined)
	}
}

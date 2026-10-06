package game

import (
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// The compact/full contract (user-designed unified tooltip): compact hides the
// Base->Stat->Mastery decomposition and the armor-interaction rules, but keeps
// the totals, cost/cooldown and per-item state (requirements, locks). Full
// reveals the breakdown, in builder order (results BEFORE their explanation).
// The "[Shift] full breakdown" hint shows only in compact when detail exists.

// indexOf returns the line index of the first line containing sub, or -1.
func ttIndexOf(tip, sub string) int {
	for i, ln := range strings.Split(tip, "\n") {
		if strings.Contains(ln, sub) {
			return i
		}
	}
	return -1
}

func TestTooltipCompactKeepsResultsFullAddsBreakdown(t *testing.T) {
	g, thief := newThiefTestGame(t)
	item := func(it items.Item, bearer *character.MMCharacter, cs *CombatSystem) func(*testing.T) (string, string) {
		return func(*testing.T) (string, string) {
			return GetItemTooltip(it, bearer, cs, false), GetItemTooltip(it, bearer, cs, true)
		}
	}
	yamlItem := func(key string) items.Item {
		it, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		return it
	}
	tests := []struct {
		name         string
		cards        func(*testing.T) (compact, full string)
		compact      []string    // results visible without Shift
		compactHides []string    // detail rows absent from compact
		full         []string    // rows the full card must show
		resultFirst  [][2]string // {result, its calculation} in full-card order
	}{
		{
			name:         "weapon",
			cards:        item(thief.Equipment[items.SlotMainHand], thief, g.combat),
			compact:      []string{"Total Damage:", "Cooldown:", "[Shift] full breakdown"},
			compactHides: []string{"Base:", "Normal Damage:", "Reduced by target Armor"},
			full:         []string{"Base:", "Total Damage:", "Reduced by target Armor"},
			resultFirst:  [][2]string{{"Total Damage:", "Base:"}},
		},
		{
			// The map editor renders the full card with no bearer and no combat system.
			name:         "editor weapon",
			cards:        item(thief.Equipment[items.SlotMainHand], nil, nil),
			compactHides: []string{"Reduced by target Armor"},
			full:         []string{"Reduced by target Armor"},
		},
		{
			// The thief lacks the Plate skill: the requirement is per-item state.
			name:         "armor",
			cards:        item(yamlItem("iron_armor"), thief, g.combat),
			compact:      []string{"Requires: Plate Skill", "Item Armor Class:"},
			compactHides: []string{"Base Armor Class:"},
			resultFirst:  [][2]string{{"Item Armor Class:", "Base Armor Class:"}},
		},
		{
			// Cooldown is a core combat stat, visible compact like weapons and spells.
			name:         "trap",
			cards:        item(thief.Equipment[items.SlotSpell], thief, g.combat),
			compact:      []string{"Cost:", "Cooldown:", "Range:", "Total Damage:"},
			compactHides: []string{"Armed Lifetime:", "Reduced by target Armor"},
			full:         []string{"Armed Lifetime:", "Reduced by target Armor"},
		},
		{
			name: "spell",
			cards: func(t *testing.T) (string, string) {
				cfg := loadTestConfig(t)
				sg := newTestGame(cfg, newTestWorld(cfg))
				sg.combat = NewCombatSystem(sg)
				caster := character.CreateCharacter("Lys", character.ClassSorcerer, cfg)
				sg.party.Members[0] = caster
				return GetSpellTooltip("fireball", caster, sg.combat, false), GetSpellTooltip("fireball", caster, sg.combat, true)
			},
			compact:      []string{"Cost:", "Cooldown:", "Total Damage:"},
			compactHides: []string{"Base ("},
			resultFirst:  [][2]string{{"Total Damage:", "Base ("}},
		},
		{
			name:    "potion advertises recovery and automatic-use details",
			cards:   item(yamlItem("health_potion"), thief, g.combat),
			compact: []string{"[Shift]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compact, full := tt.cards(t)
			for _, want := range tt.compact {
				if !strings.Contains(compact, want) {
					t.Errorf("compact card must contain %q:\n%s", want, compact)
				}
			}
			for _, hidden := range tt.compactHides {
				if strings.Contains(compact, hidden) {
					t.Errorf("compact card must NOT contain %q:\n%s", hidden, compact)
				}
			}
			for _, want := range tt.full {
				if !strings.Contains(full, want) {
					t.Errorf("full card must contain %q:\n%s", want, full)
				}
			}
			if strings.Contains(full, "[Shift]") {
				t.Errorf("full card must not show the Shift hint:\n%s", full)
			}
			for _, pair := range tt.resultFirst {
				if result, detail := ttIndexOf(full, pair[0]), ttIndexOf(full, pair[1]); result < 0 || detail < 0 || result >= detail {
					t.Errorf("full card must show %q before %q (lines %d, %d):\n%s", pair[0], pair[1], result, detail, full)
				}
			}
		})
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
		// baseCooldown wants the Speed-0 cooldown combat charges for this weapon.
		baseCooldown bool
		// cooldownLimit wants the RT safety floor row.
		cooldownLimit bool
		want          []string
		wantOnce      []string
		absent        []string
		order         []string
	}{
		{
			name: "baseline names Speed but omits inactive factors", weaponKey: "iron_sword",
			setup: func(_ *CombatSystem, char *character.MMCharacter) {
				delete(char.Skills, character.SkillOrcishFury)
				delete(char.Skills, character.SkillDualWielding)
			},
			want:   []string{"Speed ("},
			absent: []string{"Orcish Fury -", "Dual Wielding -", "Cooldown limit:", "Capped at 100%"},
		},
		{
			name: "shop omits every bearer factor", weaponKey: "iron_sword", shop: true,
			absent: []string{"Attack cooldown x", "Speed (", "Orcish Fury -", "Dual Wielding -", "Cooldown limit:", "Capped at 100%"},
		},
		{
			name: "shop keeps category cooldown property", weaponKey: "hunting_bow", shop: true,
			baseCooldown: true,
			wantOnce:     []string{"Base weapon cooldown:"},
			absent:       []string{"Speed (", "Dual Wielding -", "Cooldown limit:", "RT Cooldown:"},
		},
		{
			name: "shop keeps authored cooldown override", weaponKey: "suppressor_gun", shop: true,
			baseCooldown: true,
			wantOnce:     []string{"Base weapon cooldown:"},
			absent:       []string{"Speed (", "Dual Wielding -", "Cooldown limit:", "RT Cooldown:"},
		},
		{
			name: "equipped card keeps one category cooldown property", weaponKey: "hunting_bow",
			baseCooldown: true,
			want:         []string{"Speed (", "RT Cooldown:"},
			wantOnce:     []string{"Base weapon cooldown:"},
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
			baseCooldown: true, cooldownLimit: true,
			wantOnce: []string{"Base weapon cooldown:"},
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
			order: []string{"Total Damage:", "Active party buff: +5", "Cards: +20% melee damage"},
		},
		{
			name: "ranged card multiplier precedes buff", weaponKey: "hunting_bow",
			setup: func(cs *CombatSystem, _ *character.MMCharacter) {
				cs.game.cardSlots[0].key = "masked_huntress_card"
				cs.game.addCombatBuff(TimedCombatBuff{SpellID: "test", Frames: 60, OutBonus: 5, OutDamageType: "all"})
			},
			want:  []string{"Cards: +20% ranged damage", "Active party buff: +5"},
			order: []string{"Total Damage:", "Cards: +20% ranged damage", "Active party buff: +5"},
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

			want := tt.want
			if tt.baseCooldown {
				// Speed 0, no Dual Wielding, outside the party: no modifier applies.
				probe := character.CreateCharacter("Probe", char.Class, cs.game.config)
				probe.Speed = 0
				delete(probe.Skills, character.SkillDualWielding)
				want = append(slices.Clone(want), "Base weapon cooldown: "+cooldownSeconds(cs, cs.WeaponCooldownFramesFor(probe, weapon.Name)))
			}
			if tt.cooldownLimit {
				want = append(slices.Clone(want), "Cooldown limit: "+cooldownSeconds(cs, RTCooldownMinFrames))
			}
			for _, want := range want {
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
			for _, detail := range []string{"Attack cooldown x", "Speed (", "Orcish Fury -", "Dual Wielding -", "Cooldown limit:", "Capped at 100%"} {
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

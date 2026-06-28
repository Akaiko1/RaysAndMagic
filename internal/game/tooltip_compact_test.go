package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// The compact/full contract (user-designed unified tooltip): compact hides the
// Base→Stat→Mastery decomposition and the universal RULES, but keeps the
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
		t.Errorf("full weapon DAMAGE must read Base→…→Total (base=%d total=%d):\n%s", base, total, full)
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

	// The equip requirement is per-item state → visible in COMPACT.
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
	// Full: Base→…→Total order.
	if base, total := ttIndexOf(full, "Base Armor Class:"), ttIndexOf(full, "Total Armor Class:"); base < 0 || total < 0 || base >= total {
		t.Errorf("full armor DEFENSE must read Base→…→Total (base=%d total=%d):\n%s", base, total, full)
	}
}

func TestTooltipCompact_TrapKeepsCooldown(t *testing.T) {
	g, thief := newThiefTestGame(t)
	trap := thief.Equipment[items.SlotSpell] // cleave_trap (ItemTrap)

	compact := GetItemTooltip(trap, thief, g.combat, false)
	// Cooldown is a core combat stat — visible compact (like weapons/spells).
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
		t.Errorf("full spell DAMAGE must read Base→…→Total (base=%d total=%d):\n%s", base, total, full)
	}
}

func TestTooltipCompact_SimpleItemHasNoShiftHint(t *testing.T) {
	g, thief := newThiefTestGame(t)
	potion, err := items.TryCreateItemFromYAML("health_potion")
	if err != nil {
		t.Skip("health_potion not defined")
	}
	compact := GetItemTooltip(potion, thief, g.combat, false)
	// Consumables are small — fully compact, no detail tier, so NO hint.
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

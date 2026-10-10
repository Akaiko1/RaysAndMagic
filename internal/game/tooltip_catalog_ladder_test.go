package game

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

var (
	tierLadderPattern = regexp.MustCompile(`\d+(?:/\d+){3}`)
	cardNumberPattern = regexp.MustCompile(`\d+`)
	cardLabelSuffix   = regexp.MustCompile(` (- [A-Za-z]+|\([^)]*\))$`)
)

// Invariant: a catalog card (no caster: editor, shop, bag) states every
// mastery-scaled number at every tier, and tier N of each ladder is exactly
// what the live card of a statless caster at tier N shows.
// Case table:
//   - Content: every player spell and every trap in the shipped catalogs.
//   - Lines: totals, bases, mastery bonuses, durations (RT and TB rounds),
//     buff magnitudes, summon stats, Trapper damage and control durations.
//   - Tiers: Novice..Grandmaster; a zero contribution may be omitted live.
//
// Entry points: GetSpellTooltipRows and GetItemTooltipRows, the two builders the
// editor and the game share. Persistence: N/A (presentation only).
func TestCatalogLaddersMatchEveryTier(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	casterAt := func(tier int) *character.MMCharacter {
		c := character.CreateCharacter("Ladder", character.ClassSorcerer, cs.game.config)
		mastery := character.SkillMastery(tier)
		c.Skills = map[character.SkillType]*character.Skill{character.SkillTrapper: {Mastery: mastery}}
		for _, id := range character.AllMagicSchools {
			c.MagicSchools[id] = &character.MagicSkill{Mastery: mastery}
		}
		c.Might, c.Intellect, c.Personality, c.Endurance, c.Accuracy, c.Speed, c.Luck = 0, 0, 0, 0, 0, 0, 0
		c.Equipment = map[items.EquipSlot]items.Item{}
		return c
	}
	var spellKeys []string
	for key, def := range config.GlobalSpells.Spells {
		if def != nil && !def.MonsterOnly {
			spellKeys = append(spellKeys, key)
		}
	}
	sort.Strings(spellKeys)
	var trapKeys []string
	for key := range config.GlobalTrapConfig.Traps {
		trapKeys = append(trapKeys, key)
	}
	sort.Strings(trapKeys)

	ladders := 0
	check := func(t *testing.T, catalog string, live func(tier int) string) {
		t.Helper()
		for _, line := range strings.Split(catalog, "\n") {
			label, value, ok := strings.Cut(line, ": ")
			if !ok || !tierLadderPattern.MatchString(value) {
				continue
			}
			ladders++
			for tier := 0; tier < 4; tier++ {
				expected := tierLadderPattern.ReplaceAllStringFunc(value, func(ladder string) string {
					return strings.Split(ladder, "/")[tier]
				})
				want := cardNumbers(expected)
				card := live(tier)
				if !liveCardShows(card, label, want) {
					t.Fatalf("catalog %q tier %d wants %s %v; live card:\n%s", line, tier, label, want, card)
				}
			}
		}
	}
	for _, key := range spellKeys {
		t.Run("spell/"+key, func(t *testing.T) {
			id := spells.SpellID(key)
			check(t, GetSpellTooltipRows(id, nil, nil, true).String(), func(tier int) string {
				return GetSpellTooltipRows(id, casterAt(tier), cs, true).String()
			})
		})
	}
	for _, key := range trapKeys {
		t.Run("trap/"+key, func(t *testing.T) {
			it, ok := config.TrapItem(key)
			if !ok {
				t.Fatalf("trap %q has no catalog item", key)
			}
			check(t, GetItemTooltipRows(it, nil, nil, true).String(), func(tier int) string {
				return GetItemTooltipRows(it, casterAt(tier), cs, true).String()
			})
		})
	}
	// Positive control: the shipped catalog has mastery ladders to check.
	if ladders < len(spellKeys)/2 {
		t.Fatalf("only %d catalog ladders across %d spells", ladders, len(spellKeys))
	}
}

// liveCardShows finds the live line for a catalog label ("Current bonus" for
// "Bonus", "Fire Mastery - Master" for "Fire Mastery") whose numbers start
// with want. A zero contribution may be omitted from a live card.
func liveCardShows(card, label string, want []int) bool {
	for _, line := range strings.Split(card, "\n") {
		got, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		got = cardLabelSuffix.ReplaceAllString(strings.TrimPrefix(got, "Current "), "")
		if strings.EqualFold(got, label) {
			numbers := cardNumbers(value)
			return len(numbers) >= len(want) && slices.Equal(numbers[:len(want)], want)
		}
	}
	return !slices.ContainsFunc(want, func(n int) bool { return n != 0 })
}

func cardNumbers(text string) []int {
	var out []int
	for _, n := range cardNumberPattern.FindAllString(text, -1) {
		v, _ := strconv.Atoi(n)
		out = append(out, v)
	}
	return out
}

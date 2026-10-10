package game

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// A dual-school page must name BOTH of its schools: Town Portal is earth and
// air, and a card that says only "Earth Magic" reads as unbuyable to the Air
// caster the shop will happily sell it to.
func TestSpellTooltipNamesEverySchoolOfADualSchoolSpell(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("town_portal"))
	if err != nil {
		t.Fatalf("town_portal: %v", err)
	}
	if len(def.SchoolList()) < 2 {
		t.Fatalf("town_portal is no longer dual-school: %v", def.SchoolList())
	}
	got := buildSpellTooltipUnifiedRows(def, char, cs, false).String()
	for _, want := range []string{"Earth", "Air"} {
		if !strings.Contains(got, want) {
			t.Errorf("the Town Portal card does not mention its %s school:\n%s", want, got)
		}
	}
	// And the card is SCORED under the school this caster actually holds - the
	// mastery line must never name a school the character never opened. Combat
	// reads the same rule (MMCharacter.SpellSchoolFor), so the number on the card
	// and the number in the fight come from one place.
	air := character.MagicSchoolAir
	caster := cs.game.party.Members[0]
	caster.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{
		air: {Mastery: character.MasteryExpert},
	}
	if got := caster.SpellSchoolFor(def); got != air {
		t.Fatalf("an Air-only caster files Town Portal under %q, want air", got)
	}
	if got := spellSchoolForChar(caster, def); got != string(air) {
		t.Fatalf("the card scores Town Portal under %q for an Air-only caster", got)
	}
	if card := buildSpellTooltipUnifiedRows(def, caster, cs, true).String(); strings.Contains(card, "Earth Mastery") {
		t.Errorf("the card credits Earth mastery to a caster who only holds Air:\n%s", card)
	}
	// FILED, not merely open: opening the other school later must not move the
	// page. A sorcerer who bought Town Portal through Air keeps casting it as Air
	// even after a promotion opens Earth at Novice.
	if !caster.LearnSpell(def.ID) {
		t.Fatal("the Air caster could not learn Town Portal")
	}
	caster.MagicSchools[character.MagicSchoolEarth] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
	if got := caster.SpellSchoolFor(def); got != air {
		t.Fatalf("opening Earth moved the page to %q - it is filed under Air", got)
	}
	if skill := caster.SpellMasterySkill(def); skill == nil || skill.Mastery != character.MasteryExpert {
		t.Fatalf("mastery = %+v, want the Expert AIR skill the page is filed under", skill)
	}
	if card := buildSpellTooltipUnifiedRows(def, caster, cs, true).String(); strings.Contains(card, "Earth Mastery") {
		t.Errorf("the card credits Earth mastery to a page filed under Air:\n%s", card)
	}

	// Nobody to ask: the spell's own primary school.
	if got := spellSchoolForChar(nil, def); got != def.School {
		t.Fatalf("with no character the card scores under %q, want the primary %q", got, def.School)
	}

	// A single-school spell keeps its plain subtitle.
	fire, err := spells.GetSpellDefinitionByID(spells.SpellID("fireball"))
	if err != nil {
		t.Fatalf("fireball: %v", err)
	}
	if card := buildSpellTooltipUnifiedRows(fire, char, cs, false).String(); !strings.Contains(card, "Fire Magic") {
		t.Errorf("a single-school card lost its subtitle:\n%s", card)
	}
}

// spellCardEnv is what a card row derives its authored numbers from.
type spellCardEnv struct {
	def  spells.SpellDefinition
	char *character.MMCharacter
	cs   *CombatSystem
}

// scaled is a magnitude at the caster's tier, interpolated to its GM cap.
func (e spellCardEnv) scaled(base, gm int) int {
	return scaledMasteryValueAt(base, gm, character.SpellMasteryTier(e.char, e.def))
}

// stunSpan is the "(Ns / M TB turn" span; the plural suffix is left to the card.
func (e spellCardEnv) stunSpan() string {
	return fmt.Sprintf("%ds / %d TB turn", e.def.StunDurationSeconds, e.def.StunDurationTurns)
}

func (e spellCardEnv) duration() string {
	seconds := character.SpellDurationBreakdown(e.def, e.char).Seconds
	rounds := (seconds + character.TurnBasedTurnSeconds - 1) / character.TurnBasedTurnSeconds
	return fmt.Sprintf("Current Duration: %ds (TB: %s)", seconds, pluralizeCount(rounds, "round", "rounds"))
}

// Every spell card states the mechanics the 2026-09-30 audit checked against
// the code, through the live entry point (GetSpellTooltipRows adds the authored
// description and routes monster-only spells to their own card). Numbers come
// from the spell definitions; full rows are the Shift view, compact rows the
// default hover that shows results without their formula.
func TestSpellCardsStateAuditedMechanics(t *testing.T) {
	if prev := monster.MonsterConfig; prev == nil {
		t.Cleanup(func() { monster.MonsterConfig = prev })
		monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	}
	armorCap := fmt.Sprintf("(up to %d%%)", character.ArmorElementalMitigationCap)
	for _, tc := range []struct {
		name    string
		id      spells.SpellID
		compact bool
		tier    character.SkillMastery // in the spell's school
		card    string                 // collection card slotted for the row
		want    []string
		mustNot []string
		derive  func(spellCardEnv) (want, mustNot []string)
	}{
		{id: "fireball", want: []string{"Reduced by target Armor " + armorCap + " and Fire Resistance",
			"Bursts where it stops: on a target, a wall or at the end of its range", "a target that dodges escapes only its own hit"}, mustNot: []string{"tile AoE"},
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{fmt.Sprintf("splash radius %.0f tiles", e.def.AoeRadiusTiles)}, nil
			}},
		{id: "deadly_swarm", want: []string{"Bursts where it stops"}},
		{id: "starburst", want: []string{"Bursts where it stops"}, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("splash radius %.0f tiles", e.def.AoeRadiusTiles)}, nil
		}},
		{id: "firebolt", want: []string{"Reduced by target Armor", "Intellect ("}, mustNot: []string{"Bursts where it stops"}},
		{id: "lightning", want: []string{"A dodged hit never stuns"}, mustNot: []string{"1 TB turns"},
			derive: func(e spellCardEnv) ([]string, []string) { return []string{"(" + e.stunSpan()}, nil }},
		{id: "psychic_shock", want: []string{"A dodged hit never stuns", "Personality ("}, // self-magic school -> Personality
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{fmt.Sprintf("Stun chance: %.0f%% on hit (%s", e.def.StunChance*100, e.stunSpan())}, nil
			}},
		{name: "disintegrate", id: "disintegrate", derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Disintegrate: %.0f%% chance", e.def.DisintegrateChance*100)}, []string{"Cards: +"}
		}},
		{name: "disintegrate/card", id: "disintegrate", card: "alien_card", derive: func(e spellCardEnv) ([]string, []string) {
			bonus := cardDef("alien_card").CardDisintegratePct
			return []string{fmt.Sprintf("Disintegrate: %.0f%% chance", e.def.DisintegrateChance*100+float64(bonus)), fmt.Sprintf("Cards: +%d%% disintegrate chance", bonus)},
				[]string{fmt.Sprintf("Disintegrate: %.0f%% chance", e.def.DisintegrateChance*100)}
		}},
		{id: "alien_dark_bolt", want: []string{"chance to eradicate the hero it hits", "Strikes your party and the undead your party has bound"}, mustNot: []string{"undead and dragons immune", "not other monsters"}},
		{id: "ice_bolt", mustNot: []string{"chilling"}},
		{id: "inferno", want: []string{"Reduced by enemy Armor " + armorCap, "Self-damage is reduced by party Armor", "Radius: Current map"}},
		{name: "inferno/compact", id: "inferno", compact: true, tier: character.MasteryGrandMaster, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Damage: %d", e.def.DamageForMastery(int(character.MasteryGrandMaster)))},
				[]string{fmt.Sprintf("%d-%d damage by mastery", e.def.DamageForMastery(0), e.def.DamageForMastery(int(character.MasteryGrandMaster)))}
		}},
		{id: "earthquake", tier: character.MasteryGrandMaster, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Base (Grandmaster): %d", e.def.DamageByMastery[character.MasteryGrandMaster])}, nil
		}},
		{id: "firewall", want: []string{"Reduced by target Armor " + armorCap + " and Fire Resistance", "% of max HP per second", "ignoring fire resistance",
			"A monster that walks in is hit at once"}, mustNot: []string{"Beside the wall", "scalds", "Ignores armor"},
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{
					fmt.Sprintf("Around the wall (%s): %d%% damage", pluralizeCount(e.def.ZoneEdgeTiles, "tile", "tiles"), e.def.ZoneEdgeDamagePercent),
					fmt.Sprintf("Sets monsters it touches burning for %.0fs", e.def.ZoneBurnSeconds),
				}, nil
			}},
		{id: "hot_steam", want: []string{"Reduced by target Armor " + armorCap + " and Water Resistance", "A monster that walks in is hit at once",
			"DAMAGE PER TICK", "harms any monster inside (your party is unharmed)", "Intellect ("}, mustNot: []string{"Ignores armor"},
			// Cadence lives in the structured ZONE section.
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{fmt.Sprintf("RT: one tick every %.0fs", e.def.ZoneTickSeconds)}, nil
			}},
		{id: "stone_blossom", want: []string{"Reduced by target Armor"}, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{"(" + e.stunSpan(), fmt.Sprintf("blooms exactly %.0f tiles out", e.def.MortarRangeTiles)}, nil
		}},
		{name: "stone_blossom/compact", id: "stone_blossom", compact: true, tier: character.MasteryGrandMaster,
			want: []string{"Total Damage:"}, mustNot: []string{"Earth Mastery -"}},
		{id: "stun", derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Stuns every monster within %.1f tiles for %s", e.def.StunRadiusTiles, e.stunSpan())}, nil
		}},
		{id: "darkness", derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Stuns every monster within %.1f tiles for %s", e.def.StunRadiusTiles, e.stunSpan())}, nil
		}},
		{id: "charm", derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Pacifies a living target, not a boss, for %ds", e.def.PacifyDurationSeconds)}, nil
		}},
		{id: "bind_undead", want: []string{"Your attacks and area spells still hit it", "crumbles if you leave the map"},
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{fmt.Sprintf("undead target for %ds", e.def.BindDurationSeconds)}, nil
			}},
		{id: "summon_ice_elemental", want: []string{"It stays until killed and vanishes when you leave the map"},
			derive: func(e spellCardEnv) ([]string, []string) {
				m, err := monster.MonsterConfig.GetMonsterByKey(e.def.SummonMonster)
				if err != nil {
					t.Fatal(err)
				}
				shot, err := spells.GetSpellDefinitionByID(spells.SpellID(m.ProjectileSpell))
				if err != nil {
					t.Fatal(err)
				}
				return []string{
					fmt.Sprintf("up to %d for the whole party", e.def.SummonMax),
					fmt.Sprintf("Attacks with %s (%s, range %.0f tiles)", shot.Name, config.TitleWords(shot.School), m.RangedAttackRange),
				}, nil
			}},
		{id: "jump", want: []string{"TB: ends the party's turn", "If the landing is blocked or the party is rooted, the SP is kept"}},
		{id: "resurrect", want: []string{"the eradicated first, then the others in party order", "With no one fallen, the SP is kept", "full HP"}},
		{id: "raise_dead", derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Revives the first fallen ally in party order to %d%% HP (not the eradicated)", e.def.ReviveHpPct)}, nil
		}},
		{id: "awaken", want: []string{"With no one unconscious, the SP is kept", "Wakes all unconscious allies"}},
		{id: "heal", want: []string{"Personality ("}},
		{id: "heal_other", want: []string{"Heals the ally you point at, else the selected hero; from the spellbook, the most wounded ally"}, mustNot: []string{"Can target any party member"}},
		{id: "mass_heal", want: []string{"Skips allies at 0 HP"}},
		{id: "town_portal", want: []string{"nothing is spent until you choose one"}},
		{id: "fly", want: []string{"Inside a wall or tree nobody can attack or cast", "Ends at once on a map without open sky", "No RT cooldown - TB: 1 action", "(TB: ", "Only under an open sky"}},
		{name: "fly/compact", id: "fly", compact: true, tier: character.MasteryExpert, mustNot: []string{"Air Mastery -"},
			derive: func(e spellCardEnv) ([]string, []string) { return []string{e.duration()}, nil }},
		{id: "water_breathing", want: []string{"Deep water takes the party down to the Ocean Depths", "surfaces where it dove in"}, mustNot: []string{"Allows underwater travel"}},
		{id: "torch_light", want: []string{"no effect in daylight"}, mustNot: []string{"Sight/radar", "radar"},
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{fmt.Sprintf("Lights the dark within %.0f tiles", e.def.LightRadiusTiles)}, nil
			}},
		{id: "wizard_eye", want: []string{"Shows nearby monsters on the compass"}, mustNot: []string{"Sight/radar", "Extends your vision"},
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{fmt.Sprintf("Compass radar: monsters within %.0f tiles, even through walls", e.def.RadarRadiusTiles)}, nil
			}},
		{id: "bless", mustNot: []string{"Recasting"}, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Current bonus to all stats: +%d", e.scaled(e.def.StatBonus, e.def.StatBonusGrandmaster)), e.duration()}, nil
		}},
		{id: "fire_shield", mustNot: []string{"Recasting"}, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Party resists %s +%d%%", config.TitleWords(e.def.ResistBuffSchool), e.def.ResistBuffSchoolPct)}, nil
		}},
		{name: "fire_shield/compact", id: "fire_shield", compact: true, tier: character.MasteryMaster, mustNot: []string{"Fire Mastery -"},
			derive: func(e spellCardEnv) ([]string, []string) { return []string{e.duration()}, nil }},
		{id: "day_of_the_gods", mustNot: []string{"resistance: -"}, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Current incoming damage: -%d%%", e.scaled(e.def.ResistBuffPct, e.def.ResistBuffPctGrandmaster))}, nil
		}},
		{id: "heroism", derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Current physical damage bonus: +%d", e.scaled(e.def.OutgoingDamageBonus, e.def.OutgoingDamageBonusGrandmaster))}, nil
		}},
		{name: "heroism/compact", id: "heroism", compact: true, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Current physical damage bonus: +%d", e.scaled(e.def.OutgoingDamageBonus, e.def.OutgoingDamageBonusGrandmaster))},
				[]string{fmt.Sprintf("deal +%d to +%d", e.def.OutgoingDamageBonus, e.def.OutgoingDamageBonusGrandmaster)}
		}},
		{id: "stone_skin", want: []string{"true damage and damage over time pass"}, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Current reduction: -%d per hit", e.scaled(e.def.IncomingDamageReduction, e.def.IncomingDamageReductionGrandmaster))}, nil
		}},
		{name: "stone_skin/compact", id: "stone_skin", compact: true, tier: character.MasteryGrandMaster, derive: func(e spellCardEnv) ([]string, []string) {
			return []string{fmt.Sprintf("Current reduction: -%d per hit", e.scaled(e.def.IncomingDamageReduction, e.def.IncomingDamageReductionGrandmaster))},
				[]string{fmt.Sprintf("Party takes -%d to -%d", e.def.IncomingDamageReduction, e.def.IncomingDamageReductionGrandmaster)}
		}},
		{id: "hour_of_power", want: []string{"true damage and damage over time pass", "No RT cooldown - TB: 1 action"},
			derive: func(e spellCardEnv) ([]string, []string) {
				return []string{
					fmt.Sprintf("Current damage bonus: +%d", e.scaled(e.def.OutgoingDamageBonus, e.def.OutgoingDamageBonusGrandmaster)),
					fmt.Sprintf("Current reduction: -%d per hit", e.scaled(e.def.IncomingDamageReduction, e.def.IncomingDamageReductionGrandmaster)),
				}, nil
			}},
	} {
		name := tc.name
		if name == "" {
			name = string(tc.id)
		}
		t.Run(name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			char := cs.game.party.Members[0]
			cs.game.cardSlots = [MaxCardSlots]cardSlot{}
			cs.game.cardSlots[0].key = tc.card
			def, err := spells.GetSpellDefinitionByID(tc.id)
			if err != nil {
				t.Fatal(err)
			}
			char.MagicSchools[character.MagicSchoolID(def.School)] = &character.MagicSkill{Mastery: tc.tier}
			card := GetSpellTooltipRows(def.ID, char, cs, !tc.compact).String()
			want, mustNot := tc.want, tc.mustNot
			if tc.derive != nil {
				w, n := tc.derive(spellCardEnv{def: def, char: char, cs: cs})
				want, mustNot = append(slices.Clone(want), w...), append(slices.Clone(mustNot), n...)
			}
			for _, w := range want {
				if !strings.Contains(card, w) {
					t.Errorf("missing %q:\n%s", w, card)
				}
			}
			for _, bad := range mustNot {
				if strings.Contains(card, bad) {
					t.Errorf("still says %q:\n%s", bad, card)
				}
			}
			if !tc.compact {
				assertSpellCooldownStagesAddUp(t, card, char)
			}
		})
	}
}

// The displayed cooldown stages sum to the RT cooldown at two decimals, and
// the Speed stage names the caster's effective Speed.
func assertSpellCooldownStagesAddUp(t *testing.T, card string, char *character.MMCharacter) {
	t.Helper()
	var base, delta, total float64
	var speed int
	var haveBase, haveSpeed, haveTotal bool
	for _, line := range strings.Split(card, "\n") {
		switch {
		case strings.HasPrefix(line, "Base cooldown:"):
			_, err := fmt.Sscanf(line, "Base cooldown: %fs", &base)
			haveBase = err == nil
		case strings.HasPrefix(line, "Speed ("):
			_, err := fmt.Sscanf(line, "Speed (%d): %fs", &speed, &delta)
			haveSpeed = err == nil
		case strings.HasPrefix(line, "RT Cooldown:"):
			_, err := fmt.Sscanf(line, "RT Cooldown: %fs", &total)
			haveTotal = err == nil
		case strings.HasPrefix(line, "Cooldown limit:"), strings.HasPrefix(line, "Quickening:"), strings.Contains(line, " cooldown") && strings.Contains(line, ": x"):
			return // further stages: the two-stage sum does not apply
		}
	}
	if !haveBase && !haveSpeed {
		return
	}
	if !haveBase || !haveSpeed || !haveTotal {
		t.Fatalf("cooldown breakdown incomplete:\n%s", card)
	}
	if speed != char.GetEffectiveSpeed() {
		t.Errorf("Speed stage names %d, caster has %d:\n%s", speed, char.GetEffectiveSpeed(), card)
	}
	if math.Abs(base+delta-total) > 0.0051 {
		t.Errorf("cooldown stages %.2f%+.2f do not add up to %.2f:\n%s", base, delta, total, card)
	}
}

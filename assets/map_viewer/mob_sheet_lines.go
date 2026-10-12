package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
)

// tbPreviewRounds is how many rounds the sheet plays out below a TB rate.
const tbPreviewRounds = 5

// tbRateText is a turn-based count per action ("attacks", "moves") to the
// second decimal. Rates are fractional because the remainder accumulates
// between actions (monster/turn_based_cadence.go).
func tbRateText(what string, rate float64) string {
	return fmt.Sprintf("TB %s/round: %.2f", what, rate)
}

// tbSequenceText plays the first rounds from a fresh carry exactly as combat
// deals them, with their total and average.
func tbSequenceText(rate float64) string {
	counts := monster.TurnBasedSequence(rate, tbPreviewRounds)
	parts := make([]string, len(counts))
	total := 0
	for i, n := range counts {
		parts[i] = strconv.Itoa(n)
		total += n
	}
	return fmt.Sprintf("First %d rounds: %s = %d (avg %.2f)", tbPreviewRounds, strings.Join(parts, " "), total, float64(total)/tbPreviewRounds)
}

// appendTBRateLines adds one rate line and its five-round preview below it.
func appendTBRateLines(out []infoLine, label, what string, rate float64) []infoLine {
	return append(out,
		infoLine{text: label + tbRateText(what, rate), col: mobStatDefault},
		infoLine{text: tbSequenceText(rate), col: overlayMuted, depth: 1})
}

// appendTurnBasedAttackLines adds the TB swing rate, then the same pair while
// enraged when the monster authors an enrage speed-up. Champions follow their
// own fixed rule instead.
func appendTurnBasedAttackLines(out []infoLine, def monster.MonsterDefinition, cooldownMult float64) []infoLine {
	if def.Champion != "" {
		return append(out, infoLine{text: fmt.Sprintf("TB attacks/round: %d (champion rule)", monster.ChampionTurnBasedSwings), col: mobStatDefault})
	}
	out = appendTBRateLines(out, "", "attacks", monster.TurnBasedAttackRate(false, cooldownMult, def.Abilities.Enrage.CooldownMult, false))
	if e := def.Abilities.Enrage; e.AtHP > 0 && e.CooldownMult > 0 {
		out = appendTBRateLines(out, "Enraged ", "attacks", monster.TurnBasedAttackRate(false, cooldownMult, e.CooldownMult, true))
	}
	return out
}

// tbMoveRate is the monster's TB steps per movement action for a speed.
// Ambient monsters (any disposition) may idle below one step.
func tbMoveRate(def monster.MonsterDefinition, speed float64) float64 {
	return monster.TurnBasedStepRate(speed, config.GlobalConfig.GetTurnStepSpeed(), def.IsAmbient())
}

// tbStationaryText marks a monster that never moves.
const tbStationaryText = "TB moves/round: 0 (stationary)"

// appendTurnBasedMoveLines adds the TB step rate for the movement speed.
func appendTurnBasedMoveLines(out []infoLine, def monster.MonsterDefinition, speed float64) []infoLine {
	if speed <= 0 {
		return append(out, infoLine{text: tbStationaryText, col: mobStatDefault})
	}
	return appendTBRateLines(out, "", "moves", tbMoveRate(def, speed))
}

// appendChampionTierLines lays a champion's arena tiers side by side, easiest
// first ("20 / 23 / 25"). Every number comes from sheets the game builds
// through the duel spawn path; the loadouts follow, one block per tier.
func appendChampionTierLines(out []infoLine, tiers []game.ChampionTierSheet) []infoLine {
	if len(tiers) == 0 {
		return out
	}
	ladder := func(value func(game.ChampionTierSheet) string) string {
		parts := make([]string, len(tiers))
		for i, t := range tiers {
			parts[i] = value(t)
		}
		return strings.Join(parts, " / ")
	}
	add := func(col color.Color, depth int, format string, args ...any) {
		out = append(out, infoLine{text: fmt.Sprintf(format, args...), col: col, depth: depth})
	}
	itoa := func(n int) string { return strconv.Itoa(n) }

	out = appendInfoHeader(out, "CHAMPION TIERS")
	add(mobStatHeader, 0, "%s", ladder(func(t game.ChampionTierSheet) string { return titleCase(t.Tier) }))
	// One fact per line: a ladder must never wrap in the middle.
	add(mobStatDefault, 0, "Level %s", ladder(func(t game.ChampionTierSheet) string { return itoa(t.Rules.Level) }))
	add(mobStatDefault, 0, "Mastery %s", ladder(func(t game.ChampionTierSheet) string { return t.Rules.Mastery }))
	add(mobStatHP, 0, "HP %s", ladder(func(t game.ChampionTierSheet) string { return itoa(t.Mob.MaxHitPoints) }))
	add(mobStatDefault, 0, "Armor Class %s", ladder(func(t game.ChampionTierSheet) string { return itoa(t.Mob.ArmorClass) }))
	add(mobStatDefault, 0, "Perfect dodge %s", ladder(func(t game.ChampionTierSheet) string { return itoa(t.Mob.PerfectDodge) + "%" }))

	tps := float64(config.GetTargetTPS())
	hands := 0
	for _, t := range tiers {
		hands = max(hands, len(t.Hands))
	}
	for h := 0; h < hands; h++ {
		hand := func(value func(game.ChampionHandSheet) string) string {
			return ladder(func(t game.ChampionTierSheet) string {
				if h >= len(t.Hands) {
					return "-"
				}
				return value(t.Hands[h])
			})
		}
		label := "Main hand"
		if h > 0 {
			label = "Off hand"
		}
		add(mobStatDamage, 0, "%s damage %s", label, hand(func(s game.ChampionHandSheet) string { return itoa(s.Damage) }))
		add(mobStatDefault, 1, "Crit %s", hand(func(s game.ChampionHandSheet) string { return itoa(s.CritChance) + "%" }))
		add(mobStatDefault, 1, "RT cooldown %s", hand(func(s game.ChampionHandSheet) string { return fmt.Sprintf("%.2fs", float64(s.CooldownFrames)/tps) }))
	}

	attributes := []struct {
		name  string
		value func(*character.MMCharacter) int
	}{
		{"Might", (*character.MMCharacter).GetEffectiveMight},
		{"Intellect", (*character.MMCharacter).GetEffectiveIntellect},
		{"Personality", (*character.MMCharacter).GetEffectivePersonality},
		{"Endurance", (*character.MMCharacter).GetEffectiveEndurance},
		{"Accuracy", (*character.MMCharacter).GetEffectiveAccuracy},
		{"Speed", (*character.MMCharacter).GetEffectiveSpeed},
		{"Luck", (*character.MMCharacter).GetEffectiveLuck},
	}
	for _, a := range attributes {
		add(mobStatDefault, 0, "%s %s", a.name, ladder(func(t game.ChampionTierSheet) string { return itoa(a.value(t.Build)) }))
	}
	for _, school := range monster.DamageTypes() {
		present := false
		for _, t := range tiers {
			present = present || t.Mob.Resistances[school] != 0
		}
		if present {
			add(mobStatDefault, 0, "Resist %s %s", school, ladder(func(t game.ChampionTierSheet) string { return itoa(t.Mob.Resistances[school]) + "%" }))
		}
	}
	add(mobStatGold, 0, "XP %s", ladder(func(t game.ChampionTierSheet) string { return itoa(t.Rules.Experience) }))
	add(mobStatGold, 0, "Arena points %s", ladder(func(t game.ChampionTierSheet) string { return itoa(t.Rules.ArenaPoints) }))

	out = appendInfoHeader(out, "CHAMPION LOADOUTS")
	for _, t := range tiers {
		add(mobStatHeader, 0, "%s", titleCase(t.Tier))
		for _, it := range t.Loadout {
			add(game.ItemRarityColor(it), 1, "%s", it.Name)
		}
	}
	return out
}

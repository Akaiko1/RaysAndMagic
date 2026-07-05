package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
)

const (
	autoStatSpeedTarget = 16
	// autoSecondarySoftCap: AUTO lifts a class's secondary stat only this far while
	// the primary is still climbing; the secondary is taken the rest of the way to
	// 99 only AFTER the primary is maxed.
	autoSecondarySoftCap = 50
)

func autoEnduranceTarget(class character.CharacterClass) int {
	switch class {
	case character.ClassKnight:
		return 28
	case character.ClassMonk:
		// No armor slots at all (Iron Body is the only AC source besides
		// Endurance) - target as high as the tankiest class to compensate.
		return 28
	case character.ClassArmsMaster:
		return 26
	case character.ClassPaladin:
		return 24
	case character.ClassCleric:
		return 22
	case character.ClassDruid:
		return 20
	case character.ClassArcher, character.ClassThief:
		return 18
	case character.ClassSorcerer:
		return 16
	default:
		return 16
	}
}

func primaryDamageStat(member *character.MMCharacter) *int {
	if member == nil {
		return nil
	}
	switch member.Class {
	case character.ClassSorcerer, character.ClassDruid:
		return &member.Intellect
	case character.ClassCleric:
		return &member.Personality
	case character.ClassArcher, character.ClassThief:
		return &member.Accuracy
	default:
		// Knight, Paladin, Arms Master, Monk: Might drives their weapon damage
		// (fists included - Might/3 is the larger of the Monk's two terms).
		return &member.Might
	}
}

func secondaryAutoStat(member *character.MMCharacter) *int {
	if member == nil {
		return nil
	}
	switch member.Class {
	case character.ClassPaladin:
		return &member.Personality
	case character.ClassArcher, character.ClassThief:
		return &member.Intellect
	case character.ClassDruid:
		return &member.Personality
	case character.ClassMonk:
		// Speed/4 is the Monk's other direct damage term (fists), unlike a
		// normal melee class where Speed is only cooldown/initiative.
		return &member.Speed
	default:
		return nil
	}
}

// autoDistributeStatPoints spends only base-stat points. Skill and spell choices
// remain queued for the player.
func autoDistributeStatPoints(member *character.MMCharacter, cfg *config.Config) int {
	if member == nil || member.FreeStatPoints <= 0 {
		return 0
	}
	start := member.FreeStatPoints
	spendOne := func(stat *int, cap int) bool {
		if stat == nil || *stat >= cap || member.FreeStatPoints <= 0 {
			return false
		}
		*stat++
		member.FreeStatPoints--
		return true
	}

	primary := primaryDamageStat(member)
	secondary := secondaryAutoStat(member)
	enduranceTarget := autoEnduranceTarget(member.Class)

	// 1) Speed to its flat target.
	for spendOne(&member.Speed, autoStatSpeedTarget) {
	}

	// 2) Alternate Endurance / primary (1 each) until Endurance reaches its target.
	for member.FreeStatPoints > 0 && member.Endurance < enduranceTarget {
		spent := spendOne(&member.Endurance, enduranceTarget)
		spent = spendOne(primary, MaxStatValue) || spent
		if !spent {
			break
		}
	}

	// 3) Primary -> 99, alternating with the secondary capped at 50: once the
	//    secondary hits its soft cap the primary keeps climbing alone.
	if secondary == nil {
		for spendOne(primary, MaxStatValue) {
		}
	} else {
		for member.FreeStatPoints > 0 {
			spent := spendOne(primary, MaxStatValue)
			spent = spendOne(secondary, autoSecondarySoftCap) || spent
			if !spent {
				break
			}
		}
	}

	// 4) Only after the primary is maxed: lift the secondary past 50 -> 99 and fill
	//    Endurance -> 99, alternating.
	for member.FreeStatPoints > 0 {
		spent := false
		if secondary != nil {
			spent = spendOne(secondary, MaxStatValue)
		}
		if spendOne(&member.Endurance, MaxStatValue) {
			spent = true
		}
		if !spent {
			break
		}
	}

	// 5) Safety net so AUTO never leaves points unspent (the button must always do
	//    something): round-robin every remaining stat toward 99.
	allStats := []*int{&member.Might, &member.Intellect, &member.Personality,
		&member.Endurance, &member.Accuracy, &member.Speed, &member.Luck}
	for member.FreeStatPoints > 0 {
		progressed := false
		for _, s := range allStats {
			if spendOne(s, MaxStatValue) {
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}

	spent := start - member.FreeStatPoints
	if spent > 0 {
		member.RecalculateMaxStatsGrantingGain(cfg)
	}
	return spent
}

func autoDistributePartyStatPoints(members []*character.MMCharacter, cfg *config.Config) int {
	total := 0
	for _, member := range members {
		total += autoDistributeStatPoints(member, cfg)
	}
	return total
}

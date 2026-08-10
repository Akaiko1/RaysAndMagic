package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
)

const (
	autoStatSpeedTarget = 16
	autoMonkSpeedTarget = 26
	// autoSecondarySoftCap: AUTO lifts a class's secondary stat only this far while
	// the primary is still climbing; the secondary is taken the rest of the way to
	// 99 only AFTER the primary is maxed.
	autoSecondarySoftCap = 50
)

// autoSpeedTarget is the class-aware early Speed floor. Most classes stop at
// the RT baseline; Monk reaches the first TB bonus-action threshold while also
// feeding the Speed/4 term on Fists.
func autoSpeedTarget(class character.CharacterClass) int {
	if class == character.ClassMonk {
		return autoMonkSpeedTarget
	}
	return autoStatSpeedTarget
}

func autoEnduranceTarget(class character.CharacterClass) int {
	switch class {
	case character.ClassKnight:
		return 28
	case character.ClassBattleMage:
		// Plate helps with mitigation, but Strong Magic spends HP to empower
		// offensive casts, so the hybrid needs the highest early HP target.
		return 36
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
	case character.ClassSorcerer, character.ClassDruid, character.ClassBattleMage:
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
		// Personality scales the offensive self-magic that Spiritual Training
		// fires for free; Fists' Speed scaling is covered by autoSpeedTarget.
		return &member.Personality
	case character.ClassBattleMage:
		return &member.Might
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
	for spendOne(&member.Speed, autoSpeedTarget(member.Class)) {
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

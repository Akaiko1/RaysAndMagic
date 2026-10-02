package game

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/config"
)

// autoSecondarySoftCap: AUTO lifts a class's secondary stat only this far while
// the primary is still climbing; the secondary is taken the rest of the way to
// 99 only AFTER the primary is maxed.
const autoSecondarySoftCap = 50

// autoClassStats reads the same authored priorities for the game and champion builds.
// Missing authoring is an error at load; never invent a balance fallback here.
func autoClassStats(class character.CharacterClass, configs ...*config.Config) config.AutoStatsConfig {
	cfg := config.GlobalConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	if cfg != nil {
		if a := cfg.Characters.Classes[class.Key()].AutoStats; a.Primary != "" {
			return a
		}
	}
	panic(fmt.Sprintf("class %q: auto_stats not loaded", class.Key()))
}
func autoSpeedTarget(class character.CharacterClass, cfg ...*config.Config) int {
	return autoClassStats(class, cfg...).Speed
}
func autoEnduranceTarget(class character.CharacterClass, cfg ...*config.Config) int {
	return autoClassStats(class, cfg...).Endurance
}
func autoStatField(m *character.MMCharacter, key string) *int {
	if m == nil {
		return nil
	}
	switch key {
	case "might":
		return &m.Might
	case "intellect":
		return &m.Intellect
	case "personality":
		return &m.Personality
	case "endurance":
		return &m.Endurance
	case "accuracy":
		return &m.Accuracy
	case "speed":
		return &m.Speed
	case "luck":
		return &m.Luck
	}
	return nil
}
func primaryDamageStat(m *character.MMCharacter, cfg ...*config.Config) *int {
	if m == nil {
		return nil
	}
	return autoStatField(m, autoClassStats(m.Class, cfg...).Primary)
}
func secondaryAutoStat(m *character.MMCharacter, cfg ...*config.Config) *int {
	if m == nil {
		return nil
	}
	return autoStatField(m, autoClassStats(m.Class, cfg...).Secondary)
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

	primary := primaryDamageStat(member, cfg)
	secondary := secondaryAutoStat(member, cfg)
	enduranceTarget := autoEnduranceTarget(member.Class, cfg)

	// 1) Speed to its flat target.
	for spendOne(&member.Speed, autoSpeedTarget(member.Class, cfg)) {
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

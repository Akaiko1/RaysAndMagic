package game

import (
	"fmt"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/stats"
)

// autoSecondarySoftCap: AUTO lifts a class's secondary stat only this far while
// the primary is still climbing; the secondary is taken the rest of the way to
// 99 only AFTER the primary is maxed.
const autoSecondarySoftCap = 50

// autoClassStats is a class's default AUTO build (auto_stats), the one
// champions, bench parties and test scenarios level by. Missing authoring is an
// error at load; never invent a balance fallback here.
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

// autoDistributeStatPoints spends only base-stat points, by build. Skill and
// spell choices remain queued for the player. It reads the hero's current
// stats, never a history, so a build switched mid-game takes over from there.
func autoDistributeStatPoints(member *character.MMCharacter, build config.AutoStatsConfig, cfg *config.Config) int {
	spent := spendAutoStatPoints(member, build)
	if spent > 0 {
		member.RecalculateMaxStatsGrantingGain(cfg)
	}
	return spent
}

// spendAutoStatPoints is the allocation alone: it moves FreeStatPoints into
// stats and touches nothing else, so a copy of a hero previews it safely.
// Points go one at a time, each chosen from the current stats, so spending
// them all at once or level by level ends in the same stats.
func spendAutoStatPoints(member *character.MMCharacter, build config.AutoStatsConfig) int {
	if member == nil || member.FreeStatPoints <= 0 {
		return 0
	}
	start := member.FreeStatPoints
	for member.FreeStatPoints > 0 {
		stat := nextAutoStat(member, build)
		if stat == nil {
			break
		}
		*stat++
		member.FreeStatPoints--
	}
	return start - member.FreeStatPoints
}

// nextAutoStat is the stat AUTO raises next. A pair alternates by the parity
// of the hero's stat total, which every point flips, so the pair still splits
// 1-for-1 however the points are batched; an even total serves the first.
func nextAutoStat(m *character.MMCharacter, build config.AutoStatsConfig) *int {
	total := 0
	for _, name := range stats.Names {
		total += *autoStatField(m, name)
	}
	below := func(stat *int, cap int) bool { return stat != nil && *stat < cap }
	pair := func(a *int, aCap int, b *int, bCap int) *int {
		if total%2 == 1 {
			a, aCap, b, bCap = b, bCap, a, aCap
		}
		switch {
		case below(a, aCap):
			return a
		case below(b, bCap):
			return b
		}
		return nil
	}
	primary := autoStatField(m, build.Primary)
	secondary := autoStatField(m, build.Secondary)

	// 1) Speed to its flat target.
	if below(&m.Speed, build.Speed) {
		return &m.Speed
	}
	// 2) Endurance to its target, paired with the primary.
	if below(&m.Endurance, build.Endurance) {
		return pair(&m.Endurance, build.Endurance, primary, MaxStatValue)
	}
	// 3) Primary to 99, paired with the secondary up to its soft cap; past
	//    that the primary climbs alone.
	if stat := pair(primary, MaxStatValue, secondary, autoSecondarySoftCap); stat != nil {
		return stat
	}
	// 4) Only after the primary is maxed: the secondary and Endurance to 99.
	if stat := pair(secondary, MaxStatValue, &m.Endurance, MaxStatValue); stat != nil {
		return stat
	}
	// 5) Safety net so AUTO never strands points: the lowest stat below 99.
	var low *int
	for _, name := range stats.Names {
		if stat := autoStatField(m, name); below(stat, MaxStatValue) && (low == nil || *stat < *low) {
			low = stat
		}
	}
	return low
}

// autoDistributeMemberStatPoints spends one hero's points by the build chosen
// for their class in Settings.
func (g *MMGame) autoDistributeMemberStatPoints(member *character.MMCharacter) int {
	if member == nil {
		return 0
	}
	return autoDistributeStatPoints(member, g.autoBuildFor(member.Class).AutoStatsConfig, g.config)
}

// autoDistributePartyStatPoints is the party AUTO button.
func (g *MMGame) autoDistributePartyStatPoints() int {
	total := 0
	for _, member := range g.party.Members {
		total += g.autoDistributeMemberStatPoints(member)
	}
	return total
}

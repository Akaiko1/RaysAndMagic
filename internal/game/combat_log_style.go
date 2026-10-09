package game

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// The combat log draws a line as colored pieces: who acts (by side), what it
// does (damage by school, heals, control) and, as a stripe at the left edge,
// what the line means for the party. Text stays the plain line.

// logTone is what a log line means for the party.
type logTone int

const (
	logToneNone   logTone = iota // system lines, misses
	logToneGood                  // the party deals damage, heals, kills
	logToneBad                   // the party is hit, afflicted, knocked out
	logToneReward                // experience, loot, levels
)

var logToneStripes = map[logTone]color.RGBA{
	logToneGood:   {90, 200, 110, 255},
	logToneBad:    {230, 80, 70, 255},
	logToneReward: {240, 200, 60, 255},
}

// The prose dims so names and numbers carry the line.
var (
	logPlain   = color.RGBA{175, 175, 185, 255}
	logMuted   = color.RGBA{130, 130, 140, 255}
	logHero    = color.RGBA{255, 235, 200, 255}
	logAlly    = keywordColors["utility"]
	logEnemy   = color.RGBA{240, 120, 110, 255}
	logNeutral = color.RGBA{210, 190, 130, 255}
	logTrue    = color.RGBA{255, 255, 255, 255}
	logPoison  = color.RGBA{190, 210, 70, 255}
	logLowHP   = color.RGBA{240, 190, 70, 255}
	logDyingHP = color.RGBA{240, 90, 80, 255}
)

// logPart is a styled argument of logCombat; any other argument prints plain.
type logPart interface {
	logSegments() []coloredTextSegment
}

type logStyled coloredTextSegment

func (p logStyled) logSegments() []coloredTextSegment {
	return []coloredTextSegment{coloredTextSegment(p)}
}

func logColored(text string, col color.Color) logPart { return logStyled{text, col} }

// logHeroName is a party member's name.
func logHeroName(c *character.MMCharacter) logPart {
	if c == nil {
		return logStyled{"", logHero}
	}
	return logStyled{c.Name, logHero}
}

// logParts joins pieces into one argument ("Mara's" + a school-colored spell).
func logParts(parts ...logPart) logPart { return logJoined(parts) }

type logJoined []logPart

func (p logJoined) logSegments() []coloredTextSegment {
	var segs []coloredTextSegment
	for _, part := range p {
		segs = append(segs, part.logSegments()...)
	}
	return segs
}

// logHeroText is a party-side actor named by text ("The party", a hero).
func logHeroText(name string) logPart { return logStyled{name, logHero} }

// logMonsterAs is a monster's attack under another name (its breath, its
// trap field), colored by the monster's side.
func logMonsterAs(m *monsterPkg.Monster3D, name string) logPart {
	if m == nil {
		return logStyled{name, logEnemy}
	}
	return logStyled{name, logMonsterSide(m)}
}

// logAbility is a named ability or trap: Piercing Shot, a Bear Trap.
func logAbility(name string) logPart { return logStyled{name, keywordColors["auto"]} }

// logRecoveredSP is spell points restored.
func logRecoveredSP(n int) logPart { return logStyled{strconv.Itoa(n), keywordColors["buff"]} }

// logMonsterName colors a monster by its side: the party's own summons and
// bound undead are allies, wildlife, travelers and the charmed are neutral.
func logMonsterName(m *monsterPkg.Monster3D) logPart {
	if m == nil {
		return logStyled{"", logEnemy}
	}
	return logStyled{m.Name, logMonsterSide(m)}
}

// monsterIsPartyAlly: the party's own summons and bound undead fight for it.
func monsterIsPartyAlly(m *monsterPkg.Monster3D) bool {
	return m != nil && (m.Bound || isPurePartySummon(m))
}

func logMonsterSide(m *monsterPkg.Monster3D) color.RGBA {
	switch {
	case monsterIsPartyAlly(m):
		return logAlly
	case m.IsAmbient() || m.Pacified:
		return logNeutral
	}
	return logEnemy
}

// logDamage is a damage figure in the color of its school.
func logDamage(n int, school string) logPart {
	return logStyled{strconv.Itoa(n), logSchoolColor(school)}
}

// logSchoolColor is a school's tint in the log: the tooltip palette, with
// physical lifted to white so it stands off the grey prose.
func logSchoolColor(school string) color.Color {
	if school == monsterPkg.DamagePhysical.String() {
		return logTrue
	}
	return SchoolColor(school)
}

// logTrueDamage is true damage, which no school or armor touches.
func logTrueDamage(n int) logPart { return logStyled{strconv.Itoa(n), logTrue} }

// logHealed is HP restored.
func logHealed(n int) logPart { return logStyled{strconv.Itoa(n), keywordColors["heal"]} }

// logHP is the "(HP: cur/max)" readout; the current value warns as it falls.
func logHP(cur, maxHP int) logPart { return logHPReadout{cur, maxHP} }

type logHPReadout struct{ cur, max int }

func (p logHPReadout) logSegments() []coloredTextSegment {
	col := color.Color(logPlain)
	switch {
	case p.max > 0 && 4*p.cur <= p.max:
		col = logDyingHP
	case p.max > 0 && 2*p.cur <= p.max:
		col = logLowHP
	}
	return []coloredTextSegment{{"(HP: ", logMuted}, {strconv.Itoa(p.cur), col}, {"/" + strconv.Itoa(p.max) + ")", logMuted}}
}

// logKeyword is a word in a tooltip keyword color (stunned, rooted, blessed).
func logKeyword(kind, text string) logPart { return logStyled{text, keywordColors[kind]} }

// logSchoolWord is a word in a school's color (fire, a Fire Shield).
func logSchoolWord(school, text string) logPart { return logStyled{text, logSchoolColor(school)} }

// logItemName is an item in its rarity color.
func logItemName(it items.Item) logPart { return logStyled{it.Name, rarityColor(itemRarity(it))} }

// logKillXP shows the base reward/share, before personal Learning bonuses.
// The same format applies in chambers until a cap reduces active rewards.
func (g *MMGame) logKillXP(xp int) string {
	return formatKillXP(xp, g.xpShare(xp), g.party != nil && len(g.party.Members) >= 2)
}

func formatKillXP(xp, share int, multiple bool) string {
	switch {
	case xp <= 0:
		return ""
	case !multiple:
		return uitext.Text("combat.kill_xp", xp)
	}
	return uitext.Text("combat.kill_xp_each", xp, share)
}

func (g *MMGame) logMonsterKillXP(m *monsterPkg.Monster3D, xp int) logPart {
	suffix := g.logKillXP(xp)
	preview := g.adventureKillExperience(m, xp)
	if !preview.chamber {
		return logStyled{suffix, combatMessageGold}
	}
	if preview.activeLimited {
		// Near the cap, nominal "each" would advertise XP some heroes cannot
		// receive. Show actual active shares, not the sum over every roster.
		total, same, share := 0, true, 0
		parts := make([]string, 0, len(preview.activeShares))
		for i, award := range preview.activeShares {
			if i == 0 {
				share = award.amount
			}
			same = same && award.amount == share
			total += award.amount
			parts = append(parts, uitext.Text("combat.kill_xp_hero", award.hero.Name, award.amount))
		}
		suffix = formatKillXP(total, share, len(parts) >= 2)
		if total > 0 && !same {
			suffix = uitext.Text("combat.kill_xp_partial", total, strings.Join(parts, ", "))
		}
	}
	if preview.atLimit > 0 {
		if preview.atLimit == preview.recipients {
			suffix += uitext.Text("adventure.xp_limit")
		} else {
			suffix += uitext.Text("adventure.xp_limit_some")
		}
	}
	return logStyled{suffix, combatMessageGold}
}

// logCrit is the critical-hit prefix, empty for a normal hit.
func logCrit(crit bool) logPart {
	if !crit {
		return logStyled{"", combatMessageGold}
	}
	return logStyled{critPrefix(true), combatMessageGold}
}

// logStatusLength is how long a status lasts, in the clock the fight runs on:
// " (2 turns)" in turn-based mode, " (3s)" in real time.
func (g *MMGame) logStatusLength(turns, frames int) logPart {
	text := ""
	switch {
	case g.turnBasedMode && turns == 1:
		text = " (1 turn)"
	case g.turnBasedMode && turns > 1:
		text = fmt.Sprintf(" (%d turns)", turns)
	case !g.turnBasedMode && frames > 0:
		text = fmt.Sprintf(" (%ds)", int(math.Ceil(float64(frames)/float64(g.config.GetTPS()))))
	}
	return logStyled{text, logMuted}
}

// logSegments formats format with args: logPart arguments keep their
// pieces, the rest prints plain with its verb.
func logSegments(format string, args ...any) []coloredTextSegment {
	var segs []coloredTextSegment
	add := func(text string, col color.Color) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && sameColor(segs[n-1].color, col) {
			segs[n-1].text += text
			return
		}
		segs = append(segs, coloredTextSegment{text, col})
	}
	next := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			j := strings.IndexByte(format[i:], '%')
			if j < 0 {
				j = len(format) - i
			}
			add(format[i:i+j], logPlain)
			i += j - 1
			continue
		}
		end := i + 1
		for end < len(format) && strings.IndexByte("+-# 0123456789.", format[end]) >= 0 {
			end++
		}
		if end >= len(format) {
			add(format[i:], logPlain)
			break
		}
		verb := format[i : end+1]
		i = end
		if verb == "%%" {
			add("%", logPlain)
			continue
		}
		if next >= len(args) {
			add("%!"+verb[len(verb)-1:]+"(MISSING)", logPlain)
			continue
		}
		arg := args[next]
		next++
		if part, ok := arg.(logPart); ok {
			for _, s := range part.logSegments() {
				add(s.text, s.color)
			}
			continue
		}
		add(fmt.Sprintf(verb, arg), logPlain)
	}
	return segs
}

// logCombat appends a styled line to the combat log.
func (g *MMGame) logCombat(tone logTone, format string, args ...any) {
	segs := logSegments(format, args...)
	text := ""
	for _, s := range segs {
		text += s.text
	}
	g.appendCombatLog(combatLogEntry{Text: text, Color: logPlain, Segments: segs, Tone: tone})
}

// logTurnDivider marks where a turn-based side starts acting. A divider with
// nothing after it gives way to the next, so empty turns leave no trail.
func (g *MMGame) logTurnDivider(label string) {
	entry := combatLogEntry{Text: "-- " + label + " --", Color: logMuted, Divider: true}
	if n := len(g.combatLogHistory); n > 0 && g.combatLogHistory[n-1].Divider {
		g.combatLogVersion++
		g.combatLogHistory[n-1] = entry
		return
	}
	g.appendCombatLog(entry)
}

// logLineSegments is an entry's pieces: its styled segments, or its whole
// text in its one color.
func (e combatLogEntry) logLineSegments() []coloredTextSegment {
	if len(e.Segments) > 0 {
		return e.Segments
	}
	return []coloredTextSegment{{e.Text, e.Color}}
}

// wrapLogEntry wraps an entry to maxW with wrapUIText's breaks, each line
// keeping its pieces' colors.
func wrapLogEntry(e combatLogEntry, maxW int) []combatLogEntry {
	var lines []combatLogEntry
	for _, segs := range wrapStyled(styledRuns(e.logLineSegments()), maxW) {
		line := combatLogEntry{Color: e.Color, Tone: e.Tone, Divider: e.Divider}
		for _, r := range segs {
			line.Text += r.text
			line.Segments = append(line.Segments, coloredTextSegment{r.text, r.style})
		}
		lines = append(lines, line)
	}
	return lines
}

func styledRuns(segs []coloredTextSegment) []styledRun[color.Color] {
	runs := make([]styledRun[color.Color], len(segs))
	for i, s := range segs {
		runs[i] = styledRun[color.Color]{s.text, s.color}
	}
	return runs
}

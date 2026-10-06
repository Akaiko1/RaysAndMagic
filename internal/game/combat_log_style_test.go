package game

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/damage"
	"ugataima/internal/monster"
)

// A log piece is colored by who it names and what it carries: heroes, allies,
// neutrals and enemies apart; damage in its school; heals, HP and true damage
// in their own.
func TestCombatLogPiecesColorWhoAndWhat(t *testing.T) {
	foe := &monster.Monster3D{Name: "Ocelot"}
	summon := &monster.Monster3D{Name: "Wolf", SummonedBy: spellSummonOwnerPrefix + "summon_wolf"}
	bound := &monster.Monster3D{Name: "Skeleton", Bound: true}
	wild := &monster.Monster3D{Name: "Rabbit", Disposition: monster.DispositionWildlife}
	charmed := &monster.Monster3D{Name: "Kappa", Pacified: true}
	rows := map[string]struct {
		part logPart
		want []coloredTextSegment
	}{
		"hero":              {logHeroName(&character.MMCharacter{Name: "Mara"}), []coloredTextSegment{{"Mara", logHero}}},
		"party by name":     {logHeroText("The party"), []coloredTextSegment{{"The party", logHero}}},
		"enemy":             {logMonsterName(foe), []coloredTextSegment{{"Ocelot", logEnemy}}},
		"party summon":      {logMonsterName(summon), []coloredTextSegment{{"Wolf", logAlly}}},
		"bound undead":      {logMonsterName(bound), []coloredTextSegment{{"Skeleton", logAlly}}},
		"wildlife":          {logMonsterName(wild), []coloredTextSegment{{"Rabbit", logNeutral}}},
		"charmed":           {logMonsterName(charmed), []coloredTextSegment{{"Kappa", logNeutral}}},
		"enemy's breath":    {logMonsterAs(foe, "Ocelot's Breath"), []coloredTextSegment{{"Ocelot's Breath", logEnemy}}},
		"true damage":       {logTrueDamage(12), []coloredTextSegment{{"12", logTrue}}},
		"unknown school":    {logDamage(7, ""), []coloredTextSegment{{"7", SchoolColor("")}}},
		"heal":              {logHealed(35), []coloredTextSegment{{"35", keywordColors["heal"]}}},
		"spell points":      {logRecoveredSP(9), []coloredTextSegment{{"9", keywordColors["buff"]}}},
		"no crit":           {logCrit(false), []coloredTextSegment{{"", combatMessageGold}}},
		"crit":              {logCrit(true), []coloredTextSegment{{"Critical! ", combatMessageGold}}},
		"HP full":           {logHP(100, 100), hpReadout("100", logPlain, "/100)")},
		"HP over half":      {logHP(51, 100), hpReadout("51", logPlain, "/100)")},
		"HP at half":        {logHP(50, 100), hpReadout("50", logLowHP, "/100)")},
		"HP at a quarter":   {logHP(25, 100), hpReadout("25", logDyingHP, "/100)")},
		"HP gone":           {logHP(0, 100), hpReadout("0", logDyingHP, "/100)")},
		"joined pieces":     {logParts(logHeroText("Mara's "), logSchoolWord("fire", "Fireball")), []coloredTextSegment{{"Mara's ", logHero}, {"Fireball", SchoolColor("fire")}}},
		"control keyword":   {logKeyword("control", "stunned"), []coloredTextSegment{{"stunned", keywordColors["control"]}}},
		"ability":           {logAbility("Piercing Shot"), []coloredTextSegment{{"Piercing Shot", keywordColors["auto"]}}},
		"poison":            {logColored("poisoned", logPoison), []coloredTextSegment{{"poisoned", logPoison}}},
		"school word":       {logSchoolWord("water", "Ice Bolt"), []coloredTextSegment{{"Ice Bolt", SchoolColor("water")}}},
		"physical is white": {logDamage(53, "physical"), []coloredTextSegment{{"53", logTrue}}},
		"physical word":     {logSchoolWord("physical", "Blowgun"), []coloredTextSegment{{"Blowgun", logTrue}}},
		"nil hero is blank": {logHeroName(nil), []coloredTextSegment{{"", logHero}}},
	}
	for _, school := range damage.Types() {
		rows["damage in "+school.String()] = struct {
			part logPart
			want []coloredTextSegment
		}{logDamage(42, school.String()), []coloredTextSegment{{"42", logSchoolColor(school.String())}}}
	}
	for name, row := range rows {
		t.Run(name, func(t *testing.T) {
			got := row.part.logSegments()
			if len(got) != len(row.want) {
				t.Fatalf("pieces %v, want %v", got, row.want)
			}
			for i := range got {
				if got[i].text != row.want[i].text || !sameColor(got[i].color, row.want[i].color) {
					t.Errorf("piece %d is %q in %v, want %q in %v", i, got[i].text, got[i].color, row.want[i].text, row.want[i].color)
				}
			}
		})
	}
}

func hpReadout(cur string, col color.Color, rest string) []coloredTextSegment {
	return []coloredTextSegment{{"(HP: ", logMuted}, {cur, col}, {rest, logMuted}}
}

// Allies, neutrals and enemies read apart; each side reads apart from the
// prose, and damage numbers never share a side's color.
func TestCombatLogSidesStandApart(t *testing.T) {
	sides := map[string]color.RGBA{"hero": logHero, "ally": logAlly, "neutral": logNeutral, "enemy": logEnemy}
	for name, c := range sides {
		if d := rgbDistance(c, logPlain); d < 60 {
			t.Errorf("%s is %.0f from the prose, want 60+", name, d)
		}
		for other, o := range sides {
			if name < other && rgbDistance(c, o) < 60 {
				t.Errorf("%s and %s are %.0f apart, want 60+", name, other, rgbDistance(c, o))
			}
		}
	}
}

func rgbDistance(a, b color.Color) float64 {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	dr, dg, db := float64(ar>>8)-float64(br>>8), float64(ag>>8)-float64(bg>>8), float64(ab>>8)-float64(bb>>8)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// The formatter prints plain arguments with their verbs, keeps styled ones,
// and its text is exactly what fmt would print.
func TestLogSegmentsFormatLikeSprintf(t *testing.T) {
	hero := &character.MMCharacter{Name: "Mara"}
	for name, tc := range map[string]struct {
		format string
		args   []any
		text   string
	}{
		"plain verbs":   {"%s hits %d times at %.1f%%", []any{"Mara", 3, 12.5}, fmt.Sprintf("%s hits %d times at %.1f%%", "Mara", 3, 12.5)},
		"styled parts":  {"%s hits %s for %s damage! %s", []any{logHeroName(hero), "Ocelot", logDamage(42, "fire"), logHP(18, 60)}, "Mara hits Ocelot for 42 damage! (HP: 18/60)"},
		"no verbs":      {"Nothing happens.", nil, "Nothing happens."},
		"missing arg":   {"%s and %d", []any{"Mara"}, "Mara and %!d(MISSING)"},
		"trailing per":  {"100%", nil, "100%"},
		"empty styled":  {"%s%s hits", []any{logCrit(false), logHeroName(hero)}, "Mara hits"},
		"width and pad": {"[%3d|%-4s]", []any{7, "ab"}, fmt.Sprintf("[%3d|%-4s]", 7, "ab")},
	} {
		t.Run(name, func(t *testing.T) {
			text := ""
			for _, s := range logSegments(tc.format, tc.args...) {
				text += s.text
			}
			if text != tc.text {
				t.Errorf("text %q, want %q", text, tc.text)
			}
		})
	}
	segs := logSegments("%s hits %s", logHeroName(hero), "Ocelot")
	if len(segs) != 2 || segs[0].text != "Mara" || segs[1].text != " hits Ocelot" || !sameColor(segs[1].color, logPlain) {
		t.Errorf("plain text after a styled piece should merge into one plain piece: %v", segs)
	}
}

// A styled line wraps with exactly wrapUIText's breaks, so color never moves
// a line, and every character keeps the color of the piece it came from.
func TestStyledLinesWrapLikePlainText(t *testing.T) {
	lines := [][]coloredTextSegment{
		{{"Gareth", logHero}, {" hits ", logPlain}, {"Ancient God of Death", logEnemy}, {" for ", logPlain}, {"1234", SchoolColor("dark")}, {" damage and kills it! ", logPlain}, {"+5559 XP", combatMessageGold}},
		{{"Critical! ", combatMessageGold}, {"Silvelyn's Arbalest", SchoolColor("physical")}, {" hit ", logPlain}, {"Elder Dragon", logEnemy}, {" for 88 physical damage! (HP: 1/3600)", logPlain}},
		{{"Supercalifragilisticexpialidociousandthensomemoretext", logEnemy}, {" falls  down   hard", logPlain}},
		{{"short", logHero}},
		{{"", logHero}, {"   leading spaces", logPlain}},
	}
	forEachUIFont(t, func(t *testing.T) {
		for _, width := range []int{60, 120, hudMessageWidth - 10, 646} {
			for _, segs := range lines {
				plain, colors := "", []color.Color{}
				for _, s := range segs {
					plain += s.text
					for range len(s.text) {
						colors = append(colors, s.color)
					}
				}
				got := wrapLogEntry(combatLogEntry{Text: plain, Segments: segs}, width)
				want := wrapUIText(plain, width)
				if len(got) != len(want) {
					t.Fatalf("width %d: %d lines, plain wraps to %d: %q", width, len(got), len(want), want)
				}
				at := 0
				for i, line := range got {
					if line.Text != want[i] {
						t.Fatalf("width %d line %d: %q, plain %q", width, i, line.Text, want[i])
					}
					for _, s := range line.Segments {
						for k := 0; k < len(s.text); k++ {
							for at < len(plain) && plain[at] != s.text[k] {
								at++
							}
							if at >= len(plain) || !sameColor(colors[at], s.color) {
								t.Fatalf("width %d: %q in line %q lost its color", width, s.text, line.Text)
							}
							at++
						}
					}
				}
			}
		}
	})
}

// Turn-based fights mark each side's turn in the game log. A marker with
// nothing after it gives way to the next; the HUD skips markers; entering the
// mode opens with its own line instead.
func TestTurnDividersMarkEachSidesTurn(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.maxMessages = 4
	g.ToggleTurnBasedMode()
	texts := func() []string {
		var out []string
		for _, e := range g.combatLogHistory {
			out = append(out, e.Text)
		}
		return out
	}
	if last := texts()[len(texts())-1]; last != "Turn-based mode activated!" {
		t.Fatalf("entering turn-based mode ends with %q, want the mode line and no marker", last)
	}
	g.logCombat(logToneGood, "%s hits %s", logHeroText("Mara"), "Ocelot")
	g.endPartyTurn()
	g.logCombat(logToneBad, "%s hits %s", "Ocelot", logHeroText("Mara"))
	g.startPartyTurn()
	g.endPartyTurn() // nothing happened in this party turn
	g.startPartyTurn()
	got := texts()
	want := []string{"Turn-based mode activated!", "Mara hits Ocelot", "-- Enemy turn --", "Ocelot hits Mara", "-- Party turn --"}
	if tail := got[len(got)-len(want):]; strings.Join(tail, "|") != strings.Join(want, "|") {
		t.Errorf("log %q, want it to end %q", got, want)
	}
	for _, text := range g.GetCombatMessages() {
		if strings.HasPrefix(text, "--") {
			t.Errorf("the HUD shows the marker %q", text)
		}
	}
	if hud := g.GetCombatMessages(); len(hud) == 0 || hud[len(hud)-1] != "Ocelot hits Mara" {
		t.Errorf("HUD %q, want the last real lines", hud)
	}
	for _, line := range g.hudMessageLines() {
		if line.Divider {
			t.Errorf("a HUD line is a marker: %q", line.Text)
		}
	}
}

// The HUD stripe left of each line shows what the line means for the party;
// a line without a tone has none, and legacy lines keep their single color.
func TestHUDLogDrawsToneStripes(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.maxMessages = 4
	g.logCombat(logToneGood, "%s hits %s", logHeroText("Mara"), "Ocelot")
	g.logCombat(logToneBad, "%s hits %s", "Ocelot", logHeroText("Mara"))
	g.logCombat(logToneReward, "Found %s!", "a coin")
	g.AddCombatMessage("Saved to slot 1")
	h.ui.Draw(h.screen)
	lines := g.hudMessageLines()
	lx, ly, _, _ := g.hudMessageLogRect(len(lines))
	for i, want := range []color.Color{logToneStripes[logToneGood], logToneStripes[logToneBad], logToneStripes[logToneReward], nil} {
		y := ly + 5 + i*hudMessageSpacing + uiTextCharHeight/2
		got := h.screen.At(lx+2, y)
		if want != nil && !sameColor(got, want) {
			t.Errorf("line %d stripe is %v, want %v", i, got, want)
		}
		if want == nil {
			if r, gg, b, _ := got.RGBA(); r>>8 > 40 || gg>>8 > 40 || b>>8 > 40 {
				t.Errorf("line %d has a stripe %v, want none", i, got)
			}
		}
	}
}

// Lines take their tone from what happened: a party ally striking is good,
// one struck is bad, a fight between two foes and a mode switch carry none,
// and a level-up is a reward.
func TestLogLinesCarryTheirTone(t *testing.T) {
	cfg := loadTestConfig(t)
	ally := &monster.Monster3D{Name: "Wolf", SummonedBy: spellSummonOwnerPrefix + "summon_wolf"}
	foe := &monster.Monster3D{Name: "Ocelot"}
	rival := &monster.Monster3D{Name: "Kappa"}
	bare := func() *MMGame {
		g := &MMGame{config: cfg}
		g.combat = NewCombatSystem(g)
		return g
	}
	fight := func(attacker, target *monster.Monster3D) func(t *testing.T) *MMGame {
		return func(t *testing.T) *MMGame {
			g := bare()
			g.addActorCombatMessage(attacker, target, "%s strikes %s for %s!", logMonsterName(attacker), logMonsterName(target), logDamage(9, "physical"))
			return g
		}
	}
	for name, tc := range map[string]struct {
		act  func(t *testing.T) *MMGame
		text string
		tone logTone
	}{
		"ally strikes": {fight(ally, foe), "Wolf strikes Ocelot for 9!", logToneGood},
		"ally struck":  {fight(foe, ally), "Ocelot strikes Wolf for 9!", logToneBad},
		"two foes":     {fight(foe, rival), "Ocelot strikes Kappa for 9!", logToneNone},
		"level-up": {func(t *testing.T) *MMGame {
			g := bare()
			hero := character.CreateCharacter("Mara", character.ClassKnight, cfg)
			hero.Level, hero.Experience, hero.HitPoints = 1, 99, hero.MaxHitPoints
			g.party = &character.Party{Members: []*character.MMCharacter{hero}}
			g.grantSharedXP(5)
			return g
		}, "Mara reached level 2! (was level 1) [+5 stat points]", logToneReward},
		"mode switch": {func(t *testing.T) *MMGame {
			g, _ := newThiefTestGame(t)
			g.ToggleTurnBasedMode()
			return g
		}, "Turn-based mode activated!", logToneNone},
	} {
		t.Run(name, func(t *testing.T) {
			g := tc.act(t)
			if len(g.combatLogHistory) == 0 {
				t.Fatal("nothing logged")
			}
			if e := g.combatLogHistory[len(g.combatLogHistory)-1]; e.Text != tc.text || e.Tone != tc.tone {
				t.Errorf("logged %q with tone %d, want %q with tone %d", e.Text, e.Tone, tc.text, tc.tone)
			}
		})
	}
}

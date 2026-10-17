package game

import (
	"time"

	"ugataima/internal/character"
	"ugataima/internal/highscore"
	"ugataima/internal/items"
)

// GetScoreData collects score-input data from the current game state.
func (g *MMGame) GetScoreData() highscore.ScoreData {
	avgLevel := g.party.AverageLevel()

	playTime := time.Since(g.sessionStartTime)
	if !g.victoryTime.IsZero() {
		playTime = g.victoryTime.Sub(g.sessionStartTime)
	}

	return highscore.ScoreData{
		TotalExperience: g.totalExperienceEarned,
		Gold:            g.totalGoldEarned,
		AverageLevel:    avgLevel,
		PlayTime:        playTime,
	}
}

// highScoreParty is the party as the run ended, for its high score record:
// each hero and every piece they wear.
func (g *MMGame) highScoreParty() []highscore.Hero {
	var heroes []highscore.Hero
	for _, m := range g.party.Members {
		if m == nil {
			continue
		}
		hero := highscore.Hero{Name: m.Name, Class: m.Class.Key(), Level: m.Level, Portrait: g.portraitSpriteName(m)}
		for _, slot := range items.DisplayEquipSlots {
			if it, ok := m.Equipment[slot]; ok {
				hero.Gear = append(hero.Gear, it)
			}
		}
		heroes = append(heroes, hero)
	}
	return heroes
}

func (g *MMGame) awardGold(amount int) {
	if amount <= 0 || g.party == nil {
		return
	}
	g.party.Gold += amount
	g.totalGoldEarned += amount
	g.profileAdd("gold", int64(amount))
	g.playSound(soundCoins)
}

// awardArenaPoints is the single crediting path for the arena victory currency
// (crate jackpots, champion kills) - the counterpart to awardGold, so any future
// telemetry or guard lives in one place instead of scattered raw increments.
func (g *MMGame) awardArenaPoints(amount int) {
	if amount <= 0 || g.party == nil {
		return
	}
	g.party.ArenaPoints += amount
}

// xpStepCost is the experience required to advance FROM `level` to level+1:
// the classic linear 100 x L early, the quadratic branch from L13 up (see
// XPQuadPerLevel for why). Single source for the level gate and the score
// reconstruction below.
func xpStepCost(level int) int {
	linear := XPRequiredPerLevel * level
	if quad := XPQuadPerLevel * level * level; quad > linear {
		return quad
	}
	return linear
}

// xpSpentToReach sums the step costs from level 1 up to `level` - the XP a
// character has already paid into level-ups (their Experience field holds only
// the remainder toward the next level).
func xpSpentToReach(level int) int {
	spent := 0
	for l := 1; l < level; l++ {
		spent += xpStepCost(l)
	}
	return spent
}

func earnedExperienceForCharacter(level, remaining int) int {
	if level < 1 {
		level = 1
	}
	if remaining < 0 {
		remaining = 0
	}
	return xpSpentToReach(level) + remaining
}

func earnedExperienceForParty(p *character.Party) int {
	if p == nil {
		return 0
	}
	total := 0
	add := func(m *character.MMCharacter) {
		if m != nil {
			total += earnedExperienceForCharacter(m.Level, m.Experience)
		}
	}
	for _, m := range p.Members {
		add(m)
	}
	for _, m := range p.Reserve {
		add(m)
	}
	for _, m := range p.Captive {
		add(m)
	}
	return total
}

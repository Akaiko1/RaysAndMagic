package game

// Both NPC presence and ambient packs use the highest average active-party
// level reached in this playthrough. Roster changes cannot revoke an unlock.
func (g *MMGame) unlockedPartyLevel() int {
	return max(g.maxPartyLevel, g.party.AverageLevel())
}

func (g *MMGame) updatePartyLevelUnlocks() {
	g.maxPartyLevel = g.unlockedPartyLevel()
}

func (g *MMGame) partyLevelUnlocked(required int) bool {
	return required <= g.unlockedPartyLevel()
}

package game

import (
	"math"

	"ugataima/internal/character"
)

// restParty fully restores every living member's HP/SP and wakes the
// unconscious. The dead and eradicated stay down — revival is a separate rite.
func (g *MMGame) restParty() {
	for i, m := range g.party.Members {
		if m == nil || m.HasCondition(character.ConditionDead) || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		healed := m.HitPoints < m.MaxHitPoints
		m.HitPoints = m.MaxHitPoints
		m.SpellPoints = m.MaxSpellPoints
		m.RemoveCondition(character.ConditionUnconscious)
		if healed {
			g.TriggerPartyHeal(i) // same rising green "+" the heal spells show
		}
	}
}

// TryCamp spends CampFoodCost food to rest in the field (full HP/SP), refused
// while any living monster prowls within CampEnemyRadiusTiles of the party or
// when the larder is empty. Returns the message to show and whether it worked.
func (g *MMGame) TryCamp() (string, bool) {
	if g.party.Food < CampFoodCost {
		return "Not enough food to make camp.", false
	}
	radius := CampEnemyRadiusTiles * float64(g.config.World.TileSize)
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		// No resting mid-combat: a pursuer kited beyond the radius (or a
		// ranged monster shooting from outside it) still blocks the camp.
		if m.IsEngagingPlayer {
			return "Enemies are upon you - you cannot rest mid-fight.", false
		}
		// Measure to the monster's box EDGE, not its center — a large monster
		// whose body pokes into the radius counts as near.
		mw, mh := m.GetSize()
		if math.Hypot(m.X-g.camera.X, m.Y-g.camera.Y) <= radius+math.Max(mw, mh)/2 {
			return "Enemies are near - you cannot rest here.", false
		}
	}
	g.party.Food -= CampFoodCost
	g.restParty()
	return "The party rests. HP and spell points fully restored.", true
}

// applyPartyStatBonuses pushes the aggregate buff bonuses (g.statBonuses) onto
// every active member and re-derives MaxHP/MaxSP preserving current values.
// MUST be called after every change to g.statBonuses — it is what makes buffs
// behave like real stats everywhere (combat formulas AND HP/SP maxima).
func (g *MMGame) applyPartyStatBonuses() {
	for _, m := range g.party.Members {
		if m == nil {
			continue
		}
		m.BuffBonuses = g.statBonuses
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}
}

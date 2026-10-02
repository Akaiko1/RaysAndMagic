package game

import (
	"math"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"
	"ugataima/internal/monster"
)

// restParty cures afflictions, fully restores every living member's HP/SP and wakes the
// unconscious. The dead and eradicated stay down - revival is a separate rite.
func (g *MMGame) restParty() {
	g.partyRoot = PartyRootState{}
	for i, m := range g.party.Members {
		if m == nil || m.HasCondition(character.ConditionDead) || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		healed := m.HitPoints < m.MaxHitPoints
		m.HitPoints = m.MaxHitPoints
		m.SpellPoints = m.MaxSpellPoints
		m.CureRestConditions()
		m.RareClass.Anchor = character.SpatialAnchor{}
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
		return uitext.Text("ui.camp_no_food"), false
	}
	if reason, safe := g.safeToPrepare(campActivity()); !safe {
		return reason, false
	}
	g.party.Food -= CampFoodCost
	g.restParty()
	return uitext.Text("ui.camp_rested"), true
}

// applyPartyStatBonuses pushes the aggregate buff bonuses (g.statBonuses) onto
// every active member and re-derives MaxHP/MaxSP preserving current values.
// MUST be called after every change to g.statBonuses - it is what makes buffs
// behave like real stats everywhere (combat formulas AND HP/SP maxima).
func (g *MMGame) applyPartyStatBonuses() {
	for _, m := range g.party.Members {
		if m == nil {
			continue
		}
		m.BuffBonuses = g.statBonuses
		m.BonusMaxHP = g.cardMaxHPBonus()  // Jungle Idol Card: flat party max HP
		m.BonusRegenPct = g.cardRegenPct() // Troll Card(s): % max HP regen per tick
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}
}

// fieldActivity holds one field action's refusal texts; the threat rule is
// shared, the wording belongs to the action.
type fieldActivity struct{ inCombat, enemiesNear string }

func campActivity() fieldActivity {
	return fieldActivity{uitext.Text("ui.camp_in_combat"), uitext.Text("ui.camp_enemies_near")}
}

func brewActivity() fieldActivity {
	return fieldActivity{uitext.Text("ui.brew_in_combat"), uitext.Text("ui.brew_enemies_near")}
}

// safeToPrepare is the one safe-field rule for camping and brewing, without
// spending food or restoring resources. Inventory pause never makes an unsafe
// field suitable for either.
func (g *MMGame) safeToPrepare(activity fieldActivity) (string, bool) {
	if g.world == nil || g.camera == nil || g.config == nil {
		return "The field is not ready.", false
	}
	radius := CampEnemyRadiusTiles * float64(g.config.World.TileSize)
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		// Bound covers every summon plus Bind Undead - real allies. NOT
		// IsPartyControlled: that adds Charm, a countdown that breaks on any hit,
		// so resting through it banks a full heal before the monster turns.
		if m.Bound || m.IsAmbient() {
			continue
		}
		// No resting mid-combat: a pursuer kited beyond the radius (or a
		// ranged monster shooting from outside it) still blocks the camp.
		if g.monsterPressesParty(m) {
			return activity.inCombat, false
		}
		// Measure to the monster's box EDGE, not its center - a large monster
		// whose body pokes into the radius counts as near.
		mw, mh := m.GetSize()
		if math.Hypot(m.X-g.camera.X, m.Y-g.camera.Y) <= radius+math.Max(mw, mh)/2 {
			return activity.enemiesNear, false
		}
	}
	return "", true
}

// monsterPressesParty is the "party is in a fight" scope shared by camping,
// brewing, automatic techniques and scale shedding. Allies never press.
func (g *MMGame) monsterPressesParty(m *monster.Monster3D) bool {
	return m != nil && m.IsAlive() && !m.Bound && g.camera != nil && m.PressesParty(g.camera.X, g.camera.Y)
}

package game

import (
	"math"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/monster"
)

func (g *MMGame) grantMonsterXP(m *monster.Monster3D, amount int) {
	key := m.HomeMap
	if key == "" {
		key = currentMapKey()
	}
	g.grantSharedXPFromMap(amount, key)
}

// adventureExperienceBudget reads the current run without creating a visit.
// A new generation replenishes each hero's monster-XP budget independently.
func (g *MMGame) adventureExperienceBudget(hero *character.MMCharacter, key string) (character.AdventureExperience, int) {
	a := g.adventureConfig(key)
	if key == "" || a == nil || a.ExperienceCap <= 0 {
		return character.AdventureExperience{}, 0
	}
	generation := 0
	if v := g.adventure.Visits[key]; v != nil {
		generation = v.Generation
	}
	earned := hero.AdventureXP[key]
	if earned.Generation != generation {
		earned = character.AdventureExperience{Generation: generation}
	}
	return earned, a.ExperienceCap
}

// remainingAdventureExperience is pure: previews never spend a budget.
func (g *MMGame) remainingAdventureExperience(hero *character.MMCharacter, key string, amount int) int {
	earned, cap := g.adventureExperienceBudget(hero, key)
	if cap <= 0 {
		return amount
	}
	return min(amount, max(0, cap-earned.Amount))
}

func (g *MMGame) capAdventureExperience(hero *character.MMCharacter, key string, amount int) int {
	earned, cap := g.adventureExperienceBudget(hero, key)
	if cap <= 0 {
		return amount
	}
	gain := min(amount, max(0, cap-earned.Amount))
	if hero.AdventureXP == nil {
		hero.AdventureXP = map[string]character.AdventureExperience{}
	}
	earned.Amount += gain
	hero.AdventureXP[key] = earned
	return gain
}

func (g *MMGame) adventureKillExperience(m *monster.Monster3D, xp int) (int, bool) {
	key := m.HomeMap
	if key == "" {
		key = currentMapKey()
	}
	a := g.adventureConfig(key)
	if a == nil || a.ExperienceCap <= 0 || g.party == nil {
		return xp, false
	}
	amount, teacher, total := g.xpShare(xp), g.learningTeacherBonusPct(), 0
	g.forEachLivingXPRecipient(func(hero *character.MMCharacter, _ bool) {
		gain := experienceWithLearning(hero, amount, teacher)
		total += g.remainingAdventureExperience(hero, key, gain)
	})
	return total, true
}

func (g *MMGame) adventureResetAt(key string, v *AdventureVisit) float64 {
	if v == nil || !v.BossGranted {
		return 0
	}
	a := g.adventureConfig(key)
	if a == nil || a.ResetDays == 0 {
		return 0
	}
	return v.VictoryAt + float64(a.ResetDays)
}

func (g *MMGame) adventureAvailable(key string, at float64) bool {
	v := g.adventure.Visits[key]
	// Ignore sub-tick floating-point noise when converting legacy deadlines.
	return v == nil || !v.BossGranted || at+1e-9 >= g.adventureResetAt(key, v)
}

func (g *MMGame) adventureArenaBounds() collision.MovementBounds {
	key := currentMapKey()
	a, v := g.adventureConfig(key), g.adventure.Visits[key]
	if a == nil || !a.OpeningOwned || a.Boss == nil || a.Boss.Arena == nil || v == nil || !v.ArenaLocked || v.BossGranted {
		return collision.MovementBounds{}
	}
	boss := g.adventureBoss(a)
	if boss == nil || !boss.IsAlive() {
		return collision.MovementBounds{}
	}
	r, ts := a.Boss.Arena, float64(g.config.GetTileSize())
	return collision.MovementBounds{Enabled: true, MinX: float64(r[0]) * ts, MinY: float64(r[1]) * ts, MaxX: float64(r[2]+1) * ts, MaxY: float64(r[3]+1) * ts}
}

func (g *MMGame) syncAdventureArenaBounds() {
	if g.collisionSystem != nil {
		g.collisionSystem.SetMovementBounds("player", g.adventureArenaBounds())
	}
}

func (g *MMGame) adventureArenaAllows(x, y float64) bool {
	return g.adventureArenaBounds().Allows(collision.NewBoundingBox(x, y, partyCollisionBoxSize, partyCollisionBoxSize))
}

func (g *MMGame) announceAdventureArenaBarrier() {
	if g.arenaBarrierMessageAfter > g.frameCount {
		return
	}
	g.arenaBarrierMessageAfter = g.frameCount + int64(max(1, g.config.GetTPS())*3)
	g.AddCombatMessage(uitext.Text("adventure.arena_barrier"))
}

// Collision owns movement legality. Searches and sliding probes have no UI effects.
func (g *MMGame) canMovePartyTo(x, y float64) bool {
	return g.collisionSystem != nil && g.collisionSystem.CanMoveTo("player", x, y)
}

// Called only after an actual movement command has failed, never during probes.
func (g *MMGame) announceBlockedPartyMove(x, y float64) {
	if !g.adventureArenaAllows(x, y) {
		g.announceAdventureArenaBarrier()
	}
}

// Runs only at serial AI boundaries, never inside a damage packet. The lock
// survives disengagement and save/load; Town Portal explicitly ends the attempt.
func (g *MMGame) updateAdventureArena() {
	key := currentMapKey()
	a := g.adventureConfig(key)
	if a == nil || !a.OpeningOwned || a.Boss == nil || a.Boss.Arena == nil || g.camera == nil {
		g.syncAdventureArenaBounds()
		return
	}
	boss := g.adventureBoss(a)
	if boss == nil || !boss.IsAlive() {
		g.syncAdventureArenaBounds()
		return
	}
	v := g.adventure.Visits[key]
	locked := v != nil && v.ArenaLocked && !v.BossGranted
	if !locked && (!boss.IsInCombat() && !boss.CanStartPlayerEngagement(g.collisionSystem, g.camera.X, g.camera.Y) || boss.IsPartyControlled() || boss.IsInertSetPiece()) {
		return
	}
	v = g.adventureVisit(key)
	if v.BossGranted {
		return
	}
	if locked && g.adventureArenaAllows(g.camera.X, g.camera.Y) {
		g.syncAdventureArenaBounds()
		return
	}
	// Use the nearest safe tile to the geometric center when the center is
	// occupied or hazardous. Never pull the party into a monster or live trap.
	r := a.Boss.Arena
	cx, cy := float64(r[0]+r[2])/2, float64(r[1]+r[3])/2
	best, bx, by := math.Inf(1), 0, 0
	for y := r[1]; y <= r[3]; y++ {
		for x := r[0]; x <= r[2]; x++ {
			d := math.Pow(float64(x)-cx, 2) + math.Pow(float64(y)-cy, 2)
			if d < best && g.safeForcedArrival(x, y, "") {
				best, bx, by = d, x, y
			}
		}
	}
	if math.IsInf(best, 1) {
		// Even a legacy lock outside the arena must not strand the party.
		v.ArenaLocked = false
		g.syncAdventureArenaBounds()
		return // Retry at the next AI boundary when a safe tile becomes free.
	}
	x, y := TileCenterFromTile(bx, by, float64(g.config.GetTileSize()))
	g.setPartyPosition(x, y)
	v.ArenaLocked = true
	g.syncAdventureArenaBounds()
	g.relocateTravelAllies()
	boss.BeginPlayerEngagement()
	g.AddCombatMessage(uitext.Text("adventure.arena_pull"))
}

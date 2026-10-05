package game

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func (v *AdventureVisit) effectState(id string) *EnvironmentState {
	s := v.Effects[id]
	if s == nil {
		s = &EnvironmentState{}
		v.Effects[id] = s
	}
	return s
}
func (g *MMGame) adventureBoss(a *config.AdventureConfig) *monster.Monster3D {
	if a == nil || a.Boss == nil || g.world == nil {
		return nil
	}
	for _, m := range g.world.Monsters {
		if m != nil && m.Key == a.Boss.Monster && m.SummonedBy == "" {
			return m
		}
	}
	return nil
}
func (g *MMGame) effectEnabled(a *config.AdventureConfig, v *AdventureVisit, e config.EnvironmentEffect) bool {
	if e.RequiresControl != "" && !v.Controls[e.RequiresControl] {
		return false
	}
	if e.TriggerLane != "" {
		lane := v.Effects[e.TriggerLane]
		if lane == nil || (lane.Warning <= 0 && lane.Remaining <= 0) {
			return false
		}
	}
	if state := v.Effects[e.ID]; state != nil && state.Disarmed {
		return false
	}
	for _, c := range a.Controls {
		if v.Controls[c.ID] && slices.Contains(c.Disable, e.ID) {
			return false
		}
	}
	if e.BossBelowPercent > 0 {
		m := g.adventureBoss(a)
		if m == nil || !m.IsAlive() || m.HitPoints*100 > m.MaxHitPoints*e.BossBelowPercent {
			return false
		}
	}
	return true
}
func (g *MMGame) damageEnvironment(e config.EnvironmentEffect, ticks int) {
	if g.combat == nil || ticks <= 0 {
		return
	}
	g.combat.forEachDamageablePartyMember(func(i int, c *character.MMCharacter) {
		first, second := e.Damage, 0
		if e.SecondarySchool != "" {
			first = e.Damage / 2
			second = e.Damage - first
		}
		dealt := g.combat.damagePartyMemberElement(i, c, first*ticks, e.School, false)
		if second > 0 && c.HitPoints > 0 {
			dealt += g.combat.damagePartyMemberElement(i, c, second*ticks, e.SecondarySchool, false)
		}
		if dealt > 0 {
			g.logCombat(logToneBad, "%s hits %s for %d.", e.Name, logHeroName(c), dealt)
		}
	})
}

// Durations are fractions of an authored RT or TB budget. Switching modes
// preserves the spent fraction rather than granting a fresh warning or action.
func advanceEnvironmentBudget(value, seconds float64, rounds int, dt float64, tb bool) float64 {
	if value <= 0 {
		return 0
	}
	step := 0.0
	if tb {
		if rounds > 0 {
			step = 1 / float64(rounds)
		}
	} else if seconds > 0 {
		step = dt / seconds
	}
	return max(0, value-step)
}
func (g *MMGame) tickEnvironment(dt float64, tb bool) {
	key := currentMapKey()
	a := g.adventureConfig(key)
	if g.world == nil {
		return
	}
	if a == nil {
		g.damageEnvironmentExposures(g.adventureVisit(key), g.monsterFieldExposures(dt))
		return
	}
	v := g.adventureVisit(key)
	tx, ty := g.GetPlayerTilePosition()
	continuous := g.monsterFieldExposures(dt)
	releaseTransfers := false
	for _, e := range a.Effects {
		s := v.effectState(e.ID)
		inside := e.Contains(tx, ty)
		s.Rearm = advanceEnvironmentBudget(s.Rearm, e.RearmSeconds, e.RearmRounds, dt, tb)
		if !inside {
			s.WasInside = false
			s.Suppressed = false
			if s.Rearm == 0 {
				s.Triggered = false
			}
		}
		if !g.effectEnabled(a, v, e) {
			continue
		}
		if e.BossBelowPercent > 0 && !s.Activated {
			s.Activated = true
			s.Warning = 1
			g.AddCombatMessage(e.Name + " is charging. Leave the marked tiles!")
			continue
		}
		if e.Kind != "lane" && s.Warning > 0 {
			s.Warning = advanceEnvironmentBudget(s.Warning, e.WarningSeconds, e.WarningRounds, dt, tb)
			releaseTransfers = releaseTransfers || (e.Kind == "transfer" && s.Warning == 0)
			continue
		}
		if e.Kind == "lane" {
			if s.Warning > 0 {
				s.Warning = advanceEnvironmentBudget(s.Warning, e.WarningSeconds, e.WarningRounds, dt, tb)
				if s.Warning == 0 {
					releaseTransfers = true
					s.Remaining = 1
					s.Carry = 0
					if inside {
						g.damageEnvironment(e, 1)
					}
					g.AddCombatMessage(e.Name + " fires.")
				}
				continue
			}
			if s.Remaining > 0 && inside && e.SustainDamage > 0 {
				span := e.DurationSeconds
				if tb {
					span = float64(e.DurationRounds) * 3
				}
				field := e
				field.Damage = e.SustainDamage
				continuous = append(continuous, environmentExposure{effect: field, seconds: min(dt, s.Remaining*span)})
			}
			s.Remaining = advanceEnvironmentBudget(s.Remaining, e.DurationSeconds, e.DurationRounds, dt, tb)
			continue
		}
		if e.Kind == "occupation" && inside {
			continuous = append(continuous, environmentExposure{effect: e, seconds: dt})
		}
	}
	g.damageEnvironmentExposures(v, continuous)
	if releaseTransfers {
		g.resolveEnvironmentArrival(false)
	}

	if b := a.Boss; b != nil && b.HealPercent > 0 {
		m := g.adventureBoss(a)
		if m == nil || !m.IsAlive() || !m.IsEngagingPlayer || m.HitPoints >= m.MaxHitPoints {
			return
		}
		allClosed := true
		for _, c := range a.Controls {
			if c.StopHealing && !v.Controls[c.ID] {
				allClosed = false
			}
		}
		if allClosed {
			return
		}
		v.HealCarry += dt
		for v.HealCarry >= 6 {
			v.HealCarry -= 6
			budget := m.MaxHitPoints*b.HealCapPercent/100 - v.BossHealed
			heal := min(budget, max(1, m.MaxHitPoints*b.HealPercent/100), m.MaxHitPoints-m.HitPoints)
			if heal > 0 {
				m.HitPoints += heal
				v.BossHealed += heal
			}
		}
	}
}

// applyEnvironmentArrival is called once from the common live-arrival path.
// Loading restores states directly and never calls this method.
func (g *MMGame) applyEnvironmentArrival() { g.resolveEnvironmentArrival(true) }

// Forced movement must end on a safe receiving tile, never in another active
// hazard or telegraph. Voluntary movement retains its normal arrival effects.
func (g *MMGame) safeForcedArrival(x, y int, sourceEffect string) bool {
	wx, wy := TileCenterFromTile(x, y, float64(g.config.GetTileSize()))
	if g.collisionSystem == nil || !g.collisionSystem.CanMoveTo("player", wx, wy) {
		return false
	}
	if a := g.adventureConfig(currentMapKey()); a != nil {
		v := g.adventureVisit(currentMapKey())
		for _, e := range a.Effects {
			if e.ID == sourceEffect || !e.Contains(x, y) || !g.effectEnabled(a, v, e) {
				continue
			}
			if e.Kind == "lane" {
				s := v.Effects[e.ID]
				if s == nil || (s.Warning <= 0 && s.Remaining <= 0) {
					continue
				}
			}
			if e.Kind == "transfer" || e.Kind == "occupation" || e.Kind == "trap" || e.Kind == "lane" {
				return false
			}
		}
	}
	if g.world != nil {
		for _, m := range g.world.Monsters {
			if m != nil && m.IsAlive() && TileIndex(m.X, g.config.GetTileSize()) == x && TileIndex(m.Y, g.config.GetTileSize()) == y {
				return false
			}
			if g.telegraphAllowed(m) && m.TargetsParty() && m.Telegraph.TargetID == "" &&
				(m.Telegraph.Warning > 0 || m.Telegraph.Remaining > 0) && slices.Contains(m.Telegraph.Tiles, [2]int{x, y}) {
				return false
			}
		}
	}
	return true
}

func (g *MMGame) resolveEnvironmentArrival(chargeTraps bool) {
	key := currentMapKey()
	a := g.adventureConfig(key)
	if a == nil || g.world == nil {
		return
	}
	v := g.adventureVisit(key)
	tx, ty := g.GetPlayerTilePosition()
	transferred := false
	for _, e := range a.Effects {
		s := v.effectState(e.ID)
		inside := e.Contains(tx, ty)
		if !inside {
			s.WasInside = false
			s.Suppressed = false
			if s.Rearm == 0 {
				s.Triggered = false
			}
			continue
		}
		wasInside := s.WasInside
		s.WasInside = true
		if !g.effectEnabled(a, v, e) {
			continue
		}
		if e.BossBelowPercent > 0 && (!s.Activated || s.Warning > 0) {
			continue
		}
		if chargeTraps && e.Kind == "trap" && !wasInside && !s.Triggered && s.Rearm <= 0 {
			s.Triggered = true
			s.Rearm = 1
			g.damageEnvironment(e, 1)
		}
		if e.Kind != "transfer" || s.Suppressed || transferred {
			continue
		}
		if e.TriggerLane != "" {
			lane := v.Effects[e.TriggerLane]
			if lane == nil || lane.Warning > 0 || lane.Remaining <= 0 {
				continue
			}
		}
		x, y := e.Destination[0], e.Destination[1]
		tile := float64(g.config.GetTileSize())
		wx, wy := TileCenterFromTile(x, y, tile)
		if !g.safeForcedArrival(x, y, e.ID) {
			continue
		}
		oldX, oldY := g.camera.X, g.camera.Y
		g.setPartyPosition(wx, wy)
		g.notifyPilgrimDisplacement(oldX, oldY)
		s.Suppressed = true
		transferred = true
		g.autoPickupLootBags()
		// One source can move the party once. Reset entry latches at the legal
		// receiving tile without recursively activating another transfer.
		for _, other := range a.Effects {
			if !other.Contains(x, y) {
				v.effectState(other.ID).WasInside = false
			}
		}
		g.AddCombatMessage(e.Name + " carries the party to the receiving platform.")
	}
}

func (g *MMGame) disarmEnvironment(id string) {
	a := g.adventureConfig(currentMapKey())
	e := a.Effect(id)
	if e == nil || e.Kind != "trap" {
		return
	}
	v := g.adventureVisit(currentMapKey())
	s := v.effectState(id)
	if s.Disarmed {
		g.AddCombatMessage("The plate is already safe.")
		return
	}
	chance, hand := g.partyTrapAvoidChancePct()
	if hand == nil {
		g.AddCombatMessage("An active hero needs Disarm Trap.")
		return
	}
	if !g.turnBasedMode && hand.RTCooldown > 0 {
		return
	}
	if rand.Intn(100) < chance {
		s.Disarmed = true
		g.AddCombatMessage(e.Name + " is disarmed for this visit.")
	} else {
		g.AddCombatMessage("The catch slips. The plate remains armed.")
	}
	if g.turnBasedMode {
		g.endPartyTurnAfterMovement()
	} else if g.combat != nil {
		hand.RTCooldown = max(hand.RTCooldown, g.combat.CalculateActionCooldownFrames(hand))
	}
}
func (g *MMGame) useAdventureControl(id string) {
	key := currentMapKey()
	a := g.adventureConfig(key)
	c := a.Control(id)
	if c == nil {
		return
	}
	v := g.adventureVisit(key)
	if v.Controls[id] {
		g.AddCombatMessage("The mechanism is already secured.")
		return
	}
	// Validate the complete edit before committing any control or tile.
	for _, t := range c.Tiles {
		if g.world == nil || t.X >= g.world.Width || t.Y >= g.world.Height {
			return
		}
		if _, ok := world.GlobalTileManager.GetTileTypeFromKey(t.Tile); !ok {
			return
		}
	}
	v.Controls[id] = true
	for _, t := range c.Tiles {
		tile, _ := world.GlobalTileManager.GetTileTypeFromKey(t.Tile)
		before := g.world.Tiles[t.Y][t.X]
		g.recordTerrainChange(t.X, t.Y, before, tile)
		g.world.Tiles[t.Y][t.X] = tile
	}
	if m := g.adventureBoss(a); m != nil && a.Boss != nil {
		m.ArmorClass = max(a.Boss.ArmorFloor, m.ArmorClass-c.ArmorReduction)
	}
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}
	if c.Message != "" {
		g.AddCombatMessage(c.Message)
	} else {
		g.AddCombatMessage("The service mechanism locks into its safe position.")
	}
	if g.turnBasedMode {
		g.endPartyTurnAfterMovement()
	}
}
func (cs *CombatSystem) runAdventureBossAction(m *monster.Monster3D, attackTick bool) bool {
	g := cs.game
	a := g.adventureConfig(m.HomeMap)
	if a == nil || a.Boss == nil || a.Boss.Monster != m.Key || cs.bossDisabled(m) {
		return false
	}
	if a.Boss.LowHealthRange > 0 {
		if def, err := monster.MonsterConfig.GetMonsterByKey(m.Key); err == nil {
			reach := def.RangedAttackRange
			if m.HitPoints*100 <= m.MaxHitPoints*25 {
				reach = a.Boss.LowHealthRange
			}
			m.RangedAttackRange = reach * float64(g.config.GetTileSize())
		}
	}
	if !attackTick {
		return false
	}
	v := g.adventureVisit(m.HomeMap)
	v.BossActions++
	pattern := a.Boss.Pattern(m.HitPoints, m.MaxHitPoints, v.BossActions)
	// A pattern is one atomic action: never charge or arm just half of it.
	for _, id := range pattern {
		e := a.Effect(id)
		if e == nil || !g.effectEnabled(a, v, *e) {
			return false
		}
		s := v.effectState(id)
		if s.Warning > 0 || s.Remaining > 0 {
			return false
		}
	}
	if len(pattern) == 0 {
		return false
	}
	for _, id := range pattern {
		e := a.Effect(id)
		v.effectState(id).Warning = 1
		g.AddCombatMessage(fmt.Sprintf("%s announces %s (%s): tiles %d,%d to %d,%d. Leave the marked lane!", m.Name, e.Name, e.School, e.Rect[0], e.Rect[1], e.Rect[2], e.Rect[3]))
	}
	return true
}
func (g *MMGame) recordAdventureBoss(m *monster.Monster3D) {
	if m == nil || m.SummonedBy != "" {
		return
	}
	a := g.adventureConfig(m.HomeMap)
	if a == nil || a.Boss == nil || a.Boss.Monster != m.Key {
		return
	}
	v := g.adventureVisit(m.HomeMap)
	if v.BossGranted {
		return
	}
	v.BossGranted = true
	v.ArenaLocked = false
	if a.ResetDays > 0 {
		v.VictoryAt = g.currentQuestDay()
	}
	g.syncAdventureArenaBounds()
	if g.adventure.Records == nil {
		g.adventure.Records = map[string]bool{}
	}
	g.adventure.Records[m.Key] = true
}

// Continuous fields share one clock per damage-school pair. Integrate the
// strongest active rate over each interval; initial lane hits remain separate.
type environmentExposure struct {
	effect  config.EnvironmentEffect
	seconds float64
	source  *monster.Monster3D
}

func (g *MMGame) damageEnvironmentExposures(v *AdventureVisit, exposures []environmentExposure) {
	groups := map[string][]environmentExposure{}
	for _, e := range exposures {
		if e.seconds > 0 {
			key := "continuous:" + e.effect.School + ":" + e.effect.SecondarySchool
			groups[key] = append(groups[key], e)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fields := groups[key]
		ends := make([]float64, 0, len(fields))
		for _, f := range fields {
			ends = append(ends, f.seconds)
		}
		sort.Float64s(ends)
		state := v.effectState(key)
		start := 0.0
		for _, end := range ends {
			if end <= start {
				continue
			}
			// Even a fully weakened interval keeps the group's school for
			// contributions accumulated before the rate dropped to zero.
			strongest := environmentExposure{effect: fields[0].effect}
			strongest.effect.Damage = 0
			for _, f := range fields {
				if f.seconds >= end && f.effect.Damage > strongest.effect.Damage {
					strongest = f
				}
			}
			remaining := end - start
			for remaining > 1e-9 {
				step := min(remaining, 1-state.Carry)
				state.Carry += step
				contribution := step * float64(strongest.effect.Damage)
				if strongest.source == nil {
					state.DamageCarry += contribution
				} else {
					if state.SourceDamageCarry == nil {
						state.SourceDamageCarry = map[string]float64{}
					}
					state.SourceDamageCarry[strongest.source.ID] += contribution
				}
				remaining -= step
				if state.Carry >= 1-1e-9 {
					damage := int(math.Floor(state.DamageCarry + 1e-9))
					state.Carry = 0
					state.DamageCarry -= float64(damage)
					pulse := strongest.effect
					pulse.Damage = damage
					g.damageEnvironment(pulse, 1)
					g.damageMonsterFieldContributions(state, pulse.School)
				}
			}
			start = end
		}
	}
}

// Keep the owner of each integrated interval, even when the strongest field
// changes before the next pulse or the game is saved mid-second. Legacy
// DamageCarry remains unowned environmental damage.
func (g *MMGame) damageMonsterFieldContributions(state *EnvironmentState, school string) {
	ids := make([]string, 0, len(state.SourceDamageCarry))
	for id := range state.SourceDamageCarry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		carry := state.SourceDamageCarry[id]
		damage := int(math.Floor(carry + 1e-9))
		state.SourceDamageCarry[id] = max(0, carry-float64(damage))
		if state.SourceDamageCarry[id] == 0 {
			delete(state.SourceDamageCarry, id)
		}
		if damage <= 0 {
			continue
		}
		for _, m := range g.world.Monsters {
			if m.ID == id {
				if a := telegraphDefinition(m); a != nil {
					g.damageTelegraphParty(m, a.Name, school, damagecalc.Parts{Normal: damage})
				}
				break
			}
		}
	}
}

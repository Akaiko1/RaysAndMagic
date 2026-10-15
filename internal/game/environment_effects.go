package game

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/mathutil"
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

// effectDamage resolves an effect's percent damage against the boss attack
// range (adventureBossAttack), so chamber hazards grow with the chamber.
func effectDamage(e config.EnvironmentEffect, lo, hi int) config.EnvironmentEffect {
	if e.DamagePercent > 0 {
		e.Damage = max(1, percentOfAttack(lo, hi, e.DamagePercent))
	}
	if e.SustainPercent > 0 {
		e.SustainDamage = max(1, percentOfAttack(lo, hi, e.SustainPercent))
	}
	return e
}

// AdventureHazard is a chamber effect's damage at a visit level: on a hit, or
// per second for occupation and a lane's lingering field.
type AdventureHazard struct {
	Name, Kind              string
	Percent, SustainPercent int
	Damage, Sustain         int
}

// AdventureHazards resolves every damaging chamber effect at a visit level.
func AdventureHazards(s config.AdventureScalingConfig, a *config.AdventureConfig, level int) []AdventureHazard {
	lo, hi := adventureBossAttack(s, a, &AdventureVisit{Level: level})
	var out []AdventureHazard
	for _, e := range a.Effects {
		if e.Damaging() {
			r := effectDamage(e, lo, hi)
			out = append(out, AdventureHazard{Name: e.Name, Kind: e.Kind, Percent: e.DamagePercent, SustainPercent: e.SustainPercent, Damage: r.Damage, Sustain: r.SustainDamage})
		}
	}
	return out
}

// percentOfAttack is a percent of the average of an attack range: the one
// rule for telegraphs and chamber hazards.
func percentOfAttack(lo, hi, percent int) int { return (lo + hi) * percent / 200 }

// adventureBossAttack is the map boss's attack range at the visit's level
// (load validation guarantees a leveled, attacking boss for percent damage).
func adventureBossAttack(s config.AdventureScalingConfig, a *config.AdventureConfig, v *AdventureVisit) (int, int) {
	if a == nil || a.Boss == nil || monster.MonsterConfig == nil {
		return 0, 0
	}
	d, ok := monster.MonsterConfig.Monsters[a.Boss.Monster]
	if !ok || d.Level < 1 {
		return 0, 0
	}
	level := d.Level
	if v != nil && v.Level > 0 {
		level = v.Level
	}
	return adventureDamageRange(s, d, d.Boss, level)
}

// applyAdventureBossArmor sets the map boss's armor: its armor at the visit's
// level, less the percent every operated switch removes, never under the floor.
func (g *MMGame) applyAdventureBossArmor(m *monster.Monster3D, a *config.AdventureConfig, v *AdventureVisit) {
	if m == nil || a == nil || a.Boss == nil || a.Boss.Monster != m.Key || v == nil || monster.MonsterConfig == nil {
		return
	}
	d, ok := monster.MonsterConfig.Monsters[m.Key]
	if !ok || d.Level < 1 {
		return
	}
	level := m.AdventureScaleLevel
	if level < 1 {
		level = d.Level
	}
	removed := 0
	for _, c := range a.Controls {
		if v.Controls[c.ID] {
			removed += c.ArmorReductionPercent
		}
	}
	m.ArmorClass = adventureArmorClass(g.config.AdventureScaling, d, level)
	if removed > 0 {
		m.ArmorClass = max(a.Boss.ArmorFloor, m.ArmorClass*max(0, 100-removed)/100)
	}
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

// damageEnvironment resolves both schools before reporting one combined hit
// per member. Zero-damage ticks stay silent; the first school colors the total.
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
			g.reportDamage(logToneBad, damageLog{
				Source: logSchoolWord(e.School, e.Name),
				Target: heroLogTarget(c),
				Amount: dealt,
				School: e.School,
			})
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
	lo, hi := adventureBossAttack(g.config.AdventureScaling, a, v)
	for _, e := range a.Effects {
		e = effectDamage(e, lo, hi)
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
			// Whole rounds: the field spans the same seconds in both modes.
			span := float64(e.DurationSeconds)
			if s.Remaining > 0 && inside && e.SustainDamage > 0 {
				field := e
				field.Damage = e.SustainDamage
				continuous = append(continuous, environmentExposure{effect: field, seconds: min(dt, s.Remaining*span)})
			}
			s.Remaining = advanceEnvironmentBudget(s.Remaining, span, config.CombatRounds(e.DurationSeconds), dt, tb)
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
		if m == nil || !m.IsAlive() || !m.IsEngagingPlayer || m.HitPoints*100 >= m.MaxHitPoints*b.HealCeilingPercent {
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
		// Every round (its seconds in RT) the boss heals toward its ceiling.
		v.HealCarry += dt
		for round := float64(config.CombatRoundSeconds); v.HealCarry >= round; v.HealCarry -= round {
			ceiling := m.MaxHitPoints * b.HealCeilingPercent / 100
			if heal := min(max(1, m.MaxHitPoints*b.HealPercent/100), ceiling-m.HitPoints); heal > 0 {
				m.HitPoints += heal
			}
		}
	}
}

// applyEnvironmentArrival runs on every live arrival: a step, a forced step
// and a map arrival. Loading restores states directly and never calls it.
func (g *MMGame) applyEnvironmentArrival() { g.resolveEnvironmentArrival(true) }

// forcedArrivalOpen reports a tile a forced move may end on: one the party
// can stand on that no monster holds. Hazards, lanes and telegraphs do not
// matter.
func (g *MMGame) forcedArrivalOpen(x, y int) bool {
	ts := g.config.GetTileSize()
	wx, wy := TileCenterFromTile(x, y, float64(ts))
	if g.collisionSystem == nil || !g.collisionSystem.CanMoveTo("player", wx, wy) {
		return false
	}
	if g.world != nil {
		for _, m := range g.world.Monsters {
			if m != nil && m.IsAlive() && TileIndex(m.X, ts) == x && TileIndex(m.Y, ts) == y {
				return false
			}
		}
	}
	return true
}

// forcePartyStep moves the party onto an open tile and lands it there
// (arriveOnTile). False leaves the party where it was.
func (g *MMGame) forcePartyStep(x, y int) bool {
	if !g.forcedArrivalOpen(x, y) {
		return false
	}
	g.displacePartyToTile(x, y)
	g.arriveOnTile()
	return true
}

// displacePartyToTile is displaceParty onto a tile centre (a push, a current,
// the arena pull).
func (g *MMGame) displacePartyToTile(x, y int) {
	wx, wy := TileCenterFromTile(x, y, float64(g.config.GetTileSize()))
	g.displaceParty(wx, wy, "")
}

// displaceParty moves the party anywhere but by its own step - a jump, a
// Pilgrim teleport, a push, a current, the arena pull: movement effects see
// the distance and credit names the adventure movement it counts as ("" for
// none). The caller lands it (applyLandingTileEffects or arriveOnTile).
func (g *MMGame) displaceParty(x, y float64, credit string) {
	oldX, oldY := g.camera.X, g.camera.Y
	g.setPartyPosition(x, y)
	g.creditAdventureMovement(credit, oldX, oldY, x, y)
	g.partyMoved(oldX, oldY)
}

// arriveOnTile is the arrival every move shares, in the step's order: loot
// is picked up, then chamber effects arrive (plates fire, a current carries
// the party on and picks up the receiver's loot). A step and a jump add
// teleporters and deep water (applyLandingTileEffects); forced moves stop
// here, since they run inside the AI pass where a map switch must not happen.
func (g *MMGame) arriveOnTile() {
	g.autoPickupLootBags()
	g.applyEnvironmentArrival()
}

func (g *MMGame) resolveEnvironmentArrival(chargeTraps bool) {
	key := currentMapKey()
	a := g.adventureConfig(key)
	if a == nil || g.world == nil {
		return
	}
	v := g.adventureVisit(key)
	lo, hi := adventureBossAttack(g.config.AdventureScaling, a, v)
	// One source can move the party once. The source tile resolves whole;
	// the receiving tile then arrives like a step (its plates fire) without
	// carrying the party on again.
	transferred := false
	for pass := range 2 {
		tx, ty := g.GetPlayerTilePosition()
		charge := chargeTraps || pass > 0
		carried := false
		for _, e := range a.Effects {
			e = effectDamage(e, lo, hi)
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
			if charge && e.Kind == "trap" && !wasInside && !s.Triggered && s.Rearm <= 0 {
				s.Triggered = true
				s.Rearm = 1
				g.damageEnvironment(e, 1)
			}
			// A current never chains: one under the receiving tile sleeps until
			// the party steps off it (Suppressed clears once it is outside),
			// whatever later releases transfers.
			if e.Kind == "transfer" && pass > 0 {
				s.Suppressed = true
				continue
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
			if !g.forcedArrivalOpen(x, y) {
				continue
			}
			g.displacePartyToTile(x, y)
			s.Suppressed = true
			transferred, carried = true, true
			g.autoPickupLootBags()
			g.AddCombatMessage(e.Name + " carries the party to the receiving platform.")
		}
		if !carried {
			return
		}
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
	if m := g.adventureBoss(a); m != nil {
		g.applyAdventureBossArmor(m, a, v)
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

// chamberBoss returns a chamber boss's adventure policy, or nil for any other
// monster.
func (g *MMGame) chamberBoss(m *monster.Monster3D) *config.AdventureConfig {
	a := g.adventureConfig(m.HomeMap)
	if a == nil || a.Boss == nil || a.Boss.Monster != m.Key {
		return nil
	}
	return a
}

// updateChamberBossReach sets a chamber boss's ranged reach for its HP before
// it acts: low_health_range at or below 25% HP.
func (cs *CombatSystem) updateChamberBossReach(m *monster.Monster3D) {
	a := cs.game.chamberBoss(m)
	if a == nil || a.Boss.LowHealthRange <= 0 || cs.bossDisabled(m) {
		return
	}
	if def, err := monster.MonsterConfig.GetMonsterByKey(m.Key); err == nil {
		reach := def.RangedAttackRange
		if m.HitPoints*100 <= m.MaxHitPoints*25 {
			reach = a.Boss.LowHealthRange
		}
		m.RangedAttackRange = reach * float64(cs.game.config.GetTileSize())
	}
}

// monsterAttackLanded runs when a monster's attack reaches its target, the
// party or another actor, dodged or not: a melee strike at once, a shot when
// it hits. A shot that flies past lands nothing.
func (cs *CombatSystem) monsterAttackLanded(m *monster.Monster3D) {
	if m != nil && m.IsBoss() {
		cs.runChamberBossFreeActions(m)
	}
}

// runChamberBossFreeActions follows each landed attack of a chamber boss
// (monsterAttackLanded), RT and TB alike, whoever it struck: the next lane
// pattern once the lane cooldown is over, then the push once its own is.
// Neither spends the action; both cooldowns run in rounds like any other.
func (cs *CombatSystem) runChamberBossFreeActions(m *monster.Monster3D) {
	g := cs.game
	a := g.chamberBoss(m)
	if a == nil || !m.IsAlive() || m.IsAfraid() || m.IsSilenced() || cs.bossDisabled(m) {
		return
	}
	v := g.adventureVisit(m.HomeMap)
	tps := g.config.GetTPS()
	if !m.LaneCD.Active() {
		m.LaneCD.Arm(a.Boss.LaneCooldown(m.HitPoints, m.MaxHitPoints), tps)
		g.announceAdventureBossLanes(m, a, v)
	}
	if cooldown := m.Abilities.Push.CooldownSeconds; cooldown > 0 && !m.PushCD.Active() {
		m.PushCD.Arm(cooldown, tps)
		g.pushPartyTowardLane(m, a, v)
	}
}

// announceAdventureBossLanes marks the phase's next pattern; the turn passes
// to the one after it even when this one cannot fire. A pattern is atomic:
// never charge or arm just half of it.
func (g *MMGame) announceAdventureBossLanes(m *monster.Monster3D, a *config.AdventureConfig, v *AdventureVisit) {
	pattern := a.Boss.Pattern(m.HitPoints, m.MaxHitPoints, v.LanePatterns)
	v.LanePatterns++
	if len(pattern) == 0 {
		return
	}
	for _, id := range pattern {
		e := a.Effect(id)
		if e == nil || !g.effectEnabled(a, v, *e) {
			return
		}
		s := v.effectState(id)
		if s.Warning > 0 || s.Remaining > 0 {
			return
		}
	}
	for _, id := range pattern {
		e := a.Effect(id)
		v.effectState(id).Warning = 1
		g.AddCombatMessage(fmt.Sprintf("%s announces %s (%s): tiles %d,%d to %d,%d. Leave the marked lane!", m.Name, e.Name, e.School, e.Rect[0], e.Rect[1], e.Rect[2], e.Rect[3]))
	}
}

// pushPartyTowardLane shoves the party one tile toward the nearest enabled
// lane of the boss's current phase, in whatever state that lane is. A party
// already standing in one stays put; the longer axis is tried first.
func (g *MMGame) pushPartyTowardLane(m *monster.Monster3D, a *config.AdventureConfig, v *AdventureVisit) {
	px, py := g.GetPlayerTilePosition()
	var lane *config.EnvironmentEffect
	gapX, gapY, best := 0, 0, math.MaxInt
	for _, id := range a.Boss.PhaseLanes(m.HitPoints, m.MaxHitPoints) {
		e := a.Effect(id)
		if e == nil || !g.effectEnabled(a, v, *e) {
			continue
		}
		dx := min(max(px, e.Rect[0]), e.Rect[2]) - px
		dy := min(max(py, e.Rect[1]), e.Rect[3]) - py
		if d := mathutil.IntAbs(dx) + mathutil.IntAbs(dy); d < best {
			lane, gapX, gapY, best = e, dx, dy, d
		}
	}
	if lane == nil || best == 0 {
		return
	}
	steps := [][2]int{{sign(gapX), 0}, {0, sign(gapY)}}
	if mathutil.IntAbs(gapY) > mathutil.IntAbs(gapX) {
		steps[0], steps[1] = steps[1], steps[0]
	}
	for _, step := range steps {
		if step != [2]int{} && g.forcePartyStep(px+step[0], py+step[1]) {
			g.logCombat(logToneBad, styledFormat(uitext.Text("adventure.boss_push", logSlot, logSlot, logSlot)),
				logMonsterName(m), logAbility(uitext.Text("adventure.boss_push_action")), lane.Name)
			return
		}
	}
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

package game

import (
	"math"
	"slices"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/status"
)

// A landed hinder lasts four seconds / two rounds, followed by an equally long
// recovery window shared by these roots and slows. Recasts cannot extend it.
type PartyHinderState struct {
	Slow     float64 `json:"slow,omitempty"`
	Recovery float64 `json:"recovery,omitempty"`
}

func (g *MMGame) tickPartyHinder(tb bool) {
	dt := 1 / float64(max(1, g.config.GetTPS()))
	g.partyHinder.Slow = advanceEnvironmentBudget(g.partyHinder.Slow, 4, 2, dt, tb)
	g.partyHinder.Recovery = advanceEnvironmentBudget(g.partyHinder.Recovery, 4, 2, dt, tb)
}
func (g *MMGame) applyTelegraphHinder(kind string) {
	if g.partyHinder.Recovery > 0 {
		return
	}
	g.partyHinder.Recovery = 2
	if kind == "root" {
		r := &g.partyRoot
		status.RefreshDualRated(&r.Frames, &r.Turns, &r.Rate, 4*g.config.GetTPS(), 2)
		g.AddCombatMessage("Roots bind the party. Hold your ground or fight free!")
	} else {
		g.partyHinder.Slow = 1
		g.AddCombatMessage("Cold water slows the party's movement and recovery.")
	}
}
func (g *MMGame) hinderRecovery(cooldown int) int {
	if g.partyHinder.Slow > 0 {
		return int(math.Ceil(float64(cooldown) / .75))
	}
	return cooldown
}
func telegraphDefinition(m *monster.Monster3D) *monster.TelegraphedAttack {
	if m == nil || monster.MonsterConfig == nil {
		return nil
	}
	return monster.MonsterConfig.Monsters[m.Key].Telegraph
}
func (g *MMGame) telegraphAllowed(m *monster.Monster3D) bool {
	return m != nil && m.IsAlive() && !m.IsPartyControlled() && !m.Pacified && !m.BossDormant && m.CurrentAIBehavior().Caps().MayAttack && m.StunFramesRemaining == 0 && m.StunTurnsRemaining == 0 && !g.turnBasedMonsterStunned[m]
}
func (g *MMGame) telegraphLOS(m *monster.Monster3D, x, y float64) bool {
	return g.collisionSystem != nil && g.collisionSystem.CheckLineOfSight(m.X, m.Y, x, y)
}
func cardinalDelta(x, y int) [2]int {
	if absInt(x) >= absInt(y) {
		if x < 0 {
			return [2]int{-1, 0}
		}
		return [2]int{1, 0}
	}
	if y < 0 {
		return [2]int{0, -1}
	}
	return [2]int{0, 1}
}
func (g *MMGame) telegraphTiles(m *monster.Monster3D, a *monster.TelegraphedAttack, target [2]int) [][2]int {
	ts := float64(g.config.GetTileSize())
	origin := [2]int{TileIndex(m.X, ts), TileIndex(m.Y, ts)}
	dir := cardinalDelta(target[0]-origin[0], target[1]-origin[1])
	var out [][2]int
	add := func(x, y int) {
		if x < 0 || y < 0 || x >= g.world.Width || y >= g.world.Height {
			return
		}
		wx, wy := TileCenterFromTile(x, y, ts)
		if g.telegraphLOS(m, wx, wy) {
			out = append(out, [2]int{x, y})
		}
	}
	switch a.Shape {
	case "target":
		add(target[0], target[1])
	case "burst":
		for y := -a.Range; y <= a.Range; y++ {
			for x := -a.Range; x <= a.Range; x++ {
				if x*x+y*y <= a.Range*a.Range {
					add(origin[0]+x, origin[1]+y)
				}
			}
		}
	case "lane", "cone":
		for d := 1; d <= a.Range; d++ {
			width := 0
			if a.Shape == "cone" {
				width = (d - 1) / 2
			}
			for side := -width; side <= width; side++ {
				add(origin[0]+dir[0]*d-dir[1]*side, origin[1]+dir[1]*d+dir[0]*side)
			}
		}
	}
	return out
}

// Called from both combat schedulers after faction/disable gates. Starting a
// warning and releasing it each consume an action; normal attacks never leak
// through a wind-up, including extra TB passes or an RT state transition.
func (cs *CombatSystem) runTelegraphedAction(m *monster.Monster3D, ready bool) bool {
	g := cs.game
	a := telegraphDefinition(m)
	if a == nil || !g.telegraphAllowed(m) {
		return false
	}
	s := &m.Telegraph
	if s.Warning > 0 || s.Released {
		return true
	}
	// Support abilities obey the same combat-target gate as offensive casts.
	// Keep an already committed warning intact when its target moves away.
	if !m.TargetsParty() && (m.AIFoe == nil || !m.AIFoe.IsAlive() || !m.CanAttackActor(m.AIFoe)) {
		return false
	}
	if !ready || s.Cooldown > 0 || s.Remaining > 0 {
		return false
	}
	if a.BelowHPPercent > 0 && m.HitPoints*100 > m.MaxHitPoints*a.BelowHPPercent {
		return false
	}
	tx, ty := g.camera.X, g.camera.Y
	targetID := ""
	if a.Kind == "shield" {
		var ally *monster.Monster3D
		for _, other := range g.world.Monsters {
			if other == m || !other.IsAlive() || other.IsPartyControlled() || other.Pacified || other.BossDormant || m.CanAttackActor(other) || other.SoakDamage > 0 {
				continue
			}
			if Distance(m.X, m.Y, other.X, other.Y) > float64(a.Range)*g.config.GetTileSize() || !g.telegraphLOS(m, other.X, other.Y) {
				continue
			}
			if ally == nil || other.ID < ally.ID {
				ally = other
			}
		}
		if ally == nil {
			return false
		}
		tx, ty, targetID = ally.X, ally.Y, ally.ID
	} else if m.AIFoe != nil {
		if !m.AIFoe.IsAlive() || !m.CanAttackActor(m.AIFoe) {
			return false
		}
		tx, ty, targetID = m.AIFoe.X, m.AIFoe.Y, m.AIFoe.ID
	} else if !m.TargetsParty() {
		return false
	}
	if Distance(m.X, m.Y, tx, ty) > float64(a.Range)*g.config.GetTileSize() || !g.telegraphLOS(m, tx, ty) {
		return false
	}
	ts := float64(g.config.GetTileSize())
	target := [2]int{TileIndex(tx, ts), TileIndex(ty, ts)}
	tiles := g.telegraphTiles(m, a, target)
	if !slices.Contains(tiles, target) {
		return false
	}
	*s = monster.TelegraphState{Cooldown: 1, Warning: 1, Tiles: tiles, Origin: [2]int{TileIndex(m.X, ts), TileIndex(m.Y, ts)}, TargetID: targetID}
	s.Direction = cardinalDelta(target[0]-s.Origin[0], target[1]-s.Origin[1])
	g.releaseMonsterAttackPost(m)
	m.ResetPathCache()
	g.armMonsterAttackAnimation(m)
	cs.armMonsterRTAttackCooldowns(m)
	g.AddCombatMessage(m.Name + " prepares " + a.Name + ". Leave the marked tiles!")
	return true
}
func (g *MMGame) tickMonsterTelegraphs(dt float64, tb bool) {
	if g.world == nil {
		return
	}
	for _, m := range g.world.Monsters {
		a := telegraphDefinition(m)
		if a == nil {
			continue
		}
		s := &m.Telegraph
		s.Released = false
		s.Cooldown = advanceEnvironmentBudget(s.Cooldown, a.CooldownSeconds, a.CooldownRounds, dt, tb)
		if !g.telegraphAllowed(m) {
			s.Warning = 0
			s.Remaining = 0
			s.Tiles = nil
			continue
		}
		if s.Warning <= 0 {
			continue
		}
		s.Warning = advanceEnvironmentBudget(s.Warning, a.WarningSeconds, a.WarningRounds, dt, tb)
		if s.Warning > 0 {
			continue
		}
		s.Released = true
		g.resolveMonsterTelegraph(m, a)
		g.combat.armMonsterRTAttackCooldowns(m)
	}
}
func (g *MMGame) resolveMonsterTelegraph(m *monster.Monster3D, a *monster.TelegraphedAttack) {
	s := &m.Telegraph
	ts := float64(g.config.GetTileSize())
	if a.Kind == "shield" {
		for _, ally := range g.world.Monsters {
			if ally.ID != s.TargetID || !ally.IsAlive() || ally.IsPartyControlled() || ally.Pacified || m.CanAttackActor(ally) || !g.telegraphLOS(m, ally.X, ally.Y) || Distance(m.X, m.Y, ally.X, ally.Y) > float64(a.Range)*ts {
				continue
			}
			ally.ApplySoak(monster.TelegraphShieldAbsorption(m.DamageMin, m.DamageMax), int(a.DurationSeconds*float64(g.config.GetTPS())), a.DurationRounds)
			g.AddCombatMessage(m.Name + " shields " + ally.Name + " with circulating water.")
			vx, vy := g.combat.monsterVisualPos(ally)
			g.createElementalAbilityImpact(vx, vy, "water", ally)
		}
		return
	}
	if a.Kind == "charge" {
		g.moveTelegraphMonster(m, s.Direction, a.Range)
	}
	damage := (m.DamageMin + m.DamageMax) * a.DamagePercent / 200
	parts := m.OutgoingDamage(damagecalc.Parts{Normal: damage, True: m.TrueDamage})
	if s.TargetID == "" && m.TargetsParty() {
		px, py := g.GetPlayerTilePosition()
		if slices.Contains(s.Tiles, [2]int{px, py}) && g.telegraphLOS(m, g.camera.X, g.camera.Y) {
			g.damageTelegraphParty(m, a.Name, a.School, parts)
			g.observeOverwatchAttack(m)
			switch a.Kind {
			case "slow", "root":
				g.applyTelegraphHinder(a.Kind)
			case "push":
				g.pushTelegraphParty(s.Direction)
			}
		}
	} else if s.TargetID != "" {
		for _, foe := range g.world.Monsters {
			if foe.ID != s.TargetID || !foe.IsAlive() || !m.CanAttackActor(foe) || !slices.Contains(s.Tiles, [2]int{TileIndex(foe.X, ts), TileIndex(foe.Y, ts)}) || !g.telegraphLOS(m, foe.X, foe.Y) {
				continue
			}
			g.combat.applyMonsterDamagePacket(foe, singleMonsterDamagePacket(parts, a.School, 0), monsterDamageOptions{IsRanged: true})
			if a.Kind == "slow" {
				foe.ApplySlow(25, 4*g.config.GetTPS(), 2)
			}
			if a.Kind == "root" {
				status.RefreshDualRated(&foe.RootFramesRemaining, &foe.RootTurnsRemaining, &foe.RootRate, 4*g.config.GetTPS(), 2)
			}
			if !foe.IsAlive() {
				g.combat.finishMonsterKill(foe)
			}
		}
	}
	if a.SustainDamage > 0 {
		s.Remaining = 1
	}
	if a.Kind == "retreat" {
		g.moveTelegraphMonster(m, [2]int{-s.Direction[0], -s.Direction[1]}, 2)
	}
	g.armMonsterAttackAnimation(m)
	g.AddCombatMessage(m.Name + " releases " + a.Name + "!")
}

// Telegraph hits remain undodgeable, with the same absorption policy as the
// authored hazard. Retain the attacker for reflection and hit attribution.
func (g *MMGame) damageTelegraphParty(source *monster.Monster3D, name, school string, parts damagecalc.Parts) {
	defer g.beginProfileMonsterHit(source, name)()
	g.combat.forEachDamageablePartyMember(func(i int, c *character.MMCharacter) {
		dealt := g.combat.damagePartyMemberPartsFromSource(i, c, parts, school, false, source)
		if dealt > 0 {
			g.logCombat(logToneBad, "%s hits %s for %d.", name, logHeroName(c), dealt)
		}
	})
}

func (g *MMGame) moveTelegraphMonster(m *monster.Monster3D, dir [2]int, distance int) {
	if g.gameLoop == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	for range distance {
		x, y := TileIndex(m.X, ts)+dir[0], TileIndex(m.Y, ts)+dir[1]
		wx, wy := TileCenterFromTile(x, y, ts)
		if !g.gameLoop.commitMonsterMoveTB(m, wx, wy) {
			break
		}
	}
}
func (g *MMGame) pushTelegraphParty(dir [2]int) {
	x, y := g.GetPlayerTilePosition()
	wx, wy := TileCenterFromTile(x+dir[0], y+dir[1], float64(g.config.GetTileSize()))
	if !g.safeForcedArrival(x+dir[0], y+dir[1], "") {
		return
	}
	oldX, oldY := g.camera.X, g.camera.Y
	g.setPartyPosition(wx, wy)
	g.notifyPilgrimDisplacement(oldX, oldY)
	g.resolveEnvironmentArrival(false)
	g.autoPickupLootBags()
}
func (g *MMGame) monsterFieldExposures(dt float64) []environmentExposure {
	var out []environmentExposure
	if g.world == nil {
		return out
	}
	x, y := g.GetPlayerTilePosition()
	for _, m := range g.world.Monsters {
		a := telegraphDefinition(m)
		if a == nil || a.SustainDamage <= 0 || m.Telegraph.Remaining <= 0 {
			continue
		}
		s := &m.Telegraph
		span := a.DurationSeconds
		if g.turnBasedMode {
			span = float64(a.DurationRounds) * 3
		}
		elapsed := min(dt, s.Remaining*span)
		s.Remaining = advanceEnvironmentBudget(s.Remaining, a.DurationSeconds, a.DurationRounds, dt, g.turnBasedMode)
		if !g.telegraphAllowed(m) {
			continue
		}
		// A field is authored as normal damage per second, not a new weapon hit.
		rate := m.OutgoingDamage(damagecalc.Parts{Normal: a.SustainDamage}).Normal
		if s.TargetID == "" && m.TargetsParty() && slices.Contains(s.Tiles, [2]int{x, y}) {
			out = append(out, environmentExposure{effect: config.EnvironmentEffect{Name: a.Name, School: a.School, Damage: rate}, seconds: elapsed, source: m})
		} else if s.TargetID != "" {
			for _, foe := range g.world.Monsters {
				ts := g.config.GetTileSize()
				if foe.ID != s.TargetID || !foe.IsAlive() || !m.CanAttackActor(foe) || !slices.Contains(s.Tiles, [2]int{TileIndex(foe.X, ts), TileIndex(foe.Y, ts)}) {
					continue
				}
				s.Carry += float64(rate) * elapsed
				damage := int(math.Floor(s.Carry + 1e-9))
				s.Carry = max(0, s.Carry-float64(damage))
				if damage > 0 {
					g.combat.applyMonsterDamagePacket(foe, singleMonsterDamagePacket(damagecalc.Parts{Normal: damage}, a.School, 0), monsterDamageOptions{IsRanged: true})
					if !foe.IsAlive() {
						g.combat.finishMonsterKill(foe)
					}
				}
			}
		}
	}
	return out
}
func (r *Renderer) collectMonsterTelegraphs() {
	g := r.game
	if g.world == nil {
		return
	}
	alpha, density, radius := r.auraEdgeParams()
	ts := float64(g.config.GetTileSize())
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() || (m.Telegraph.Warning <= 0 && m.Telegraph.Remaining <= 0) {
			continue
		}
		rgb := [3]int{255, 235, 90}
		if m.Telegraph.Warning <= 0 {
			rgb = [3]int{255, 90, 30}
		}
		for _, tile := range m.Telegraph.Tiles {
			r.collectAuraTileEdges(tile[0], tile[1], ts, density, alpha, float64(radius)*ts, rgb)
		}
	}
}

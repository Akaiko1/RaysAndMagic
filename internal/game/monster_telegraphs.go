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

// A landed hinder (a slowing or rooting telegraph) lasts the telegraph's
// duration, then an equally long recovery window shared by these roots and
// slows: Recovery covers both. Recasts cannot extend it.
type PartyHinderState struct {
	Slow     status.DualClock `json:"slow_clock,omitzero"`
	Recovery status.DualClock `json:"recovery_clock,omitzero"`
	// Legacy saves: spent fractions of a Window of rounds (two when unset).
	LegacySlow     float64 `json:"slow,omitempty"`
	LegacyRecovery float64 `json:"recovery,omitempty"`
	LegacyWindow   int     `json:"window,omitempty"`
}

// migrateLegacy turns an old save's fractions into clocks.
func (h *PartyHinderState) migrateLegacy(tps int) {
	window := h.LegacyWindow
	if window <= 0 {
		window = 2
	}
	span := float64(config.RoundFrames(window, tps))
	if h.LegacySlow > 0 {
		h.Slow = status.FromFrames(int(math.Ceil(h.LegacySlow*span)), tps)
	}
	if h.LegacyRecovery > 0 {
		h.Recovery = status.FromFrames(int(math.Ceil(h.LegacyRecovery*span)), tps)
	}
	h.LegacySlow, h.LegacyRecovery, h.LegacyWindow = 0, 0, 0
}

func (g *MMGame) tickPartyHinder(turn bool) {
	g.partyHinder.Slow.Tick(turn)
	g.partyHinder.Recovery.Tick(turn)
}
func (g *MMGame) applyTelegraphHinder(a *monster.TelegraphedAttack) {
	h := &g.partyHinder
	if h.Recovery.Active() {
		return
	}
	tps := g.config.GetTPS()
	h.Recovery.Arm(2*a.DurationSeconds, tps)
	if a.Kind == "root" {
		g.rootParty(config.CombatRounds(a.DurationSeconds))
		g.AddCombatMessage("Roots bind the party. Hold your ground or fight free!")
	} else {
		h.Slow.Arm(a.DurationSeconds, tps)
		g.AddCombatMessage("Cold water slows the party's movement and recovery.")
	}
}

// hinderRecovery stretches a hero's recovery while the party is slowed.
func (g *MMGame) hinderRecovery(cooldown int) int {
	if g.partyHinder.Slow.Active() {
		keep := config.HinderKeepPercent()
		return (cooldown*100 + keep - 1) / keep
	}
	return cooldown
}
func telegraphDefinition(m *monster.Monster3D) *monster.TelegraphedAttack {
	if m == nil || monster.MonsterConfig == nil {
		return nil
	}
	return monster.MonsterConfig.Monsters[m.Key].Abilities.Telegraph
}
func (g *MMGame) telegraphAllowed(m *monster.Monster3D) bool {
	if m.IsSilenced() {
		if a := telegraphDefinition(m); a != nil && a.School != "physical" {
			return false
		}
	}
	return m != nil && m.IsAlive() && !m.IsPartyControlled() && !m.Pacified && !m.BossDormant && m.CurrentAIBehavior().Caps().MayAttack && !m.Stun.Active() && !g.turnBasedMonsterStunned[m]
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
	if s.Warning.Active() || s.Released {
		return true
	}
	// Support abilities obey the same combat-target gate as offensive casts.
	// Keep an already committed warning intact when its target moves away.
	if !m.TargetsParty() && (m.AIFoe == nil || !m.AIFoe.IsAlive() || !m.CanAttackActor(m.AIFoe)) {
		return false
	}
	if !ready || s.Cooldown.Active() || s.Remaining.Active() {
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
	tps := g.config.GetTPS()
	*s = monster.TelegraphState{Cooldown: a.CooldownClock(tps), Warning: a.WarningClock(tps), Tiles: tiles, Origin: [2]int{TileIndex(m.X, ts), TileIndex(m.Y, ts)}, TargetID: targetID}
	s.Direction = cardinalDelta(target[0]-s.Origin[0], target[1]-s.Origin[1])
	g.releaseMonsterAttackPost(m)
	m.ResetPathCache()
	g.armMonsterAttackAnimation(m)
	cs.armMonsterRTAttackCooldowns(m)
	g.AddCombatMessage(m.Name + " prepares " + a.Name + ". Leave the marked tiles!")
	return true
}
func (g *MMGame) tickMonsterTelegraphs(turn bool) {
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
		s.Cooldown.Tick(turn)
		if !g.telegraphAllowed(m) {
			s.Warning, s.Remaining = status.DualClock{}, status.DualClock{}
			s.Tiles = nil
			continue
		}
		if !s.Warning.Active() {
			continue
		}
		if s.Warning.Tick(turn); s.Warning.Active() {
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
			frames, rounds := config.EffectClock(a.DurationSeconds, g.config.GetTPS())
			ally.ApplySoak(monster.TelegraphShieldAbsorption(m.DamageMin, m.DamageMax), frames, rounds)
			g.AddCombatMessage(m.Name + " shields " + ally.Name + " with circulating water.")
			vx, vy := g.combat.monsterVisualPos(ally)
			g.createElementalAbilityImpact(vx, vy, "water", ally)
		}
		return
	}
	if a.Kind == "charge" {
		g.moveTelegraphMonster(m, s.Direction, a.Range)
	}
	damage := percentOfAttack(m.DamageMin, m.DamageMax, a.DamagePercent)
	parts := m.OutgoingDamage(damagecalc.Parts{Normal: damage, True: m.TrueDamage})
	if s.TargetID == "" && m.TargetsParty() {
		px, py := g.GetPlayerTilePosition()
		if slices.Contains(s.Tiles, [2]int{px, py}) && g.telegraphLOS(m, g.camera.X, g.camera.Y) {
			unblocked := g.damageTelegraphParty(m, a.Name, a.School, parts)
			g.observeOverwatchAttack(m)
			if unblocked {
				switch a.Kind {
				case "slow", "root":
					g.applyTelegraphHinder(a)
				case "push":
					g.pushTelegraphParty(s.Direction)
				}
			}
		}
	} else if s.TargetID != "" {
		for _, foe := range g.world.Monsters {
			if foe.ID != s.TargetID || !foe.IsAlive() || !m.CanAttackActor(foe) || !slices.Contains(s.Tiles, [2]int{TileIndex(foe.X, ts), TileIndex(foe.Y, ts)}) || !g.telegraphLOS(m, foe.X, foe.Y) {
				continue
			}
			g.combat.applyMonsterDamagePacket(foe, singleMonsterDamagePacket(parts, a.School, 0), monsterDamageOptions{})
			frames, rounds := config.EffectClock(a.DurationSeconds, g.config.GetTPS())
			if a.Kind == "slow" {
				foe.ApplySlow(config.HinderPercent(), frames, rounds)
			}
			if a.Kind == "root" {
				foe.Root.Refresh(frames, rounds)
			}
			if !foe.IsAlive() {
				g.combat.finishMonsterKill(foe)
			}
		}
	}
	if a.SustainDamage > 0 {
		s.Remaining = status.DualClock{}
		s.Remaining.Arm(a.DurationSeconds, g.config.GetTPS())
	}
	if a.Kind == "retreat" {
		g.moveTelegraphMonster(m, [2]int{-s.Direction[0], -s.Direction[1]}, 2)
	}
	g.armMonsterAttackAnimation(m)
	g.AddCombatMessage(m.Name + " releases " + a.Name + "!")
}

// Telegraph hits remain undodgeable, with the same absorption policy as the
// authored hazard. Retain the attacker for reflection and hit attribution.
// False means the ward blocked the whole impact, including its control rider.
// Zero damage from ordinary resistance still permits control. The member loop
// shares one ward decision and spends no charge when no member can take damage.
func (g *MMGame) damageTelegraphParty(source *monster.Monster3D, name, school string, parts damagecalc.Parts) bool {
	defer g.beginProfileMonsterHit(source, name)()
	blocked := false
	g.combat.forEachDamageablePartyMember(func(i int, c *character.MMCharacter) {
		if g.combat.magicWardBlocks(school, false, source != nil) {
			blocked = true
			return
		}
		dealt := g.combat.damagePartyMemberPartsFromSource(i, c, parts, school, false, source)
		if dealt > 0 {
			g.reportDamage(logToneBad, damageLog{
				Source: logMonsterAs(source, name),
				Target: heroLogTarget(c),
				Amount: dealt,
				School: school,
			})
		}
	})
	return !blocked
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
	g.forcePartyStep(x+dir[0], y+dir[1])
}
func (g *MMGame) monsterFieldExposures(turn bool) []environmentExposure {
	var out []environmentExposure
	if g.world == nil {
		return out
	}
	x, y := g.GetPlayerTilePosition()
	for _, m := range g.world.Monsters {
		a := telegraphDefinition(m)
		if a == nil || a.SustainDamage <= 0 || !m.Telegraph.Remaining.Active() {
			continue
		}
		s := &m.Telegraph
		// Whole rounds: the field spans the same seconds in both modes.
		tps := g.config.GetTPS()
		elapsed := min(tickSeconds(turn, tps), float64(s.Remaining.Frames)/float64(tps))
		s.Remaining.Tick(turn)
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
				if damage := status.Accrue(&s.Carry, float64(rate)*elapsed, 1); damage > 0 {
					g.combat.applyMonsterDamagePacket(foe, singleMonsterDamagePacket(damagecalc.Parts{Normal: damage}, a.School, 0), monsterDamageOptions{})
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
		if m == nil || !m.IsAlive() || (!m.Telegraph.Warning.Active() && !m.Telegraph.Remaining.Active()) {
			continue
		}
		rgb := [3]int{255, 235, 90}
		if !m.Telegraph.Warning.Active() {
			rgb = [3]int{255, 90, 30}
		}
		for _, tile := range m.Telegraph.Tiles {
			r.collectAuraTileEdges(tile[0], tile[1], ts, density, alpha, float64(radius)*ts, rgb)
		}
	}
}

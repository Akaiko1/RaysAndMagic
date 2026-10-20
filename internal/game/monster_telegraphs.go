package game

import (
	"math"
	"slices"

	uitext "ugataima/assets/text"
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
	if a.Kind == monster.TelegraphRoot {
		g.rootParty(config.CombatRounds(a.DurationSeconds))
		g.AddCombatMessage(uitext.Text("combat.telegraph.root", a.Name))
	} else {
		h.Slow.Arm(a.DurationSeconds, tps)
		g.AddCombatMessage(uitext.Text("combat.telegraph.slow", a.Name))
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

// monsterTelegraph is the monster's own telegraph (auto-level scales it).
func monsterTelegraph(m *monster.Monster3D) *monster.TelegraphedAttack {
	if m == nil {
		return nil
	}
	return m.Abilities.Telegraph
}

// telegraphAllowed: every telegraph is elemental, so silence stops it.
func (g *MMGame) telegraphAllowed(m *monster.Monster3D) bool {
	return m != nil && !m.IsSilenced() && m.IsAlive() && !m.IsPartyControlled() && !m.Pacified && !m.BossDormant && m.CurrentAIBehavior().Caps().MayAttack && !m.Stun.Active() && !g.turnBasedMonsterStunned[m]
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
	case monster.TelegraphTarget:
		add(target[0], target[1])
	case monster.TelegraphBurst:
		for y := -a.Range; y <= a.Range; y++ {
			for x := -a.Range; x <= a.Range; x++ {
				if x*x+y*y <= a.Range*a.Range {
					add(origin[0]+x, origin[1]+y)
				}
			}
		}
	case monster.TelegraphLane, monster.TelegraphCone:
		for d := 1; d <= a.Range; d++ {
			width := 0
			if a.Shape == monster.TelegraphCone {
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
	a := monsterTelegraph(m)
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
	if a.Kind == monster.TelegraphShield {
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
	g.AddCombatMessage(uitext.Text("combat.telegraph.prepare", m.Name, a.Name))
	return true
}
func (g *MMGame) tickMonsterTelegraphs(turn bool) {
	if g.world == nil {
		return
	}
	for _, m := range g.world.Monsters {
		a := monsterTelegraph(m)
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
	if a.Kind == monster.TelegraphShield {
		for _, ally := range g.world.Monsters {
			if ally.ID != s.TargetID || !ally.IsAlive() || ally.IsPartyControlled() || ally.Pacified || m.CanAttackActor(ally) || !g.telegraphLOS(m, ally.X, ally.Y) || Distance(m.X, m.Y, ally.X, ally.Y) > float64(a.Range)*ts {
				continue
			}
			frames, rounds := config.EffectClock(a.DurationSeconds, g.config.GetTPS())
			ally.ApplySoak(monster.TelegraphShieldAbsorption(m.DamageMin, m.DamageMax), frames, rounds)
			g.AddCombatMessage(uitext.Text("combat.telegraph.shield", m.Name, ally.Name, a.Name))
			vx, vy := g.combat.monsterVisualPos(ally)
			g.createElementalAbilityImpact(vx, vy, a.School.String(), ally)
		}
		return
	}
	if a.Kind == monster.TelegraphCharge {
		g.moveTelegraphMonster(m, s.Direction, a.Range)
	}
	damage := attackShare(m, a.DamagePercent)
	parts := m.OutgoingDamage(damagecalc.Parts{Normal: damage, True: m.TrueDamage})
	if s.TargetID == "" && m.TargetsParty() {
		px, py := g.GetPlayerTilePosition()
		if slices.Contains(s.Tiles, [2]int{px, py}) && g.telegraphLOS(m, g.camera.X, g.camera.Y) {
			if g.strikeTelegraphParty(m, a.Name, a.School.String(), parts, a.Riders.Carried()) {
				switch {
				case a.Kind.Hinders():
					g.applyTelegraphHinder(a)
				case a.Kind == monster.TelegraphPush:
					g.pushTelegraphParty(s.Direction)
				}
			}
		}
	} else if s.TargetID != "" {
		for _, foe := range g.world.Monsters {
			if foe.ID != s.TargetID || !foe.IsAlive() || !m.CanAttackActor(foe) || !slices.Contains(s.Tiles, [2]int{TileIndex(foe.X, ts), TileIndex(foe.Y, ts)}) || !g.telegraphLOS(m, foe.X, foe.Y) {
				continue
			}
			g.combat.landMonsterOnMonster(m, foe, monsterOnMonsterHit{
				packet:   singleMonsterDamagePacket(parts, a.School.String(), 0),
				canDodge: true, kind: damageLogHit, evades: a.Name, riders: a.Riders.Carried(),
			})
			// The lane's control lands like on a hero: a dodge evades only
			// the damage.
			if foe.IsAlive() {
				frames, rounds := config.EffectClock(a.DurationSeconds, g.config.GetTPS())
				if a.Kind == monster.TelegraphSlow {
					foe.ApplySlow(config.HinderPercent(), frames, rounds)
				}
				if a.Kind == monster.TelegraphRoot {
					foe.Root.Refresh(frames, rounds)
				}
			}
		}
	}
	if a.SustainPercent > 0 {
		s.Remaining = status.DualClock{}
		s.Remaining.Arm(a.DurationSeconds, g.config.GetTPS())
	}
	if a.Kind == monster.TelegraphRetreat {
		g.moveTelegraphMonster(m, [2]int{-s.Direction[0], -s.Direction[1]}, 2)
	}
	g.armMonsterAttackAnimation(m)
	g.AddCombatMessage(uitext.Text("combat.telegraph.release", m.Name, a.Name))
}

// strikeTelegraphParty releases a telegraph on the party: one monster action,
// each member taking its own hit with its own dodge, and the monster's on-hit
// riders when the telegraph authors riders: true. A special is not a spell
// (no absorption) nor a weapon shot (no ranged ward). False means the ward
// blocked the whole impact, including its control rider; zero damage from
// ordinary resistance or a dodge still permits control.
func (g *MMGame) strikeTelegraphParty(source *monster.Monster3D, name, school string, parts damagecalc.Parts, riders bool) (unblocked bool) {
	cs := g.combat
	hit := monsterCharacterHit{Parts: parts, DamageType: school, NoRiders: !riders}
	unblocked = true
	cs.monsterActionScope(source, nil, func() bool {
		cs.forEachDamageablePartyMember(func(_ int, c *character.MMCharacter) {
			cs.monsterHitCharacter(source, c, name, hit)
			unblocked = unblocked && !cs.magicWardBlockedEvent()
		})
		return true
	})
	return unblocked
}

// burnTelegraphFieldParty is a telegraph's lingering field pulsing on the
// party: a hazard over time, undodgeable like the chamber fields. The member
// loop shares one ward decision and spends no charge when no member can take
// damage.
func (g *MMGame) burnTelegraphFieldParty(source *monster.Monster3D, name, school string, parts damagecalc.Parts) {
	defer g.beginProfileMonsterHit(source, name)()
	g.combat.forEachDamageablePartyMember(func(i int, c *character.MMCharacter) {
		if g.combat.magicWardBlocks(school, false, source != nil) {
			return
		}
		dealt, shares := g.combat.damagePartyMemberPartsFromSource(i, c, parts, school, false, source)
		if dealt > 0 {
			g.reportDamage(logToneBad, damageLog{
				Source: logMonsterAs(source, name),
				Target: heroLogTarget(c),
				Amount: dealt,
				School: school,
				Shares: shares,
			})
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
	g.forcePartyStep(x+dir[0], y+dir[1])
}
func (g *MMGame) monsterFieldExposures(turn bool) []environmentExposure {
	var out []environmentExposure
	if g.world == nil {
		return out
	}
	x, y := g.GetPlayerTilePosition()
	for _, m := range g.world.Monsters {
		a := monsterTelegraph(m)
		if a == nil || a.SustainPercent <= 0 || !m.Telegraph.Remaining.Active() {
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
		// A field is normal damage per second (a share of the attack), not a
		// new weapon hit.
		rate := m.OutgoingDamage(damagecalc.Parts{Normal: attackShare(m, a.SustainPercent)}).Normal
		if s.TargetID == "" && m.TargetsParty() && slices.Contains(s.Tiles, [2]int{x, y}) {
			out = append(out, environmentExposure{effect: config.EnvironmentEffect{Name: a.Name, School: a.School.String(), Damage: rate}, seconds: elapsed, source: m})
		} else if s.TargetID != "" {
			for _, foe := range g.world.Monsters {
				ts := g.config.GetTileSize()
				if foe.ID != s.TargetID || !foe.IsAlive() || !m.CanAttackActor(foe) || !slices.Contains(s.Tiles, [2]int{TileIndex(foe.X, ts), TileIndex(foe.Y, ts)}) {
					continue
				}
				if damage := status.Accrue(&s.Carry, float64(rate)*elapsed, 1); damage > 0 {
					// A field pulse is a hazard over time: no dodge, no riders.
					g.combat.landMonsterOnMonster(m, foe, monsterOnMonsterHit{
						packet: singleMonsterDamagePacket(damagecalc.Parts{Normal: damage}, a.School.String(), 0), source: a.Name,
						kind: damageLogHit,
					})
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

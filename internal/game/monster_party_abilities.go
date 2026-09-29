package game

import (
	"fmt"
	"math"
	"math/rand"

	"ugataima/internal/monster"
	"ugataima/internal/status"
)

// PartyRootState shares the rated status clock with monster roots and stuns.
// Root restricts voluntary translation only; turning and combat remain legal.
type PartyRootState struct {
	Frames int `json:"frames,omitempty"`
	Turns  int `json:"turns,omitempty"`
	Rate   int `json:"rate,omitempty"`
}

func (g *MMGame) partyRooted() bool { return g.partyRoot.Frames > 0 || g.partyRoot.Turns > 0 }
func (g *MMGame) tickPartyRoot(turn bool) {
	r := &g.partyRoot
	if turn {
		status.TickTurnRated(&r.Turns, &r.Frames, &r.Rate)
	} else {
		status.TickFrameRated(&r.Frames, &r.Turns, &r.Rate)
	}
}

// indexAuthoredBands visits the roster once per simulation pass. Peers are
// indexed by saved encounter instance, never by the reusable content tag alone.
// Legacy/debug placements without an instance bind only near their spawn point.
func (g *MMGame) indexAuthoredBands() map[string][]*monster.Monster3D {
	if g.world == nil {
		return nil
	}
	var actors []*monster.Monster3D
	for _, m := range g.world.Monsters {
		if m != nil && m.BandGroup != "" {
			actors = append(actors, m)
		}
	}
	if len(actors) == 0 {
		return nil
	}
	sortMonstersByID(actors)
	radius := 3 * float64(g.config.GetTileSize())
	for _, m := range actors {
		if m.BandInstance != "" {
			continue
		}
		for _, peer := range actors {
			if peer == m || peer.BandInstance == "" || peer.BandGroup != m.BandGroup {
				continue
			}
			if math.Hypot(peer.SpawnX-m.SpawnX, peer.SpawnY-m.SpawnY) <= radius {
				m.BandInstance = peer.BandInstance
				break
			}
		}
		if m.BandInstance == "" {
			m.BandInstance = "spawn:" + m.ID
		}
	}
	groups := make(map[string][]*monster.Monster3D)
	for _, m := range actors {
		groups[m.BandInstance] = append(groups[m.BandInstance], m)
	}
	for _, peers := range groups {
		for _, m := range peers {
			m.BandPeers = peers
		}
	}
	return groups
}

// Sight shares a normal leashed engagement. Only a real attack creates sticky
// retaliation, and a charmed/bound peer remains outside either transition.
func (g *MMGame) updateAuthoredBandAggro() {
	if g.camera == nil {
		return
	}
	for _, peers := range g.indexAuthoredBands() {
		sight, hit := false, false
		for _, m := range peers {
			if m.IsPartyControlled() {
				continue
			}
			hit = hit || m.WasAttacked
			if !m.IsAlive() || m.AIFoe != nil {
				continue
			}
			sight = sight || (m.IsEngagingPlayer && !m.ShouldDisengageFromPlayer(g.collisionSystem, g.camera.X, g.camera.Y)) || m.CanStartPlayerEngagement(g.collisionSystem, g.camera.X, g.camera.Y)
		}
		for _, m := range peers {
			if !m.IsAlive() || m.IsPartyControlled() || m.AIFoe != nil {
				continue
			}
			if hit {
				m.WasAttacked = true
			}
			if hit || sight {
				if !m.IsEngagingPlayer {
					m.BeginPlayerEngagement()
				}
			} else if m.IsEngagingPlayer {
				m.EndPlayerEngagement()
			}
		}
	}
}

// Damage is event-driven and visits this encounter's peers only. The fallback
// builds an index for synchronous damage before the first simulation frame.
func (g *MMGame) rallyAuthoredBandHit(target *monster.Monster3D) {
	if target == nil || target.BandGroup == "" || !target.WasAttacked {
		return
	}
	if target.BandPeers == nil {
		g.indexAuthoredBands()
	}
	for _, peer := range target.BandPeers {
		if !peer.IsAlive() || peer.IsPartyControlled() {
			continue
		}
		peer.WasAttacked = true
		if !peer.IsEngagingPlayer && peer.AIFoe == nil {
			peer.BeginPlayerEngagement()
		}
	}
}

// Called once at the committed party attack boundary, never on movement ticks
// or separately for each hit in a multi-attack action.
func (cs *CombatSystem) tryMonsterPartyAbilities(m *monster.Monster3D, cadence monsterAttackCadence) {
	if m.RootPartyChance > 0 && rand.Float64() < m.RootPartyChance {
		r := &cs.game.partyRoot
		status.RefreshDualRated(&r.Frames, &r.Turns, &r.Rate, m.RootPartySeconds*cs.game.config.GetTPS(), m.RootPartyTurns)
		cs.game.AddCombatMessage(fmt.Sprintf("%s roots the party in place!", m.Name))
	}
	if m.RearBlinkChance > 0 && !cs.game.monsterMovementHeld(m) && rand.Float64() < m.RearBlinkChance {
		cs.blinkBehindParty(m, cadence)
	}
}

// Seek the rear firing lane, farthest first. Both modes use cardinal lanes so
// the resulting immediate shot is legal in TB as well as RT.
func (cs *CombatSystem) blinkBehindParty(m *monster.Monster3D, cadence monsterAttackCadence) bool {
	g := cs.game
	if g.collisionSystem == nil {
		return false
	}
	tile := float64(g.config.GetTileSize())
	px, py := cs.logicalCameraXY()
	tx, ty := TileIndex(px, tile), TileIndex(py, tile)
	angle := g.camera.Angle
	fx, fy := math.Cos(angle), math.Sin(angle)
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	oldX, oldY := m.X, m.Y
	// Releasing an attack post resets the AI timer. This relocation is inside
	// an already committed action, so preserve its attack moment.
	oldState, oldTimer := m.State, m.StateTimer
	defer func() { m.State, m.StateTimer = oldState, oldTimer }()
	bestDot := 0.0
	var rear [2]int
	for _, d := range dirs {
		if dot := float64(d[0])*fx + float64(d[1])*fy; dot < bestDot {
			bestDot, rear = dot, d
		}
	}
	for n := int(m.RearBlinkRangeTiles); n >= 2; n-- {
		x, y := TileCenterFromTile(tx+rear[0]*n, ty+rear[1]*n, tile)
		if g.collisionSystem.IsMonsterAttackPostReserved(m.ID, x, y) || !g.collisionSystem.CanMoveToWithTileOverrides(m.ID, x, y, m.WalkableTileOverrides, m.Flying) || !cs.attackLineClear(x, y, px, py) {
			continue
		}
		g.releaseMonsterAttackPost(m)
		m.X, m.Y = x, y
		g.collisionSystem.UpdateEntity(m.ID, x, y)
		if !cs.monsterAttackStillValid(m, monsterAttackDestination{}, cadence) || !g.tryClaimMonsterAttackPost(m) {
			m.X, m.Y = oldX, oldY
			g.collisionSystem.UpdateEntity(m.ID, oldX, oldY)
			g.tryClaimMonsterAttackPost(m)
			continue
		}
		m.ResetPathfinding()
		m.Direction = math.Atan2(py-y, px-x)
		g.spawnBlinkLightColumn(oldX, oldY)
		g.spawnBlinkLightColumn(x, y)
		g.AddCombatMessage(fmt.Sprintf("%s slips behind the party!", m.Name))
		return true
	}
	return false
}

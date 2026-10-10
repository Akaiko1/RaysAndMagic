package game

import (
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// The context identifies the physical world, not a cached answer about scope.
// PartyScope is read during the serial phase and parallel AI phases, while
// logical camera/world ownership is stable within each phase.
type monsterAggroContext struct {
	game  *MMGame
	world *world.World3D
}

func (c monsterAggroContext) PartyScope() (bool, string) {
	g := c.game
	if g.world != c.world {
		return false, ""
	}
	if g.camera == nil || g.config == nil {
		return true, ""
	}
	ts := g.config.GetTileSize()
	return true, g.mapKeyAtTile(TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts))
}

// Departure always cancels temporary retaliation too. It never heals the actor
// or erases provocation memory, and does not send calm actors on a fake return.
func endWorldPursuit(w *world.World3D) {
	if w == nil {
		return
	}
	for _, m := range w.Monsters {
		if m == nil || m.IsPartyControlled() || m.IsAmbient() {
			continue
		}
		if m.TargetsParty() || m.IsEngagingPlayer || m.AIFoe != nil && m.AIFoe.IsPartyControlled() || m.State == monster.StateFleeing {
			m.EndPlayerEngagement()
		}
		m.Retaliation = monster.RetaliationState{}
		m.BandHitPending = false
		m.BossAggro = false
	}
}

func (g *MMGame) reconcileWorldPursuit(w *world.World3D) {
	if w == nil {
		return
	}
	for _, m := range w.Monsters {
		if m == nil {
			continue
		}
		g.stampMonsterHome(w, m)
		m.BossAggro = m.IsBoss() && m.AggroWholeMap && !m.BossDormant && !m.BossEvasive && !m.BossWarded
		if w == g.world && g.camera != nil {
			m.LimitPlayerEngagement(g.camera.X, g.camera.Y)
		}
	}
	if w != g.world {
		endWorldPursuit(w)
	}
}

// Loading/travel reconcile all saved worlds. A seamless biome change calls the
// current-world variant only; it has no effect on inactive maps.
func (g *MMGame) reconcileRestoredPursuit(wm *world.WorldManager) {
	if wm == nil || g.camera == nil {
		return
	}
	wm.EachWorld(func(_ string, w *world.World3D) { g.reconcileWorldPursuit(w) })
}

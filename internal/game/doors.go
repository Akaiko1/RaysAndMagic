package game

import (
	"fmt"
	"math"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/world"
)

// Champion portcullises (type "door", door_behavior "champion_portcullis")
// bar a doorway while a duel champion lives on their map: drawn and solid only
// while closed. Closed-ness is DERIVED every frame from "any living champion
// here" - no persisted door state, so it self-heals across save-load and map
// switches (same pattern as ward idols).

// npcIsDoor is the single category test for the door render class. Unlike
// npcIsWall it does not depend on standee mode: the collision block applies in
// both render modes.
func (g *MMGame) npcIsDoor(npc *character.NPC) bool {
	return npcRenderCatOf(npc) == catDoor
}

// npcDoorOpen: an open door is invisible and non-interactive - the render
// collector and the focus/click resolvers all skip through this one test. Two
// explicit door behaviors share the render class: a lockable door is open once
// unlocked (Visited); the champion portcullis is open while no champion lives.
func (g *MMGame) npcDoorOpen(npc *character.NPC) bool {
	if !g.npcIsDoor(npc) {
		return false
	}
	if character.IsLockedDoor(npc) {
		return npc.Visited // a closed locked door stays interactive so it can be opened
	}
	if character.IsChampionPortcullisDoor(npc) {
		return !g.doorsClosed
	}
	return false
}

// doorPose returns the render pose for a door: centered on its tile with the
// slab spanning the doorway (connecting the two flanking walls), which is
// perpendicular to wallStickPose's along-the-wall yaw. ok=false when the tile
// has no opposing solid pair - a mis-authored door falls back to a plain standee.
func (g *MMGame) doorPose(npcX, npcY float64) (x, y, yaw float64, ok bool) {
	w := g.GetCurrentWorld()
	if w == nil || world.GlobalTileManager == nil {
		return 0, 0, 0, false
	}
	ts := float64(g.config.GetTileSize())
	cx := (math.Floor(npcX/ts) + 0.5) * ts
	cy := (math.Floor(npcY/ts) + 0.5) * ts
	solid := func(dx, dy float64) bool {
		return world.GlobalTileManager.IsSolid(w.GetTileAt(cx+dx*ts, cy+dy*ts))
	}
	switch {
	case solid(0, -1) && solid(0, 1): // walls N+S -> slab spans N-S across an E-W passage
		return cx, cy, math.Pi / 2, true
	case solid(-1, 0) && solid(1, 0): // walls W+E -> slab spans E-W across a N-S passage
		return cx, cy, 0, true
	}
	return 0, 0, 0, false
}

// refreshDoors recomputes the shared closed state each frame and reconciles
// the solid door collision entities only on the closed/open TRANSITION (door
// NPC sets are static per map; map switches reset the state via
// clearDoorState, so arrival on a duel map re-runs the reconcile).
func (g *MMGame) refreshDoors() {
	closed := g.livingChampion() != nil
	if closed == g.doorsClosed {
		return
	}
	if g.doorsClosed && !closed {
		g.AddCombatMessage("The portcullises rise - the way is open!")
	}
	g.doorsClosed = closed

	if g.collisionSystem == nil {
		return
	}
	if !closed {
		g.clearDoorEntities()
		return
	}
	if g.doorEntityIDs == nil {
		g.doorEntityIDs = make(map[string]bool)
	}
	ts := float64(g.config.GetTileSize())
	for _, npc := range g.world.NPCs {
		if !character.IsChampionPortcullisDoor(npc) {
			continue // locked doors have their own persisted per-door registration
		}
		id := fmt.Sprintf("door:%.0f:%.0f", npc.X, npc.Y)
		if !g.doorEntityIDs[id] {
			g.collisionSystem.RegisterEntity(collision.NewEntity(id, npc.X, npc.Y, ts*0.9, ts*0.9, collision.CollisionTypeNPC, true))
			g.doorEntityIDs[id] = true
		}
	}
}

// clearDoorEntities unregisters every door collision entity.
func (g *MMGame) clearDoorEntities() {
	for id := range g.doorEntityIDs {
		if g.collisionSystem != nil {
			g.collisionSystem.UnregisterEntity(id)
		}
		delete(g.doorEntityIDs, id)
	}
}

// clearDoorState drops the door state on map switch/load: entities unregister
// and doorsClosed resets, so the rise message can never fire on a map the duel
// isn't on, and arriving back on the duel map re-closes the gates cleanly.
func (g *MMGame) clearDoorState() {
	g.clearDoorEntities()
	g.doorsClosed = false
}

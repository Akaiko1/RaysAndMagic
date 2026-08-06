package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
)

// A landmark NPC (church, tower, gate) is a building: the party walks around
// it, never through it. People stay pass-through.
func TestLandmarkNPCBlocksPartyMovement(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	g := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	ih := &InputHandler{game: g}
	ts := float64(cfg.GetTileSize())

	walkInto := func(npcKey string) float64 {
		t.Helper()
		npc, err := character.CreateNPCFromConfig(npcKey, 5.5*ts, 5.5*ts)
		if err != nil {
			t.Fatal(err)
		}
		g.world.NPCs = []*character.NPC{npc}
		g.clearBuildingEntities()
		g.registerBuildingFootprints()
		g.camera.X, g.camera.Y = 5.5*ts, 8.0*ts
		g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
		minDist := math.Inf(1)
		for i := 0; i < 80; i++ {
			ih.movePlayer(0, -4)
			if d := math.Hypot(g.camera.X-npc.X, g.camera.Y-npc.Y) / ts; d < minDist {
				minDist = d
			}
		}
		return minDist
	}

	// Blocked = the closest approach stays at the landmark's box edge (0.475
	// tile) plus the party's half-width; pass-through = the walk crosses the
	// NPC's tile center.
	if dist := walkInto("abandoned_church"); dist < 0.55 {
		t.Fatalf("party reached %.2f tiles from the church center - walked into it", dist)
	}
	// A person must stay pass-through: villagers never wall the party.
	if dist := walkInto("desert_merchant"); dist > 0.3 {
		t.Fatalf("party never crossed the merchant tile (closest %.2f tiles) - a person blocked movement", dist)
	}
}

// A pre-solidity save can hold the party ON a landmark tile. Registering the
// box around them would wall them in with no move out, so that landmark is
// skipped for the visit.
func TestLandmarkNPCDoesNotTrapPartyStandingOnIt(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	g := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	ih := &InputHandler{game: g}
	ts := float64(cfg.GetTileSize())

	npc, err := character.CreateNPCFromConfig("abandoned_church", 5.5*ts, 5.5*ts)
	if err != nil {
		t.Fatal(err)
	}
	g.world.NPCs = []*character.NPC{npc}
	g.camera.X, g.camera.Y = 5.5*ts, 5.5*ts // the party IS on the church tile
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
	g.clearBuildingEntities()
	g.registerBuildingFootprints()

	for i := 0; i < 40; i++ {
		ih.movePlayer(0, 4)
	}
	if dist := math.Hypot(g.camera.X-npc.X, g.camera.Y-npc.Y) / ts; dist < 1.0 {
		t.Fatalf("party is walled in on the landmark tile (moved only %.2f tiles)", dist)
	}
}

// Map switches register static collision BEFORE finishMapArrival places the
// party, so the on-tile skip judges the OLD map's coordinates. The refresh at
// the final position must cover both stale outcomes: a destination landmark
// falsely left passable, and the party arriving onto a landmark tile that was
// already registered.
func TestRefreshLandmarkCollisionUsesFinalArrivalPosition(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	g := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	ih := &InputHandler{game: g}
	ts := float64(cfg.GetTileSize())

	npc, err := character.CreateNPCFromConfig("abandoned_church", 5.5*ts, 5.5*ts)
	if err != nil {
		t.Fatal(err)
	}
	g.world.NPCs = []*character.NPC{npc}

	// Stale skip: the pre-arrival camera happens to sit on the landmark's
	// coordinates, so registration leaves it passable...
	g.camera.X, g.camera.Y = npc.X, npc.Y
	g.clearBuildingEntities()
	g.registerBuildingFootprints()
	// ...then the party actually arrives elsewhere; the refresh must solidify it.
	g.camera.X, g.camera.Y = 5.5*ts, 8.0*ts
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
	g.refreshLandmarkCollision()
	minDist := math.Inf(1)
	for i := 0; i < 80; i++ {
		ih.movePlayer(0, -4)
		if d := math.Hypot(g.camera.X-npc.X, g.camera.Y-npc.Y) / ts; d < minDist {
			minDist = d
		}
	}
	if minDist < 0.55 {
		t.Fatalf("stale-skip landmark stayed passable after refresh (reached %.2f tiles)", minDist)
	}

	// Arrival ONTO the landmark: registered against far-away stale coordinates,
	// then the party lands on its tile; the refresh must free them.
	g.clearBuildingEntities()
	g.camera.X, g.camera.Y = 15.5*ts, 15.5*ts
	g.registerBuildingFootprints()
	g.camera.X, g.camera.Y = npc.X, npc.Y
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
	g.refreshLandmarkCollision()
	for i := 0; i < 40; i++ {
		ih.movePlayer(0, 4)
	}
	if dist := math.Hypot(g.camera.X-npc.X, g.camera.Y-npc.Y) / ts; dist < 1.0 {
		t.Fatalf("party walled in after arriving onto a landmark tile (moved %.2f tiles)", dist)
	}
}

// The on-tile skip is the EXACT box intersection, not a radius: a save 0.8
// tiles from a landmark does not overlap it and must NOT leave it passable.
func TestLandmarkNearbyButNotOverlappingStaysSolid(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	g := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	ih := &InputHandler{game: g}
	ts := float64(cfg.GetTileSize())

	npc, err := character.CreateNPCFromConfig("abandoned_church", 5.5*ts, 5.5*ts)
	if err != nil {
		t.Fatal(err)
	}
	g.world.NPCs = []*character.NPC{npc}

	// Registration happens with the party 0.8 tiles away on both axes - inside
	// the old 1-tile radius, outside the real boxes.
	g.camera.X, g.camera.Y = npc.X+0.8*ts, npc.Y+0.8*ts
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
	g.clearBuildingEntities()
	g.registerBuildingFootprints()

	// Walk into the church from the south: it must be solid.
	g.camera.X, g.camera.Y = npc.X, npc.Y+2.5*ts
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
	minDist := math.Inf(1)
	for i := 0; i < 80; i++ {
		ih.movePlayer(0, -4)
		if d := math.Hypot(g.camera.X-npc.X, g.camera.Y-npc.Y) / ts; d < minDist {
			minDist = d
		}
	}
	if minDist < 0.55 {
		t.Fatalf("landmark registered near (not overlapping) the party stayed passable (reached %.2f tiles)", minDist)
	}
}

package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
)

// Landmark collision case table. A landmark NPC (church, tower, gate) is a
// building: the party walks around it, never through it; people stay
// pass-through. A pre-solidity save can hold the party ON a landmark tile, so
// registration skips a landmark whose box overlaps the party (exact box
// intersection, not a radius). Map switches register BEFORE finishMapArrival
// places the party, so the refresh at the final position must cover both stale
// outcomes. Positions are tile offsets from the NPC.
func TestLandmarkCollisionCases(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	type offset struct{ x, y float64 }
	const (
		blocked = "blocked" // closest approach stays at the box edge (0.475 tile) plus the party's half-width
		crossed = "crossed" // the walk crosses the NPC's tile center
		escapes = "escapes" // the party walks off a tile it started on
	)
	south := offset{0, 2.5}
	for _, tc := range []struct {
		name     string
		npcKey   string
		register offset  // party position when collision is registered
		refresh  *offset // party position for refreshLandmarkCollision, nil = no refresh
		start    offset  // walk start; blocked/crossed walk north, escapes walks south
		want     string
	}{
		{name: "landmark blocks the party", npcKey: "abandoned_church", register: south, start: south, want: blocked},
		{name: "person stays pass-through", npcKey: "desert_merchant", register: south, start: south, want: crossed},
		{name: "party on the landmark is not trapped", npcKey: "abandoned_church", start: offset{}, want: escapes},
		{name: "stale skip solidified at arrival", npcKey: "abandoned_church", refresh: &south, start: south, want: blocked},
		{name: "arrival onto a registered landmark frees the party", npcKey: "abandoned_church", register: offset{10, 10}, refresh: &offset{}, start: offset{}, want: escapes},
		{name: "near but not overlapping stays solid", npcKey: "abandoned_church", register: offset{0.8, 0.8}, start: south, want: blocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
			ih := &InputHandler{game: g}
			ts := float64(cfg.GetTileSize())
			npc, err := character.CreateNPCFromConfig(tc.npcKey, 5.5*ts, 5.5*ts)
			if err != nil {
				t.Fatal(err)
			}
			g.world.NPCs = []*character.NPC{npc}
			place := func(o offset) {
				g.camera.X, g.camera.Y = npc.X+o.x*ts, npc.Y+o.y*ts
				g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
			}
			place(tc.register)
			g.clearBuildingEntities()
			g.registerBuildingFootprints()
			if tc.refresh != nil {
				place(*tc.refresh)
				g.refreshLandmarkCollision()
			}
			place(tc.start)

			dist := func() float64 { return math.Hypot(g.camera.X-npc.X, g.camera.Y-npc.Y) / ts }
			if tc.want == escapes {
				for i := 0; i < 40; i++ {
					ih.movePlayer(0, 4)
				}
				if d := dist(); d < 1.0 {
					t.Fatalf("party is walled in on the landmark tile (moved only %.2f tiles)", d)
				}
				return
			}
			minDist := math.Inf(1)
			for i := 0; i < 80; i++ {
				ih.movePlayer(0, -4)
				minDist = math.Min(minDist, dist())
			}
			if tc.want == blocked && minDist < 0.55 {
				t.Fatalf("party reached %.2f tiles from the landmark center - walked into it", minDist)
			}
			if tc.want == crossed && minDist > 0.3 {
				t.Fatalf("party never crossed the NPC tile (closest %.2f tiles) - a person blocked movement", minDist)
			}
		})
	}
}

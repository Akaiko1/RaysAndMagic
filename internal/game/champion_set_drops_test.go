package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// TestChampionSetDropsOnePieceMax: the champions.yaml set_drops pieces roll in
// turn and the FIRST success ends the rolling - even at 100% a kill yields
// exactly ONE piece, never a full set.
func TestChampionSetDropsOnePieceMax(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatalf("load champions: %v", err)
	}
	ladder := config.ChampionSetDrops()
	if len(ladder) != 2 || ladder[0].Set != "padded" || ladder[0].Pct != 10 ||
		ladder[1].Set != "ringmail" || ladder[1].Pct != 5 {
		t.Fatalf("set_drops ladder = %v, want padded:10 then ringmail:5 IN ORDER", ladder)
	}
	if pieces := len(itemKeysOfSet("padded")); pieces != 5 {
		t.Fatalf("padded set = %d keys, want 5", pieces)
	}

	// Certainty on every roll: the FIRST piece must win and stop the chain.
	prev := config.GlobalChampionConfig.SetDrops
	config.GlobalChampionConfig.SetDrops = []config.ChampionSetDrop{{Set: "padded", Pct: 100}, {Set: "ringmail", Pct: 100}}
	t.Cleanup(func() { config.GlobalChampionConfig.SetDrops = prev })

	m := &monster.Monster3D{X: g.camera.X, Y: g.camera.Y}
	cs.rollChampionSetDrops(m)
	if len(g.groundContainers) != 1 {
		t.Fatalf("containers = %d, want 1 bag", len(g.groundContainers))
	}
	if got := len(g.groundContainers[0].Items); got != 1 {
		t.Fatalf("bag holds %d items, want exactly 1 (first success ends the rolling)", got)
	}
	before := len(g.party.Inventory)
	g.pickupGroundContainerAt(0)
	if got := len(g.party.Inventory) - before; got != 1 {
		t.Fatalf("picked up %d items, want 1", got)
	}

	// An empty ladder drops nothing.
	config.GlobalChampionConfig.SetDrops = nil
	cs.rollChampionSetDrops(m)
	if len(g.groundContainers) != 0 {
		t.Fatal("empty set_drops ladder must drop nothing")
	}
	// At 100% the winning piece is the FIRST of the FIRST authored set - the
	// order is data, not map-iteration luck.
	config.GlobalChampionConfig.SetDrops = []config.ChampionSetDrop{{Set: "ringmail", Pct: 100}, {Set: "padded", Pct: 100}}
	cs.rollChampionSetDrops(m)
	if len(g.groundContainers) != 1 || len(g.groundContainers[0].Items) != 1 {
		t.Fatal("reordered ladder must still drop exactly one piece")
	}
	if it := g.groundContainers[0].Items[0]; it.Set != "ringmail" {
		t.Fatalf("first-listed set must roll first, got a %q piece", it.Set)
	}
	g.pickupGroundContainerAt(0)
}

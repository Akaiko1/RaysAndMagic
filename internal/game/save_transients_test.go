package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

// clearTransientCombatState is the ONE cleaner for world swaps (map switch +
// save load): every in-flight slice must empty and every projectile-family
// collision entity must unregister, or leftovers hit monsters on the new map.
func TestClearTransientCombatState_DropsEverything(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)

	g.magicProjectiles = append(g.magicProjectiles, MagicProjectile{ID: "mp_1", Active: true})
	g.arrows = append(g.arrows, Arrow{ID: "ar_1", Active: true})
	for _, id := range []string{"mp_1", "ar_1"} {
		g.collisionSystem.RegisterEntity(collision.NewEntity(id, 64, 64, 8, 8, collision.CollisionTypeProjectile, false))
	}
	g.slashEffects = append(g.slashEffects, SlashEffect{ID: "sl_1"})
	g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Active: true})
	g.impactLights = append(g.impactLights, ImpactLight{X: 64, Y: 64, Radius: 2})
	g.deadMonsterIDs = append(g.deadMonsterIDs, "monster_1")
	g.turnBasedTurnSuspended = true

	g.clearTransientCombatState()

	if len(g.magicProjectiles) != 0 || len(g.arrows) != 0 {
		t.Fatalf("projectile slices not cleared: %d/%d",
			len(g.magicProjectiles), len(g.arrows))
	}
	if len(g.slashEffects) != 0 || len(g.spellHitEffects) != 0 || len(g.impactLights) != 0 {
		t.Fatalf("VFX slices not cleared: %d/%d/%d",
			len(g.slashEffects), len(g.spellHitEffects), len(g.impactLights))
	}
	if len(g.deadMonsterIDs) != 0 {
		t.Fatalf("deadMonsterIDs not cleared: %d", len(g.deadMonsterIDs))
	}
	if g.turnBasedTurnSuspended {
		t.Error("a world swap must discard a suspended turn-based turn")
	}
	for _, id := range []string{"mp_1", "ar_1"} {
		if g.collisionSystem.GetEntityByID(id) != nil {
			t.Errorf("collision entity %q must be unregistered", id)
		}
	}
}

// Merchant remainders persist per NPC, and NPC identity in saves is spawn
// coordinates - two same-named NPCs must not share state.
func TestSaveLoad_PersistsMerchantStockByNPCCoordinates(t *testing.T) {
	cfg := loadTestConfig(t)

	newStockedWorld := func() *world.World3D {
		w := newTestWorld(cfg)
		merchant := &character.NPC{
			Name: "Trader", RenderCategory: "npc", X: 96, Y: 96,
			MerchantStock: []*character.MerchantStockItem{
				{Item: items.Item{Name: "Health Potion"}, Cost: 10, Quantity: 3},
				{Item: items.Item{Name: "Dead Branch"}, Cost: 25, Quantity: 1},
			},
		}
		gateA := &character.NPC{RenderCategory: "npc", Name: "City Gate", X: 160, Y: 96}
		gateB := &character.NPC{RenderCategory: "npc", Name: "City Gate", X: 96, Y: 160}
		w.NPCs = append(w.NPCs, merchant, gateA, gateB)
		return w
	}

	wmSave := world.NewWorldManager(cfg)
	worldSave := newStockedWorld()
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": worldSave}
	wmSave.CurrentMapKey = "forest"
	game := newTestGame(cfg, worldSave)

	worldSave.NPCs[0].MerchantStock[0].Quantity = 2 // bought one potion
	worldSave.NPCs[0].MerchantStock[1].Quantity = 0 // sold out
	worldSave.NPCs[2].Visited = true                // only gateB visited

	save := game.buildSave(wmSave)

	wmLoad := world.NewWorldManager(cfg)
	worldLoad := newStockedWorld() // fresh YAML-state stock
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": worldLoad}
	wmLoad.CurrentMapKey = "forest"

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	defer func() { world.GlobalWorldManager = oldWorldManager }()

	loaded := newTestGame(cfg, worldLoad)
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	stock := worldLoad.NPCs[0].MerchantStock
	if stock[0].Quantity != 2 || stock[1].Quantity != 0 {
		t.Errorf("merchant stock not restored: got %d/%d want 2/0", stock[0].Quantity, stock[1].Quantity)
	}
	if worldLoad.NPCs[1].Visited {
		t.Errorf("gateA shares a name with gateB but must stay unvisited (coordinate identity)")
	}
	if !worldLoad.NPCs[2].Visited {
		t.Errorf("gateB visited flag lost")
	}
}

// TestPlaythroughIDLifecycle pins the run-identity invariants whose breakage
// poisons saves with divergent ids (the save-glow bug class):
//  1. legacy adoption is DETERMINISTIC - loading a pre-run-id save twice yields
//     the identical id (an interim build minted randomness here and stamped
//     sibling saves of one run with different ids);
//  2. a saved id is adopted verbatim;
//  3. the hash algorithm itself is pinned by a golden value - changing it would
//     orphan every legacy save already stamped on disk;
//  4. fresh runs mint unique ids.
func TestPlaythroughIDLifecycle(t *testing.T) {
	names := []string{"Auberon", "Gwen", "Sora", "Mirelle"}

	a := adoptPlaythroughID("", names)
	b := adoptPlaythroughID("", []string{"Mirelle", "Sora", "Gwen", "Auberon"}) // order-independent
	if a != b {
		t.Fatalf("legacy adoption not deterministic/order-independent: %q vs %q", a, b)
	}
	if got, want := a, "legacy-27926ec1084d05a1"; got != want {
		t.Fatalf("legacy id algorithm changed: %q, want pinned %q (changing it orphans stamped saves)", got, want)
	}

	if got := adoptPlaythroughID("some-run", names); got != "some-run" {
		t.Fatalf("saved id must be adopted verbatim, got %q", got)
	}

	if mintPlaythroughID() == mintPlaythroughID() {
		t.Fatal("fresh runs must mint unique ids")
	}
}

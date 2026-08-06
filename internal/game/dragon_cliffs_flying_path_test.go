package game

import (
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
)

func TestGreenDragonCanPursueAcrossPinnacleChasm(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "dragon_cliffs")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("dragon_cliffs world did not load")
	}

	tile := float64(cfg.GetTileSize())
	partyX, partyY := TileCenterFromTile(26, 19, tile)
	dragonX, dragonY := TileCenterFromTile(26, 15, tile)
	dragon := monsterPkg.NewMonster3DFromConfig(dragonX, dragonY, "dragon_green", cfg)
	if dragon == nil {
		t.Fatal("dragon_green config did not load")
	}
	dragon.ID = "pinnacle_guard"
	dragon.WasAttacked = true
	dragon.BeginPlayerEngagement()

	w.Monsters = []*monsterPkg.Monster3D{dragon}
	checker := collision.NewCollisionSystem(w, tile)
	checker.RegisterEntity(collision.NewEntity("player", partyX, partyY, 16, 16, collision.CollisionTypePlayer, false))
	w.RegisterMonstersWithCollisionSystem(checker)

	nextX, nextY, ok := dragon.NextPathStepTile(checker, partyX, partyY)
	if !ok {
		t.Fatal("flying green dragon found no route across the Pinnacle chasm")
	}
	if nextX != 26 || nextY != 16 {
		t.Fatalf("first flight step = (%d,%d), want direct chasm step (26,16)", nextX, nextY)
	}

	dragon.Flying = false
	dragon.ResetPathfinding()
	if x, y, ok := dragon.NextPathStepTile(checker, partyX, partyY); ok {
		t.Fatalf("grounded dragon crossed the chasm via (%d,%d)", x, y)
	}
}

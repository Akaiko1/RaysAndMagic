package game

import (
	"testing"

	"ugataima/internal/world"
)

// The TB front-diagonal pull must keep the TRUE-position LOS gate: a melee
// neighbour behind a wall corner cannot attack, so pulling it into a
// targetable front slot misrepresents the fight.
func TestPulledFrontSlotRequiresTruePositionLOS(t *testing.T) {
	setup := func(t *testing.T) (*MMGame, float64) {
		game, _, tile := tbBehaviorGame(t, 20, 20)
		placePlayerAtTile(game, 10, 10, tile)
		game.camera.Angle = 0 // facing +X: front diagonals are (11,9) and (11,11)
		return game, tile
	}

	t.Run("open corner pulls", func(t *testing.T) {
		game, tile := setup(t)
		wolf := spawnMonsterAtTile(game, "wolf", 11, 11, tile)
		_, _, _, pulled, ok := game.combat.pulledFrontSlot(wolf)
		if !ok || !pulled {
			t.Fatalf("clear front-diagonal melee neighbour not pulled (pulled=%v ok=%v)", pulled, ok)
		}
	})

	t.Run("walled corner does not pull", func(t *testing.T) {
		// Which corner tile the LOS ray steps through is a DDA tie-break
		// detail, so probe both diagonals and both corners and assert on every
		// configuration that actually cuts the true-position LOS.
		//
		// Measured 2026-08-03: every such configuration currently also owns the
		// fake-spot tile (sight blocking is tile-indexed, entities included), so
		// the pull is refused by EITHER the true-position gate or the fake-spot
		// LOS. This pin holds the invariant itself and starts discriminating the
		// moment the pull constants, the corner tie-break, or the sight model
		// change.
		type scene struct{ mobX, mobY, wallX, wallY int }
		scenes := []scene{
			{11, 11, 10, 11}, {11, 11, 11, 10},
			{11, 9, 10, 9}, {11, 9, 11, 10},
		}
		cutConfigs := 0
		for _, s := range scenes {
			game, tile := setup(t)
			game.world.Tiles[s.wallY][s.wallX] = world.TileWall
			mob := spawnMonsterAtTile(game, "wolf", s.mobX, s.mobY, tile)
			if game.combat.monsterMeleeAdjacentToParty(mob) {
				continue // this corner does not cut the diagonal LOS - not a test case
			}
			cutConfigs++
			if _, _, _, _, ok := game.combat.pulledFrontSlot(mob); ok {
				t.Fatalf("mob at (%d,%d) with wall (%d,%d): LOS-cut neighbour was pulled into a front slot",
					s.mobX, s.mobY, s.wallX, s.wallY)
			}
		}
		if cutConfigs == 0 {
			t.Skip("no single-corner wall cuts the diagonal LOS under the current corner rule")
		}
	})
}

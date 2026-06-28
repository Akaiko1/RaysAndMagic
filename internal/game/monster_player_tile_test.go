package game

import (
	"testing"
)

// A pursuing melee monster must hold one tile out: the player is a non-solid
// collision entity, so without the entersTargetTile guard a pixel final-approach
// step would carry the monster onto the party tile and overlap their sprite
// ("wolf on the head"). This asserts the guarantee for every melee archetype:
// during RT pursuit of a stationary party the monster (a) never occupies the
// player's tile and (b) still closes to tile-adjacency and lands a hit — proving
// the standoff distance is a real attack position, independent of attack_radius
// or sprite size.
func TestRealTime_MeleePursuerNeverEntersPlayerTileButStillHits(t *testing.T) {
	const ptx, pty = 15, 15
	// One per melee archetype (varied size/speed/attack_radius). All have a clear
	// straight lane to the party so the only thing that can stop them short is the
	// reach/standoff logic, not terrain.
	for _, key := range []string{"wolf", "orc", "bear", "goblin", "troll", "treant", "skeleton", "minotaur"} {
		t.Run(key, func(t *testing.T) {
			game, _, ts := tbBehaviorGame(t, 40, 40)
			game.turnBasedMode = false
			cs := game.combat
			placePlayerAtTile(game, ptx, pty, ts)

			m := spawnMonsterAtTile(game, key, ptx, pty-4, ts) // 4 tiles north, open lane
			if m.HasRangedAttack() {
				t.Skipf("%s is not melee", key)
			}
			m.State = 2 // StatePursuing
			m.AttackCDFrames = 0

			hp0 := partyHPSum(game)
			reachedAdjacent := false
			for i := 0; i < 720; i++ { // 6s at 120 TPS — covers even the slowest (treant, speed 0.8)
				m.Update(game.collisionSystem, game.camera.X, game.camera.Y)
				cs.HandleMonsterInteractions()

				mtx, mty := monsterTileCoords(m, ts)
				if mtx == ptx && mty == pty {
					t.Fatalf("%s entered the player's tile (%d,%d) at tick %d — overlap on the party", key, ptx, pty, i)
				}
				if abs(mtx-ptx) <= 1 && abs(mty-pty) <= 1 {
					reachedAdjacent = true
				}
			}

			if !reachedAdjacent {
				mtx, mty := monsterTileCoords(m, ts)
				t.Fatalf("%s never reached a tile adjacent to the party (stuck at (%d,%d))", key, mtx, mty)
			}
			if partyHPSum(game) >= hp0 {
				t.Fatalf("%s reached the standoff tile but never landed a hit (HP unchanged)", key)
			}
		})
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

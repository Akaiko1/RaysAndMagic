package game

import (
	"slices"
	"testing"

	"ugataima/internal/monster"
)

// ordinaryMeleeMonsterKeys lists every authored monster that pursues and
// swings in melee: no ranged attack, no special disposition, boss, champion,
// idol, passivity, flight or terrain override.
func ordinaryMeleeMonsterKeys(t *testing.T) []string {
	t.Helper()
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	keys := monster.MonsterConfig.GetAllMonsterKeys()
	slices.Sort(keys)
	var out []string
	for _, key := range keys {
		def := monster.MonsterConfig.Monsters[key]
		if def.DamageMax <= 0 || def.Speed <= 0 || def.Disposition != "" || def.Boss || def.Champion != "" ||
			def.WarlordIdol || def.PassiveUntilHit || def.Flying || len(def.WalkableTileOverrides) > 0 {
			continue
		}
		if monster.NewMonster3DFromConfig(0, 0, key, nil).HasRangedAttack() {
			continue
		}
		out = append(out, key)
	}
	if len(out) == 0 {
		t.Fatal("no ordinary melee monster authored")
	}
	return out
}

// A pursuing melee monster must hold one tile out. This asserts the guarantee
// for every ordinary melee monster: during RT pursuit of a stationary party the
// monster (a) never occupies the player's tile and (b) still closes to
// tile-adjacency and lands a hit - proving the standoff distance is a real
// attack position, independent of attack_radius or sprite size.
func TestRealTime_MeleePursuerNeverEntersPlayerTileButStillHits(t *testing.T) {
	const ptx, pty = 15, 15
	// All have a clear straight lane to the party so the only thing that can stop
	// them short is the reach/standoff logic, not terrain.
	for _, key := range ordinaryMeleeMonsterKeys(t) {
		t.Run(key, func(t *testing.T) {
			game, _, ts := tbBehaviorGame(t, 40, 40)
			game.turnBasedMode = false
			cs := game.combat
			placePlayerAtTile(game, ptx, pty, ts)

			m := spawnMonsterAtTile(game, key, ptx, pty-4, ts) // 4 tiles north, open lane
			m.State = monster.StatePursuing
			m.AttackCDFrames = 0

			hp0 := partyHPSum(game)
			reachedAdjacent := false
			for i := 0; i < 720; i++ { // 6s at 120 TPS - covers even a speed-0.8 walker
				m.Update(game.collisionSystem, game.camera.X, game.camera.Y)
				cs.HandleMonsterInteractions()

				mtx, mty := monsterTileCoords(m, ts)
				if mtx == ptx && mty == pty {
					t.Fatalf("%s entered the player's tile (%d,%d) at tick %d - overlap on the party", key, ptx, pty, i)
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

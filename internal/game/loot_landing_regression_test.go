package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestMonsterDropsRemainCollectibleWithoutDeathAnimation(t *testing.T) {
	for _, animated := range []bool{false, true} {
		for _, terrainKey := range []string{"empty", "water", "dragon_cliffs_chasm_floor", "dragon_cliffs_boulder", "rock_cluster", "solid_prop"} {
			for _, restored := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/animated=%v/restored=%v", terrainKey, animated, restored), func(t *testing.T) {
					g := deathTestGame(t)
					previous := world.GlobalTileManager
					t.Cleanup(func() { world.GlobalTileManager = previous })
					tm := world.NewTileManager(testTileSizeClasses())
					if err := tm.LoadTileConfig("assets/tiles.yaml"); err != nil {
						t.Fatal(err)
					}
					world.GlobalTileManager = tm
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
					setTestWorldManager(t, wm)
					ts := g.config.GetTileSize()
					m := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, "bandit", g.config)
					if !animated {
						def, err := monster.MonsterConfig.GetMonsterByKey("bandit")
						if err != nil {
							t.Fatal(err)
						}
						copy := *def
						copy.Sprite = "test_missing_death_animation"
						m.SetupMonsterFromConfig(&copy)
					}
					m.Flying = true
					key := terrainKey
					if key == "rock_cluster" {
						key = "dragon_cliffs_boulder"
					}
					if key == "solid_prop" {
						key = "empty"
					}
					ground, _ := tm.GetTileTypeFromKey(key)
					g.world.Tiles[3][3] = ground
					if terrainKey == "rock_cluster" {
						for y := 2; y <= 4; y++ {
							for x := 2; x <= 4; x++ {
								g.world.Tiles[y][x] = ground
							}
						}
					}
					if terrainKey == "solid_prop" {
						g.collisionSystem.RegisterEntity(collision.NewEntity("building", m.X, m.Y, ts*.9, ts*.9, collision.CollisionTypeNPC, true))
					}
					g.addMonsterLootDrop(m, nil, 17)
					if len(g.groundContainers) != 1 {
						t.Fatal("drop missing")
					}
					bag := g.groundContainers[0]
					if !animated && bag.hop.duration != 0 {
						t.Fatal("immediate drop acquired animation")
					}
					if (rewardReachTerrain{World3D: g.world, passage: true}).IsTileBlocking(TileIndex(bag.X, ts), TileIndex(bag.Y, ts)) {
						t.Fatal("bag landed on uncollectible terrain")
					}
					g.frameCount = bag.hop.started + bag.hop.duration
					if restored {
						save := g.buildSave(wm)
						g.restoreSavedContainers(wm, &save)
						if g.groundContainers[0].X != bag.X || g.groundContainers[0].Y != bag.Y {
							t.Fatal("save changed landing")
						}
					}
					g.camera.X, g.camera.Y = bag.X, bag.Y
					g.flyActive = true
					gold := g.party.Gold
					g.pickupGroundContainerAt(0)
					if len(g.groundContainers) != 0 || g.party.Gold != gold+17 {
						t.Fatal("landed loot could not be picked up")
					}
				})
			}
		}
	}
}

package game

import (
	"fmt"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

func BenchmarkMonsterScheduler(b *testing.B) {
	for _, count := range []int{32, 128} {
		for _, scenario := range []string{"calm", "pursuit", "crossfire"} {
			b.Run(fmt.Sprintf("%s/%d", scenario, count), func(b *testing.B) {
				cfg := loadTestConfig(b)
				g := newTestGame(cfg, newTestWorldSized(cfg, 48, 48))
				g.combat = NewCombatSystem(g)
				tile := float64(cfg.GetTileSize())
				g.camera.X, g.camera.Y = 24.5*tile, 24.5*tile
				g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
				for i := 0; i < count; i++ {
					m := monster.NewMonster3DFromConfig(float64(12+i%16)*tile+tile/2, float64(12+i/16)*tile+tile/2, "goblin", cfg)
					m.ID = fmt.Sprintf("benchmark-%03d", i)
					m.WasAttacked = scenario != "calm"
					m.IsEngagingPlayer = scenario != "calm"
					m.Bound = scenario == "crossfire" && i%8 == 0
					g.world.Monsters = append(g.world.Monsters, m)
				}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				updater := entities.NewEntityUpdater()
				b.Cleanup(updater.Stop)
				gl := &GameLoop{game: g}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					g.frameCount++
					g.refreshMonsterAIState()
					gl.reconcileMonsterAttackPosts()
					updater.UpdateMonstersParallel(g.ConvertMonstersToWrappers())
					gl.reconcileMonsterAttackPosts()
				}
				b.StopTimer()
				var queries uint64
				for _, m := range g.world.Monsters {
					queries += m.PathSearchCount
				}
				b.ReportMetric(float64(count), "actors")
				b.ReportMetric(float64(queries)/float64(b.N), "paths/op")
			})
		}
	}
}

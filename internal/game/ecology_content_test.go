package game

import (
	"fmt"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func TestEcologyAuthoredRoutes(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	oldTiles, oldNPC := world.GlobalTileManager, character.NPCConfigInstance
	t.Cleanup(func() { world.GlobalTileManager, character.NPCConfigInstance = oldTiles, oldNPC })
	old := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = old })
	if err := config.LoadEcology("assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, merged := range []bool{false, true} {
		cfg.World.OpenWorld = &merged
		world.GlobalTileManager = world.NewTileManager(cfg.Graphics.SizeClasses)
		if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
			t.Fatal(err)
		}
		if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
			t.Fatal(err)
		}
		if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
			t.Fatal(err)
		}
		wm := world.NewWorldManager(cfg)
		if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
			t.Fatal(err)
		}
		if merged {
			ow, err := config.LoadOpenWorldConfig("assets/open_world.yaml")
			if err != nil {
				t.Fatal(err)
			}
			wm.SetOpenWorldConfig(ow)
		}
		if err := wm.LoadAllMaps(); err != nil {
			t.Fatal(err)
		}
		setTestWorldManager(t, wm)
		qc, err := quests.LoadQuestConfig("assets/quests.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateEcologyContent(cfg, quests.NewQuestManager(qc)); err != nil {
			t.Fatal(err)
		}
		for _, r := range config.GlobalEcology.Caravan.Routes {
			for i, p := range r.Points {
				t.Run(fmt.Sprintf("merged%v/%s/blocked%d", merged, r.ID, i), func(t *testing.T) {
					ts := float64(cfg.GetTileSize())
					w, x, y := ecologyPoint(p, ts)
					tx, ty := int(x/ts), int(y/ts)
					original := w.Tiles[ty][tx]
					w.Tiles[ty][tx] = world.TileWall
					defer func() { w.Tiles[ty][tx] = original }()
					if err := ValidateEcologyContent(cfg, quests.NewQuestManager(qc)); (err == nil) != p.Skip {
						t.Fatalf("blocked skipped=%v anchor: %v", p.Skip, err)
					}
				})
			}

			// Legacy saves may be at any retired anchor, traveling either way.
			// Exercise the normal target policy, then the bounded runtime A*.
			for i, p := range r.Points {
				if !p.Skip {
					continue
				}
				for _, returning := range []bool{false, true} {
					t.Run(fmt.Sprintf("merged%v/%s/legacy%d/return%v", merged, r.ID, i, returning), func(t *testing.T) {
						ts := float64(cfg.GetTileSize())
						w, x, y := ecologyPoint(p, ts)
						g := newTestGame(cfg, w)
						m := monster.NewMonster3DFromConfig(x, y, config.GlobalEcology.Caravan.Monster, cfg)
						g.ecology = EcologyState{Unlocked: true, ActorID: m.ID, Route: r.ID, Checkpoint: i, Returning: returning}
						g.setCaravanTarget(m)
						checker := collision.NewCollisionSystem(w, ts)
						bw, bh := m.GetSize()
						checker.RegisterEntity(collision.NewEntity(m.ID, x, y, bw, bh, collision.CollisionTypeMonster, false))
						if r.Points[g.ecology.Checkpoint].Skip || !m.HasPathToTile(checker, int(m.AITargetX/ts), int(m.AITargetY/ts)) {
							t.Fatalf("legacy checkpoint cannot reach active target %d at %.1f,%.1f", g.ecology.Checkpoint, m.AITargetX/ts, m.AITargetY/ts)
						}
					})
				}
			}

			var previous *world.World3D
			var px, py float64
			for i, p := range r.Points {
				if p.Skip {
					continue
				}
				w, x, y := ecologyPoint(p, float64(cfg.GetTileSize()))
				if w == nil {
					t.Fatalf("%s: missing map %s", r.ID, p.Map)
				}
				checker := collision.NewCollisionSystem(w, float64(cfg.GetTileSize()))
				m := monster.NewMonster3DFromConfig(x, y, config.GlobalEcology.Caravan.Monster, cfg)
				if w.IsTileBlockingForMonster(int(x/float64(cfg.GetTileSize())), int(y/float64(cfg.GetTileSize())), nil, false) {
					t.Errorf("merged=%v route=%s point=%d blocked %+v", merged, r.ID, i, p)
				}
				if previous == w {
					m.X, m.Y = px, py
					bw, bh := m.GetSize()
					checker.RegisterEntity(collision.NewEntity(m.ID, px, py, bw, bh, collision.CollisionTypeMonster, false))
					if !m.HasPathToTile(checker, int(x/float64(cfg.GetTileSize())), int(y/float64(cfg.GetTileSize()))) {
						t.Errorf("merged=%v route=%s segment ending %d unreachable %+v", merged, r.ID, i, p)
					}
				}
				previous, px, py = w, x, y
			}
		}
	}
}

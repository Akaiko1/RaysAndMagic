package game

import (
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
			var previous *world.World3D
			var px, py float64
			for i, p := range r.Points {
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

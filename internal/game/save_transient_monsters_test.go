package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// Persistent actors retain their own identity and coordinates when transient
// fish are omitted. Case table: split/unified worlds x manual save/dive autosave
// x no fish/leading/interleaved/trailing/all fish. Every cell reloads the file.
func TestSaveWithTransientFish(t *testing.T) {
	t.Chdir("../..")
	for _, unified := range []bool{true, false} {
		t.Run(fmt.Sprintf("unified=%v", unified), func(t *testing.T) {
			root := t.TempDir()
			storage.SetDataRootForTesting(root)
			t.Cleanup(func() { storage.SetDataRootForTesting("") })
			g, wm, cfg := bootOpenWorldGame(t, unified)
			// These cells exercise persistence and travel, without GPU preparation.
			g.gameLoop.renderer = nil
			g.questManager = nil // Empty test rosters must not complete campaign quests.
			ts := cfg.GetTileSize()
			for _, entry := range []string{"manual", "dive"} {
				for _, layout := range []string{"none", "leading", "interleaved", "trailing", "all"} {
					t.Run(entry+"/"+layout, func(t *testing.T) {
						wm.EachWorld(func(_ string, w *world.World3D) {
							w.Monsters = nil
							// An explicitly cleared farming roster is not a legacy,
							// unstamped map that the loader should repopulate.
							w.LastRespawnDay = 1
						})
						if err := g.switchToMap("forest"); err != nil {
							t.Fatal(err)
						}
						forest := wm.WorldByKey("forest")
						desert := wm.WorldByKey("desert")
						type actorState struct {
							key          string
							x, y, sx, sy float64
							hp           int
						}
						expected := map[string]actorState{}
						var actors []*monster.Monster3D
						for i, key := range []string{"forest", "desert"} {
							x, y := 8.5*ts, 10.5*ts
							sx, sy := 9.5*ts, 11.5*ts
							px, py := wm.ProjectWorldPos(key, x, y)
							psx, psy := wm.ProjectWorldPos(key, sx, sy)
							m := monster.NewMonster3DFromConfig(px, py, "wolf", cfg)
							m.ID = fmt.Sprintf("persistent_%d", i)
							m.SpawnX, m.SpawnY = psx, psy
							m.HitPoints = 3 + i
							actors = append(actors, m)
							if layout != "all" {
								expected[m.ID] = actorState{key, x, y, sx, sy, m.HitPoints}
							}
						}
						fish := func() *monster.Monster3D {
							x, y := wm.ProjectWorldPos("forest", 12.5*ts, 13.5*ts)
							return monster.NewMonster3DFromConfig(x, y, "common_carp", cfg)
						}
						switch layout {
						case "leading":
							actors = append([]*monster.Monster3D{fish()}, actors...)
						case "interleaved":
							actors = []*monster.Monster3D{actors[0], fish(), actors[1]}
						case "trailing":
							actors = append(actors, fish())
						case "all":
							actors = []*monster.Monster3D{fish(), fish()}
						}
						for _, m := range actors {
							w := forest
							if !unified && m.ID == "persistent_1" {
								w = desert
							}
							w.Monsters = append(w.Monsters, m)
						}
						path := filepath.Join(root, "manual.json")
						if entry == "manual" {
							if err := g.SaveGameToFile(path); err != nil {
								t.Fatal(err)
							}
						} else {
							x, y := wm.ProjectWorldPos("forest", 8.5*ts, 10.5*ts)
							g.setPartyPosition(x, y)
							tx, ty := TileIndex(x, ts), TileIndex(y, ts)
							previous := forest.Tiles[ty][tx]
							forest.Tiles[ty][tx] = world.TileDeepWater
							defer func() { forest.Tiles[ty][tx] = previous }()
							g.waterBreathingActive = true
							g.waterBreathingDuration = 600
							NewInputHandler(g).checkDeepWater()
							if wm.CurrentMapKey != "water" {
								t.Fatal("dive did not commit")
							}
							path = saveRowPath(0)
						}
						data, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						var save GameSave
						if err := json.Unmarshal(data, &save); err != nil {
							t.Fatal(err)
						}
						count := 0
						for _, key := range []string{"forest", "desert"} {
							bucket, ok := save.MapMonsters[key]
							if !ok {
								t.Fatalf("missing %s bucket", key)
							}
							for _, s := range bucket {
								want, ok := expected[s.ID]
								if !ok {
									t.Fatalf("transient or unknown actor saved: %s", s.ID)
								}
								if key != want.key || s.X != want.x || s.Y != want.y || s.HitPoints != want.hp || s.SpawnPosition == nil || *s.SpawnPosition != [2]float64{want.sx, want.sy} {
									t.Fatalf("actor %s saved with another actor's region/state: %+v", s.ID, s)
								}
								count++
							}
						}
						if count != len(expected) {
							t.Fatalf("saved %d persistent actors, want %d", count, len(expected))
						}
						if err := g.LoadGameFromFile(path); err != nil {
							t.Fatal(err)
						}
						restored := 0
						wm.EachWorld(func(_ string, w *world.World3D) {
							for _, m := range w.Monsters {
								want, ok := expected[m.ID]
								if !ok {
									t.Errorf("unexpected restored actor %s", m.ID)
									continue
								}
								x, y := wm.ProjectWorldPos(want.key, want.x, want.y)
								sx, sy := wm.ProjectWorldPos(want.key, want.sx, want.sy)
								if m.X != x || m.Y != y || m.SpawnX != sx || m.SpawnY != sy || m.HitPoints != want.hp {
									t.Errorf("restored actor %s lost its own state", m.ID)
								}
								restored++
							}
						})
						if restored != len(expected) {
							t.Fatalf("restored %d actors, want %d", restored, len(expected))
						}
					})
				}
			}
		})
	}
}

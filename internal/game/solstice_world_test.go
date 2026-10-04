package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/storage"
)

// Both world modes keep dungeon travel; only the stitched mode replaces the
// outer gates with terrain. Saves must also survive switching world modes.
func TestSolsticeWorldConnections(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, unified := range []bool{false, true} {
		t.Run(fmt.Sprintf("open=%v", unified), func(t *testing.T) {
			g, wm, cfg := bootOpenWorldGame(t, unified)
			ts := cfg.GetTileSize()
			find := func(mapKey, key string) *character.NPC {
				for _, npc := range wm.WorldByKey(mapKey).NPCs {
					if npc.Key == key {
						return npc
					}
				}
				return nil
			}
			travel := func(key, destination string) {
				t.Helper()
				npc := find(wm.CurrentMapKey, key)
				if npc == nil {
					t.Fatalf("missing travel NPC %s", key)
				}
				for _, choice := range npc.DialogueData.Choices {
					if choice.Action == "enter_map" {
						dialogActions[choice.Action](&InputHandler{game: g}, npc, choice)
						if wm.CurrentMapKey != destination {
							t.Fatalf("%s arrived in %s, want %s", key, wm.CurrentMapKey, destination)
						}
						return
					}
				}
				t.Fatalf("%s has no travel action", key)
			}
			for _, gate := range [][2]string{{"deep_jungle", "solstice_jungle_entrance"}, {"solstice_approach", "solstice_approach_exit"}} {
				if present := find(gate[0], gate[1]) != nil; present == unified {
					t.Fatalf("outer gate %s present=%v", gate[1], present)
				}
			}
			if err := g.transitionToMap(mapTransition{mapKey: "deep_jungle", arrival: mapArrivalEntrance, arrivalTile: &[2]int{47, 28}}); err != nil {
				t.Fatal(err)
			}
			if unified {
				if wm.WorldByKey("deep_jungle") != wm.WorldByKey("solstice_approach") {
					t.Fatal("approach is not stitched into the jungle world")
				}
				startX, y := wm.ProjectTile("deep_jungle", 47, 28)
				endX, endY := wm.ProjectTile("solstice_approach", 4, 29)
				if y != endY {
					t.Fatal("camp passage is not aligned")
				}
				for x := startX; x <= endX; x++ {
					if g.world.IsTileBlockingTerrainAt(x, y) {
						t.Fatalf("blocked passage at %d,%d", x, y)
					}
					placePlayerAtTile(g, x, y, ts)
					g.syncOpenWorldRegion()
				}
				if wm.CurrentMapKey != "solstice_approach" {
					t.Fatal("crossing did not select the approach region")
				}
			} else {
				travel("solstice_jungle_entrance", "solstice_approach")
				travel("solstice_approach_exit", "deep_jungle")
				if math.Abs(g.camera.X-47.5*ts) > .01 || math.Abs(g.camera.Y-28.5*ts) > .01 {
					t.Fatal("jungle return position lost")
				}
				travel("solstice_jungle_entrance", "solstice_approach")
			}
			x, y := wm.ProjectTile("solstice_approach", 16, 3)
			placePlayerAtTile(g, x, y, ts)
			originX, originY := g.camera.X, g.camera.Y
			travel("solstice_vestibule_entrance", "solstice_vestibule")
			if g.world == wm.OpenWorld {
				t.Fatal("dungeon must remain a separate world")
			}
			travel("solstice_vestibule_exit", "solstice_approach")
			if math.Abs(g.camera.X-originX) > .01 || math.Abs(g.camera.Y-originY) > .01 {
				t.Fatal("dungeon return position lost")
			}
			save := g.buildSave(wm)
			if save.MapKey != "solstice_approach" || math.Abs(save.PlayerX-16.5*ts) > .01 || math.Abs(save.PlayerY-3.5*ts) > .01 {
				t.Fatal("approach save must use local coordinates")
			}
			for _, restoreUnified := range []bool{false, true} {
				restored, restoreWM, _ := bootOpenWorldGame(t, restoreUnified)
				if err := restored.applySave(restoreWM, &save); err != nil {
					t.Fatal(err)
				}
				px, py := restoreWM.ProjectWorldPos("solstice_approach", save.PlayerX, save.PlayerY)
				if restoreWM.CurrentMapKey != save.MapKey || math.Abs(restored.camera.X-px) > .01 || math.Abs(restored.camera.Y-py) > .01 {
					t.Fatalf("incorrect restored position in open=%v", restoreUnified)
				}
			}
		})
	}
}

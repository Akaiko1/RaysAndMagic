package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestFileRestoreReplacesTimelineInDependencyOrder(t *testing.T) {
	for _, format := range []string{"current", "legacy_map_key"} {
		t.Run(format, func(t *testing.T) {
			g, wm, ts := travelFixture(t)
			mob := monster.NewMonster3DFromConfig(20.5*ts, 20.5*ts, "goblin", g.config)
			mob.ID = "restored_actor"
			g.world.Monsters = []*monster.Monster3D{mob}
			g.turnBasedMonsterStatusTick = true
			g.turnBasedMonsterStunned = map[*monster.Monster3D]bool{mob: true}
			g.waterBreathingActive, g.waterBreathingDuration = true, 123
			g.questSpawnsDone = map[string]bool{"saved_spawn": true}
			g.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 91, Bonuses: character.UniformStatBonuses(2)})
			g.mapReturnPoses = map[string]MapPose{"other": {X: 5.5 * ts, Y: 6.5 * ts, Angle: 0}}
			save := g.buildSave(wm)
			if format == "legacy_map_key" {
				save.MapKey = ""
			}
			data, err := json.Marshal(save)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "timeline.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			for load := 0; load < 2; load++ {
				g.dialogState = staleConversation(&character.NPC{Name: "Previous conversation"})
				g.menuState = menuState{mainMenuOpen: true, mainMenuMode: MenuLoadSelect, saveRenameOpen: true, saveRenameInput: "Previous slot"}
				g.questSpawnsDone = map[string]bool{"previous_spawn": true}
				g.waterBreathingActive, g.waterBreathingDuration = false, 0
				g.statBuffs = nil
				g.utilitySpellStatuses = nil
				g.turnBasedMonsterStunned = nil
				g.camera.X = 2.5 * ts
				g.arrows = []Arrow{{ID: "previous_projectile", Active: true}}
				if err := g.LoadGameFromFile(path); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(g.dialogState, dialogState{dialogLastClickedIdx: -1}) || !reflect.DeepEqual(g.menuState, menuState{saveRenameSlot: -1, audioSliderDrag: -1}) {
					t.Fatal("file load retained the previous UI owners")
				}
				if !g.waterBreathingActive || g.waterBreathingDuration != 123 || len(g.statBuffs) != 1 || g.statBuffs[0].Frames != 91 || g.utilitySpellStatuses[spells.SpellID("bless")] == nil {
					t.Fatal("restored effects and their presentation disagree before the first tick")
				}
				if len(g.world.Monsters) != 1 || len(g.turnBasedMonsterStunned) != 1 || !g.turnBasedMonsterStunned[g.world.Monsters[0]] || g.world.Monsters[0].ID != mob.ID {
					t.Fatal("scheduler did not bind to the restored actor roster")
				}
				if !reflect.DeepEqual(g.questSpawnsDone, map[string]bool{"saved_spawn": true}) || len(g.arrows) != 0 || g.camera.X != save.PlayerX {
					t.Fatal("file load leaked the previous timeline or lost saved state")
				}
				if got := g.mapReturnPoses["other"]; got != (MapPose{X: 5.5 * ts, Y: 6.5 * ts, Angle: 0}) {
					t.Fatalf("load %d changed the saved return pose: %+v", load, got)
				}
			}
		})
	}
}

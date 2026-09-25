package quests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func activityFixture(t *testing.T, id string) *QuestManager {
	t.Helper()
	cfg, err := LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	qm := NewQuestManager(cfg)
	if err := qm.ActivateQuest(id); err != nil {
		t.Fatal(err)
	}
	return qm
}
func TestActivitySequenceTransitions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		tokens       []string
		index, count int
		done         bool
	}{
		{"wrong first", []string{"monastery"}, 0, 0, false},
		{"partial", []string{"spring", "travelers"}, 2, 0, false},
		{"repeat resets", []string{"spring", "spring"}, 0, 0, false},
		{"wrong final resets", []string{"spring", "travelers", "spring"}, 0, 0, false},
		{"complete once", []string{"spring", "travelers", "monastery", "monastery"}, 3, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qm := activityFixture(t, "unbroken_desert")
			qm.OnInteract("pilgrim_sluice")
			qm.AdvanceInteractQuest("unbroken_desert", "pilgrim_sluice")
			for _, token := range tc.tokens {
				qm.InteractActivity("unbroken_desert", "pilgrim_sluice", token, false)
			}
			q := qm.GetQuest("unbroken_desert")
			if q.Activity.SequenceIndex != tc.index || q.CurrentCount != tc.count || q.Completed != tc.done {
				t.Fatalf("state %+v", q)
			}
		})
	}
}
func TestActivityForagePersistenceAndPhase(t *testing.T) {
	qm := activityFixture(t, "unbroken_jungle")
	q := qm.GetQuest("unbroken_jungle")
	selected := append([]string(nil), q.Activity.Selected...)
	for _, token := range selected {
		night := q.Definition.Activity.TokenPhase(token) == "night"
		for _, tc := range []struct {
			tag, token    string
			night, credit bool
		}{
			{"wrong", token, night, false}, {"pilgrim_flower", "missing", night, false}, {"pilgrim_flower", token, !night, false}, {"pilgrim_flower", token, night, true}, {"pilgrim_flower", token, night, false},
		} {
			got, _, _ := qm.InteractActivity(q.ID, tc.tag, tc.token, tc.night)
			if got != tc.credit {
				t.Fatalf("%+v credited%v", tc, got)
			}
		}
		b, _ := json.Marshal(q.Activity)
		var restored ActivityState
		json.Unmarshal(b, &restored)
		if !reflect.DeepEqual(restored, q.Activity) {
			t.Fatal("activity JSON lost state")
		}
	}
	if !q.Completed || q.CurrentCount != 5 {
		t.Fatal("quota not completed")
	}
	if !reflect.DeepEqual(q.Activity.Selected, selected) {
		t.Fatal("selection changed")
	}
}
func TestActivityConfigRejectsInvalidRules(t *testing.T) {
	for _, extra := range []string{
		"activity: {sequence: [a], forage: [{phase: day, count: 1, tokens: [x]}]}",
		"activity: {sequence: [a]}",
		"activity: {forage: [{phase: twilight, count: 1, tokens: [x]}], dormant_message: wait}",
		"activity: {forage: [{phase: day, count: 2, tokens: [x]}], dormant_message: wait}",
		"activity: {forage: [{phase: day, count: 1, tokens: [x,x]}], dormant_message: wait}",
		"next_quest: missing",
		"next_quest: test",
		"on_accept_spawns: [{id: one, map: highlands}]",
	} {
		t.Run(extra, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "quests.yaml")
			os.WriteFile(path, []byte("quests:\n  test:\n    name: Test\n    type: interact\n    target_monster: thing\n    target_count: 1\n    progress_text: done\n    "+extra+"\n"), 0600)
			if _, err := LoadQuestConfig(path); err == nil {
				t.Fatal("invalid activity accepted")
			}
		})
	}
}

func TestActivePropsConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name, props     string
		activity, valid bool
	}{
		{"valid", "[{npc: spring, map: desert, x: 1, y: 2}]", true, true},
		{"missing npc", "[{map: desert, x: 1, y: 2}]", true, false},
		{"missing map", "[{npc: spring, x: 1, y: 2}]", true, false},
		{"negative coordinate", "[{npc: spring, map: desert, x: -1, y: 2}]", true, false},
		{"duplicate identity", "[{npc: spring, map: desert, x: 1, y: 2}, {npc: spring, map: desert, x: 2, y: 2}]", true, false},
		{"overlap", "[{npc: spring, map: desert, x: 1, y: 2}, {npc: other, map: desert, x: 1, y: 2}]", true, false},
		{"no activity state", "[{npc: spring, map: desert, x: 1, y: 2}]", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := "quests:\n  test:\n    name: Test\n    type: interact\n    target_monster: thing\n    target_count: 1\n    progress_text: done\n    active_props: " + tc.props + "\n"
			if tc.activity {
				data += "    activity: {sequence: [spring], wrong_message: Try again}\n"
			}
			path := filepath.Join(t.TempDir(), "quests.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadQuestConfig(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestActivePropLayoutsValidation(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"three layouts", "", true},
		{"flat and layouts", "    active_props: [{npc: spring, map: desert, x: 1, y: 2}]\n", false},
		{"spacing", "    min_prop_spacing_tiles: 20\n", false},
		{"negative spacing", "    min_prop_spacing_tiles: -1\n", false},
		{"nonfinite spacing", "    min_prop_spacing_tiles: .inf\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := "quests:\n  test:\n    name: Test\n    type: interact\n    target_monster: thing\n    target_count: 1\n    progress_text: done\n    activity: {sequence: [spring, travelers], wrong_message: Try again}\n" + tc.extra + "    active_prop_layouts:\n"
			for _, id := range []string{"a", "b", "c"} {
				data += "      - id: " + id + "\n        props: [{npc: spring, map: desert, x: 1, y: 2}, {npc: travelers, map: desert, x: 8, y: 8}]\n"
			}
			path := filepath.Join(t.TempDir(), "quests.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadQuestConfig(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v got %v", tc.valid, err)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*QuestDefinition)
	}{
		{"duplicate id", func(d *QuestDefinition) { d.ActivePropLayouts[1].ID = "a" }},
		{"empty id", func(d *QuestDefinition) { d.ActivePropLayouts[1].ID = "" }},
		{"empty props", func(d *QuestDefinition) { d.ActivePropLayouts[1].Props = nil }},
		{"different identities", func(d *QuestDefinition) { d.ActivePropLayouts[1].Props[0].NPC = "other" }},
		{"missing identity", func(d *QuestDefinition) {
			d.ActivePropLayouts[1].Props = append(d.ActivePropLayouts[1].Props, QuestProp{NPC: "other", Map: "desert", X: 5, Y: 5})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &QuestDefinition{Type: QuestTypeInteract, TargetCount: 1, Activity: &ActivityDefinition{Sequence: []string{"spring"}, WrongMessage: "Again"}, ActivePropLayouts: []QuestPropLayout{{ID: "a", Props: []QuestProp{{NPC: "spring", Map: "desert", X: 1, Y: 2}}}, {ID: "b", Props: []QuestProp{{NPC: "spring", Map: "desert", X: 3, Y: 4}}}}}
			tc.change(d)
			if err := validateActivity("test", d, &QuestConfig{Quests: map[string]*QuestDefinition{"test": d}}); err == nil {
				t.Fatal("invalid layout accepted")
			}
		})
	}
}

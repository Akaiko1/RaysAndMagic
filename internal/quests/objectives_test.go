package quests

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSolsticeObjectiveGraph(t *testing.T) {
	cfg, err := LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for id, d := range cfg.Quests {
		if d.Activity == nil || len(d.Activity.Objectives) == 0 {
			continue
		}
		t.Run(id, func(t *testing.T) {
			qm := activityFixture(t, id)
			count := 0
			for _, o := range d.Activity.Objectives {
				ev := ObjectiveEvent{Token: o.Token, Map: o.Map, Event: o.Event, Night: o.Phase == "night", From: o.From, To: o.To}
				wrong := ev
				wrong.Map = "wrong"
				if yes, _, _ := qm.CreditObjective(id, wrong); yes {
					t.Fatal("foreign map credited")
				}
				if o.Phase != "" {
					wrong = ev
					wrong.Night = !ev.Night
					if yes, _, _ := qm.CreditObjective(id, wrong); yes {
						t.Fatal("wrong phase credited")
					}
				}
				if len(o.Sequence) > 0 {
					ev.Step = o.Sequence[0]
					qm.CreditObjective(id, ev)
					ev.Step = "wrong"
					qm.CreditObjective(id, ev)
					if qm.GetQuest(id).CurrentCount != count {
						t.Fatal("mistake erased prior readings")
					}
					for i, step := range o.Sequence {
						ev.Step = step
						yes, _, _ := qm.CreditObjective(id, ev)
						if yes != (i == len(o.Sequence)-1) {
							t.Fatalf("sequence step %d credit=%v", i, yes)
						}
					}
				} else if yes, _, msg := qm.CreditObjective(id, ev); !yes {
					t.Fatalf("objective %s rejected: %s", o.Token, msg)
				}
				count++
				if yes, _, _ := qm.CreditObjective(id, ev); yes {
					t.Fatal("duplicate credited")
				}
				q := qm.GetQuest(id)
				if q.CurrentCount != count {
					t.Fatalf("count=%d want=%d", q.CurrentCount, count)
				}
				// Each checkpoint must survive JSON without sharing mutable maps.
				raw, _ := json.Marshal(q.Activity)
				var restored ActivityState
				json.Unmarshal(raw, &restored)
				q.Activity = RestoreActivityState(d.Activity, restored)
			}
			if !qm.GetQuest(id).Completed {
				t.Fatal("full graph did not finish")
			}
		})
	}
}

func TestObjectiveDependenciesAndCrossing(t *testing.T) {
	for _, kind := range []string{"dependency", "walk", "wrong start", "wrong landing", "valid"} {
		t.Run(kind, func(t *testing.T) {
			qm := activityFixture(t, "solstice_connection")
			d := qm.GetQuest("solstice_connection").Definition
			var jump ActivityObjective
			for _, o := range d.Activity.Objectives {
				if o.Event == "crossing" {
					jump = o
				}
			}
			ev := ObjectiveEvent{Token: jump.Token, Map: jump.Map, Event: "crossing", From: jump.From, To: jump.To}
			switch kind {
			case "dependency":
				for _, o := range d.Activity.Objectives {
					if len(o.Requires) > 0 {
						yes, _, _ := qm.CreditObjective("solstice_connection", ObjectiveEvent{Token: o.Token, Map: o.Map, Event: o.Event})
						if yes {
							t.Fatal("dependency bypass")
						}
					}
				}
				return
			case "walk":
				ev.Event = "walk"
			case "wrong start":
				ev.From[0]++
			case "wrong landing":
				ev.To[0]--
			}
			yes, _, msg := qm.CreditObjective("solstice_connection", ev)
			if yes != (kind == "valid") {
				t.Fatal(fmt.Sprintf("credit=%v: %s", yes, msg))
			}
		})
	}
}

func TestCrossingBanksAndSpellOnlyObjectives(t *testing.T) {
	for _, kind := range []string{"jump", "crossing"} {
		for _, tc := range []struct {
			name           string
			from, to       [2]int
			crossing, jump bool
		}{
			{"exact", [2]int{7, 7}, [2]int{9, 7}, true, true},
			{"long fold", [2]int{5, 7}, [2]int{13, 7}, true, false},
			{"far landing", [2]int{7, 7}, [2]int{11, 7}, true, false},
			{"reverse", [2]int{9, 7}, [2]int{7, 7}, false, false},
			{"wrong corridor", [2]int{7, 6}, [2]int{9, 6}, false, false},
			{"stops in gap", [2]int{7, 7}, [2]int{8, 7}, false, false},
			{"already across", [2]int{9, 7}, [2]int{12, 7}, false, false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				qm := activityFixture(t, "solstice_connection")
				o := qm.GetQuest("solstice_connection").Definition.Activity.Objective("crossing")
				o.Event = kind
				yes, _, _ := qm.CreditObjective("solstice_connection", ObjectiveEvent{Token: o.Token, Map: o.Map, Event: kind, From: tc.from, To: tc.to})
				want := tc.crossing
				if kind == "jump" {
					want = tc.jump
				}
				if yes != want {
					t.Fatalf("credit=%v want=%v", yes, want)
				}
			})
		}
	}
}

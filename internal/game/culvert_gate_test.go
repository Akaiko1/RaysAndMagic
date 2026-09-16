package game

import (
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// The new gate inherits every lock rule; it never owns a second unlock path.
func TestCulvertGateUsesReinforcedDoorRules(t *testing.T) {
	for _, tc := range []struct {
		name, key        string
		might, intellect int
		jammed           bool
	}{
		{name: "sealed"}, {name: "wrong key", key: "ordinary_key"},
		{name: "inlaid key", key: "inlaid_key"}, {name: "master key", key: "skeleton_key"},
		{name: "might", might: 90}, {name: "intellect", intellect: 70},
		{name: "jammed force", might: 90, intellect: 70, jammed: true},
		{name: "jammed key", key: "inlaid_key", jammed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := crateTestGame(t)
			gate, err := character.CreateNPCFromConfig("culvert_gate", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			reinforced, err := character.CreateNPCFromConfig("reinforced_door", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if gate.Sprite != "culvert_gate" || gate.Type != reinforced.Type || gate.DoorBehavior != reinforced.DoorBehavior || gate.RenderCategory != reinforced.RenderCategory || !reflect.DeepEqual(gate.DoorKeyItemKeys, reinforced.DoorKeyItemKeys) || !reflect.DeepEqual(gate.DoorStatReqs, reinforced.DoorStatReqs) {
				t.Fatal("gate lock configuration diverged from reinforced door")
			}
			for _, m := range g.party.Members {
				m.Might = 1
				m.Intellect = 1
				m.Skills = map[character.SkillType]*character.Skill{}
				m.Equipment = map[items.EquipSlot]items.Item{}
			}
			g.party.Members[0].Might = max(1, tc.might)
			g.party.Members[0].Intellect = max(1, tc.intellect)
			if tc.key != "" {
				g.party.AddItem(items.CreateItemFromYAML(tc.key))
			}
			gate.DoorLockBroken = tc.jammed
			reinforced.DoorLockBroken = tc.jammed
			got, want := g.availableDoorUnlocks(gate), g.availableDoorUnlocks(reinforced)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("unlock options differ: %+v vs %+v", got, want)
			}
			if tc.key == "inlaid_key" || tc.key == "skeleton_key" {
				if len(got) == 0 {
					t.Fatal("matching key does not open gate")
				}
				g.openLockedDoor(gate, 0)
				if !gate.Visited {
					t.Fatal("gate failed to open through shared door handler")
				}
				key := items.CreateItemFromYAML(tc.key)
				expected := 0
				if tc.key == "skeleton_key" {
					expected = 1
				}
				if g.party.CountItemsByName(key.Name) != expected {
					t.Fatal("key consumption differs from standard lock rules")
				}
			}
		})
	}
}

package character

import (
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/items"
)

func TestCarriedPhysicalContainers(t *testing.T) {
	for _, source := range []string{"shared", "personal", "other personal", "quick", "other quick", "reserve", "captive"} {
		for _, virtual := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/virtual=%v", source, virtual), func(t *testing.T) {
				owner, other := &MMCharacter{}, &MMCharacter{}
				p := &Party{Members: []*MMCharacter{owner, nil, other}}
				it := trinket(3)
				it.InstanceID = 42
				if virtual {
					it.Type = items.ItemThrowable
				}
				switch source {
				case "shared":
					p.Inventory = []items.Item{it}
				case "personal":
					owner.Inventory = []items.Item{it}
				case "other personal":
					other.Inventory = []items.Item{it}
				case "quick":
					owner.QuickSlots[4] = &it
				case "other quick":
					other.QuickSlots[2] = &it
				case "reserve", "captive":
					p.Members = []*MMCharacter{other}
					owner.Inventory, owner.QuickSlots[4] = []items.Item{it}, &it
					if source == "reserve" {
						p.Reserve = []*MMCharacter{owner}
					} else {
						p.Captive = []*MMCharacter{owner}
					}
				}
				eligible := !virtual && source != "reserve" && source != "captive"
				want := 0
				if eligible {
					want = 3
				}
				before := p.ContentRevision()
				if got := p.CountItemsByName(it.Name); got != want {
					t.Fatalf("count=%d want=%d", got, want)
				}
				if p.RemoveItemsByName(it.Name, 4) || p.ContentRevision() != before {
					t.Fatal("short payment mutated carried contents")
				}
				if got := p.RemoveItemsByName(it.Name, 2); got != eligible {
					t.Fatalf("payment=%v want=%v", got, eligible)
				}
				if eligible {
					carried := p.CarriedItems()
					if len(carried) != 1 || carried[0].Count() != 1 || carried[0].InstanceID != 42 || p.ContentRevision() <= before {
						t.Fatal("partial payment lost physical identity or revision")
					}
					if !p.ConsumeCarriedUnitsAt(0, 1) || len(p.CarriedItems()) != 0 || p.CountItemsByName(it.Name) != 0 {
						t.Fatal("flattened payment left a spent stack or quick slot behind")
					}
				} else if len(p.CarriedItems()) != 0 {
					t.Fatal("unavailable or virtual stock leaked into snapshot")
				}
			})
		}
	}
}

func TestCarriedPaymentPriorityAndAtomicity(t *testing.T) {
	for _, pay := range []int{-1, 0, 1, 3, 5, 7, 8, 10, 11} {
		t.Run(fmt.Sprint(pay), func(t *testing.T) {
			hero := &MMCharacter{}
			p := &Party{Members: []*MMCharacter{hero}, Inventory: []items.Item{trinket(1), sword(), trinket(2)}}
			hero.Inventory = []items.Item{trinket(2)}
			first, last := trinket(2), trinket(3)
			last.InstanceID, last.Lineages = 22, []items.StackLineage{{ID: 22, Quantity: 1}, {ID: 33, Quantity: 2}}
			hero.QuickSlots[0], hero.QuickSlots[4] = &first, &last
			virtual := items.Item{Name: "Clock Hand", Type: items.ItemTechnique}
			hero.QuickSlots[1] = &virtual
			before := p.CarriedItems()
			valid := pay >= 0 && pay <= 10
			if got := p.RemoveItemsByName("Clock Hand", pay); got != valid {
				t.Fatalf("payment=%v want=%v", got, valid)
			}
			if !valid {
				if !reflect.DeepEqual(before, p.CarriedItems()) {
					t.Fatal("failed payment changed containers")
				}
				return
			}
			want := []int{max(0, 3-pay), max(0, 2-max(0, pay-3)), max(0, 2-max(0, pay-5)), max(0, 3-max(0, pay-7))}
			count := func(inv []items.Item) int {
				n := 0
				for _, it := range inv {
					if it.Name == "Clock Hand" {
						n += it.Count()
					}
				}
				return n
			}
			got := []int{count(p.Inventory), count(hero.Inventory), 0, 0}
			for i, slot := range []int{0, 4} {
				if it := hero.QuickSlots[slot]; it != nil {
					got[i+2] = it.Count()
				}
			}
			if !reflect.DeepEqual(got, want) || hero.QuickSlots[1] != &virtual {
				t.Fatalf("container priority=%v want=%v; shortcut must remain", got, want)
			}
			if pay == 7 && !reflect.DeepEqual(last.StackLineageParts(), []items.StackLineage{{ID: 22, Quantity: 1}, {ID: 33, Quantity: 2}}) {
				t.Fatal("unspent quick stack provenance changed")
			}
			if pay == 8 && !reflect.DeepEqual(last.StackLineageParts(), []items.StackLineage{{ID: 33, Quantity: 2}}) {
				t.Fatal("partial quick payment lost remaining stack provenance")
			}
		})
	}
}

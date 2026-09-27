package game

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"ugataima/internal/config"
)

// Rarity is presentation only: layout must not reorder the pool consumed by
// drag origins and prison/tavern assignment. Party-create state is not saved.
func TestPartyCreateAutomaticRarityOrder(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, scenario := range []string{"initial", "reversed_config", "replace_slot", "empty_slot", "return_to_pool", "swap_slots", "pool_noop"} {
		for _, size := range [][2]int{{800, 600}, {1920, 1080}} {
			t.Run(fmt.Sprintf("%s/%v", scenario, size), func(t *testing.T) {
				copyCfg := *cfg
				if scenario == "reversed_config" {
					copyCfg.Characters.TavernRecruits = append([]config.RosterEntry(nil), cfg.Characters.TavernRecruits...)
					recruits := copyCfg.Characters.TavernRecruits
					for i, j := 0, len(recruits)-1; i < j; i, j = i+1, j-1 {
						recruits[i], recruits[j] = recruits[j], recruits[i]
					}
				}
				pc := newPartyCreateState(&copyCfg)
				switch scenario {
				case "replace_slot", "empty_slot":
					if scenario == "empty_slot" {
						pc.slots[0] = nil
					}
					pc.drag = pc.pool[len(pc.pool)-1]
					pc.dragFromPool = len(pc.pool) - 1
					pc.dragFromSlot = -1
					pc.applyDrop(0)
				case "return_to_pool":
					pc.drag = pc.slots[0]
					pc.dragFromSlot = 0
					pc.applyDrop(-1)
				case "swap_slots":
					pc.drag = pc.slots[0]
					pc.dragFromSlot = 0
					pc.applyDrop(1)
				case "pool_noop":
					pc.drag = pc.pool[0]
					pc.dragFromPool = 0
					pc.dragFromSlot = -1
					pc.applyDrop(-1)
				}
				original := append([]*pcHero(nil), pc.pool...)
				seen := map[int]bool{}
				previousTier, previousIndex := -1, -1
				maximum := partyCreateLayout(pc, size[0], size[1]).poolMaxScroll
				for scroll := 0; scroll <= maximum; scroll++ {
					pc.poolScroll = scroll
					layout := partyCreateLayout(pc, size[0], size[1])
					var indices []int
					for i, r := range layout.pool {
						if r.w > 0 {
							indices = append(indices, i)
						}
					}
					sort.Slice(indices, func(i, j int) bool {
						a, b := layout.pool[indices[i]], layout.pool[indices[j]]
						if a.y != b.y {
							return a.y < b.y
						}
						return a.x < b.x
					})
					for _, i := range indices {
						if seen[i] {
							continue
						}
						seen[i] = true
						tier := config.RarityTier(pc.pool[i].cardRarity(&copyCfg))
						if tier < previousTier {
							t.Fatalf("rarity grouping regressed at %s: tier %d follows %d", pc.pool[i].char.Name, tier, previousTier)
						}
						if tier == previousTier && i < previousIndex {
							t.Fatal("equal-rarity order is unstable")
						}
						previousTier, previousIndex = tier, i
					}
				}
				if len(seen) != len(original) {
					t.Fatal("sorting hid or duplicated a roster card")
				}
				if !reflect.DeepEqual(pc.pool, original) {
					t.Fatal("visual sort changed domain roster order")
				}
			})
		}
	}
}

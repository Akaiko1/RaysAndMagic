package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// authoredPack returns the shipped day/night pack of one map.
func authoredPack(t *testing.T, mapKey string) config.DayNightPackConfig {
	t.Helper()
	for _, pack := range loadTestConfig(t).DayNight.Packs {
		if pack.Map == mapKey {
			return pack
		}
	}
	t.Fatalf("no day/night pack authored for %s", mapKey)
	return config.DayNightPackConfig{}
}

// replacedMember is the first phase member carrying a level-gated replacement.
func replacedMember(t *testing.T, pack config.DayNightPackConfig, night bool) config.PackMemberConfig {
	t.Helper()
	for _, member := range pack.PhaseMembers(night) {
		if member.Replacement != nil && member.Replacement.MinPartyLevel > 0 {
			return member
		}
	}
	t.Fatalf("%s night=%v has no level-gated replacement", pack.Map, night)
	return config.PackMemberConfig{}
}

// The forest's level-gated variant replaces its phase member only from the
// authored min_party_level on, at that level, and survives a save round trip.
func TestForestReplacementLevelPhaseAndPersistence(t *testing.T) {
	forest := authoredPack(t, "forest")
	for _, night := range []bool{false, true} {
		unlock := replacedMember(t, forest, night).Replacement.MinPartyLevel
		for _, active := range []bool{false, true} {
			for _, level := range []int{unlock - 1, unlock, unlock + 1} {
				t.Run(fmt.Sprintf("active=%v/night=%v/level=%d", active, night, level), func(t *testing.T) {
					g, wm, w := packSlotTestGame(t, active)
					member := replacedMember(t, forest, night)
					// Force the roll to hit so the production level gate has a deterministic oracle.
					replacement := *member.Replacement
					replacement.Chance = 1
					member.Replacement = &replacement
					member.Count = 5
					pack := config.DayNightPackConfig{Map: "pack_test", MinPlayerDistTiles: .5}
					if night {
						pack.NightMonsters = []config.PackMemberConfig{member}
					} else {
						pack.DayMonsters = []config.PackMemberConfig{member}
					}
					g.config.DayNight.Packs = []config.DayNightPackConfig{pack}
					g.party.Members[0].Level = level
					for _, c := range g.party.Members {
						c.Level = level
					}
					for _, m := range w.Monsters {
						m.HitPoints = 0
					}
					g.syncDayNightPacks(night)
					spawned := livePhasePack(w, night)
					if len(spawned) != 5 {
						t.Fatalf("spawned %d", len(spawned))
					}
					want := member.Monster
					if level >= unlock {
						want = replacement.Monster
					}
					identities := map[string]string{}
					for _, m := range spawned {
						if m.Key != want || m.QuestProgressIgnored == member.QuestProgress {
							t.Fatalf("wrong member %s / quest flag", m.Key)
						}
						if level >= unlock && m.Level != unlock {
							t.Fatalf("variant is level %d, want its unlock level %d", m.Level, unlock)
						}
						identities[m.ID] = m.Key
						if active && g.collisionSystem.GetEntityByID(m.ID) == nil {
							t.Fatal("missing spawn collision")
						}
					}
					save := g.buildSave(wm)
					raw, err := json.Marshal(save)
					if err != nil {
						t.Fatal(err)
					}
					var decoded GameSave
					if err := json.Unmarshal(raw, &decoded); err != nil {
						t.Fatal(err)
					}
					g.restoreSavedMonsters(wm, &decoded)
					for _, m := range w.Monsters {
						if key, ok := identities[m.ID]; ok {
							if m.Key != key {
								t.Fatal("load rerolled variant")
							}
							delete(identities, m.ID)
						}
					}
					if len(identities) > 0 {
						t.Fatal("load lost spawned identities")
					}
				})
			}
		}
	}
}

func TestHighlandsRespawnAuthoredPhases(t *testing.T) {
	for _, night := range []bool{false, true} {
		t.Run(fmt.Sprint(night), func(t *testing.T) {
			g, _, w := packSlotTestGame(t, false)
			p := authoredPack(t, "highlands")
			allowed := map[string]bool{}
			total := 0
			for _, member := range p.PhaseMembers(night) {
				allowed[member.Monster] = true
				if member.Replacement != nil {
					allowed[member.Replacement.Monster] = true
				}
				total += member.Count
			}
			if total == 0 {
				t.Fatalf("highlands authors no night=%v pack", night)
			}
			want := min(total, len(w.MonsterSpawns)) // a pack refills vacant authored slots
			p.Map = "pack_test"
			p.MinPlayerDistTiles = .5
			g.config.DayNight.Packs = []config.DayNightPackConfig{p}
			for _, m := range w.Monsters {
				m.HitPoints = 0
			}
			g.syncDayNightPacks(night)
			pack := livePhasePack(w, night)
			if len(pack) != want {
				t.Fatalf("highlands count=%d, want %d", len(pack), want)
			}
			for _, m := range pack {
				if !allowed[m.Key] {
					t.Fatalf("highlands spawned %s, want one of the authored %v", m.Key, allowed)
				}
			}
		})
	}
}

func TestForestLootTablesAndDropWiring(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	previousMonsters := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previousMonsters })
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, material string }{
		{"wolf", "wolf_pelt"}, {"spider", "spider_silk"}, {"forest_spider", "spider_silk"}, {"pixie", "fairy_dust"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			entries := config.GetLootTable(tc.key, false)
			found := false
			mooncap := false
			for _, e := range entries {
				if e.Key == tc.material && e.Chance > 0 && e.RollCount() == 1 {
					found = true
				}
				if e.Key == "mooncap" && e.Type == "harvest" && e.Chance > 0 {
					mooncap = true
				}
			}
			if !found {
				t.Fatal("crafting material must be one independent roll")
			}
			if tc.material == "spider_silk" && !mooncap {
				t.Fatal("missing the Mooncap harvest roll")
			}
			// Force only the material chance after checking the shipped probability.
			// Exercise the death dispatcher without a random success requirement.
			prior := config.GlobalLoots.Loots[tc.key]
			t.Cleanup(func() { config.GlobalLoots.Loots[tc.key] = prior })
			forced := append([]config.LootEntry(nil), entries...)
			for i := range forced {
				if forced[i].Key == tc.material {
					forced[i].Chance = 1
				}
			}
			config.GlobalLoots.Loots[tc.key] = forced
			drops := cs.checkMonsterLootDrop(monster.NewMonster3DFromConfig(0, 0, tc.key, cs.game.config))
			def, _ := config.GetItemDefinition(tc.material)
			n := 0
			for _, it := range drops {
				if it.Name == def.Name {
					n += it.Count()
				}
			}
			if n != 1 {
				t.Fatal("death path did not award material")
			}
		})
	}
	// Forest elites draw from the local harvest population, excluding resources
	// owned by closed dungeons and the Solstice approach.
	growing := map[string]bool{}
	spawnConfig, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, pop := range spawnConfig.Populations {
		if pop.Map == "forest" {
			for key := range pop.Weights {
				growing[key] = true
			}
		}
	}
	if len(growing) == 0 {
		t.Fatal("no growing reagent is authored (positive control)")
	}
	for _, tc := range []struct{ key, material, card string }{{"dire_wolf", "wolf_pelt", "dire_wolf_card"}, {"giant_spider", "spider_silk", ""}} {
		t.Run(tc.key, func(t *testing.T) {
			entries := config.GetLootTable(tc.key, false)
			herbs := map[string]bool{}
			materials, cards := 0, 0
			for _, e := range entries {
				switch {
				case e.Key == tc.material:
					if e.Type != "item" || e.Chance <= 0 || e.RollCount() <= 1 {
						t.Fatal("elite material must have several independent rolls")
					}
					materials++
				case tc.card != "" && e.Key == tc.card:
					if e.Type != "item" || e.Chance <= 0 || e.RollCount() != 1 {
						t.Fatal("own card must retain one roll")
					}
					cards++
				case e.Type == "harvest":
					def, _ := config.GetItemDefinition(e.Key)
					if def == nil || !growing[e.Key] || e.Chance <= 0 || e.RollCount() != 1 || herbs[e.Key] {
						t.Fatal("bonus roll is not one unique growing reagent")
					}
					herbs[e.Key] = true
				default:
					t.Fatalf("elite inherited nonmaterial base drop %s", e.Key)
				}
			}
			if len(herbs) != len(growing) || materials != 1 || (tc.card == "" && cards != 0) || (tc.card != "" && cards != 1) {
				t.Fatalf("elite pool: herbs=%d of %d growing, materials=%d cards=%d", len(herbs), len(growing), materials, cards)
			}
			forced := append([]config.LootEntry(nil), entries...)
			want := map[string]int{}
			for i := range forced {
				forced[i].Chance = 1
				it, err := createLootItem(forced[i].Type, forced[i].Key)
				if err != nil {
					t.Fatal(err)
				}
				want[it.Name] += forced[i].RollCount()
			}
			got := map[string]int{}
			for _, it := range rollLootEntries(forced) {
				got[it.Name] += it.Count()
			}
			for key, n := range want {
				if got[key] != n {
					t.Fatalf("drop %s=%d, want %d", key, got[key], n)
				}
			}
		})
	}
	for _, key := range []string{"mooncap", "grave_orchid", "brewed_revival_potion"} {
		if _, err := createLootItem("item", key); err == nil {
			t.Fatalf("ordinary source leaked %s", key)
		}
	}
}

func TestForestMaterialRollsAreIndependent(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	previousMonsters := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previousMonsters })
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, material string }{{"dire_wolf", "Wolf Pelt"}, {"giant_spider", "Spider Silk"}} {
		t.Run(tc.key, func(t *testing.T) {
			m := monster.NewMonster3DFromConfig(0, 0, tc.key, cs.game.config)
			sawZero, sawPartial := false, false
			for range 128 {
				count := 0
				for _, it := range cs.checkMonsterLootDrop(m) {
					if it.Name == tc.material {
						count += it.Count()
					}
				}
				if count < 0 || count > 5 {
					t.Fatalf("five rolls awarded %d material units", count)
				}
				sawZero = sawZero || count == 0
				sawPartial = sawPartial || (count > 0 && count < 5)
			}
			// Independent 17% rolls produce both outcomes with overwhelming probability;
			// one success awarding a fixed stack of five can never produce a partial.
			if !sawZero || !sawPartial {
				t.Fatalf("rolls act as a guaranteed/bulk stack: zero=%v partial=%v", sawZero, sawPartial)
			}
		})
	}
}

func TestRevivalBatchAllMasteriesWithFairyDust(t *testing.T) {
	for tier := 0; tier < 4; tier++ {
		t.Run(fmt.Sprint(tier), func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassAlchemist, false)
			c.Skills[character.SkillAlchemy].Mastery = character.SkillMastery(tier)
			r := config.AlchemyRecipeByKey("revival_potion")
			choices := []int{0, len(r.Ingredients[1].Alternatives) - 1}
			g.party.Inventory = nil
			for i, group := range r.Ingredients {
				a := group.Alternatives[choices[i]]
				it := items.CreateItemFromYAML(a.Items[0])
				it.Quantity = a.Count
				g.party.AddItem(it)
			}
			result, _, err := g.party.BrewSelected(c, r, character.AlchemySourceSelection(r, choices), 1)
			if err != nil {
				t.Fatal(err)
			}
			if result != tier+1 {
				t.Fatalf("batch=%d, want %d", result, tier+1)
			}
		})
	}
}

func TestHarvestLootIsMonsterOnlyAndSurvivesSave(t *testing.T) {
	for _, entry := range []string{"death", "theft", "crate"} {
		t.Run(entry, func(t *testing.T) {
			g := crateTestGame(t)
			setTestWorldManager(t, nil)
			prior := config.GlobalLoots.Loots["giant_spider"]
			t.Cleanup(func() { config.GlobalLoots.Loots["giant_spider"] = prior })
			config.GlobalLoots.Loots["giant_spider"] = []config.LootEntry{{Type: "harvest", Key: "grave_orchid", Chance: 1}}
			m := monster.NewMonster3DFromConfig(0, 0, "giant_spider", g.config)
			g.world.Monsters = []*monster.Monster3D{m}
			g.world.InitialMonsterKeys = map[string]struct{}{"giant_spider": {}}
			g.party.Inventory = nil
			switch entry {
			case "death":
				for _, it := range g.combat.checkMonsterLootDrop(m) {
					g.party.AddItem(it)
				}
			case "theft":
				c := g.party.Members[0]
				c.Skills[character.SkillSleightOfHand] = &character.Skill{Mastery: character.MasteryGrandMaster}
				for n := 0; n < 1000 && !m.Pilfered; n++ {
					g.combat.trySleightOfHand(c, m)
				}
				if !m.Pilfered {
					t.Fatal("theft never attempted loot")
				}
			case "crate":
				if it, ok := g.rollMapLootEntry(config.RarityRange{Min: 0, Max: config.RarityTier("unique")}); ok {
					t.Fatalf("herb escaped into map crate: %s", it.Name)
				}
				return
			}
			if len(g.party.Inventory) != 1 || g.party.Inventory[0].Name != "Grave Orchid" {
				t.Fatal("monster herb did not reach inventory")
			}
			raw, err := json.Marshal(g.party.Inventory)
			if err != nil {
				t.Fatal(err)
			}
			var restored []items.Item
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			if len(restored) != 1 || restored[0].Name != "Grave Orchid" || restored[0].Count() != 1 {
				t.Fatal("herb identity lost in save")
			}
		})
	}
}

package game

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
)

func TestSolsticeVisitLifecycle(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, stitched := range []bool{false, true} {
		t.Run(fmt.Sprintf("stitched=%v", stitched), func(t *testing.T) {
			g, wm, _ := bootOpenWorldGame(t, stitched)
			if len(wm.FailedMaps) > 0 {
				t.Fatal(wm.FailedMaps)
			}
			g.questManager.ActivateQuest("solstice_connection")
			g.questManager.MarkCompleted("solstice_connection")
			g.questManager.ClaimRewards("solstice_connection")
			hero := g.party.Members[0]
			hero.LearnSpell("jump")
			hero.SpellPoints = 10000
			for _, element := range []struct {
				key   string
				day   int
				night bool
			}{{"fire", 1, false}, {"water", 1, true}, {"earth", 2, false}, {"air", 4, false}} {
				t.Run(element.key, func(t *testing.T) {
					key := "solstice_" + element.key
					for _, h := range g.party.Members {
						h.Level = 20
					}
					g.calendarDay, g.dayNightIsNight = element.day, element.night
					g.dayNightFrames = 0
					if element.night {
						g.dayNightFrames = g.dayNightCycleFrames() / 2
					}
					if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
						t.Fatal(err)
					}
					if err := g.enterAdventureSchedule("solstice_vestibule"); err != nil {
						t.Fatal(err)
					}
					if currentMapKey() != key {
						t.Fatalf("destination=%s want=%s", currentMapKey(), key)
					}
					visit := g.adventure.Visits[key]
					if len(visit.Loot) < 2 {
						t.Fatalf("opening loot was not fixed: %+v", visit.Loot)
					}
					if visit.Level != 25 || visit.Generation != 1 || len(g.world.Monsters) != 19 {
						t.Fatalf("bad new visit: %+v monsters=%d", visit, len(g.world.Monsters))
					}
					for _, m := range g.world.Monsters {
						if m.Level != 25 {
							t.Fatalf("unscaled %s: %d", m.Key, m.Level)
						}
					}
					boss := g.adventureBoss(g.adventureConfig(key))
					if boss == nil {
						t.Fatal("missing boss")
					}
					arena := g.adventureConfig(key).Boss.Arena
					tile := float64(g.config.GetTileSize())
					if arena == nil || g.collisionSystem.CanMoveTo(boss.ID, (float64(arena[0])-.5)*tile, boss.Y) {
						t.Fatal("boss can leave its authored arena")
					}

					g.world.Monsters[0].HitPoints -= 7
					g.world.Monsters[0].SetElementalMark("pressure", monster.ElementalWeaponMark{Count: 2}, 360, 3)
					markedID := g.world.Monsters[0].ID
					g.useAdventureControl(g.adventureConfig(key).Controls[0].ID)
					visit.effectState("checkpoint").Carry = .375
					boss.HitPoints = 0
					g.recordAdventureBoss(boss)
					// Remove the dead actor exactly as the normal death queue does.
					for i, m := range g.world.Monsters {
						if m == boss {
							g.world.Monsters = append(g.world.Monsters[:i], g.world.Monsters[i+1:]...)
							break
						}
					}
					lootBefore := cloneAdventureState(g.adventure).Visits[key].Loot
					snapshot := g.buildSave(wm)
					raw, _ := json.Marshal(snapshot)
					var decoded GameSave
					if err := json.Unmarshal(raw, &decoded); err != nil {
						t.Fatal(err)
					}
					if err := g.applySave(wm, &decoded); err != nil {
						t.Fatal(err)
					}
					foundMark := false
					for _, m := range g.world.Monsters {
						if m.ID == markedID {
							foundMark = m.ElementalMarks["pressure"].Count == 2 && m.ElementalMarks["pressure"].Frames == 360
						}
					}
					if !foundMark {
						t.Fatal("reload lost elemental weapon mark")
					}
					visit = g.adventure.Visits[key]
					if !visit.BossGranted || visit.effectState("checkpoint").Carry != .375 || !reflect.DeepEqual(visit.Loot, lootBefore) {
						t.Fatal("save lost visit state")
					}
					for _, h := range g.party.Members {
						h.Level = 30
						h.SpellPoints = 10000
					}
					if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
						t.Fatal(err)
					}
					if err := g.enterAdventureSchedule("solstice_vestibule"); err != nil {
						t.Fatal(err)
					}
					if g.adventure.Visits[key].Level != 25 || len(g.world.Monsters) != 18 || !g.adventure.Visits[key].BossGranted {
						t.Fatal("same opening reset a cleared visit")
					}
					// A clock change while occupied is inert.
					g.calendarDay += 7
					g.tickEnvironment(0, false)
					if g.adventure.Visits[key].Generation != 1 {
						t.Fatal("occupied visit refreshed")
					}
					if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
						t.Fatal(err)
					}
					if err := g.enterAdventureSchedule("solstice_vestibule"); err != nil {
						t.Fatal(err)
					}
					visit = g.adventure.Visits[key]
					if visit.Generation != 2 || visit.Level != 35 || visit.BossGranted || len(visit.Controls) != 0 || len(g.world.Monsters) != 19 {
						t.Fatalf("refresh failed: %+v", visit)
					}
					if !g.adventure.Records[boss.Key] {
						t.Fatal("refresh erased lifetime record")
					}
				})
			}
		})
	}
}

func TestAdventureTerrainCadence(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			g, wm, ts := travelFixture(t)
			g.combat = NewCombatSystem(g)
			g.turnBasedMode = tb
			a := &config.AdventureConfig{Effects: []config.EnvironmentEffect{{ID: "plate", Kind: "trap", Name: "Plate", Rect: [4]int{6, 7, 6, 7}, School: "physical", Damage: 35, RearmSeconds: 6, RearmRounds: 2}}}
			wm.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
			for _, c := range g.party.Members {
				c.HitPoints = 10000
				c.MaxHitPoints = 10000
				delete(c.Skills, character.SkillDisarmTrap)
			}
			g.setPartyPosition(6.5*ts, 7.5*ts)
			g.applyEnvironmentArrival()
			after := g.party.Members[0].HitPoints
			if after >= 10000 {
				t.Fatal("entry did not trigger")
			}
			g.applyEnvironmentArrival()
			g.tickEnvironment(3, tb)
			g.tickEnvironment(3, tb)
			g.applyEnvironmentArrival()
			if g.party.Members[0].HitPoints != after {
				t.Fatal("standing retriggered trap")
			}
			g.setPartyPosition(5.5*ts, 7.5*ts)
			g.applyEnvironmentArrival()
			g.setPartyPosition(6.5*ts, 7.5*ts)
			g.applyEnvironmentArrival()
			if g.party.Members[0].HitPoints >= after {
				t.Fatal("trap failed to rearm after leave")
			}
		})
	}
}

func TestAdventureScalingIsIdempotent(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, key := range []string{"solstice_ember_elemental", "solstice_aureth"} {
		m := monster.NewMonster3DFromConfig(0, 0, key, cfg)
		scaleAdventureMonster(m, 35)
		hp, damage, armor := m.MaxHitPoints, m.DamageMax, m.ArmorClass
		scaleAdventureMonster(m, 35)
		if m.Level != 35 || m.MaxHitPoints != hp || m.DamageMax != damage || m.ArmorClass != armor {
			t.Fatal("compounding scale")
		}
		scaleAdventureMonster(m, 6)
		if m.MaxHitPoints >= hp || m.DamageMax >= damage || m.Level != 6 {
			t.Fatal("downscale failed")
		}
	}
}

func TestSolsticeWardRecipes(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	for _, c := range []struct{ element, signature, harvest, outdoor, herb string }{
		{"fire", "living_ember", "furnace_bloom", "sun_resin", "embercap"},
		{"water", "condensed_tideglass", "moonwater_lotus", "rain_pearl", "mooncap"},
		{"earth", "anchor_crystal", "deepvein_moss", "calendar_jade", "bitterroot"},
		{"air", "bound_breath", "sky_reed", "whisper_silk", "dawnleaf"},
	} {
		for _, field := range []bool{false, true} {
			key := "solstice_" + c.element
			first, second := "solstice_"+c.signature, "solstice_"+c.harvest
			if field {
				key += "_field"
				first, second = "solstice_"+c.outdoor, c.herb
			}
			key += "_ward"
			r := config.AlchemyRecipeByKey(key)
			if r == nil || r.Output != "brewed_"+key || r.Family != "empowering" || len(r.Ingredients) != 2 {
				t.Fatalf("invalid ward recipe %s", key)
			}
			for i, want := range []string{first, second} {
				if r.Ingredients[i].Materials()[want] != 1 {
					t.Fatalf("%s ingredient %d differs from design", key, i)
				}
			}
		}
	}
}

func TestAdventureScalingSaveProvenance(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	const key = "solstice_earth"
	if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	stats := func(m *monster.Monster3D) [8]int {
		return [8]int{m.Level, m.MaxHitPoints, m.HitPoints, m.ArmorClass, m.DamageMin, m.DamageMax, m.Experience, m.EnrageAtHP}
	}
	for _, format := range []string{"current", "legacy", "legacy_no_anchor"} {
		t.Run(format, func(t *testing.T) {
			g.adventureVisit(key).Level = 56
			g.world.Monsters = nil
			want := map[string][8]int{}
			spawn := wm.LoadedMaps[key].MonsterSpawns[0]
			for i, kind := range []string{"authored", "charmed", "boss", "boss_add", "quest", "party_summon", "unscaled"} {
				if kind == "unscaled" && format != "current" {
					continue
				}
				x, y := TileCenterFromTile(spawn.X, spawn.Y, g.config.GetTileSize())
				m := monster.NewMonster3DFromConfig(x, y, spawn.MonsterKey, g.config)
				m.HomeMap = key
				switch kind {
				case "authored", "charmed":
					scaleAdventureMonster(m, 56)
					if kind == "charmed" {
						m.Bound, m.CharmedByParty, m.BoundFramesRemaining = true, true, 100000
					}
				case "boss":
					for _, sp := range wm.LoadedMaps[key].MonsterSpawns {
						if sp.MonsterKey == "solstice_talura" {
							x, y = TileCenterFromTile(sp.X, sp.Y, g.config.GetTileSize())
							m = monster.NewMonster3DFromConfig(x, y, sp.MonsterKey, g.config)
							m.HomeMap = key
						}
					}
					scaleAdventureMonster(m, 56)
				case "boss_add":
					m.SummonedBy, m.QuestProgressIgnored = "boss-owner", true
				case "quest":
					m.IsEncounterMonster, m.EncounterRewards = true, &monster.EncounterRewards{}
				case "party_summon":
					m.SummonedBy, m.QuestProgressIgnored, m.Bound = spellSummonOwnerPrefix+"test", true, true
					m.BoundFramesRemaining = 100000
					m.MaxHitPoints, m.ArmorClass, m.DamageMin, m.DamageMax = 900, 90, 45, 67
				}
				m.ID = fmt.Sprintf("%s-%d", kind, i)
				m.HitPoints = m.MaxHitPoints - 7
				want[m.ID] = stats(m)
				g.world.Monsters = append(g.world.Monsters, m)
			}
			for reload := 0; reload < 2; reload++ {
				saved := auditSaveJSON(t, g.buildSave(wm))
				if format != "current" && reload == 0 {
					// Exercise pre-provenance JSON, including older saves without anchors.
					raw, _ := json.Marshal(saved)
					var document map[string]any
					if err := json.Unmarshal(raw, &document); err != nil {
						t.Fatal(err)
					}
					strip := func(records []any) {
						for _, record := range records {
							fields := record.(map[string]any)
							delete(fields, "adventure_scale_level")
							if format == "legacy_no_anchor" {
								delete(fields, "spawn_position")
							}
						}
					}
					strip(document["monsters"].([]any))
					for _, records := range document["map_monsters"].(map[string]any) {
						if records != nil {
							strip(records.([]any))
						}
					}
					raw, _ = json.Marshal(document)
					saved = GameSave{}
					if err := json.Unmarshal(raw, &saved); err != nil {
						t.Fatal(err)
					}
				}
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				if len(g.world.Monsters) != len(want) {
					t.Fatal("reload changed roster")
				}
				for _, m := range g.world.Monsters {
					if got := stats(m); got != want[m.ID] {
						t.Errorf("reload %d %s stats=%v want=%v", reload, m.ID, got, want[m.ID])
					}
				}
			}
		})
	}
}

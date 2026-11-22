package game

import (
	"encoding/json"
	"fmt"
	"math"
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
					// Its add appears in the arena at the visit level and shares the
					// arena, also after a reload.
					if !g.combat.summonBossAdds(boss) {
						t.Fatal("boss could not summon its add")
					}
					add := g.world.Monsters[len(g.world.Monsters)-1]
					addID := add.ID
					for reload := range 2 {
						if reload == 1 {
							snapshot := g.buildSave(wm)
							if err := g.applySave(wm, &snapshot); err != nil {
								t.Fatal(err)
							}
							boss, add, visit = g.adventureBoss(g.adventureConfig(key)), nil, g.adventure.Visits[key]
							for _, m := range g.world.Monsters {
								if m.ID == addID {
									add = m
								}
							}
						}
						if add == nil || add.SummonedBy != boss.ID || add.Level != 25 {
							t.Fatalf("reload=%d: add %+v is not the boss's level-25 add", reload, add)
						}
						if x, y := TileIndex(add.X, tile), TileIndex(add.Y, tile); x < arena[0] || y < arena[1] || x > arena[2] || y > arena[3] ||
							g.collisionSystem.CanMoveTo(add.ID, (float64(arena[0])-.5)*tile, add.Y) {
							t.Fatalf("reload=%d: add at %d,%d is not held in the arena", reload, x, y)
						}
					}
					for i, m := range g.world.Monsters {
						if m == add {
							g.world.Monsters = append(g.world.Monsters[:i], g.world.Monsters[i+1:]...)
							g.collisionSystem.UnregisterEntity(add.ID)
							break
						}
					}

					g.world.Monsters[0].HitPoints -= 7
					g.world.Monsters[0].SetElementalMark("pressure", monster.ElementalWeaponMark{Count: 2}, 360, 3)
					markedID := g.world.Monsters[0].ID
					g.useAdventureControl(g.adventureConfig(key).Controls[0].ID)
					visit.effectState("checkpoint").Carry = .375
					boss.HitPoints = 0
					g.recordAdventureBoss(boss)
					victoryAt := g.currentQuestDay()
					if visit.VictoryAt != victoryAt || g.adventureResetAt(key, visit) != victoryAt+3 {
						t.Fatal("victory did not start a three-day chamber cooldown")
					}
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
					if !visit.BossGranted || visit.VictoryAt != victoryAt || visit.effectState("checkpoint").Carry != .375 || !reflect.DeepEqual(visit.Loot, lootBefore) {
						t.Fatal("save lost visit state")
					}
					for _, h := range g.party.Members {
						h.Level = 30
						h.SpellPoints = 10000
					}
					if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
						t.Fatal(err)
					}
					if err := g.enterAdventureSchedule("solstice_vestibule"); err == nil {
						t.Fatal("cleared chamber remained available")
					}
					if g.adventure.Visits[key].Level != 25 || !g.adventure.Visits[key].BossGranted {
						t.Fatal("blocked entry changed the visit")
					}
					g.calendarDay += 2
					if g.adventureAvailable(key, g.currentQuestDay()) {
						t.Fatal("chamber reopened before three days")
					}
					// The cooldown ends independently of the entrance's rotation.
					if !g.adventureAvailable(key, victoryAt+3) {
						t.Fatal("chamber stayed locked after three days")
					}
					// Return at the same weekly window; time alone never spawns a visit.
					g.calendarDay += 5
					g.tickEnvironment(false)
					if g.adventure.Visits[key].Generation != 1 {
						t.Fatal("visit refreshed without entering")
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
			tickEnvironmentFor(g, 3, tb)
			tickEnvironmentFor(g, 3, tb)
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
	for _, tc := range []struct {
		key                                 string
		level, hp, damageMin, damageMax, xp int
	}{
		{"solstice_aureth", 27, 3150, 40, 60, 4220},
		{"solstice_aureth", 34, 6388, 65, 98, 7333},
		{"solstice_aureth", 35, 7067, 70, 105, 7848},
		{"solstice_aureth", 36, 7564, 75, 113, 8379},
		{"solstice_aureth", 40, 9555, 99, 149, 10682},
		{"solstice_aureth", 45, 12043, 141, 212, 13952},
		{"solstice_aureth", 50, 14531, 161, 241, 17658},
		{"solstice_aureth", 60, 19506, 200, 299, 26378},
		{"solstice_aureth", 75, 26970, 258, 387, 42728},
		{"solstice_talura", 35, 7057, 65, 98, 7848},
		{"solstice_talura", 50, 14511, 150, 225, 17658},
		{"solstice_ember_elemental", 35, 2249, 75, 109, 1728},
		{"solstice_ember_elemental", 50, 4051, 151, 218, 3888},
		{"solstice_ember_elemental", 75, 7525, 242, 351, 9408},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.key, tc.level), func(t *testing.T) {
			m := monster.NewMonster3DFromConfig(0, 0, tc.key, cfg)
			for _, level := range []int{tc.level, tc.level, 6, tc.level} {
				scaleAdventureMonster(cfg.AdventureScaling, m, level)
				if level == 6 {
					if m.MaxHitPoints >= tc.hp || m.DamageMax >= tc.damageMax || m.Level != 6 {
						t.Fatal("downscale failed")
					}
					continue
				}
				if m.Level != tc.level || m.MaxHitPoints != tc.hp || m.HitPoints != tc.hp || m.DamageMin != tc.damageMin || m.DamageMax != tc.damageMax || m.Experience != tc.xp {
					t.Fatalf("level=%d HP=%d/%d damage=%d..%d XP=%d; want level=%d HP=%d damage=%d..%d XP=%d", m.Level, m.HitPoints, m.MaxHitPoints, m.DamageMin, m.DamageMax, m.Experience, tc.level, tc.hp, tc.damageMin, tc.damageMax, tc.xp)
				}
			}
		})
	}
}

// Auto-level moves a monster's attack, and every ability authored as a share
// of it follows: a breath at 100% hits the bite's band at every level, a
// telegraph's field and the brood mother's trap volley grow with the attack.
// At the authored level the shares keep the old flat numbers (field 8 per
// second, volley 40), and scaling never writes through to the catalog.
func TestAdventureScalingMovesAbilityDamage(t *testing.T) {
	cfg := loadTestConfig(t)
	field := func(m *monster.Monster3D) int {
		if m.Abilities.Telegraph == nil {
			return 0
		}
		return percentOfAttack(m.DamageMin, m.DamageMax, m.Abilities.Telegraph.SustainPercent)
	}
	volley := func(m *monster.Monster3D) int {
		return percentOfAttack(m.DamageMin, m.DamageMax, m.Abilities.TrapVolley.DamagePercent)
	}
	for _, c := range []struct {
		key           string
		field, volley int // at the authored level
	}{{"dragon", 0, 0}, {"solstice_ember_elemental", 8, 0}, {"dragon_brood_mother", 0, 40}} {
		t.Run(c.key, func(t *testing.T) {
			d := monster.MonsterConfig.Monsters[c.key]
			authored := d.Abilities.Clone()
			m := monster.NewMonster3DFromConfig(0, 0, c.key, cfg)
			if field(m) != c.field || volley(m) != c.volley {
				t.Fatalf("authored level: field %d volley %d, want %d and %d", field(m), volley(m), c.field, c.volley)
			}
			for _, level := range []int{d.Level + 20, 6, d.Level + 20} {
				scaleAdventureMonster(cfg.AdventureScaling, m, level)
				ratio := float64(m.DamageMin+m.DamageMax) / float64(d.DamageMin+d.DamageMax)
				// A whole percent rounds the authored number by under 1, which the
				// ratio scales.
				for _, got := range []struct{ now, base int }{{field(m), c.field}, {volley(m), c.volley}} {
					if math.Abs(float64(got.now)-float64(got.base)*ratio) > 1+ratio {
						t.Fatalf("level %d: %d, want %d moved with the attack (x%.2f)", level, got.now, got.base, ratio)
					}
				}
				for _, a := range m.Abilities.PartyAttacks {
					lo, hi := a.DamageBand(m.DamageMin, m.DamageMax)
					share := float64(lo+hi) / float64(m.DamageMin+m.DamageMax) * 100
					if math.Abs(share-float64(a.DamagePercent)) > 2 {
						t.Fatalf("level %d: %s band %d-%d is %.0f%% of the attack %d-%d, want %d%%", level, a.Key, lo, hi, share, m.DamageMin, m.DamageMax, a.DamagePercent)
					}
					if a.DamagePercent == 100 && (lo != m.DamageMin || hi != m.DamageMax) {
						t.Fatalf("level %d: %s at 100%% hits %d-%d, the attack %d-%d", level, a.Key, lo, hi, m.DamageMin, m.DamageMax)
					}
				}
			}
			if !reflect.DeepEqual(monster.MonsterConfig.Monsters[c.key].Abilities, authored) {
				t.Fatal("scaling a monster wrote through to the catalog")
			}
		})
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
			r := alchemyRecipeByKey(key)
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
		return [8]int{m.Level, m.MaxHitPoints, m.HitPoints, m.ArmorClass, m.DamageMin, m.DamageMax, m.Experience, m.Abilities.Enrage.AtHP}
	}
	for _, format := range []string{"current", "legacy", "legacy_no_anchor", "old_boss_curve"} {
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
					scaleAdventureMonster(g.config.AdventureScaling, m, 56)
					if kind == "charmed" {
						m.Bound, m.CharmedByParty, m.BoundClock = true, true, framesClock(100000)
					}
				case "boss":
					for _, sp := range wm.LoadedMaps[key].MonsterSpawns {
						if sp.MonsterKey == "solstice_talura" {
							x, y = TileCenterFromTile(sp.X, sp.Y, g.config.GetTileSize())
							m = monster.NewMonster3DFromConfig(x, y, sp.MonsterKey, g.config)
							m.HomeMap = key
						}
					}
					scaleAdventureMonster(g.config.AdventureScaling, m, 56)
				case "boss_add":
					m.SummonedBy, m.QuestProgressIgnored = "boss-owner", true
				case "quest":
					m.IsEncounterMonster, m.EncounterRewards = true, &monster.EncounterRewards{}
				case "party_summon":
					m.SummonedBy, m.QuestProgressIgnored, m.Bound = spellSummonOwnerPrefix+"test", true, true
					m.BoundClock = framesClock(100000)
					m.MaxHitPoints, m.ArmorClass, m.DamageMin, m.DamageMax = 900, 90, 45, 67
				}
				m.ID = fmt.Sprintf("%s-%d", kind, i)
				m.HitPoints = m.MaxHitPoints - 7
				if kind == "boss" && format == "old_boss_curve" {
					m.HitPoints = m.MaxHitPoints
					want[m.ID] = stats(m)
					// Old exponential saves can exceed the new maximum HP.
					m.HitPoints *= 2
				} else {
					want[m.ID] = stats(m)
				}
				g.world.Monsters = append(g.world.Monsters, m)
			}
			for reload := 0; reload < 2; reload++ {
				saved := auditSaveJSON(t, g.buildSave(wm))
				if (format == "legacy" || format == "legacy_no_anchor") && reload == 0 {
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

// Percent hazards are a share of the boss attack at the visit's level: they
// grow with the chamber and match the boss's scaled attack; flat ones stay put.
func TestAdventurePercentDamageFollowsTheBoss(t *testing.T) {
	cfg := loadTestConfig(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	a := &config.AdventureConfig{Boss: &config.AdventureBoss{Monster: "solstice_aureth"}}
	lane := config.EnvironmentEffect{ID: "lane", Kind: "lane", DamagePercent: 150, SustainPercent: 25}
	vent := config.EnvironmentEffect{ID: "vent", Kind: "occupation", Damage: 12}
	previous := 0
	for _, level := range []int{27, 35, 45} {
		boss := monster.NewMonster3DFromConfig(0, 0, a.Boss.Monster, cfg)
		scaleAdventureMonster(cfg.AdventureScaling, boss, level)
		lo, hi := adventureBossAttack(cfg.AdventureScaling, a, &AdventureVisit{Level: level})
		if lo != boss.DamageMin || hi != boss.DamageMax {
			t.Fatalf("L%d: hazard attack %d-%d, scaled boss %d-%d", level, lo, hi, boss.DamageMin, boss.DamageMax)
		}
		got := effectDamage(lane, lo, hi)
		if sum := boss.DamageMin + boss.DamageMax; got.Damage != sum*150/200 || got.SustainDamage != sum*25/200 {
			t.Fatalf("L%d: lane %d / lingering %d, boss attack %d-%d", level, got.Damage, got.SustainDamage, boss.DamageMin, boss.DamageMax)
		}
		if got.Damage <= previous {
			t.Fatalf("L%d: lane damage %d did not grow past %d", level, got.Damage, previous)
		}
		previous = got.Damage
		if flat := effectDamage(vent, lo, hi); flat.Damage != 12 {
			t.Fatalf("L%d: flat damage became %d", level, flat.Damage)
		}
	}
}

// tickEnvironmentFor runs seconds of game time through tickEnvironment: RT
// frames, or TB rounds.
func tickEnvironmentFor(g *MMGame, seconds float64, turn bool) {
	n := int(math.Round(seconds * float64(g.config.GetTPS())))
	if turn {
		n = int(math.Ceil(seconds / config.CombatRoundSeconds))
	}
	for range n {
		g.tickEnvironment(turn)
	}
}

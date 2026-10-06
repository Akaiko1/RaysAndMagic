package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func TestEditorBossMechanicsRuntimeAndSave(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	previous := config.GlobalBossMechanics
	t.Cleanup(func() { config.GlobalBossMechanics = previous })
	g, wm, _ := bootOpenWorldGame(t, false)
	g.config.MonsterCombat.ElementalAttack.Chance = 0
	key := "solstice_water"
	if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	a := wm.MapConfigs[key].Adventure
	a.Boss.HealPercent = 5
	a.Boss.HealCapPercent = 25
	a.Controls = []config.EnvironmentControl{{ID: "first", StopHealing: true}, {ID: "second", StopHealing: true}}
	config.GlobalBossMechanics = &config.BossMechanicsConfig{Maps: map[string][]config.BossMechanic{key: {
		{ID: "crit", Stat: "critical_chance", Base: 100, Reductions: map[string]int{"first": 100}},
		{ID: "dodge", Stat: "perfect_dodge", Base: 100, Reductions: map[string]int{"second": 100}},
	}}}
	// Merely evaluating combat stats must not create a visit or saved progress.
	g.adventure = AdventureState{}
	before := g.buildSave(wm)
	boss := g.adventureBoss(a)
	if got := g.bossMechanicValue(boss, "perfect_dodge", 0); got != 100 {
		t.Fatalf("base dodge=%d", got)
	}
	g.combat.bossCriticalDamage(boss, damagecalc.Parts{Normal: 10})
	after := g.buildSave(wm)
	beforeJSON, _ := json.Marshal(before.Adventure)
	afterJSON, _ := json.Marshal(after.Adventure)
	if string(beforeJSON) != string(afterJSON) || len(g.adventure.Visits) != 0 {
		t.Fatal("reading boss mechanics created saved visit state")
	}
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g.turnBasedMode = tb
			g.adventureVisit(key)
			g.adventure.Visits[key] = &AdventureVisit{}
			boss := g.adventureBoss(a)
			boss.HitPoints = 500
			boss.MaxHitPoints = 1000
			boss.IsEngagingPlayer = true
			boss.ArmorClass = 0
			boss.Resistances = nil
			boss.TrueDamage = 3
			hero := g.party.Members[0]
			for _, stage := range []struct {
				name, control string
				normal, dodge int
				heals         bool
			}{{"initial", "", 20, 100, true}, {"first switch", "first", 10, 100, true}, {"all switches", "second", 10, 0, false}} {
				t.Run(stage.name, func(t *testing.T) {
					if stage.control != "" {
						dialogActions["adventure_control"](&InputHandler{game: g}, nil, &character.NPCDialogueChoice{Control: stage.control})
					}
					for _, reload := range []bool{false, true} {
						if reload {
							snapshot := g.buildSave(wm)
							raw, err := json.Marshal(snapshot)
							if err != nil {
								t.Fatal(err)
							}
							var decoded GameSave
							if err = json.Unmarshal(raw, &decoded); err != nil {
								t.Fatal(err)
							}
							if err = g.applySave(wm, &decoded); err != nil {
								t.Fatal(err)
							}
							boss = g.adventureBoss(g.adventureConfig(key))
							hero = g.party.Members[0]
						}
						boss.MaxHitPoints = 1000
						boss.Resistances = nil
						boss.ArmorClass = 0
						boss.TrueDamage = 3
						boss.IsEngagingPlayer = true
						hit := g.combat.normalMonsterMeleeHit(boss, 10)
						if hit.Parts.Normal != stage.normal || hit.Parts.True != 3 {
							t.Fatalf("reload=%v: outgoing hit %+v, want normal %d, true 3", reload, hit, stage.normal)
						}
						// Resolve the produced attack through the actual party hit sink.
						hero.HitPoints = hero.MaxHitPoints
						hit.IgnoresArmor = true
						hit.IgnoresDodge = true
						before := hero.HitPoints
						g.combat.monsterHitCharacter(boss, hero, boss.Name, hit)
						if hero.HitPoints >= before {
							t.Fatal("boss attack did not damage party")
						}
						if got := g.bossMechanicValue(boss, "perfect_dodge", 0); got != stage.dodge {
							t.Fatalf("reload=%v: dodge %d want %d", reload, got, stage.dodge)
						}
						attacker := &monster.Monster3D{ID: "test_ally", Name: "Ally", HitPoints: 100, MaxHitPoints: 100, SummonedBy: "party"}
						before = boss.HitPoints
						g.combat.strikeMonsterFor(attacker, boss, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 10, True: 3}, DamageType: "physical", IgnoresArmor: true}, nil, false)
						want := 13
						if stage.dodge == 100 {
							want = 3
						}
						if before-boss.HitPoints != want {
							t.Fatalf("reload=%v: incoming damage %d, want %d", reload, before-boss.HitPoints, want)
						}
						// Dodge-piercing attacks retain their existing bypass.
						before = boss.HitPoints
						g.combat.strikeMonsterFor(attacker, boss, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 10}, DamageType: "physical", IgnoresArmor: true, IgnoresDodge: true}, nil, false)
						if before-boss.HitPoints != 10 {
							t.Fatal("ignore-dodge attack was evaded")
						}
						boss.HitPoints = 500
						visit := g.adventureVisit(key)
						visit.HealCarry = 0
						visit.BossHealed = 0
						g.tickEnvironment(6, tb)
						expected := 500
						if stage.heals {
							expected = 550
						}
						if boss.HitPoints != expected {
							t.Fatalf("reload=%v: regen HP=%d want %d", reload, boss.HitPoints, expected)
						}
					}
				})
			}
		})
	}
}

func TestEditorArmorControlsSurviveLoadAndRespawn(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	const key = "solstice_earth"
	mc := wm.MapConfigs[key]
	// Exercise an ordinary respawning map, plus a legacy scaled visit. The
	// shipped chamber geometry supplies real map transitions and boss spawns.
	mc.RespawnDays = 3
	mc.Adventure = &config.AdventureConfig{
		Boss: &config.AdventureBoss{Monster: "solstice_talura", ArmorFloor: 20},
		Controls: []config.EnvironmentControl{
			{ID: "brace", ArmorReduction: 18}, {ID: "release", ArmorReduction: 100},
		},
	}
	for _, tc := range []struct {
		name         string
		level, armor int
	}{{"ordinary", 0, 48}, {"scaled visit", 56, 68}} {
		t.Run(tc.name, func(t *testing.T) {
			g.adventureVisit(key)
			g.adventure.Visits[key] = &AdventureVisit{Level: tc.level}
			wm.LoadedMaps[key].Monsters = nil
			wm.LoadedMaps[key].LastRespawnDay = 0
			if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
				t.Fatal(err)
			}
			boss := g.adventureBoss(mc.Adventure)
			if boss == nil || boss.ArmorClass != tc.armor {
				t.Fatalf("fresh boss armor: %+v, want %d", boss, tc.armor)
			}
			for _, stage := range []struct {
				control string
				armor   int
			}{{"brace", tc.armor - 18}, {"release", 20}} {
				dialogActions["adventure_control"](&InputHandler{game: g}, nil, &character.NPCDialogueChoice{Control: stage.control})
				boss.HitPoints = 123
				for reload := 0; reload < 2; reload++ {
					if boss.ArmorClass != stage.armor || boss.HitPoints != 123 {
						t.Fatalf("%s reload=%d armor/HP=%d/%d, want %d/123", stage.control, reload, boss.ArmorClass, boss.HitPoints, stage.armor)
					}
					raw, err := json.Marshal(g.buildSave(wm))
					if err != nil {
						t.Fatal(err)
					}
					var saved GameSave
					if err = json.Unmarshal(raw, &saved); err != nil {
						t.Fatal(err)
					}
					if err = g.applySave(wm, &saved); err != nil {
						t.Fatal(err)
					}
					boss = g.adventureBoss(mc.Adventure)
				}
				if boss.ArmorClass != stage.armor || boss.HitPoints != 123 {
					t.Fatal("repeated load changed armor or health")
				}
			}
			// Migrating an empty unstamped legacy roster must use the same rule.
			saved := g.buildSave(wm)
			saved.MapMonsters[key] = nil
			delete(saved.MapRespawnDay, key)
			if err := g.applySave(wm, &saved); err != nil {
				t.Fatal(err)
			}
			boss = g.adventureBoss(mc.Adventure)
			if boss == nil || boss.ArmorClass != 20 || boss.HitPoints != boss.MaxHitPoints {
				t.Fatal("legacy roster migration lost boss controls")
			}
			// Re-entry before the deadline preserves the live boss. Expired and
			// unknown-age rosters create a fresh boss under the same controls.
			for _, stamp := range []string{"current", "expired", "unknown"} {
				retained := boss
				retained.CharmedByParty = true
				retained.ArmorClass, retained.HitPoints = 17, 111
				if err := g.transitionToMap(mapTransition{mapKey: "forest", arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				switch stamp {
				case "expired":
					g.calendarDay += 3
				case "unknown":
					wm.LoadedMaps[key].LastRespawnDay = 0
				}
				if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				if retained.ArmorClass != 17 || retained.HitPoints != 111 {
					t.Fatal("respawn reset a retained ally")
				}
				var fresh *monster.Monster3D
				for _, m := range g.world.Monsters {
					if m.Key == "solstice_talura" && !m.CharmedByParty {
						fresh = m
					}
				}
				if stamp == "current" {
					if fresh != nil {
						t.Fatal("early re-entry respawned the boss")
					}
				} else {
					if fresh == nil || fresh.ArmorClass != 20 || fresh.HitPoints != fresh.MaxHitPoints {
						t.Fatalf("%s respawn did not preserve controls: %+v", stamp, fresh)
					}
					boss = fresh
				}
			}
		})
	}
}

func TestEditorSceneProjectionThroughRuntime(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	for _, key := range []string{"forest", "deep_jungle", "highlands"} {
		t.Run(key, func(t *testing.T) {
			r := wm.OpenWorldRegionByKey(key)
			if r == nil {
				t.Fatalf("region missing %s", key)
			}
			for _, orient := range []string{"", "rot90", "rot180", "rot270", "mirror_x", "mirror_y"} {
				t.Run(orient, func(t *testing.T) {
					oldOrient := r.Orient
					r.Orient = orient
					defer func() { r.Orient = oldOrient }()
					mc := wm.MapConfigs[key]
					old := mc.Adventure
					defer func() { mc.Adventure = old }()
					local := [4]int{2, 3, 4, 5}
					mc.Adventure = &config.AdventureConfig{Effects: []config.EnvironmentEffect{{ID: "zone", Kind: "trap", Rect: local, School: "physical", Damage: 10}}, Controls: []config.EnvironmentControl{{ID: "edit", Tiles: []config.EnvironmentTile{{X: 3, Y: 4, Tile: "empty"}}}}, JumpLinks: [][4]int{{2, 3, 4, 3}}}
					wm.CurrentMapKey = key
					g.world = wm.OpenWorld
					a := g.adventureConfig(key)
					if allocations := testing.AllocsPerRun(20, func() { g.adventureConfig(key) }); allocations != 0 {
						t.Fatalf("cached projection allocated %g times", allocations)
					}

					x0, y0 := wm.ProjectTile(key, 2, 3)
					x1, y1 := wm.ProjectTile(key, 4, 5)
					want := [4]int{min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)}
					if a.Effects[0].Rect != want {
						t.Fatalf("zone=%v want %v", a.Effects[0].Rect, want)
					}
					x, y := wm.ProjectTile(key, 3, 4)
					oldTile := g.world.Tiles[y][x]
					defer func() { g.world.Tiles[y][x] = oldTile }()
					g.adventureVisit(key)
					g.adventure.Visits[key] = &AdventureVisit{}
					dialogActions["adventure_control"](&InputHandler{game: g}, nil, &character.NPCDialogueChoice{Control: "edit"})
					empty, _ := world.GlobalTileManager.GetTileTypeFromKey("empty")
					if g.world.Tiles[y][x] != empty || !g.adventureVisit(key).Controls["edit"] {
						t.Fatal("control edited the wrong region tile")
					}
					if mc.Adventure.Effects[0].Rect != local || mc.Adventure.Controls[0].Tiles[0].X != 3 {
						t.Fatal("projection mutated source YAML model")
					}
				})
			}
		})
	}
	// A retained stitched manager must not project indoor coordinates.
	g.world = wm.LoadedMaps["solstice_water"]
	a := g.adventureConfig("forest")
	if a != nil {
		t.Fatal("test source should be restored")
	}
}

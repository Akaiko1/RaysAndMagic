package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// Spawn cases cross RT/TB, current/remote map, below/exact/retained level,
// day/night, and zero/partial/full vacant base slots. Existing merged-world
// pack tests cover coordinate projection in the same spawn path.
func TestDervishPackLevelAndVacancies(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, active := range []bool{false, true} {
			for _, level := range []int{19, 20} {
				for _, night := range []bool{false, true} {
					for _, vacancies := range []int{0, 3, 5} {
						t.Run(fmt.Sprintf("TB=%v/active=%v/level=%d/night=%v/free=%d", tb, active, level, night, vacancies), func(t *testing.T) {
							g, _, w := packSlotTestGame(t, active)
							g.turnBasedMode = tb
							g.party = &character.Party{Members: []*character.MMCharacter{{Level: level}}}
							g.config.DayNight.Packs[0] = config.DayNightPackConfig{Map: "pack_test", DayMonsters: []config.PackMemberConfig{{Monster: "desert_dervish", Count: 5, MinPartyLevel: 20, QuestProgress: true}}, NightMonster: "mummy", Count: 5, MinPlayerDistTiles: .5}
							for i := 0; i < vacancies; i++ {
								w.Monsters[i].HitPoints = 0
							}
							g.syncDayNightPacks(night)
							want := vacancies
							if !night && level < 20 {
								want = 0
							}
							spawned := livePhasePack(w, night)
							if len(spawned) != want {
								t.Fatalf("spawned %d, want %d", len(spawned), want)
							}
							for _, m := range spawned {
								if !night && (m.Key != "desert_dervish" || m.QuestProgressIgnored) {
									t.Fatalf("wrong daytime member: %s ignored=%v", m.Key, m.QuestProgressIgnored)
								}
							}
							if level == 20 {
								g.party.Members[0].Level = 1
								if !g.partyLevelUnlocked(20) {
									t.Fatal("roster change revoked unlock")
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestSafiyaUnlockAndTimelinePersistence(t *testing.T) {
	g, wm := dailyTimerTestGame(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	npc, err := character.CreateNPCFromConfig("nomad_city_safiya", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, night := range []bool{false, true} {
		g.dayNightIsNight = night
		g.maxPartyLevel = 0
		g.party = &character.Party{Members: []*character.MMCharacter{{Level: 19}, {Level: 20}}}
		if !g.npcAbsent(npc) {
			t.Fatal("mean level below20 must hide giver")
		}
		g.party.Members[0].Level = 20
		g.updatePartyLevelUnlocks()
		if g.npcAbsent(npc) {
			t.Fatal("level20 must reveal giver")
		}
		saved := g.buildSave(wm)
		if saved.MaxPartyLevel != 20 {
			t.Fatal("save lost permanent level unlock")
		}
		g.maxPartyLevel = 99
		g.restoreSavedTimeline(wm, &saved, g.world)
		g.party.Members[0].Level = 1
		g.party.Members[1].Level = 1
		if g.npcAbsent(npc) {
			t.Fatal("loaded unlock lost after roster change")
		}
		saved.MaxPartyLevel = 0
		g.restoreSavedTimeline(wm, &saved, g.world)
		if !g.npcAbsent(npc) {
			t.Fatal("loading another timeline leaked level unlock")
		}
	}
}

func TestDervishFixedQuotaAcrossPacks(t *testing.T) {
	t.Chdir("../..")
	for _, merged := range []bool{false, true} {
		t.Run(fmt.Sprintf("merged=%v", merged), func(t *testing.T) {
			g, wm, _ := bootOpenWorldGame(t, merged)
			w := wm.WorldByKey("desert")
			w.Monsters = nil
			g.world = w
			wm.CurrentMapKey = "desert"
			g.combat = NewCombatSystem(g)
			ih := &InputHandler{game: g}
			ih.handleGiveQuest("toll_of_blades")
			q := g.questManager.GetQuest("toll_of_blades")
			if q == nil || q.Target() != 5 || q.Completed {
				t.Fatalf("empty map changed fixed quota: %+v", q)
			}
			for n := 1; n <= 5; n++ {
				tx, ty := wm.ProjectTile("desert", 2, 2)
				xw, yw := TileCenterFromTile(tx, ty, g.config.GetTileSize())
				m := monster.NewMonster3DFromConfig(xw, yw, "desert_dervish", g.config)
				m.HitPoints = 0
				g.combat.updateQuestProgress(m)
				g.completeKillQuestIfCleared(q, false)
				if q.CurrentCount != n || q.Completed != (n == 5) || q.Target() != 5 {
					t.Fatalf("kill %d: %+v", n, q)
				}
				if n == 3 {
					g.refreshRepeatableQuests("night")
					if g.questManager.GetQuest(q.ID) != q {
						t.Fatal("night lost unfinished progress")
					}
				}
			}
		})
	}
}

func TestMirageSaltCombinedBuff(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.turnBasedMode = tb
			before := make([]int, len(g.party.Members))
			for i, c := range g.party.Members {
				before[i] = cs.PerfectDodgeChance(c)
			}
			for use := 0; use < 2; use++ {
				g.party.Inventory = []items.Item{items.CreateItemFromYAML("mirage_salt")}
				if !g.UseConsumableFromInventory(0, 0) || len(g.party.Inventory) != 0 {
					t.Fatal("use failed")
				}
				b, ok := g.combatBuffByID("mirage_salt")
				if !ok || b.Frames != 120*g.config.GetTPS() || g.combatBuffSchoolResistPct("fire") != 30 {
					t.Fatalf("buff=%+v", b)
				}
				for i, c := range g.party.Members {
					if cs.PerfectDodgeChance(c) != min(100, before[i]+15) {
						t.Fatal("dodge missing or stacked")
					}
				}
				if len(g.combatBuffs) != 1 {
					t.Fatal("recast stacked")
				}
				g.combatBuffs[0].Frames = 10
			}
			g.combatBuffs = restoreCombatBuffs(buildCombatBuffSaves(g.combatBuffs))
			if g.combatBuffDodgePct() != 15 || g.combatBuffSchoolResistPct("fire") != 30 || g.combatBuffs[0].Frames != 10 {
				t.Fatal("load lost combined buff")
			}
			for i := 0; i < 10; i++ {
				g.tickCombatBuffs()
			}
			if g.combatBuffDodgePct() != 0 || g.combatBuffSchoolResistPct("fire") != 0 {
				t.Fatal("expired buff still active")
			}
			def, _ := config.GetItemDefinition("mirage_salt")
			lines := strings.Join(def.EffectLines(), " ")
			if !strings.Contains(lines, "15%") || !strings.Contains(lines, "30%") || !strings.Contains(lines, "120s") {
				t.Fatalf("shared tooltip missing effect: %s", lines)
			}
		})
	}
}

func TestDervishUsesBanditAttackRules(t *testing.T) {
	for _, distance := range []int{1, 2} {
		for _, proc := range []bool{false, true} {
			t.Run(fmt.Sprintf("distance=%d/proc=%v", distance, proc), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				elementalTestBiome(t, cs, "fire")
				d := monster.NewMonster3DFromConfig(0, 0, "desert_dervish", g.config)
				if d.Level != 24 || d.MaxHitPoints != 900 || d.PerfectDodge != 20 {
					t.Fatalf("unexpected dervish stats")
				}
				if d.ProjectileWeapon != "throwing_knife" {
					t.Fatal("lost bandit ranged attack")
				}
				g.party.Members = g.party.Members[:1]
				ch := g.party.Members[0]
				isolateTrueDamageMember(ch, 0)
				ch.HitPoints, ch.MaxHitPoints = 1000, 1000
				g.camera.X, g.camera.Y = float64(distance)*g.config.GetTileSize(), 0
				d.DamageMin, d.DamageMax = 40, 40
				rolls := 0
				cs.elementalAttackRoll = func() float64 {
					rolls++
					if proc {
						return 0
					}
					return .99
				}
				cs.performMonsterAttackAgainstParty(d)
				if distance == 1 {
					want := 40
					if proc {
						want = 80
					}
					if len(g.arrows) != 0 || rolls != 1 || ch.HitPoints != 1000-want {
						t.Fatalf("melee: arrows=%d rolls=%d HP=%d", len(g.arrows), rolls, ch.HitPoints)
					}
				} else if len(g.arrows) != 1 || rolls != 0 || ch.HitPoints != 1000 {
					t.Fatalf("ranged: arrows=%d rolls=%d HP=%d", len(g.arrows), rolls, ch.HitPoints)
				}
			})
		}
	}
}

package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func TestTrainerRegionalProgressionContract(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	ih := NewInputHandler(g)
	for _, key := range []string{"city_mastery_trainer", "nomad_city_trainer"} {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range []string{"untaken", "active", "unclaimed", "claimed"} {
			for mastery := -1; mastery <= int(character.MasteryGrandMaster); mastery++ {
				for _, magic := range []bool{false, true} {
					for _, short := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/mastery=%d/magic=%v/short=%v", key, state, mastery, magic, short), func(t *testing.T) {
							qm.Reset()
							if state != "untaken" {
								if err := qm.ActivateQuest("pit_standing"); err != nil {
									t.Fatal(err)
								}
								if state == "unclaimed" || state == "claimed" {
									qm.MarkCompleted("pit_standing")
								}
								if state == "claimed" {
									if _, err := qm.ClaimRewards("pit_standing"); err != nil {
										t.Fatal(err)
									}
								}
							}
							member := g.party.Members[0]
							member.Skills = map[character.SkillType]*character.Skill{}
							member.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
							if mastery >= 0 {
								if magic {
									member.MagicSchools[character.MagicSchoolFire] = &character.MagicSkill{Mastery: character.SkillMastery(mastery)}
								} else {
									member.Skills[character.SkillSword] = &character.Skill{Mastery: character.SkillMastery(mastery)}
								}
							}
							cost := 0
							if key == "city_mastery_trainer" {
								if mastery == int(character.MasteryNovice) {
									cost = 1000
								}
								if mastery == int(character.MasteryExpert) {
									cost = 4000
								}
							} else if mastery == int(character.MasteryMaster) {
								cost = 10000
							}
							offers := trainerOptions(member, npc)
							if (len(offers) == 1) != (cost > 0) {
								t.Fatalf("offers=%v, want eligible=%v", offers, cost > 0)
							}
							if cost > 0 && (offers[0].Cost != cost || int(offers[0].Next) != mastery+1) {
								t.Fatalf("wrong displayed offer: %+v", offers[0])
							}
							gold := 20000
							if cost > 0 {
								gold = cost
							}
							if short {
								gold--
							}
							g.party.Gold = gold
							g.dialogNPC = npc
							g.selectedCharIdx = 0
							g.dialogSelectedSpell = 0
							ih.purchaseSelectedTraining()
							allowed := cost > 0 && !short && (key == "city_mastery_trainer" || state == "claimed")
							wantGold := gold
							if allowed {
								wantGold -= cost
							}
							if g.party.Gold != wantGold {
								t.Fatalf("gold=%d, want %d", g.party.Gold, wantGold)
							}
							got := mastery
							if mastery >= 0 {
								if magic {
									got = int(member.MagicSchools[character.MagicSchoolFire].Mastery)
								} else {
									got = int(member.Skills[character.SkillSword].Mastery)
								}
							}
							want := mastery
							if allowed {
								want++
							}
							if got != want {
								t.Fatalf("mastery=%d, want %d", got, want)
							}
						})
					}
				}
			}
		}
	}
}

func TestLevelUpKeepsResourcesAcrossAwardsAndRosters(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := quests.GlobalQuestManager
	quests.GlobalQuestManager = nil
	t.Cleanup(func() { quests.GlobalQuestManager = previous })
	for _, tb := range []bool{false, true} {
		for _, source := range []string{"kill", "bound exit", "quest", "encounter"} {
			for _, xp := range []int{100, 600} {
				for _, full := range []bool{false, true} {
					t.Run(fmt.Sprintf("TB=%v/%s/xp=%d/full=%v", tb, source, xp, full), func(t *testing.T) {
						g := newTestGame(cfg, newTestWorld(cfg))
						g.combat = NewCombatSystem(g)
						g.turnBasedMode = tb
						original := g.party.Members
						active, reserve, captive, dead := original[0], original[1], original[2], original[3]
						g.party.Members = []*character.MMCharacter{active, dead}
						g.party.Reserve = []*character.MMCharacter{reserve}
						g.party.Captive = []*character.MMCharacter{captive}
						living := []*character.MMCharacter{active, reserve, captive}
						hp, sp, maxHP, maxSP := []int{}, []int{}, []int{}, []int{}
						for _, m := range append(living, dead) {
							m.Level = 1
							m.Experience = 0
							m.FreeStatPoints = 0
							m.Skills = map[character.SkillType]*character.Skill{}
							m.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
							m.CalculateDerivedStats(cfg)
							if !full {
								m.HitPoints = 1
								m.SpellPoints = 0
							}
							if m != dead {
								hp = append(hp, m.HitPoints)
								sp = append(sp, m.SpellPoints)
								maxHP = append(maxHP, m.MaxHitPoints)
								maxSP = append(maxSP, m.MaxSpellPoints)
							}
						}
						dead.HitPoints = 0
						switch source {
						case "kill", "bound exit":
							mob := mkTestMonster("xp contract", 1)
							mob.Experience = xp * len(g.party.Members)
							if source == "kill" {
								g.combat.awardExperienceAndGold(mob)
							} else {
								g.combat.awardExperienceOnly(mob)
							}
						case "quest":
							g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"test": {Name: "Test", Type: quests.QuestTypeKill, Rewards: quests.QuestRewards{Experience: xp}}}})
							if err := g.questManager.ActivateQuest("test"); err != nil {
								t.Fatal(err)
							}
							g.questManager.MarkCompleted("test")
							if !g.claimQuestReward("test") {
								t.Fatal("quest reward refused")
							}
						case "encounter":
							(&GameLoop{game: g}).awardEncounterRewards(&monster.EncounterRewards{Experience: xp})
						}
						wantLevel := 2
						if xp == 600 {
							wantLevel = 4
						}
						for i, m := range living {
							if m.Level != wantLevel || m.FreeStatPoints != (wantLevel-1)*StatPointsPerLevel {
								t.Fatalf("roster %d progression = L%d points %d", i, m.Level, m.FreeStatPoints)
							}
							if m.HitPoints != hp[i] || m.SpellPoints != sp[i] {
								t.Fatalf("roster %d refilled: HP/SP %d/%d, want %d/%d", i, m.HitPoints, m.SpellPoints, hp[i], sp[i])
							}
							if m.MaxHitPoints <= maxHP[i] || m.MaxSpellPoints <= maxSP[i] {
								t.Fatalf("roster %d lost max-stat growth", i)
							}
						}
						if dead.HitPoints != 0 || dead.Experience != 0 || dead.Level != 1 {
							t.Fatal("level award revived or trained a dead hero")
						}
					})
				}
			}
		}
	}
}

func TestRebalancedProgressionSurvivesSaveLoad(t *testing.T) {
	boot, qm := bootQuestGiverTest(t)
	g := newTestGame(boot.config, boot.world)
	g.questManager = qm
	g.combat = NewCombatSystem(g)
	w := g.world
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	previous := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	npc, err := character.CreateNPCFromConfig("nomad_city_trainer", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	g.dialogNPC = npc
	m := g.party.Members[0]
	m.Skills = map[character.SkillType]*character.Skill{character.SkillSword: {Mastery: character.MasteryMaster}}
	m.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
	m.Level = 1
	m.Experience = 100
	m.CalculateDerivedStats(g.config)
	m.HitPoints = 7
	m.SpellPoints = 3
	g.combat.checkLevelUp(m, false)
	for _, claimed := range []bool{false, true} {
		t.Run(fmt.Sprintf("claimed=%v", claimed), func(t *testing.T) {
			qm.Reset()
			if err := qm.ActivateQuest("pit_standing"); err != nil {
				t.Fatal(err)
			}
			qm.MarkCompleted("pit_standing")
			if claimed {
				if _, err := qm.ClaimRewards("pit_standing"); err != nil {
					t.Fatal(err)
				}
			}
			saved := g.buildSave(wm)
			if err := g.applySave(wm, &saved); err != nil {
				t.Fatal(err)
			}
			member := g.party.Members[0]
			if member.HitPoints != 7 || member.SpellPoints != 3 || member.Level != 2 {
				t.Fatal("save/load changed level-up resources")
			}
			if g.npcServiceGateOpen(npc) != claimed {
				t.Fatal("trainer quest gate changed after load")
			}
			options := trainerOptions(member, npc)
			if len(options) != 1 || options[0].Next != character.MasteryGrandMaster || options[0].Cost != 10000 {
				t.Fatalf("restored mastery offer=%v", options)
			}
		})
	}
}

func TestRebalancedEncounterRewardsSurviveSaveLoad(t *testing.T) {
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	boot, qm := bootQuestGiverTest(t)
	oldWM, oldQM := world.GlobalWorldManager, quests.GlobalQuestManager
	t.Cleanup(func() { world.GlobalWorldManager, quests.GlobalQuestManager = oldWM, oldQM })
	for _, tc := range []struct {
		id        string
		oldXP, xp int
	}{
		{"shipwreck_bandits", 500, 250},
		{"dragon_cliffs_bone_lair", 1500, 750},
		{"dragon_cliffs_ember_lair", 1500, 750},
		{"retired_legacy_encounter", 123, 123},
	} {
		for _, completed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/completed=%v", tc.id, completed), func(t *testing.T) {
				g := newTestGame(boot.config, newTestWorld(boot.config))
				g.combat, g.questManager = NewCombatSystem(g), qm
				qm.Reset()
				quests.GlobalQuestManager = qm
				wm := world.NewWorldManager(g.config)
				wm.CurrentMapKey = "forest"
				wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
				world.GlobalWorldManager = wm
				qm.CreateEncounterQuest(tc.id, "Old encounter", "Old description", 1, tc.oldXP)
				if completed {
					qm.CompleteEncounterQuest(tc.id)
				}
				mob := monster.NewMonster3DFromConfig(64, 64, "bandit", g.config)
				mob.IsEncounterMonster = true
				mob.EncounterRewards = &monster.EncounterRewards{QuestID: tc.id, Experience: tc.oldXP}
				g.world.Monsters = []*monster.Monster3D{mob}
				saved := g.buildSave(wm)
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				if len(g.world.Monsters) != 1 {
					t.Fatal("encounter monster missing after load")
				}
				rewards := g.world.Monsters[0].EncounterRewards
				if rewards == nil || rewards.Experience != tc.xp {
					t.Fatalf("restored pending reward = %+v, want XP %d", rewards, tc.xp)
				}
				encounter := character.NPCConfigInstance.EncounterByQuestID(tc.id)
				if encounter != nil {
					q := qm.GetQuest(tc.id)
					if q == nil || q.Definition.Rewards.Experience != tc.xp || q.Definition.Name != encounter.QuestName || q.RewardsClaimed != completed {
						t.Fatalf("encounter quest not reconstructed from content/progress: %+v", q)
					}
					if completed {
						if g.claimQuestReward(tc.id) {
							t.Fatal("claimed quest can be claimed twice after load")
						}
						return
					}
				}
				// Include XP spent on levels in the actual completion award.
				m := g.party.Members[0]
				m.Skills = map[character.SkillType]*character.Skill{}
				before := earnedExperienceForCharacter(m.Level, m.Experience)
				(&GameLoop{game: g}).awardEncounterRewards(rewards)
				if earnedExperienceForCharacter(m.Level, m.Experience)-before != tc.xp {
					t.Fatalf("awarded %d XP, want %d", earnedExperienceForCharacter(m.Level, m.Experience)-before, tc.xp)
				}
			})
		}
	}
}

func TestTowerRepeatAndRestoredMonstersKeepFullRebalancedXP(t *testing.T) {
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	cfg := loadTestConfig(t)
	for _, tc := range []struct {
		key string
		xp  int
	}{
		{"alien", 660}, {"dust_slime", 680}, {"grandfather_clock", 960},
		{"possessed_tome", 760}, {"alarm_clock", 1000},
	} {
		t.Run(tc.key, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorld(cfg))
			g.world.MonsterSpawns = []world.MonsterSpawn{{X: 1, Y: 1, MonsterKey: tc.key}}
			wm := world.NewWorldManager(cfg)
			wm.CurrentMapKey = "tower"
			wm.LoadedMaps = map[string]*world.World3D{"tower": g.world}
			for clear := 0; clear < 3; clear++ {
				g.world.RespawnAuthoredMonsters()
				if len(g.world.Monsters) != 1 || g.world.Monsters[0].Experience != tc.xp {
					t.Fatalf("clear %d changed full authored XP", clear)
				}
				saved := g.buildSave(wm)
				g.restoreSavedMonsters(wm, &saved)
				if len(g.world.Monsters) != 1 || g.world.Monsters[0].Experience != tc.xp {
					t.Fatalf("clear %d save/load changed authored XP", clear)
				}
			}
		})
	}
}

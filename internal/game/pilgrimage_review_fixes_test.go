package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"math"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Reusable content labels do not identify a placement. Sight retains the leash
// in both modes and only actual hits turn this instance into sticky retaliation.
func TestAuthoredGroupInstancesAndLeash(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, hit := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB%v/hit%v", tb, hit), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 80, 80)
				g.turnBasedMode = tb
				var peers []*monster.Monster3D
				for i, p := range [][2]int{{10, 10}, {10, 10}, {50, 50}, {50, 50}} {
					key := "bronze_gatekeeper"
					if i%2 == 1 {
						key = "gale_novice"
					}
					m := monster.NewMonster3DFromConfig((float64(p[0])+.5)*tile, (float64(p[1])+.5)*tile, key, g.config)
					m.ID = fmt.Sprint("member", i)
					peers = append(peers, m)
				}
				g.world.Monsters = peers
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				placePlayerAtTile(g, 14, 10, tile)
				// The same scheduler hook as production, including the TB standalone pass.
				if tb {
					runTBMonsterTurns(g, gl, 1)
				} else {
					gl.prepareMonsterFrame()
				}
				for i, m := range peers {
					if m.IsEngagingPlayer != (i < 2) || m.WasAttacked {
						t.Fatalf("sight leaked/latched: member%d %+v", i, m.IsEngagingPlayer)
					}
				}
				if peers[0].BandInstance != peers[1].BandInstance || peers[0].BandInstance == peers[2].BandInstance {
					t.Fatal("placement identities merged")
				}
				if hit {
					g.combat.applyMonsterDamagePacket(peers[0], singleMonsterDamagePacket(damagecalc.Parts{Normal: 1}, "physical", 0), monsterDamageOptions{})
				}
				placePlayerAtTile(g, 30, 30, tile)
				if tb {
					runTBMonsterTurns(g, gl, 1)
				} else {
					gl.prepareMonsterFrame()
				}
				for i, m := range peers {
					want := hit && i < 2
					if m.IsEngagingPlayer != want || m.WasAttacked != want {
						t.Fatalf("retreat state member%d: engaged%v attacked%v", i, m.IsEngagingPlayer, m.WasAttacked)
					}
				}
				wm := world.NewWorldManager(g.config)
				wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
				wm.CurrentMapKey = "forest"
				raw, err := json.Marshal(g.buildSave(wm))
				if err != nil {
					t.Fatal(err)
				}
				var save GameSave
				if err = json.Unmarshal(raw, &save); err != nil {
					t.Fatal(err)
				}
				loadedWorld := newTestWorldSized(g.config, 80, 80)
				wm.LoadedMaps["forest"] = loadedWorld
				loaded := newTestGame(g.config, loadedWorld)
				if err = loaded.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
				for _, m := range loadedWorld.Monsters {
					for _, old := range peers {
						if old.ID == m.ID && (old.BandInstance != m.BandInstance || old.WasAttacked != m.WasAttacked) {
							t.Fatal("save changed instance or hostility")
						}
					}
				}
			})
		}
	}
}

func TestRestrictedEquipmentAllEntryPoints(t *testing.T) {
	for _, class := range []character.CharacterClass{character.ClassMonk, character.ClassCleric, character.ClassKnight} {
		for _, key := range []string{"unbroken_robe", "unbroken_headband", "unbroken_sash", "unbroken_mantle"} {
			t.Run(fmt.Sprint(class, "/", key), func(t *testing.T) {
				cfg := loadTestConfig(t)
				g := newTestGame(cfg, newTestWorld(cfg))
				g.combat = NewCombatSystem(g)
				c := character.CreateCharacter("Wearer", class, cfg)
				g.party.Members = []*character.MMCharacter{c}
				item := items.CreateItemFromYAML(key)
				g.party.Inventory = []items.Item{item}
				ui := &UISystem{game: g, lastClickedItem: -1}
				want := class != character.ClassKnight
				if ui.canSelectedCharacterEquipInventoryItem(item) != want {
					t.Fatal("icon eligibility differs from model")
				}
				slot, _ := c.EquipDestination(item)
				if c.ItemFitsSlot(item, slot) != want {
					t.Fatal("drag gate differs")
				}
				for _, at := range []int64{1000, 1050} {
					g.mouseLeftClicks = []queuedClick{{x: 10, y: 10, at: at}}
					ui.handleInventoryItemClick(0, 0, 0, 20, 20)
				}
				if (len(g.party.Inventory) == 0) != want {
					t.Fatal("double-click disagrees with preview")
				}
				if !want && countCombatLog(g, "cannot wear") == 0 {
					t.Fatal("rejection was silent")
				}
			})
		}
	}
}

func reviewQuestGame(t *testing.T, defs map[string]*quests.QuestDefinition) *MMGame {
	t.Helper()
	g, _ := newSpecialsTestGame(t)
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: defs})
	return g
}

func TestChainedActivationUsesCommonLifecycle(t *testing.T) {
	for _, entry := range []string{"direct", "chain"} {
		for _, kind := range []string{"cleared", "spawn", "level"} {
			t.Run(entry+"/"+kind, func(t *testing.T) {
				next := &quests.QuestDefinition{Name: "Next", Type: "kill", TargetMonster: "bronze_gatekeeper", TargetCount: 1}
				if kind == "level" {
					next.MinPartyLevel = 99
				}
				if kind == "spawn" {
					next.OnAcceptSpawns = []quests.QuestSpawn{{ID: "trial", Map: "forest", X: 8, Y: 8, Monster: "bronze_gatekeeper"}}
				}
				prev := &quests.QuestDefinition{Name: "First", Type: "interact", TargetMonster: "token", TargetCount: 1, NextQuest: "next"}
				g := reviewQuestGame(t, map[string]*quests.QuestDefinition{"next": next, "prev": prev})
				if entry == "chain" {
					if err := g.questManager.ActivateQuest("prev"); err != nil {
						t.Fatal(err)
					}
					g.questManager.MarkCompleted("prev")
					got := g.claimQuestReward("prev")
					if got != (kind != "level") {
						t.Fatal("claim violated activation preflight")
					}
				} else {
					err := g.activateQuest("next")
					if (err == nil) != (kind != "level") {
						t.Fatal("activation level gate")
					}
				}
				q := g.questManager.GetQuest("next")
				if kind == "level" {
					if q != nil {
						t.Fatal("ineligible chapter activated")
					}
					return
				}
				if q == nil || q.Completed != (kind == "cleared") {
					t.Fatalf("census state %+v", q)
				}
				if kind == "spawn" && len(g.pendingQuestSpawns) != 1 {
					t.Fatal("activation omitted spawns")
				}
			})
		}
	}
}

func TestActivityCompletionUsesNPCAndCommonHook(t *testing.T) {
	def := &quests.QuestDefinition{Name: "Water", Type: "interact", TargetMonster: "valve", TargetCount: 1, AutoClaim: true, Activity: &quests.ActivityDefinition{Sequence: []string{"wheel"}}}
	g := reviewQuestGame(t, map[string]*quests.QuestDefinition{"water": def})
	attachTestProfile(t, g)
	if err := g.activateQuest("water"); err != nil {
		t.Fatal(err)
	}
	npc := &character.NPC{DialogueData: &character.NPCDialogue{VisitedMessage: "Water threads the channels."}}
	words := &character.NPCPropCopy{Tag: "valve", Token: "wheel", Took: "Turned.", Completed: "Flow restored."}
	for i := 0; i < 2; i++ {
		g.dialogNPC = npc
		(&InputHandler{game: g}).handleQuestPropInteract("water", words)
	}
	if countCombatLog(g, "Flow restored.") != 1 || countCombatLog(g, "Water threads the channels.") != 1 {
		t.Fatal("lost completion/revisit response")
	}
	if g.playerProfile.Data.Counters["quest_rewards"] != 1 {
		t.Fatal("activity bypassed common completion hook or repeated it")
	}
}

func TestQuestReferenceErrorsNameSource(t *testing.T) {
	for _, field := range []string{"items", "item_pool", "on_accept_spawns", "on_complete_spawns"} {
		t.Run(field, func(t *testing.T) {
			d := &quests.QuestDefinition{Name: "Bad"}
			switch field {
			case "items":
				d.Rewards.Items = []string{"missing-item"}
			case "item_pool":
				d.Rewards.ItemPool = []string{"missing-item"}
			case "on_accept_spawns":
				d.OnAcceptSpawns = []quests.QuestSpawn{{Monster: "missing-mob"}}
			case "on_complete_spawns":
				d.OnCompleteSpawns = []quests.QuestSpawn{{Monster: "missing-mob"}}
			}
			g := reviewQuestGame(t, map[string]*quests.QuestDefinition{"bad": d})
			err := g.validateQuestWorldReferences(g.questManager)
			if err == nil || !strings.Contains(err.Error(), field+"[0]") {
				t.Fatalf("wrong source: %v", err)
			}
		})
	}
}

func TestJournalRewardPreviewCache(t *testing.T) {
	loadTestConfig(t)
	ui := &UISystem{}
	first, ok := ui.journalRewardItem("unbroken_robe")
	if !ok {
		t.Fatal("missing preview")
	}
	allocations := testing.AllocsPerRun(20, func() { ui.journalRewardItem("unbroken_robe") })
	if allocations != 0 {
		t.Fatalf("cached lookup allocated %v", allocations)
	}
	old := config.GlobalItems
	t.Cleanup(func() { config.GlobalItems = old })
	replaced := *old
	replaced.Items = make(map[string]*config.ItemDefinitionConfig, len(old.Items))
	for k, v := range old.Items {
		replaced.Items[k] = v
	}
	def := *old.Items["unbroken_robe"]
	def.Name = "Revised Robe"
	replaced.Items["unbroken_robe"] = &def
	config.GlobalItems = &replaced
	fresh, ok := ui.journalRewardItem("unbroken_robe")
	if !ok || fresh.Name == first.Name {
		t.Fatal("catalog replacement retained old preview")
	}
}

func TestAuthoredProjectileStunPreserved(t *testing.T) {
	for _, key := range []string{"ancient_god_of_death", "alien_enforcer", "dragon_gold", "elder_dragon_gold", "gale_novice"} {
		for _, route := range []string{"party", "crossfire"} {
			t.Run(key+"/"+route, func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				// Fixture helpers load item/spell catalogs; load the monster catalog too.
				monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
				m := monster.NewMonster3DFromConfig(0, 0, key, g.config)
				d := monster.MonsterConfig.Monsters[key]
				rider := d.ProjectileStun(m.ProjectileSpell)
				if rider.Chance <= 0 {
					t.Fatal("authored projectile lost stun")
				}
				if d.StunCharChance > 0 && (rider.Chance != d.StunCharChance || rider.Seconds != d.StunCharSeconds || rider.Turns != d.StunCharTurns) {
					t.Fatal("explicit monster rider was replaced")
				}
				expected := fmt.Sprintf("%ds / %d turn(s)", rider.Seconds, rider.Turns)
				found := false
				for _, line := range d.CombatEffectLines() {
					if strings.Contains(line.Text, "Projectile stun") && strings.Contains(line.Text, expected) {
						found = true
					}
				}
				if !found {
					t.Fatal("tooltip disagrees with projectile rider")
				}
				m.DamageMin, m.DamageMax, m.TrueDamage = 1, 1, 0
				if m.StunCharChance > 0 {
					m.StunCharChance = 1
				} else {
					old := config.GlobalSpells.Spells[m.ProjectileSpell]
					copy := *old
					copy.StunChance = 1
					config.GlobalSpells.Spells[m.ProjectileSpell] = &copy
					t.Cleanup(func() { config.GlobalSpells.Spells[m.ProjectileSpell] = old })
				}
				cs.spawnMonsterSpellProjectile(m, spells.SpellID(m.ProjectileSpell), g.camera.X, g.camera.Y, ProjectileOwnerMonster)
				p := &g.magicProjectiles[0]
				p.IgnoresDodge = true
				var frames, turns int
				if route == "party" {
					g.party.Members = g.party.Members[:1]
					c := g.party.Members[0]
					c.HitPoints, c.MaxHitPoints = 10000, 10000
					p.X, p.Y = g.camera.X, g.camera.Y
					g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
					cs.CheckProjectilePlayerCollisions()
					frames, turns = c.StunFramesRemaining, c.StunTurnsRemaining
				} else {
					target := &monster.Monster3D{ID: "target", Name: "Target", HitPoints: 10000, MaxHitPoints: 10000}
					p.Owner = ProjectileOwnerMonsterAtBound
					cs.resolveMonsterProjectileVsMonster(p, "magic_projectile", target, p.ID)
					frames, turns = target.StunFramesRemaining, target.StunTurnsRemaining
				}
				if frames != rider.Seconds*g.config.GetTPS() || turns != rider.Turns {
					t.Fatalf("rider changed: %d frames/%d turns", frames, turns)
				}
			})
		}
	}
}

func TestChampionSpellSplashDoesNotInheritWeaponStun(t *testing.T) {
	for _, spellStun := range []bool{false, true} {
		t.Run(fmt.Sprint(spellStun), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			old := config.GlobalSpells.Spells["fireball"]
			d := *old
			d.StunChance = 0
			if spellStun {
				d.StunChance = 1
			}
			d.StunDurationSeconds, d.StunDurationTurns = 2, 1
			config.GlobalSpells.Spells["fireball"] = &d
			t.Cleanup(func() { config.GlobalSpells.Spells["fireball"] = old })
			source := &monster.Monster3D{ID: "source", Name: "Caster", ChampionKey: "test", HitPoints: 10000, StunCharChance: 1, StunCharSeconds: 9, StunCharTurns: 9, DamageMin: 1, DamageMax: 1}
			target := &monster.Monster3D{ID: "target", Name: "Target", Bound: true, HitPoints: 10000, MaxHitPoints: 10000, X: g.camera.X + 64, Y: g.camera.Y}
			g.world.Monsters = []*monster.Monster3D{source, target}
			g.party.Members = g.party.Members[:1]
			c := g.party.Members[0]
			c.HitPoints, c.MaxHitPoints = 10000, 10000
			cs.spawnMonsterSpellProjectile(source, "fireball", target.X, target.Y, ProjectileOwnerMonsterAtBound)
			p := &g.magicProjectiles[0]
			p.IgnoresDodge = true
			cs.resolveMonsterProjectileVsMonster(p, "magic_projectile", target, p.ID)
			want := 0
			if spellStun {
				want = 2 * g.config.GetTPS()
			}
			if c.StunFramesRemaining != want {
				t.Fatalf("party splash stun=%d want%d", c.StunFramesRemaining, want)
			}
		})
	}
}

func TestActivitySpriteVisibleHeightUsesCurrentPhase(t *testing.T) {
	g, _ := newSpecialsTestGame(t)
	t.Chdir("../..")
	g.sprites = graphics.NewSpriteManager()
	g.renderHelper = NewRenderingHelper(g)
	def := &quests.QuestDefinition{Name: "Flowers", Type: "interact", TargetMonster: "flower", TargetCount: 1, Activity: &quests.ActivityDefinition{Forage: []quests.ForageGroup{{Phase: "day", Count: 1, Tokens: []string{"sun"}}}}}
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"flowers": def}})
	if err := g.questManager.ActivateQuest("flowers"); err != nil {
		t.Fatal(err)
	}
	npc := &character.NPC{Sprite: "pilgrim_sunlotus", RenderCategory: "scenery", SizeClass: "small", DialogueData: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{{QuestID: "flowers", Prop: &character.NPCPropCopy{Token: "sun", DormantSprite: "pilgrim_flower_bud"}}}}}
	target, _ := config.ResolveSizeClassTiles(g.config.Graphics.SizeClasses, npc.SizeClass)
	for _, night := range []bool{false, true} {
		g.dayNightIsNight = night
		name := g.activityNPCSprite(npc)
		want := "pilgrim_sunlotus"
		if night {
			want = "pilgrim_flower_bud"
		}
		if name != want {
			t.Fatalf("phase sprite=%s", name)
		}
		bounds, _, height, known := g.sprites.SpriteVisibleFrameBounds(name)
		if !known || bounds.Dy() == 0 {
			t.Fatal("missing real sprite bounds")
		}
		visible := g.renderHelper.npcSizeTiles(npc) * float64(bounds.Dy()) / float64(height)
		if math.Abs(visible-target) > 1e-9 {
			t.Fatalf("%s visible height=%f want%f", name, visible, target)
		}
	}
}

// Case table: plain/sequence/forage x root/nested x each required copy field
// missing/blank/valid. Activity props also traverse the producer-validation
// entry point. This is a load-time rule; no persistent state changes.
func TestQuestPropCopyValidation(t *testing.T) {
	oldNPCs, oldWorld, oldMonsters := character.NPCConfigInstance, world.GlobalWorldManager, monster.MonsterConfig
	t.Cleanup(func() {
		character.NPCConfigInstance, world.GlobalWorldManager, monster.MonsterConfig = oldNPCs, oldWorld, oldMonsters
	})
	world.GlobalWorldManager, monster.MonsterConfig = nil, nil
	for _, kind := range []string{"plain", "sequence", "forage"} {
		for _, nested := range []bool{false, true} {
			for _, field := range []string{"valid", "not_yet", "took", "completed"} {
				for _, blank := range []string{"", " \t\n"} {
					if field == "valid" && blank != "" {
						continue
					}
					for _, entry := range []string{"references", "producers"} {
						if kind == "plain" && entry == "producers" {
							continue // Ordinary prop copy is owned by reference validation.
						}
						t.Run(fmt.Sprintf("%s/nested=%v/%s/whitespace=%v/%s", kind, nested, field, blank != "", entry), func(t *testing.T) {
							def := &quests.QuestDefinition{Name: "Trial", Type: quests.QuestTypeInteract, TargetMonster: "trial", TargetCount: 1}
							p := &character.NPCPropCopy{Tag: "trial", NotYet: "Wait.", Took: "Done.", Completed: "The trial is complete."}
							switch kind {
							case "sequence":
								def.Activity = &quests.ActivityDefinition{Sequence: []string{"wheel"}}
								p.Token = "wheel"
							case "forage":
								def.Activity = &quests.ActivityDefinition{Forage: []quests.ForageGroup{{Phase: "day", Count: 1, Tokens: []string{"flower"}}}}
								p.Token = "flower"
							}
							switch field {
							case "not_yet":
								p.NotYet = blank
							case "took":
								p.Took = blank
							case "completed":
								p.Completed = blank
							}
							choice := &character.NPCDialogueChoice{Action: questPropAction, QuestID: "trial", Prop: p}
							if nested {
								choice = &character.NPCDialogueChoice{Action: "info", Choices: []*character.NPCDialogueChoice{choice}}
							}
							character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
								"trial_prop": {HideWhenVisited: true, Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{choice}}},
							}}
							qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"trial": def}})
							if kind != "plain" {
								npc := &character.NPC{DialogueData: character.NPCConfigInstance.NPCs["trial_prop"].Dialogue}
								if id, words := activityProp(npc); id != "trial" || words != p {
									t.Fatal("runtime activity lookup differs from validation traversal")
								}
							}
							var err error
							if entry == "references" {
								err = (&MMGame{}).validateQuestWorldReferences(qm)
							} else {
								err = ValidateInteractTagProducers(qm)
							}
							if field == "valid" {
								if err != nil {
									t.Fatal(err)
								}
							} else if err == nil || !strings.Contains(err.Error(), "trial_prop") || !strings.Contains(err.Error(), "prop."+field) {
								t.Fatalf("expected NPC and prop.%s copy error, got %v", field, err)
							}
						})
					}
				}
			}
		}
	}
}

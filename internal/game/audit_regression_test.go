package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/arena"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

// Audit case table:
// - Encounter release true/false x loaded combat/departure; queued kills x travel.
// - Ground bag/chest x item/stack x repeated old-save load; stash split x empty/merge.
// - Promotion Light/Dark x current/legacy save; skip state x load/new game.
// - Pack active/remote x controlled/hostile; fast travel pose/portal x RT/TB.
// - Score missing/corrupt/unreadable x record; phase newer/equal/older x run/tier.
// - Input move+turn, held trainer input, consumed Escape through a modal.
// Terrain uses a separate split/stitched save-load table below. Persistence is
// exercised for deferred state; immediate input/combat has no saved state.
func auditSaveJSON(t *testing.T, s GameSave) GameSave {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var out GameSave
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAuditEncounterSaveAndDeparture(t *testing.T) {
	for _, frees := range []bool{false, true} {
		for _, entry := range []string{"kill_after_load", "bound_departure", "queued_death_departure"} {
			t.Run(fmt.Sprintf("free%v/%s", frees, entry), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				captive := *g.party.Members[0]
				captive.Name = "Audit Captive"
				g.party.Captive = []*character.MMCharacter{&captive}
				g.stash = &stash.Stash{}
				m := monster.NewMonster3DFromConfig(12.5*ts, 12.5*ts, "bandit", g.config)
				m.NoKillRewards = true
				m.IsEncounterMonster = true
				m.EncounterRewards = &monster.EncounterRewards{Gold: 7, FreesCaptives: frees}
				g.world.Monsters = []*monster.Monster3D{m}
				saved := auditSaveJSON(t, g.buildSave(wm))
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				m = g.world.Monsters[0]
				m.NoKillRewards = true
				before := g.party.Gold
				oldWorld := g.world
				switch entry {
				case "bound_departure":
					m.Bound = true
				default:
					m.HitPoints = 0
					g.combat.finishMonsterKill(m)
				}
				if entry == "kill_after_load" {
					g.flushDepartureDeaths()
				} else if err := g.transitionToMap(mapTransition{mapKey: "other", arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				if g.party.Gold != before+7 {
					t.Fatalf("encounter reward lost or repeated: %d -> %d", before, g.party.Gold)
				}
				if (len(g.party.Captive) == 0) != frees {
					t.Fatal("loaded encounter lost captive-release semantics")
				}
				if len(oldWorld.Monsters) != 0 {
					t.Fatal("departing roster retained a rewarded actor")
				}
				if err := g.transitionToMap(mapTransition{mapKey: "forest", arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				g.flushDepartureDeaths()
				if g.party.Gold != before+7 {
					t.Fatal("return to map awarded the same encounter twice")
				}
			})
		}
	}
}

func TestAuditGroundStashReconciliation(t *testing.T) {
	for _, kind := range []ContainerKind{0, 1} {
		for _, stack := range []bool{false, true} {
			t.Run(fmt.Sprintf("kind%d/stack%v", kind, stack), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				item := items.Item{Name: "Audit Sword", Type: items.ItemWeapon, InstanceID: 991}
				if stack {
					item = items.Item{Name: "Health Potion", Type: items.ItemConsumable, InstanceID: 991, Quantity: 5}
				}
				g.groundContainers = []GroundContainer{{Kind: kind, ID: "audit", MapKey: "forest", X: 12.5 * ts, Y: 12.5 * ts, Items: []items.Item{item}}}
				saved := auditSaveJSON(t, g.buildSave(wm))
				g.stash = &stash.Stash{}
				g.stash.Slots[0] = item
				if stack {
					g.stash.Slots[0].Quantity = 2
				}
				for load := 0; load < 3; load++ {
					source := auditSaveJSON(t, saved)
					if err := g.applySave(wm, &source); err != nil {
						t.Fatal(err)
					}
					remaining := g.groundContainers[0].Items
					if !stack && len(remaining) != 0 {
						t.Fatal("ground container duplicated a stash-owned item")
					}
					if stack && (len(remaining) != 1 || remaining[0].Count() != 3 || remaining[0].InstanceID == 991) {
						t.Fatalf("wrong surviving stack: %+v", remaining)
					}
				}
			})
		}
	}
}

func TestAuditInternalStashSplit(t *testing.T) {
	for _, merge := range []bool{false, true} {
		t.Run(fmt.Sprint(merge), func(t *testing.T) {
			g := stashTestGame(t)
			item := items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 10, InstanceID: 771}
			g.stash.Slots[0] = item
			if merge {
				g.stash.Slots[1] = items.Item{Name: item.Name, Type: item.Type, Quantity: 2, InstanceID: 772}
			}
			g.stashDragFrom = 0
			g.stashDragItem = item
			g.stashDragSplitQuantity = 9
			g.resolveStashDrop(stashAddr{stashKindChest, 1})
			g.party.Inventory = []items.Item{item}
			g.reconcilePartyAgainstStash()
			if len(g.party.Inventory) != 0 {
				t.Fatalf("internal split lost stash ownership: %+v", g.party.Inventory)
			}
		})
	}
}

func TestAuditPromotionSave(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("light%v/legacy%v", light, legacy), func(t *testing.T) {
				g, wm, _ := travelFixture(t)
				idx := sorcererIndex(t, g)
				school := character.MagicSchoolDark
				if light {
					school = character.MagicSchoolLight
					g.applyArchmagePromotion(idx)
				} else {
					g.applyLichPromotion(idx)
				}
				g.toggleLevelUpSelection(0)
				saved := auditSaveJSON(t, g.buildSave(wm))
				if legacy {
					saved.PendingLevelUpChoices = []PendingLevelUpChoiceSave{{CharIndex: idx}}
				}
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				if len(g.levelUpChoiceQueue) != 1 {
					t.Fatalf("lost promotion choices: %+v", g.levelUpChoiceQueue)
				}
				req := &g.levelUpChoiceQueue[0]
				if req.padToMinimum || req.maxSelections != 2 {
					t.Fatal("promotion restored as mastery choices")
				}
				for _, option := range req.options {
					if option.choice.Type != "spell" {
						t.Fatal("unearned mastery option")
					}
				}
				if !legacy && !req.selected[0] {
					t.Fatal("pending selection lost")
				}
				g.openLevelUpChoiceForChar(idx)
				if legacy {
					g.toggleLevelUpSelection(0)
				}
				g.toggleLevelUpSelection(1)
				g.confirmLevelUpSelections()
				if len(g.party.Members[idx].MagicSchools[school].KnownSpells) != 2 {
					t.Fatal("promotion did not grant its two spells")
				}
			})
		}
	}
}

func TestAuditTimelineReset(t *testing.T) {
	for _, entry := range []string{"load", "new_game"} {
		t.Run(entry, func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			saved := auditSaveJSON(t, g.buildSave(wm))
			g.dayNightSkipActive = true
			g.dayNightSkipPhases = []bool{true, false}
			g.dayNightSkipTargetFrame = 999
			g.skyFadeFrames, g.skyFadeTotal = 30, 60
			g.victoryScoreSaved = true
			g.victoryNameInput = "Old Run"
			g.victoryTime = time.Now()
			if entry == "load" {
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
			} else {
				g.startNewGameWithParty(character.NewParty(g.config))
			}
			if g.dayNightSkipActive || len(g.dayNightSkipPhases) > 0 || g.dayNightSkipTargetFrame != 0 || g.skyFadeFrames != 0 || g.skyFadeTotal != 0 {
				t.Fatal("old paid skip survived timeline replacement")
			}
			if g.victoryScoreSaved || g.victoryNameInput != "" || !g.victoryTime.IsZero() {
				t.Fatal("old victory submission survived timeline replacement")
			}
		})
	}
}

func TestAuditSeasonalAllies(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprint(remote), func(t *testing.T) {
			g, wm, ts := travelFixture(t)
			w := g.world
			if remote {
				w = wm.LoadedMaps["other"]
			}
			ally := monster.NewMonster3DFromConfig(5.5*ts, 5.5*ts, "bandit", g.config)
			ally.Bound = true
			ally.PackKey = "seasonal"
			foe := monster.NewMonster3DFromConfig(6.5*ts, 5.5*ts, "bandit", g.config)
			foe.PackKey = "seasonal"
			w.Monsters = []*monster.Monster3D{ally}
			if worldHasLivingMonstersInRect(w, 0, 0, 40, 40, ts) {
				t.Fatal("ally blocks map-clear pack gate")
			}
			w.Monsters = append(w.Monsters, foe)
			if !worldHasLivingMonstersInRect(w, 0, 0, 40, 40, ts) {
				t.Fatal("hostile fails to block clear gate")
			}
			g.despawnPackMonsters(w, "seasonal")
			if !remote {
				g.flushDepartureDeaths()
			}
			if len(w.Monsters) != 1 || w.Monsters[0] != ally {
				t.Fatal("season change removed bound ally")
			}
		})
	}
}

func TestAuditSameWorldTravelAllies(t *testing.T) {
	for _, portal := range []bool{false, true} {
		for _, turn := range []bool{false, true} {
			t.Run(fmt.Sprintf("portal%v/turn%v", portal, turn), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				wm.LoadedMaps["other"] = g.world
				g.world.StartX, g.world.StartY = 20, 20
				ally := monster.NewMonster3DFromConfig(3.5*ts, 3.5*ts, "bandit", g.config)
				ally.Bound = true
				g.world.Monsters = []*monster.Monster3D{ally}
				g.turnBasedMode = turn
				if portal {
					g.townPortalTeleport("other")
				} else if err := g.transitionToMap(mapTransition{mapKey: "other", pose: MapPose{X: 20.5 * ts, Y: 20.5 * ts}}); err != nil {
					t.Fatal(err)
				}
				if !ally.IsAlive() || !ally.Bound || Distance(ally.X, ally.Y, g.camera.X, g.camera.Y) > 3*ts {
					t.Fatal("same-world fast travel stranded its ally")
				}
				saved := readTravelAutosave(t)
				if len(saved.MapMonsters["other"])+len(saved.MapMonsters["forest"]) == 0 {
					t.Fatal("travel lost its ally in the autosave")
				}
			})
		}
	}
}

func TestAuditDeadMeleeTarget(t *testing.T) {
	g, _, ts := travelFixture(t)
	m := monster.NewMonster3DFromConfig(5.5*ts, 5.5*ts, "bandit", g.config)
	m.HitPoints = 0
	g.world.Monsters = []*monster.Monster3D{m}
	beforeGold, beforeXP := g.party.Gold, g.totalExperienceEarned
	g.combat.ApplyDamageToMonster(m, 100, "Ember Egg", false)
	if len(g.combatLogHistory) > 0 || len(g.deadMonsterIDs) > 0 || beforeGold != g.party.Gold || beforeXP != g.totalExperienceEarned {
		t.Fatal("melee committed a second hit against a corpse")
	}
}

func TestAuditInputPressOwnership(t *testing.T) {
	t.Run("step_and_turn", func(t *testing.T) {
		g, _, ts := travelFixture(t)
		g.turnBasedMode = true
		g.currentTurn = 0
		g.camera.X, g.camera.Y = 10.5*ts, 10.5*ts
		g.snapFacing(0)
		for _, m := range g.party.Members {
			m.ActionsRemaining = 1
		}
		ih := NewInputHandler(g)
		ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyW || k == ebiten.KeyA })
		ih.handleTurnBasedInput()
		if g.currentTurn == 0 {
			t.Fatal("combined step and rotation did not spend the turn")
		}
	})
	t.Run("consumed_escape", func(t *testing.T) {
		g, _, _ := travelFixture(t)
		g.menuOpen = true
		ih := NewInputHandler(g)
		ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyEscape })
		if !ih.keys.Consume(ebiten.KeyEscape) {
			t.Fatal("fixture did not press Escape")
		}
		ih.handleTabbedMenuInput()
		if !g.menuOpen {
			t.Fatal("same Escape press closed parent menu")
		}
		ih.keys.BeginFrame()
		ih.handleTabbedMenuInput()
		if g.menuOpen {
			t.Fatal("fresh Escape did not close parent menu")
		}
	})
}

func TestAuditCorruptBoardsPreserved(t *testing.T) {
	for _, name := range []string{"highscores.json", "arena_leaderboard.json"} {
		for _, kind := range []string{"invalid_json", "unreadable"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				g, _, _ := travelFixture(t)
				path := storage.AppSavePath(name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if kind == "unreadable" {
					if err := os.Mkdir(path, 0755); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte("broken board"), 0600); err != nil {
					t.Fatal(err)
				}
				if name == "highscores.json" {
					(&InputHandler{game: g}).saveVictoryScore()
					if g.victoryScoreSaved {
						t.Fatal("failed score write marked saved")
					}
				} else if arena.RecordVictory("audit", nil, "champion", "easy", 10, 1) {
					t.Fatal("recorded over unreadable arena board")
				}
				if kind == "invalid_json" {
					got, err := os.ReadFile(path)
					if err != nil || string(got) != "broken board" {
						t.Fatal("corrupt board overwritten")
					}
				}
			})
		}
	}
}

func TestAuditArenaPhaseHighWater(t *testing.T) {
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, tc := range []struct {
		phase int
		want  bool
	}{{1, true}, {4, true}, {1, false}, {4, false}, {3, false}, {5, true}} {
		if got := arena.RecordVictory("audit", nil, "champion", "easy", 10, tc.phase); got != tc.want {
			t.Fatalf("phase %d accepted=%v", tc.phase, got)
		}
	}
	if arena.Load().Entries[0].TotalKills() != 3 {
		t.Fatal("older saves farmed arena board")
	}
}

func TestAuditTrapItemNormalization(t *testing.T) {
	cfg := loadTestConfig(t)
	for key := range config.GlobalTrapConfig.Traps {
		trap, ok := config.TrapItem(key)
		if !ok {
			continue
		}
		trap.InstanceID = 823
		member := character.NewParty(cfg).Members[0]
		member.QuickSlots[0] = &trap
		for i := 0; i < 3; i++ {
			normalizeItemFromConfig(member.QuickSlots[0])
			if items.EnsureInstanceID(member.QuickSlots[0]) || member.QuickSlots[0].InstanceID != 823 {
				t.Fatal("trap normalization erased persistent identity")
			}
		}
	}
}

func TestAuditArchmageRumorRetiresAcrossRosters(t *testing.T) {
	for _, roster := range []string{"active", "reserve", "captive"} {
		t.Run(roster, func(t *testing.T) {
			g, _, _ := travelFixture(t)
			m := g.party.Members[0]
			m.Promotion = character.PromotionArchmage
			if roster != "active" {
				g.party.Members = g.party.Members[1:]
				if roster == "reserve" {
					g.party.Reserve = []*character.MMCharacter{m}
				} else {
					g.party.Captive = []*character.MMCharacter{m}
				}
			}
			if g.rumorAvailable(RumorDef{Quest: "archmage_trial"}) {
				t.Fatal("completed promotion advertised its trial again")
			}
		})
	}
}

func TestAuditTrainerPresses(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyEnter} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			g, _, _ := travelFixture(t)
			g.dialogNPC = &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer, Training: map[string]int{"expert": 1000, "master": 4000, "grandmaster": 10000}}
			g.selectedCharIdx = 0
			g.party.Gold = 100000
			options := trainerOptions(g.party.Members[0], g.dialogNPC)
			if len(options) < 2 {
				t.Fatal("fixture needs multiple training options")
			}
			pressed := true
			ih := NewInputHandler(g)
			ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return pressed && k == key })
			ih.handleSkillTrainerInput()
			if key == ebiten.KeyEnter {
				if g.party.Gold != 100000-options[0].Cost {
					t.Fatal("press did not buy exactly one training")
				}
			} else if g.dialogSelectedSpell == 0 {
				t.Fatal("press did not move selection")
			}
			gold, selection := g.party.Gold, g.dialogSelectedSpell
			pressed = false
			for frame := 0; frame < 20; frame++ {
				ih.keys.BeginFrame()
				g.spellInputCooldown = 0
				ih.handleSkillTrainerInput()
			}
			if g.party.Gold != gold || g.dialogSelectedSpell != selection {
				t.Fatal("held key repeated training or navigation")
			}
		})
	}
}

func TestAuditDisintegratePreservesSplash(t *testing.T) {
	for _, entry := range []string{"melee", "arrow", "spell"} {
		for _, immune := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/immune%v", entry, immune), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				ts := float64(cs.game.config.GetTileSize())
				main := newRacialTarget("Primary", "beast")
				main.ID = "primary"
				if immune {
					main.MonsterType = "undead"
				}
				near := newRacialTarget("Nearby", "beast")
				near.ID = "near"
				near.X = ts
				far := newRacialTarget("Outside", "beast")
				far.ID = "far"
				far.X = 4 * ts
				cs.game.world.Monsters = []*monster.Monster3D{main, near, far}
				def, _ := config.GetWeaponDefinition("tonbogiri")
				old := *def
				t.Cleanup(func() { *def = old })
				def.DisintegrateChance = 1
				switch entry {
				case "melee":
					cs.ApplyDamageToMonster(main, 20, def.Name, false)
				case "arrow":
					cs.applyProjectileDamage(&Arrow{Active: true, LifeTime: 1, Damage: 20, DisintegrateChance: 1, BowKey: "tonbogiri", Owner: ProjectileOwnerPlayer}, "arrow", main, main.ID)
				case "spell":
					cs.applyProjectileDamage(&MagicProjectile{Active: true, LifeTime: 1, Damage: 20, DisintegrateChance: 1, SpellType: "fireball"}, "magic_projectile", main, main.ID)
				}
				if main.IsAlive() != immune {
					t.Fatalf("primary alive=%v immune=%v", main.IsAlive(), immune)
				}
				if near.HitPoints >= near.MaxHitPoints || far.HitPoints != far.MaxHitPoints {
					t.Fatal("instakill changed ordinary splash coverage")
				}
			})
		}
	}
}

func TestAuditTrapBinding(t *testing.T) {
	for _, key := range []string{"bear_trap", "cleave_trap", "stasis_trap"} {
		for _, immune := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/immune%v", key, immune), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				forceRacialProc(cs, t)
				owner := cs.game.party.Members[0]
				owner.Race = "dark_elf"
				main := newRacialTarget("Primary", "beast")
				if immune {
					main.MonsterType = "undead"
				}
				near := newRacialTarget("Nearby", "beast")
				near.X = float64(cs.game.config.GetTileSize())
				cs.game.world.Monsters = []*monster.Monster3D{main, near}
				cs.fireTrap(&PlacedTrap{Key: key, Owner: owner}, main)
				if main.Bound == immune {
					t.Fatal("trap bypassed racial binding eligibility")
				}
				if !immune && (main.HitPoints != main.MaxHitPoints || main.StunTurnsRemaining != 0 || main.RootTurnsRemaining != 0) {
					t.Fatal("binding also applied trap damage/control")
				}
				if key == "cleave_trap" && (!near.Bound || near.HitPoints != near.MaxHitPoints) {
					t.Fatal("trap splash skipped binding")
				}
			})
		}
	}
}

func TestAuditTerrainTimeline(t *testing.T) {
	for _, from := range []bool{false, true} {
		for _, to := range []bool{false, true} {
			t.Run(fmt.Sprintf("stitched%v_to_%v", from, to), func(t *testing.T) {
				t.Chdir("../..")
				storage.SetDataRootForTesting(t.TempDir())
				t.Cleanup(func() { storage.SetDataRootForTesting("") })
				g, wm, cfg := bootOpenWorldGame(t, from)
				// Locate an authored natural prop in Forest, through its real projection.
				x, y := -1, -1
				w := wm.WorldByKey("forest")
				for ty, row := range w.Tiles {
					for tx, tile := range row {
						if tileIsNaturalCross(tile) && g.mapKeyAtTile(tx, ty) == "forest" {
							x, y = tx, ty
							break
						}
					}
					if x >= 0 {
						break
					}
				}
				if x < 0 {
					t.Fatal("no natural Forest prop")
				}
				lx, ly := wm.LocalizeTile("forest", x, y)
				before := w.Tiles[y][x]
				g.combat = NewCombatSystem(g)
				g.combat.topplePropsInRadius((float64(x)+.5)*cfg.GetTileSize(), (float64(y)+.5)*cfg.GetTileSize(), 1, 1)
				after := w.Tiles[y][x]
				if before == after || len(g.terrainChanges) != 1 {
					t.Fatal("Earthquake did not record its changed tile")
				}
				g.setPartyPosition((float64(x)+.5)*cfg.GetTileSize(), (float64(y)+.5)*cfg.GetTileSize())
				saved := auditSaveJSON(t, g.buildSave(wm))
				g2, wm2, _ := bootOpenWorldGame(t, to)
				g2.gameLoop = nil
				if err := g2.applySave(wm2, &saved); err != nil {
					t.Fatal(err)
				}
				px, py := wm2.ProjectTile("forest", lx, ly)
				w2 := wm2.WorldByKey("forest")
				if w2.Tiles[py][px] != after {
					t.Fatal("saved Earthquake terrain was not restored")
				}
				if TileIndex(g2.camera.X, cfg.GetTileSize()) != px || TileIndex(g2.camera.Y, cfg.GetTileSize()) != py {
					t.Fatal("restored party displaced from its saved cleared tile")
				}
				clean := auditSaveJSON(t, saved)
				clean.TerrainChanges = nil
				if err := g2.applySave(wm2, &clean); err != nil {
					t.Fatal(err)
				}
				if w2.Tiles[py][px] != before {
					t.Fatal("older save retained later terrain destruction")
				}
				if err := g2.applySave(wm2, &saved); err != nil {
					t.Fatal(err)
				}
				g2.startNewGameWithParty(character.NewParty(g2.config))
				if len(g2.terrainChanges) != 0 || wm2.WorldByKey("forest").Tiles[py][px] != before {
					t.Fatal("new game inherited terrain destruction")
				}
			})
		}
	}
}

func TestAuditPendingChoicesRoundTrip(t *testing.T) {
	for _, kind := range []string{"random_mastery", "benched_archmage", "benched_lich"} {
		t.Run(kind, func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			idx := sorcererIndex(t, g)
			if kind == "random_mastery" {
				g.queueLevelUpChoices(g.party.Members[idx], 6, nil)
			} else {
				if kind == "benched_archmage" {
					g.applyArchmagePromotion(idx)
				} else {
					g.applyLichPromotion(idx)
				}
				if len(g.party.Reserve) == 0 {
					copy := *g.party.Members[0]
					g.party.Reserve = append(g.party.Reserve, &copy)
				}
				if !g.swapRosterMember(idx, 0) {
					t.Fatal("could not bench promoted hero")
				}
			}
			saved := auditSaveJSON(t, g.buildSave(wm))
			if kind == "random_mastery" && (len(saved.PendingLevelUpChoices) != 1 || len(saved.PendingLevelUpChoices[0].Options) != len(g.levelUpChoiceQueue[0].options)) {
				t.Fatal("snapshot omitted the offered mastery choices")
			}
			if err := g.applySave(wm, &saved); err != nil {
				t.Fatal(err)
			}
			if kind == "random_mastery" {
				got := g.buildSave(wm).PendingLevelUpChoices
				if len(got) != 1 || !reflect.DeepEqual(got[0].Options, saved.PendingLevelUpChoices[0].Options) {
					t.Fatal("reload rerolled earned mastery choices")
				}
			} else {
				if !g.swapRosterMember(idx, 0) {
					t.Fatal("could not restore promoted hero")
				}
				if len(g.levelUpChoiceQueue) != 1 || g.levelUpChoiceQueue[0].padToMinimum || g.levelUpChoiceQueue[0].maxSelections != 2 {
					t.Fatal("bench reload converted promotion into mastery")
				}
			}
		})
	}
}

func TestAuditTrapQuickslotFileReload(t *testing.T) {
	g, _, _ := travelFixture(t)
	item, ok := config.TrapItem("bear_trap")
	if !ok {
		t.Fatal("missing trap")
	}
	item.InstanceID = 987
	g.party.Members[0].QuickSlots[0] = &item
	path := filepath.Join(t.TempDir(), "save.json")
	if err := g.SaveGameToFile(path); err != nil {
		t.Fatal(err)
	}
	// Complete any unrelated legacy roster migrations, then pin an old timestamp.
	if err := g.LoadGameFromFile(path); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1000000000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := g.LoadGameFromFile(path); err != nil {
			t.Fatal(err)
		}
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !stat.ModTime().Equal(stamp) || g.party.Members[0].QuickSlots[0].InstanceID != 987 {
		t.Fatalf("ordinary quickslot reload rewrote the save or its identity: time=%v id=%d", stat.ModTime(), g.party.Members[0].QuickSlots[0].InstanceID)
	}
}

func TestAuditMeleeSwingSkipsSplashCorpses(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(fmt.Sprint(turn), func(t *testing.T) {
			g, cs, ts := rtMeleeGame(t)
			g.turnBasedMode = turn
			g.camera.X, g.camera.Y, g.camera.Angle = 10.5*ts, 10.5*ts, 0
			primary := tankMobAt(g, 11.5*ts, 10.5*ts)
			primary.HitPoints = 1
			secondary := tankMobAt(g, 11.5*ts, 11.5*ts)
			secondary.HitPoints = 1
			setMobs(g, primary, secondary)
			weapon := items.CreateWeaponFromYAML("tonbogiri")
			hits := cs.performMeleeHitDetection(weapon, 100, &config.MeleeAttackConfig{ArcType: 4}, false)
			if primary.IsAlive() || secondary.IsAlive() || hits != 1 {
				t.Fatalf("swing hit splash corpse: hits=%d alive=%v/%v", hits, primary.IsAlive(), secondary.IsAlive())
			}
		})
	}
}

func TestAuditArchmageTurnInPersistsConclusion(t *testing.T) {
	g, wm, _ := travelFixture(t)
	t.Chdir("../..")
	g.sprites = graphics.NewSpriteManager()
	hero := g.party.Members[sorcererIndex(t, g)]
	g.party.Members = []*character.MMCharacter{hero}
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"archmage_trial": {Type: quests.QuestTypeKill, TargetCount: 1}}})
	if err := g.questManager.ActivateQuest("archmage_trial"); err != nil {
		t.Fatal(err)
	}
	g.questManager.MarkCompleted("archmage_trial")
	(&InputHandler{game: g}).handleTurnInQuest("archmage_trial")
	q := g.questManager.GetQuest("archmage_trial")
	if q == nil || !q.RewardsClaimed || !hero.IsArchmage() {
		t.Fatal("turn-in discarded its completed journal record")
	}
	saved := auditSaveJSON(t, g.buildSave(wm))
	if err := g.applySave(wm, &saved); err != nil {
		t.Fatal(err)
	}
	if g.rumorAvailable(RumorDef{Quest: "archmage_trial"}) {
		t.Fatal("reload re-advertised a concluded promotion")
	}
}

func TestAuditEncounterDepartureWaitsForRemainingEnemies(t *testing.T) {
	g, _, ts := travelFixture(t)
	rewards := &monster.EncounterRewards{Gold: 7}
	bound := monster.NewMonster3DFromConfig(12.5*ts, 12.5*ts, "bandit", g.config)
	foe := monster.NewMonster3DFromConfig(13.5*ts, 12.5*ts, "bandit", g.config)
	for _, m := range []*monster.Monster3D{bound, foe} {
		m.IsEncounterMonster = true
		m.EncounterRewards = rewards
		m.NoKillRewards = true
	}
	bound.Bound = true
	g.world.Monsters = []*monster.Monster3D{bound, foe}
	before := g.party.Gold
	if err := g.transitionToMap(mapTransition{mapKey: "other", arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	if g.party.Gold != before {
		t.Fatal("encounter paid while an enemy remained")
	}
	if err := g.transitionToMap(mapTransition{mapKey: "forest", arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	foe.HitPoints = 0
	g.flushDepartureDeaths()
	if g.party.Gold != before+7 {
		t.Fatal("remaining enemy did not resolve encounter")
	}
}

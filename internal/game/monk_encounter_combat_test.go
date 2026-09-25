package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Drive RT interaction and the real TB monster pass, including cooldown and CC.
func TestMonkEncounterAbilities(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"root", "blink"} {
			for _, gate := range []string{"proc", "no_proc", "stun", "blocked", "rooted", "occupied"} {
				if kind == "root" && (gate == "blocked" || gate == "rooted" || gate == "occupied") {
					continue
				}
				t.Run(fmt.Sprintf("%s/TB%v/%s", kind, tb, gate), func(t *testing.T) {
					g, gl := newSpecialsTestGame(t)
					g.turnBasedMode = tb
					g.camera.X, g.camera.Y = 6.5*64, 6.5*64
					g.camera.Angle = 0
					g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
					key, x := "bronze_gatekeeper", 7
					if kind == "blink" {
						key, x = "gale_novice", 9
					}
					m := spawnSpecialsMonster(g, key, x, 6)
					m.RootPartyChance, m.RearBlinkChance = 0, 0
					chance := 1.0
					if gate == "no_proc" {
						chance = 0
					}
					if kind == "root" {
						m.RootPartyChance = chance
					} else {
						m.RearBlinkChance = chance
					}
					if gate == "stun" {
						m.StunFramesRemaining, m.StunTurnsRemaining = 240, 2
					}
					if gate == "blocked" {
						for x := 0; x < 6; x++ {
							g.world.Tiles[6][x] = world.TileWall
						}
						g.collisionSystem.UpdateTileChecker(g.world)
					}
					if gate == "rooted" {
						m.RootFramesRemaining, m.RootTurnsRemaining = 240, 1
					}
					if gate == "occupied" {
						for x := 2; x <= 4; x++ {
							e := collision.NewEntity(fmt.Sprint("block", x), (float64(x)+0.5)*64, 6.5*64, 48, 48, collision.CollisionTypeNPC, true)
							g.collisionSystem.RegisterEntity(e)
						}
					}
					m.State, m.StateTimer = monster.StateAttacking, 1
					g.refreshMonsterAIState()
					if tb {
						runTBMonsterTurns(g, gl, 1)
					} else {
						g.combat.HandleMonsterInteractions()
					}
					if kind == "root" {
						want := gate == "proc"
						if g.partyRooted() != want {
							t.Fatalf("root=%v want %v", g.partyRooted(), want)
						}
					} else {
						want := gate == "proc"
						behind := m.X < g.camera.X
						if behind != want {
							t.Fatalf("rear position=%v want %v, x=%v", behind, want, m.X)
						}
						shots := 1
						if gate == "stun" {
							shots = 0
						}
						if len(g.magicProjectiles) != shots {
							t.Fatalf("shots=%d want %d", len(g.magicProjectiles), shots)
						}
						if want && m.X != 2.5*64 {
							t.Fatalf("blink range: x=%v", m.X)
						}
					}
					if gate != "stun" && m.AttackCDFrames <= 0 {
						t.Fatal("attack did not arm cadence")
					}
					if !tb && gate != "stun" {
						before := len(g.magicProjectiles)
						g.combat.HandleMonsterInteractions()
						if len(g.magicProjectiles) != before {
							t.Fatal("ability bypassed attack cooldown")
						}
					}
				})
			}
		}
	}
}

func TestMonkEncounterGroup(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, trigger := range []string{"calm", "sight", "hit", "lethal", "controlled"} {
			t.Run(fmt.Sprintf("TB%v/%s", tb, trigger), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 40, 40)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 30, 30, tile)
				a := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "bronze_gatekeeper", g.config)
				b := monster.NewMonster3DFromConfig(10.6*tile, 10.5*tile, "gale_novice", g.config)
				a.ID, b.ID = "guardian", "novice"
				g.world.Monsters = []*monster.Monster3D{a, b}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				gl.updateMonsterBands()
				if a.BandID == 0 || a.BandID != b.BandID {
					t.Fatal("mixed group did not form")
				}
				a.X += tile / 8
				g.collisionSystem.UpdateEntity(a.ID, a.X, a.Y)
				gl.updateMonsterBands()
				if a.X != b.X || a.Y != b.Y {
					t.Fatal("partners failed to travel together")
				}
				if gl.lootGuardEligible(a) || gl.lootGuardEligible(b) {
					t.Fatal("quest group can be stolen by ambient guard assignment")
				}
				switch trigger {
				case "calm":
					return
				case "sight":
					placePlayerAtTile(g, 14, 10, tile)
					b.AlertRadius = 0 // only the guardian can acquire first sight
				case "hit", "lethal", "controlled":
					if trigger == "controlled" {
						b.Pacified = true
					}
					damage := 1
					if trigger == "lethal" {
						damage = a.HitPoints + 1000
					}
					g.combat.applyMonsterDamagePacket(a, singleMonsterDamagePacket(damagecalc.Parts{Normal: damage}, "physical", 0), monsterDamageOptions{IgnoreArmor: true})
				}
				if tb {
					runTBMonsterTurns(g, gl, 1)
				} else {
					gl.prepareMonsterFrame()
					gl.resolveMonsterFrameActions()
				}
				if trigger == "controlled" {
					if b.IsEngagingPlayer {
						t.Fatal("shared aggro overrode control")
					}
				} else if !b.IsEngagingPlayer || b.WasAttacked != (trigger != "sight") {
					t.Fatal("partner did not immediately share encounter aggro")
				}
			})
		}
	}
}

func TestMonsterSpellStunUsesImpactSpell(t *testing.T) {
	for _, champion := range []bool{false, true} {
		for _, spell := range []string{"lightning", "firebolt"} {
			for _, targetKind := range []string{"party", "crossfire"} {
				t.Run(fmt.Sprintf("champion%v/%s/%s", champion, spell, targetKind), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					original := config.GlobalSpells.Spells["lightning"]
					forced := *original
					forced.StunChance = 1
					config.GlobalSpells.Spells["lightning"] = &forced
					t.Cleanup(func() { config.GlobalSpells.Spells["lightning"] = original })
					m := &monster.Monster3D{ID: "caster", Name: "Caster", HitPoints: 100, DamageMin: 1, DamageMax: 1, StunCharChance: 1, StunCharSeconds: 9, StunCharTurns: 9}
					if champion {
						m.ChampionKey = "test"
					}
					cs.spawnMonsterSpellProjectile(m, spells.SpellID(spell), g.camera.X, g.camera.Y, ProjectileOwnerMonster)
					p := &g.magicProjectiles[0]
					p.X, p.Y = g.camera.X, g.camera.Y
					p.IgnoresDodge = true
					if targetKind == "party" {
						g.party.Members = g.party.Members[:1]
						c := g.party.Members[0]
						c.HitPoints, c.MaxHitPoints = 10000, 10000
						g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
						cs.CheckProjectilePlayerCollisions()
						want := !champion || spell == "lightning"
						if (c.StunFramesRemaining > 0) != want {
							t.Fatalf("stun frames=%d want active %v", c.StunFramesRemaining, want)
						}
						seconds := 2
						if !champion {
							seconds = 9
						}
						if want && c.StunFramesRemaining != seconds*g.config.GetTPS() {
							t.Fatal("stun inherited melee duration")
						}
					} else {
						target := &monster.Monster3D{ID: "victim", Name: "Victim", HitPoints: 100, MaxHitPoints: 100}
						p.Owner = ProjectileOwnerMonsterAtBound
						cs.resolveMonsterProjectileVsMonster(p, "magic_projectile", target, p.ID)
						if (target.StunFramesRemaining > 0) != (!champion || spell == "lightning") {
							t.Fatalf("crossfire stun frames=%d", target.StunFramesRemaining)
						}
					}
					if m.StunCharSeconds != 9 {
						t.Fatal("spell mutated source melee rider")
					}
				})
			}
		}
	}
}

func TestPartyRootMovementAndPersistence(t *testing.T) {
	g, _ := newSpecialsTestGame(t)
	g.partyRoot = PartyRootState{Frames: 240, Turns: 1, Rate: 240}
	ih := &InputHandler{game: g}
	x, y := g.camera.X, g.camera.Y
	ih.movePlayer(10, 0)
	if g.camera.X != x || g.camera.Y != y {
		t.Fatal("RT root allowed movement")
	}
	g.turnBasedMode = true
	if ih.moveTurnBasedInDirection(1, 0) {
		t.Fatal("TB root allowed movement")
	}
	def, err := spells.GetSpellDefinitionByID("jump")
	if err != nil {
		t.Fatal(err)
	}
	g.combat.tryCastJump(def, g.party.Members[0])
	if g.camera.X != x || g.camera.Y != y {
		t.Fatal("Jump bypassed root")
	}
	if !g.party.Members[0].CanAct() {
		t.Fatal("root disabled combat actions")
	}
	g.turnBasedMode = false
	for i := 0; i < 60; i++ {
		g.tickPartyRoot(false)
	}
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
	raw, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err = json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	g.partyRoot = PartyRootState{}
	g.restoreSavedTurnState(&save)
	if g.partyRoot.Frames != 180 || g.partyRoot.Rate != 240 {
		t.Fatalf("lost root clock: %+v", g.partyRoot)
	}
	g.turnBasedMode = true
	g.endPartyTurn()
	if g.partyRooted() {
		t.Fatal("root survived its party turn")
	}
	g.turnBasedMode = false
	if g.partyRooted() {
		t.Fatal("mode switch revived root")
	}
	g.partyRoot = PartyRootState{Frames: 2, Turns: 1, Rate: 2}
	gl := &GameLoop{game: g}
	gl.updateSpecialEffects()
	gl.updateSpecialEffects()
	if g.partyRooted() {
		t.Fatal("RT production update failed to expire root")
	}
}

func TestMonkEncounterSaveRestoreAndReset(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	for _, key := range []string{"bronze_gatekeeper", "gale_novice"} {
		w.Monsters = append(w.Monsters, monster.NewMonster3DFromConfig(64, 64, key, cfg))
	}
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"
	g := newTestGame(cfg, w)
	g.partyRoot = PartyRootState{Frames: 180, Turns: 1, Rate: 240}
	raw, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err = json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	loadedWorld := newTestWorld(cfg)
	wm.LoadedMaps["forest"] = loadedWorld
	loaded := newTestGame(cfg, loadedWorld)
	if err = loaded.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if loaded.partyRoot != g.partyRoot {
		t.Fatal("full restore lost root")
	}
	for _, m := range loadedWorld.Monsters {
		if !m.Banding || m.BandGroup != "unbroken_trial" {
			t.Fatal("restore lost encounter group")
		}
		switch m.Key {
		case "bronze_gatekeeper":
			if m.RootPartyChance != 0.07 || m.RootPartySeconds != 2 || m.RootPartyTurns != 1 {
				t.Fatal("root content/restoration mismatch")
			}
		case "gale_novice":
			if m.RearBlinkChance != 0.07 || m.RearBlinkRangeTiles != 4 {
				t.Fatal("blink content/restoration mismatch")
			}
		}
	}
	loaded.resetTimedEffects()
	if loaded.partyRooted() {
		t.Fatal("new-game effect reset retained root")
	}
	loaded.partyRoot = g.partyRoot
	loaded.restParty()
	if loaded.partyRooted() {
		t.Fatal("rest retained root")
	}
}

func TestMonkPartyAbilitiesDoNotFireAgainstActor(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, gl := newSpecialsTestGame(t)
			m := spawnSpecialsMonster(g, "bronze_gatekeeper", 5, 5)
			foe := spawnSpecialsMonster(g, "gale_novice", 6, 5)
			foe.Bound = true
			m.AIFoe = foe
			m.RootPartyChance = 1
			m.State, m.StateTimer = monster.StateAttacking, 1
			if tb {
				runTBMonsterTurns(g, gl, 1)
			} else {
				g.combat.HandleMonsterInteractions()
			}
			if g.partyRooted() {
				t.Fatal("attack against an actor rooted the party")
			}
		})
	}
}

// The shared hit gate still suppresses spell riders on absorption and dodge.
func TestMonsterSpellStunRespectsDefenses(t *testing.T) {
	for _, defense := range []string{"absorb", "dodge"} {
		t.Run(defense, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			old := config.GlobalSpells.Spells["lightning"]
			forced := *old
			forced.StunChance = 1
			config.GlobalSpells.Spells["lightning"] = &forced
			t.Cleanup(func() { config.GlobalSpells.Spells["lightning"] = old })
			g.party.Members = g.party.Members[:1]
			c := g.party.Members[0]
			c.MaxHitPoints = 10000
			c.Luck = 0
			if defense == "absorb" {
				c.Skills[character.SkillSpellAbsorption] = &character.Skill{Mastery: character.MasteryGrandMaster}
			} else {
				c.Luck = 10000
			}
			m := &monster.Monster3D{ID: "caster", Name: "Caster", HitPoints: 100, DamageMin: 10, DamageMax: 10}
			defended := 0
			for i := 0; i < 64; i++ {
				c.HitPoints = 5000
				c.StunFramesRemaining, c.StunTurnsRemaining, c.StunRate = 0, 0, 0
				cs.spawnMonsterSpellProjectile(m, "lightning", g.camera.X, g.camera.Y, ProjectileOwnerMonster)
				p := &g.magicProjectiles[len(g.magicProjectiles)-1]
				p.X, p.Y = g.camera.X, g.camera.Y
				g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
				cs.CheckProjectilePlayerCollisions()
				if c.HitPoints >= 5000 {
					defended++
					if c.StunFramesRemaining > 0 {
						t.Fatal("defended bolt still applied stun")
					}
				}
			}
			if defended == 0 {
				t.Fatal("fixture did not exercise defense")
			}
		})
	}
}

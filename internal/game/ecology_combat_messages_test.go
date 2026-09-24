package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
)

// Case table: wildlife/caravan/party-controlled combat x melee/arrow/spell x
// hit/dodge/kill x foreground/remote. Shared packet resolution is mode-neutral;
// committed RT/TB attacks and movement are covered separately below.
// Alert deadlines are transient HUD state; save/load coverage lives with routes.
func TestEcologyCombatMessagePolicy(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = previous })
	if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []bool{false, true} {
		for _, kind := range []string{"wildlife", "caravan", "bound"} {
			for _, delivery := range []string{"melee", "arrow", "spell"} {
				for _, outcome := range []string{"hit", "dodge", "kill", "true_dodge", "disintegrate"} {
					if delivery == "melee" && outcome == "disintegrate" {
						continue
					}
					t.Run(fmt.Sprintf("remote%v/%s/%s/%s", remote, kind, delivery, outcome), func(t *testing.T) {
						owner := newTestGame(cfg, newTestWorldSized(cfg, 30, 30))
						g := owner
						if remote {
							g = newTestGame(cfg, newTestWorldSized(cfg, 30, 30))
							g.ecologyOwner = owner
						}
						cs := NewCombatSystem(g)
						g.combat = cs
						ts := float64(cfg.GetTileSize())
						source := monster.NewMonster3DFromConfig(10.5*ts, 10.5*ts, "fennec", cfg)
						key := "desert_rabbit"
						if kind == "caravan" {
							key = "desert_caravan"
						}
						target := monster.NewMonster3DFromConfig(11.5*ts, 10.5*ts, key, cfg)
						if kind == "bound" {
							source.Bound = true
						}
						target.MaxHitPoints, target.HitPoints = 200, 200
						target.ArmorClass, target.PerfectDodge = 0, 0
						if outcome == "dodge" || outcome == "true_dodge" {
							target.PerfectDodge = 100
						}
						if outcome == "kill" {
							target.HitPoints = 1
						}
						if kind == "caravan" {
							owner.ecology = EcologyState{Unlocked: true, ActorID: target.ID}
							g.ecology = owner.ecology
						}
						g.world.Monsters = []*monster.Monster3D{source, target}
						g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
						switch delivery {
						case "melee":
							source.DamageMin, source.DamageMax = 20, 20
							if outcome == "true_dodge" {
								cs.strikeMonsterPacketFor(source, target, singleMonsterDamagePacket(damagecalc.Parts{Normal: 20, True: 3}, "physical", 0), nil, false, false, false, true)
							} else {
								cs.monsterStrikeMonster(source, target)
							}
						case "arrow":
							p := &Arrow{ID: "test", Active: true, LifeTime: 10, Damage: 20, DamageType: "physical", SourceName: source.Name, SourceMonster: source, Owner: ProjectileOwnerMonsterAtBound}
							if outcome == "true_dodge" {
								p.TrueDamage = 3
							}
							if outcome == "disintegrate" {
								p.DisintegrateChance = 1
							}
							cs.resolveMonsterProjectileVsMonster(p, "arrow", target, p.ID)
						case "spell":
							p := &MagicProjectile{ID: "test", Active: true, LifeTime: 10, Damage: 20, SpellType: "fireball", SourceName: source.Name, SourceMonster: source, Owner: ProjectileOwnerMonsterAtBound}
							if outcome == "true_dodge" {
								p.TrueDamage = 3
							}
							if outcome == "disintegrate" {
								p.DisintegrateChance = 1
							}
							cs.resolveMonsterProjectileVsMonster(p, "magic_projectile", target, p.ID)
						}
						switch kind {
						case "wildlife":
							if len(g.combatLogHistory) != 0 || len(owner.combatLogHistory) != 0 {
								t.Fatal("wildlife combat leaked into log")
							}
						case "caravan":
							if outcome == "kill" || outcome == "disintegrate" {
								assertCaravanNotices(t, owner, "caravan.under_attack", "caravan.destroyed")
							} else {
								assertCaravanAlerts(t, owner, 1)
							}
							if remote && len(g.combatLogHistory) != 0 {
								t.Fatal("remote hit details were logged")
							}
						case "bound":
							if len(g.combatLogHistory) == 0 {
								t.Fatal("party-controlled combat lost its messages")
							}
						}
						if outcome == "dodge" && target.HitPoints != 200 {
							t.Fatal("dodge damage changed")
						}
						if outcome == "true_dodge" && target.HitPoints != 197 {
							t.Fatal("true damage through dodge changed")
						}
						if outcome == "hit" && target.HitPoints >= 200 {
							t.Fatal("logging suppressed actual damage")
						}
						if (outcome == "kill" || outcome == "disintegrate") && target.IsAlive() {
							t.Fatal("logging suppressed death")
						}
					})
				}
			}
		}
	}
}

func assertCaravanAlerts(t *testing.T, g *MMGame, count int) {
	t.Helper()
	if len(g.combatLogHistory) != count {
		t.Fatalf("want %d alerts, got %+v", count, g.combatLogHistory)
	}
	for _, entry := range g.combatLogHistory {
		if entry.Text != uitext.Text("caravan.under_attack") || entry.Color != combatMessageRed {
			t.Fatalf("unexpected alert: %+v", entry)
		}
	}
}

func TestCaravanAlertCooldownSharedAcrossMapsAndAttackers(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = previous })
	if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	owner := newTestGame(cfg, newTestWorldSized(cfg, 30, 30))
	remote := newTestGame(cfg, newTestWorldSized(cfg, 30, 30))
	remote.ecologyOwner = owner
	target := monster.NewMonster3DFromConfig(500, 500, "desert_caravan", cfg)
	target.HitPoints = 10000
	source := monster.NewMonster3DFromConfig(400, 500, "bandit", cfg)
	hit := func(g *MMGame) {
		NewCombatSystem(g).strikeMonsterPacketFor(source, target, singleMonsterDamagePacket(damagecalc.Parts{True: 1}, "physical", 0), nil, false, true, true, false)
	}
	config.GlobalEcology.Caravan.AttackAlertCooldownSeconds = 37
	before := time.Now()
	hit(remote)
	if owner.caravanAttackAlertUntil.Before(before.Add(37*time.Second)) || owner.caravanAttackAlertUntil.After(time.Now().Add(37*time.Second)) {
		t.Fatal("cooldown does not honor the content setting")
	}
	assertCaravanAlerts(t, owner, 1)
	source = monster.NewMonster3DFromConfig(400, 500, "bandit", cfg)
	for _, g := range []*MMGame{owner, remote, owner} {
		g.turnBasedMode = !g.turnBasedMode
		g.frameCount += 100000
		hit(g)
	}
	assertCaravanAlerts(t, owner, 1)
	owner.caravanAttackAlertUntil = time.Now().Add(-time.Millisecond)
	hit(owner)
	hit(remote)
	assertCaravanAlerts(t, owner, 2)
}

func TestCaravanKeepsMovingUnderCommittedAttacks(t *testing.T) {
	for _, turn := range []bool{false, true} {
		for _, remote := range []bool{false, true} {
			t.Run(fmt.Sprintf("turn%v/remote%v", turn, remote), func(t *testing.T) {
				owner, wm, ts := ecologyTestGame(t)
				owner.ecology.Unlocked = true
				owner.spawnCaravan()
				w, caravan := owner.ecologyActor()
				source := monster.NewMonster3DFromConfig(caravan.X-ts, caravan.Y, "bandit", owner.config)
				source.ProjectileWeapon, source.RangedAttackRange = "", 0
				source.DamageMin, source.DamageMax = 1, 1
				w.Monsters = append(w.Monsters, source)
				g := owner
				if remote {
					owner.world = newTestWorldSized(owner.config, 30, 30)
					wm.LoadedMaps["other"] = owner.world
					owner.simulateRemoteEcology(false, false)
					g = owner.ecologyViews[w]
				}
				g.turnBasedMode = turn
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				placePlayerAtTile(g, 1, 1, ts)
				start := caravan.X
				hp := caravan.HitPoints
				for tick := 0; tick < 20; tick++ {
					g.ecology = owner.ecology
					g.prepareAmbientTarget(caravan)
					source.AIFoe = caravan
					source.AttackCDFrames = 0
					cadence := monsterAttackRealtime
					if turn {
						cadence = monsterAttackTurn
					}
					pathLen := len(caravan.PathTiles)
					spent := g.combat.commitMonsterAttack(source, monsterAttackDestination{foe: caravan}, cadence)
					if spent && (caravan.IsEngagingPlayer || len(caravan.PathTiles) != pathLen) {
						t.Fatal("hit interrupted caravan path")
					}
					caravan.UpdateAmbient(g.collisionSystem, caravan.AITargetX, caravan.AITargetY, turn)
					g.collisionSystem.UpdateEntity(caravan.ID, caravan.X, caravan.Y)
				}
				if caravan.X <= start || caravan.HitPoints >= hp {
					t.Fatalf("caravan must take damage and continue: x %.2f -> %.2f HP %d -> %d", start, caravan.X, hp, caravan.HitPoints)
				}
				assertCaravanAlerts(t, owner, 1)
			})
		}
	}
}

func TestCaravanRangedLaunchWarnsBeforeImpactAndCancelsRest(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(fmt.Sprint(turn), func(t *testing.T) {
			g, _, ts := ecologyTestGame(t)
			g.ecology.Unlocked = true
			g.spawnCaravan()
			_, target := g.ecologyActor()
			source := monster.NewMonster3DFromConfig(target.X-3*ts, target.Y, "bandit", g.config)
			source.RangedAttackRange = 5 * ts
			source.AIFoe = target
			g.world.Monsters = append(g.world.Monsters, source)
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			g.turnBasedMode = turn
			g.ecology.StopFrames = 15 * g.config.GetTPS()
			hp := target.HitPoints
			cadence := monsterAttackRealtime
			if turn {
				cadence = monsterAttackTurn
			}
			if !g.combat.commitMonsterAttack(source, monsterAttackDestination{foe: target}, cadence) {
				t.Fatal("ranged attack not committed")
			}
			if target.HitPoints != hp {
				t.Fatal("expected projectile still in flight")
			}
			assertCaravanAlerts(t, g, 1)
			if g.ecology.StopFrames != 0 {
				t.Fatal("caravan waits under attack")
			}
			g.setCaravanTarget(target)
			if target.AITargetX == target.X && target.AITargetY == target.Y {
				t.Fatal("rest cancellation did not restore travel target")
			}
		})
	}
}

func TestCaravanTargetDoesNotTurnSupportActionsIntoAttackAlerts(t *testing.T) {
	g, _, ts := ecologyTestGame(t)
	g.ecology.Unlocked = true
	g.spawnCaravan()
	_, target := g.ecologyActor()
	source := monster.NewMonster3DFromConfig(target.X-ts, target.Y, "bandit", g.config)
	source.AIFoe = target
	source.HitPoints = source.MaxHitPoints / 2
	source.AllyHealChance, source.AllyHealAmount = 1, 5
	g.world.Monsters = append(g.world.Monsters, source)
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	if !g.combat.commitMonsterAttack(source, monsterAttackDestination{foe: target}, monsterAttackRealtime) {
		t.Fatal("support action was not consumed")
	}
	if !g.caravanAttackAlertUntil.IsZero() {
		t.Fatal("self-heal raised a caravan attack alert")
	}
}

func TestCaravanProjectileStunDoesNotSpamLog(t *testing.T) {
	g, _, ts := ecologyTestGame(t)
	g.ecology.Unlocked = true
	g.spawnCaravan()
	_, target := g.ecologyActor()
	source := monster.NewMonster3DFromConfig(target.X-ts, target.Y, "bandit", g.config)
	original := config.GlobalSpells.Spells["psychic_shock"]
	guaranteed := *original
	guaranteed.StunChance = 1
	config.GlobalSpells.Spells["psychic_shock"] = &guaranteed
	t.Cleanup(func() { config.GlobalSpells.Spells["psychic_shock"] = original })
	// Force the stun roll; verify the rider retains its effect without
	// adding a white stun/resist line beside the single red caravan alert.
	for i := 0; i < 5; i++ {
		p := &MagicProjectile{ID: fmt.Sprint(i), Active: true, LifeTime: 10, Damage: 1, SpellType: "psychic_shock", SourceName: source.Name, SourceMonster: source, Owner: ProjectileOwnerMonsterAtBound}
		g.combat.resolveMonsterProjectileVsMonster(p, "magic_projectile", target, p.ID)
	}
	assertCaravanAlerts(t, g, 1)
	if target.StunDRStacks == 0 {
		t.Fatal("quiet logging removed the stun effect")
	}
}

func assertCaravanNotices(t *testing.T, g *MMGame, keys ...string) {
	t.Helper()
	if len(g.combatLogHistory) != len(keys) {
		t.Fatalf("want notices %v, got %+v", keys, g.combatLogHistory)
	}
	for i, key := range keys {
		if got := g.combatLogHistory[i]; got.Text != uitext.Text(key) || got.Color != combatMessageRed {
			t.Fatalf("notice %d: %+v", i, got)
		}
	}
}

// Case table: combat finalization (foreground/remote), dead actor fallback,
// missing actor fallback x cooldown active/expired. Each loss survives a real
// save/apply cycle without replay, and a replacement can announce its own loss.
func TestCaravanLossLifecycle(t *testing.T) {
	for _, entry := range []string{"combat", "remote", "dead", "missing"} {
		for _, cooling := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cooling%v", entry, cooling), func(t *testing.T) {
				g, wm, _ := ecologyTestGame(t)
				config.GlobalEcology.Populations = nil
				config.GlobalEcology.Fish = nil
				g.ecology.Unlocked = true
				g.spawnCaravan()
				w, m := g.ecologyActor()
				if m == nil {
					t.Fatal("caravan did not spawn")
				}
				if cooling {
					g.caravanAttackAlertUntil = time.Now().Add(time.Hour)
				}
				m.HitPoints = 0
				switch entry {
				case "combat":
					g.combat.finishMonsterKill(m)
				case "remote":
					v := newTestGame(g.config, w)
					v.ecologyOwner, v.ecology = g, g.ecology
					v.combat = NewCombatSystem(v)
					v.combat.finishMonsterKill(m)
					v.combat.finishMonsterKill(m)
				case "missing":
					w.Monsters = nil
				}
				g.updateEcology()
				g.updateEcology()
				assertCaravanNotices(t, g, "caravan.destroyed")
				if g.ecology.RespawnDay != g.currentCalendarDay()+1 {
					t.Fatal("loss did not schedule next dawn")
				}
				data, err := json.Marshal(g.buildSave(wm))
				if err != nil {
					t.Fatal(err)
				}
				var saved GameSave
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				g.combatLogHistory = nil
				g.updateEcology()
				assertCaravanNotices(t, g)
				g.calendarDay++
				g.updateEcology()
				_, replacement := g.ecologyActor()
				if replacement == nil || replacement.ID == m.ID {
					t.Fatal("no replacement at dawn")
				}
				replacement.HitPoints = 0
				g.combat.finishMonsterKill(replacement)
				g.updateEcology()
				assertCaravanNotices(t, g, "caravan.destroyed")
			})
		}
	}
}

func TestCaravanAlertNewGameReset(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = previous })
	if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	wm, qm := loadRealWorldForTest(t, cfg, "forest")
	quests.GlobalQuestManager = qm
	g := newTestGame(cfg, wm.LoadedMaps["forest"])
	g.questManager = qm
	g.caravanAttackAlertUntil = time.Now().Add(time.Hour)
	g.startNewGameWithParty(character.NewParty(cfg))
	if !g.caravanAttackAlertUntil.IsZero() {
		t.Fatal("new game inherited caravan alert cooldown")
	}
	target := monster.NewMonster3DFromConfig(500, 500, "desert_caravan", cfg)
	source := monster.NewMonster3DFromConfig(400, 500, "bandit", cfg)
	g.combatLogHistory = nil
	NewCombatSystem(g).strikeMonsterPacketFor(source, target, singleMonsterDamagePacket(damagecalc.Parts{True: 1}, "physical", 0), nil, false, true, true, false)
	assertCaravanAlerts(t, g, 1)
}

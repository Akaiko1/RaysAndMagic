package game

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
	"ugataima/internal/world"
)

// tarnTrident loads the shipped weapons; call it before any fixture chdir.
func tarnTrident(t *testing.T) *config.WeaponDefinitionConfig {
	t.Helper()
	prev := config.GlobalWeapons
	t.Cleanup(func() { config.GlobalWeapons = prev })
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatal(err)
	}
	def, ok := config.GetWeaponDefinition("tarn_trident")
	if !ok || def.SlowPct <= 0 {
		t.Fatal("tarn_trident with slow_pct missing")
	}
	return def
}

func reloadStatusFixture(t *testing.T, g *MMGame, wm *world.WorldManager) {
	t.Helper()
	save := auditSaveJSON(t, g.buildSave(wm))
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
}

// Slow in real time drags movement, attack cadence and pounce cadence by the
// same SlowPct. Turn-based clocks are never stretched: TB pays with skips.
func TestSlowRealtimeCadence(t *testing.T) {
	trident := tarnTrident(t)
	factor := 100 / float64(100-trident.SlowPct)
	type sample struct {
		step               float64
		attackCD, pounceCD int
		pounceTurns        int
		ambientMoves       string
	}
	measure := func(t *testing.T, slowed bool) sample {
		var s sample
		hit := func(g *MMGame, m *monster.Monster3D) {
			if slowed {
				g.combat.tryApplyWeaponHitRiders(m, trident)
			}
		}

		g, _, ts := tbBehaviorGame(t, 40, 40)
		g.turnBasedMode = false
		placePlayerAtTile(g, 5, 10, ts)
		walker := monster.NewMonster3DFromConfig(20.5*ts, 10.5*ts, "goblin", g.config)
		walker.WasAttacked = true
		walker.BeginPlayerEngagement()
		g.world.Monsters = []*monster.Monster3D{walker}
		g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
		hit(g, walker)
		g.refreshMonsterAIState()
		x := walker.X
		for range 30 {
			wr := CreateMonsterWrapper(walker, g.collisionSystem, g.collisionSystem.Snapshot(), g)
			wr.Update()
			wr.ApplyCollisionUpdate()
		}
		s.step = x - walker.X

		g, gl, ts := tbBehaviorGame(t, 40, 40)
		g.turnBasedMode = false
		placePlayerAtTile(g, 5, 10, ts)
		striker := monster.NewMonster3DFromConfig(6.5*ts, 10.5*ts, "goblin", g.config)
		striker.WasAttacked = true
		striker.BeginPlayerEngagement()
		g.world.Monsters = []*monster.Monster3D{striker}
		g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
		hit(g, striker)
		g.refreshMonsterAIState()
		striker.State = monster.StateAttacking
		striker.StateTimer = 1
		gl.reconcileMonsterAttackPosts()
		g.combat.HandleMonsterInteractions()
		s.attackCD = striker.AttackCDFrames

		for _, tb := range []bool{false, true} {
			g, gl := newSpecialsTestGame(t)
			ts := float64(g.config.GetTileSize())
			g.turnBasedMode = tb
			placePlayerAtTile(g, 5, 6, ts)
			puma := monster.NewMonster3DFromConfig(8.5*ts, 6.5*ts, "puma", g.config)
			puma.WasAttacked = true
			puma.BeginPlayerEngagement()
			puma.State = monster.StatePursuing
			g.world.Monsters = []*monster.Monster3D{puma}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			hit(g, puma)
			if tb {
				g.refreshMonsterAIState()
				runOneMonsterTurn(g, gl)
				s.pounceTurns = puma.PounceCDTurns
			} else {
				g.combat.HandleMonsterInteractions()
				s.pounceCD = puma.PounceCDFrames
			}
		}

		prev := config.GlobalEcology
		t.Cleanup(func() { config.GlobalEcology = prev })
		grazer := monster.NewMonster3DFromConfig(0, 0, "goblin", g.config)
		grazer.Disposition = "grazer"
		config.GlobalEcology = &config.EcologyConfig{TurnStepSpeed: 2 * grazer.Speed}
		hit(g, grazer)
		for range 4 {
			if grazer.SpendAmbientTurnMove() {
				s.ambientMoves += "x"
			} else {
				s.ambientMoves += "-"
			}
		}
		return s
	}

	base, slowed := measure(t, false), measure(t, true)
	if base.step <= 0 || base.attackCD <= 0 || base.pounceCD <= 0 {
		t.Fatalf("unslowed control did not act: %+v", base)
	}
	if ratio := slowed.step / base.step; math.Abs(ratio-1/factor) > 0.02 {
		t.Errorf("RT movement ratio %.3f, want %.3f", ratio, 1/factor)
	}
	if want := int(math.Round(float64(base.attackCD) * factor)); slowed.attackCD != want {
		t.Errorf("RT attack cooldown %d, want %d (unslowed %d)", slowed.attackCD, want, base.attackCD)
	}
	if want := int(math.Round(float64(base.pounceCD) * factor)); slowed.pounceCD != want {
		t.Errorf("RT pounce cooldown %d, want %d (unslowed %d)", slowed.pounceCD, want, base.pounceCD)
	}
	if base.pounceTurns != TurnBasedPounceCooldownTurns || slowed.pounceTurns != base.pounceTurns {
		t.Errorf("TB pounce cooldown turns %d/%d, want %d both", base.pounceTurns, slowed.pounceTurns, TurnBasedPounceCooldownTurns)
	}
	if slowed.ambientMoves != base.ambientMoves {
		t.Errorf("TB ambient credit %q under slow, want the unslowed %q", slowed.ambientMoves, base.ambientMoves)
	}
}

// A slowed monster loses every SlowSkipEveryTurns-th turn outright: no step and
// no attack. The cadence survives save/load and refreshes, and a fresh slow
// always starts counting from zero.
func TestSlowTurnBasedSkipCadence(t *testing.T) {
	trident := tarnTrident(t)
	const every = 3 // fixture cadence, set on the loaded config below
	slowTurns := config.WeaponStatusTurns(trident.SlowSeconds)
	turns := max(2*every, slowTurns+every)
	// cadence is the expected run: x acted, - skipped (every k-th turn while slowed).
	cadence := func(slowedTurns int) string {
		var b strings.Builder
		for k := 1; k <= turns; k++ {
			if k <= slowedTurns && k%every == 0 {
				b.WriteByte('-')
			} else {
				b.WriteByte('x')
			}
		}
		return b.String()
	}
	moved := func(_ *MMGame, m *monster.Monster3D, x, y float64) bool { return m.X != x || m.Y != y }
	actors := []struct {
		name  string
		key   string
		tileX float64
		prep  func(g *MMGame, m *monster.Monster3D)
		acted func(g *MMGame, m *monster.Monster3D, x, y float64) bool
	}{
		{"pursue", "goblin", 16.5, func(_ *MMGame, m *monster.Monster3D) { m.State = monster.StatePursuing }, moved},
		{"flee", "kasa_obake", 10.5, func(_ *MMGame, m *monster.Monster3D) {
			m.State = monster.StateFleeing
			m.StateTimer = 0
		}, moved},
		{"ranged", "dragon_brood_mother", 13.5, func(_ *MMGame, m *monster.Monster3D) { disableRandomSpecials(m) },
			func(g *MMGame, _ *monster.Monster3D, _, _ float64) bool { return len(g.magicProjectiles) > 0 }},
	}
	type env struct {
		t  *testing.T
		g  *MMGame
		wm *world.WorldManager
	}
	hit := func(e env, m *monster.Monster3D) { e.g.combat.tryApplyWeaponHitRiders(m, trident) }
	// A slow one turn shorter than the cadence, reapplied as it lapses.
	shortTurns := every - 1
	shortSlow := func(m *monster.Monster3D) {
		m.ApplySlow(trident.SlowPct, shortTurns*TurnBasedPeriodicEffectSeconds*config.GetTargetTPS(), shortTurns)
	}
	scenarios := []struct {
		name   string
		before func(e env, m *monster.Monster3D, turn int)
		want   string
	}{
		{"unslowed", func(env, *monster.Monster3D, int) {}, cadence(0)},
		{"hit_every_turn", func(e env, m *monster.Monster3D, _ int) { hit(e, m) }, cadence(turns)},
		{"hit_once_expires", func(e env, m *monster.Monster3D, turn int) {
			if turn == 1 {
				hit(e, m)
			}
		}, cadence(slowTurns)},
		{"save_before_skip", func(e env, m *monster.Monster3D, turn int) {
			hit(e, m)
			if turn == every {
				reloadStatusFixture(e.t, e.g, e.wm)
			}
		}, cadence(turns)},
		{"short_slows_reapplied", func(_ env, m *monster.Monster3D, turn int) {
			if (turn-1)%shortTurns == 0 {
				shortSlow(m)
			}
		}, cadence(0)},
	}
	for _, a := range actors {
		for _, sc := range scenarios {
			t.Run(a.name+"/"+sc.name, func(t *testing.T) {
				g, gl, wm := clockSaveFixture(t)
				prevEvery := config.GlobalConfig.StatusEffects.SlowSkipEveryTurns
				config.GlobalConfig.StatusEffects.SlowSkipEveryTurns = every
				t.Cleanup(func() { config.GlobalConfig.StatusEffects.SlowSkipEveryTurns = prevEvery })
				g.turnBasedMode = true
				ts := float64(g.config.GetTileSize())
				for _, c := range g.party.Members {
					c.HitPoints, c.MaxHitPoints = 100000, 100000
				}
				m := monster.NewMonster3DFromConfig(a.tileX*ts, 10.5*ts, a.key, g.config)
				m.WasAttacked = true
				m.BeginPlayerEngagement()
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				got := ""
				for turn := 1; turn <= turns; turn++ {
					sc.before(env{t, g, wm}, m, turn)
					m = g.world.Monsters[0]
					m.X, m.Y = a.tileX*ts, 10.5*ts
					a.prep(g, m)
					m.ResetPathfinding()
					g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
					g.magicProjectiles = g.magicProjectiles[:0]
					g.refreshMonsterAIState()
					x, y := m.X, m.Y
					runOneMonsterTurn(g, gl)
					if a.acted(g, m, x, y) {
						got += "x"
					} else {
						got += "-"
					}
				}
				if got != sc.want {
					t.Fatalf("turns %q, want %q", got, sc.want)
				}
			})
		}
	}
}

// Root keeps burning its own turn clock on the turn slow skips.
func TestSlowSkippedTurnStillTicksRoot(t *testing.T) {
	g, gl, _ := clockSaveFixture(t)
	g.turnBasedMode = true
	m := monster.NewMonster3DFromConfig(400, 400, "goblin", g.config)
	g.world.Monsters = []*monster.Monster3D{m}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.combat.applyMonsterRoot(m, 5, 5*TurnBasedPeriodicEffectSeconds*g.config.GetTPS())
	for turn := 1; turn <= 3; turn++ {
		m.ApplySlow(30, 10*g.config.GetTPS(), 5)
		runOneMonsterTurn(g, gl)
		if m.RootTurnsRemaining != 5-turn {
			t.Fatalf("turn %d: root turns %d, want %d", turn, m.RootTurnsRemaining, 5-turn)
		}
	}
}

// Eradication ends every running affliction with the hero, so Resurrect brings
// back a clean body. Without eradication the same afflictions keep running.
func TestEradicationClearsAfflictionClocks(t *testing.T) {
	prevSpells := config.GlobalSpells
	t.Cleanup(func() { config.GlobalSpells = prevSpells })
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatal(err)
	}
	resurrect, err := spells.GetSpellDefinitionByID("resurrect")
	if err != nil || !resurrect.Revive {
		t.Fatal("resurrect spell missing")
	}
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"poison", "burn", "stun", "all"} {
			for _, outcome := range []string{"control", "eradicated", "eradicated_saved"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, kind, outcome), func(t *testing.T) {
					g, _, wm := clockSaveFixture(t)
					g.turnBasedMode = tb
					tps := g.config.GetTPS()
					c := g.party.Members[0]
					c.Conditions = nil
					c.HitPoints, c.MaxHitPoints = 1000, 1000
					if kind == "poison" || kind == "all" {
						c.ApplyPoison(20 * tps)
					}
					if kind == "burn" || kind == "all" {
						c.ApplyBurn(20 * tps)
					}
					if kind == "stun" || kind == "all" {
						c.ApplyCharStun(20*tps, 5)
					}
					mob := monster.NewMonster3DFromConfig(400, 400, "goblin", g.config)
					if outcome != "control" {
						g.combat.monsterHitCharacter(mob, c, "Test", monsterCharacterHit{DisintegrateChance: 1})
						if !c.HasCondition(character.ConditionEradicated) {
							t.Fatal("setup: hero was not eradicated")
						}
						if outcome == "eradicated_saved" {
							reloadStatusFixture(t, g, wm)
							c = g.party.Members[0]
						}
						if g.combat.tryCastResurrect(resurrect, g.party.Members[1]) != castCommitted {
							t.Fatal("setup: resurrect failed")
						}
					}
					hp := c.HitPoints
					if tb {
						g.startPartyTurn()
					} else {
						for range 2 * tps {
							g.updatePartyClocks()
						}
					}
					afflicted := c.HitPoints < hp || c.IsStunned() || c.PoisonFramesRemaining > 0 || c.BurnFramesRemaining > 0
					tagged := c.HasCondition(character.ConditionPoisoned) || c.HasCondition(character.ConditionBurning) || c.HasCondition(character.ConditionStunned)
					if outcome == "control" {
						if !afflicted || !tagged {
							t.Fatal("control: afflictions stopped without eradication")
						}
						return
					}
					if afflicted || tagged {
						t.Fatalf("resurrected hero still afflicted: HP %d->%d stun=%v poison=%d burn=%d conditions=%v",
							hp, c.HitPoints, c.IsStunned(), c.PoisonFramesRemaining, c.BurnFramesRemaining, c.Conditions)
					}
				})
			}
		}
	}
}

// Charm takes hold of living, non-formless targets only. Formless covers every
// boss; undead answer to Bind Undead instead.
func TestCharmTargetEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		charmed   bool
	}{
		{"living", "goblin", true},
		{"undead", "skeleton", false},
		{"formless", "enforcer_alien", false},
		{"formless_boss", "golden_thief_bug", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, _ := tbBehaviorGame(t, 20, 20)
			m := monster.NewMonster3DFromConfig(400, 400, tc.key, g.config)
			if (m.MonsterType == "formless") != strings.HasPrefix(tc.name, "formless") || (m.MonsterType == "undead") != (tc.name == "undead") {
				t.Fatalf("setup: %s has type %q", tc.key, m.MonsterType)
			}
			m.WasAttacked = true
			g.combat.applyPacify(m, 60, "Charm")
			if m.Pacified != tc.charmed || m.CharmedByParty != tc.charmed || m.WasAttacked == tc.charmed {
				t.Fatalf("pacified=%v charmedByParty=%v wasAttacked=%v, want charmed=%v", m.Pacified, m.CharmedByParty, m.WasAttacked, tc.charmed)
			}
			if rejected := countCombatLog(g, "has no hold over") == 1; rejected == tc.charmed {
				t.Fatalf("rejection message=%v, want %v", rejected, !tc.charmed)
			}
		})
	}
}

// The last turn of a TB debuff stays in force through that turn, including
// across a save made inside it; the turn boundary still clears it.
func TestTurnDebuffLatchesSurviveSave(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(g *MMGame, m *monster.Monster3D)
	}{
		{"root", func(g *MMGame, m *monster.Monster3D) { g.combat.applyMonsterRoot(m, 1, g.config.GetTPS()) }},
		{"slow", func(g *MMGame, m *monster.Monster3D) { m.ApplySlow(30, g.config.GetTPS(), 1) }},
		{"weaken", func(g *MMGame, m *monster.Monster3D) { m.ApplyWeaken(25, g.config.GetTPS(), 1) }},
	} {
		for _, saved := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/saved=%v", tc.name, saved), func(t *testing.T) {
				g, gl, wm := clockSaveFixture(t)
				g.turnBasedMode = true
				m := monster.NewMonster3DFromConfig(400, 400, "goblin", g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				tc.apply(g, m)
				g.currentTurn = 1
				g.tickMonsterTurnStatuses(m, true)
				if saved {
					reloadStatusFixture(t, g, wm)
					m = g.world.Monsters[0]
				}
				root, slow, weak := m.TurnDebuffLatches()
				if root != (tc.name == "root") || (slow > 0) != (tc.name == "slow") || (weak > 0) != (tc.name == "weaken") {
					t.Fatalf("final-turn latch lost: root=%v slow=%d weaken=%d", root, slow, weak)
				}
				if tc.name == "root" && !m.RootHeld() {
					t.Fatal("rooted final turn released the monster")
				}
				gl.endMonsterTurn()
				if root, slow, weak := m.TurnDebuffLatches(); root || slow != 0 || weak != 0 {
					t.Fatal("latch survived the turn boundary")
				}
			})
		}
	}
}

// Enrage is announced exactly once, on the first boss tick that sees HP at or
// below the threshold. A save at any point neither repeats nor drops it; a
// legacy save without the latch announces once more. Loading clears the combat
// log, so each phase is counted on its own.
func TestEnrageAnnouncesOnceAcrossSave(t *testing.T) {
	cases := []struct {
		name string
		// hpBefore is the boss HP relative to the threshold before the first
		// phase; ranBefore is whether the boss ticks before the save.
		hpBefore, hpAfter     int
		ranBefore, legacy     bool
		wantBefore, wantAfter int
	}{
		{name: "no_save", hpBefore: 0, hpAfter: 0, ranBefore: true, wantBefore: 1, wantAfter: 0},
		{name: "announced", hpBefore: 0, hpAfter: 0, ranBefore: true, wantBefore: 1, wantAfter: 0},
		{name: "calm", hpBefore: 1, hpAfter: 0, ranBefore: true, wantBefore: 0, wantAfter: 1},
		{name: "crossed_unannounced", hpBefore: 0, hpAfter: 0, ranBefore: false, wantBefore: 0, wantAfter: 1},
		{name: "legacy_announced", hpBefore: 0, hpAfter: 0, ranBefore: true, legacy: true, wantBefore: 1, wantAfter: 1},
	}
	for _, mode := range []string{"rt", "tb"} {
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				g, gl, wm := clockSaveFixture(t)
				ts := float64(g.config.GetTileSize())
				for _, c := range g.party.Members {
					c.HitPoints, c.MaxHitPoints = 100000, 100000
				}
				spawn := func() *monster.Monster3D {
					m := g.world.Monsters[0]
					aggro(m)
					noSummons(m)
					m.IsEngagingPlayer, m.WasAttacked = true, true
					return m
				}
				boss := monster.NewMonster3DFromConfig(9.5*ts, 10.5*ts, "old_samurai", g.config)
				g.world.Monsters = []*monster.Monster3D{boss}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				boss = spawn()
				if boss.EnrageAtHP <= 0 {
					t.Fatal("setup: old_samurai has no enrage threshold")
				}
				run := func() {
					if mode == "rt" {
						g.turnBasedMode = false
						runRTCombatSeconds(g, 1)
					} else {
						g.refreshMonsterAIState() // the frame loop's pass before the monster turn
						runTBMonsterTurns(g, gl, 1)
					}
				}
				const rage = "flies into a furious rage"
				boss.HitPoints = boss.EnrageAtHP + tc.hpBefore
				if tc.ranBefore {
					run()
				}
				before := countCombatLog(g, rage)
				if tc.name != "no_save" {
					save := auditSaveJSON(t, g.buildSave(wm))
					if tc.legacy {
						for i := range save.Monsters {
							save.Monsters[i].EnrageAnnounced = false
						}
						for key := range save.MapMonsters {
							for i := range save.MapMonsters[key] {
								save.MapMonsters[key][i].EnrageAnnounced = false
							}
						}
					}
					if err := g.applySave(wm, &save); err != nil {
						t.Fatal(err)
					}
					boss = spawn()
				}
				boss.HitPoints = boss.EnrageAtHP + tc.hpAfter
				run()
				after := countCombatLog(g, rage)
				if tc.name == "no_save" {
					after -= before
				}
				if before != tc.wantBefore || after != tc.wantAfter {
					t.Fatalf("enrage announced %d before and %d after, want %d and %d", before, after, tc.wantBefore, tc.wantAfter)
				}
			})
		}
	}
}

// Every real-time hand stream a slowed monster arms is its unslowed cadence
// stretched by the same factor: main and off hand, against the party, against
// a summon, and when a TB attack carries its cooldowns into real time.
func TestSlowChampionHandStreams(t *testing.T) {
	trident := tarnTrident(t)
	factor := 100 / float64(100-trident.SlowPct)
	measure := func(t *testing.T, entry string, slowed bool) (mainCD, offCD int) {
		g, gl, ts := tbBehaviorGame(t, 40, 40)
		g.turnBasedMode = entry == "tb_carry"
		primeTestChampions(t, g)
		fillTestParty(t, g)
		for _, c := range g.party.Members {
			c.HitPoints, c.MaxHitPoints = 100000, 100000
		}
		placePlayerAtTile(g, 5, 10, ts)
		champX := 6.5 * ts
		if entry == "summon" {
			champX = 20.5 * ts
		}
		champ := monster.NewMonster3DFromConfig(champX, 10.5*ts, "weapon_master", g.config)
		champ.ChampionTier = "impossible"
		champ.WasAttacked = true
		monsters := []*monster.Monster3D{champ}
		if entry == "summon" {
			ally := monster.NewMonster3DFromConfig(21.5*ts, 10.5*ts, "masked_huntress", g.config)
			ally.MaxHitPoints, ally.HitPoints = 5000, 5000
			markCardAlly(ally)
			monsters = append(monsters, ally)
		} else {
			champ.BeginPlayerEngagement()
		}
		g.world.Monsters = monsters
		g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
		g.refreshMonsterAIState()
		if slowed {
			g.combat.tryApplyWeaponHitRiders(champ, trident)
		}
		switch entry {
		case "party":
			champ.State = monster.StateAttacking
			champ.StateTimer = 1
			gl.reconcileMonsterAttackPosts()
			g.combat.HandleMonsterInteractions()
		case "summon":
			g.combat.HandleMonsterInteractions()
		case "tb_carry":
			runOneMonsterTurn(g, gl)
		}
		return champ.AttackCDFrames, champ.OffHandCDFrames
	}
	for _, entry := range []string{"party", "summon", "tb_carry"} {
		t.Run(entry, func(t *testing.T) {
			baseMain, baseOff := measure(t, entry, false)
			slowMain, slowOff := measure(t, entry, true)
			if baseMain <= 0 || baseOff <= 0 {
				t.Fatalf("unslowed control did not arm both hands: main=%d off=%d", baseMain, baseOff)
			}
			for _, hand := range []struct {
				name       string
				base, slow int
			}{{"main", baseMain, slowMain}, {"off", baseOff, slowOff}} {
				if want := int(math.Round(float64(hand.base) * factor)); hand.slow != want {
					t.Errorf("%s hand cooldown %d under slow, want %d (unslowed %d)", hand.name, hand.slow, want, hand.base)
				}
			}
		})
	}
}

// Heroes share the monster stun DR: 100/50/25/0 inside the reset window, and a
// full stun again once the hero stays stun-free for the whole window.
func TestHeroStunDiminishingReturns(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, tail := range []string{"chain", "window_minus_one", "window", "saved", "rest"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, tail), func(t *testing.T) {
				g, _, wm := clockSaveFixture(t)
				g.turnBasedMode = tb
				tps := g.config.GetTPS()
				c := g.party.Members[0]
				c.Conditions = nil
				mob := monster.NewMonster3DFromConfig(400, 400, "goblin", g.config)
				mob.StunCharChance, mob.StunCharSeconds, mob.StunCharTurns = 1, 4, 4
				full, window := 4*tps, status.StunDRResetSeconds*tps
				if tb {
					full, window = 4, status.StunDRResetTurns
				}
				advance := func() {
					if tb {
						g.startPartyTurn()
					} else {
						g.updatePartyClocks()
					}
				}
				stun := func() int {
					g.combat.applyMonsterHitRiders(mob, "", heroHitTarget{g.combat, c})
					got := c.StunFramesRemaining
					if tb {
						got = c.StunTurnsRemaining
					}
					for c.IsStunned() {
						advance()
					}
					return got
				}
				chain := []int{full, (full + 1) / 2, (full + 3) / 4, 0}
				switch tail {
				case "saved":
					stun()
					stun()
					reloadStatusFixture(t, g, wm)
					c = g.party.Members[0]
					if got := stun(); got != chain[2] {
						t.Fatalf("third stun after reload %d, want %d", got, chain[2])
					}
					return
				}
				for i, want := range chain {
					if got := stun(); got != want {
						t.Fatalf("stun %d landed %d, want %d", i+1, got, want)
					}
				}
				if countCombatLog(g, c.Name+" resists the stun!") != 1 {
					t.Fatal("immune step did not report the resist")
				}
				want := 0
				switch tail {
				case "window_minus_one":
					for range window - 1 {
						advance()
					}
				case "window":
					for range window {
						advance()
					}
					want = full
				case "rest":
					c.CureRestConditions()
					want = full
				default:
					return
				}
				if got := stun(); got != want {
					t.Fatalf("stun after %s landed %d, want %d", tail, got, want)
				}
			})
		}
	}
}

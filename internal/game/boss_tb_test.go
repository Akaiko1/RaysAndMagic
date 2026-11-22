package game

import (
	"encoding/json"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"
	"ugataima/internal/threading/monitoring"
	"ugataima/internal/world"
)

// newBossTestGame builds a 6x6 open world with one quest-gated evasive boss
// placed far from the party (outside its evade radius), so only a party blow
// can trigger a blink.
func newBossTestGame(t *testing.T, cfg *config.Config) (*MMGame, *monsterPkg.Monster3D) {
	t.Helper()
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 6, 6
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := 0; y < w.Height; y++ {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := 0; x < w.Width; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	g := newTestGame(cfg, w)
	g.turnBasedMode = true

	boss := &monsterPkg.Monster3D{
		ID: "boss_tb_1", Name: "Golden Thief Bug",
		Boss: true,
		X:    352, Y: 352, // tile (5,5) center; party camera sits at (64,64)
		HitPoints: 200, MaxHitPoints: 200,
		PassiveUntilQuest: "culverts_valves", // no quest manager in test -> evasive
		EvadeRadiusTiles:  3,
		BossCooldownSecs:  1,
	}
	w.Monsters = append(w.Monsters, boss)
	g.collisionSystem.RegisterEntity(collision.NewEntity(boss.ID, boss.X, boss.Y, 32, 32, collision.CollisionTypeMonster, false))
	return g, boss
}

// strikeBoss lands 10 physical damage on m through the party attack boundary
// every hero hit takes. With hero unset it is no hero's blow, as zones and
// card procs land theirs.
func strikeBoss(cs *CombatSystem, m *monsterPkg.Monster3D, hero bool) {
	attack := cs.newPartyMonsterAttack(10, 0, "physical", 0, nil, "Blow", false, false, true)
	if hero {
		attack.byHero(cs.game.party.Members[0])
	}
	cs.applyPartyMonsterAttack(m, attack)
}

// The evasive boss dodges MID-party-round in TB, on the first frame after it
// loses HP to anything, instead of waiting for the monster turn (which let a
// focused party round kill it before it ever dodged). None of it waits for a
// summon roll.
func TestEvasiveBossTB_BlinksWhenHurtMidPartyRound(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, tc := range []struct {
		name   string
		blow   func(g *MMGame, boss *monsterPkg.Monster3D)
		blinks bool
	}{
		{name: "hero attack", blow: func(g *MMGame, b *monsterPkg.Monster3D) { strikeBoss(g.combat, b, true) }, blinks: true},
		{name: "zone", blow: func(g *MMGame, b *monsterPkg.Monster3D) { strikeBoss(g.combat, b, false) }, blinks: true},
		{name: "poison", blow: func(g *MMGame, b *monsterPkg.Monster3D) {
			b.ApplyPoison(10 * g.config.GetTPS())
			g.tickMonsterTurnStatuses(b, true)
		}, blinks: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, boss := newBossTestGame(t, cfg)
			g.combat = NewCombatSystem(g)
			g.combat.tickEvasiveBossesTB()
			if boss.BossCD.Active() {
				t.Fatalf("boss far from party must not blink unprovoked")
			}
			tc.blow(g, boss)
			if boss.HitPoints >= boss.MaxHitPoints {
				t.Fatal("fixture: the blow took no HP")
			}
			g.combat.tickEvasiveBossesTB()
			if boss.BossCD.Active() != tc.blinks {
				t.Fatalf("blinked = %v, want %v", boss.BossCD.Active(), tc.blinks)
			}
			if boss.BossHurtPending || boss.BossBlowRolls != 0 {
				t.Errorf("no blow may stay owed after the frame, nor wait for a summon roll")
			}
		})
	}
}

// Crowd control suppresses the per-frame evasive blink like any other action.
func TestEvasiveBossTB_CrowdControlSuppressesBlink(t *testing.T) {
	cfg := loadTestConfig(t)

	cases := []struct {
		name string
		cc   func(m *monsterPkg.Monster3D)
	}{
		{"stun_turns", func(m *monsterPkg.Monster3D) { m.Stun.Turns = 1 }},
		{"stun_frames", func(m *monsterPkg.Monster3D) { m.Stun.Frames = 10 }},
		{"charm", func(m *monsterPkg.Monster3D) { m.Pacified = true }},
		{"bind", func(m *monsterPkg.Monster3D) { m.Bound = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, boss := newBossTestGame(t, cfg)
			g.combat = NewCombatSystem(g)
			tc.cc(boss)
			strikeBoss(g.combat, boss, true)
			g.combat.tickEvasiveBossesTB()
			if boss.BossCD.Active() {
				t.Fatalf("%s boss must not blink while controlled", tc.name)
			}
		})
	}
}

// The hurt-blink cooldown's TB round starts with the party turn: a blink in
// one round, even on the monster turn, leaves the boss ready to dodge the
// next party round's first blow, and not before that round.
func TestEvasiveBossTB_BlinkReadyEachPartyRound(t *testing.T) {
	cfg := loadTestConfig(t)
	g, boss := newBossTestGame(t, cfg)
	g.combat = NewCombatSystem(g)
	// A blink consumes the blow; a refused one leaves it owed.
	blink := func() bool {
		strikeBoss(g.combat, boss, true)
		g.combat.tickEvasiveBossesTB()
		return !boss.BossHurtPending
	}
	if !blink() {
		t.Fatal("fixture: the first blow did not blink")
	}
	// The monster turn spends the ability clocks, not the blink's.
	g.tickMonsterTurnStatuses(boss, true)
	if blink() {
		t.Fatal("blinked twice in one round")
	}
	g.startPartyTurn()
	if !blink() {
		t.Fatalf("not ready to dodge in the next party round: %+v", boss.BossCD)
	}
	// A round with no blow, then a blink on the monster turn.
	g.startPartyTurn()
	g.tickMonsterTurnStatuses(boss, true)
	if !blink() {
		t.Fatal("fixture: no blink on the monster turn")
	}
	g.startPartyTurn()
	if !blink() {
		t.Fatal("a monster-turn blink kept the boss from dodging the next party round")
	}
}

// newSummonBoss spawns an aggressive boss whose every summon roll succeeds,
// one add a summon and no live cap, with no mitigation so each blow takes HP,
// and party members that outlast any test.
func newSummonBoss(t *testing.T, tileX, tileY int) (*MMGame, *GameLoop, *monsterPkg.Monster3D) {
	t.Helper()
	g, gl := newSpecialsTestGame(t)
	for _, c := range g.party.Members {
		c.MaxHitPoints, c.HitPoints = 100000, 100000
	}
	m := spawnSpecialsMonster(g, "alien_enforcer", tileX, tileY)
	if !m.IsBoss() || len(m.Abilities.Summon.Monsters) == 0 {
		t.Fatal("fixture: alien_enforcer must be a summoning boss")
	}
	m.Abilities.Teleport.Chance, m.Abilities.Stun.Chance = 0, 0
	m.Abilities.Summon.Chance, m.Abilities.Summon.FirstGuaranteed = 1, false
	m.Abilities.Summon.Count, m.Abilities.Summon.Max = 1, 0
	m.MaxHitPoints, m.HitPoints = 100000, 100000
	m.ArmorClass, m.PerfectDodge, m.Resistances = 0, 0, nil
	return g, gl, m
}

// shootBoss lands a party arrow on m through the real impact path: the hero's
// own shot, or a card-proc bolt (the Bandit Card's) when own is unset.
func shootBoss(g *MMGame, m *monsterPkg.Monster3D, own bool) {
	arrow := Arrow{Attacker: g.party.Members[0], HeroBlow: own, Damage: 10, DamageType: "physical",
		Owner: ProjectileOwnerPlayer, Active: true, LifeTime: 10, BowKey: "hunting_bow"}
	if !own {
		arrow.Label = "Bonus Bolt"
	}
	g.combat.applyProjectileDamage(&arrow, m, "")
}

func summonCount(g *MMGame) int { return countCombatLog(g, "raises the war-banner") }

// monsterFrame is one real world step of the running mode: monsters, shots,
// then the bosses' answers to the frame's blows.
func monsterFrame(t *testing.T, g *MMGame, gl *GameLoop) {
	t.Helper()
	if g.reusableDeadSet == nil { // NewMMGame normally allocates these
		g.reusableDeadSet = make(map[string]bool)
		g.reusableEncounterRewardsMap = make(map[*monsterPkg.EncounterRewards]int)
	}
	if g.threading == nil {
		g.threading = &threading.ThreadingComponents{
			EntityUpdater:      entities.NewEntityUpdaterWithWorkers(1),
			PerformanceMonitor: monitoring.NewPerformanceMonitor(),
		}
		t.Cleanup(g.threading.Shutdown)
	}
	gl.runWorldFrame()
}

// partyHP is the party's total HP: what a landed boss attack takes.
func partyHP(g *MMGame) int {
	total := 0
	for _, c := range g.party.Members {
		total += c.HitPoints
	}
	return total
}

// A hero's attack that takes HP off a boss rolls its summon on the next frame,
// in both modes, at most once a round on the shared clock (from each party
// turn in TB, 3 s in RT). A blow the boss cannot answer is not kept for later,
// and no other damage rolls. At the live cap nothing rolls and the round's
// roll is not spent.
func TestBossSummonRollsOnPartyBlows(t *testing.T) {
	type fixture struct {
		t  *testing.T
		g  *MMGame
		m  *monsterPkg.Monster3D
		tb bool
	}
	newRound := func(f fixture) {
		if f.tb {
			f.g.startPartyTurn()
			return
		}
		// The real RT loop spends the round; its attack moments roll nothing.
		f.m.Abilities.Summon.Chance = 0
		runRTCombatSeconds(f.g, config.CombatRoundSeconds)
		f.m.Abilities.Summon.Chance = 1
	}
	hero := func(f fixture) { strikeBoss(f.g.combat, f.m, true) }
	// lastBlow is a TB hero's blow that spends the party's last action: the
	// frame after it runs the whole monster turn and opens the next round.
	lastBlow := func(f fixture) {
		f.m.AttackSummonCD.Arm(100*config.CombatRoundSeconds, f.g.config.GetTPS()) // its attack rolls nothing
		hero(f)
		for _, c := range f.g.party.Members {
			c.ActionsRemaining = 0
		}
	}
	nextRoundOpened := func(f fixture) {
		if f.g.partyAllExhausted() {
			f.t.Fatal("fixture: the frame did not open the next party round")
		}
	}
	// shotInFlight puts the hero's arrow on the boss: it lands in the frame's
	// shot pass, after the monster turn. The party stands next to the boss, so
	// that turn is a swing, not a step off the shot's line.
	const shotID = "shot_in_flight"
	shotInFlight := func(f fixture) {
		f.g.setPartyPosition(f.m.X, f.m.Y-float64(f.g.config.GetTileSize()))
		f.g.arrows = append(f.g.arrows, Arrow{ID: shotID, Attacker: f.g.party.Members[0], HeroBlow: true,
			Damage: 10, DamageType: "physical", Owner: ProjectileOwnerPlayer, Active: true, LifeTime: 10,
			BowKey: "hunting_bow", X: f.m.X, Y: f.m.Y})
		f.g.collisionSystem.RegisterEntity(collision.NewEntity(shotID, f.m.X, f.m.Y, 16, 16, collision.CollisionTypeProjectile, false))
	}
	shotLanded := func(f fixture) {
		for _, a := range f.g.arrows {
			if a.ID == shotID && a.Active {
				f.t.Fatal("fixture: the shot in flight did not land")
			}
		}
	}
	type step struct {
		act   func(f fixture)
		want  int           // summons so far, after the frame that follows the act
		after func(fixture) // fixture checks after that frame
	}
	held := func(on, off func(m *monsterPkg.Monster3D)) []step {
		return []step{
			{act: func(f fixture) { on(f.m); hero(f) }},
			{act: func(f fixture) { off(f.m) }}, // the blow was not kept
			{act: hero, want: 1},                // a fresh blow rolls
		}
	}
	for _, tc := range []struct {
		name  string
		rtToo bool // the row runs in RT as well as TB
		steps []step
	}{
		{name: "hero attack, once a round", rtToo: true, steps: []step{
			{act: hero, want: 1},
			{act: hero, want: 1},
			{act: func(f fixture) { newRound(f); hero(f) }, want: 2},
		}},
		{name: "graze through a perfect dodge", rtToo: true, steps: []step{{act: func(f fixture) {
			attack := f.g.combat.newPartyMonsterAttack(0, 20, "physical", 0, nil, "Blow", false, false, true)
			attack.byHero(f.g.party.Members[0])
			if !f.g.combat.applyDamageThroughDodge(f.m, attack, attack.Attacker, "Hero", nil) {
				f.t.Fatal("fixture: nothing came through the dodge")
			}
		}, want: 1}}},
		{name: "hero's arrow", rtToo: true, steps: []step{{act: func(f fixture) { shootBoss(f.g, f.m, true) }, want: 1}}},
		{name: "card-proc bolt", steps: []step{{act: func(f fixture) { shootBoss(f.g, f.m, false) }}}},
		{name: "zone or card proc", steps: []step{{act: func(f fixture) { strikeBoss(f.g.combat, f.m, false) }}}},
		{name: "rider (Toll echo)", steps: []step{{act: func(f fixture) {
			attack := f.g.combat.newPartyMonsterAttack(10, 0, "physical", 0, nil, "Toll", false, false, true)
			attack.Attacker = f.g.party.Members[0] // copied from the hero's hit, no blow of its own
			f.g.combat.applyPartyMonsterAttack(f.m, attack)
		}}}},
		{name: "poison", steps: []step{{act: func(f fixture) {
			f.m.ApplyPoison(10 * f.g.config.GetTPS())
			f.g.tickMonsterTurnStatuses(f.m, true)
		}}}},
		{name: "trap", steps: []step{{act: func(f fixture) { f.g.combat.applyTrapDamage(f.m, 50, "fire", "Trap") }}}},
		{name: "ally", steps: []step{{act: func(f fixture) {
			ally := spawnSpecialsMonster(f.g, "goblin", 9, 8)
			ally.Bound, ally.BoundClock, ally.AIFoe = true, framesClock(100000), f.m
			ally.DamageMin, ally.DamageMax = 50, 50
			f.g.combat.performMonsterAttackAgainstMonster(ally, f.m, ProjectileOwnerPartyAlly)
		}}}},
		{name: "killed by the blow", steps: []step{{act: func(f fixture) { f.m.HitPoints = 10; hero(f) }}}},
		{name: "stunned", steps: held(func(m *monsterPkg.Monster3D) { m.Stun.Turns, m.Stun.Frames = 1, 60 },
			func(m *monsterPkg.Monster3D) { m.Stun.Turns, m.Stun.Frames = 0, 0 })},
		{name: "silenced", steps: held(func(m *monsterPkg.Monster3D) { m.Silence.Turns = 1 },
			func(m *monsterPkg.Monster3D) { m.Silence.Turns = 0 })},
		{name: "afraid", steps: held(func(m *monsterPkg.Monster3D) { m.Fear.Turns = 1 },
			func(m *monsterPkg.Monster3D) { m.Fear.Turns = 0 })},
		// The blow is judged when it lands, not by the round the frame ends in.
		{name: "stun runs out in the frame of the blow", rtToo: true, steps: []step{
			{act: func(f fixture) {
				if !f.tb {
					f.m.Stun.ArmFrames(1, f.g.config.GetTPS())
					hero(f)
					return
				}
				f.m.Stun.Arm(config.CombatRoundSeconds, f.g.config.GetTPS())
				lastBlow(f)
			}, after: func(f fixture) {
				if f.m.Stun.Active() {
					f.t.Fatal("fixture: the stun outlived the frame")
				}
			}},
			{act: hero, want: 1},
		}},
		{name: "the round's last blow after its roll", steps: []step{
			{act: hero, want: 1},
			{act: lastBlow, want: 1, after: nextRoundOpened},
			{act: hero, want: 2},
		}},
		{name: "the round's last blow is its first", steps: []step{
			{act: lastBlow, want: 1, after: nextRoundOpened},
			{act: hero, want: 2},
		}},
		{name: "a shot lands once the round's last blow opened the next", steps: []step{
			{act: func(f fixture) { lastBlow(f); shotInFlight(f) }, want: 2,
				after: func(f fixture) { nextRoundOpened(f); shotLanded(f) }},
			{act: hero, want: 2}, // the shot spent the new round's roll
		}},
		{name: "at the live cap", rtToo: true, steps: []step{
			{act: func(f fixture) { f.m.Abilities.Summon.Max = 1; hero(f) }, want: 1},
			{act: func(f fixture) { newRound(f); hero(f) }, want: 1},
			{act: func(f fixture) {
				if f.m.HurtSummonCD.Active() {
					f.t.Fatal("a blow at the cap spent the round's roll")
				}
				for _, add := range f.g.world.Monsters {
					if add.SummonedBy == f.m.ID {
						add.HitPoints = 0
					}
				}
				hero(f)
			}, want: 2},
		}},
	} {
		for _, tb := range []bool{true, false} {
			if !tb && !tc.rtToo {
				continue
			}
			mode := "rt"
			if tb {
				mode = "tb"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				g, gl, m := newSummonBoss(t, 9, 9)
				f := fixture{t: t, g: g, m: m, tb: tb}
				if tb {
					g.turnBasedMode = true
					g.startPartyTurn()
				}
				for i, s := range tc.steps {
					before := m.HitPoints
					s.act(f)
					if i == 0 && m.HitPoints >= before {
						t.Fatal("fixture: the first blow took no HP")
					}
					monsterFrame(t, g, gl)
					if s.after != nil {
						s.after(f)
					}
					if got := summonCount(g); got != s.want {
						t.Fatalf("step %d: summons = %d, want %d", i, got, s.want)
					}
					if tb && g.currentTurn != 0 {
						t.Fatal("fixture: the party turn ended")
					}
				}
			})
		}
	}
}

// holdAdds stuns the boss's adds: they count toward the cap and hurt nobody,
// so every hit on the party is the boss's own.
func holdAdds(g *MMGame, boss *monsterPkg.Monster3D) {
	for _, add := range g.world.Monsters {
		if add.SummonedBy == boss.ID {
			add.Stun.Turns, add.Stun.Frames = 50, 50*g.config.GetTPS()
		}
	}
}

// The attack roll happens at most once a round and takes the attack's place
// (in TB the whole action); a summon on a party blow spends neither the
// attack nor its roll; at the live cap no roll happens and the attack lands.
// Driven through the real TB monster turn of a boss next to the party.
func TestBossSummonRollsOnItsAttack(t *testing.T) {
	for _, tc := range []struct {
		name         string
		max          int
		blowFirst    bool // a hero's blow in the party turn before
		extraAction  bool // the monster turn has a second pass
		turns        int  // monster turns; 1 unless set
		wantSummons  int
		wantAttacked bool
	}{
		{name: "the summon replaces the attack", wantSummons: 1},
		{name: "at the cap the attack lands", max: 1, blowFirst: true, wantSummons: 1, wantAttacked: true},
		{name: "a blow's summon leaves the attack roll", max: 2, blowFirst: true, wantSummons: 2},
		{name: "a second action in the round rolls nothing", extraAction: true, wantSummons: 1, wantAttacked: true},
		{name: "the next round rolls again", turns: 2, wantSummons: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, gl, m := newSummonBoss(t, 3, 2)
			m.Abilities.Summon.Max = tc.max
			g.turnBasedMode = true
			g.startPartyTurn()
			if tc.blowFirst {
				strikeBoss(g.combat, m, true)
				monsterFrame(t, g, gl)
			}
			hp := partyHP(g)
			for range max(1, tc.turns) {
				holdAdds(g, m)
				g.turnBasedExtraMonsterAction = tc.extraAction
				runTBMonsterTurns(g, gl, 1)
				if tc.extraAction {
					holdAdds(g, m)
					g.turnBasedMonsterPassDelay = 0
					gl.updateMonstersTurnBased()
					if !g.monsterTurnResolved {
						t.Fatal("fixture: the second pass did not end the turn")
					}
				}
			}
			if got := summonCount(g); got != tc.wantSummons {
				t.Fatalf("summons = %d, want %d", got, tc.wantSummons)
			}
			if attacked := partyHP(g) < hp; attacked != tc.wantAttacked {
				t.Fatalf("boss attacked = %v, want %v", attacked, tc.wantAttacked)
			}
		})
	}
}

// In real time the attack roll is once a round too: the first attack moment
// summons in the attack's place, the boss's further attacks of those 3 s
// land, and the next round's first attack rolls again. Real RT frames.
func TestBossSummonAttackRollOnceARoundRT(t *testing.T) {
	g, gl, m := newSummonBoss(t, 3, 2)
	tps := g.config.GetTPS()
	frame := func() {
		monsterFrame(t, g, gl)
		holdAdds(g, m)
	}
	for f := 0; summonCount(g) == 0; f++ {
		if f > 3*tps {
			t.Fatal("fixture: no attack moment in 3 s next to the party")
		}
		frame()
	}
	hp := partyHP(g)
	for range config.CombatRoundSeconds*tps - tps/2 {
		frame()
	}
	if got := summonCount(g); got != 1 {
		t.Fatalf("summons within the round = %d, want 1", got)
	}
	if partyHP(g) >= hp {
		t.Fatal("the boss stopped attacking after its summon")
	}
	for range 2 * tps {
		frame()
	}
	if got := summonCount(g); got != 2 {
		t.Fatalf("summons after the round = %d, want 2", got)
	}
}

// The round's rolls and a blow's claimed roll survive a save: the claim rolls
// after the load, and another blow in that round rolls nothing, as without
// the save.
func TestBossBlowSummonClockAcrossSaveLoad(t *testing.T) {
	g, wm, _ := travelFixture(t)
	m := monsterPkg.NewMonster3DFromConfig(5*64+32, 5*64+32, "alien_enforcer", g.config)
	m.IsEngagingPlayer, m.WasAttacked = true, true
	m.ArmorClass, m.PerfectDodge, m.Resistances = 0, 0, nil
	m.Abilities.Summon.Chance, m.Abilities.Summon.Max = 1, 0
	g.world.Monsters = []*monsterPkg.Monster3D{m}
	strikeBoss(g.combat, m, true) // claims the round's roll, not rolled yet
	clock := m.HurtSummonCD
	if !clock.Active() || m.BossBlowRolls != 1 {
		t.Fatal("fixture: the blow claimed no roll")
	}
	m.AttackSummonCD.Arm(config.CombatRoundSeconds, g.config.GetTPS()) // an attack rolled this round too
	attackClock := m.AttackSummonCD

	save := g.buildSave(wm)
	data, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	var loaded GameSave
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := g.applySave(wm, &loaded); err != nil {
		t.Fatal(err)
	}
	var boss *monsterPkg.Monster3D
	for _, o := range g.world.Monsters {
		if o.IsBoss() {
			boss = o
		}
	}
	if boss == nil {
		t.Fatal("fixture: the boss did not come back")
	}
	if boss.HurtSummonCD != clock || boss.AttackSummonCD != attackClock || boss.BossBlowRolls != 1 {
		t.Fatalf("after load: clocks %+v %+v owed %d, want %+v %+v and the blow's roll owed",
			boss.HurtSummonCD, boss.AttackSummonCD, boss.BossBlowRolls, clock, attackClock)
	}
	boss.Abilities.Summon.Chance, boss.Abilities.Summon.Max = 1, 0 // the load rebuilds the kit from config
	summons := summonCount(g)
	g.combat.answerBossHurt()
	if got := summonCount(g); got != summons+1 || boss.BossBlowRolls != 0 {
		t.Fatalf("the claimed roll after the load: %d summons, owed %d, want %d", got, boss.BossBlowRolls, summons+1)
	}
	strikeBoss(g.combat, boss, true)
	g.combat.answerBossHurt()
	if got := summonCount(g); got != summons+1 {
		t.Fatalf("another blow in the round rolled: %d summons, want %d", got, summons+1)
	}
}

package game

import (
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
)

// Regression: a monster's TB stun (StunTurnsRemaining) expiring must also clear
// its RT counterpart (StunFramesRemaining) — that field only ticks down inside
// the RT-only Monster3D.Update, which never runs while turnBasedMode is true, so
// it was left stuck nonzero after the TB stun wore off. The stun-star overlay
// and bossDisabled both read "StunFramesRemaining > 0 OR StunTurnsRemaining > 0",
// so the stars kept floating over a monster that could already act again.
// Mirrors character.TickStunTurn, which already zeroes its RT counterpart.
func TestMonsterTBStunExpiry_ClearsRTCounterpartToo(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	game.turnBasedMode = true

	m := &monsterPkg.Monster3D{ID: "goblin_1", Name: "Goblin", X: 64, Y: 0, HitPoints: 10, MaxHitPoints: 10}
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	// A 1-TB-turn stun with an RT component, as every stun source in this
	// codebase authors it (spells pair stun_duration_seconds with _turns;
	// weapon/card stun always defaults turns to at least 1).
	cs.applyStunDR(m, 1, 2*game.config.GetTPS(), false)
	if m.StunTurnsRemaining != 1 || m.StunFramesRemaining <= 0 {
		t.Fatalf("setup: expected both counters set, got turns=%d frames=%d", m.StunTurnsRemaining, m.StunFramesRemaining)
	}

	gl := &GameLoop{game: game}
	game.currentTurn = 1
	game.monsterTurnResolved = false
	gl.updateMonstersTurnBased() // the monster's one stunned turn

	if m.StunTurnsRemaining != 0 {
		t.Fatalf("expected TB stun to expire after its one turn, got %d", m.StunTurnsRemaining)
	}
	if m.StunFramesRemaining != 0 {
		t.Errorf("StunFramesRemaining still %d after TB stun expired — stun-star overlay and bossDisabled would still read this monster as stunned", m.StunFramesRemaining)
	}
}

// Regression: the mirror image of the test above — a monster's RT stun
// (StunFramesRemaining) expiring must also clear its TB counterpart
// (StunTurnsRemaining). A trap or any other stun source that authors both
// stun_turns/stun_seconds (every one in this codebase does) leaves
// StunTurnsRemaining stuck nonzero forever in PURE RT play (never entering TB
// at all) — nothing decrements it outside the TB scheduler. Reported live: a
// bandit stunned by a Stasis Trap in real-time recovered and walked off with
// the stun-star overlay still floating over it, because that overlay reads
// "StunFramesRemaining > 0 OR StunTurnsRemaining > 0". Mirrors
// character.tickStunFrames, which already zeroes its TB counterpart.
func TestMonsterRTStunExpiry_ClearsTBCounterpartToo(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	game.turnBasedMode = false // pure RT — this player never touches TB

	m := &monsterPkg.Monster3D{ID: "bandit_1", Name: "Bandit", X: 64, Y: 0, HitPoints: 10, MaxHitPoints: 10}
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	// Stasis Trap: stun_turns=1, stun_seconds=2 — authored as a pair, like
	// every stun source in this codebase.
	tps := game.config.GetTPS()
	cs.applyStunDR(m, 1, 2*tps, false)
	if m.StunTurnsRemaining <= 0 || m.StunFramesRemaining <= 0 {
		t.Fatalf("setup: expected both counters set, got turns=%d frames=%d", m.StunTurnsRemaining, m.StunFramesRemaining)
	}

	mw := &MonsterWrapper{Monster: m, collisionSystem: game.collisionSystem, game: game}
	for i := 0; i < 2*tps+1; i++ { // outlast the RT stun duration
		mw.snapshot = game.collisionSystem.Snapshot()
		mw.Update()
		mw.ApplyCollisionUpdate()
	}

	if m.StunFramesRemaining != 0 {
		t.Fatalf("expected RT stun to expire after %d frames, got %d remaining", 2*tps, m.StunFramesRemaining)
	}
	if m.StunTurnsRemaining != 0 {
		t.Errorf("StunTurnsRemaining still %d after RT stun expired in pure RT play — stun-star overlay would still read this monster as stunned", m.StunTurnsRemaining)
	}
}

// Regression: a monster's RT poison DoT ticks inside the parallel
// Monster3D.Update (TickPoison), which has no CombatSystem in scope to run
// finishMonsterKill itself. Before finalizeIndirectKills existed, a monster
// poisoned to death this way never got queued in deadMonsterIDs — no XP/loot,
// no quest kill-credit, no band scatter, and its collision entity was never
// unregistered (removeDeadMonstersByID only clears queued IDs), leaving a
// permanent invisible collision blocker on the map.
func TestMonsterPoisonKillRT_FinalizedByIndirectSweep(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	game.turnBasedMode = false

	m := &monsterPkg.Monster3D{ID: "rt_poison_victim", Name: "Rat", X: 64, Y: 0, HitPoints: 1, MaxHitPoints: 1, Experience: 5}
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	tps := game.config.GetTPS()
	m.ApplyPoison(2 * tps)

	mw := &MonsterWrapper{Monster: m, collisionSystem: game.collisionSystem, game: game}
	for i := 0; i < tps+1; i++ { // outlast one poison tick (1/sec)
		mw.snapshot = game.collisionSystem.Snapshot()
		mw.Update()
		mw.ApplyCollisionUpdate()
	}
	if m.IsAlive() {
		t.Fatalf("setup: expected the 1-HP monster to die from its first poison tick, got HP %d", m.HitPoints)
	}

	game.reusableDeadSet = make(map[string]bool) // NewMMGame normally allocates this
	gl := &GameLoop{game: game}
	before := len(game.deadMonsterIDs)
	gl.finalizeIndirectKills()
	if len(game.deadMonsterIDs) != before+1 {
		t.Fatalf("expected the poison-killed monster to be finalized, deadMonsterIDs went from %d to %d", before, len(game.deadMonsterIDs))
	}

	// A second sweep this same frame must not re-finalize an already-queued kill
	// (double XP/gold).
	gl.finalizeIndirectKills()
	if len(game.deadMonsterIDs) != before+1 {
		t.Error("finalizeIndirectKills re-queued an already-queued dead monster")
	}
}

// Regression: TB's per-monster loop rolled TickPoisonTurn but never re-checked
// IsAlive before falling through to root/boss/attack handling for the same
// monster in the same pass, so a monster that died to its own poison tick could
// still act on its death turn. finalizeIndirectKills (called once per frame from
// the main Update loop, same as here) is what actually finishes the kill.
func TestMonsterPoisonKillTB_FinalizesKill(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	game.turnBasedMode = true

	m := &monsterPkg.Monster3D{ID: "tb_poison_victim", Name: "Rat", X: 64, Y: 0, HitPoints: 1, MaxHitPoints: 1, Experience: 5}
	m.ApplyPoison(2 * game.config.GetTPS())
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.reusableDeadSet = make(map[string]bool) // NewMMGame normally allocates this
	gl := &GameLoop{game: game}
	game.currentTurn = 1
	game.monsterTurnResolved = false
	before := len(game.deadMonsterIDs)
	gl.updateMonstersTurnBased()

	if m.IsAlive() {
		t.Fatalf("setup: expected the TB poison tick to kill the 1-HP monster, got HP %d", m.HitPoints)
	}

	gl.finalizeIndirectKills() // same-frame sweep, as the real Update loop runs it
	if len(game.deadMonsterIDs) != before+1 {
		t.Error("TB poison kill should register in deadMonsterIDs via finalizeIndirectKills — monster was never finalized")
	}
}

func TestMonsterPoisonAppliesStatus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)

	// Melee picks a random LIVING member — KO everyone but slot 0 so the hit is
	// deterministic, then assert that survivor gets poisoned.
	for i, m := range cs.game.party.Members {
		if i != 0 {
			m.HitPoints = 0
		}
	}
	target := cs.game.party.Members[0]
	target.Luck = 0
	target.HitPoints = target.MaxHitPoints

	monster := &monsterPkg.Monster3D{
		Name:              "Troll",
		DamageMin:         1,
		DamageMax:         1,
		PoisonChance:      1.0,
		PoisonDurationSec: 5,
	}

	cs.applyMonsterMeleeDamage(monster)

	if !target.HasCondition(character.ConditionPoisoned) {
		t.Fatalf("expected target to be poisoned")
	}
	expectedFrames := cs.game.config.GetTPS() * monster.PoisonDurationSec
	if target.PoisonFramesRemaining != expectedFrames {
		t.Fatalf("expected poison frames %d, got %d", expectedFrames, target.PoisonFramesRemaining)
	}
}

func TestMonsterFireburstDamagesParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)

	for _, member := range cs.game.party.Members {
		member.HitPoints = 50
	}

	monster := &monsterPkg.Monster3D{
		Name:               "Pixie",
		DamageMin:          1,
		DamageMax:          1,
		FireburstChance:    1.0,
		FireburstDamageMin: 6,
		FireburstDamageMax: 6,
	}

	cs.applyMonsterMeleeDamage(monster)

	for i, member := range cs.game.party.Members {
		if member.HitPoints != 44 {
			t.Fatalf("member %d expected HP 44 after fireburst, got %d", i, member.HitPoints)
		}
	}
}

// Regression: antivenom (cure_poison) must stop the background HP drain, not
// just clear the Poisoned icon. RemoveCondition alone left PoisonFramesRemaining
// ticking, so the cure looked like it worked while damage kept landing.
func TestCurePoison_StopsBackgroundTick(t *testing.T) {
	tps := config.GetTargetTPS()
	member := &character.MMCharacter{HitPoints: 100, MaxHitPoints: 100}
	member.ApplyPoison(tps * 10)
	if !member.HasCondition(character.ConditionPoisoned) {
		t.Fatal("expected Poisoned condition after ApplyPoison")
	}

	member.CurePoison()
	if member.HasCondition(character.ConditionPoisoned) {
		t.Error("CurePoison should clear the Poisoned condition")
	}

	before := member.HitPoints
	for i := 0; i < tps*3; i++ {
		member.UpdateWithMode(true) // TB path ticks poison directly every frame too
	}
	if member.HitPoints != before {
		t.Errorf("HP dropped from %d to %d after cure — background poison timer kept ticking", before, member.HitPoints)
	}
}

// Regression: startPartyTurn's TB poison/burn tick can drop a member to 0 HP,
// and the lethal-DoT KO sweep must run BEFORE ActionsRemaining is handed out —
// not next frame, from the main loop's separate knockOutLethalDoTVictims call.
// Reported failure mode: a member the tick killed showed 0 HP with no
// Unconscious condition and no "falls unconscious!" message until the frame
// after the round had already started (and, with a Lich Card, the save would
// land a frame too late to un-zero the member before ActionsRemaining=0 was
// already locked in for that round).
func TestStartPartyTurn_KOsLethalDoTBeforeActionSlots(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true

	member := g.party.Members[0]
	member.HitPoints, member.MaxHitPoints = 1, 100
	member.ApplyPoison(g.config.GetTPS()) // one tick, due immediately this turn
	member.ActionsRemaining = 1           // prove the sweep, not the zero default, causes 0 below

	msgsBefore := len(g.combatLogHistory)
	g.startPartyTurn()

	if member.HitPoints != 0 {
		t.Fatalf("setup: expected the TB poison tick to zero HP, got %d", member.HitPoints)
	}
	if !member.HasCondition(character.ConditionUnconscious) {
		t.Error("expected startPartyTurn to KO the lethally-poisoned member synchronously (no Lich Card = no save), not defer it to next frame")
	}
	if member.ActionsRemaining != 0 {
		t.Errorf("ActionsRemaining = %d, want 0 for a member KO'd this same turn", member.ActionsRemaining)
	}
	if len(g.combatLogHistory) <= msgsBefore {
		t.Error("expected the knockOut message to be logged synchronously within startPartyTurn")
	}
}

// Regression: a lethal poison/burn tick must go through the real knockOut
// (Lich Card save roll + "falls unconscious!" message), not silently flip
// Unconscious in the character package.
func TestLethalDoTTick_RoutesThroughKnockOut(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]

	member.HitPoints, member.MaxHitPoints = 1, 100
	member.ApplyPoison(config.GetTargetTPS()) // ticks once after ~1s
	for i := 0; i < config.GetTargetTPS(); i++ {
		member.UpdateWithMode(false)
	}
	if member.HitPoints != 0 {
		t.Fatalf("setup: expected the poison tick to zero HP, got %d", member.HitPoints)
	}
	if member.HasCondition(character.ConditionUnconscious) {
		t.Fatal("setup: updatePoison must not set Unconscious directly anymore")
	}

	msgsBefore := len(g.combatLogHistory)
	cs.knockOutLethalDoTVictims()
	if !member.HasCondition(character.ConditionUnconscious) && member.HitPoints == 0 {
		t.Error("knockOutLethalDoTVictims should have knocked the member out (no Lich Card = no save)")
	}
	if len(g.combatLogHistory) <= msgsBefore {
		t.Error("expected a combat message from the knockOut sweep")
	}
}

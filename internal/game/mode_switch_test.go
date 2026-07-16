package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestModeSwitch_PreservesRTCooldowns(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5) // starts in turn-based mode
	for i, member := range game.party.Members {
		member.RTCooldown = 40 + i
		member.OffHandRTCooldown = 20 + i
		member.NextTBAttackOffHand = i%2 == 0
	}
	game.spellInputCooldown = 9

	game.ToggleTurnBasedMode() // TB -> RT
	if game.turnBasedMode || !game.turnBasedTurnSuspended {
		t.Fatalf("TB -> RT = turnBased:%v suspended:%v, want false/true", game.turnBasedMode, game.turnBasedTurnSuspended)
	}
	assertModeSwitchCooldowns(t, game)

	game.ToggleTurnBasedMode() // RT -> TB, resume the prior turn
	if !game.turnBasedMode || game.turnBasedTurnSuspended {
		t.Fatalf("RT -> TB = turnBased:%v suspended:%v, want true/false", game.turnBasedMode, game.turnBasedTurnSuspended)
	}
	assertModeSwitchCooldowns(t, game)
}

func assertModeSwitchCooldowns(t *testing.T, game *MMGame) {
	t.Helper()
	for i, member := range game.party.Members {
		if want := 40 + i; member.RTCooldown != want {
			t.Errorf("member %d main RT cooldown = %d, want %d", i, member.RTCooldown, want)
		}
		if want := 20 + i; member.OffHandRTCooldown != want {
			t.Errorf("member %d off-hand RT cooldown = %d, want %d", i, member.OffHandRTCooldown, want)
		}
		if want := i%2 == 0; member.NextTBAttackOffHand != want {
			t.Errorf("member %d TB hand cursor = %v, want %v", i, member.NextTBAttackOffHand, want)
		}
	}
	if game.spellInputCooldown != 9 {
		t.Errorf("spell input cooldown = %d, want 9", game.spellInputCooldown)
	}
}

func TestModeSwitch_ResumesTurnBasedState(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	game.currentTurn = 0
	game.partyActionsUsed = 2
	game.party.Members[0].ActionsRemaining = 2
	game.party.Members[0].NextTBAttackOffHand = true
	game.party.Members[1].ActionsRemaining = 1
	for i := 2; i < len(game.party.Members); i++ {
		game.party.Members[i].ActionsRemaining = 0
	}

	game.ToggleTurnBasedMode()
	game.ToggleTurnBasedMode()

	if game.currentTurn != 0 || game.partyActionsUsed != 2 {
		t.Fatalf("resumed party turn = current:%d actionsUsed:%d, want 0/2", game.currentTurn, game.partyActionsUsed)
	}
	if got := game.party.Members[0].ActionsRemaining; got != 2 {
		t.Errorf("member 0 actions after resume = %d, want 2", got)
	}
	if got := game.party.Members[1].ActionsRemaining; got != 1 {
		t.Errorf("member 1 actions after resume = %d, want 1", got)
	}
	if !game.party.Members[0].NextTBAttackOffHand {
		t.Error("resuming the turn reset the dual-wield hand cursor")
	}
}

func TestModeSwitch_FreshTurnBasedEntryStartsRound(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	game.turnBasedMode = false
	game.currentTurn = 1
	game.partyActionsUsed = 2
	for _, member := range game.party.Members {
		member.ActionsRemaining = 0
	}

	game.ToggleTurnBasedMode()

	if !game.turnBasedMode || game.turnBasedTurnSuspended {
		t.Fatalf("fresh RT -> TB = turnBased:%v suspended:%v, want true/false", game.turnBasedMode, game.turnBasedTurnSuspended)
	}
	if game.currentTurn != 0 || game.partyActionsUsed != 0 {
		t.Fatalf("fresh TB turn = current:%d actionsUsed:%d, want 0/0", game.currentTurn, game.partyActionsUsed)
	}
	if game.party.Members[0].ActionsRemaining == 0 {
		t.Fatal("fresh TB entry did not grant the party an action")
	}
}

func TestModeSwitch_ResumesPendingMonsterTurn(t *testing.T) {
	game, gl, _ := tbBehaviorGame(t, 5, 5)
	game.currentTurn = 1
	game.monsterTurnResolved = false
	game.turnBasedExtraMonsterAction = true
	game.turnBasedMonsterPassesLeft = 1
	game.turnBasedMonsterPassDelay = 3
	game.turnBasedMonsterStatusTick = true

	game.ToggleTurnBasedMode()
	game.ToggleTurnBasedMode()

	if game.currentTurn != 1 || game.monsterTurnResolved {
		t.Fatalf("resumed monster turn = current:%d resolved:%v, want 1/false", game.currentTurn, game.monsterTurnResolved)
	}
	if !game.turnBasedExtraMonsterAction || game.turnBasedMonsterPassesLeft != 1 || game.turnBasedMonsterPassDelay != 3 || !game.turnBasedMonsterStatusTick {
		t.Errorf("monster turn scheduling was reset: extra=%v passes=%d delay=%d statusTick=%v",
			game.turnBasedExtraMonsterAction, game.turnBasedMonsterPassesLeft, game.turnBasedMonsterPassDelay, game.turnBasedMonsterStatusTick)
	}

	// The resumed pass was midway through its delay. It must still resolve and
	// return control to the party rather than remaining stuck on monster turn.
	for i := 0; i < 4; i++ {
		gl.updateMonstersTurnBased()
	}
	if game.currentTurn != 0 || !game.monsterTurnResolved {
		t.Fatalf("resumed monster turn did not complete: current:%d resolved:%v", game.currentTurn, game.monsterTurnResolved)
	}
}

func TestTurnBasedActionsCarryRTCadenceAcrossModeSwitch(t *testing.T) {
	t.Run("main hand", func(t *testing.T) {
		game, _, _ := tbBehaviorGame(t, 5, 5)
		member := game.party.Members[0]
		member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
		member.ActionsRemaining = 1
		want := game.combat.WeaponCooldownFrames(member)

		game.consumeSelectedCharWeaponAction()
		if member.RTCooldown != want {
			t.Fatalf("TB main-hand cooldown = %d, want %d", member.RTCooldown, want)
		}

		game.ToggleTurnBasedMode()
		if game.rtActionReady(0, rtActWeapon) {
			t.Fatal("TB main-hand swing became immediately ready after switching to RT")
		}
	})

	t.Run("spell action preserves longer prior cooldown", func(t *testing.T) {
		game, _, _ := tbBehaviorGame(t, 5, 5)
		member := game.party.Members[0]
		member.ActionsRemaining = 1
		member.RTCooldown = 53

		game.consumeSelectedCharActionWithRTCooldown(37)
		if member.RTCooldown != 53 {
			t.Fatalf("TB spell action shortened carried RT cooldown to %d, want 53", member.RTCooldown)
		}
	})

	t.Run("off hand", func(t *testing.T) {
		game, _, _ := tbBehaviorGame(t, 5, 5)
		member := game.party.Members[0]
		makeDualWielder(t, member)
		member.ActionsRemaining = 1
		member.NextTBAttackOffHand = true
		want := game.combat.OffHandWeaponCooldownFrames(member)

		game.consumeSelectedCharWeaponAction()
		if member.RTCooldown != 0 || member.OffHandRTCooldown != want {
			t.Fatalf("TB off-hand cooldowns = main:%d off:%d, want 0/%d", member.RTCooldown, member.OffHandRTCooldown, want)
		}
		if member.NextTBAttackOffHand {
			t.Fatal("off-hand TB swing did not advance the hand cursor")
		}
	})
}

func TestTurnBasedSmartAttackIgnoresRetainedRTCooldown(t *testing.T) {
	combat := newTestCombatSystemWithConfig(t)
	game := combat.game
	game.turnBasedMode = true
	member := game.party.Members[0]
	member.Class = character.ClassKnight
	spell, err := spells.CreateSpellItem("firebolt")
	if err != nil {
		t.Fatalf("create firebolt: %v", err)
	}
	member.Equipment[items.SlotSpell] = spell
	member.SpellPoints, member.MaxSpellPoints = 99, 99
	member.RTCooldown = 999
	game.selectedChar = 0

	acted, spellID := combat.SmartAttack()
	if !acted || spellID != "firebolt" {
		t.Fatalf("TB SmartAttack with retained RT cooldown = acted:%v spell:%q, want true/firebolt", acted, spellID)
	}
}

func TestSaveLoad_PreservesSuspendedTurnBasedTurn(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"

	game := newTestGame(cfg, w)
	game.combat = NewCombatSystem(game)
	game.turnBasedMode = true
	game.currentTurn = 0
	game.partyActionsUsed = 1
	game.party.Members[0].ActionsRemaining = 0
	game.party.Members[1].ActionsRemaining = 1
	game.party.Members[1].NextTBAttackOffHand = true
	game.ToggleTurnBasedMode() // suspend the party turn in RT

	save := game.buildSave(wm)
	if !save.TurnBasedTurnSuspended || save.TurnBased {
		t.Fatalf("saved mode state = turnBased:%v suspended:%v, want false/true", save.TurnBased, save.TurnBasedTurnSuspended)
	}

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	defer func() { world.GlobalWorldManager = oldWorldManager }()

	loaded := newTestGame(cfg, w)
	loaded.combat = NewCombatSystem(loaded)
	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	loaded.ToggleTurnBasedMode()

	if !loaded.turnBasedMode || loaded.turnBasedTurnSuspended {
		t.Fatalf("loaded mode state = turnBased:%v suspended:%v, want true/false", loaded.turnBasedMode, loaded.turnBasedTurnSuspended)
	}
	if loaded.currentTurn != 0 || loaded.partyActionsUsed != 1 {
		t.Fatalf("loaded party turn = current:%d actionsUsed:%d, want 0/1", loaded.currentTurn, loaded.partyActionsUsed)
	}
	if got := loaded.party.Members[0].ActionsRemaining; got != 0 {
		t.Errorf("loaded member 0 actions = %d, want 0", got)
	}
	if got := loaded.party.Members[1].ActionsRemaining; got != 1 {
		t.Errorf("loaded member 1 actions = %d, want 1", got)
	}
	if !loaded.party.Members[1].NextTBAttackOffHand {
		t.Error("loaded TB hand cursor was reset")
	}
}

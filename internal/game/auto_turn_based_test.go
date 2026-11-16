package game

import (
	"encoding/json"
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// Settings > Gameplay > Auto turn-based enters turn-based mode the first time
// a monster presses the party in a fight, through the real frame loop: once
// per fight, on a fresh round, never under a conversation, and never for a
// fight loaded from a save.
func TestAutoTurnBasedOnAggro(t *testing.T) {
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	frames := func(h *displayedModalHarness, n int) {
		t.Helper()
		for range n {
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
		}
	}
	aggro := func(h *displayedModalHarness) {
		t.Helper()
		ts := float64(h.g.config.GetTileSize())
		spawnMonsterAtTile(h.g, "goblin", TileIndex(h.g.camera.X, ts)+3, TileIndex(h.g.camera.Y, ts), ts)
		if !h.g.partyUnderAttack() {
			t.Fatal("fixture: the goblin does not press the party")
		}
	}
	calm := func(h *displayedModalHarness) {
		t.Helper()
		h.g.world.Monsters = nil
		frames(h, 1)
		if h.g.partyUnderAttack() {
			t.Fatal("fixture: the party is still pressed")
		}
	}

	// witness keeps the fight alive without pressing the party: a retaliation
	// for a long shot from beyond the pursuit cap.
	witness := func(h *displayedModalHarness) *monsterPkg.Monster3D {
		t.Helper()
		ts := float64(h.g.config.GetTileSize())
		m := monsterPkg.NewMonster3DFromConfig(h.g.camera.X, h.g.camera.Y, "goblin", h.g.config)
		far := m.PursuitLimitTiles() + 4
		m.X += far * ts
		m.IsEngagingPlayer, m.WasAttacked = true, true
		m.Retaliation = monsterPkg.RetaliationState{Frames: 1 << 20, RadiusTiles: far + 8}
		h.g.world.Monsters = []*monsterPkg.Monster3D{m}
		if !h.g.partyInCombat() || h.g.partyUnderAttack() {
			t.Fatalf("fixture: witness in fight %v, presses %v; want true, false", h.g.partyInCombat(), h.g.partyUnderAttack())
		}
		return m
	}

	t.Run("off keeps real time", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		frames(h, 1)
		aggro(h)
		frames(h, 3)
		if h.g.turnBasedMode {
			t.Fatal("the mode switched with the option off")
		}
	})

	t.Run("on switches once per aggro", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		frames(h, 1)
		if g.turnBasedMode {
			t.Fatal("a calm party entered turn-based mode")
		}
		aggro(h)
		frames(h, 1)
		if !g.turnBasedMode {
			t.Fatal("aggro did not start turn-based mode")
		}
		// Left by hand while the goblin still presses: real time stands.
		g.ToggleTurnBasedMode()
		frames(h, 3)
		if g.turnBasedMode {
			t.Fatal("the mode came back while the same fight went on")
		}
		// The goblin lets go and comes back while the fight goes on.
		goblin := g.world.Monsters[0]
		far := witness(h)
		frames(h, 2)
		g.world.Monsters = []*monsterPkg.Monster3D{far, goblin}
		if !g.partyUnderAttack() {
			t.Fatal("fixture: the returned goblin does not press the party")
		}
		frames(h, 2)
		if g.turnBasedMode {
			t.Fatal("the mode came back when the same fight's pressure returned")
		}
		calm(h)
		aggro(h)
		frames(h, 1)
		if !g.turnBasedMode {
			t.Fatal("a new fight did not start turn-based mode")
		}
		// Already turn-based: the next aggro changes nothing.
		calm(h)
		aggro(h)
		frames(h, 1)
		if !g.turnBasedMode {
			t.Fatal("aggro in turn-based mode left it")
		}
	})

	t.Run("a new fight starts a fresh round", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		spend := func() {
			g.partyActionsUsed = 1
			g.party.Members[0].ActionsRemaining = 0
		}
		aggro(h)
		frames(h, 1)
		spend()
		// Tab out and back inside the fight resumes the spent round.
		g.ToggleTurnBasedMode()
		frames(h, 2)
		g.ToggleTurnBasedMode()
		if g.partyActionsUsed != 1 || g.party.Members[0].ActionsRemaining != 0 {
			t.Fatalf("resumed round: used %d, first hero's actions %d; want 1, 0", g.partyActionsUsed, g.party.Members[0].ActionsRemaining)
		}
		g.ToggleTurnBasedMode()
		calm(h)
		aggro(h)
		frames(h, 1)
		if !g.turnBasedMode || g.currentTurn != 0 || g.partyActionsUsed != 0 || g.party.Members[0].ActionsRemaining == 0 {
			t.Fatalf("new fight: turn-based %v, turn %d, used %d, first hero's actions %d; want a fresh party round",
				g.turnBasedMode, g.currentTurn, g.partyActionsUsed, g.party.Members[0].ActionsRemaining)
		}
	})

	// corners puts the party and one monster a whole number of tiles apart on
	// x, both by the facing edges of their tiles one row apart: the tile snap
	// of a turn-based entry then parts them by a further diagonal. A quarter
	// tile from the row border keeps a moving monster in its row.
	corners := func(h *displayedModalHarness, m *monsterPkg.Monster3D, tiles float64) {
		t.Helper()
		ts := float64(h.g.config.GetTileSize())
		h.g.setPartyPosition(4.99*ts, 4.75*ts)
		m.X, m.Y = (4.97+tiles)*ts, 5.25*ts
		h.g.world.Monsters = []*monsterPkg.Monster3D{m}
		h.g.world.RegisterMonstersWithCollisionSystem(h.g.collisionSystem)
		if !h.g.partyUnderAttack() {
			t.Fatal("fixture: the monster does not press the party")
		}
	}

	// A guard that saw the party at its sight edge: the snap puts it past that
	// edge, where a guard on duty goes back to its post.
	edgeGuard := func(h *displayedModalHarness) *monsterPkg.Monster3D {
		guard := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", h.g.config)
		guard.IsEngagingPlayer, guard.LootGuardAlerted = true, true
		corners(h, guard, monsterPkg.LootGuardAggroRadiusTiles)
		return guard
	}

	t.Run("the entry takes its guards off duty", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		guard := edgeGuard(h)
		frames(h, 1)
		if !g.turnBasedMode || !guard.LootGuardRelieved {
			t.Fatalf("guard aggro: turn-based %v, relieved %v; want both", g.turnBasedMode, guard.LootGuardRelieved)
		}
		frames(h, 1)
		if !guard.IsEngagingPlayer || !g.partyUnderAttack() {
			t.Fatalf("the snap ended the fight: engaging %v, pressed %v", guard.IsEngagingPlayer, g.partyUnderAttack())
		}
		// A guard that joins the fight later goes off duty too.
		ts := float64(g.config.GetTileSize())
		late := monsterPkg.NewMonster3DFromConfig(g.camera.X, g.camera.Y+monsterPkg.LootGuardAggroRadiusTiles*ts, "goblin", g.config)
		late.IsEngagingPlayer, late.LootGuardAlerted = true, true
		g.world.Monsters = append(g.world.Monsters, late)
		frames(h, 1)
		if !late.LootGuardRelieved {
			t.Fatal("a guard that joined the fight later stayed on duty")
		}
	})

	t.Run("without the option guards stay on duty", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		guard := edgeGuard(h)
		frames(h, 2)
		ts := float64(g.config.GetTileSize())
		if g.turnBasedMode || guard.LootGuardRelieved || guard.PursuitLeashPixels() != monsterPkg.LootGuardAggroRadiusTiles*ts {
			t.Fatalf("option off: turn-based %v, relieved %v, leash %.1f tiles; want real time and the sight-edge leash",
				g.turnBasedMode, guard.LootGuardRelieved, guard.PursuitLeashPixels()/ts)
		}
		// Tab is not the automatic entry: the guard keeps its duty.
		g.ToggleTurnBasedMode()
		frames(h, 1)
		if guard.LootGuardRelieved {
			t.Fatal("a manual entry relieved the guard")
		}
	})

	t.Run("waits until the pressure holds on the aligned tiles", func(t *testing.T) {
		// A map hunter presses only inside the pursuit cap; the snap would put
		// it back outside, so the switch waits for it to close in.
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		hunter := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", g.config)
		hunter.Relentless = true
		limit := hunter.PursuitLimitTiles()
		corners(h, hunter, limit)
		frames(h, 2)
		if g.turnBasedMode {
			t.Fatal("the mode switched for a fight its own snap would end")
		}
		corners(h, hunter, limit-1)
		frames(h, 2)
		if !g.turnBasedMode || !g.partyUnderAttack() {
			t.Fatalf("hunter inside the cap: turn-based %v, pressed %v; want both", g.turnBasedMode, g.partyUnderAttack())
		}
	})

	t.Run("a manual leave while the switch waits holds", func(t *testing.T) {
		// A long-shot retaliation keeps the fight beyond the pursuit cap, so
		// the fight goes on while the switch waits for the monster to close in.
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		m := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", g.config)
		m.IsEngagingPlayer, m.WasAttacked = true, true
		limit := m.PursuitLimitTiles()
		m.Retaliation = monsterPkg.RetaliationState{Frames: 1 << 20, RadiusTiles: limit + 8}
		corners(h, m, limit)
		frames(h, 1)
		if g.turnBasedMode {
			t.Fatal("fixture: the switch did not wait")
		}
		g.ToggleTurnBasedMode()
		frames(h, 1)
		g.ToggleTurnBasedMode()
		corners(h, m, limit-1)
		frames(h, 2)
		if g.turnBasedMode {
			t.Fatal("the mode came back after the player left it in the same fight")
		}
	})

	t.Run("a save while the switch waits keeps the switch", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		hunter := monsterPkg.NewMonster3DFromConfig(0, 0, "goblin", g.config)
		hunter.Relentless = true
		limit := hunter.PursuitLimitTiles()
		corners(h, hunter, limit)
		frames(h, 1)
		data, err := json.Marshal(g.buildSave(world.GlobalWorldManager))
		if err != nil {
			t.Fatal(err)
		}
		var loaded GameSave
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		if err := g.applySave(world.GlobalWorldManager, &loaded); err != nil {
			t.Fatal(err)
		}
		if g.turnBasedMode || len(g.world.Monsters) != 1 {
			t.Fatalf("loaded: turn-based %v, monsters %d; want real time and the hunter", g.turnBasedMode, len(g.world.Monsters))
		}
		corners(h, g.world.Monsters[0], limit-1)
		frames(h, 2)
		if !g.turnBasedMode {
			t.Fatal("the loaded fight never switched once the hunter closed in")
		}
	})

	t.Run("waits for a conversation to close", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		g.dialogActive = true
		aggro(h)
		frames(h, 2)
		if !g.dialogActive {
			t.Fatal("fixture: the conversation closed by itself")
		}
		if g.turnBasedMode {
			t.Fatal("the mode switched under an open conversation")
		}
		g.dialogActive = false
		frames(h, 1)
		if !g.turnBasedMode {
			t.Fatal("the mode did not switch once the conversation closed")
		}
	})

	t.Run("a loaded fight keeps its saved mode", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.combatPreferences.AutoTurnBased = true
		aggro(h)
		data, err := json.Marshal(g.buildSave(world.GlobalWorldManager))
		if err != nil {
			t.Fatal(err)
		}
		var loaded GameSave
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		g.fightAggroAnswered = false
		if err := g.applySave(world.GlobalWorldManager, &loaded); err != nil {
			t.Fatal(err)
		}
		if !g.partyUnderAttack() {
			t.Fatal("fixture: the loaded goblin does not press the party")
		}
		frames(h, 2)
		if g.turnBasedMode {
			t.Fatal("loading a real-time fight switched it to turn-based mode")
		}
	})
}

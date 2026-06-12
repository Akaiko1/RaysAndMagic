package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// newThiefTestGame builds an open 8x8 world with a thief at tile (1,1) facing
// +X, and a combat system wired up.
func newThiefTestGame(t *testing.T) (*MMGame, *character.MMCharacter) {
	t.Helper()
	cfg := loadTestConfig(t)
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 8, 8
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := 0; y < w.Height; y++ {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := 0; x < w.Width; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	// Isolate from tests that leave a GlobalWorldManager behind (it would
	// hijack currentMapKey/GetCurrentWorld for this game's separate world).
	oldWM := world.GlobalWorldManager
	world.GlobalWorldManager = nil
	t.Cleanup(func() { world.GlobalWorldManager = oldWM })

	g := newTestGame(cfg, w)
	g.combat = NewCombatSystem(g)
	g.maxMessages = 50                                 // newTestGame leaves 0 → AddCombatMessage trims everything
	g.camera.X, g.camera.Y, g.camera.Angle = 96, 96, 0 // tile (1,1), facing +X

	thief := character.CreateCharacter("Nyra", character.ClassThief, cfg)
	thief.SpellPoints, thief.MaxSpellPoints = 100, 100
	g.party.Members[0] = thief
	g.selectedChar = 0
	return g, thief
}

func spawnTestMonsterAt(g *MMGame, tileX, tileY int) *monsterPkg.Monster3D {
	ts := float64(g.config.GetTileSize())
	cx, cy := TileCenterFromTile(tileX, tileY, ts)
	m := monsterPkg.NewMonster3DFromConfig(cx, cy, "goblin", g.config)
	g.world.Monsters = append(g.world.Monsters, m)
	g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 32, 32, collision.CollisionTypeMonster, false))
	return m
}

// A trap thrown at a monster's feet fires immediately: damage lands, SP is
// paid, and the one-shot trap is gone.
func TestTrap_PlacedUnderMonsterFiresImmediately(t *testing.T) {
	g, thief := newThiefTestGame(t)
	mob := spawnTestMonsterAt(g, 3, 1) // straight ahead, within range
	startHP := mob.HitPoints
	startSP := thief.SpellPoints

	key, placed := g.combat.tryPlaceQuickTrap(thief, true)
	if !placed || key != "cleave_trap" {
		t.Fatalf("place failed: key=%q placed=%v", key, placed)
	}
	if mob.HitPoints >= startHP {
		t.Errorf("cleave trap under monster must damage it: HP %d -> %d", startHP, mob.HitPoints)
	}
	if len(g.traps) != 0 {
		t.Errorf("traps are one-shot: %d remain", len(g.traps))
	}
	if thief.SpellPoints >= startSP {
		t.Errorf("trap must cost SP: %d -> %d", startSP, thief.SpellPoints)
	}
}

// Per-owner arming limit: the 4th trap is refused.
func TestTrap_OwnerLimit(t *testing.T) {
	g, thief := newThiefTestGame(t)
	for i := 0; i < MaxTrapsPerOwner; i++ {
		// Empty tiles ahead: trap lands at max range; shift the camera so each
		// trap takes a different tile.
		g.camera.Y = float64(96 + i*64)
		if _, placed := g.combat.tryPlaceQuickTrap(thief, true); !placed {
			t.Fatalf("trap %d should place", i+1)
		}
	}
	g.camera.Y = 96 + 3*64
	if _, placed := g.combat.tryPlaceQuickTrap(thief, true); placed {
		t.Fatalf("4th trap must be refused (limit %d)", MaxTrapsPerOwner)
	}
	if len(g.traps) != MaxTrapsPerOwner {
		t.Fatalf("expected %d armed traps, got %d", MaxTrapsPerOwner, len(g.traps))
	}
}

// Trap damage formula: base + (Int+Acc)/divisor + Trapper mastery — the same
// function drives combat and the trap-book tooltip.
func TestTrapDamage_ScalesWithStatsAndMastery(t *testing.T) {
	g, thief := newThiefTestGame(t)
	_ = g
	def, ok := config.GetTrapDefinition("cleave_trap")
	if !ok {
		t.Fatal("cleave_trap missing")
	}
	base := def.DamageBase + (thief.GetEffectiveIntellect()+thief.GetEffectiveAccuracy())/character.TrapStatScalingDivisor
	if got := trapDamage(def, thief); got != base {
		t.Errorf("novice trapper: want %d, got %d", base, got)
	}
	thief.Skills[character.SkillTrapper].Mastery = character.MasteryMaster // tier 2
	if want, got := base+2*character.TrapperDamagePerTier, trapDamage(def, thief); got != want {
		t.Errorf("master trapper: want %d, got %d", want, got)
	}
}

// Stasis stuns; bear trap roots — rooted monsters skip TB movement (the move
// attempt consumes the root) but are NOT stunned.
func TestTrap_StasisStunsAndBearRootsTB(t *testing.T) {
	g, thief := newThiefTestGame(t)
	g.turnBasedMode = true
	gl := &GameLoop{game: g, combat: g.combat}
	thief.Level = 10 // unlock everything

	if !equipTrap(thief, "stasis_trap") {
		t.Fatal("equip stasis_trap")
	}
	stunned := spawnTestMonsterAt(g, 3, 1)
	if _, placed := g.combat.tryPlaceQuickTrap(thief, true); !placed {
		t.Fatal("stasis trap should place")
	}
	if want := config.GlobalTrapConfig.Traps["stasis_trap"].StunTurns; stunned.StunTurnsRemaining != want {
		t.Errorf("stasis: want %d stun turns, got %d", want, stunned.StunTurnsRemaining)
	}

	if !equipTrap(thief, "bear_trap") {
		t.Fatal("equip bear_trap")
	}
	g.camera.Y = 96 + 2*64 // new lane
	rooted := spawnTestMonsterAt(g, 3, 3)
	if _, placed := g.combat.tryPlaceQuickTrap(thief, true); !placed {
		t.Fatal("bear trap should place")
	}
	if rooted.StunTurnsRemaining != 0 {
		t.Errorf("bear trap must not stun")
	}
	if want := config.GlobalTrapConfig.Traps["bear_trap"].RootTurns; rooted.RootTurnsRemaining != want {
		t.Errorf("bear: want %d root turns, got %d", want, rooted.RootTurnsRemaining)
	}

	// Rooted: the root burns one charge per monster TURN (TickRootTurn) and
	// pins movement for that whole turn — even if the monster only attacks.
	baseTurns := config.GlobalTrapConfig.Traps["bear_trap"].RootTurns
	x, y := rooted.X, rooted.Y
	for turn := 0; turn < baseTurns; turn++ {
		rooted.TickRootTurn()
		gl.monsterMoveTurnBased(rooted)
		if rooted.X != x || rooted.Y != y {
			t.Fatalf("turn %d: rooted monster must not move", turn)
		}
	}
	if rooted.RootTurnsRemaining != 0 {
		t.Errorf("root must be spent after %d turns, %d left", baseTurns, rooted.RootTurnsRemaining)
	}
	rooted.TickRootTurn() // root expired: movement unpinned
	if rooted.RootHeld() {
		t.Error("expired root must release the monster")
	}
}

// All four mastery tiers map to the designed 10/20/30/40% chance; no skill = 0.
func TestSleightChance_AllMasteries(t *testing.T) {
	_, thief := newThiefTestGame(t)
	cases := []struct {
		mastery character.SkillMastery
		want    int
	}{
		{character.MasteryNovice, 10},
		{character.MasteryExpert, 20},
		{character.MasteryMaster, 30},
		{character.MasteryGrandMaster, 40},
	}
	for _, tc := range cases {
		thief.Skills[character.SkillSleightOfHand].Mastery = tc.mastery
		if got := sleightChancePct(thief); got != tc.want {
			t.Errorf("%v: want %d%%, got %d%%", tc.mastery, tc.want, got)
		}
	}
	delete(thief.Skills, character.SkillSleightOfHand)
	if got := sleightChancePct(thief); got != 0 {
		t.Errorf("without the skill: want 0%%, got %d%%", got)
	}
}

// Sleight of Hand: melee hits eventually pick the victim's pocket — once.
func TestSleightOfHand_PaysGoldOnce(t *testing.T) {
	g, thief := newThiefTestGame(t)
	thief.Skills[character.SkillSleightOfHand].Mastery = character.MasteryGrandMaster // 40%
	mob := spawnTestMonsterAt(g, 2, 1)
	mob.Level = 3 // low-level victim → consolation gold 5 (goblin has loot, may steal an item instead)
	mob.MaxHitPoints, mob.HitPoints = 100000, 100000

	startGold := g.party.Gold
	startItems := len(g.party.Inventory)
	for i := 0; i < 500 && !mob.Pilfered; i++ {
		g.combat.trySleightOfHand(thief, mob)
	}
	if !mob.Pilfered {
		t.Fatal("GM sleight (40%) did not land in 500 swings — statistically impossible")
	}
	gotGold := g.party.Gold - startGold
	gotItems := len(g.party.Inventory) - startItems
	if gotGold == 0 && gotItems == 0 {
		t.Fatal("successful pick must yield loot or gold")
	}
	if gotGold != 0 && gotGold != character.SleightGoldLow {
		t.Errorf("low-level victim pays %d gold, got %d", character.SleightGoldLow, gotGold)
	}

	// Already pilfered: nothing more to take.
	goldAfter := g.party.Gold
	for i := 0; i < 200; i++ {
		g.combat.trySleightOfHand(thief, mob)
	}
	if g.party.Gold != goldAfter || len(g.party.Inventory) != startItems+gotItems {
		t.Error("a pilfered monster must not pay twice")
	}
}

// Space with the trap unavailable falls back to the dagger SILENTLY: no
// fizzle/limit chatter (the canPay pre-check keeps quick spells just as
// quiet), no SP spent. Explicit F keeps the refusal message.
func TestSmartAttack_TrapFallbackIsSilent(t *testing.T) {
	cases := []struct {
		name string
		prep func(g *MMGame, thief *character.MMCharacter)
	}{
		{"no_sp", func(g *MMGame, thief *character.MMCharacter) {
			thief.SpellPoints = 0
		}},
		{"limit_reached", func(g *MMGame, thief *character.MMCharacter) {
			for i := 0; i < MaxTrapsPerOwner; i++ {
				g.camera.Y = float64(96 + i*64)
				if _, placed := g.combat.tryPlaceQuickTrap(thief, true); !placed {
					t.Fatalf("setup trap %d failed", i+1)
				}
			}
			g.camera.Y = 96 + 3*64
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, thief := newThiefTestGame(t)
			tc.prep(g, thief)
			spBefore := thief.SpellPoints
			msgsBefore := len(g.combatMessages)

			acted, castID := g.combat.SmartAttack()

			if castID != "" {
				t.Errorf("no trap should have been armed, castID=%q", castID)
			}
			if !acted {
				t.Error("Space must still act (weapon swing fallback)")
			}
			if thief.SpellPoints != spBefore {
				t.Errorf("silent fallback must not spend SP: %d -> %d", spBefore, thief.SpellPoints)
			}
			for _, m := range g.combatMessages[msgsBefore:] {
				low := strings.ToLower(m)
				if strings.Contains(low, "fizzles") || strings.Contains(low, "traps armed") {
					t.Errorf("Space fallback must be silent, got message %q", m)
				}
			}
		})
	}
}

// Explicit F (CastEquippedSpell) announces WHY the trap was refused.
func TestExplicitF_TrapRefusalAnnounces(t *testing.T) {
	g, thief := newThiefTestGame(t)
	thief.SpellPoints = 0
	g.selectedChar = 0
	msgsBefore := len(g.combatMessages)
	if g.combat.CastEquippedSpell() {
		t.Fatal("cast must fail without SP")
	}
	found := false
	for _, m := range g.combatMessages[msgsBefore:] {
		if strings.Contains(strings.ToLower(m), "fizzles") {
			found = true
		}
	}
	if !found {
		t.Error("explicit F must announce the SP shortfall")
	}
}

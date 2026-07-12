package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// These tests cover the turn-based / real-time monster movement and attack
// rules: tile-centering, adjacent melee that never enters the player's tile,
// ranged firing only when row/column-aligned, the 1-tile real-time melee reach,
// and the puma pounce landing on an adjacent tile (not on the player).

func absI(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tbBehaviorGame builds a turn-based game on an empty wxh world with the combat
// system wired, and returns it plus a GameLoop and the tile size.
func tbBehaviorGame(t *testing.T, w, h int) (*MMGame, *GameLoop, float64) {
	t.Helper()
	cfg := loadTestConfig(t)
	worldTest := newTestWorldSized(cfg, w, h)
	game := newTestGame(cfg, worldTest)
	game.turnBasedMode = true
	game.combat = NewCombatSystem(game)
	// Zero party Luck so perfect-dodge (luck/5 % RNG) never fires - these tests
	// assert that a melee/pounce hit lands, which must be deterministic.
	for _, c := range game.party.Members {
		c.Luck = 0
	}
	gl := &GameLoop{game: game}
	return game, gl, float64(cfg.GetTileSize())
}

func placePlayerAtTile(game *MMGame, tx, ty int, ts float64) {
	game.camera.X = float64(tx)*ts + ts/2
	game.camera.Y = float64(ty)*ts + ts/2
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
}

func spawnMonsterAtTile(game *MMGame, key string, tx, ty int, ts float64) *monster.Monster3D {
	m := monster.NewMonster3DFromConfig(float64(tx)*ts+ts/2, float64(ty)*ts+ts/2, key, game.config)
	m.IsEngagingPlayer = true // ensure it participates regardless of vision
	m.WasAttacked = true      // bypass any passive-until-attacked gating
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	return m
}

func runOneMonsterTurn(game *MMGame, gl *GameLoop) {
	game.currentTurn = 1
	game.monsterTurnResolved = false
	gl.updateMonstersTurnBased()
}

func partyHPSum(game *MMGame) int {
	sum := 0
	for _, c := range game.party.Members {
		sum += c.HitPoints
	}
	return sum
}

func monsterTileCoords(m *monster.Monster3D, ts float64) (int, int) {
	return int(m.X / ts), int(m.Y / ts)
}

func TestTurnBased_MoveAfterActionGrantsExtraMonsterAction(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	game.currentTurn = 0
	for _, m := range game.party.Members {
		m.ActionsRemaining = 1
	}

	game.consumeSelectedCharAction()
	if !game.turnBasedMode || game.partyActionsUsed != 1 {
		t.Fatalf("test setup failed: turnBased=%v partyActionsUsed=%d", game.turnBasedMode, game.partyActionsUsed)
	}

	game.endPartyTurnAfterMovement()

	if !game.turnBasedExtraMonsterAction {
		t.Fatalf("moving after spending a TB action must grant monsters an extra action pass")
	}
	if game.currentTurn != 1 {
		t.Fatalf("currentTurn=%d, want monster turn", game.currentTurn)
	}
	for i, m := range game.party.Members {
		if m.ActionsRemaining != 0 {
			t.Fatalf("member %d ActionsRemaining=%d, want 0 after movement", i, m.ActionsRemaining)
		}
	}
}

func TestTurnBased_ExtraMonsterActionMovesTwiceAndResets(t *testing.T) {
	run := func(extra bool) (int, bool) {
		game, gl, ts := tbBehaviorGame(t, 20, 20)
		placePlayerAtTile(game, 10, 10, ts)
		m := spawnMonsterAtTile(game, "goblin", 10, 7, ts)
		game.turnBasedExtraMonsterAction = extra

		runOneMonsterTurn(game, gl)

		_, my := monsterTileCoords(m, ts)
		return my, game.turnBasedExtraMonsterAction
	}

	normalY, normalExtra := run(false)
	if normalY != 8 {
		t.Fatalf("normal monster turn moved to y=%d, want 8 (one tile)", normalY)
	}
	if normalExtra {
		t.Fatalf("normal monster turn should not set extra action flag")
	}

	game, gl, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 10, 7, ts)
	game.turnBasedExtraMonsterAction = true

	runOneMonsterTurn(game, gl)
	_, firstPassY := monsterTileCoords(m, ts)
	if firstPassY != 8 {
		t.Fatalf("first pass moved to y=%d, want 8 before the delayed extra pass", firstPassY)
	}
	if game.currentTurn != 1 {
		t.Fatalf("currentTurn=%d, want monster turn while waiting for delayed extra pass", game.currentTurn)
	}
	if game.turnBasedMonsterPassDelay <= 0 {
		t.Fatalf("turnBasedMonsterPassDelay=%d, want visible delay before extra pass", game.turnBasedMonsterPassDelay)
	}

	for frames := 0; game.currentTurn == 1 && frames < 120; frames++ {
		runOneMonsterTurn(game, gl)
	}
	if game.currentTurn != 0 {
		t.Fatalf("monster turn did not finish after delayed extra pass")
	}

	_, finalY := monsterTileCoords(m, ts)
	if finalY != 9 {
		t.Fatalf("extra monster turn moved to y=%d, want 9 after two tile actions", finalY)
	}
	if game.turnBasedExtraMonsterAction {
		t.Fatalf("extra monster action flag must reset after the monster turn consumes it")
	}
}

// Melee monsters approach one tile per turn, strike from any adjacent tile
// (including diagonals), and never step onto the player's own tile.
func TestTurnBased_MeleeOnlyHitsFromAdjacent(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	m := spawnMonsterAtTile(game, "goblin", 13, 10, ts) // 3 tiles east, same row

	everHit := false
	for turn := 0; turn < 8; turn++ {
		hpBefore := partyHPSum(game)
		runOneMonsterTurn(game, gl)
		tx, ty := monsterTileCoords(m, ts)

		if tx == ptx && ty == pty {
			t.Fatalf("turn %d: monster stepped onto the player's tile", turn)
		}
		if partyHPSum(game) < hpBefore {
			everHit = true
			dx, dy := absI(tx-ptx), absI(ty-pty)
			if dx > 1 || dy > 1 || dx+dy == 0 {
				t.Fatalf("turn %d: melee hit from a non-adjacent tile (dx=%d dy=%d)", turn, dx, dy)
			}
		}
	}
	if !everHit {
		t.Fatalf("melee monster never closed in to strike the party")
	}
}

// A melee monster diagonally adjacent to the player can attack; diagonal contact
// counts as point-blank so packs can surround the party.
func TestTurnBased_MeleeAttacksDiagonally(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 11, 11, ts) // diagonal neighbour

	hp0 := partyHPSum(game)
	runOneMonsterTurn(game, gl)

	tx, ty := monsterTileCoords(m, ts)
	if dx, dy := absI(tx-10), absI(ty-10); dx != 1 || dy != 1 {
		t.Fatalf("expected diagonal monster to stay diagonally adjacent, got dx=%d dy=%d", dx, dy)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("melee monster should attack from a diagonal tile")
	}
}

func TestTurnBased_FrontDiagonalMeleeMonsterHasPulledVisualPosition(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	game.camera.Angle = 0 // facing east
	r := &Renderer{game: game}

	frontDiag := spawnMonsterAtTile(game, "goblin", 11, 9, ts)
	vx, vy := r.monsterVisualPosition(frontDiag)
	if vx == frontDiag.X && vy == frontDiag.Y {
		t.Fatalf("front-diagonal melee monster should be visually pulled into the TB front view")
	}
	if got, want := vx-game.camera.X, tbFrontDiagonalMonsterForwardTiles*ts; math.Abs(got-want) > 1e-6 {
		t.Fatalf("visual forward offset = %.2f, want %.2f", got, want)
	}
	if got, want := game.camera.Y-vy, tbFrontDiagonalMonsterLateralTiles*ts; math.Abs(got-want) > 1e-6 {
		t.Fatalf("visual lateral offset = %.2f, want %.2f", got, want)
	}

	backDiag := spawnMonsterAtTile(game, "goblin", 9, 9, ts)
	vx, vy = r.monsterVisualPosition(backDiag)
	if vx != backDiag.X || vy != backDiag.Y {
		t.Fatalf("back-diagonal monster should not be visually pulled")
	}

	game.turnBasedMode = false
	vx, vy = r.monsterVisualPosition(frontDiag)
	if vx != frontDiag.X || vy != frontDiag.Y {
		t.Fatalf("real-time mode should use the monster's real position")
	}
}

// Each turn a participating monster snaps to the center of its tile (fixes
// off-center spawns / drift).
func TestTurnBased_CentersOnTileEachTurn(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	// Spawn off-center inside the cardinally-adjacent tile (11,10): it will
	// center, then attack (no move), so the final position is the tile center.
	m := monster.NewMonster3DFromConfig(11*ts+7, 10*ts+13, "goblin", game.config)
	m.IsEngagingPlayer = true
	m.WasAttacked = true
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	runOneMonsterTurn(game, gl)

	wantX, wantY := 11*ts+ts/2, 10*ts+ts/2
	if m.X != wantX || m.Y != wantY {
		t.Fatalf("expected monster centered at (%.0f,%.0f), got (%.0f,%.0f)", wantX, wantY, m.X, m.Y)
	}
}

// Ranged monsters fire only when on the player's row or column; from a diagonal
// tile they reposition instead of shooting.
func TestTurnBased_RangedAttacksOnlyWhenAligned(t *testing.T) {
	// Diagonal -> must NOT shoot.
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	diag := spawnMonsterAtTile(game, "elf_archer", 12, 12, ts)
	if len(game.arrows) != 0 {
		t.Fatalf("unexpected pre-existing arrows")
	}
	runOneMonsterTurn(game, gl)
	if len(game.arrows) != 0 {
		t.Fatalf("ranged monster fired from a diagonal (unaligned) tile")
	}
	dtx, dty := monsterTileCoords(diag, ts)
	if dtx == 12 && dty == 12 {
		t.Fatalf("diagonal ranged monster should reposition toward alignment, but stayed put")
	}

	// Aligned and in range -> must shoot.
	game2, gl2, ts2 := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game2, 10, 10, ts2)
	spawnMonsterAtTile(game2, "elf_archer", 13, 10, ts2) // same row, 3 tiles (range 5)
	runOneMonsterTurn(game2, gl2)
	if len(game2.arrows) == 0 {
		t.Fatalf("aligned ranged monster in range should fire")
	}
}

func TestTurnBased_RangedAttackCountUsesCooldownMultiplier(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	archer := spawnMonsterAtTile(game, "elf_archer", 13, 10, ts) // same row, 3 tiles (range 5)
	archer.AttackCooldownMultiplier = 0.6

	runOneMonsterTurn(game, gl)

	if got := len(game.arrows); got != 2 {
		t.Fatalf("turn-based ranged attack count = %d projectiles, want 2", got)
	}
}

func TestTurnBased_RangedMonsterFindsAlternateFiringLaneWhenBlockedByArcher(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)

	// First archer already owns the east firing lane. The second starts diagonal:
	// generic circular-range pathing may pick diagonal "in range" goals where TB
	// ranged still cannot shoot. It should instead path to another row/column lane.
	first := monster.NewMonster3DFromConfig(13*ts+ts/2, 10*ts+ts/2, "elf_archer", game.config)
	second := monster.NewMonster3DFromConfig(13*ts+ts/2, 12*ts+ts/2, "elf_archer", game.config)
	for _, m := range []*monster.Monster3D{first, second} {
		m.IsEngagingPlayer = true
		m.WasAttacked = true
		m.RangedAttackRange = 5 * ts
	}
	game.world.Monsters = []*monster.Monster3D{first, second}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	for turn := 0; turn < 4; turn++ {
		runOneMonsterTurn(game, gl)
	}

	sx, sy := monsterTileCoords(second, ts)
	if sx != 10 && sy != 10 {
		t.Fatalf("second archer should take a row/column firing lane, got tile (%d,%d)", sx, sy)
	}
	if sx == 13 && sy == 10 {
		t.Fatalf("second archer moved into the first archer's occupied firing tile")
	}
	if len(game.arrows) < 5 {
		t.Fatalf("expected both archers to be firing after repositioning, got %d arrows", len(game.arrows))
	}
}

// A pouncing monster (puma) leaps onto an adjacent tile - never onto the
// player's tile - and strikes. Turn-based path.
func TestTurnBased_PounceLandsAdjacentAndStrikes(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	puma := spawnMonsterAtTile(game, "puma", 13, 10, ts) // 3 tiles east, within pounce range 4

	hp0 := partyHPSum(game)
	runOneMonsterTurn(game, gl)

	tx, ty := monsterTileCoords(puma, ts)
	if tx == ptx && ty == pty {
		t.Fatalf("puma pounced onto the player's tile")
	}
	dx, dy := absI(tx-ptx), absI(ty-pty)
	if dx > 1 || dy > 1 || dx+dy == 0 {
		t.Fatalf("puma should land on an adjacent tile, dx=%d dy=%d", dx, dy)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("puma pounce should strike the party")
	}
}

// Real-time pounce lands on an adjacent tile (not the player's) and deals damage.
func TestRealTime_PounceLandsAdjacentNotPlayerTile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	puma := spawnMonsterAtTile(game, "puma", 13, 10, ts)

	hp0 := partyHPSum(game)
	game.combat.HandleMonsterInteractions()

	tx, ty := monsterTileCoords(puma, ts)
	if tx == ptx && ty == pty {
		t.Fatalf("real-time puma pounced onto the player's tile")
	}
	dx, dy := absI(tx-ptx), absI(ty-pty)
	if dx > 1 || dy > 1 || dx+dy == 0 {
		t.Fatalf("real-time puma should land adjacent, dx=%d dy=%d", dx, dy)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("real-time puma pounce should strike the party")
	}
}

// Darkness stuns every monster within its radius (5 tiles) of the caster and
// deals no damage; monsters outside the radius are untouched.
func TestDarkness_StunsMonstersInRadiusOnly(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)

	near := monster.NewMonster3DFromConfig(float64(12)*ts+ts/2, float64(10)*ts+ts/2, "goblin", game.config) // 2 tiles
	edge := monster.NewMonster3DFromConfig(float64(15)*ts+ts/2, float64(10)*ts+ts/2, "goblin", game.config) // 5 tiles (== radius)
	far := monster.NewMonster3DFromConfig(float64(18)*ts+ts/2, float64(10)*ts+ts/2, "goblin", game.config)  // 8 tiles
	game.world.Monsters = []*monster.Monster3D{near, edge, far}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	hp0 := partyHPSum(game)
	equipSpellAndPrepareCaster(t, game.combat, "darkness", 100, 30)

	if !game.combat.CastEquippedSpell() {
		t.Fatalf("darkness cast failed")
	}

	if near.StunFramesRemaining <= 0 || near.StunTurnsRemaining <= 0 {
		t.Errorf("monster at 2 tiles should be stunned (RT %d, TB %d)", near.StunFramesRemaining, near.StunTurnsRemaining)
	}
	if edge.StunFramesRemaining <= 0 {
		t.Errorf("monster at exactly 5 tiles (radius) should be stunned, got %d", edge.StunFramesRemaining)
	}
	if far.StunFramesRemaining != 0 || far.StunTurnsRemaining != 0 {
		t.Errorf("monster at 8 tiles (outside radius) must NOT be stunned, got RT %d TB %d", far.StunFramesRemaining, far.StunTurnsRemaining)
	}
	if partyHPSum(game) != hp0 {
		t.Errorf("darkness should deal no damage to the party")
	}
	// Stun durations come from YAML (data-driven), not hardcoded here.
	def, _ := spells.GetSpellDefinitionByID("darkness")
	if def.StunRadiusTiles != 5 {
		t.Errorf("darkness stun_radius_tiles should be 5, got %v", def.StunRadiusTiles)
	}
}

// Disintegrate: undead and dragons are immune to the instakill; generic mobs
// are not. Also verifies the spell is configured as a no-damage 15% instakill.
func TestDisintegrate_ImmunityAndConfig(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5) // loads monster + spell configs
	cfg := game.config

	skeleton := monster.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	lich := monster.NewMonster3DFromConfig(0, 0, "lich", cfg)
	goblin := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	dragon := monster.NewMonster3DFromConfig(0, 0, "dragon", cfg)

	if skeleton.MonsterType != "undead" || lich.MonsterType != "undead" {
		t.Errorf("skeleton/lich should be tagged undead, got %q / %q", skeleton.MonsterType, lich.MonsterType)
	}
	if goblin.MonsterType != "" {
		t.Errorf("goblin should have empty type (generic), got %q", goblin.MonsterType)
	}
	if !monsterImmuneToDisintegrate(skeleton) || !monsterImmuneToDisintegrate(lich) {
		t.Errorf("undead must be immune to disintegrate")
	}
	if dragon.MonsterType != "dragon" {
		t.Errorf("dragon should be tagged type 'dragon', got %q", dragon.MonsterType)
	}
	if !monsterImmuneToDisintegrate(dragon) {
		t.Errorf("dragon must be immune to disintegrate (type-driven)")
	}
	if monsterImmuneToDisintegrate(goblin) {
		t.Errorf("generic mob (goblin) must NOT be immune to disintegrate")
	}

	def, _ := spells.GetSpellDefinitionByID("disintegrate")
	if !def.DealsNoDamage {
		t.Errorf("disintegrate should deal no direct damage")
	}
	if def.DisintegrateChance != 0.15 {
		t.Errorf("disintegrate_chance should be 0.15, got %v", def.DisintegrateChance)
	}
}

// Day of the Gods grants the party a mastery-scaled incoming-damage reduction
// for its YAML-driven duration.
func TestDayOfTheGods_ResistBuff(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "day_of_the_gods", 100, 30)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("day_of_the_gods cast failed")
	}
	if got := game.combatBuffResistPct(); got != 10 {
		t.Fatalf("expected 10%% resist active, got %d", got)
	}
	// Day of the Gods boosts every school's resist -> 100 fire -> 90 at Novice.
	m := game.party.Members[0]
	if got := game.combat.mitigateCharacterDamage(100, "fire", m, false); got != 90 {
		t.Errorf("100 incoming with 10%% resist should be 90, got %d", got)
	}
	def, _ := spells.GetSpellDefinitionByID("day_of_the_gods")
	buff, ok := game.combatBuffByID("day_of_the_gods")
	if want := def.Duration * game.config.GetTPS(); !ok || buff.Frames != want {
		t.Errorf("duration frames: got %d (ok=%v), want %d (%ds x TPS)", buff.Frames, ok, want, def.Duration)
	}
}

// Hour of Power: +5 outgoing damage and -1 incoming at Novice.
func TestHourOfPower_DamageBuffs(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "hour_of_power", 100, 30)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("hour_of_power cast failed")
	}
	if out, in := game.combatBuffOutBonus(), game.combatBuffInReduce(); out != 5 || in != 1 {
		t.Fatalf("hour_of_power: out=%d in=%d (want 5/1)", out, in)
	}
	m := game.party.Members[0]
	if got := game.combat.mitigateCharacterDamage(10, "fire", m, false); got != 9 {
		t.Errorf("10 incoming -1 should be 9, got %d", got)
	}
	if got := game.combat.mitigateCharacterDamage(3, "fire", m, false); got != 2 {
		t.Errorf("3 incoming -1 should be 2, got %d", got)
	}
}

// Both party buffs stack on incoming damage: % reduction first, then flat reduction.
func TestPartyBuffs_StackOnIncoming(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "day_of_the_gods", 100, 30)
	game.combat.CastEquippedSpell()
	equipSpellAndPrepareCaster(t, game.combat, "hour_of_power", 100, 30)
	game.combat.CastEquippedSpell()
	// 100 fire -> 10% resist -> 90 -> flat -1 -> 89
	m := game.party.Members[0]
	if got := game.combat.mitigateCharacterDamage(100, "fire", m, false); got != 89 {
		t.Errorf("100 with 10%% resist then -1 should be 89, got %d", got)
	}
}

// Bind Undead only charms undead; the living are immune. Also checks the spell
// is a no-damage charm in YAML.
func TestBindUndead_BindsUndeadOnly(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	gob := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)

	// bind_undead: undead only, takes control to fight others.
	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	game.combat.applyBindUndead(gob, 300, "Bind Undead")

	if !skel.Bound {
		t.Errorf("undead skeleton should be bound")
	}
	if skel.BoundFramesRemaining != 300*cfg.GetTPS() {
		t.Errorf("bind frames: got %d, want %d", skel.BoundFramesRemaining, 300*cfg.GetTPS())
	}
	if gob.Bound {
		t.Errorf("living goblin must NOT be bindable by bind_undead")
	}

	def, _ := spells.GetSpellDefinitionByID("bind_undead")
	if !def.BindUndead || !def.DealsNoDamage {
		t.Errorf("bind_undead should be a no-damage bind spell (bind=%v, noDmg=%v)", def.BindUndead, def.DealsNoDamage)
	}
}

// Charm (living/pacify): only the LIVING are affected and they are pacified
// (stop attacking); a pacified monster snaps free the instant it is hit.
func TestCharm_PacifiesLivingAndBreaksOnHit(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	gob := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)

	// Charm: undead immune, living pacified.
	game.combat.applyPacify(skel, 120, "Charm")
	game.combat.applyPacify(gob, 120, "Charm")

	if skel.Pacified {
		t.Errorf("undead skeleton must NOT be pacified by Charm")
	}
	if !gob.Pacified {
		t.Fatalf("living goblin should be pacified, got pacified=%v", gob.Pacified)
	}

	// Any hit frees the pacified goblin.
	game.combat.breakPacifyOnHit(gob)
	if gob.Pacified {
		t.Errorf("a hit should free the pacified goblin, got pacified=%v", gob.Pacified)
	}
}

// A bound undead attacks the nearest other monster and never the party.
func TestBindUndead_BoundFightsOtherMonsterNotParty(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	// Both monsters far from the player (out of vision), adjacent to each other.
	skel := monster.NewMonster3DFromConfig(float64(20)*ts+ts/2, float64(10)*ts+ts/2, "skeleton", game.config)
	gob := monster.NewMonster3DFromConfig(float64(21)*ts+ts/2, float64(10)*ts+ts/2, "goblin", game.config)
	game.world.Monsters = []*monster.Monster3D{skel, gob}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	gobHP0 := gob.HitPoints
	partyHP0 := partyHPSum(game)

	game.refreshBoundAllyCache() // mirrors updateExploration: sets AIFoe before the turn
	runOneMonsterTurn(game, gl)

	if gob.HitPoints >= gobHP0 {
		t.Errorf("bound skeleton should have struck the goblin (HP %d -> %d)", gobHP0, gob.HitPoints)
	}
	if partyHPSum(game) != partyHP0 {
		t.Errorf("bound undead must never damage the party")
	}
}

// alien_dark_bolt is a monster-only projectile (the alien's attack) and must
// never appear in a player-learnable school list - e.g. the Lich Dark picks.
func TestSpells_AlienDarkBoltNotLearnable(t *testing.T) {
	loadTestConfig(t)
	dark, err := spells.GetSpellIDsBySchool("dark")
	if err != nil {
		t.Fatalf("dark school lookup: %v", err)
	}
	var hasBolt, hasAlien bool
	for _, id := range dark {
		switch id {
		case spells.SpellID("darkbolt"):
			hasBolt = true
		case spells.SpellID("alien_dark_bolt"):
			hasAlien = true
		}
	}
	if hasAlien {
		t.Errorf("alien_dark_bolt is monster_only and must not be a learnable Dark spell")
	}
	if !hasBolt {
		t.Errorf("regular darkbolt should still be a learnable Dark spell")
	}
}

// Real-time crossfire melee must connect even from a DIAGONAL tile (~1.41 tiles):
// a bound undead and a mob diagonally adjacent should trade blows, not stand one
// pixel out of reach forever (the can't-reach bug).
func TestCrossfire_RTMeleeConnectsDiagonally(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	placePlayerAtTile(game, 5, 5, ts) // far - the two monsters only see each other
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(float64(20)*ts+ts/2, float64(20)*ts+ts/2, "skeleton", cfg)
	gob := monster.NewMonster3DFromConfig(float64(21)*ts+ts/2, float64(21)*ts+ts/2, "goblin", cfg) // diagonal neighbour
	skel.MaxHitPoints, skel.HitPoints = 300, 300
	gob.MaxHitPoints, gob.HitPoints = 300, 300
	game.world.Monsters = []*monster.Monster3D{skel, gob}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(skel, 300, "Bind Undead")

	skelHP0, gobHP0 := skel.HitPoints, gob.HitPoints
	game.refreshBoundAllyCache()
	game.combat.HandleMonsterInteractions()

	if gob.HitPoints >= gobHP0 {
		t.Errorf("bound undead should strike the diagonally-adjacent enemy (HP %d -> %d)", gobHP0, gob.HitPoints)
	}
	if skel.HitPoints >= skelHP0 {
		t.Errorf("mob should strike the diagonally-adjacent bound undead (HP %d -> %d)", skelHP0, skel.HitPoints)
	}
}

// monsterAITargetPoint redirects charmed monsters off the party: a pacified
// charm holds position, a bound undead seeks the nearest enemy, and a normal mob
// is lured at a bound undead nearer than the party (but chases the party when no
// bound undead is around).
func TestCharm_AITargetRedirectsOffParty(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	cfg := game.config
	tc := func(tx, ty int) (float64, float64) { return float64(tx)*ts + ts/2, float64(ty)*ts + ts/2 }

	sx, sy := tc(12, 10)
	gx, gy := tc(13, 10)
	px, py := tc(10, 12)
	fx, fy := tc(10, 30) // far enemy, away from any bound undead

	skel := monster.NewMonster3DFromConfig(sx, sy, "skeleton", cfg) // bound undead
	gob := monster.NewMonster3DFromConfig(gx, gy, "goblin", cfg)    // enemy 1 tile from skel, 3 from party
	paci := monster.NewMonster3DFromConfig(px, py, "goblin", cfg)   // pacified living
	far := monster.NewMonster3DFromConfig(fx, fy, "goblin", cfg)    // far enemy, no undead nearby
	game.world.Monsters = []*monster.Monster3D{skel, gob, paci, far}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.combat.applyBindUndead(skel, 300, "Bind Undead") // bound undead
	game.combat.applyPacify(paci, 120, "Charm")           // pacified living
	game.refreshBoundAllyCache()

	if tx, ty := game.combat.monsterAITargetPoint(paci); tx != paci.X || ty != paci.Y {
		t.Errorf("pacified charm should hold position (%.0f,%.0f), got (%.0f,%.0f)", paci.X, paci.Y, tx, ty)
	}
	if tx, ty := game.combat.monsterAITargetPoint(skel); tx != gob.X || ty != gob.Y {
		t.Errorf("bound undead should seek the enemy goblin (%.0f,%.0f), got (%.0f,%.0f)", gob.X, gob.Y, tx, ty)
	}
	if foe := game.combat.monsterAIFoeMonster(gob); foe != skel {
		t.Errorf("a mob next to a bound undead should target it, got %v", foe)
	}
	if foe := game.combat.monsterAIFoeMonster(far); foe != nil {
		t.Errorf("a mob with no bound undead near should target the party (nil foe), got %v", foe)
	}
	if tx, ty := game.combat.monsterAITargetPoint(far); tx != game.camera.X || ty != game.camera.Y {
		t.Errorf("far mob should chase the party (%.0f,%.0f), got (%.0f,%.0f)", game.camera.X, game.camera.Y, tx, ty)
	}
}

// A normal mob retaliates against a bound undead in its midst - they trade blows,
// and the party is never touched.
func TestBindUndead_MobsAttackTheBoundUndead(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(float64(12)*ts+ts/2, float64(10)*ts+ts/2, "skeleton", cfg)
	gob := monster.NewMonster3DFromConfig(float64(13)*ts+ts/2, float64(10)*ts+ts/2, "goblin", cfg)
	// Tough enough that one trade kills neither.
	skel.MaxHitPoints, skel.HitPoints = 200, 200
	gob.MaxHitPoints, gob.HitPoints = 200, 200
	game.world.Monsters = []*monster.Monster3D{skel, gob}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	skelHP0, gobHP0, partyHP0 := skel.HitPoints, gob.HitPoints, partyHPSum(game)

	game.refreshBoundAllyCache()
	runOneMonsterTurn(game, gl)

	if gob.HitPoints >= gobHP0 {
		t.Errorf("bound undead should have struck the mob (HP %d -> %d)", gobHP0, gob.HitPoints)
	}
	if skel.HitPoints >= skelHP0 {
		t.Errorf("mob should have retaliated against the bound undead (HP %d -> %d)", skelHP0, skel.HitPoints)
	}
	if partyHPSum(game) != partyHP0 {
		t.Errorf("neither combatant may touch the party")
	}
}

// monsterStrikeMonster rewards the party whenever an ENEMY or a bound UNDEAD (a
// former enemy) falls, but a fallen CARD ALLY (a pure summon) yields nothing.
func TestMonsterStrike_RewardsRules(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cfg := game.config

	// Bound undead slays an enemy -> party gains XP.
	skel := monster.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	enemy := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	enemy.HitPoints = 1
	game.world.Monsters = []*monster.Monster3D{skel, enemy}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	xp0 := game.party.Members[0].Experience
	game.combat.monsterStrikeMonster(skel, enemy)
	if enemy.IsAlive() {
		t.Fatalf("1-HP enemy should have died")
	}
	if game.party.Members[0].Experience <= xp0 {
		t.Errorf("party should gain XP when a bound ally slays an enemy")
	}

	// A mob cuts down a bound UNDEAD (a former enemy) -> party STILL gains its XP.
	skel2 := monster.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	skel2.HitPoints = 1
	mob := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	game.world.Monsters = []*monster.Monster3D{skel2, mob}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(skel2, 300, "Bind Undead")
	xp1 := game.party.Members[0].Experience
	game.combat.monsterStrikeMonster(mob, skel2)
	if skel2.IsAlive() {
		t.Fatalf("1-HP bound undead should have died")
	}
	if game.party.Members[0].Experience <= xp1 {
		t.Errorf("party should gain a slain bound undead's XP (was %d, now %d)", xp1, game.party.Members[0].Experience)
	}

	// A mob cuts down a CARD ALLY (a pure summon) -> NO party XP.
	huntress := monster.NewMonster3DFromConfig(0, 0, "masked_huntress", cfg)
	huntress.HitPoints = 1
	mob2 := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)
	game.world.Monsters = []*monster.Monster3D{huntress, mob2}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	markCardAlly(huntress)
	xp2 := game.party.Members[0].Experience
	game.combat.monsterStrikeMonster(mob2, huntress)
	if huntress.IsAlive() {
		t.Fatalf("1-HP card ally should have died")
	}
	if game.party.Members[0].Experience != xp2 {
		t.Errorf("party must NOT gain XP when a card ally is slain (was %d, now %d)", xp2, game.party.Members[0].Experience)
	}
}

// A ranged bound undead (lich) looses a visible charmed-owned projectile at its
// enemy instead of dealing instant damage; the bolt never threatens the party.
func TestBindUndead_RangedLichFiresBoundProjectile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	cfg := game.config
	lich := monster.NewMonster3DFromConfig(float64(12)*ts+ts/2, float64(10)*ts+ts/2, "lich", cfg)
	enemy := monster.NewMonster3DFromConfig(float64(14)*ts+ts/2, float64(10)*ts+ts/2, "goblin", cfg)
	game.world.Monsters = []*monster.Monster3D{lich, enemy}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	if !lich.HasRangedAttack() {
		t.Fatalf("lich should be a ranged attacker (projectile_spell in monsters.yaml)")
	}
	game.combat.applyBindUndead(lich, 300, "Bind Undead")

	projCount := func() int { return len(game.magicProjectiles) + len(game.arrows) }
	n0, enemyHP0, partyHP0 := projCount(), enemy.HitPoints, partyHPSum(game)

	game.refreshBoundAllyCache() // sets lich.AIFoe (= the enemy)
	if !game.combat.boundAttackNearest(lich) {
		t.Fatalf("bound lich should have acted against the enemy")
	}
	if projCount() != n0+1 {
		t.Fatalf("bound lich should have fired exactly one projectile (was %d, now %d)", n0, projCount())
	}
	bolt := game.magicProjectiles[len(game.magicProjectiles)-1]
	if bolt.Owner != ProjectileOwnerBoundUndead {
		t.Errorf("lich bolt should be charmed-owned, got %v", bolt.Owner)
	}
	if bolt.VelX <= 0 {
		t.Errorf("lich bolt should fly toward the enemy (+X), got VelX=%.2f", bolt.VelX)
	}
	if enemy.HitPoints != enemyHP0 {
		t.Errorf("ranged bound undead must not deal instant damage; HP resolved on impact (%d -> %d)", enemyHP0, enemy.HitPoints)
	}
	// A charmed-owned bolt never damages the party.
	game.combat.CheckProjectilePlayerCollisions()
	if partyHPSum(game) != partyHP0 {
		t.Errorf("charmed projectile must never damage the party")
	}
}

// Crossfire symmetry: a ranged mob looses a visible bolt at either kind of bound
// ally (owner-tagged so it never hits the party). A slain bound undead was a
// former enemy, so it gives its normal XP and loot; a card ally is a pure summon
// and gives neither. A bound undead's bolt slaying an enemy also rewards party.
func TestCrossfire_MonsterProjectileVsMonster(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	cfg := game.config

	bandit := monster.NewMonster3DFromConfig(float64(12)*ts+ts/2, float64(10)*ts+ts/2, "bandit", cfg)
	undead := monster.NewMonster3DFromConfig(float64(13)*ts+ts/2, float64(10)*ts+ts/2, "skeleton", cfg)
	undead.MaxHitPoints, undead.HitPoints = 200, 1
	undead.Experience, undead.Gold = 40, 17
	game.world.Monsters = []*monster.Monster3D{bandit, undead}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(undead, 300, "Bind Undead")
	if !bandit.HasRangedAttack() {
		t.Fatalf("bandit should be a ranged mob")
	}

	// Mob fires an anti-undead bolt; it is tagged so it can only hit the undead.
	if !game.combat.spawnMonsterRangedAttackAtMonster(bandit, undead, ProjectileOwnerMonsterAtBound) {
		t.Fatalf("mob should have spawned a bolt")
	}
	bolt := &game.arrows[len(game.arrows)-1]
	if bolt.Owner != ProjectileOwnerMonsterAtBound {
		t.Errorf("mob bolt should be owner MonsterAtBound, got %v", bolt.Owner)
	}

	// Resolve the bolt on the bound undead -> it dies and, as a former enemy,
	// gives its normal XP and gold bag.
	xp0, bags0 := game.party.Members[0].Experience, len(game.groundContainers)
	game.combat.resolveMonsterProjectileVsMonster(bolt, "arrow", undead, bolt.ID)
	if undead.IsAlive() {
		t.Error("bound undead should die to the lethal bolt")
	}
	if game.party.Members[0].Experience <= xp0 || len(game.groundContainers) != bags0+1 {
		t.Errorf("a slain bound undead must give XP and loot (xp %d -> %d, bags %d -> %d)", xp0, game.party.Members[0].Experience, bags0, len(game.groundContainers))
	}

	// The identical projectile path must not reward a card ally.
	ally := monster.NewMonster3DFromConfig(float64(14)*ts+ts/2, float64(10)*ts+ts/2, "masked_huntress", cfg)
	ally.HitPoints, ally.Experience, ally.Gold = 1, 40, 17
	markCardAlly(ally)
	game.world.Monsters = []*monster.Monster3D{bandit, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	if !game.combat.spawnMonsterRangedAttackAtMonster(bandit, ally, ProjectileOwnerMonsterAtBound) {
		t.Fatal("mob should fire at a card ally")
	}
	cardBolt := &game.arrows[len(game.arrows)-1]
	xpCard, bagsCard := game.party.Members[0].Experience, len(game.groundContainers)
	game.combat.resolveMonsterProjectileVsMonster(cardBolt, "arrow", ally, cardBolt.ID)
	if ally.IsAlive() {
		t.Error("card ally should die to the lethal bolt")
	}
	if game.party.Members[0].Experience != xpCard || len(game.groundContainers) != bagsCard {
		t.Errorf("a slain card ally must give no XP or loot (xp %d -> %d, bags %d -> %d)", xpCard, game.party.Members[0].Experience, bagsCard, len(game.groundContainers))
	}

	// A bound undead's bolt that slays an enemy DOES reward the party.
	enemy := monster.NewMonster3DFromConfig(float64(20)*ts+ts/2, float64(10)*ts+ts/2, "goblin", cfg)
	enemy.HitPoints = 1
	lich := monster.NewMonster3DFromConfig(float64(18)*ts+ts/2, float64(10)*ts+ts/2, "lich", cfg)
	game.world.Monsters = append(game.world.Monsters, enemy, lich)
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(lich, 300, "Bind Undead")
	if !game.combat.spawnMonsterRangedAttackAtMonster(lich, enemy, ProjectileOwnerBoundUndead) {
		t.Fatalf("lich should have spawned a bolt")
	}
	mbolt := &game.magicProjectiles[len(game.magicProjectiles)-1]
	xp1 := game.party.Members[0].Experience
	game.combat.resolveMonsterProjectileVsMonster(mbolt, "magic_projectile", enemy, mbolt.ID)
	if enemy.IsAlive() {
		t.Fatalf("1-HP enemy should die to the charmed bolt")
	}
	if game.party.Members[0].Experience <= xp1 {
		t.Errorf("party should gain XP when a bound ally's bolt slays an enemy")
	}
}

// In turn-based mode a bound undead actively HUNTS: it walks toward an enemy
// that is out of reach and only strikes once it has closed to attack range
// (no more hitting across the room, no standing still).
func TestBindUndead_TBSeeksAndWalksToEnemy(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 5, 5, ts) // far from the fight (>vision) so the enemy stays put
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(float64(10)*ts+ts/2, float64(10)*ts+ts/2, "skeleton", cfg)
	enemy := monster.NewMonster3DFromConfig(float64(13)*ts+ts/2, float64(10)*ts+ts/2, "goblin", cfg) // 3 tiles east
	skel.MaxHitPoints, skel.HitPoints = 200, 200
	enemy.MaxHitPoints, enemy.HitPoints = 200, 200
	game.world.Monsters = []*monster.Monster3D{skel, enemy}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(skel, 300, "Bind Undead")
	game.refreshBoundAllyCache()

	// Out of melee reach (3 tiles) -> must NOT strike yet...
	if game.combat.boundAttackNearest(skel) {
		t.Errorf("bound melee undead should not strike from 3 tiles away")
	}
	// ...but it should be seeking that enemy (its pursuit target is the enemy).
	if tx, ty := game.combat.monsterAITargetPoint(skel); tx != enemy.X || ty != enemy.Y {
		t.Errorf("bound undead should target the enemy to hunt it, got (%.0f,%.0f)", tx, ty)
	}

	startDist, enemyHP0 := Distance(skel.X, skel.Y, enemy.X, enemy.Y), enemy.HitPoints
	for turn := 0; turn < 6; turn++ {
		game.refreshBoundAllyCache()
		runOneMonsterTurn(game, gl)
	}
	if Distance(skel.X, skel.Y, enemy.X, enemy.Y) >= startDist {
		t.Errorf("bound undead should have walked closer to the enemy (start %.0f)", startDist)
	}
	if enemy.HitPoints >= enemyHP0 {
		t.Errorf("bound undead should have closed in and struck the enemy (HP still %d)", enemy.HitPoints)
	}
}

// In real-time mode a bound undead also HUNTS: it walks toward an out-of-reach
// enemy (overriding normal detection range) and only strikes once in range.
func TestBindUndead_RTSeeksAndWalksToEnemy(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	placePlayerAtTile(game, 5, 5, ts) // far away - the bound undead, not the player, drives this
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(float64(10)*ts+ts/2, float64(10)*ts+ts/2, "skeleton", cfg)
	enemy := monster.NewMonster3DFromConfig(float64(14)*ts+ts/2, float64(10)*ts+ts/2, "goblin", cfg) // 4 tiles east, outside skel's alert radius
	skel.MaxHitPoints, skel.HitPoints = 300, 300
	enemy.MaxHitPoints, enemy.HitPoints = 300, 300
	game.world.Monsters = []*monster.Monster3D{skel, enemy}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.combat.applyBindUndead(skel, 999, "Bind Undead")

	startDist, enemyHP0 := Distance(skel.X, skel.Y, enemy.X, enemy.Y), enemy.HitPoints
	for f := 0; f < 1500; f++ { // ~12s at 120 TPS - plenty to close 3 tiles and strike
		game.refreshBoundAllyCache()
		// Fresh snapshot + wrapper each tick, mirroring the real two-phase RT
		// tick (a snapshot taken once at the top of the loop would go stale).
		mw := CreateMonsterWrapper(skel, game.collisionSystem, game.collisionSystem.Snapshot(), game)
		mw.Update()
		mw.ApplyCollisionUpdate()
		game.combat.HandleMonsterInteractions()
		if enemy.HitPoints < enemyHP0 {
			break
		}
	}
	if Distance(skel.X, skel.Y, enemy.X, enemy.Y) >= startDist {
		t.Errorf("bound undead should have walked closer in real time (start %.0f)", startDist)
	}
	if enemy.HitPoints >= enemyHP0 {
		t.Errorf("bound undead should have hunted down and struck the enemy in real time")
	}
}

// A bound monster perishing on map-leave grants XP but no gold.
func TestBindUndead_MapLeaveGrantsXPNoGold(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	skel := monster.NewMonster3DFromConfig(0, 0, "skeleton", game.config)
	gold0 := game.party.Gold
	xp0 := game.party.Members[0].Experience

	game.combat.awardExperienceOnly(skel)

	if game.party.Members[0].Experience <= xp0 {
		t.Errorf("party should gain XP from a bound monster's demise")
	}
	if game.party.Gold != gold0 {
		t.Errorf("bound monster demise must grant NO gold (was %d, now %d)", gold0, game.party.Gold)
	}
}

// Resurrect restores a fallen ally - even an eradicated one - to full HP.
func TestResurrect_RestoresFallenAlly(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	fallen := game.party.Members[1]
	fallen.HitPoints = 0
	fallen.AddCondition(character.ConditionEradicated) // hardest case: eradicated

	equipSpellAndPrepareCaster(t, game.combat, "resurrect", 100, 30) // caster = Members[0] (alive)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("resurrect cast failed")
	}

	if fallen.HitPoints != fallen.MaxHitPoints {
		t.Errorf("resurrect should restore to full HP, got %d/%d", fallen.HitPoints, fallen.MaxHitPoints)
	}
	if fallen.HasCondition(character.ConditionEradicated) ||
		fallen.HasCondition(character.ConditionUnconscious) ||
		fallen.HasCondition(character.ConditionDead) {
		t.Errorf("resurrect should clear all death conditions")
	}
	def, _ := spells.GetSpellDefinitionByID("resurrect")
	if !def.Revive || !def.FullHeal {
		t.Errorf("resurrect should be revive + full_heal in YAML")
	}
}

// Magic Ring (intellect_scaling_divisor 6, personality_scaling_divisor 8) boosts
// the wearer's effective Intellect and Personality - confirming the scaling-
// divisor accessory chain (YAML -> Attributes -> calculateEquipmentBonuses) works.
func TestMagicRing_BoostsEffectiveStats(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	c := game.party.Members[0]
	c.Intellect = 60
	c.Personality = 40
	baseInt := c.GetEffectiveIntellect()
	basePer := c.GetEffectivePersonality()

	ring := items.CreateItemFromYAML("magic_ring")
	if ring.Attributes["intellect_scaling_divisor"] != 6 || ring.Attributes["personality_scaling_divisor"] != 8 {
		t.Fatalf("magic_ring attributes not populated: %v", ring.Attributes)
	}
	if _, _, ok := c.EquipItem(ring); !ok {
		t.Fatalf("could not equip magic_ring")
	}

	if got, want := c.GetEffectiveIntellect(), baseInt+c.Intellect/6; got != want {
		t.Errorf("effective Intellect with ring: got %d, want %d (+Int/6)", got, want)
	}
	if got, want := c.GetEffectivePersonality(), basePer+c.Personality/8; got != want {
		t.Errorf("effective Personality with ring: got %d, want %d (+Per/8)", got, want)
	}
}

// Status-icon resolution: a legacy token maps to its dedicated status_* sprite;
// an unknown token (no status_ art, no sprite manager in tests) degrades to a
// text fallback rather than crashing. (The real status_<token> -> icon_spell_<token>
// preference is exercised at runtime where sprites/CWD are available.)
func TestStatusIconResolver_LegacyAndFallback(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	if icon, _ := game.resolveStatusIconSprite("bless"); icon != "status_bless" {
		t.Errorf("legacy token 'bless' should resolve to status_bless, got %q", icon)
	}
	if icon, _ := game.resolveStatusIconSprite("water_breathing"); icon != "status_water_breathing" {
		t.Errorf("legacy token 'water_breathing' should resolve to status_water_breathing, got %q", icon)
	}
	// Unknown token must not panic even when no status_/icon_spell_ sprite resolves.
	_, _ = game.resolveStatusIconSprite("day_of_the_gods")
}

// bind_undead is a dark-school spell.
func TestBindUndead_IsDarkSchool(t *testing.T) {
	tbBehaviorGame(t, 5, 5) // loads spell config
	def, _ := spells.GetSpellDefinitionByID("bind_undead")
	if def.School != "dark" {
		t.Errorf("bind_undead should be dark school, got %q", def.School)
	}
}

// Real-time melee reaches exactly one tile (inclusive): a monster sitting on an
// adjacent tile center (64px away == attack radius) still lands its hit.
func TestRealTime_MeleeHitsAtExactlyOneTile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 11, 10, ts) // exactly one tile away
	m.State = monster.StateAttacking
	m.StateTimer = 1 // attack fires on the first frame of the attacking state

	hp0 := partyHPSum(game)
	game.combat.HandleMonsterInteractions()
	if partyHPSum(game) >= hp0 {
		t.Fatalf("real-time melee should hit at exactly one tile (inclusive reach)")
	}
}

func TestRealTime_MeleeHitsFromDiagonalAdjacentTile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 11, 11, ts)
	m.State = monster.StateAttacking
	m.StateTimer = 1

	hp0 := partyHPSum(game)
	game.combat.HandleMonsterInteractions()
	if partyHPSum(game) >= hp0 {
		t.Fatalf("real-time melee should hit from a diagonal adjacent tile")
	}
}

// Switching modes clears per-character RT cooldowns so a cooldown set before a
// turn-based fight doesn't gate RT actions afterwards.
func TestModeSwitch_ClearsRTCooldowns(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5) // starts in turn-based
	for _, m := range game.party.Members {
		if m != nil {
			m.RTCooldown = 600 // ~5s of frozen cooldown from a recent RT attack
		}
	}
	game.spellInputCooldown = 50

	game.ToggleTurnBasedMode() // TB -> RT

	if game.turnBasedMode {
		t.Fatalf("expected real-time mode after the toggle")
	}
	for i, m := range game.party.Members {
		if m != nil && m.RTCooldown != 0 {
			t.Errorf("member %d RTCooldown should be cleared on mode switch, got %d", i, m.RTCooldown)
		}
	}
	if game.spellInputCooldown != 0 {
		t.Errorf("global input stagger should be cleared on mode switch, got %d", game.spellInputCooldown)
	}
}

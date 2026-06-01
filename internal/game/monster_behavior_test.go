package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// These tests cover the turn-based / real-time monster movement and attack
// rules: tile-centering, cardinal-only melee that never enters the player's
// tile, ranged firing only when row/column-aligned, the 1-tile real-time melee
// reach, and the puma pounce landing on an adjacent tile (not on the player).

func absI(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tbBehaviorGame builds a turn-based game on an empty w×h world with the combat
// system wired, and returns it plus a GameLoop and the tile size.
func tbBehaviorGame(t *testing.T, w, h int) (*MMGame, *GameLoop, float64) {
	t.Helper()
	cfg := loadTestConfig(t)
	worldTest := newTestWorldSized(cfg, w, h)
	game := newTestGame(cfg, worldTest)
	game.turnBasedMode = true
	game.combat = NewCombatSystem(game)
	// Zero party Luck so perfect-dodge (luck/5 % RNG) never fires — these tests
	// assert that a melee/pounce hit lands, which must be deterministic.
	for _, c := range game.party.Members {
		c.Luck = 0
	}
	gl := &GameLoop{game: game, combat: game.combat}
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

// Melee monsters approach one tile per turn, only ever strike from a
// cardinally-adjacent tile (Manhattan distance 1), and never step onto the
// player's own tile.
func TestTurnBased_MeleeOnlyHitsFromCardinalAdjacent(t *testing.T) {
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
			if man := absI(tx-ptx) + absI(ty-pty); man != 1 {
				t.Fatalf("turn %d: melee hit from a non-cardinal-adjacent tile (Manhattan %d)", turn, man)
			}
		}
	}
	if !everHit {
		t.Fatalf("melee monster never closed in to strike the party")
	}
}

// A melee monster diagonally adjacent to the player must NOT attack; it should
// reposition onto a cardinally-adjacent tile instead.
func TestTurnBased_MeleeDoesNotAttackDiagonally(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 11, 11, ts) // diagonal neighbour

	hp0 := partyHPSum(game)
	runOneMonsterTurn(game, gl)

	if partyHPSum(game) < hp0 {
		t.Fatalf("melee monster attacked from a diagonal tile")
	}
	tx, ty := monsterTileCoords(m, ts)
	if man := absI(tx-10) + absI(ty-10); man != 1 {
		t.Fatalf("expected diagonal monster to reposition to a cardinal-adjacent tile, Manhattan=%d", man)
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
	// Diagonal → must NOT shoot.
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

	// Aligned and in range → must shoot.
	game2, gl2, ts2 := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game2, 10, 10, ts2)
	spawnMonsterAtTile(game2, "elf_archer", 13, 10, ts2) // same row, 3 tiles (range 5)
	runOneMonsterTurn(game2, gl2)
	if len(game2.arrows) == 0 {
		t.Fatalf("aligned ranged monster in range should fire")
	}
}

// A pouncing monster (puma) leaps onto a cardinally-adjacent tile — never onto
// the player's tile — and strikes. Turn-based path.
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
	if man := absI(tx-ptx) + absI(ty-pty); man != 1 {
		t.Fatalf("puma should land on a cardinally-adjacent tile, Manhattan=%d", man)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("puma pounce should strike the party")
	}
}

// Real-time pounce lands on a cardinally-adjacent tile (not the player's) and
// deals damage.
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
	if man := absI(tx-ptx) + absI(ty-pty); man != 1 {
		t.Fatalf("real-time puma should land cardinally adjacent, Manhattan=%d", man)
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

// Day of the Gods grants the party a 50% incoming-damage reduction for its
// (YAML-driven) duration.
func TestDayOfTheGods_ResistBuff(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "day_of_the_gods", 100, 30)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("day_of_the_gods cast failed")
	}
	if !game.dayGodsActive || game.dayGodsResistPct != 50 {
		t.Fatalf("expected 50%% resist active, got active=%v pct=%d", game.dayGodsActive, game.dayGodsResistPct)
	}
	if got := game.combat.mitigateIncoming(100); got != 50 {
		t.Errorf("100 incoming with 50%% resist should be 50, got %d", got)
	}
	def, _ := spells.GetSpellDefinitionByID("day_of_the_gods")
	if want := def.Duration * game.config.GetTPS(); game.dayGodsDuration != want {
		t.Errorf("duration frames: got %d, want %d (%ds × TPS)", game.dayGodsDuration, want, def.Duration)
	}
}

// Hour of Power: +15 outgoing damage and -5 incoming (floored at 0).
func TestHourOfPower_DamageBuffs(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "hour_of_power", 100, 30)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("hour_of_power cast failed")
	}
	if !game.hourPowerActive || game.hourPowerOutBonus != 15 || game.hourPowerInReduce != 5 {
		t.Fatalf("hour_of_power: active=%v out=%d in=%d (want true/15/5)", game.hourPowerActive, game.hourPowerOutBonus, game.hourPowerInReduce)
	}
	if got := game.combat.mitigateIncoming(10); got != 5 {
		t.Errorf("10 incoming -5 should be 5, got %d", got)
	}
	if got := game.combat.mitigateIncoming(3); got != 0 {
		t.Errorf("3 incoming -5 should floor at 0, got %d", got)
	}
}

// Both party buffs stack on incoming damage: % reduction first, then the flat -5.
func TestPartyBuffs_StackOnIncoming(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "day_of_the_gods", 100, 30)
	game.combat.CastEquippedSpell()
	equipSpellAndPrepareCaster(t, game.combat, "hour_of_power", 100, 30)
	game.combat.CastEquippedSpell()
	// 100 → 50% reduction → 50 → flat -5 → 45
	if got := game.combat.mitigateIncoming(100); got != 45 {
		t.Errorf("100 with 50%% resist then -5 should be 45, got %d", got)
	}
}

// Bind Undead only charms undead; the living are immune. Also checks the spell
// is a no-damage charm in YAML.
func TestBindUndead_CharmsUndeadOnly(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cfg := game.config
	skel := monster.NewMonster3DFromConfig(0, 0, "skeleton", cfg)
	gob := monster.NewMonster3DFromConfig(0, 0, "goblin", cfg)

	game.combat.applyCharm(skel, 300, "Bind Undead")
	game.combat.applyCharm(gob, 300, "Bind Undead")

	if !skel.Charmed {
		t.Errorf("undead skeleton should be charmed")
	}
	if skel.CharmFramesRemaining != 300*cfg.GetTPS() {
		t.Errorf("charm frames: got %d, want %d", skel.CharmFramesRemaining, 300*cfg.GetTPS())
	}
	if gob.Charmed {
		t.Errorf("living goblin must NOT be charmable")
	}

	def, _ := spells.GetSpellDefinitionByID("bind_undead")
	if !def.Charm || !def.DealsNoDamage {
		t.Errorf("bind_undead should be a no-damage charm spell (charm=%v, noDmg=%v)", def.Charm, def.DealsNoDamage)
	}
}

// A charmed undead attacks the nearest other monster and never the party.
func TestBindUndead_CharmedFightsOtherMonsterNotParty(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	// Both monsters far from the player (out of vision), adjacent to each other.
	skel := monster.NewMonster3DFromConfig(float64(20)*ts+ts/2, float64(10)*ts+ts/2, "skeleton", game.config)
	gob := monster.NewMonster3DFromConfig(float64(21)*ts+ts/2, float64(10)*ts+ts/2, "goblin", game.config)
	game.world.Monsters = []*monster.Monster3D{skel, gob}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.combat.applyCharm(skel, 300, "Bind Undead")
	gobHP0 := gob.HitPoints
	partyHP0 := partyHPSum(game)

	runOneMonsterTurn(game, gl)

	if gob.HitPoints >= gobHP0 {
		t.Errorf("charmed skeleton should have struck the goblin (HP %d -> %d)", gobHP0, gob.HitPoints)
	}
	if partyHPSum(game) != partyHP0 {
		t.Errorf("charmed undead must never damage the party")
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

// Resurrect restores a fallen ally — even an eradicated one — to full HP.
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
// the wearer's effective Intellect and Personality — confirming the scaling-
// divisor accessory chain (YAML → Attributes → calculateEquipmentBonuses) works.
func TestMagicRing_BoostsEffectiveStats(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	c := game.party.Members[0]
	c.Intellect = 60
	c.Personality = 40
	baseInt := c.GetEffectiveIntellect(0)
	basePer := c.GetEffectivePersonality(0)

	ring := items.CreateItemFromYAML("magic_ring")
	if ring.Attributes["intellect_scaling_divisor"] != 6 || ring.Attributes["personality_scaling_divisor"] != 8 {
		t.Fatalf("magic_ring attributes not populated: %v", ring.Attributes)
	}
	if _, _, ok := c.EquipItem(ring); !ok {
		t.Fatalf("could not equip magic_ring")
	}

	if got, want := c.GetEffectiveIntellect(0), baseInt+c.Intellect/6; got != want {
		t.Errorf("effective Intellect with ring: got %d, want %d (+Int/6)", got, want)
	}
	if got, want := c.GetEffectivePersonality(0), basePer+c.Personality/8; got != want {
		t.Errorf("effective Personality with ring: got %d, want %d (+Per/8)", got, want)
	}
}

// Status-icon resolution: a legacy token maps to its dedicated status_* sprite;
// an unknown token (no status_ art, no sprite manager in tests) degrades to a
// text fallback rather than crashing. (The real status_<token> → icon_spell_<token>
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

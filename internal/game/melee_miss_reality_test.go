package game

// Reality checks for the reported RT melee "misses on an adjacent monster".
// Each test pins the behavior of one concrete miss cause, across arc types
// 1-4 where the arc matters. Causes 2-4 (arc dead zones, one-flank arc 2,
// off-hand arc/range) are deliberate design and pinned as such; cause 1
// (tile-index reach) is FIXED and pinned: true pixel reach within (range+0.5)
// tiles. Whiffs spend the action/cooldown but stay silent.

import (
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// rtMeleeGame is tbBehaviorGame flipped to real-time (the reported mode).
func rtMeleeGame(t *testing.T) (*MMGame, *CombatSystem, float64) {
	t.Helper()
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	return game, game.combat, ts
}

// tankMobAt spawns an unkillable goblin at exact pixel coords so a test swing
// can never remove it from the world mid-test.
func tankMobAt(game *MMGame, x, y float64) *monster.Monster3D {
	m := monster.NewMonster3DFromConfig(x, y, "goblin", game.config)
	m.HitPoints, m.MaxHitPoints = 1000, 1000
	return m
}

func setMobs(game *MMGame, mobs ...*monster.Monster3D) {
	game.world.Monsters = mobs
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
}

func hurt(m *monster.Monster3D) bool { return m.HitPoints < m.MaxHitPoints }

func hasCombatMsg(game *MMGame, substr string) bool {
	for _, e := range game.combatLogHistory {
		if strings.Contains(e.Text, substr) {
			return true
		}
	}
	return false
}

// Cause 1 (FIXED): reach is tile-index Chebyshev PLUS a true pixel-distance
// fallback of (range+0.5) tiles. A mob ~1.06 tiles away straddling two tile
// boundaries (tile-cheb 2) is now in reach for every arc; tile adjacency
// still hits on its own (no nerf to the old envelope); genuinely distant mobs
// stay out of reach.
func TestRTMelee_TileStraddle_CloseIsReachable_AllArcs(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const pty = 10
	axe, err := items.TryCreateWeaponFromYAML("steel_axe") // range 1
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}

	// Player at the right edge of tile 10; mob at the left edge of tile 12:
	// actual gap = ts+4 px (~1.06 tiles), tile-index gap = 2.
	game.camera.X, game.camera.Y = 11*ts-2, float64(pty)*ts+ts/2
	game.camera.Angle = 0 // face +X
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	for arc := 1; arc <= 4; arc++ {
		mob := tankMobAt(game, 12*ts+2, game.camera.Y)
		setMobs(game, mob)
		cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: arc}, false)
		if !hurt(mob) {
			t.Errorf("arc %d: mob 1.06 tiles away (tile-cheb 2) must be in pixel reach now", arc)
		}
	}

	// Beyond the pixel envelope AND tile adjacency: still a miss.
	out := tankMobAt(game, 12*ts+44, game.camera.Y) // 1.72 tiles away, tile-cheb 2
	setMobs(game, out)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 4}, false)
	if hurt(out) {
		t.Errorf("mob 1.72 tiles away must stay out of range-1 reach")
	}

	// Tile adjacency alone still suffices (old envelope kept): player at the
	// LEFT edge of tile 10, mob at the right edge of tile 11 - 1.94 tiles
	// apart but tile-cheb 1 -> hittable.
	game.camera.X = 10*ts + 2
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
	far := tankMobAt(game, 12*ts-2, game.camera.Y)
	setMobs(game, far)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if !hurt(far) {
		t.Errorf("tile-adjacent mob (1.94 tiles, cheb=1) must stay hittable")
	}
}

// Cause 2 (dead zones): the swing cone is centered on the facing; adjacent
// tiles outside it are unreachable no matter how close. Matrix of a LONE
// adjacent mob per position per arc. Note arc 1's diagonal ASSIST: with no
// front target it still picks one front-diagonal mob.
func TestRTMelee_ArcDeadZones_LoneAdjacentMob_AllArcs(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}

	positions := []struct {
		name   string
		dx, dy int
	}{
		{"front 0deg", 1, 0},
		{"diagonal 45deg", 1, 1},
		{"side 90deg", 0, 1},
		{"rear-diagonal 135deg", -1, 1},
		{"rear 180deg", -1, 0},
	}
	// want[arc-1][position]
	want := [4][5]bool{
		{true, true, false, false, false}, // arc 1 (diag via assist)
		{true, true, false, false, false}, // arc 2
		{true, true, false, false, false}, // arc 3
		{true, true, true, false, false},  // arc 4 (side included, rear never)
	}

	for arc := 1; arc <= 4; arc++ {
		for i, pos := range positions {
			mob := tankMobAt(game,
				float64(ptx+pos.dx)*ts+ts/2, float64(pty+pos.dy)*ts+ts/2)
			setMobs(game, mob)
			cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: arc}, false)
			if got := hurt(mob); got != want[arc-1][i] {
				t.Errorf("arc %d, lone mob %s: hit=%v, want %v", arc, pos.name, got, want[arc-1][i])
			}
		}
	}
}

// Cause 2 (arc-1 assist suppression): once ANY front target is hit, arc 1
// ignores the diagonal completely - the assist only fires on an empty front.
func TestRTMelee_ArcOne_FrontTargetSuppressesDiagonalAssist(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	front := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty)*ts+ts/2)
	diag := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty+1)*ts+ts/2)
	setMobs(game, front, diag)

	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if !hurt(front) || hurt(diag) {
		t.Errorf("arc 1 with a front target must hit front only, got front=%v diag=%v",
			hurt(front), hurt(diag))
	}
}

// Cause 3: arc 2 (the starter iron_sword) hits front + exactly ONE flank,
// randomly chosen when both diagonals hold a foe - the one you were watching
// can be the one skipped.
func TestRTMelee_ArcTwo_OnlyOneFlankPerSwing(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	front := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty)*ts+ts/2)
	left := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty-1)*ts+ts/2)
	right := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty+1)*ts+ts/2)
	setMobs(game, front, left, right)

	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 2}, false)

	if !hurt(front) {
		t.Fatalf("arc 2 must always hit the front target")
	}
	if hurt(left) == hurt(right) {
		t.Errorf("arc 2 must hit exactly ONE flank, got left=%v right=%v", hurt(left), hurt(right))
	}
}

// Cause 4 (arc): a dual-wielder's off-hand swing silently uses the OFF-hand
// weapon's arc. Main iron_sword (arc 2) reaches the diagonal; the off-hand
// magic_dagger (arc 1) does not once a front target absorbs the swing - so
// every other swing "misses" the diagonal mob the sword just hit.
func TestRTMelee_OffHandSwing_UsesOffHandArc(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0
	game.selectedChar = 0
	member := game.party.Members[0]
	makeDualWielder(t, member) // main iron_sword (arc 2) + off magic_dagger (arc 1)
	member.Might = 60          // damage well past goblin armor

	front := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty)*ts+ts/2)
	diag := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty+1)*ts+ts/2)
	setMobs(game, front, diag)

	// Main hand ready: sword swing (arc 2) reaches front + the diagonal flank.
	member.RTCooldown, member.OffHandRTCooldown = 0, 0
	if !cs.EquipmentMeleeAttack() {
		t.Fatalf("main-hand swing should act")
	}
	if !hurt(front) || !hurt(diag) {
		t.Fatalf("sword (arc 2) swing must hit front+diagonal, got front=%v diag=%v",
			hurt(front), hurt(diag))
	}

	front.HitPoints, diag.HitPoints = front.MaxHitPoints, diag.MaxHitPoints

	// Main hand on cooldown: the SAME key now swings the off-hand dagger
	// (arc 1) - front absorbs it, the diagonal mob is untouched.
	member.RTCooldown, member.OffHandRTCooldown = 30, 0
	if !cs.EquipmentMeleeAttack() {
		t.Fatalf("off-hand swing should act")
	}
	if !hurt(front) || hurt(diag) {
		t.Errorf("dagger (arc 1) off-hand swing must hit front only, got front=%v diag=%v",
			hurt(front), hurt(diag))
	}
}

// Cause 4 (reach) + cause 5 (FIXED): off-hand swing also uses the OFF-hand
// weapon's RANGE - a spear main hand reaches 2 tiles, the dagger off-hand
// doesn't; the whiffed off-hand swing still reports acted=true (cooldown is
// charged) but now announces itself in the combat log.
func TestRTMelee_OffHandSwing_UsesOffHandRange_AndWhiffStillActs(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0
	game.selectedChar = 0
	member := game.party.Members[0]
	member.Skills[character.SkillSpear] = &character.Skill{Mastery: character.MasteryNovice}
	member.Skills[character.SkillDagger] = &character.Skill{Mastery: character.MasteryNovice}
	member.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryNovice}
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_spear")  // range 2
	member.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("magic_dagger") // range 1
	member.Might = 60

	mob := tankMobAt(game, float64(ptx+2)*ts+ts/2, float64(pty)*ts+ts/2) // 2 tiles ahead
	setMobs(game, mob)

	member.RTCooldown, member.OffHandRTCooldown = 0, 0
	if !cs.EquipmentMeleeAttack() || !hurt(mob) {
		t.Fatalf("spear (range 2) must reach the mob 2 tiles ahead")
	}

	mob.HitPoints = mob.MaxHitPoints

	member.RTCooldown, member.OffHandRTCooldown = 30, 0
	acted := cs.EquipmentMeleeAttack()
	if hurt(mob) {
		t.Fatalf("dagger (range 1) off-hand swing must NOT reach 2 tiles")
	}
	if !acted {
		t.Errorf("whiffed swing still reports acted=true (cooldown gets charged) by design")
	}
	if hasCombatMsg(game, "swings at air") {
		t.Errorf("whiffs are silent by design")
	}
}

// A swing with the only mob BEHIND the player connects with nothing, still
// costs the action, and logs nothing.
func TestRTMelee_WhiffBehindIsSilentButActs(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0
	game.selectedChar = 0
	member := game.party.Members[0]
	member.Skills[character.SkillSword] = &character.Skill{Mastery: character.MasteryNovice}
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")

	mob := tankMobAt(game, float64(ptx-1)*ts+ts/2, float64(pty)*ts+ts/2) // directly behind
	setMobs(game, mob)

	if !cs.EquipmentMeleeAttack() {
		t.Fatalf("swing at air still counts as acting (this is the documented behavior)")
	}
	if hurt(mob) {
		t.Errorf("no arc reaches a mob directly behind; if it got hit, the dead-zone matrix changed")
	}
	if hasCombatMsg(game, "swings at air") {
		t.Errorf("whiffs are silent by design")
	}
}

// Facing at the swing instant: the cone is centered on the camera angle in the
// frame the swing resolves. A mob straight ahead by TILE is missed by arcs 1-3
// when the player has turned 60deg away (held-R while strafing), while arc 4's
// 90deg cone still connects.
func TestRTMelee_SwingUsesCameraFacingAtThatInstant_AllArcs(t *testing.T) {
	game, cs, ts := rtMeleeGame(t)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = math.Pi / 3 // turned 60deg off the +X mob

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	want := map[int]bool{1: false, 2: false, 3: false, 4: true}
	for arc := 1; arc <= 4; arc++ {
		mob := tankMobAt(game, float64(ptx+1)*ts+ts/2, float64(pty)*ts+ts/2)
		setMobs(game, mob)
		cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: arc}, false)
		if got := hurt(mob); got != want[arc] {
			t.Errorf("arc %d with facing 60deg off: hit=%v, want %v", arc, got, want[arc])
		}
	}
}

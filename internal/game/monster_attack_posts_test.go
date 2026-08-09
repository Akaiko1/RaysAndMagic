package game

import (
	"math"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

func hostileMonsterAt(game *MMGame, tx, ty int, tileSize float64) *monster.Monster3D {
	m := monster.NewMonster3DFromConfig(float64(tx)*tileSize+tileSize/2, float64(ty)*tileSize+tileSize/2, "goblin", game.config)
	m.IsEngagingPlayer = true
	m.WasAttacked = true
	return m
}

func TestCombatAttackPostsReserveOneMobPerTile(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	const ptx, pty = 14, 14
	placePlayerAtTile(game, ptx, pty, tileSize)

	offsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	mobs := make([]*monster.Monster3D, 0, len(offsets)+1)
	for _, off := range offsets {
		m := hostileMonsterAt(game, ptx+off[0], pty+off[1], tileSize)
		m.State = monster.StateAttacking
		mobs = append(mobs, m)
	}
	duplicate := hostileMonsterAt(game, ptx+1, pty, tileSize)
	duplicate.State = monster.StateAttacking
	mobs = append(mobs, duplicate)
	game.world.Monsters = mobs
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	gl.reconcileMonsterAttackPosts()

	posts := 0
	postTiles := map[[2]int]bool{}
	for _, m := range mobs {
		if !m.AttackPost {
			continue
		}
		posts++
		key := [2]int{int(m.X / tileSize), int(m.Y / tileSize)}
		if postTiles[key] {
			t.Fatalf("two monsters claimed attack post %v", key)
		}
		postTiles[key] = true
		entity := game.collisionSystem.GetEntityByID(m.ID)
		if entity == nil || entity.CollisionType != collision.CollisionTypeMonsterEngaged || entity.Solid {
			t.Fatalf("post holder %s collision = %#v, want non-solid logical post", m.ID, entity)
		}
	}
	if posts != len(offsets) {
		t.Fatalf("claimed posts = %d, want one for every free adjacent tile (%d)", posts, len(offsets))
	}
	// The tie-break between two contenders on one tile is the stable ID order -
	// deliberately arbitrary (random IDs), not spawn order. The rule is only:
	// exactly one claims the post, the loser demotes to pursuing transit.
	holders := 0
	for _, contender := range []*monster.Monster3D{mobs[0], duplicate} {
		if contender.AttackPost {
			holders++
			continue
		}
		if contender.State != monster.StatePursuing || !contender.AttackTransit {
			t.Fatalf("losing contender must become transit: post=%v state=%v transit=%v",
				contender.AttackPost, contender.State, contender.AttackTransit)
		}
	}
	if holders != 1 {
		t.Fatalf("same-tile contenders holding posts = %d, want exactly 1", holders)
	}
}

func TestRTMonsterOnReservedPostKeepsSeeking(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	game.turnBasedMode = false
	const ptx, pty = 14, 14
	placePlayerAtTile(game, ptx, pty, tileSize)

	holder := hostileMonsterAt(game, ptx+1, pty, tileSize)
	holder.State = monster.StateAttacking
	contender := hostileMonsterAt(game, ptx+1, pty, tileSize)
	contender.State = monster.StatePursuing
	for _, m := range []*monster.Monster3D{holder, contender} {
		m.AITargetX, m.AITargetY = game.camera.X, game.camera.Y
	}
	game.world.Monsters = []*monster.Monster3D{holder, contender}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshMonsterCollisionState(holder)

	wrapper := &MonsterWrapper{
		Monster:         contender,
		collisionSystem: game.collisionSystem,
		snapshot:        game.collisionSystem.Snapshot(),
		game:            game,
	}
	wrapper.Update()
	wrapper.ApplyCollisionUpdate()
	gl.reconcileMonsterAttackPosts()

	if contender.State == monster.StateAttacking || contender.AttackPost {
		t.Fatalf("RT contender claimed an occupied post: state=%v post=%v", contender.State, contender.AttackPost)
	}
	if !contender.AttackTransit {
		t.Fatal("RT contender left on a claimed tile must be marked transit until it finds another post")
	}
	if entity := game.collisionSystem.GetEntityByID(contender.ID); entity == nil || entity.CollisionType != collision.CollisionTypeMonster || entity.Solid {
		t.Fatalf("RT transit collision = %#v, want pass-through monster", entity)
	}
}

func TestCombatAttackTransitIsWalkableSkipsArcAndTakesAoe(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	game.turnBasedMode = false
	const ptx, pty = 14, 14
	placePlayerAtTile(game, ptx, pty, tileSize)
	game.camera.Angle = 0

	holder := hostileMonsterAt(game, ptx+1, pty, tileSize)
	holder.State = monster.StateAttacking
	transit := hostileMonsterAt(game, ptx+1, pty, tileSize)
	transit.State = monster.StatePursuing
	game.world.Monsters = []*monster.Monster3D{holder, transit}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	gl.reconcileMonsterAttackPosts()
	gl.updateCombatTransitVisualStacks()

	if !holder.AttackPost || !transit.AttackTransit {
		t.Fatalf("setup: holder post=%v transit=%v", holder.AttackPost, transit.AttackTransit)
	}
	if holder.TransitStackCount != 2 || transit.TransitStackCount != 2 {
		t.Fatalf("co-located post/transit mobs must be fanned, counts=%d/%d", holder.TransitStackCount, transit.TransitStackCount)
	}
	if !game.collisionSystem.CanMoveTo("player", holder.X, holder.Y) ||
		!game.collisionSystem.Snapshot().CanMoveToWithHabitat(transit.ID, holder.X, holder.Y, transit.HabitatPrefs, transit.Flying) {
		t.Fatal("the party and transit monster must pass through the claimed post")
	}
	if !game.combat.monsterCanAttackParty(holder, Distance(game.camera.X, game.camera.Y, holder.X, holder.Y), holder.GetAttackRangePixels()) {
		t.Fatal("the post holder must retain its normal party attack")
	}
	if game.combat.monsterCanAttackParty(transit, Distance(game.camera.X, game.camera.Y, transit.X, transit.Y), transit.GetAttackRangePixels()) {
		t.Fatal("a transit mob must not attack from another monster's claimed post")
	}

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	game.combat.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if holder.HitPoints >= holder.MaxHitPoints {
		t.Fatal("the claimed front post must be hit by a party arc")
	}
	if transit.HitPoints != transit.MaxHitPoints {
		t.Fatal("a transit mob on the same post must be skipped by a party arc")
	}

	attack := game.combat.newPartyMonsterAttack(40, 0, "fire", 0, nil, "test", false, true, false)
	game.combat.applyAoeSplash(holder, attack, 1)
	if transit.HitPoints >= transit.MaxHitPoints {
		t.Fatal("AoE must still damage a transit mob sharing the post")
	}
}

func TestCombatTransitVisualStackEasesAcrossTileBoundary(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	game.turnBasedMode = false
	placePlayerAtTile(game, 14, 14, tileSize)

	first := hostileMonsterAt(game, 16, 14, tileSize)
	second := hostileMonsterAt(game, 16, 14, tileSize)
	first.State = monster.StatePursuing
	second.State = monster.StatePursuing
	game.world.Monsters = []*monster.Monster3D{first, second}

	gl.updateCombatTransitVisualStacks()
	var follower *monster.Monster3D
	for _, m := range game.world.Monsters {
		if m.TransitStackIndex == 1 {
			follower = m
			break
		}
	}
	if follower == nil || follower.TransitStackCount != 2 {
		t.Fatal("setup: co-located pursuers did not form a visual transit stack")
	}
	fullX, fullY := bandFanOffset(1, 2, tileSize)
	fullDistance := math.Hypot(fullX, fullY)
	firstDistance := math.Hypot(follower.TransitStackOffsetX, follower.TransitStackOffsetY)
	if firstDistance <= 0 || firstDistance >= fullDistance {
		t.Fatalf("first transit fan step = %.2f, want an eased value between 0 and %.2f", firstDistance, fullDistance)
	}

	// Crossing one integer-tile boundary used to clear the complete fan offset
	// in a single tick, producing the repeated snap/un-snap seen while two
	// same-speed guards followed the same route.
	follower.X += tileSize
	gl.updateCombatTransitVisualStacks()
	secondDistance := math.Hypot(follower.TransitStackOffsetX, follower.TransitStackOffsetY)
	if follower.TransitStackCount != 0 {
		t.Fatalf("separated pursuer still has transit stack count %d", follower.TransitStackCount)
	}
	if secondDistance <= 0 || secondDistance >= firstDistance {
		t.Fatalf("released fan offset = %.2f, want a smooth decay from %.2f rather than an instant clear", secondDistance, firstDistance)
	}
}

func TestCombatTransitVisualStackIncludesCalmOccupant(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	game.turnBasedMode = false
	placePlayerAtTile(game, 14, 14, tileSize)

	pursuer := hostileMonsterAt(game, 16, 14, tileSize)
	pursuer.State = monster.StatePursuing
	calm := monster.NewMonster3DFromConfig(pursuer.X, pursuer.Y, "goblin", game.config)
	calm.State = monster.StateIdle
	game.world.Monsters = []*monster.Monster3D{pursuer, calm}

	gl.updateCombatTransitVisualStacks()
	if pursuer.TransitStackCount != 2 || calm.TransitStackCount != 2 {
		t.Fatalf("active/calm overlap must fan both standees, counts=%d/%d",
			pursuer.TransitStackCount, calm.TransitStackCount)
	}

	pursuer.X += tileSize
	gl.updateCombatTransitVisualStacks()
	if pursuer.TransitStackCount != 0 || calm.TransitStackCount != 0 {
		t.Fatalf("separated active/calm standees retained stack counts=%d/%d",
			pursuer.TransitStackCount, calm.TransitStackCount)
	}
}

func TestSummonAttackPostsArePassThroughAndArbitrateTransit(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	game.turnBasedMode = false
	placePlayerAtTile(game, 2, 2, tileSize)

	ally := monster.NewMonster3DFromConfig(15*tileSize+tileSize/2, 15*tileSize+tileSize/2, "masked_huntress", game.config)
	markCardAlly(ally)
	first := hostileMonsterAt(game, 16, 15, tileSize)
	second := hostileMonsterAt(game, 16, 15, tileSize)
	first.State = monster.StateAttacking
	second.State = monster.StateAttacking
	game.world.Monsters = []*monster.Monster3D{first, second, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshMonsterAIState()

	if first.AIFoe != ally || second.AIFoe != ally {
		t.Fatal("setup: both hostile mobs must select the closer card summon")
	}
	gl.reconcileMonsterAttackPosts()
	gl.updateCombatTransitVisualStacks()

	posts := 0
	var holder, transit *monster.Monster3D
	for _, m := range []*monster.Monster3D{first, second} {
		entity := game.collisionSystem.GetEntityByID(m.ID)
		if entity == nil || entity.Solid {
			t.Fatalf("summon-targeting %s must remain physically pass-through", m.ID)
		}
		if m.AttackPost {
			posts++
			holder = m
		} else if m.AttackTransit {
			transit = m
		}
	}
	if posts != 1 || holder == nil || transit == nil {
		t.Fatalf("same summon post must produce one holder and one transit mob: posts=%d holder=%v transit=%v", posts, holder != nil, transit != nil)
	}
	if holder.AttackPostTargetID != ally.ID {
		t.Fatalf("holder target = %q, want summon %q", holder.AttackPostTargetID, ally.ID)
	}
	if transit.State != monster.StatePursuing || game.combat.monsterCanAttackMonster(transit, ally) {
		t.Fatal("summon-targeting transit mob must keep seeking and cannot strike from the occupied post")
	}
	if holder.TransitStackCount != 2 || transit.TransitStackCount != 2 {
		t.Fatalf("summon post stack must fan both mobs, counts=%d/%d", holder.TransitStackCount, transit.TransitStackCount)
	}
	if !game.collisionSystem.CanMoveTo("player", holder.X, holder.Y) {
		t.Fatal("the party must be able to walk through a summon-targeting attack post")
	}
}

func TestPartyTargetEjectionSkipsFullyClaimedAttackRing(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	const ptx, pty = 14, 14
	placePlayerAtTile(game, ptx, pty, tileSize)

	offsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	mobs := make([]*monster.Monster3D, 0, len(offsets)+1)
	for _, off := range offsets {
		m := hostileMonsterAt(game, ptx+off[0], pty+off[1], tileSize)
		m.State = monster.StateAttacking
		mobs = append(mobs, m)
	}
	intruder := hostileMonsterAt(game, ptx, pty, tileSize)
	intruder.State = monster.StatePursuing
	mobs = append(mobs, intruder)
	game.world.Monsters = mobs
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	gl.reconcileMonsterAttackPosts()

	game.ejectPartyTargetingMonsters()
	itx, ity := int(intruder.X/tileSize), int(intruder.Y/tileSize)
	if itx == ptx && ity == pty {
		t.Fatal("a transit mob may not remain on the party tile when the attack ring is full")
	}
	if absInt(itx-ptx) <= 1 && absInt(ity-pty) <= 1 {
		t.Fatalf("intruder moved onto a claimed attack post at (%d,%d)", itx, ity)
	}
}

func TestTurnBasedDuplicateAttackPostBecomesTransit(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	const ptx, pty = 14, 14
	placePlayerAtTile(game, ptx, pty, tileSize)

	first := hostileMonsterAt(game, ptx+1, pty, tileSize)
	second := hostileMonsterAt(game, ptx+1, pty, tileSize)
	game.world.Monsters = []*monster.Monster3D{first, second}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.currentTurn = 1
	game.monsterTurnResolved = false
	gl.updateMonstersTurnBased()

	posts := 0
	for _, m := range game.world.Monsters {
		if m.AttackPost {
			posts++
		}
	}
	if posts != 1 {
		t.Fatalf("TB duplicate attack tile produced %d post holders, want 1", posts)
	}
	if second.AttackPost && first.AttackPost {
		t.Fatal("both TB mobs retained the same attack post")
	}
}

type rearMeleeTransitSetup struct {
	game       *MMGame
	gl         *GameLoop
	tileSize   float64
	targetTile [2]int
	front      *monster.Monster3D
	rear       *monster.Monster3D
}

func setupRearMeleeTransit(t *testing.T, targetSummon bool) rearMeleeTransitSetup {
	t.Helper()
	game, gl, tileSize := tbBehaviorGame(t, 30, 30)
	targetTile := [2]int{14, 14}
	if targetSummon {
		placePlayerAtTile(game, 2, 2, tileSize)
	} else {
		placePlayerAtTile(game, targetTile[0], targetTile[1], tileSize)
	}

	newTreant := func(id string, tx int) *monster.Monster3D {
		m := monster.NewMonster3DFromConfig(
			float64(tx)*tileSize+tileSize/2,
			float64(targetTile[1])*tileSize+tileSize/2,
			"treant",
			game.config,
		)
		m.ID = id
		m.IsEngagingPlayer = true
		m.WasAttacked = true
		return m
	}
	front := newTreant("front_treant", targetTile[0]+1)
	front.State = monster.StateAttacking
	rear := newTreant("rear_treant", targetTile[0]+2)
	rear.State = monster.StatePursuing
	game.world.Monsters = []*monster.Monster3D{front, rear}
	var summon *monster.Monster3D
	if targetSummon {
		summon = monster.NewMonster3DFromConfig(
			float64(targetTile[0])*tileSize+tileSize/2,
			float64(targetTile[1])*tileSize+tileSize/2,
			"masked_huntress",
			game.config,
		)
		markCardAlly(summon)
		summon.MaxHitPoints, summon.HitPoints = 100000, 100000
		game.world.Monsters = append(game.world.Monsters, summon)
	}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshMonsterAIState()
	if targetSummon {
		if front.AIFoe != summon || rear.AIFoe != summon {
			t.Fatal("setup: both treants must target the closer summon")
		}
	} else if front.AIFoe != nil || rear.AIFoe != nil {
		t.Fatal("setup: party-targeting treants unexpectedly selected a monster foe")
	}
	gl.reconcileMonsterAttackPosts()

	return rearMeleeTransitSetup{
		game:       game,
		gl:         gl,
		tileSize:   tileSize,
		targetTile: targetTile,
		front:      front,
		rear:       rear,
	}
}

func TestTurnBasedRearMeleeKeepsTransitProgressAcrossTargets(t *testing.T) {
	for _, tc := range []struct {
		name         string
		targetSummon bool
	}{
		{name: "party"},
		{name: "summon", targetSummon: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := setupRearMeleeTransit(t, tc.targetSummon)
			setup.gl.monsterMoveTurnBased(setup.rear)
			if frontTile, rearTile := tileOf(setup.front, setup.tileSize), tileOf(setup.rear, setup.tileSize); frontTile != rearTile {
				t.Fatalf("rear treant did not enter the occupied transit tile: front=%v rear=%v", frontTile, rearTile)
			}

			// endMonsterTurn calls startPartyTurn before the outer game-loop
			// reconciliation. That boundary must not scatter the rear treant back
			// and erase the movement action it just spent.
			setup.game.startPartyTurn()
			if frontTile, rearTile := tileOf(setup.front, setup.tileSize), tileOf(setup.rear, setup.tileSize); frontTile != rearTile {
				t.Fatalf("party-turn start undid transit progress: front=%v rear=%v", frontTile, rearTile)
			}

			setup.gl.reconcileMonsterAttackPosts()
			if !setup.rear.AttackTransit {
				t.Fatal("rear treant sharing the occupied attack post must be transit")
			}

			setup.gl.monsterMoveTurnBased(setup.rear)
			rearTile := tileOf(setup.rear, setup.tileSize)
			if rearTile == tileOf(setup.front, setup.tileSize) {
				t.Fatal("rear treant did not leave the transit tile for a free attack post")
			}
			if dx, dy := absInt(rearTile[0]-setup.targetTile[0]), absInt(rearTile[1]-setup.targetTile[1]); dx > 1 || dy > 1 {
				t.Fatalf("rear treant moved to %v, want a free tile adjacent to the target", rearTile)
			}
		})
	}
}

func TestRealTimeRearMeleeReachesDistinctPostAcrossTargets(t *testing.T) {
	for _, tc := range []struct {
		name         string
		targetSummon bool
	}{
		{name: "party"},
		{name: "summon", targetSummon: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := setupRearMeleeTransit(t, tc.targetSummon)
			setup.game.turnBasedMode = false
			setup.game.config.MonsterAI.FleeAfterAttacks = 10000

			for frame := 0; frame < 600; frame++ {
				setup.game.frameCount++
				setup.game.refreshMonsterAIState()
				setup.gl.reconcileMonsterAttackPosts()
				snapshot := setup.game.collisionSystem.Snapshot()
				wrappers := []*MonsterWrapper{
					{Monster: setup.front, collisionSystem: setup.game.collisionSystem, snapshot: snapshot, game: setup.game},
					{Monster: setup.rear, collisionSystem: setup.game.collisionSystem, snapshot: snapshot, game: setup.game},
				}
				for _, wrapper := range wrappers {
					wrapper.Update()
				}
				for _, wrapper := range wrappers {
					wrapper.ApplyCollisionUpdate()
				}
				setup.gl.reconcileMonsterAttackPosts()

				rearTile := tileOf(setup.rear, setup.tileSize)
				if setup.rear.AttackPost && rearTile != tileOf(setup.front, setup.tileSize) {
					if dx, dy := absInt(rearTile[0]-setup.targetTile[0]), absInt(rearTile[1]-setup.targetTile[1]); dx <= 1 && dy <= 1 {
						return
					}
				}
			}
			t.Fatalf("rear RT treant never reached a distinct attack post: front=%v rear=%v state=%v post=%v transit=%v",
				tileOf(setup.front, setup.tileSize), tileOf(setup.rear, setup.tileSize),
				setup.rear.State, setup.rear.AttackPost, setup.rear.AttackTransit)
		})
	}
}

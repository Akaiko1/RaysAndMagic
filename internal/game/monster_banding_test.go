package game

import (
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

type bandingTileChecker struct{}

func (bandingTileChecker) IsTileBlocking(tileX, tileY int) bool { return false }

func (bandingTileChecker) IsTileBlockingForHabitat(tileX, tileY int, habitatPrefs []string, flying bool) bool {
	return false
}

func (bandingTileChecker) IsTileOpaque(tileX, tileY int) bool { return false }

func (bandingTileChecker) GetWorldBounds() (int, int) { return 100, 100 }

func TestUpdateMonsterBandsCapsStackSizeAtThree(t *testing.T) {
	game := newBandingTestGame()

	for i := 0; i < 7; i++ {
		addBandingTestMonster(game, "bander-"+string(rune('a'+i)), "rat", 128+float64(i), 128, 0)
	}

	(&GameLoop{game: game}).updateMonsterBands()

	counts := bandStackCounts(game.world.Monsters)
	if counts[3] != 6 {
		t.Fatalf("mobs in capped three-stacks = %d, want 6", counts[3])
	}
	if counts[0] != 1 {
		t.Fatalf("solo mobs = %d, want 1", counts[0])
	}
	for _, m := range game.world.Monsters {
		if m.BandStackCount > maxBandStackCount {
			t.Fatalf("%s stack count = %d, want <= %d", m.ID, m.BandStackCount, maxBandStackCount)
		}
	}
}

func TestUpdateMonsterBandsExistingBandsDoNotMerge(t *testing.T) {
	game := newBandingTestGame()
	addBandingTestMonster(game, "a1", "rat", 128, 128, 1)
	addBandingTestMonster(game, "a2", "rat", 129, 128, 1)
	addBandingTestMonster(game, "b1", "rat", 130, 128, 2)
	addBandingTestMonster(game, "b2", "rat", 131, 128, 2)

	(&GameLoop{game: game}).updateMonsterBands()

	byBand := bandMembershipCounts(game.world.Monsters)
	if byBand[1] != 2 || byBand[2] != 2 {
		t.Fatalf("existing bands merged or changed: got band counts %#v, want band 1=2 and band 2=2", byBand)
	}
	if got := bandStackCounts(game.world.Monsters)[2]; got != 4 {
		t.Fatalf("mobs in two-stacks = %d, want 4", got)
	}
}

func TestUpdateMonsterBandsExistingBandRecruitsOnlySolo(t *testing.T) {
	game := newBandingTestGame()
	addBandingTestMonster(game, "a1", "rat", 128, 128, 1)
	addBandingTestMonster(game, "a2", "rat", 129, 128, 1)
	addBandingTestMonster(game, "b1", "rat", 130, 128, 2)
	addBandingTestMonster(game, "b2", "rat", 131, 128, 2)
	addBandingTestMonster(game, "solo", "rat", 132, 128, 0)

	(&GameLoop{game: game}).updateMonsterBands()

	byBand := bandMembershipCounts(game.world.Monsters)
	if byBand[1] != 3 {
		t.Fatalf("first existing band members = %d, want 3 after recruiting solo", byBand[1])
	}
	if byBand[2] != 2 {
		t.Fatalf("second existing band members = %d, want 2; existing bands must not merge", byBand[2])
	}
	if byBand[0] != 0 {
		t.Fatalf("solo/unbanded members = %d, want 0 after one solo is recruited", byBand[0])
	}
}

func TestUpdateMonsterBandsKeepsStableLeaderWhenLowerIDPresent(t *testing.T) {
	game := newBandingTestGame()
	// Band 1 established last tick with a2 as leader; a1 (lower ID) is a follower.
	addBandingTestMonster(game, "a1", "rat", 130, 128, 1)
	addBandingTestMonster(game, "a2", "rat", 128, 128, 1)
	addBandingTestMonster(game, "a3", "rat", 129, 128, 1)
	for _, m := range game.world.Monsters {
		m.BandLeaderID = "a2" // leader marks itself; followers point at it
	}

	(&GameLoop{game: game}).updateMonsterBands()

	leader := bandLeader(game.world.Monsters, 1)
	if leader == nil {
		t.Fatal("band 1 has no leader after update")
	}
	if leader.ID != "a2" {
		t.Fatalf("band leader = %s, want a2 (lowest-ID follower must not usurp the leader)", leader.ID)
	}
	for _, m := range game.world.Monsters {
		if m.BandLeaderID != "a2" {
			t.Fatalf("%s BandLeaderID = %q, want a2", m.ID, m.BandLeaderID)
		}
	}
}

func bandLeader(monsters []*monsterPkg.Monster3D, bandID int) *monsterPkg.Monster3D {
	for _, m := range monsters {
		if m.BandID == bandID && m.BandStackIndex == 0 {
			return m
		}
	}
	return nil
}

func newBandingTestGame() *MMGame {
	const tile = 64
	cfg := &config.Config{World: config.WorldConfig{TileSize: tile}}
	game := &MMGame{
		config: cfg,
		world:  &world.World3D{},
	}
	game.collisionSystem = collision.NewCollisionSystem(bandingTileChecker{}, tile)
	return game
}

func addBandingTestMonster(game *MMGame, id, key string, x, y float64, bandID int) {
	m := &monsterPkg.Monster3D{
		ID:        id,
		Key:       key,
		X:         x,
		Y:         y,
		HitPoints: 1,
		State:     monsterPkg.StatePatrolling,
		Banding:   true,
		BandID:    bandID,
	}
	game.world.Monsters = append(game.world.Monsters, m)
	game.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, true))
}

func bandStackCounts(monsters []*monsterPkg.Monster3D) map[int]int {
	counts := map[int]int{}
	for _, m := range monsters {
		counts[m.BandStackCount]++
	}
	return counts
}

func bandMembershipCounts(monsters []*monsterPkg.Monster3D) map[int]int {
	counts := map[int]int{}
	for _, m := range monsters {
		counts[m.BandID]++
	}
	return counts
}

// TestScatterBand_SightVsHitPropagation: a band member being HIT marks the
// whole band as attacked (sticky aggro); a band member merely NOTICING the
// player engages the band without the sticky flag, so it can calm down by the
// normal distance hysteresis.
func TestScatterBand_SightVsHitPropagation(t *testing.T) {
	cases := []struct {
		name    string
		trigger func(m *monsterPkg.Monster3D)
		wantHit bool
	}{
		{"sighted", func(m *monsterPkg.Monster3D) { m.IsEngagingPlayer = true }, false},
		{"hit", func(m *monsterPkg.Monster3D) { m.IsEngagingPlayer = true; m.WasAttacked = true }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			game := newBandingTestGame()
			addBandingTestMonster(game, "w_a", "wolf", 128, 128, 5)
			addBandingTestMonster(game, "w_b", "wolf", 128, 128, 5)
			addBandingTestMonster(game, "w_c", "wolf", 128, 128, 5)
			tc.trigger(game.world.Monsters[0])

			(&GameLoop{game: game}).updateMonsterBands()

			for _, m := range game.world.Monsters[1:] {
				if !m.IsEngagingPlayer {
					t.Errorf("%s: %s should engage when a bandmate triggers", tc.name, m.ID)
				}
				if m.WasAttacked != tc.wantHit {
					t.Errorf("%s: %s WasAttacked = %v, want %v", tc.name, m.ID, m.WasAttacked, tc.wantHit)
				}
				if m.BandID != 0 {
					t.Errorf("%s: %s should leave the band on scatter", tc.name, m.ID)
				}
			}
		})
	}
}

// TestScatterBandOnMemberDeath_OneShotKillAggrosSurvivors covers the gap the
// hit-propagation path can't reach: a one-shot kill drops the victim out of the
// band collection before the next banding tick, so without the explicit kill
// hook the survivors stay calm and stacked while being sniped one by one.
func TestScatterBandOnMemberDeath_OneShotKillAggrosSurvivors(t *testing.T) {
	game := newBandingTestGame()
	game.combat = NewCombatSystem(game)
	game.gameLoop = &GameLoop{game: game}

	tile := float64(game.config.GetTileSize())
	cx, cy := TileCenterFromTile(3, 3, tile)
	addBandingTestMonster(game, "wolf_a", "wolf", cx, cy, 7)
	addBandingTestMonster(game, "wolf_b", "wolf", cx, cy, 7)
	addBandingTestMonster(game, "wolf_c", "wolf", cx, cy, 7)
	victim, s1, s2 := game.world.Monsters[0], game.world.Monsters[1], game.world.Monsters[2]

	victim.HitPoints = 0 // one-shot kill
	game.combat.finishMonsterKill(victim)

	for _, m := range []*monsterPkg.Monster3D{s1, s2} {
		if !m.IsEngagingPlayer || !m.WasAttacked {
			t.Errorf("%s should aggro when a bandmate is slain (engaging=%v wasAttacked=%v)",
				m.ID, m.IsEngagingPlayer, m.WasAttacked)
		}
		if m.BandID != 0 {
			t.Errorf("%s should leave the band on scatter, BandID=%d", m.ID, m.BandID)
		}
	}
	t1 := [2]int{int(s1.X / tile), int(s1.Y / tile)}
	t2 := [2]int{int(s2.X / tile), int(s2.Y / tile)}
	if t1 == t2 {
		t.Errorf("scattered survivors should land on distinct tiles, both on %v", t1)
	}
}

// TestScatterBandOnMemberDeath_FightingSurvivorsStayPut: survivors already in
// combat must not be teleported by the death burst — scatter repositions only
// still-calm members.
func TestScatterBandOnMemberDeath_FightingSurvivorsStayPut(t *testing.T) {
	game := newBandingTestGame()
	game.combat = NewCombatSystem(game)
	game.gameLoop = &GameLoop{game: game}

	tile := float64(game.config.GetTileSize())
	cx, cy := TileCenterFromTile(3, 3, tile)
	addBandingTestMonster(game, "wolf_a", "wolf", cx, cy, 9)
	addBandingTestMonster(game, "wolf_b", "wolf", cx+tile, cy, 9)
	victim, fighter := game.world.Monsters[0], game.world.Monsters[1]
	fighter.IsEngagingPlayer = true

	victim.HitPoints = 0
	fx, fy := fighter.X, fighter.Y
	game.combat.finishMonsterKill(victim)

	if fighter.X != fx || fighter.Y != fy {
		t.Errorf("already-fighting survivor moved by death burst: (%.0f,%.0f)->(%.0f,%.0f)", fx, fy, fighter.X, fighter.Y)
	}
}

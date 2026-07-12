package game

// Wiring tests for monster specials in BOTH combat modes: they drive the real
// RT loop (Monster3D.Update + HandleMonsterInteractions) / the real TB monster
// pass (updateMonstersTurnBased), not updateBoss directly - guarding that the
// attack-moment plumbing actually reaches each special, not just that the
// special works once invoked (the culverts/castle tests already cover that).
// Chances are pinned to 1.0 so a single attack moment MUST produce a cast;
// runs stay under 5 attack cycles so the generic flee-after-attacks roll can
// never trigger. Every boss ability and both dragon-breath modes are cases in
// one table - add a new boss ability as one more row.

import (
	"strings"
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

func newSpecialsTestGame(t *testing.T) (*MMGame, *GameLoop) {
	t.Helper()
	cfg := loadTestConfig(t)

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM })
	world.GlobalWorldManager = nil // GetCurrentWorld falls back to g.world
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 12, 12
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	g := newTestGame(cfg, w)
	g.combat = NewCombatSystem(g)
	// Party off-center in tile (2,2), like live play; the offset is what keeps
	// tile-adjacent melee standoffs at >1 tile of pixel distance.
	g.camera.X, g.camera.Y = 2*64+25, 2*64+35
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
	return g, &GameLoop{game: g}
}

func spawnSpecialsMonster(g *MMGame, key string, tileX, tileY int) *monsterPkg.Monster3D {
	m := monsterPkg.NewMonster3DFromConfig(float64(tileX)*64+32, float64(tileY)*64+32, key, g.config)
	m.IsEngagingPlayer = true
	m.WasAttacked = true
	g.world.Monsters = append(g.world.Monsters, m)
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	return m
}

func runRTCombatSeconds(g *MMGame, seconds int) {
	for tick := 0; tick < seconds*g.config.GetTPS(); tick++ {
		g.frameCount++
		for _, m := range g.world.Monsters {
			if !m.IsAlive() {
				continue
			}
			m.Update(g.collisionSystem, g.camera.X, g.camera.Y)
			g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			g.refreshMonsterCollisionSolidity(m)
		}
		g.combat.HandleMonsterInteractions()
	}
}

func runTBMonsterTurns(g *MMGame, gl *GameLoop, turns int) {
	g.turnBasedMode = true
	for i := 0; i < turns; i++ {
		g.currentTurn = 1
		g.monsterTurnResolved = false
		g.turnBasedMonsterPassDelay = 0
		g.turnBasedMonsterPassesLeft = 0
		gl.updateMonstersTurnBased()
	}
}

func countCombatLog(g *MMGame, substr string) int {
	n := 0
	for _, e := range g.combatLogHistory {
		if strings.Contains(e.Text, substr) {
			n++
		}
	}
	return n
}

// aggro flips a config-spawned boss into its post-quest aggressive state.
func aggro(m *monsterPkg.Monster3D) {
	m.PassiveUntilQuest = ""
	m.BossAggro = true
}

// noSummons strips the summon kit so it can't preempt the ability under test
// (a handled summon returns true and skips the normal attack).
func noSummons(m *monsterPkg.Monster3D) {
	m.SummonChance = 0
	m.SummonFirstGuaranteed = false
}

// TestMonsterSpecialAbilityWiring: every boss ability plus dragon breath, each
// driven through the full RT and TB combat loops from a realistic position
// (melee bosses on an adjacent tile; ranged dragons 3 tiles out on the row).
func TestMonsterSpecialAbilityWiring(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		tileX int // spawn tile (party at 2,2)
		prep  func(m *monsterPkg.Monster3D)
		want  string // combat-log substring proving the ability fired
	}{
		{
			name: "gtb-inferno", key: "golden_thief_bug", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) {
				aggro(m)
				m.InfernoChance = 1.0
				m.TeleportChance = 0
			},
			want: "Inferno scorches",
		},
		{
			name: "gtb-lowhp-blink", key: "golden_thief_bug", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) {
				aggro(m)
				m.InfernoChance = 0
				m.TeleportChance = 1.0
				m.HitPoints = m.TeleportAtHP // at the low-HP threshold
			},
			want: "blinks away in a golden flash",
		},
		{
			// Quest NOT done (no quest manager in tests): the evasive boss must
			// skitter away from an adjacent party instead of fighting.
			name: "gtb-evasive-blink", key: "golden_thief_bug", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) {},
			want: "skitters away into the dark",
		},
		{
			name: "samurai-summon", key: "old_samurai", tileX: 3,
			prep: aggro, // config: summon_first_guaranteed
			want: "raises the war-banner",
		},
		{
			name: "samurai-enrage", key: "old_samurai", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) {
				aggro(m)
				noSummons(m)
				m.HitPoints = m.EnrageAtHP // at the enrage threshold
			},
			want: "flies into a furious rage",
		},
		{
			// WardedByIdols stays set; the transient BossWarded is refreshed per
			// frame from live idols and there are none here - the idols-broken state.
			name: "orc-warlord-summon", key: "orc_hero_boss", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) { m.BossAggro = true },
			want: "raises the war-banner",
		},
		{
			name: "orc-warlord-enrage", key: "orc_hero_boss", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) {
				m.BossAggro = true
				noSummons(m)
				m.HitPoints = m.EnrageAtHP
			},
			want: "flies into a furious rage",
		},
		{
			name: "gorilla-titan-summon", key: "gorilla_titan", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) { m.BossAggro = true },
			want: "raises the war-banner",
		},
		{
			// Fireburst lives in the shared attack wrapper (tryMonsterAoeAttack),
			// not in updateBoss - the summon kit must be off or it eats the attack.
			name: "gorilla-titan-fireburst", key: "gorilla_titan", tileX: 3,
			prep: func(m *monsterPkg.Monster3D) {
				m.BossAggro = true
				noSummons(m)
				m.FireburstChance = 1.0
			},
			want: "casts Fireburst",
		},
		{
			name: "dragon-breath", key: "dragon_gold", tileX: 5, // in range 4, on the row
			prep: func(m *monsterPkg.Monster3D) { m.DragonBreathChance = 1.0 },
			want: "breathes",
		},
	}

	for _, tc := range cases {
		for _, mode := range []string{"rt", "tb"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				g, gl := newSpecialsTestGame(t)
				m := spawnSpecialsMonster(g, tc.key, tc.tileX, 2)
				tc.prep(m)

				if mode == "rt" {
					runRTCombatSeconds(g, 4)
				} else {
					runTBMonsterTurns(g, gl, 3)
				}

				if countCombatLog(g, tc.want) == 0 {
					acted := countCombatLog(g, m.Name)
					t.Errorf("%s/%s: no %q in combat log (chance pinned to 1.0; %d log entries mention %s)",
						tc.name, mode, tc.want, acted, m.Name)
				}
			})
		}
	}
}

package game

import (
	"fmt"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Movement detection -> renderer coverage: RT/editor and TB, patrol/pursuit/
// flee/idle, both sprite paths, slow and configured cadence, stops and restarts.
// Save/load starts with no presentation events; stale action stamps cannot walk.
func TestMonsterWalkingPlaybackCadence(t *testing.T) {
	t.Chdir("../..")
	previous := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previous })
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	sprites := graphics.NewSpriteManager()
	t.Cleanup(func() {
		sprites.EvictResource("goblin", "walking_r")
		sprites.EvictResource("goblin", "attacking_r")
	})
	for _, tps := range []int{60, 120, 240} {
		for _, seconds := range []float64{0, 0.375, 0.125} {
			t.Run(fmt.Sprintf("tps_%d/seconds_%g", tps, seconds), func(t *testing.T) {
				g := &MMGame{config: &config.Config{Engine: config.EngineConfig{TPS: tps}, World: config.WorldConfig{TileSize: 64}}, sprites: sprites, camera: &FirstPersonCamera{}, world: &world.World3D{}}
				g.gameLoop = &GameLoop{game: g}
				g.config.Graphics.Monster.WalkFrameSeconds = seconds
				r := &Renderer{game: g}
				walk := sprites.GetAnimation("goblin", "walking_r")
				if walk == nil || len(walk.Frames) != 4 {
					t.Fatal("missing walking fixture")
				}
				period := (3*tps + 4) / 8
				if seconds == 0.125 {
					period = (tps + 4) / 8
				}
				for _, tb := range []bool{false, true} {
					g.turnBasedMode = tb
					for _, state := range []monster.MonsterState{monster.StatePatrolling, monster.StatePursuing, monster.StateFleeing, monster.StateIdle} {
						m := &monster.Monster3D{Key: "goblin", State: state, HitPoints: 1}
						g.world.Monsters = []*monster.Monster3D{m}
						g.gameLoop.monsterWalkPlayback = nil
						// A nonzero, non-period-aligned start catches global-clock playback.
						for tick := 0; tick <= 5*period; tick++ {
							g.frameCount = int64(37 + tick)
							stepFacing(g.gameLoop, func() {
								if tb {
									if tick == 0 {
										m.X += 64
									}
								} else {
									m.X += 0.2
								}
							})
							index := (tick / period) % 4
							if tb && tick >= 4*period {
								index = 0
							}
							x, y := m.X, m.Y
							billboard, _ := r.getMonsterSprite(m)
							standee, _ := r.getMonsterStandeeSprite(m)
							if billboard != walk.Frames[index] || standee != walk.Frames[index] {
								t.Fatalf("TB=%v state=%v tick=%d: wrong walk frame, want %d", tb, state, tick, index)
							}
							if m.X != x || m.Y != y {
								t.Fatal("drawing moved the monster")
							}
						}
						// Blocked or waiting despite the same AI intent: no walk.
						g.frameCount++
						stepFacing(g.gameLoop, func() {})
						if got, _ := r.getMonsterSprite(m); got != walk.Frames[0] {
							t.Fatal("stationary monster animated")
						}
						// A new move starts at frame zero, never in the middle of the cycle.
						g.frameCount++
						stepFacing(g.gameLoop, func() { m.X += 64 })
						if got, _ := r.getMonsterStandeeSprite(m); got != walk.Frames[0] {
							t.Fatal("new movement did not restart at first frame")
						}
					}
				}
				// Reconstructed actors and combat's legacy move stamp cannot invent a step.
				m := &monster.Monster3D{Key: "goblin", State: monster.StatePursuing, LastMoveTick: g.frameCount}
				g.turnBasedMode = true
				if got, _ := r.getMonsterSprite(m); got != walk.Frames[0] {
					t.Fatal("fresh/restored stationary actor animated")
				}
				m.AttackAnimFrames = 1
				if got := r.monsterAnimFrameImage(walk, m); got != walk.Frames[0] {
					t.Fatal("missing attack art triggered walking in place")
				}
				attack := sprites.GetAnimation("goblin", "attacking_r")
				m.AttackAnimFrames = g.monsterAttackAnimationDuration(m) / 2
				if got, _ := r.getMonsterStandeeSprite(m); attack == nil || got != attack.Frames[2] {
					t.Fatal("authored attack timing changed")
				}
			})
		}
	}
}

func TestAnimationTimingAtDefaultTPS(t *testing.T) {
	const tps = 120

	if got, want := animationTicksPerFrame(tps, NPCIdleAnimationFPS), 30; got != want {
		t.Fatalf("NPC idle ticks per frame = %d, want %d", got, want)
	}
	if got, want := animationDurationFrames(tps, AuthoredMonsterAttackFPS, 4), 48; got != want {
		t.Fatalf("four-frame authored attack duration = %d, want %d", got, want)
	}
}

func TestMonsterAttackTimingOnlySlowsAuthoredSheets(t *testing.T) {
	t.Chdir("../..")
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	g := &MMGame{
		config:  &config.Config{Engine: config.EngineConfig{TPS: 120}},
		sprites: graphics.NewSpriteManager(),
	}

	authored := &monster.Monster3D{Key: "weapon_master"}
	if got, want := g.authoredMonsterAttackFrameCount(authored), 4; got != want {
		t.Fatalf("weapon master attack frame count = %d, want %d", got, want)
	}
	g.armMonsterAttackAnimation(authored)
	if got, want := authored.AttackAnimFrames, 48; got != want {
		t.Fatalf("authored attack duration = %d, want %d", got, want)
	}

	fallback := &monster.Monster3D{Key: "dire_wolf"}
	def, err := monster.MonsterConfig.GetMonsterByKey(fallback.Key)
	if err != nil {
		t.Fatal(err)
	}
	withoutArt := *def
	withoutArt.Sprite = "test_missing_attack_animation"
	fallback.SetupMonsterFromConfig(&withoutArt)
	if got := g.authoredMonsterAttackFrameCount(fallback); got != 0 {
		t.Fatalf("missing-art fixture unexpectedly has %d authored attack frames", got)
	}
	g.armMonsterAttackAnimation(fallback)
	if got, want := fallback.AttackAnimFrames, MonsterAttackAnimFrames; got != want {
		t.Fatalf("walking-sheet fallback duration = %d, want unchanged %d", got, want)
	}
}

package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Exercise the real YAML-resolved sheet through both monster render paths.
func TestPilgrimageGatekeeperWalkingPlayback(t *testing.T) {
	t.Chdir("../..")
	previous := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previous })
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	sprites := graphics.NewSpriteManager()
	g := &MMGame{config: &config.Config{Engine: config.EngineConfig{TPS: 120}, World: config.WorldConfig{TileSize: 64}}, sprites: sprites, camera: &FirstPersonCamera{}, world: &world.World3D{}}
	g.gameLoop = &GameLoop{game: g}
	r := &Renderer{game: g}
	anim := sprites.GetAnimation("bronze_gatekeeper", "walking_r")
	if anim == nil || len(anim.Frames) != 4 {
		t.Fatal("missing runtime walking animation")
	}
	for _, tb := range []bool{false, true} {
		g.turnBasedMode = tb
		m := &monster.Monster3D{Key: "bronze_gatekeeper", Speed: 1, State: monster.StatePursuing, HitPoints: 1}
		g.world.Monsters = []*monster.Monster3D{m}
		g.gameLoop.monsterWalkPlayback = nil
		period := r.monsterWalkTicksPerFrame()
		for tick := 0; tick < period*4; tick++ {
			g.frameCount = int64(37 + tick)
			stepFacing(g.gameLoop, func() {
				if tb {
					if tick == 0 {
						m.X += 64
					}
				} else {
					m.X += 1
				}
			})
			a, _ := r.getMonsterSprite(m)
			b, _ := r.getMonsterStandeeSprite(m)
			if a != anim.Frames[tick/period] || b != a {
				t.Fatalf("TB=%v tick=%d did not select walking pose", tb, tick)
			}
		}
	}
}

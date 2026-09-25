package game

import (
	"image"
	"image/png"
	"os"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// The near shin is brighter than the far shin. Its highlight must cross from
// right to left and back, not stay fixed while only arms/sash pixels change.
// This content check complements visual hip-to-foot tracing and loop QA.
func TestPilgrimageGatekeeperAlternatesLegs(t *testing.T) {
	t.Chdir("../..")
	f, err := os.Open("assets/sprites/mobs/bronze_gatekeeper_walking_r.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 1024, 1024) {
		t.Fatal("wrong walking sheet geometry")
	}
	var centers [4]float64
	for i := range centers {
		total, count := 0, 0
		for y := 350; y < 440; y++ {
			for x := 0; x < 512; x++ {
				r, g, b, a := img.At(i%2*512+x, i/2*512+y).RGBA()
				if a > 200*257 && (r+g+b)/3 > 125*257 {
					total += x
					count++
				}
			}
		}
		if count == 0 {
			t.Fatal("missing near shin highlights")
		}
		centers[i] = float64(total) / float64(count)
	}
	if centers[0]-centers[1] < 20 || centers[2]-centers[1] < 20 || centers[2]-centers[3] < 20 || centers[0]-centers[3] < 20 {
		t.Fatalf("near/far/near/far gait lost: near-shin centers %v", centers)
	}
}

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

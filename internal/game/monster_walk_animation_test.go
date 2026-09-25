package game

import (
	"testing"

	"ugataima/internal/monster"
)

// Cases supplement the renderer cadence matrix with shared movement ownership:
// band formation, leader motion/stops, TB recenter, mode changes and map removal.
// Playback is transient: a reconstructed game loop never restores it from saves.
func TestMonsterWalkMovementLifecycle(t *testing.T) {
	for _, tb := range []bool{false, true} {
		name := "RT"
		if tb {
			name = "TB"
		}
		t.Run(name, func(t *testing.T) {
			g := newBandingTestGame()
			g.turnBasedMode = tb
			g.gameLoop = &GameLoop{game: g}
			addBandingTestMonster(g, "a", "rat", 160, 160, 0)
			addBandingTestMonster(g, "b", "rat", 161, 160, 0)
			gl := g.gameLoop
			r := &Renderer{game: g}
			gl.updateMonsterBands()
			leader := bandLeader(g.world.Monsters, g.world.Monsters[0].BandID)
			if leader == nil {
				t.Fatal("band did not form")
			}
			for _, m := range g.world.Monsters {
				if r.shouldAnimateMonster(m) {
					t.Fatal("formation snap started walking")
				}
			}
			g.frameCount++
			stepFacing(gl, func() { leader.X += 64 })
			gl.updateMonsterBands()
			for _, m := range g.world.Monsters {
				if elapsed, ok := r.monsterWalkElapsed(m); !ok || elapsed != 0 {
					t.Fatal("follower did not inherit leader step")
				}
			}
			g.frameCount++
			stepFacing(gl, func() {})
			gl.updateMonsterBands()
			for _, m := range g.world.Monsters {
				elapsed, ok := r.monsterWalkElapsed(m)
				if ok != tb || (ok && elapsed != 1) {
					t.Fatal("stop must end RT walk but continue TB cycle")
				}
			}
			// Switching modes clears pending playback even without moving.
			g.turnBasedMode = !tb
			g.frameCount++
			stepFacing(gl, func() {})
			for _, m := range g.world.Monsters {
				if r.shouldAnimateMonster(m) {
					t.Fatal("mode switch reused a previous walk")
				}
			}
			g.turnBasedMode = true
			g.frameCount++
			stepFacing(gl, func() { leader.X += 1 })
			if r.shouldAnimateMonster(leader) {
				t.Fatal("TB same-tile recenter started a walk")
			}
			g.frameCount++
			stepFacing(gl, func() { leader.X += 64 })
			if !r.shouldAnimateMonster(leader) {
				t.Fatal("new TB tile step did not start walking")
			}
			// Removed actors cannot keep rendering or retain references in the loop.
			g.world.Monsters = nil
			for i := 0; i < 2; i++ {
				g.frameCount++
				stepFacing(gl, func() {})
			}
			if len(gl.monsterWalkPlayback) != 0 {
				t.Fatal("old map actors retained playback")
			}
			g.world.Monsters = []*monster.Monster3D{leader}
			g.gameLoop = &GameLoop{game: g}
			if r.shouldAnimateMonster(leader) {
				t.Fatal("fresh game loop restored a transient walk")
			}
		})
	}
}

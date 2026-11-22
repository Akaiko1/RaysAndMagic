package game

import (
	"math"
	"testing"
	"time"

	"ugataima/internal/monster"
)

// A turn-based step moves the party and the monsters at once in the rules and
// slides them on screen; teleports, blinks, real time and a zero duration jump.
func TestTurnStepGlide(t *testing.T) {
	// shown is the camera position the scene pass renders from.
	shown := func(g *MMGame) (float64, float64) {
		restore := g.beginRenderCameraSwap(time.Now())
		x, y := g.camera.X, g.camera.Y
		restore()
		return x, y
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

	t.Run("party", func(t *testing.T) {
		g, _, ts := tbBehaviorGame(t, 24, 24)
		ih := NewInputHandler(g)
		placePlayerAtTile(g, 5, 5, ts)
		frames := g.stepGlideFrames()
		if frames < 4 {
			t.Fatalf("fixture: the slide lasts %d frames, too few to sample", frames)
		}
		startX, startY := g.camera.X, g.camera.Y
		if !ih.moveTurnBasedInDirection(1, 0) {
			t.Fatal("fixture: the step was refused")
		}
		if g.camera.X != startX+ts || g.camera.Y != startY {
			t.Fatalf("the rules did not move at once: (%.1f,%.1f)", g.camera.X, g.camera.Y)
		}
		g.frameCount += frames / 2
		x, y := shown(g)
		if want := startX + ts*float64(frames/2)/float64(frames); !near(x, want) || y != startY {
			t.Fatalf("halfway: view at (%.2f,%.2f), want (%.2f,%.2f)", x, y, want, startY)
		}
		if g.camera.X != startX+ts {
			t.Fatal("drawing moved the party")
		}
		// A second step mid-slide continues from the slid view, not from the tile.
		ih.moveTurnBasedInDirection(1, 0)
		if x2, _ := shown(g); !near(x2, x) {
			t.Fatalf("a chained step jumped the view from %.2f to %.2f", x, x2)
		}
		g.frameCount += frames
		if x, y := shown(g); x != startX+2*ts || y != startY {
			t.Fatalf("after the slide: view at (%.2f,%.2f), want the party's tile", x, y)
		}

		// A teleport never slides.
		ih.moveTurnBasedInDirection(1, 0)
		g.setPartyPosition(startX, startY)
		if x, y := shown(g); x != startX || y != startY {
			t.Fatalf("teleport: view at (%.2f,%.2f), want (%.2f,%.2f)", x, y, startX, startY)
		}

		// Real time keeps its own interpolation: no slide.
		g.turnBasedMode = false
		g.movePartyPosition(startX+ts, startY)
		if _, _, moving := g.viewStepPos(); moving {
			t.Fatal("a real-time step slid")
		}

		// Zero seconds restores the jump.
		g.turnBasedMode = true
		g.config.Graphics.View.TurnStepSeconds = 0
		g.movePartyPosition(startX+2*ts, startY)
		if x, _ := shown(g); x != startX+2*ts {
			t.Fatalf("zero duration: view at %.2f, want the tile", x)
		}
	})

	t.Run("monsters", func(t *testing.T) {
		g, _, ts := tbBehaviorGame(t, 24, 24)
		placePlayerAtTile(g, 3, 3, ts)
		m := spawnMonsterAtTile(g, "goblin", 12, 12, ts)
		frames := g.stepGlideFrames()
		tick := func() {
			g.frameCount++
			g.advanceMonsterGlides()
		}
		tick()
		startX, startY := m.X, m.Y
		if x, y := g.combat.monsterVisualPos(m); x != startX || y != startY {
			t.Fatalf("first sighting slid: drawn at (%.1f,%.1f)", x, y)
		}

		m.X += ts
		tick()
		for range frames/2 - 1 {
			tick()
		}
		x, y := g.combat.monsterVisualPos(m)
		if want := startX + ts*float64(frames/2)/float64(frames); !near(x, want) || y != startY {
			t.Fatalf("halfway: drawn at (%.2f,%.2f), want (%.2f,%.2f)", x, y, want, startY)
		}
		if m.X != startX+ts {
			t.Fatal("drawing moved the monster")
		}
		for range frames {
			tick()
		}
		if x, _ := g.combat.monsterVisualPos(m); x != m.X {
			t.Fatalf("after the slide: drawn at %.2f, want %.2f", x, m.X)
		}

		// A blink jumps.
		oldX, oldY := m.X, m.Y
		m.X += 3 * ts
		g.combat.settleBlink(m, oldX, oldY, g.camera.X, g.camera.Y)
		tick()
		if x, _ := g.combat.monsterVisualPos(m); x != m.X {
			t.Fatalf("blink slid: drawn at %.2f, want %.2f", x, m.X)
		}

		// Real time draws the monster where it is; entering turn-based mode
		// slides it onto its tile centre.
		g.turnBasedMode = false
		m.X += ts / 4
		tick()
		if x, _ := g.combat.monsterVisualPos(m); x != m.X {
			t.Fatalf("real time slid: drawn at %.2f, want %.2f", x, m.X)
		}
		offX := m.X
		g.ToggleTurnBasedMode()
		tick()
		tick()
		if x, _ := g.combat.monsterVisualPos(m); m.X == offX || !(x > m.X && x < offX) {
			t.Fatalf("entry snap: monster at %.2f from %.2f, drawn at %.2f; want a slide between them", m.X, offX, x)
		}
	})
}

// A several-tile walk slides along the tiles it crossed, never through the
// corner it walked around; the random blink of an evasive boss jumps; a body
// lies where the monster rests; the rules read the party's tile mid-slide.
func TestTurnStepGlideFollowsTheRules(t *testing.T) {
	g, gl, ts := tbBehaviorGame(t, 24, 24)
	placePlayerAtTile(g, 3, 3, ts)
	m := spawnMonsterAtTile(g, "goblin", 12, 12, ts)
	frames := g.stepGlideFrames()
	tick := func() {
		g.frameCount++
		g.advanceMonsterGlides()
	}
	tick()
	startX, startY := m.X, m.Y

	// Two tiles east, then two north, in one action.
	for _, step := range [][2]float64{{1, 0}, {2, 0}, {2, -1}, {2, -2}} {
		if !gl.commitMonsterMoveTB(m, startX+step[0]*ts, startY+step[1]*ts) {
			t.Fatalf("fixture: step to %+v refused", step)
		}
	}
	tick()
	corner := [2]float64{startX + 2*ts, startY}
	reached := false
	for range frames {
		x, y := g.combat.monsterVisualPos(m)
		onPath := (y == startY && x >= startX && x <= corner[0]) || (math.Abs(x-corner[0]) < 1e-9 && y <= startY && y >= startY-2*ts)
		if !onPath {
			t.Fatalf("slide left the walked tiles at (%.2f,%.2f)", (x-startX)/ts, (y-startY)/ts)
		}
		reached = reached || (math.Abs(x-corner[0]) < 1e-9 && y < startY)
		tick()
	}
	if x, y := g.combat.monsterVisualPos(m); !reached || x != m.X || y != m.Y {
		t.Fatalf("slide ended at (%.2f,%.2f), second leg seen %v; want the monster's tile", x, y, reached)
	}

	// The evasive boss's random blink is a jump.
	if !g.combat.blinkMonsterRandom(m) {
		t.Fatal("fixture: no blink spot")
	}
	tick()
	if x, y := g.combat.monsterVisualPos(m); x != m.X || y != m.Y {
		t.Fatalf("random blink slid: drawn at (%.1f,%.1f), monster at (%.1f,%.1f)", x, y, m.X, m.Y)
	}

	// Killed mid-slide, the body lies on its tile.
	gl.commitMonsterMoveTB(m, m.X+ts, m.Y)
	tick()
	if x, _ := g.combat.monsterVisualPos(m); x == m.X {
		t.Fatal("fixture: the monster is not sliding")
	}
	m.HitPoints = 0
	g.combat.finishMonsterKill(m)
	if m.DeathDrawX != m.X || m.DeathDrawY != m.Y {
		t.Fatalf("body at (%.1f,%.1f), want the tile (%.1f,%.1f)", m.DeathDrawX, m.DeathDrawY, m.X, m.Y)
	}

	// While the view slides, the rules still stand on the party's tile.
	g.movePartyPosition(g.camera.X+ts, g.camera.Y)
	g.frameCount += frames / 2
	wantX, wantY := g.camera.X, g.camera.Y
	restore := g.beginRenderCameraSwap(time.Now())
	slidX := g.camera.X
	x, y := g.combat.logicalCameraXY()
	restore()
	if slidX == wantX || x != wantX || y != wantY {
		t.Fatalf("mid-slide: view at %.1f, rules at (%.1f,%.1f); want the view between tiles and the rules on (%.1f,%.1f)", slidX, x, y, wantX, wantY)
	}
}

// A step taken after this update's clock tick (the automatic entry's snap)
// starts on the same frame as the monster slides of that update.
func TestTurnStepGlideSharesOneStartFrame(t *testing.T) {
	g, _, ts := tbBehaviorGame(t, 24, 24)
	placePlayerAtTile(g, 5, 5, ts)
	m := spawnMonsterAtTile(g, "goblin", 12, 12, ts)
	g.advanceMonsterGlides()
	for _, ticked := range []bool{false, true} {
		g.viewStep = stepGlide{}
		if !ticked {
			g.movePartyPosition(g.camera.X+ts, g.camera.Y)
		}
		g.frameCount++
		g.worldTicked = true
		if ticked {
			g.movePartyPosition(g.camera.X+ts, g.camera.Y)
		}
		m.X += ts
		g.advanceMonsterGlides()
		g.worldTicked = false
		if got, want := g.viewStep.start, g.monsterGlides[m].start; got != want {
			t.Fatalf("after tick %v: view starts on frame %d, monsters on %d", ticked, got, want)
		}
		g.frameCount += g.stepGlideFrames()
		g.advanceMonsterGlides()
	}
}

// The catalog keeps no stale slide: a monster that left the world is dropped.
func TestMonsterGlidesFollowTheWorld(t *testing.T) {
	g, _, ts := tbBehaviorGame(t, 24, 24)
	placePlayerAtTile(g, 3, 3, ts)
	spawnMonsterAtTile(g, "goblin", 12, 12, ts)
	g.advanceMonsterGlides()
	g.world.Monsters = []*monster.Monster3D{}
	g.frameCount++
	g.advanceMonsterGlides()
	if len(g.monsterGlides) != 0 {
		t.Fatalf("%d slides kept for monsters that left the world", len(g.monsterGlides))
	}
}

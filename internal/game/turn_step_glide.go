package game

import (
	"math"

	"ugataima/internal/monster"
)

// Turn-based steps are instant in the rules and eased on screen: the party's
// view and every monster slide to where they now stand over
// graphics.view.turn_step_seconds. Teleports, blinks, loads and real time never
// slide. Rules read the rest position (monsterRestPos, the logical camera);
// only the picture and the effects anchored on it read a slid one.
type stepGlide struct {
	fromX, fromY float64
	toX, toY     float64
	start        int64 // world frame the slide left from
	on           bool
}

// progress is the slide's share done on world frame now; moving is false
// before it starts and once it ended.
func (s stepGlide) progress(now, frames int64) (t float64, moving bool) {
	elapsed := now - s.start
	if !s.on || elapsed < 0 || elapsed >= frames {
		return 1, false
	}
	return float64(elapsed) / float64(frames), true
}

// at is where the slide stands on world frame now, and whether it still moves.
func (s stepGlide) at(now, frames int64) (x, y float64, moving bool) {
	t, moving := s.progress(now, frames)
	if !moving {
		return s.toX, s.toY, false
	}
	return s.fromX + (s.toX-s.fromX)*t, s.fromY + (s.toY-s.fromY)*t, true
}

func (g *MMGame) stepGlideFrames() int64 {
	if g.config == nil {
		return 0
	}
	return int64(g.framesForSeconds(g.config.Graphics.View.TurnStepSeconds))
}

// glideStartFrame is the world frame a slide begun now leaves from: one before
// the frame that first shows it, so that frame already moves and the view and
// the monsters stay in step whether the move came before or after this
// update's clock tick.
func (g *MMGame) glideStartFrame() int64 {
	if g.worldTicked {
		return g.frameCount - 1
	}
	return g.frameCount
}

// glidePartyStep starts the view's slide for an ordinary turn-based step to
// (x, y); call it before the camera moves.
func (g *MMGame) glidePartyStep(x, y float64) {
	if !g.turnBasedMode || g.camera == nil {
		return
	}
	// A step taken before the last one finished continues from the slid view.
	frames := g.stepGlideFrames()
	fromX, fromY, moving := g.viewStep.at(g.frameCount, frames)
	if !moving {
		fromX, fromY = g.camera.X, g.camera.Y
	}
	g.viewStep = stepGlide{fromX: fromX, fromY: fromY, toX: x, toY: y, start: g.glideStartFrame(), on: frames > 0}
}

// viewStepPos is the sliding view position while a turn-based step plays.
func (g *MMGame) viewStepPos() (x, y float64, moving bool) {
	if !g.turnBasedMode {
		return 0, 0, false
	}
	return g.viewStep.at(g.frameCount, g.stepGlideFrames())
}

type glidePoint struct{ x, y float64 }

// monsterGlide is one monster's slide. via holds the tiles a several-step walk
// crossed, so the slide follows the walk around a corner; steps collects them
// until the frame's rest position is known.
type monsterGlide struct {
	stepGlide
	via, steps []glidePoint
	seen       int64
}

// at follows from -> via... -> to at one even speed.
func (s monsterGlide) at(now, frames int64) (x, y float64, moving bool) {
	if len(s.via) == 0 {
		return s.stepGlide.at(now, frames)
	}
	t, moving := s.progress(now, frames)
	if !moving {
		return s.toX, s.toY, false
	}
	leg := func(i int) glidePoint {
		if i < len(s.via) {
			return s.via[i]
		}
		return glidePoint{s.toX, s.toY}
	}
	total, prev := 0.0, glidePoint{s.fromX, s.fromY}
	for i := 0; i <= len(s.via); i++ {
		p := leg(i)
		total += math.Hypot(p.x-prev.x, p.y-prev.y)
		prev = p
	}
	left, prev := total*t, glidePoint{s.fromX, s.fromY}
	for i := 0; i <= len(s.via); i++ {
		p := leg(i)
		d := math.Hypot(p.x-prev.x, p.y-prev.y)
		if left <= d && d > 0 {
			return prev.x + (p.x-prev.x)*left/d, prev.y + (p.y-prev.y)*left/d, true
		}
		left, prev = left-d, p
	}
	return s.toX, s.toY, true
}

// beginMonsterGlides records where every monster is drawn as turn-based mode
// starts, so the entry's snap to tile centres slides. Real time keeps no record.
func (g *MMGame) beginMonsterGlides() {
	clear(g.monsterGlides)
	if g.world == nil || g.combat == nil {
		return
	}
	if g.monsterGlides == nil {
		g.monsterGlides = make(map[*monster.Monster3D]monsterGlide)
	}
	for _, m := range g.world.Monsters {
		if m != nil && m.IsAlive() {
			x, y := g.combat.monsterRestPos(m)
			g.monsterGlides[m] = monsterGlide{stepGlide: stepGlide{toX: x, toY: y}, seen: g.frameCount}
		}
	}
}

// noteMonsterStep records one walked tile of a turn-based move, for a monster
// already on record.
func (g *MMGame) noteMonsterStep(m *monster.Monster3D) {
	if s, known := g.monsterGlides[m]; known {
		s.steps = append(s.steps, glidePoint{m.X, m.Y})
		g.monsterGlides[m] = s
	}
}

// advanceMonsterGlides follows where every monster is drawn at rest, once per
// world frame after the monsters acted. A change starts a slide; a first
// sighting and a snapped monster only record the spot.
func (g *MMGame) advanceMonsterGlides() {
	if g.world == nil || g.combat == nil || !g.turnBasedMode {
		clear(g.monsterGlides)
		return
	}
	if g.monsterGlides == nil {
		g.monsterGlides = make(map[*monster.Monster3D]monsterGlide)
	}
	now, frames := g.frameCount, g.stepGlideFrames()
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		x, y := g.combat.monsterRestPos(m)
		s, known := g.monsterGlides[m]
		switch {
		case !known || frames <= 0:
			s.stepGlide, s.via = stepGlide{toX: x, toY: y}, s.via[:0]
		case x != s.toX || y != s.toY:
			fromX, fromY, _ := s.at(now, frames)
			s.stepGlide = stepGlide{fromX: fromX, fromY: fromY, toX: x, toY: y, start: now - 1, on: true}
			// The last walked tile is the rest position itself.
			s.via = s.via[:0]
			if n := len(s.steps); n > 1 {
				s.via = append(s.via, s.steps[:n-1]...)
			}
		}
		s.steps, s.seen = s.steps[:0], now
		g.monsterGlides[m] = s
	}
	for m, s := range g.monsterGlides {
		if s.seen != now {
			delete(g.monsterGlides, m)
		}
	}
}

// snapMonsterGlide makes a monster's next move a jump, not a slide: a blink.
func (g *MMGame) snapMonsterGlide(m *monster.Monster3D) {
	delete(g.monsterGlides, m)
}

// glidedMonsterPos is the monster's drawn spot: (x, y) at rest, or the point
// of its turn-based slide.
func (g *MMGame) glidedMonsterPos(m *monster.Monster3D, x, y float64) (float64, float64) {
	if !g.turnBasedMode {
		return x, y
	}
	if gx, gy, moving := g.monsterGlides[m].at(g.frameCount, g.stepGlideFrames()); moving {
		return gx, gy
	}
	return x, y
}

package game

import (
	"image/color"
	"math"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/status"
)

// These visuals describe existing states, not a second status/timer registry.
// One painter supplies the same motifs on a portrait and a projected monster.
// Poison, fire and stun keep their established specialized renderers.
type statusVisuals uint32

const (
	visualRoot statusVisuals = 1 << iota
	visualSlow
	visualWeaken
	visualShred
	visualCharm
	visualBind
	visualEnrage
	visualDisease
	visualCurse
	visualSleep
	visualFear
	visualParalysis
	visualStone
)

// statusBadgeVisuals are drawn as a small marker above the head, never as
// geometry over the actor's art.
const statusBadgeVisuals = visualBind

// statusVisualEntry pairs a motif with the smallest real state that shows it.
// The editor FX tab stages motifs from these states and the tests assert the
// round trip, so a new motif cannot ship without a preview.
type statusVisualEntry struct {
	flag    statusVisuals
	key     string
	label   string
	monster func(*monster.Monster3D)              // nil for hero-only motifs
	hero    func(*MMGame, *character.MMCharacter) // nil for monster-only motifs
}

const (
	statusStageFrames = 900
	statusStageTurns  = 3
)

func heroStatusCondition(c character.Condition) func(*MMGame, *character.MMCharacter) {
	return func(_ *MMGame, h *character.MMCharacter) { h.AddCondition(c) }
}

var statusVisualCatalog = []statusVisualEntry{
	{visualRoot, "root", "rooted",
		func(m *monster.Monster3D) {
			status.RefreshDualRated(&m.RootFramesRemaining, &m.RootTurnsRemaining, &m.RootRate, statusStageFrames, statusStageTurns)
		},
		func(g *MMGame, _ *character.MMCharacter) {
			r := &g.partyRoot
			status.RefreshDualRated(&r.Frames, &r.Turns, &r.Rate, statusStageFrames, statusStageTurns)
		}},
	{visualSlow, "slow", "slowed", func(m *monster.Monster3D) { m.ApplySlow(50, statusStageFrames, statusStageTurns) },
		func(g *MMGame, _ *character.MMCharacter) { g.partyHinder.Slow = 1 }},
	{visualWeaken, "weaken", "weakened", func(m *monster.Monster3D) { m.ApplyWeaken(50, statusStageFrames, statusStageTurns) }, nil},
	{visualShred, "shred", "armor shredded", func(m *monster.Monster3D) { m.ApplyArmorShred(50, statusStageFrames, statusStageTurns) }, nil},
	{visualCharm, "charm", "charmed", func(m *monster.Monster3D) { m.Pacified, m.PacifiedFramesRemaining = true, statusStageFrames }, nil},
	{visualBind, "bind", "bound", func(m *monster.Monster3D) { m.Bound, m.BoundFramesRemaining = true, statusStageFrames }, nil},
	{visualEnrage, "enrage", "enraged", func(m *monster.Monster3D) { m.EnrageAtHP = max(1, m.HitPoints) }, nil},
	{visualDisease, "disease", "diseased", nil, heroStatusCondition(character.ConditionDiseased)},
	{visualCurse, "curse", "cursed", nil, heroStatusCondition(character.ConditionCursed)},
	{visualSleep, "sleep", "asleep", nil, heroStatusCondition(character.ConditionAsleep)},
	{visualFear, "fear", "afraid", nil, heroStatusCondition(character.ConditionFear)},
	{visualParalysis, "paralysis", "paralyzed", nil, heroStatusCondition(character.ConditionParalyzed)},
	{visualStone, "stone", "stoned", nil, heroStatusCondition(character.ConditionStone)},
}

func statusVisualEntryFor(flag statusVisuals) (statusVisualEntry, bool) {
	for _, e := range statusVisualCatalog {
		if e.flag == flag {
			return e, true
		}
	}
	return statusVisualEntry{}, false
}

func monsterStatusVisuals(m *monster.Monster3D, monsterTurn bool) statusVisuals {
	if m == nil || !m.IsAlive() {
		return 0
	}
	var v statusVisuals
	if m.RootFramesRemaining > 0 || m.RootTurnsRemaining > 0 || (monsterTurn && m.RootHeld()) {
		v |= visualRoot
	}
	if (m.SlowPct > 0 && (m.SlowFramesRemaining > 0 || m.SlowTurnsRemaining > 0)) || (monsterTurn && m.ActiveSlowPct() > 0) {
		v |= visualSlow
	}
	if (m.WeakenPct > 0 && (m.WeakenFramesRemaining > 0 || m.WeakenTurnsRemaining > 0)) || (monsterTurn && m.ActiveWeakenPct() > 0) {
		v |= visualWeaken
	}
	if m.ArmorShredPct > 0 && (m.ArmorShredFramesRemaining > 0 || m.ArmorShredTurnsRemaining > 0) {
		v |= visualShred
	}
	if m.Bound && !isPurePartySummon(m) {
		v |= visualBind
	} else if m.Pacified {
		v |= visualCharm
	}
	if m.IsEnraged() {
		v |= visualEnrage
	}
	return v
}

func (g *MMGame) partyStatusVisuals(m *character.MMCharacter) statusVisuals {
	if m == nil {
		return 0
	}
	// Death and eradication are states: the card darkens, nothing animates.
	if m.HasCondition(character.ConditionDead) || m.HasCondition(character.ConditionEradicated) {
		return 0
	}
	var v statusVisuals
	if g.partyHinder.Slow > 0 {
		v |= visualSlow
	}
	if g.partyRooted() {
		v |= visualRoot
	}
	// Preserve the established unconscious-card policy: darkening replaces
	// personal status particles. A shared root still marks the whole party.
	if m.IsIncapacitated() {
		return v
	}
	for _, c := range m.Conditions {
		switch c {
		case character.ConditionDiseased:
			v |= visualDisease
		case character.ConditionCursed:
			v |= visualCurse
		case character.ConditionAsleep:
			v |= visualSleep
		case character.ConditionFear:
			v |= visualFear
		case character.ConditionParalyzed:
			v |= visualParalysis
		case character.ConditionStone:
			v |= visualStone
		}
	}
	return v
}

func (g *MMGame) partyConditionLabel(m *character.MMCharacter) string {
	var parts []string
	for _, c := range m.Conditions {
		if c != character.ConditionNormal {
			parts = append(parts, c.String())
		}
	}
	if g.partyRooted() && !m.HasCondition(character.ConditionDead) && !m.HasCondition(character.ConditionEradicated) {
		parts = append([]string{"Rooted"}, parts...)
	}
	if g.partyHinder.Slow > 0 && !m.HasCondition(character.ConditionDead) && !m.HasCondition(character.ConditionEradicated) {
		parts = append([]string{"Slowed"}, parts...)
	}
	if len(parts) == 0 {
		return "OK"
	}
	return strings.Join(parts, ", ")
}

// statusCanvas draws in normalized actor bounds. Fine outlined geometry remains
// readable at portrait scale without hiding faces, HP text, or monster art.
// A monster supplies the existing per-primitive wall/actor depth gate; a card
// supplies its already-clipped reusable effect layer. No image allocations.
type statusCanvas struct {
	dst        *ebiten.Image
	x, y, w, h float64
	clock      float64
	visible    func(centerX, width float64) bool
	batch      *statusPathBatch
}

// Reuse geometry storage and submit one path per paint style, rather than two
// antialiased draws per line segment. Depth rejection remains per primitive,
// before batching, so walls and closer actors keep their original occlusion.
type statusPaintPath struct {
	path  vector.Path
	col   color.RGBA
	width float32 // zero means fill
}

type statusPathBatch struct {
	paths []statusPaintPath
	used  int
}

var statusPathPool = sync.Pool{New: func() any { return &statusPathBatch{} }}

func (b *statusPathBatch) path(col color.RGBA, width float32) *vector.Path {
	for i := 0; i < b.used; i++ {
		if b.paths[i].col == col && b.paths[i].width == width {
			return &b.paths[i].path
		}
	}
	if b.used == len(b.paths) {
		b.paths = append(b.paths, statusPaintPath{})
	}
	p := &b.paths[b.used]
	b.used++
	p.col, p.width = col, width
	p.path.Reset()
	return &p.path
}

func (b *statusPathBatch) flush(dst *ebiten.Image) {
	for i := 0; i < b.used; i++ {
		p := &b.paths[i]
		op := &vector.DrawPathOptions{ColorScale: statusColorScale(p.col), AntiAlias: true}
		if p.width == 0 {
			uiFillPath(dst, &p.path, nil, op)
		} else {
			uiStrokePath(dst, &p.path, &vector.StrokeOptions{Width: p.width}, op)
		}
	}
}

func (c statusCanvas) stroke(x1, y1, x2, y2, width float64, col color.RGBA) {
	x1, x2 = c.x+x1*c.w, c.x+x2*c.w
	y1, y2 = c.y+y1*c.h, c.y+y2*c.h
	px := max(1, min(3, width*min(c.w, c.h)))
	if c.visible != nil && !c.visible((x1+x2)/2, math.Abs(x2-x1)+px+1) {
		return
	}
	for _, paint := range []struct {
		col   color.RGBA
		width float32
	}{
		{color.RGBA{12, 15, 20, col.A / 2}, float32(px + 1.2)}, {col, float32(px)},
	} {
		p := c.batch.path(paint.col, paint.width)
		p.MoveTo(float32(x1), float32(y1))
		p.LineTo(float32(x2), float32(y2))
	}
}

func (c statusCanvas) dot(x, y, rad float64, col color.RGBA) {
	x, y, rad = c.x+x*c.w, c.y+y*c.h, max(0.8, min(c.w, c.h)*rad)
	if c.visible != nil && !c.visible(x, rad*2+1) {
		return
	}
	for _, paint := range []struct {
		col    color.RGBA
		radius float32
	}{
		{color.RGBA{12, 15, 20, col.A / 2}, float32(rad + .65)}, {col, float32(rad)},
	} {
		p := c.batch.path(paint.col, 0)
		p.MoveTo(float32(x)+paint.radius, float32(y))
		p.Arc(float32(x), float32(y), paint.radius, 0, 2*math.Pi, vector.Clockwise)
		p.Close()
	}
}

func (c statusCanvas) diamond(x, y, rx, ry float64, col color.RGBA) {
	c.stroke(x-rx, y, x, y-ry, .012, col)
	c.stroke(x, y-ry, x+rx, y, .012, col)
	c.stroke(x+rx, y, x, y+ry, .012, col)
	c.stroke(x, y+ry, x-rx, y, .012, col)
}

// A faceted chip has a solid dark body and a lit edge; unlike a glyph, it reads
// as material circling the actor. Roots use the same primitive for small leaves.
func (c statusCanvas) chip(x, y, rx, ry float64, col color.RGBA) {
	if c.visible != nil && !c.visible(c.x+x*c.w, rx*c.w*2+2) {
		return
	}
	fill := color.RGBA{col.R / 3, col.G / 3, col.B / 3, col.A}
	p := c.batch.path(fill, 0)
	p.MoveTo(float32(c.x+(x-rx)*c.w), float32(c.y+y*c.h))
	p.LineTo(float32(c.x+x*c.w), float32(c.y+(y-ry)*c.h))
	p.LineTo(float32(c.x+(x+rx)*c.w), float32(c.y+y*c.h))
	p.LineTo(float32(c.x+x*c.w), float32(c.y+(y+ry)*c.h))
	p.Close()
	c.diamond(x, y, rx, ry, col)
}

// fill paints a solid polygon given in normalized coordinates.
func (c statusCanvas) fill(col color.RGBA, pts ...[2]float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, q := range pts {
		lo, hi = min(lo, c.x+q[0]*c.w), max(hi, c.x+q[0]*c.w)
	}
	if c.visible != nil && !c.visible((lo+hi)/2, hi-lo+1) {
		return
	}
	p := c.batch.path(col, 0)
	for i, q := range pts {
		x, y := float32(c.x+q[0]*c.w), float32(c.y+q[1]*c.h)
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	p.Close()
}

// square maps an offset measured in units of the canvas's shorter side into
// normalized coordinates, so a motif keeps its proportions on any aspect.
func (c statusCanvas) square(cx, cy, dx, dy float64) (float64, float64) {
	u := min(c.w, c.h)
	return cx + dx*u/c.w, cy + dy*u/c.h
}

func statusColorScale(col color.RGBA) ebiten.ColorScale {
	var scale ebiten.ColorScale
	scale.ScaleWithColor(col)
	return scale
}

func (c statusCanvas) arc(cx, cy, rx, ry, from, to float64, col color.RGBA) {
	const segments = 16
	for i := 0; i < segments; i++ {
		a := from + (to-from)*float64(i)/segments
		b := from + (to-from)*float64(i+1)/segments
		c.stroke(cx+rx*math.Cos(a), cy+ry*math.Sin(a), cx+rx*math.Cos(b), cy+ry*math.Sin(b), .012, col)
	}
}

func (c statusCanvas) draw(v statusVisuals) {
	if v == 0 {
		return
	}
	b := statusPathPool.Get().(*statusPathBatch)
	b.used = 0
	c.batch = b
	c.build(v)
	b.flush(c.dst)
	statusPathPool.Put(b)
}

func (c statusCanvas) build(v statusVisuals) {
	f := c.clock
	if v&visualRoot != 0 {
		// Two thorned vines coil around the ankles, with a low floor loop.
		bark, leaf := color.RGBA{130, 102, 58, 245}, color.RGBA{158, 190, 83, 240}
		c.arc(.5, .94, .46, .055, 0, 2*math.Pi, bark)
		for side := 0; side < 2; side++ {
			base, dir := .08, 1.0
			if side == 1 {
				base, dir = .92, -1
			}
			lastX, lastY := base, .96
			for j := 1; j <= 9; j++ {
				u := float64(j) / 9
				x := base + dir*(.08+.06*math.Sin(u*6+f*.015))
				y := .96 - u*.38
				c.stroke(lastX, lastY, x, y, .022, bark)
				if j%3 == 0 {
					c.chip(x+dir*.065, y-.03, .045, .025, leaf)
					c.stroke(x, y, x-dir*.05, y-.065, .014, leaf)
				}
				lastX, lastY = x, y
			}
		}
	}
	if v&visualSlow != 0 {
		// An hourglass beside the actor: the sand drains, then the glass turns
		// over smoothly, so the loop never jumps.
		frame, glass, sand := color.RGBA{132, 211, 245, 235}, color.RGBA{190, 232, 250, 200}, color.RGBA{232, 214, 160, 235}
		const period = 480.0
		ph := math.Mod(f, period) / period
		flow, turn := min(1, ph/.8), 0.0
		if ph > .8 {
			t := (ph - .8) / .2
			turn = t * t * (3 - 2*t) * math.Pi
		}
		sin, cos := math.Sincos(turn)
		at := func(dx, dy float64) [2]float64 {
			x, y := c.square(.86, .70, dx*cos-dy*sin, dx*sin+dy*cos)
			return [2]float64{x, y}
		}
		line := func(ax, ay, bx, by, width float64, col color.RGBA) {
			a, b := at(ax, ay), at(bx, by)
			c.stroke(a[0], a[1], b[0], b[1], width, col)
		}
		const hw, hh, neck = .06, .10, .012
		line(-hw-.014, -hh, hw+.014, -hh, .022, frame)
		line(-hw-.014, hh, hw+.014, hh, .022, frame)
		for _, side := range []float64{-1, 1} {
			line(side*hw, -hh+.01, side*hw*.8, -hh*.45, .01, glass)
			line(side*hw*.8, -hh*.45, side*neck, 0, .01, glass)
			line(side*neck, 0, side*hw*.8, hh*.45, .01, glass)
			line(side*hw*.8, hh*.45, side*hw, hh-.01, .01, glass)
		}
		if left := 1 - flow; left > 0 {
			top := -.006 - left*(hh-.03)
			half := neck + (hw*.8-neck)*(-top/(hh*.45))*.9
			c.fill(sand, at(-min(half, hw*.8), top), at(min(half, hw*.8), top), at(0, -.004))
		}
		pile := hh - .014 - flow*hh*.7
		c.fill(sand, at(-hw*.85, hh-.014), at(hw*.85, hh-.014), at(0, pile))
		if flow < 1 && turn == 0 {
			line(0, 0, 0, pile, .007, sand)
		}
	}
	if v&visualWeaken != 0 {
		ash := color.RGBA{182, 151, 204, 220}
		for i := 0; i < 3; i++ {
			ph := math.Mod(f*.007+float64(i)/3, 1)
			y := .33 + ph*.36
			col := ash
			col.A = uint8(210*(1-ph) + 25)
			c.stroke(.02, y, .10, y+.07, .019, col)
			c.stroke(.10, y+.07, .18, y, .019, col)
		}
	}
	if v&visualShred != 0 {
		rust := color.RGBA{245, 175, 99, 240}
		// A visibly split shield with flakes falling from its broken seam.
		c.stroke(.80, .32, .67, .38, .018, rust)
		c.stroke(.67, .38, .71, .55, .018, rust)
		c.stroke(.71, .55, .79, .62, .018, rust)
		c.stroke(.86, .34, .96, .40, .018, rust)
		c.stroke(.96, .40, .91, .57, .018, rust)
		c.stroke(.91, .57, .85, .62, .018, rust)
		c.stroke(.81, .34, .77, .45, .015, rust)
		c.stroke(.77, .45, .84, .49, .015, rust)
		c.stroke(.84, .49, .80, .60, .015, rust)
		for i := 0; i < 3; i++ {
			ph := math.Mod(f*.01+float64(i)/3, 1)
			col := rust
			col.A = uint8(190 * (1 - ph))
			c.dot(.76+float64(i)*.05, .64+ph*.19, .016, col)
		}
	}
	if v&visualCharm != 0 {
		rose := color.RGBA{255, 148, 196, 235}
		for i := 0; i < 3; i++ {
			ph := math.Mod(f*.003+float64(i)/3, 1)
			x, y := .27+float64(i)*.23, .25-ph*.15
			col := rose
			col.A = uint8(235 * math.Sin(math.Pi*ph))
			const r = .022
			lx, ly := c.square(x, y, -r*.9, 0)
			rx, ry := c.square(x, y, r*.9, 0)
			ax, ay := c.square(x, y, -r*1.9, 0)
			bx, by := c.square(x, y, r*1.9, 0)
			tx, ty := c.square(x, y, 0, r*2.3)
			arcW, arcH := r*min(c.w, c.h)/c.w, r*min(c.w, c.h)/c.h
			c.arc(lx, ly, arcW, arcH, math.Pi, 2*math.Pi, col)
			c.arc(rx, ry, arcW, arcH, math.Pi, 2*math.Pi, col)
			c.stroke(ax, ay, tx, ty, .011, col)
			c.stroke(bx, by, tx, ty, .011, col)
		}
	}
	if v&visualEnrage != 0 {
		for side := 0; side < 2; side++ {
			x := .07 + float64(side)*.86
			for i := 0; i < 3; i++ {
				ph := math.Mod(f*.005+float64(i)/3, 1)
				y := .78 - ph*.44
				col := color.RGBA{255, 96, 60, uint8(230 * math.Sin(math.Pi*ph))}
				lx, ly := c.square(x, y, -.035, .028)
				tx, ty := c.square(x, y, 0, -.028)
				rx, ry := c.square(x, y, .035, .028)
				c.stroke(lx, ly, tx, ty, .016, col)
				c.stroke(tx, ty, rx, ry, .016, col)
			}
		}
	}
	if v&visualDisease != 0 {
		for i := 0; i < 7; i++ {
			a := f*.035 + float64(i)*2.4
			x := .5 + math.Cos(a)*(.30+.05*math.Sin(a*2))
			y := .46 + math.Sin(a*.8)*.31
			c.dot(x, y, .022, color.RGBA{182, 185, 68, 215})
			c.stroke(x-.02, y-.02, x+.035, y+.01, .009, color.RGBA{99, 116, 47, 185})
		}
	}
	if v&visualCurse != 0 {
		violet := color.RGBA{201, 138, 244, 225}
		for i := 0; i < 4; i++ {
			a := f*.014 + float64(i)*math.Pi/2
			c.arc(.5, .22, .38, .10, a, a+.9, violet)
			c.diamond(.5+.38*math.Cos(a), .22+.10*math.Sin(a), .025, .04, violet)
		}
	}
	if v&visualSleep != 0 {
		for i := 0; i < 3; i++ {
			ph := math.Mod(f*.004+float64(i)/3, 1)
			x := .56 + ph*.26
			y := .5 - ph*.40
			sz := .045 + ph*.025
			col := color.RGBA{190, 207, 245, uint8(240 - 120*ph)}
			c.stroke(x-sz, y-sz, x+sz, y-sz, .012, col)
			c.stroke(x+sz, y-sz, x-sz, y+sz, .012, col)
			c.stroke(x-sz, y+sz, x+sz, y+sz, .012, col)
		}
	}
	if v&visualFear != 0 {
		pale := color.RGBA{208, 225, 226, 225}
		for side := 0; side < 2; side++ {
			x := .1 + float64(side)*.80
			wob := .02 * math.Sin(f*.25+float64(side))
			c.stroke(x, .25, x+wob, .37, .014, pale)
			c.stroke(x+wob, .37, x-wob, .48, .014, pale)
			c.stroke(x-wob, .48, x, .58, .014, pale)
		}
	}
	if v&visualParalysis != 0 {
		gold := color.RGBA{250, 219, 108, uint8(175 + 65*math.Sin(f*.09)*math.Sin(f*.09))}
		for side := 0; side < 2; side++ {
			x := .1 + float64(side)*.8
			dir := 1.0
			if side == 1 {
				dir = -1
			}
			c.stroke(x, .2, x-dir*.035, .47, .019, gold)
			c.stroke(x-dir*.035, .47, x+dir*.10, .42, .019, gold)
			c.stroke(x+dir*.10, .42, x, .78, .019, gold)
		}
	}
	if v&visualStone != 0 {
		// Stone chips circle the hero like a slowly closing shell.
		stone := color.RGBA{181, 195, 203, 235}
		for i := 0; i < 5; i++ {
			a := f*.012 + float64(i)*2*math.Pi/5
			x, y := .5+math.Cos(a)*.44, .62+math.Sin(a)*.08
			c.chip(x, y, .065, .085, stone)
			c.stroke(x-.02, y-.025, x+.025, y-.045, .015, color.RGBA{239, 241, 226, 210})
		}
	}
}

func (r *Renderer) drawAdditionalMonsterStatusFX(screen *ebiten.Image, s UnifiedSpriteRenderData, screenY int, v statusVisuals) {
	v &^= statusBadgeVisuals
	if v == 0 || s.spriteSize <= 0 {
		return
	}
	// Use a per-piece depth test, slightly ahead of the actor's
	// own stamp. Nearby walls and other actors must still hide the particles.
	depth := math.Nextafter(s.depthPerp, math.Inf(-1))
	size := float64(s.spriteSize)
	viewBottom := worldViewportBottom(r.game)
	// As with stun stars, keep a melee-range actor's status readable when its
	// head/feet project beyond the viewport. Never put the cue under the HUD.
	top := max(4, float64(screenY))
	bottom := min(float64(viewBottom-4), float64(screenY)+size)
	if bottom <= top {
		return
	}
	width := min(size*.64, (bottom-top)*1.1)
	c := statusCanvas{dst: screen, x: float64(s.screenX) - width/2, y: top, w: width, h: bottom - top,
		clock:   float64(r.game.frameCount) + float64(monsterBurnSalt(s.monster.ID)),
		visible: func(x, w float64) bool { return r.statusFxSpanVisible(depth, x, w) }}
	c.draw(v)
}

// statusFxSpanVisible conservatively depth-tests the complete horizontal span of
// one status primitive against wall/actor columns already drawn. The painter
// pass handles nearer sprite silhouettes. Culling a small primitive when any
// column is hidden avoids painting its edge through a wall beside its centre.
func (r *Renderer) statusFxSpanVisible(depth float64, centerX, width float64) bool {
	if r == nil || r.game == nil || width <= 0 {
		return false
	}
	left := int(math.Floor(centerX - width/2))
	// Glow quads occupy a half-open destination span. Convert its exclusive
	// right edge to the final covered depth-buffer column before the inclusive
	// scan below, otherwise a wall immediately beside the glow makes it pop out.
	right := int(math.Ceil(centerX+width/2)) - 1
	bufferWidth := max(len(r.game.depthBuffer), len(r.game.actorDepthBuffer))
	if bufferWidth == 0 {
		return true // synthetic gallery anchors have no world depth buffers
	}
	if right < 0 || left >= bufferWidth {
		return false
	}
	left = max(0, left)
	right = min(bufferWidth-1, right)
	for x := left; x <= right; x++ {
		if x < len(r.game.actorDepthBuffer) && depth >= r.game.actorDepthBuffer[x] {
			return false
		}
		if x < len(r.game.depthBuffer) && depth >= r.game.depthBuffer[x] {
			return false
		}
	}
	return true
}

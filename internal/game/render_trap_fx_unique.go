package game

import (
	"fmt"
	"math"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Bespoke armed-trap renderers, selected by armed_fx in traps.yaml (the same
// registry+validation shape as spellFxStyleDraw for projectiles). Every armed
// trap used to be the SAME square of rising aura bubbles tinted by
// border_color, so a bear trap and a stasis snare read identically; these
// styles give each one its own silhouette anchored at the tile CENTRE, so the
// floor no longer shows an outlined square.
// Animation runs on frameCount and per-particle constants come from auraHash,
// so nothing here needs state.

// trapFxStyleDraw maps armed_fx to its renderer. A trap without armed_fx keeps
// the old edge glow (drawTrapTileBorders).
var trapFxStyleDraw = map[string]func(*Renderer, *ebiten.Image, trapAnchor, [3]int, int){
	"cleave_blades": (*Renderer).drawTrapFxCleaveBlades,
	"bear_jaws":     (*Renderer).drawTrapFxBearJaws,
	"stasis_rings":  (*Renderer).drawTrapFxStasisRings,
	"blast_coals":   (*Renderer).drawTrapFxBlastCoals,
}

// validateTrapFxStyles fails fast on an armed_fx naming a style with no
// renderer - a YAML typo would otherwise silently fall back to the edge glow.
func validateTrapFxStyles() {
	if config.GlobalTrapConfig == nil {
		return
	}
	for key, def := range config.GlobalTrapConfig.Traps {
		if def.ArmedFx == "" {
			continue
		}
		if _, ok := trapFxStyleDraw[def.ArmedFx]; !ok {
			panic(fmt.Sprintf("trap %q: unknown armed_fx style %q", key, def.ArmedFx))
		}
	}
}

// trapFloorSquash foreshortens the tile plane on screen. EVERY part of EVERY
// armed trap is drawn with it: a trap lies flat on its tile, so nothing may be
// lifted off the floor or tilted toward the camera.
const trapFloorSquash = 0.42

// trapAnchor is one armed trap's tile centre resolved to screen space, with the
// perspective unit every size in these renderers is measured in.
type trapAnchor struct {
	cx    float64 // screen X of the tile centre
	fy    float64 // floor screen Y at that depth
	unit  float64 // fy-horizon: the same perspective scale emitBubbleColumn uses
	fade  float64 // distance fade, 1 near -> 0 at the cull radius
	depth float64 // perpendicular depth used to clip every primitive against walls/actors
}

// trapFloorAnchor projects a trap tile's centre and applies the near/far clip.
// Wall and actor occlusion is deliberately deferred to trapFxSpanVisible: a
// centre-only rejection would erase visible edge pieces of a partially covered
// trap, while accepting the centre would let the other pieces paint through.
func (r *Renderer) trapFloorAnchor(tileX, tileY int, ts, maxDepth float64) (trapAnchor, bool) {
	wx := (float64(tileX) + 0.5) * ts
	wy := (float64(tileY) + 0.5) * ts
	screenX, depth, ok := r.game.renderHelper.projectToScreenX(wx, wy)
	if !ok || depth < auraMinDepth || depth > maxDepth {
		return trapAnchor{}, false
	}
	fade := 1.0 - depth/maxDepth
	if fade <= 0 {
		return trapAnchor{}, false
	}
	horizon := float64(r.game.config.GetScreenHeight()) / 2
	fy := float64(r.game.renderHelper.calculateFloorScreenY(depth))
	unit := fy - horizon
	if unit <= 0 {
		return trapAnchor{}, false
	}
	return trapAnchor{cx: screenX2Float(screenX), fy: fy, unit: unit, fade: fade, depth: depth}, true
}

// screenX2Float keeps the column index -> float conversion in one place.
func screenX2Float(x int) float64 { return float64(x) }

// trapFxSpanVisible conservatively depth-tests the complete horizontal span of
// one trap primitive. Trap effects are emitted after walls and actors, so a
// centre-only test lets a ring or blade paint back over an occluder beside the
// tile centre. Culling the whole small primitive when any covered column is
// hidden gives the effect a clean per-piece silhouette without a screen-sized
// mask or per-frame image allocation.
func (r *Renderer) trapFxSpanVisible(a trapAnchor, centerX, width float64) bool {
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
		if x < len(r.game.actorDepthBuffer) && a.depth >= r.game.actorDepthBuffer[x] {
			return false
		}
		if x < len(r.game.depthBuffer) && a.depth >= r.game.depthBuffer[x] {
			return false
		}
	}
	return true
}

func (r *Renderer) drawTrapGlowSprite(screen *ebiten.Image, a trapAnchor, x, y, size float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	if !r.trapFxSpanVisible(a, x, size) {
		return
	}
	r.drawGlowSprite(screen, x, y, size, rgb, alpha, blend)
}

func (r *Renderer) drawTrapGlowSpriteStretched(screen *ebiten.Image, a trapAnchor, x, y, w, h float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	if !r.trapFxSpanVisible(a, x, w) {
		return
	}
	r.drawGlowSpriteStretched(screen, x, y, w, h, rgb, alpha, blend)
}

// trapFloorMark is the shared "something is armed here" hint: a dim round scar
// on the floor. Round on purpose - the old four-edge outline is what made every
// trap read as a square.
func (r *Renderer) trapFloorMark(screen *ebiten.Image, a trapAnchor, rgb [3]int, radius, alpha float64) {
	r.drawTrapGlowSpriteStretched(screen, a, a.cx, a.fy, a.unit*radius*2, a.unit*radius*0.9,
		rgb, alpha*a.fade, ebiten.BlendSourceOver)
}

// trapFlamePetal draws one flame lick lying IN the floor plane: it runs outward
// from (x,y) along (dirX,dirY), tapering and cooling. Same round-chain density
// rule as the trap teeth (step ~2px) so it fuses instead of beading, and no part
// of it leaves the tile's own plane.
func (r *Renderer) trapFlamePetal(screen *ebiten.Image, a trapAnchor, x, y, dirX, dirY, length, baseW float64, deep, flame, hot [3]int, alpha float64) {
	const drops = 11 // step ~2px at tile scale: the petal fuses instead of beading
	for k := 0; k < drops; k++ {
		f := float64(k) / (drops - 1)
		w := baseW * (1 - 0.60*f)
		px := x + dirX*length*f
		py := y + dirY*length*f*trapFloorSquash
		col := mixColor(flame, deep, f)
		r.drawTrapGlowSprite(screen, a, px, py, w*1.3, col, alpha*0.55, ebiten.BlendSourceOver)
		r.drawTrapGlowSprite(screen, a, px, py, w, col, alpha, ebiten.BlendSourceOver)
		if f < 0.35 { // white heart at the root
			r.drawTrapGlowSprite(screen, a, px, py, w*0.55, hot, alpha*(1-f/0.35)*0.85, additiveGlowBlend)
		}
	}
}

// -----------------------------------------------------------------------------
// Cleave Trap - a scything blade circle: two steel crescents sweeping the tile
// at different radii and speeds, with a glint where they cross. The wide AoE is
// shown as a ring of dim floor ticks, not a filled square.
// -----------------------------------------------------------------------------
func (r *Renderer) drawTrapFxCleaveBlades(screen *ebiten.Image, a trapAnchor, rgb [3]int, id int) {
	fc := float64(r.game.frameCount)
	steel := rgb
	dark := mixColor(rgb, [3]int{40, 45, 55}, 0.65)
	glint := [3]int{255, 255, 240}

	// AoE ring: sparse ticks around the blast circle so the radius is legible
	// without drawing a disc that hides the floor.
	for k := 0; k < 16; k++ {
		ang := float64(k)/16*2*math.Pi + fc*0.010
		rx, ry := a.unit*0.95, a.unit*0.95*trapFloorSquash
		r.drawTrapGlowSprite(screen, a, a.cx+math.Cos(ang)*rx, a.fy+math.Sin(ang)*ry,
			a.unit*0.055, mixColor(dark, steel, 0.55), 0.80*a.fade, additiveGlowBlend)
	}

	// Two crescents. Each is a short arc of overlapping stretched glows, so it
	// reads as a moving blade edge rather than a chain of dots.
	for b := 0; b < 2; b++ {
		spin := fc * (0.075 + 0.028*float64(b))
		if b == 1 {
			spin = -spin * 0.8 // counter-rotation: the pair looks like shears
		}
		radius := a.unit * (0.40 + 0.26*float64(b))
		for s := 0; s < 13; s++ {
			t := float64(s) / 12
			ang := spin + (t-0.5)*1.15     // ~65 deg of arc
			taper := math.Sin(t * math.Pi) // thin at both tips
			px := a.cx + math.Cos(ang)*radius
			py := a.fy + math.Sin(ang)*radius*trapFloorSquash
			r.drawTrapGlowSpriteStretched(screen, a, px, py,
				a.unit*(0.06+0.15*taper), a.unit*(0.026+0.060*taper),
				mixColor(dark, steel, taper), (0.55+0.45*taper)*a.fade, ebiten.BlendSourceOver)
		}
		// Leading edge highlight - the part that would catch the light.
		lead := spin + 0.55
		r.drawTrapGlowSprite(screen, a, a.cx+math.Cos(lead)*radius, a.fy+math.Sin(lead)*radius*trapFloorSquash,
			a.unit*0.11, steel, 1.0*a.fade, additiveGlowBlend)
	}

	// Crossing glint: both blades line up twice per cycle; flash then.
	cross := math.Abs(math.Sin(fc * 0.075 * 1.8))
	if cross > 0.93 {
		flash := (cross - 0.93) / 0.07
		r.drawTrapGlowSprite(screen, a, a.cx, a.fy, a.unit*0.30*flash, glint, 0.7*flash*a.fade, additiveGlowBlend)
	}
}

// -----------------------------------------------------------------------------
// Bear Trap - a compact iron object, not a field: two opposed toothed jaws held
// under tension, half-buried in scuffed dirt. It twitches instead of pulsing so
// the metal reads as sprung, and the camouflage litter keeps the silhouette
// irregular.
// -----------------------------------------------------------------------------
func (r *Renderer) drawTrapFxBearJaws(screen *ebiten.Image, a trapAnchor, rgb [3]int, id int) {
	fc := float64(r.game.frameCount)
	iron := [3]int{198, 200, 210}
	ironDark := [3]int{96, 92, 88}
	dirt := rgb // earth tone from border_color
	shine := [3]int{240, 245, 250}

	// Disturbed soil the jaws are set into.
	r.trapFloorMark(screen, a, mixColor(ironDark, [3]int{20, 16, 14}, 0.55), 0.50, 0.55)

	// Spring tension: mostly still, with a sharp snap-test twitch - a sprung trap
	// is not a breathing thing, but a dead-still one reads as scenery.
	shiver, flash := 0.0, 0.0
	if ph := frac(fc * 0.012); ph > 0.88 {
		k := (ph - 0.88) / 0.12
		shiver = math.Sin(k*math.Pi) * 0.075
		flash = math.Sin(k * math.Pi)
	}
	// The maw: a dark elliptical throat the jaws are set around. Drawn first and
	// wide, it gives the iron something to bite into and keeps the silhouette
	// round instead of a lattice of bars.
	r.drawTrapGlowSpriteStretched(screen, a, a.cx, a.fy, a.unit*0.90, a.unit*0.40,
		[3]int{14, 12, 11}, 0.85*a.fade, ebiten.BlendSourceOver)

	// A real trap is pan + jaws + teeth + two leaf springs, lying FLAT on the
	// ground - every part is drawn in the tile plane (y squashed), never tilted at
	// the camera.
	//
	// DENSITY IS THE WHOLE TRICK. A soft glow's bright core is only ~40% of its
	// diameter, so at tile scale (parts are 4-8px) a chain only fuses when the
	// step is ~2px. That is why the cleave blades read and why a sparser chain
	// beads. Arcs here are short and tightly stepped for the same reason.
	gap := 1.0 + shiver
	rx := a.unit * 0.40
	ry := rx * trapFloorSquash

	// Jaws: two arcs of ~140 deg facing each other across the pan.
	for side := 0; side < 2; side++ {
		dir := -1.0 // far jaw
		if side == 1 {
			dir = 1.0
		}
		const arcSteps = 26
		const sweep = 2.45 // radians (~140 deg)
		for s := 0; s <= arcSteps; s++ {
			t := float64(s) / arcSteps
			ang := (math.Pi-sweep)/2 + sweep*t
			taper := 0.30 + 0.70*math.Sin(math.Pi*t)
			ax := a.cx + math.Cos(ang)*rx
			ay := a.fy + dir*math.Sin(ang)*ry*gap
			lit := 0.35 + 0.45*math.Sin(ang)
			if dir < 0 {
				lit += 0.20 // the far jaw catches the light
			}
			r.drawTrapGlowSpriteStretched(screen, a, ax, ay,
				a.unit*(0.05+0.05*taper), a.unit*(0.030+0.028*taper),
				mixColor(ironDark, iron, lit), 1.0*a.fade, ebiten.BlendSourceOver)
		}
		// Teeth pointing inward at the pan, in the floor plane.
		const teeth = 4
		for k := 0; k < teeth; k++ {
			t := (float64(k) + 0.5) / teeth
			ang := (math.Pi-sweep)/2 + sweep*t
			bx := a.cx + math.Cos(ang)*rx*0.92
			by := a.fy + dir*math.Sin(ang)*ry*gap*0.86
			dx, dy := a.cx-bx, a.fy-by
			d := math.Hypot(dx, dy)
			if d <= 0 {
				continue
			}
			dx, dy = dx/d, dy/d
			long := 1.0
			if k%2 == 1 {
				long = 0.70
			}
			toothLen := a.unit * 0.095 * long
			const drops = 7 // step ~2px: the cores overlap in any direction
			for j := 0; j < drops; j++ {
				f := float64(j) / (drops - 1)
				w := a.unit * 0.070 * (1 - 0.62*f)
				r.drawTrapGlowSprite(screen, a, bx+dx*toothLen*f, by+dy*toothLen*f, w,
					mixColor(ironDark, iron, 0.45+0.45*f), 1.0*a.fade, ebiten.BlendSourceOver)
			}
			if flash > 0.02 {
				r.drawTrapGlowSprite(screen, a, bx+dx*toothLen, by+dy*toothLen, a.unit*0.05,
					shine, flash*0.9*a.fade, additiveGlowBlend)
			}
		}
	}

	// Leaf springs: one flat steel bar past each hinge.
	for _, sx := range [2]float64{-1, 1} {
		const bar = 9
		for j := 0; j < bar; j++ {
			f := float64(j) / (bar - 1)
			px := a.cx + sx*(rx*1.02+a.unit*0.22*f)
			py := a.fy
			r.drawTrapGlowSpriteStretched(screen, a, px, py,
				a.unit*(0.075-0.030*f), a.unit*(0.055-0.022*f),
				mixColor(ironDark, iron, 0.32+0.24*(1-f)), 1.0*a.fade, ebiten.BlendSourceOver)
		}
	}

	// Pan: the round trigger disc at the centre, flat and dull so the teeth stay
	// the bright part.
	r.drawTrapGlowSpriteStretched(screen, a, a.cx, a.fy, a.unit*0.30, a.unit*0.30*trapFloorSquash,
		mixColor(ironDark, dirt, 0.45), 0.95*a.fade, ebiten.BlendSourceOver)
	r.drawTrapGlowSpriteStretched(screen, a, a.cx, a.fy, a.unit*0.20, a.unit*0.20*trapFloorSquash,
		mixColor(dirt, iron, 0.25), 0.75*a.fade, ebiten.BlendSourceOver)

	// Two dirt specks - just enough to say the iron is set in the ground. A
	// fuller litter scatter fights the frame for attention at tile scale.
	for k := 0; k < 2; k++ {
		hx := auraHash(id, k, 41, 0)
		r.drawTrapGlowSpriteStretched(screen, a,
			a.cx+(hx-0.5)*a.unit*0.80, a.fy+a.unit*0.14,
			a.unit*0.06, a.unit*0.026, mixColor(dirt, ironDark, 0.3), 0.50*a.fade, ebiten.BlendSourceOver)
	}

	// One cold specular line so the iron never looks like painted stone.
	r.drawTrapGlowSpriteStretched(screen, a, a.cx-a.unit*0.10, a.fy-ry*gap*0.92, a.unit*0.24, a.unit*0.016,
		shine, (0.55+0.35*flash)*a.fade, additiveGlowBlend)
}

// -----------------------------------------------------------------------------
// Stasis Trap - frozen time: rings that do NOT flow. They advance in quantised
// stutter steps and hold, and the motes drifting inward freeze mid-path, which
// is what sells "time stopped" better than any smooth animation.
// -----------------------------------------------------------------------------
func (r *Renderer) drawTrapFxStasisRings(screen *ebiten.Image, a trapAnchor, rgb [3]int, id int) {
	fc := float64(r.game.frameCount)
	violet := rgb
	pale := mixColor(rgb, [3]int{255, 255, 255}, 0.55)

	// Quantised clock: 6 discrete steps per cycle, each held - no interpolation.
	// The cycle is fast enough that the player SEES it step (the old 0.0035 rate
	// took ~48s per cycle, which read as a static sprinkle).
	const steps = 6
	cycle := frac(fc * 0.0045)
	step := math.Floor(cycle * steps)
	stepT := step / steps
	// Age within the held step drives the tick flash: each jump strikes, then
	// settles. This is the whole animation an onlooker registers.
	stepAge := frac(cycle * steps)
	tick := math.Max(0, 1-stepAge*3)

	// A dark scar under the rings so violet light has something to read against
	// on a pale floor.
	r.trapFloorMark(screen, a, [3]int{18, 10, 26}, 0.62, 0.70)

	// Three rings, each offset in the same stepped clock. Draw all dark shoulders
	// first, then all additive cores. Alternating blend modes for every dot breaks
	// Ebitengine batching and turned 84 ring points into roughly 168 GPU commands;
	// two passes preserve the same layers with one batch per blend.
	for pass := 0; pass < 2; pass++ {
		for ring := 0; ring < 3; ring++ {
			phase := frac(stepT + float64(ring)/3)
			rad := a.unit * (0.20 + 0.70*phase)
			alpha := (1 - phase*0.75) * a.fade
			if alpha <= 0.02 {
				continue
			}
			dots := 22 + ring*6
			size := a.unit * (0.075 - 0.02*phase)
			for k := 0; k < dots; k++ {
				ang := float64(k)/float64(dots)*2*math.Pi + float64(ring)*0.4
				px := a.cx + math.Cos(ang)*rad
				py := a.fy + math.Sin(ang)*rad*trapFloorSquash
				if pass == 0 {
					r.drawTrapGlowSprite(screen, a, px, py, size*1.5,
						[3]int{40, 14, 60}, alpha*0.55, ebiten.BlendSourceOver)
					continue
				}
				r.drawTrapGlowSprite(screen, a, px, py, size,
					mixColor(violet, pale, phase), alpha*1.15, additiveGlowBlend)
			}
		}
	}

	// Motes falling inward that freeze: position is a stepped function of the
	// clock, so each mote jumps, holds, jumps.
	// Motes hop inward ALONG THE FLOOR: the snare is a figure drawn on its tile,
	// so nothing here lifts out of that plane.
	for k := 0; k < 14; k++ {
		seed := auraHash(id, k, 51, 0)
		lane := seed * 2 * math.Pi
		local := frac(stepT + seed) // stepped -> discrete hops
		rad := a.unit * (0.85 - 0.70*local)
		px := a.cx + math.Cos(lane)*rad
		py := a.fy + math.Sin(lane)*rad*trapFloorSquash
		hold := 0.55 + 0.45*math.Sin(local*math.Pi)
		r.drawTrapGlowSprite(screen, a, px, py, a.unit*0.075, pale, hold*1.1*a.fade, additiveGlowBlend)
	}

	// Core: an almost still glow with a bright pip - the stopped point of time -
	// struck harder for a moment on every clock step.
	breath := 1 + 0.05*math.Sin(fc*0.02)
	r.drawTrapGlowSpriteStretched(screen, a, a.cx, a.fy, a.unit*0.40*breath,
		a.unit*0.40*breath*trapFloorSquash, violet, (0.70+0.30*tick)*a.fade, additiveGlowBlend)
	r.drawTrapGlowSprite(screen, a, a.cx, a.fy, a.unit*0.14, pale, 1.0*a.fade, additiveGlowBlend)

	// The step itself: a thin shock ring snapping outward, then gone. Without it
	// the quantised clock has nothing an onlooker can catch.
	if tick > 0.02 {
		rad := a.unit * (0.30 + 0.55*(1-tick))
		for k := 0; k < 26; k++ {
			ang := float64(k) / 26 * 2 * math.Pi
			r.drawTrapGlowSprite(screen, a, a.cx+math.Cos(ang)*rad, a.fy+math.Sin(ang)*rad*trapFloorSquash,
				a.unit*0.05, pale, tick*0.85*a.fade, additiveGlowBlend)
		}
	}
}

// -----------------------------------------------------------------------------
// Blast Trap - a bed of smouldering coals waiting to become a pillar: banked
// embers breathing on their own cycles, heat shimmer above them, and the odd
// spark popping off. Source-over for the coals so they have mass on bright
// floors; additive only for the heat and sparks.
// -----------------------------------------------------------------------------
func (r *Renderer) drawTrapFxBlastCoals(screen *ebiten.Image, a trapAnchor, rgb [3]int, id int) {
	fc := float64(r.game.frameCount)
	deep := [3]int{190, 45, 12}
	flame := rgb
	hot := [3]int{255, 240, 180}
	soot := [3]int{55, 40, 34}

	// Scorch ring the charge is buried in.
	r.trapFloorMark(screen, a, soot, 0.50, 0.55)

	// Coal bed: each coal breathes between soot and flame on its own phase, so
	// the cluster glows unevenly like real embers.
	for k := 0; k < 13; k++ {
		hx := auraHash(id, k, 71, 0)
		hy := auraHash(id, k, 72, 0)
		beat := frac(fc*0.045*(0.6+auraHash(id, k, 73, 0)*0.9) + hx)
		glow := 0.35 + 0.65*math.Sin(beat*math.Pi)
		px := a.cx + (hx-0.5)*a.unit*0.60
		py := a.fy + (hy-0.5)*a.unit*0.20
		r.drawTrapGlowSprite(screen, a, px, py, a.unit*(0.055+0.035*hy),
			mixColor(soot, deep, glow), 0.95*a.fade, ebiten.BlendSourceOver)
		r.drawTrapGlowSprite(screen, a, px, py, a.unit*(0.030+0.025*hy),
			mixColor(deep, flame, glow), glow*0.95*a.fade, ebiten.BlendSourceOver)
		if glow > 0.8 { // the hottest coals show a white heart
			r.drawTrapGlowSprite(screen, a, px, py, a.unit*0.018, hot, (glow-0.8)/0.2*0.8*a.fade, additiveGlowBlend)
		}
	}

	// Flame petals licking OUTWARD across the floor. Upright tongues (and the old
	// vertical heat column) stood the effect up out of its tile; a buried charge
	// burns as a low rose of fire lying on the ground.
	const petals = 7
	for k := 0; k < petals; k++ {
		seed := auraHash(id, k, 81, 0)
		ang := float64(k)/petals*2*math.Pi + seed*0.6
		ph := frac(fc*0.030*(0.7+seed*0.7) + seed)
		lick := math.Sin(ph * math.Pi) // grows and dies each cycle
		if lick <= 0.05 {
			continue
		}
		r.trapFlamePetal(screen, a, a.cx, a.fy, math.Cos(ang), math.Sin(ang),
			a.unit*(0.16+0.34*lick), a.unit*(0.105+0.040*lick),
			deep, flame, hot, (0.60+0.35*lick)*a.fade)
	}

	// Heat bloom: a flat pulse of light over the bed instead of a rising column.
	pulse := 0.5 + 0.5*math.Sin(fc*0.05)
	r.drawTrapGlowSpriteStretched(screen, a, a.cx, a.fy, a.unit*(0.80+0.10*pulse),
		a.unit*(0.80+0.10*pulse)*trapFloorSquash,
		mixColor(flame, soot, 0.45), (0.22+0.14*pulse)*a.fade, additiveGlowBlend)

	// Sparks skittering off the bed ALONG the ground, cooling as they go.
	for k := 0; k < 9; k++ {
		seed := auraHash(id, k, 91, 0)
		ang := auraHash(id, k, 92, 0) * 2 * math.Pi
		ph := frac(fc*0.035*(0.7+seed*0.9) + seed)
		reach := a.unit * (0.12 + 0.62*ph)
		px := a.cx + math.Cos(ang)*reach
		py := a.fy + math.Sin(ang)*reach*trapFloorSquash
		col := mixColor(hot, flame, math.Min(1, ph*2))
		if ph > 0.5 {
			col = mixColor(flame, soot, (ph-0.5)/0.5)
		}
		r.drawTrapGlowSprite(screen, a, px, py, a.unit*0.035*(1-ph*0.4), col, (1-ph)*0.9*a.fade, additiveGlowBlend)
	}
}

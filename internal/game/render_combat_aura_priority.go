package game

import "math"

type combatAuraMask struct {
	start, end, strength float64
	next                 int
}

func (c *combatAuraScratch) mask(edge tileEdge, start, end, strength float64) {
	start, end = max(0, start), min(1, end)
	if end <= start || strength <= 0 {
		return
	}
	c.spans = append(c.spans, combatAuraMask{start, end, strength, c.priority[edge]})
	c.priority[edge] = len(c.spans)
}

func (c *combatAuraScratch) priorityStrength(edge tileEdge, t float64) float64 {
	strength := 0.0
	for i := c.priority[edge]; i > 0; i = c.spans[i-1].next {
		mask := c.spans[i-1]
		if t >= mask.start && t <= mask.end {
			strength = max(strength, mask.strength)
		}
	}
	return strength
}

// Only actual aurora intersections suppress the tactical glow. Ordinary VFX
// coexist. As an aura fades out, the underlying tactical edge returns smoothly.
func (r *Renderer) reserveCombatAuraPriority(ts float64) {
	c := &r.combatAura
	clear(c.priority)
	c.spans = c.spans[:0]
	for _, edge := range r.auraCurtainEdges {
		x0, y0, x1, y1 := edge.world0[0]/ts, edge.world0[1]/ts, edge.world1[0]/ts, edge.world1[1]/ts
		opacity := func(x, y float64) float64 {
			_, depth, _ := r.game.renderHelper.cameraSpaceXY(x*ts, y*ts)
			return min(1, max(0, edge.alpha*(1-max(0, depth)/edge.maxDepth)/.15))
		}
		// Collinear authored tile edges share their exact covered interval.
		if math.Abs(x1-x0) < 1e-8 && math.Abs(x0-math.Round(x0)) < 1e-8 {
			for y := int(math.Floor(min(y0, y1))); y < int(math.Ceil(max(y0, y1))); y++ {
				a, b := max(float64(y), min(y0, y1)), min(float64(y+1), max(y0, y1))
				c.mask(tileEdge{int(math.Round(x0)), y, true}, a-float64(y), b-float64(y), opacity(x0, (a+b)*.5))
			}
			continue
		}
		if math.Abs(y1-y0) < 1e-8 && math.Abs(y0-math.Round(y0)) < 1e-8 {
			for x := int(math.Floor(min(x0, x1))); x < int(math.Ceil(max(x0, x1))); x++ {
				a, b := max(float64(x), min(x0, x1)), min(float64(x+1), max(x0, x1))
				c.mask(tileEdge{x, int(math.Round(y0)), false}, a-float64(x), b-float64(x), opacity((a+b)*.5, y0))
			}
			continue
		}
		// A curved ward crosses a grid edge at a point, not an entire 3x3 block.
		// Leave a narrow gap around that crossing so the two curtains do not mix.
		const halfGap = .06
		if math.Abs(x1-x0) > 1e-8 {
			for x := int(math.Ceil(min(x0, x1))); x <= int(math.Floor(max(x0, x1))); x++ {
				y := y0 + (y1-y0)*(float64(x)-x0)/(x1-x0)
				for cell := int(math.Floor(y - halfGap)); cell <= int(math.Floor(y+halfGap)); cell++ {
					c.mask(tileEdge{x, cell, true}, y-halfGap-float64(cell), y+halfGap-float64(cell), opacity(float64(x), y))
				}
			}
		}
		if math.Abs(y1-y0) > 1e-8 {
			for y := int(math.Ceil(min(y0, y1))); y <= int(math.Floor(max(y0, y1))); y++ {
				x := x0 + (x1-x0)*(float64(y)-y0)/(y1-y0)
				for cell := int(math.Floor(x - halfGap)); cell <= int(math.Floor(x+halfGap)); cell++ {
					c.mask(tileEdge{cell, y, false}, x-halfGap-float64(cell), x+halfGap-float64(cell), opacity(x, float64(y)))
				}
			}
		}
	}
}

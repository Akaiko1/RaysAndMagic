package game

import (
	"math"
	"slices"
	"ugataima/internal/world"
)

const combatAuraRiseFraction = auraRiseFraction / 3

var (
	combatAuraBlue  = [3]int{130, 205, 255}
	combatAuraGreen = [3]int{75, 225, 120}
	combatAuraRed   = [3]int{255, 70, 65}
)

type combatAuraStyle uint8

const (
	combatAuraGrid combatAuraStyle = iota + 1
	combatAuraMove
	combatAuraAttack
)

// Each grid edge has one owner even when two tiles or several ranges share it.
// Red wins over green; other auroras reserve their edges before either.
type combatAuraEdge = tileEdge

type combatAuraScratch struct {
	valid    bool
	key      uint64
	world    *world.World3D
	edges    map[combatAuraEdge]combatAuraStyle
	priority map[combatAuraEdge]int
	spans    []combatAuraMask
	cuts     []float64
	tiles    map[[2]int]bool
	queue    [][2]int
	ordered  []combatAuraEdge
}

func (c *combatAuraScratch) reset() {
	if c.edges == nil {
		c.edges = make(map[combatAuraEdge]combatAuraStyle)
		c.priority = make(map[combatAuraEdge]int)
		c.tiles = make(map[[2]int]bool)
	}
	clear(c.edges)
	c.ordered = c.ordered[:0]
}

func combatTileEdges(tx, ty int) [4]combatAuraEdge { return tileEdges(tx, ty) }

func (c *combatAuraScratch) add(edge combatAuraEdge, style combatAuraStyle) {
	if style > c.edges[edge] {
		c.edges[edge] = style
	}
}

func (c *combatAuraScratch) outline(style combatAuraStyle) {
	for tile := range c.tiles {
		for i, edge := range combatTileEdges(tile[0], tile[1]) {
			d := auraCardinalDirections[i]
			if !c.tiles[[2]int{tile[0] + d[0], tile[1] + d[1]}] {
				c.add(edge, style)
			}
		}
	}
}

func (r *Renderer) collectCombatAurora() {
	g := r.game
	// No inventory, map or actor scans while disabled or outside TB.
	if !g.turnBasedMode || !g.combatPreferences.TurnBasedOverlay || g.world == nil || g.combat == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	c := &r.combatAura
	if !g.prepareCombatAura(c, ts) {
		return
	}
	r.reserveCombatAuraPriority(ts)
	_, density, _ := r.auraEdgeParams()
	for _, edge := range c.ordered {
		if !r.combatAuraTileVisible(edge.x, edge.y, ts) {
			continue
		}
		rgb, alpha := combatAuraBlue, .9
		switch c.edges[edge] {
		case combatAuraMove:
			rgb, alpha = combatAuraGreen, 1
		case combatAuraAttack:
			rgb, alpha = combatAuraRed, 1
		}
		alpha *= 0.5
		x0, y0, x1, y1 := edge.points(ts)
		c.cuts = append(c.cuts[:0], 0, 1)
		for i := c.priority[edge]; i > 0; i = c.spans[i-1].next {
			mask := c.spans[i-1]
			c.cuts = append(c.cuts, mask.start, mask.end)
		}
		slices.Sort(c.cuts)
		for i := 1; i < len(c.cuts); i++ {
			a, b := c.cuts[i-1], c.cuts[i]
			if b-a < 1e-6 {
				continue
			}
			strength := 1 - c.priorityStrength(edge, (a+b)*.5)
			if curtain := r.collectAuraSegment(x0+(x1-x0)*a, y0+(y1-y0)*a, x0+(x1-x0)*b, y0+(y1-y0)*b, ts, combatAuraRiseFraction, density, alpha*strength, g.camera.ViewDist, rgb); curtain != nil {
				curtain.coverageBlend = true
			}
		}
	}
}

func (r *Renderer) combatAuraTileVisible(tx, ty int, ts float64) bool {
	x, y := TileCenterFromTile(tx, ty, ts)
	side, depth, ok := r.game.renderHelper.cameraSpaceXY(x, y)
	margin := ts / math.Tan(r.game.viewFOV()/2)
	return ok && depth+ts > auraMinDepth && depth-ts < r.game.camera.ViewDist && math.Abs(side) <= depth+margin+ts
}

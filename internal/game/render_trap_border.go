package game

import (
	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawTrapTileBorders draws every armed trap. A trap whose armed_fx names a
// bespoke style (trapFxStyleDraw) gets that centre-anchored effect instead of an
// outline, so traps stop reading as identical squares; traps without armed_fx
// keep the original edge-bubble technique from the impassable aura, tinted by
// border_color.
func (r *Renderer) drawTrapTileBorders(screen *ebiten.Image) {
	traps := r.game.traps
	if len(traps) == 0 || r.game.world == nil {
		return
	}
	baseAlpha, perEdge, radius := r.auraEdgeParams()

	ts := float64(r.game.config.GetTileSize())
	maxDepth := float64(radius) * ts

	for i := range traps {
		t := &traps[i]
		if !mapKeyOnCurrentWorld(t.MapKey) {
			continue
		}
		def, ok := config.GetTrapDefinition(t.Key)
		if !ok {
			continue
		}
		rgb := [3]int{
			clampColor(def.BorderColor[0]),
			clampColor(def.BorderColor[1]),
			clampColor(def.BorderColor[2]),
		}
		if draw, ok := trapFxStyleDraw[def.ArmedFx]; ok {
			// Hash id from the tile so every armed trap scatters its particles
			// differently while staying stable frame to frame.
			if a, visible := r.trapFloorAnchor(t.TileX, t.TileY, ts, maxDepth); visible {
				draw(r, screen, a, rgb, t.TileX*73+t.TileY*131)
			}
			continue
		}
		r.emitAuraTileEdges(screen, t.TileX, t.TileY, ts, perEdge, baseAlpha, maxDepth, rgb)
	}
}

// Brood-fire trap edging. The tongues are FIREWALL's flames (emitFlameColumn's
// profile: soft tapered columns, white-hot base to ember tip, source-over so
// they occlude), scaled down and packed dense - the earlier version borrowed
// the aura's BUBBLE profile and read as glowing dots, not fire.
const (
	trapFlamePerEdge     = 7    // tongues per tile edge: many and small
	trapFlamePerColumn   = 2    // staggered tongues per sample point
	trapFlameRiseMult    = 0.85 // ~a third of a tile: ground fire, not a wall
	trapFlameBaseAlpha   = 0.8
	trapFlameSizeFloor   = 2.2
	trapFlameSizeCoef    = 0.1
	trapFlameFadeTiles   = 14.0
	trapFlameTongueRatio = 2.4 // taller than wide, like the firewall tongues
)

// drawBossFireTrapBorders marks the Brood Mother's field: every armed tile is
// edged with small flame tongues on the SAME sample line the aura/trap borders
// use, so the smouldering ground reads as fire without becoming a firewall.
func (r *Renderer) drawBossFireTrapBorders(screen *ebiten.Image) {
	field := r.game.bossFireTraps
	if len(field) == 0 {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	maxDepth := trapFlameFadeTiles * ts
	for _, t := range field {
		for _, d := range auraCardinalDirections {
			for s := 0; s < trapFlamePerEdge; s++ {
				wx, wy := tileEdgeSamplePoint(t.TX, t.TY, d, ts, s, trapFlamePerEdge)
				r.emitTrapFlameColumn(screen, wx, wy, t.TX, t.TY, d[0]*2+d[1], s, maxDepth)
			}
		}
	}
}

// emitTrapFlameColumn draws one small flame tongue stack at a sampled point on
// a trap tile's edge. Same machinery and colour ramp as the Firewall tongues,
// tuned short and thin.
func (r *Renderer) emitTrapFlameColumn(screen *ebiten.Image, wx, wy float64, tx, ty, edgeKey, sIdx int, maxDepth float64) {
	r.emitBubbleColumn(screen, bubbleColumnFx{
		wx: wx, wy: wy,
		hx: tx, hy: ty, salt: edgeKey + 31, hi: sIdx,
		maxDepth:     maxDepth,
		riseFraction: auraRiseFraction * trapFlameRiseMult,
		baseAlpha:    trapFlameBaseAlpha,
		colBright:    1.0,
		perColumn:    trapFlamePerColumn,
		periodTick:   flamePeriodTick,
		jitterMin:    auraSpeedJitterMin,
		jitterSpan:   (1.0 - auraSpeedJitterMin) * 2,
		sizeFloor:    trapFlameSizeFloor,
		sizeCoef:     trapFlameSizeCoef,
		wobbleCoef:   0.5,
		sizeJitter:   flameSizeJitter,
		soft:         true,
		sizeTaper:    flameTipSizeScale,
		color:        flameCoreColor,
		heightScale:  trapFlameTongueRatio,
		srcOver:      true,
		colorTop:     flameTipColor,
	})
}

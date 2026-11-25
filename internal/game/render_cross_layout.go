package game

import (
	"math"
	"math/bits"

	"ugataima/internal/world"
)

// crossPlane is one upright plane of a crossed tile, through the tile centre.
// Each half of it, centre to end, is an arm; an arm that is not raised is not
// drawn.
type crossPlane struct {
	yaw       float64
	footprint float64 // world length, end to end
	arms      [2]bool // the +yaw half, the -yaw half
}

// crossLayout is the planes a crossed tile draws. A tree or a built prop is
// two whole planes; a linked panel raises only the arms that meet a neighbour.
type crossLayout struct {
	planes [maxCrossPlanes]crossPlane
	count  int
}

const (
	maxCrossPlanes = 4
	maxCrossArms   = 2 * maxCrossPlanes
)

// twoPlaneCross is the ordinary cross: two whole planes of one length.
func twoPlaneCross(yawA, yawB, footprint float64) crossLayout {
	whole := [2]bool{true, true}
	return crossLayout{count: 2, planes: [maxCrossPlanes]crossPlane{
		{yaw: yawA, footprint: footprint, arms: whole},
		{yaw: yawB, footprint: footprint, arms: whole},
	}}
}

// The static diagonals of a tree or prop cross.
const (
	crossDiagonalYawA = math.Pi / 4
	crossDiagonalYawB = 3 * math.Pi / 4
)

// panelArms is the set of half-panels a linked-panel tile raises, one bit per
// neighbour direction. Zero means the tile is not a linked panel.
type panelArms uint8

const (
	panelArmE panelArms = 1 << iota
	panelArmW
	panelArmS
	panelArmN
	panelArmSE
	panelArmNW
	panelArmSW
	panelArmNE
)

// panelPlanes are the four planes a linked panel can stand in: along the two
// sides of the tile and along its two diagonals. dx, dy is the neighbour the
// positive arm points at; the negative arm points at the opposite one.
var panelPlanes = [maxCrossPlanes]struct {
	yaw                float64
	diagonal           bool
	positive, negative panelArms
	dx, dy             int
}{
	{yaw: 0, positive: panelArmE, negative: panelArmW, dx: 1, dy: 0},
	{yaw: math.Pi / 2, positive: panelArmS, negative: panelArmN, dx: 0, dy: 1},
	{yaw: crossDiagonalYawA, diagonal: true, positive: panelArmSE, negative: panelArmNW, dx: 1, dy: 1},
	{yaw: crossDiagonalYawB, diagonal: true, positive: panelArmSW, negative: panelArmNE, dx: -1, dy: 1},
}

// linkedPanelArms picks the arms of one linked-panel tile. isPanel reports a
// linked panel at an offset from the tile. A diagonal neighbour counts only
// when the tiles do not already join through a shared side. A tile with one
// link stands as a whole straight panel on that line; a tile with none stands
// whole along a side of the tile, lengthwise or crosswise by the coin.
func linkedPanelArms(isPanel func(dx, dy int) bool, coin bool) panelArms {
	var arms panelArms
	for _, plane := range panelPlanes {
		for side, arm := range [2]panelArms{plane.positive, plane.negative} {
			dx, dy := plane.dx, plane.dy
			if side == 1 {
				dx, dy = -dx, -dy
			}
			if !isPanel(dx, dy) {
				continue
			}
			if plane.diagonal && (isPanel(dx, 0) || isPanel(0, dy)) {
				continue
			}
			arms |= arm
		}
	}
	switch bits.OnesCount8(uint8(arms)) {
	case 0:
		plane := panelPlanes[0]
		if coin {
			plane = panelPlanes[1]
		}
		return plane.positive | plane.negative
	case 1:
		for _, plane := range panelPlanes {
			if whole := plane.positive | plane.negative; arms&whole != 0 {
				return whole
			}
		}
	}
	return arms
}

// linkedPanelLayout is the layout of raised arms. A side plane spans the tile
// edge to edge, a diagonal one corner to corner, so neighbours meet exactly.
func linkedPanelLayout(arms panelArms, tileSize float64) crossLayout {
	var layout crossLayout
	for _, plane := range panelPlanes {
		raised := [2]bool{arms&plane.positive != 0, arms&plane.negative != 0}
		if !raised[0] && !raised[1] {
			continue
		}
		footprint := tileSize
		if plane.diagonal {
			footprint *= math.Sqrt2
		}
		layout.planes[layout.count] = crossPlane{yaw: plane.yaw, footprint: footprint, arms: raised}
		layout.count++
	}
	return layout
}

// crossLayoutOf is the layout of a crossed sprite entry: its linked panels,
// or the static diagonal cross over the art's own footprint.
func (r *Renderer) crossLayoutOf(s UnifiedSpriteRenderData, artFootprint float64) crossLayout {
	if s.panelArms != 0 {
		return linkedPanelLayout(s.panelArms, float64(r.game.config.GetTileSize()))
	}
	return twoPlaneCross(crossDiagonalYawA, crossDiagonalYawB, artFootprint)
}

// crossReachWorld is how far a crossed tile's arms reach from its centre: half
// a panel's longest plane, or half the width a cross draws (crossWidth).
func (r *Renderer) crossReachWorld(td *TransparentSpriteData) float64 {
	tileSize := float64(r.game.config.GetTileSize())
	if td.panelArms != 0 {
		reach := 0.0
		layout := linkedPanelLayout(td.panelArms, tileSize)
		for _, plane := range layout.planes[:layout.count] {
			reach = math.Max(reach, plane.footprint/2)
		}
		return reach
	}
	return r.crossWidth(td.tileType, td.spriteName, tileSize*r.game.renderHelper.envHeightMultiplier(td.tileType, 1.0)) / 2
}

// linkPanels aims every linked-panel tile of the crossed-tile cache at its
// neighbours in the current world. Called whenever that cache is built or
// loses tiles. A lone panel takes its side from the world's layout seed, like
// a sprite variant: it holds while that layout stands.
func (r *Renderer) linkPanels(cache []TransparentSpriteData) {
	current := r.game.GetCurrentWorld()
	tiles := world.GlobalTileManager
	if current == nil || tiles == nil {
		return
	}
	for i := range cache {
		tile := &cache[i]
		tile.panelArms = 0
		if !tiles.IsLinkedPanel(tile.tileType) {
			continue
		}
		tile.panelArms = linkedPanelArms(func(dx, dy int) bool {
			return tiles.IsLinkedPanel(current.GetTileAtGrid(tile.tileX+dx, tile.tileY+dy))
		}, current.EnvironmentCoin(tile.tileX, tile.tileY))
	}
}

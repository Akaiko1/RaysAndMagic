package game

import (
	"math"

	"ugataima/internal/monster"
)

// View preferences affect presentation only. The logical camera FOV continues
// to define the existing combat aim cone.
func (g *MMGame) viewFocal() float64 {
	if g.combatPreferences.WideView {
		return float64(worldViewportBottom(g)) * g.config.Graphics.View.WideFocalRatio
	}
	return float64(g.worldHeight())
}

func (g *MMGame) viewHorizon() float64 {
	if g.combatPreferences.WideView {
		return float64(worldViewportBottom(g)) * g.config.Graphics.View.WideHorizonRatio
	}
	return float64(g.worldHeight()) / 2
}

func (g *MMGame) viewFOV() float64 {
	if !g.combatPreferences.WideView {
		return g.camera.FOV
	}
	return 2 * math.Atan(float64(g.worldWidth())/(2*g.viewFocal()))
}

// Bound the correction by the HUD's covered height, then release it before a
// creature reaches the camera. Never clamp a ground anchor to the HUD edge:
// that would launch a growing sprite up the screen during a crossing.
func (g *MMGame) monsterHUDLift(depth, bottom, size float64) float64 {
	if g.combatPreferences.WideView || depth <= 0 {
		return 0
	}
	viewport := float64(worldViewportBottom(g))
	covered := float64(g.worldHeight()) - viewport
	if covered <= 0 {
		return 0
	}
	v := g.config.Graphics.View
	smooth := func(t float64) float64 {
		t = max(0, min(1, t))
		return t * t * (3 - 2*t)
	}
	distanceFade := smooth((depth/g.config.GetTileSize() - v.LiftFadeTiles) / (v.LiftFullTiles - v.LiftFadeTiles))
	// Small actors fit fully; oversized actors smoothly retain their framing.
	sizeFade := smooth((viewport - size) / covered)
	return min(covered, max(0, bottom-viewport)) * distanceFade * sizeFade
}

// Snapshot visual proportions so an impact retains its anchor after a kill.
type monsterVisualAnchor struct {
	sizeTiles, heightTiles float64
	flying                 bool
}

func visualAnchorFor(mon *monster.Monster3D) monsterVisualAnchor {
	if mon == nil {
		return monsterVisualAnchor{}
	}
	return monsterVisualAnchor{mon.GetSizeGameMultiplier(), mon.VisualHeightTiles(), mon.Flying}
}

func (a monsterVisualAnchor) bottom(g *MMGame, depth, ground, size float64) float64 {
	return a.bottomDuringFall(g, depth, ground, size, 0)
}

// Live actors and corpses share both endpoints. An airborne actor starts at
// its hover/perch anchor and lands where the same grounded body would stand.
func (a monsterVisualAnchor) bottomDuringFall(g *MMGame, depth, ground, size, fall float64) float64 {
	land := ground - g.monsterHUDLift(depth, ground, size)
	air := ground
	if a.flying {
		air = monsterFlyingBottom(g.viewHorizon(), ground, size)
	}
	if a.heightTiles > 0 {
		air = arborealBottom(air, g.viewFocal()*g.config.GetTileSize()/depth, a.heightTiles)
	} else if !a.flying {
		return land
	}
	return air + (land-air)*fall*fall
}

func (a monsterVisualAnchor) centerOffset(g *MMGame, depth float64) float64 {
	if a.sizeTiles <= 0 || depth <= 0 {
		return 0
	}
	size := max(float64(g.config.Graphics.Monster.MinSpriteSize), g.viewFocal()*g.config.GetTileSize()*a.sizeTiles/depth)
	ground := g.renderHelper.calculateFloorScreenYF(depth)
	return a.bottom(g, depth, ground, size) - size/2 - g.viewHorizon()
}

// This is a render-only bend toward the target's visible body. Physical flight,
// range and collision remain in world coordinates.
func (p projectileLaunch) screenOffsetY(g *MMGame, x, y float64) float64 {
	if g.combat == nil {
		return 0
	}
	w, _, start, _ := p.presentationWeights(x, y, g.config.GetTileSize())
	center := func(a monsterVisualAnchor, x, y float64) float64 {
		if a.sizeTiles <= 0 {
			return 0
		}
		b := g.cameraBasis()
		depth := (x-g.camera.X)*b.dirX + (y-g.camera.Y)*b.dirY
		return a.centerOffset(g, depth)
	}
	offset := start * center(p.startAnchor, p.x+p.startX, p.y+p.startY)
	if dx, dy, ok := p.targetOffset(g.combat); ok {
		anchor := p.passedAnchor
		if !p.passed {
			anchor = visualAnchorFor(p.target)
		}
		offset += w * center(anchor, p.aimX+dx, p.aimY+dy)
	}
	return offset
}

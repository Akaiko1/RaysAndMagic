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
	bodyCenter             float64 // visible body centre as a share of frame height; 0 = frame centre
	floor                  bool    // a point on the floor: a shot that sank into the ground
}

// floorAnchor places an impact on the floor at its own depth.
var floorAnchor = monsterVisualAnchor{floor: true}

func visualAnchorFor(mon *monster.Monster3D) monsterVisualAnchor {
	if mon == nil {
		return monsterVisualAnchor{}
	}
	return monsterVisualAnchor{sizeTiles: mon.GetSizeGameMultiplier(), heightTiles: mon.VisualHeightTiles(), flying: mon.Flying, bodyCenter: mon.BodyCenterFrac}
}

// measureMonsterBody records where a drawn monster's visible body sits inside
// its frame. Art planted by its feet with headroom (dragons, rats) sits below
// the frame centre. Measured once, from the base sprite's visible bounds.
func (g *MMGame) measureMonsterBody(mon *monster.Monster3D) {
	if mon.BodyCenterFrac > 0 || g.sprites == nil {
		return
	}
	if b, _, h, ok := g.sprites.SpriteVisibleFrameBounds(mon.GetSpriteType()); ok && h > 0 {
		mon.BodyCenterFrac = float64(b.Min.Y+b.Max.Y) / (2 * float64(h))
	}
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
	if a.floor && depth > 0 {
		return g.renderHelper.calculateFloorScreenYF(depth) - g.viewHorizon()
	}
	if a.sizeTiles <= 0 || depth <= 0 {
		return 0
	}
	size := max(float64(g.config.Graphics.Monster.MinSpriteSize), g.viewFocal()*g.config.GetTileSize()*a.sizeTiles/depth)
	ground := g.renderHelper.calculateFloorScreenYF(depth)
	body := a.bodyCenter
	if body <= 0 {
		body = .5
	}
	return a.bottom(g, depth, ground, size) - size*(1-body) - g.viewHorizon()
}

// eyeHeightTiles is the camera's height above the floor; the floor projects
// this far below the horizon at every depth.
const eyeHeightTiles = .5

// bodyHeight is how high above the floor (world units) the drawn body centre
// stands: centerOffset's body without the HUD lift, which only the screen
// adds. Gameplay uses it to find where a shot's line meets the floor.
func (a monsterVisualAnchor) bodyHeight(tile float64) float64 {
	size, bottom := a.sizeTiles*tile, 0.0
	if a.flying {
		// monsterFlyingBottom: centred on the eye, never below half its height.
		eye := eyeHeightTiles * tile
		bottom = max(eye-size/2, eye/2)
	}
	bottom += max(0, a.heightTiles) * tile
	body := a.bodyCenter
	if body <= 0 {
		body = .5
	}
	return bottom + size*(1-body)
}

// lobHeight is a thrown shot's height above the floor at flight fraction t:
// it leaves the hand at start, crests apex above that line and lands on the floor.
func lobHeight(t, start, apex float64) float64 {
	return start*(1-t) + 4*apex*t*(1-t)
}

// eyeDrop is how far below eye level a shot is drawn, in world units. A lob
// follows its arc; any other shot runs a straight 3D line from the launch (or
// the actor a continuation left) to the target's visible body. Past that spot
// a shot whose target is still ahead flies level at that body's height; one
// whose target stepped aside keeps the line until it meets the floor (where it
// lands). Only a shot through its target (pierce) eases back to eye level.
// Physical flight and collision remain in world XY.
func (p projectileLaunch) eyeDrop(g *MMGame, x, y float64) float64 {
	tile := g.config.GetTileSize()
	if p.lobApex > 0 && p.distance > 0 {
		t := max(0, min(1, ((x-p.x)*p.dx+(y-p.y)*p.dy)/p.distance))
		return eyeHeightTiles*tile - lobHeight(t, lobHandTiles*tile, p.lobApex)
	}
	if g.combat == nil {
		return 0
	}
	w, _, start, _ := p.presentationWeights(x, y, tile)
	travel := (x-p.x)*p.dx + (y-p.y)*p.dy
	if p.aimDistance > 0 {
		// A leg toward a body is one straight line from the body it left.
		start = 1 - max(0, min(1, travel/p.aimDistance))
	}
	// The body centre's drawn offset at its own depth, as a height below the eye.
	drop := func(a monsterVisualAnchor, x, y float64) float64 {
		if a.sizeTiles <= 0 {
			return 0
		}
		// A body at or behind the camera plane keeps its height, measured one
		// tile out, instead of dropping to eye level.
		b := g.cameraBasis()
		depth := max(tile, (x-g.camera.X)*b.dirX+(y-g.camera.Y)*b.dirY)
		return a.centerOffset(g, depth) * depth / g.viewFocal()
	}
	startDrop := drop(p.startAnchor, p.x+p.startX, p.y+p.startY)
	// A target that moved or died after release keeps the body it was aimed at.
	anchor, ax, ay := p.aimAnchor, p.aimX, p.aimY
	if dx, dy, ok := p.targetOffset(g.combat); ok {
		anchor, ax, ay = p.passedAnchor, p.aimX+dx, p.aimY+dy
		if !p.passed {
			// Live proportions, but the body centre measured at release, so a
			// late measurement cannot make the shot hop between two heights.
			anchor = visualAnchorFor(p.target)
			anchor.bodyCenter = p.aimAnchor.bodyCenter
		}
	}
	aimDrop := drop(anchor, ax, ay)
	if p.aimDistance > 0 && !p.passed && travel > p.aimDistance {
		if p.level {
			return aimDrop
		}
		if p.floorDistance > p.aimDistance {
			t := (travel - p.aimDistance) / (p.floorDistance - p.aimDistance)
			return min(eyeHeightTiles*tile, aimDrop+(eyeHeightTiles*tile-aimDrop)*t)
		}
		return aimDrop + (aimDrop-startDrop)*(travel/p.aimDistance-1)
	}
	return start*startDrop + w*aimDrop
}

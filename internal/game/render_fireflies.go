package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

// The original firefly_swarm PNG contains 10 visible warm light components.
// Keep their measured centers as sprite-normalized coordinates, then render
// them procedurally so there is no chroma-keyed billboard for the lights.
var fireflySwarmMotes = [...]struct {
	u float64
	v float64
}{
	{0.3695, 0.3325},
	{0.8069, 0.5954},
	{0.5940, 0.7436},
	{0.6300, 0.1847},
	{0.3689, 0.6053},
	{0.2002, 0.2151},
	{0.7807, 0.3158},
	{0.1364, 0.4615},
	{0.6036, 0.4568},
	{0.2009, 0.7420},
}

func isFireflySwarmTile(tileType world.TileType3D) bool {
	if world.GlobalTileManager == nil {
		return false
	}
	tile := world.GlobalTileManager.GetTileData(tileType)
	return tile != nil && tile.ProceduralEffect == config.TileEffectFireflySwarm
}

func fireflySwarmSeed(tileX, tileY int) int {
	return tileX*73 + tileY*193
}

// The floor light follows the average of the same independent insect flashes.
func fireflySwarmFlicker(seed int, seconds float64) float64 {
	sum := 0.0
	for i := range fireflySwarmMotes {
		sum += fireflyPulse(seconds, auraHash(seed, i, 41, 0)*2*math.Pi)
	}
	flash := (sum/float64(len(fireflySwarmMotes)) - fireflyPulseFloor) / (1 - fireflyPulseFloor)
	// Preserve the original floor-light envelope while synchronizing its pulse.
	return .4 + .85*flash
}

// World-space paths keep the swarm volumetric when the party turns or strafes.
func (r *Renderer) drawFireflySwarmEffect(screen *ebiten.Image, s UnifiedSpriteRenderData, _ float64) {
	ts := r.game.config.GetTileSize()
	wx, wy := TileCenterFromTile(s.tileX, s.tileY, ts)
	seed := fireflySwarmSeed(s.tileX, s.tileY)
	seconds := r.weaponMaterialClock()
	for i, mote := range fireflySwarmMotes {
		phase := auraHash(seed, i, 41, 0) * 2 * math.Pi
		x := wx + ts*((mote.u-.5)*.8+.055*math.Sin(seconds*.7+phase)+.025*math.Sin(seconds*1.3+phase*2))
		y := wy + ts*((auraHash(seed, i, 43, 0)-.5)*.65+.055*math.Cos(seconds*.6+phase))
		height := ts*(1-mote.v)*.75 + ts*.035*math.Sin(seconds*.9+phase)
		sx, depth, ok := r.game.renderHelper.projectToScreenXF(x, y)
		if !ok || depth < 1 || sx < 0 || sx >= float64(len(r.game.depthBuffer)) || depth >= r.game.depthBuffer[int(sx)] {
			continue
		}
		sy := r.game.viewHorizon() + (.5*ts-height)*r.game.viewFocal()/depth
		radius := math.Max(3, ts*.065*r.game.viewFocal()/depth)
		alpha := fireflyPulse(seconds, phase)
		r.drawFirefly(screen, sx, sy, radius, alpha, seconds*53+phase, [3]int{255, 218, 80}, [3]int{255, 252, 170})
	}
}

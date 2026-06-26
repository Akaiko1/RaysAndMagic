package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
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
	return world.GlobalTileManager != nil && world.GlobalTileManager.GetTileKey(tileType) == "firefly_swarm"
}

func fireflySwarmSeed(tileX, tileY int) int {
	return tileX*73 + tileY*193
}

// fireflySwarmFlicker is intentionally slower than torch flicker but has enough
// contrast to read in the forest shade.
func fireflySwarmFlicker(seed int, frameCount int64) float64 {
	phase := auraHash(seed, 0, 31, 0) * 2 * math.Pi
	f := 0.72 + 0.36*math.Sin(float64(frameCount)*0.075+phase) +
		0.17*math.Sin(float64(frameCount)*0.031+phase*2.7)
	if f < 0.40 {
		return 0.40
	}
	if f > 1.25 {
		return 1.25
	}
	return f
}

func (r *Renderer) drawFireflySwarmEffect(screen *ebiten.Image, s UnifiedSpriteRenderData, distance float64) {
	if s.spriteSize <= 0 {
		return
	}

	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, float64(r.game.config.GetTileSize()))
	brightness := r.calculateBrightnessWithTorchLight(worldX, worldY, distance)
	if brightness < 0.2 {
		brightness = 0.2
	}

	seed := fireflySwarmSeed(s.tileX, s.tileY)
	frame := float64(r.game.frameCount)
	drawLeft := float64(s.screenX - s.spriteSize/2)
	drawTop := float64(s.screenY)
	size := float64(s.spriteSize)
	glowBase := math.Max(3, size*0.105)
	coreBase := math.Max(1.25, size*0.018)
	globalFlicker := fireflySwarmFlicker(seed, r.game.frameCount)

	for i, mote := range fireflySwarmMotes {
		localPhase := auraHash(seed, i, 41, 0) * 2 * math.Pi
		slowPulse := 0.66 + 0.34*math.Sin(frame*0.070+localPhase) +
			0.16*math.Sin(frame*0.027+localPhase*1.9)
		if slowPulse < 0.28 {
			slowPulse = 0.28
		}
		if slowPulse > 1.15 {
			slowPulse = 1.15
		}

		driftX := math.Sin(frame*0.012+localPhase*1.3) * size * 0.018
		driftY := math.Sin(frame*0.010+localPhase*0.7) * size * 0.014
		x := drawLeft + mote.u*size + driftX
		y := drawTop + mote.v*size + driftY
		alpha := brightness * globalFlicker * slowPulse
		if alpha > 1 {
			alpha = 1
		}
		scale := 0.82 + 0.36*auraHash(seed, i, 42, 0)

		r.drawGlowSprite(screen, x, y, glowBase*scale, [3]int{255, 218, 80}, 0.50*alpha, additiveGlowBlend)
		r.drawGlowRect(screen, x, y, coreBase*scale, [3]int{255, 252, 170}, 0.95*alpha, additiveGlowBlend)
	}
}

package game

import "math"

const (
	sealedAuraRadiusTiles = 13.0
	sealedAuraSegments    = 32
	sealedAuraRingTiles   = 0.9
	sealedAuraBaseAlpha   = 0.6
	sealedAuraRiseFrac    = 0.9
)

var sealedAuraColor = [3]int{172, 176, 190} // cool spectral grey (sealed/dormant boss)
var wardAuraColor = [3]int{214, 168, 72}    // amber ritual glow (idol-warded boss + its idols)

// Protective auroras follow the boss's actual position. Segments join the same
// painter pass as the boss, so its silhouette hides the far half of the ring.
func (r *Renderer) collectSealedBossAuroras() {
	if r.game.world == nil {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	maxDepth := sealedAuraRadiusTiles * ts
	ring := sealedAuraRingTiles * ts
	_, density, _ := r.auraEdgeParams()
	for _, m := range r.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		var auraColor [3]int
		switch {
		case m.BossDormant:
			auraColor = sealedAuraColor
		case m.BossWarded || m.WarlordIdol:
			auraColor = wardAuraColor
		default:
			continue
		}
		_, depth, ok := r.game.renderHelper.cameraSpaceXY(m.X, m.Y)
		if !ok || depth+ring <= auraMinDepth || depth-ring >= maxDepth {
			continue
		}
		x0, y0 := m.X+ring, m.Y
		rays := math.Round(2 * math.Pi * sealedAuraRingTiles * density)
		u0 := 0.0
		for k := 1; k <= sealedAuraSegments; k++ {
			angle := 2 * math.Pi * float64(k) / sealedAuraSegments
			x1, y1 := m.X+math.Cos(angle)*ring, m.Y+math.Sin(angle)*ring
			r.collectAuraSegmentUV(x0, y0, x1, y1, u0, angle, ts, sealedAuraRiseFrac, -rays,
				sealedAuraBaseAlpha, maxDepth, auraColor)
			x0, y0, u0 = x1, y1, angle
		}
	}
}

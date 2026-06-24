package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Sealed-boss aura: a ring of rising grey "smoke" around a dormant boss, telling
// the player it's sealed — inert and invulnerable — until its quest unseals it.
// Same depth-tested rising-glow plumbing as the impassable-tile aura
// (emitBubbleColumn), just sampled on a ring at the boss's feet instead of along
// tile edges, and in a cool spectral grey rather than the tile's own colour.
const (
	sealedAuraRadiusTiles = 13.0 // far clip + distance-fade reference
	sealedAuraColumns     = 14   // smoke columns around the ring
	sealedAuraRingTiles   = 0.9  // ring radius from the boss centre (tiles)
	sealedAuraBaseAlpha   = 0.6
	sealedAuraRiseFrac    = 0.9
	sealedAuraPeriodTick  = 110.0
)

var sealedAuraColor = [3]int{172, 176, 190} // cool spectral grey (sealed/dormant boss)
var wardAuraColor = [3]int{214, 168, 72}    // amber ritual glow (idol-warded boss + its idols)

// drawSealedBossAura wreathes invulnerable bosses in rising "smoke": a dormant
// boss in cool grey (sealed until its quest), and an idol-warded boss plus each
// of its live idols in amber — so the protective link reads at a glance and the
// glow clears the instant the idols fall. Drawn after walls/sprites so the wall
// depth buffer occludes it correctly.
func (r *Renderer) drawSealedBossAura(screen *ebiten.Image) {
	if r.game.world == nil {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	maxDepth := sealedAuraRadiusTiles * ts
	ring := sealedAuraRingTiles * ts
	for _, m := range r.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		auraColor := sealedAuraColor
		switch {
		case m.BossDormant:
			auraColor = sealedAuraColor
		case m.BossWarded || m.WarlordIdol:
			auraColor = wardAuraColor
		default:
			continue
		}
		tx, ty := int(m.X/ts), int(m.Y/ts)
		// Self-cull plane: columns behind the boss billboard are hidden by it.
		_, centerDepth, centerOK := r.game.renderHelper.projectToScreenX(m.X, m.Y)
		for k := 0; k < sealedAuraColumns; k++ {
			ang := 2 * math.Pi * float64(k) / float64(sealedAuraColumns)
			wx := m.X + math.Cos(ang)*ring
			wy := m.Y + math.Sin(ang)*ring
			colBright := auraColBrightMin + (1.0-auraColBrightMin)*auraHash(tx, ty, k+700, 0)
			r.emitBubbleColumn(screen, bubbleColumnFx{
				wx: wx, wy: wy,
				hx: tx, hy: ty, salt: k + 700, hi: k,
				maxDepth:     maxDepth,
				riseFraction: sealedAuraRiseFrac,
				baseAlpha:    sealedAuraBaseAlpha,
				colBright:    colBright,
				perColumn:    3,
				periodTick:   sealedAuraPeriodTick,
				jitterMin:    auraSpeedJitterMin,
				jitterSpan:   (1.0 - auraSpeedJitterMin) * 2,
				sizeFloor:    2.0,
				sizeCoef:     0.06,
				wobbleCoef:   0.7,
				color:        auraColor,
				centerDepth:  centerDepth,
				hasCenter:    centerOK,
			})
		}
	}
}

package game

import (
	"math"
	"strings"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// MeleeFxLingerFrames is the minimum lifetime of a melee swing's visual so the
// shaped trail (росчерк) fades slowly after the fast swing completes.
const MeleeFxLingerFrames = 22

// meleeSweepFrac: the swing itself completes in this fraction of the lifetime;
// the rest is the trail lingering/fading.
const meleeSweepFrac = 0.35

// meleeAnchorYFrac places the swing's vertical anchor lower on screen (1.0 =
// bottom), so it reads as the party's own weapon rather than floating mid-view.
const meleeAnchorYFrac = 0.68

// meleeSizeScale shrinks every swing dimension; 1/√2 ≈ 0.707 → exactly half the
// occupied area.
const meleeSizeScale = 0.7071

// meleeFxKind maps a weapon category to its swing flavor so every weapon type
// gets a distinct effect (sword crescent, axe chop, mace smash, dagger stab,
// spear lunge).
func meleeFxKind(def *config.WeaponDefinitionConfig) string {
	if def == nil {
		return "slash"
	}
	switch strings.ToLower(def.Category) {
	case "axe":
		return "chop"
	case "mace", "hammer", "club", "flail":
		return "smash"
	case "dagger", "knife":
		return "stab"
	case "spear", "rapier", "halberd", "pike":
		return "lunge"
	default: // sword and anything else
		return "slash"
	}
}

// seedFromID hashes a slash ID to a stable int for the deterministic particle
// hash (no per-frame randomness).
func seedFromID(s string) int {
	h := 0
	for i := 0; i < len(s); i++ {
		h = h*31 + int(s[i])
	}
	if h < 0 {
		h = -h
	}
	return h
}

// drawMeleeParticles renders a melee swing as a shaped, slowly-fading trail
// (росчерк) plus particle sparks, in screen space around the first-person
// centre. The trail is a smooth ribbon of overlapping soft sprites; sparks fly
// off the leading edge during the fast sweep. Geometry differs per weapon kind.
func (r *Renderer) drawMeleeParticles(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	if s.MaxFrames <= 0 {
		return
	}
	progress := float64(s.AnimationFrame) / float64(s.MaxFrames)
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}
	fade := 1.0 - progress // slow fade over the (lingering) lifetime
	if fade <= 0 {
		return
	}
	sweepT := progress / meleeSweepFrac // 0→1 over the fast swing, then clamps
	if sweepT > 1 {
		sweepT = 1
	}
	lead := 1.0 - (1.0-sweepT)*(1.0-sweepT) // easeOut: how far the blade has swept

	seed := seedFromID(s.ID)
	fc := int(r.game.frameCount)
	col := s.Color
	if col == [3]int{0, 0, 0} {
		col = [3]int{255, 255, 255}
	}
	edge := [3]int{255, 255, 255}

	// All dimensions derive from h (= screenH × scale) so the whole swing shrinks
	// uniformly; positions anchor around the (already-lowered) cx, cy.
	h := screenH * meleeSizeScale

	// --- Thrust kinds: dagger (short/quick) and spear (long/lean) ---
	if s.Kind == "stab" || s.Kind == "lunge" {
		reach := h * 0.18
		thick := h * 0.045
		if s.Kind == "lunge" {
			reach = h * 0.34
			thick = h * 0.03
		}
		baseX, baseY := cx, cy+reach*0.5
		dirX, dirY := 0.0, -1.0 // jab forward/up the view
		tipLen := reach * lead
		tipX, tipY := baseX+dirX*tipLen, baseY+dirY*tipLen

		// Tapered ribbon (thick at base → thin at tip), lingering.
		const n = 16
		for k := 0; k < n; k++ {
			t := float64(k) / float64(n-1)
			px := baseX + dirX*tipLen*t
			py := baseY + dirY*tipLen*t
			w := thick * (1.0 - 0.8*t)
			a := fade * (0.45 + 0.55*t)
			r.drawGlowSprite(screen, px, py, math.Max(2, w), mixColor(col, edge, t), a, additiveGlowBlend)
		}
		// Bright tip flash + sparks while extending.
		r.drawGlowSprite(screen, tipX, tipY, thick*1.7, edge, fade*0.9, additiveGlowBlend)
		if sweepT < 1 {
			for k := 0; k < 10; k++ {
				ang := auraHash(seed, k, 1, fc) * 2 * math.Pi
				rr := auraHash(seed, k, 2, fc) * thick * 1.6
				r.drawGlowRect(screen, tipX+math.Cos(ang)*rr, tipY+math.Sin(ang)*rr, math.Max(2, thick*0.4), edge, fade*0.85, additiveGlowBlend)
			}
		}
		return
	}

	// --- Arc kinds: sword crescent, axe chop, mace smash ---
	var pivotX, pivotY, R, thetaStart, thetaEnd, thick float64
	switch s.Kind {
	case "chop": // heavy diagonal downward chop (upper-right → lower-left)
		reach := h * 0.24
		pivotX, pivotY = cx-reach*0.15, cy-reach*0.1
		R = reach * 1.25
		thetaStart, thetaEnd = -1.35, 0.85
		thick = h * 0.075
	case "smash": // short, near-vertical overhead bash with an impact flash
		reach := h * 0.22
		pivotX, pivotY = cx, cy-reach*0.2
		R = reach * 1.1
		thetaStart, thetaEnd = -math.Pi/2-0.3, math.Pi/2
		thick = h * 0.08
	default: // "slash": flat, wide crescent across the top (shallow = less curved)
		reach := h * 0.22
		pivotX, pivotY = cx, cy+reach*0.95
		R = reach * 1.7
		thetaStart, thetaEnd = -math.Pi/2-0.6, -math.Pi/2+0.6
		thick = h * 0.06
	}

	curTheta := thetaStart + (thetaEnd-thetaStart)*lead

	// Shaped ribbon along the swept arc [start … current]. Two offset rows make a
	// fuller, juicier blade-streak than a single line. Thick in the middle, thin
	// at the ends; brighter at the leading edge; fades slowly via `fade`.
	const n = 26
	for k := 0; k < n; k++ {
		tf := float64(k) / float64(n-1) // 0 start → 1 leading edge
		theta := thetaStart + (curTheta-thetaStart)*tf
		w := thick * (0.3 + 0.7*math.Sin(math.Pi*tf))
		a := fade * (0.4 + 0.6*tf)
		cosT, sinT := math.Cos(theta), math.Sin(theta)
		c := mixColor(col, edge, tf)
		for _, off := range [2]float64{-0.25, 0.25} {
			rr := R + off*w
			r.drawGlowSprite(screen, pivotX+cosT*rr, pivotY+sinT*rr, math.Max(2, w*0.7), c, a, additiveGlowBlend)
		}
	}

	// Sparks off the leading edge during the sweep.
	tipX := pivotX + math.Cos(curTheta)*R
	tipY := pivotY + math.Sin(curTheta)*R
	if sweepT < 1 {
		for k := 0; k < 14; k++ {
			ang := auraHash(seed, k, 3, fc) * 2 * math.Pi
			rr := auraHash(seed, k, 4, fc) * thick * 1.7
			r.drawGlowRect(screen, tipX+math.Cos(ang)*rr, tipY+math.Sin(ang)*rr, math.Max(2, thick*0.3), edge, fade*0.9, additiveGlowBlend)
		}
	}

	// Mace: heavy impact flash where the swing lands.
	if s.Kind == "smash" && sweepT >= 1 {
		r.drawGlowSprite(screen, tipX, tipY, thick*2.4*fade+thick, edge, fade*0.85, additiveGlowBlend)
	}
}

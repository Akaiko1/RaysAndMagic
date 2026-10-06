package game

import (
	"math"
	"strings"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// MeleeFxLingerFrames is the minimum lifetime of a melee swing's visual so the
// shaped trail (flourish) fades slowly after the fast swing completes.
const MeleeFxLingerFrames = 22 // ~0.18s at 120 TPS

// meleeSweepFrac: the swing itself completes in this fraction of the lifetime;
// the rest is the trail lingering/fading.
const meleeSweepFrac = 0.35

// meleeAnchorYFrac places the swing's vertical anchor lower on screen (1.0 =
// bottom), so it reads as the party's own weapon rather than floating mid-view.
const meleeAnchorYFrac = 0.68

// meleeSizeScale shrinks every swing dimension; 1/sqrt2 ~ 0.707 -> exactly half the
// occupied area.
const meleeSizeScale = 0.7071

// Category controls the weight of every swing ribbon, including bespoke FX.
// Limits are half-widths in viewport pixels; material and motifs stay authored.
type meleeTrailProfile struct {
	widthScale, widthLimit float64
	tail, tip              float64
	angular                bool
}

func meleeTrailShape(kind string, screenH float64, crit bool) meleeTrailProfile {
	if crit {
		screenH *= 1.2
	}
	switch kind {
	case "stab":
		return meleeTrailProfile{.42, screenH * .009, .24, .32, false}
	case "smash":
		return meleeTrailProfile{.95, screenH * .055, .14, .12, false}
	case "chop":
		return meleeTrailProfile{.85, screenH * .038, .26, .09, true}
	case "lunge":
		return meleeTrailProfile{.65, screenH * .016, .22, .24, false}
	default:
		return meleeTrailProfile{.52, screenH * .015, .18, .18, false}
	}
}

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
	case "martial_arts":
		return "punch"
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

// meleeFxTiming derives the shared swing-animation parameters: total progress,
// the lingering fade, the fast-sweep fraction and the eased blade position.
func meleeFxTiming(s SlashEffect) (progress, fade, sweepT, lead float64) {
	progress = float64(s.AnimationFrame) / float64(s.MaxFrames)
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}
	// Stretch the visual tail without slowing the initial weapon motion.
	if s.SweepFrames > 0 && s.MaxFrames > s.SweepFrames {
		if s.AnimationFrame <= s.SweepFrames {
			progress = meleeSweepFrac * math.Max(0, float64(s.AnimationFrame)) / float64(s.SweepFrames)
		} else {
			progress = math.Min(1, meleeSweepFrac+(1-meleeSweepFrac)*float64(s.AnimationFrame-s.SweepFrames)/float64(s.MaxFrames-s.SweepFrames))
		}
	}
	fade = 1.0 - progress
	sweepT = progress / meleeSweepFrac
	if sweepT > 1 {
		sweepT = 1
	}
	lead = 1.0 - (1.0-sweepT)*(1.0-sweepT)
	return
}

// drawMeleeParticles renders a melee swing as a shaped, slowly-fading trail
// (flourish) plus particle sparks, in screen space around the first-person
// centre. Material ribbons and shaped weapon heads retain each category's
// silhouette; impact shards have stable ballistic paths.
func (r *Renderer) drawMeleeParticles(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	if s.MaxFrames <= 0 {
		return
	}
	progress, _, _, _ := meleeFxTiming(s)
	previous := r.weaponMaterialState
	r.weaponMaterialState = weaponMaterialState{
		weaponKey: s.WeaponKey, material: weaponMaterial(s.Style), phase: progress * 6, seed: seedFromID(s.ID),
		trail: meleeTrailShape(s.Kind, screenH, s.Crit),
	}
	defer func() { r.weaponMaterialState = previous }()
	if draw, ok := meleeFxStyleDraw[s.Style]; ok {
		draw(r, screen, s, cx, cy, screenH)
		r.drawSignatureSilhouette(screen, s, cx, cy, screenH)
		return
	}
	if s.Kind == "punch" {
		r.drawIdentityStrike(screen, s, cx, cy, screenH, "", "punch")
		return
	}
	_, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	col := s.Color
	if col == [3]int{} {
		col = [3]int{210, 225, 240}
	}
	edge := [3]int{240, 247, 255}
	h := screenH * meleeSizeScale
	if s.Crit {
		h *= 1.25
		edge = [3]int{255, 232, 170}
	}
	var path func(float64) (float64, float64)
	var thick, length float64
	if s.Kind == "stab" || s.Kind == "lunge" {
		reach := h * .20
		thick = h * .028
		if s.Kind == "lunge" {
			reach = h * .38
			thick = h * .022
		}
		path = func(t float64) (float64, float64) { return cx + h*.05*(1-t), cy + reach*.5 - reach*t }
		length = reach
	} else {
		reach := h * .22
		px, py, R, start, end := cx, cy+reach*.95, reach*1.7, -math.Pi/2-.6, -math.Pi/2+.6
		thick = h * .047
		switch s.Kind {
		case "chop":
			px, py, R, start, end = cx-reach*.15, cy-reach*.1, reach*1.25, -1.35, .85
			thick = h * .065
		case "smash":
			px, py, R, start, end = cx, cy-reach*.2, reach*1.1, -math.Pi/2-.3, math.Pi/2
			thick = h * .07
		}
		path = func(t float64) (float64, float64) {
			a := start + (end-start)*t
			return px + math.Cos(a)*R, py + math.Sin(a)*R
		}
		length = R * math.Abs(end-start)
	}
	r.drawDissolveStroke(screen, dissolveStroke{
		path: path, width: func(t float64) float64 { return thick * (.25 + .75*math.Sin(math.Pi*t)) },
		color: func(t float64) [3]int { return mixColor(col, edge, t*.4) }, alpha: func(t float64) float64 { return .6 + .3*t },
		length: length, seed: seed, salt: 10, blend: additiveGlowBlend,
	}, lead, progress)
	tipX, tipY := path(lead)
	angle := tangentAt(path, lead)
	if s.Kind == "slash" || s.Kind == "chop" {
		angle -= math.Pi / 2
	}
	if progress < .65 {
		headSize := h * .065
		if s.Kind == "stab" {
			headSize *= .62
		}
		r.drawWeaponHead(screen, s.Kind, tipX, tipY, angle, headSize, col, fade)
	}
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		count := 9
		shardScale := 1.0
		if s.Kind == "stab" {
			count, shardScale = 5, .48
		}
		if s.Kind == "smash" || s.Kind == "chop" {
			count = 15
		}
		if s.Crit {
			count += 5
		}
		for k := 0; k < count; k++ {
			a := -math.Pi*.5 + (auraHash(seed, k, 31, 0)-.5)*4.5
			speed := h * (.12 + .18*auraHash(seed, k, 32, 0))
			x, y := tipX+math.Cos(a)*speed*u, tipY+math.Sin(a)*speed*u+h*.18*u*u
			r.drawWeaponShard(screen, x, y, h*shardScale*(.01+.009*auraHash(seed, k, 33, 0)), mixColor(col, edge, auraHash(seed, k, 34, 0)), .8*math.Sqrt(fade), u, seed+k*37, false)
		}
		if s.Kind == "smash" {
			r.arenaImpactCloud(screen, tipX, tipY, u, h*.25, .45, col, fade)
		}
	}
}

package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// A readable weapon leads each authored strike; the material wake stays behind
// the blade instead of substituting for it. Paired weapons have staggered poses.
func (r *Renderer) drawIdentityStrike(dst *ebiten.Image, s SlashEffect, cx, cy, screenH float64, style, kind string) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h := screenH * meleeSizeScale
	if s.Crit {
		h *= 1.2
	}
	col := s.Color
	if col == [3]int{} {
		col = [3]int{185, 211, 228}
	}
	count := 1
	if style == "kage_kunai" || style == "agility_katar" || style == "arena_cesti" || kind == "punch" {
		count = 2
	}
	for i := 0; i < count; i++ {
		lag := float64(i) * .11
		local := math.Max(0, (p-lag)/(1-lag))
		if local <= 0 {
			continue
		}
		ld := math.Min(1, local/meleeSweepFrac)
		ld = 1 - (1-ld)*(1-ld)
		if count == 1 {
			ld = lead
		}
		side := 1.0
		if i == 1 {
			side = -1
		}
		path := func(t float64) (float64, float64) {
			if kind == "stab" || kind == "punch" || kind == "lunge" {
				reach := .34
				if kind == "lunge" {
					reach = .46
				}
				lateral := .11 - .13*t
				if count > 1 {
					lateral = .17 - .055*t
				}
				return cx + side*h*lateral, cy + h*(.19-reach*t)
			}

			if kind == "smash" {
				return cx + h*(.16-.10*t), cy + h*(-.32+.36*t*t)
			}
			if kind == "chop" {
				return cx + h*(.30-.51*t), cy + h*(-.26+.40*t)
			}
			return cx + h*(-.23+.46*t), cy + h*(.04-.16*math.Sin(t*math.Pi))
		}
		if style == "war_fan" {
			path = func(t float64) (float64, float64) {
				a := -2.6 + t*1.9
				return cx + math.Cos(a)*h*.22, cy + h*.18 + math.Sin(a)*h*.24
			}
		}
		width := h * .012
		if kind == "lunge" {
			width = h * .009
		}
		if kind == "punch" {
			width = h * .021
		}
		r.drawDissolveStroke(dst, dissolveStroke{path: path, width: func(t float64) float64 { return width * math.Sin(math.Pi*t) }, color: func(float64) [3]int { return col }, alpha: func(float64) float64 { return .36 }, length: h * .5, seed: seedFromID(s.ID), salt: 811 + i, blend: additiveGlowBlend}, ld, local)
		x, y := path(ld)
		angle := tangentAt(path, ld)
		if kind == "slash" || kind == "chop" {
			angle -= math.Pi / 2
		}
		bodyFade := 1 - math.Min(1, math.Max(0, (local-.40)/.24))
		size := h * .065
		if kind == "stab" {
			size = h * .070
		}
		if kind == "lunge" {
			size = h * .067
		}
		if kind == "punch" {
			size = h * (.045 + .035*ld)
		}
		if style == "war_fan" {
			size = h * .062
			angle += .5
		}
		r.weaponMaterialState.hand = i
		r.drawWeaponSilhouette(dst, kind, style, x, y, angle, size, col, bodyFade)
		if local > .34 {
			u := (local - .34) / .66
			for k := 0; k < 4; k++ {
				seed := seedFromID(s.ID) + i*53 + k*19
				a := -math.Pi/2 + (auraHash(seed, k, 802, 0)-.5)*2.8
				d := h * (.06 + .05*auraHash(seed, k, 803, 0)) * u
				r.drawWeaponShard(dst, x+math.Cos(a)*d, y+math.Sin(a)*d+h*.04*u*u, h*.004, col, fade*.65, u, seed, false)
			}
		}
	}
}

func (r *Renderer) drawMeleeFxWarFan(dst *ebiten.Image, s SlashEffect, x, y, h float64) {
	r.drawIdentityStrike(dst, s, x, y, h, "war_fan", "slash")
}
func (r *Renderer) drawMeleeFxKatana(dst *ebiten.Image, s SlashEffect, x, y, h float64) {
	r.drawIdentityStrike(dst, s, x, y, h, "katana", "slash")
}

type weaponStrokePose struct {
	x, y, angle float64
	ready       bool
}

// Bespoke effects also carry solid weapon silhouettes. Their existing secondary
// choreography remains visible behind the moving weapon.
func (r *Renderer) drawSignatureSilhouette(dst *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	switch s.Style {
	case "agility_katar", "kage_kunai", "arena_gladius", "arena_hasta", "arena_trident", "arena_cesti", "arena_parry", "arena_morningstar", "clock_minute", "war_fan", "katana", "solstice_anchor", "solstice_thermal", "idol_breaker", "dragon_jaws":
		return
	}
	p, fade, _, _ := meleeFxTiming(s)
	if p > .64 {
		return
	}
	h := screenH * meleeSizeScale
	if s.Crit {
		h *= 1.2
	}
	kind := s.Kind
	style := s.Style
	if kind == "" {
		kind = "slash"
	}
	pose := r.weaponMaterialState.pose
	if !pose.ready {
		return
	}
	x, y, angle := pose.x, pose.y, pose.angle
	if kind == "slash" || kind == "chop" || style == "naginata" {
		angle -= math.Pi / 2
	}
	if style == "arena_labrys" {
		// The primary crescent turns counterclockwise: keep the head outside
		// the arc and the haft pointing back toward its pivot.
		angle += math.Pi
	}
	col := s.Color
	if col == [3]int{} {
		col = [3]int{190, 207, 216}
	}
	r.drawWeaponSilhouette(dst, kind, style, x, y, angle, h*.062, col, math.Min(fade*1.5, (.64-p)/.15))
}

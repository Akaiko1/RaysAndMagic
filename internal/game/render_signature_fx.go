package game

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"ugataima/internal/config"
)

var flaskProjectileFxStyles = map[string]func(*Renderer, *ebiten.Image, flaskRenderPose){
	"solstice_everflow_retort": (*Renderer).drawEverflowRetort,
}

func validateFlaskFxStyles() {
	if config.GlobalItems == nil {
		return
	}
	for key, d := range config.GlobalItems.Items {
		if d.Flask != nil && d.Flask.ProjectileFx != "" && flaskProjectileFxStyles[d.Flask.ProjectileFx] == nil {
			panic(fmt.Sprintf("item %q: unknown flask projectile_fx %q", key, d.Flask.ProjectileFx))
		}
	}
}

func (r *Renderer) prepareRelicMaterial() bool {
	if r.relicMaterialShader == nil {
		if compileShader(&r.relicMaterialShader, "boss_relic.kage") != nil {
			return false
		}
		r.relicMaterialOpts.Uniforms = map[string]interface{}{"Time": r.relicMaterialTime[:]}
	}
	r.relicMaterialTime[0] = float32(r.weaponMaterialClock())
	r.relicMaterialOpts.Blend = ebiten.BlendSourceOver
	return true
}

// Staves retain the shared charged core. Three uneven solar prominences echo
// the icon's ribs as light around the shot, at the same scale as other staves.
func (r *Renderer) drawSunCrucibleCorona(dst *ebiten.Image, x, y, size, dx, dy, alpha float64, seed int) {
	phase := r.weaponMaterialClock() * 2.1
	for i := 0; i < 3; i++ {
		start := float64(i)*2*math.Pi/3 + phase*.35
		r.drawDissolveStroke(dst, dissolveStroke{
			path: func(t float64) (float64, float64) {
				a := start + t*1.35
				radius := size * (.68 + .30*math.Sin(t*math.Pi) + .08*math.Sin(phase+float64(i)*2))
				return x + math.Cos(a)*radius - dx*size*t*.55, y + math.Sin(a)*radius - dy*size*t*.55
			},
			width:  func(t float64) float64 { return size * (.055 + .11*math.Sin(t*math.Pi)) },
			color:  func(t float64) [3]int { return mixColor([3]int{255, 121, 28}, [3]int{255, 223, 127}, 1-t) },
			alpha:  func(t float64) float64 { return alpha * .65 * math.Sin(t*math.Pi) },
			length: size * 1.7, seed: seed, salt: 850 + i, blend: additiveGlowBlend,
		}, 1, -1-phase)
	}
}

func (r *Renderer) drawSunCrucibleCoronaHeadOn(dst *ebiten.Image, x, y, size, alpha float64, seed int) {
	r.drawSunCrucibleCorona(dst, x, y, size, 0, 0, alpha, seed)
}

// A thrown blade follows the same hand convergence and camera-relative flight
// axis as directional spell volumes. The caller supplies the continuous return
// turn; the complete centered model rotates instead of swapping end-face sprites.
func (r *Renderer) drawReturningGaleProjectile(dst *ebiten.Image, x, y, size float64, view projectileView, boost, convergence float64, seed int) {
	if r.ensureWeaponMaterialShaders() != nil {
		return
	}
	axis := view.spellProjection(convergence).axis
	// A modest pitched attitude reveals the blade and guard during axial flight;
	// perspective and the turning flight axis still determine its silhouette.
	axis[1] -= .34 * math.Abs(view.depth)
	length := math.Sqrt(axis[0]*axis[0] + axis[1]*axis[1] + axis[2]*axis[2])
	if length < .001 {
		return
	}
	for i := range axis {
		axis[i] /= length
	}
	side := [3]float64{axis[2], 0, -axis[0]}
	across := math.Hypot(side[0], side[2])
	if across < .001 {
		side = [3]float64{1, 0, 0}
		across = 1
	}
	for i := range side {
		side[i] /= across
	}
	up := [3]float64{axis[1]*side[2] - axis[2]*side[1], axis[2]*side[0] - axis[0]*side[2], axis[0]*side[1] - axis[1]*side[0]}
	bank := .22 + .60*view.faceWeight()
	cr, sr := math.Cos(bank), math.Sin(bank)
	transform := volumeTransform{uvY: 1}
	for i := range axis {
		transform.rotation[i] = [3]float64{axis[i], up[i]*cr + side[i]*sr, side[i]*cr - up[i]*sr}
	}
	scale := size * math.Sqrt(boost) * .92
	r.drawCameraVolume(dst, x, y, scale, 1, transform, r.weaponBodyShader, &r.weaponMaterialOpts, relicMeshes.get(relicReturningGale, buildRelicMesh), [3]int{255, 255, 255}, 0, false)
}

func (r *Renderer) drawRelicFlask(dst *ebiten.Image, pose flaskRenderPose, style string) bool {
	draw := flaskProjectileFxStyles[style]
	if draw == nil {
		return false
	}
	draw(r, dst, pose)
	return true
}

func (r *Renderer) drawEverflowRetort(dst *ebiten.Image, pose flaskRenderPose) {
	if !r.prepareRelicMaterial() {
		return
	}
	ca, sa := math.Cos(pose.angle), math.Sin(pose.angle)
	roll := pose.angle * .18
	cp, sp := math.Cos(roll), math.Sin(roll)
	transform := volumeTransform{rotation: [3][3]float64{{ca, -sa * cp, sa * sp}, {sa, ca * cp, -ca * sp}, {0, sp, cp}}, uvY: 1}
	r.drawCameraVolume(dst, pose.x, pose.y, pose.size*.55, 1, transform, r.relicMaterialShader, &r.relicMaterialOpts, relicMeshes.get(relicEverflowRetort, buildRelicMesh), [3]int{255, 255, 255}, 0, false)
}

func (r *Renderer) drawMeleeFxStoneScript(dst *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, _ := arenaFxScale(s, screenH)
	path := func(t float64) (float64, float64) {
		a := -2.47 + 1.80*t
		return cx + math.Cos(a)*h*.31, cy + h*.24 + math.Sin(a)*h*.36
	}
	r.drawDissolveStroke(dst, dissolveStroke{path: path, width: func(t float64) float64 { return h * (.012 + .012*math.Sin(t*math.Pi)) }, color: func(t float64) [3]int { return mixColor([3]int{57, 179, 129}, [3]int{237, 182, 73}, .25*t) }, alpha: func(t float64) float64 { return .7 * math.Sin(t*math.Pi) }, length: h * .60, seed: seedFromID(s.ID), salt: 861, blend: ebiten.BlendSourceOver}, lead, p)
	if p > .25 {
		age := (p - .25) / .75
		x, y := path(1)
		for i := 0; i < 4; i++ {
			a := -1.8 + float64(i)*.43
			r.drawWeaponShard(dst, x+math.Cos(a)*h*.11*age, y+math.Sin(a)*h*.11*age+h*.04*age*age, h*.011, [3]int{84, 191, 136}, fade, age, seedFromID(s.ID)+i*31, false)
		}
	}
}

package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// Model numbers select procedural shapes, never item icon textures.
var weaponBodyModels = map[string]int{
	"iron_sword": 0, "silver_sword": 1, "gold_sword": 2, "gladius": 3,
	"katana": 4, "wakizashi": 5, "muramasa": 6, "jungle_machete": 7,
	"magic_dagger": 8, "parry_dagger": 9, "throwing_knife": 10, "kage_kunai": 11,
	"serpent_fang": 12, "widow_hairpin": 13, "hatchling_fang": 14,
	"drakefang_blade": 15, "vibro_blade": 16, "solstice_transfer_blade": 17,
	"iron_spear": 20, "culvert_pike": 21, "hasta": 22, "yari": 23,
	"tonbogiri": 24, "chitin_spear": 25, "obsidian_spear": 26, "broodspike": 27,
	"trident": 28, "tarn_trident": 29, "naginata": 30, "solstice_thermal_lance": 31,
	"holy_mace": 40, "steel_mace": 41, "bone_warclub": 42, "kanabo": 43,
	"morningstar": 44, "ember_egg_mace": 45, "solstice_anchor_hammer": 46,
	"idol_breakers_maul": 47, "lion_warhammer": 48, "scalebreaker_maul": 49,
	"bronze_cesti": 50, "monk_fists": 52, "agility_katar": 53, "gale_war_fan": 54,
	"steel_axe": 60, "bronze_labrys": 61, "arena_labrys": 62,
	"gorehorn_greataxe": 63, "wyrmcleaver": 64,
	"cogfang_blade": 70, "chime_maul": 71, "escapement_mace": 72, "mainspring_pike": 73, "minute_hand": 74,
}

func tangentAt(path func(float64) (float64, float64), t float64) float64 {
	ax, ay := path(math.Max(0, t-.015))
	bx, by := path(math.Min(1, t+.015))
	return math.Atan2(by-ay, bx-ax)
}

// The tip is local x=0; the complete grip extends backwards along the same axis.
// One bounded shader quad draws the material, bevel, fittings and ornament.
func (r *Renderer) drawWeaponBody(dst *ebiten.Image, x, y, angle, size, alpha float64, model int, tint [3]int) {
	if size <= 0 || alpha <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	left, right, half := -5.2, .45, 1.4
	if model >= 50 && model < 60 {
		left, right, half = -2.5, 1, 2
	}
	ca, sa := math.Cos(angle), math.Sin(angle)
	mirror := 1.0
	// These single-edged models were authored with the edge on local -Y.
	// A slash/chop moves toward local +Y, so reflect across the shaft.
	switch model {
	case 4, 6, 17, 30, 60: // Katana, Muramasa, Transfer Blade, Naginata, Steel Axe.
		mirror = -1
	}
	if (model == 50 || model == 52 || model == 53) && r.weaponMaterialState.hand == 1 {
		mirror = -1
	}
	for i, p := range [4][2]float64{{left, -half}, {right, -half}, {left, half}, {right, half}} {
		ly := p[1] * mirror
		v := weaponMaterialVertex(x+(p[0]*ca-ly*sa)*size, y+(p[0]*sa+ly*ca)*size, p[0], p[1], tint, alpha)
		v.Custom0, v.Custom1, v.Custom2, v.Custom3 = float32(r.weaponMaterialClock()), float32(model)+.25, float32(1/math.Max(1, size)), 0
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	dst.DrawTrianglesShader(r.weaponMaterialQuad[:], weaponQuadIndices, r.weaponBodyShader, &r.weaponMaterialOpts)
}

package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Model numbers select procedural shapes, never item icon textures.
var weaponBodyModels = map[string]int{
	"iron_sword": 0, "silver_sword": 1, "gold_sword": 2, "gladius": 3,
	"katana": 4, "wakizashi": 5, "muramasa": 6, "jungle_machete": 7,
	"magic_dagger": 8, "parry_dagger": 9, "kage_kunai": 11,
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

// A rigid camera-space model turns around its authored striking point. The
// tip stays on the stroke path while the shaft, bevels and fittings turn in
// depth. That preserves the exact contact/ribbon anchor through perspective.
func (r *Renderer) drawWeaponBody(dst *ebiten.Image, x, y, angle, size, alpha float64, kind string, model int) {
	r.weaponMaterialState.bodyHandled = true
	if size <= 0 || alpha <= 0 || isGauntletModel(model) || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	mesh := meleeWeaponMeshes.get(model, buildMeleeWeaponMesh)
	if len(mesh.faces) == 0 {
		return
	}
	progress := r.weaponMaterialState.progress
	sweep := math.Min(1, progress/meleeSweepFrac)
	lead := 1 - (1-sweep)*(1-sweep)
	pitch, roll := .10+.64*math.Sin(lead*math.Pi), -.62+1.24*lead
	if kind == "stab" || kind == "lunge" {
		pitch = .70 + .28*math.Sin(lead*math.Pi)
		roll = -.18 + .36*lead
	}
	if kind == "smash" {
		pitch = .18 + .50*lead
		roll = -.3 + .60*lead
	}
	// Recovery is a rigid retreat of the whole object. The spent wake and
	// sparks stay at contact, rather than following the returning handle.
	recovery := math.Min(1, math.Max(0, (progress-meleeSweepFrac)/.30))
	y += size * .65 * recovery * recovery
	size *= 1 - .16*recovery
	mirror := 1.0
	// Single-edged models are authored with their cutting side at local +Y,
	// which leads a slash whose shaft angle is tangent - pi/2.
	if (model == 53 && r.weaponMaterialState.hand == 1) || (model == 62 && r.weaponMaterialState.hand == 0) {
		mirror = -1
	}
	ca, sa, cp, sp, cr, sr := math.Cos(angle), math.Sin(angle), math.Cos(pitch), math.Sin(pitch), math.Cos(roll), math.Sin(roll)
	transform := volumeTransform{rotation: [3][3]float64{
		{cp * ca, -mirror*sr*sp*ca - mirror*cr*sa, -cr*sp*ca + sr*sa},
		{cp * sa, -mirror*sr*sp*sa + mirror*cr*ca, -cr*sp*sa - sr*ca},
		{sp, mirror * sr * cp, cr * cp},
	}, uvY: 1}
	r.drawCameraVolume(dst, x, y, size, alpha, transform, r.weaponBodyShader, &r.weaponMaterialOpts, mesh, [3]int{255, 255, 255}, 0, true)
}

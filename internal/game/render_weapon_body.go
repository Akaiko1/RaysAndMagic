package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// meleeBody names one procedural melee model; it selects geometry, never an
// item icon texture. Values come from iota, so a new model can never silently
// take another's cache slot; no code relies on a value or on the zero body.
type meleeBody int

const (
	bodyIronSword meleeBody = iota
	bodySilverSword
	bodyGoldSword
	bodyGladius
	bodyKatana
	bodyWakizashi
	bodyMuramasa
	bodyJungleMachete
	bodyMagicDagger
	bodyParryDagger
	bodyKageKunai
	bodySerpentFang
	bodyWidowHairpin
	bodyHatchlingFang
	bodyDrakefangBlade
	bodyVibroBlade
	bodyTransferBlade
	bodyRainfang
	bodyIronSpear
	bodyCulvertPike
	bodyHasta
	bodyYari
	bodyTonbogiri
	bodyChitinSpear
	bodyObsidianSpear
	bodyBroodspike
	bodyTrident
	bodyTarnTrident
	bodyNaginata
	bodyThermalLance
	bodyHolyMace
	bodySteelMace
	bodyBoneWarclub
	bodyKanabo
	bodyMorningstar
	bodyEmberEggMace
	bodyAnchorHammer
	bodyIdolMaul
	bodyLionWarhammer
	bodyScalebreakerMaul
	bodyBronzeCesti
	bodyMonkFists
	bodyAgilityKatar
	bodyGaleWarFan
	bodySteelAxe
	bodyBronzeLabrys
	bodyArenaLabrys
	bodyGorehornGreataxe
	bodyWyrmcleaver
	bodyShellknocker
	bodyCogfangBlade
	bodyChimeMaul
	bodyEscapementMace
	bodyMainspringPike
	bodyMinuteHand
	bodyStoneScript
	meleeBodyCount
)

// meleeFamily is the parametric builder a body shares with its siblings;
// familyUnique bodies have a builder of their own.
type meleeFamily int

const (
	familyUnique meleeFamily = iota
	familySword
	familyPole
	familyHeavy
	familyGauntlet
	familyAxe
	familyClock
)

func (b meleeBody) family() meleeFamily { return meleeBodies[b].family }

// The four endgame hammers carry a sculpted block head instead of a flanged one.
func (b meleeBody) blockHammer() bool {
	return b == bodyAnchorHammer || b == bodyIdolMaul || b == bodyLionWarhammer || b == bodyScalebreakerMaul
}

// meleeBodies is the one table of procedural melee models: the weapon that
// draws each body and the builder family it shares (familyUnique: its own
// builder). family() and weaponBodyModels read it; validateWeaponFxStyles
// requires a body on every melee weapon.
var meleeBodies = [meleeBodyCount]struct {
	key    string
	family meleeFamily
}{
	bodyIronSword:        {"iron_sword", familySword},
	bodySilverSword:      {"silver_sword", familySword},
	bodyGoldSword:        {"gold_sword", familySword},
	bodyGladius:          {"gladius", familySword},
	bodyKatana:           {"katana", familySword},
	bodyWakizashi:        {"wakizashi", familySword},
	bodyMuramasa:         {"muramasa", familySword},
	bodyJungleMachete:    {"jungle_machete", familySword},
	bodyMagicDagger:      {"magic_dagger", familySword},
	bodyParryDagger:      {"parry_dagger", familySword},
	bodyKageKunai:        {"kage_kunai", familySword},
	bodySerpentFang:      {"serpent_fang", familySword},
	bodyWidowHairpin:     {"widow_hairpin", familySword},
	bodyHatchlingFang:    {"hatchling_fang", familySword},
	bodyDrakefangBlade:   {"drakefang_blade", familySword},
	bodyVibroBlade:       {"vibro_blade", familySword},
	bodyTransferBlade:    {"solstice_transfer_blade", familySword},
	bodyRainfang:         {"rainfang", familyUnique},
	bodyIronSpear:        {"iron_spear", familyPole},
	bodyCulvertPike:      {"culvert_pike", familyPole},
	bodyHasta:            {"hasta", familyPole},
	bodyYari:             {"yari", familyPole},
	bodyTonbogiri:        {"tonbogiri", familyPole},
	bodyChitinSpear:      {"chitin_spear", familyPole},
	bodyObsidianSpear:    {"obsidian_spear", familyPole},
	bodyBroodspike:       {"broodspike", familyPole},
	bodyTrident:          {"trident", familyPole},
	bodyTarnTrident:      {"tarn_trident", familyPole},
	bodyNaginata:         {"naginata", familyPole},
	bodyThermalLance:     {"solstice_thermal_lance", familyPole},
	bodyHolyMace:         {"holy_mace", familyHeavy},
	bodySteelMace:        {"steel_mace", familyHeavy},
	bodyBoneWarclub:      {"bone_warclub", familyHeavy},
	bodyKanabo:           {"kanabo", familyHeavy},
	bodyMorningstar:      {"morningstar", familyHeavy},
	bodyEmberEggMace:     {"ember_egg_mace", familyHeavy},
	bodyAnchorHammer:     {"solstice_anchor_hammer", familyHeavy},
	bodyIdolMaul:         {"idol_breakers_maul", familyHeavy},
	bodyLionWarhammer:    {"lion_warhammer", familyHeavy},
	bodyScalebreakerMaul: {"scalebreaker_maul", familyHeavy},
	bodyBronzeCesti:      {"bronze_cesti", familyGauntlet},
	bodyMonkFists:        {"monk_fists", familyGauntlet},
	bodyAgilityKatar:     {"agility_katar", familyUnique},
	bodyGaleWarFan:       {"gale_war_fan", familyUnique},
	bodySteelAxe:         {"steel_axe", familyAxe},
	bodyBronzeLabrys:     {"bronze_labrys", familyAxe},
	bodyArenaLabrys:      {"arena_labrys", familyAxe},
	bodyGorehornGreataxe: {"gorehorn_greataxe", familyAxe},
	bodyWyrmcleaver:      {"wyrmcleaver", familyAxe},
	bodyShellknocker:     {"shellknocker", familyUnique},
	bodyCogfangBlade:     {"cogfang_blade", familyClock},
	bodyChimeMaul:        {"chime_maul", familyClock},
	bodyEscapementMace:   {"escapement_mace", familyClock},
	bodyMainspringPike:   {"mainspring_pike", familyClock},
	bodyMinuteHand:       {"minute_hand", familyClock},
	bodyStoneScript:      {"solstice_stone_script", familyUnique},
}

var weaponBodyModels = func() map[string]meleeBody {
	out := make(map[string]meleeBody, len(meleeBodies))
	for body, row := range meleeBodies {
		out[row.key] = meleeBody(body)
	}
	return out
}()

func tangentAt(path func(float64) (float64, float64), t float64) float64 {
	ax, ay := path(math.Max(0, t-.015))
	bx, by := path(math.Min(1, t+.015))
	return math.Atan2(by-ay, bx-ax)
}

// A rigid camera-space model turns around its authored striking point. The
// tip stays on the stroke path while the shaft, bevels and fittings turn in
// depth. That preserves the exact contact/ribbon anchor through perspective.
func (r *Renderer) drawWeaponBody(dst *ebiten.Image, x, y, angle, size, alpha float64, kind string, body meleeBody) {
	r.weaponMaterialState.bodyHandled = true
	if size <= 0 || alpha <= 0 || body.family() == familyGauntlet || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	mesh := meleeWeaponMeshes.get(body, buildMeleeWeaponMesh)
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
	if (body == bodyAgilityKatar && r.weaponMaterialState.hand == 1) || (body == bodyArenaLabrys && r.weaponMaterialState.hand == 0) {
		mirror = -1
	}
	r.drawCameraVolume(dst, x, y, size, alpha, equipmentTransform(angle, pitch, roll, mirror), r.weaponBodyShader, &r.weaponMaterialOpts, mesh, [3]int{255, 255, 255}, 0, true)
}

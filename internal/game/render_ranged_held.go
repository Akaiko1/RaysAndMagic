package game

import (
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

type rangedHeldKind uint8

const (
	heldStaff rangedHeldKind = iota
	heldBow
	heldCrossbow
	heldGun
	heldBook
	heldBlowgun
)

// category is the weapon category a held kind serves.
func (k rangedHeldKind) category() string {
	switch k {
	case heldStaff, heldBook:
		return "staff"
	case heldBow, heldCrossbow, heldBlowgun:
		return "bow"
	case heldGun:
		return "blaster"
	}
	return ""
}

type rangedHeldAsset struct {
	kind         rangedHeldKind
	scale, crown float64
	build        func() [][]modelFace // rest geometry, one slice per part
	limbs        *rangedLimbs         // bows and crossbows only
	flame        bool
}

// Assets describe geometry and its attachment point. Selection is authored in
// weapons.yaml (held_model) and must match the weapon's category; projectile
// styles stay independent. The asset key also keys its compiled rest model.
var rangedHeldAssets = map[string]rangedHeldAsset{
	"oak_staff":             {heldStaff, .155, .94, onePart(buildOakHeldModel), nil, false},
	"hunting_bow":           {heldBow, .192, 0, onePart(buildBowHeldModel), bowLimbs(1.67, [3]float64{1.67, -.16, 0}, [3]int{199, 172, 112}), false},
	"duskneedle":            {heldBow, .192, 0, onePart(buildDuskHeldModel), bowLimbs(1.77, [3]float64{1.77, -.06, 0}, [3]int{130, 125, 135}), false},
	"arena_shortbow":        {heldBow, .175, 0, onePart(buildArenaHeldModel), bowLimbs(1.56, [3]float64{1.48, -.18, 0}, [3]int{180, 136, 49}), false},
	"solstice_sun_crucible": {heldStaff, .155, .99, onePart(buildSunHeldModel), nil, true},
	"archmage_staff":        {heldStaff, .155, 1.1, onePart(buildArchmageHeldModel), nil, false},
	"solstice_flowstaff":    {heldStaff, .155, 1.04, onePart(buildFlowHeldModel), nil, false},
	"battle_staff":          {heldStaff, .155, 1.35, onePart(buildBattleHeldModel), nil, false},
	"lanista_scepter":       {heldStaff, .155, 1.0, onePart(buildLanistaHeldModel), nil, false},
	"verdant_eye_scepter":   {heldStaff, .155, 1.03, onePart(buildEyeHeldModel), nil, false},
	"escapement_staff":      {heldStaff, .155, 1.05, onePart(buildEscapementHeldModel), nil, false},
	"book_of_darkness":      {heldBook, .185, 0, buildBookHeldModel, nil, false},
	"long_bow":              {heldBow, .192, 0, onePart(buildLongBowHeldModel), bowLimbs(1.89, [3]float64{1.86, -.10, 0}, [3]int{157, 128, 78}), false},
	"elven_bow":             {heldBow, .192, 0, onePart(buildElvenBowHeldModel), bowLimbs(1.62, [3]float64{1.59, -.21, 0}, [3]int{202, 162, 58}), false},
	"bow_of_hellfire":       {heldBow, .192, 0, onePart(buildHellfireHeldModel), bowLimbs(1.82, [3]float64{1.72, -.15, 0}, [3]int{238, 58, 9}), false},
	"wyrmspine_bow":         {heldBow, .192, 0, onePart(buildWyrmHeldModel), bowLimbs(1.82, [3]float64{1.70, .015, 0}, [3]int{174, 139, 64}), false},
	"compound_bow":          {heldBow, .192, 0, onePart(buildCompoundHeldModel), bowLimbs(1.62, [3]float64{1.49, -.07, 0}, [3]int{133, 151, 157}), false},
	"arbalest":              {heldCrossbow, .22, 0, onePart(func() []modelFace { return buildCrossbowHeldModel(false) }), crossbowLimbs([3]int{190, 151, 61}), false},
	"nest_arbalest":         {heldCrossbow, .22, 0, onePart(func() []modelFace { return buildCrossbowHeldModel(true) }), crossbowLimbs([3]int{190, 151, 61}), false},
	"blowgun":               {heldBlowgun, .20, 0, onePart(buildBlowgunHeldModel), nil, false},
	"surveyors_rifle":       {heldGun, .20, 0, onePart(func() []modelFace { return buildMatchlockHeldModel(0) }), nil, false},
	"tanegashima":           {heldGun, .20, 0, onePart(func() []modelFace { return buildMatchlockHeldModel(1) }), nil, false},
	"inazuma_matchlock":     {heldGun, .20, 0, onePart(func() []modelFace { return buildMatchlockHeldModel(2) }), nil, false},
	"alien_blaster":         {heldGun, .23, 0, onePart(buildAlienHeldModel), nil, false},
	"clockwork_pistol":      {heldGun, .23, 0, onePart(buildClockPistolHeldModel), nil, false},
	"suppressor_gun":        {heldGun, .21, 0, onePart(buildSuppressorHeldModel), nil, false},
	"longlance_rifle":       {heldGun, .20, 0, onePart(buildLonglanceHeldModel), nil, false},
}

// rangedHeldCategories are the weapon categories whose ranged weapons must
// author a held model.
var rangedHeldCategories = func() map[string]bool {
	out := map[string]bool{}
	for _, a := range rangedHeldAssets {
		out[a.kind.category()] = true
	}
	return out
}()

type rangedHeldAnimation struct {
	hero          *character.MMCharacter
	hand          int // 0 main hand, 1 off hand
	model         string
	age, duration int
	angle         float64
	glow          [3]int
	charged       bool
	release       rangedRelease // a bow's screen point and aim at the shot
	// The camera angle at release: a held model keeps its aim relative to it,
	// so turning never swivels the weapon.
	sourceAngle float64
}

// aim is the shot's direction relative to the camera at release.
func (fx rangedHeldAnimation) aim() float64 { return fx.angle - fx.sourceAngle }

// One held model per hero and hand, kept in game state like melee slashes: the
// hero's next shot from that hand replaces it, other heroes' models play on as
// their swings do, and the hero's own melee swing ends them. Identity, aim and
// cooldown are captured after a successful shot, never read from later
// equipment or selection. It neither delays nor moves the gameplay projectile,
// and a run without a renderer records nothing.
func (cs *CombatSystem) presentRangedShot(hero *character.MMCharacter, def *config.WeaponDefinitionConfig, slot items.EquipSlot, launch projectileLaunch, charged bool) {
	g := cs.game
	if hero == nil || def == nil || def.Graphics == nil || g.gameLoop == nil || g.gameLoop.renderer == nil {
		return
	}
	asset, ok := rangedHeldAssets[def.Graphics.HeldModel]
	if !ok {
		return
	}
	hand := 0
	if slot == items.SlotOffHand {
		hand = 1
	}
	timing := g.config.Graphics.RangedHeld
	cooldown := cs.weaponCooldownBreakdown(hero, def).TotalFrames
	fx := rangedHeldAnimation{hero: hero, hand: hand, model: def.Graphics.HeldModel, duration: max(1, min(g.framesForSeconds(timing.MaxDurationSeconds), int(timing.CooldownFraction*float64(cooldown)))), angle: launch.angle, glow: def.Graphics.Color, charged: charged, sourceAngle: g.camera.Angle}
	if asset.kind == heldBow {
		fx.release = g.gameLoop.renderer.rangedReleaseRay(launch)
	}
	for i, held := range g.rangedHeld {
		if held.hero == hero && held.hand == hand {
			g.rangedHeld[i] = fx
			return
		}
	}
	g.rangedHeld = append(g.rangedHeld, fx)
}

// endRangedHeld drops a hero's held models when that hero swings in melee.
func (g *MMGame) endRangedHeld(hero *character.MMCharacter) {
	g.rangedHeld = slices.DeleteFunc(g.rangedHeld, func(fx rangedHeldAnimation) bool { return fx.hero == hero })
}

func (g *MMGame) updateRangedHeld() {
	kept := g.rangedHeld[:0]
	for _, fx := range g.rangedHeld {
		if fx.age++; fx.age < fx.duration {
			kept = append(kept, fx)
		}
	}
	clear(g.rangedHeld[len(kept):]) // finished entries release their hero
	g.rangedHeld = kept
}

// tilt turns a camera transform about the camera's horizontal axis.
func (t *volumeTransform) tilt(angle float64) {
	c, s := math.Cos(angle), math.Sin(angle)
	for j := 0; j < 3; j++ {
		y, z := t.rotation[1][j], t.rotation[2][j]
		t.rotation[1][j], t.rotation[2][j] = c*y-s*z, s*y+c*z
	}
}

func (r *Renderer) drawRangedHeld(dst *ebiten.Image) {
	previous := activeWorldRaster
	activeWorldRaster = nil
	defer func() { activeWorldRaster = previous }()
	w, h := float64(r.game.worldWidth()), float64(r.game.worldHeight())
	for _, fx := range r.game.rangedHeld {
		a := rangedHeldAssets[fx.model]
		p := float64(fx.age) / float64(fx.duration)
		retreat := smoothStep((p - .38) / .62)
		fade := 1 - smoothStep((p-.75)/.25)
		aim := fx.aim()
		side := 1.0
		if fx.hand == 1 {
			side = -1
		}
		var tr volumeTransform
		var x, y, gx, gy, pose float64
		modelScale := r.game.config.Graphics.RangedHeld.ModelScale
		size := h * a.scale * modelScale
		switch a.kind {
		case heldBow:
			pose = math.Max(0, math.Exp(-p*24)*math.Cos(p*32))
			var pitch float64
			x, y, aim, pitch = r.rangedBowRelease(fx)
			tr = rangedBowTransform(-aim+.42*retreat*side, side*(.12+.08*retreat))
			tr.tilt(-pitch)
			x, y = x+side*w*.07*retreat, y+h*.60*retreat
		case heldCrossbow, heldGun, heldBlowgun:
			// +Y is the bore/rail; +X is horizontal, +Z is vertical. Unlike a bow,
			// the prod lies across the stock, never upright beside it.
			recoil := math.Sin(math.Min(1, p/.24)*math.Pi) * .10
			if a.kind == heldBlowgun {
				recoil *= .20
			}
			tr = rangedBowTransform(.35*side-aim, math.Pi/2)
			// Show the top of the rail, then add the short upward recoil.
			tr.tilt(.16 + recoil*.6)
			x, y = w*(.5+side*.13)+side*w*.07*retreat, h*(.64-recoil*.35+.5*retreat)
			if a.kind == heldCrossbow {
				pose = math.Exp(-p * 28)
			}
		case heldBook:
			// The two covers hinge around their common spine while the book rises.
			// The shot already exists at release. Expose the pages from frame one
			// and finish opening as it rises; never fire through a closed cover.
			pose = (.66 + .34*smoothStep(p/.18)) * (1 - .22*retreat)
			lift := smoothStep(p / .16)
			tr = equipmentTransform(-.10, .14, .60, 1)
			// Reflect through the plane perpendicular to the shot axis. The
			// open pages face downrange; the cover is on the caster's side.
			forward := [3]float64{math.Sin(aim), 0, math.Cos(aim)}
			for axis := 0; axis < 3; axis++ {
				dot := forward[0]*tr.rotation[0][axis] + forward[2]*tr.rotation[2][axis]
				tr.rotation[0][axis] -= 2 * forward[0] * dot
				tr.rotation[2][axis] -= 2 * forward[2] * dot
			}
			x, y = w*(.5+side*.055), h*(.75-.09*lift+.5*retreat)
		case heldStaff:
			tr = equipmentTransform(-1.92+.28*smoothStep(p/.28)-.14*retreat, .30-.20*retreat, -.12+.20*retreat, 1)
			// Place the model so its crown projects onto the grip point.
			cx, cy, focal := r.cameraVolumeProjection(dst.Bounds(), true)
			lx, ly, lz := a.crown*tr.rotation[0][0], a.crown*tr.rotation[1][0], a.crown*tr.rotation[2][0]
			q := 1 / (1 + lz*size/focal)
			gx, gy = w*(.5+side*.06)+side*w*.17*retreat, r.game.viewHorizon()+h*.53*retreat
			x = cx + (gx-cx)/q - lx*size
			y = cy + (gy-cy)/q - ly*size
			if a.flame && fx.charged {
				r.drawRangedSunFlame(dst, gx, gy, h*.55*modelScale, fade)
			}
		}
		r.drawRangedHeldMesh(dst, dst.Bounds().Size(), x, y, size, fade, tr, r.heldMesh(fx.model, a, pose, size))
		if a.kind == heldStaff && float64(fx.age)/float64(r.game.config.GetTPS()) < .08 {
			glow := 1 - smoothStep(float64(fx.age)/float64(r.game.config.GetTPS())/.08)
			r.drawWeaponCharge(dst, gx, gy, h*.014, 0, 0, fx.glow, 1, .65*glow, 9)
		}
	}
}

package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/monster"
)

// All geometry is procedural: a first proc never loads a sprite or pauses play.
type elementalAttackEffect struct {
	MonsterTarget *monster.Monster3D
	X, Y          float64
	PartyTarget   *character.MMCharacter
	School        string
	Age, Frames   int
	RadiusTiles   float64
	Particles     int
}

func (g *MMGame) addElementalAttackFX(x, y float64, target *character.MMCharacter, school string) *elementalAttackEffect {
	if g == nil || g.config == nil {
		return nil
	}
	cfg := g.config.Graphics.ElementalAttack
	if cfg.MaxActive <= 0 || cfg.DurationSeconds <= 0 {
		return nil
	}
	if len(g.elementalAttackEffects) >= cfg.MaxActive {
		keep := cfg.MaxActive - 1
		copy(g.elementalAttackEffects, g.elementalAttackEffects[len(g.elementalAttackEffects)-keep:])
		clear(g.elementalAttackEffects[keep:])
		g.elementalAttackEffects = g.elementalAttackEffects[:keep]
	}
	g.elementalAttackEffects = append(g.elementalAttackEffects, elementalAttackEffect{
		X: x, Y: y, PartyTarget: target, School: school,
		Frames:      max(1, int(math.Round(cfg.DurationSeconds*float64(g.config.GetTPS())))),
		RadiusTiles: cfg.RadiusTiles, Particles: cfg.ParticleCount,
	})
	return &g.elementalAttackEffects[len(g.elementalAttackEffects)-1]
}

func (g *MMGame) addMonsterElementalAttackFX(m *monster.Monster3D, school string) {
	if m == nil {
		return
	}
	if fx := g.addElementalAttackFX(m.X, m.Y, nil, school); fx != nil {
		fx.MonsterTarget = m
	}
}

func (g *MMGame) tickElementalAttackFX() {
	dst := g.elementalAttackEffects[:0]
	for _, fx := range g.elementalAttackEffects {
		fx.Age++
		if fx.Age < fx.Frames {
			dst = append(dst, fx)
		}
	}
	clear(g.elementalAttackEffects[len(dst):])
	g.elementalAttackEffects = dst
}

// drawElementalAttackGlyph is shared by world hits, portrait hits and the GIF
// harness. It never changes gameplay or consumes random numbers.
func (r *Renderer) drawElementalAttackGlyph(dst *ebiten.Image, fx elementalAttackEffect, cx, cy, radius float64) {
	if fx.Frames <= 0 || radius <= 0 {
		return
	}
	p := float64(fx.Age) / float64(fx.Frames)
	if p < 0 || p >= 1 {
		return
	}
	base := color.RGBAModel.Convert(SchoolColor(fx.School)).(color.RGBA)
	rgb := [3]int{int(base.R), int(base.G), int(base.B)}
	previous := r.weaponMaterialState
	r.weaponMaterialState = weaponMaterialState{material: weaponMaterial(fx.School), phase: p * 5, seed: 17}
	defer func() { r.weaponMaterialState = previous }()
	if p < .25 {
		for k := 0; k < 3; k++ {
			a := float64(k)*2*math.Pi/3 - .4
			rad := radius * (1 - .5*p/.25)
			r.drawWeaponShard(dst, cx+math.Cos(a)*rad, cy+math.Sin(a)*rad, math.Max(2, radius*.09), rgb, .5+.5*p/.25, p, k*37, false)
		}
		return
	}
	u := (p - .25) / .75
	fade := math.Pow(1-u, .65)
	kind := spellPsyshock
	switch fx.School {
	case "fire":
		kind = spellFireball
	case "water":
		kind = spellIce
	case "earth":
		kind = spellRock
	case "air":
		kind = spellLightning
	case "dark":
		kind = spellShadow
	case "light":
		kind = spellLight
	case "spirit":
		kind = spellStarburst
	case "body":
		kind = spellHarm
	}
	r.drawImpactCloud(dst, cx, cy, radius*1.3, radius, u, rgb, fade*.6, 17, fx.School != "fire")
	r.drawSpellMaterialFade(dst, cx, cy, radius*(.55+.3*u), 0, 0, rgb, 1, 17, kind, fade*.82)

	for k := 0; k < fx.Particles; k++ {
		a := float64(k)*2*math.Pi/float64(fx.Particles) + .35
		rad := radius * (.3 + 1.4*u)
		r.drawWeaponShard(dst, cx+math.Cos(a)*rad, cy+math.Sin(a)*rad, math.Max(1.5, radius*.075), rgb, fade, u, k*37, false)
	}
	if core := math.Max(0, 1-u*4); core > 0 {
		r.drawSparkStar(dst, cx, cy, radius*(.13+.25*core), rgb, [3]int{245, 244, 219}, core, 1)
	}
}

func (r *Renderer) drawElementalAttackFX(screen *ebiten.Image) {
	for _, fx := range r.game.elementalAttackEffects {
		if fx.PartyTarget != nil {
			continue
		}
		if fx.MonsterTarget != nil && fx.MonsterTarget.IsAlive() {
			for _, sprite := range r.unifiedSprites {
				if sprite.monster != fx.MonsterTarget || !r.spriteDepthBufferVisible(sprite) {
					continue
				}
				top := sprite.bottomF - sprite.sizeF
				radius := r.elementalAttackScreenRadius(fx, sprite.depthPerp, screen.Bounds().Dy())
				r.drawElementalAttackGlyph(screen, fx, sprite.screenXF, top+sprite.sizeF*.45, radius)
				break
			}
			continue
		}
		x, depth, ok := r.game.renderHelper.projectToScreenX(fx.X, fx.Y)
		if !ok || depth <= 0 {
			continue
		}
		if r.game.collisionSystem != nil && !r.game.collisionSystem.CheckLineOfSight(r.game.camera.X, r.game.camera.Y, fx.X, fx.Y) {
			continue
		}
		radius := r.elementalAttackScreenRadius(fx, depth, screen.Bounds().Dy())
		r.drawElementalAttackGlyph(screen, fx, float64(x), float64(worldViewportBottom(r.game))*.5, radius)
	}
}

func (ui *UISystem) drawPortraitElementalAttackFX(screen *ebiten.Image, member *character.MMCharacter, x, y, w, h int) {
	if ui.game.gameLoop == nil || ui.game.gameLoop.renderer == nil {
		return
	}
	r := ui.game.gameLoop.renderer
	for _, fx := range ui.game.elementalAttackEffects {
		if fx.PartyTarget == member {
			r.drawElementalAttackGlyph(screen, fx, float64(x+w/2), float64(y+h/2), float64(min(w, h))*.28)
		}
	}
}

// YAML radii are world tiles, independent of the target sprite's size class.
func (r *Renderer) elementalAttackScreenRadius(fx elementalAttackEffect, depth float64, screenHeight int) float64 {
	if depth <= 0 {
		return 0
	}
	radius := fx.RadiusTiles * r.game.config.GetTileSize() * r.game.viewFocal() / (depth * r.game.camera.FOV)
	return math.Min(radius, float64(screenHeight)*.12)
}

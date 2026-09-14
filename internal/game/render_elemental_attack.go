package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
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
func drawElementalAttackGlyph(dst *ebiten.Image, fx elementalAttackEffect, cx, cy, radius float64) {
	if fx.Frames <= 0 || radius <= 0 {
		return
	}
	p := float64(fx.Age) / float64(fx.Frames)
	if p < 0 || p >= 1 {
		return
	}
	base := color.RGBAModel.Convert(SchoolColor(fx.School)).(color.RGBA)
	col := func(a float64) color.RGBA {
		return color.RGBA{uint8(float64(base.R) * a), uint8(float64(base.G) * a), uint8(float64(base.B) * a), uint8(255 * a)}
	}
	if p < .25 {
		for k := 0; k < 3; k++ {
			a := float64(k)*2*math.Pi/3 - .4
			r := radius * (1 - .5*p/.25)
			x, y := cx+math.Cos(a)*r, cy+math.Sin(a)*r
			vector.FillRect(dst, float32(x-2), float32(y-2), 4, 4, col(.5+.5*p/.25), false)
		}
		return
	}
	u := (p - .25) / .75
	fade := math.Pow(1-u, .65)
	for k := 0; k < 3; k++ {
		for q := 1; q < 15; q++ {
			point := func(n int) (float32, float32) {
				a := float64(k)*2*math.Pi/3 - .75 + float64(n)*.055
				r := radius * (1 - float64(n)*.027) * (1 + .35*u)
				return float32(cx + math.Cos(a)*r), float32(cy + math.Sin(a)*r)
			}
			x1, y1 := point(q - 1)
			x2, y2 := point(q)
			vector.StrokeLine(dst, x1, y1, x2, y2, float32(math.Max(1, radius*.10*(1-u))), col(fade), false)
		}
	}
	for k := 0; k < fx.Particles; k++ {
		a := float64(k)*2*math.Pi/float64(fx.Particles) + .35
		r := radius * (.3 + 1.4*u)
		x, y := cx+math.Cos(a)*r, cy+math.Sin(a)*r
		s := math.Max(1, radius*.07*(1-u))
		vector.FillRect(dst, float32(x-s), float32(y-s), float32(2*s), float32(2*s), col(fade), false)
	}
	if core := math.Max(0, 1-u*4); core > 0 {
		s := radius * (.13 + .25*core)
		c := color.RGBA{uint8(245 * core), uint8(244 * core), uint8(219 * core), uint8(255 * core)}
		vector.StrokeLine(dst, float32(cx-s), float32(cy), float32(cx+s), float32(cy), 2, c, false)
		vector.StrokeLine(dst, float32(cx), float32(cy-s), float32(cx), float32(cy+s), 2, c, false)
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
				top := clampMonsterSpriteTopToGameplayViewport(r.game, sprite.bottomF-sprite.sizeF, sprite.sizeF)
				radius := r.elementalAttackScreenRadius(fx, sprite.depthPerp, screen.Bounds().Dy())
				drawElementalAttackGlyph(screen, fx, sprite.screenXF, top+sprite.sizeF*.45, radius)
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
		drawElementalAttackGlyph(screen, fx, float64(x), float64(gameplayViewportBottom(r.game))*.5, radius)
	}
}

func (ui *UISystem) drawPortraitElementalAttackFX(screen *ebiten.Image, member *character.MMCharacter, x, y, w, h int) {
	for _, fx := range ui.game.elementalAttackEffects {
		if fx.PartyTarget == member {
			drawElementalAttackGlyph(screen, fx, float64(x+w/2), float64(y+h/2), float64(min(w, h))*.28)
		}
	}
}

// YAML radii are world tiles, independent of the target sprite's size class.
func (r *Renderer) elementalAttackScreenRadius(fx elementalAttackEffect, depth float64, screenHeight int) float64 {
	if depth <= 0 {
		return 0
	}
	radius := fx.RadiusTiles * r.game.config.GetTileSize() * float64(screenHeight) / (depth * r.game.camera.FOV)
	return math.Min(radius, float64(screenHeight)*.12)
}

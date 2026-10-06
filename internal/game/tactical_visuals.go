package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

var tacticalBadgeBackground = color.RGBA{12, 18, 22, 230}

// One reticle links the ready hero badge to the designated enemy.
func drawTacticalReticle(screen *ebiten.Image, cx, cy, radius float32) {
	uiFillCircle(screen, cx, cy, radius, tacticalBadgeBackground, true)
	tint := color.RGBA{235, 193, 92, 255}
	uiStrokeCircle(screen, cx, cy, radius*0.56, 1.2, tint, true)
	arm := radius * 0.78
	uiStrokeLine(screen, cx-arm, cy, cx+arm, cy, 1, tint, true)
	uiStrokeLine(screen, cx, cy-arm, cx, cy+arm, 1, tint, true)
}

// A bound former enemy wears two linked rings: allegiance at a glance, without
// covering the actor the way a full-body overlay would.
func drawBindBadge(screen *ebiten.Image, cx, cy, radius float32) {
	uiFillCircle(screen, cx, cy, radius, tacticalBadgeBackground, true)
	tint := color.RGBA{116, 224, 208, 255}
	link := radius * 0.32
	uiStrokeCircle(screen, cx-link*0.7, cy, link, 1.4, tint, true)
	uiStrokeCircle(screen, cx+link*0.7, cy, link, 1.4, tint, true)
}

// drawSummonerBadge marks an ally with its summoner: the hero's face in a
// ring, so a glance tells whose it is.
func drawSummonerBadge(screen, portrait *ebiten.Image, cx, cy, radius float32) {
	uiFillCircle(screen, cx, cy, radius, tacticalBadgeBackground, true)
	if portrait != nil {
		// Portraits frame the face at the middle, a little above centre.
		b := portrait.Bounds()
		side := float32(min(b.Dx(), b.Dy()))
		fx := float32(b.Min.X) + float32(b.Dx())/2
		fy := float32(b.Min.Y) + side*0.42
		src := side * 0.36
		inner := radius - 2
		const segments = 28
		white := func(x, y, sx, sy float32) ebiten.Vertex {
			return ebiten.Vertex{DstX: x, DstY: y, SrcX: sx, SrcY: sy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
		}
		vs := make([]ebiten.Vertex, 0, segments+1)
		vs = append(vs, white(cx, cy, fx, fy))
		is := make([]uint16, 0, segments*3)
		for i := 0; i < segments; i++ {
			a := 2 * math.Pi * float64(i) / segments
			dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
			vs = append(vs, white(cx+dx*inner, cy+dy*inner, fx+dx*src, fy+dy*src))
			is = append(is, 0, uint16(1+i), uint16(1+(i+1)%segments))
		}
		uiDrawTriangles(screen, vs, is, portrait, &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear, AntiAlias: true})
	}
	uiStrokeCircle(screen, cx, cy, radius-1, 1.6, color.RGBA{116, 224, 208, 255}, true)
}

// statusBadgeRenderers draw the head-badge motifs, keyed by statusBadgeVisuals.
var statusBadgeRenderers = map[statusVisuals]func(dst *ebiten.Image, cx, cy, radius float32){
	visualBind: drawBindBadge,
}

// drawMonsterHeadBadges rides the small state markers in one row above the
// head: the Sniper's designation reticle, the summoner's face, then each badge
// motif in catalog order.
func (r *Renderer) drawMonsterHeadBadges(screen *ebiten.Image, s UnifiedSpriteRenderData, screenY int, badges statusVisuals) {
	designated := r.game.designationBonus(s.monster) > 0
	badges &= statusBadgeVisuals
	summoner := r.game.summonerOf(s.monster)
	if !designated && badges == 0 && summoner == nil {
		return
	}
	// Hide the markers when their anchor is behind terrain, including a partly
	// visible monster peeking from a doorway. The normal sprite pass owns FOV.
	if s.screenX < 0 || s.screenX >= len(r.game.depthBuffer) || s.depthPerp >= r.game.depthBuffer[s.screenX] {
		return
	}
	radius := float32(min(14, max(9, s.spriteSize/12)))
	type headBadge struct {
		radius float32
		draw   func(x, y, radius float32)
	}
	var row []headBadge
	if designated {
		row = append(row, headBadge{radius, func(x, y, r float32) { drawTacticalReticle(screen, x, y, r) }})
	}
	if summoner != nil {
		// A face needs more pixels than a motif to read.
		portrait := r.game.sprites.GetSprite(r.game.portraitSpriteName(summoner))
		row = append(row, headBadge{radius * 4 / 3, func(x, y, r float32) { drawSummonerBadge(screen, portrait, x, y, r) }})
	}
	for _, e := range statusVisualCatalog {
		if badges&e.flag != 0 {
			row = append(row, headBadge{radius, func(x, y, r float32) { statusBadgeRenderers[e.flag](screen, x, y, r) }})
		}
	}
	const gap = 4
	var width, tallest float32
	for _, b := range row {
		width += 2*b.radius + gap
		tallest = max(tallest, b.radius)
	}
	y := max(tallest+3, float32(screenY)-tallest-5)
	x := float32(s.screenX) - (width-gap)/2
	for _, b := range row {
		b.draw(x+b.radius, y, b.radius)
		x += 2*b.radius + gap
	}
}

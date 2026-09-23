package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Pick geometry belongs to the displayed frame, including pulled positions,
// flying height, standee yaw and hit shake. No GPU readback is needed by input.
type monsterPickHit struct {
	sprite                         *ebiten.Image
	flipped                        bool
	monster                        *monster.Monster3D
	left, top, size, depth, bottom float64
	p0x, p0y, dx, dy               float64
	standee                        bool
}
type monsterPickFrame struct {
	world                                  *world.World3D
	width, height                          int
	camX, camY, dirX, dirY, planeX, planeY float64
	hits                                   []monsterPickHit
}

func (r *Renderer) beginMonsterPickFrame() {
	f := &r.monsterPick
	g := r.game
	f.world = g.world
	f.width, f.height = g.config.GetScreenWidth(), g.config.GetScreenHeight()
	f.camX, f.camY = g.camera.X, g.camera.Y
	f.dirX, f.dirY = math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
	f.planeX, f.planeY = -f.dirY*math.Tan(g.camera.FOV/2), f.dirX*math.Tan(g.camera.FOV/2)
	f.hits = f.hits[:0]
}
func (r *Renderer) selectMonsterHover() {
	g := r.game
	r.hoveredMonster = nil
	if g.worldClickAllowed() && !g.dragArmed && !g.dragActive && !g.dragPickedUp {
		x, y := pointerPosition()
		r.hoveredMonster = g.monsterAtScreen(x, y)
	}
}

func (r *Renderer) makeMonsterPick(s UnifiedSpriteRenderData, x, y, yaw, left, top float64, standee bool) monsterPickHit {
	h := monsterPickHit{sprite: s.sprite, flipped: s.monsterFlip, monster: s.monster, left: left, top: top, size: s.sizeF, depth: s.depthPerp, bottom: s.bottomF, standee: standee}
	if standee {
		h.flipped = s.monster.StandeeMirror
		length := r.spriteFootprintWorld(s.sizeF, s.depthPerp)
		h.dx, h.dy = math.Cos(yaw)*length, math.Sin(yaw)*length
		h.p0x, h.p0y = x-h.dx/2, y-h.dy/2
	}
	return h
}
func (g *MMGame) monsterAtScreen(x, y int) *monster.Monster3D {
	m, depth := g.pickMonsterAtScreen(x, y)
	if m == nil {
		return nil
	}
	// A background door/NPC must not steal a foreground monster's press.
	// Keep the same visible-depth priority for hover and click; range is still
	// checked by the winning object's interaction or attack dispatcher.
	if npc, _ := g.findNPCAtScreen(x, y); npc != nil {
		f := &g.gameLoop.renderer.monsterPick
		nx, ny := g.npcEffectivePos(npc)
		npcDepth := (nx-f.camX)*f.dirX + (ny-f.camY)*f.dirY
		if npcDepth <= depth {
			return nil
		}
	}
	return m
}

func (g *MMGame) pickMonsterAtScreen(x, y int) (*monster.Monster3D, float64) {
	if g.gameLoop == nil || g.gameLoop.renderer == nil {
		return nil, 0
	}
	if ui := g.gameLoop.ui; ui != nil {
		if ui.modalRedrawBarrierActive() {
			return nil, 0
		}
		for _, cmd := range ui.displayedInput.commands {
			b := cmd.bounds
			if x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h {
				return nil, 0
			}
		}
	}
	r := g.gameLoop.renderer
	f := &r.monsterPick
	if f.world != g.world || f.width != g.config.GetScreenWidth() || f.height != g.config.GetScreenHeight() || x < 0 || y < 0 || x >= f.width || y >= f.height {
		return nil, 0
	}
	var best *monster.Monster3D
	nearest := math.Inf(1)
	for _, h := range f.hits {
		m := h.monster
		if m == nil || !m.IsAlive() || m.IsPartyControlled() {
			continue
		}
		depth, top, bottom := h.depth, h.top, h.top+h.size
		u := (float64(x) + 0.5 - h.left) / h.size
		if h.standee {
			rx, ry := standeeRayAtScreenX(float64(x)+0.5, f.width, f.dirX, f.dirY, f.planeX, f.planeY)
			t, columnU, ok := standeeColumnHit(f.camX, f.camY, rx, ry, h.p0x, h.p0y, h.dx, h.dy)
			if !ok {
				continue
			}
			depth = t
			u = columnU
			horizon := float64(f.height) / 2
			bottom = horizon + (h.bottom-horizon)*h.depth/t
			top = bottom - h.size*h.depth/t
		} else if float64(x) < h.left || float64(x) >= h.left+h.size {
			continue
		}
		if float64(y) < top || float64(y) >= bottom || depth >= nearest {
			continue
		}
		if x < len(g.depthBuffer) && standeeColumnOccluded(depth, g.depthBuffer[x], 0) {
			continue
		}
		if h.sprite != nil && g.sprites != nil {
			if h.flipped {
				u = 1 - u
			}
			bounds := h.sprite.Bounds()
			px := int(u * float64(bounds.Dx()))
			py := int((float64(y) + 0.5 - top) / (bottom - top) * float64(bounds.Dy()))
			if opaque, known := g.sprites.ImageOpaqueAt(h.sprite, px, py); known && !opaque {
				continue
			}
		}
		present := false
		for _, live := range g.world.Monsters {
			if live == m {
				present = true
				break
			}
		}
		if !present {
			continue
		}
		nearest, best = depth, m
	}
	return best, nearest
}

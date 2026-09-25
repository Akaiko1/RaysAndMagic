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

// monsterPointerFrameAllowed shares the displayed UI and viewport gates between
// pixel picking and an existing hold. HUD pixels must not count as empty world.
func (g *MMGame) monsterPointerFrameAllowed(x, y int) bool {
	if g.gameLoop == nil || g.gameLoop.renderer == nil {
		return false
	}
	if ui := g.gameLoop.ui; ui != nil {
		if ui.modalRedrawBarrierActive() {
			return false
		}
		for _, cmd := range ui.displayedInput.commands {
			b := cmd.bounds
			if x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h {
				return false
			}
		}
	}
	f := &g.gameLoop.renderer.monsterPick
	return f.world == g.world && f.width == g.config.GetScreenWidth() && f.height == g.config.GetScreenHeight() && x >= 0 && y >= 0 && x < f.width && y < f.height
}

// heldMonsterVisible requires some displayed column of the target inside the
// viewport and in front of the walls. Sprite alpha, shake and yaw affect
// acquisition, not continued ownership of a held attack.
func (g *MMGame) heldMonsterVisible(target *monster.Monster3D) bool {
	if !pointerAttackable(target) {
		return false
	}
	f := &g.gameLoop.renderer.monsterPick
	for _, live := range g.world.Monsters {
		if live != target {
			continue
		}
		for _, hit := range f.hits {
			if hit.monster == target && g.monsterPickHitVisible(f, hit) {
				return true
			}
		}
		break
	}
	return false
}

// column projects one screen column of a hit with the geometry the renderer
// drew, so pointer picking and held-target visibility cannot disagree.
func (f *monsterPickFrame) column(h monsterPickHit, x int) (depth, top, bottom, u float64, ok bool) {
	if h.standee {
		rx, ry := standeeRayAtScreenX(float64(x)+0.5, f.width, f.dirX, f.dirY, f.planeX, f.planeY)
		t, columnU, hit := standeeColumnHit(f.camX, f.camY, rx, ry, h.p0x, h.p0y, h.dx, h.dy)
		if !hit {
			return 0, 0, 0, 0, false
		}
		horizon := float64(f.height) / 2
		bottom = horizon + (h.bottom-horizon)*h.depth/t
		return t, bottom - h.size*h.depth/t, bottom, columnU, true
	}
	if float64(x) < h.left || float64(x) >= h.left+h.size {
		return 0, 0, 0, 0, false
	}
	return h.depth, h.top, h.top + h.size, (float64(x) + 0.5 - h.left) / h.size, true
}

func (g *MMGame) monsterPickHitVisible(f *monsterPickFrame, h monsterPickHit) bool {
	x0, x1 := 0, f.width
	if !h.standee {
		x0, x1 = max(x0, int(math.Floor(h.left))), min(x1, int(math.Ceil(h.left+h.size)))
	}
	for x := x0; x < x1; x++ {
		depth, top, bottom, _, ok := f.column(h, x)
		if !ok || bottom <= 0 || top >= float64(f.height) {
			continue
		}
		if x < len(g.depthBuffer) && standeeColumnOccluded(depth, g.depthBuffer[x], 0) {
			continue
		}
		return true
	}
	return false
}

func (g *MMGame) pickMonsterAtScreen(x, y int) (*monster.Monster3D, float64) {
	if !g.monsterPointerFrameAllowed(x, y) {
		return nil, 0
	}
	f := &g.gameLoop.renderer.monsterPick
	var best *monster.Monster3D
	nearest := math.Inf(1)
	for _, h := range f.hits {
		m := h.monster
		if !pointerAttackable(m) {
			continue
		}
		depth, top, bottom, u, ok := f.column(h, x)
		if !ok {
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

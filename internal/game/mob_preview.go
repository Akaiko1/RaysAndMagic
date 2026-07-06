package game

import (
	"fmt"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// MobPreview is the map editor's live monster stage: a sandbox MMGame with a
// flat arena where the selected monster patrols through the game's real AI,
// banding and renderer. Banding mobs are staged as a whole flock so the band
// stack/patrol animation plays exactly as in the game.
type MobPreview struct {
	g     *MMGame
	scene *ebiten.Image
	arena *world.World3D
	key   string
}

const mobStageMapKey = "mob_stage"

// mobBandSize is how many members a banding monster's preview flock gets.
const mobBandSize = 4

// NewMobPreview builds the sandbox: a flat arena registered under the global
// world manager and a real MMGame whose camera watches the stage.
func NewMobPreview(cfg *config.Config) (*MobPreview, error) {
	if world.GlobalTileManager == nil || monster.MonsterConfig == nil {
		return nil, fmt.Errorf("mob preview: game data not loaded (boot.LoadGameData first)")
	}

	p := &MobPreview{}
	p.arena = buildFlatArena(cfg, 17)
	if world.GlobalWorldManager == nil {
		world.GlobalWorldManager = world.NewWorldManager(cfg)
	}
	world.GlobalWorldManager.LoadedMaps[mobStageMapKey] = p.arena
	world.GlobalWorldManager.CurrentMapKey = mobStageMapKey

	p.g = NewMMGame(cfg)
	p.g.turnBasedMode = false

	ts := float64(cfg.GetTileSize())
	p.g.camera.X, p.g.camera.Y, p.g.camera.Angle = 1.5*ts, 8.5*ts, 0 // west edge, looking east at the stage
	if e := p.g.collisionSystem.GetEntityByID("player"); e != nil {
		p.g.collisionSystem.UpdateEntity("player", p.g.camera.X, p.g.camera.Y)
	}
	return p, nil
}

// Select stages a monster: banding mobs get a whole flock (stacked on one
// tile - the banding pass owns fanning them out), everyone else a single
// specimen. Preview mobs are passive-until-attacked (a zeroed alert radius
// does NOT work: detection falls back to the configured default radius), so
// the calm patrol/band behaviour keeps playing instead of a charge at the
// camera - or, for a ranged band, a scatter/reposition teleport frenzy.
func (p *MobPreview) Select(key string) {
	p.key = key
	g := p.g
	for _, m := range p.arena.Monsters {
		g.collisionSystem.UnregisterEntity(m.ID)
	}
	p.arena.Monsters = p.arena.Monsters[:0]

	def, err := monster.MonsterConfig.GetMonsterByKey(key)
	if err != nil {
		return
	}
	count := 1
	if def.Banding {
		count = mobBandSize
	}
	ts := float64(g.config.GetTileSize())
	// Stage distance scales with the mob's render size so rats fill the frame
	// and elder dragons still fit in it; a tight tether keeps the wander close.
	stageX := g.camera.X + (1.1+0.35*def.GetSizeGameMultiplier())*ts
	for i := 0; i < count; i++ {
		m := monster.NewMonster3DFromConfig(stageX, 8.5*ts, key, g.config)
		m.PassiveUntilAttacked = true
		m.TetherRadius = 2 * ts
		p.arena.Monsters = append(p.arena.Monsters, m)
	}
	p.arena.RegisterMonstersWithCollisionSystem(g.collisionSystem)
}

// Step advances the sandbox one tick - the same monster sub-updates the RT
// game loop runs: movement AI, overlap separation, band flocking, and the
// frame-motion facing pass (without it walkers moonwalk on stale Direction).
func (p *MobPreview) Step() {
	// Editor preview sandboxes share the global world manager; re-pin our stage
	// in case another preview tab switched the current map.
	world.GlobalWorldManager.CurrentMapKey = mobStageMapKey
	g := p.g
	gl := g.gameLoop
	g.frameCount++
	monsterFrameStart := gl.captureMonsterFramePositions()
	gl.updateMonstersParallel()
	gl.faceMonstersAlongFrameMotion(monsterFrameStart) // walk-only window, matching the game loop
	gl.separateOverlappingMonsters()
	gl.updateMonsterBands()
}

// Monsters exposes the staged monsters (the editor shows live HP/state).
func (p *MobPreview) Monsters() []*monster.Monster3D {
	return p.arena.Monsters
}

// Scene renders the sandbox through the real renderer into an offscreen image
// sized to the game's configured resolution; the editor scales it into its
// panel.
func (p *MobPreview) Scene() *ebiten.Image {
	cw, ch := p.g.config.GetScreenWidth(), p.g.config.GetScreenHeight()
	if p.scene == nil || p.scene.Bounds().Dx() != cw || p.scene.Bounds().Dy() != ch {
		p.scene = ebiten.NewImage(cw, ch)
	}
	p.scene.Clear()
	p.g.gameLoop.renderer.RenderFirstPersonView(p.scene)
	return p.scene
}

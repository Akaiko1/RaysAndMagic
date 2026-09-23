package game

import (
	"fmt"
	"math"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared by Mobs and FX. Editor scenes have no world pointer interaction and
// may supply an arena-local caravan route without changing campaign content.
type editorPreviewState struct {
	caravanRoute *config.CaravanRoute
}

// MobPreview is the map editor's live monster stage: a sandbox MMGame with a
// flat arena using the game's movement AI, animations, banding and renderer.
// Invisible stage boundaries keep patrols in view and clear of the camera.
type MobPreview struct {
	g            *MMGame
	scene        *ebiten.Image
	arena        *world.World3D
	key          string
	attackTick   int
	attackFrames int
}

const mobStageMapKey = "mob_stage"

// mobBandSize is how many members a banding monster's preview flock gets.
const mobBandSize = 4

const mobPreviewAttackSeconds = 4.0

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

	p.g = newMMGame(cfg, true)
	p.g.turnBasedMode = false
	p.g.appScreen = AppScreenInGame

	ts := float64(cfg.GetTileSize())
	p.g.camera.X, p.g.camera.Y = 1.5*ts, 8.5*ts // west edge, looking east at the stage
	p.g.snapFacing(0)
	if e := p.g.collisionSystem.GetEntityByID("player"); e != nil {
		p.g.collisionSystem.UpdateEntity("player", p.g.camera.X, p.g.camera.Y)
	}
	return p, nil
}

// Select stages a monster: banding mobs get a whole flock (stacked on one
// tile - the banding pass owns fanning them out), everyone else a single
// specimen. Passive flags keep patrol groups calm; strikes are visual only.
func (p *MobPreview) Select(key string) {
	p.key = key
	g := p.g
	world.GlobalWorldManager.CurrentMapKey = mobStageMapKey
	// Retire the previous specimen's sources and derived standees, including
	// unfinished preparation, before queuing the replacement.
	g.gameLoop.renderer.resetMapRenderResourceResidency()
	p.attackTick, p.attackFrames = 0, 0
	g.editorPreview.caravanRoute = nil
	g.ecology.Checkpoint = 0
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
	// Keep the original close framing; constrain the stage, not the camera.
	stageX := g.camera.X + (1.1+0.35*def.GetSizeGameMultiplier())*ts
	patrolX := p.boundPatrolStage(stageX)
	if def.Disposition == "caravan" {
		x := int(patrolX / ts)
		g.editorPreview.caravanRoute = &config.CaravanRoute{ID: mobStageMapKey, Points: []config.RoutePoint{
			{Map: mobStageMapKey, X: x, Y: 7},
			{Map: mobStageMapKey, X: x + 1, Y: 7},
			{Map: mobStageMapKey, X: x + 1, Y: 9},
			{Map: mobStageMapKey, X: x, Y: 9},
		}}
	}
	for i := 0; i < count; i++ {
		m := monster.NewMonster3DFromConfig(stageX, 8.5*ts, key, g.config)
		if m.IsFish() && config.GlobalEcology != nil && config.GlobalEcology.Fish != nil {
			f := config.GlobalEcology.Fish
			m.FishLeap = &monster.FishLeapState{FromX: m.X, FromY: m.Y, ToX: m.X + ts, ToY: m.Y, Duration: f.FlightSeconds, PeakHeight: f.HeightTiles}
			m.AdvanceFishLeap(0)
		}
		if m.IsChampion() {
			// The editor reads this runtime instance immediately after Select.
			// Mirror now so the first frame and stat sheet never expose the
			// placeholder values in monsters.yaml.
			g.mirrorChampionStats(m)
		}
		m.PassiveUntilAttacked = true
		m.SpawnX = patrolX
		m.TetherRadius = 1.5 * ts
		if m.Speed > 0 {
			m.State = monster.StatePatrolling
		}
		p.arena.Monsters = append(p.arena.Monsters, m)
	}
	p.arena.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	// Metadata lookup does not decode/upload the animation. The common map
	// prewarmer fills later frames in the background; the first frame can use
	// the normal immediate renderer fallback without waiting for the whole set.
	for _, direction := range []string{"attacking_r", "attacking_l"} {
		p.attackFrames = max(p.attackFrames, g.sprites.AnimationFrameCount(p.arena.Monsters[0].GetSpriteType(), direction))
	}
	g.gameLoop.renderer.scheduleMapRenderResourcePrewarm(mobStageMapKey)
}

// Collision-only boundaries leave a two-dimensional patrol clearing. Unlike
// an unrestricted spawn tether, this cannot admit a path through the camera.
// The normal collision snapshot and A* observe the same rails for every mob,
// including flying actors; no post-movement teleport or clamp is needed.
func (p *MobPreview) boundPatrolStage(stageX float64) float64 {
	ts := float64(p.g.config.GetTileSize())
	w, h := float64(p.arena.Width)*ts, float64(p.arena.Height)*ts
	left := math.Floor(stageX/ts-.5) * ts
	right := left + 3*ts
	top, bottom := 7*ts, 10*ts
	for _, rail := range []struct {
		id         string
		x, y, w, h float64
	}{
		{"preview_left", left / 2, h / 2, left, h},
		{"preview_right", (right + w) / 2, h / 2, w - right, h},
		{"preview_top", w / 2, top / 2, w, top},
		{"preview_bottom", w / 2, (bottom + h) / 2, w, h - bottom},
		// Only the near corners lie outside the view. Farther from the camera,
		// both side rows remain available to the normal random patrol AI.
		{"preview_near_top", left + ts/2, top + ts/2, ts, ts},
		{"preview_near_bottom", left + ts/2, bottom - ts/2, ts, ts},
	} {
		p.g.collisionSystem.UnregisterEntity(rail.id)
		p.g.collisionSystem.RegisterEntity(collision.NewEntity(rail.id, rail.x, rail.y, rail.w, rail.h, collision.CollisionTypeNPC, true))
	}
	// Patrol selection excludes home; the surrounding clearing offers multiple
	// reachable destinations in both axes instead of a pair of lane endpoints.
	return left + 1.5*ts
}

// Step advances real patrol movement and animation timers. Only the periodic
// attack demonstration holds movement; it never applies combat damage.
func (p *MobPreview) Step() {
	// Editor preview sandboxes share the global world manager; re-pin our stage
	// in case another preview tab switched the current map.
	world.GlobalWorldManager.CurrentMapKey = mobStageMapKey
	g := p.g
	gl := g.gameLoop
	g.updateInterfacePresentation()
	g.frameCount++
	gl.renderer.prewarmPendingMapRenderResources()
	g.UpdateMonsterHitTintTimers()
	if !p.loading() && p.attackFrames > 0 {
		p.attackTick++
		if p.attackTick >= g.framesForSeconds(mobPreviewAttackSeconds) {
			p.attackTick = 0
			for _, m := range p.arena.Monsters {
				g.armMonsterAttackAnimation(m)
			}
		}
	}
	// Fish have no walking animation. Loop the real flight on the stage without
	// invoking campaign spawning, despawning or loot.
	for _, m := range p.arena.Monsters {
		if m.FishLeap == nil {
			continue
		}
		if m.AdvanceFishLeap(1 / float64(g.config.GetTPS())) {
			m.FishLeap.Progress = 0
			m.AdvanceFishLeap(0)
		}
		g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	}
	attacking := false
	for _, m := range p.arena.Monsters {
		attacking = attacking || m.AttackAnimFrames > 0
	}
	start := gl.captureMonsterFramePositions()
	if !attacking {
		p.advanceCaravanRoute()
		gl.updateMonstersParallel()
	}
	gl.faceMonstersAlongFrameMotion(start)
	gl.updateMonsterBands()
}

// The specimen walks the normal ambient path. Only checkpoint advancement is
// local: previews never run campaign arrivals, trade rewards or respawning.
func (p *MobPreview) advanceCaravanRoute() {
	g := p.g
	route := g.editorPreview.caravanRoute
	if route == nil || len(p.arena.Monsters) == 0 {
		return
	}
	m := p.arena.Monsters[0]
	_, x, y := ecologyPoint(route.Points[g.ecology.Checkpoint], float64(g.config.GetTileSize()))
	if Distance(m.X, m.Y, x, y) <= float64(g.config.GetTileSize())*.1 {
		g.ecology.Checkpoint = (g.ecology.Checkpoint + 1) % len(route.Points)
	}
	g.setCaravanTarget(m)
}

func (p *MobPreview) loading() bool {
	r := p.g.gameLoop.renderer
	return r.mapRenderResourcePrewarmPending || len(r.mapRenderUploadQueue) > 0 || len(r.mapRenderShaderWarmTasks) > 0
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
	r := p.g.gameLoop.renderer
	r.drawMapRenderPrewarmUploads(p.scene)
	r.drawMapRenderShaderWarm(p.scene)
	r.RenderFirstPersonView(p.scene)
	return p.scene
}

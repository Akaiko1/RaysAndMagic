package game

import (
	"cmp"
	"math"
	"math/rand"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

type nightMotePalette struct {
	glow [3]int
	core [3]int
}

func nightMotePaletteForConfig(tile *config.TileData) (nightMotePalette, bool) {
	if tile == nil || tile.NightMotes == nil {
		return nightMotePalette{}, false
	}
	return nightMotePalette{
		glow: tile.NightMotes.GlowColor,
		core: tile.NightMotes.CoreColor,
	}, true
}

type nightMote struct {
	startX, startY   float64
	targetX, targetY float64
	bornTick         int64
	dieTick          int64
	phase            float64
	sizeScale        float64
	source           nightMoteTreeID
	palette          nightMotePalette
}

type nightMoteTreeID struct {
	tileX int
	tileY int
}

type nightMoteCandidate struct {
	treeIndex  int
	distanceSq float64
}

type nightMotePose struct {
	x, y        float64
	heightTiles float64
	alpha       float64
}

func (f nightMote) pose(tick int64, tileSize float64) (nightMotePose, bool) {
	span := f.dieTick - f.bornTick
	if span <= 0 || tick < f.bornTick || tick >= f.dieTick {
		return nightMotePose{}, false
	}
	progress := float64(tick-f.bornTick) / float64(span)
	travel := progress * progress * (3 - 2*progress)
	dx, dy := f.targetX-f.startX, f.targetY-f.startY
	distance := math.Hypot(dx, dy)
	perpX, perpY := 0.0, 0.0
	if distance > 0 {
		perpX, perpY = -dy/distance, dx/distance
	}
	// The mote crosses into one adjacent tile while looping gently around the
	// direct route. Sway tapers to zero at both endpoints.
	sway := math.Sin(progress*4*math.Pi+f.phase) * math.Sin(progress*math.Pi) * tileSize * 0.13
	x := f.startX + dx*travel + perpX*sway
	y := f.startY + dy*travel + perpY*sway
	height := 0.34 + 0.09*math.Sin(progress*6*math.Pi+f.phase)

	fadeIn := math.Min(1, progress/0.10)
	fadeOut := math.Min(1, (1-progress)/0.22)
	flicker := 0.78 + 0.22*math.Sin(float64(tick)*0.17+f.phase*2.3)
	return nightMotePose{x: x, y: y, heightTiles: height, alpha: fadeIn * fadeOut * flicker}, true
}

func (r *Renderer) resetNightMotes() {
	r.nightMotes = r.nightMotes[:0]
	r.nightMoteDraws = r.nightMoteDraws[:0]
	r.nightMoteNextByTree = nil
	r.nightMoteScanTick = 0
	r.nightMoteLastTick = 0
}

func (r *Renderer) nightMotesActive() bool {
	return r != nil && r.game != nil && r.game.dayNightOutdoor && r.game.dayNightIsNight
}

func (r *Renderer) nightMoteMaxDepth() float64 {
	maxDepth := r.game.config.Graphics.NightMotes.EmissionRadiusTiles * float64(r.game.config.GetTileSize())
	if viewDist := r.game.camera.ViewDist; viewDist > 0 && viewDist < maxDepth {
		return viewDist
	}
	return maxDepth
}

func (r *Renderer) nightMoteSpawnInterval() int64 {
	tps := r.game.config.GetTPS()
	if tps <= 0 {
		tps = 120
	}
	return max(1, int64(math.Round(r.game.config.Graphics.NightMotes.EmissionIntervalSeconds*float64(tps))))
}

func randomNightMoteSizeScale() float64 {
	return 0.70 + rand.Float64()*0.60
}

func (r *Renderer) updateNightMotes() {
	if r == nil || r.game == nil || r.game.config == nil {
		return
	}
	tick := r.game.frameCount
	if tick == r.nightMoteLastTick {
		return
	}
	if tick < r.nightMoteLastTick {
		r.resetNightMotes()
	}
	r.nightMoteLastTick = tick
	if !r.nightMotesActive() {
		r.nightMotes = r.nightMotes[:0]
		r.nightMoteNextByTree = nil
		r.nightMoteScanTick = 0
		return
	}

	write := 0
	for i := range r.nightMotes {
		if tick < r.nightMotes[i].dieTick {
			r.nightMotes[write] = r.nightMotes[i]
			write++
		}
	}
	r.nightMotes = r.nightMotes[:write]

	tps := r.game.config.GetTPS()
	if tps <= 0 {
		tps = 120
	}
	if tick < r.nightMoteScanTick {
		return
	}
	r.nightMoteScanTick = tick + int64(max(1, tps/6))
	r.updateNightMoteTrees(tick)
}

func (r *Renderer) updateNightMoteTrees(tick int64) {
	currentWorld := r.game.GetCurrentWorld()
	if currentWorld == nil || len(r.treeTilesCache) == 0 {
		r.nightMoteNextByTree = nil
		return
	}
	if r.nightMoteNextByTree == nil {
		r.nightMoteNextByTree = make(map[nightMoteTreeID]int64)
	}

	activeByTree := r.nightMoteActive
	if activeByTree == nil {
		activeByTree = make(map[nightMoteTreeID]int, r.game.config.Graphics.NightMotes.MaxActive)
		r.nightMoteActive = activeByTree
	} else {
		clear(activeByTree)
	}
	for _, mote := range r.nightMotes {
		activeByTree[mote.source]++
	}
	maxDistance := r.nightMoteMaxDepth()
	maxDistanceSq := maxDistance * maxDistance
	candidates := r.nightMoteCandidates[:0]
	for treeIndex := range r.treeTilesCache {
		tree := &r.treeTilesCache[treeIndex]
		dx := tree.worldX - r.game.camera.X
		dy := tree.worldY - r.game.camera.Y
		distanceSq := dx*dx + dy*dy
		if distanceSq > maxDistanceSq {
			continue
		}
		if !tree.emitsNightMotes {
			continue
		}
		screenX, _, projected := r.game.renderHelper.projectToScreenX(tree.worldX, tree.worldY)
		if !projected || screenX < 0 || screenX >= r.game.config.GetScreenWidth() {
			continue
		}
		candidates = append(candidates, nightMoteCandidate{treeIndex: treeIndex, distanceSq: distanceSq})
	}
	slices.SortFunc(candidates, func(a, b nightMoteCandidate) int {
		return cmp.Compare(a.distanceSq, b.distanceSq)
	})
	r.nightMoteCandidates = candidates

	seen := r.nightMoteSeen
	if seen == nil {
		seen = make(map[nightMoteTreeID]struct{}, r.game.config.Graphics.NightMotes.MaxActive)
		r.nightMoteSeen = seen
	} else {
		clear(seen)
	}
	for _, candidate := range candidates {
		tree := &r.treeTilesCache[candidate.treeIndex]
		id := nightMoteTreeID{tileX: tree.tileX, tileY: tree.tileY}
		seen[id] = struct{}{}
		nextTick, scheduled := r.nightMoteNextByTree[id]
		if !scheduled {
			// Stagger only the first roll. Later rolls stay exactly one configured
			// interval apart, but trees entering range together do not pulse in lock-step.
			interval := r.nightMoteSpawnInterval()
			r.nightMoteNextByTree[id] = tick + rand.Int63n(interval)
			continue
		}
		if tick < nextTick {
			continue
		}
		r.nightMoteNextByTree[id] = tick + r.nightMoteSpawnInterval()
		if activeByTree[id] >= r.game.config.Graphics.NightMotes.MaxPerTree {
			continue
		}
		if rand.Float64() >= r.game.config.Graphics.NightMotes.EmissionChance {
			continue
		}
		if r.spawnNightMote(tick, tree, currentWorld) {
			activeByTree[id]++
		}
	}
	for id := range r.nightMoteNextByTree {
		if _, ok := seen[id]; !ok {
			delete(r.nightMoteNextByTree, id)
		}
	}
}

func (r *Renderer) spawnNightMote(tick int64, chosen *TransparentSpriteData, currentWorld *world.World3D) bool {
	if len(r.nightMotes) >= r.game.config.Graphics.NightMotes.MaxActive {
		return false
	}
	tileSize := float64(r.game.config.GetTileSize())

	directions := [...]struct{ x, y int }{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}
	start := rand.Intn(len(directions))
	var targetTileX, targetTileY int
	found := false
	for i := range directions {
		d := directions[(start+i)%len(directions)]
		tx, ty := chosen.tileX+d.x, chosen.tileY+d.y
		if tx >= 0 && tx < currentWorld.Width && ty >= 0 && ty < currentWorld.Height &&
			!currentWorld.IsTileBlockingTerrainAt(tx, ty) {
			targetTileX, targetTileY, found = tx, ty, true
			break
		}
	}
	if !found {
		return false
	}

	targetX, targetY := TileCenterFromTile(targetTileX, targetTileY, tileSize)
	jitter := tileSize * 0.14
	targetX += (rand.Float64()*2 - 1) * jitter
	targetY += (rand.Float64()*2 - 1) * jitter
	tps := r.game.config.GetTPS()
	if tps <= 0 {
		tps = 120
	}
	r.nightMotes = append(r.nightMotes, nightMote{
		startX: chosen.worldX, startY: chosen.worldY,
		targetX: targetX, targetY: targetY,
		bornTick:  tick,
		dieTick:   tick + max(1, int64(math.Round(r.game.config.Graphics.NightMotes.LifetimeSeconds*float64(tps)))),
		phase:     rand.Float64() * 2 * math.Pi,
		sizeScale: randomNightMoteSizeScale(),
		source:    nightMoteTreeID{tileX: chosen.tileX, tileY: chosen.tileY},
		palette:   chosen.nightMotePalette,
	})
	return true
}

type nightMoteDraw struct {
	x, y               float64
	glowSize, coreSize float64
	alpha              float64
	palette            nightMotePalette
}

func (r *Renderer) nightMoteHasLineOfSight(x, y float64) bool {
	return r.game.collisionSystem == nil ||
		r.game.collisionSystem.CheckLineOfSight(r.game.camera.X, r.game.camera.Y, x, y)
}

func (r *Renderer) drawNightMotes(screen *ebiten.Image) {
	if !r.nightMotesActive() || len(r.nightMotes) == 0 {
		return
	}
	tileSize := float64(r.game.config.GetTileSize())
	screenH := float64(r.game.config.GetScreenHeight())
	maxDepth := r.nightMoteMaxDepth()
	draws := r.nightMoteDraws[:0]
	for _, mote := range r.nightMotes {
		pose, ok := mote.pose(r.game.frameCount, tileSize)
		if !ok || pose.alpha <= 0 {
			continue
		}
		screenX, depth, visible := r.game.renderHelper.projectToScreenX(pose.x, pose.y)
		if !visible || depth <= 0 || depth >= maxDepth || screenX < 0 || screenX >= len(r.game.depthBuffer) {
			continue
		}
		if screenX < len(r.game.actorDepthBuffer) && depth >= r.game.actorDepthBuffer[screenX] {
			continue
		}
		if depth >= r.game.depthBuffer[screenX] {
			continue
		}
		if !r.nightMoteHasLineOfSight(pose.x, pose.y) {
			continue
		}
		// Ground is 0.5 tiles below eye level. Lift the mote by its authored
		// hover height using the same perspective relation as spell particles.
		screenY := screenH/2 + (0.5-pose.heightTiles)*tileSize*screenH/depth
		glowSize := tileSize * 0.075 * screenH / depth * mote.sizeScale
		if glowSize < 3 {
			glowSize = 3
		} else if glowSize > 18 {
			glowSize = 18
		}
		coreSize := math.Max(1.25, glowSize*0.16)
		distanceAlpha := 1 - depth/maxDepth
		alpha := pose.alpha * distanceAlpha
		if alpha <= 0.01 {
			continue
		}
		draws = append(draws, nightMoteDraw{
			x: float64(screenX), y: screenY,
			glowSize: glowSize, coreSize: coreSize,
			alpha:   alpha,
			palette: mote.palette,
		})
	}
	r.nightMoteDraws = draws
	for i := range draws {
		d := &draws[i]
		r.drawGlowSprite(screen, d.x, d.y, d.glowSize, d.palette.glow, 0.58*d.alpha, additiveGlowBlend)
	}
	for i := range draws {
		d := &draws[i]
		r.drawGlowRect(screen, d.x, d.y, d.coreSize, d.palette.core, d.alpha, additiveGlowBlend)
	}
}

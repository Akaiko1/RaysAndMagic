package game

import (
	"context"
	"image"
	"image/draw"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// mapRenderPrewarmPlan is the data-only inventory of resources that can render
// in the current world. It is built from the map caches, live/authored monster
// rosters, NPC definitions, and container sprite index. Keeping discovery
// separate from GPU preparation makes the coverage testable without a Draw
// frame and keeps the prewarmer independent of camera position.
type mapRenderPrewarmPlan struct {
	tileTypes          []world.TileType3D
	tileSprites        []string
	wallSprites        []string
	treeSprites        []string
	crossedPropSprites []string
	environmentSprites []processedSpriteKey
	npcDecodeSprites   []string
	npcSprites         []mapNPCPrewarmResource
	monsterDecode      []mapMonsterPrewarmResource
	monsterSprites     []mapMonsterPrewarmResource
	containerDecode    []string
	containerSprites   []string
}

type mapNPCPrewarmResource struct {
	name              string
	prefix            string
	stableImage       bool
	warmVisibleBounds bool
}

type mapMonsterPrewarmResource struct {
	key        string
	spriteName string
}

type mapRenderSourceKey struct {
	name          string
	animationType string
}

// mapRenderRegionResources is the single ownership manifest for one logical
// region. The same manifest drives retention and eviction, so resource scope
// cannot drift between source images and their derived render caches.
type mapRenderRegionResources struct {
	standees  map[standeeCoreKey]struct{}
	sources   map[mapRenderSourceKey]struct{}
	processed map[processedSpriteKey]struct{}
	walls     map[*ebiten.Image]struct{}
	skies     map[string]struct{}
}

type mapRenderPrewarmStats struct {
	spriteFiles     int
	animationSheets int
	standeeFrames   int
	wallTextures    int
	uploadImages    int
}

type mapRenderPrewarmTask struct {
	mapKey             string
	plan               mapRenderPrewarmPlan
	prewarmer          *mapRenderPrewarmer
	preparedSprites    <-chan graphics.PreparedSpriteResource
	spriteCommit       *graphics.PreparedSpriteCommit
	spriteCommitReq    graphics.SpriteResourceRequest
	spriteCommitFound  bool
	preparedSkies      <-chan mapRenderPreparedSky
	skyCommit          *mapRenderSkyCommit
	spritesDone        bool
	skiesDone          bool
	steps              []mapRenderPrewarmStep
	nextStep           int
	ctx                context.Context
	cancel             context.CancelFunc
	cancelled          bool
	committed          bool
	cpuImages          map[*ebiten.Image]*image.RGBA
	standeeJobs        []mapRenderStandeeJob
	wallRipmapBuilders []*mapRenderWallRipmapBuilder
	preparedStandees   <-chan mapRenderPreparedStandee
	standeeCommit      *mapRenderStandeeCommit
	standeesDone       bool
}

type mapRenderPrewarmStep func(deadline time.Time) bool

type mapRenderStandeeJob struct {
	key    standeeCoreKey
	source *ebiten.Image
	cpu    *image.RGBA
}

type mapRenderPreparedStandee struct {
	key      standeeCoreKey
	source   *ebiten.Image
	prepared standeePreparedPixels
}

type mapRenderImageWrite struct {
	image *ebiten.Image
	cpu   *image.RGBA
	row   int
}

// mapRenderStandeeCommit keeps all GPU images private until every mip level is
// populated. It makes a multi-megabyte standee chain obey the same per-Update
// pixel budget as decoded sprites and gives cancellation an explicit owner for
// every in-flight image.
type mapRenderStandeeCommit struct {
	key           standeeCoreKey
	source        *ebiten.Image
	sticker       *ebiten.Image
	core          *ebiten.Image
	stickerLevels []*ebiten.Image
	coreLevels    []*ebiten.Image
	writes        []mapRenderImageWrite
	owned         []*ebiten.Image
	done          bool
}

func newMapRenderStandeeCommit(prepared mapRenderPreparedStandee) *mapRenderStandeeCommit {
	c := &mapRenderStandeeCommit{key: prepared.key, source: prepared.source}
	if prepared.source == nil || prepared.prepared.sticker == nil || prepared.prepared.core == nil {
		c.done = true
		return c
	}
	makeImage := func(cpu *image.RGBA) *ebiten.Image {
		if cpu == nil || cpu.Bounds().Dx() <= 0 || cpu.Bounds().Dy() <= 0 {
			return nil
		}
		img := ebiten.NewImage(cpu.Bounds().Dx(), cpu.Bounds().Dy())
		c.writes = append(c.writes, mapRenderImageWrite{image: img, cpu: cpu})
		c.owned = append(c.owned, img)
		return img
	}
	c.sticker = prepared.source
	if standeeMipBaseNeedsCopy(prepared.source, prepared.prepared.sticker) {
		c.sticker = makeImage(prepared.prepared.sticker)
	}
	c.core = makeImage(prepared.prepared.core)
	if c.sticker == nil || c.core == nil {
		c.cancel()
		return c
	}
	c.stickerLevels = append(c.stickerLevels, c.sticker)
	for _, cpu := range prepared.prepared.stickerMips[1:] {
		if level := makeImage(cpu); level != nil {
			c.stickerLevels = append(c.stickerLevels, level)
		}
	}
	c.coreLevels = append(c.coreLevels, c.core)
	for _, cpu := range prepared.prepared.coreMips[1:] {
		if level := makeImage(cpu); level != nil {
			c.coreLevels = append(c.coreLevels, level)
		}
	}
	return c
}

func (c *mapRenderStandeeCommit) advance(r *Renderer, maxBytes int) (*ebiten.Image, *ebiten.Image, bool) {
	if c == nil || c.done {
		if c == nil {
			return nil, nil, true
		}
		return c.sticker, c.core, true
	}
	budget := maxBytes
	for len(c.writes) > 0 && (maxBytes <= 0 || budget > 0) {
		write := &c.writes[0]
		bounds := write.cpu.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		rowBytes := 4 * width
		rows := height - write.row
		if maxBytes > 0 {
			rows = min(rows, max(1, budget/rowBytes))
		}
		start := write.cpu.PixOffset(bounds.Min.X, bounds.Min.Y+write.row)
		end := start + rows*write.cpu.Stride
		region := image.Rect(0, write.row, width, write.row+rows)
		write.image.SubImage(region).(*ebiten.Image).WritePixels(write.cpu.Pix[start:end])
		write.row += rows
		if maxBytes > 0 {
			budget -= rows * rowBytes
		}
		if write.row < height {
			break
		}
		c.writes[0] = mapRenderImageWrite{}
		c.writes = c.writes[1:]
	}
	if len(c.writes) > 0 {
		return nil, nil, false
	}
	if existing := r.standeeCoreCache[c.key]; existing != nil {
		sticker := c.source
		if bounded := r.standeeRenderSourceCache[c.key]; bounded != nil {
			sticker = bounded
		}
		c.cancel()
		c.sticker, c.core, c.done = sticker, existing, true
		r.trackResidentStandeeKey(c.key, c.source)
		return sticker, existing, true
	}
	if c.sticker != c.source {
		if r.standeeRenderSourceCache == nil {
			r.standeeRenderSourceCache = make(map[standeeCoreKey]*ebiten.Image)
		}
		r.standeeRenderSourceCache[c.key] = c.sticker
	}
	if r.standeeCoreCache == nil {
		r.standeeCoreCache = make(map[standeeCoreKey]*ebiten.Image)
	}
	r.standeeCoreCache[c.key] = c.core
	if r.standeeMipCache == nil {
		r.standeeMipCache = make(map[standeeMipKey]*mipChain)
	}
	stickerOwned := append([]*ebiten.Image(nil), c.stickerLevels[1:]...)
	if c.sticker != c.source {
		stickerOwned = append(stickerOwned, c.sticker)
	}
	coreOwned := append([]*ebiten.Image(nil), c.coreLevels...)
	r.standeeMipCache[standeeMipKey{frame: c.key, layer: standeeMipSticker}] = &mipChain{
		levels: c.stickerLevels, owned: stickerOwned,
	}
	r.standeeMipCache[standeeMipKey{frame: c.key, layer: standeeMipCore}] = &mipChain{
		levels: c.coreLevels, owned: coreOwned,
	}
	c.owned = nil
	c.done = true
	r.trackResidentStandeeKey(c.key, c.source)
	return c.sticker, c.core, true
}

func (c *mapRenderStandeeCommit) cancel() {
	if c == nil || c.done {
		return
	}
	seen := make(map[*ebiten.Image]struct{}, len(c.owned))
	for _, img := range c.owned {
		if img == nil {
			continue
		}
		if _, duplicate := seen[img]; duplicate {
			continue
		}
		seen[img] = struct{}{}
		img.Deallocate()
	}
	c.writes = nil
	c.owned = nil
	c.stickerLevels = nil
	c.coreLevels = nil
	c.sticker = nil
	c.core = nil
	c.done = true
}

const (
	mapRenderSpriteCommitFrameBytes = 256 << 10
	mapRenderDerivedFrameBudget     = 1500 * time.Microsecond
	mapRenderLoadMarginInTiles      = 4
	mapRenderLoadFOVMargin          = 20 * math.Pi / 180
)

type mapRenderPreparedSky struct {
	name  string
	image *image.RGBA
}

type mapRenderSkyCommit struct {
	name  string
	image *ebiten.Image
	cpu   *image.RGBA
	row   int
}

func (c *mapRenderSkyCommit) cancel() {
	if c == nil {
		return
	}
	if c.image != nil {
		c.image.Deallocate()
	}
	c.image = nil
	c.cpu = nil
	c.row = 0
}

func (c *mapRenderSkyCommit) advance(maxBytes int) bool {
	if c == nil || c.image == nil || c.cpu == nil {
		return true
	}
	bounds := c.cpu.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || c.row >= height {
		return true
	}
	rowBytes := 4 * width
	rows := min(height-c.row, max(1, maxBytes/rowBytes))
	start := c.cpu.PixOffset(bounds.Min.X, bounds.Min.Y+c.row)
	end := start + rows*c.cpu.Stride
	region := image.Rect(0, c.row, width, c.row+rows)
	c.image.SubImage(region).(*ebiten.Image).WritePixels(c.cpu.Pix[start:end])
	c.row += rows
	return c.row >= height
}

type mapRenderUpload struct {
	image *ebiten.Image
	task  *mapRenderPrewarmTask
}

type mapRenderPrewarmScope struct {
	mapKey   string
	region   *world.OpenWorldRegion
	tileSize float64
}

func (s mapRenderPrewarmScope) containsTile(tx, ty int) bool {
	if s.region == nil {
		return true
	}
	return tx >= s.region.OffsetX && tx < s.region.OffsetX+s.region.Width &&
		ty >= s.region.OffsetY && ty < s.region.OffsetY+s.region.Height
}

func (s mapRenderPrewarmScope) containsWorld(x, y float64) bool {
	if s.region == nil || s.tileSize <= 0 {
		return true
	}
	return s.containsTile(int(x/s.tileSize), int(y/s.tileSize))
}

func (r *Renderer) mapRenderPrewarmScope(mapKey string) mapRenderPrewarmScope {
	currentWorld := r.game.GetCurrentWorld()
	scope := mapRenderPrewarmScope{
		mapKey:   mapKey,
		tileSize: float64(r.game.config.GetTileSize()),
	}
	if wm := world.GlobalWorldManager; wm != nil && currentWorld == wm.OpenWorld {
		scope.region = wm.OpenWorldRegionByKey(mapKey)
	}
	return scope
}

func normalizedAuthoredSpriteName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".png")
	if name == "" || name == "none" {
		return ""
	}
	return name
}

func sortedStringSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// collectMapRenderPrewarmPlan enumerates every resource the requested logical
// map can reveal later. In the unified open world, mapKey scopes both source
// images and derived resources to one placed region; the previous region stays
// resident across a seam. In particular it includes authored respawns, both
// day/night packs, NPC-triggered and boss-triggered summons, visited NPC art,
// encounter reward chests, and the indexed loot-bag family.
func (r *Renderer) collectMapRenderPrewarmPlan(mapKey string) mapRenderPrewarmPlan {
	return r.collectMapRenderPrewarmPlanForScope(r.mapRenderPrewarmScope(mapKey))
}

func (r *Renderer) collectMapRenderPrewarmPlanForScope(scope mapRenderPrewarmScope) mapRenderPrewarmPlan {
	var plan mapRenderPrewarmPlan
	if r == nil || r.game == nil || r.game.sprites == nil {
		return plan
	}
	currentWorld := r.game.GetCurrentWorld()
	if currentWorld == nil {
		return plan
	}
	mapKey := scope.mapKey

	tileTypes := make(map[world.TileType3D]struct{})
	tileSprites := make(map[string]struct{})
	wallSprites := make(map[string]struct{})
	treeSprites := make(map[string]struct{})
	crossedPropSprites := make(map[string]struct{})
	environmentSprites := make(map[processedSpriteKey]struct{})
	npcDecodeSprites := make(map[string]struct{})
	npcSprites := make(map[mapNPCPrewarmResource]struct{})
	monsterDecode := make(map[mapMonsterPrewarmResource]struct{})
	monsterSprites := make(map[mapMonsterPrewarmResource]struct{})
	decodeMonsterKeys := make(map[string]struct{})
	monsterKeys := make(map[string]struct{})
	containerDecode := make(map[string]struct{})
	containerSprites := make(map[string]struct{})

	addContainerSprite := func(name string) {
		name = normalizedAuthoredSpriteName(name)
		if name == "" {
			return
		}
		containerDecode[name] = struct{}{}
		containerSprites[name] = struct{}{}
	}
	addRewardSprites := func(rewards *monster.EncounterRewards, inScope bool) {
		if rewards == nil || !inScope {
			return
		}
		if rewards.TreasureChest != nil {
			addContainerSprite(rewards.TreasureChest.Sprite)
		}
		for i := range rewards.TreasureChests {
			addContainerSprite(rewards.TreasureChests[i].Sprite)
		}
	}

	if tm := world.GlobalTileManager; tm != nil {
		if scope.region == nil {
			for _, tileType := range r.mapRenderTileTypes {
				tileTypes[tileType] = struct{}{}
			}
		} else {
			// The stitched world caches cover every region. Inventory only tiles
			// physically placed in this logical region; the retained neighbour
			// provides the art visible across a seam.
			for ty := scope.region.OffsetY; ty < scope.region.OffsetY+scope.region.Height; ty++ {
				for tx := scope.region.OffsetX; tx < scope.region.OffsetX+scope.region.Width; tx++ {
					x, y := TileCenterFromTile(tx, ty, scope.tileSize)
					tileTypes[currentWorld.GetTileAt(x, y)] = struct{}{}
				}
			}
		}
		for tileType := range tileTypes {
			if isFireflySwarmTile(tileType) {
				continue // procedural motes; the legacy PNG is never drawn
			}
			renderType := tm.GetRenderType(tileType)
			switch renderType {
			case config.TileRenderWall, config.TileRenderCrossedStandee, config.TileRenderCrossedProp,
				config.TileRenderStandee, config.TileRenderLandmarkStandee:
			default:
				continue
			}
			name := normalizedAuthoredSpriteName(tm.GetSprite(tileType))
			if name == "" {
				continue
			}
			tileSprites[name] = struct{}{}
			if renderType == config.TileRenderWall {
				wallSprites[name] = struct{}{}
			}
		}
		for i := range r.treeTilesCache {
			if !scope.containsTile(r.treeTilesCache[i].tileX, r.treeTilesCache[i].tileY) {
				continue
			}
			name := r.treeTilesCache[i].spriteName
			if name == "" {
				name = treeStandeeSpriteName(r.treeTilesCache[i].tileType)
			}
			if name = normalizedAuthoredSpriteName(name); name == "" {
				continue
			}
			// Prop crosses are tracked apart from trees: they draw as crosses
			// whatever trees_as_billboards says, so their prewarm cannot hide
			// behind that flag.
			if tm.GetRenderType(r.treeTilesCache[i].tileType) == config.TileRenderCrossedProp {
				crossedPropSprites[name] = struct{}{}
				continue
			}
			treeSprites[name] = struct{}{}
		}
		for i := range r.transparentSpritesCache {
			resource := r.transparentSpritesCache[i]
			if !scope.containsTile(resource.tileX, resource.tileY) {
				continue
			}
			if isFireflySwarmTile(resource.tileType) {
				continue
			}
			resource.spriteName = normalizedAuthoredSpriteName(resource.spriteName)
			if resource.spriteName != "" {
				environmentSprites[processedSpriteKey{
					tileType:   resource.tileType,
					spriteName: resource.spriteName,
				}] = struct{}{}
			}
		}
	}

	for _, npc := range currentWorld.NPCs {
		if npc == nil {
			continue
		}
		inScope := scope.containsWorld(npc.X, npc.Y)
		baseName := normalizedAuthoredSpriteName(npc.Sprite)
		visitedName := normalizedAuthoredSpriteName(npc.VisitedSprite)
		if baseName != "" || visitedName != "" {
			category := npcRenderCatOf(npc)
			prefix, stableImage := "npc", false
			// Validation guarantees a landmark never carries grid_span_tiles.
			if category == catLandmark {
				prefix, stableImage = "landmark", true
			}
			warmBounds := npc.SizeClass != "" && category != catNPC
			for _, name := range []string{baseName, visitedName} {
				if name == "" || !inScope {
					continue
				}
				npcDecodeSprites[name] = struct{}{}
				npcSprites[mapNPCPrewarmResource{
					name: name, prefix: prefix, stableImage: stableImage,
					warmVisibleBounds: warmBounds,
				}] = struct{}{}
			}
		}
		for _, summon := range npc.Summons {
			if inScope && summon != nil && summon.Monster != "" {
				decodeMonsterKeys[summon.Monster] = struct{}{}
				monsterKeys[summon.Monster] = struct{}{}
			}
		}
		if encounter := npc.EncounterData; encounter != nil {
			for _, encounterMonster := range encounter.Monsters {
				if encounterMonster == nil || encounterMonster.Type == "" {
					continue
				}
				if inScope {
					decodeMonsterKeys[encounterMonster.Type] = struct{}{}
					monsterKeys[encounterMonster.Type] = struct{}{}
				}
			}
			addRewardSprites(encounter.Rewards, inScope)
		}
	}

	// Seed exact live sprite overrides first, then resolve every key through the
	// YAML definition below so its potential summons are included recursively.
	for _, mon := range currentWorld.Monsters {
		if mon == nil {
			continue
		}
		inScope := scope.containsWorld(mon.X, mon.Y)
		if name := normalizedAuthoredSpriteName(mon.GetSpriteType()); inScope && name != "" {
			resource := mapMonsterPrewarmResource{key: mon.Key, spriteName: name}
			monsterDecode[resource] = struct{}{}
			monsterSprites[resource] = struct{}{}
		}
		if inScope && mon.Key != "" {
			decodeMonsterKeys[mon.Key] = struct{}{}
			monsterKeys[mon.Key] = struct{}{}
		}
		for _, key := range mon.SummonMonsters {
			if inScope && key != "" {
				decodeMonsterKeys[key] = struct{}{}
				monsterKeys[key] = struct{}{}
			}
		}
		addRewardSprites(mon.EncounterRewards, inScope)
	}
	for _, spawn := range currentWorld.MonsterSpawns {
		if spawn.MonsterKey == "" {
			continue
		}
		if scope.containsTile(spawn.X, spawn.Y) {
			decodeMonsterKeys[spawn.MonsterKey] = struct{}{}
			monsterKeys[spawn.MonsterKey] = struct{}{}
		}
	}

	for _, pack := range r.game.config.DayNight.Packs {
		if pack.Map != mapKey {
			continue
		}
		for _, night := range []bool{false, true} {
			for _, member := range pack.PhaseMembers(night) {
				if member.Monster != "" {
					decodeMonsterKeys[member.Monster] = struct{}{}
					monsterKeys[member.Monster] = struct{}{}
				}
			}
		}
	}

	// Resolve summon chains iteratively. A malformed cycle is harmless because
	// resolved is the recursion guard; content validation owns errors.
	resolveMonsterResources := func(keys map[string]struct{}) map[mapMonsterPrewarmResource]struct{} {
		resources := make(map[mapMonsterPrewarmResource]struct{})
		resolved := make(map[string]struct{})
		for len(keys) > 0 {
			var key string
			for key = range keys {
				break
			}
			delete(keys, key)
			if _, done := resolved[key]; done || key == "" {
				continue
			}
			resolved[key] = struct{}{}
			if monster.MonsterConfig == nil {
				continue
			}
			def, err := monster.MonsterConfig.GetMonsterByKey(key)
			if err != nil || def == nil {
				continue
			}
			if name := normalizedAuthoredSpriteName(def.GetSpriteFromConfig()); name != "" {
				resources[mapMonsterPrewarmResource{key: key, spriteName: name}] = struct{}{}
			}
			for _, summonKey := range def.SummonMonsters {
				if summonKey != "" {
					keys[summonKey] = struct{}{}
				}
			}
		}
		return resources
	}
	for resource := range resolveMonsterResources(decodeMonsterKeys) {
		monsterDecode[resource] = struct{}{}
	}
	for resource := range resolveMonsterResources(monsterKeys) {
		monsterSprites[resource] = struct{}{}
	}

	for _, defaults := range groundContainerDefaults {
		addContainerSprite(defaults.sprite)
	}
	for i := range r.game.groundContainers {
		container := &r.game.groundContainers[i]
		if scope.containsWorld(container.X, container.Y) {
			addContainerSprite(container.effectiveSprite())
		}
	}
	// Every rarity bag can be created by a kill on any map. Chest art is
	// discovered from authored NPC/reward/current-container data above; loading
	// unrelated chest families here only bloats the open-world residency set.
	for _, name := range r.game.sprites.SpriteNamesWithPrefix("bag_") {
		addContainerSprite(name)
	}

	for tileType := range tileTypes {
		plan.tileTypes = append(plan.tileTypes, tileType)
	}
	sort.Slice(plan.tileTypes, func(i, j int) bool { return plan.tileTypes[i] < plan.tileTypes[j] })
	plan.tileSprites = sortedStringSet(tileSprites)
	plan.wallSprites = sortedStringSet(wallSprites)
	plan.treeSprites = sortedStringSet(treeSprites)
	plan.crossedPropSprites = sortedStringSet(crossedPropSprites)
	plan.npcDecodeSprites = sortedStringSet(npcDecodeSprites)
	plan.containerDecode = sortedStringSet(containerDecode)
	plan.containerSprites = sortedStringSet(containerSprites)
	for resource := range environmentSprites {
		plan.environmentSprites = append(plan.environmentSprites, resource)
	}
	sort.Slice(plan.environmentSprites, func(i, j int) bool {
		if plan.environmentSprites[i].tileType != plan.environmentSprites[j].tileType {
			return plan.environmentSprites[i].tileType < plan.environmentSprites[j].tileType
		}
		return plan.environmentSprites[i].spriteName < plan.environmentSprites[j].spriteName
	})
	for resource := range npcSprites {
		plan.npcSprites = append(plan.npcSprites, resource)
	}
	sort.Slice(plan.npcSprites, func(i, j int) bool {
		if plan.npcSprites[i].name != plan.npcSprites[j].name {
			return plan.npcSprites[i].name < plan.npcSprites[j].name
		}
		return plan.npcSprites[i].prefix < plan.npcSprites[j].prefix
	})
	for resource := range monsterSprites {
		if resource.spriteName != "" {
			plan.monsterSprites = append(plan.monsterSprites, resource)
		}
	}
	sort.Slice(plan.monsterSprites, func(i, j int) bool {
		if plan.monsterSprites[i].key != plan.monsterSprites[j].key {
			return plan.monsterSprites[i].key < plan.monsterSprites[j].key
		}
		return plan.monsterSprites[i].spriteName < plan.monsterSprites[j].spriteName
	})
	for resource := range monsterDecode {
		if resource.spriteName != "" {
			plan.monsterDecode = append(plan.monsterDecode, resource)
		}
	}
	sort.Slice(plan.monsterDecode, func(i, j int) bool {
		if plan.monsterDecode[i].key != plan.monsterDecode[j].key {
			return plan.monsterDecode[i].key < plan.monsterDecode[j].key
		}
		return plan.monsterDecode[i].spriteName < plan.monsterDecode[j].spriteName
	})
	return plan
}

type mapRenderPrewarmer struct {
	renderer          *Renderer
	task              *mapRenderPrewarmTask
	imagesByName      map[string]*ebiten.Image
	uploads           map[*ebiten.Image]struct{}
	resources         *mapRenderRegionResources
	animations        map[[2]string]struct{}
	shaderStickerMips *mipChain
	shaderCoreMips    *mipChain
	stats             mapRenderPrewarmStats
	lastResource      string
}

func newMapRenderPrewarmer(r *Renderer, task *mapRenderPrewarmTask) *mapRenderPrewarmer {
	return &mapRenderPrewarmer{
		renderer:     r,
		task:         task,
		imagesByName: make(map[string]*ebiten.Image),
		uploads:      make(map[*ebiten.Image]struct{}),
		resources: &mapRenderRegionResources{
			standees:  make(map[standeeCoreKey]struct{}),
			sources:   make(map[mapRenderSourceKey]struct{}),
			processed: make(map[processedSpriteKey]struct{}),
			walls:     make(map[*ebiten.Image]struct{}),
			skies:     make(map[string]struct{}),
		},
		animations: make(map[[2]string]struct{}),
	}
}

func (p *mapRenderPrewarmer) addUpload(img *ebiten.Image) {
	if img == nil {
		return
	}
	if _, seen := p.uploads[img]; seen {
		return
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return
	}
	p.uploads[img] = struct{}{}
}

func (p *mapRenderPrewarmer) sprite(name string) *ebiten.Image {
	if name == "" {
		return nil
	}
	if img, resolved := p.imagesByName[name]; resolved {
		return img
	}
	p.imagesByName[name] = nil
	if !p.renderer.game.sprites.HasSprite(name) {
		return nil
	}
	img := p.renderer.game.sprites.GetSprite(name)
	p.imagesByName[name] = img
	p.resources.sources[mapRenderSourceKey{name: name}] = struct{}{}
	p.addUpload(img)
	p.stats.spriteFiles++
	return img
}

func (p *mapRenderPrewarmer) processedSprite(resource processedSpriteKey) *ebiten.Image {
	base := p.sprite(resource.spriteName)
	if base == nil || world.GlobalTileManager == nil {
		return base
	}
	data := world.GlobalTileManager.GetTileData(resource.tileType)
	if data == nil || data.AlphaFromBrightness <= 0 {
		return base
	}
	if cached, ok := p.renderer.processedSpriteCache[resource]; ok {
		return cached
	}
	cpu := p.cpuImage(base)
	if cpu == nil {
		return p.renderer.getProcessedSpriteByName(resource.tileType, resource.spriteName)
	}
	processed, processedCPU := applyBrightnessToAlphaCPU(cpu, data.AlphaFromBrightness)
	p.renderer.processedSpriteCache[resource] = processed
	if processed != nil && processedCPU != nil {
		p.task.cpuImages[processed] = processedCPU
	}
	return processed
}

func (p *mapRenderPrewarmer) auraTileColor(tileType world.TileType3D) {
	if _, cached := p.renderer.auraTileColorCache[tileType]; cached || world.GlobalTileManager == nil {
		return
	}
	sprite := p.sprite(world.GlobalTileManager.GetSprite(tileType))
	cpu := p.cpuImage(sprite)
	if cpu == nil {
		p.renderer.auraTileColor(tileType)
		return
	}
	if p.renderer.auraTileColorCache == nil {
		p.renderer.auraTileColorCache = make(map[world.TileType3D][3]int)
	}
	if rgb, ok := computeAuraTileColorFromPixels(cpu); ok {
		p.renderer.auraTileColorCache[tileType] = rgb
	} else {
		p.renderer.auraTileColorCache[tileType] = [3]int{-1, -1, -1}
	}
}

func (p *mapRenderPrewarmer) animationFrames(name, animationType string) []*ebiten.Image {
	cacheKey := [2]string{name, animationType}
	if _, resolved := p.animations[cacheKey]; resolved {
		if anim := p.renderer.game.sprites.GetAnimation(name, animationType); anim != nil {
			return anim.Frames
		}
		return nil
	}
	p.animations[cacheKey] = struct{}{}
	anim := p.renderer.game.sprites.GetAnimation(name, animationType)
	if anim == nil || len(anim.Frames) == 0 {
		return nil
	}
	p.resources.sources[mapRenderSourceKey{name: name, animationType: animationType}] = struct{}{}
	p.stats.animationSheets++
	for _, frame := range anim.Frames {
		p.addUpload(frame)
	}
	return anim.Frames
}

func (p *mapRenderPrewarmer) cpuImage(img *ebiten.Image) *image.RGBA {
	if p == nil || p.task == nil || img == nil {
		return nil
	}
	return p.task.cpuImages[img]
}

// rememberCPUFrames mirrors Renderer.animationFrames without reading any
// newly committed Ebiten image back from the GPU. Static environment and NPC
// sheets use SubImages, while monster animations already arrive as individual
// CPU-backed frames from SpriteManager.
func (p *mapRenderPrewarmer) rememberCPUFrames(sprite *ebiten.Image, frames []*ebiten.Image) {
	if p == nil || p.task == nil || sprite == nil {
		return
	}
	sheet := p.task.cpuImages[sprite]
	if sheet == nil {
		return
	}
	sheetBounds := sprite.Bounds()
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		frameBounds := frame.Bounds()
		srcMin := image.Pt(
			sheet.Bounds().Min.X+frameBounds.Min.X-sheetBounds.Min.X,
			sheet.Bounds().Min.Y+frameBounds.Min.Y-sheetBounds.Min.Y,
		)
		srcRect := image.Rectangle{Min: srcMin, Max: srcMin.Add(frameBounds.Size())}
		if !srcRect.In(sheet.Bounds()) {
			continue
		}
		cpuFrame := image.NewRGBA(image.Rect(0, 0, frameBounds.Dx(), frameBounds.Dy()))
		draw.Draw(cpuFrame, cpuFrame.Bounds(), sheet, srcRect.Min, draw.Src)
		p.task.cpuImages[frame] = cpuFrame
	}
}

func (p *mapRenderPrewarmer) standee(prefix, name string, img *ebiten.Image, stableImage bool) {
	if img == nil {
		return
	}
	key := makeStandeeCoreKey(p.renderer.prefixedStandeeKeyName(prefix, name), img, stableImage)
	p.lastResource = key.name
	if _, seen := p.resources.standees[key]; seen {
		return
	}
	p.resources.standees[key] = struct{}{}
	if p.renderer.standeeCoreCache[key] != nil {
		return
	}
	cpu := p.cpuImage(img)
	if cpu != nil && p.task != nil {
		p.task.standeeJobs = append(p.task.standeeJobs, mapRenderStandeeJob{key: key, source: img, cpu: cpu})
		p.stats.standeeFrames++
		return
	}
	img = p.renderer.boundedStandeeRenderSource(key, img)
	p.addUpload(img)
	core := p.renderer.standeeCoreSilhouette(key, img)
	p.addUpload(core)
	p.stats.standeeFrames++
	p.recordStandeeUploads(key)
}

func (p *mapRenderPrewarmer) recordStandeeUploads(key standeeCoreKey) {
	for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
		chain := p.renderer.standeeMipCache[standeeMipKey{frame: key, layer: layer}]
		if chain == nil {
			continue
		}
		if layer == standeeMipSticker && p.shaderStickerMips == nil {
			p.shaderStickerMips = chain
		}
		if layer == standeeMipCore && p.shaderCoreMips == nil {
			p.shaderCoreMips = chain
		}
		for _, level := range chain.levels {
			p.addUpload(level)
		}
	}
}

func prepareMapRenderStandees(ctx context.Context, jobs []mapRenderStandeeJob, tint float64) <-chan mapRenderPreparedStandee {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make(chan mapRenderPreparedStandee, 1)
	go func() {
		defer close(results)
		for i := range jobs {
			job := jobs[i]
			select {
			case <-ctx.Done():
				return
			default:
			}
			prepared := mapRenderPreparedStandee{
				key:      job.key,
				source:   job.source,
				prepared: prepareStandeePixels(job.cpu, tint, true),
			}
			jobs[i] = mapRenderStandeeJob{}
			select {
			case results <- prepared:
			case <-ctx.Done():
				return
			}
		}
	}()
	return results
}

func (p *mapRenderPrewarmer) monster(resource mapMonsterPrewarmResource) {
	if !p.renderer.game.config.Graphics.Standee.Enabled {
		return
	}
	visualFrames := p.monsterVisualFrames(resource)
	seenFrames := make(map[*ebiten.Image]struct{}, len(visualFrames))
	for _, frame := range visualFrames {
		if frame == nil {
			continue
		}
		if _, seen := seenFrames[frame]; seen {
			continue
		}
		seenFrames[frame] = struct{}{}
		p.standee("mob", resource.key, frame, true)
	}
}

func (p *mapRenderPrewarmer) decodeMonster(resource mapMonsterPrewarmResource) {
	if resource.spriteName == "" {
		return
	}
	hasWalk := false
	for _, animationType := range []string{"walking_r", "walking_l", "attacking_r", "attacking_l"} {
		frames := p.animationFrames(resource.spriteName, animationType)
		if strings.HasPrefix(animationType, "walking_") && len(frames) > 0 {
			hasWalk = true
		}
	}
	if !hasWalk {
		p.sprite(resource.spriteName)
	}
}

func (r *Renderer) deallocateStandeeKeys(keys, keep map[standeeCoreKey]struct{}) {
	if len(keys) == 0 {
		return
	}
	deallocate := make(map[*ebiten.Image]struct{})
	for key := range keys {
		if _, retained := keep[key]; retained {
			continue
		}
		if core := r.standeeCoreCache[key]; core != nil {
			deallocate[core] = struct{}{}
			delete(r.standeeCoreCache, key)
		}
		if source := r.standeeRenderSourceCache[key]; source != nil {
			deallocate[source] = struct{}{}
			delete(r.standeeRenderSourceCache, key)
		}
		for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
			mipKey := standeeMipKey{frame: key, layer: layer}
			if chain := r.standeeMipCache[mipKey]; chain != nil {
				for _, img := range chain.owned {
					if img != nil {
						deallocate[img] = struct{}{}
					}
				}
				delete(r.standeeMipCache, mipKey)
			}
		}
	}
	for img := range deallocate {
		img.Deallocate()
	}
}

// deallocateStandeeMipSourceAliases drops a complete cached standee frame when
// its sticker mip level 0 aliases a render source that was just evicted - a
// SpriteManager image or a processed (alpha_from_brightness) copy.
// Reduced mip levels and the generated core are still valid on their own, but
// retaining them would let the stable environment key reuse a chain whose base
// image has been deallocated and cleared.
func (r *Renderer) deallocateStandeeMipSourceAliases(sources map[*ebiten.Image]struct{}) {
	if len(sources) == 0 || len(r.standeeMipCache) == 0 {
		return
	}
	keys := make(map[standeeCoreKey]struct{})
	for mipKey, chain := range r.standeeMipCache {
		if chain == nil || len(chain.levels) == 0 || chain.levels[0] == nil {
			continue
		}
		if _, evicted := sources[chain.levels[0]]; evicted {
			keys[mipKey.frame] = struct{}{}
		}
	}
	r.deallocateStandeeKeys(keys, nil)
}

func (r *Renderer) resetMapRenderResourceResidency() {
	if r == nil {
		return
	}
	if task := r.mapRenderResourcePrewarmActive; task != nil {
		r.cancelMapRenderPrewarmTask(task)
		r.mapRenderResourcePrewarmActive = nil
		r.deallocateMapRenderRegion(task.prewarmer.resources, r.retainedMapRenderResources())
	}
	for len(r.mapRenderResidentMapKeys) > 0 {
		evicted := r.mapRenderResidentMapKeys[0]
		r.mapRenderResidentMapKeys[0] = ""
		r.mapRenderResidentMapKeys = r.mapRenderResidentMapKeys[1:]
		r.deallocateMapRenderRegion(r.mapRenderResourcesByMap[evicted], r.retainedMapRenderResources())
		delete(r.mapRenderResourcesByMap, evicted)
	}
	allKeys := make(map[standeeCoreKey]struct{}, len(r.standeeCoreCache))
	for key := range r.standeeCoreCache {
		allKeys[key] = struct{}{}
	}
	for key := range r.standeeMipCache {
		allKeys[key.frame] = struct{}{}
	}
	r.deallocateStandeeKeys(allKeys, nil)
	r.clearWallRipmaps()
	for key, img := range r.processedSpriteCache {
		if img != nil {
			img.Deallocate()
		}
		delete(r.processedSpriteCache, key)
	}
	r.standeeCoreCache = nil
	r.standeeRenderSourceCache = nil
	r.standeeMipCache = nil
	r.mapRenderResidentMapKeys = nil
	r.mapRenderResourcesByMap = nil
	r.mapRenderResourcePrewarmPending = false
	r.mapRenderResourcePrewarmMapKeys = nil
	r.mapRenderUploadQueue = nil
	r.mapRenderUploadQueued = nil
	r.mapRenderShaderWarmTasks = nil
	r.mapRenderLastCameraX = 0
	r.mapRenderLastCameraY = 0
	r.mapRenderLastCameraValid = false
}

func (r *Renderer) cancelMapRenderPrewarmTask(task *mapRenderPrewarmTask) {
	if task == nil {
		return
	}
	if !task.cancelled {
		task.cancelled = true
		if task.cancel != nil {
			task.cancel()
		}
	}
	if task.spriteCommit != nil {
		task.spriteCommit.Cancel()
		task.spriteCommit = nil
	}
	if task.skyCommit != nil {
		task.skyCommit.cancel()
		task.skyCommit = nil
	}
	if task.standeeCommit != nil {
		task.standeeCommit.cancel()
		task.standeeCommit = nil
	}
	for _, builder := range task.wallRipmapBuilders {
		builder.cancel()
	}
	task.wallRipmapBuilders = nil
}

func (r *Renderer) scheduleMapRenderResourcePrewarm(mapKey string) {
	if r == nil {
		return
	}
	for _, resident := range r.mapRenderResidentMapKeys {
		if resident == mapKey {
			return
		}
	}
	if active := r.mapRenderResourcePrewarmActive; active != nil && active.mapKey == mapKey && !active.cancelled {
		return
	}
	for _, queued := range r.mapRenderResourcePrewarmMapKeys {
		if queued == mapKey {
			return
		}
	}
	if mapKey == currentMapKey() {
		r.mapRenderResourcePrewarmMapKeys = append(r.mapRenderResourcePrewarmMapKeys, "")
		copy(r.mapRenderResourcePrewarmMapKeys[1:], r.mapRenderResourcePrewarmMapKeys[:len(r.mapRenderResourcePrewarmMapKeys)-1])
		r.mapRenderResourcePrewarmMapKeys[0] = mapKey
	} else {
		r.mapRenderResourcePrewarmMapKeys = append(r.mapRenderResourcePrewarmMapKeys, mapKey)
	}
	r.mapRenderResourcePrewarmPending = true
}

func (r *Renderer) cancelMapRenderPrewarmOutside(keep map[string]struct{}) {
	if r == nil {
		return
	}
	mapKeys := r.mapRenderResourcePrewarmMapKeys
	queued := mapKeys[:0]
	for _, mapKey := range mapKeys {
		if _, retained := keep[mapKey]; retained {
			queued = append(queued, mapKey)
		}
	}
	clear(mapKeys[len(queued):])
	r.mapRenderResourcePrewarmMapKeys = queued
	if active := r.mapRenderResourcePrewarmActive; active != nil {
		if _, retained := keep[active.mapKey]; !retained {
			r.cancelMapRenderPrewarmTask(active)
			r.mapRenderResourcePrewarmActive = nil
			r.deallocateMapRenderRegion(active.prewarmer.resources, r.retainedMapRenderResources())
		}
	}
	r.mapRenderResourcePrewarmPending = r.mapRenderResourcePrewarmActive != nil || len(r.mapRenderResourcePrewarmMapKeys) > 0
}

// prepareMapRenderResidency returns false when mapKey is already resident.
// Visible-region reconciliation owns eviction; this helper only touches an
// existing region or reserves a new one for the pending prewarm.
func (r *Renderer) prepareMapRenderResidency(mapKey string) bool {
	for i, resident := range r.mapRenderResidentMapKeys {
		if resident != mapKey {
			continue
		}
		copy(r.mapRenderResidentMapKeys[i:], r.mapRenderResidentMapKeys[i+1:])
		r.mapRenderResidentMapKeys[len(r.mapRenderResidentMapKeys)-1] = mapKey
		return false
	}
	return true
}

type mapRenderViewPoint struct {
	x float64
	y float64
}

const (
	mapRenderViewArcSegments       = 12
	mapRenderUnloadDistanceInTiles = 8
)

func mapRenderPointInRect(p mapRenderViewPoint, minX, minY, maxX, maxY float64) bool {
	return p.x >= minX && p.x <= maxX && p.y >= minY && p.y <= maxY
}

func mapRenderPointInPolygon(p mapRenderViewPoint, polygon []mapRenderViewPoint) bool {
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		a, b := polygon[i], polygon[j]
		if (a.y > p.y) == (b.y > p.y) {
			continue
		}
		x := (b.x-a.x)*(p.y-a.y)/(b.y-a.y) + a.x
		if p.x < x {
			inside = !inside
		}
	}
	return inside
}

func mapRenderSegmentsIntersect(a, b, c, d mapRenderViewPoint) bool {
	cross := func(p, q, r mapRenderViewPoint) float64 {
		return (q.x-p.x)*(r.y-p.y) - (q.y-p.y)*(r.x-p.x)
	}
	onSegment := func(p, q, r mapRenderViewPoint) bool {
		const epsilon = 1e-9
		return math.Abs(cross(p, q, r)) <= epsilon &&
			r.x >= math.Min(p.x, q.x)-epsilon && r.x <= math.Max(p.x, q.x)+epsilon &&
			r.y >= math.Min(p.y, q.y)-epsilon && r.y <= math.Max(p.y, q.y)+epsilon
	}
	abC, abD := cross(a, b, c), cross(a, b, d)
	cdA, cdB := cross(c, d, a), cross(c, d, b)
	if (abC > 0 && abD < 0 || abC < 0 && abD > 0) &&
		(cdA > 0 && cdB < 0 || cdA < 0 && cdB > 0) {
		return true
	}
	return onSegment(a, b, c) || onSegment(a, b, d) ||
		onSegment(c, d, a) || onSegment(c, d, b)
}

func mapRenderRegionIntersectsView(region *world.OpenWorldRegion, tileSize, cameraX, cameraY, angle, fov, distance float64) bool {
	if region == nil || tileSize <= 0 || fov <= 0 || distance <= 0 {
		return false
	}
	minX := float64(region.OffsetX) * tileSize
	minY := float64(region.OffsetY) * tileSize
	maxX := float64(region.OffsetX+region.Width) * tileSize
	maxY := float64(region.OffsetY+region.Height) * tileSize
	camera := mapRenderViewPoint{x: cameraX, y: cameraY}
	if mapRenderPointInRect(camera, minX, minY, maxX, maxY) {
		return true
	}

	// Circumscribe each small arc segment so the polygon never misses a region
	// that touches the circular far edge of the actual camera sector.
	step := fov / mapRenderViewArcSegments
	arcDistance := distance / math.Cos(step/2)
	polygon := make([]mapRenderViewPoint, 1, mapRenderViewArcSegments+2)
	polygon[0] = camera
	for i := 0; i <= mapRenderViewArcSegments; i++ {
		a := angle - fov/2 + float64(i)*step
		polygon = append(polygon, mapRenderViewPoint{
			x: cameraX + math.Cos(a)*arcDistance,
			y: cameraY + math.Sin(a)*arcDistance,
		})
	}

	rect := [4]mapRenderViewPoint{
		{x: minX, y: minY}, {x: maxX, y: minY},
		{x: maxX, y: maxY}, {x: minX, y: maxY},
	}
	for _, p := range polygon {
		if mapRenderPointInRect(p, minX, minY, maxX, maxY) {
			return true
		}
	}
	for _, p := range rect {
		if mapRenderPointInPolygon(p, polygon) {
			return true
		}
	}
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		for j, c := range rect {
			d := rect[(j+1)%len(rect)]
			if mapRenderSegmentsIntersect(a, b, c, d) {
				return true
			}
		}
	}
	return false
}

func visibleOpenWorldMapKeys(wm *world.WorldManager, camera *FirstPersonCamera, tileSize, fovMargin, distanceMargin float64) []string {
	if wm == nil || camera == nil || tileSize <= 0 {
		return nil
	}
	keys := make([]string, 0, len(wm.OpenWorldRegions))
	for i := range wm.OpenWorldRegions {
		region := &wm.OpenWorldRegions[i]
		if mapRenderRegionIntersectsView(region, tileSize, camera.X, camera.Y, camera.Angle,
			camera.FOV+fovMargin, camera.ViewDist+distanceMargin) {
			keys = append(keys, region.MapKey)
		}
	}
	sort.Strings(keys)
	return keys
}

func nearbyOpenWorldMapKeys(wm *world.WorldManager, camera *FirstPersonCamera, tileSize, distanceMargin float64) []string {
	if wm == nil || camera == nil || tileSize <= 0 {
		return nil
	}
	distance := camera.ViewDist + distanceMargin
	distanceSq := distance * distance
	keys := make([]string, 0, len(wm.OpenWorldRegions))
	for i := range wm.OpenWorldRegions {
		region := &wm.OpenWorldRegions[i]
		minX := float64(region.OffsetX) * tileSize
		minY := float64(region.OffsetY) * tileSize
		maxX := float64(region.OffsetX+region.Width) * tileSize
		maxY := float64(region.OffsetY+region.Height) * tileSize
		closestX := math.Max(minX, math.Min(camera.X, maxX))
		closestY := math.Max(minY, math.Min(camera.Y, maxY))
		dx, dy := closestX-camera.X, closestY-camera.Y
		if dx*dx+dy*dy <= distanceSq {
			keys = append(keys, region.MapKey)
		}
	}
	sort.Strings(keys)
	return keys
}

func mapRenderRegionDistanceSq(wm *world.WorldManager, mapKey string, x, y, tileSize float64) float64 {
	if wm == nil || tileSize <= 0 {
		return math.Inf(1)
	}
	var region *world.OpenWorldRegion
	for i := range wm.OpenWorldRegions {
		if wm.OpenWorldRegions[i].MapKey == mapKey {
			region = &wm.OpenWorldRegions[i]
			break
		}
	}
	if region == nil {
		return math.Inf(1)
	}
	minX := float64(region.OffsetX) * tileSize
	minY := float64(region.OffsetY) * tileSize
	maxX := float64(region.OffsetX+region.Width) * tileSize
	maxY := float64(region.OffsetY+region.Height) * tileSize
	closestX := math.Max(minX, math.Min(x, maxX))
	closestY := math.Max(minY, math.Min(y, maxY))
	dx, dy := closestX-x, closestY-y
	return dx*dx + dy*dy
}

func prioritizeMapRenderKeys(wm *world.WorldManager, keys []string, current string, camera *FirstPersonCamera, tileSize, moveX, moveY float64) {
	if len(keys) < 2 || camera == nil {
		return
	}
	directionLength := math.Hypot(moveX, moveY)
	if directionLength > 0.01 {
		moveX /= directionLength
		moveY /= directionLength
	} else {
		moveX, moveY = math.Cos(camera.Angle), math.Sin(camera.Angle)
	}
	predictedX := camera.X + moveX*mapRenderLoadMarginInTiles*tileSize
	predictedY := camera.Y + moveY*mapRenderLoadMarginInTiles*tileSize
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i] == current || keys[j] == current {
			return keys[i] == current
		}
		di := mapRenderRegionDistanceSq(wm, keys[i], predictedX, predictedY, tileSize)
		dj := mapRenderRegionDistanceSq(wm, keys[j], predictedX, predictedY, tileSize)
		if di != dj {
			return di < dj
		}
		return keys[i] < keys[j]
	})
}

func (r *Renderer) prioritizeMapRenderPrewarmQueue(loadKeys []string, moveX, moveY, tileSize float64) {
	if r == nil || r.game == nil || r.game.camera == nil {
		return
	}
	load := make(map[string]struct{}, len(loadKeys))
	for _, key := range loadKeys {
		load[key] = struct{}{}
	}
	current := currentMapKey()
	wm := world.GlobalWorldManager
	sort.SliceStable(r.mapRenderResourcePrewarmMapKeys, func(i, j int) bool {
		a, b := r.mapRenderResourcePrewarmMapKeys[i], r.mapRenderResourcePrewarmMapKeys[j]
		if a == current || b == current {
			return a == current
		}
		_, aVisible := load[a]
		_, bVisible := load[b]
		if aVisible != bVisible {
			return aVisible
		}
		pair := []string{a, b}
		prioritizeMapRenderKeys(wm, pair, current, r.game.camera, tileSize, moveX, moveY)
		return pair[0] == a
	})
}

func (r *Renderer) evictMapRenderResidencyOutside(keep map[string]struct{}) {
	if r == nil || len(r.mapRenderResidentMapKeys) == 0 {
		return
	}
	evictedResources := make([]*mapRenderRegionResources, 0)
	residentKeys := r.mapRenderResidentMapKeys
	retainedKeys := residentKeys[:0]
	for _, mapKey := range residentKeys {
		if _, retained := keep[mapKey]; retained {
			retainedKeys = append(retainedKeys, mapKey)
			continue
		}
		r.dropMapRenderUploads(mapKey)
		evictedResources = append(evictedResources, r.mapRenderResourcesByMap[mapKey])
		delete(r.mapRenderResourcesByMap, mapKey)
	}
	clear(residentKeys[len(retainedKeys):])
	r.mapRenderResidentMapKeys = retainedKeys
	retainedResources := r.retainedMapRenderResources()
	for _, resources := range evictedResources {
		r.deallocateMapRenderRegion(resources, retainedResources)
	}
}

// syncVisibleMapRenderResidency warms only regions in the current FOV. Once a
// neighbour is warm, distance-only retention prevents camera turns from
// destroying and rebuilding it; eviction happens after the party moves beyond
// view distance plus the spatial hysteresis margin.
func (r *Renderer) syncVisibleMapRenderResidency() {
	if r == nil || r.game == nil || !r.game.openWorldActive() {
		return
	}
	wm := world.GlobalWorldManager
	tileSize := float64(r.game.config.GetTileSize())
	moveX, moveY := 0.0, 0.0
	if r.mapRenderLastCameraValid {
		moveX = r.game.camera.X - r.mapRenderLastCameraX
		moveY = r.game.camera.Y - r.mapRenderLastCameraY
	}
	r.mapRenderLastCameraX = r.game.camera.X
	r.mapRenderLastCameraY = r.game.camera.Y
	r.mapRenderLastCameraValid = true
	loadKeys := visibleOpenWorldMapKeys(wm, r.game.camera, tileSize,
		mapRenderLoadFOVMargin, mapRenderLoadMarginInTiles*tileSize)
	prioritizeMapRenderKeys(wm, loadKeys, currentMapKey(), r.game.camera, tileSize, moveX, moveY)
	retainKeys := nearbyOpenWorldMapKeys(wm, r.game.camera, tileSize,
		mapRenderUnloadDistanceInTiles*tileSize)
	keep := make(map[string]struct{}, len(retainKeys)+1)
	for _, mapKey := range retainKeys {
		keep[mapKey] = struct{}{}
	}
	keep[currentMapKey()] = struct{}{}
	r.cancelMapRenderPrewarmOutside(keep)
	r.evictMapRenderResidencyOutside(keep)
	r.scheduleMapRenderResourcePrewarm(currentMapKey())
	for _, mapKey := range loadKeys {
		r.scheduleMapRenderResourcePrewarm(mapKey)
	}
	r.prioritizeMapRenderPrewarmQueue(loadKeys, moveX, moveY, tileSize)
	r.deallocateUnusedSkyPanoramas(r.retainedMapRenderResources().skies)
}

func (r *Renderer) commitMapRenderResidency(mapKey string, resources *mapRenderRegionResources) {
	if r.mapRenderResourcesByMap == nil {
		r.mapRenderResourcesByMap = make(map[string]*mapRenderRegionResources)
	}
	r.mapRenderResourcesByMap[mapKey] = resources
	r.mapRenderResidentMapKeys = append(r.mapRenderResidentMapKeys, mapKey)
	r.deallocateUnusedSkyPanoramas(r.retainedMapRenderResources().skies)
}

func (r *Renderer) deallocateUnusedSkyPanoramas(keep map[string]struct{}) {
	if r == nil || r.game == nil {
		return
	}
	for name, img := range r.game.skyPanoramaCache {
		if _, retained := keep[name]; retained || img == nil ||
			img == r.game.skyPanorama || img == r.game.skyPanoramaPrev {
			continue
		}
		img.Deallocate()
		delete(r.game.skyPanoramaCache, name)
	}
}

func (r *Renderer) retainedMapRenderResources() *mapRenderRegionResources {
	keep := &mapRenderRegionResources{
		standees:  make(map[standeeCoreKey]struct{}),
		sources:   make(map[mapRenderSourceKey]struct{}),
		processed: make(map[processedSpriteKey]struct{}),
		walls:     make(map[*ebiten.Image]struct{}),
		skies:     make(map[string]struct{}),
	}
	for _, mapKey := range r.mapRenderResidentMapKeys {
		retainMapRenderRegionResources(keep, r.mapRenderResourcesByMap[mapKey])
	}
	if active := r.mapRenderResourcePrewarmActive; active != nil &&
		!active.cancelled && active.prewarmer != nil {
		retainMapRenderRegionResources(keep, active.prewarmer.resources)
	}
	return keep
}

func retainMapRenderRegionResources(keep, resources *mapRenderRegionResources) {
	if keep == nil || resources == nil {
		return
	}
	for key := range resources.standees {
		keep.standees[key] = struct{}{}
	}
	for key := range resources.sources {
		keep.sources[key] = struct{}{}
	}
	for key := range resources.processed {
		keep.processed[key] = struct{}{}
	}
	for img := range resources.walls {
		keep.walls[img] = struct{}{}
	}
	for name := range resources.skies {
		keep.skies[name] = struct{}{}
	}
}

func (r *Renderer) deallocateMapRenderRegion(resources, keep *mapRenderRegionResources) {
	if resources == nil {
		return
	}
	if keep == nil {
		keep = &mapRenderRegionResources{}
	}
	r.deallocateStandeeKeys(resources.standees, keep.standees)
	evictedSources := make(map[*ebiten.Image]struct{})
	for key := range resources.processed {
		if _, retained := keep.processed[key]; retained {
			continue
		}
		if img := r.processedSpriteCache[key]; img != nil {
			evictedSources[img] = struct{}{}
			delete(r.wallSliceColumns, img)
			delete(r.animFrameCache, img)
			img.Deallocate()
		}
		delete(r.processedSpriteCache, key)
	}
	for img := range resources.walls {
		if _, retained := keep.walls[img]; !retained {
			r.deallocateWallRipmap(img)
		}
	}
	for name := range resources.skies {
		if _, retained := keep.skies[name]; retained {
			continue
		}
		img := r.game.skyPanoramaCache[name]
		if img == nil || img == r.game.skyPanorama || img == r.game.skyPanoramaPrev {
			continue
		}
		img.Deallocate()
		delete(r.game.skyPanoramaCache, name)
	}
	for key := range resources.sources {
		if _, retained := keep.sources[key]; retained {
			continue
		}
		for _, img := range r.game.sprites.EvictResource(key.name, key.animationType) {
			evictedSources[img] = struct{}{}
			delete(r.wallSliceColumns, img)
			delete(r.animFrameCache, img)
		}
	}
	r.deallocateStandeeMipSourceAliases(evictedSources)
}

func (r *Renderer) trackResidentSourceRequest(request graphics.SpriteResourceRequest) {
	if request.Name == "" {
		return
	}
	r.trackCurrentMapRenderResource(nil, &mapRenderSourceKey{
		name: request.Name, animationType: request.AnimationType,
	}, nil)
}

func (r *Renderer) trackResidentProcessedKey(key processedSpriteKey) {
	r.trackCurrentMapRenderResource(nil, &mapRenderSourceKey{name: key.spriteName}, &key)
}

// trackResidentStandeeKey accounts for an uncommon resource that escaped the
// authored prewarm inventory and was built lazily while rendering. The derived
// key and its underlying static/animation/processed source get the same owner,
// so eviction cannot retain the source or leave an aliased mip base dangling.
func (r *Renderer) trackResidentStandeeKey(key standeeCoreKey, source ...*ebiten.Image) {
	if r == nil {
		return
	}
	mapKey := currentMapKey()
	needsOwnership := false
	if resources := r.mapRenderResourcesByMap[mapKey]; resources != nil {
		_, owned := resources.standees[key]
		needsOwnership = !owned
	}
	if active := r.mapRenderResourcePrewarmActive; active != nil && !active.cancelled &&
		active.mapKey == mapKey && active.prewarmer != nil {
		_, owned := active.prewarmer.resources.standees[key]
		needsOwnership = needsOwnership || !owned
	}
	if !needsOwnership {
		return
	}
	var sourceKey *mapRenderSourceKey
	var processedKey *processedSpriteKey
	if len(source) > 0 && source[0] != nil {
		img := source[0]
		for key, candidate := range r.processedSpriteCache {
			if candidate == img {
				owned := key
				processedKey = &owned
				base := mapRenderSourceKey{name: key.spriteName}
				sourceKey = &base
				break
			}
		}
		if sourceKey == nil && r.game != nil && r.game.sprites != nil {
			request, ok := r.game.sprites.ResourceForImage(img)
			if !ok {
				for base, frames := range r.animFrameCache {
					for _, frame := range frames {
						if frame != img {
							continue
						}
						request, ok = r.game.sprites.ResourceForImage(base)
						break
					}
					if ok {
						break
					}
				}
			}
			if ok {
				owned := mapRenderSourceKey{name: request.Name, animationType: request.AnimationType}
				sourceKey = &owned
			}
		}
	}
	r.trackCurrentMapRenderResource(&key, sourceKey, processedKey)
}

func (r *Renderer) trackCurrentMapRenderResource(standee *standeeCoreKey, source *mapRenderSourceKey, processed *processedSpriteKey) {
	if r == nil {
		return
	}
	mapKey := currentMapKey()
	track := func(resources *mapRenderRegionResources) {
		if resources == nil {
			return
		}
		if standee != nil {
			if resources.standees == nil {
				resources.standees = make(map[standeeCoreKey]struct{})
			}
			resources.standees[*standee] = struct{}{}
		}
		if source != nil {
			if resources.sources == nil {
				resources.sources = make(map[mapRenderSourceKey]struct{})
			}
			resources.sources[*source] = struct{}{}
		}
		if processed != nil {
			if resources.processed == nil {
				resources.processed = make(map[processedSpriteKey]struct{})
			}
			resources.processed[*processed] = struct{}{}
		}
	}
	track(r.mapRenderResourcesByMap[mapKey])
	if active := r.mapRenderResourcePrewarmActive; active != nil &&
		!active.cancelled && active.mapKey == mapKey && active.prewarmer != nil {
		track(active.prewarmer.resources)
	}
}

func mapRenderSourceRequests(plan mapRenderPrewarmPlan) []graphics.SpriteResourceRequest {
	requests := make(map[graphics.SpriteResourceRequest]struct{})
	addSprite := func(name string) {
		if name != "" {
			requests[graphics.SpriteResourceRequest{Name: name}] = struct{}{}
		}
	}
	addMonster := func(resource mapMonsterPrewarmResource) {
		if resource.spriteName == "" {
			return
		}
		addSprite(resource.spriteName)
		for _, animationType := range []string{"walking_r", "walking_l", "attacking_r", "attacking_l"} {
			requests[graphics.SpriteResourceRequest{Name: resource.spriteName, AnimationType: animationType}] = struct{}{}
		}
	}
	for _, names := range [][]string{plan.tileSprites, plan.wallSprites, plan.treeSprites, plan.crossedPropSprites, plan.npcDecodeSprites, plan.containerDecode, plan.containerSprites} {
		for _, name := range names {
			addSprite(name)
		}
	}
	for _, resource := range plan.environmentSprites {
		addSprite(resource.spriteName)
	}
	for _, resource := range plan.npcSprites {
		addSprite(resource.name)
	}
	for _, resource := range plan.monsterDecode {
		addMonster(resource)
	}
	for _, resource := range plan.monsterSprites {
		addMonster(resource)
	}
	out := make([]graphics.SpriteResourceRequest, 0, len(requests))
	for request := range requests {
		out = append(out, request)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].AnimationType < out[j].AnimationType
	})
	return out
}

func prepareMapRenderSkies(ctx context.Context, names []string) <-chan mapRenderPreparedSky {
	results := make(chan mapRenderPreparedSky, 1)
	type skyDecodeJob struct {
		name string
		path string
	}
	jobs := make([]skyDecodeJob, 0, len(names))
	for _, name := range names {
		path := resolveNamedPNG("assets/sprites/sky", name)
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		jobs = append(jobs, skyDecodeJob{name: name, path: path})
	}
	go func() {
		defer close(results)
		for _, job := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			img, err := decodePNG(job.path)
			if err != nil {
				img = nil
			}
			var rgba *image.RGBA
			if img != nil {
				bounds := img.Bounds()
				rgba = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
				draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
			}
			select {
			case results <- mapRenderPreparedSky{name: job.name, image: rgba}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return results
}

func (r *Renderer) startNextMapRenderPrewarm() {
	if r == nil || r.game == nil || r.game.sprites == nil || r.mapRenderResourcePrewarmActive != nil {
		return
	}
	for len(r.mapRenderResourcePrewarmMapKeys) > 0 {
		mapKey := r.mapRenderResourcePrewarmMapKeys[0]
		r.mapRenderResourcePrewarmMapKeys[0] = ""
		r.mapRenderResourcePrewarmMapKeys = r.mapRenderResourcePrewarmMapKeys[1:]
		if !r.prepareMapRenderResidency(mapKey) {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		task := &mapRenderPrewarmTask{mapKey: mapKey, ctx: ctx, cancel: cancel}
		task.cpuImages = make(map[*ebiten.Image]*image.RGBA)
		task.plan = r.collectMapRenderPrewarmPlan(mapKey)
		task.prewarmer = newMapRenderPrewarmer(r, task)
		task.preparedSprites = r.game.sprites.PrepareResources(ctx, mapRenderSourceRequests(task.plan))
		task.preparedSkies = prepareMapRenderSkies(ctx, skyTextureNamesForMap(mapKey))
		r.mapRenderResourcePrewarmActive = task
		break
	}
	r.mapRenderResourcePrewarmPending = r.mapRenderResourcePrewarmActive != nil || len(r.mapRenderResourcePrewarmMapKeys) > 0
}

func (r *Renderer) drainPreparedMapRenderResource(task *mapRenderPrewarmTask) bool {
	if task == nil || task.cancelled {
		return false
	}
	if task.standeeCommit != nil {
		task.prewarmer.lastResource = "standee-write:" + task.standeeCommit.key.name
		sticker, core, done := task.standeeCommit.advance(r, mapRenderSpriteCommitFrameBytes)
		if done {
			task.prewarmer.addUpload(sticker)
			task.prewarmer.addUpload(core)
			task.prewarmer.recordStandeeUploads(task.standeeCommit.key)
			task.standeeCommit = nil
		}
		return true
	}
	if task.spriteCommit != nil {
		task.prewarmer.lastResource = "write:" + task.spriteCommitReq.Name
		if task.spriteCommitReq.AnimationType != "" {
			task.prewarmer.lastResource += ":" + task.spriteCommitReq.AnimationType
		}
		images, done := task.spriteCommit.Advance(mapRenderSpriteCommitFrameBytes)
		if done {
			for img, cpu := range images {
				task.cpuImages[img] = cpu
			}
			if task.spriteCommitFound {
				task.prewarmer.resources.sources[mapRenderSourceKey{
					name: task.spriteCommitReq.Name, animationType: task.spriteCommitReq.AnimationType,
				}] = struct{}{}
			}
			task.spriteCommit = nil
		}
		return true
	}
	if task.skyCommit != nil {
		task.prewarmer.lastResource = "sky:" + task.skyCommit.name
		if task.skyCommit.advance(mapRenderSpriteCommitFrameBytes) {
			commit := task.skyCommit
			if r.game.skyPanoramaCache == nil {
				r.game.skyPanoramaCache = make(map[string]*ebiten.Image)
			}
			if existing := r.game.skyPanoramaCache[commit.name]; existing != nil {
				commit.image.Deallocate()
				commit.image = existing
			} else {
				r.game.skyPanoramaCache[commit.name] = commit.image
			}
			task.prewarmer.resources.skies[commit.name] = struct{}{}
			task.prewarmer.addUpload(commit.image)
			task.skyCommit = nil
		}
		return true
	}
	if !task.spritesDone {
		select {
		case prepared, ok := <-task.preparedSprites:
			if !ok {
				task.spritesDone = true
			} else {
				task.prewarmer.lastResource = "begin:" + prepared.Request.Name
				if prepared.Request.AnimationType != "" {
					task.prewarmer.lastResource += ":" + prepared.Request.AnimationType
				}
				task.spriteCommit = r.game.sprites.BeginPreparedResourceCommit(prepared)
				task.spriteCommitReq = prepared.Request
				task.spriteCommitFound = prepared.Found
				return true
			}
		default:
		}
	}
	if !task.skiesDone {
		select {
		case prepared, ok := <-task.preparedSkies:
			if !ok {
				task.skiesDone = true
			} else {
				if prepared.image != nil {
					img := r.game.skyPanoramaCache[prepared.name]
					if img == nil {
						img = ebiten.NewImage(prepared.image.Bounds().Dx(), prepared.image.Bounds().Dy())
						task.skyCommit = &mapRenderSkyCommit{name: prepared.name, image: img, cpu: prepared.image}
						return true
					}
					task.prewarmer.resources.skies[prepared.name] = struct{}{}
					task.prewarmer.addUpload(img)
				}
				return true
			}
		default:
		}
	}
	if task.preparedStandees != nil && !task.standeesDone {
		select {
		case prepared, ok := <-task.preparedStandees:
			if !ok {
				task.standeesDone = true
			} else {
				task.prewarmer.lastResource = "standee:" + prepared.key.name
				task.standeeCommit = newMapRenderStandeeCommit(prepared)
				return true
			}
		default:
		}
	}
	return false
}

func (p *mapRenderPrewarmer) monsterVisualFrames(resource mapMonsterPrewarmResource) []*ebiten.Image {
	if resource.spriteName == "" {
		return nil
	}
	var visualFrames []*ebiten.Image
	appendFrames := func(frames []*ebiten.Image) bool {
		if len(frames) == 0 {
			return false
		}
		visualFrames = append(visualFrames, frames...)
		return true
	}
	hasWalk := false
	if p.renderer.game.config.Graphics.Standee.Enabled {
		walk := p.animationFrames(resource.spriteName, "walking_r")
		if len(walk) == 0 {
			walk = p.animationFrames(resource.spriteName, "walking_l")
		}
		hasWalk = appendFrames(walk)
		attack := p.animationFrames(resource.spriteName, "attacking_r")
		if len(attack) == 0 {
			attack = p.animationFrames(resource.spriteName, "attacking_l")
		}
		appendFrames(attack)
	} else {
		hasWalk = appendFrames(p.animationFrames(resource.spriteName, "walking_r"))
		hasWalk = appendFrames(p.animationFrames(resource.spriteName, "walking_l")) || hasWalk
		appendFrames(p.animationFrames(resource.spriteName, "attacking_r"))
		appendFrames(p.animationFrames(resource.spriteName, "attacking_l"))
	}
	if !hasWalk {
		if base := p.sprite(resource.spriteName); base != nil {
			visualFrames = append(visualFrames, base)
		}
	}
	return visualFrames
}

func (r *Renderer) buildMapRenderPrewarmSteps(task *mapRenderPrewarmTask) []mapRenderPrewarmStep {
	plan := task.plan
	p := task.prewarmer
	steps := make([]mapRenderPrewarmStep, 0, len(mapRenderSourceRequests(plan))+len(plan.monsterSprites)*4)
	appendStep := func(step func()) {
		steps = append(steps, func(time.Time) bool {
			step()
			return true
		})
	}
	appendTimedStep := func(step mapRenderPrewarmStep) { steps = append(steps, step) }
	warmVisibleBounds := func(name string) {
		names := r.game.sprites.GetSpriteVariants(name)
		if len(names) == 0 {
			names = []string{name}
		}
		for _, candidate := range names {
			r.game.sprites.SpriteVisibleFrameBounds(candidate)
		}
	}

	for _, name := range plan.tileSprites {
		name := name
		appendStep(func() { p.sprite(name) })
	}
	for _, name := range plan.wallSprites {
		name := name
		var sprite *ebiten.Image
		var builder *mapRenderWallRipmapBuilder
		nextColumn := 0
		appendTimedStep(func(deadline time.Time) bool {
			if sprite == nil {
				sprite = p.sprite(name)
				if sprite == nil {
					return true
				}
				p.resources.walls[sprite] = struct{}{}
				builder = newMapRenderWallRipmapBuilder(r, sprite, p.cpuImage(sprite))
				task.wallRipmapBuilders = append(task.wallRipmapBuilders, builder)
			}
			if builder != nil && !builder.done {
				if _, done := builder.advance(mapRenderSpriteCommitFrameBytes); !done {
					return false
				}
			}
			if builder != nil && builder.ripmap != nil {
				for _, level := range builder.ripmap.owned {
					p.addUpload(level)
				}
			}
			bounds := sprite.Bounds()
			for nextColumn < bounds.Dx() {
				r.spriteColumn(sprite, nextColumn, bounds.Dx(), bounds.Dy())
				nextColumn++
				if time.Now().After(deadline) {
					return false
				}
			}
			p.stats.wallTextures++
			return true
		})
	}
	if r.game.config.Graphics.TreesAsBillboards {
		for _, name := range plan.treeSprites {
			name := name
			appendStep(func() { p.standee("tree", name, p.sprite(name), true) })
		}
	}
	for _, name := range plan.crossedPropSprites {
		name := name
		appendStep(func() {
			warmVisibleBounds(name)
			p.standee("tree", name, p.sprite(name), true)
		})
	}
	for _, resource := range plan.environmentSprites {
		resource := resource
		appendStep(func() {
			warmVisibleBounds(resource.spriteName)
			sprite := p.processedSprite(resource)
			if _, ok := r.processedSpriteCache[resource]; ok {
				p.resources.processed[resource] = struct{}{}
			}
			p.addUpload(sprite)
			if sprite == nil || !r.game.config.Graphics.Standee.Enabled {
				return
			}
			renderType := world.GlobalTileManager.GetRenderType(resource.tileType)
			frames := r.animationFrames(sprite)
			p.rememberCPUFrames(sprite, frames)
			for _, frame := range frames {
				switch {
				case renderType == config.TileRenderLandmarkStandee:
					p.standee("landmark", resource.spriteName, frame, true)
				case world.GlobalTileManager.IsWallMounted(resource.tileType):
					p.standee("wallprop", resource.spriteName, frame, false)
					p.standee("tile", resource.spriteName, frame, false)
				default:
					p.standee("tile", resource.spriteName, frame, false)
				}
			}
		})
	}
	if tm := world.GlobalTileManager; tm != nil {
		for _, tileType := range plan.tileTypes {
			tileType := tileType
			if tileShowsImpassableAura(tm.GetTileData(tileType)) {
				appendStep(func() { p.auraTileColor(tileType) })
			}
		}
	}
	for _, resource := range plan.npcSprites {
		resource := resource
		sprite := p.sprite(resource.name)
		if resource.warmVisibleBounds {
			appendStep(func() { warmVisibleBounds(resource.name) })
		}
		if sprite == nil || !r.game.config.Graphics.Standee.Enabled {
			continue
		}
		frames := r.animationFrames(sprite)
		p.rememberCPUFrames(sprite, frames)
		for _, frame := range frames {
			frame := frame
			appendStep(func() { p.standee(resource.prefix, resource.name, frame, resource.stableImage) })
		}
	}
	for _, resource := range plan.monsterSprites {
		resource := resource
		seen := make(map[*ebiten.Image]struct{})
		for _, frame := range p.monsterVisualFrames(resource) {
			if frame == nil {
				continue
			}
			if _, duplicate := seen[frame]; duplicate {
				continue
			}
			seen[frame] = struct{}{}
			frame := frame
			appendStep(func() {
				if r.game.config.Graphics.Standee.Enabled {
					p.standee("mob", resource.key, frame, true)
				}
			})
		}
	}
	for _, name := range plan.containerSprites {
		name := name
		appendStep(func() {
			sprite := p.sprite(name)
			if r.game.config.Graphics.Standee.Enabled {
				p.standee("container", name, sprite, false)
			}
		})
	}
	appendStep(func() {
		tint := r.game.config.Graphics.Standee.CoreTint
		jobs := task.standeeJobs
		task.standeeJobs = nil
		task.cpuImages = nil
		task.preparedStandees = prepareMapRenderStandees(task.ctx, jobs, tint)
	})
	return steps
}

func (r *Renderer) finalizeMapRenderPrewarm(task *mapRenderPrewarmTask) {
	p := task.prewarmer
	p.addUpload(r.whiteImg)
	p.addUpload(r.floorColorMap)
	p.addUpload(r.floorTextureIndexMap)
	p.addUpload(r.floorTexAtlas)
	if len(r.tileLightCache) > 0 {
		p.addUpload(r.ensureSoftGlow())
	}
	if p.stats.standeeFrames > 0 {
		_, _ = r.ensureStandeeTrilinearShader()
		_, _ = r.ensureStandeeVolumeShader()
		r.reserveStandeeBuffers()
	}
	_, _ = r.ensureFloorShader()
	_, _ = r.game.ensureSkyShader()
	p.stats.uploadImages = len(p.uploads)
	for img := range p.uploads {
		r.queueMapRenderUpload(img, task)
	}
	if p.shaderStickerMips != nil && p.shaderCoreMips != nil {
		r.mapRenderShaderWarmTasks = append(r.mapRenderShaderWarmTasks, task)
	}
}

// prewarmPendingMapRenderResources advances at most one CPU commit or one
// derived-resource step. A region can take many ticks, but no Update consumes
// the whole cold region and only one region owns in-flight GPU allocations.
func (r *Renderer) prewarmPendingMapRenderResources() mapRenderPrewarmStats {
	if r == nil || r.game == nil {
		return mapRenderPrewarmStats{}
	}
	if r.game.appScreen != AppScreenInGame {
		return mapRenderPrewarmStats{}
	}
	r.startNextMapRenderPrewarm()
	task := r.mapRenderResourcePrewarmActive
	if task == nil {
		return mapRenderPrewarmStats{}
	}
	if r.drainPreparedMapRenderResource(task) {
		return mapRenderPrewarmStats{}
	}
	if !task.spritesDone || !task.skiesDone {
		return mapRenderPrewarmStats{}
	}
	if task.steps == nil {
		task.steps = r.buildMapRenderPrewarmSteps(task)
		return mapRenderPrewarmStats{}
	}
	if task.nextStep < len(task.steps) {
		deadline := time.Now().Add(mapRenderDerivedFrameBudget)
		if task.steps[task.nextStep](deadline) {
			task.nextStep++
		}
	}
	if task.nextStep < len(task.steps) {
		return mapRenderPrewarmStats{}
	}
	if task.standeeCommit != nil || task.preparedStandees != nil && !task.standeesDone {
		return mapRenderPrewarmStats{}
	}
	r.finalizeMapRenderPrewarm(task)
	task.committed = true
	if task.cancel != nil {
		task.cancel()
	}
	r.commitMapRenderResidency(task.mapKey, task.prewarmer.resources)
	stats := task.prewarmer.stats
	r.mapRenderResourcePrewarmActive = nil
	r.mapRenderResourcePrewarmPending = len(r.mapRenderResourcePrewarmMapKeys) > 0
	return stats
}

const (
	mapRenderUploadFrameBytes  int64 = 8 << 20
	mapRenderUploadFrameImages       = 32
)

func mapRenderUploadFrameFull(imageCount int, frameBytes, nextImageBytes int64) bool {
	return imageCount > 0 &&
		(imageCount >= mapRenderUploadFrameImages || frameBytes+nextImageBytes > mapRenderUploadFrameBytes)
}

func (r *Renderer) queueMapRenderUpload(img *ebiten.Image, task *mapRenderPrewarmTask) {
	if r == nil || img == nil || task == nil || task.cancelled {
		return
	}
	if r.mapRenderUploadQueued == nil {
		r.mapRenderUploadQueued = make(map[*ebiten.Image]struct{})
	}
	if _, queued := r.mapRenderUploadQueued[img]; queued {
		return
	}
	r.mapRenderUploadQueued[img] = struct{}{}
	r.mapRenderUploadQueue = append(r.mapRenderUploadQueue, mapRenderUpload{image: img, task: task})
}

func (r *Renderer) dropMapRenderUploads(mapKey string) {
	if r == nil {
		return
	}
	uploads := r.mapRenderUploadQueue
	kept := uploads[:0]
	for _, upload := range uploads {
		if upload.task != nil && upload.task.mapKey == mapKey {
			delete(r.mapRenderUploadQueued, upload.image)
			continue
		}
		kept = append(kept, upload)
	}
	clear(uploads[len(kept):])
	r.mapRenderUploadQueue = kept
	shaderTasks := r.mapRenderShaderWarmTasks
	warmTasks := shaderTasks[:0]
	for _, task := range shaderTasks {
		if task == nil || task.mapKey == mapKey {
			continue
		}
		warmTasks = append(warmTasks, task)
	}
	clear(shaderTasks[len(warmTasks):])
	r.mapRenderShaderWarmTasks = warmTasks
}

type mapRenderUploadDestination interface {
	DrawImage(*ebiten.Image, *ebiten.DrawImageOptions)
}

// drawMapRenderPrewarmUploads submits a bounded amount of GPU work directly to
// the current screen. The transparent draws keep the screen unchanged and use
// it only as a destination, so no mutable offscreen becomes a render source.
// There is deliberately no ReadPixels fence: Ebitengine may batch and upload
// normally without a forced GPU-to-CPU round trip.
func (r *Renderer) drawMapRenderPrewarmUploads(screen *ebiten.Image) {
	if screen == nil {
		return
	}
	r.submitMapRenderPrewarmUploads(screen)
}

func (r *Renderer) submitMapRenderPrewarmUploads(dst mapRenderUploadDestination) {
	if r == nil || dst == nil || len(r.mapRenderUploadQueue) == 0 {
		return
	}
	consumed := 0
	var consumedBytes int64
	for consumed < len(r.mapRenderUploadQueue) && consumed < mapRenderUploadFrameImages {
		upload := r.mapRenderUploadQueue[consumed]
		if upload.image == nil || upload.task == nil || upload.task.cancelled {
			delete(r.mapRenderUploadQueued, upload.image)
			consumed++
			continue
		}
		bounds := upload.image.Bounds()
		imageBytes := int64(bounds.Dx()) * int64(bounds.Dy()) * 4
		if mapRenderUploadFrameFull(consumed, consumedBytes, imageBytes) {
			break
		}
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(-bounds.Min.X), float64(-bounds.Min.Y))
		opts.GeoM.Scale(1/float64(bounds.Dx()), 1/float64(bounds.Dy()))
		opts.GeoM.Translate(float64(consumed), 0)
		opts.ColorScale.ScaleAlpha(0)
		dst.DrawImage(upload.image, opts)
		delete(r.mapRenderUploadQueued, upload.image)
		consumedBytes += imageBytes
		consumed++
	}
	if consumed > 0 {
		queue := r.mapRenderUploadQueue
		remaining := copy(queue, queue[consumed:])
		clear(queue[remaining:])
		r.mapRenderUploadQueue = queue[:remaining]
	}
}

func (r *Renderer) drawMapRenderShaderWarm(screen *ebiten.Image) {
	if r == nil || screen == nil {
		return
	}
	for len(r.mapRenderShaderWarmTasks) > 0 {
		task := r.mapRenderShaderWarmTasks[0]
		r.mapRenderShaderWarmTasks[0] = nil
		r.mapRenderShaderWarmTasks = r.mapRenderShaderWarmTasks[1:]
		if task == nil || task.cancelled {
			continue
		}
		r.drawMapRenderStandeeShaderWarm(screen, task)
		return
	}
}

func (r *Renderer) drawMapRenderStandeeShaderWarm(target *ebiten.Image, task *mapRenderPrewarmTask) {
	if r == nil || target == nil || task == nil || task.prewarmer == nil {
		return
	}
	stickerMips := task.prewarmer.shaderStickerMips
	coreMips := task.prewarmer.shaderCoreMips
	if stickerMips == nil || coreMips == nil || len(stickerMips.levels) == 0 || len(coreMips.levels) == 0 {
		return
	}
	stickerLevel1 := min(1, len(stickerMips.levels)-1)
	stickerLevel2 := min(2, len(stickerMips.levels)-1)
	coreLevel := min(1, len(coreMips.levels)-1)
	origin := stickerMips.levels[0].Bounds().Min
	srcX, srcY := float32(origin.X)+0.5, float32(origin.Y)+0.5
	indices := []uint32{0, 1, 2, 1, 3, 2}
	shaderOpts := func() *ebiten.DrawTrianglesShaderOptions {
		opts := &ebiten.DrawTrianglesShaderOptions{}
		opts.Images[0] = stickerMips.levels[0]
		opts.Images[1] = stickerMips.levels[stickerLevel1]
		opts.Images[2] = stickerMips.levels[stickerLevel2]
		opts.Images[3] = coreMips.levels[coreLevel]
		return opts
	}
	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
		{DstX: 1, DstY: 0, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
		{DstX: 0, DstY: 1, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
		{DstX: 1, DstY: 1, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
	}
	if r.standeeTrilinearShader != nil {
		target.DrawTrianglesShader32(vertices, indices, r.standeeTrilinearShader, shaderOpts())
	}
	if r.standeeVolumeShader != nil {
		for i := range vertices {
			vertices[i].SrcX = srcX + 1
			vertices[i].ColorG = 100
			vertices[i].ColorA = standeeVolumeMinShells
			vertices[i].Custom0 = 2
			vertices[i].Custom1 = 1
			vertices[i].Custom2 = 0.5
			vertices[i].Custom3 = 0.5
		}
		target.DrawTrianglesShader32(vertices, indices, r.standeeVolumeShader, shaderOpts())
	}
}

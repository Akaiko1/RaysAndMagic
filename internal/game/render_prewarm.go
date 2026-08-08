package game

import (
	"math"
	"sort"
	"strings"

	"ugataima/internal/config"
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
	renderer           *Renderer
	imagesByName       map[string]*ebiten.Image
	uploads            map[*ebiten.Image]struct{}
	pendingUploads     map[*ebiten.Image]struct{}
	pendingUploadBytes int64
	resources          *mapRenderRegionResources
	animations         map[[2]string]struct{}
	shaderStickerMips  *mipChain
	shaderCoreMips     *mipChain
	stats              mapRenderPrewarmStats
}

func newMapRenderPrewarmer(r *Renderer) *mapRenderPrewarmer {
	return &mapRenderPrewarmer{
		renderer:       r,
		imagesByName:   make(map[string]*ebiten.Image),
		uploads:        make(map[*ebiten.Image]struct{}),
		pendingUploads: make(map[*ebiten.Image]struct{}),
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
	imageBytes := int64(bounds.Dx()) * int64(bounds.Dy()) * 4
	if prewarmUploadBatchFull(len(p.pendingUploads), p.pendingUploadBytes, imageBytes) {
		p.flushUploads(false)
	}
	p.uploads[img] = struct{}{}
	p.pendingUploads[img] = struct{}{}
	p.pendingUploadBytes += imageBytes
}

func (p *mapRenderPrewarmer) flushUploads(warmShaders bool) {
	if len(p.pendingUploads) == 0 {
		return
	}
	var stickerMips, coreMips *mipChain
	if warmShaders {
		stickerMips = p.shaderStickerMips
		coreMips = p.shaderCoreMips
	}
	p.renderer.flushPrewarmedImageUploads(p.pendingUploads, stickerMips, coreMips)
	p.pendingUploads = make(map[*ebiten.Image]struct{})
	p.pendingUploadBytes = 0
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

func (p *mapRenderPrewarmer) standee(prefix, name string, img *ebiten.Image, stableImage bool) {
	if img == nil {
		return
	}
	key := makeStandeeCoreKey(p.renderer.prefixedStandeeKeyName(prefix, name), img, stableImage)
	if _, seen := p.resources.standees[key]; seen {
		return
	}
	p.resources.standees[key] = struct{}{}
	if p.renderer.standeeCoreCache[key] != nil {
		return
	}
	img = p.renderer.boundedStandeeRenderSource(key, img)
	p.addUpload(img)
	core := p.renderer.standeeCoreSilhouette(key, img)
	p.addUpload(core)
	p.stats.standeeFrames++

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

func (p *mapRenderPrewarmer) monster(resource mapMonsterPrewarmResource) {
	if resource.spriteName == "" {
		return
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
	if !p.renderer.game.config.Graphics.Standee.Enabled {
		return
	}
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

func (r *Renderer) resetMapRenderResourceResidency() {
	if r == nil {
		return
	}
	for len(r.mapRenderResidentMapKeys) > 0 {
		evicted := r.mapRenderResidentMapKeys[0]
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
	for _, queued := range r.mapRenderResourcePrewarmMapKeys {
		if queued == mapKey {
			return
		}
	}
	r.mapRenderResourcePrewarmMapKeys = append(r.mapRenderResourcePrewarmMapKeys, mapKey)
	r.mapRenderResourcePrewarmPending = true
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

func (r *Renderer) evictMapRenderResidencyOutside(keep map[string]struct{}) {
	if r == nil || len(r.mapRenderResidentMapKeys) == 0 {
		return
	}
	evictedResources := make([]*mapRenderRegionResources, 0)
	retainedKeys := r.mapRenderResidentMapKeys[:0]
	for _, mapKey := range r.mapRenderResidentMapKeys {
		if _, retained := keep[mapKey]; retained {
			retainedKeys = append(retainedKeys, mapKey)
			continue
		}
		evictedResources = append(evictedResources, r.mapRenderResourcesByMap[mapKey])
		delete(r.mapRenderResourcesByMap, mapKey)
	}
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
	loadKeys := visibleOpenWorldMapKeys(wm, r.game.camera, tileSize, 0, 0)
	retainKeys := nearbyOpenWorldMapKeys(wm, r.game.camera, tileSize,
		mapRenderUnloadDistanceInTiles*tileSize)
	keep := make(map[string]struct{}, len(retainKeys)+1)
	for _, mapKey := range retainKeys {
		keep[mapKey] = struct{}{}
	}
	keep[currentMapKey()] = struct{}{}
	r.evictMapRenderResidencyOutside(keep)
	for _, mapKey := range loadKeys {
		r.scheduleMapRenderResourcePrewarm(mapKey)
	}
	r.scheduleMapRenderResourcePrewarm(currentMapKey())
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
		resources := r.mapRenderResourcesByMap[mapKey]
		if resources == nil {
			continue
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
	return keep
}

func (r *Renderer) deallocateMapRenderRegion(resources, keep *mapRenderRegionResources) {
	if resources == nil {
		return
	}
	if keep == nil {
		keep = &mapRenderRegionResources{}
	}
	r.deallocateStandeeKeys(resources.standees, keep.standees)
	for key := range resources.processed {
		if _, retained := keep.processed[key]; retained {
			continue
		}
		if img := r.processedSpriteCache[key]; img != nil {
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
			delete(r.wallSliceColumns, img)
			delete(r.animFrameCache, img)
		}
	}
}

// trackResidentStandeeKey accounts for an uncommon resource that escaped the
// authored prewarm inventory and was built lazily while rendering. It belongs
// to the logical region active when the renderer first needed it.
func (r *Renderer) trackResidentStandeeKey(key standeeCoreKey) {
	if r == nil {
		return
	}
	resources := r.mapRenderResourcesByMap[currentMapKey()]
	if resources != nil {
		resources.standees[key] = struct{}{}
	}
}

// prewarmMapRenderResources moves all cold render work into the map-load frame:
// PNG decode/color key, animation slicing, brightness-alpha processing, wall
// helper textures, standee cores/mips, shader compilation, and first GPU upload.
func (r *Renderer) prewarmMapRenderResources(mapKey string) (mapRenderPrewarmStats, *mapRenderRegionResources) {
	if r == nil || r.game == nil || r.game.sprites == nil {
		return mapRenderPrewarmStats{}, nil
	}
	plan := r.collectMapRenderPrewarmPlan(mapKey)
	p := newMapRenderPrewarmer(r)
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
		p.sprite(name)
	}
	for _, name := range plan.wallSprites {
		sprite := p.sprite(name)
		if sprite == nil {
			continue
		}
		p.resources.walls[sprite] = struct{}{}
		p.flushUploads(false)
		// Every affordable ripmap level is uploaded here: a level built mid-frame
		// would cost a GPU sync exactly when a distant wall first comes into view.
		if rm := r.wallRipmapFor(sprite); rm != nil {
			for _, level := range rm.owned {
				p.addUpload(level)
			}
		}
		bounds := sprite.Bounds()
		for x := 0; x < bounds.Dx(); x++ {
			r.spriteColumn(sprite, x, bounds.Dx(), bounds.Dy())
		}
		p.stats.wallTextures++
	}

	if r.game.config.Graphics.TreesAsBillboards {
		for _, name := range plan.treeSprites {
			p.standee("tree", name, p.sprite(name), true)
		}
	}
	// Unconditional: no setting turns a prop cross into a billboard. Its class
	// targets visible height, so resolve its alpha bounds before first sight.
	for _, name := range plan.crossedPropSprites {
		warmVisibleBounds(name)
		p.standee("tree", name, p.sprite(name), true)
	}
	for _, resource := range plan.environmentSprites {
		warmVisibleBounds(resource.spriteName)
		p.sprite(resource.spriteName)
		sprite := r.getProcessedSpriteByName(resource.tileType, resource.spriteName)
		if _, ok := r.processedSpriteCache[resource]; ok {
			p.resources.processed[resource] = struct{}{}
		}
		p.addUpload(sprite)
		if sprite == nil || !r.game.config.Graphics.Standee.Enabled {
			continue
		}
		frames := r.animationFrames(sprite)
		renderType := world.GlobalTileManager.GetRenderType(resource.tileType)
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
	}
	for _, name := range plan.npcDecodeSprites {
		p.sprite(name)
	}
	for _, resource := range plan.monsterDecode {
		p.decodeMonster(resource)
	}
	for _, name := range plan.containerDecode {
		p.sprite(name)
	}

	// Aura colour extraction is another first-sighting ReadPixels sync.
	if tm := world.GlobalTileManager; tm != nil {
		for _, tileType := range plan.tileTypes {
			if tileShowsImpassableAura(tm.GetTileData(tileType)) {
				r.auraTileColor(tileType)
			}
		}
	}

	for _, resource := range plan.npcSprites {
		if resource.warmVisibleBounds {
			warmVisibleBounds(resource.name)
		}
		sprite := p.sprite(resource.name)
		if sprite == nil || !r.game.config.Graphics.Standee.Enabled {
			continue
		}
		for _, frame := range r.animationFrames(sprite) {
			p.standee(resource.prefix, resource.name, frame, resource.stableImage)
		}
	}
	for _, resource := range plan.monsterSprites {
		p.monster(resource)
	}
	for _, name := range plan.containerSprites {
		sprite := p.sprite(name)
		if r.game.config.Graphics.Standee.Enabled {
			p.standee("container", name, sprite, false)
		}
	}

	p.addUpload(r.whiteImg)
	p.addUpload(r.floorColorMap)
	p.addUpload(r.floorTextureIndexMap)
	p.addUpload(r.floorTexAtlas)
	for _, name := range skyTextureNamesForMap(mapKey) {
		img := r.game.ensureSkyPanoramaCached(name)
		if img == nil {
			continue
		}
		p.resources.skies[name] = struct{}{}
		p.addUpload(img)
	}
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
	p.flushUploads(true)
	p.stats.uploadImages = len(p.uploads)
	return p.stats, p.resources
}

func (r *Renderer) prewarmPendingMapRenderResources() mapRenderPrewarmStats {
	if r == nil || !r.mapRenderResourcePrewarmPending || r.game == nil {
		return mapRenderPrewarmStats{}
	}
	// NewMMGame is built before the entry menu. Keep the expensive world
	// preparation pending until Start/Load actually enters gameplay; the same
	// deferred Update call then completes it before the player can move.
	if r.game.appScreen != AppScreenInGame {
		return mapRenderPrewarmStats{}
	}
	mapKeys := append([]string(nil), r.mapRenderResourcePrewarmMapKeys...)
	r.mapRenderResourcePrewarmPending = false
	r.mapRenderResourcePrewarmMapKeys = nil
	var total mapRenderPrewarmStats
	for _, mapKey := range mapKeys {
		if !r.prepareMapRenderResidency(mapKey) {
			continue
		}
		stats, resources := r.prewarmMapRenderResources(mapKey)
		r.commitMapRenderResidency(mapKey, resources)
		total.spriteFiles += stats.spriteFiles
		total.animationSheets += stats.animationSheets
		total.standeeFrames += stats.standeeFrames
		total.wallTextures += stats.wallTextures
		total.uploadImages += stats.uploadImages
	}
	return total
}

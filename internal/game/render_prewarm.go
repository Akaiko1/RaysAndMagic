package game

import (
	"sort"
	"strings"

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
	environmentSprites []processedSpriteKey
	npcDecodeSprites   []string
	npcSprites         []mapNPCPrewarmResource
	monsterDecode      []mapMonsterPrewarmResource
	monsterSprites     []mapMonsterPrewarmResource
	containerDecode    []string
	containerSprites   []string
}

type mapNPCPrewarmResource struct {
	name        string
	prefix      string
	stableImage bool
}

type mapMonsterPrewarmResource struct {
	key        string
	spriteName string
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
// map can reveal later. In the unified open world, mapKey scopes heavy
// standees to one placed region instead of eagerly retaining all five regions;
// decoded source sheets stay shared, and the previous region's standees remain
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
	environmentSprites := make(map[processedSpriteKey]struct{})
	npcDecodeSprites := make(map[string]struct{})
	npcSprites := make(map[mapNPCPrewarmResource]struct{})
	monsterDecode := make(map[mapMonsterPrewarmResource]struct{})
	monsterSprites := make(map[mapMonsterPrewarmResource]struct{})
	decodeMonsterKeys := make(map[string]struct{})
	monsterKeys := make(map[string]struct{})
	containerDecode := make(map[string]struct{})
	containerSprites := make(map[string]struct{})

	addContainerDecode := func(name string) {
		if name = normalizedAuthoredSpriteName(name); name != "" {
			containerDecode[name] = struct{}{}
		}
	}
	addContainerSprite := func(name string) {
		name = normalizedAuthoredSpriteName(name)
		if name == "" {
			return
		}
		containerDecode[name] = struct{}{}
		containerSprites[name] = struct{}{}
	}
	addRewardSprites := func(rewards *monster.EncounterRewards, heavy bool) {
		if rewards == nil {
			return
		}
		add := addContainerDecode
		if heavy {
			add = addContainerSprite
		}
		if rewards.TreasureChest != nil {
			add(rewards.TreasureChest.Sprite)
		}
		for i := range rewards.TreasureChests {
			add(rewards.TreasureChests[i].Sprite)
		}
	}

	if tm := world.GlobalTileManager; tm != nil {
		// Static map art is a small shared set and can be visible across a
		// stitched-region boundary at the 50-tile camera distance. Warm it for
		// the physical world once; only the much heavier NPC/monster standees
		// below are scoped to the logical region.
		for _, tileType := range r.mapRenderTileTypes {
			tileTypes[tileType] = struct{}{}
		}
		for tileType := range tileTypes {
			if isFireflySwarmTile(tileType) {
				continue // procedural motes; the legacy PNG is never drawn
			}
			renderType := tm.GetRenderType(tileType)
			switch renderType {
			case "textured_wall", "tree_sprite", "environment_sprite", "landmark":
			default:
				continue
			}
			name := normalizedAuthoredSpriteName(tm.GetSprite(tileType))
			if name == "" {
				continue
			}
			tileSprites[name] = struct{}{}
			if renderType == "textured_wall" {
				wallSprites[name] = struct{}{}
			}
		}
		for i := range r.treeTilesCache {
			name := r.treeTilesCache[i].spriteName
			if name == "" {
				name = treeStandeeSpriteName(r.treeTilesCache[i].tileType)
			}
			if name = normalizedAuthoredSpriteName(name); name != "" {
				treeSprites[name] = struct{}{}
			}
		}
		for i := range r.transparentSpritesCache {
			resource := r.transparentSpritesCache[i]
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
			prefix, stableImage := "npc", false
			if npc.GridSpanTiles < 2 && npcRenderCatOf(npc) == catLandmark {
				prefix, stableImage = "landmark", true
			}
			for _, name := range []string{baseName, visitedName} {
				if name == "" {
					continue
				}
				npcDecodeSprites[name] = struct{}{}
				if !inScope {
					continue
				}
				npcSprites[mapNPCPrewarmResource{
					name: name, prefix: prefix, stableImage: stableImage,
				}] = struct{}{}
			}
		}
		for _, summon := range npc.Summons {
			if summon != nil && summon.Monster != "" {
				decodeMonsterKeys[summon.Monster] = struct{}{}
				if inScope {
					// Resolved below together with authored and live monsters.
					monsterKeys[summon.Monster] = struct{}{}
				}
			}
		}
		if encounter := npc.EncounterData; encounter != nil {
			for _, encounterMonster := range encounter.Monsters {
				if encounterMonster == nil || encounterMonster.Type == "" {
					continue
				}
				decodeMonsterKeys[encounterMonster.Type] = struct{}{}
				if inScope {
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
		if name := normalizedAuthoredSpriteName(mon.GetSpriteType()); name != "" {
			resource := mapMonsterPrewarmResource{key: mon.Key, spriteName: name}
			monsterDecode[resource] = struct{}{}
			if inScope {
				monsterSprites[resource] = struct{}{}
			}
		}
		if mon.Key != "" {
			decodeMonsterKeys[mon.Key] = struct{}{}
			if inScope {
				monsterKeys[mon.Key] = struct{}{}
			}
		}
		for _, key := range mon.SummonMonsters {
			if key != "" {
				decodeMonsterKeys[key] = struct{}{}
				if inScope {
					monsterKeys[key] = struct{}{}
				}
			}
		}
		addRewardSprites(mon.EncounterRewards, inScope)
	}
	for _, spawn := range currentWorld.MonsterSpawns {
		if spawn.MonsterKey == "" {
			continue
		}
		decodeMonsterKeys[spawn.MonsterKey] = struct{}{}
		if scope.containsTile(spawn.X, spawn.Y) {
			monsterKeys[spawn.MonsterKey] = struct{}{}
		}
	}

	for _, pack := range r.game.config.DayNight.Packs {
		sameWorld := pack.Map == mapKey
		if wm := world.GlobalWorldManager; wm != nil {
			sameWorld = wm.WorldByKey(pack.Map) == currentWorld
		}
		if !sameWorld {
			continue
		}
		for _, night := range []bool{false, true} {
			for _, member := range pack.PhaseMembers(night) {
				if member.Monster != "" {
					decodeMonsterKeys[member.Monster] = struct{}{}
					if pack.Map == mapKey {
						monsterKeys[member.Monster] = struct{}{}
					}
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
		addContainerDecode(container.effectiveSprite())
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
	imagesByName      map[string]*ebiten.Image
	uploads           map[*ebiten.Image]struct{}
	standees          map[standeeCoreKey]struct{}
	animations        map[[2]string]struct{}
	shaderStickerMips *standeeMipChain
	shaderCoreMips    *standeeMipChain
	stats             mapRenderPrewarmStats
}

func newMapRenderPrewarmer(r *Renderer) *mapRenderPrewarmer {
	return &mapRenderPrewarmer{
		renderer:     r,
		imagesByName: make(map[string]*ebiten.Image),
		uploads:      make(map[*ebiten.Image]struct{}),
		standees:     make(map[standeeCoreKey]struct{}),
		animations:   make(map[[2]string]struct{}),
	}
}

func (p *mapRenderPrewarmer) addUpload(img *ebiten.Image) {
	if img != nil {
		p.uploads[img] = struct{}{}
	}
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
	if _, seen := p.standees[key]; seen {
		return
	}
	p.standees[key] = struct{}{}
	if p.renderer.standeeCoreCache[key] != nil {
		return
	}
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

const maxMapRenderResidentMaps = 2

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
	allKeys := make(map[standeeCoreKey]struct{}, len(r.standeeCoreCache))
	for key := range r.standeeCoreCache {
		allKeys[key] = struct{}{}
	}
	for key := range r.standeeMipCache {
		allKeys[key.frame] = struct{}{}
	}
	r.deallocateStandeeKeys(allKeys, nil)
	r.standeeCoreCache = nil
	r.standeeMipCache = nil
	r.mapRenderResidentMapKeys = nil
	r.mapRenderStandeeKeysByMap = nil
	r.mapRenderSharedResourcesReady = false
	r.mapRenderSharedStandeeKeys = nil
	r.mapRenderResourcePrewarmPending = false
	r.mapRenderResourcePrewarmMapKey = ""
}

func (r *Renderer) scheduleMapRenderResourcePrewarm(mapKey string) {
	if r == nil {
		return
	}
	r.mapRenderResourcePrewarmMapKey = mapKey
	r.mapRenderResourcePrewarmPending = true
}

// prepareMapRenderResidency returns false when mapKey is already resident.
// New regions are committed only after prewarm succeeds, so a visible old
// region is never evicted and immediately rebuilt during the same crossing.
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

// commitMapRenderResidency installs a completed region and then enforces the
// LRU limit. The temporary third set lets the keep-set preserve standees shared
// by the newly entered region and the previous side of the seam.
func (r *Renderer) commitMapRenderResidency(mapKey string, standeeKeys map[standeeCoreKey]struct{}) {
	if r.mapRenderStandeeKeysByMap == nil {
		r.mapRenderStandeeKeysByMap = make(map[string]map[standeeCoreKey]struct{})
	}
	r.mapRenderStandeeKeysByMap[mapKey] = standeeKeys
	r.mapRenderResidentMapKeys = append(r.mapRenderResidentMapKeys, mapKey)
	for len(r.mapRenderResidentMapKeys) > maxMapRenderResidentMaps {
		evicted := r.mapRenderResidentMapKeys[0]
		r.mapRenderResidentMapKeys = r.mapRenderResidentMapKeys[1:]
		keep := make(map[standeeCoreKey]struct{}, len(r.mapRenderSharedStandeeKeys))
		for key := range r.mapRenderSharedStandeeKeys {
			keep[key] = struct{}{}
		}
		for _, resident := range r.mapRenderResidentMapKeys {
			for key := range r.mapRenderStandeeKeysByMap[resident] {
				keep[key] = struct{}{}
			}
		}
		r.deallocateStandeeKeys(r.mapRenderStandeeKeysByMap[evicted], keep)
		delete(r.mapRenderStandeeKeysByMap, evicted)
	}
}

// trackResidentStandeeKey accounts for an uncommon resource that escaped the
// authored prewarm inventory and was built lazily while rendering. Registering
// cache hits as well as new cores matters at open-world seams: the same token
// can be visible from both logical regions and must belong to both LRU sets.
func (r *Renderer) trackResidentStandeeKey(key standeeCoreKey) {
	if r == nil || !r.mapRenderSharedResourcesReady {
		return
	}
	if _, shared := r.mapRenderSharedStandeeKeys[key]; shared {
		return
	}
	keys := r.mapRenderStandeeKeysByMap[currentMapKey()]
	if keys != nil {
		keys[key] = struct{}{}
	}
}

// prewarmMapRenderResources moves all cold render work into the map-load frame:
// PNG decode/color key, animation slicing, brightness-alpha processing, wall
// helper textures, standee cores/mips, shader compilation, and first GPU upload.
func (r *Renderer) prewarmMapRenderResources(mapKey string) (mapRenderPrewarmStats, map[standeeCoreKey]struct{}) {
	if r == nil || r.game == nil || r.game.sprites == nil {
		return mapRenderPrewarmStats{}, nil
	}
	plan := r.collectMapRenderPrewarmPlan(mapKey)
	p := newMapRenderPrewarmer(r)

	buildShared := !r.mapRenderSharedResourcesReady
	if buildShared {
		for _, name := range plan.tileSprites {
			p.sprite(name)
		}
		for _, name := range plan.wallSprites {
			sprite := p.sprite(name)
			if sprite == nil {
				continue
			}
			repeated := r.repeatedWallTexture(sprite)
			p.addUpload(repeated)
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
		for _, resource := range plan.environmentSprites {
			sprite := r.getProcessedSpriteByName(resource.tileType, resource.spriteName)
			p.addUpload(sprite)
			if sprite == nil || !r.game.config.Graphics.Standee.Enabled {
				continue
			}
			frames := r.animationFrames(sprite)
			renderType := world.GlobalTileManager.GetRenderType(resource.tileType)
			for _, frame := range frames {
				switch {
				case renderType == "landmark":
					p.standee("landmark", resource.spriteName, frame, true)
				case world.GlobalTileManager.IsWallMounted(resource.tileType):
					// The wall-mounted path falls back to a centered tile standee if
					// the authored tile has no adjacent wall; prepare both exact keys.
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
				if tm.IsSolid(tileType) && isAuraBillboardRenderType(tm.GetRenderType(tileType)) {
					r.auraTileColor(tileType)
				}
			}
		}
		r.mapRenderSharedStandeeKeys = make(map[standeeCoreKey]struct{}, len(p.standees))
		for key := range p.standees {
			r.mapRenderSharedStandeeKeys[key] = struct{}{}
		}
	}

	for _, resource := range plan.npcSprites {
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

	regionStandeeKeys := make(map[standeeCoreKey]struct{}, len(p.standees))
	for key := range p.standees {
		if _, shared := r.mapRenderSharedStandeeKeys[key]; !shared {
			regionStandeeKeys[key] = struct{}{}
		}
	}

	p.addUpload(r.whiteImg)
	p.addUpload(r.floorColorMap)
	p.addUpload(r.floorTextureIndexMap)
	p.addUpload(r.floorTexAtlas)
	p.addUpload(r.game.skyPanorama)
	p.addUpload(r.game.skyPanoramaPrev)
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
	r.flushPrewarmedImageUploads(p.uploads, p.shaderStickerMips, p.shaderCoreMips)
	p.stats.uploadImages = len(p.uploads)
	if buildShared {
		r.mapRenderSharedResourcesReady = true
	}
	return p.stats, regionStandeeKeys
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
	mapKey := r.mapRenderResourcePrewarmMapKey
	r.mapRenderResourcePrewarmPending = false
	r.mapRenderResourcePrewarmMapKey = ""
	if !r.prepareMapRenderResidency(mapKey) {
		return mapRenderPrewarmStats{}
	}
	stats, standeeKeys := r.prewarmMapRenderResources(mapKey)
	r.commitMapRenderResidency(mapKey, standeeKeys)
	return stats
}

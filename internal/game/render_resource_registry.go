package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

type renderResourceKind uint8

const (
	renderResourceSource renderResourceKind = iota
	renderResourceProcessed
	renderResourceStandee
	renderResourceWall
	renderResourceSky
)

type renderResourceID struct {
	kind      renderResourceKind
	source    mapRenderSourceKey
	processed processedSpriteKey
	standee   standeeCoreKey
	image     *ebiten.Image
	name      string
}

type renderResourceOwner struct {
	region string
	task   *mapRenderPrewarmTask
	role   string
}

type renderResourceRecord struct {
	revision     uint64
	owners       map[renderResourceOwner]struct{}
	dependencies map[renderResourceID]struct{}
	owned        map[*ebiten.Image]struct{}
	forget       func()
}

// Region manifests describe scope; this registry owns the allocation graph.
// Views are dependencies, never allocations. The release callback detaches
// caches only; the registry releases shared allocation roots exactly once.
type renderResourceRegistry struct {
	records     map[renderResourceID]*renderResourceRecord
	allocations map[*ebiten.Image]map[renderResourceID]struct{}
	dependents  map[renderResourceID]map[renderResourceID]struct{}
	revision    uint64
	release     func(*ebiten.Image)
}

func newRenderResourceRegistry() *renderResourceRegistry {
	return &renderResourceRegistry{
		records:     make(map[renderResourceID]*renderResourceRecord),
		allocations: make(map[*ebiten.Image]map[renderResourceID]struct{}),
		dependents:  make(map[renderResourceID]map[renderResourceID]struct{}),
		release:     func(img *ebiten.Image) { img.Deallocate() },
	}
}

func (reg *renderResourceRegistry) put(id renderResourceID, images []*ebiten.Image, dependencies []renderResourceID, forget func()) {
	record := reg.records[id]
	if record == nil {
		reg.revision++
		record = &renderResourceRecord{revision: reg.revision, owners: make(map[renderResourceOwner]struct{})}
		reg.records[id] = record
	}
	nextOwned := make(map[*ebiten.Image]struct{}, len(images))
	for _, img := range images {
		if img != nil {
			nextOwned[img] = struct{}{}
		}
	}
	changed := len(record.owned) != len(nextOwned)
	if !changed {
		for img := range nextOwned {
			if _, ok := record.owned[img]; !ok {
				changed = true
				break
			}
		}
	}
	if changed && record.owned != nil {
		reg.revision++
		record.revision = reg.revision
	}
	for img := range record.owned {
		delete(reg.allocations[img], id)
		if len(reg.allocations[img]) == 0 {
			delete(reg.allocations, img)
		}
	}
	for dep := range record.dependencies {
		delete(reg.dependents[dep], id)
	}
	record.owned = nextOwned
	for img := range nextOwned {
		if reg.allocations[img] == nil {
			reg.allocations[img] = make(map[renderResourceID]struct{})
		}
		reg.allocations[img][id] = struct{}{}
	}
	record.dependencies = make(map[renderResourceID]struct{}, len(dependencies))
	for _, dep := range dependencies {
		if dep == id {
			continue
		}
		record.dependencies[dep] = struct{}{}
		if reg.dependents[dep] == nil {
			reg.dependents[dep] = make(map[renderResourceID]struct{})
		}
		reg.dependents[dep][id] = struct{}{}
	}
	record.forget = forget
}

func (reg *renderResourceRegistry) retain(id renderResourceID, owner renderResourceOwner) {
	if record := reg.records[id]; record != nil {
		record.owners[owner] = struct{}{}
	}
}

func (reg *renderResourceRegistry) releaseResources(candidates map[renderResourceID]struct{}) {
	protected := make(map[renderResourceID]bool)
	var protect func(renderResourceID)
	protect = func(id renderResourceID) {
		if protected[id] {
			return
		}
		protected[id] = true
		if record := reg.records[id]; record != nil {
			for dep := range record.dependencies {
				protect(dep)
			}
		}
	}
	for id, record := range reg.records {
		if len(record.owners) > 0 {
			protect(id)
		}
	}
	visited := make(map[renderResourceID]bool)
	order := []renderResourceID{}
	var visit func(renderResourceID)
	visit = func(id renderResourceID) {
		if visited[id] || protected[id] {
			return
		}
		visited[id] = true
		for dependent := range reg.dependents[id] {
			visit(dependent)
		}
		order = append(order, id)
	}
	for id := range candidates {
		visit(id)
	}
	released := make(map[*ebiten.Image]struct{})
	for _, id := range order {
		record := reg.records[id]
		if record == nil {
			continue
		}
		if record.forget != nil {
			record.forget()
		}
		for img := range record.owned {
			delete(reg.allocations[img], id)
			if len(reg.allocations[img]) == 0 {
				delete(reg.allocations, img)
				released[img] = struct{}{}
			}
		}
		for dep := range record.dependencies {
			delete(reg.dependents[dep], id)
		}
		delete(reg.dependents, id)
		delete(reg.records, id)
	}
	for img := range released {
		reg.release(img)
	}
}

func (reg *renderResourceRegistry) estimatedGPUBytes() int64 {
	var bytes int64
	for img := range reg.allocations {
		b := img.Bounds()
		bytes += int64(b.Dx()) * int64(b.Dy()) * 4
	}
	return bytes
}

func (r *Renderer) indexAnimationViews(source *ebiten.Image, frames []*ebiten.Image) {
	if r.animFrameOrigins == nil {
		r.animFrameOrigins = make(map[*ebiten.Image]*ebiten.Image)
	}
	for _, frame := range frames {
		if frame != nil && frame != source {
			r.animFrameOrigins[frame] = source
		}
	}
}

func (r *Renderer) indexProcessedSource(key processedSpriteKey, img *ebiten.Image) {
	if img == nil {
		return
	}
	if r.processedSpriteOrigins == nil {
		r.processedSpriteOrigins = make(map[*ebiten.Image]processedSpriteKey)
	}
	r.processedSpriteOrigins[img] = key
}

func (r *Renderer) forgetRenderSourceViews(img *ebiten.Image) {
	for _, frame := range r.animFrameCache[img] {
		delete(r.animFrameOrigins, frame)
	}
	delete(r.animFrameCache, img)
	delete(r.wallSliceColumns, img)
}

func sourceResourceID(key mapRenderSourceKey) renderResourceID {
	return renderResourceID{kind: renderResourceSource, source: key}
}

func (r *Renderer) resourceOrigin(img *ebiten.Image) (renderResourceID, bool) {
	if key, ok := r.processedSpriteOrigins[img]; ok {
		return renderResourceID{kind: renderResourceProcessed, processed: key}, true
	}
	if root := r.animFrameOrigins[img]; root != nil {
		img = root
	}
	if r.game != nil && r.game.sprites != nil {
		if request, ok := r.game.sprites.ResourceForImage(img); ok {
			return sourceResourceID(mapRenderSourceKey{name: request.Name, animationType: request.AnimationType}), true
		}
	}
	return renderResourceID{}, false
}

func (r *Renderer) registerSourceResource(key mapRenderSourceKey) renderResourceID {
	id := sourceResourceID(key)
	var images []*ebiten.Image
	if r.game != nil && r.game.sprites != nil {
		images = r.game.sprites.ResourceImages(graphics.SpriteResourceRequest{Name: key.name, AnimationType: key.animationType})
	}
	r.mapRenderRegistry.put(id, images, nil, func() {
		if r.game != nil && r.game.sprites != nil {
			for _, img := range r.game.sprites.DetachResource(key.name, key.animationType) {
				r.forgetRenderSourceViews(img)
			}
		}
	})
	return id
}

func (r *Renderer) registerProcessedResource(key processedSpriteKey) renderResourceID {
	id := renderResourceID{kind: renderResourceProcessed, processed: key}
	img := r.processedSpriteCache[key]
	r.indexProcessedSource(key, img)
	parent := r.registerSourceResource(mapRenderSourceKey{name: key.spriteName})
	r.mapRenderRegistry.put(id, []*ebiten.Image{img}, []renderResourceID{parent}, func() {
		r.forgetRenderSourceViews(img)
		delete(r.processedSpriteOrigins, img)
		delete(r.processedSpriteCache, key)
	})
	return id
}

func (r *Renderer) registerStandeeResource(key standeeCoreKey) renderResourceID {
	id := renderResourceID{kind: renderResourceStandee, standee: key}
	images := []*ebiten.Image{r.standeeCoreCache[key], r.standeeRenderSourceCache[key]}
	dependencies := []renderResourceID{}
	for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
		if chain := r.standeeMipCache[standeeMipKey{frame: key, layer: layer}]; chain != nil {
			images = append(images, chain.owned...)
			if len(chain.levels) > 0 {
				if parent, ok := r.resourceOrigin(chain.levels[0]); ok {
					dependencies = append(dependencies, parent)
				}
			}
		}
	}
	if parent, ok := r.resourceOrigin(key.img); ok {
		dependencies = append(dependencies, parent)
	}
	r.mapRenderRegistry.put(id, images, dependencies, func() {
		delete(r.standeeCoreCache, key)
		delete(r.standeeRenderSourceCache, key)
		for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
			delete(r.standeeMipCache, standeeMipKey{frame: key, layer: layer})
		}
	})
	return id
}

func (r *Renderer) registerWallResource(img *ebiten.Image) renderResourceID {
	id := renderResourceID{kind: renderResourceWall, image: img}
	var owned []*ebiten.Image
	if rm := r.wallRipmaps[img]; rm != nil {
		owned = rm.owned
	}
	var deps []renderResourceID
	if parent, ok := r.resourceOrigin(img); ok {
		deps = append(deps, parent)
	}
	r.mapRenderRegistry.put(id, owned, deps, func() {
		if rm := r.wallRipmaps[img]; rm != nil {
			r.wallRipmapBytes = max(int64(0), r.wallRipmapBytes-rm.bytes)
		}
		delete(r.wallRipmaps, img)
	})
	return id
}

func (r *Renderer) registerSkyResource(name string) renderResourceID {
	id := renderResourceID{kind: renderResourceSky, name: name}
	var img *ebiten.Image
	if r.game != nil {
		img = r.game.skyPanoramaCache[name]
	}
	r.mapRenderRegistry.put(id, []*ebiten.Image{img}, nil, func() {
		if r.game != nil {
			delete(r.game.skyPanoramaCache, name)
		}
	})
	if img != nil {
		if img == r.game.skyPanorama {
			r.mapRenderRegistry.retain(id, renderResourceOwner{role: "current-sky"})
		}
		if img == r.game.skyPanoramaPrev {
			r.mapRenderRegistry.retain(id, renderResourceOwner{role: "fading-sky"})
		}
	}
	return id
}

func (r *Renderer) registerResourceManifest(resources *mapRenderRegionResources, owner *renderResourceOwner) map[renderResourceID]struct{} {
	ids := make(map[renderResourceID]struct{})
	if resources == nil {
		return ids
	}
	add := func(id renderResourceID) {
		ids[id] = struct{}{}
		if owner != nil {
			r.mapRenderRegistry.retain(id, *owner)
		}
	}
	for key := range resources.sources {
		add(r.registerSourceResource(key))
	}
	for key := range resources.processed {
		add(r.registerProcessedResource(key))
	}
	for key := range resources.standees {
		add(r.registerStandeeResource(key))
	}
	for img := range resources.walls {
		add(r.registerWallResource(img))
	}
	for name := range resources.skies {
		add(r.registerSkyResource(name))
	}
	return ids
}

// Refresh at ownership boundaries, not per sprite. Include unmanifested lazy
// derivatives so final source release still invalidates all aliased children.
func (r *Renderer) refreshRenderResourceRegistry() {
	if r.mapRenderRegistry == nil {
		r.mapRenderRegistry = newRenderResourceRegistry()
	}
	for _, record := range r.mapRenderRegistry.records {
		clear(record.owners)
	}
	for key := range r.processedSpriteCache {
		r.registerProcessedResource(key)
	}
	keys := make(map[standeeCoreKey]struct{})
	for key := range r.standeeCoreCache {
		keys[key] = struct{}{}
	}
	for key := range r.standeeMipCache {
		keys[key.frame] = struct{}{}
	}
	for key := range keys {
		r.registerStandeeResource(key)
	}
	for img := range r.wallRipmaps {
		r.registerWallResource(img)
	}
	if r.game != nil {
		for name := range r.game.skyPanoramaCache {
			r.registerSkyResource(name)
		}
	}
	for _, mapKey := range r.mapRenderResidentMapKeys {
		owner := renderResourceOwner{region: mapKey, role: "region"}
		r.registerResourceManifest(r.mapRenderResourcesByMap[mapKey], &owner)
	}
	if task := r.mapRenderResourcePrewarmActive; r.mapRenderTaskCurrent(task) && task.prewarmer != nil {
		owner := renderResourceOwner{region: task.mapKey, task: task, role: "preparation"}
		r.registerResourceManifest(task.prewarmer.resources, &owner)
		if task.spriteCommit != nil && task.spriteCommitReq.Name != "" {
			id := r.registerSourceResource(mapRenderSourceKey{name: task.spriteCommitReq.Name, animationType: task.spriteCommitReq.AnimationType})
			r.mapRenderRegistry.retain(id, owner)
		}
	}
	for _, upload := range r.mapRenderUploadQueue {
		if r.mapRenderTaskCurrent(upload.task) {
			for id := range r.mapRenderRegistry.allocations[upload.image] {
				r.mapRenderRegistry.retain(id, renderResourceOwner{task: upload.task, role: "upload"})
			}
		}
	}
	for _, task := range r.mapRenderShaderWarmTasks {
		if r.mapRenderTaskCurrent(task) && task.prewarmer != nil {
			owner := renderResourceOwner{task: task, role: "shader"}
			r.registerResourceManifest(task.prewarmer.resources, &owner)
		}
	}
}

// CPU bytes are retained decoded pixels, separately from estimated owned GPU
// pixels. The same RGBA referenced by multiple views is counted once.
func (r *Renderer) retainedRenderCPUBytes() int64 {
	seen := make(map[*image.RGBA]struct{})
	var bytes int64
	add := func(cpu *image.RGBA) {
		if cpu == nil {
			return
		}
		if _, ok := seen[cpu]; !ok {
			seen[cpu] = struct{}{}
			bytes += int64(len(cpu.Pix))
		}
	}
	if task := r.mapRenderResourcePrewarmActive; task != nil {
		for _, cpu := range task.cpuImages {
			add(cpu)
		}
		for _, job := range task.standeeJobs {
			add(job.cpu)
		}
		if task.standeeCommit != nil {
			for _, write := range task.standeeCommit.writes {
				add(write.cpu)
			}
		}
		if task.skyCommit != nil {
			add(task.skyCommit.cpu)
		}
		task.spriteCommit.VisitCommitPixels(add)
		for _, builder := range task.wallRipmapBuilders {
			if builder != nil {
				add(builder.cpuRow)
				add(builder.cpuLevel)
			}
		}
	}
	for _, cpu := range r.lazySpriteCPUPixels {
		add(cpu)
	}
	return bytes
}

func (r *Renderer) cacheProcessedSprite(key processedSpriteKey, img *ebiten.Image) {
	if r.processedSpriteCache == nil {
		r.processedSpriteCache = make(map[processedSpriteKey]*ebiten.Image)
	}
	r.processedSpriteCache[key] = img
	r.indexProcessedSource(key, img)
}

type renderResidencyStats struct {
	resources, allocations                                                    int
	ownedGPUBytes, retainedCPUBytes, queuedCPUReservation, peakCPUReservation int64
}

func (r *Renderer) renderResourceStats() renderResidencyStats {
	if r == nil {
		return renderResidencyStats{}
	}
	r.refreshRenderResourceRegistry()
	stats := renderResidencyStats{resources: len(r.mapRenderRegistry.records), allocations: len(r.mapRenderRegistry.allocations), ownedGPUBytes: r.mapRenderRegistry.estimatedGPUBytes(), retainedCPUBytes: r.retainedRenderCPUBytes()}
	if task := r.mapRenderResourcePrewarmActive; task != nil {
		stats.queuedCPUReservation, stats.peakCPUReservation = task.queueBudget.Usage()
	}
	return stats
}

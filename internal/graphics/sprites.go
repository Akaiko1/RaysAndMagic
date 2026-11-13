package graphics

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/config"
	"ugataima/internal/spritecatalog"
)

type SpriteManager struct {
	iconFrames      map[string]iconFrameEntry
	deferResource   func(SpriteResourceRequest) bool
	failedResources map[SpriteResourceRequest]bool
	decodeGate      *WorkerGate

	imageResources       map[*ebiten.Image]SpriteResourceRequest
	sprites              map[string]*ebiten.Image
	spriteTypeCache      map[string]string // Cache sprite types to avoid repeated file checks
	animations           map[animationCacheKey]*SpriteAnimation
	animationMissing     map[animationCacheKey]bool
	animationFrameCounts map[animationCacheKey]int
	// CPU alpha masks for the rare pixel-perfect hit tests. Reading an
	// *ebiten.Image with At/ReadPixels flushes the GPU command queue, and At
	// reads the WHOLE image back - a stall per interaction probe.
	alphaMasks map[string]*spriteAlphaMask
	// Compact visible-frame geometry used by visual-size normalization. Unlike
	// alphaMasks, each entry retains only a rectangle and frame dimensions.
	visibleFrameBounds map[string]spriteVisibleFrameBounds
	// iconArtBounds caches IconArtBounds per icon; an empty rectangle is a
	// cached failure. Authored icons never change while the game runs.
	iconArtBounds map[string]image.Rectangle

	// spritePaths maps a sprite basename (no extension) to its PNG path, built
	// once by walking the sprite roots recursively (see ensureIndex). Lets
	// sprites live in any subfolder layout - lookup is by name, not location.
	// spriteDirType records the originating root's placeholder type.
	spritePaths   map[string]string
	spriteDirType map[string]string

	// Load-time color key (set by ConfigureColorKey). Key pixels (within keyTol
	// of the key color) joined to the background become transparent; keyFull
	// families lose every key pixel. With keyDespill, only listed families have
	// the magenta cast subtracted, kept opaque: keyFull everywhere, keyEdgeOnly
	// at the rim.
	keyEnabled bool
	keyR       uint8
	keyG       uint8
	keyB       uint8
	keyTol     int
	keyDespill bool
	// Sprite families whose rim keeps a magenta cast: despill only within
	// keyEdgeRadius px of the background or the image border, never the body.
	keyEdgeOnly   map[string]bool
	keyEdgeRadius int
	// Sprite families with key residue inside the body: key every key-close
	// pixel and despill everywhere.
	keyFull map[string]bool
	// lazyResourceObserver assigns synchronous fallback loads to the renderer's
	// current region. It also receives the created root images with their
	// decoded CPU pixels so same-frame derived builders (standee cores, mips)
	// can skip the ReadPixels round trip. Background prepared commits
	// deliberately do not notify it; their owner is the prewarm task manifest.
	lazyResourceObserver func(SpriteResourceRequest, map[*ebiten.Image]*image.RGBA)
}

type spriteAlphaMask struct {
	width, height int
	alpha         []uint8
}

type spriteVisibleFrameBounds struct {
	bounds                  image.Rectangle
	frameWidth, frameHeight int
	known                   bool
}

type animationCacheKey struct {
	name     string
	animType string
}

// SpriteResourceRequest identifies one authored PNG-backed render source. An
// empty AnimationType requests a static sprite; otherwise it requests one
// animation sheet. The CPU loader uses this small value across its goroutine
// boundary and leaves every Ebitengine call on the game-loop goroutine.
type SpriteResourceRequest struct {
	Name          string
	AnimationType string
}

func (sm *SpriteManager) SetLazyResourceObserver(observer func(SpriteResourceRequest, map[*ebiten.Image]*image.RGBA)) {
	if sm == nil {
		return
	}
	sm.lazyResourceObserver = observer
}

// ResourceForImage resolves a published allocation in constant time. Entries
// are installed at publication and removed with the source cache entry.
func (sm *SpriteManager) ResourceForImage(img *ebiten.Image) (SpriteResourceRequest, bool) {
	if sm == nil || img == nil {
		return SpriteResourceRequest{}, false
	}
	request, ok := sm.imageResources[img]
	return request, ok
}

// PreparedSpriteResource is a decoded and color-keyed CPU image. Found is
// false for an absent or invalid source. CommitPreparedResource is the only
// path that turns it into Ebitengine images.
type PreparedSpriteResource struct {
	metadataReady bool
	visible       spriteVisibleFrameBounds
	alpha         *spriteAlphaMask

	QueueLease *PreparationLease
	Request    SpriteResourceRequest
	Image      image.Image
	CPU        *image.RGBA
	Frames     []*image.RGBA
	Found      bool
}

type preparedSpriteTarget struct {
	image *ebiten.Image
	cpu   *image.RGBA
	row   int
}

// PreparedSpriteCommit incrementally copies CPU pixels into Ebitengine images.
// It lets callers cap WritePixels work per Update while the synchronous sprite
// APIs can still drain the same state in one call outside gameplay streaming.
type PreparedSpriteCommit struct {
	manager   *SpriteManager
	request   SpriteResourceRequest
	targets   []preparedSpriteTarget
	completed []preparedSpriteTarget
	result    map[*ebiten.Image]*image.RGBA
	done      bool
}

// Cancel releases every image allocated by an unfinished commit. Streaming
// callers use this when a region leaves residency before all pixel rows have
// been written; waiting for the Go GC would make the temporary GPU allocation
// lifetime nondeterministic on memory-constrained devices.
func (c *PreparedSpriteCommit) Cancel() {
	if c == nil || c.done {
		return
	}
	for i := range c.targets {
		if c.targets[i].image != nil {
			c.targets[i].image.Deallocate()
		}
		c.targets[i] = preparedSpriteTarget{}
	}
	for i := range c.completed {
		if c.completed[i].image != nil {
			c.completed[i].image.Deallocate()
		}
		c.completed[i] = preparedSpriteTarget{}
	}
	c.targets = nil
	c.completed = nil
	c.result = nil
	c.done = true
}

// despillHueFloor is the magenta excess (min(R,B)-G) at or below which despill
// leaves a pixel alone. Low on purpose: inside a despill scope (the rim band of
// an edge-only family, all of a full family) even a faint cast is residue.
// Outside it purple and pink are art and never reach despill.
const despillHueFloor = 8

// despillEdgeRadiusDefault is the rim band (px from the background or the
// image border) despilled in edge-only families when the config leaves the
// radius unset.
const despillEdgeRadiusDefault = 3

// SetDespillEdgeOnly despills the rim of a whole animation family, whether the
// configuration names its base sprite or any directional sheet. Other sprite
// names remain exact matches. Configure before loading resources.
func (sm *SpriteManager) SetDespillEdgeOnly(names []string, radius int) {
	sm.keyEdgeOnly = make(map[string]bool, len(names))
	for _, n := range names {
		sm.keyEdgeOnly[spriteDespillFamily(n)] = true
	}
	sm.keyEdgeRadius = radius
}

// SetDespillFull keys every key-close pixel of each named animation family and
// despills it everywhere, whether the configuration names its base sprite or
// any directional sheet. Configure before loading resources.
func (sm *SpriteManager) SetDespillFull(names []string) {
	sm.keyFull = make(map[string]bool, len(names))
	for _, n := range names {
		sm.keyFull[spriteDespillFamily(n)] = true
	}
}

// IsEdgeOnlyDespill reports whether the named sprite's family is edge-only.
func (sm *SpriteManager) IsEdgeOnlyDespill(name string) bool {
	return sm.keyEdgeOnly[spriteDespillFamily(name)]
}

// Directional motion suffixes belong to one source identity. Strip only the
// complete suffix, never an arbitrary prefix (lich and lich_king are distinct).
func spriteDespillFamily(name string) string {
	if !strings.HasSuffix(name, "_r") && !strings.HasSuffix(name, "_l") {
		return name
	}
	stem := name[:len(name)-2]
	separator := strings.LastIndexByte(stem, '_')
	if separator < 1 {
		return name
	}
	switch stem[separator+1:] {
	case "walking", "attacking", "dying", "climbing", "descending", "jumping", "perched", "leaping":
		return stem[:separator]
	}
	return name
}

// ConfigureColorKey applies graphics.color_key: key colour (magenta when
// unset), tolerance, despill and both family lists. The game, the map editor
// and the asset tests all configure through it, so they key alike.
func (sm *SpriteManager) ConfigureColorKey(ck config.ColorKeyConfig) {
	r, g, b := ck.Color[0], ck.Color[1], ck.Color[2]
	if r == 0 && g == 0 && b == 0 {
		r, g, b = 255, 0, 255
	}
	sm.SetColorKey(ck.Enabled, r, g, b, ck.Tolerance, ck.Despill)
	sm.SetDespillEdgeOnly(ck.EdgeOnlyDespill, ck.EdgeDespillRadius)
	sm.SetDespillFull(ck.FullDespill)
}

// SetColorKey enables/configures the load-time color key (see SpriteManager).
// Must be called before sprites are first loaded to take effect on them.
func (sm *SpriteManager) SetColorKey(enabled bool, r, g, b, tolerance int, despill bool) {
	sm.keyEnabled = enabled
	sm.keyR, sm.keyG, sm.keyB = uint8(r), uint8(g), uint8(b)
	sm.keyTol = tolerance
	sm.keyDespill = despill
}

// isKeyColor reports whether a pixel lies within keyTol of the key per channel.
func (sm *SpriteManager) isKeyColor(p color.NRGBA) bool {
	return max(int(p.R)-int(sm.keyR), int(sm.keyR)-int(p.R)) <= sm.keyTol &&
		max(int(p.G)-int(sm.keyG), int(sm.keyG)-int(p.G)) <= sm.keyTol &&
		max(int(p.B)-int(sm.keyB), int(sm.keyB)-int(p.B)) <= sm.keyTol
}

// hasKeyPixel reports whether any visible pixel of src is a key pixel, in the
// colour the loader's NRGBA copy would hold.
func (sm *SpriteManager) hasKeyPixel(src image.Image) bool {
	b := src.Bounds()
	switch img := src.(type) {
	case *image.NRGBA:
		return sm.hasKeyPixelRows(img.Pix, img.Stride, img.PixOffset(b.Min.X, b.Min.Y), b.Dx(), b.Dy(), false)
	case *image.RGBA:
		return sm.hasKeyPixelRows(img.Pix, img.Stride, img.PixOffset(b.Min.X, b.Min.Y), b.Dx(), b.Dy(), true)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if p := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA); p.A != 0 && sm.isKeyColor(p) {
				return true
			}
		}
	}
	return false
}

// hasKeyPixelRows walks w*h 4-byte pixels from offset, rows stride apart;
// premultiplied pixels convert to NRGBA as the loader's copy does.
func (sm *SpriteManager) hasKeyPixelRows(pix []byte, stride, offset, w, h int, premultiplied bool) bool {
	for y := 0; y < h; y++ {
		row := pix[offset+y*stride : offset+y*stride+w*4]
		for i := 0; i < len(row); i += 4 {
			p := color.NRGBA{row[i], row[i+1], row[i+2], row[i+3]}
			if premultiplied && p.A != 255 && p.A != 0 {
				p = color.NRGBAModel.Convert(color.RGBA(p)).(color.NRGBA)
			}
			if p.A != 0 && sm.isKeyColor(p) {
				return true
			}
		}
	}
	return false
}

// applyColorKey returns src with the key color removed: a copy, or src itself
// when nothing can change (key off, a default sprite without key pixels).
// Three modes by family:
//   - default: the key erases background alone (pixels within keyTol of the key
//     8-connected to the image border or a transparent pixel); every other
//     pixel, key-close art inside the body included, stays as drawn.
//   - edge-only: as default, then with keyDespill every magenta-hue pixel
//     (R and B above G) within keyEdgeRadius px of that background or of the
//     image border has the excess subtracted and stays opaque.
//   - full: every key-close pixel goes transparent and despill runs everywhere.
//
// Despill assumes a magenta-style key (high R,B / low G).
func (sm *SpriteManager) applyColorKey(name string, src image.Image) image.Image {
	if !sm.keyEnabled || src == nil {
		return src
	}
	family := spriteDespillFamily(name)
	full := sm.keyFull[family]
	edgeOnly := !full && sm.keyEdgeOnly[family]
	// A default sprite without a key pixel has nothing to erase: skip the copy.
	if !full && !edgeOnly && !sm.hasKeyPixel(src) {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	// Pass 1: erase the key core. Outside a full family, the outside (trans) is
	// the transparency plus the key flooded in from it and from the image border.
	var trans []bool
	if !full {
		trans = make([]bool, w*h)
		for y := 0; y < h; y++ {
			row := dst.Pix[y*dst.Stride:]
			for x := 0; x < w; x++ {
				trans[y*w+x] = row[x*4+3] == 0
			}
		}
		var queue []int
		// erase clears a key pixel joined to the outside and queues it.
		erase := func(x, y int) {
			if trans[y*w+x] || !sm.isKeyColor(dst.NRGBAAt(b.Min.X+x, b.Min.Y+y)) {
				return
			}
			trans[y*w+x] = true
			dst.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{})
			queue = append(queue, y*w+x)
		}
		joined := func(x, y int) bool {
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				return true
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if trans[(y+dy)*w+x+dx] {
						return true
					}
				}
			}
			return false
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				// Most pixels are art: test the key before the neighbourhood.
				if !trans[y*w+x] && sm.isKeyColor(dst.NRGBAAt(b.Min.X+x, b.Min.Y+y)) && joined(x, y) {
					erase(x, y)
				}
			}
		}
		for len(queue) > 0 {
			i := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			x, y := i%w, i/w
			for ny := max(y-1, 0); ny <= min(y+1, h-1); ny++ {
				for nx := max(x-1, 0); nx <= min(x+1, w-1); nx++ {
					erase(nx, ny)
				}
			}
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if p := dst.NRGBAAt(b.Min.X+x, b.Min.Y+y); p.A != 0 && sm.isKeyColor(p) {
					dst.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{})
				}
			}
		}
	}
	if !sm.keyDespill || !full && !edgeOnly {
		return dst
	}

	radius := sm.keyEdgeRadius
	if radius <= 0 {
		radius = despillEdgeRadiusDefault
	}
	nearEdge := func(x, y int) bool {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					return true // image border is an edge
				}
				if trans[ny*w+nx] {
					return true
				}
			}
		}
		return false
	}

	// Pass 2: despill kept pixels. Edge-only sprites despill the fringe band
	// only, so their intentional interior magenta survives.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := b.Min.X+x, b.Min.Y+y
			p := dst.NRGBAAt(px, py)
			if p.A == 0 {
				continue
			}
			excess := int(min(p.R, p.B)) - int(p.G)
			if excess <= despillHueFloor {
				continue
			}
			if edgeOnly && !nearEdge(x, y) {
				continue
			}
			p.R = uint8(int(p.R) - excess)
			p.B = uint8(int(p.B) - excess)
			dst.SetNRGBA(px, py, p)
		}
	}
	return dst
}

func NewSpriteManager() *SpriteManager {
	return &SpriteManager{
		iconFrames:         loadIconFrameEntries(),
		sprites:            make(map[string]*ebiten.Image),
		spriteTypeCache:    make(map[string]string),
		animations:         make(map[animationCacheKey]*SpriteAnimation),
		animationMissing:   make(map[animationCacheKey]bool),
		alphaMasks:         make(map[string]*spriteAlphaMask),
		visibleFrameBounds: make(map[string]spriteVisibleFrameBounds),
	}
}

// ensureIndex lazily builds this manager's basename->path index.
func (sm *SpriteManager) ensureIndex() {
	if sm == nil || sm.spritePaths != nil {
		return
	}
	sm.spritePaths, sm.spriteDirType = spritecatalog.BuildIndex()
	sm.indexHUDIcons()
}

var (
	sharedIndexOnce   sync.Once
	sharedSpritePaths map[string]string
)

// ResolveSpritePath returns the on-disk PNG path for a sprite basename, found
// anywhere under the sprite roots (recursive; archive/_-prefixed dirs excluded).
// The index is built once and shared. ok=false if no such sprite exists. Lets
// external tools (e.g. the map editor) resolve sprites by name, layout-agnostic.
func ResolveSpritePath(name string) (string, bool) {
	sharedIndexOnce.Do(func() {
		sharedSpritePaths, _ = spritecatalog.BuildIndex()
	})
	p, ok := sharedSpritePaths[name]
	return p, ok
}

type SpriteAnimation struct {
	Frames      []*ebiten.Image
	FrameWidth  int
	FrameHeight int
}

func animationKey(name, animType string) animationCacheKey {
	return animationCacheKey{name: name, animType: animType}
}

// PrepareResources decodes sources on background workers when a shared byte
// budget is supplied: as many at once as the manager's decode gate admits, two
// without a gate. Results retain request order; see PrepareOrdered.
func (sm *SpriteManager) PrepareResources(ctx context.Context, requests []SpriteResourceRequest, budgets ...*PreparationBudget) <-chan PreparedSpriteResource {
	return sm.prepareResources(ctx, requests, false, budgets...)
}

// SetDecodeGate bounds the decoders of every budgeted PrepareResources call.
// Demand loads keep their own pair of decoders.
func (sm *SpriteManager) SetDecodeGate(gate *WorkerGate) { sm.decodeGate = gate }

func (sm *SpriteManager) prepareResources(ctx context.Context, requests []SpriteResourceRequest, interactive bool, budgets ...*PreparationBudget) <-chan PreparedSpriteResource {
	if sm == nil {
		results := make(chan PreparedSpriteResource)
		close(results)
		return results
	}
	sm.ensureIndex()
	type decodeJob struct {
		request SpriteResourceRequest
		path    string
	}
	jobs := make([]decodeJob, 0, len(requests))
	for _, request := range requests {
		indexedName := request.Name
		if request.AnimationType != "" {
			indexedName += "_" + request.AnimationType
		}
		path := sm.spritePaths[indexedName]
		if path != "" {
			if absolute, err := filepath.Abs(path); err == nil {
				path = absolute
			}
		}
		jobs = append(jobs, decodeJob{request: request, path: path})
	}
	budget := preparationBudget(budgets)
	gate := sm.decodeGate
	switch {
	case budget == nil:
		gate = nil
	case gate == nil || interactive:
		workers := min(2, max(1, runtime.GOMAXPROCS(0)-1))
		gate = NewWorkerGate(workers, workers)
	}
	return PrepareOrdered(ctx, jobs, gate,
		func(ctx context.Context, job decodeJob) (*PreparationLease, bool) {
			return ReservePNGPreparation(ctx, job.path, budget)
		},
		func(job decodeJob, lease *PreparationLease) PreparedSpriteResource {
			prepared := sm.decodePreparedWithMask(job.request, job.path, interactive)
			prepared.QueueLease = lease
			return prepared
		})
}

func (sm *SpriteManager) decodePreparedResource(request SpriteResourceRequest) PreparedSpriteResource {
	if request.Name == "" {
		return PreparedSpriteResource{Request: request}
	}
	indexedName := request.Name
	if request.AnimationType != "" {
		indexedName += "_" + request.AnimationType
	}
	spritePath, ok := sm.spritePaths[indexedName]
	if !ok {
		return PreparedSpriteResource{Request: request}
	}
	return sm.decodePreparedResourceAtPath(request, spritePath)
}

func (sm *SpriteManager) decodePreparedResourceAtPath(request SpriteResourceRequest, spritePath string) PreparedSpriteResource {
	return sm.decodePreparedWithMask(request, spritePath, false)
}

// decodePreparedWithMask also prepares the hit mask when mask is set; world
// actors (npc_mob) always get one. Bounds and mask share one alpha plane.
func (sm *SpriteManager) decodePreparedWithMask(request SpriteResourceRequest, spritePath string, mask bool) PreparedSpriteResource {
	prepared := PreparedSpriteResource{Request: request}
	if spritePath == "" {
		return prepared
	}
	img, err := DecodeImageFile(spritePath)
	if err != nil {
		return prepared
	}
	indexedName := request.Name
	if request.AnimationType != "" {
		indexedName += "_" + request.AnimationType
	}
	prepared.Image = sm.prepareSpritePixels(indexedName, img)
	prepared.Found = prepared.Image != nil
	if prepared.Found {
		prepared.CPU = rgbaFromImage(prepared.Image)
		prepared.metadataReady = true
		w, h := prepared.Image.Bounds().Dx(), prepared.Image.Bounds().Dy()
		alpha := spriteAlphaPlane(prepared.Image)
		prepared.visible = spriteVisibleFrameBoundsFromAlpha(alpha, w, h)
		// World actors are pointer targets too. Prepare their mask with the
		// source pixels so clicking a published frame never decodes on input.
		if (mask || sm.spriteDirType[indexedName] == "npc_mob") && w > 0 && h > 0 {
			prepared.alpha = &spriteAlphaMask{width: w, height: h, alpha: alpha}
		}
		if request.AnimationType != "" {
			prepared.Frames = animationCPUFrames(prepared.CPU)
		}
	}
	return prepared
}

// BeginPreparedResourceCommit creates lightweight image handles and returns an
// incremental pixel writer. Advance must run from the Ebitengine game loop.
func (sm *SpriteManager) BeginPreparedResourceCommit(prepared PreparedSpriteResource) *PreparedSpriteCommit {
	defer prepared.QueueLease.Release()
	commit := &PreparedSpriteCommit{manager: sm, request: prepared.Request}
	if sm == nil || prepared.Request.Name == "" {
		commit.done = true
		return commit
	}
	request := prepared.Request
	// A CPU-only re-decode can fail while the published GPU resource is still
	// valid. Keep its bounds, hit mask and classification until explicit eviction.
	if !prepared.Found && len(sm.ResourceImages(request)) > 0 {
		commit.done = true
		return commit
	}
	indexedName := request.Name
	if request.AnimationType != "" {
		indexedName += "_" + request.AnimationType
	}
	if prepared.Found {
		delete(sm.failedResources, request)
		if sm.visibleFrameBounds == nil {
			sm.visibleFrameBounds = make(map[string]spriteVisibleFrameBounds)
		}
		bounds := prepared.visible
		if !prepared.metadataReady {
			bounds = spriteVisibleFrameBoundsFromImage(prepared.Image)
		}
		sm.visibleFrameBounds[indexedName] = bounds
		if prepared.alpha != nil {
			if sm.alphaMasks == nil {
				sm.alphaMasks = make(map[string]*spriteAlphaMask)
			}
			sm.alphaMasks[indexedName] = prepared.alpha
		}
	}
	if !prepared.Found {
		if sm.failedResources == nil {
			sm.failedResources = make(map[SpriteResourceRequest]bool)
		}
		sm.failedResources[request] = true
		if sm.visibleFrameBounds == nil {
			sm.visibleFrameBounds = make(map[string]spriteVisibleFrameBounds)
		}
		sm.visibleFrameBounds[indexedName] = spriteVisibleFrameBounds{}
		if sm.alphaMasks == nil {
			sm.alphaMasks = make(map[string]*spriteAlphaMask)
		}
		sm.alphaMasks[indexedName] = nil
	}
	if request.AnimationType == "" {
		if !prepared.Found {
			sm.spriteTypeCache[request.Name] = "unknown"
			commit.done = true
			return commit
		}
		cpu := prepared.CPU
		if cpu == nil {
			cpu = rgbaFromImage(prepared.Image)
		}
		if loaded := sm.sprites[request.Name]; loaded != nil {
			commit.result = map[*ebiten.Image]*image.RGBA{loaded: cpu}
			commit.done = true
			return commit
		}
		if cpu == nil {
			commit.done = true
			return commit
		}
		commit.targets = []preparedSpriteTarget{{
			image: ebiten.NewImage(cpu.Bounds().Dx(), cpu.Bounds().Dy()), cpu: cpu,
		}}
		return commit
	}

	key := animationKey(request.Name, request.AnimationType)
	if !prepared.Found {
		sm.animationMissing[key] = true
		commit.done = true
		return commit
	}
	animation := sm.animations[key]
	if animation != nil {
		cpuFrames := prepared.Frames
		if len(cpuFrames) == 0 {
			cpuFrames = animationCPUFrames(prepared.Image)
		}
		out := make(map[*ebiten.Image]*image.RGBA, min(len(animation.Frames), len(cpuFrames)))
		for i := 0; i < len(animation.Frames) && i < len(cpuFrames); i++ {
			out[animation.Frames[i]] = cpuFrames[i]
		}
		commit.result = out
		commit.done = true
		return commit
	}
	cpuFrames := prepared.Frames
	if len(cpuFrames) == 0 {
		cpuFrames = animationCPUFrames(prepared.Image)
	}
	if len(cpuFrames) == 0 {
		sm.animationMissing[key] = true
		commit.done = true
		return commit
	}
	commit.targets = make([]preparedSpriteTarget, 0, len(cpuFrames))
	for _, frame := range cpuFrames {
		if frame == nil || frame.Bounds().Dx() <= 0 || frame.Bounds().Dy() <= 0 {
			sm.animationMissing[key] = true
			commit.targets = nil
			commit.done = true
			return commit
		}
		commit.targets = append(commit.targets, preparedSpriteTarget{
			image: ebiten.NewImage(frame.Bounds().Dx(), frame.Bounds().Dy()), cpu: frame,
		})
	}
	return commit
}

// Advance writes at most maxBytes of source pixels. A non-positive limit drains
// the commit completely for legacy synchronous loading paths.
func (c *PreparedSpriteCommit) Advance(maxBytes int) (map[*ebiten.Image]*image.RGBA, bool) {
	if c == nil || c.done {
		if c == nil {
			return nil, true
		}
		return c.result, true
	}
	budget := maxBytes
	for len(c.targets) > 0 && (maxBytes <= 0 || budget > 0) {
		target := &c.targets[0]
		bounds := target.cpu.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		rowBytes := 4 * width
		rows := height - target.row
		if maxBytes > 0 {
			rows = min(rows, max(1, budget/rowBytes))
		}
		start := target.cpu.PixOffset(bounds.Min.X, bounds.Min.Y+target.row)
		end := start + rows*target.cpu.Stride
		region := image.Rect(0, target.row, width, target.row+rows)
		WritePixelsRegion(target.image, region, target.cpu.Pix[start:end])
		target.row += rows
		if maxBytes > 0 {
			budget -= rows * rowBytes
		}
		if target.row < height {
			break
		}
		c.completed = append(c.completed, *target)
		c.targets = c.targets[1:]
	}
	if len(c.targets) > 0 {
		return nil, false
	}
	c.result = make(map[*ebiten.Image]*image.RGBA)
	request := c.request
	if request.AnimationType == "" {
		if len(c.completed) != 1 {
			c.done = true
			return nil, true
		}
		target := c.completed[0]
		if existing := c.manager.sprites[request.Name]; existing != nil {
			target.image.Deallocate()
			target.image = existing
		} else {
			c.manager.sprites[request.Name] = target.image
		}
		c.manager.spriteTypeCache[request.Name] = c.manager.spriteDirType[request.Name]
		c.result[target.image] = target.cpu
	} else {
		if len(c.completed) == 0 {
			c.done = true
			return nil, true
		}
		key := animationKey(request.Name, request.AnimationType)
		if existing := c.manager.animations[key]; existing != nil {
			for i, target := range c.completed {
				target.image.Deallocate()
				if i < len(existing.Frames) {
					c.result[existing.Frames[i]] = target.cpu
				}
			}
		} else {
			frames := make([]*ebiten.Image, 0, len(c.completed))
			for _, target := range c.completed {
				frames = append(frames, target.image)
				c.result[target.image] = target.cpu
			}
			frameBounds := c.completed[0].cpu.Bounds()
			c.manager.animations[key] = &SpriteAnimation{
				Frames: frames, FrameWidth: frameBounds.Dx(), FrameHeight: frameBounds.Dy(),
			}
		}
		delete(c.manager.animationMissing, key)
	}
	c.manager.indexResource(request)
	c.done = true
	return c.result, true
}

// CommitPreparedResource creates or reuses the GPU-facing cache entry for a
// CPU-prepared source. Streaming callers use BeginPreparedResourceCommit and a
// bounded Advance instead.
func (sm *SpriteManager) CommitPreparedResource(prepared PreparedSpriteResource) map[*ebiten.Image]*image.RGBA {
	commit := sm.BeginPreparedResourceCommit(prepared)
	for {
		images, done := commit.Advance(0)
		if done {
			return images
		}
	}
}

func rgbaFromImage(src image.Image) *image.RGBA {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
}

func animationFrameRects(bounds image.Rectangle) []image.Rectangle {
	frameWidth, frameHeight := bounds.Dx(), bounds.Dy()
	if frameHeight <= 0 || frameWidth <= 0 {
		return nil
	}
	if frameWidth%frameHeight == 0 {
		if frameCount := frameWidth / frameHeight; frameCount > 1 {
			frames := make([]image.Rectangle, 0, frameCount)
			for i := 0; i < frameCount; i++ {
				frames = append(frames, image.Rect(
					bounds.Min.X+i*frameHeight, bounds.Min.Y,
					bounds.Min.X+(i+1)*frameHeight, bounds.Min.Y+frameHeight,
				))
			}
			return frames
		}
	}
	if frameWidth == frameHeight && frameWidth%2 == 0 {
		frameSize := frameWidth / 2
		frames := make([]image.Rectangle, 0, 4)
		for row := 0; row < 2; row++ {
			for col := 0; col < 2; col++ {
				frames = append(frames, image.Rect(
					bounds.Min.X+col*frameSize, bounds.Min.Y+row*frameSize,
					bounds.Min.X+(col+1)*frameSize, bounds.Min.Y+(row+1)*frameSize,
				))
			}
		}
		return frames
	}
	return nil
}

func animationCPUFrames(img image.Image) []*image.RGBA {
	if img == nil {
		return nil
	}
	frames := make([]*image.RGBA, 0, 4)
	for _, rect := range animationFrameRects(img.Bounds()) {
		frame := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(frame, frame.Bounds(), img, rect.Min, draw.Src)
		frames = append(frames, frame)
	}
	return frames
}

func (sm *SpriteManager) createPlaceholder(name string) *ebiten.Image {
	// Create larger sprites for trees and biome obstacles to prevent transparency issues
	var img *ebiten.Image
	if name == "tree" || name == "ancient_tree" || name == "sand_dune" || name == "large_dune" || name == "coral_reef" || name == "large_coral" {
		img = ebiten.NewImage(32, 32) // Larger obstacle sprites
	} else {
		img = ebiten.NewImage(16, 16)
	}

	// Determine sprite type based on search paths (cached)
	spriteType := sm.getCachedSpriteType(name)

	switch spriteType {
	case "environment":
		img.Fill(color.RGBA{128, 0, 128, 255}) // Purple for environment
	case "npc_mob":
		img.Fill(color.RGBA{0, 128, 0, 255}) // Green for NPCs/mobs
	default:
		img.Fill(color.RGBA{128, 128, 128, 255}) // Gray for unknown
	}

	return img
}

// getCachedSpriteType determines sprite type with caching to avoid repeated file checks
func (sm *SpriteManager) getCachedSpriteType(name string) string {
	if spriteType, exists := sm.spriteTypeCache[name]; exists {
		return spriteType
	}

	spriteType := sm.determineSpritePaths(name)
	sm.spriteTypeCache[name] = spriteType
	return spriteType
}

// determineSpritePaths returns the placeholder type for a name from the index.
func (sm *SpriteManager) determineSpritePaths(name string) string {
	sm.ensureIndex()
	if t, ok := sm.spriteDirType[name]; ok {
		return t
	}
	return "unknown"
}

func (sm *SpriteManager) GetSprite(name string) *ebiten.Image {
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// Runtime callers can defer a cold source without publishing a placeholder.
	if sm.deferMissingResource(SpriteResourceRequest{Name: name}) {
		return nil
	}
	if !sm.failedResources[SpriteResourceRequest{Name: name}] {
		sm.loadSpriteIfExists(name)
	}

	// Check again after attempting to load
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// If still not found, create placeholder
	sprite := sm.createPlaceholder(name)
	sm.sprites[name] = sprite
	sm.indexResource(SpriteResourceRequest{Name: name})
	return sprite
}

// HasSprite reports whether name resolves to art; a nil manager has none.
func (sm *SpriteManager) HasSprite(name string) bool {
	if sm == nil || name == "" {
		return false
	}
	if _, exists := sm.sprites[name]; exists {
		return true
	}
	return sm.spriteExists(name)
}

// SpriteOpaqueAt reports whether a source-local pixel is visible. The mask is
// decoded from the authored PNG and color-keyed exactly like GetSprite, keeping
// interaction pixel-perfect without ever reading the GPU render source back.
// known is false only when the sprite source cannot be decoded.
func (sm *SpriteManager) SpriteOpaqueAt(name string, x, y int) (opaque, known bool) {
	if sm == nil || name == "" {
		return false, false
	}
	if sm.alphaMasks == nil {
		sm.alphaMasks = make(map[string]*spriteAlphaMask)
	}
	mask, cached := sm.alphaMasks[name]
	if !cached {
		if sm.deferMissingResource(SpriteResourceRequest{Name: name}) {
			return false, false
		}
		mask = sm.loadSpriteAlphaMask(name)
		sm.alphaMasks[name] = mask // nil is a cached decode failure
	}
	if mask == nil {
		return false, false
	}
	if x < 0 || y < 0 || x >= mask.width || y >= mask.height {
		return false, true
	}
	return mask.alpha[y*mask.width+x] != 0, true
}

// ImageOpaqueAt resolves a published sprite or animation frame against its
// existing CPU alpha mask, without a GPU readback or a second frame registry.
func (sm *SpriteManager) ImageOpaqueAt(img *ebiten.Image, x, y int) (opaque, known bool) {
	request, ok := sm.ResourceForImage(img)
	if !ok {
		return false, false
	}
	name := request.Name
	if request.AnimationType == "" {
		return sm.SpriteOpaqueAt(name, x, y)
	}
	name += "_" + request.AnimationType
	mask := sm.alphaMasks[name]
	if mask == nil {
		return false, false
	}
	anim := sm.GetAnimation(request.Name, request.AnimationType)
	if anim == nil {
		return false, false
	}
	rects := animationFrameRects(image.Rect(0, 0, mask.width, mask.height))
	for i, frame := range anim.Frames {
		if frame == img && i < len(rects) {
			r := rects[i]
			if x < 0 || y < 0 || x >= r.Dx() || y >= r.Dy() {
				return false, true
			}
			return mask.alpha[(r.Min.Y+y)*mask.width+r.Min.X+x] != 0, true
		}
	}
	return false, false
}

// SpriteVisibleFrameBounds returns the visible alpha bounds inside one logical
// animation frame. A horizontal 4-frame sheet is folded into one union bound,
// keeping its scale stable while frames animate. The data comes from the same
// CPU-side, color-keyed source. The compact cache deliberately does not retain
// the per-pixel alpha bytes needed only by SpriteOpaqueAt.
func (sm *SpriteManager) SpriteVisibleFrameBounds(name string) (bounds image.Rectangle, frameWidth, frameHeight int, known bool) {
	if sm == nil || name == "" {
		return image.Rectangle{}, 0, 0, false
	}
	if sm.visibleFrameBounds == nil {
		sm.visibleFrameBounds = make(map[string]spriteVisibleFrameBounds)
	}
	entry, cached := sm.visibleFrameBounds[name]
	if !cached {
		if sm.deferMissingResource(SpriteResourceRequest{Name: name}) {
			return image.Rectangle{}, 0, 0, false
		}
		entry = sm.loadSpriteVisibleFrameBounds(name)
		sm.visibleFrameBounds[name] = entry
	}
	if !entry.known {
		return image.Rectangle{}, 0, 0, false
	}
	return entry.bounds, entry.frameWidth, entry.frameHeight, true
}

func (sm *SpriteManager) loadSpriteVisibleFrameBounds(name string) spriteVisibleFrameBounds {
	sm.ensureIndex()
	spritePath, ok := sm.spritePaths[name]
	if !ok {
		return spriteVisibleFrameBounds{}
	}
	img, err := DecodeImageFile(spritePath)
	if err != nil {
		return spriteVisibleFrameBounds{}
	}
	return spriteVisibleFrameBoundsFromImage(sm.prepareSpritePixels(name, img))
}

func spriteVisibleFrameBoundsFromImage(img image.Image) spriteVisibleFrameBounds {
	if img == nil {
		return spriteVisibleFrameBounds{}
	}
	return spriteVisibleFrameBoundsFromAlpha(spriteAlphaPlane(img), img.Bounds().Dx(), img.Bounds().Dy())
}

// spriteVisibleFrameBoundsFromAlpha folds a width*height alpha plane into the
// union of its frames' visible bounds (a width == 4*height sheet has 4 frames).
func spriteVisibleFrameBoundsFromAlpha(alpha []uint8, width, height int) spriteVisibleFrameBounds {
	if width <= 0 || height <= 0 {
		return spriteVisibleFrameBounds{}
	}
	frameWidth := width
	if width == height*4 {
		frameWidth = height
	}
	const visibleAlphaThreshold = uint8(24)
	minX, minY := frameWidth, height
	maxX, maxY := -1, -1
	for y := 0; y < height; y++ {
		row := alpha[y*width : (y+1)*width]
		for f := 0; f < width; f += frameWidth {
			frame := row[f:min(f+frameWidth, width)]
			for x, a := range frame {
				if a >= visibleAlphaThreshold {
					minX, minY, maxY = min(minX, x), min(minY, y), y
					break
				}
			}
			for x := len(frame) - 1; x > maxX; x-- {
				if frame[x] >= visibleAlphaThreshold {
					maxX = x
					break
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return spriteVisibleFrameBounds{}
	}
	return spriteVisibleFrameBounds{
		bounds:     image.Rect(minX, minY, maxX+1, maxY+1),
		frameWidth: frameWidth, frameHeight: height, known: true,
	}
}

func (sm *SpriteManager) loadSpriteAlphaMask(name string) *spriteAlphaMask {
	sm.ensureIndex()
	spritePath, ok := sm.spritePaths[name]
	if !ok {
		return nil
	}
	img, err := DecodeImageFile(spritePath)
	if err != nil {
		return nil
	}
	img = sm.prepareSpritePixels(name, img)
	return spriteAlphaMaskFromImage(img)
}

func spriteAlphaMaskFromImage(img image.Image) *spriteAlphaMask {
	if img == nil {
		return nil
	}
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	return &spriteAlphaMask{width: width, height: height, alpha: spriteAlphaPlane(img)}
}

// spriteAlphaPlane copies img's alpha channel row by row (width*height bytes,
// subimage origins and row strides honoured; nil for an empty image). Both
// metadata consumers share this one alpha rule; uncommon image types go
// through image.Image.At.
func spriteAlphaPlane(img image.Image) []uint8 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	alpha := make([]uint8, w*h)
	switch src := img.(type) {
	case *image.RGBA:
		copyAlphaChannel(alpha, w, h, src.Pix[src.PixOffset(b.Min.X, b.Min.Y)+3:], src.Stride, 4)
	case *image.NRGBA:
		copyAlphaChannel(alpha, w, h, src.Pix[src.PixOffset(b.Min.X, b.Min.Y)+3:], src.Stride, 4)
	case *image.Alpha:
		copyAlphaChannel(alpha, w, h, src.Pix[src.PixOffset(b.Min.X, b.Min.Y):], src.Stride, 1)
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				alpha[y*w+x] = uint8(a >> 8)
			}
		}
	}
	return alpha
}

// copyAlphaChannel copies w*h alpha bytes into dst: src starts at the first
// pixel's alpha byte, pixels are step bytes apart and rows stride apart.
func copyAlphaChannel(dst []uint8, w, h int, src []byte, stride, step int) {
	for y := 0; y < h; y++ {
		row, out := src[y*stride:], dst[y*w:(y+1)*w]
		for x := range out {
			out[x] = row[x*step]
		}
	}
}

func (sm *SpriteManager) GetSpriteVariants(baseName string) []string {
	variants := []string{}
	if sm.spriteExists(baseName) {
		variants = append(variants, baseName)
	}
	for i := 0; ; i++ {
		name := baseName + strconv.Itoa(i)
		if !sm.spriteExists(name) {
			break
		}
		if name != baseName {
			variants = append(variants, name)
		}
	}
	return variants
}

// SpriteNamesWithPrefix returns every indexed sprite basename beginning with
// prefix in stable order. Resource prewarmers use the index itself as the
// source of truth for families such as bag_* and chest_*; adding an asset does
// not require a parallel hardcoded list.
func (sm *SpriteManager) SpriteNamesWithPrefix(prefix string) []string {
	if sm == nil || prefix == "" {
		return nil
	}
	sm.ensureIndex()
	names := make([]string, 0)
	for name := range sm.spritePaths {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (sm *SpriteManager) spriteExists(name string) bool {
	if sm == nil {
		return false
	}
	sm.ensureIndex()
	_, ok := sm.spritePaths[name]
	return ok
}

// AnimationFrameCount reads only the PNG header for a cold sheet. Death
// presentation can select a sheet without uploading an off-screen GPU texture.
// Counts share the loader's frame-layout rule and survive resource eviction.
func (sm *SpriteManager) AnimationFrameCount(name, animType string) int {
	if sm == nil {
		return 0
	}
	key := animationKey(name, animType)
	if animation := sm.animations[key]; animation != nil {
		return len(animation.Frames)
	}
	if count, ok := sm.animationFrameCounts[key]; ok {
		return count
	}
	sm.ensureIndex()
	count := 0
	if path := sm.spritePaths[name+"_"+animType]; path != "" {
		if f, err := os.Open(path); err == nil {
			metadata, _, err := image.DecodeConfig(f)
			f.Close()
			if err == nil {
				count = len(animationFrameRects(image.Rect(0, 0, metadata.Width, metadata.Height)))
			}
		}
	}
	if sm.animationFrameCounts == nil {
		sm.animationFrameCounts = make(map[animationCacheKey]int)
	}
	sm.animationFrameCounts[key] = count
	return count
}

func (sm *SpriteManager) GetAnimation(name, animType string) *SpriteAnimation {
	key := animationKey(name, animType)
	if anim, exists := sm.animations[key]; exists {
		return anim
	}
	if sm.animationMissing[key] {
		return nil
	}
	if sm.deferMissingResource(SpriteResourceRequest{Name: name, AnimationType: animType}) {
		return nil
	}
	sm.loadAnimationIfExists(name, animType)
	if anim, exists := sm.animations[key]; exists {
		return anim
	}
	sm.animationMissing[key] = true
	return nil
}

// EvictResource releases one cached render source so a later lookup decodes it
// again. An empty animationType addresses a static sprite; otherwise it
// addresses one animation strip. Region residency uses this single eviction
// path for every SpriteManager-owned GPU image.
func (sm *SpriteManager) EvictResource(name, animationType string) []*ebiten.Image {
	images := sm.DetachResource(name, animationType)
	for _, img := range images {
		if img != nil {
			img.Deallocate()
		}
	}
	return images
}

// loadSpriteIfExists attempts to load a sprite by basename from the index.
func (sm *SpriteManager) loadSpriteIfExists(name string) {
	sm.ensureIndex()
	prepared := sm.decodePreparedResource(SpriteResourceRequest{Name: name})
	images := sm.CommitPreparedResource(prepared)
	if prepared.Found && sm.lazyResourceObserver != nil {
		sm.lazyResourceObserver(prepared.Request, images)
	}
}

func (sm *SpriteManager) loadAnimationIfExists(name, animType string) {
	sm.ensureIndex()
	prepared := sm.decodePreparedResource(SpriteResourceRequest{Name: name, AnimationType: animType})
	images := sm.CommitPreparedResource(prepared)
	if prepared.Found && sm.lazyResourceObserver != nil {
		sm.lazyResourceObserver(prepared.Request, images)
	}
}

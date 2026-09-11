package graphics

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/assetmanifest"
)

type SpriteManager struct {
	imageResources   map[*ebiten.Image]SpriteResourceRequest
	sprites          map[string]*ebiten.Image
	spriteTypeCache  map[string]string // Cache sprite types to avoid repeated file checks
	animations       map[animationCacheKey]*SpriteAnimation
	animationMissing map[animationCacheKey]bool
	// CPU alpha masks for the rare pixel-perfect hit tests. Reading an
	// *ebiten.Image with At/ReadPixels flushes the GPU command queue, and At
	// reads the WHOLE image back - a stall per interaction probe.
	alphaMasks map[string]*spriteAlphaMask
	// Compact visible-frame geometry used by visual-size normalization. Unlike
	// alphaMasks, each entry retains only a rectangle and frame dimensions.
	visibleFrameBounds map[string]spriteVisibleFrameBounds

	// spritePaths maps a sprite basename (no extension) to its PNG path, built
	// once by walking the sprite roots recursively (see ensureIndex). Lets
	// sprites live in any subfolder layout - lookup is by name, not location.
	// spriteDirType records the originating root's placeholder type.
	spritePaths   map[string]string
	spriteDirType map[string]string

	// Load-time color key (configured via SetColorKey): pixels within keyTol of the
	// key color become transparent; with keyDespill, tinted fringe pixels have the
	// cast subtracted instead (kept opaque).
	keyEnabled bool
	keyR       uint8
	keyG       uint8
	keyB       uint8
	keyTol     int
	keyDespill bool
	// Sprites whose interior magenta is real art: despill only their edge fringe
	// (within keyEdgeRadius px of a transparent pixel), not the whole body.
	keyEdgeOnly   map[string]bool
	keyEdgeRadius int
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

// despillHueFloor is the magenta-excess (min(R,B)-G) below which a kept pixel is
// left alone. Deliberately low (aggressive): project art reserves any magenta
// hue for removable background/fringe, so even faint casts are subtracted.
const despillHueFloor = 8

// despillEdgeRadiusDefault is the fringe band (px from a transparent edge) used
// for edge-only despill sprites when the config leaves the radius unset.
const despillEdgeRadiusDefault = 3

// SetDespillEdgeOnly marks sprites (by name; animation sheets as
// "<name>_<animType>") whose interior magenta must be preserved - despill on
// them is restricted to within `radius` px of a transparent edge.
func (sm *SpriteManager) SetDespillEdgeOnly(names []string, radius int) {
	sm.keyEdgeOnly = make(map[string]bool, len(names))
	for _, n := range names {
		sm.keyEdgeOnly[n] = true
	}
	sm.keyEdgeRadius = radius
}

// SetColorKey enables/configures the load-time color key (see SpriteManager).
// Must be called before sprites are first loaded to take effect on them.
func (sm *SpriteManager) SetColorKey(enabled bool, r, g, b, tolerance int, despill bool) {
	sm.keyEnabled = enabled
	sm.keyR, sm.keyG, sm.keyB = uint8(r), uint8(g), uint8(b)
	sm.keyTol = tolerance
	sm.keyDespill = despill
}

// applyColorKey returns a copy of src with the key color removed: pixels within
// keyTol of the key go transparent; with keyDespill, every remaining magenta-hue
// pixel (R and B above G) has that excess subtracted and stays opaque. Despill
// assumes a magenta-style key (high R,B / low G). No-op when the key is off.
//
// For names in keyEdgeOnly (intentional magenta art), despill is restricted to
// the fringe band within keyEdgeRadius px of a transparent edge, leaving the
// interior purple/magenta untouched.
func (sm *SpriteManager) applyColorKey(name string, src image.Image) image.Image {
	if !sm.keyEnabled || src == nil {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	near := func(v, target uint8) bool {
		d := int(v) - int(target)
		if d < 0 {
			d = -d
		}
		return d <= sm.keyTol
	}
	min2 := func(a, b uint8) uint8 {
		if a < b {
			return a
		}
		return b
	}
	isKey := func(p color.NRGBA) bool {
		return near(p.R, sm.keyR) && near(p.G, sm.keyG) && near(p.B, sm.keyB)
	}

	edgeOnly := sm.keyEdgeOnly[name]
	// Transparency mask, needed only when despill is limited to the fringe band.
	var trans []bool
	if edgeOnly {
		trans = make([]bool, w*h)
	}

	// Pass 1: erase the key core (and record transparency for edge-only despill).
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := dst.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			keyed := p.A != 0 && isKey(p)
			if edgeOnly {
				trans[y*w+x] = p.A == 0 || keyed
			}
			if keyed {
				dst.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{})
			}
		}
	}
	if !sm.keyDespill {
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

	// Pass 2: despill kept pixels. Edge-only sprites despill the fringe band only,
	// so their intentional interior magenta survives.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := b.Min.X+x, b.Min.Y+y
			p := dst.NRGBAAt(px, py)
			if p.A == 0 {
				continue
			}
			excess := int(min2(p.R, p.B)) - int(p.G)
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
		sprites:            make(map[string]*ebiten.Image),
		spriteTypeCache:    make(map[string]string),
		animations:         make(map[animationCacheKey]*SpriteAnimation),
		animationMissing:   make(map[animationCacheKey]bool),
		alphaMasks:         make(map[string]*spriteAlphaMask),
		visibleFrameBounds: make(map[string]spriteVisibleFrameBounds),
	}
}

// spriteBaseDirs are the roots indexed by basename (recursively). Each maps to
// the placeholder type used when a named sprite is missing. floor/ and sky/ are
// loaded separately via resolveNamedPNG and intentionally omitted here.
var spriteBaseDirs = []struct{ dir, typ string }{
	{"assets/sprites/mobs", "npc_mob"},
	{"assets/sprites/characters", "npc_mob"},
	{"assets/sprites/environment", "environment"},
	{"assets/sprites/interface", "interface"},
}

// isIgnoredSpriteDir reports folders excluded from the sprite index: archives
// (any case) and any name starting with "_" or "." - a convention to park
// unused/duplicate art in the tree without it shadowing live sprites.
func isIgnoredSpriteDir(name string) bool {
	return strings.EqualFold(name, "archive") ||
		strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// buildSpriteIndex walks the sprite roots recursively (skipping ignored dirs)
// and returns basename->path and basename->placeholder-type maps. Sprites may
// therefore be grouped into arbitrary subfolders; basenames must be unique
// across the whole tree. On a seeded install a current shipped path wins over
// untracked legacy/custom duplicates; equal ownership keeps root/lexical order.
// Shared by SpriteManager.ensureIndex and the package-level resolver.
func buildSpriteIndex() (paths, dirType map[string]string) {
	shipped := assetmanifest.Load(".")
	paths = make(map[string]string)
	dirType = make(map[string]string)
	for _, root := range spriteBaseDirs {
		_ = filepath.WalkDir(root.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // missing root (e.g. tests run outside the repo) - skip
			}
			if d.IsDir() {
				if isIgnoredSpriteDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".png" {
				return nil
			}
			base := strings.TrimSuffix(d.Name(), ".png")
			if existing, dup := paths[base]; dup {
				keep := existing
				if shipped.ContainsRuntimePath(path) && !shipped.ContainsRuntimePath(existing) {
					keep = path
					paths[base], dirType[base] = path, root.typ
				}
				log.Printf("sprite index: duplicate basename %q (%q vs %q); keeping %q", base, existing, path, keep)
				return nil
			}
			paths[base] = path
			dirType[base] = root.typ
			return nil
		})
	}
	return paths, dirType
}

// ensureIndex lazily builds this manager's basename->path index.
func (sm *SpriteManager) ensureIndex() {
	if sm == nil || sm.spritePaths != nil {
		return
	}
	sm.spritePaths, sm.spriteDirType = buildSpriteIndex()
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
		sharedSpritePaths, _ = buildSpriteIndex()
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

// PrepareResources decodes sources serially on one bounded worker. The
// one-result buffer prevents a fast disk from retaining a whole region of
// decoded RGBA images while the game loop is still committing earlier work.
func (sm *SpriteManager) PrepareResources(ctx context.Context, requests []SpriteResourceRequest, budgets ...*PreparationBudget) <-chan PreparedSpriteResource {
	results := make(chan PreparedSpriteResource, 1)
	if sm == nil {
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
	go func() {
		defer close(results)
		for _, job := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			lease, ok := ReservePNGPreparation(ctx, job.path, preparationBudget(budgets))
			if !ok {
				return
			}
			prepared := sm.decodePreparedResourceAtPath(job.request, job.path)
			prepared.QueueLease = lease
			lease.ReleaseOnCancel(ctx)
			select {
			case results <- prepared:
			case <-ctx.Done():
				return
			}
		}
	}()
	return results
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
	prepared := PreparedSpriteResource{Request: request}
	if spritePath == "" {
		return prepared
	}
	file, err := os.Open(spritePath)
	if err != nil {
		return prepared
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return prepared
	}
	indexedName := request.Name
	if request.AnimationType != "" {
		indexedName += "_" + request.AnimationType
	}
	prepared.Image = sm.applyColorKey(indexedName, img)
	prepared.Found = prepared.Image != nil
	if prepared.Found {
		prepared.CPU = rgbaFromImage(prepared.Image)
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
	indexedName := request.Name
	if request.AnimationType != "" {
		indexedName += "_" + request.AnimationType
	}
	if prepared.Found {
		if sm.visibleFrameBounds == nil {
			sm.visibleFrameBounds = make(map[string]spriteVisibleFrameBounds)
		}
		sm.visibleFrameBounds[indexedName] = spriteVisibleFrameBoundsFromImage(prepared.Image)
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
		target.image.SubImage(region).(*ebiten.Image).WritePixels(target.cpu.Pix[start:end])
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

	// Try to dynamically load the sprite if it's not already loaded
	sm.loadSpriteIfExists(name)

	// Check again after attempting to load
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// If still not found, create placeholder
	return sm.createPlaceholder(name)
}

func (sm *SpriteManager) HasSprite(name string) bool {
	if name == "" {
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
	file, err := os.Open(spritePath)
	if err != nil {
		return spriteVisibleFrameBounds{}
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return spriteVisibleFrameBounds{}
	}
	return spriteVisibleFrameBoundsFromImage(sm.applyColorKey(name, img))
}

func spriteVisibleFrameBoundsFromImage(img image.Image) spriteVisibleFrameBounds {
	if img == nil {
		return spriteVisibleFrameBounds{}
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
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
		for x := 0; x < width; x++ {
			_, _, _, alpha := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if uint8(alpha>>8) < visibleAlphaThreshold {
				continue
			}
			frameX := x % frameWidth
			minX = min(minX, frameX)
			minY = min(minY, y)
			maxX = max(maxX, frameX)
			maxY = max(maxY, y)
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
	file, err := os.Open(spritePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil
	}
	img = sm.applyColorKey(name, img)
	return spriteAlphaMaskFromImage(img)
}

func spriteAlphaMaskFromImage(img image.Image) *spriteAlphaMask {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	mask := &spriteAlphaMask{
		width:  width,
		height: height,
		alpha:  make([]uint8, width*height),
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			_, _, _, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			mask.alpha[y*width+x] = uint8(a >> 8)
		}
	}
	return mask
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

func (sm *SpriteManager) GetAnimation(name, animType string) *SpriteAnimation {
	key := animationKey(name, animType)
	if anim, exists := sm.animations[key]; exists {
		return anim
	}
	if sm.animationMissing[key] {
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

// VisitCommitPixels reports retained CPU allocations on the game owner. It
// never reads GPU pixels; callers can deduplicate aliases across caches.
func (c *PreparedSpriteCommit) VisitCommitPixels(visit func(*image.RGBA)) {
	if c == nil || visit == nil {
		return
	}
	for _, target := range c.targets {
		visit(target.cpu)
	}
	for _, target := range c.completed {
		visit(target.cpu)
	}
	for _, cpu := range c.result {
		visit(cpu)
	}
}

package rendering

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Cache size constants - proactive limits prevent GC spikes from bulk eviction
const (
	wallSliceCacheMaxSize    = 512 // Reduced from 1000 to prevent large evictions
	wallSliceCacheTargetSize = 384 // Target after eviction (75% of max)
)

// WallSliceCache provides thread-safe caching of pre-rendered wall slices to improve rendering performance.
// This cache prevents redundant wall slice generation by storing commonly used combinations of wall parameters.
// The cache uses quantized distance and texture coordinates to maximize cache hit rates while maintaining
// visual quality. Memory usage is controlled through automatic eviction when size limits are reached.
//
// Eviction policy is FIFO: cache hits do not promote entries, so the oldest
// inserted entries are evicted first regardless of recent access. This is
// cheaper than tracking access order and works well when texture coordinates
// quantize predictably.
type WallSliceCache struct {
	cache      map[WallSliceKey]*ebiten.Image // Map of wall configurations to pre-rendered images
	mutex      sync.RWMutex                   // Reader-writer mutex for thread-safe access
	cacheOrder []WallSliceKey                 // FIFO insertion order for eviction
}

// WallSliceKey represents a unique wall slice configuration used as a cache key.
// Distance-based shading is applied at draw time so it's not part of the key;
// texture coordinates are quantized during caching to improve hit rates.
//
// TileType is `int` rather than `interface{}` to avoid boxing the caller's
// world.TileType3D value into an interface header on every cache lookup -
// that allocation showed up in the hot raycast path. Callers cast
// `int(tileType)` when building the key.
type WallSliceKey struct {
	Height   int     // Rendered wall height in pixels
	Width    int     // Rendered wall width in pixels
	TileType int     // Tile type identifier (caller casts world.TileType3D)
	Side     int     // Wall orientation: 0 for N-S walls, 1 for E-W walls
	WallX    float64 // Quantized texture x-coordinate (1/16 increments)
}

// NewWallSliceCache creates a new wall slice cache with an empty cache map.
// The cache is thread-safe and ready for concurrent access from multiple rendering goroutines.
func NewWallSliceCache() *WallSliceCache {
	return &WallSliceCache{
		cache:      make(map[WallSliceKey]*ebiten.Image, wallSliceCacheMaxSize),
		cacheOrder: make([]WallSliceKey, 0, wallSliceCacheMaxSize),
	}
}

// GetOrCreate retrieves a cached wall slice or creates a new one if not found.
// This method is thread-safe and handles cache quantization, lookup, creation, and eviction.
// Parameters:
//   - key: Wall slice configuration parameters
//   - createFunc: Function to generate the wall slice, receives quantized height
//
// Returns the cached or newly created wall slice image.
func (wsc *WallSliceCache) GetOrCreate(key WallSliceKey, createFunc func(quantizedHeight int) *ebiten.Image) *ebiten.Image {
	// Quantize texture coordinate for better cache hit rates
	// Use 1/16 increments for smooth texture mapping
	key.WallX = float64(int(key.WallX*16)) / 16

	// Adaptive height quantization: finer steps for distant walls (small heights),
	// coarser steps for close walls (large heights) where precision matters less
	// - Distant walls (<64px): 2px steps - fine detail visible
	// - Medium walls (64-256px): 4px steps - balanced
	// - Close walls (256-512px): 8px steps - less noticeable
	// - Very close walls (>512px): capped and scaled by renderer
	var quantStep int
	switch {
	case key.Height < 64:
		quantStep = 2
	case key.Height < 256:
		quantStep = 4
	default:
		quantStep = 8
	}
	key.Height = ((key.Height + quantStep/2) / quantStep) * quantStep
	if key.Height < 2 {
		key.Height = 2
	}
	// Cap maximum cached height to prevent memory bloat for very close walls
	// Walls taller than this will all use the same cached slice and scale it
	if key.Height > 512 {
		key.Height = 512
	}

	// First attempt: try to get cached image with read lock (allows concurrent reads)
	wsc.mutex.RLock()
	if cachedImage, exists := wsc.cache[key]; exists {
		wsc.mutex.RUnlock()
		return cachedImage
	}
	wsc.mutex.RUnlock()

	// Cache miss: generate new wall slice using provided creation function
	// Pass the quantized height so the created image matches the cache key
	newImage := createFunc(key.Height)

	// Second phase: store the new image with write lock (exclusive access)
	wsc.mutex.Lock()
	defer wsc.mutex.Unlock()

	// Check again in case another goroutine added it while we were creating
	if cachedImage, exists := wsc.cache[key]; exists {
		return cachedImage
	}

	// Proactive eviction: evict before we exceed max size to avoid large batch deletions
	// This prevents GC spikes by doing smaller, more frequent evictions
	if len(wsc.cache) >= wallSliceCacheMaxSize {
		// Evict oldest-inserted entries until we reach target size
		evictCount := len(wsc.cacheOrder) - wallSliceCacheTargetSize
		if evictCount > 0 && evictCount <= len(wsc.cacheOrder) {
			for i := 0; i < evictCount; i++ {
				delete(wsc.cache, wsc.cacheOrder[i])
			}
			wsc.cacheOrder = wsc.cacheOrder[evictCount:]
		}
	}

	// Store the newly created image in cache
	wsc.cache[key] = newImage
	wsc.cacheOrder = append(wsc.cacheOrder, key)
	return newImage
}

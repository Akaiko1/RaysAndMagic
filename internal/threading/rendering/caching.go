package rendering

import (
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// WallSliceCache provides thread-safe caching of pre-rendered wall slices to improve rendering performance.
// This cache prevents redundant wall slice generation by storing commonly used combinations of wall parameters.
// The cache uses quantized distance and texture coordinates to maximize cache hit rates while maintaining
// visual quality. Memory usage is controlled through automatic cache eviction when size limits are reached.
type WallSliceCache struct {
	cache map[WallSliceKey]*ebiten.Image // Map of wall configurations to pre-rendered images
	mutex sync.RWMutex                   // Reader-writer mutex for thread-safe access
}

// WallSliceKey represents a unique wall slice configuration used as a cache key.
// This struct defines all the parameters that affect the base appearance of a rendered wall slice.
// Distance-based shading is now applied at draw time to improve cache hit rates.
// Texture coordinates are quantized during caching to improve hit rates.
type WallSliceKey struct {
	Height   int         // Rendered wall height in pixels
	Width    int         // Rendered wall width in pixels
	TileType interface{} // Type of tile being rendered (affects color and texture)
	Side     int         // Wall orientation: 0 for north-south walls, 1 for east-west walls (affects base shading)
	WallX    float64     // Texture coordinate for horizontal position on wall (quantized to 1/16 increments)
}

// NewWallSliceCache creates a new wall slice cache with an empty cache map.
// The cache is thread-safe and ready for concurrent access from multiple rendering goroutines.
func NewWallSliceCache() *WallSliceCache {
	return &WallSliceCache{
		cache: make(map[WallSliceKey]*ebiten.Image),
	}
}

// GetOrCreate retrieves a cached wall slice or creates a new one if not found.
// This method is thread-safe and handles cache quantization, lookup, creation, and eviction.
// Parameters:
//   - key: Wall slice configuration parameters
//   - createFunc: Function to generate the wall slice if not cached
//
// Returns the cached or newly created wall slice image.
func (wsc *WallSliceCache) GetOrCreate(key WallSliceKey, createFunc func() *ebiten.Image) *ebiten.Image {
	// Quantize texture coordinate for better cache hit rates
	// Texture coordinate is quantized to 1/16 increments for smooth texture mapping
	key.WallX = float64(int(key.WallX*16)) / 16

	// First attempt: try to get cached image with read lock (allows concurrent reads)
	wsc.mutex.RLock()
	if cachedImage, exists := wsc.cache[key]; exists {
		wsc.mutex.RUnlock()
		return cachedImage
	}
	wsc.mutex.RUnlock()

	// Cache miss: generate new wall slice using provided creation function
	newImage := createFunc()

	// Second phase: store the new image with write lock (exclusive access)
	wsc.mutex.Lock()
	defer wsc.mutex.Unlock()

	// Implement simple cache eviction to prevent memory bloat
	if len(wsc.cache) > 1000 { // Maximum cache size threshold
		// Simple LRU approximation: remove half the cache entries
		// This prevents excessive memory usage while maintaining reasonable performance
		for cacheKey := range wsc.cache {
			delete(wsc.cache, cacheKey)
			if len(wsc.cache) <= 500 { // Target cache size after eviction
				break
			}
		}
	}

	// Store the newly created image in cache
	wsc.cache[key] = newImage
	return newImage
}

// ColorCalculator provides thread-safe color calculations with caching for distance-based lighting effects.
// This calculator handles the complex task of adjusting base colors based on distance from the camera,
// applying realistic lighting falloff while maintaining performance through intelligent caching.
// Colors are quantized during caching to balance visual quality with memory efficiency.
type ColorCalculator struct {
	cache map[ColorKey]color.RGBA // Map of color calculation parameters to computed results
	mutex sync.RWMutex            // Reader-writer mutex for thread-safe concurrent access
}

// ColorKey represents a unique color calculation configuration used as a cache key.
// This struct contains all parameters that affect the final computed color, including
// the base color, distance-based lighting parameters, and minimum brightness constraints.
type ColorKey struct {
	BaseColor     color.RGBA // Original color before distance-based lighting modifications
	Distance      float64    // Distance from camera (quantized to groups of 5 for caching efficiency)
	MinBrightness float64    // Minimum brightness threshold to prevent complete darkness
}

// NewColorCalculator creates a new thread-safe color calculator with an empty cache.
// The calculator is ready for concurrent use across multiple rendering goroutines and
// automatically handles cache management to prevent excessive memory usage.
func NewColorCalculator() *ColorCalculator {
	return &ColorCalculator{
		cache: make(map[ColorKey]color.RGBA),
	}
}

// CalculateDistanceColor computes realistic distance-based color shading with caching optimization.
// This method applies atmospheric perspective by darkening colors based on their distance from the camera,
// simulating how objects appear dimmer when farther away. The calculation uses a linear falloff model
// with a configurable minimum brightness to prevent complete darkness.
//
// Parameters:
//   - baseColor: Original color before distance modification
//   - distance: Distance from camera to the object
//   - viewDist: Maximum viewing distance (100% darkness threshold)
//   - minBrightness: Minimum brightness factor (prevents complete darkness)
//
// Returns the color adjusted for distance-based lighting effects.
func (cc *ColorCalculator) CalculateDistanceColor(baseColor color.RGBA, distance, viewDistance, minBrightness float64) color.RGBA {
	// Quantize distance to groups of 5 units for improved cache hit rates
	// This balances visual quality with caching efficiency
	quantizedDistance := float64(int(distance/5)) * 5

	// Create cache key from all parameters that affect the final color
	cacheKey := ColorKey{
		BaseColor:     baseColor,
		Distance:      quantizedDistance,
		MinBrightness: minBrightness,
	}

	// First attempt: try to get cached result with read lock (allows concurrent reads)
	cc.mutex.RLock()
	if cachedColor, exists := cc.cache[cacheKey]; exists {
		cc.mutex.RUnlock()
		return cachedColor
	}
	cc.mutex.RUnlock()

	// Cache miss: perform distance-based brightness calculation
	// Linear falloff: brightness decreases linearly with distance
	brightnessMultiplier := 1.0 - (quantizedDistance / viewDistance)

	// Apply minimum brightness constraint to prevent complete darkness
	if brightnessMultiplier < minBrightness {
		brightnessMultiplier = minBrightness
	}

	// Apply brightness multiplier to RGB channels (preserve alpha)
	calculatedColor := color.RGBA{
		R: uint8(float64(baseColor.R) * brightnessMultiplier),
		G: uint8(float64(baseColor.G) * brightnessMultiplier),
		B: uint8(float64(baseColor.B) * brightnessMultiplier),
		A: baseColor.A, // Alpha channel remains unchanged
	}

	// Second phase: store result with write lock (exclusive access)
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// Implement cache size management to prevent excessive memory usage
	if len(cc.cache) > 500 { // Maximum cache size threshold
		// Simple cache eviction: remove half the entries
		// This maintains reasonable memory usage while preserving performance benefits
		for existingKey := range cc.cache {
			delete(cc.cache, existingKey)
			if len(cc.cache) <= 250 { // Target cache size after eviction
				break
			}
		}
	}

	// Store the calculated color in cache for future use
	cc.cache[cacheKey] = calculatedColor
	return calculatedColor
}

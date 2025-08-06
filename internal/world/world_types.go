package world

// TileType3D represents different types of tiles in the 3D world.
// Each tile type has specific visual properties, collision behavior, and height characteristics.
type TileType3D int

const (
	// Basic terrain types
	TileEmpty  TileType3D = iota // Walkable empty space - no collision
	TileWall                     // Standard stone wall - full height, blocks movement
	TileWater                    // Water bodies - walkable but rendered differently
	TileDoor                     // Interactive doors - can be opened/closed
	TileStairs                   // Level transitions - special movement behavior

	// Natural features
	TileTree        // Standard forest tree - tall, blocks movement
	TileAncientTree // Large old-growth tree - extra tall, blocks movement
	TileThicket     // Dense undergrowth - short height, blocks movement
	TileMossRock    // Moss-covered stone - standard height, blocks movement

	// Environmental features
	TileMushroomRing // Magical fairy ring - walkable, visible
	TileForestStream // Flowing water - walkable, special rendering
	TileFernPatch    // Dense ferns - walkable, visible undergrowth
	TileFireflySwarm // Magical lights - walkable, glowing effect
	TileClearing     // Open grass areas - walkable, different floor color

	// Special tiles
	TileSpawn           // Player starting position - walkable, highlighted
	TileVioletTeleporter // Violet teleporter - walkable, floor-only rendering
	TileRedTeleporter    // Red teleporter - walkable, floor-only rendering

	// Variable height walls
	TileLowWall  // Short defensive wall - half height
	TileHighWall // Tall fortification - extra height
)

// Global tile manager instance
var GlobalTileManager *TileManager

// GetTileHeight returns the vertical scale multiplier for a given tile type.
// This function now fully depends on the tile manager configuration.
func GetTileHeight(tileType TileType3D) float64 {
	if GlobalTileManager != nil {
		return GlobalTileManager.GetHeightMultiplier(tileType)
	}

	// If tile manager is not initialized, return default height
	// This should not happen in normal operation since tile manager is initialized at startup
	return 1.0
}

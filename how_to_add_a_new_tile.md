# How to Add a New Tile to UgaTaima

This manual provides a step-by-step guide for adding new tiles to the UgaTaima RPG game, with precise details on working vs non-working features.

## System Overview

The tile system consists of:
- **YAML Configuration**: Data-driven tile definitions in `tiles.yaml` and `special_tiles.yaml`
- **Dynamic Type Assignment**: New tiles automatically get TileType3D values (1000+)
- **Render System**: Multiple rendering modes for different visual styles
- **Collision System**: Walkable/solid/transparent properties for movement and visibility
- **Biome Support**: Tiles can be specific to forest, desert, or water environments

## ✅ WORKING FEATURES

### **🎨 Core Tile Properties** ✅ **FULLY IMPLEMENTED**
- Collision detection (solid, walkable, transparent)
- Visual properties (colors, sprites, height)
- Map placement using single letters
- Biome-specific variants

### **🖼️ Render Types** ✅ **FULLY IMPLEMENTED**
- `tree_sprite` - 3D tree/tall object rendering
- `environment_sprite` - 3D environment objects  
- `flooring_object` - Floor-level decorative sprites
- `textured_wall` - Solid wall rendering
- `floor_only` - Floor color only (teleporters)

### **🌐 Dynamic Tile System** ✅ **FULLY IMPLEMENTED**
- Add tiles via YAML without code changes
- Automatic TileType3D assignment (1000+)
- Runtime tile property modification

### **🚪 Teleporter System** ✅ **FULLY IMPLEMENTED**
- Cross-map teleportation
- Cooldown system (5 seconds)
- Group-based teleportation (violet/red)
- Random destination selection

## ❌ NOT YET WORKING

### **🎵 Audio System** ❌ **NOT IMPLEMENTED**
- Sound effects (activation_sound, arrival_sound)
- Audio configuration ignored

### **✨ Visual Effects** ❌ **NOT IMPLEMENTED**  
- Particle effects (violet_swirl, red_flames)
- Visual effect configuration ignored

### **⚙️ Advanced Properties** ❌ **IGNORED**
- Special tile `properties` section ignored except for teleporters
- Complex behavior like spike traps, magic circles not implemented
- Most `effects` configurations unused

## Quick Reference: Tile Types

| Category | Examples | Render Type | Purpose |
|----------|----------|-------------|---------|
| **Solid Walls** | wall, thicket, moss_rock | `textured_wall` | Block movement & vision |
| **Tall Objects** | tree, ancient_tree, dune | `tree_sprite` | 3D sprites, block movement |
| **Decorations** | mushroom_ring, firefly_swarm | `environment_sprite` | Walkable 3D objects |
| **Floor Objects** | forest_stream, clearing | `flooring_object` | Floor-level decoration |
| **Special Areas** | spawn, teleporters | `floor_only` | Colored floor areas |

## Property Usage Status

| Property | Status | Purpose | Notes |
|----------|--------|---------|--------|
| `name` | ✅ **Working** | Display name | Used in debugging |
| `solid` | ✅ **Working** | Blocks movement | Core collision |
| `transparent` | ✅ **Working** | Blocks vision | Raycasting |
| `walkable` | ✅ **Working** | Movement allowed | Collision detection |
| `height_multiplier` | ✅ **Working** | Visual height | 3D rendering |
| `sprite` | ✅ **Working** | Image file | Must exist in assets/sprites/environment/ |
| `render_type` | ✅ **Working** | Rendering mode | Determines visual style |
| `floor_color` | ✅ **Working** | Floor tint | RGB color array |
| `floor_near_color` | ✅ **Working** | Nearby floor tint | Affects adjacent tiles |
| `wall_color` | ✅ **Working** | Wall tint | For textured_wall type |
| `letter` | ✅ **Working** | Map symbol | Single character for maps |
| `biomes` | ✅ **Working** | Environment filter | ["forest", "desert", "water"] |
| `properties.*` | ❌ **Ignored** | Special behavior | Only teleporter properties work |
| `effects.*` | ❌ **Ignored** | Audio/visual | Not implemented |

## Step-by-Step Implementation

### 1. Choose Configuration File

**For Regular Tiles**: `assets/tiles.yaml`
**For Special Tiles**: `assets/special_tiles.yaml`

### 2. Define Basic Tile

**File**: `assets/tiles.yaml`

#### **✅ MINIMAL WORKING EXAMPLE**
```yaml
tiles:
  my_crystal:                    # ← Unique tile key
    name: "Magic Crystal"        # ✅ Display name
    solid: false                 # ✅ Doesn't block movement
    transparent: true            # ✅ Doesn't block vision
    walkable: true               # ✅ Player can walk on it
    height_multiplier: 1.2       # ✅ 20% taller than normal
    sprite: "crystal"            # ✅ Uses crystal.png sprite
    render_type: "environment_sprite"  # ✅ 3D object rendering
    floor_color: [150, 100, 255] # ✅ Purple floor glow
    letter: "X"                  # ✅ Map symbol
```

#### **🌍 BIOME-SPECIFIC TILE**
```yaml
tiles:
  desert_cactus:
    name: "Desert Cactus"
    solid: true                  # ✅ Blocks movement
    transparent: false           # ✅ Blocks vision  
    walkable: false              # ✅ Cannot walk through
    height_multiplier: 1.8       # ✅ Tall cactus
    sprite: "cactus"             # ✅ Must exist in sprites/environment/
    render_type: "tree_sprite"   # ✅ Tall object rendering
    wall_color: [34, 139, 34]    # ✅ Green cactus color
    letter: "C"                  # ✅ C symbol in maps
    biomes: ["desert"]           # ✅ Only in desert maps
```

### 3. Advanced Examples

#### **✅ WORKING TELEPORTER**
```yaml
special_tiles:
  blue_teleporter:
    name: "Blue Teleporter"
    solid: false
    transparent: true
    walkable: true
    height_multiplier: 1.0
    render_type: "floor_only"    # ✅ Floor color only
    floor_color: [0, 100, 255]   # ✅ Blue floor
    # Teleporter functionality works automatically for tiles ending in "teleporter"
```

#### **❌ NON-WORKING ADVANCED FEATURES**
```yaml
special_tiles:
  spike_trap:
    name: "Spike Trap"
    solid: false
    transparent: true
    walkable: true
    render_type: "floor_only"
    floor_color: [139, 69, 19]
    # All properties below are IGNORED ❌
    properties:                  # ❌ Not implemented
      damage: 10
      trigger_chance: 0.8
    effects:                     # ❌ Not implemented  
      trigger_sound: "spike_trap"
      particle_effect: "blood_splatter"
```

### 4. Add Sprite Asset (If Needed)

**For `sprite` property, add to**: `assets/sprites/environment/`

**Available Sprites**: ✅
- `ancient_tree.png`, `coral_reef.png`, `fern_patch.png`
- `firefly_swarm.png`, `forest_stream.png`, `grass.png`
- `large_coral.png`, `large_dune.png`, `moss_rock.png`
- `mushroom_ring.png`, `sand_dune.png`, `tree.png`

### 5. Use in Map Files

**Add to any `.map` file**:
```
# Map layout using letter symbols
..........
....X.....    ← X represents your new tile
..........
```

**No additional configuration needed** - the letter mapping works automatically!

## Render Type Details

### **`tree_sprite`** ✅ **WORKING**
```yaml
render_type: "tree_sprite"
sprite: "ancient_tree"
height_multiplier: 2.0          # Extra tall
solid: true                     # Blocks movement
transparent: false              # Blocks vision
```
- **Purpose**: Tall 3D objects (trees, towers, large rocks)
- **Behavior**: Scaled based on distance, depth buffering
- **Best For**: Obstacles that should look imposing

### **`environment_sprite`** ✅ **WORKING**  
```yaml
render_type: "environment_sprite"
sprite: "mushroom_ring"
height_multiplier: 1.0
solid: false                    # Usually walkable
transparent: true               # Usually see-through
```
- **Purpose**: 3D decorative objects
- **Behavior**: 3D rendering with distance scaling
- **Best For**: Walkable decorations, interactive objects

### **`flooring_object`** ✅ **WORKING**
```yaml
render_type: "flooring_object"  
sprite: "grass"
height_multiplier: 1.0
walkable: true                  # Floor-level objects
```
- **Purpose**: Floor-level decorative sprites
- **Behavior**: Rendered on ground plane
- **Best For**: Grass, streams, floor patterns

### **`textured_wall`** ✅ **WORKING**
```yaml
render_type: "textured_wall"
wall_color: [64, 64, 64]        # Gray stone
solid: true                     # Blocks movement
transparent: false              # Blocks vision
```
- **Purpose**: Solid walls and barriers
- **Behavior**: Traditional raycasting wall rendering
- **Best For**: Walls, doors, solid barriers

### **`floor_only`** ✅ **WORKING**
```yaml
render_type: "floor_only"
floor_color: [255, 0, 0]        # Red floor
solid: false                    # Usually walkable
walkable: true
```
- **Purpose**: Colored floor areas only
- **Behavior**: Only affects floor color
- **Best For**: Teleporters, spawn points, special areas

## Advanced Features

### **✅ WORKING: Dynamic Properties**
```go
// Runtime tile modification (advanced)
world.GlobalTileManager.SetTileProperty(tileType, "walkable", false)
world.GlobalTileManager.SetTileProperty(tileType, "height_multiplier", 2.0)
```

### **✅ WORKING: Biome Letter Reuse System**
```yaml
# Same letter "T", different tiles per biome
tree:                            # Forest version
  letter: "T"                    # T = Tree in forests
  biomes: ["forest"]
  sprite: "tree"
  wall_color: [101, 67, 33]      # Brown bark

desert_dune:                     # Desert version  
  letter: "T"                    # T = Dune in deserts (same letter!)
  biomes: ["desert"]
  sprite: "sand_dune"
  wall_color: [238, 203, 173]    # Sandy brown

coral_reef:                      # Water version
  letter: "T"                    # T = Coral underwater (same letter!)
  biomes: ["water"]
  sprite: "coral_reef"
  wall_color: [255, 127, 80]     # Coral orange
```

**Result**: The same map file with "T" symbols will show trees in forest, dunes in desert, and coral in water environments!

### **✅ WORKING: Floor Color Effects**
```yaml
my_tile:
  floor_color: [100, 200, 100]        # This tile's floor color
  floor_near_color: [80, 160, 80]     # Affects adjacent empty tiles
```

## Testing Checklist

### **✅ BASIC VALIDATION**
- [ ] **Required Properties**:
  - [ ] `name` field exists
  - [ ] `solid`, `transparent`, `walkable` defined
  - [ ] `height_multiplier` specified  
  - [ ] `render_type` is valid
  - [ ] `letter` is single character and unique

### **✅ VISUAL TESTING**
- [ ] Tile appears correctly in maps using letter symbol
- [ ] Collision behavior works (solid/walkable as expected)
- [ ] Height appears correct in 3D view
- [ ] Colors render properly (floor_color, wall_color)
- [ ] Sprite loads if specified (check console for errors)

### **✅ SPRITE VALIDATION** 
- [ ] Sprite file exists in `assets/sprites/environment/`
- [ ] Sprite name matches `sprite` property exactly
- [ ] File is PNG format

### **❌ SKIP TESTING (Won't Work)**
- [ ] ~~Sound effects~~ (Not implemented)
- [ ] ~~Particle effects~~ (Not implemented)
- [ ] ~~Complex properties~~ (Ignored except teleporters)
- [ ] ~~Special behaviors~~ (Only teleportation works)

## Important Notes

### **🔥 Critical Requirements**

#### **Letter Uniqueness** ⚠️ **BIOME-AWARE**
```yaml
# WRONG ❌ - Letter conflict within same biome
tree: { letter: "T", biomes: ["forest"] }
tower: { letter: "T", biomes: ["forest"] }  # Conflict in forest!

# CORRECT ✅ - Same letter, different biomes  
tree: { letter: "T", biomes: ["forest"] }     # T = Tree in forests
desert_dune: { letter: "T", biomes: ["desert"] }  # T = Dune in deserts
coral_reef: { letter: "T", biomes: ["water"] }    # T = Coral underwater

# ALSO CORRECT ✅ - Unique letters within biome
tree: { letter: "T", biomes: ["forest"] }
tower: { letter: "R", biomes: ["forest"] }  # Different letters
```

**Letter Reuse Rules**:
- ✅ **Same letter, different biomes**: Allowed and encouraged
- ❌ **Same letter, same biome**: Will cause conflicts  
- ✅ **No biome specified**: Letter must be globally unique

#### **Sprite File Requirements** ⚠️ **IMPORTANT**
- Must exist in `assets/sprites/environment/`
- PNG format required
- Filename must match `sprite` property exactly

#### **Render Type Validation** ⚠️ **IMPORTANT**
```yaml
# VALID ✅
render_type: "tree_sprite"      # For tall objects
render_type: "environment_sprite"  # For decorations
render_type: "flooring_object"  # For floor items
render_type: "textured_wall"    # For walls
render_type: "floor_only"       # For floor colors only

# INVALID ❌ 
render_type: "custom_type"      # Will default to textured_wall
```

### **🎯 Best Practices**

#### **Collision Logic**
```yaml
# Walls/Barriers
solid: true
transparent: false
walkable: false

# Decorations/Objects  
solid: false
transparent: true
walkable: true

# See-through barriers
solid: true          # Blocks movement
transparent: true    # Allows vision
walkable: false      # Cannot walk through
```

#### **Height Guidelines**
- `height_multiplier: 0.5` - Low walls, rocks
- `height_multiplier: 1.0` - Normal height  
- `height_multiplier: 1.5` - Tall walls
- `height_multiplier: 2.0+` - Trees, towers

## Architecture

```
Tile Creation Flow:
YAML Definition → TileManager.LoadTileConfig() → Dynamic TileType3D Assignment → 
Map Loading → Letter Mapping → World Placement → Rendering

Rendering Flow:
Map Symbol → TileType3D → TileManager.GetRenderType() → Renderer Switch → 
Specific Render Function → 3D Display
```

## Current Limitations

### **✅ What Actually Works**
- **Complete YAML-based tile system**
- **All core visual and collision properties**  
- **Dynamic tile addition without code changes**
- **Full teleporter functionality**
- **Biome-specific tile variants**

### **❌ What Doesn't Work (Despite YAML Support)**
- **Audio system** (no sound implementation)
- **Particle effects** (no particle system)
- **Complex special tile behaviors** (spike traps, magic circles)
- **Most `properties` and `effects` configurations**

### **💡 Recommendation**
Focus on the **core tile properties** and **basic render types** for reliable functionality. The tile system is very robust for basic tiles, decoration, and environment creation. Advanced interactive features need additional implementation work.

The tile system provides an excellent foundation for creating diverse, visually appealing game environments with minimal code changes! 🎮
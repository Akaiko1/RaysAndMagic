# How to Add a New Monster to UgaTaima

This manual provides a step-by-step guide for adding new monsters to the UgaTaima RPG game, with precise details on working vs non-working features.

## System Overview

The monster system consists of:
- **YAML Configuration**: Fully dynamic monster definitions in `assets/monsters.yaml`
- **AI Behavior System**: State-based AI with configurable parameters
- **Habitat Placement**: Rule-based spawning in appropriate locations
- **Combat Integration**: Damage, resistances, and loot systems
- **Visual Rendering**: Sprite-based 3D rendering with depth testing
- **Map Integration**: Letter-based placement and random generation

## ✅ WORKING FEATURES

### **🎯 Dynamic Configuration System** ✅ **FULLY IMPLEMENTED**
- Add monsters by editing YAML only - no code changes needed
- Automatic monster creation from configuration
- Runtime validation and error checking
- Hot-reloadable configuration system

### **🧠 AI Behavior System** ✅ **FULLY IMPLEMENTED**
- **5 AI States**: Idle, Patrolling, Alert, Attacking, Fleeing
- **Configurable Parameters**: Alert radius, attack radius, speed, timers
- **Collision-Aware Movement**: Monsters avoid walls and obstacles
- **State Transitions**: Automatic transitions based on conditions and timers

### **🏞️ Habitat Placement System** ✅ **FULLY IMPLEMENTED**
- **Habitat Preferences**: Spawn on preferred tile types
- **Habitat Near Rules**: Spawn near specific tile types within radius
- **Random Placement**: Automatic distribution across maps
- **Manual Placement**: Letter-based placement in map files

### **⚔️ Combat System** ✅ **FULLY IMPLEMENTED**
- **Damage Calculation**: Min/max damage with attack bonus
- **Armor Class**: Defense rating affecting hit chance
- **Damage Resistances**: Percentage-based resistance/vulnerability system
- **Experience & Loot**: Gold drops and experience rewards

### **🎨 Visual System** ✅ **FULLY IMPLEMENTED**
- **Sprite Rendering**: Distance-based scaling and depth testing
- **Collision Visualization**: Optional collision box display
- **Distance Shading**: Brightness effects based on distance
- **Size Configuration**: Custom width/height per monster

## ❌ NOT YET WORKING

### **🎵 Audio System** ❌ **NOT IMPLEMENTED**
- No monster sound effects
- No death/attack/movement sounds

### **✨ Advanced Visual Effects** ❌ **NOT IMPLEMENTED**
- No particle effects
- No special animations beyond basic sprite rendering
- No glowing or magical effects

### **🏆 Advanced Loot System** ❌ **PARTIALLY IMPLEMENTED**
- Gold drops work, but item drops are not implemented
- No equipment drops from monsters
- No rare/special loot tables

## 📋 Property Usage Analysis

### **✅ FULLY WORKING PROPERTIES**

| Property | Purpose | Example | Notes |
|----------|---------|---------|--------|
| `name` | Display name | `"Goblin"` | Used in combat messages |
| `level` | Monster level | `3` | Affects scaling and difficulty |
| `max_hit_points` | Health pool | `25` | Maximum and starting HP |
| `armor_class` | Defense rating | `8` | Affects hit chance calculations |
| `experience` | XP reward | `65` | Given on death |
| `attack_bonus` | Attack modifier | `2` | Added to hit rolls |
| `damage_min` / `damage_max` | Damage range | `2` / `8` | Random damage on hit |
| `alert_radius` | Detection range | `120` | Distance to notice threats |
| `attack_radius` | Melee range | `32` | Distance to attack |
| `speed` | Movement speed | `1.2` | Multiplier for base speed |
| `gold_min` / `gold_max` | Loot range | `5` / `25` | Random gold drop |
| `sprite` | Visual sprite | `"orc"` | PNG file in assets/sprites/mobs/ |
| `letter` | Map symbol | `"o"` | Single character for map placement |
| `width` / `height` | Collision size | `32` / `32` | Bounding box dimensions |
| `resistances` | Damage modifiers | `fire: 50` | Percentage resistance/vulnerability |
| `habitat_preferences` | Preferred tiles | `["empty", "clearing"]` | Where monster can spawn |
| `habitat_near` | Proximity rules | `type: "clearing", radius: 2` | Spawn near specific tiles |

### **❌ IGNORED/UNUSED PROPERTIES**
Currently all defined properties are working! The monster system is very complete.

## Step-by-Step Implementation

### 1. Define Monster in Configuration

**File**: `assets/monsters.yaml`

#### **✅ MINIMAL WORKING EXAMPLE**
```yaml
monsters:
  ice_troll:                      # ← Unique monster key
    name: "Ice Troll"             # ✅ Display name
    level: 7                      # ✅ Monster level
    max_hit_points: 95            # ✅ Health pool
    armor_class: 11               # ✅ Defense rating
    experience: 350               # ✅ XP reward
    attack_bonus: 5               # ✅ Attack modifier
    damage_min: 4                 # ✅ Minimum damage
    damage_max: 20                # ✅ Maximum damage
    alert_radius: 140             # ✅ Detection range
    attack_radius: 40             # ✅ Melee range
    speed: 1.1                    # ✅ Movement speed
    gold_min: 25                  # ✅ Minimum gold drop
    gold_max: 80                  # ✅ Maximum gold drop
    sprite: "goblin"              # ✅ Sprite file (placeholder)
    letter: "i"                   # ✅ Map symbol (must be unique)
    width: 40                     # ✅ Collision width
    height: 40                    # ✅ Collision height
    resistances: {}               # ✅ No special resistances
    habitat_preferences:          # ✅ Spawn locations
      - "empty"
```

#### **🔥 ADVANCED EXAMPLE WITH ALL FEATURES**
```yaml
monsters:
  frost_dragon:
    name: "Frost Dragon"
    level: 18
    max_hit_points: 500
    armor_class: 20
    experience: 3000
    attack_bonus: 12
    damage_min: 15
    damage_max: 50
    alert_radius: 400
    attack_radius: 100
    speed: 2.8
    gold_min: 800
    gold_max: 2000
    sprite: "dragon"
    letter: "F"
    width: 56
    height: 56
    resistances:
      fire: -75                   # ✅ Vulnerable to fire (takes 175% damage)
      water: 90                   # ✅ Resistant to water (takes 10% damage)
      physical: 40                # ✅ Resistant to physical (takes 60% damage)
      air: 50                     # ✅ Resistant to air magic
    habitat_preferences:          # ✅ Where it can spawn randomly
      - "empty"
      - "clearing"
    habitat_near:                 # ✅ Must be near these tiles
      - type: "ancient_tree"      # ✅ Near ancient trees
        radius: 5                 # ✅ Within 5 tiles
      - type: "water"             # ✅ Or near water
        radius: 3                 # ✅ Within 3 tiles
```

**🔑 Key Requirements:**
- **Letter uniqueness**: Each monster needs a unique single character
- **Sprite files**: Must exist in `assets/sprites/mobs/`
- **Numeric ranges**: All numeric values must be > 0
- **Resistance format**: Use integers (-100 to 100+)

### 2. Add Sprite Asset (If Needed)

**Directory**: `assets/sprites/mobs/`

**Available Sprites**: ✅
- `dragon.png`, `dire_wolf.png`, `forest_orc.png`
- `forest_spider.png`, `goblin.png`, `orc.png`
- `pixie.png`, `skeleton.png`, `treant.png`

**For new sprites**: Add PNG file matching the `sprite` property exactly.

### 3. Configure Special Placement (Optional)

**File**: `assets/monsters.yaml` (bottom section)

```yaml
# Monster placement configuration  
placement:
  common:
    count_min: 15                 # ✅ Minimum random monsters per map
    count_max: 30                 # ✅ Maximum random monsters per map
  special:
    treant_chance: 0.3            # ✅ 30% chance for treant
    pixie_count_max: 3            # ✅ Up to 3 pixies
    dragon_chance: 0.05           # ✅ 5% chance for dragon
    troll_chance: 0.1             # ✅ 10% chance for troll
    frost_dragon_chance: 0.02     # ← Add custom special spawns
```

### 4. Use in Map Files (Optional)

**File**: Any `.map` file (forest.map, desert.map, etc.)

```
# Map layout with monster placement
%.@..T...T....T...R.....T........T....T..........%
%.....P....T......................T..........d...%  
%..T.T.......CCCCCC..T.....A.......T.......i.....%  ← i = ice_troll
%g.T.......T.CCCCCC...T.........T.....T..........%  ← g = goblin
```

**How it works:**
- Letters in map files automatically spawn monsters
- Monster spawns at tile coordinates, converted to world coordinates
- Overrides habitat preferences for manual placement

### 5. Testing Your Monster

#### **✅ MONSTER VALIDATION CHECKLIST**
- [ ] **Configuration Loads**: No YAML parsing errors
- [ ] **Letter Uniqueness**: No conflicts with existing monsters
- [ ] **Sprite Exists**: PNG file present in mobs directory
- [ ] **Spawns Correctly**: Appears in game world
- [ ] **AI Functions**: Moves and changes states properly
- [ ] **Combat Works**: Takes/deals damage correctly
- [ ] **Resistances Work**: Damage modification applies
- [ ] **Loot Drops**: Gold drops on death
- [ ] **Habitat Rules**: Spawns in correct locations

## Advanced Features

### **✅ WORKING: Damage Resistance System**

```yaml
resistances:
  physical: 30      # Takes 70% physical damage
  fire: -50         # Takes 150% fire damage (vulnerable)
  water: 100        # Takes 0% water damage (immune)
  mind: 75          # Takes 25% mind magic damage
```

**Damage Types Available:**
- `physical`, `fire`, `water`, `air`, `earth`
- `spirit`, `mind`, `body`, `light`, `dark`

**Resistance Values:**
- **Positive**: Reduces damage (50 = takes 50% damage)
- **Negative**: Increases damage (-25 = takes 125% damage)
- **100+**: Immunity (takes 0% damage)

### **✅ WORKING: AI Behavior Configuration**

**Global AI settings in** `config.yaml`:
```yaml
monster_ai:
  idle_patrol_timer: 300         # Frames before considering patrol
  patrol_direction_timer: 120    # Frames before direction change
  patrol_idle_timer: 600         # Frames before returning to idle
  alert_timeout: 180             # Frames in alert state
  attack_cooldown: 60            # Frames between attacks
  flee_duration: 300             # Frames to flee when damaged
  
  # Behavior chances (0.0 to 1.0)
  idle_to_patrol_chance: 0.1     # 10% chance to start patrolling
  patrol_direction_chance: 0.3   # 30% chance to change direction
```

### **✅ WORKING: Habitat System**

#### **Basic Habitat Preferences**
```yaml
habitat_preferences:
  - "empty"           # Spawn on empty tiles
  - "clearing"        # Spawn on clearings
  - "mushroom_ring"   # Spawn on mushroom rings
```

#### **Advanced Proximity Rules**
```yaml
habitat_near:
  - type: "forest_stream"    # Must be near streams
    radius: 3                # Within 3 tiles
  - type: "ancient_tree"     # OR near ancient trees
    radius: 2                # Within 2 tiles
```

**Available Tile Types:**
- `empty`, `tree`, `ancient_tree`, `thicket`, `clearing`
- `forest_stream`, `mushroom_ring`, `firefly_swarm`, `fern_patch`
- `moss_circle`, `fallen_log`, `boulder`, `flower_patch`

## Quick Reference: Monster Categories

| Category | Level Range | Examples | Purpose |
|----------|-------------|----------|---------|
| **Basic** | 1-3 | Goblin, Wolf, Spider | Early game enemies |
| **Intermediate** | 4-7 | Orc, Bear, Troll | Mid-game challenges |
| **Advanced** | 8-12 | Treant, Forest Orc | Late game content |
| **Boss** | 15+ | Dragon | End-game encounters |

## Monster AI States

| State | Behavior | Transitions |
|-------|----------|-------------|
| **Idle** | Stationary, occasionally starts patrolling | → Patrolling, Alert |
| **Patrolling** | Random movement, direction changes | → Idle, Alert |
| **Alert** | Looking for threats, heightened awareness | → Idle, Attacking |
| **Attacking** | Combat actions, damage dealing | → Alert |
| **Fleeing** | Escape movement when heavily damaged | → Idle, Alert |

## Testing Checklist

### **✅ BASIC FUNCTIONALITY**
- [ ] Monster spawns in world
- [ ] Sprite renders correctly
- [ ] Collision detection works
- [ ] AI state changes occur
- [ ] Combat damage applies

### **✅ ADVANCED FEATURES**
- [ ] Habitat placement rules work
- [ ] Resistances modify damage correctly
- [ ] Gold drops on death
- [ ] Letter placement in maps works
- [ ] Alert/attack radiuses function

### **🔧 PERFORMANCE TESTING**
- [ ] Multiple monsters don't cause lag
- [ ] Pathfinding doesn't get stuck
- [ ] Rendering performs well with many monsters

## Important Notes

### **Monster Letter Conflicts**
```yaml
# WRONG ❌ - Letter conflict
goblin: { letter: "g" }
giant: { letter: "g" }    # Conflict! System will detect and error

# CORRECT ✅ - Unique letters
goblin: { letter: "g" }
giant: { letter: "G" }    # Different letter (case sensitive)
```

### **Sprite File Requirements**
- Must be PNG format
- Located in `assets/sprites/mobs/`
- Filename must match `sprite` property exactly
- Recommended size: 32x32 to 64x64 pixels

### **Performance Considerations**
- Many monsters (30+) may impact performance
- Large alert radiuses can be expensive
- Complex habitat rules slow placement

### **Damage Resistance Logic**
```
final_damage = base_damage * (100 - resistance) / 100

Examples:
- 50% resistance: 100 damage → 50 damage
- -25% resistance: 100 damage → 125 damage  
- 100% resistance: 100 damage → 0 damage
```

## Architecture

```
Monster Creation Flow:
YAML Definition → MonsterConfig.LoadMonsterConfig() → World.populateWithMonsters() → 
MonsterPlacement → NewMonster3DFromConfig() → AI System → Rendering → Combat Integration

Monster Update Loop:
AI State Machine → Collision Checking → Position Update → Rendering → Combat Processing
```

## Current Limitations

### **✅ What Actually Works**
- **Complete YAML-based configuration system**
- **Full AI behavior with 5 states**
- **Advanced habitat placement rules**
- **Comprehensive combat and resistance system**
- **Professional sprite rendering with depth testing**
- **Map-based and random placement**

### **❌ What Doesn't Work**
- **Audio effects** (no monster sounds)
- **Advanced visual effects** (no particles or special animations)
- **Item drops** (only gold drops work)
- **Advanced AI behaviors** (no pathfinding to specific targets)

### **💡 Summary**

The monster system supports dynamic YAML configuration. You can add monsters with AI behaviors, resistances, and placement rules without code changes.

**Use cases**: Enemy encounters, boss monsters, environmental spawning, combat progression.
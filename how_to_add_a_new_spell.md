# How to Add a New Spell to UgaTaima

This manual provides a step-by-step guide for adding new spells to the UgaTaima RPG game using the **fully dynamic spell system**.

## System Overview

The spell system has been **completely refactored** to be data-driven with **zero hardcoded maintenance**:

- **Spell Definitions**: YAML-based spell properties (damage, cost, duration, school)
- **Spell IDs**: Dynamic string identifiers (e.g., "firebolt", "lightning") 
- **Magic Schools**: YAML-defined spell categorization and class restrictions
- **Combat Configuration**: YAML-based projectile behavior (speed, lifetime, collision)
- **Graphics Configuration**: YAML-based visual rendering (size, color, effects)
- **Character Setup**: YAML-based spell assignment to character classes

## Quick Reference: Magic Schools

| School | Classes | Available Spells | Behavior |
|--------|---------|------------------|----------|
| `fire` | Sorcerer | Fireball, Fire Bolt, Torch Light | Offensive + utility |
| `water` | Druid | Walk on Water | Movement magic |
| `air` | Archer | Lightning, Wizard Eye | Offensive + vision |
| `earth` | - | - | Not yet implemented |
| `body` | Cleric | Heal, Heal Other | Healing magic |
| `mind` | - | Awaken | Status effects |
| `spirit` | Paladin | Bless | Party buffs |
| `light` | - | - | Restricted magic |
| `dark` | - | - | Restricted magic |

## Spell Categories

### **Projectile Spells** ✅ Fully Automatic
- Create moving projectiles that hit targets
- Use combat + graphics configuration
- **No code changes needed** - just add YAML config!
- Examples: Fireball, Fire Bolt, Lightning

### **Utility Spells** - Two Types:

#### **Simple Utility Spells** ✅ Mostly Automatic
- Basic healing, damage, or instant effects
- Handled by generic utility spell logic
- **Minimal code changes needed**
- Examples: Heal, First Aid

#### **State-Based Utility Spells** ❌ Manual Implementation
- Complex effects requiring game state management
- Need custom switch cases and state variables
- **Requires code changes in multiple files**
- Examples: Torch Light, Wizard Eye, Bless, Walk on Water

## ⚠️ Parameter Usage Reality

**Before adding spells, understand which parameters actually work:**

| Parameter | Projectile Spells | Healing Spells | Notes |
|-----------|-------------------|----------------|-------|
| `spell_points_cost` | ✅ **CRITICAL** | ✅ Used | SP cost AND damage calculation base |
| `damage` | ❌ **IGNORED!** | ✅ **CRITICAL** | Projectiles use `spell_points_cost * 3` instead |
| `range` | ❌ **IGNORED!** | ❌ **IGNORED!** | No range validation - use `lifetime` for range |
| `projectile_speed` | ✅ Used | N/A | Speed multiplier |  
| `projectile_size` | ✅ Used | N/A | Collision radius |
| `lifetime` | ✅ **CRITICAL** | N/A | Controls actual range via time |
| `is_projectile` | ✅ Used | ✅ Used | Determines spell behavior type |
| `is_utility` | ✅ Used | ✅ Used | Utility vs projectile routing |
| `visual_effect` | ✅ Basic | ✅ Basic | Simple visual system |

**Key Insights:**
- **Projectile damage** = `spell_points_cost * 3` + intellect bonus (config `damage` ignored!)
- **Healing amount** = config `damage` + personality bonus
- **Range control** = `lifetime` in frames, not `range` parameter
- **All tooltips use same calculations as combat** (fixed for accuracy)

## Casting Methods Overview

The game has **two different ways** to cast spells, each using different functions:

### **Method 1: Equipped Spell Casting (F Key)**
- **Function**: `CastEquippedSpell()` in `combat.go`  
- **Usage**: Cast the spell equipped in the unified spell slot
- **Key**: F key (or H key for targeted healing)
- **Equipment**: Spell must be equipped as an item in `SlotSpell`
- **Behavior**: Uses equipped spell item properties

### **Method 2: Spellbook Casting (M Key)**  
- **Function**: `CastSelectedSpell()` in `combat.go`
- **Usage**: Cast spells directly from character's known spells
- **Key**: M key opens spellbook, then cast selected spell
- **Requirements**: Character must know the spell in their magic school
- **Behavior**: Uses spell definition directly from config

### **Implementation Impact**
When implementing utility spells, you need to handle **both casting paths**:
- Both functions call the same `ApplyUtilitySpell()` method
- Both apply effects through centralized helper methods (e.g., `applyBlessEffect()`)
- **DO NOT** duplicate spell effect logic in both functions

## Step-by-Step Implementation

### 1. Add Spell Definition to YAML

**File**: `config.yaml` (around line 250)

Add your spell to the `spells.definitions` section:

```yaml
spells:
  definitions:
    # ... existing spells ...
    
    ice_shard:                    # ← Your spell ID (lowercase, underscores)
      name: "Ice Shard"           # ← Display name
      description: "Launches a piercing shard of ice"
      school: "water"             # ← Magic school (determines class compatibility)
      level: 2                    # ← Spell level requirement
      spell_points_cost: 6        # ← Spell points required to cast
      duration: 0                 # ← 0 for instant, >0 for duration effects (seconds)
      damage: 10                  # ← Base damage (for projectile spells)
      range: 10                   # ← Range in tiles
      projectile_speed: 1.2       # ← Speed multiplier (base_speed * this)
      projectile_size: 14         # ← Visual size
      lifetime: 80                # ← Projectile lifetime in frames
      is_projectile: true         # ← Creates moving projectile
      is_utility: false           # ← Not a utility spell
      visual_effect: "ice_shard"  # ← Visual effect identifier
```

**Key Fields Explained:**
- **school**: Determines which classes can learn it
- **is_projectile**: `true` = creates projectile, `false` = instant effect
- **is_utility**: `true` = healing/buffs, `false` = damage/offensive
- **projectile_speed**: Multiplier for base config speed (config_speed * this)
- **duration**: In seconds (0 for instant, >0 for duration effects)

### 2. Add Combat Configuration

**File**: `config.yaml` (around line 20)

Add to the `combat.spells` section:

```yaml
combat:
  spells:
    # ... existing spells ...
    
    ice_shard:                    # ← MUST MATCH your spell ID exactly
      speed: 10.0                 # ← Base projectile speed (pixels/frame)
      lifetime: 80                # ← Projectile lifetime (frames)
      hit_radius: 250             # ← Hit detection radius (pixels)
      collision_size: 14          # ← Size for collision detection (pixels)
```

### 3. Add Graphics Configuration

**File**: `config.yaml` (around line 450)

Add to the `graphics.projectiles.spells` section:

```yaml
graphics:
  projectiles:
    spells:
      # ... existing spells ...
      
      ice_shard:                  # ← MUST MATCH your spell ID exactly
        max_size: 42              # ← Maximum visual size
        min_size: 3               # ← Minimum visual size
        base_size: 14             # ← Base visual size
        color: [100, 200, 255]   # ← RGB color values
```

### 4. Add to Character Magic Schools (Optional)

**File**: `internal/character/character.go` (around line 254)

Add to appropriate class setup (e.g., Druid for Water magic):

```go
func (c *MMCharacter) setupDruid(cfg *config.Config) {
    // ... existing setup ...
    c.MagicSchools[MagicWater] = &MagicSkill{
        Level:   1,
        Mastery: MasteryNovice,
        KnownSpells: []spells.SpellID{
            spells.SpellIDAwaken,
            spells.SpellIDIceShard,  // ← Add to water school
        },
    }
}
```

**Note**: This step is optional. The spell system is now fully dynamic and will work even if you don't add the spell to character starting spells. Players can learn spells from NPCs or find them as loot.

## Advanced: Utility Spell Examples

For non-projectile spells (healing, buffs, vision, movement), the process is different:

### Example 1: Torch Light (Duration Utility Spell)

**1. Spell Definition:**
```yaml
spells:
  definitions:
    torch_light:
      name: "Torch Light"
      description: "Creates a magical light that illuminates the surroundings"
      school: "fire"
      level: 1
      spell_points_cost: 1
      duration: 300              # 5 minutes
      damage: 0
      range: 0
      is_projectile: false
      is_utility: true
      visual_effect: "light"
```

**2. Utility Spell Handler:**

**File**: `internal/spells/casting_system.go` (around line 75)

```go
func (cs *CastingSystem) ApplyUtilitySpell(spellType SpellType, casterPersonality int) UtilitySpellResult {
    def := GetSpellDefinition(spellType)
    spellID := SpellTypeToID(spellType)

    switch string(spellID) {
    case "torch_light":
        return UtilitySpellResult{
            Type:        spellType,
            Success:     true,
            Message:     "A magical light illuminates the area!",
            VisionBonus: 50.0,
            Duration:    def.Duration * 60,  // Convert to frames
        }
    }
}
```

**3. Game State Management:**

**File**: `internal/game/combat.go` (around line 85)

For **both casting methods**, add your spell to the utility spell effects handler:

```go
// Apply utility spell effects dynamically based on spell ID
spellID := spells.SpellTypeToID(spellType)
switch string(spellID) {
case "torch_light":
    cs.game.torchLightActive = true
    cs.game.torchLightDuration = result.Duration
    cs.game.torchLightRadius = 4.0 // 4-tile radius
}
```

**⚠️ CRITICAL**: This switch statement appears in **TWO different functions**:
- `CastEquippedSpell()` (F key casting)
- `CastSelectedSpell()` (M key spellbook casting)

**Best Practice**: Create a centralized helper method to avoid code duplication:

```go
// Add helper method at end of combat.go
func (cs *CombatSystem) applyTorchLightEffect(duration int) {
    cs.game.torchLightActive = true
    cs.game.torchLightDuration = duration
    cs.game.torchLightRadius = 4.0
}

// Then call from both switch statements:
case "torch_light":
    cs.applyTorchLightEffect(result.Duration)
```

**4. UI Timer Display:**

**File**: `internal/game/ui.go` (around line 200)

```go
// Torch Light effect
if ui.game.torchLightActive && ui.game.torchLightDuration > 0 {
    ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "🔥", ui.game.torchLightDuration, 18000)
    currentX += iconSpacing
    hasActiveSpells = true
}
```

### Example 2: Heal (Instant Utility Spell)

**1. Spell Definition:**
```yaml
spells:
  definitions:
    heal:
      name: "First Aid"
      description: "Restores health to the caster"
      school: "body"
      level: 1
      spell_points_cost: 2
      duration: 0                # Instant spell
      damage: 15                 # Used for healing amount (base healing)
      range: 0                   # Unused for utility spells
      is_projectile: false
      is_utility: true
      visual_effect: "heal"
```

**2. Utility Spell Handler:**

**File**: `internal/spells/casting_system.go` (around line 85)

```go
case "heal":
    _, _, healAmount := CalculateHealingAmount(spellType, casterPersonality)
    result.Message = "You feel renewed!"
    result.HealAmount = healAmount
    result.TargetSelf = true
```

**3. Combat Application:**

**File**: `internal/game/combat.go` (around line 70)

```go
// Apply healing effects for heal spells
if result.HealAmount > 0 {
    spellID := spells.SpellTypeToID(spellType)
    if result.TargetSelf || string(spellID) == "heal" {
        // Heal self
        caster.HitPoints += result.HealAmount
        if caster.HitPoints > caster.MaxHitPoints {
            caster.HitPoints = caster.MaxHitPoints
        }
    }
}
```

**No combat or graphics config needed** - utility spells don't create projectiles!

## Testing Checklist

- [ ] Spell definition added to `config.yaml` under `spells.definitions`
- [ ] Combat configuration added to `config.yaml` under `combat.spells` (projectile spells only)
- [ ] Graphics configuration added to `config.yaml` under `graphics.projectiles.spells` (projectile spells only)
- [ ] **Spell can be cast from spellbook (M key method)**
- [ ] **Spell can be equipped and cast with F key (equipped method)**
- [ ] **Both casting methods produce identical effects** (no duplicate logic)
- [ ] Projectile collision works (if projectile spell)
- [ ] Visual effects display correctly
- [ ] Damage/healing calculation is accurate for both methods
- [ ] Tooltip shows correct information
- [ ] Utility spell timers display in UI (if duration spell)
- [ ] No code duplication between `CastEquippedSpell()` and `CastSelectedSpell()`

## Important Notes

### Spell Schools and Class Restrictions

Classes can only learn spells from specific schools:
- **Sorcerer**: Fire, Earth (elemental magic)
- **Cleric**: Body, Mind, Spirit (self magic)  
- **Paladin**: Spirit (blessed magic)
- **Archer**: Air (nature magic)
- **Druid**: Water, Earth (nature magic)
- **Knight**: No magic

**Note**: Magic school access is hardcoded in the character system. The `school` field in spell definitions is used for organization and character access control, but the schools themselves are not configurable.

### Projectile vs Utility Spells

**Projectile Spells** (`is_projectile: true`):
- Create moving projectiles that travel and hit targets
- Need combat + graphics configuration
- Use collision detection system
- Examples: Fireball, Fire Bolt, Lightning

**Utility Spells** (`is_utility: true`):
- Apply effects instantly (heal, buff, vision, movement)
- No projectiles or collision detection needed
- Handled by `ApplyUtilitySpell()` function
- Examples: Heal, Torch Light, Wizard Eye, Bless, Walk on Water

### Configuration Naming

**All configuration names must match exactly:**
- Spell ID: `"ice_shard"` (in YAML definitions)
- Combat config: `"ice_shard"` (in combat.spells)
- Graphics config: `"ice_shard"` (in graphics.projectiles.spells)

**All three must match for the system to work!**

### Centralized Calculation Functions

The spell system uses centralized calculation functions for consistency between combat and tooltips:

**Projectile Spell Damage:**
```go
// CalculateSpellDamage(spellType, casterIntellect)
// Base damage = spell_points_cost * 3  (NOTE: config 'damage' field is IGNORED!)
// Intellect bonus = caster_intellect / 2
// Total damage = base_damage + intellect_bonus
```

**Healing Spell Amount:**
```go
// CalculateHealingAmount(spellType, casterPersonality)
// Base healing = config 'damage' field  (NOTE: config 'damage' IS used for healing!)
// Personality bonus = caster_personality / 2
// Total healing = base_healing + personality_bonus
```

Both functions are used consistently for combat calculations and tooltip display.

### Utility Spell Timers

Utility spell durations are managed in frames:
- YAML `duration` field is in seconds
- Converted to frames: `duration * 60` (60 FPS)
- UI displays progress bars based on remaining frames

## Architecture

```
Spell Cast Flow:
SpellID → SpellDefinition → School Check → SpellPointsCost Check → 
  ├─ Projectile: CalculateSpellDamage() → CreateProjectile() → Combat Config → Graphics Config → Collision System
  └─ Utility: CalculateHealingAmount() → ApplyUtilitySpell() → UtilitySpellResult → Game state changes

Tooltip Display:
SpellID → SpellDefinition → 
  ├─ Projectile: CalculateSpellDamage() → Display base + intellect bonus + total
  └─ Healing: CalculateHealingAmount() → Display base + personality bonus + total
```

**Centralized Functions:**
- `CalculateSpellDamage(spellType, intellect)` - Used by both combat and tooltips
- `CalculateHealingAmount(spellType, personality)` - Used by both combat and tooltips  
- `ApplyUtilitySpell(spellType, personality)` - Handles all utility spell effects
- `CreateProjectile(spellType, x, y, angle, intellect)` - Creates combat projectiles

The spell system integrates with:
- **Character System**: Magic schools, spell point costs, class restrictions
- **Combat System**: Centralized damage/healing calculations, projectile creation, collision detection  
- **Equipment System**: Spell items for F/H key casting
- **UI System**: Spellbook interface, spell selection, utility spell timers, accurate tooltips
- **Configuration System**: Data-driven spell behavior and appearance

# How to Add a New Spell

This guide shows how to add new spells to UgaTaima using the **pure YAML-based spell system**. No Go code changes are required!

## Overview

The spell system is completely YAML-driven with **zero hardcoded fallbacks**. All spell-related configuration is in the dedicated `assets/spells.yaml` file, including spell definitions, projectile physics, and visual appearance. However, **complex utility spells still require game logic changes** for effects like vision, buffs, and movement.

## Architecture

```
YAML-Driven Spell System:
SpellID (string) → assets/spells.yaml → SpellDefinition + Physics + Graphics → Dynamic Effects

✅ NO hardcoded constants
✅ NO fallback functions  
✅ NO enum types
✅ Single assets/spells.yaml file for ALL spell configuration
✅ Projectile spells: NO code changes needed
⚠️ Utility spells: Game logic changes required
```

## Spell Types and Code Requirements

| Spell Type | Code Changes Required | Examples |
|------------|----------------------|----------|
| **Projectile Spells** | ❌ None (Pure YAML) | Fireball, Ice Shard, Lightning |
| **Healing Spells** | ❌ None (Pure YAML) | Heal, Greater Heal |
| **Complex Utility** | ✅ Game Logic Needed | Torch Light, Bless, Wizard Eye |

## Step 1: Add Your Spell to assets/spells.yaml

All spell configuration is now in a single `assets/spells.yaml` file. Add your spell under the `spells:` section:

### For Projectile Spells (Complete Example)

```yaml
spells:
  ice_shard:
    # Basic spell properties
    name: "Ice Shard"
    description: "Launches a sharp shard of ice at enemies"
    school: "water"
    level: 2
    spell_points_cost: 6
    duration: 0                # 0 for instant spells
    damage: 15
    range: 10                  # Range in tiles
    projectile_speed: 1.2      # Speed multiplier
    projectile_size: 12
    lifetime: 80               # Projectile lifetime in frames
    is_projectile: true
    is_utility: false
    visual_effect: "ice_shard"
    
    # Projectile physics (required for projectile spells)
    physics:
      speed: 10.0              # Base projectile speed (pixels/frame)
      lifetime: 80             # Lifetime in frames
      hit_radius: 200          # Hit detection radius (pixels)
      collision_size: 12       # Collision box size (pixels)
    
    # Visual appearance (required for projectile spells)
    graphics:
      max_size: 50
      min_size: 3
      base_size: 12            # Should match collision_size
      color: [100, 200, 255]   # Light blue RGB
```

### For Utility Spells (Healing Example)

```yaml
spells:
  greater_heal:
    # Basic spell properties
    name: "Greater Heal"
    description: "Powerful healing magic"
    school: "body"
    level: 3
    spell_points_cost: 8
    duration: 0
    damage: 0
    range: 0
    is_projectile: false
    is_utility: true
    visual_effect: "greater_heal"
    
    # Effect properties (no physics/graphics needed)
    heal_amount: 35
    target_self: true
    message: "Powerful healing energy flows through you!"
```

## Step 4: Utility Spells - Add Effect Properties + Game Logic

⚠️ **Important**: While spell definitions are YAML-driven, **utility spells still require game logic changes** for complex effects like vision, buffs, and movement abilities.

### Simple Utility Spells (Healing Only)
Basic healing spells work purely from YAML configuration:

```yaml
spells:
  definitions:
    greater_heal:
      name: "Greater Heal"
      description: "Powerful healing magic"
      school: "body"
      level: 3
      spell_points_cost: 8
      duration: 0
      damage: 0
      range: 0
      is_projectile: false
      is_utility: true
      visual_effect: "greater_heal"
      # Effect configuration - read directly by the system
      heal_amount: 35          # Base healing amount
      target_self: true        # Whether spell targets self
      message: "Powerful healing energy flows through you!"
```

### Complex Utility Spells (Require Code Changes)
For spells with game state effects like vision, buffs, or movement, you need to add game logic.

#### Example 1: Torch Light (Vision Enhancement)

**1. YAML Configuration in assets/spells.yaml:**
```yaml
spells:
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
    vision_bonus: 50.0
    message: "A magical light illuminates the area!"
```

**2. Game Logic (Required Code Changes):**

Add to `internal/game/combat.go` in the utility spell effects switch:

```go
// Apply utility spell effects dynamically based on spell ID
switch string(spellID) {
case "torch_light":
    cs.game.torchLightActive = true
    cs.game.torchLightDuration = result.Duration
    cs.game.torchLightRadius = 4.0 // 4-tile radius
// ... other cases
}
```

Add game state variables to `internal/game/game.go`:

```go
type Game struct {
    // ... existing fields ...
    
    // Torch Light spell state
    torchLightActive   bool
    torchLightDuration int
    torchLightRadius   float64
}
```

Add rendering logic to `internal/game/renderer.go`:

```go
func (r *Renderer) calculateLightRadius() float64 {
    baseRadius := 3.0
    
    // Apply torch light bonus
    if r.game.torchLightActive && r.game.torchLightDuration > 0 {
        baseRadius += r.game.torchLightRadius
    }
    
    return baseRadius
}
```

Add UI timer display to `internal/game/ui.go`:

```go
// Torch Light effect
if ui.game.torchLightActive && ui.game.torchLightDuration > 0 {
    ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "🔥", ui.game.torchLightDuration, 18000)
    currentX += iconSpacing
    hasActiveSpells = true
}
```

#### Example 2: Bless (Stat Buff)

**1. YAML Configuration in assets/spells.yaml:**
```yaml
spells:
  bless:
    name: "Bless"
    description: "Blesses the party with divine protection (+20 to all stats)"
    school: "spirit"
    level: 2
    spell_points_cost: 6
    duration: 300              # 5 minutes
    damage: 0
    range: 0
    stat_bonus: 20             # +20 to all stats
    is_projectile: false
    is_utility: true
    visual_effect: "bless"
    message: "The party feels blessed! (+20 to all stats)"
```

**2. Game Logic (Required Code Changes):**

Add to `internal/game/combat.go` in the utility spell effects switch:

```go
case "bless":
    cs.applyBlessEffect(result.Duration, result.StatBonus)
```

Add helper method to `internal/game/combat.go`:

```go
// applyBlessEffect applies the bless spell effect to the party
func (cs *CombatSystem) applyBlessEffect(duration int, statBonus int) {
    cs.game.blessActive = true
    cs.game.blessDuration = duration
    cs.game.statBonus = statBonus
    
    // Apply to all characters immediately
    for i := range cs.game.party.Characters {
        char := &cs.game.party.Characters[i]
        char.TempStatBonus = statBonus
    }
}
```

Add game state variables to `internal/game/game.go`:

```go
type Game struct {
    // ... existing fields ...
    
    // Bless spell state
    blessActive   bool
    blessDuration int
    statBonus     int
}
```

Add stat calculation integration to character system:

```go
// In internal/character/character.go GetEffectiveStats method:
func (c *MMCharacter) GetEffectiveStats(globalStatBonus int) (int, int, int, int, int, int, int) {
    might := c.Might + globalStatBonus + c.TempStatBonus
    intellect := c.Intellect + globalStatBonus + c.TempStatBonus
    // ... etc for all stats
    return might, intellect, personality, endurance, accuracy, speed, luck
}
```

Add UI timer display to `internal/game/ui.go`:

```go
// Bless effect
if ui.game.blessActive && ui.game.blessDuration > 0 {
    ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "✨", ui.game.blessDuration, 18000)
    currentX += iconSpacing
    hasActiveSpells = true
}
```

### Available Effect Properties

Properties read directly from YAML (no code changes needed):
- **`heal_amount`**: Healing power for healing spells
- **`target_self`**: Whether spell affects caster (true) or others (false)
- **`message`**: Message displayed when spell is cast

Properties that require game logic implementation:
- **`vision_bonus`**: Vision enhancement (requires renderer changes)
- **`stat_bonus`**: Stat bonus amount (requires character system integration)
- **`awaken`**: Awakening effect (requires status effect system)
- **`water_walk`**: Water walking (requires movement system changes)
- **`duration`**: Effect duration (requires game state management)

## Step 5: Give Spells to Character Classes (Optional)

To give starting spells to character classes, edit the character creation:

```go
// In internal/character/character.go, setupSorcerer() function:
c.MagicSchools[MagicWater] = &MagicSkill{
    Level:   1,
    Mastery: MasteryNovice,
    KnownSpells: []spells.SpellID{
        spells.SpellID("ice_shard"), // Add your new spell
    },
}
```

## Spell Types and Examples

### 1. Combat Projectile Spell

**Example**: Frost Bolt (Complete in assets/spells.yaml)

```yaml
spells:
  frost_bolt:
    # Basic spell properties
    name: "Frost Bolt"
    description: "A bolt of freezing energy"
    school: "water"
    level: 2
    spell_points_cost: 5
    duration: 0
    damage: 10
    range: 8
    projectile_speed: 1.3
    projectile_size: 10
    lifetime: 70
    is_projectile: true
    is_utility: false
    visual_effect: "frost_bolt"
    
    # Projectile physics
    physics:
      speed: 9.0
      lifetime: 70
      hit_radius: 180
      collision_size: 10
    
    # Visual appearance
    graphics:
      max_size: 42
      min_size: 3
      base_size: 10
      color: [150, 220, 255]  # Light blue
```

### 2. Healing Spell

**Example**: Greater Heal (in assets/spells.yaml)

```yaml
spells:
  greater_heal:
    name: "Greater Heal"
    description: "Powerful healing magic"
    school: "body"
    level: 3
    spell_points_cost: 8
    duration: 0
    damage: 0
    range: 0
    is_projectile: false
    is_utility: true
    visual_effect: "greater_heal"
    # Effect properties (no physics/graphics needed)
    heal_amount: 35
    target_self: true
    message: "Powerful healing energy flows through you!"
```

### 3. Buff Spell

**Example**: Divine Protection (in assets/spells.yaml)

```yaml
spells:
  divine_protection:
    name: "Divine Protection"
    description: "Grants divine protection to the party"
    school: "spirit"
    level: 4
    spell_points_cost: 12
    duration: 600            # 10 minutes
    damage: 0
    range: 0
    is_projectile: false
    is_utility: true
    visual_effect: "divine_protection"
    # Effect properties (no physics/graphics needed)
    stat_bonus: 25           # +25 to all stats
    message: "Divine protection surrounds the party!"
```

### 4. Vision Spell

**Example**: Eagle Eye (in assets/spells.yaml)

```yaml
spells:
  eagle_eye:
    name: "Eagle Eye"
    description: "Enhances vision beyond normal limits"
    school: "air"
    level: 3
    spell_points_cost: 6
    duration: 300            # 5 minutes
    damage: 0
    range: 0
    is_projectile: false
    is_utility: true
    visual_effect: "eagle_eye"
    # Effect properties (no physics/graphics needed)
    vision_bonus: 150.0      # 150% vision bonus
    message: "Your vision becomes supernaturally sharp!"
```

## Magic Schools

Available schools and their typical uses:

- **fire**: Offensive projectiles, light spells
- **water**: Ice/water magic, some healing, movement
- **air**: Lightning, vision spells, wind magic
- **earth**: Earth-based attacks, defensive spells
- **body**: Healing spells, physical enhancement
- **mind**: Mental effects, awakening, sleep
- **spirit**: Divine/holy magic, party buffs
- **light**: Advanced light magic (restricted)
- **dark**: Advanced dark magic (restricted)

## Testing Your Spell

1. **Build and run**: `go run .`
2. **Create a character** with access to your spell's school
3. **Open spellbook** with `M` key
4. **Navigate** to your spell's school
5. **Select and cast** your new spell

## Advanced: Complete Example

Here's a complete water walking spell in the new assets/spells.yaml format:

```yaml
spells:
  water_stride:
    name: "Water Stride"
    description: "Allows walking on water surfaces with grace"
    school: "water"
    level: 2
    spell_points_cost: 4
    duration: 180            # 3 minutes
    damage: 0
    range: 0
    is_projectile: false
    is_utility: true
    visual_effect: "water_stride"
    # Effect properties (no physics/graphics needed)
    water_walk: true
    message: "Your feet gain the ability to walk on water!"
```

**That's it!** All configuration is in one place. No need to edit multiple files.

## Error Handling

The system provides clear error messages for common issues:

- **Missing spell**: `spell 'unknown_spell' not found in assets/spells.yaml`
- **Uninitialized system**: `spell system not initialized - call SetGlobalConfig first`
- **Invalid configuration**: YAML parsing errors are reported at startup

## Key Differences from Old System

### ✅ What's New and Better

- **Single assets/spells.yaml file** - all spell configuration in one place
- **Pure YAML configuration** - no hardcoded constants
- **Dynamic spell loading** - add spells without code changes
- **Unified configuration** - definitions, physics, and graphics together
- **Declarative effects** - spell behavior defined in YAML
- **Error handling** - clear messages for missing spells
- **Type safety** - SpellID type prevents string errors

### ❌ What's Removed

- **Hardcoded spell constants** - no more `SpellIDFireball`
- **Fallback functions** - no more `getHardcodedFallback()`
- **SpellType enum** - replaced with dynamic SpellID strings
- **Switch statements** - replaced with YAML-driven logic

## Tips for Success

1. **Start simple** - begin with basic projectile or healing spells
2. **Copy existing spells** as templates for consistency
3. **Test immediately** after adding each spell
4. **Use clear naming** - spell IDs should be descriptive
5. **Balance carefully** - consider spell points cost vs. power
6. **Match school themes** - fire=damage, body=healing, etc.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Spell not appearing | Check YAML syntax and indentation |
| Spell fails to cast | Verify all required fields are present |
| Graphics not working | Ensure graphics config matches spell ID exactly |
| Build errors | Check YAML syntax - invalid YAML breaks builds |
| Effects not working | Verify effect properties are set correctly |

The spell system is now completely data-driven with everything in `assets/spells.yaml` - enjoy creating new spells without touching Go code!
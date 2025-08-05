# How to Add a New Weapon to UgaTaima

This manual provides a step-by-step guide for adding new weapons to the UgaTaima RPG game.

## System Overview

The weapon system consists of:
- **Weapon Definitions**: Core weapon properties (damage, range, category)
- **Class Restrictions**: Which classes can equip which weapon categories
- **Combat Configuration**: Category-based attack behavior (speed, lifetime, collision)
- **Graphics Configuration**: Category-based visual rendering (size, color)

## Quick Reference: Weapon Categories

| Category | Classes | Examples | Behavior |
|----------|---------|----------|----------|
| `sword` | Knight, Paladin | Iron Sword, Silver Sword | Medium speed, balanced |
| `dagger` | Archer, Sorcerer, Druid | Magic Dagger | Fast, small collision |
| `axe` | Knight | Steel Axe | Slow, large collision |
| `mace` | Knight, Paladin, Cleric | Holy Mace | Medium speed, medium collision |
| `spear` | Knight, Paladin, Druid | Iron Spear | Fast, long reach |
| `staff` | Cleric, Sorcerer, Druid | Oak Staff, Battle Staff | Medium speed, magical |
| `bow` | Archer | Hunting Bow, Elven Bow | Ranged projectiles |

## Step-by-Step Implementation

### 1. Define the Weapon Type

**File**: `internal/items/items.go` (around line 50)

```go
const (
    WeaponTypeIronSword WeaponType = iota
    WeaponTypeMagicDagger
    // ... existing weapons ...
    WeaponTypeMithrilSword  // Add your new weapon here
)
```

### 2. Create the Weapon Definition

**File**: `internal/items/items.go` (around line 137)

```go
WeaponTypeMithrilSword: {
    Type:        WeaponTypeMithrilSword,
    Name:        "Mithril Sword",
    Description: "A legendary sword forged from pure mithril",
    Category:    "sword",        // Determines combat/graphics config used
    Damage:      15,             // Base damage
    Range:       2,              // Range in tiles (>3 = ranged weapon)
    BonusStat:   "Might",        // Stat for damage bonus (Might/3)
    HitBonus:    12,             // Currently unused
    CritChance:  20,             // Currently unused
    Rarity:      "legendary",    // Currently unused
},
```

**Key Field**: `Category` determines which config section is used:
- **Combat**: `combat.sword` for attack behavior
- **Graphics**: `graphics.projectiles.sword` for visual rendering

### 3. Add Name Mapping

**File**: `internal/items/items.go` (around line 291)

```go
nameMap := map[string]WeaponType{
    "Iron Sword":    WeaponTypeIronSword,
    // ... existing weapons ...
    "Mithril Sword": WeaponTypeMithrilSword,  // Add this line
}
```

### 4. Configure Class Restrictions (if needed)

**File**: `internal/character/character.go` (around line 470)

Only needed if creating a new category. For existing categories, class restrictions already exist:

```go
switch c.Class {
case ClassKnight:
    return category == "sword" || category == "axe" || category == "mace" || category == "spear"
case ClassPaladin:
    return category == "sword" || category == "mace" || category == "spear"
case ClassArcher:
    return category == "bow" || category == "dagger"
case ClassCleric:
    return category == "mace" || category == "staff"
case ClassSorcerer:
    return category == "staff" || category == "dagger"
case ClassDruid:
    return category == "staff" || category == "spear" || category == "dagger"
}
```

## Configuration System

All weapons in the same category share the same combat and visual behavior.

### Combat Configuration

**File**: `config.yaml`

```yaml
combat:
  sword:     { speed: 10.0, lifetime: 15, collision_size: 24 }
  dagger:    { speed: 15.0, lifetime: 10, collision_size: 16 }
  axe:       { speed: 8.0,  lifetime: 20, collision_size: 32 }
  mace:      { speed: 9.0,  lifetime: 18, collision_size: 26 }
  spear:     { speed: 12.0, lifetime: 12, collision_size: 20 }
  staff:     { speed: 11.0, lifetime: 14, collision_size: 22 }
  bow:       { speed: 9.6,  lifetime: 54, collision_size: 12 }
```

- **speed**: Currently unused
- **lifetime**: Attack duration in frames ✅ USED
- **collision_size**: Hit detection size in pixels ✅ USED

### Graphics Configuration

**File**: `config.yaml`

```yaml
graphics:
  projectiles:
    sword:     { max_size: 48, min_size: 3, base_size: 24, color: [200,200,220] }
    dagger:    { max_size: 32, min_size: 2, base_size: 16, color: [150,150,180] }
    axe:       { max_size: 64, min_size: 4, base_size: 32, color: [180,120,80] }
    mace:      { max_size: 52, min_size: 3, base_size: 26, color: [160,160,100] }
    spear:     { max_size: 56, min_size: 3, base_size: 20, color: [140,100,60] }
    staff:     { max_size: 44, min_size: 3, base_size: 22, color: [100,80,150] }
    bow:       { max_size: 36, min_size: 2, base_size: 12, color: [180,140,100] }
```

- **max_size/min_size**: Size limits for distance scaling ✅ USED
- **base_size**: Base size for distance calculation ✅ USED  
- **color**: RGB color values ✅ USED

## Advanced: Custom Weapon with Unique Projectile

To create a weapon with completely custom behavior (not shared with other weapons), you need to create a **new category**.

### Example: Magic Wand with Custom Projectile

**1. Create New Category in Weapon Definition:**

```go
WeaponTypeMagicWand: {
    Type:        WeaponTypeMagicWand,
    Name:        "Magic Wand",
    Description: "A wand that shoots magic missiles",
    Category:    "wand",           // NEW custom category
    Damage:      8,
    Range:       6,                // >3 = ranged weapon
    BonusStat:   "Intellect",
    // ... other fields
},
```

**2. Add Class Restrictions for New Category:**

**File**: `internal/character/character.go` (around line 470)

```go
case ClassSorcerer:
    return category == "staff" || category == "dagger" || category == "wand"  // Add wand
case ClassCleric:
    return category == "mace" || category == "staff" || category == "wand"     // Add wand
```

**3. Add Custom Combat Configuration:**

**File**: `config.yaml`

```yaml
combat:
  # ... existing categories ...
  wand:       { speed: 14.0, lifetime: 72, collision_size: 14 }  # Custom behavior
```

**4. Add Custom Graphics Configuration:**

**File**: `config.yaml`

```yaml
graphics:
  projectiles:
    # ... existing categories ...
    wand:       { max_size: 40, min_size: 2, base_size: 14, color: [255,0,255] }  # Pink projectiles
```

**5. Update Configuration Code (if needed):**

The system already supports new categories automatically through the `GetWeaponConfig()` and `GetWeaponGraphicsConfig()` methods. However, if the new category isn't found, it will default to "sword". To add explicit support:

**File**: `internal/config/config.go` (around line 452)

```go
func (c *Config) GetWeaponConfig(weaponCategory string) *MeleeWeaponConfig {
    switch weaponCategory {
    case "sword":
        return &c.Combat.Sword
    case "dagger":
        return &c.Combat.Dagger
    // ... existing cases ...
    case "wand":
        return &c.Combat.Wand    // Add new case
    default:
        return &c.Combat.Sword   // Default fallback
    }
}
```

And add the `Wand` field to both config structs:

```go
type CombatConfig struct {
    // ... existing fields ...
    Wand    MeleeWeaponConfig `yaml:"wand"`
}

type ProjectilesConfig struct {
    // ... existing fields ...
    Wand    ProjectileRenderConfig `yaml:"wand"`
}
```

And update the graphics getter method:

```go
func (c *Config) GetWeaponGraphicsConfig(weaponCategory string) *ProjectileRenderConfig {
    switch weaponCategory {
    // ... existing cases ...
    case "wand":
        return &c.Graphics.Projectiles.Wand
    default:
        return &c.Graphics.Projectiles.Sword
    }
}
```

**Important**: No changes needed to `renderer.go` - it automatically uses your new category's config!

**Result**: Your Magic Wand will have completely unique projectile behavior, speed, lifetime, collision size, color, and visual appearance - totally separate from all other weapons!

## Optional: Add to Starting Equipment

**File**: `internal/character/character.go` (around line 170)

```go
func (c *MMCharacter) setupKnight(cfg *config.Config) {
    // ... stat setup ...
    c.Equipment[items.SlotMainHand] = items.CreateWeaponFromDefinition(items.WeaponTypeMithrilSword)
}
```

## Testing Checklist

- [ ] Weapon appears in `WeaponType` enum
- [ ] Weapon definition exists in `GetWeaponDefinition` map
- [ ] Name mapping exists in `GetWeaponTypeByName` map
- [ ] Appropriate classes can equip the weapon
- [ ] Weapon uses correct category-based combat behavior
- [ ] Weapon uses correct category-based visual appearance

## Important Notes

### Category-Based System
- **All weapons in the same category behave identically** (except base damage)
- Iron Sword, Silver Sword, Gold Sword all use "sword" config
- Only base damage differs between individual weapons

### Damage Calculation
```
Total Damage = Base Damage + (BonusStat ÷ 3)
```

### Range Behavior
- **Range ≤ 3**: Melee weapon (instant hit at target location)  
- **Range > 3**: Ranged weapon (creates arrow projectile)

### Fallback Safety
- Unknown weapon names default to `WeaponTypeIronSword`
- Unknown categories default to "sword" config

## Architecture

```
Weapon Attack Flow:
WeaponName → WeaponType → WeaponDefinition → Category → Combat Config
                                                    → Graphics Config
```

The system is fully data-driven through config.yaml with zero hardcoded combat values.
# How to Add a New Weapon to UgaTaima

This guide shows how to add new weapons using the **pure YAML-based weapon system**. No Go code changes are required!

## Overview

The weapon system is completely YAML-driven. All weapon configuration is in `assets/weapons.yaml`, including definitions, physics, graphics, and melee configurations.

## Architecture

```
YAML-Driven Weapon System:
weapon_key (string) → assets/weapons.yaml → WeaponDefinition + Physics + Graphics + Melee → Combat

✅ NO Go code changes needed
✅ Single assets/weapons.yaml file for ALL weapon configuration
✅ Per-weapon physics and graphics (not shared by category)
```

## Quick Reference: Weapon Categories

| Category | Classes | Examples | Behavior |
|----------|---------|----------|----------|
| `sword` | Knight, Paladin | Iron Sword, Silver Sword | Melee, medium arc |
| `dagger` | Archer, Sorcerer, Druid | Magic Dagger | Melee, fast, narrow arc |
| `axe` | Knight | Steel Axe | Melee, slow, wide arc |
| `mace` | Knight, Paladin, Cleric | Holy Mace | Melee, medium |
| `spear` | Knight, Paladin, Druid | Iron Spear | Melee, thrust attack |
| `staff` | Cleric, Sorcerer, Druid | Oak Staff, Battle Staff | Ranged projectile |
| `bow` | Archer | Hunting Bow, Elven Bow | Ranged arrows |

## Step 1: Add Your Weapon to assets/weapons.yaml

All weapon configuration is in `assets/weapons.yaml`. Add your weapon under the `weapons:` section.

### Melee Weapon Example (Sword/Axe/Mace/Dagger/Spear)

```yaml
weapons:
  mithril_sword:
    # Basic weapon properties
    name: "Mithril Sword"
    description: "A legendary sword forged from pure mithril"
    category: "sword"
    damage: 15
    range: 1                    # 1-2 for melee weapons
    bonus_stat: "Might"         # Stat for damage bonus
    hit_bonus: 12
    crit_chance: 20
    rarity: "legendary"
    value: 800

    # Melee-specific configuration
    melee:
      arc_angle: 75             # Swing arc in degrees
      animation_frames: 10      # Attack duration in frames
      hit_delay: 3              # Frames before damage applies

    # Visual effects for slash animation
    graphics:
      slash_color: [200, 255, 255]  # Light blue mithril flash
      slash_width: 38
      slash_length: 54
```

### Ranged Weapon Example (Bow)

```yaml
weapons:
  longbow:
    # Basic weapon properties
    name: "Longbow"
    description: "A powerful longbow with extended range"
    category: "bow"
    damage: 9
    range: 12                   # Display range in tiles
    bonus_stat: "Accuracy"
    hit_bonus: 18
    crit_chance: 15
    rarity: "rare"
    value: 300

    # Projectile physics (tile-based units - required for ranged weapons)
    physics:
      speed_tiles: 12.0         # Speed in tiles per second
      range_tiles: 12.0         # Maximum range in tiles
      collision_size: 14        # Arrow collision box (pixels)

    # Visual appearance for arrows
    graphics:
      max_size: 40
      min_size: 2
      base_size: 14
      color: [160, 120, 80]     # Brown wood color
```

### Staff Weapon Example (Ranged Magic)

```yaml
weapons:
  crystal_staff:
    # Basic weapon properties
    name: "Crystal Staff"
    description: "A staff that channels arcane energy"
    category: "staff"
    damage: 8
    range: 3                    # Display range in tiles
    bonus_stat: "Intellect"
    hit_bonus: 12
    crit_chance: 10
    rarity: "rare"
    value: 250

    # Projectile physics (tile-based units)
    physics:
      speed_tiles: 14.0         # Speed in tiles per second
      range_tiles: 3.0          # Maximum range in tiles
      collision_size: 24        # Collision box size (pixels)

    # Visual appearance
    graphics:
      max_size: 48
      min_size: 3
      base_size: 24
      color: [150, 100, 200]    # Purple crystal
```

## Step 2: Add Name Mapping (Required)

Add your weapon to the name-to-key mapping in `internal/items/items.go`:

```go
// Around line 291 in GetWeaponKeyByName function
var nameToKeyMap = map[string]string{
    "Iron Sword":    "iron_sword",
    "Silver Sword":  "silver_sword",
    // ... existing weapons ...
    "Mithril Sword": "mithril_sword",  // Add this line
}
```

**This is the only Go code change needed!**

## Property Reference

### Basic Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | ✅ | Display name (must match name mapping) |
| `description` | string | ✅ | Item description |
| `category` | string | ✅ | Weapon category (sword/dagger/axe/mace/spear/staff/bow) |
| `damage` | int | ✅ | Base damage |
| `range` | int | ✅ | Attack range (1-2 = melee, >3 = ranged) |
| `bonus_stat` | string | ✅ | Stat for damage bonus (Might/Accuracy/Intellect) |
| `hit_bonus` | int | Optional | Bonus to hit chance |
| `crit_chance` | int | Optional | Critical hit percentage |
| `rarity` | string | Optional | common/uncommon/rare/legendary |
| `value` | int | Optional | Gold value |

### Melee Configuration (for melee weapons)

| Property | Type | Description |
|----------|------|-------------|
| `melee.arc_angle` | int | Swing arc in degrees (30-100) |
| `melee.animation_frames` | int | Attack duration in frames |
| `melee.hit_delay` | int | Frames before damage applies |

### Physics Configuration (for ranged weapons)

| Property | Type | Description |
|----------|------|-------------|
| `physics.speed_tiles` | float | Speed in tiles per second |
| `physics.range_tiles` | float | Maximum range in tiles |
| `physics.collision_size` | int | Collision box size (pixels) |

### Graphics Configuration

**For Melee Weapons:**

| Property | Type | Description |
|----------|------|-------------|
| `graphics.slash_color` | [R,G,B] | Slash effect color |
| `graphics.slash_width` | int | Slash visual width |
| `graphics.slash_length` | int | Slash visual length |

**For Ranged Weapons:**

| Property | Type | Description |
|----------|------|-------------|
| `graphics.max_size` | int | Maximum projectile size |
| `graphics.min_size` | int | Minimum projectile size |
| `graphics.base_size` | int | Base size for distance scaling |
| `graphics.color` | [R,G,B] | Projectile color |

## Class Restrictions

Weapons are restricted by category. Current class permissions:

| Class | Allowed Categories |
|-------|-------------------|
| Knight | sword, axe, mace, spear |
| Paladin | sword, mace, spear |
| Archer | bow, dagger |
| Cleric | mace, staff |
| Sorcerer | staff, dagger |
| Druid | staff, spear, dagger |

To add a new category, edit `internal/character/character.go` (CanEquipWeapon function).

## Damage Calculation

```
Total Damage = Base Damage + (BonusStat ÷ 3) + CritBonus
```

Where:
- **Base Damage**: From weapon definition
- **BonusStat**: Character's Might/Accuracy/Intellect divided by 3
- **CritBonus**: Extra damage on critical hits based on crit_chance

## Range Behavior

- **Range 1-2**: Melee weapon (instant arc attack)
- **Range 3+**: Ranged weapon (creates projectile)

## Testing Checklist

- [ ] Weapon definition exists in `assets/weapons.yaml`
- [ ] Name mapping exists in `GetWeaponKeyByName`
- [ ] For melee: `melee` section with arc_angle, animation_frames, hit_delay
- [ ] For ranged: `physics` section with speed_tiles, range_tiles, collision_size
- [ ] `graphics` section present (slash_* for melee, projectile settings for ranged)
- [ ] Category matches one of: sword, dagger, axe, mace, spear, staff, bow
- [ ] Build succeeds: `go build .`
- [ ] Weapon can be equipped by appropriate class
- [ ] Attack animation/projectile works correctly

## Advanced: Special Weapon Properties

Some weapons support additional properties:

```yaml
bow_of_hellfire:
    name: "Bow of Hellfire"
    description: "A cursed bow wreathed in dark flames"
    category: "bow"
    damage: 7
    range: 12
    bonus_stat: "Accuracy"
    bonus_stat_secondary: "Intellect"  # Secondary scaling stat
    damage_type: "dark"                # Element type
    max_projectiles: 2                 # Limit active arrows

    physics:
      speed_tiles: 4.0                 # Very slow (4 tiles per second)
      range_tiles: 12.0                # Long range (12 tiles)
      collision_size: 64               # Large collision

    graphics:
      color: [139, 0, 139]             # Dark magenta
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Weapon not found | Check name mapping in items.go matches exactly |
| No melee config panic | Add `melee:` section for melee weapons |
| No graphics config panic | Add `graphics:` section |
| Wrong projectile behavior | Check `physics:` values |
| Class can't equip | Verify category matches class restrictions |

## Architecture Flow

```
Weapon Creation:
weapon_key → assets/weapons.yaml → WeaponDefinition → Item

Combat Flow:
Weapon Item → GetWeaponDefinition(key) → Physics/Graphics/Melee Config → Attack

Rendering:
Attack → weaponDef.Graphics → Visual Effects
```

The weapon system is fully data-driven - add new weapons by editing `assets/weapons.yaml` and adding one name mapping line!

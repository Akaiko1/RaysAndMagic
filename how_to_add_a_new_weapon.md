# How to Add a New Weapon

Weapons are fully data-driven via `assets/weapons.yaml`.

## Overview
- Each YAML key is the weapon key used by loot tables and config lookups.
- Weapon display names are converted to keys by lowercasing and replacing spaces.
- Ranged vs melee is determined by `range` (tiles): `range > 3` is ranged.

## Step 1: Add the weapon to assets/weapons.yaml

### Melee example
```yaml
weapons:
  mithril_sword:
    name: "Mithril Sword"
    description: "A legendary sword forged from pure mithril"
    category: "sword"
    damage: 15
    range: 2
    bonus_stat: "Might"
    hit_bonus: 12
    crit_chance: 20
    rarity: "legendary"
    value: 800

    melee:
      arc_angle: 75
      animation_frames: 10
      hit_delay: 3

    graphics:
      slash_color: [200, 255, 255]
      slash_width: 38
      slash_length: 54
```

### Ranged example (bow)
```yaml
weapons:
  longbow:
    name: "Longbow"
    description: "A powerful longbow with extended range"
    category: "bow"
    damage: 9
    range: 12
    bonus_stat: "Accuracy"
    hit_bonus: 18
    crit_chance: 15
    rarity: "rare"
    value: 300

    physics:
      speed_tiles: 12.0
      range_tiles: 12.0
      collision_size_tiles: 0.5

    graphics:
      max_size: 40
      min_size: 2
      base_size: 14
      color: [160, 120, 80]
```

## Step 2: Make it obtainable
Common options:
- Add it to `assets/loots.yaml` under a monster key.
- Add it to a map or scripted reward (custom logic).

## Important fields
- `category`: used for class restrictions and mastery.
- `range`: in tiles; `> 3` is ranged.
- `melee`: required for melee weapons.
- `physics`: required for ranged weapons.
- `graphics`: required for visuals (slash or projectile).

## Optional fields
- `bonus_stat_secondary` (dual scaling)
- `damage_type` (e.g., "fire", "dark")
- `max_projectiles` (limit active arrows for this weapon)
- `bonus_vs` (map of monster name or key to damage multiplier)
- `disintegrate_chance`
- `hit_bonus`, `crit_chance`, `value`, `rarity`

## Class restrictions
Weapon categories are restricted by class in `internal/character/character.go`.
Update that file if you add a new category or want to change permissions.

## Testing checklist
- YAML loads without errors.
- Melee weapons have `melee` and `graphics`.
- Ranged weapons have `physics` and `graphics`.
- Weapon can be acquired and equipped.
- Attacks render and hit correctly.

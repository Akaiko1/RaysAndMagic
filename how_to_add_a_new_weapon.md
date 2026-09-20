# How to Add a New Weapon

Weapons use `assets/weapons.yaml`. Read [shared authoring rules](docs/content-authoring.md) first.

## Overview
- Each YAML key is the weapon key used by loot tables and config lookups.
- Display-name lookups use the authored catalog index. Keep names unique; do not infer a key from the name.
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
    crit_chance: 20
    rarity: "legendary"
    value: 800

    melee:
      arc_type: 3
      animation_frames: 10

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
- Add to `assets/loots.yaml` under a monster key.
- Add to merchant inventory (`assets/npcs.yaml`).
- Use an encounter reward chest for an explicit weapon reward; ordinary quest
  `item_pool` rewards accept item keys, not weapons. See [quests](docs/adding-quests.md).

## Important fields
- `category`: used for class restrictions and mastery.
- `range`: in tiles; `> 3` is ranged.
- `melee`: required for melee weapons.
- `physics`: required for ranged weapons.
- `graphics`: required for visuals (slash or projectile).

## Optional fields
- `bonus_stat_secondary`
- `damage_type`
- `max_projectiles`
- `bonus_vs` (monster name, key, or family/type to damage multiplier)
- `stun_chance` (0.0-1.0) + `stun_turns`
- `disintegrate_chance`
- `aoe_radius_tiles` (splash radius; hits all monsters within N tiles)
- `crit_chance`, `value`, `rarity`
- `cooldown_multiplier` (overrides the category's attack-speed multiplier, e.g. Bow of Hellfire 1.7)
- `spell_cooldown_multiplier` (scales the wielder's spell cooldowns, e.g. Archmage Staff 0.8)
- `projectile_school` (renders the projectile as that school's spell orb instead of an arrow)

For melee, `arc_type` is 1 (single target), 2 (front and flank), 3 (three
positions), or 4 (five positions). `arc_angle` and `hit_delay` are removed fields.
Ranged magic weapons with `projectile_school` must use the matching `damage_type`.

Additional supported mechanics include `volley`, `pierce_count`,
`ricochet_targets` with `ricochet_range_tiles`, `double_strike`, `true_damage`,
and paired status fields. Copy the complete mechanic from an existing weapon;
load-time validation checks its required pairs. `no_loot: true` excludes a weapon
from generated rarity pools, but does not forbid explicit authored drops or stock.

[Shared damage rules](docs/content-authoring.md#damage-and-hit-scope) explain
critical hits, Designate Target, true damage, splash, and primary-only riders.
Do not implement these independently for a new weapon.

## Art and presentation

Add `assets/sprites/interface/weapons/icon_weapon_<key>.png` using the
[shared icon contract](docs/content-authoring.md#assets-and-animation).
Effect text is derived through `WeaponDefinitionConfig.EffectLines` and
`CoreEffectLines`, then composed by the shared character card template. Check
both the game tooltip and editor card when adding a new mechanic.

## Class restrictions
Weapon access and mastery use the class/skill catalog in
`internal/character/catalog.go` and the character equipment rules. Verify the
intended class can equip the weapon; a YAML category does not grant a skill.

## Testing checklist
- Run the [shared validation checklist](docs/content-authoring.md#verification).
- Melee weapons have `melee` and `graphics`.
- Ranged weapons have `physics` and `graphics`.
- Weapon can be acquired and equipped.
- Attacks render and hit correctly.

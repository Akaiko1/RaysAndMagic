# How to Add a New Spell

All spells are defined in `assets/spells.yaml`. The YAML key is the SpellID used by combat, tooltips, and items.

## Overview
- Projectile spells are fully data-driven (YAML only).
- Utility spells are mostly data-driven, but some effects are wired by SpellID in code.
- Spell durations in YAML are in seconds and converted to frames at runtime.

## Step 1: Add the spell to assets/spells.yaml

### Projectile spell example
```yaml
spells:
  ice_shard:
    name: "Ice Shard"
    description: "Launches a shard of ice"
    school: "water"
    level: 2
    spell_points_cost: 6
    duration: 0
    damage: 15
    projectile_size: 12
    disintegrate_chance: 0.0
    is_projectile: true
    is_utility: false
    visual_effect: "ice_shard"

    physics:
      speed_tiles: 10.0
      range_tiles: 10.0
      collision_size_tiles: 0.5

    graphics:
      max_size: 50
      min_size: 3
      base_size: 12
      color: [100, 200, 255]
```

### Utility spell example
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
    heal_amount: 35
    is_projectile: false
    is_utility: true
    visual_effect: "greater_heal"
    target_self: true
    message: "Powerful healing energy flows through you!"
```

## Supported utility fields
- `heal_amount`
- `stat_bonus`
- `water_walk`
- `water_breathing`
- `vision_bonus` (see note below)
- `awaken` (flag exists but no effect yet)
- `message`
- `status_icon` (HUD icon for active utility spells)

### Vision bonus note
The `vision_bonus` value is read for any utility spell, but the gameplay
effect (which buff to activate — torch light radius vs wizard eye compass
range) is dispatched by SpellID in `internal/game/combat.go`. New vision
spells still require code there.

### Quick-heal note
The quick-heal (H key) targets only `heal` and `heal_other`. If you add new heal IDs and want H targeting, update `internal/game/input.go`.

## Step 2: Grant the spell to players
Choose one (or more):
- Add to class starting spells in `internal/character/character.go`.
- Add to `assets/level_up.yaml` as a level-up choice.
- Add to a spell trader in `assets/npcs.yaml`.

## Spell trader requirements
NPC spell requirements are enforced. The structure is `min_level` plus an
optional `schools` list. A character must already have the spell's magic
school (and the school's level must meet `min_level`) to learn from an NPC.

```yaml
requirements:
  min_level: 3
  schools:
    - school: "water"
      min_level: 1
```

## Testing checklist
- YAML loads without errors.
- Spell appears in spellbook or NPC trader list.
- Casting works and shows expected effects.
- Tooltips show the right values.

# How to Add a New Spell

All spells are defined in `assets/spells.yaml`. The YAML key is the SpellID used by combat, tooltips, and items.

## Overview
- Projectile spells are fully data-driven (YAML only).
- Utility spells are mostly data-driven, but some effects are wired by SpellID in code.
- Spell durations in YAML are **seconds** and are converted to frames at runtime.

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
These fields are read from YAML and applied by the utility spell system:
- `heal_amount` (healing spells)
- `stat_bonus` (buff spells like Bless)
- `water_walk` (true/false)
- `water_breathing` (true/false)
- `vision_bonus` (see note below)
- `awaken` (flag exists but no effect yet)
- `message` (combat log)
- `status_icon` (HUD icon for active utility spells)

### Vision bonus note
`vision_bonus` only affects gameplay for the built-in IDs `torch_light` and `wizard_eye`. If you add a new vision spell, you must add it to the vision switch in `internal/game/combat.go`.

### Quick-heal note
The quick-heal (H key) targets only spell IDs `heal` and `heal_other`. If you add new healing spell IDs and want H targeting, update the checks in `internal/game/input.go`.

## Step 2: Grant the spell to players
Choose one (or more):
- Add to class starting spells in `internal/character/character.go`.
- Add to `assets/level_up.yaml` as a level-up choice.
- Add to an NPC spell trader in `assets/npcs.yaml` (see NPC guide).

## Common fields (reference)
- `name`, `description`, `school`, `level`
- `spell_points_cost`, `duration`, `damage`
- `is_projectile`, `is_utility`, `visual_effect`
- `projectile_size`, `disintegrate_chance`

## Testing checklist
- YAML loads without errors.
- Spell appears in spellbook or NPC trader list.
- Casting works and shows expected effects.
- Tooltips show the right values.

## Known limitations
- `awaken` is defined but not implemented.
- Vision effects are wired by specific SpellID names.

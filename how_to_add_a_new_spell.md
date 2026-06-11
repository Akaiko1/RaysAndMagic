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
    spell_points_cost: 6   # damage derives from this: cost × 3 (SpellDamagePerSP)
    duration: 0
    projectile_size: 12
    is_projectile: true
    is_utility: false

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
    heal_amount: 35
    is_projectile: false
    is_utility: true
    target_self: true
    message: "Powerful healing energy flows through you!"
```

## Damage model

There is no `damage` field. A projectile spell's base damage is
`spell_points_cost × 3` (`spells.SpellDamagePerSP`), optionally scaled by
`damage_cost_multiplier` and boosted by Intellect (plus Personality when
`scales_with_personality` is set). `deals_no_damage: true` makes a projectile
purely a status carrier.

## Supported projectile fields

- `projectile_size`, `cooldown_seconds`
- `aoe_radius_tiles` (splash radius; 0 = single-target, >0 splashes all monsters within N tiles)
- damage tuning: `damage_cost_multiplier`, `scales_with_personality`, `deals_no_damage`
- riders: `stun_chance` + `stun_duration_seconds`/`stun_duration_turns` (Lightning, Psychic Shock), `disintegrate_chance` (instakill roll), `bind_undead` + `bind_duration_seconds`, `pacify` + `pacify_duration_seconds` (Charm), `starburst_fx`
- `physics` (`speed_tiles`, `range_tiles`, `collision_size_tiles`), `graphics`

## Supported utility fields

- healing: `heal_amount`, `heal_party` (Mass Heal), `revive` + `full_heal` (Resurrect), `revive_hp_pct` (Raise Dead)
- party buffs: `stat_bonus`, `resist_buff_pct`, `outgoing_damage_bonus`, `incoming_damage_reduction`, `duration`
- AoE: `stun_radius_tiles` + `stun_duration_seconds`/`stun_duration_turns` (Stun/Darkness), `party_aoe_radius_tiles` (Inferno nova), `zone_radius_tiles` + `zone_tick_damage` + `zone_tick_seconds` (Hot Steam)
- world: `water_walk`, `water_breathing`, `vision_bonus` (see note below), `awaken`
- presentation: `message`, `status_icon` (HUD icon for active utility spells)
- unknown fields are silently ignored by the YAML loader — typos won't error, they just do nothing

### Vision bonus note
The `vision_bonus` value is read for any utility spell, but the gameplay
effect (which buff to activate — torch light radius vs wizard eye compass
range) is dispatched by SpellID in `internal/game/combat.go`. New vision
spells still require code there.

### Quick-heal note
The quick-heal key (C, or legacy H) is data-driven: any spell with
`heal_amount > 0` or `heal_party: true` qualifies automatically
(`SpellDefinition.IsHeal`); the best known one is picked. No code changes needed.

## Step 2: Grant the spell to players
Choose one (or more):
- Add to class starting spells in `internal/character/character.go`.
- Add to `assets/level_up.yaml` as a level-up choice.
- Add to a spell trader in `assets/npcs.yaml`.

## Spell trader requirements
A trader catalog entry only needs `cost: N` — name, school, level, description
and requirements are backfilled from spells.yaml at load
(`backfillTraderSpells`; a missing `cost` fails the load). When `requirements`
is omitted it defaults to the spell's own `level` as `min_level`. Author an
explicit block only to override that:

```yaml
requirements:
  min_level: 3
  schools:
    - school: "water"
      min_level: 1
```

A character must already have the spell's school open (and meet `min_level`)
to learn from an NPC.

## Testing checklist
- YAML loads without errors.
- Spell appears in spellbook or NPC trader list.
- Casting works and shows expected effects.
- Tooltips show the right values.

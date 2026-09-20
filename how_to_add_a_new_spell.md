# How to Add a New Spell

Spells live in `assets/spells.yaml`. The YAML key is the SpellID used by combat,
tooltips, and traders. Read [shared authoring rules](docs/content-authoring.md) first.

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
    spell_points_cost: 6   # damage derives from this: cost x 3 (SpellDamagePerSP)
    cooldown_seconds: 1.0  # required for every castable spell (forbidden on category: buff)
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
    spell_points_cost: 8
    cooldown_seconds: 1.5
    duration: 0
    heal_amount: 35
    is_projectile: false
    is_utility: true
    target_self: true
    message: "Powerful healing energy flows through you!"
```

## Damage model

There is no generic `damage` field. The default projectile basis is
`spell_points_cost * 3` (`SpellDamagePerSP`), modified by
`damage_cost_multiplier`, spell-school mastery and character stats. An authored
`damage_by_mastery: [N, E, M, G]` replaces the cost-derived ladder; it must contain
exactly four nonnegative, nondecreasing values. `mastery_damage_per_tier` is a
separate option for supported nova/map-wide spells. These are not interchangeable
with `zone_tick_damage`, which authors persistent-zone ticks.

`deals_no_damage: true` makes a projectile a status carrier. Use the runtime
formula and shared tooltip builders for the final damage, including true damage,
Strong Magic, criticals, and target defenses. See
[damage and hit scope](docs/content-authoring.md#damage-and-hit-scope).

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
- unknown fields are silently ignored by the YAML loader - typos won't error, they just do nothing

### Vision bonus note
The `vision_bonus` value is read for any utility spell, but the gameplay
effect (which buff to activate - torch light radius vs wizard eye compass
range) is dispatched by SpellID in the utility-casting runtime. New vision
spells require an explicit dispatch path.

### Quick-heal note
The quick-heal key (C, or legacy H) is data-driven: any spell with
`heal_amount > 0` or `heal_party: true` qualifies automatically
(`SpellDefinition.IsHeal`); the best known one is picked. No code changes needed.

## Other supported forms

Use a shipped definition with the same casting form as your starting point:
`stone_blossom` for mortar impact (`mortar_range_tiles`, positive projectile
speed), `inferno` for a party-centered nova, `earthquake` for map-wide damage,
`hot_steam` or `firewall` for persistent zones, `jump` for forward relocation,
and `summon_ice_elemental` for mastery-scaled allied summons. Inspect the exact
keys in [spells.yaml](assets/spells.yaml) before copying. Adding a field alone
cannot create a new casting form.

Castable spells require positive `cooldown_seconds`. `category: buff` spells
forbid that field and instead need a supported beneficial timed effect. A new
school must be supported by the canonical school catalog; spelling one in YAML
is not enough.

## Art

Add `assets/sprites/interface/spells/icon_spell_<id>.png`; timed effects may also need
a `status_icon`. Follow [shared asset rules](docs/content-authoring.md#assets-and-animation).

## Step 2: Grant the spell to players
Choose one (or more):
- Extend the character creation rules if the spell should be known at start.
- Add to `assets/level_up.yaml` as a level-up choice.
- Add to a spell trader in `assets/npcs.yaml`.

## Spell trader availability
A trader catalog entry only needs `cost: N` - name, school and description are
backfilled from spells.yaml at load (`backfillTraderSpells`; a missing `cost`
fails the load).

There are no spell-level or mastery requirements. A character only needs the
spell's matching magic school to be open before learning it from an NPC.

## Testing checklist
- Run the [shared validation checklist](docs/content-authoring.md#verification).
- Spell appears in spellbook or NPC trader list.
- Casting works and shows expected effects.
- Tooltips show the right values.

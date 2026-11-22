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
    cooldown_seconds: 1.0  # required except on category: buff
    projectile_size: 12
    is_projectile: true
    is_utility: false

    physics:
      speed_tiles: 10.0
      range_tiles: 10.0
      hitbox: standard     # standard | wide; sizes in config.yaml world.projectile_hitboxes

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

## Duration authoring

Author the exact lifetime in gameplay seconds at spell level:

```yaml
spells:
  bless:
    name: "Bless"
    school: spirit
    spell_points_cost: 15
    category: buff
    is_utility: true
    stat_bonus: 5
    duration_by_mastery: [300, 360, 420, 480]
```

The four entries are Novice, Expert, Master and Grandmaster. They must be positive
and nondecreasing. There is no automatic duration multiplier. Repeat the same
value four times for a fixed lifetime; omit the field for instant or charge-only
effects. The obsolete `duration` field and nested `mechanic.duration_by_mastery`
are rejected, including `duration: 0`.

Runtime casts, fields, projectile riders and tooltips read the same ladder via
`SpellDefinition.DurationSeconds`; caster-based callers resolve school mastery
through `character.SpellDurationSeconds`. Convert seconds to frames with configured
TPS and to TB rounds with the shared combat clock. Never add another mastery
multiplier or recompute a saved remaining timer from a full lifetime.

Haste authors its own duration, `power_by_mastery` (RT recovery %) and
`extra_actions_by_mastery` (party bonus actions per TB round); the Quickening
technique keeps separate values in `assets/techniques.yaml`. The two share only
the haste effect's eligibility check and no-stacking rule. Frozen Ground's main ladder
controls roots; `mechanic.water_freeze_seconds` separately controls how long its
water crossing lasts, without mastery scaling. Stun, Charm and Bind durations
(one seconds field each, whole 3 s rounds, the same span in RT and TB), reuse,
action recovery and visual animation timings are independent fields; they are
not spell-lifetime multipliers.

## Supported projectile fields

- `projectile_size`, `cooldown_seconds`
- `aoe_radius_tiles` (splash radius; 0 = single-target, >0 splashes all monsters within N tiles)
- damage tuning: `damage_cost_multiplier`, `scales_with_personality`, `deals_no_damage`
- riders: `stun_chance` + `stun_duration_seconds` (Lightning, Psychic Shock), `disintegrate_chance` (instakill roll), `bind_undead` + `bind_duration_seconds`, `pacify` + `pacify_duration_seconds` (Charm), `starburst_fx`
- `physics` (`speed_tiles`, `range_tiles`, `hitbox`), `graphics`. `hitbox` is `standard` (0.5 tile: bolts, arrows) or `wide` (1.0 tile: large shots such as fireball; speed is `speed_tiles` alone); area damage comes from `aoe_radius_tiles`, not the hitbox. A raw `collision_size_tiles` is rejected at load.

## Supported utility fields

- healing: `heal_amount`, `heal_party` (Mass Heal), `revive` + `full_heal` (Resurrect), `revive_hp_pct` (Raise Dead)
- party buffs: `stat_bonus`, `resist_buff_pct`, `outgoing_damage_bonus`, `incoming_damage_reduction`, `duration_by_mastery`
- AoE: `stun_radius_tiles` + `stun_duration_seconds` (Stun/Darkness), `party_aoe_radius_tiles` (Inferno nova), `zone_radius_tiles` + `zone_tick_damage` + `zone_tick_seconds` (Hot Steam)
- world: `water_walk`, `water_breathing`, `vision_bonus` (see note below), `awaken`
- presentation: `message`, `status_icon` (HUD icon for active utility spells)
- unknown fields are rejected by the strict YAML loader; fix typos before launch

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

Castable spells require positive `cooldown_seconds`. Every timed or charged
enchantment (a timed utility field or a personal/party mechanic) needs a
category, and only those may have one. `category: buff` (long preparation
effects such as Bless or Shield) forbids `cooldown_seconds` and has no RT
recovery; `category: combat_buff` (short or charge-spent effects such as
Celestial Armor, Haste or Preservation) keeps its required cooldown so it
cannot be refilled on demand. An instant spell may not author `duration_by_mastery`. A new
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

## Advanced mechanics and shared timing

The optional `mechanic:` block selects a registered effect, not a second spell
implementation. `internal/config/spell_mechanics.go` owns classification and
required fields. Add a new kind there, implement its effect through the existing
cast transaction, and reuse its ally/offensive/party classification in consumers.
The transaction owns mana, personal reuse and successful-cast accounting.

Mastery arrays have four entries (Novice, Expert, Master, Grandmaster):
`power_by_mastery`, `charges_by_mastery` and `projectiles_by_mastery`
live in the mechanic block; `duration_by_mastery` lives at spell level. Author
only fields used by that kind. Targeting uses
`radius_tiles`, `cone_degrees` (full cone angle), `max_targets` and
`max_hits_per_target`; execution uses `instant_kill_chance` in 0..1. Inspect a
shipped spell of the same kind before extending one. Required projectile kinds
must also set `is_projectile: true`.

`mechanic.summary` describes the player effect; substitutions are resolved by
`SummaryText`: `{duration}` reads the spell-level mastery ladder, and
`{water_freeze_seconds}` reads Frozen Ground's independent crossing lifetime.
Keep UI wording in YAML. Mechanics must not charge Strong Magic HP unless they
actually have a damage formula.

New spell controls use one combat round = 3 seconds. Preserve separately authored
weapon control conversions. `config.yaml`'s `control_saves` owns the resistance
ladder and participating mechanics: Turn Undead and fear share stun diminishing
returns; Feeblemind rolls resistance without DR. Feeblemind cannot refresh an
active silence; `reapply_immunity_seconds` begins after silence ends and persists
in saves.

`mechanic.autocast: true` lets the caster set the spell to cast itself; the
trigger belongs to the mechanic kind (haste, purify, dispel_magic and
celestial_armor have one). `autocast_interval_rounds` in the shared
technique configuration defines the personal automatic budget: automatic casts
pay mana but no action or RT recovery, and share that budget with automatic
class techniques. Manual casts retain normal action and recovery costs; a manual
TB Haste, Purify or Dispel Magic does not grant monsters the movement pass.

Optional top-level `reuse_seconds` is a separate personal spell lockout, started
only by a successful cast. It uses integer frames, advances by a combat round in
TB, survives saves and mode changes, and is clamped to current content on load.
It does not replace `cooldown_seconds` (hero action recovery). Spellbook and
inventory spell icons show the same clockwise shade as reusable devices.

## Additional damage and presentation fields

- `damage_base` supplies a fixed base before the ordinary stat/mastery scaling.
- `damage_source` borrows a named projectile's formula; `damage_divisor` must be
  positive alongside it. Sources cannot chain. Do not combine these alternatives
  with an incompatible mastery ladder or `deals_no_damage`.
- `graphics.target_cursor` selects supported targeting presentation; it does not bypass
  the shared reach, visibility or payment rules.
- `graphics.world_fx` selects a registered world effect. `cast_overlay` and
  `cast_overlay_scale` select and size the cast flourish. Sword overlays require
  an authored `cast_overlay_weapon`, validated against the model catalog.
- `graphics.target_flights` supplies visual-only flights for instant effects:
  `projectile`, `palette`, `size_scale`, optional `projectile_fx`,
  `return_to_party` and `duration_seconds`. Never apply a second damage event
  when these rays arrive.

Spell YAML decoding is strict. Unknown keys, missing required mechanic fields,
invalid references, and malformed control-save message placeholders fail loading.

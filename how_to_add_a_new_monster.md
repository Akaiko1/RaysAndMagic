# How to Add a New Monster

Monsters use `assets/monsters.yaml`. Read [shared authoring rules](docs/content-authoring.md) first.
Take level, HP, damage and experience from [Monster design](docs/monster-design.md).

## Overview
- Monsters live in `assets/monsters.yaml`.
- Loot tables are in `assets/loots.yaml`.
- Authored map placement uses a lowercase letter; scheduled or summoned creatures
  can omit `letter`.
- Radii in `monsters.yaml` are in tiles (1 tile = 64px).
- `size_class` sets the sprite size: one of `small`, `medium`, `person`, `large`, `huge`. The per-class height in tiles lives in `config.yaml` under `graphics.size_classes`. A raw `size_multiplier` is rejected at load.
- `size_class` also sets the collision body (movement and projectile hits) from `config.yaml` `world.monster_bodies`: small 24 px, medium 40 px, person/large/huge 48 px. Bodies are capped at 48 px so every monster fits 1-wide corridors. Raw `box_w`/`box_h` are rejected at load.
- Monsters render as thin slabs (standees). The slab side takes the art's average colour, where saturated pixels count more. If small bright accents on mostly grey art make the side look wrong (a coloured rim, thin spikes that look cut off), set `standee_core: silhouette_edge`: the side then takes the average colour of the silhouette's outline. Any other value is rejected at load.

## Step 1: Define the monster
Add a new entry under `monsters:` in `assets/monsters.yaml`.

Minimal example:
```yaml
monsters:
  ice_troll:
    name: "Ice Troll"
    level: 7
    max_hit_points: 160
    armor_class: 11
    experience: 190
    damage_min: 17
    damage_max: 25
    alert_radius: 3        # tiles
    attack_radius: 1       # tiles
    speed: 1.1
    animate_when_idle: false # optional; true loops the walking sheet at rest
    sprite: "goblin"       # assets/sprites/mobs/goblin.png
    biomes: [forest]
    letter: "v"            # lowercase, unique in its biome scope (see "Biome restriction")
    size_class: large       # small | medium | person | large | huge
    resistances: {}
```

Key requirements:
- When authored, `letter` must be lowercase (map spawns recognize a-z) and unique
  within its biome scope - see "Biome restriction" below.
- `sprite` must exist in `assets/sprites/mobs/` (without `.png`).
- `alert_radius` and `attack_radius` are in tiles.
- `speed` is the one movement speed (real time: pixels per 60 Hz frame).
  Turn-based combat turns it into steps per
  movement action, `speed / monster_ai.turn_step_speed` (2.0 in
  `config.yaml`). The rate can be fractional: the remainder carries over, so
  2.4 walks 1, 1, 2, 1, 1 and 3.0 walks 2, 1, 2, 1, 2. A walking monster
  moves at least one tile and at most 4 per action, and a walk stops where
  the monster can strike. `speed: 0` never moves. Wildlife and caravans
  stroll instead: they have no one-tile floor, so below 2.0 they idle some
  turns. The editor's Mobs page shows the rate and the first five rounds.

## Biome scope

Add `biomes: [water]` inside a monster definition to restrict its map-letter
lookup to that biome. A biome-specific definition wins over a universal one;
omitting `biomes` provides a universal fallback. Letters may be reused across
disjoint biome scopes. A letter with no matching definition produces no spawn.
Check the existing catalog before choosing a letter.

## Step 2: Add a sprite
Place the static PNG and directional animation sheets under
`assets/sprites/mobs/`, named from the `sprite` key. Walking and attacking use
`<sprite>_walking_l/r.png` and `<sprite>_attacking_l/r.png`. Follow the
[shared animation contract](docs/content-authoring.md#assets-and-animation).
Check the silhouette, baseline, directions, and size in the editor's Mobs tab
and at gameplay distance.

Set `animate_when_idle: true` to loop a living monster's walking sheet while it
rests. The flag defaults to `false` and is independent of `speed`: `speed: 0`
alone does not enable animation. Currently only `dragon_brood_mother` combines
`speed: 0` with `animate_when_idle: true`.

This works in real-time, turn-based play, and the editor's Mobs preview. Movement
keeps its existing cycle timing, and dedicated attack or special-motion sheets
retain priority. A missing attack sheet holds the first walking frame. Dead
monsters, inert encounter props, and fish do not play the resting loop.
Without the flag, waiting, rooting, or slowing a movable monster does not animate
it. The loop uses `graphics.monster.walk_frame_seconds`; no separate idle sheet
is required. The flag is reloaded from YAML on save restoration; the visual phase
is transient and is not saved.

## Step 3: Optional ranged attacks

Set `projectile_weapon` to a weapon key or `projectile_spell` to a spell key,
plus `ranged_attack_range` in tiles. Use a supported projectile definition and
check both combat modes; a display name is not a catalog key.

## Step 4: Optional flags and abilities
Plain flags stay top-level keys:
- `perfect_dodge`
- `flying`
- `passive_until_attacked` (won't aggro until struck)
- `attack_cooldown_multiplier`: the one attack speed (default 1; 0.5 attacks
  twice as often). Real time scales the attack cooldown by it. Turn-based
  combat turns it into swings per attack action, `1 / multiplier`, which can
  be fractional: the remainder carries over to the next action, so 0.7 (1.43)
  swings 1, 2, 1, 2, 1. Enrage's `cooldown_mult` multiplies the rate the same
  way. A monster always swings at least once per action, at most 8 times.
  There is no separate attacks-per-round field. Champions are the exception:
  they always swing twice per turn-based action. The editor's Mobs page shows
  the rate to two decimals and the first five rounds.

Every combat ability lives in one `abilities:` map, one entry per kind. A
kind you leave out is absent; there are no per-ability on/off flags.

```yaml
monsters:
  ice_troll:
    name: "Ice Troll"
    level: 7
    max_hit_points: 160
    armor_class: 11
    experience: 190
    damage_min: 17
    damage_max: 25
    alert_radius: 3
    attack_radius: 1
    speed: 1.1
    sprite: "goblin"
    biomes: [forest]
    letter: "v"
    size_class: large
    resistances: {}
    abilities:
      stun: {chance: 0.15, seconds: 3}
      melt_armor: {chance: 0.25}
```

| Kind | Fields | Effect |
|---|---|---|
| `telegraph` | `name`, `kind`, `school`, `shape`, `range`, `cooldown_seconds`, `cooldown_rounds`, `warning_seconds`, `warning_rounds`, `damage_percent`; optional `duration_seconds` (required for shield, slow, root and sustained fields: how long the field, shield or landed status lasts), `sustain_damage`, `below_hp_percent` | A marked-tile attack the party sees coming and can step out of. `kind`: strike, charge, slow, root, shield, push, retreat. `shape`: lane, burst, cone, target. `school`: fire, water, earth, air. See the solstice monsters. |
| `root_party` | `chance`, `seconds` | An attack roots the whole party. |
| `rear_blink` | `chance`, `range_tiles` | A ranged attacker blinks behind the party and strikes. Needs a ranged attack; `range_tiles` runs from 2 up to `ranged_attack_range`. |
| `pounce` | `range_tiles`, `cooldown_seconds` | Leaps to melee from range, e.g. the puma. |
| `phase_shift` | `range_tiles`, `cooldown_seconds` | A walker blinks up to `range_tiles` (2-8) straight along a row or column across chasms or water, onto the first walkable tile beyond them (it may pass over the party), toward the party, only when its pursuit finds no walking route to an attack position or that walk is more than `monster_ai.phase_shift.detour_factor` times the walk-and-blink plan (config.yaml; the cooldown still to run counts as walking; with no route it waits on the launch tile). Walls, doors and exits block it. Not for `flying: true`; `cooldown_seconds` is whole 3 s rounds. Planning it costs several pursuit repaths per replan: author it for small dungeon maps, not open-world packs. E.g. the solstice chamber mobs. |
| `poison` | `chance`, `duration_seconds` | A hit poisons the target. |
| `ignite` | `chance`, `duration_seconds` | A hit sets the target on fire. |
| `stun` | `chance`, `seconds` | A hit stuns the target. |
| `dispel` | `chance` | A hit strips a buff from the party. |
| `melt_armor` | `chance` | A melee or projectile hit melts the struck target's armor (heroes and party allies): for 5 TB rounds / 15 s, armor reduces physical damage by at most 33%. It refreshes and never stacks; breath, novas, telegraphs and traps never melt armor; Purify cleanses it. |
| `fireburst` | `chance`, `damage_min`, `damage_max` | A fire blast instead of the normal attack. |
| `dragon_breath` | `chance`, `damage_type` | The attack hits every living party member in that school. |
| `piercing_shot` | `chance`, `targets` | An armor-piercing shot at several party members. |
| `ally_heal` | `chance`, `amount`, `radius_tiles` | Heals itself or a nearby ally instead of attacking. |

A `chance` is in (0, 1]. Each kind's fields are validated at load: a kind
without its magnitude (poison without `duration_seconds`, stun without
`seconds`) fails startup. Every status duration is seconds of whole 3 s combat
rounds: it lasts those seconds in real time and seconds/3 turns in turn-based
play, and a slow lasts whole steps of 3 turns. What each status does to the
party and to monsters lives in `assets/statuses.yaml` (the editor's Actionables
page shows it). The loader is strict: an unknown kind or
field anywhere in a monster entry fails the load, including a flat key from the
old schema such as `poison_chance`.

The editor's Mobs page lists a monster's abilities under ABILITIES, after its
ATTACK lines, and can group the catalog by ability.

### Boss kit (all data-driven; see golden_thief_bug)
- `boss: true` - a disintegrate proc deals 100 true damage to a boss
  instead of the kill, whatever its type (undead and dragons stay immune).
- `type: "formless"` - a boss-only type (it fails the load without
  `boss: true`) that blocks Bind Undead and Charm. A boss does not have to
  be formless.
- `passive_until_quest` + `evade_radius_tiles` + `boss_cooldown_seconds` -
  evades (blinks away, never attacks) until the named quest completes

Six ability kinds need `boss: true`:

| Kind | Fields | Effect |
|---|---|---|
| `trap_volley` | `count`, `radius_tiles`, `interval_seconds`, `interval_turns`, `damage` | Sows fire traps around itself on a cadence; each new volley replaces the old field. |
| `inferno` | `chance`, `damage`, `range_tiles` | A party-wide fire nova within `range_tiles`. |
| `teleport` | `at_hp`, `chance` | At or below `at_hp` it may blink to a random tile. |
| `summon` | `monsters` and either `chance` (optional `first_guaranteed`) or `cooldown_seconds` (whole rounds); optional `count` (default 1), `max` (0 = no cap) | Rallies adds picked from `monsters` (monster keys) in place of its attack: on a roll at its action, or surely each time the timer runs out while below `max` (the first right as the fight starts; kiting cannot stall it). Adds spawn inside the boss's arena, at its auto-level; they give no XP but drop their own gold and loot (intended). |
| `push` | `cooldown_seconds` (whole rounds) | Once its cooldown is over, the boss's next landed attack (on anyone, dodged or not) also shoves the party 1 tile toward the nearest lane of its current health phase; free, the boss still attacks, then the cooldown restarts. Only for the boss of a chamber with `lanes` (map load fails otherwise). |
| `enrage` | `at_hp`, `damage_mult` and/or `cooldown_mult` | At or below `at_hp` it hits harder and/or faster. |

`at_hp` values are absolute HP; scale them with `max_hit_points`. An evasive
phase without its tuning fails startup like an ability without its magnitude.

### Adding a new ability kind
Add a field to `monster.Abilities` and a row to `abilityRows`
(`internal/monster/abilities.go`): the row carries validation, the editor and
Identify line, and the kind name the editor groups by. Then write its combat
consumer with a combat-log line, and add a row to `balanceAbilities`
(`internal/game/balance_coverage_test.go`) so the balance sweep can see it fire;
the sweep marks an ability without a row as UNTRACKED.

## Step 5: Add loot
Loot is controlled by `assets/loots.yaml` using the monster key.
Loot drops into a bag on the ground and must be picked up by the player.
The monster's gold is the table's `gold` entry: `min` to `max` coins, rolled
when it spawns and dropped in the same bag. A table has at most one, and it
takes no `chance`. An item entry may also add `min`/`max`: when it lands, that
many units drop (1-4 potions below; gear drops as separate pieces).

Example:
```yaml
loots:
  ice_troll:
    - {type: gold, min: 25, max: 80}
    - {type: item, key: health_potion, chance: 0.3, min: 1, max: 4}
    - type: "weapon"
      key: "elven_bow"
      chance: 0.05
    - type: "item"
      key: "iron_armor"
      chance: 0.10
```

## Step 6: Place in a map (optional)
On a forest-biome `.map`, place `v` (the example ice troll's letter) in the grid:
```
...v.....
```
Map placement chooses the spawn point; normal movement rules still apply afterward.

## Terrain movement overrides
`walkable_tile_overrides` lists normally blocked tile keys from `assets/tiles.yaml` that this monster may traverse. It grants movement permission; it does not choose spawn locations or preferred terrain. Omit it when no exception is needed. Do not list already walkable tiles or tiles covered by the monster's flight rules. For example, a ground monster that may cross dunes can list `desert_dune`.

Movement permission does not permit attacking from inside a blocking object.
Melee and ranged monsters must first leave walls, trees, rocks, and dunes, even
with `walkable_tile_overrides` or flight. AI attack positions use the same
height clearance as projectiles: open water and chasms are valid positions for
actors able to occupy them. A flying party can also attack and cast above these
floors, but cannot do so from inside blocking scenery. Combat line checks are
reciprocal; vision for detecting a target remains separate.

## Wildlife and tree movement

`disposition: wildlife` selects ambient behavior: flee threats and resume roaming
when safe. Ordinary hostile combat AI is not the wildlife state machine, though
both share movement/collision helpers. Optional `prey` (monster keys) and
`prey_radius` allow hunting other wildlife, as with the fennec. Use the shipped
lemurs and desert rabbit as complete definition examples.

Tree movement adds an `arboreal` block with `tree_tiles`, `height_tiles`,
`jump_range_tiles`, `climb_seconds`, `jump_seconds`, and `rest_seconds`.
`tree_tiles` names traversable tree targets; it is separate from ordinary ground
`walkable_tile_overrides`. Supply four extra directional animation pairs:
`climbing`, `perched`, `jumping`, and `descending`. Check transitions back to
solid ground, blocked routes, missing trees, and save/load while airborne.

This complete example reuses the shipped ring-tailed lemur art. It has no map
letter; register its key in an ecology population to spawn it.

```yaml
monsters:
  canopy_lemur:
    name: "Canopy Lemur"
    type: beast
    disposition: wildlife
    level: 1
    max_hit_points: 18
    armor_class: 0
    experience: 0
    damage_min: 0
    damage_max: 0
    alert_radius: 5
    attack_radius: 1
    speed: 2.1
    sprite: ring_tailed_lemur
    size_class: small
    biomes: [jungle]
    arboreal:
      tree_tiles: [jungle_vine_tree, jungle_palm]
      height_tiles: 1.15
      jump_range_tiles: 3
      climb_seconds: 1.4
      jump_seconds: 0.8
      rest_seconds: 4
```

### Population and respawn

Three mechanisms add monsters after a map loads. Pick one per intended group;
[monster spawns over time](docs/monster-spawns.md) compares them in full.

- `config.yaml day_night.packs`: the group exists only during its phase and is
  swapped at every dusk and dawn.
- `assets/ecology.yaml populations`: the group lives permanently; when its
  phase begins, only the missing count is added (wildlife, or `hostile: true`).
- `respawn_days` on a map: the authored roster returns on arrival after N days.

## Testing checklist
- Run the [shared validation checklist](docs/content-authoring.md#verification).
- `letter` is unique and lowercase.
- Static sprite and required animation sheets exist.
- Monster spawns and behaves correctly.
- Loot drops into a bag when killed.
- The [balance sweep](docs/monster-design.md#checking-a-monster) shows no new holes at its levels.

### Brief leaping fish encounters

Fish use `disposition: fish` and the `fish.species` map in
`assets/ecology.yaml`. They do not use map spawn letters, patrols, wildlife
population caps, or combat AI. Each configured region schedules one nearby
fish after a random real-time cooldown, including in turn-based mode; menus
and loading pause that clock. Eligible sources are tiles with `type: water`
and a cardinal water neighbor in the same region, including walkable streams.

Provide a four-frame `<sprite>_leaping_r.png` sheet (2x2 square cells, rising,
apex, descending, diving); the shared renderer supplies left-facing mirroring
and flight height. Timing, height, radius, and the rare open-bank landing chance
are data in the ecology configuration. A water landing silently removes the
fish. A bank landing uses its ordinary loot table without awarding kill credit;
a player kill uses normal combat and loot resolution. Fish themselves are
transient across saves, while their cooldown and dropped loot persist. Define
the scale item in `assets/items.yaml`, its icon, and the one-scale entry in
`assets/loots.yaml` together. See `internal/game/fish_test.go` for integration coverage.

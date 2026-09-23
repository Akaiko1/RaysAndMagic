# How to Add a New Monster

Monsters use `assets/monsters.yaml`. Read [shared authoring rules](docs/content-authoring.md) first.

## Overview
- Monsters live in `assets/monsters.yaml`.
- Loot tables are in `assets/loots.yaml`.
- Authored map placement uses a lowercase letter; scheduled or summoned creatures
  can omit `letter`.
- Radii in `monsters.yaml` are in tiles (1 tile = 64px).
- `size_class` sets the sprite size: one of `small`, `medium`, `person`, `large`, `huge`. The per-class height in tiles lives in `config.yaml` under `graphics.size_classes`. A raw `size_multiplier` is rejected at load.

## Step 1: Define the monster
Add a new entry under `monsters:` in `assets/monsters.yaml`.

Minimal example:
```yaml
monsters:
  ice_troll:
    name: "Ice Troll"
    level: 7
    max_hit_points: 95
    armor_class: 11
    experience: 350
    damage_min: 4
    damage_max: 20
    alert_radius: 3        # tiles
    attack_radius: 1       # tiles
    speed: 1.1
    animate_when_idle: false # optional; true loops the walking sheet at rest
    gold_min: 25
    gold_max: 80
    sprite: "goblin"       # assets/sprites/mobs/goblin.png
    biomes: [forest]
    letter: "v"            # lowercase, unique in its biome scope (see "Biome restriction")
    box_w: 40              # keep < 64 (tile size) or it can't fit 1-wide corridors
    box_h: 40
    size_class: large       # small | medium | person | large | huge
    resistances: {}
```

Key requirements:
- When authored, `letter` must be lowercase (map spawns recognize a-z) and unique
  within its biome scope - see "Biome restriction" below.
- `sprite` must exist in `assets/sprites/mobs/` (without `.png`).
- `alert_radius` and `attack_radius` are in tiles.

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

## Step 4: Optional special effects
Supported fields (from `monsters.yaml`):
- `perfect_dodge`
- `fireburst_chance`, `fireburst_damage_min`, `fireburst_damage_max`
- `poison_chance`, `poison_duration_seconds`
- `flying`
- `passive_until_attacked` (won't aggro until struck)
- `pounce_range_tiles`, `pounce_cooldown_seconds` (leap to melee from range, e.g. puma)
- `attack_cooldown_multiplier`, `attacks_per_round`

### Boss kit (all data-driven; see golden_thief_bug)
- `ignores_armor` - melee bypasses party armor class
- `inferno_chance` + `inferno_damage` - party-wide fire nova
- `teleport_at_hp` + `teleport_chance` - low-HP blink to a random tile
- `passive_until_quest` + `evade_radius_tiles` + `boss_cooldown_seconds` -
  evades (blinks away, never attacks) until the named quest completes

Paired boss fields are validated at load: a chance without its magnitude (or an
evasive phase without its tuning) fails startup. Beware: any OTHER unknown YAML
field is silently ignored - a typo won't error, the feature just won't work.

## Step 5: Add loot
Loot is controlled by `assets/loots.yaml` using the monster key.
Loot drops into a bag on the ground and must be picked up by the player.

Example:
```yaml
loots:
  ice_troll:
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
    box_w: 18
    box_h: 18
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

[assets/ecology.yaml](assets/ecology.yaml) owns persistent wildlife populations.
Each `populations` entry names `map`, `monster`, target `count`, and replenishment
`phase` (`day` or `night`). Survivors remain; the phase replenishes deficits.
The shipped jungle targets are 10 ring-tailed and 10 red ruffed lemurs at the day
phase. This is a target population, not 10 additional animals every dawn.

`config.yaml day_night.packs` instead defines phase-swapping packs, including
mixed `day_monsters`/`night_monsters` lists. Pack members normally do not advance
kill quests; opt in with `quest_progress` where intended. An authored map's
`respawn_days` is a third mechanism, checked on arrival; see [maps](docs/adding-maps.md).
Do not register the same intended population in more than one mechanism.

## Testing checklist
- Run the [shared validation checklist](docs/content-authoring.md#verification).
- `letter` is unique and lowercase.
- Static sprite and required animation sheets exist.
- Monster spawns and behaves correctly.
- Loot drops into a bag when killed.

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
`assets/loots.yaml` together. See `internal/game/fish_test.go` and the opt-in
`TestDebugSim_FishGallery` renderer capture for integration coverage.

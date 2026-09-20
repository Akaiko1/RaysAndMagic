# How to Add a New Tile

Tiles are loaded by `TileManager` at startup. Read
[shared authoring rules](docs/content-authoring.md) first.

## Overview
- Base tiles: `assets/tiles.yaml`
- Special tiles: `assets/special_tiles.yaml` (merged into the same tile database)
- Map placement: single-letter symbols in `.map` files
- `render_type` must be one of: `wall`, `crossed_standee`, `crossed_prop`, `standee`, `floor`, `landmark_standee` (validated at load - an unknown value refuses to boot)
- `transparent` controls raycast pass vs sprite pass; be explicit to avoid visual artifacts.

## Choose the render type first

| Type | Intended use and size |
| --- | --- |
| `floor` | Ground color/texture, no vertical sprite |
| `wall` | Opaque vertical texture slices; source fills its square |
| `crossed_standee` | Natural trees/rocks; size class controls frame width; tree mechanics apply |
| `crossed_prop` | Built blockers such as crates/logs; size class controls visible height; no tree mechanics or billboard LOD |
| `standee` | Camera-facing prop; must be walkable unless wall-mounted |
| `landmark_standee` | Landmark sprite; shared prop size class |

`solid`, `walkable`, `transparent`, and `render_type` are separate controls.
Choose collision, standing permission, ray continuation, and drawing explicitly.
`wall_mounted` is valid only on `standee`; `no_spin` only on `standee` or
`landmark_standee`. A cross already has fixed planes.

## Step 1: Add the tile
Add a new entry under `tiles:` in `assets/tiles.yaml`.

Example:
```yaml
tiles:
  magic_crystal:
    name: "Magic Crystal"
    type: prop
    solid: false
    transparent: true
    walkable: true
    size_class: tall_prop  # visible alpha height; values live in config.yaml
    sprite: "moss_rock"    # must exist in assets/sprites/environment/
    render_type: "standee"
    floor_color: [150, 100, 255]
    short_label: "magic_crystal"
```

Sprite files live in `assets/sprites/environment/` (no `.png` suffix in YAML).
A missing sprite does NOT error - it renders as a placeholder image, so verify
the file exists.

Advanced optional fields: `impassable_aura` (rising-bubble hint on blockers),
`fly_over` (explicit airspace for flying monsters on a transparent, non-solid,
non-walkable `floor`), `light` (`enabled`, `radius_tiles`, `intensity` - the
tile lights the scene), `floor_near_color`, `alpha_from_brightness`.

## Step 2: Place it in a map
Use a general-tile placeholder and its label for this letterless prop:
```text
....$....>[tile:magic_crystal]
```
A tile with an authored `letter` instead uses that character directly.

Ordinary tiles also require a `type` from the editor taxonomy (`floor`, `water`,
`marker`, `wall`, `wall_decor`, `nature`, `rock`, `structure`, `prop`). This is
separate from `render_type`. Special tiles use their own behavior type.

## Letter rules
- Letters must be unique per biome. TileManager errors on conflicts.
- If `biomes` is omitted, the letter must be unique globally.
- Lowercase letters are reserved for monsters. Terrain uses uppercase letters,
  digits, or punctuation.
- Letterless general decor uses `short_label` in YAML and `$` with
  `>[tile:short_label]` in the map. See [map syntax](docs/adding-maps.md#map-syntax).

## Special tiles (data-driven placement)
Special tiles in `assets/special_tiles.yaml` can be placed by key using:
```
....@.....>[stile:spike_trap]
```
This replaces the `@` with the special tile matching `spike_trap`.

### Teleporters
Teleporter behavior is driven by `special_tiles.yaml` properties. Example:
```yaml
special_tiles:
  vteleporter:
    name: "Violet Teleporter"
    type: "teleporter"
    solid: false
    transparent: true
    walkable: true
    render_type: "floor"
    floor_color: [138, 43, 226]
    inherit_floor: true
    properties:
      cooldown_seconds: 5
      teleporter_group: "violet"
      cross_map: true
      auto_activate: true
      random_destination: true
      exclude_self: true
```

### Non-teleporter special tiles
Special tiles like `spike_trap` or `magic_circle` are now placeable by key, but their gameplay effects are still not implemented.

## Floor textures, color, and the biome default
Floor textures are biome-driven. The named texture groups live per-biome in the
top-level `biomes:` section of `assets/map_configs.yaml` (NOT on the tile and
NOT per-map). A tile picks a group with `floor_texture_group`:
- Set `floor_texture_group: "water"` (etc.) to use that biome group.
- Omit it and the tile borrows the biome's `"default"` group - UNLESS the tile
  sets a `floor_color`, in which case the color IS its look and stays
  untextured (teleporters, traps, spawn are coloured squares this way).
- Empty `.` (and any tile on the biome default) bordering water auto-uses the
  biome's `"beach"` group, if defined, for shoreline sand.

`floor_color` is a BASE color blended UNDER the texture (~80% texture up close,
fading to more color with distance), and shown 100% when no texture resolves
(no group, or the group isn't defined for the current biome). `floor_near_color`
is different: it tints ADJACENT empty floor tiles (grass darkens near trees).

## Supported fields
Core fields are fully supported:
- `solid`, `transparent`, `walkable`
- `fly_over` for transparent, non-solid, non-walkable floor terrain such as
  water or a chasm; never use it on walls, doors, or opaque terrain
- `wall_height_multiplier` for `wall`
- `size_class` for `standee` and `landmark_standee`; valid prop classes live
  under `graphics.size_classes` in `config.yaml`
- `crossed_standee` normally uses `size_class: tree` (2-tile frame width), but may
  select another known class for a smaller/larger tree; height always follows
  the source texture aspect
- optional `night_motes` on a `crossed_standee` tree enables moving night motes and
  authors `glow_color` plus `core_color`; absent means the tile emits none
- `procedural_effect: firefly_swarm` selects the fixed authored swarm renderer;
  unknown procedural effects fail tile loading
- `sprite`, `render_type`
- `floor_color`, `floor_near_color`, `wall_color`
- `floor_texture_group` (selects a biome floor-texture group; see above)
- `letter`, `biomes`
- `type` (used for teleporter detection)

### Night-mote runtime budget

Per-tree `night_motes` only opts a tree in and selects its colours. Shared
timing and performance limits live under `graphics.night_motes` in
`config.yaml`:

- `emission_radius_tiles`: only trees within this camera radius are scheduled
- `emission_interval_seconds`: each scheduled tree rolls once per interval
- `emission_chance`: probability that an interval roll emits one mote
- `max_per_tree`: maximum simultaneously living motes from one tree
- `lifetime_seconds`: lifetime of each emitted mote
- `max_active`: global safety cap for moving motes on the current map

The first roll is randomly staggered within one interval so trees entering the
radius together do not emit in synchronized bursts. Later rolls keep the exact
configured interval.

## Testing checklist
- Run the [shared validation checklist](docs/content-authoring.md#verification).
- Letter is unique for its biome.
- Sprite exists if specified.
- Tile renders and collides as expected.
- Special tile placement with `>[stile:key]` works.

## Known limitations
- Non-teleporter special tile behaviors require code changes.

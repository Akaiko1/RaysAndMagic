# How to Add a New Tile

Tiles are defined in YAML and loaded by the TileManager at startup.

## Overview
- Base tiles: `assets/tiles.yaml`
- Special tiles: `assets/special_tiles.yaml` (merged into the same tile database)
- Map placement: single-letter symbols in `.map` files
- `render_type` must be one of: `textured_wall`, `tree_sprite`, `environment_sprite`, `floor_only`, `flooring_object`
- `transparent` controls raycast pass vs sprite pass; be explicit to avoid visual artifacts.

## Step 1: Add the tile
Add a new entry under `tiles:` in `assets/tiles.yaml`.

Example:
```yaml
tiles:
  magic_crystal:
    name: "Magic Crystal"
    solid: false
    transparent: true
    walkable: true
    size_tiles: 1.2        # sprite height in tiles (1.0 == a 1-tile wall)
    sprite: "moss_rock"    # must exist in assets/sprites/environment/
    render_type: "environment_sprite"
    floor_color: [150, 100, 255]
    letter: "X"
```

Sprite files live in `assets/sprites/environment/` (no `.png` suffix in YAML).
A missing sprite does NOT error - it renders as a placeholder image, so verify
the file exists.

Advanced optional fields: `impassable_aura` (rising-bubble hint on blockers),
`light` (`enabled`, `radius_tiles`, `intensity` - the tile lights the scene),
`floor_near_color`, `alpha_from_brightness`.

## Step 2: Place it in a map
Use the `letter` in the map ASCII grid:
```
....X....
```

## Letter rules
- Letters must be unique per biome. TileManager errors on conflicts.
- If `biomes` is omitted, the letter must be unique globally.

## Special tiles (data-driven placement)
Special tiles in `assets/special_tiles.yaml` can be placed by key using:
```
%...@....%  >[stile:spike_trap]
```
This replaces the `@` with the special tile matching `spike_trap`.

### Teleporters
Teleporter behavior is driven by `special_tiles.yaml` properties. Example:
```yaml
special_tiles:
  vteleporter:
    type: "teleporter"
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
- `wall_height_multiplier` for `textured_wall`
- `size_tiles` (sprite height in tiles) for `tree_sprite`, `environment_sprite`, and `flooring_object`
- `sprite`, `render_type`
- `floor_color`, `floor_near_color`, `wall_color`
- `floor_texture_group` (selects a biome floor-texture group; see above)
- `letter`, `biomes`
- `type` (used for teleporter detection)

## Testing checklist
- YAML loads without errors.
- Letter is unique for its biome.
- Sprite exists if specified.
- Tile renders and collides as expected.
- Special tile placement with `>[stile:key]` works.

## Known limitations
- Audio and particle effects are not implemented.
- Non-teleporter special tile behaviors require code changes.

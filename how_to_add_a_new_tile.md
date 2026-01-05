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
    height_multiplier: 1.2
    sprite: "crystal"
    render_type: "environment_sprite"
    floor_color: [150, 100, 255]
    letter: "X"
```

Sprite files live in `assets/sprites/environment/` (no `.png` suffix in YAML).

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

## Supported fields
Core fields are fully supported:
- `solid`, `transparent`, `walkable`
- `height_multiplier`, `sprite`, `render_type`
- `floor_color`, `floor_near_color`, `wall_color`
- `letter`, `biomes`
- `type` (used for teleporter detection)

`effects` are still not used (no audio/VFX system yet).

## Testing checklist
- YAML loads without errors.
- Letter is unique for its biome.
- Sprite exists if specified.
- Tile renders and collides as expected.
- Special tile placement with `>[stile:key]` works.

## Known limitations
- Audio and particle effects are not implemented.
- Non-teleporter special tile behaviors require code changes.

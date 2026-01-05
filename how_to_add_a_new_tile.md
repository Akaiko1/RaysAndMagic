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
- Letters must be unique **per biome**. TileManager will error on conflicts.
- If `biomes` is omitted, the letter must be unique globally.

## Special tiles (teleporters)
Special tiles are defined in `assets/special_tiles.yaml` and are placed using the `@[stile:key]` map syntax.
Currently only these keys are wired by the map loader:
- `vteleporter`
- `rteleporter`

Example map line:
```
%...@....%  >[stile:vteleporter]
```
This replaces the `@` with the special tile and keeps the floor underneath walkable.

## Supported fields
Core fields are fully supported:
- `solid`, `transparent`, `walkable`
- `height_multiplier`, `sprite`, `render_type`
- `floor_color`, `floor_near_color`, `wall_color`
- `letter`, `biomes`

`properties` and `effects` are parsed but not used (except teleporter types being recognized by name).

## Render type quick notes
- `textured_wall`: solid walls, blocks vision
- `tree_sprite`: tall objects
- `environment_sprite`: 3D decorations
- `flooring_object`: ground-level decoration
- `floor_only`: floor color only (teleporters/spawn)

## Testing checklist
- YAML loads without errors.
- Letter is unique for its biome.
- Sprite exists if specified.
- Tile renders and collides as expected.

## Known limitations
- Audio and particle effects are not implemented.
- Non-teleporter special tile behaviors require code changes.

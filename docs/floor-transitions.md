# Floor texture transitions

Floor transition profiles control how neighboring ground textures blend. They
never change tile collision, walkability, flight rules, height, or water gameplay.
In particular, `void` is a visual blending rule, not a way to create a chasm.
Keep the gameplay flags in `assets/tiles.yaml` consistent with the artwork.

## Terms and abbreviations

| Term | Meaning |
| --- | --- |
| GPU | Graphics processing unit (graphics processor): runs the floor shader. |
| NPC | Non-player character: a person or prop placed on a map. |
| UV | Texture coordinates: where on its texture a floor pixel is read. |
| YAML | YAML Ain't Markup Language: the text format of the content files. |

## Configure a texture group

Tiles select a `floor_texture_group` from their biome. Assign its transition
profile in `biomes.<biome>.floor_transitions` in
[assets/map_configs.yaml](../assets/map_configs.yaml). The key is a texture-group
name, not a tile key or sprite filename. Every texture variant in that group
uses the same profile.

For example, the forest uses these groups and profiles (excerpt):

```yaml
biomes:
  forest:
    floor_texture_groups:
      default: [forest_grass_0, forest_grass_1, forest_grass_2, forest_grass_3]
      beach: [beach_sand_0, beach_sand_1]
      quest_bridge: [quest_bridge_0]
    floor_transitions:
      default: natural
      beach: natural
      quest_bridge: hard
```

`shared_floor_texture_groups` and `shared_floor_transitions` supply defaults
for every biome. A biome can override either by group name. The two mappings
are independent: overriding the texture list does not remove an inherited
profile. Use an explicit `hard` override to disable inherited blending.

A group with no profile after inheritance uses `hard`. Map-config loading
rejects unknown profile names and profile entries without a nonempty texture
group. Existing shipped groups declare their intended profiles explicitly.

## Profiles

| Value | Intended use | Behavior |
| --- | --- | --- |
| `hard` | Bridges, paving, boards, and painted transition art | Never blends with neighboring tiles, even variants within its own group. |
| `natural` | Grass, sand, soil, and ordinary rock ground | Blends with natural ground and water; accepts a cliff rim only on its solid side. Receives a narrow wet-darkening band near water. |
| `water` | Water surfaces and streams | Meets natural ground along a thin waterline that turns tile corners on a small arc; fades into another water group (shallow into deep) in one smooth gradient. Contributes to the distance field that drives shoreline effects. Whether waves break on the bank is set per body of water, not by the profile (see Surf). |
| `void` | Textures depicting the bottom of a chasm | Blends only with other `void` textures; never with land, water, rims, or bridges. |
| `cliff_east` | Rim art with its drop on the right (+X) | Blends only with a `natural` tile immediately to its left (-X), on the solid side. |
| `cliff_west` | Rim art with its drop on the left (-X) | Blends only with a `natural` tile immediately to its right (+X), on the solid side. |

Pair compatibility is symmetric: both tiles must permit the transition.

| Pair | `hard` | `natural` | `water` | `void` | `cliff_east` | `cliff_west` |
| --- | --- | --- | --- | --- | --- | --- |
| `hard` | No | No | No | No | No | No |
| `natural` | No | Yes | Yes | No | Solid side only | Solid side only |
| `water` | No | Yes | Yes | No | No | No |
| `void` | No | No | No | Yes | No | No |
| `cliff_east` | No | Solid side only | No | No | No | No |
| `cliff_west` | No | Solid side only | No | No | No | No |

Compatibility also applies to variants of the same group. Sand variants blend
under `natural`; board variants remain separate under `hard`. Cliff profiles
do not blend along their north/south edges or diagonally, including with other
cliff tiles. Profiles do not rotate, mirror, or rewrite the source artwork.

## Dragon Cliffs and directional artwork

East and west describe the drop side of the source texture in world coordinates,
not the side of the screen. Texture UVs remain world-aligned in both separate
maps and the stitched open world. Rotating a region's tile grid in
`assets/open_world.yaml` does not rotate its floor textures.

The shipped Dragon Cliffs assignments are:

```yaml
floor_transitions:
  default: natural
  basalt: natural
  chasm_edge_0: cliff_east
  chasm_edge_1: cliff_west
  chasm_floor_0: void
  chasm_floor_1: void
  bridge_0: hard
  bridge_1: hard
```

The renderer clamps each neighbor sample to that texture's own border so its
drop side cannot wrap onto the opposite landward edge. Do not mark a painted
rim `natural`: that would allow its drop to blend into surrounding ground.
There are currently no north/south cliff profiles; artwork requiring them needs
an explicit renderer and validation extension, not a misleading east/west label.

## Automatic ground and shorelines

Transitions use resolved ground: map-loader ground replacements under monster
markers and NPCs are applied first. Tiles with `inherit_floor` then select the
surrounding ground unless they explicitly name a texture group. Moving a monster
does not repaint the ground or move a shoreline.

A biome's `beach` group is an automatic shoreline layer only when its profile
is `natural`. It overlays resolved `default` ground near tiles with the `water`
profile; it does not replace the underlying tile or change its collision.
The shipped automatic-shore biomes also give `default` the `natural` profile.
Other ground groups do not receive this automatic sand layer.

The beach fades with distance from water, and its texture variants blend too.
A painted directional bank, such as Sakura Garden's beach artwork, stays `hard`
and is used only where authored. It is not repeated around every water tile.
The literal group name `water` alone does not drive shoreline detection: the
resolved `water` profile does, including named stream groups.

Variant selection is deterministic from tile coordinates and tile type. It
breaks the former checkerboard pattern without rerolling on every frame or load.
Changing group contents or coordinates can change which variant is selected.
Derived material and shoreline maps rebuild on map load, save restoration, and
quest terrain changes; they are not stored in saves.

## What can be tuned

YAML currently selects profiles and texture lists. Transition width, noise
frequency/amplitude, wet darkening, and shoreline distance are renderer settings
in code, not supported YAML attributes. Do not add guessed `blend_width` or
similar keys and expect them to work.

Band widths, noise strengths and the look of the surf are named constants at
the top of [floor.kage](../internal/shaders/floor.kage) (`floorBand*`,
`floorSurf*`, in tiles). How fast crests travel is `floorSurfPeriodSeconds` in
[render_floor_transitions.go](../internal/game/render_floor_transitions.go):

| Pair | Band |
| --- | --- |
| Natural against natural | Wide cross-fade, slight noise. |
| Water against natural | A thin waterline, about one texel, no noise. It follows the tile edge and turns a corner, convex or concave, on a circular arc of radius `floorBankRound` tiles; past its own tiles a texture tiles on, so no edge pixel is stretched. A hard tile among the four cells keeps the corner square. |
| Two water groups (shallow and deep, a stream and a lake) | One smooth gradient from tile centre to tile centre, no noise, so a deep patch has a soft rim and a stream runs into a lake. Both textures tile on across it; no edge pixel is stretched. |
| Variants of one water group; ice against open water | Narrow cross-fade; a variant keeps its own art inside its tile and ice keeps a crisp edge. |
| Void or cliff pairings | Narrow cross-fade. |

### Surf

Every body of water on a map is classified in `assets/map_configs.yaml` under
the map's `water_bodies`: one entry per 8-connected body of `type: water`
tiles (water, deep water, streams, fords, koi), named by any of its cells in
map-local coordinates, with a `kind` of `river`, `lake`, `sea`, `pool` or
`stream`. Waves break on the bank of a lake or sea only, over every water cell
of the body within `floorSurfReach` tiles of land, deep cells included. A body
may mix tiles; its kind covers all of them. The boot fails on an unlisted
body, an entry off water, two entries on one body or an unknown kind, and the
message lists each unlisted body by its first cell.

Crests `floorSurfSpacing` apart move one spacing every
`floorSurfPeriodSeconds` on the world clock, each a crisp line with flecks of
foam behind it, broken into runs, cut on the texture's pixel grid. Surf
covers water a rounded corner places inside a land tile. Nothing outlines the
bank. Frozen water has no surf. Off the map counts as a bank: a wall is drawn
there in both world modes. Water an NPC's or monster's `ground_tile` stamps is
part of the map and needs its entry like any other; water a terrain change
places after load belongs to no body and stays calm.

Noise is anchored in world coordinates, with reduced detail at distance.
Texture interiors retain their source art; compatible boundaries mix
neighboring samples. This is texture blending, not generated transition art or
terrain geometry. Collision stays on the tile grid; a rounded bank corner departs from
it by a few texels.

The implementation is split between:

- [Profile names and validation](../internal/config/floor_transitions.go).
- [Inheritance and load validation](../internal/world/world_manager.go), `LoadMapConfigs`.
- [Resolved materials and shore maps](../internal/game/render_floor_transitions.go).
- [Variant selection](../internal/game/renderer.go), `stableFloorTextureIndex`.
- [GPU compatibility and blending](../internal/game/render_helper.go), `floorShaderSrc`.

## Verify a content change

Inspect the boundary from both directions and at near and far distances. Check
same-group variants, intersections of three or four materials, shorelines,
under-entity ground, and save/load. For directional art, inspect both the separate
map and its stitched placement. Confirm bridges and drop edges remain distinct.

Automated coverage lives in `internal/game/render_floor_transitions_test.go`,
`internal/game/debug_floor_sampling_test.go`,
`internal/config/floor_transitions_test.go` and
`internal/world/water_bodies_test.go`. From the repository root:

```sh
go test ./internal/config ./internal/world ./internal/game -run 'TestTerrain|TestFloorTransition|TestKageShadersCompile|TestShaderConstantsMatchGo|TestResolveWaterBodies'
RAM_DEBUG_SIM=1 go test -tags debug ./internal/game -run '^TestDebugSim_FloorMinificationStable$' -count=1
```

The second command requires a working graphics environment. How a boundary
looks is judged in the game: stand at the bank and compare the three view modes,
near and far.

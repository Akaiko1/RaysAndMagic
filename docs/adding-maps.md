# Adding maps and world connections

Read [shared authoring rules](content-authoring.md). Classify the location first:
stitched outdoor region, separate outdoor map, town, or closed dungeon. The
[map editor](../assets/map_viewer/README.md) edits the source maps even when the
game stitches outdoor regions together.

## Register the map

Create `assets/herb_clearing.map`, then merge a map entry into
`assets/map_configs.yaml`. Reuse an existing biome instead of copying its floor
texture lists into every map. This standalone example includes the minimal
biome declaration for validation; in the real catalog keep the existing, richer
`forest` biome definition.

```yaml
maps:
  herb_clearing:
    name: "Herb Clearing"
    file: "assets/herb_clearing.map"
    biome: forest
    sky_color: [100, 150, 220]
    default_floor_color: [55, 85, 40]
    ambient_light: 1.0
biomes:
  forest:
    elemental_attack_school: earth
```

`sky_texture`, biome `floor_texture_groups`, `out_of_bounds_tile`, and
`canopy_shade` let the map share or specialize its appearance. Dark interiors
can set lower `ambient_light` and `wall_torches`. Add a supported entrance and
exit (NPC travel action or teleporter group), and test arrival tiles and return
travel. Merely registering a file does not make it reachable.

## Map syntax

Each grid cell is one ASCII character. Keep rows rectangular and put entity
definitions after `>` on the same row. Use the editor to serialize multiple
definitions in placeholder order; never put explanatory comments inside a grid.

| Cell / definition | Meaning |
| --- | --- |
| `+` | Party start |
| `.` | Empty floor |
| Lowercase `a`..`z` | Monster letter, resolved for the map biome |
| Other registered tile letter | Tile lookup for that biome; do not assume `#` always means wall |
| `@` and `>[npc:key]` | NPC placement |
| `@` and `>[stile:key]` | Special tile placement |
| `$` and `>[tile:short_label]` | Letterless general tile placement |
| `>[npc:key@tile_key]` | NPC with a placement-specific ground tile |

For example, this is a five-column grid with a shipped NPC:

```text
.....
.+...
..@..>[npc:merchant_general]
.....
.....
```

Replace the NPC key with one installed in your catalog (the merchant example is
in the [NPC guide](../how_to_add_a_new_npc.md)). Use the editor's biome palette
to select valid walls and blockers. All authored coordinates are zero-based
map-local tile coordinates, including chest rewards, quest changes, and markers.

## Encounters and respawning

`clear_encounter` attaches one reward to the map's initial monster group.
`clear_encounters` declares multiple groups by monster `type` and `count`; each
binds the nearest matching pre-placed monsters to its reward chest. The plural
form takes precedence. Put chests near their intended groups and use stable,
unique chest IDs. Dynamically scheduled wildlife is a separate population.

Rewards can contain gold/experience, completion text, and `treasure_chest` or
`treasure_chests`. Chests use local `tile_x`/`tile_y`, item/weapon keys, or a
named `loot_table`; see [items and loot](adding-items-and-loot.md).

`respawn_days > 0` refreshes the authored monster roster on arrival after the
configured calendar interval. Arena maps use `duel` staging geometry. Neither
kind may be merged into the open world. Wildlife replenishment and phase-swapped
combat packs have separate schedules; see [monsters](../how_to_add_a_new_monster.md#population-and-respawn).

## Open-world stitching

`config.yaml world.open_world` enables
[assets/open_world.yaml](../assets/open_world.yaml). Use that complete file as
the geometry example:

- `placements` assigns source-map origins and optional `orient` (rotation or
  mirror). Placements must not overlap.
- `connections` uses each source map's own `edge`, `at`, optional `depth`, and a
  passage `width`. Aligned opposing edges use the configured corridor length;
  other arrangements route through void and must not cross another map.
- `removals` lists travel NPCs/special tiles replaced by passages. Preserve
  entrances needed in split mode in the source `.map`; stitching removes them
  only from the assembled world.

Keep authored and saved positions as source map key plus local coordinates.
Runtime `ProjectTile`/`ProjectWorldPos` handles placement transforms. Do not
rewrite quests or saves to global stitched coordinates. Moving a placement is
different from changing the source map's coordinate system or removing its key.

## Verify

Follow [shared verification](content-authoring.md#verification). Open the map in
the editor, walk entrances and exits, check collision/visibility and every entity
tag, then test encounters and rewards. For stitched regions check corridor
geometry, region lighting/sky changes, quest scope, and a save round-trip in both
world modes. Keep drafts and generated asset previews outside the repository.

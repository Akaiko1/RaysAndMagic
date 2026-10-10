# Monster spawns over time

A map's authored monsters appear when the map loads. Three separate
mechanisms add or restore monsters later:

- **Day/night packs** (`config.yaml`, `day_night.packs`): a group that exists
  only during its phase and is swapped at every dusk and dawn.
- **Ecology populations** (`assets/ecology.yaml`, `populations`): a group that
  lives permanently and is topped up once per occurrence of its phase.
- **Map respawn** (`assets/map_configs.yaml`, `respawn_days`): the whole
  authored roster of a map returns when the party arrives after enough days.

Register each intended group in exactly one mechanism. Two mechanisms that
target the same group add their counts together, and neither knows about the
other.

## Choosing a mechanism

Ask what should happen to the survivors when the phase changes.

| Question | Use |
| --- | --- |
| Should the group exist only by day (or only by night) and vanish at the other phase? | Day/night pack |
| Should the group stay around and only be refilled after losses? | Ecology population |
| Should a dungeon restock its own authored monsters every few days? | `respawn_days` |

| | Day/night pack | Ecology population | `respawn_days` |
| --- | --- | --- | --- |
| File | `config.yaml` `day_night.packs` | `assets/ecology.yaml` `populations` | `assets/map_configs.yaml` |
| When it runs | At every dusk and dawn | Once each time its phase begins | When the party enters the map |
| Survivors | The outgoing phase's pack is removed | Stay; only the missing count is added | Replaced by the full authored roster |
| Where monsters appear | Free authored monster spawn points of the map | Any free bare-ground tile of the map | Their authored positions |
| Distance from the party | `min_player_dist_tiles` (default 8) | At least 4 tiles (fixed) | Not applicable (runs on entry) |
| Counts toward kill quests | Only members with `quest_progress: true` | Never | Like any authored monster |
| Party-level options | `min_party_level`, `replacement` | None | None |
| Open world | Allowed; scoped to the map's region | Allowed; scoped to the map's region | Not allowed |

## Day/night packs

Each entry in `day_night.packs` names a `map` and the monsters for each phase.
The shipped forest pack is a complete example:

```yaml
day_night:
  packs:
    - map: forest
      day_monsters:
        - monster: wolf
          count: 8
          replacement: {monster: dire_wolf, min_party_level: 20, chance: 0.20}
      night_monsters:
        - monster: forest_spider
          count: 8
          quest_progress: true
          replacement: {monster: giant_spider, min_party_level: 20, chance: 0.20}
      min_player_dist_tiles: 8
```

Fields:

- `day_monster` / `night_monster` with `count`: the short form for one kind
  per phase. Leave a phase empty for no pack in that phase.
- `day_monsters` / `night_monsters`: a list for a mixed phase (for example
  grunts plus an elite). When a list is present, it wins over the short form
  for that phase. Each member has `monster` and `count`, and optionally:
  - `quest_progress: true`: kills advance kill quests. Without it, pack
    members are ignored by kill quests.
  - `min_party_level`: the member spawns only once the party reaches this level.
  - `replacement: {monster, min_party_level, chance}`: once the party reaches
    the level, each spawn has this chance to use the stronger monster instead.
- `min_player_dist_tiles`: no pack member appears closer to the party than
  this. The default is 8.
- `require_map_clear: true`: a new pack spawns only while the map (or its
  open-world region) has no living hostile monster, including survivors of
  the previous pack.

How it behaves:

- Packs change only at a dusk or dawn. Loading a save or entering a map never
  spawns a pack. A new game starts at noon, so the first night pack arrives
  at the first dusk, and the first day pack at the next dawn.
- At each change, the outgoing phase's pack is removed from every configured
  map, not only the one the party is on. Removed members give no experience or
  loot. They vanish even in the middle of a fight. Only bound or charmed
  members stay.
- `count` is a cap, not a guarantee. Members are placed on free authored
  monster spawn points (positions from the map's monster letters). A living
  monster keeps its own spawn point reserved. If too few points are free,
  fewer monsters appear. All members of one phase share the same pool.

## Ecology populations

Each entry in `populations` keeps a target number of animals or monsters on a
map. The shipped entries are complete examples:

```yaml
populations:
  - map: solstice_approach
    key: dawn_hunters
    monsters: [solstice_dusk_moth, solstice_sunscarab, solstice_rain_serpent]
    hostile: true
    count: 7
    phase: day
  - {map: deep_jungle, monster: ring_tailed_lemur, count: 10, phase: day}
  - {map: desert, monster: fennec, count: 2, phase: night}
```

Fields:

- `map`: the map key.
- `monster`: one species. Or use `key` with a `monsters` list instead: each
  new spawn picks a random species from the list. Use one form, not both; a
  list needs a `key` and must not repeat a species. The map plus the `key` (or
  `monster`) must be unique across all entries.
- `count`: the target population, not an amount added each time. Must be
  positive.
- `phase`: `day` or `night`, the phase at whose start the refill happens.
- `hostile`: `false` (default) for wildlife; every species must then have
  `disposition: wildlife`. `true` for ordinary combat monsters; the species
  must not have a disposition. The game checks this at startup.

How it behaves:

- Survivors are never removed, at any phase. When the named phase begins, the
  game counts the living members and spawns only the difference up to `count`.
- The refill happens once per occurrence of the phase. The game saves which
  phase it last refilled, so reloading or re-entering the map during the same
  phase does not refill again. In a new game, day populations fill
  immediately and night populations at the first dusk.
- New members appear on random free bare-ground tiles of the map (its region
  on the open world): tiles with `type: floor` in `assets/tiles.yaml`. Grass,
  ferns, heather, props, water and markers never take a spawn, even when they
  are walkable, because such tiles also fill the space outside a region's
  walls. A spawn is never on a tile with a monster or NPC, and at least 4 tiles
  from the party. Populations do not use the map's spawn points. A hostile
  population can therefore appear fairly close to the party when its phase
  begins.
- A map with a population needs enough free floor tiles inside its playable
  area. If too few are free, fewer members appear.
- A hostile population also counts monsters of its species that the map
  itself places, so authored and refilled ones share the same target.
- Ecology monsters never advance kill quests.
- Keep `key` (or `monster` for a single-species entry) stable. Saves track the
  population by it; renaming it makes the old survivors stop counting.

## Map respawn

`respawn_days: N` in a map's entry in `assets/map_configs.yaml` restores the
map's full authored monster roster when the party enters the map at least N
calendar days after the previous restore. The shipped Clock Tower floors use
`respawn_days: 3`. Such a map cannot be merged into the open world. The restore
shows the Clock Tower's message ("The tower mechanism grinds...") on any map
that uses this setting. See [adding maps](adding-maps.md#encounters-and-respawning).

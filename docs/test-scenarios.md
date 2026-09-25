# Developer test scenarios

Run `./build_bin.sh`, then double-click `bin/test_pilgrimage.command`, or run:

```sh
bin/test_scenario.command pilgrimage
bin/test_scenario.command arena
```

The binary accepts `--test-scenario NAME` (also `--test-scenario=NAME`).
`--test-arena` remains an alias for `--test-scenario arena`.
Scenario saves, profile and stash live under `bin/test-runs/NAME/saves/` for the
local binary. Normal saves are not read or overwritten. Each launch constructs
a fresh scenario; its own saved games can be loaded from the in-game menu.

Add an entry to `assets/test_scenarios.yaml` to create another fixture. No Go
branch is needed. A convenience `.command` can delegate to
`test_scenario.command NAME`; tracked launcher sources live in `scripts/` and
`build_bin.sh` installs them into `bin/`.

Supported fields:

| Field | Meaning |
| --- | --- |
| `party` | Up to four members, with `name`, class key, optional `skills` mastery map and `equipment` item keys. Omit to keep the normal starting party. |
| `level` | Raise active members through the normal XP progression to this level. Earned level-up choices remain unspent. |
| `speed_target`, `endurance_target` | Spend earned attribute points toward these targets, then the class's primary attribute. |
| `learn_school_spells` | Learn spells from each member's existing schools. |
| `map`, `x`, `y`, `angle` | Map-local tile position and heading in radians; works in separate and stitched worlds. |
| `safe_radius` | Clear existing monsters within this many tiles of the start, without rewards; later quest spawns are unaffected. |
| `items`, `gold` | Extra inventory item keys and gold. |
| `quests` | Activate quests through the normal acceptance and spawn rules. Omit to test the NPC offer. |
| `clear_maps` | Resolve monster loot and XP and remove those map populations. |
| `complete_npc_encounters` | NPC keys: resolve their encounter monsters and reward. |
| `reward_map_encounters` | Map keys: grant their authored clear-encounter rewards. |

Unknown fields, classes, skills, mastery names, items, maps and blocked start
positions fail loudly. The launcher is a development fixture, not a balance
migration or an automatic quest completion tool.

## Pilgrimage check

The fixture starts four level-15 heroes (Monk, Cleric, Archer, Knight) beside
Sister Mira on Brae Meadow. The Monk has Expert Iron Body; the quest
does not train mastery. Earned level-up choices remain available through the portraits.

1. Speak to Mira. Defeat the level-16 Bronze Gatekeeper and level-15 Gale Novice
   northeast of her. Claim the headband and handwraps in the journal.
2. Find the three sluices northeast of the desert's central oasis. Turn Spring,
   Travelers, Monastery. A wrong order resets without damage. Claim sandals
   and sash in the journal.
3. Search the west-bank jungle clearings east of the oxbow, around the fern
   waystone, and north of the landing tavern. Gather three Sunlotuses by day
   and two Moonbells by night. Five locations are chosen once from ten authored,
   reachable candidates; selections and collected tokens survive save/load.
   Flowers have no map markers; opposite-phase flowers show closed buds. Claim mantle and robe.

All six pieces are restricted to Monk and Cleric. Their base armor totals 60;
the complete set adds 35 armor and 8 Personality. Grandmaster Iron Body adds
40, bringing a Monk to the useful 135 AC cap. A Cleric gets 95 before other gear
or buffs. The chain grants exactly two items per chapter and no random gear.

## Quest activity authoring

Quest `rewards.items` is a guaranteed list, separate from the existing random
`item_pool`. `next_quest` activates the next chapter atomically when a reward is
claimed. `on_accept_spawns` uses stable IDs and the same world/save machinery as
completion spawns. Acceptance spawns cannot defer to map entry.

An `interact` quest may declare exactly one `activity.sequence` or
`activity.forage`. A sequence lists ordered prop tokens and a `wrong_message`;
its objective count is one. Forage groups specify `phase: day|night`, `count`
and stable candidate `tokens`; total group counts equal the objective count.
Each token belongs to a top-level NPC `prop` choice carrying the quest ID and
interaction tag. A flower can author `dormant_sprite`. Activity props are
repeatable interactions; their quest state owns consumption, not NPC `Visited`.
Activity quests are nonrepeatable and cannot start automatically.

Use `active_prop_layouts` in `assets/quests.yaml` for alternate temporary
activity placements; a layout is chosen when a new game starts, before the
quest is accepted, and its ID is saved. Do not also place these NPC keys in
`.map` files. Coordinates are map-local tiles. For example:

```yaml
    min_prop_spacing_tiles: 18
    active_prop_layouts:
      - id: north_east_south
        props:
          - {npc: pilgrim_sluice_spring, map: desert, x: 15, y: 6}
          - {npc: pilgrim_sluice_travelers, map: desert, x: 37, y: 16}
          - {npc: pilgrim_sluice_monastery, map: desert, x: 8, y: 38}
```

Every alternate layout contains the same NPC identities. `min_prop_spacing_tiles`
checks pairwise distances within a map. Pilgrimage authors three desert routes
(minimum 18 tiles apart) and three jungle routes (minimum 16), all tested by a
walking-only flood fill from the region's arrival paths. For a single fixed
layout, use a flat `active_props` list instead; the two fields are exclusive.
Old saves without layout IDs receive a choice once and persist it thereafter.
Forage groups can declare `legacy_tokens` when retiring old candidate identities;
loading remaps those tokens within the same day/night group without losing credit.

Each key must name a transparent scenery NPC with a top-level activity prop
for this quest. Painted `ground_tile`, combat encounters, merchants and doors
are not supported: activity state is the sole source of progress. All candidate
tiles must be walkable, in bounds, distinct and free of authored NPCs. Unknown
keys/maps, duplicate identities and missing activity-token placements fail at
load. Nothing is written into the map terrain.

| State | Physical objects |
| --- | --- |
| Not accepted | None |
| Active sequence | All declared props |
| Active forage | Only randomly selected, uncollected candidates |
| Objectives complete, reward unclaimed | Remaining props stay until turn-in |
| Reward claimed or quest failed | None |

Turn-in through Mira and the journal share this lifetime. Completing one
chapter removes its objects and spawns the next chapter's objects. Gathering
a flower removes it immediately; the other time of day still shows a closed
bud for an uncollected flower. Save/load reconstructs the roster from saved
activity state, preserving selected locations and collected tokens. Loading an
older save or starting a new game removes objects from the previous timeline.
The same YAML works for split maps and stitched regions.

`safe_radius` optionally clears existing monsters around the start (in tiles), without rewards. Pilgrimage uses 5 to keep the opening conversation safe. Later quest spawns are unaffected.

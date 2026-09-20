# Adding quests

Read [shared authoring rules](content-authoring.md) and
[dialogue guidelines](content-authoring.md#dialogue). Quest definitions live in
`assets/quests.yaml`; the offer, turn-in, and service gates live on NPC choices.

## Define the objective

This example uses the existing forest goblins. Keep `target_map` as the source
map key, including when that map is part of the stitched world.

```yaml
quests:
  herb_path_patrol:
    name: "The Herb Path"
    description: "Goblins have taken the path to Mara's herb beds. Kill {target_count} Goblins in the forest, then return for payment."
    type: kill
    target_monster: goblin
    target_count: 3
    target_map: forest
    is_starting_quest: false
    rewards:
      gold: 40
      experience: 80
      item_pool: [health_potion]
```

Supported quest types are `kill`, `encounter`, and `interact`. An `interact`
quest matches a runtime interaction tag through `target_monster` and requires
`progress_text`; it needs a real producer of that tag. `target_monsters` supports
mixed target lists. A new quest type needs code, not just a new YAML string.

For example, this objective uses the existing arena victory event. It starts
active, so it does not need a new giver:

```yaml
quests:
  first_practice_victory:
    name: "A Place in the Sand"
    description: "Win one champion duel in the arena."
    type: interact
    target_monster: arena_duel
    target_count: 1
    progress_text: "champion duels won"
    is_starting_quest: true
    rewards:
      gold: 25
      experience: 50
```

For an authored interaction prop instead, pair its `action: prop` choice with a
`prop` block naming the same tag and the required state-dependent response text.
Use a shipped valve or lamp as the complete prop example. `target_map` is not
supported for `interact` quests; the event/prop establishes the objective.

## Connect an NPC

Install this entry together with the quest above, then place it using
`@` plus `>[npc:herb_path_mara]` as shown in the [NPC guide](../how_to_add_a_new_npc.md#step-2-place-the-npc-in-a-map).

```yaml
npcs:
  herb_path_mara:
    name: "Mara the Herb-Picker"
    type: quest_giver
    sprite: elf
    render_category: npc
    size_class: person
    dialogue:
      greeting: "The goblins have my herb path. They can keep the nettles, but I need the feverleaf."
      active_message: "Three goblins, then the path is ours again. Mind the nettles."
      completed_message: "No shouting from the herb beds? Good. Here is your pay."
      visited_message: "The feverleaf is coming back. So are my customers."
      choices:
        - text: "We will clear the path."
          action: give_quest
          quest_id: herb_path_patrol
        - text: "The path is clear."
          action: turn_in_quest
          quest_id: herb_path_patrol
        - text: "Leave."
          action: leave
```

Use per-choice `requires_quest` to gate the next offer in a chain. Completion
and claiming the reward are distinct: this gate requires the prior reward to
have been claimed. NPC-level `requires_quest` withholds a service until that
condition holds. `quest_messages` supplies per-step dialogue for multi-quest
NPCs; avoid repeating the entire tutorial in every state.

## Rewards, repeats, and world changes

Rewards support `gold`, `experience`, `arena_points`, and `item_pool` (one random
item key). Explicit weapons belong in encounter reward chests, not `item_pool`.
`auto_claim` is for objective-only quests; repeatable quests require an explicit
claim.

`repeatable` is omitted for a one-off quest, or is `day`, `night`, or an interval
such as `3d`. It is not a boolean. Phase repeats reset at the named phase;
interval repeats are measured from claiming. `fixed_quota: true` is for
non-exterminate kill quests that accumulate progress across replenishing packs.
Coordinate quest targets with the actual spawn source and its `quest_progress`
policy; spawning a monster does not automatically opt every source into quests.

`on_complete_tiles` applies tile-key changes on completion and reapplies them
on save load. `on_complete_spawns` fires once; each entry requires a stable `id`,
`map`, `x`, `y`, and `monster`. Optional `on_entry` defers it until arrival.
Coordinates and quest markers stay map-local. Never change a spawn ID just to
reorder a list: that ID records whether the event has already happened.

## Verify

Run [shared verification](content-authoring.md#verification). Play the whole
offer -> progress -> completion -> turn-in -> concluded sequence. Verify wrong
targets/maps do not advance it, reward claiming is not duplicated, and save/load
preserves each state. For repeats, test the actual calendar boundary; for world
changes, test leaving and returning in both split and stitched modes.

# How to Add a New NPC

NPCs live in `assets/npcs.yaml` and are placed in maps. Read
[shared authoring rules](docs/content-authoring.md) and
[dialogue guidelines](docs/content-authoring.md#dialogue) first.

## Overview
- Placement uses `@` in the map grid plus a trailing `>[npc:key]` definition on the same line.
- NPC sprites are looked up by name (no `.png` suffix).
- NPC types supported: `spell_trader`, `merchant`, `encounter`, `quest_giver`
  (dialogue with `give_quest`/`turn_in_quest` choices), `skill_trainer`,
  `card_collector`, `spell_lectern`, `loot_crate`, and `door`.
  The canonical list is `NPCTypeOrder` in `internal/character/npcs.go`.

### Render category and size
- `render_category` is REQUIRED and sets how the NPC renders: `npc` (a person;
  a `w == h*4` idle sheet animates automatically), `scenery` (a
  prop), `landmark` (tall crossed monument), `wall_mounted` (a flush wall standee,
  slides onto the adjacent wall), `door` (a doorway blocker - stands ACROSS the
  opening between two flanking walls; `door_behavior` determines whether a lock
  or a living arena champion keeps it closed), `wide_landmark` (fixed facade using `grid_span_tiles` and `grid_span_dir`),
  or `invisible` (no sprite). A `wide_landmark` omits `size_class` and uses
  a span of at least two tiles; an invisible anchor also omits `size_class`.
- Size: people use `size_class: person`. Props and landmarks select one of
  `tiny_prop`, `small_prop`, `medium_prop`, `full_tile`, `tall_prop`,
  `large_prop`, or `structure`. The target visible heights live once under
  `config.yaml graphics.size_classes`; raw per-object sizes are rejected.

## Step 1: Define the NPC
Add an entry under `npcs:` in `assets/npcs.yaml`.

### Spell trader example
```yaml
npcs:
  my_spell_mage:
    name: "Archmage Merlin"
    type: "spell_trader"
    sprite: "elf_warrior"
    render_category: "npc"       # a person; a 4-frame idle sheet animates automatically
    size_class: "person"         # people use the shared person size (config graphics.size_classes)
    dialogue:
      greeting: "Greetings, traveler!"
      insufficient_gold: "You need {cost} gold."
      already_known: "{name} already knows {spell}."
      success: "The knowledge flows into {name}."
    spells:
      fireball:
        cost: 500            # the ONLY required field per spell
```

Notes:
- The YAML key (`fireball`) is the spells.yaml SpellID. A catalog entry only
  authors the price: `name`, `school` and `description` are backfilled from
  spells.yaml at load
  (`backfillTraderSpells`); a missing `cost` fails the load.
- There are no spell-level or mastery requirements.
- A character must already have the matching magic school open to learn the spell.
- Dialogue strings are used. You can use `{name}`, `{spell}`, `{cost}` or printf-style placeholders.

### Merchant example (sell + buy)
```yaml
npcs:
  merchant_general:
    name: "Trader Marcus"
    type: "merchant"
    sprite: "elf"
    render_category: "npc"
    size_class: "person"
    sell_available: true
    dialogue:
      greeting: "Welcome to my shop!"
    inventory:
      - type: "potion"
        name: "Health Potion"
        cost: 50
        quantity: 10
      - type: "weapon"
        name: "Iron Sword"
        cost: 200
        quantity: 3
```

### Merchant example (sell only)
```yaml
npcs:
  desert_merchant:
    name: "Sahim the Wayfarer"
    type: "merchant"
    sprite: "merchant"
    render_category: "npc"
    size_class: "person"
    sell_available: true
    dialogue:
      greeting: "Spare tools and trinkets? I pay fair coin."
```

Notes:
- If `inventory` is empty, the left list shows "No stock for sale."
- If `sell_available: false`, the merchant won't buy your items.
- Inventory item names are resolved by display name from `items.yaml` or `weapons.yaml`.
- `type` is only special-cased for `"weapon"` (resolved via `weapons.yaml`); any other value (`"potion"`, `"armor"`, etc.) falls back to a name lookup in `items.yaml`.

### Encounter example
```yaml
npcs:
  bandit_camp:
    name: "Abandoned Shipwreck"
    type: "encounter"
    sprite: "shipwreck"
    render_category: "scenery"   # npc/wall_mounted/door/landmark/scenery/invisible
    size_class: medium_prop       # quantized visible height from config.yaml
    transparent: true
    dialogue:
      greeting: "You hear voices inside the wreck."
      visited_message: "The wreck is quiet now."
      choice_prompt: "What do you do?"
      choices:
        - text: "Leave"
          action: "leave"
        - text: "Attack"
          action: "combat"
    encounter:
      start_message: "Bandits burst from the wreck to attack!"
      quest_id: "shipwreck_bandits"          # optional: links a kill-quest
      quest_name: "Clear the Shipwreck"
      quest_description: "Defeat the bandits hiding in the wreck."
      monsters:
        - type: "bandit"   # monsters.yaml key
          count_min: 2     # NPC encounters roll a count in [min, max]
          count_max: 5
      rewards:
        gold: 200
        experience: 100
        completion_message: "The wreck falls silent."
      first_visit_only: true
```

Notes on encounters:
- Monsters are spawned dynamically near the NPC on trigger (count rolled in
  `[count_min, count_max]`). The encounter `type` (e.g. `bandit_camp`) is just
  a label - it is not branched on in code.
- `rewards` may also carry a `treasure_chest` (same shape as map encounters,
  see below) that spawns when the encounter is cleared.

### Other services and encounters

For quest offer/turn-in choices, state-dependent dialogue, and service unlocks,
use the [quest guide](docs/adding-quests.md). A new dialogue `action` needs an
implemented handler; arbitrary action names do not create mechanics.

`stock_refresh_weeks` refills authored finite stock on the calendar schedule.
`stock_weapons_rarity` with positive `stock_weapons_cost` supplies a generated
weapon rack; weapons with `no_loot` are excluded. Non-gold merchants cannot set
`sell_available: true`.

A `loot_crate` NPC gets its loot from `loots.yaml crates`, keyed by NPC ID.
A `spell_lectern` uses its `lectern` block. A `door` uses explicit `door_behavior`;
a champion portcullis is different from a key/stat-unlocked door. Copy a matching
shipped definition and validate its complete behavior block.

For pre-placed monster groups and reward chests, use map `clear_encounter` or
`clear_encounters`; see [maps](docs/adding-maps.md#encounters-and-respawning).

## Step 2: Place the NPC in a map
Example map line:
```
...@.....>[npc:my_spell_mage]
```
The `@` marks the NPC tile; the tag binds it to your NPC key.

## Testing checklist
- Run the [shared validation checklist](docs/content-authoring.md#verification).
- NPC appears at intended map location.
- Interaction works with `T`.
- Spell trader teaches priced spells to characters with the matching open school.
- Merchant buy/sell works as expected.
- Encounter spawns monsters and rewards properly.

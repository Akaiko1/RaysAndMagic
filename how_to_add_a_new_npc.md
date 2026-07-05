# How to Add a New NPC

NPCs are defined in `assets/npcs.yaml` and placed directly in map files.

## Overview
- Placement uses `@` in the map grid plus a trailing `>[npc:key]` definition on the same line.
- NPC sprites are looked up by name (no `.png` suffix).
- NPC types supported: `spell_trader`, `merchant`, `encounter`, `quest_giver`
  (dialogue with `give_quest`/`turn_in_quest` choices), `skill_trainer`.

## Step 1: Define the NPC
Add an entry under `npcs:` in `assets/npcs.yaml`.

### Spell trader example
```yaml
npcs:
  my_spell_mage:
    name: "Archmage Merlin"
    type: "spell_trader"
    sprite: "elf_warrior"
    size_multiplier: 4
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
  authors the price: `name`, `school`, `level`, `description` and
  `requirements` are backfilled from spells.yaml at load
  (`backfillTraderSpells`); a missing `cost` fails the load.
- Default requirement is the spell's own level; author an explicit
  `requirements:` block (`min_level`, `schools` list) only to override it.
- A character must already have the matching magic school to learn the spell.
- Dialogue strings are used. You can use `{name}`, `{spell}`, `{cost}` or printf-style placeholders.

### Merchant example (sell + buy)
```yaml
npcs:
  merchant_general:
    name: "Trader Marcus"
    type: "merchant"
    sprite: "elf"
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
    render_type: "environment_sprite"
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

### Map-clear encounters (alternative, no NPC)
A different mechanism lives in `assets/map_configs.yaml`, for monsters
PRE-PLACED in the `.map` grid (not spawned by an NPC). Killing the group drops
a treasure chest. Use `clear_encounter` (singular) to tie EVERY monster on the
map to one reward, or `clear_encounters` (plural) for several independent
groups - each declares `monsters: [{type, count}]` and a `treasure_chest`, and
the engine binds the `count` nearest monsters of each type to that chest:
```yaml
maps:
  desert:
    biome: "desert"
    clear_encounters:
      - monsters:
          - type: "bandit"
            count: 5
        rewards:
          completion_message: "The oasis is clear."
          treasure_chest:
            id: "desert_oasis_chest"
            tile_x: 12
            tile_y: 8
            sprite: "chest"
            gold: 500
```

## Step 2: Place the NPC in a map
Example map line:
```
%..@....%  >[npc:my_spell_mage]
```
The `@` marks the NPC tile; the tag binds it to your NPC key.

## Testing checklist
- NPC appears at intended map location.
- Interaction works with `T`.
- Spell trader teaches spells with requirements and proper dialogue.
- Merchant buy/sell works as expected.
- Encounter spawns monsters and rewards properly.

# How to Add a New NPC

NPCs are defined in `assets/npcs.yaml` and placed directly in map files.

## Overview
- Placement uses `@` in the map grid plus a trailing `>[npc:key]` definition on the same line.
- NPC sprites are looked up by name (no `.png` suffix).
- NPC types supported: `spell_trader`, `merchant`, `encounter`.

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
        name: "Fireball"     # must match spells.yaml name
        school: "fire"
        cost: 500
        requirements:
          min_level: 3
```

Notes:
- `spells.*.name` must match the display name in `assets/spells.yaml`.
- `requirements.min_level` and `requirements.min_water_skill` are enforced.
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
- If `inventory` is empty, the left list shows “No stock for sale.”
- If `sell_available: false`, the merchant won’t buy your items.
- Inventory item names are resolved by display name from `items.yaml` or `weapons.yaml`.

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
      monsters:
        - type: "bandit"   # monsters.yaml key
          count_min: 2
          count_max: 5
      rewards:
        gold: 200
        experience: 100
      first_visit_only: true
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

# How to Add a New NPC

NPCs are defined in `assets/npcs.yaml` and placed directly in map files.

## Overview
- Placement uses `@` in the map grid plus a trailing `>[npc:key]` definition on the same line.
- NPC sprites are looked up by name (no `.png` suffix).
- NPC types with gameplay logic: `spell_trader`, `merchant`, `encounter`.

## Step 1: Define the NPC
Add an entry under `npcs:` in `assets/npcs.yaml`.

Minimal spell trader example:
```yaml
npcs:
  my_spell_mage:
    name: "Archmage Merlin"
    type: "spell_trader"
    sprite: "elf_warrior"
    size_multiplier: 4
    spells:
      fireball:
        name: "Fireball"     # must match spells.yaml name
        school: "fire"       # used for class gating
        cost: 500
```

Key points for spell traders:
- `spells.*.name` must match the **display name** in `assets/spells.yaml`.
- `school` and `cost` are used; `level`, `spell_points`, `description`, and `requirements` are not enforced.
- Spell trader dialogue text is currently hardcoded in the UI.

## Step 2: Place the NPC in a map
Example map line:
```
%..@....%  >[npc:my_spell_mage]
```
The `@` marks the NPC tile; the tag binds it to your NPC key.

## NPC types and current support
- `spell_trader`: fully supported (teaches spells for gold)
- `merchant`: player can sell items; no buying or NPC inventory
- `encounter`: supported; uses `dialogue` choices and `encounter` data
- `quest_giver`: not wired beyond generic dialogue

## Encounter NPC example
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
      choice_prompt: "What do you do?"
      choices:
        - text: "Leave"
          action: "leave"
        - text: "Attack"
          action: "combat"
    encounter:
      type: "bandit_camp"
      monsters:
        - type: "bandit"   # monsters.yaml key
          count_min: 2
          count_max: 5
      rewards:
        gold: 200
        experience: 100
      first_visit_only: true
```
Notes:
- `monsters[*].type` must match a monster key in `assets/monsters.yaml`.
- `quest_id`, `quest_name`, `quest_description` are supported for encounter quests.

## Dialogue usage
- `encounter` and generic NPCs use `dialogue.greeting` and choice text.
- `spell_trader` and `merchant` greetings are hardcoded.

## Placement rules
`placement_rules` in `assets/npcs.yaml` are not enforced. NPCs are placed only via map files.

## Testing checklist
- NPC appears at the intended map location.
- Interaction works with `T`.
- Spell trader can teach a spell.
- Encounter spawns monsters and rewards properly.

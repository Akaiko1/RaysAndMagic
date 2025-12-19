# RaysAndMagic

RaysAndMagic is a first‑person, party‑based RPG prototype written in Go using Ebiten. Content is data‑driven via YAML for items, weapons, spells, monsters, tiles, maps, and NPCs.

## Overview

- First‑person 2D rendering (Simple raycasting‑style engine)
- 4‑member party with class‑based starting gear
- Real‑time and turn‑based exploration/combat modes
- Item/weapon/spell systems driven by YAML
- NPC system (spell traders, encounters, merchants)
- Status effects including Unconscious (0 HP) and a soft game‑over overlay
- Notepad‑friendly map format with inline NPC/special tile placement

## Build and Run

Requirements: Go 1.23+ (toolchain auto‑managed), Ebiten v2 (via go.mod).

```
go mod tidy
go run .
```

Build:
```
go build -o bin/raysandmagic .
```

Tests:
```
go test ./...
go test -race -cover ./...
```

## Controls

- WASD / Arrow Keys: Move/turn
- Q / E: Strafe left/right
- Space: Melee attack (in combat) / acts in turn‑based
- F: Cast equipped spell (in combat)
- H (with mouse): Targeted heal when a heal spell is equipped
- 1–4: Select party member
- /: Toggle FPS
- M / I / C: Open Spellbook / Inventory / Characters
- T: Interact with nearby NPCs
- ESC: Exit or close dialogs/menus
- Return/Enter: Turn-based/Real-time

## Gameplay Basics

### Party and Equipment
- Starting party: Knight, Sorcerer, Cleric, Archer.
- Class equipment (equipped): Iron Sword (Knight), Magic Dagger (Sorcerer), Holy Mace (Cleric), Hunting Bow (Archer).
- Starting inventory (shared): Leather Armor, Health Potion, Revival Potion, Magic Ring, Iron Spear.

### Combat
- Real‑time mode: free movement; press Space/F to attack/cast.
- Turn‑based mode: party and monsters act in rounds; action count is limited per round.
- Armor is additive from multiple pieces and reduces damage by AC/2.

### Spells
- Defined in `spells.yaml` with damage/heal cost and visuals.
- Damage scales with effective Intellect; healing scales with effective Personality.

### Items
- Items are defined in `assets/items.yaml` and carry attributes:
  - Armor: `armor_class_base`, `endurance_scaling_divisor`.
  - Accessories: `intellect_scaling_divisor`, `personality_scaling_divisor`, `bonus_might`.
  - Consumables: `heal_base`, `heal_endurance_divisor`, `summon_distance_tiles`, `revive`, `full_heal`.
  - Value: integer gold value for tooltips/merchant pricing.
- Revival Potion: cures Unconscious and Dead; fully restores HP.
  - Normal healing potions do not remove Unconscious.

### Status & Game Over
- Unconscious: applied at 0 HP. Characters cannot attack/cast, portraits darken, SP regen pauses, equipping is blocked. Direct heal spells or a Revival Potion remove it.
- Game Over: when all party members are at 0 HP, a game‑over overlay appears. Press N for a soft new game (recreate party, move to spawn) or L to load.

### Merchants
- Merchant dialog lists party inventory for selling. Double‑click an item to sell it for its `value`.

## Content and Configuration

- Items: `assets/items.yaml` (non‑weapons). Fields include `equip_slot`, `value`, and attributes described above.
- Weapons: `assets/weapons.yaml`. Includes damage, range, scaling, graphics/physics, and `value` for selling.
- Spells: `assets/spells.yaml`. Defines spell damage/heal costs and render/physics.
- Monsters: `assets/monsters.yaml` with stats and spawn letters.
- Tiles & Special Tiles: `assets/tiles.yaml`, `assets/special_tiles.yaml`.
- Maps: `assets/*.map` (ASCII); per‑map config in `assets/map_configs.yaml`.
- NPCs: `assets/npcs.yaml` (spell traders, encounters, merchants). Place in maps with `@` markers and `>[npc:key]`.
- Loot tables: `assets/loots.yaml` (per‑monster drop entries).

## Map Format (summary)

Maps are ASCII files. Non‑comment lines are equal width.
- Place monsters using lowercase letters from `assets/monsters.yaml`.
- Place NPCs/special tiles using `@` in the grid and list placements after `>` at line end:

```
.............@...........@.........  >[stile:rteleporter], [npc:shipwreck_bandit_camp]
```

Common tile letters (biome‑dependent):
`. # W Q D ^ T A % R M S P F C + - = X`

## Project Structure

- `main.go`: entry point (loads configs, sets up bridges, creates world/party/game)
- `assets/`: maps, sprites, YAML configs
- `internal/`:
  - `game`: combat, input, UI, renderer, game loop
  - `character`: party, classes, statuses, skills
  - `items`: item types, creation from YAML
  - `spells`: spell system and casting
  - `monster`: YAML‑driven monsters and AI
  - `world`: maps, tiles, loading, placement
  - `bridge`: adapters between config and runtime structs (avoid cycles)
  - `config`: YAML loaders and accessors
  - `graphics`, `threading`, `collision`: rendering and engine utilities

## Development Notes

- Go formatting/vetting: `go fmt ./... && go vet ./...`
- Testing: `go test ./...` (use `-race -cover` when needed)
- Data‑driven approach: add/adjust YAML and consume attributes in code (avoid or limit name‑based checks).
## Map Editor

Design handcrafted maps with a simple, notepad‑friendly text format. Each line is a row of tiles; comments start with `#`. All non‑comment lines must be the same width.

- File: place your `.map` under `assets/` and reference it in `assets/map_configs.yaml`.
- Start: include exactly one `+` for the party start tile (TileSpawn).
- Monsters: use lowercase letters (see “Monster Letters”) directly in the map grid.
- NPCs and Special Tiles: place `@` on the map, then append placements at the end of the line using markers:
  - `>[npc:key]` for an NPC from `assets/npcs.yaml`
  - `>[stile:key]` for a special tile from `assets/special_tiles.yaml`
  - Multiple placements on the same line are separated by `, ` (comma + space). Each placement is matched in order to the `@` symbols on that line.

Example (teleporter + NPC on one line):
```
.............@...........@.........  >[stile:rteleporter], [npc:shipwreck_bandit_camp]
```

### Tile Letters (from assets/tiles.yaml)

Letters can be biome‑specific. For example, `T` means a forest tree in the “forest” biome, a dune in the “desert” biome, and coral in the “water” biome. The loader resolves letters per the current biome.

- .: empty (walkable floor)
- #: wall (stone wall)
- W: water (oasis/water)
- Q: deep_water
- D: door (closed)
- ^: stairs
- T: forest tree (forest biome) / desert_dune (desert) / coral_reef (water)
- A: ancient_tree (forest) / large_dune (desert) / large_coral (water)
- %: thicket (dense undergrowth)
- R: moss_rock (solid but transparent to sight)
- M: mushroom_ring (walkable)
- S: forest_stream (walkable water)
- P: fern_patch (walkable)
- F: firefly_swarm (walkable, visual)
- C: clearing (open grass/sand)
- +: spawn (starting position)
- -: low_wall (short wall)
- =: high_wall (tall wall)
- X: crystal (environment object)

Special teleporter tiles are not letters; place them via `@` + `>[stile:key]`:
- stile keys: `vteleporter` (violet), `rteleporter` (red)

### Monster Letters (from assets/monsters.yaml)

Place these lowercase letters directly in the map to spawn monsters at that tile.

- g: goblin
- o: orc
- w: wolf
- b: bear
- s: spider
- k: skeleton
- l: troll
- d: dragon
- f: forest_orc
- u: dire_wolf
- i: forest_spider
- t: treant
- p: pixie
- n: bandit

Notes
- Biome resolution: the same letter can map to different tiles depending on map biome (forest/desert/water), configured in `assets/tiles.yaml` and selected per‑map in `assets/map_configs.yaml`.
- Visibility and collision come from tile properties: `solid`, `transparent`, `walkable`.
- Keep lines consistent: the loader rejects maps where non‑comment lines differ in length.

# RaysAndMagic

First-person party-based RPG prototype written in Go using Ebiten.

## Features

- 2D first-person view (raycasting style)
- 4-character party
- Forest map (`assets/forest.map`)
- Basic movement and collision (WASD/arrow keys)
- Turn-based and real-time combat (Space)
- Multiple monster types
- Simple UI (party stats, FPS counter, menus)
- NPC system (traders, spellcasters, etc.)
- Many terrain types (trees, water, walls, magical areas, etc.)
- Map is constructed in a notepad-friendly text format for easy editing

## Controls

- WASD / Arrow Keys: Move, turn
- Q / E: Strafe left/right
- Space: Melee attack (in combat) / Toggle real-time and turn-based combat
- F: Cast equipped spell (in combat)
- 1–4: Select party member
- /: Toggle FPS counter
- M: Open/close Spellbook tab
- I: Open/close Inventory tab
- C: Open/close Character tab
- T: Interact with NPC
- ESC: Exit game

## Weapons (starting)

- Iron Sword (Knight)
- Magic Dagger (Sorcerer)
- Holy Mace (Cleric)
- Hunting Bow (Archer)
- Silver Sword (Paladin)
- Oak Staff (Druid)

## Spells (starting)

- Fire School (Sorcerer): Torch Light, Fire Bolt, Fireball
- Body School (Cleric): First Aid, Heal
- Air School (Archer): Wizard Eye
- Spirit School (Paladin): Bless
- Water School (Druid): Awaken

## Run

Requires Go 1.21+ and Ebiten v2.8.8.

```
go mod tidy
go run main.go
```

## Structure

- `main.go` — Entry point
- `assets/` — Maps, sprites, config
- `internal/` — Game logic, entities, rendering

## MAP EDITOR

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

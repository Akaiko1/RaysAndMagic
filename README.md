# RaysAndMagic

A retro first-person party RPG built with Go and Ebiten. Lead a four-member party through raycasted dungeons, fight in real-time or turn-based combat, and slay the four dragons to save the realm.

![Spellbook UI with bookmarks and spell cards](src/spellbook.png)
![Turn-based combat with bandits](src/turn_based_combat.png)
![Dragon boss battle](src/dragon_battle.png)
![Forest biome screenshot](src/forest.png)
![Desert biome screenshot](src/desert.png)
![Shipwreck encounter screenshot](src/shipwreck.png)

## Features

- First-person raycasting engine with sprite-based monsters and NPCs
- 4-member party with distinct classes (Knight, Sorcerer, Cleric, Archer)
- Hybrid combat: switch between real-time and turn-based modes
- YAML-driven content: items, weapons, spells, monsters, quests, and maps
- NPC interactions: merchants, spell traders, and encounter triggers
- Victory system with score tracking and local high scores

## Quick Start

**Requirements:** Go 1.23+, Ebiten v2

```bash
go mod tidy
go run .
```

**Build:**

```bash
mkdir -p bin
go build -o bin/raysandmagic .
go build -o bin/map_viewer ./assets/map_viewer
```

The game and map viewer both locate `config.yaml`/`assets/` next to the binary or one directory above it, so local `bin/` builds can run against the repo-root data files.

**Release bundles:**

```bash
./build_mac_release.sh
```

Release archives include both the game executable and `RaysAndMagicMapViewer` for macOS Intel, macOS Apple Silicon, and Windows.

## Controls

| Key           | Action                             |
| ------------- | ---------------------------------- |
| WASD / Arrows | Move and turn                      |
| Q / E         | Strafe                             |
| Space         | Melee attack / confirm action      |
| F             | Cast equipped spell                |
| H + click     | Targeted heal                      |
| 1-4           | Select party member                |
| Tab           | Toggle real-time / turn-based      |
| I / C / M     | Inventory / Characters / Spellbook |
| T             | Talk to nearby NPC                 |
| ESC           | Menu / close dialogs               |

## Project Structure

```text
├── main.go              # Entry point
├── assets/              # Game data (YAML configs, maps, sprites)
│   ├── *.yaml           # Items, weapons, spells, monsters, quests, NPCs
│   ├── *.map            # ASCII map files
│   └── sprites/         # Character and monster sprites
└── internal/            # Game packages
    ├── game/            # Core game loop, combat, UI, rendering
    ├── character/       # Party, classes, stats, equipment
    ├── monster/         # Monster AI and configuration
    ├── items/           # Item system
    ├── spells/          # Spell casting system
    ├── quests/          # Quest tracking
    ├── world/           # Map loading and tile system
    └── config/          # YAML loaders
```

## Content Files

| File               | Purpose                                | Guide                                              |
| ------------------ | -------------------------------------- | -------------------------------------------------- |
| `items.yaml`       | Armor, accessories, consumables        |                                                    |
| `weapons.yaml`     | Melee and ranged weapons               | [Adding a weapon](how_to_add_a_new_weapon.md)      |
| `spells.yaml`      | Magic spells with damage/healing       | [Adding a spell](how_to_add_a_new_spell.md)        |
| `monsters.yaml`    | Monster stats, AI, and map letters     | [Adding a monster](how_to_add_a_new_monster.md)    |
| `quests.yaml`      | Quest definitions and rewards          |                                                    |
| `npcs.yaml`        | NPCs (merchants, trainers, encounters) | [Adding an NPC](how_to_add_a_new_npc.md)           |
| `loots.yaml`       | Monster drop tables                    |                                                    |
| `tiles.yaml`       | Tile types per biome                   | [Adding a tile](how_to_add_a_new_tile.md)          |
| `map_configs.yaml` | Per-map settings + per-biome floor textures + clear-encounter chests |                                       |

### Adding new content

Step-by-step guides for the most common content additions:

- [How to add a new weapon](how_to_add_a_new_weapon.md)
- [How to add a new spell](how_to_add_a_new_spell.md)
- [How to add a new monster](how_to_add_a_new_monster.md)
- [How to add a new NPC](how_to_add_a_new_npc.md)
- [How to add a new tile](how_to_add_a_new_tile.md)

## Map Format

Maps are ASCII files where each character represents a tile or entity:

- `.` Floor / `#` Wall / `+` Player start
- `T` Tree (biome-dependent) / `W` Water / `D` Door
- Lowercase letters spawn monsters (e.g., `w` = wolf, `d` = dragon)
- `@` marks NPC/special tile positions, defined at line end with `>[npc:key]`

## Development

```bash
go fmt ./... && go vet ./...   # Format and lint
go test ./...                   # Run tests
```

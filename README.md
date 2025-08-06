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
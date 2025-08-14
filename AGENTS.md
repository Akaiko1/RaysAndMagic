# Repository Guidelines

## Project Structure & Module Organization
- Entry point: `main.go`. Core packages live under `internal/`:
  `game`, `world`, `character`, `monster`, `items`, `spells`, `graphics`, `threading`, `collision`, `config`, and `bridge`.
- Game data and art in `assets/` (maps, sprites, YAML configs). Runtime configs in `config.yaml`, plus data files like `spells.yaml` and `weapons.yaml`.
- Tests live next to code as `*_test.go` in `internal/` and integration tests in `test/`.

## Build, Test, and Development Commands
- Run locally: `go run .`
- Build (local): `go build -o bin/raysandmagic .`
- Windows builds: `pwsh ./build_debug_console.ps1` or `pwsh ./build_no_console.ps1`
- Unit and integration tests: `go test ./...`
- With race/coverage: `go test -race -cover ./...`

## Coding Style & Naming Conventions
- Format and vet before committing: `go fmt ./... && go vet ./...`
- Use idiomatic Go:
  - Packages: short, lowercase (e.g., `world`, `spells`).
  - Files: lowercase with underscores (e.g., `world_manager.go`).
  - Exported identifiers: CamelCase; receivers are short (e.g., `w *World`).
- Keep packages cohesive; avoid name stutter (e.g., `monster.Stats`, not `monster.MonsterStats`).

## Testing Guidelines
- Prefer table-driven tests; keep unit tests close to implementations in `internal/<pkg>/*_test.go`.
- Integration tests belong in `test/` and may exercise Ebiten-driven flows where feasible.
- Aim for meaningful coverage on combat, world generation, and I/O parsing. Run `go test -cover ./...` before PRs.

## Commit & Pull Request Guidelines
- Commits: imperative, concise, scoped (e.g., "add world chunk culling"). Group related changes.
- PRs must include: summary, motivation/approach, testing notes (`go test` output), and screenshots/GIFs for gameplay/UI changes. Link issues when applicable.

## Content & Data Changes
- When adding monsters, NPCs, spells, tiles, or weapons, follow the how-to guides in the repo (e.g., `how_to_add_a_new_monster.md`). Keep YAML schemas consistent and validate by running the game.
- Do not commit secrets or large binaries. Prefer referencing assets under `assets/` and JSON saves (`save*.json`) only for debugging.

## YAML-Driven Items
- File: `assets/items.yaml` now drives non-weapon items with optional numeric stats.
- Schema fields:
  - `name` (string), `type` (armor|accessory|consumable|quest), `description` (string)
  - Optional: `armor_class_base` (int), `endurance_scaling_divisor` (int), `intellect_scaling_divisor` (int), `personality_scaling_divisor` (int)
- Runtime wiring:
  - `main.go` loads with `config.MustLoadItemConfig("assets/items.yaml")`.
  - `bridge.SetupItemBridge()` sets `items.GlobalItemAccessor` → `config.GetItemDefinition`.
  - `items.CreateItemFromYAML(key)` maps YAML into `items.Item` and populates `Item.Attributes` with configured numbers.
- Usage in game logic:
  - Character bonuses: `internal/character/character.go` → `calculateEquipmentBonuses` uses `intellect_scaling_divisor` and `personality_scaling_divisor` for accessories; `endurance_scaling_divisor` for armor.
  - Damage reduction: `internal/game/combat.go` → `ApplyArmorDamageReduction` uses `armor_class_base` and `endurance_scaling_divisor` from equipped armor.
  - Tooltips: `internal/game/item_tooltip.go` renders armor/accessory details from attributes.
- Adding new item stats:
  - Extend `internal/config/ItemDefinitionConfig` with new YAML fields.
  - Mirror fields in `internal/items/ItemDefinitionFromYAML` and pass via `internal/bridge/item_bridge.go`.
  - Populate into `Item.Attributes` in `items.CreateItemFromYAML` with stable, snake_case keys.
  - Consume those attributes in the relevant systems (character, combat, tooltips) rather than matching on item names.
- Backward compatibility:
  - Existing items without optional fields default to no bonus; prefer updating YAML to define effects explicitly.
  - Avoid reintroducing name-based checks in gameplay code; attributes are the source of truth.
- Testing:
  - Run `go test ./...` and check character/combat tests; loot and armor reduction tests should remain green with YAML-driven items.

# How to Add a New Monster

Monsters are defined in YAML and driven by runtime config.

## Overview
- Monsters live in `assets/monsters.yaml`.
- Loot tables are in `assets/loots.yaml`.
- Map placement uses a single lowercase letter in `.map` files.
- Radii in `monsters.yaml` are in tiles (1 tile = 64px).
- `size_multiplier` multiplies render size and stacks with `graphics.monster.size_distance_multiplier`.

## Step 1: Define the monster
Add a new entry under `monsters:` in `assets/monsters.yaml`.

Minimal example:
```yaml
monsters:
  ice_troll:
    name: "Ice Troll"
    level: 7
    max_hit_points: 95
    armor_class: 11
    experience: 350
    damage_min: 4
    damage_max: 20
    alert_radius: 3        # tiles
    attack_radius: 1       # tiles
    speed: 1.1
    gold_min: 25
    gold_max: 80
    sprite: "goblin"       # assets/sprites/mobs/goblin.png
    letter: "a"            # lowercase, unique in its biome scope (see "Biome restriction")
    box_w: 40              # keep < 64 (tile size) or it can't fit 1-wide corridors
    box_h: 40
    size_multiplier: 2.0
    resistances: {}
    habitat_preferences:
      - "empty"
```

Key requirements:
- `letter` must be lowercase (map spawns recognize a-z). It must be unique
  within its biome scope - see "Biome restriction" below.
- `sprite` must exist in `assets/sprites/mobs/` (without `.png`).
- `alert_radius` and `attack_radius` are in tiles.

## Biome restriction (optional)
Add a `biomes` list to restrict a monster to specific map biomes. Omit it for
a universal monster that can appear in any biome.
```yaml
  medusa:
    # ...stats...
    letter: "m"
    biomes: ["water"]   # only spawns on maps whose biome is "water"
```
A map letter is resolved biome-aware (`GetMonsterByLetterForBiome`): a
biome-specific monster wins for that biome, and a universal monster (no
`biomes`) is the fallback. So the SAME letter may map to different monsters in
different biomes - `letter` only has to be unique among monsters sharing a
biome (and among the universal set). A letter on a map whose biome no monster
matches resolves to nothing (no spawn).

## Step 2: Add a sprite
Place a PNG in `assets/sprites/mobs/` that matches the `sprite` key.

## Step 3: Optional ranged attacks
```yaml
  bandit:
    projectile_weapon: "throwing_knife"   # weapons.yaml key
    ranged_attack_range: 2                # tiles
```
Or spell-based:
```yaml
  pixie:
    projectile_spell: "firebolt"          # spells.yaml key
    ranged_attack_range: 3                # tiles
```

## Step 4: Optional special effects
Supported fields (from `monsters.yaml`):
- `perfect_dodge`
- `fireburst_chance`, `fireburst_damage_min`, `fireburst_damage_max`
- `poison_chance`, `poison_duration_seconds`
- `flying`
- `passive_until_attacked` (won't aggro until struck)
- `pounce_range_tiles`, `pounce_cooldown_seconds` (leap to melee from range, e.g. puma)
- `attack_cooldown_multiplier`, `attacks_per_round`

### Boss kit (all data-driven; see golden_thief_bug)
- `ignores_armor` - melee bypasses party armor class
- `inferno_chance` + `inferno_damage` - party-wide fire nova
- `teleport_at_hp` + `teleport_chance` - low-HP blink to a random tile
- `passive_until_quest` + `evade_radius_tiles` + `boss_cooldown_seconds` -
  evades (blinks away, never attacks) until the named quest completes

Paired boss fields are validated at load: a chance without its magnitude (or an
evasive phase without its tuning) fails startup. Beware: any OTHER unknown YAML
field is silently ignored - a typo won't error, the feature just won't work.

## Step 5: Add loot
Loot is controlled by `assets/loots.yaml` using the monster key.
Loot drops into a bag on the ground and must be picked up by the player.

Example:
```yaml
loots:
  ice_troll:
    - type: "weapon"
      key: "elven_bow"
      chance: 0.05
    - type: "item"
      key: "iron_armor"
      chance: 0.10
```

## Step 6: Place in a map (optional)
In any `.map` file, place the monster letter in the ASCII grid:
```
%..v.....%   # v spawns ice_troll
```
Map placement overrides habitat rules for that tile.

## Habitat rules
`habitat_preferences` and `habitat_near` use tile keys defined in the `tile_types` section at the bottom of `assets/monsters.yaml`. If you add new tile keys, update `tile_types` accordingly.

## Testing checklist
- YAML loads without errors.
- `letter` is unique and lowercase.
- Sprite file exists.
- Monster spawns and behaves correctly.
- Loot drops into a bag when killed.

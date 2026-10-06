# Adding items and loot

Read [shared authoring rules](content-authoring.md) first. Items live in
`assets/items.yaml`, weapons in their own catalog, and drop sources in
`assets/loots.yaml`. Keep these identities separate from display names.

## Define an item

This example contains an accessory, a consumable, and a trade good:

```yaml
items:
  pathfinder_ring:
    name: "Pathfinder Ring"
    type: accessory
    equip_slot: ring
    description: "A copper band engraved with an old road."
    bonus_speed: 2
    rarity: uncommon
    value: 60
  field_draught:
    name: "Field Draught"
    type: consumable
    description: "Bitter herbs in a travel flask."
    heal_base: 20
    heal_endurance_divisor: 2
    rarity: common
    value: 25
  polished_pebble:
    name: "Polished Pebble"
    type: trinket
    description: "A river stone with a silver seam."
    rarity: common
    value: 5
```

Equipment uses `equip_slot`, armor category, numeric stats, and resistances.
Consumables use supported attributes such as healing, mana restoration, revival,
or timed buffs. Timed item resistance uses `resist_buff_school_pct`, not the
spell-only `resist_buff_pct`, and requires the complete duration/icon contract.
See matching shipped draughts before adding one.

Sets live in the same file under `item_sets`; pieces use `set: <key>`.
`pieces_required` counts equipped pieces on one wearer. For a mixed item/weapon
set use `required_pieces` with exact keys. Cards use `type: card` and supported
`card_*` attributes; their bonuses apply through the collection system.

Add `assets/sprites/interface/items/icon_item_<key>.png` for a normal catalog
item. Special inventory types (weapons, spells, traps) have their own icon lookup;
`itemTooltipIconName` in `internal/game/ui_helpers.go` owns this mapping.
Gameplay-neutral `description`/`flavor` and optional `tooltip_effects`/
`tooltip_usage` do not implement mechanics. Check both editor and inventory
tooltips; common usage wording lives in `tooltip_usage_defaults`.

## Choose the drop model

The following standalone example uses existing shipped items so its references
resolve without first installing the examples above:

```yaml
loots:
  goblin:
    - type: item
      key: health_potion
      chance: 0.15
      rolls: 1
loot_tables:
  trail_supplies:
    rolls: 2
    gold_min: 5
    gold_max: 15
    entries:
      - type: item
        key: health_potion
        weight: 3
      - type: weapon
        key: iron_sword
        weight: 1
```

| Source | Meaning |
| --- | --- |
| `loots.<monster key>` | Each entry rolls independently; `chance` is 0..1, omitted `rolls` means one attempt |
| `boss_loot` | Entries appended to each YAML-classified boss's normal drops |
| `loot_tables.<pool>` | Each roll makes a weighted pick; weights are relative, not percentages |
| `crates.<NPC key>` | Loot/trap behavior for a `loot_crate` NPC; use a named `loot_table` or complete `roll_sources` |

Monster drops go into a ground bag. An encounter's `treasure_chest` can reference
a named `loot_table`, or explicit `items`/`weapons` keys.

A crate either rolls a named `loot_table` or fills `rolls` slots from
`roll_sources`. Each slot picks one source by `weight` and draws from it.
Weights are percents in 0.1 steps, and a crate's weights total 100.

| `pool` | One slot draws | Fields |
| --- | --- | --- |
| `map` | one entry from the drop tables of the monsters the map (or open-world region) was created with | optional `rarity` |
| `catalog` | a uniform pick of `item_type`: armor, accessory, consumable, trinket, weapon or any | `item_type`, optional `rarity` |
| `loot_table` | one roll of that named pool, items and gold as authored | `loot_table` |
| `gold`, `arena_points` | a fixed sum, paid again by every slot that picks it | `amount` |
| `nothing` | an empty slot | none |

`rarity` is `rare` (exactly), `common-uncommon` (a span) or `uncommon+` (that
tier and up). A `map` source with nothing in its span on the current map sits
out, and its share goes to the others. A source with share w over n slots shows
up in 1-(1-w)^n of the chests. The catalog never rolls the unique tier, quest
items or `no_loot` weapons; a named `loot_table` may still list them. The loader
is strict: an unknown or removed field (such as the old `special_rolls`) fails
the boot. Copy a shipped crate with the intended pool and trap behavior,
including paired fields.

To make a new item obtainable, add its key to a chosen source after installing
the definition. Merchant `inventory` instead uses the exact display name and
`type: weapon` for weapons; other stock resolves through the item catalog.
Ordinary quest `rewards.item_pool` picks one item key. See [NPCs](../how_to_add_a_new_npc.md)
and [quests](adding-quests.md). Keep names unique: stacking, stock, and some
currency paths depend on them.

## Verify

Follow [shared verification](content-authoring.md#verification). Acquire the
item through its intended source, equip/use/sell it as applicable, and reload a
save. Check stack behavior, finite stock, set activation/removal, and that a
generated pool does not accidentally include restricted rewards.

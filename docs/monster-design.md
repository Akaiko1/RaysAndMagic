# Monster design: levels, stats and balance

This guide sets a monster's numbers: level, HP, damage and experience. It
also covers how to check them in play. For the YAML fields, sprites and
placement, read [How to add a new monster](../how_to_add_a_new_monster.md).

The curves were fitted on the shipped monsters, then corrected where the old
balance went flat too early (2026-10). Regular monsters and bosses have
separate curves. A monster may leave its curve only as a named archetype.

## What a level means

`level` is a label for the monster's strength. It changes almost nothing in
play. When Sleight of Hand finds no item, it lifts more gold from a monster
above level 5. The editor's Mobs page sorts by level. The balance sweep picks
its opponents by level, and the bench builds treat a dropped item as
obtainable from its dropper's level. Experience is authored separately.

So the level must describe the stats, not the zone the monster lives in. A
level-15 label on level-25 stats breaks the balance tools and misleads anyone
reading the catalog.

## Stat curves

Regular HP grows about 13% per level up to level 25, then about 4% per level:

    HP(L) = exp(4.181 + 0.1255 * min(L, 25) + 0.04 * max(0, L - 25))

Damage is per hit: the average of `damage_min` and `damage_max`. It grows
about 6% per level. A monster is ranged when it has a `projectile_spell` or
a `projectile_weapon`:

    melee damage(L)  = exp(2.634 + 0.057 * L)
    ranged damage(L) = melee damage(L) * 1.10

Boss HP grows about 10.6% per level. The curve pivots at level 18, where
the first boss (`golden_thief_bug`, 1200 HP) sits. It reaches about 7100 at
level 35 (it was about 4300 before the 2026-10-02 retune):

    boss HP(L) = exp(5.333 + 0.101 * L)

Boss damage is about 0.85x the melee curve. A boss is dangerous because of
its HP and its kit (summons, novas, enrage, blinks), not because of bigger
hits.

| Level | Regular HP | Melee damage | Ranged damage | XP target | Boss HP | Boss damage |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 74 | 15 | 16 | 27 | - | - |
| 5 | 123 | 19 | 20 | 133 | - | - |
| 10 | 230 | 25 | 27 | 267 | - | - |
| 15 | 430 | 33 | 36 | 480 | - | - |
| 18 | 626 | 39 | 43 | 691 | 1275 | 33 |
| 20 | 805 | 44 | 48 | 853 | 1561 | 37 |
| 25 | 1508 | 58 | 64 | 1333 | 2586 | 49 |
| 30 | 1842 | 77 | 85 | 1920 | 4286 | 65 |
| 35 | 2250 | 102 | 113 | 2613 | 7101 | 87 |

Keep `damage_min` and `damage_max` at about 0.8x and 1.2x of the average,
as the shipped monsters do. A double striker (`attacks_per_round: 2`) keeps
its per-hit damage on the curve, so it deals twice the curve each turn.

`enrage_at_hp` and `teleport_at_hp` are absolute HP values. When you change
`max_hit_points`, scale them by the same factor so they trigger at the same
fraction of HP.

## Experience

`config.yaml` sets the pace: a four-hero party needs about 15 kills of its
own level per level-up. Kill experience is split by party size, so the
target for one regular kill is:

    XP target(L) = 4 * max(100 * L, 8 * L * L) / 15

The shipped monsters pay against it as follows:

| Monsters | Pay |
| --- | --- |
| Regular, level 13 and up | about 1.5x the target (about 10 kills per level) |
| Bosses | about 3x |
| Clock Tower monsters (they respawn every 3 days) | 0.5x-0.7x |
| Starters, levels 1-5 | about 0.7x |
| Boss-only adds | nothing |

## Archetypes and outliers

A monster is on its curve when its HP and its damage are each between 0.67x
and 1.5x of its archetype's values. If it is outside that band, it must
belong to another archetype. Name the archetype in a YAML comment next to the
stats.

| Archetype | HP | Damage per hit | Shipped examples |
| --- | --- | --- | --- |
| Melee | curve | curve | `ocelot`, `kappa`, `dire_wolf` |
| Ranged | curve | ranged curve | `kitsune_onmyoji`, `masked_hexer_girl`, `lich_king` |
| Young dragon | about 1.4x | about 0.9x | `dragon`, `dragon_red`, `dragon_gold`, `dragon_green` |
| Elder dragon | about 1.9x | about 0.75x | the four `elder_dragon*` |
| Tank | armor class and resistances far above its level | curve or lower | `treant` (AC 33 and 30% physical resistance at level 8) |
| Starter (levels 1-4) | 0.5x-1.2x | mostly 0.25x-0.6x | `goblin`, `spider`, `wolf` |
| Boss escort | curve at its boss's level | below the curve (open decision) | `deathbound_mummy`, `enforcer_alien` |
| Boss | boss curve | about 0.85x | `orc_hero_boss`, `dragon_brood_mother` |

Boss escorts carry their boss's level;
`TestEndgameBossesSummonDedicatedLevelMatchedAdds` holds that rule.

## One-shots

Any monster attack may become an Elemental Attack, which deals double damage
20% of the time (`monster_combat.elemental_attack` in `config.yaml`). A
regular monster's `damage_max`, doubled, should stay below the HP of the
weakest bench hero of its level. Otherwise that hero can die from full health
in one hit. Armor is not counted, so the limit is a safe one.

| Level | Weakest bench hero HP | Keep `damage_max` under |
| --- | --- | --- |
| 15 | about 87 | 43 |
| 20 | about 102 | 51 |
| 25 | about 131 | 65 |
| 30 | about 140 | 70 |

Damage grows exponentially, but hero HP grows about linearly. From level 20
on, the melee curve's top hit (1.2x the average) already reaches this limit;
see Open decisions.

## Encounters

The balance sweep reads its encounters from the world, so placement is part
of the design:

- **Single:** each monster alone. A boss still calls its summons.
- **Pack:** one kind in its usual number. Monsters of one kind within 6 tiles
  of each other on a map count as a pack, and so do the day/night packs in
  `config.yaml`.
- **Mixed:** different kinds within 6 tiles of each other, including a boss
  and its guards.

A group takes its strongest member's level. Three ranged casters beside a
boss make a harder fight than the boss alone, and the sweep measures that
fight too. Do not balance a monster only alone.

## Abilities must announce themselves

Every authored ability writes a combat-log line when it fires: "X fires a
Piercing Shot!", "X breathes ...", "X roots the party in place!", "X rips Y
from the party!". Enrage is the exception: the sweep reads it from the
monster's state.

The sweep's coverage report matches these lines. If a monster fights but one
of its abilities never fires, the report lists that ability under NEVER. A new
ability needs both a log line and a row in `balanceAbilities`
(`internal/game/balance_coverage_test.go`).

## Checking a monster

The sweep sends every [bench party](test-scenarios.md#benchmark-parties)
(levels 15, 20, 25 and 30) against every encounter within five levels of
its own. The fights are turn-based and run through the real combat loop:

    go test -tags balance ./internal/game -run TestBalanceSweep -v

`BALANCE_LEVEL`, `BALANCE_PARTY` and `BALANCE_KIND` narrow the run, and
`BALANCE_TRIALS` sets the fights per cell (default 10). `BALANCE_OUT` writes
every cell as JSON. A full sweep takes under a minute.

The party plays like a sensible player:

- Before the fight it casts its buffs, summons its allies and turns on the
  Wayfarer's autocast.
- It smart-attacks the nearest enemy. That one action heals a wounded ally,
  casts the readied spell, throws the readied flask, arms the readied trap,
  or strikes.
- If most of the party fights in melee, it advances first.
- It revives a fallen hero from a quick slot.
- Heroes drink potions on their own below 35% HP (`characters.auto_drink`),
  as in the game.

Monsters notice the party by sight or when they are hit. Banding and alarm
rallies work as they do on the maps.

The report flags each cell that falls outside its band:

| Hole | Rule |
| --- | --- |
| too hard | at or below the party's level: under 95% wins, or 0.5+ KOs per fight; above it: under 50% wins. Bosses: under 80% and under 30% |
| trivial | a pack 3+ levels above the party: won in 2 rounds or less, losing under 5% HP. A boss at or above the party's level: the same, under 15% HP |
| stalemate | half the fights reach the 60-round cap |

The report also lists:

- composition gaps (one party wins an encounter that another loses);
- monster abilities that never fired;
- what each class actually did;
- for each cell, the biggest hit as a share of a hero's HP and the one-shots
  per fight.

The sweep does not cover these yet:

- real-time combat, whose mechanics differ and need their own pass;
- quick-slot spells other than revives;
- spell choice that takes resistances into account.

## Open decisions

- **Late one-shots.** Once doubled by an Elemental Attack, late hits outgrow
  hero HP (see One-shots). The options: a smaller elemental multiplier late in
  the game, a cap on regular damage, or more hero HP.
- **Escort damage.** The escorts keep their original damage, 0.55x-0.62x of
  the curve. The full curve would make them one-shot casters.
- **Ranged HP.** Ranged monsters get extra damage but keep regular HP. Giving
  them less HP has not been decided.

## Workflow for a new monster

1. Pick the archetype and the level its stats should mean.
2. Read HP, damage and experience off the curves for that archetype.
3. Add the kit. Each ability needs its log line and its paired fields.
4. Place the monster: its pack size and its neighbours make its encounters.
5. Run the sweep at its levels and read its cells: the holes, the biggest
   hits and its coverage line.
6. If you relabel a monster that drops gear, run
   `TestBenchPartiesBuildAsAuthored`. It fails when a bench party wears gear
   that the new level puts out of reach.

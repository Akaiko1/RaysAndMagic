# Achievements and player statistics

Achievement names, artwork keys and unlock conditions are defined in
`assets/achievements.yaml`. Each achievement unlocks once per player profile.

| Achievement | Unlock condition |
| --- | --- |
| First Steps | Leave Seabright (`city`) through a successful map transition |
| First Blood | Defeat an enemy |
| Jailbreak | Free the imprisoned party members |
| Band of Heroes | Have each authored hero in the active party at least once |
| Warlord Slayer | Defeat either the Samurai Warlord or Orc Warlord |
| Master of the Arcane | Complete an Archmage promotion |
| Beyond Death | Complete a Lich promotion |
| Champion of the Realm | Complete the game's victory objective |

Achievement announcements use the existing animated top banner. They survive ordinary news overflow and save restoration, and can animate over a paused interface. Each achievement is earned once per player profile.

## Persistence

`player_profile.json` uses `storage.AppSavePath`, independently of save slots. A macOS bundle stores it under `~/Library/Application Support/RaysAndMagic/saves/`; a local binary uses its local `saves/` directory. `go run` uses the working directory's `saves/` directory. Existing storage fallbacks remain in effect.

One background writer atomically saves serialized snapshots every five seconds, immediately after an unlock, and at normal shutdown. Invalid existing profiles are reported and preserved. Loading an older save does not reset lifetime records.

Old saves can establish victory, promotions and freed captives when progress has not been explicitly reset. Active hero history starts from heroes actually observed in the active party; reserves do not count. Historical kills, casts, loot and time cannot be reconstructed and start counting when profile tracking is enabled.

## Metric definitions

| Display | What counts |
| --- | --- |
| Adventures / victories / party wipes | Distinct playthrough IDs observed, and each playthrough's first recorded victory or complete party defeat. Reloading the same outcome does not add another result. A playthrough may have both outcomes after reloading and continuing. |
| Active play time | Unpaused gameplay, excluding loading and long update stalls. Both real-time and turn-based play count. |
| Favorite classes | Active party time per hero. Four active heroes contribute four hero-seconds per second. |
| Favorite spells | Successful player-initiated casts, excluding rejected casts, echoes and automatic effects. |
| Favorite regions | Active time in each region. |
| Regions explored | Unique occupied local tile coordinates divided by the full current region area (width times height), including non-walkable tiles. The percentage rank is independent of time. |
| Monsters defeated | Credited enemy deaths. Allies and repeated cleanup of a corpse are excluded. Replaying combat after loading is new lifetime activity. |
| Most dangerous | Actual party HP lost to direct monster attacks after mitigation and survival effects, including area attacks. Ongoing poison/burn and environmental damage lack persistent attacker attribution and are excluded. |
| Heroes knocked out | Individual heroes reduced from positive HP to zero by those attacks. |
| Loot found | Units in monster drops, opened treasure chests and opened crates. Purchases, transfers and collecting an already counted loot bag are excluded. |
| Quest rewards claimed | Successful manual reward claims. |
| Steps | Successful party moves across tile boundaries, in either combat mode; blocked moves and teleportation are excluded. |

## Shared image resizing and exploration history

Game and editor interface images share `internal/graphics/image_scale.go`: mipmapped linear minification on either shrinking axis, nearest sampling at native size or larger, with tint/alpha preserved. Inventory, paperdoll, status/achievement icons, portraits, card art, compass tiles, cached loading scenes and editor previews delegate to it. World projection, procedural quads and soft additive effects retain their distinct rendering contracts.

`visited_tiles` in the profile stores sets of `x,y` coordinate strings under stable region keys. Open-world placement uses the existing map-local inverse transform. Repeated steps, tile-type changes, save reloads and achievement resets do not erase or duplicate coordinates. Resizing changes the denominator; coordinates outside new bounds remain in history and count again if the region expands. Corridors outside authored region bounds have no region-area cell and are excluded. Only visited regions appear in the ranking. Old exploration cannot be inferred from region playtime.

Achievement resets clear unlocks, the current achievement counters and active-hero history while preserving every lifetime statistic and explored coordinate. Historical promotion/prison facts from an old save do not undo an explicit reset. Band of Heroes uses the current authored roster, with no duplicated hero list or class-based approximation.

## Trophy statistics

Trophies exposes boss kills, legendary drops (excluding all cards), item-currency units paid to merchants, and cards found at every rarity. Each counter has an art-backed ranking with quantities. The existing boss total remains intact; boss detail, rarity-specific loot and trade history start counting when profile tracking first records them; older profiles do not contain those facts.

Legendary and card metrics share the existing loot commit boundary: monster bags on drop, chests on opening, and crates when granted. Picking up the same bag, moving inventory items or restoring a save does not recount that loot. Purchases do not count as drops. Merchant statistics count the successfully consumed item currency, including per-entry overrides, gold surcharges and multi-unit purchases; refused purchases, gold/arena payments and ordinary sales do not add item trades. All fields use the existing profile counters/rankings and survive achievement resets.

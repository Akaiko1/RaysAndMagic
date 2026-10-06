# Content authoring

Use the [README content index](../README.md#add-content) to choose a guide.
The guides explain supported extension points; the production structs, loaders,
and shipped YAML define the complete field set. An unknown YAML field is often
ignored by runtime loaders, so successful parsing alone does not prove it works.

## One definition, all consumers

Choose a stable YAML key and a unique display name. Use keys for references
unless the destination explicitly requires a display name (merchant inventory
does). Keep saved identities stable when changing names, quest spawns, or maps.

Merge example entries into the existing top-level block; do not paste a second
`weapons:`, `npcs:`, or other duplicate root into a file. All `yaml` fences in the
guides are complete example documents, not replacement catalogs. Their small
catalogs still need integration with the shipped content, art, and placement.

Prefer an existing attribute over an object-name branch. A new mechanic needs
one shared rule used by gameplay, save restoration, the game tooltip, and the
editor card. Audit all attack forms or state transitions that consume that rule.
Do not invent unsupported fields or describe effects only in tooltip prose.

Distances ending in `_tiles` and monster radii use tiles (normally 64 world
pixels). Author durations in seconds where the schema asks for seconds; turn
durations are separate. Do not assume 60 frames is one second: use configured TPS.
Use `graphics.size_classes` for world sprite sizes, not retired raw size fields.
Collision sizes are classes too: a monster body comes from its `size_class`
(`world.monster_bodies`, capped at 48 px so it fits 1-wide corridors) and a
projectile names `physics.hitbox: standard | wide` (`world.projectile_hitboxes`).
Raw `box_w`/`box_h` and `collision_size_tiles` are rejected at load.

## Assets and animation

Runtime asset compatibility comes first: readable silhouette at gameplay scale,
correct camera and direction, consistent identity, stable baseline, and complete
safe margins. Follow the project's asset-generation instructions when producing new art.

| Asset | Location / contract |
| --- | --- |
| Monster | `assets/sprites/mobs/`; YAML `sprite` is the basename |
| NPC | `assets/sprites/characters/npcs/`; inspect a matching shipped NPC layout |
| Terrain/prop | `assets/sprites/environment/` subdirectories; match the tile render type |
| Weapon tooltip icon | `assets/sprites/interface/weapons/icon_weapon_<key>.png` |
| Spell tooltip icon | `assets/sprites/interface/spells/icon_spell_<id>.png` |
| Item tooltip icon | `assets/sprites/interface/items/icon_item_<key>.png`; shared resolver: `itemTooltipIconName` |

New detailed weapon/spell icons are 128x128; preserve existing 64x64 legacy
icons. Generate item, weapon, spell, and trap artwork without a baked-in frame,
on a uniform opaque black background, with the complete silhouette inside safe
margins. The engine supplies the frame and its rarity or school tint. Register
only unframed sources in [icon_frames.yaml](../assets/icon_frames.yaml), with
basic, Asian, or boss provenance; do not add a runtime frame over a legacy rim.
Optimize PNG losslessly and verify decoded pixels are unchanged.

Monster walking and attacking sheets each contain four square frames. Preserve
an existing layout: 512x128 means four 128x128 cells; 256x256 means four 128x128
cells in row-major 2x2 order. `_l` and `_r` mean screen-left and screen-right.
Do not infer direction from a map letter. Keep scale and baseline shared across
walking, attacking, and any additional motion sheets. NPC idle detection is a
separate path; use the layout required by the [NPC guide](../how_to_add_a_new_npc.md).

Raw generated sources must remain untouched. Process or register art only when
that is part of the task. Store intermediate sources, overlays, previews, and
logs in the system temporary directory outside the repository. Only selected
runtime assets and deliberate documentation belong in the change.

## Damage and hit scope

The shared party damage path is in
[combat_damage.go](../internal/game/combat_damage.go); its integration coverage is
[party_damage_contract_test.go](../internal/game/party_damage_contract_test.go).
Extend this rule when adding mechanics instead of duplicating damage arithmetic
in each projectile, weapon, or effect handler.

| Rule | Authoring consequence |
| --- | --- |
| Primary and splash share source damage | Never compute splash from the primary victim's remaining HP or already mitigated hit |
| Each victim resolves its defenses and target bonuses | Armor, resistance, soak, and `bonus_vs` must use that victim |
| One base critical roll covers a weapon impact and its splash | Do not independently reroll base critical chance for splash |
| Designate Target adds weapon critical chance only for marked victims | Snapshot marks before that impact's primary-hit effects; a newly applied mark does not improve the same impact |
| Weapon critical scaling precedes flat outgoing buffs and physical conversion | Preserve the normal and critical source packets; multiplying the final result changes flat bonuses |
| True damage remains a separate component | It bypasses armor/dodge, still uses school resistance, and is not multiplied by ordinary critical scaling |
| Primary-only riders stay primary-only | Splash damage does not spread weapon stun, disintegration, or on-hit status effects |
| Area spell forms have their own explicit scope | A nova, mortar AoE stun, or zone tick is not a primary-only projectile rider |
| Continuations retain source damage | Pierce/ricochet must not reuse a previous victim's mitigated damage; later impacts see the marks current at that impact |

Persistent zones retain their own tick and armor policy while using shared
per-victim damage resolution. Card target bonuses apply through the common
resolver. A data field's availability does not imply every attack form runs
every rider: verify the relevant dispatch path and tooltip together.

## Dialogue

Write concise adventure dialogue with a distinct speaker, concrete motives and
places, and occasional dry humor. Explain the objective once; let reminders and
completion lines react to the state of the story. Keep exact objectives intact
and prices/eligibility in shared service UI. English prose uses ASCII punctuation.
Read the [dialogue style guide](dialogue-style.md) for the full writing workflow.

## Verification

1. Check keys, references, units, complete required field pairs, and sprite paths.
2. Run the executable documentation examples:

   ```sh
   go test ./internal/game -run '^TestContentGuideExamples$' -count=1
   ```

   The test reads the actual fenced YAML, decodes it against production structs
   with unknown fields rejected, merges weapon/item additions with the shipped catalogs for set references,
   then calls production loaders and applicable boot validators. It does not validate art or every cross-file gameplay rule.
3. Run relevant content tests (including required fields, visual sizes, quest
   references, and ecology when affected), then `go test ./...` and `go vet ./...`
   for code changes. Format changed Go files with `gofmt`.
4. Check the editor and game: acquisition or spawn, tooltip, animation, both
   combat modes when relevant, and save/load. For connected maps, test both
   stitched and split-world modes. Use a disposable save copy outside the repo.
5. After Go or game-code changes, finish with `./build_bin.sh`. A plain
   `go build` does not replace this verification.

Document what was actually run. A loader test cannot establish that a sprite
looks correct, a new item is obtainable, or a quest can be completed in a save.

## Combat input and tooltips

Mouse smart attack shares Space's healing, spell, and weapon priorities and
cooldowns. Pointer targeting uses the last displayed view in both combat modes;
an unmatched click expires in the same input update. Movement or turning must
not retarget an old click. Friendly party-controlled creatures and transparent
sprite margins are excluded, and background doors or NPCs must not intercept
a foreground monster's click.

Explicit aim keeps its direction through melee resolution and projectile flight,
with the normal weapon arcs, splash, and physical interception. A held attack
ends on release, leaving the target, losing line of sight, or opening a menu;
acquiring another target requires a new press. Keep these rules consistent when
extending [combat input](../internal/game/input.go).

Equipped item tooltips mark completed set bonuses in green with `[ACTIVE]`.
Loose items show requirements without claiming an active bonus. Skill effects
and detailed calculations belong in the shared tooltip producers, not in a
separate README balance table.

## Regular skill balance

Regular skills, including Ballistics, Field Medicine, Designate Target and
Overwatch, share `internal/character/catalog.go` for mastery tables and tooltip
calculations. These four skills use the ordinary `SkillType` model; they do not
form a separate tactical configuration category. Their former YAML tables were
moved into the existing regular-skill catalog to keep that model consistent.
Changing these tables requires a rebuild. Content such as weapons, spells,
monsters and class skill access remains YAML-driven and validated at load time.

Overwatch reacts to enemy attacks at half its movement reaction chance.
Ballistics' bow/blaster critical chance bonus uses the same mastery table as its
tooltip in [the skill catalog](../internal/character/catalog.go).

Automatic drinking is a party-wide mechanic configured by
`characters.auto_drink` in `config.yaml` (`threshold_pct`, `interval_seconds`).
Field Medicine improves both manual and automatic potion recovery through the
same catalog used by the skill description; it does not enable automatic use
or restrict it to Snipers.

# Main storyline: implementation plan

Status: implemented 2026-10-23, except E4 (the Noble/Selfish ending). The
art arrived on 2026-10-22 in `~/Downloads/RaysAndMagic_story_art_20261022/`.

## The problem

The main storyline is not visible in play:

- No prologue, epilogue or ending text exists. The victory screen is three
  hard-coded lines in `drawVictory` (ui_dialogs.go). The only zone text is
  "You enter %s." (`syncOpenWorldRegion`).
- A new game starts with `broodmother_nests`, `water_purge` and
  `endgame_triad` already active: level 23-35 targets with no giver.
- Nothing in the forest points to the Mage Tower.
- Sylwen offers the hunt for the level 29-31 Elder Dragons in a desert of
  level 3 bandits, with no level gate.
- The journal has no main/side distinction.
- The desert to jungle jump (level 3 to 18) has no warning
  (docs/game-design-proposals.md, item 1).

## Story spine

Main line (required for victory):

1. Prologue (new game). The Tower envoy at the forest start tavern recruits
   the party as ash-walkers and gives the first main step.
2. The Tower (Misty Highlands). The party delivers the letter. The Tower
   explains the empty throne and sends the party on to the dragons and the
   sea. It stays the hub of the main line with or without an Archmage
   candidate.
3. The dragon hunt: Sylwen's oath, statuettes from the Pyramid Sanctum, the
   four Elder Dragons (`dragon_slayer`), then the Ancient God of Death.
4. The Dragon Cliffs: `broodmother_nests`, then the Brood Mother.
5. The sea: the lamps off Seabright, `water_purge`, then the Alien Enforcer.
6. The Last Reckoning (`endgame_triad`) and the ending.

Optional chapter: the Lich King and the Raising (`archmage_trial`). The Tower
offers the trial only when a party member can be promoted to Archmage
(`canPromote`: unpromoted, with Archmage art) and the party carries no lich
(`promotionRefusal`). Nothing on the main line may depend on it.

Side arcs stay optional and unchanged: Eastern Isle Castle, Deep Jungle,
Sakura Garden, Solstice Ruins, Clock Tower.

Level bands (base levels, measured 2026-10-22): forest 1-8, desert 3 (pyramid
5-13), highlands 6-16, ocean 10-15, Lich Nexus 14-21, Dragon Cliffs 11-25,
jungle 18-24, Solstice 22-28, Elder Dragons 29-31, the three final bosses
32-35.

### Lore rules for all story text

- Two layers: the text plays as fantasy; the physical oddities (rivets, seams,
  lamps in rows, vents instead of breath) are stated flatly. Nobody native
  states that magic is technology. The captain knows, and never says it
  either: he calls the Light "what the tower keeps" or "the seed".
- MM3-5 register: short declarative sentences, the task and place named
  plainly, no mood clauses. Re-read docs/dialogue-style.md and the MM sources
  before writing; Crodo's "It is now up to you, adventurers, to right what
  has been wronged" is the ceiling for formality.
- ASCII only.

## Beat sheet

| Event id | Trigger | Content | Captain | Plate |
|---|---|---|---|---|
| prologue | new game | empty throne, plague, dragons, lamps; the party are ash-walkers; the envoy waits at the tavern | - | story_prologue |
| tower | `main_tower_letter` claimed | the Tower on the empty throne; points to the dragons and the sea; the trial if a candidate exists | - | story_tower |
| captain_first | first boss or Elder Dragon kill (bosses die in any order) | - | introduces himself, stage 1 | story_captain |
| raising (optional) | `archmage_trial` claimed | an ash-walker is raised to Archmage | stage 3, Raising branch | story_raising |
| four_seals | `dragon_slayer` accepted | Sylwen's oath and the seals | - | story_four_seals |
| dais | `dragon_slayer` completed | the God of Death rises on the dais | comment | story_dais |
| crater | `broodmother_nests` completed | the Brood Mother wakes | comment | story_crater |
| lamps | first arrival in Seabright (the harbor watch) or on `water` (`maps: [city, water]`) | reveals `water_purge` | - | story_lamps |
| enforcer | `water_purge` completed | the Enforcer surfaces | stage 4: "I will come up myself" | story_enforcer |
| reckoning | the first of `dragon_slayer`, `broodmother_nests`, `water_purge` completed (its boss rises); reveals `endgame_triad` | the three enemies, any order: one awake, the rest wake with their kin dead | - | story_reckoning |
| ending | victory | replaces the hard-coded victory text; later hosts the Noble/Selfish choice | final scene | story_ending |

The desert to jungle warning is the desert scout's dialogue, not a story
screen.

## The watching captain

The Pursuers' captain watches the party the way Sheltem watches it in MM3.

- Name: Oruun (candidate; Hask and Ithren are also unused). Seabright's
  sailors call him the Lamp-Captain. The Alien Enforcer, "the Pursuers' field
  commander" in monsters.yaml, answers to him.
- Motive: he wants the party to succeed. He needs the Tower to raise an
  Archmage so the prize exists, then he will come for it. If no Archmage is
  raised, he is glad the party is killing the dragons the Tower built to stop
  him. His warmth is the warning.
- His eyes: a light blinking at a slow count, and his salvage drones (the
  Golden Thief Bug is one).

Stages (his tone):

1. Notice: from his first plate (first boss kill). Indifferent.
2. Interest: until stage 3. Evaluating, pushes the party toward the Tower.
3. Claim: from the Raising, or, without one, from the first Elder Dragon kill.
   - Raising: "It is in you now. I can see it from the sea."
   - No Raising: "The tower built those to stop me. You are killing them for
     me. Keep going."
4. Reckoning: from the Enforcer's death. Openly hostile; the face of the
   ending choice (Noble: he withdraws and waits; Selfish: the party takes his
   ship, as in the selfish ending draft).

Channels:

- Story plates: scripted, once each (first contact, Raising branch, Enforcer,
  ending).
- Story bosses (the main-quest kills: elder dragon, Brood Mother, God of
  Death, Enforcer): their lines always play.
- Everything else is a rare random remark, at most 5 per run, at least 2
  in-game days apart (3-4 a run on average, a different few each run):
  - camp: 20% per rest; his line plays over the camp scene and the hold
    waits for a click instead of 2.5 s;
  - other kills: 35% per kill, the monster's own line, else for a boss the
    general pool. Shown only after the fight ends.

Rules: every line plays once per save, dealt without repeats (the tavern
rumor deck pattern); lines are gated by story progress (`after`/`until`
quest, as in rumors.yaml); context lines exist for a hurt party, night and the
sea. When a pool runs out he falls silent.

Draft lines (approve before building):

- First plate: "I am Oruun. Your sailors call me the Lamp-Captain. I have
  watched you since the forest. You kill well. Go on to the tower,
  ash-walkers. They have something for you. I want to see it."
- Camp: "Sleep. Nothing will come for you tonight. I have told it not to."
- Camp, party hurt: "You are bleeding. Rest. I can wait."
- Golden Thief Bug: "That gold thing was mine. It picked up rings. I have
  more of them."
- Orc Warlord: "I promised that orc a crown. He would have made a poor king.
  You saved me the cost."
- Samurai Warlord: "The rot got into him before you did. It gets into
  everything the tower ever touched."
- Lich King: "The broken one is dead. Go back to the tower. Take what they
  give you."
- Elder Dragon: "The tower built those to stop me. One fewer, and I did not
  lift a hand."
- Enforcer: "You broke my Enforcer. Then I will come up myself."

Line inventory to write (about 40): 4 story plates; about 22 camp lines
(stage 1: 4, stage 2: 6, stage 3: 6 split between the two branches, stage 4:
3, context: 3); about 14 boss lines (Thief Bug, Golden Thief Bug, Orc
Warlord, Samurai Warlord, Jorogumo, gorilla titan, Lich King, Elder Dragons,
Brood Mother, God of Death, Enforcer, one shared for the Solstice bosses) plus
a general pool of 4.

## Work plan

Load the fitting skills before each stage: interface-design (with the written
checkpoint) before any UI code, game-test-strategy before any test,
mm-level-design and its validator before any map edit, whole-rule-fix for
review fixes.

### A. Story event engine (no art needed)

- [x] A1. `assets/story_events.yaml`, loaded with strictyaml and validated at
  boot like rumors.yaml. Fields: `id`, `trigger` (`on` plus `quest`,
  `monster` or `map` as the kind needs), optional `requires_quest` and
  `min_party_level`, `art` (sprite key), `title`, `pages` (1-3, ASCII),
  optional `speaker` (`narrator` or `captain`), optional `reveals_quest` and
  `grants_quest`. Fail fast on unknown trigger kinds, unknown quest, monster,
  map or sprite keys, empty pages, duplicate ids.
- [x] A2. Triggers, a closed set on existing hooks:
  - `new_game`: `startNewGameWithParty`;
  - `quest_accepted`, `quest_completed`, `quest_claimed`: read from the
    journal every frame (`syncStoryQuestBeats`): the state, not its change,
    so a load, a talk or a paused journal cannot slip past it;
  - `monster_killed`: `finishMonsterKill`, by monster key;
  - `region_entered`: `syncOpenWorldRegion` and `finishMapArrival`, first
    arrival only;
  - `victory`: `checkVictory`.
- [x] A3. Queue: FIFO, one event at a time; held while the party is in
  combat (the party-in-combat flag) and while a dialog, menu or another modal
  is open.
- [x] A4. Persistence: `StorySeen []string` in `GameSave`, beside
  `QuestSpawnsDone`, with the same snapshot, restore and new-game reset. On
  load of a save from before story events, silently mark as seen every
  event whose trigger state has already passed (the same test,
  `storyQuestReached`), so an old save does not replay the story.
- [x] A5. Modal layer `modalLayerStory`: add it to the layer enum,
  `topModalLayerFor`, `pausesWorld` (pauses), `handleTopModalInput` (Space,
  Enter or click: next page; Esc: skip), `registerDisplayedModalMouse` and the
  draw order; extend modal_layer_contract_test.
- [x] A6. Presentation, built on the camp-rest code (camp_presentation.go):
  art cover-fit in the gameplay viewport with the party cards visible, the
  `camp_dissolve.kage` dissolve, the `presented`/`awaitingFrame` gate so the
  text starts once the art is on screen, a slow drift of the image. Text
  panel on the journal paper in the lower third: title, page, page marker,
  "Continue". Until a plate exists, show the region's camp scene
  (`camp_scene` in map_configs.yaml).
- [x] A7. Chronicle header: each event is dated by the in-game calendar
  (calendar day, week and month already exist).
- [x] A8. Chronicle page in the Quests tab: every seen event by date and
  title; clicking one reopens it read-only.
- [x] A9. Layout at every frame size, font and interface scale (Normal stays
  untouched; the user plays Large = 1280x720).
- [x] A10. Tests (persistent state only, no pixel tests): an event fires once
  per save; save/load keeps `StorySeen`; a legacy save resyncs without
  replaying; events wait out combat and dialogs; trigger conditions hold;
  the loader rejects bad entries (rejection cases only).

### B. Main line content

- [x] B1. `main: true` on QuestDefinition (closed bool). The journal sorts
  main quests first and marks them with their own seal and label.
- [x] B2. Main chain: `main_tower_letter` (an `interact` quest tagged on the
  Mage Tower, given by the envoy), then `dragon_slayer`, `broodmother_nests`,
  `water_purge` and `endgame_triad`, all marked `main`. `archmage_trial`
  stays unmarked (optional chapter). Journal epic line in the MM style:
  "Carry the Tower's letter, hunt the four Elder Dragons, clear the lamps
  from the sea, and save the realm."
- [x] B3. Reveal-gated quests: `broodmother_nests`, `water_purge` and
  `endgame_triad` stay active from boot (kills made early still count) but
  are hidden in the journal, and get no quest banner, until a story event
  reveals them (`reveals_quest`).
- [x] B4. Mage Tower dialogue: takes the letter, explains the empty throne,
  points to the dragons and the sea. The trial is offered only when a
  candidate exists (the existing `ineligible` reply covers the rest).
- [x] B5. Level gate on Sylwen's `dragon_slayer` offer (value to decide,
  around 24) with a reply that sends the party to train and to the Tower.
- [x] B6. Ending: the `ending` story event plays over the victory screen,
  after any beat still queued ahead of it (the screen freezes the world, so
  the queue plays in order there). The victory screen keeps its own heading
  and lines (changed from the plan: rewriting them would shift the approved
  score layout).
- [x] B7. Write every event's pages (MM register, lore rules above).
- [x] B8. Update the main-story rumors in rumors.yaml to the new chain.

### C. New NPCs

- [x] C1. `tower_envoy` at the forest start tavern: prologue follow-up, gives
  `main_tower_letter`.
- [x] C2. `desert_scout` at the desert to jungle passage: warns about the
  jungle and points to the main line (closes pacing item 1).
- [x] C3. `harbor_watch` in Seabright: reports the lamps and triggers the
  `lamps` event, which reveals `water_purge`.
- [x] C4. Place them with `@` bindings; run the mm-level-design validator on
  every map touched.
- [x] C5. Extend quest_giver_content_test: each giver offers its step, in
  chain order.

### D. The watching captain

- [x] D1. `assets/watcher.yaml`: `id`, `when` (`camp`, `monster_killed`,
  `story`), optional `monster`, `after`/`until` quest gates, `stage`,
  optional context (`party_hurt`, `night`, `region`), `text`. Validated at
  boot.
- [x] D2. `config.yaml` `watcher:` block: story-boss lines (`story: true`:
  dragon, Brood Mother, God of Death, Enforcer) always play; every other line
  is random: `camp_chance` 0.2, `kill_chance` 0.35, at most
  `max_random_lines` 5 per run, `min_days_between` 2 (3-4 a run on average).
- [x] D3. Selection: a deck per channel without repeats, gated by stage;
  an authored monster line beats the general pool.
- [x] D4. Hooks: `beginCampRest` picks a line and `tickCampRest` holds until
  it is read; `finishMonsterKill` queues a line until the fight ends; his
  story plates are story events with `speaker: captain`.
- [x] D5. Presentation:
  - the light: three slow teal blinks before he speaks, drawn in code (on the
    camp scene's horizon at a point authored per scene, in the boss card's
    corner, under his face on his plate);
  - camp: his line in the lower third in sea-lamp teal;
  - boss card: a new screen-banner kind, non-pausing, about 6 s or until
    clicked; his portrait appears only after his first plate;
  - a "watcher" tone in the combat log, so every line can be re-read.
- [x] D6. Persistence: `WatcherSeen`, the day of his last random remark and
  the kill lines still waiting (`WatcherQueue`) in `GameSave`, with new-game
  reset. The story side keeps `StoryRevealed`: a save from before story events
  reveals every hidden quest its journal already showed.
- [x] D7. Stage 3 starts at the Raising or at the first Elder Dragon kill,
  whichever comes first; the branch chooses the line set.
- [x] D8. Tests: a line plays once; gates hold; nothing during combat; the
  day cooldown holds; save/load keeps the seen lines and the cooldown; camp
  chance under a seeded RNG.
- [x] D10. Write the line inventory above and get it approved.

### E. Art and ending

- [x] E1. Import the plates and the portrait (see "Art brief").
- [x] E2. Match the plates' brightness to the camp scenes (darken-art skill).
- [x] E3. Assemble and import the idle strips for the three new NPCs.
- [ ] E4. The Noble/Selfish ending, once its form is decided: the `ending`
  event hosts the choice.

## Decisions (2026-10-22)

1. Lead: the Tower envoy at the forest start tavern gives the first step; the
   Mage Tower then takes over as the main-line hub.
2. The three boot-started final-boss quests stay active from boot but are
   hidden in the journal, with no banner, until a story event reveals them.
3. The captain is Oruun; Seabright's sailors call him the Lamp-Captain.
4. He never appears as an enemy: a voice, his light and his story plates.
   The Enforcer stays his fighting arm.
5. His comments are always on (they are rare); no settings switch.
6. Sylwen offers the dragon hunt from party level 24.

## Art brief

The full package, 24 images, with ready-to-paste ChatGPT prompts:
docs/story-art-prompts.md.

- 11 story plates, 1536x1024: prologue, tower, raising, four seals, dais,
  crater, lamps, enforcer, captain, reckoning, ending.
- The captain's portrait, 1024x1024, shipped at 512x512.
- Idle frames for the 3 new NPCs (tower envoy, desert scout, harbor watch),
  4 frames each, assembled into 2048x512 strips.

No animation is needed for the plates or the portrait. All motion is drawn
in code: the dissolve, the slow drift, the text fade and the captain's
blinking light. The NPC idle strips are the only animated art.

Raw PNGs go untouched, with the exact names from the prompt file, into
`~/Downloads/RaysAndMagic_story_art_20261022/`; they never go into the repo.
Import steps are listed at the end of the prompt file.

## References

- docs/story-art-prompts.md: the art package prompts.
- docs/dialogue-style.md: the prose register and the MM reference excerpts.
- docs/adding-quests.md, docs/content-authoring.md: quest and content rules.
- docs/game-design-proposals.md: the level pacing audit.
- assets/rumors.yaml: the gating fields (`after`, `until_monster`, `spawn`,
  `priority`) the watcher lines reuse.
- assets/IMAGE_SIZES.md: image size standards.

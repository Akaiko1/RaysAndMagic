# Game design proposals

Open proposals that need a design decision. Nothing here is implemented yet.
Each item gives the evidence, why it matters and the options; record the
decision in the item when one is made.

## Level pacing audit (2026-10-21)

All 27 configured maps were checked against the map validator and against
the pacing rules below. Structure holds: no validator errors, the critical
path is open, capability gates check out from both sides, nothing soft-locks.
The findings are about pacing between and inside levels.

Method: distance is walking distance in tiles (4-neighbour path) from the
map's start point `+`; levels are the authored `level` values in
`assets/monsters.yaml`. Coordinates are map-local (x,y).

Rules applied:

- Difficulty rises with travel distance, in a sawtooth: each peak higher than
  the last, a quiet stretch before a climax and after a transition (stairs,
  region passages).
- A route that lets a weak party walk early into a much harder area is
  allowed once, as a deliberate back door, and the game announces it (an NPC,
  a sign, a landmark of bones).
- Every dead end pays: loot, a camp, a landmark, a shortcut or a view.

### 1. Desert to Deep Jungle: from level 3 straight to level 18

- The desert holds 12 monsters, all level 3 (10 bandits, 2 skeletons).
- From the forest passage (west edge, y18) to the jungle passage (east edge,
  y43) is 72 tiles with nothing above level 3.
- In the jungle the first level 18 goblin stands 16 tiles from the landing
  start (4,47).
- Nothing warns the player: the only jungle rumor is about Velna's doorway.

Options:

- a warning at the passage (a ranger NPC, a sign, bones);
- an intermediate camp (level 8-14) in the desert's south-east dunes on the
  way to the passage.

### 2. Highlands to Dragon Cliffs: the exit to the hard region sits in the easy corner

- From the highlands start (23,45) to the Dragon Cliffs passage (east edge,
  y42-44) is 28 tiles, past level 6 pumas and one level 7 archer.
- In the Dragon Cliffs the first level 16 mountain troll is 23 tiles from the
  start.
- The highlands' own level 13-16 content (Warden Keep, Grimscree trolls)
  begins 60-80 tiles out, so the party can skip it entirely.
- The same corner holds the Slate Tomb with three level 16 liches. They stand
  behind walls and need sight to aggro; the Mage Tower quest names the Lich
  Nexus.
- The passage replaces the old violet teleporter on the same spot, so the
  split world had the same jump.

Options:

- a warning at the passage;
- an intermediate camp before the passage, on the highlands side.

### 3. The desert is the weakest and emptiest region

- 50x50 tiles, 12 monsters; two 16x16 quarters, (32,16)-(48,32) and
  (16,32)-(32,48), are 1% blocked.
- The desert sits between the forest (level 1-8) and the jungle (level 18-24)
  but does not bridge them.

Option: one intermediate camp plus two or three named landmarks in the empty
quarters. This also covers item 1.

### 4. Pyramid: a level cliff at the stair

- Floor 1 holds level 5 mummies; floor 2 holds level 13 minotaurs.
- The first minotaur, (8,20), is 9 tiles from the floor 2 arrival (16,21).
- The desert outside is level 3.

Options:

- move the minotaur at (8,20) away from the stair;
- add one tougher guardian on floor 1 before the stair.

### 5. Church: no climax

- 16 skeletons, all level 3, one monster type; 13 of them stand in one band,
  20-29 tiles in.
- The quiet entrance is right (nothing within 10 tiles). What is missing is a
  peak or a reward beat at the end.

Low priority. The other single-type floors (pyramid floors 1 and 2, Clock
Tower floor 1) read as deliberate: each floor has its own theme and the
difficulty steps up between floors.

### 6. Pyramid floor 2: three empty dead ends

The east corridor stubs at (27,16), (29,18) and (29,20) end in nothing.
Option: give each a payoff (a crate, a decor piece, a secret) or close it.
Minor.

### What already works

- The forest ramps up steadily with distance from its start.
- Bosses stand at the far end of their maps (the culverts boss at 93% of the
  longest path).
- Solstice fire chamber: weak heat tiles (25%) come before the strong
  telegraphed lanes (150%).
- Clock Tower floor 3: the alarm clocks (one wakes the monsters around it)
  appear one at a time before they come in groups.
- The validator's "maze" warnings on interiors come from the outdoor density
  rule and do not apply there.

### Not checked yet

- How well the route guides the eye in play, and how the fights feel.
- The order in which the water, earth and air Solstice chambers introduce
  their hazards.

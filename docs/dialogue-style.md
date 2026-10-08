# Dialogue style

NPC dialogue and quest prose use a literary adventure style inspired by
Might and Magic III, IV, and V. These rules apply to trainers, shops, quest
reminders, and rewards.

## Text ownership

- Author NPC voices and location-specific messages in `assets/npcs.yaml`, not
  the shared UI catalog. `quest_messages.<quest_id>` holds both conversation
  bodies (`offer`, `active`, `completed`) and optional action replies
  (`accepted`, `already_active`, `not_completed`, `ineligible`, `rejected_undead`).
  Missing optional replies use neutral UI feedback; they never change gates.
- Paid wait and tavern-rest choices require a complete `response`. Use `{cost}`
  for the choice's price; other placeholders are rejected. Percent signs in
  these messages are literal, not printf directives.
- `assets/text/*.yaml` owns shared UI labels, controls and neutral status
  templates. NPC shop headers use authored greetings, with neutral UI prompts
  only when no greeting is provided.
- UI format signatures are derived from the bundled YAML at startup. Do not
  maintain a second registry in Go or another YAML. External wording must keep
  the bundled keys and printf argument types/order; catalog tests also verify
  the production call sites against that contract.

## Writing rules

- Write the person before the instruction: a trade, grievance, desire, habit,
  or local detail should give the speaker a recognizable voice.
- Use concrete places, objects, actions, and stakes. A lost necklace, unpaid
  bill, spoiled well, or boastful fighter is stronger than abstract destiny.
- Keep conversation brisk, with room for fairy-tale strangeness and dry humor.
  Humor is optional; do not make every NPC sarcastic or every line a punchline.
- State necessary gameplay conditions once in the relevant explanation. Let
  greetings welcome, reminders recall the unfinished problem, and conclusions
  react to its resolution. Do not repeat "Master to Grandmaster" in every state.
- Keep exact costs and eligible upgrades in the existing data-driven service
  UI. Do not obscure a requirement behind metaphor or invent mechanics for a joke.
- Avoid generic epic speeches, repetitive worthiness tests, modern service
  copy, and stock contrasts of the form "not X, but Y."
- Preserve the existing quest objective, timing, reward, and world facts.
  Check the complete conversation in state order, not each field separately.
- English game text and punctuation remain ASCII-only. Write new prose;
  references below are study excerpts, not dialogue to paste into the game.

## Short verified reference excerpts

These are short excerpts from game text or spoken lines transcribed in the linked
playthroughs. They are not a comprehensive script or primary-source transcript.
Labels distinguish dialogue, introduction, and magical quest hints. The comments
about writing are our interpretation, not claims made by the sources.

### Might and Magic III: Isles of Terra

An enchanted fountain's quest hint:

> Take the twisting horn of gold to the central meadow.

[Source: Mouths and Moose Rats, fountain hints](https://crpgaddict.blogspot.com/2017/09/might-and-magic-iii-mouths-and-moose.html).

Lesson: a specific object and destination carry a small story; the instruction
has atmosphere without losing what the player needs to do. Rhyme belongs to
this magical speaker, not to every shopkeeper.

### Might and Magic IV: Clouds of Xeen

Two voiced shop greetings:

> What do you want?

> Safe and secure!

The introduction interrupts Crodo's narration with King Burlock:

> I am the king!

[Source: Game 403, shop voices and introduction transcript](https://crpgaddict.blogspot.com/2021/02/game-403-might-and-magic-world-of-xeen.html).

Lesson: very short lines can distinguish a brusque armorer, a cheerful banker,
and a pompous king. Brevity does not require a neutral tutorial voice.

### Might and Magic V: Darkside of Xeen

The tower guardian before and after the party obtains a key:

> You need a key, orc breath

> How did you get a key?

[Source: Game 409, Ellinger's tower guardian](https://crpgaddict.blogspot.com/2021/04/game-409-might-and-magic-darkside-of.html).

Lesson: the same service gate produces two different reactions. The second
line acknowledges changed circumstances instead of restating the requirement.

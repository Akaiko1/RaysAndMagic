# TODO

- [ ] Publish the game to the Snap Store (Ubuntu App Center): plan, owner
  actions and open decisions in docs/snap-store-publishing.md.

- [ ] Unify status RUNTIME the way monster abilities are (one row per kind,
  like `abilityRows`). Names, short names, side texts and mechanic numbers
  already live in assets/statuses.yaml (closed set `statusRules`); still
  parallel and kept in sync by hand:
  - state + clock: `Condition` enum and per-status field quartets on
    MMCharacter/Monster3D (`SlowPct/SlowTurnsRemaining/SlowFramesRemaining/
    SlowRate`, Weaken, ArmorShred, ...; only ArmorMelt uses status.DualClock),
    party-wide root and slow on MMGame (`partyRoot`, `partyHinder`);
  - apply/tick/expire code, save fields and the source fields on weapons,
    spells, traps and monster abilities, one set per status;
  - `statusVisualCatalog` + the `partyStatusVisuals`/`monsterStatusVisuals`
    switches (render_status_fx.go);
  - poison/burn/stun card effects drawn outside the motif catalog (ui_hud.go).

  Collisions seen in the real render (2026-10-13), to fix with slots:
  - monster shred and melt draw the same split shield in the same spot, so
    melt hides shred;
  - hero card: the melt shield sits on the face, the slow hourglass right
    under it with the flakes falling through; Asleep Z's cross the shield;
    Fear lines merge with the Paralysis bolts.

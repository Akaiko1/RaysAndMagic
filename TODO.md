# TODO

- [ ] Publish the game to the Snap Store (Ubuntu App Center): plan, owner
  actions and open decisions in docs/snap-store-publishing.md.

- [ ] Unify statuses the way monster abilities are (one row per kind, like
  `abilityRows`): key, hero/monster scope, state + clock fields, full and
  short card name, visual motif and its slot on the actor, tooltip/Identify
  and log lines, cure rules, save. Today these are parallel lists kept in
  sync by hand:
  - `Condition` enum + per-status fields on MMCharacter/Monster3D, party-wide
    root and slow on MMGame (`partyRoot`, `partyHinder`);
  - `statusVisualCatalog` + the `partyStatusVisuals`/`monsterStatusVisuals`
    switches (render_status_fx.go);
  - `partyConditionShortName` switch + `ui.status_*` texts;
  - poison/burn/stun card effects drawn outside the motif catalog (ui_hud.go).

  Collisions seen in the real render (2026-10-13), to fix with slots:
  - monster shred and melt draw the same split shield in the same spot, so
    melt hides shred;
  - hero card: the melt shield sits on the face, the slow hourglass right
    under it with the flakes falling through; Asleep Z's cross the shield;
    Fear lines merge with the Paralysis bolts.

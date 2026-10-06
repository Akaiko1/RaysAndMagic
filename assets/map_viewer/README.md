Map Viewer / Editor (Utility)
=============================

GUI tool to preview and edit maps plus browse game content and runtime previews.
Ten top tabs (F1-F10), selectable by click or hotkey:

1. **Maps** (F1) - preview/edit maps: biome-scoped legend, paint tiles and
   monsters, save back to the `.map` file.
2. **Items** (F2) - every weapon and item, grouped by category, full stats on hover.
3. **Spells** (F3) - every spell grouped by school and level.
4. **Characters** (F4) - every shipped starting hero, captive and tavern recruit:
   stats, skills, magic schools + known spells, starting equipment.
5. **Skills** (F5) - all skills with detailed descriptions of what they do.
6. **FX** (F6) - live previews of combat and environment effects.
7. **Mobs** (F7) - effective runtime stats, abilities, drops, and a live
   animation/AI preview. Champion values are mirrored from their real build.
   Wheel over the right panel to scroll long monster stat sheets.
8. **Save Stashes** (F8) - inspect save slots, party inventories, and stashes.
9. **Open World** (F9) - edit the unified-world map layout.
10. **Overlay** (F10) - author interactive zones, switches and boss patterns.

Content tabs are read-only catalogs built from the YAML configs and shared game
mechanics. Hover item, spell, skill, equipment, monster, NPC, and map elements
for detailed information.

Shop and editor tooltips use `game.GetItemTooltip` / `game.GetSpellTooltip`
with a nil bearer. They show base values and stat scaling without party buffs,
skills or active equipment bonuses. The inventory supplies the actual bearer
to the same builders. The editor always requests the full detail view.
Character equipment hovers and saved-item hovers use this same base path;
saved HP/SP are labeled as saved values, while item definitions use current YAML.
The card collection follows game-load validation: eight slots, collectible cards
only, physical-item precedence over legacy keys, and shared-stash ownership.
Pending stash transfers use the same recovery decision as the game, but browsing
never writes saves, migrates the stash, or clears its transaction journal.
Mechanics appear before description and flavor text. The Mobs and
FX 3D previews have no world hover selection. Map selection and dragging remain
available. The caravan preview follows a local loop without campaign trading
or route changes.

The legend is biome-aware: it shows only the tiles and monsters valid for the
current map's biome (universal ones plus that biome's own), rebuilding when you
switch maps. Each entry shows a color swatch matching how it's drawn on the map
grid, so the letter isn't the only cue.

Run from the repo root:

```
go run ./assets/map_viewer
```

Build a local binary:

```
mkdir -p bin
go build -o bin/map_viewer ./assets/map_viewer
```

Release archives include the viewer next to the game executable as `RaysAndMagicMapViewer` (macOS) or `RaysAndMagicMapViewer.exe` (Windows).

Controls:
- F1-F10 to switch top-level pages
- Left/Right (or A/D) to switch maps
- Tab or 1/2 to switch Info/Legend panel
- Mouse wheel or PgUp/PgDn/Up/Down to scroll legend
- Mouse wheel to zoom the map; right-drag to pan
- Click legend entry to choose a brush
- Click on the map to paint (in-memory only); HOLD to paint a stroke (tile/decor/eraser brushes)
- Hold the left button on a monster, NPC, special tile or prop to DRAG it to another cell
  (drop on the source cell changes nothing, Esc cancels the drag)
- Hold Shift while releasing a drag to COPY instead of move (ghost turns green)
- E to select the eraser quickly
- Toolbar buttons for Brush/Eraser/Save (bottom of map panel)
- Save opens a path prompt; Enter saves, Esc cancels
- Esc to quit

Interactive scene editing (Overlay, F10):
- Choose a map at the upper left. Each map retains its own unsaved draft.
- Select Trap, Hazard, Transfer or Lane and drag a rectangle over the map.
  Select a zone to move it; use Redraw area to resize it. Overlapping zones
  can be selected individually in the left list. Duplicate copies a zone.
- The indented scene tree groups boss settings, areas, switches and objects.
  Click a group heading to collapse it. Scroll either side panel independently.
- The inspector separates buttons, navigation cards, editable fields and inline
  toggles. Tab/Shift+Tab moves through editable controls; Enter opens or activates
  the focused control. Reference pickers are searchable; multi-select pickers
  connect zones to switches and lanes to attack volleys.
- Transfers have a Pick receiver tool. + Switch creates a placed mechanism with
  an operate action. Its inspector configures disabled zones and terrain changes;
  Edit switch object opens appearance and dialogue, including trap-disarm actions.
- + Boss chooses and places a boss from any biome. Its encounter page leads to
  Movement area, Mechanics and Attack patterns. The violet outline is the boss
  movement boundary, with a labeled edit button above the map. Movement area
  exposes tile width, height and position, plus redraw and remove controls.
- Mechanics has one editor each for regeneration, armor reduction, critical chance
  and perfect dodge. Switch pages show summaries and link to these editors.
  Regeneration requires a linked switch and stops after all its linked switches
  are operated. Critical/dodge reductions subtract percentage points down to the
  minimum; armor reductions subtract armor down to the armor minimum.
- Attack patterns contains cadence, lanes and nested health phases with volleys.
- + Object creates a local interactive object, places an existing NPC from the
  library, or places a working teleporter. Select its dialogue actions to edit
  their targets and parameters; info actions can contain nested follow-ups.
  Actions come from the game's dispatcher: travel, schedules, quests, quest props,
  encounters and rewards, exchanges, buffs, spell teaching and party services.
  Quest objectives and shop stock reuse the existing catalogs. Use Make local
  copy before changing a definition shared with another map. Appearance includes
  sprite, wall/floor presentation, size class and visibility conditions.
- Triggers retain their game semantics: traps fire on entry, hazards damage during
  occupation, transfers move the party, and lanes fire through boss actions or HP
  thresholds. Special trap tiles from the legacy catalog are decorative; use the
  Trap tool for a working trap. Arbitrary scripts and new event kinds need engine
  code. The engine currently supports one adventure boss policy per map.
- Stitched regions project local authored coordinates using their own rotation
  and reflection. Boss archetypes must be unique among merged regions. Operate a
  mechanism from inside its owning region, not across a region boundary.
- Map rules includes elemental visit ownership and the day/night entrance
  schedule. Jump links use two walkable endpoints two tiles apart.
- Wheel zooms the map or scrolls the panel under the pointer; right-drag pans.
  Escape cancels a tool. Ctrl/Cmd+S saves, Ctrl/Cmd+Z undoes, and
  Ctrl/Cmd+Shift+Z or Ctrl/Cmd+Y redoes, including objects, boss mechanics and placement.
- Validate checks runtime rules and links before saving. Invalid drafts stay
  editable. Linked objects must be unlinked before deletion. Renaming IDs
  updates their local references automatically.
- Save writes the selected map's adventure block in map_configs.yaml and only
  changed object entries in npcs.yaml; placement changes also update its .map.
  Critical/dodge rules use the optional boss_mechanics.yaml, keyed by map. Monster
  placements that cannot use the current biome's letter are encoded as
  [monster:exact_key@ground_tile], preserving both archetype and ground.
  Unrelated YAML entries and comments are preserved. Concurrent changes to the
  same entries or map are rejected. A failed write rolls back earlier writes.
  Undo and Redo also work across Save. Reload discards this map's in-memory edits
  in both Maps and Overlay and reads the files again.
- Switching tabs does not save or discard drafts. Closing with unsaved overlays
  asks before discarding them.

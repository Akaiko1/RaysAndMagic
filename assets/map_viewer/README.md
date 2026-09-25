Map Viewer / Editor (Utility)
=============================

GUI tool to preview and edit maps plus browse game content and runtime previews.
Nine top tabs (F1-F9), selectable by click or hotkey:

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
- F1-F9 to switch top-level pages
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

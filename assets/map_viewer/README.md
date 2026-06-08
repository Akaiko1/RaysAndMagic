Map Viewer / Editor (Utility)
=============================

GUI tool to preview and edit maps plus browse all game content. Five top tabs
(F1–F5), click or hotkey:

1. **Maps** (F1) — preview/edit maps: biome-scoped legend, paint tiles and
   monsters, save back to the `.map` file.
2. **Items** (F2) — every weapon and item, grouped by category, full stats on hover.
3. **Spells** (F3) — every spell grouped BY SCHOOL (battle then utility).
4. **Characters** (F4) — each playable class with its full starting loadout:
   stats, skills, magic schools + known spells, starting equipment.
5. **Skills** (F5) — all skills with detailed descriptions of what they do.

Content tabs are read-only catalogs built from the YAML configs (and, for
Characters, by instantiating each class), so they always match the live game.

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
- Left/Right (or A/D) to switch maps
- Tab or 1/2 to switch Info/Legend panel
- Mouse wheel or PgUp/PgDn/Up/Down to scroll legend
- Click legend entry to choose a brush
- Click on the map to paint (in-memory only)
- E to select the eraser quickly
- Toolbar buttons for Brush/Eraser/Save (bottom of map panel)
- Save opens a path prompt; Enter saves, Esc cancels
- Esc to quit

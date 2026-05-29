Map Viewer / Editor (Utility)
=============================

GUI tool to preview and edit maps: browse a biome-scoped legend, paint tiles
and monsters, and save back to the `.map` file. Also has an Items & Spells
content page.

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

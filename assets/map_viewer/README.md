Map Viewer (Utility)
====================

Simple GUI tool to preview maps with all objects and browse a legend.

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

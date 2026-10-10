# Image sizes

Recommended pixel sizes for game art, taken from what the game ships
(surveyed 2026-10-10). When you replace a file, keep the size of the file you
replace unless a row below says otherwise. Placement, naming and frame rules
live in [content authoring](../docs/content-authoring.md) and the
`how_to_add_a_new_*.md` guides in the repository root.

## General rules

- In-world size comes from `graphics.size_classes` in `config.yaml`, not from
  the pixel size. Pixel size is about detail, not about how big the sprite looks.
- A horizontal strip whose width is four times its height (`w == 4*h`, for
  example 2048x512) plays as a 4-frame animation.
- Downscale a sheet once, as a whole, never frame by frame: per-frame resampling
  makes the cycle shimmer. Convert to premultiplied alpha before resizing and
  back after, or edges show a halo.
- Save PNGs with real compression (for example Pillow `optimize=True,
  compress_level=9`).
- Keep generated sources and work files outside the repository; ship only the
  runtime files.

## World

| Element | Folder | Size | Layout and notes |
| --- | --- | --- | --- |
| Monster, static | `sprites/mobs/` | 512x512 | Cut from walking frame 0, same scale and baseline as the cycles. |
| Monster walk, attack, dying | `sprites/mobs/` | 1024x1024 | 2x2 grid of 512x512 frames; `_l`/`_r` = screen-left/right. A 2048x512 four-frame strip also works. |
| NPC idle | `sprites/characters/npcs/` | 2048x512 | 4-frame strip, figures aligned by the feet. Static NPC: 512x512. |
| Prop, structure, plant | `sprites/environment/props`, `structures`, `nature` | 512x512 | Content planted flush on the frame bottom, about 20 px headroom. Tall plants 512x1024; large structures 1024x512 or 1024x1024. |
| Wall texture | `sprites/environment/walls/` | 256x256 | Tiling texture. |
| Wall-mounted decor | `sprites/environment/walls/` | as authored | Transparent cut-out; animated decor is a 2048x512 strip. |
| Floor texture | `sprites/floor/` | 64x64 | Tiling texture. |
| Sky panorama | `sprites/sky/` | 2048x512 | `_day` / `_night` pairs; a few 1536x512 panoramas exist. |
| Buff effect | `sprites/interface/buffs/` | 2048x512 | 4-frame strip. |

## Interface

| Element | Folder | Size | Layout and notes |
| --- | --- | --- | --- |
| Item, weapon, armor, accessory, spell, trap, atlas and achievement icons | `sprites/interface/*` | 128x128 | Unframed art; the engine draws the frame (see `icon_frames.yaml`). Legacy 64x64 icons stay as they are. |
| Small glyph icons | `sprites/interface/icons/` | 24x24 | |
| Hero portraits | `sprites/characters/heroes/` | 64x64 and 256x256 | `<hero>.png` for the party card, `<hero>_full.png` for the full portrait. |
| Monster card art | `sprites/interface/cards/` | 1254x1254 | |
| Camp and theme backgrounds | `sprites/interface/camping/`, `theme/` | 1536x1024 | |
| Theme frames | `sprites/interface/theme/` | exact shipped size | The boot check rejects a frame of any other size (for example the 160x160 frames). |
| Panels | `sprites/interface/ui/` | shipped size of the panel | Each panel has its own size; keep the one you replace. |
| New decorative UI art | `sprites/interface/...` | about 4x its draw size | Measure the draw size in UI units; the largest interface scale is x4. Example: the merchant shop display draws at 164x109 and ships at 660x440, the frame corner draws at 64x64 and ships at 256x256. |

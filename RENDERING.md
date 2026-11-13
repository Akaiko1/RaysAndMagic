# Rendering and resource loading

Floor texture blending and authoring rules are documented in
[Floor texture transitions](docs/floor-transitions.md), including profile
compatibility, shorelines, and Dragon Cliffs orientation.

## Engine and build

The game and map editor use Ebitengine 2.10.6 and Go 1.27 or newer. Desktop
builds are pure Go; Cgo is not required. The module also pins the engine's
shadercollector as a Go tool, so collection and runtime use the same version.

`./build_bin.sh`, `./build_mac_release.sh`, and the Windows PowerShell build
scripts run `go run ./tools/shadergen` before building the executables.
The generator discovers marked Kage constants and engine built-ins, emits GLSL,
and compiles Metal or Windows DXBC when the host's native compiler is available.

- macOS: Xcode plus the Metal Toolchain (`xcodebuild -downloadComponent MetalToolchain`).
- Windows: Windows SDK `fxc.exe` on PATH, for example from a Developer Command Prompt.
- Cross-building Windows on macOS keeps DirectX runtime compilation unless FXC
  is available. It does not turn Metal libraries into DirectX binaries.

The GitHub release workflow first generates DirectX binaries on a Windows
runner with FXC, then downloads that artifact to the macOS job. The release
build adds Metal libraries and embeds both platforms. The merge keeps only
identical SourceIDs, so changed shaders/engine versions cannot reuse stale
binaries. The macOS job ensures the Metal Toolchain is available. No generated
archive is committed and no player-side developer tools are required.

Ebitengine 2.10 does not expose an API for exporting runtime-compiled GPU
shaders into an application-owned disk cache. The release therefore ships
precompiled artifacts; missing entries compile normally at runtime. Do not
confuse this with the self-populating pixel cache described below.

The generated `internal/shadercache/data/shaders.json.gz` is ignored by Git and
embedded in built executables. It is not a runtime dependency beside the app.
A clean checkout with no generated archive works with `go run .` or `go build`:
Ebitengine compiles shaders normally. Regenerate after changing shaders or the
engine; SourceIDs intentionally invalidate stale artifacts. Build generation
publishes only after successful compilation and reports the included backends.

Both executable entry points validate the embedded archive before loading game
content. A hidden child process draws every original Kage program with the
archive registered, then forces GPU submission. Only a successful probe allows
the main process to register the artifacts. Malformed archives, rejected native
binaries, probe failures and a 30-second timeout leave runtime compilation in
place. Registration is all-or-nothing: Ebitengine cannot unregister an invalid
native binary after registration. The archive retains the original Kage sources
for this check; it does not require external compilers on the player's machine.

Validation runs once per launch and adds a small startup cost; it never runs on
a region transition or reads GPU pixels during gameplay. No compatibility
verdict is persisted across driver or device changes. A missing archive skips
validation entirely.

Precompilation preserves Kage code, uniforms and sampling. Metal libraries skip
runtime MSL compilation; GLSL skips source conversion, but the OpenGL driver
still compiles and links it. Kage-to-IR work and backend pipeline setup remain.
See the [official 2.10 notes](https://ebitengine.org/en/documents/2.10.html) and
[shaderprecomp API](https://pkg.go.dev/github.com/hajimehoshi/ebiten/v2/exp/shaderprecomp).

## Loading invariant and scheduling

A playable frame is published only after its required regions, demand images,
floor atlas, uploads and shader warm-up are ready. Preparation may change its
schedule, never the frame's art, camera, filtering, LOD or resident region set.

CPU decoding and standee preparation run in background workers. Ebitengine
image creation/publication stays with the owner. One region owns in-flight GPU
allocations. The preparation reservation (32 MiB in play) is a queue
reservation, not a limit on all process RAM. Cancellation and generation
checks prevent a previous world's jobs from publishing into a new one.
Sprite decoding and standee sheet building run on background workers, with
byte reservations and result delivery kept in request order so smaller distant
images cannot overtake nearby ones (`graphics.PrepareOrdered`). Queued results
stay bounded and cancellation releases their reservations.

### Two paces: play and the loading screen

Play has frames to keep smooth; a held frame - the loading screen - has none.
`Renderer.loadingPace` is the one place that says how fast region loading may
run in each mode, and `applyLoadingPace` switches every limit each Update, so
a region that began loading in play speeds up the moment the frame is held.

| Limit | Play | Loading screen (`graphics.streaming.loading_screen`) |
|---|---|---|
| Scheduling passes per Update | `background_steps_per_tick`, within 1.5 ms | until 4 ms have passed |
| Empty worker queue | leave | wait for the worker until the deadline |
| Sprite decoders | 2 | `workers` (0 = logical CPUs minus 2, from 2 to 8) |
| Standee sheet builders | 1 | `workers` |
| Decoded pixels queued | 32 MiB | `queue_megabytes` |
| Texture uploads per Draw | 8 MiB, 32 images | `upload_megabytes_per_frame`, `upload_images_per_frame` |

The loading screen never runs below the play limits. The owner advances
resource preparation in 256 KiB pixel chunks. A pass cannot interrupt an
individual operation, so the time budgets are cooperative. Floor, demand and
region preparation all participate. The pass count bounds play because pixel
writes are cheap on the CPU while their GPU cost may be deferred until Draw.
Publishing a region defers the next region's planning until the following
Update, including during a pause. A held Update longer than 4 ms loads more
neighbour regions before the frame is released and ends later.

Measured on an 8-core laptop entering the forest (412 sprites, 578 standee
sheets, 1.8 GiB of textures): 5.0 s at the play quotas, where the decoders sat
3 s waiting for a game loop that polled once per tick; 2.4 s at the loading
pace. No single limit moves the time alone.

Lookups from the game loop never decode on its goroutine: during a tick a
missing source is queued through the deferred resource handler, and code that
runs after the handler is cleared (the deferred tail of Update) reads the
loading state instead of evaluating readiness.

### Open-world regions

A region is required, and its absence pauses the game, once the view sector
can reach it: view distance plus 4 tiles. Residency loads it
`graphics.streaming.prefetch_lead_tiles` earlier, in every direction, so a
walking party meets it already resident; it is retained 4 tiles further still
and evicted beyond. Measured on a straight walk across three regions at 6
tiles per second: 258 paused ticks and six "Loading area..." banners with no
lead, none with a lead of 24 tiles and 4 background steps.

A paused frame requires every region the view polygon sees as well as the
nearby batch. The polygon circumscribes its arc, so it reaches slightly past
the nearby radius; dropping such a region lifted the pause on Draw and raised
it again on the next Update.

Every tile has one owning region for preparation: its own footprint, or for
corridors and the void between maps the nearest footprint. Art that only
exists there is otherwise never planned and loads on first sight.

Lookups that run every frame must not request art for what the frame cannot
draw: an NPC past the view distance or behind the camera is rejected before
its size is resolved (`NPCSpriteMetrics`, shared by the hover hit test and the
renderer), and a building fetches its sprite only for a segment it draws. An
NPC in front of the camera but off to the side still has to be measured: its
size decides whether it reaches the screen. A request for a region that is not
resident is a pause with a banner.

An evicted region releases hundreds of textures. They are queued and disposed
`graphics.streaming.release_images_per_tick` (16) per Update; disposing them
in the eviction frame stalled presentation for 50 to 120 ms. A world switch
disposes the queue at once behind its barrier.

Demand uploads and region prewarm uploads use the same per-Draw quota of the
current pace, per queue. A standee sheet is queued once, in place of the
level views it holds. A first oversized image is admitted alone to
avoid starvation. Tiny demand images no longer consume a whole Draw each.
Transparent submission uses no GPU readback fence. Both queues must finish
before gameplay resumes. The loading banner and small-UI policy are unchanged.

## Per-frame worker pools

Two pools fan work out inside a frame and join before it continues: the
raycast (one job per 32 rays) and the monster update. Work goes to a pool only
when it costs more than waking the workers:

- Raycast, 1920 rays: 155 us on the pool, 502 us in a loop.
- Monsters: the pool pays from `engine.parallel_monster_minimum` (64) on; with
  4 monsters a loop takes 6 us and the pool 23 us. A smaller batch runs the
  same two phases (update against the frozen collision snapshot, then publish)
  on the game goroutine.
- Projectiles always move in a loop: one move is a few additions and a tile
  lookup, 4 to 10 times cheaper than its dispatch.

In the real game the pools change the frame by about 0.1 ms; more fan-out per
frame needs a measurement on the target machine first. A job's panic is raised
on the goroutine that waits for the batch (`core.JobPanic`), so it stops the
game like any other panic instead of repeating every tick. The per-tick
collision snapshot refills storage the collision system keeps
(`FrameSnapshot`); it is valid until the next one is taken.

Go's CPU profile on macOS attributes a large share to
`runtime.pthread_cond_signal`; process CPU time from `getrusage` and an A/B run
are the reliable measures of what waking threads costs.

## Disposable pixel cache

The cache stores exact premultiplied RGBA bytes using lossless compression:
standee stickers, wood cores and their mip chains, plus derived floor atlases.
It neither quantizes nor filters art differently. It retains no GPU images and
no extra decoded images after a preparation job finishes.

- Local runs: `.render-cache/` under the runtime working directory.
- macOS bundles: `.render-cache/` under the existing per-user runtime root,
  normally `~/Library/Application Support/RaysAndMagic/`.
- Tests: a private temporary root, or an explicitly disabled cache.

The key hashes source pixels and dimensions, ordered inputs, relevant settings
and an algorithm version. File timestamps are not trusted. Increment the
corresponding cache version when changing preparation, wood color, source-size
or mip algorithms. A fresh cache still decodes/prepares source art; subsequent
loads skip the cached derived work. PNG decoding and GPU uploads remain.

Entries validate their expected dimensions/count, key and compression checksum
before use. Corrupt, missing, cancelled, oversized or unwritable entries fall
back to preparation. Reads and compressed writes use 64 KiB I/O buffers; flush
and close must succeed before atomic publication. This prevents small deflate
writes from becoming individual filesystem calls. Each
entry is limited to 32 MiB of decoded pixels; larger resources simply bypass
this optional cache. After writes, old generated entries are pruned to 256 MiB
of disk space (another process can temporarily add entries concurrently).
Only `.rgba` entries in this private directory are candidates for removal.
Deleting the directory is safe and does not reset saves or statistics.

## Frame pacing and pixel metadata

The shipped game enables VSync on macOS as well as other desktop platforms.
Simulation still runs at the configured 120 TPS. With Ebitengine 2.10's unsynced
Metal path, a full drawable queue skips presentation without skipping offscreen
rendering. On the fullscreen river route this produced over 120 Draw calls per
second but single-digit drawable presentations. Keep
`display.disable_vsync_on_mac: false` for normal gameplay. The opt-out remains
available for diagnostics; it is not a performance optimization.

FPS reported by Ebitengine counts engine frames, not successful display
presentations. CPU Draw intervals, GC pauses and GPU fences alone cannot prove
smooth motion. Verify the user's fullscreen mode and display scale; compare
actual drawable presentations when displayed motion contradicts the counter.
VSync caps output to the display and applies backpressure under GPU load; it
does not promise that every scene reaches the monitor's refresh rate.

Camera presentation follows one fixed simulation timeline. Ebitengine 2.10 can
round a tick up before its nominal time. Keep the preceding pose segment so an
early Update does not reset interpolation or snap to the next segment. Loading,
TB, overlays, teleports and save loads retain their existing reset rules; only
RT presentation interpolates, never logical collision or gameplay positions.

Bounds and hit-test masks share an alpha reader. Common RGBA/NRGBA/Alpha sources
read their bytes directly, honoring origin and stride; other image types use
the original image.Image alpha contract. This avoids one allocated boxed color
per pixel during source preparation without changing pixels or alpha thresholds.

## Temporary image views

Incremental uploads share `graphics.WritePixelsRegion`. Text scratch strips,
nine-slice frames and journal material tiles use `RecyclableSubImage` and return
the wrapper after their last synchronous draw submission. Cached animation
frames, source images and retained UI patterns keep normal ownership. Never
recycle those long-lived objects or retain a recyclable wrapper in a cache.
The common `graphics.DrawImageScaled` filtering rule is unchanged.

## Draw commands, standee sheets and Panini cuts

The engine merges consecutive shader draws into one GPU command only when the
shader, blend, destination, backing textures and every uniform it reads are
equal. A shader that calls `imageSrcNOrigin`/`imageSrcNSize`, or samples
sources 1-3, reads per-image region uniforms, so draws of different sprites
never merge even on one atlas. Count commands, not triangles, when a scene is
slow: run with `-tags ebitenginedebug` and read the per-frame command list.

Standee tokens therefore keep their reduced mip levels in one sheet per token
(`render_standee_sheet.go`): each level is a block with a two-texel header
(full size, level size), the sticker and, to its right, the core. The chains'
levels above 0 are views of that sheet, written on the CPU by both the
synchronous and the streaming commit, so the sheet sits on a shared source
atlas from its first upload. `standee_sheet.kage` takes the block origin from
the vertex source position (the engine translates it to the atlas), the surface
and the mip blend from its fractions, and the sizes from the header; it reads
no region uniform. Level 0 and chains that do not pack keep
`standee_trilinear.kage` with four level images. Both paths sample at texel
centres and agree to one level in 255.

Measured on the forest start, 1920x1080, Wide: 824 commands per frame before,
229 after; the rest is the painter interleave with world-model faces.

Panini projects geometry on the CPU. A column-material quad (vertical sides,
attributes constant along each) is cut once per X knot it crosses, two shared
vertices per cut; other geometry goes through the triangle clipper, which
subdivides until attribute error is below a tenth of a pixel.

## Windows GPU choice

A cgo-free executable cannot export the `NvOptimusEnablement` /
`AmdPowerXpressRequestHighPerformance` symbols that make hybrid laptops pick
the discrete GPU. With `display.prefer_discrete_gpu_on_windows: true` the game
stores "High performance" for its executable in the per-user Graphics settings
(`internal/gpupref`) when the player has no entry there; an existing entry is
never changed. Windows applies it to processes started afterwards.

## Compass and crossed-standee geometry

The compass caches its static background and outlines separately, preserving
their order below and above the live minimap. Circles are built in local image
coordinates once per radius. Camera turns, RT/TB, world changes and save loads
do not invalidate this world-independent frame. Resizing replaces the images;
shutdown releases them. NPC dots, the heading pointer and minimap remain live.
The minimap uses a fixed six-pixel tile scale: increasing the viewport radius
reveals more terrain. It draws partial edge tiles and applies a cached circular
alpha mask, filling the disk up to its colored frame without a dark annulus or
stair-step edge. The old external black shadow is omitted. NPC markers share
this scale and stay wholly inside the disk. Out-of-world areas remain empty.
Local stroke tessellation can change a few pixels at the old circle-closing
seam; the rest is checked against the uncached draw with alpha-rounding tolerance.

The standee volume compositor (crossed trees, and fading tokens such as a
corpse, which it fades as one solid) groups adjacent screen columns with the
same effective wall clipping. Walls conservatively behind all layers are
irrelevant to that grouping. It interpolates reciprocal depth and U/depth for
both outer faces, then reconstructs depth and U per fragment. This preserves
perspective over a wide quad without skipping columns or changing shell counts,
texture sampling, art, LOD or alpha compositing. Genuine wall-depth or
wall-height changes still split spans. Conservative outer geometry is clipped by
the existing per-layer coverage tests in the shader.

Tinted tokens, single-face LOD, grazing faces and invalid volume intersections
retain the material renderer; a fading token there is drawn as its front face,
since translucent layers would stack into an opaque body. This change does not
introduce tree depth occlusion or alter floor blending. Both compass images and
standee meshes are derived render state and are never serialized.

## Verification cases

| Rule | Cases | Expected result |
| --- | --- | --- |
| Loading barrier | RT/TB, Update/Draw discovery, world/UI misses | No simulation or input escapes before a complete frame; banner animates |
| Scheduling | Background / loading pause, floor / demand / region | Bounded progress; same publication and cancellation owners |
| Upload batches | Empty / many tiny / byte limit / oversized | FIFO, exactly once, finite progress, no visible upload pixels |
| Region lifecycle | New world / generation reset / eviction / reload | Stale jobs cannot publish; shared retained resources survive |
| Pixel cache | Cold / new owner / edited pixels / changed tint or version | Same exact pixels; changed inputs cannot reuse stale data |
| Cache failure | Corrupt / unavailable / cancelled / size limit | Fall back safely without affecting saves or rendering |
| Shader backend | Accepted archive / rejected native binary / absent artifacts | Validate before registration; rejected or missing artifacts use runtime compilation |
| Shader archive | Corrupt checksum / invalid or duplicate ID / missing source / incomplete DXBC pair | No partial registration and no startup panic |
| Camera timing | Early / on-time / catch-up ticks, RT walk/run at 90/120/144/240 FPS; TB / pause / save load | Continuous fixed-timeline presentation; gameplay coordinates unchanged |
| Display pacing | Fullscreen / windowed, river / land, saved scene, VSync / explicit opt-out | Shipped synchronized output presents frames instead of free-running past a full Metal drawable queue; simulation stays at 120 TPS |
| Alpha metadata | RGBA / NRGBA / Alpha / generic, offset stride / sheet / transparent | Bounds and hit masks match the generic reference |
| Views | Full image / nonzero-origin subimage / recycled wrapper | Same pixels, correct ownership after submission |
| Compass frame | First/repeated draw, RT/TB heading, world replacement, resize, release/rebuild | Same layer order and live contents; reuse by radius only |
| Compass map | Small/medium/large radius, world interior/edge, near/far/diagonal/outside NPCs | Full in-world disk, circular clipping, more terrain at larger sizes, bounded live markers |
| Standee spans | Clear/distant/low/high/sloped walls, mirrored cutouts, 2-16 shells, screen edge | Perspective and openings preserved; irrelevant walls do not split spans |
| Standee fallback | Side fade, transient opacity, grazing/subpixel face | Existing material path and silhouettes preserved |
| Standee sheet | Levels 1-6, blend, tint, mirrored, single face, walls, column layout, Panini; eviction, world switch | Same pixels as the level images within 1/255; the sheet leaves with its token |
| Panini column quad | Several knots, viewport edge, varying side attributes | Two triangles per strip on the projected edges; other quads stay with the clipper |

Resource preparation is transient; save format and gameplay persistence do not
change. The disk-cache restart tests cover its separate persistence contract.

Run `go test ./...`, `go vet ./...`, and the focused live GPU tests. Finish any
code change with `./build_bin.sh`.

The native shader acceptance/rejection test uses isolated hidden GPU processes
on macOS or Windows:

```sh
RAM_SHADER_GPU_TESTS=1 go test ./internal/shadercache \
  -run '^TestShaderNativeValidationAndFallback$' -count=1 -v
```

Scripted gameplay captures were retired with the debug simulations. Measure
in the running game with the `/` FPS counter. CPU Draw duration and submitted
vertices are not GPU frame-time measurements; do not compare FPS while
concurrent builds/tests or system swapping distort it.

Ebitengine 2.10 also provides the experimental `exp/vmhost` workflow and its
`skills/run-ebitengine-app-headless/SKILL.md` in the downloaded engine module.
Use the same pinned engine for host and guest; opt in with `ebitenginevmguest`
only for test builds. Interleave Update ticks and Draw frames so loading/input
presentation barriers can advance. Use a private config/save root and record
actual image bounds; the host still needs a graphics context. VM execution is
for application verification, not representative gameplay FPS benchmarking.

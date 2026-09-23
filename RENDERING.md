# Rendering and resource loading

Floor texture blending and authoring rules are documented in
[Floor texture transitions](docs/floor-transitions.md), including profile
compatibility, shorelines, and Dragon Cliffs orientation.

## Engine and build

The game and map editor use Ebitengine 2.10.2 and Go 1.25 or newer. Desktop
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
allocations. The existing 32 MiB preparation reservation is unchanged; it is a
queue reservation, not a limit on all process RAM. Cancellation and generation
checks prevent a previous world's jobs from publishing into a new one.

The owner advances resource preparation in 256 KiB pixel chunks. Background
loading performs one scheduling pass per Update. While awaiting a complete
frame, it performs up to 32 passes or approximately 4 ms of work. A pass cannot
interrupt an individual operation, so this is a cooperative time budget. Floor,
demand and region preparation all participate; workers are never waited on.

Demand uploads and region prewarm uploads use the same policy: up to 32 images
and 8 MiB per queue per Draw. A first oversized image is admitted alone to
avoid starvation. Tiny demand images no longer consume a whole Draw each.
Transparent submission uses no GPU readback fence. Both queues must finish
before gameplay resumes. The loading banner and small-UI policy are unchanged.

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

For actual gameplay captures:

```sh
RAM_DEBUG_SIM=1 RAM_RIVER_PREVIEW=/absolute/output/directory \
  go test -tags debug ./internal/game -run '^TestRiverGameplay$' -count=1 -v
```

This uses production config, size classes, open-world stitching, Layout,
Update/Draw and the full loading barrier. It writes native PNGs without resizing.
Isolated shader tests are additional parity checks, not gameplay screenshots.
CPU Draw duration and submitted vertices are not GPU frame-time measurements.
Do not compare FPS while concurrent builds/tests or system swapping distort it.

Ebitengine 2.10 also provides the experimental `exp/vmhost` workflow and its
`skills/run-ebitengine-app-headless/SKILL.md` in the downloaded engine module.
Use the same pinned engine for host and guest; opt in with `ebitenginevmguest`
only for test builds. Interleave Update ticks and Draw frames so loading/input
presentation barriers can advance. Use a private config/save root and record
actual image bounds; the host still needs a graphics context. VM execution is
for application verification, not representative gameplay FPS benchmarking.

For moving-scene CPU submission diagnostics:

```sh
RAM_DEBUG_SIM=1 RAM_RIVER_CPU=/tmp/river.cpu \
  go test -tags debug ./internal/game -run '^TestRiverMotion$' -count=1 -v
```

The motion diagnostic reports Update/Draw and frame-interval p50/p95/p99/max,
allocations, GC cycles, loading frames and readbacks. It uses production movement
collision and rendering, but its outer loop is a test harness, not the shipped
120 TPS display clock. Camera timing is covered separately with early and
catch-up ticks. Neither diagnostic alone proves smooth displayed gameplay.

For the native-window route using a private copy of a bundle save:

```sh
RAM_DEBUG_SIM=1 RAM_NATIVE_RIVER=1 \
  RAM_RIVER_SAVE='/absolute/path/to/saves/save8.json' RAM_RIVER_OVERLAY=1 \
  go test -tags debug ./internal/game -run '^TestRiverNativeRoute$' -count=1 -v
```

This presents through the actual engine clock and display, first renders the
main menu, then loads the copied save with sound enabled. It uses the shipped
fullscreen/VSync settings and logs the logical viewport and display scale.
`RAM_RIVER_WINDOWED=1`, `RAM_RIVER_LAND=1`, and `RAM_RIVER_TURN=1` select the
windowed, nearby clearing and turning controls. `RAM_RIVER_CPU` and
`RAM_RIVER_TRACE` accept output file paths and start after the initial load.
The reported frame intervals are still engine-side measurements; use native
Metal presentation instrumentation to investigate dropped display frames.

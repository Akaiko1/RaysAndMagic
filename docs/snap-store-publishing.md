# Publishing to the Snap Store

Plan for shipping the game as a snap. Ubuntu App Center lists Snap Store
packages, so this is the route to the Ubuntu store. Nothing below is built yet.
Items marked **verify** are open questions or details to check against the
current snapcraft documentation before implementing them.

## 1. Owner actions (one time)

1. Create an Ubuntu One account and log in: `snapcraft login`.
2. Reserve the name: `snapcraft register raysandmagic`.
3. Content rights check before the first public release:
   - Music (Suno): commercial rights cover only tracks generated while a paid
     subscription was active. Tracks generated on the free plan are not covered
     by a later upgrade; regenerate them on a paid plan. Keep generation dates and
     subscription receipts. Re-read the current Suno Terms of Service, which
     changed several times in 2025-2026.
   - Art (hand-made and ChatGPT): output rights belong to the author under the
     OpenAI terms; distribution is fine.
   - Fonts: Alagard and Romulus ("100% Free", credit Hewett Tsoi), Tiny RPG (CC0),
     bitmapfont (Apache-2.0, OFL-1.1 and others). Notices ship in
     `assets/fonts/*/`. Add a credits line to the store description.
4. Decide the package license. The repository has no LICENSE file, so the
   default is `license: Proprietary`.
5. Store listing material: a square icon of at most 512x512 (resize
   `assets/app_icons/rays_and_magic_icon.png`, currently 1254x1254), 3-5
   screenshots (README shots), title, summary (at most 78 characters),
   description, category Games, contact address, website
   (https://akaiko1.github.io/RaysAndMagic/).

## 2. Code changes

### Save and content storage

A snap installs read-only under `/snap/raysandmagic/<revision>/`. Today only the
macOS `.app` path seeds a writable copy (`storage.setupBundleRuntime`); a bare
Linux binary would write `saves/` next to itself and fail.

Writable locations a strict snap gets:

| Variable | Path | Behavior |
| --- | --- | --- |
| `SNAP_USER_DATA` | `~/snap/raysandmagic/<revision>/` | Per revision. snapd copies the whole directory on every refresh, which with about 0.6 GB of seeded assets would mean a full copy per update. |
| `SNAP_USER_COMMON` | `~/snap/raysandmagic/common/` | Shared by all revisions, survives refresh and revert. |

Plan:

- Detect a snap install by the `SNAP` environment variable. Content dir:
  the shipped tree inside `$SNAP`. User data dir: `$SNAP_USER_COMMON/RaysAndMagic`.
- Reuse the macOS seeding: copy the shipped content into the user data dir and
  chdir there; `saves/`, the stash and the profile live in that directory. The
  seed manifest already refreshes changed files and prunes what a new build no
  longer ships.
- Do not derive the data dir from `os.UserConfigDir()` inside a snap: HOME points
  at the per-revision directory there.
- Tests: extend the existing storage path tests with snap rows: snap with
  content, snap without content, and unchanged rows for the macOS bundle and the
  bare binary.
- **verify** `snap revert`: the common dir is not rolled back, so an older
  binary would run on content seeded by a newer one. Under today's rule ("an
  older stamp never overwrites newer content") it would keep the newer assets
  and newer saves. Decide whether a revert should reseed, and make sure an older
  build refuses a save from a newer format instead of corrupting it.

### Linux build

- Ebitengine needs cgo on Linux (GLFW, X11, GL). The macOS and Windows release
  builds use `CGO_ENABLED=0`; the Linux build must use `CGO_ENABLED=1`.
- Run `go run ./tools/shadergen` before `go build`, as `build_bin.sh` does.
  Linux uses the GLSL output.
- Build packages are the ones CI already installs: `libgl1-mesa-dev
  libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev
  libasound2-dev pkg-config`.

## 3. Packaging: `snap/snapcraft.yaml`

Sketch, to be completed during implementation:

```yaml
name: raysandmagic
title: Rays and Magic
base: core24
adopt-info: game            # version set from the git tag in override-build
summary: <at most 78 characters>
description: |
  <store description, including font credits>
license: Proprietary
grade: stable
confinement: strict
platforms:
  amd64:                    # arm64 later, if wanted

apps:
  raysandmagic:
    command: bin/RaysAndMagic
    desktop: usr/share/applications/raysandmagic.desktop
    plugs: [x11, wayland, opengl, desktop, desktop-legacy, audio-playback, joystick]

parts:
  game:
    plugin: nil
    source: .
    build-snaps: [go]
    build-packages: [libgl1-mesa-dev, libxcursor-dev, libxi-dev, libxinerama-dev,
                     libxrandr-dev, libxxf86vm-dev, libasound2-dev, pkg-config]
    stage-packages: [<runtime GL, X11 and ALSA libraries>]
    override-build: |
      go run ./tools/shadergen
      CGO_ENABLED=1 go build -o $CRAFT_PART_INSTALL/bin/RaysAndMagic .
      # copy config.yaml + assets/ to the content dir the storage code expects
      # set the version from the tag: craftctl set version=...
```

Details to verify:

- **verify GPU**: on core24 Mesa comes from the `gpu-2404` interface and the
  mesa-2404 content snap, set up through a command-chain wrapper. Follow the
  current snapcraft GPU guide instead of staging Mesa by hand.
- **verify audio**: the Ebitengine audio backend (oto) talks to ALSA. Under
  strict confinement, route ALSA to PulseAudio (stage the ALSA pulse plugin and
  an `asound.conf` through a layout) so that `audio-playback` is enough. The
  `alsa` interface is not auto-connected.
- `joystick` only if gamepad input is supported; drop it otherwise.
- Desktop file and icon: `snap/gui/` or a staged `.desktop` file pointing at a
  512x512 icon.
- Size: the shipped content is about 0.6 GB; the store accepts it.
- Map editor: open question. If it ships, add a second app
  (`raysandmagic.editor`) that shares the same data dir, as on macOS.

## 4. CI publishing

- New job in `.github/workflows/release.yml` after `check-tag`, on
  `ubuntu-latest`: `snapcore/action-build`, then `snapcore/action-publish`.
- Store credentials: run `snapcraft export-login` with limited ACLs for this snap
  only and save the output as the repository secret
  `SNAPCRAFT_STORE_CREDENTIALS`. Never commit it.
- Channel follows the tag scheme: `v<version>.nightly` goes to `edge`,
  `v<version>.stable` goes to `stable` (or to `candidate` first, then promote).
- Downloads page and README: add the snap install line
  (`sudo snap install raysandmagic`) and an App Center link.

## 5. Test before stable

Install from `edge` on a real Ubuntu 24.04 x86-64 machine. A Multipass VM on
Apple Silicon is arm64 and cannot run an amd64 snap.

- Launch, GPU acceleration, X11 and Wayland sessions, fullscreen and 4K scaling.
- Audio: music and effects, volume settings.
- Saves: create, quit, relaunch; `snap refresh` to a new revision keeps them;
  `snap revert` behaves as decided above; the shared stash and the profile survive.
- Seeding update: install revision A, refresh to B, check that files B no longer
  ships are pruned and that edited maps follow the seeding rules.
- Denials: run with `snappy-debug` and fix any AppArmor or seccomp denials.

## 6. First release

1. First upload by hand: `snapcraft upload --release=edge raysandmagic_*.snap`.
2. Fill in the store listing: icon, screenshots, description, license, contact.
3. Automated review: a strict snap with standard interfaces normally passes
   without manual review.
4. After the checks in section 5, promote to `stable`; later releases go
   through CI.

## Open decisions

- Package license: `Proprietary`, or open the code under a separate license.
- Ship the map editor in the snap or not.
- `snap revert` semantics for seeded content and saves.
- Add arm64 or not.
- A credits screen in the game, or credits in the store description only.

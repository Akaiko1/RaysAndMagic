package game

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
)

// Optional production-render gallery; no player saves or settings are changed.
func TestSolsticeArtGallery(t *testing.T) {
	dir := os.Getenv("RAM_SOLSTICE_ART_GALLERY")
	if dir == "" || os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("set RAM_SOLSTICE_ART_GALLERY to inspect actual map rendering")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, cfg := bootOpenWorldGame(t, false)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"solstice_approach", "solstice_vestibule", "solstice_fire", "solstice_water", "solstice_earth", "solstice_air"} {
		if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
			t.Fatal(err)
		}
		for _, width := range []int{1024, 1440} {
			height := width * 3 / 4
			cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = width, height
			g.camera.FOV = squareProjectionFOV(width, height)
			for view, angle := range []float64{-math.Pi / 2, 0} {
				g.camera.Angle = angle
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(width, height)
					defer dst.Deallocate()
					g.gameLoop.renderer.RenderFirstPersonView(dst)
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%d-%d.png", key, width, view)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					pixels := image.NewRGBA(dst.Bounds())
					dst.ReadPixels(pixels.Pix)
					if err := png.Encode(f, pixels); err != nil {
						t.Error(err)
					}
				})
			}
		}
		// Inspect actual rooms and landmarks, not only the entrance corridor.
		views := map[string][][3]float64{
			"solstice_approach":  {{16, 6, -math.Pi / 2}, {4, 28, -math.Pi / 2}, {12, 17, -math.Pi / 2}},
			"solstice_vestibule": {{12, 9, -math.Pi / 2}},
			"solstice_fire":      {{6, 17, -math.Pi / 2}, {20, 20, -math.Pi / 2}, {27, 8, -math.Pi / 2}},
			"solstice_water":     {{16, 7, -math.Pi / 2}, {26, 17, -math.Pi / 2}, {15, 23, -math.Pi / 2}},
			"solstice_earth":     {{6, 17, -math.Pi / 2}, {20, 10, -math.Pi / 2}, {28, 9, -math.Pi / 2}},
			"solstice_air":       {{6, 17, -math.Pi / 2}, {26, 17, -math.Pi / 2}, {25, 8, -math.Pi / 2}},
		}
		for _, width := range []int{1024, 1440} {
			height := width * 3 / 4
			cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = width, height
			g.camera.FOV = squareProjectionFOV(width, height)
			for view, position := range views[key] {
				placePlayerAtTile(g, int(position[0]), int(position[1]), float64(cfg.GetTileSize()))
				g.camera.Angle = position[2]
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(width, height)
					defer dst.Deallocate()
					g.gameLoop.renderer.RenderFirstPersonView(dst)
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-room-%d-%d.png", key, width, view)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					pixels := image.NewRGBA(dst.Bounds())
					dst.ReadPixels(pixels.Pix)
					if err := png.Encode(f, pixels); err != nil {
						t.Error(err)
					}
				})
			}
		}
	}
}

func TestSolsticeMonsterGallery(t *testing.T) {
	dir := os.Getenv("RAM_SOLSTICE_MONSTER_GALLERY")
	if dir == "" || os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("optional production monster render gallery")
	}
	t.Chdir("../..")
	_, _, cfg := bootOpenWorldGame(t, false)
	cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = 640, 480
	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.g.Shutdown)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cinder_hound", "kiln_automaton", "sunscarab", "dusk_moth", "anchor_automaton", "gale_elemental", "rain_serpent", "storm_kite", "rootbound_sentinel", "cistern_eel", "vaelith", "jade_elemental", "tide_elemental", "flow_automaton", "ember_elemental", "aureth", "neressa", "talura", "transfer_automaton", "shardback_burrower"} {
		p.Select("solstice_" + name)
		if len(p.arena.Monsters) == 0 {
			t.Fatalf("missing monster %s", name)
		}
		m := p.arena.Monsters[0]
		p.arena.Monsters = []*monster.Monster3D{m}
		m.AnimateWhenIdle = true
		m.State = monster.StateIdle
		m.Direction = math.Pi
		m.StandeeYaw = 3 * math.Pi / 2
		m.StandeeYawTick = 0
		p.g.monsterCorpses = nil
		for _, state := range []string{"walking", "attacking", "dying"} {
			anim := p.g.sprites.GetAnimation(m.GetSpriteType(), state+"_r")
			if anim == nil || len(anim.Frames) != 4 {
				t.Fatalf("%s/%s: missing four-cell animation", name, state)
			}
			if state == "dying" {
				p.g.beginMonsterDeath(m)
				p.arena.Monsters = nil
			}
			for frame := 0; frame < 4; frame++ {
				m.AttackAnimFrames = 0
				switch state {
				case "walking":
					p.g.frameCount = int64(frame * p.g.gameLoop.renderer.monsterWalkTicksPerFrame())
				case "attacking":
					total := p.g.monsterAttackAnimationDuration(m)
					m.AttackAnimFrames = total - (total*frame+3)/4
				case "dying":
					age := float64(frame) / float64(p.g.monsterDeathSettings().FPS)
					if frame == 3 && m.Flying {
						age = math.Max(age, p.g.monsterDeathSettings().FallSeconds)
					}
					p.g.frameCount = p.g.monsterCorpses[0].started + int64(math.Ceil(age*float64(cfg.GetTPS())))
				}
				runOnDrawFrame(func(_ *ebiten.Image) {
					scene := p.Scene()
					pixels := image.NewRGBA(scene.Bounds())
					scene.ReadPixels(pixels.Pix)
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", name, state, frame)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, pixels); err != nil {
						t.Error(err)
					}
				})
			}
		}
	}
}

func TestSolsticeJunctionGallery(t *testing.T) {
	dir := os.Getenv("RAM_SOLSTICE_ART_GALLERY")
	if dir == "" || os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("optional open-world junction render gallery")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, cfg := bootOpenWorldGame(t, true)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for i, view := range []struct {
		key   string
		x, y  int
		angle float64
	}{{"deep_jungle", 47, 28, 0}, {"solstice_approach", 4, 29, math.Pi}, {"solstice_approach", 4, 28, -math.Pi / 2}} {
		if err := g.transitionToMap(mapTransition{mapKey: view.key, arrival: mapArrivalEntrance}); err != nil {
			t.Fatal(err)
		}
		x, y := wm.ProjectTile(view.key, view.x, view.y)
		placePlayerAtTile(g, x, y, cfg.GetTileSize())
		g.camera.Angle = view.angle
		for _, width := range []int{1024, 1440} {
			height := width * 3 / 4
			cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = width, height
			g.camera.FOV = squareProjectionFOV(width, height)
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(width, height)
				defer dst.Deallocate()
				g.gameLoop.renderer.RenderFirstPersonView(dst)
				pixels := image.NewRGBA(dst.Bounds())
				dst.ReadPixels(pixels.Pix)
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("junction-%d-%d.png", i, width)))
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				if err := png.Encode(f, pixels); err != nil {
					t.Error(err)
				}
			})
		}
	}
}

func TestSolsticeNPCGallery(t *testing.T) {
	dir := os.Getenv("RAM_SOLSTICE_NPC_GALLERY")
	if dir == "" || os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("optional production NPC idle gallery")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, cfg := bootOpenWorldGame(t, true)
	loadBenchContent(t)
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "solstice"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var master *character.NPC
	for _, npc := range g.world.NPCs {
		if npc.Key == "solstice_velna" {
			master = npc
		}
	}
	if master == nil {
		t.Fatal("missing surveyor")
	}
	// Two tiles away keeps both soles above the party HUD for landing QA.
	g.setPartyPosition(g.camera.X, g.camera.Y+cfg.GetTileSize())
	for _, name := range []string{"solstice_velna", "solstice_tovan", "solstice_sera"} {
		npc := *master
		npc.Sprite = name
		g.world.NPCs = []*character.NPC{&npc}
		for frame := 0; frame < 4; frame++ {
			g.frameCount = int64(frame * cfg.GetTPS() / NPCIdleAnimationFPS)
			runOnDrawFrame(func(_ *ebiten.Image) {
				g.handleResize(image.Pt(1024, 768), image.Pt(1024, 768))
				dst := ebiten.NewImage(1024, 768)
				defer dst.Deallocate()
				beginUIFrame(dst, g.uiPixelScale())
				g.gameLoop.drawExplorationFrame(dst)
				pixels := image.NewRGBA(dst.Bounds())
				dst.ReadPixels(pixels.Pix)
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%d.png", name, frame)))
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				if err := png.Encode(f, pixels); err != nil {
					t.Error(err)
				}
			})
		}
	}
}

func TestSolsticeAlchemyGallery(t *testing.T) {
	dir := os.Getenv("RAM_SOLSTICE_ALCHEMY_GALLERY")
	if dir == "" || os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("optional alchemy layout gallery")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, cfg := bootOpenWorldGame(t, false)
	loadBenchContent(t)
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "solstice"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g.menuOpen, g.showPartyStats, g.currentTab, g.selectedChar = true, true, TabSpellbook, 1
	for i, c := range g.party.Members {
		if c.Class == character.ClassAlchemist {
			g.selectedChar = i
		}
	}
	for _, size := range [][2]int{{1280, 720}, {1024, 768}, {1047, 680}} {
		cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = size[0], size[1]
		for _, key := range []string{"health_potion", "solstice_fire_ward"} {
			for i, r := range config.GlobalAlchemy.Recipes {
				if r.Key == key {
					g.selectedRare = i
				}
			}
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(size[0], size[1])
				defer dst.Deallocate()
				beginUIFrame(dst, 1)
				g.gameLoop.ui.Draw(dst)
				pixels := image.NewRGBA(dst.Bounds())
				dst.ReadPixels(pixels.Pix)
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%dx%d.png", key, size[0], size[1])))
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				if err := png.Encode(f, pixels); err != nil {
					t.Error(err)
				}
			})
		}
	}
}

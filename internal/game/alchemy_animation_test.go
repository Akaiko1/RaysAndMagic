package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
)

func stockAlchemyRecipe(g *MMGame, r *config.AlchemyRecipe, batches int) {
	g.party.Inventory = nil
	for _, group := range r.Ingredients {
		a := group.Alternatives[0]
		it, _ := items.TryCreateItemFromYAML(a.Items[0])
		it.Quantity = a.Count * batches
		g.party.AddItem(it)
	}
}
func TestAlchemyAnimationCommitAndLifetime(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		for _, route := range []string{"click", "enter"} {
			t.Run(fmt.Sprintf("%v/%s", size, route), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				c := character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
				g.party.Members[0] = c
				g.selectedChar = 0
				g.currentTab = TabSpellbook
				g.menuOpen = true
				r := config.AlchemyRecipeByKey("mana_potion")
				for i, recipe := range config.GlobalAlchemy.Recipes {
					if recipe.Key == r.Key {
						g.selectedRare = i
					}
				}
				g.alchemy.Choices = map[string][]int{r.Key: {0, 2}}
				g.party.Inventory = nil
				for _, key := range []string{"mooncap", "bitterroot", "embercap"} {
					it, _ := items.TryCreateItemFromYAML(key)
					g.party.AddItem(it)
				}
				before := append([]items.Item(nil), g.party.Inventory...)
				l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, true)
				cast := func() {
					if route == "click" {
						h.clicks(false, l.actions.x+25, l.actions.y+10, 1)
					} else {
						ih := NewInputHandler(g)
						ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyEnter })
						ih.keys.BeginFrame()
						ih.handleRareBookInput()
					}
				}
				cast()
				fx := g.brewAnimation
				if fx == nil || fx.Output.Name != "Brewed Mana Potion" || fx.Count != character.AlchemyYield(c.SkillTier(character.SkillAlchemy), r.Family) {
					t.Fatal("successful UI craft did not start matching animation")
				}
				assertAlchemyReceipt(t, before, g.party.Inventory, fx.Ingredients)
				inventory := append([]items.Item(nil), g.party.Inventory...)
				cast()
				if !reflect.DeepEqual(inventory, g.party.Inventory) {
					t.Fatal("repeat crafted during animation")
				}
				beforeWorld := g.frameCount
				beforeUI := g.uiFrameCount
				frames := fx.Frames
				h.pointerStep()
				if fx.Frames <= frames || g.uiFrameCount <= beforeUI || g.frameCount != beforeWorld {
					t.Fatal("paused UI animation clock did not advance independently")
				}
				for g.brewAnimation != nil {
					g.updateInterfacePresentation()
				}
				if !reflect.DeepEqual(inventory, g.party.Inventory) {
					t.Fatal("animation expiry changed committed inventory")
				}
			})
		}
	}
}
func TestAlchemyAnimationFailuresAndInterruptions(t *testing.T) {
	for _, state := range []string{"missing", "unsafe", "close", "tab", "hero", "load", "new_game"} {
		t.Run(state, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			g.menuOpen = true
			g.currentTab = TabSpellbook
			g.selectedRare = 0
			r := &config.GlobalAlchemy.Recipes[0]
			stockAlchemyRecipe(g, r, 1)
			if state == "missing" {
				g.party.Inventory = nil
			}
			if state == "unsafe" {
				zoneVictim(t, g)
			}
			ok := g.brewSelectedRecipe()
			if state == "missing" || state == "unsafe" {
				if ok || g.brewAnimation != nil {
					t.Fatal("failed brew started animation")
				}
				return
			}
			if !ok || g.brewAnimation == nil {
				t.Fatal("setup craft failed")
			}
			inv := append([]items.Item(nil), g.party.Inventory...)
			switch state {
			case "close":
				g.menuOpen = false
			case "tab":
				g.currentTab = TabInventory
			case "hero":
				g.selectedChar = 1
			case "load":
				g.restoreSavedTurnState(&GameSave{})
				if g.brewAnimation != nil {
					t.Fatal("loading retained a transient brew animation")
				}
			case "new_game":
				g.resetTimedEffects()
				if g.brewAnimation != nil {
					t.Fatal("new game retained a transient brew animation")
				}
			}
			g.updateInterfacePresentation()
			if g.brewAnimation != nil || !reflect.DeepEqual(inv, g.party.Inventory) {
				t.Fatal("interruption kept FX or changed committed batch")
			}
		})
	}
}

func TestAlchemyAnimationEveryProduct(t *testing.T) {
	g, _ := rareClassGame(t, character.ClassAlchemist, false)
	for i := range config.GlobalAlchemy.Recipes {
		r := &config.GlobalAlchemy.Recipes[i]
		t.Run(r.Key, func(t *testing.T) {
			g.brewAnimation = nil
			g.selectedRare = i
			g.alchemyBatches = 3
			stockAlchemyRecipe(g, r, 3)
			if !g.brewSelectedRecipe() {
				t.Fatal("authored recipe could not start a batch")
			}
			d, _ := config.GetItemDefinition(r.Output)
			fx := g.brewAnimation
			if fx == nil || fx.Output.Name != d.Name || fx.Color != d.BrewColor || fx.Color == [3]int{} || len(fx.Ingredients) != len(r.Ingredients) {
				t.Fatal("animation does not represent the committed product")
			}
			count := 0
			for _, item := range g.party.Inventory {
				if item.Name == d.Name {
					count += item.Count()
				}
			}
			if fx.Count != count || count <= 0 {
				t.Fatal("animation quantity differs from committed inventory")
			}
		})
	}
}

func TestFlaskSpriteTravelAndVisibility(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
			t.Run(fmt.Sprintf("%s/TB=%v", key, tb), func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassAlchemist, tb)
				d, _ := config.GetItemDefinition(key)
				it, _ := items.TryCreateItemFromYAML(key)
				it.Quantity = 1
				g.party.AddItem(it)
				if !g.throwFlask(0, key, false) {
					t.Fatal("throw refused")
				}
				p := g.magicProjectiles[len(g.magicProjectiles)-1]
				if p.FlaskFlightRange != p.FlaskRemaining || p.FlaskFlightRange <= 0 {
					t.Fatal("throw omitted visual travel snapshot")
				}
				g.depthBuffer = nil
				g.camera.ViewDist = p.FlaskFlightRange * 2
				g.camera.FOV = math.Pi / 2
				r := &Renderer{game: g}
				var poses []flaskRenderPose
				wrapper := MagicProjectileWrapper{MagicProjectile: &p}
				for _, fraction := range []float64{.1, .5, .95} {
					travel := fraction * p.FlaskFlightRange
					wrapper.SetPosition(g.camera.X+travel, g.camera.Y)
					pose, ok := r.flaskProjectilePose(p)
					if !ok || math.IsNaN(pose.y) {
						t.Fatal("active projectile invisible")
					}
					poses = append(poses, pose)
				}
				if poses[1].y >= poses[0].y || poses[2].y <= poses[1].y || poses[0].angle == poses[1].angle || poses[0].size <= poses[2].size {
					t.Fatal("no rise/fall/spin/perspective")
				}
				cameraX := g.camera.X
				g.camera.X += float64(g.config.GetTileSize()) / 2
				g.camera.Angle = .1
				moved, ok := r.flaskProjectilePose(p)
				if !ok || moved.angle != poses[2].angle {
					t.Fatal("moving or turning the camera reset flight progress")
				}
				g.camera.X, g.camera.Angle = cameraX, 0
				p.Active = false
				if _, ok := r.flaskProjectilePose(p); ok {
					t.Fatal("inactive flask rendered")
				}
				p.Active = true
				p.X = g.camera.X - 100
				if _, ok := r.flaskProjectilePose(p); ok {
					t.Fatal("flask behind camera rendered")
				}
				p.X = g.camera.X + 100
				g.depthBuffer = make([]float64, g.config.GetScreenWidth())
				for i := range g.depthBuffer {
					g.depthBuffer[i] = 50
				}
				if _, ok := r.flaskProjectilePose(p); ok {
					t.Fatal("flask rendered through a wall")
				}
				// Decode through the runtime sprite catalog as well as verifying raw alpha.
				t.Chdir("../..")
				sm := graphics.NewSpriteManager()
				if !sm.HasSprite(d.Flask.Sprite) {
					t.Fatal("authored flask sprite is outside indexed roots")
				}
				path, ok := graphics.ResolveSpritePath(d.Flask.Sprite)
				if !ok {
					t.Fatal("sprite not resolvable")
				}
				f, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				img, err := png.Decode(f)
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
					t.Fatal("unexpected projectile dimensions")
				}
				for _, xy := range [][2]int{{0, 0}, {127, 0}, {0, 127}, {127, 127}} {
					_, _, _, a := img.At(xy[0], xy[1]).RGBA()
					if a != 0 {
						t.Fatal("opaque projectile background")
					}
				}
				white := 0
				for y := 0; y < 128; y++ {
					for x := 0; x < 128; x++ {
						r, g, b, a := img.At(x, y).RGBA()
						if r > 55000 && g > 55000 && b > 55000 && a > 55000 {
							white++
						}
					}
				}
				if white < 30 {
					t.Fatal("missing white outline")
				}
				// Exercise the real rendering dispatcher, including visibility gates.
				// Pixel readback requires the live Ebiten loop provided by debug sims.
				if os.Getenv("RAM_DEBUG_SIM") == "" {
					return
				}
				g.sprites = sm
				g.depthBuffer = nil
				p.X = g.camera.X + p.FlaskFlightRange/2
				p.FlaskRemaining = p.FlaskFlightRange / 2
				screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
				defer screen.Deallocate()
				for _, active := range []bool{true, false} {
					p.Active = active
					g.magicProjectiles = []MagicProjectile{p}
					runOnDrawFrame(func(_ *ebiten.Image) {
						screen.Clear()
						r.drawMagicProjectiles(screen)
						pixels := make([]byte, 4*screen.Bounds().Dx()*screen.Bounds().Dy())
						screen.ReadPixels(pixels)
						visible := 0
						for i := 3; i < len(pixels); i += 4 {
							if pixels[i] > 0 {
								visible++
							}
						}
						if active && visible < 30 || !active && visible != 0 {
							t.Errorf("flask dispatcher active=%v visible=%d", active, visible)
						}
					})
				}
			})
		}
	}
}

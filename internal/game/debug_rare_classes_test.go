//go:build debug

package game

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
)

func TestDebugSim_RareClassBooks(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		for _, class := range []character.CharacterClass{character.ClassAlchemist, character.ClassWayfarer} {
			t.Run(fmt.Sprintf("%d/%d", size[0], class), func(t *testing.T) {
				oldFrames := config.GlobalIconFrames
				config.GlobalIconFrames = nil
				t.Cleanup(func() { config.GlobalIconFrames = oldFrames })
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				t.Chdir("../..")
				if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
					t.Fatal(err)
				}
				g.sprites = graphics.NewSpriteManager()
				g.showPartyStats = true
				name := "Elara"
				if class == character.ClassWayfarer {
					name = "Kael"
				}
				c := character.CreateCharacter(name, class, g.config)
				c.Level = 8
				c.SpellPoints = 100
				g.party.Members[0] = c
				g.selectedChar = 0
				g.menuOpen = true
				g.currentTab = TabSpellbook
				g.party.GrantClassItems(g.config)
				if class == character.ClassWayfarer {
					g.selectedRare = 3
					c.RareClass.Automatic = map[string]bool{"purify": true}
				}
				count := len(config.GlobalTechniques.Techniques)
				if class == character.ClassAlchemist {
					count = len(config.GlobalAlchemy.Recipes)
				}
				for selected := 0; selected < count; selected++ {
					g.selectedRare = selected
					if class == character.ClassAlchemist {
						for _, group := range config.GlobalAlchemy.Recipes[selected].Ingredients {
							for _, alt := range group.Alternatives {
								for _, key := range alt.Items {
									it, _ := items.TryCreateItemFromYAML(key)
									it.Quantity = 12
									g.party.AddItem(it)
								}
							}
						}
					}
					runOnDrawFrame(func(_ *ebiten.Image) {
						h.ui.Draw(h.screen)
						path := filepath.Join(os.TempDir(), fmt.Sprintf("rare_book_%d_%s_%02d.png", size[0], c.GetClassKey(), selected))
						f, err := os.Create(path)
						if err != nil {
							t.Error(err)
							return
						}
						defer f.Close()
						if err = png.Encode(f, snapshotUIImage(h.screen)); err != nil {
							t.Error(err)
						}
					})
				}

			})
		}
	}
}

func TestDebugSim_PartyRarityGroups(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	oldFrames := config.GlobalIconFrames
	config.GlobalIconFrames = nil
	t.Cleanup(func() { config.GlobalIconFrames = oldFrames })
	h := newDisplayedModalHarness(t, 1920, 1080)
	t.Chdir("../..")
	if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
		t.Fatal(err)
	}
	h.g.sprites = graphics.NewSpriteManager()
	h.g.appScreen = AppScreenPartyCreate
	h.g.partyCreate = newPartyCreateState(h.g.config)
	runOnDrawFrame(func(_ *ebiten.Image) {
		h.ui.drawPartyCreateScreen(h.screen)
		f, err := os.Create(filepath.Join(os.TempDir(), "party_rarity_groups.png"))
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		if err = png.Encode(f, snapshotUIImage(h.screen)); err != nil {
			t.Error(err)
		}
	})
}

// Exercise the real world-loading gate, Update and Draw after Begin Adventure.
func TestDebugSim_RarePartyStartup(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, cfg := bootOpenWorldGame(t, true)
	previous := config.GlobalAlchemySpawns
	t.Cleanup(func() { config.GlobalAlchemySpawns = previous })
	if _, err := config.LoadAlchemySpawns("assets/alchemy_spawns.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadAlchemyConfig("assets/alchemy_recipes.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadTechniques("assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	pc := newPartyCreateState(cfg)
	for slot, class := range []character.CharacterClass{character.ClassAlchemist, character.ClassWayfarer} {
		for i, h := range pc.pool {
			if h.char.Class == class {
				pc.pool[i], pc.slots[slot] = pc.slots[slot], h
				break
			}
		}
	}
	g.beginAdventure(pc)
	finishRareHarvest(t, g)
	assertRareHerbsRender(t, g, 9, false)
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	defer screen.Deallocate()
	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline); {
		runOnDrawFrame(func(_ *ebiten.Image) {
			if err := g.Update(); err != nil {
				t.Error(err)
			}
			g.Draw(screen)
		})
		time.Sleep(5 * time.Millisecond)
		if g.frameCount >= int64(cfg.GetTPS()) && !g.gameLoop.loadingBarrier() && g.gameLoop.loading.bannerAlpha(time.Now()) == 0 {
			runOnDrawFrame(func(_ *ebiten.Image) {
				f, err := os.Create(filepath.Join(os.TempDir(), "rare_party_startup.png"))
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				if err = png.Encode(f, snapshotUIImage(screen)); err != nil {
					t.Error(err)
				}
			})
			return
		}
	}
	t.Fatal("rare party never left startup loading")
}

func TestDebugSim_AlchemyAnimationFrames(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			oldFrames := config.GlobalIconFrames
			config.GlobalIconFrames = nil
			t.Cleanup(func() { config.GlobalIconFrames = oldFrames })
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			t.Chdir("../..")
			if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
				t.Fatal(err)
			}
			g.sprites = graphics.NewSpriteManager()
			c := character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
			g.party.Members[0] = c
			g.selectedChar = 0
			g.currentTab = TabSpellbook
			g.menuOpen = true
			for i, r := range config.GlobalAlchemy.Recipes {
				if r.Key == "venom_flask" {
					g.selectedRare = i
				}
			}
			recipe := &config.GlobalAlchemy.Recipes[g.selectedRare]
			stockAlchemyRecipe(g, recipe, 3)
			g.alchemyBatches = 3
			if !g.brewSelectedRecipe() {
				t.Fatal("brew failed")
			}
			for n := 0; n < 36; n++ {
				g.brewAnimation.Frames = n * g.brewAnimation.Duration / 36
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.ui.Draw(h.screen)
					f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("alchemy_brew_%d_%02d.png", size[0], n)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, snapshotUIImage(h.screen)); err != nil {
						t.Error(err)
					}
				})
			}
		})
	}
}

func TestDebugSim_AlchemyIngredientReceipt(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		for _, tc := range []struct {
			name, recipe    string
			choice, batches int
			keys            []string
			counts          []int
		}{
			{"mana_herbs", "mana_potion", 2, 1, []string{"mooncap", "bitterroot", "embercap"}, []int{1, 1, 1}},
			{"mana_fish", "mana_potion", 0, 1, []string{"mooncap", "carp_scale", "koi_scale", "rainbow_salmon_scale"}, []int{1, 1, 1, 2}},
			{"five_materials", "antivenom", 0, 4, []string{"bitterroot", "plague_rat_tail", "echo_membrane", "serpent_skin", "bound_cucumbers"}, []int{4, 1, 1, 1, 1}},
		} {
			t.Run(fmt.Sprintf("%d/%s", size[0], tc.name), func(t *testing.T) {
				oldFrames := config.GlobalIconFrames
				config.GlobalIconFrames = nil
				t.Cleanup(func() { config.GlobalIconFrames = oldFrames })
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				t.Chdir("../..")
				if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
					t.Fatal(err)
				}
				g.sprites = graphics.NewSpriteManager()
				g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
				g.selectedChar, g.currentTab, g.menuOpen = 0, TabSpellbook, true
				for i, r := range config.GlobalAlchemy.Recipes {
					if r.Key == tc.recipe {
						g.selectedRare = i
					}
				}
				g.alchemyBatches = tc.batches
				g.alchemy.Choices = map[string][]int{tc.recipe: {0, tc.choice}}
				g.party.Inventory = nil
				for i, key := range tc.keys {
					it, _ := items.TryCreateItemFromYAML(key)
					it.Quantity = tc.counts[i]
					g.party.AddItem(it)
				}
				before := append([]items.Item(nil), g.party.Inventory...)
				if !g.brewSelectedRecipe() {
					t.Fatal(g.rareBookMessage)
				}
				assertAlchemyReceipt(t, before, g.party.Inventory, g.brewAnimation.Ingredients)
				g.brewAnimation.Frames = g.brewAnimation.Duration / 4
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.ui.Draw(h.screen)
					f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("alchemy_receipt_%d_%s.png", size[0], tc.name)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, snapshotUIImage(h.screen)); err != nil {
						t.Error(err)
					}
				})
			})
		}
	}
}

func TestDebugSim_FlaskFlightFrames(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, cfg := bootOpenWorldGame(t, true)
	g.beginAdventure(newPartyCreateState(cfg))
	// Warm the actual world view before capturing flights against it.
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	defer screen.Deallocate()
	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline); {
		runOnDrawFrame(func(_ *ebiten.Image) {
			if err := g.Update(); err != nil {
				t.Error(err)
			}
			g.Draw(screen)
		})
		if g.frameCount >= int64(cfg.GetTPS()) && !g.gameLoop.loadingBarrier() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	c := character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
	g.party.Members[0] = c
	g.selectedChar = 0
	// Keep the generated view and camera fixed. Advance the real projectile wrapper
	// in fractions of its authored flight for a readable, reproducible visual reel.
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		g.handleResize(image.Pt(size[0], size[1]), image.Pt(size[0], size[1]))
		screen := ebiten.NewImage(size[0], size[1])
		defer screen.Deallocate()
		for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
			g.magicProjectiles = nil
			g.spellHitEffects = nil
			c.RTCooldown = 0
			c.ActionsRemaining = 2
			it, _ := items.TryCreateItemFromYAML(key)
			it.Quantity = 1
			g.party.AddItem(it)
			if !g.throwFlask(0, key, false) {
				t.Fatalf("%s refused", key)
			}
			p := &g.magicProjectiles[0]
			source := config.GlobalItems.Items[key].Flask.Sprite
			g.sprites.GetSprite(source)
			originX, originY := p.X, p.Y
			for n := 1; n <= 18; n++ {
				distance := p.FlaskFlightRange * float64(n) / 20
				wrapper := MagicProjectileWrapper{MagicProjectile: p}
				wrapper.SetPosition(originX+distance*math.Cos(g.camera.Angle), originY+distance*math.Sin(g.camera.Angle))
				runOnDrawFrame(func(_ *ebiten.Image) {
					g.Draw(screen)
					f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("alchemy_tiles_throw_%d_%s_%02d.png", size[0], key, n)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, snapshotUIImage(screen)); err != nil {
						t.Error(err)
					}
				})
			}
			wrapper := MagicProjectileWrapper{MagicProjectile: p, game: g, collisionSystem: g.collisionSystem, projectileID: p.ID}
			distance := p.FlaskFlightRange
			wrapper.SetPosition(originX+distance*math.Cos(g.camera.Angle), originY+distance*math.Sin(g.camera.Angle))
			wrapper.ApplyCollisionEffects()
			if len(g.spellHitEffects) == 0 {
				t.Fatal("flask ended without an impact")
			}
			for n := 0; n < 45; n++ {
				runOnDrawFrame(func(_ *ebiten.Image) {
					g.Draw(screen)
					f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("alchemy_tiles_impact_%d_%s_%02d.png", size[0], key, n)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, snapshotUIImage(screen)); err != nil {
						t.Error(err)
					}
				})
				g.UpdateHitEffects()
				g.UpdateHitEffects()
			}
		}
	}
}

func TestDebugSim_FlowingStaffHUD(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			oldFrames := config.GlobalIconFrames
			config.GlobalIconFrames = nil
			t.Cleanup(func() { config.GlobalIconFrames = oldFrames })
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			t.Chdir("../..")
			if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
				t.Fatal(err)
			}
			g.sprites = graphics.NewSpriteManager()
			g.showPartyStats, g.menuOpen = true, false
			for i := range g.party.Members {
				c := character.CreateCharacter("Kael", character.ClassWayfarer, g.config)
				c.Skills[character.SkillFlowingStaff] = &character.Skill{Mastery: character.MasteryGrandMaster}
				c.FreeStatPoints = 1
				c.RareClass.FlowCharges = i + 1
				g.party.Members[i] = c
				g.levelUpChoiceQueue = append(g.levelUpChoiceQueue, levelUpChoiceRequest{charIndex: i})
			}
			var charged []byte
			for _, empty := range []bool{false, true} {
				if empty {
					for _, c := range g.party.Members {
						c.RareClass.FlowCharges = 0
					}
				}
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.screen.Clear()
					h.ui.drawPartyUI(h.screen)
					pixels := make([]byte, size[0]*size[1]*4)
					h.screen.ReadPixels(pixels)
					if !empty {
						charged = pixels
					} else {
						pw, ph, left, top := partyPortraitLayout(g)
						for i := range g.party.Members {
							px, py, _, _ := partyCardPanelRect(left+i*pw, top, pw, ph)
							boxes := flowingStaffChargeRects(px+panelPortraitX, py+panelPortraitY, panelPortraitW, 4)
							for j, box := range boxes {
								changed := 0
								for y := box.y; y < box.bottom(); y++ {
									for x := box.x; x < box.right(); x++ {
										p := (y*size[0] + x) * 4
										if pixels[p] != charged[p] || pixels[p+1] != charged[p+1] || pixels[p+2] != charged[p+2] {
											changed++
										}
									}
								}
								if (j <= i && changed < 30) || (j > i && changed != 0) {
									t.Errorf("hero %d glyph %d changed %d pixels", i, j, changed)
								}
							}
						}
					}
					f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("flowing_staff_hud_%d_empty_%v.png", size[0], empty)))
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, snapshotUIImage(h.screen)); err != nil {
						t.Error(err)
					}
				})
			}
			g.party.Members[0].Skills[character.SkillFlowingStaff].Mastery = character.MasteryNovice
			if _, err := config.LoadLevelUpConfig("assets/level_up.yaml"); err != nil {
				t.Fatal(err)
			}
			g.levelUpChoiceQueue = nil
			g.queueLevelUpChoices(g.party.Members[0], 3, config.GetLevelUpChoices("wayfarer", 3))
			g.openLevelUpChoiceForChar(0)
			for i, opt := range g.levelUpChoiceQueue[0].options {
				if opt.skillType == character.SkillPathfinding {
					g.levelUpChoiceQueue[0].selection = i
				}
			}
			runOnDrawFrame(func(_ *ebiten.Image) {
				h.screen.Clear()
				h.ui.Draw(h.screen)
				f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("pathfinding_mastery_%d.png", size[0])))
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				if err := png.Encode(f, snapshotUIImage(h.screen)); err != nil {
					t.Error(err)
				}
			})
		})
	}
}

func TestDebugSim_RareFirewallExterior(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, cfg := bootOpenWorldGame(t, true)
	g.beginAdventure(newPartyCreateState(cfg))
	warm := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	defer warm.Deallocate()
	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline); {
		runOnDrawFrame(func(_ *ebiten.Image) {
			if err := g.Update(); err != nil {
				t.Error(err)
			}
			g.Draw(warm)
		})
		if g.frameCount >= int64(cfg.GetTPS()) && !g.gameLoop.loadingBarrier() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	ts := float64(cfg.GetTileSize())
	cx, cy := TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts)
	found := false
	for yy := cy - 12; yy <= cy+12 && !found; yy++ {
		for xx := cx - 12; xx <= cx+12; xx++ {
			clear := true
			for y := yy - 2; y <= yy+2; y++ {
				for x := xx; x <= xx+5; x++ {
					if g.world.IsTileBlockingTerrainAt(x, y) {
						clear = false
					}
				}
			}
			if clear {
				g.setPartyPosition((float64(xx)+.5)*ts, (float64(yy)+.5)*ts)
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("no clear field for firewall capture")
	}
	g.camera.Angle = 0
	def, err := spells.GetSpellDefinitionByID("firewall")
	if err != nil {
		t.Fatal(err)
	}
	if !g.combat.tryCastPersistentDamageZone("firewall", def, g.party.Members[0]).handled() {
		t.Fatal("wall cast failed")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		g.handleResize(image.Pt(size[0], size[1]), image.Pt(size[0], size[1]))
		screen := ebiten.NewImage(size[0], size[1])
		defer screen.Deallocate()
		runOnDrawFrame(func(_ *ebiten.Image) {
			g.frameCount += 17
			g.Draw(screen)
			f, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("rare_firewall_exterior_%d.png", size[0])))
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			if err := png.Encode(f, snapshotUIImage(screen)); err != nil {
				t.Error(err)
			}
		})
	}
}

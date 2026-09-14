//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestDebugSim_ColdCompassRetriesWholeLayer(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	for _, scenario := range []string{"first", "move", "resize", "world"} {
		t.Run(scenario, func(t *testing.T) {
			gl := loadingFixture(t)
			previous := world.GlobalTileManager
			t.Cleanup(func() { world.GlobalTileManager = previous })
			world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
			if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
				t.Fatal(err)
			}
			tile, ok := world.GlobalTileManager.GetTileTypeFromKey("firefly_swarm")
			if !ok {
				t.Fatal("missing tile")
			}
			gl.game.world.Tiles[1][1] = tile
			t.Chdir("../..")
			runOnDrawFrame(func(*ebiten.Image) {
				dst := ebiten.NewImage(120, 120)
				defer dst.Deallocate()
				radius := 40
				if scenario != "first" {
					gl.ui.drawCompassMinimap(dst, 60, 60, radius)
					gl.game.sprites.EvictResource("firefly_swarm", "")
					switch scenario {
					case "move":
						gl.game.camera.X += float64(gl.game.config.GetTileSize())
					case "resize":
						radius = 48
					case "world":
						gl.game.world = newTestWorldSized(gl.game.config, 4, 4)
						gl.game.world.Tiles[1][1] = tile
					}
				}
				caught := false
				func() {
					gl.loading.rendering = true
					gl.loading.inlineUIBytes = smallUIFrameBytes
					gl.game.sprites.SetDeferredResourceHandler(gl.deferGameplayResource)
					defer func() {
						gl.loading.rendering = false
						gl.game.sprites.SetDeferredResourceHandler(nil)
						if r := recover(); r != nil {
							if _, ok := r.(loadingRenderMiss); !ok {
								panic(r)
							}
							caught = true
						}
					}()
					gl.ui.drawCompassMinimap(dst, 60, 60, radius)
				}()
				if !caught {
					t.Error("cold draw did not defer")
					return
				}
				if gl.ui.compassCacheWorld != nil {
					t.Error("incomplete compass was marked valid")
				}
				deadline := time.Now().Add(5 * time.Second)
				for gl.loading.stream.Pending() && time.Now().Before(deadline) {
					gl.loading.stream.Advance(256 << 10)
					time.Sleep(time.Millisecond)
				}
				if gl.loading.stream.Pending() {
					t.Error("stream did not settle")
					return
				}
				gl.ui.drawCompassMinimap(dst, 60, 60, radius)
				actual := make([]byte, 4*radius*radius*4)
				gl.ui.compassTileLayer.ReadPixels(actual)
				gl.ui.invalidateCompassTileLayer()
				gl.ui.drawCompassMinimap(dst, 60, 60, radius)
				expected := make([]byte, len(actual))
				gl.ui.compassTileLayer.ReadPixels(expected)
				if !bytes.Equal(actual, expected) {
					t.Error("resumed compass reused a partial layer")
				}
			})
		})
	}
}

func assertPurchaseTextColor(t *testing.T, screen *ebiten.Image, rect image.Rectangle, canBuy bool) {
	t.Helper()
	pixels := make([]byte, rect.Dx()*rect.Dy()*4)
	screen.SubImage(rect).(*ebiten.Image).ReadPixels(pixels)
	green, red := 0, 0
	for i := 0; i < len(pixels); i += 4 {
		r, g, b := int(pixels[i]), int(pixels[i+1]), int(pixels[i+2])
		if g > r+30 && g > b+30 {
			green++
		}
		if r > g+80 && r > b+80 {
			red++
		}
	}
	if canBuy && (green == 0 || red != 0) || !canBuy && (red == 0 || green != 0) {
		t.Errorf("price color disagrees with purchase availability %v: green=%d red=%d", canBuy, green, red)
	}
}

func TestDebugSim_MerchantPriceMatchesPurchase(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	for _, currency := range []string{"gold", "discount", "arena", "item", "override", "item_gold", "free_item"} {
		for _, state := range []string{"exact", "short", "sold_out", "unlimited"} {
			t.Run(currency+"/"+state, func(t *testing.T) {
				entry := potionStock(2, 1)
				shopCurrency := ""
				switch currency {
				case "arena":
					shopCurrency = character.CurrencyArenaPoints
				case "item", "item_gold", "free_item":
					shopCurrency = "item:red_dragon_scale"
				case "override":
					shopCurrency = character.CurrencyArenaPoints
					entry.CurrencyItem = "red_dragon_scale"
				}
				if currency == "item_gold" {
					entry.GoldCost = 10
				}
				if currency == "free_item" {
					entry.Cost = 0
				}
				g, ui := merchantBuyGame(t, shopCurrency, entry)
				for _, m := range g.party.Members {
					delete(m.Skills, character.SkillMerchant)
				}
				if currency == "discount" {
					g.party.Members[0].Skills[character.SkillMerchant] = &character.Skill{Mastery: character.MasteryGrandMaster}
					entry.Cost = 100
				}
				g.party.Gold = g.merchantEntryPrice(entry)
				g.party.ArenaPoints = entry.Cost
				if entry.GoldCost > 0 {
					g.party.Gold = entry.GoldCost
				}
				g.party.Inventory = []items.Item{{Name: "Red Dragon Scale", Type: items.ItemTrinket, Quantity: max(1, entry.Cost)}}
				if state == "short" {
					switch currency {
					case "gold", "discount", "item_gold":
						g.party.Gold--
					case "arena":
						g.party.ArenaPoints--
					default:
						g.party.Inventory = nil
					}
				}
				if state == "sold_out" {
					entry.Quantity = 0
				}
				if state == "unlimited" {
					entry.Quantity = -1
				}
				want := state != "sold_out" && (state != "short" || currency == "free_item")
				runOnDrawFrame(func(*ebiten.Image) {
					dst := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
					defer dst.Deallocate()
					dlg := npcDialogLayout(g)
					left, _, top, _ := merchantGridLayout(dlg.x, dlg.y)
					x, y, w, h := merchantCellRect(left, top, 0)
					px, py, pw, ph := merchantPriceRect(x, y, w, h)
					ui.drawMerchantDialog(dst, dlg.x, dlg.y, dlg.w, dlg.h)
					assertPurchaseTextColor(t, dst, image.Rect(px, py, px+pw, py+ph), want)
					if bought := g.buyMerchantUnits(entry, 1); bought != want {
						t.Errorf("purchase=%v want=%v", bought, want)
					}
					dst.Clear()
					ui.drawMerchantDialog(dst, dlg.x, dlg.y, dlg.w, dlg.h)
					assertPurchaseTextColor(t, dst, image.Rect(px, py, px+pw, py+ph), g.merchantMaxUnits(entry) > 0)
				})
			})
		}
	}
	// A mixed payment needs its item currency even with enough gold.
	t.Run("surcharge_missing_items", func(t *testing.T) {
		entry := potionStock(2, 1)
		entry.CurrencyItem = "red_dragon_scale"
		entry.GoldCost = 10
		g, ui := merchantBuyGame(t, "", entry)
		g.party.Gold = 10
		runOnDrawFrame(func(*ebiten.Image) {
			dst := ebiten.NewImage(1280, 720)
			defer dst.Deallocate()
			dlg := npcDialogLayout(g)
			left, _, top, _ := merchantGridLayout(dlg.x, dlg.y)
			x, y, w, h := merchantCellRect(left, top, 0)
			px, py, pw, ph := merchantPriceRect(x, y, w, h)
			ui.drawMerchantDialog(dst, dlg.x, dlg.y, dlg.w, dlg.h)
			assertPurchaseTextColor(t, dst, image.Rect(px, py, px+pw, py+ph), false)
			if g.buyMerchantUnits(entry, 1) {
				t.Error("bought without required scales")
			}
		})
	})
}

func TestDebugSim_SpellPriceMatchesPurchase(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	for _, state := range []string{"exact", "short", "known", "closed_school"} {
		t.Run(state, func(t *testing.T) {
			gl := loadingFixture(t)
			g := gl.game
			g.dialogActive = true
			g.dialogNPC = &character.NPC{Name: "Tutor", SpellData: map[string]*character.NPCSpell{"fireball": {Name: "Fireball", Cost: 10}}}
			g.party.Gold = 10
			ch := g.party.Members[0]
			ch.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolFire: {Mastery: character.MasteryNovice}}
			switch state {
			case "short":
				g.party.Gold = 9
			case "known":
				ch.MagicSchools[character.MagicSchoolFire].KnownSpells = []spells.SpellID{"fireball"}
			case "closed_school":
				ch.MagicSchools = nil
			}
			runOnDrawFrame(func(*ebiten.Image) {
				dst := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
				defer dst.Deallocate()
				dlg := npcDialogLayout(g)
				gl.ui.drawSpellTraderDialog(dst, dlg.x, dlg.y, dlg.w, dlg.h)
				x, y, _, _ := spellTraderIconRect(dlg.x, dlg.y, 0)
				px, py, pw, ph := spellTraderPriceRect(x, y)
				assertPurchaseTextColor(t, dst, image.Rect(px, py, px+pw, py+ph), state == "exact")
				gold := g.party.Gold
				g.selectedSpellKey = "fireball"
				gl.inputHandler.purchaseSelectedSpell()
				if state == "exact" {
					if g.party.Gold != gold-10 || !ch.KnowsSpell("fireball") {
						t.Error("green spell price did not permit purchase")
					}
				} else if g.party.Gold != gold {
					t.Error("red spell price allowed a charge")
				}
			})
		})
	}
}

func TestDebugSim_ClockmakerTabsLoadSmallIconsWithoutPause(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	gl := loadingFixture(t)
	g := gl.game
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	npc, err := character.CreateNPCFromConfig("clockmaker", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	g.dialogActive, g.dialogNPC = true, npc
	g.party.Inventory = []items.Item{{Name: "Clock Hand", Type: items.ItemTrinket, Quantity: 7}}
	t.Chdir("../..")
	ApplySpriteColorKey(g.sprites, g.config)
	// The dialog frame is already visible when the user changes a shop tab.
	runOnDrawFrame(func(*ebiten.Image) {
		frame := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
		defer frame.Deallocate()
		gl.ui.drawNPCDialog(frame)
	})
	for _, size := range [][2]int{{1280, 720}, {1600, 900}} {
		for tab, label := range g.merchantShopTabs() {
			t.Run(fmt.Sprintf("%s/%dx%d", label, size[0], size[1]), func(t *testing.T) {
				runOnDrawFrame(func(*ebiten.Image) {
					w, h := gl.Layout(size[0], size[1])
					dst := ebiten.NewImage(w, h)
					defer dst.Deallocate()
					g.switchDialogTab(tab)
					for _, entry := range g.merchantVisibleStock() {
						g.sprites.EvictResource(itemTooltipIconName(entry.Item), "")
					}
					// Exercise a full cold shop page, not one isolated SpriteManager call.
					gl.loading.rendering = true
					gl.loading.inlineUIBytes = 0
					g.sprites.SetDeferredResourceHandler(gl.deferGameplayResource)
					defer func() {
						gl.loading.rendering = false
						g.sprites.SetDeferredResourceHandler(nil)
						if r := recover(); r != nil {
							if _, ok := r.(loadingRenderMiss); !ok {
								panic(r)
							}
							t.Error("small shop page entered loading")
						}
					}()
					gl.ui.beginDisplayedInput()
					start := time.Now()
					gl.ui.drawNPCDialog(dst)
					t.Logf("cold shop tab Draw: %s", time.Since(start))
					gl.ui.endDisplayedInput()
					if gl.loading.awaitingFrame || gl.loading.stream.Pending() || !gl.loading.started.IsZero() {
						t.Error("small shop page paused or announced loading")
					}
					if !gl.ui.displayedInput.ready {
						t.Error("completed page is not interactive")
					}
					if dir := os.Getenv("RAM_LOADING_QA_DIR"); dir != "" && tab == 0 {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Error(err)
							return
						}
						f, err := os.Create(filepath.Join(dir, fmt.Sprintf("merchant_prices_%dx%d.png", w, h)))
						if err != nil {
							t.Error(err)
							return
						}
						defer f.Close()
						if err := png.Encode(f, dst); err != nil {
							t.Error(err)
						}
					}
				})
			})
		}
	}
}

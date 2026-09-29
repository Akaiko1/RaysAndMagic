package game

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/storage"
)

func TestPotionPolicyAutomaticFamilies(t *testing.T) {
	// Run the real selector and shared manual consumption for every shipped family
	// and brewed copy, both source locations and both simulation clocks.
	for _, tb := range []bool{false, true} {
		for _, quick := range []bool{false, true} {
			for _, allowed := range []bool{false, true} {
				t.Run(fmt.Sprintf("tb=%v/quick=%v/allowed=%v", tb, quick, allowed), func(t *testing.T) {
					g, _, ch, _ := sniperFixture(t, tb)
					prefs := g.config.EnsurePotionPreferences()
					for key, def := range config.GlobalItems.Items {
						for _, mana := range []bool{false, true} {
							if !config.IsAutomaticPotion(def, mana) {
								continue
							}
							t.Run(key, func(t *testing.T) {
								ch.MaxHitPoints, ch.MaxSpellPoints = 1000, 1000
								ch.HitPoints, ch.SpellPoints = 1, 1
								ch.AutoDrinkCooldown = 0
								clear(ch.QuickSlots[:])
								g.party.Inventory = nil
								prefs.Health.ThresholdPct, prefs.Mana.ThresholdPct = 0, 0
								resource := prefs.Resource(mana)
								resource.ThresholdPct = 60
								resource.Allowed[config.PotionFamily(def)] = allowed
								p := items.CreateItemFromYAML(key)
								p.Quantity = 2
								if quick {
									ch.QuickSlots[0] = &p
								} else {
									g.party.Inventory = []items.Item{p}
								}
								g.updateAutomaticConsumables()
								left := 0
								if quick {
									left = ch.QuickSlots[0].Count()
								} else {
									left = g.party.Inventory[0].Count()
								}
								want := 2
								if allowed {
									want = 1
								}
								if left != want {
									t.Fatalf("%s consumption=%d remaining; want %d", key, left, want)
								}
								if !allowed { // Preferences never restrict deliberate manual use.
									used := false
									if quick {
										used = g.useQuickConsumable(0, 0)
									} else {
										used = g.UseConsumableFromInventory(0, 0)
									}
									if !used {
										t.Fatal("excluded type was also blocked for manual use")
									}
								}
							})
						}
					}
				})
			}
		}
	}
}

func TestPotionPolicyThresholdsAndFallback(t *testing.T) {
	for _, mana := range []bool{false, true} {
		for _, threshold := range []int{0, 60, 100} {
			for _, percent := range []int{1, 59, 60, 61, 99, 100} {
				t.Run(fmt.Sprintf("mana=%v/threshold=%d/current=%d", mana, threshold, percent), func(t *testing.T) {
					g, _, ch, _ := sniperFixture(t, false)
					prefs := g.config.EnsurePotionPreferences()
					prefs.Health.ThresholdPct, prefs.Mana.ThresholdPct = 0, 0
					prefs.Resource(mana).ThresholdPct = threshold
					ch.MaxHitPoints, ch.MaxSpellPoints = 100, 100
					ch.HitPoints, ch.SpellPoints = 100, 100
					key := "health_potion"
					if mana {
						key = "mana_potion"
						ch.SpellPoints = percent
					} else {
						ch.HitPoints = percent
					}
					p := items.CreateItemFromYAML(key)
					p.Quantity = 2
					g.party.Inventory = []items.Item{p}
					g.updateAutomaticConsumables()
					want := 2
					if percent < threshold {
						want = 1
					}
					if g.party.Inventory[0].Count() != want {
						t.Fatal("threshold/off rule violated")
					}
				})
			}
		}
	}
	for _, healthAllowed := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback/health=%v", healthAllowed), func(t *testing.T) {
			g, _, ch, _ := sniperFixture(t, false)
			ch.MaxHitPoints, ch.MaxSpellPoints = 1000, 1000
			ch.HitPoints, ch.SpellPoints = 1, 1
			prefs := g.config.EnsurePotionPreferences()
			prefs.Health.Allowed["sake_flask"] = false
			prefs.Health.Allowed["health_potion"] = healthAllowed
			sake := items.CreateItemFromYAML("sake_flask")
			sake.Quantity = 2
			ch.QuickSlots[0] = &sake
			hp, sp := items.CreateItemFromYAML("health_potion"), items.CreateItemFromYAML("mana_potion")
			hp.Quantity, sp.Quantity = 2, 2
			g.party.Inventory = []items.Item{hp, sp}
			g.updateAutomaticConsumables()
			if ch.QuickSlots[0].Count() != 2 {
				t.Fatal("reserved sake consumed")
			}
			if healthAllowed && (ch.HitPoints <= 1 || ch.SpellPoints != 1) {
				t.Fatal("allowed bag health did not take priority")
			}
			if !healthAllowed && (ch.SpellPoints <= 1 || ch.HitPoints != 1) {
				t.Fatal("unavailable health blocked mana")
			}
		})
	}
}

func TestPotionPolicyAntivenomManual(t *testing.T) {
	for _, poison := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			for _, quick := range []bool{false, true} {
				t.Run(fmt.Sprintf("poison=%v/full=%v/quick=%v", poison, full, quick), func(t *testing.T) {
					g, _, ch, _ := sniperFixture(t, false)
					ch.MaxHitPoints = 1000
					ch.HitPoints = 1
					if full {
						ch.HitPoints = 1000
					}
					if poison {
						ch.Conditions = append(ch.Conditions, character.ConditionPoisoned)
					}
					g.config.EnsurePotionPreferences().Health.Allowed["antivenom"] = false
					p := items.CreateItemFromYAML("antivenom")
					p.Quantity = 2
					used := false
					if quick {
						ch.QuickSlots[0] = &p
						used = g.useQuickConsumable(0, 0)
					} else {
						g.party.Inventory = []items.Item{p}
						used = g.UseConsumableFromInventory(0, 0)
					}
					if used != (!full || poison) || ch.HasCondition(character.ConditionPoisoned) {
						t.Fatal("wrong cure/heal eligibility")
					}
					if !full && ch.HitPoints <= 1 {
						t.Fatal("antivenom did not heal")
					}
				})
			}
		}
	}
}

func TestPotionPolicyTooltip(t *testing.T) {
	g, _, ch, _ := sniperFixture(t, false)
	prefs := g.config.EnsurePotionPreferences()
	for _, key := range []string{"antivenom", "brewed_antivenom", "health_potion", "brewed_health_potion", "mana_potion", "brewed_mana_potion"} {
		def, ok := config.GetItemDefinition(key)
		if !ok {
			t.Fatalf("missing catalog fixture %s", key)
		}
		mana := def.ManaBase > 0
		r := prefs.Resource(mana)
		r.ThresholdPct = 61
		for _, allowed := range []bool{false, true} {
			r.Allowed[config.PotionFamily(def)] = allowed
			tip := GetItemTooltip(items.CreateItemFromYAML(key), ch, g.combat, true)
			if strings.Contains(tip, "61%") != allowed || strings.Contains(tip, "While poisoned:") {
				t.Fatalf("tooltip contradicts policy: %s", tip)
			}
		}
	}
}

func TestPotionSettingsDisplayedControls(t *testing.T) {
	for _, entry := range []bool{false, true} {
		for row := 0; row < 2; row++ {
			t.Run(fmt.Sprintf("entry=%v/row=%d", entry, row), func(t *testing.T) {
				h, _ := audioGestureHarness(t, entry, 0)
				g := h.g
				fp := installFakePointer(t)
				layout := makeAudioSettingsPanelLayout(1024, 768, entry)
				presentInputScreen(h)
				tab := settingsTabRect(layout.px, layout.py, layout.panelW, 1)
				fp.moveTo(tab.x+8, tab.y+8)
				fp.press()
				updateInputScreen(h)
				if g.settingsTab != 1 {
					t.Fatal("displayed tab did not open")
				}
				fp.release()
				updateInputScreen(h)
				presentInputScreen(h)
				r := makePotionSettingsLayout(layout.px, layout.py, layout.panelW, row).slider
				fp.moveTo(r.x1+(r.x2-r.x1)/4, (r.y1+r.y2)/2)
				fp.press()
				updateInputScreen(h)
				if g.audioSliderDrag != row || g.config.AutoPotionThreshold(row == 1) != 25 {
					t.Fatal("wrong threshold drag target")
				}
				fp.hold()
				fp.moveTo(r.x1+3*(r.x2-r.x1)/4, (r.y1+r.y2)/2)
				for range 3 {
					updateInputScreen(h)
				}
				fp.release()
				updateInputScreen(h)
				if g.config.AutoPotionThreshold(row == 1) != 75 || g.potionSettingsDirty || g.audioSliderDrag != -1 {
					t.Fatal("threshold gesture did not persist")
				}
				presentInputScreen(h)
				choice := config.AutomaticPotionChoices(row == 1)[0]
				card := makePotionSettingsLayout(layout.px, layout.py, layout.panelW, row).card(0, 0)
				fp.moveTo(card.x+10, card.y+10)
				fp.press()
				updateInputScreen(h)
				if g.config.PlayerPotions.Resource(row == 1).Allowed[choice.Key] {
					t.Fatal("displayed checkbox did not exclude type")
				}
				fp.release()
				updateInputScreen(h)
				expected := g.config.PlayerPotions
				g.config.PlayerPotions = nil
				g.loadPotionPreferences()
				if !reflect.DeepEqual(expected, g.config.PlayerPotions) {
					t.Fatal("displayed edits did not survive restart")
				}
				presentInputScreen(h)
				g.audioSettingsSelection = row
				presentInputScreen(h)
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyLeft })
				updateInputScreen(h)
				if g.config.AutoPotionThreshold(row == 1) != 70 {
					t.Fatal("keyboard did not adjust selected threshold")
				}
			})
		}
	}
}

func TestPotionSettingsOwnerLossAndSaveFailure(t *testing.T) {
	for _, entry := range []bool{false, true} {
		for _, loss := range []string{"tab", "resize", "close", "modal"} {
			if entry && loss == "modal" {
				continue
			}
			t.Run(fmt.Sprintf("entry=%v/%s", entry, loss), func(t *testing.T) {
				h, _ := audioGestureHarness(t, entry, 0)
				g := h.g
				g.switchSettingsTab(1)
				fp := installFakePointer(t)
				layout := makeAudioSettingsPanelLayout(1024, 768, entry)
				r := makePotionSettingsLayout(layout.px, layout.py, layout.panelW, 0).slider
				presentInputScreen(h)
				fp.moveTo(r.x1+(r.x2-r.x1)/2, r.y1+5)
				fp.press()
				updateInputScreen(h)
				fp.hold()
				fp.moveTo(r.x2, r.y1+5)
				switch loss {
				case "tab":
					g.switchSettingsTab(0)
				case "resize":
					g.config.Display.ScreenWidth += 80
				case "close":
					g.closeAudioSettings()
				case "modal":
					g.mapOverlayOpen = true
				}
				presentInputScreen(h)
				updateInputScreen(h)
				if g.audioSliderDrag != -1 || g.potionSettingsDirty || g.config.AutoPotionThreshold(false) != 50 {
					t.Fatal("lost owner still changed threshold or failed to save")
				}
				g.mapOverlayOpen = false
				g.settingsTab = 1
				if entry {
					g.entryMenuMode = EntryMenuSettings
				} else {
					g.mainMenuMode = MenuSettings
				}
				presentInputScreen(h)
				updateInputScreen(h)
				if g.audioSliderDrag != -1 || g.config.AutoPotionThreshold(false) != 50 {
					t.Fatal("held drag revived")
				}
			})
		}
	}
	t.Run("failed save remains retryable", func(t *testing.T) {
		h, _ := audioGestureHarness(t, false, 0)
		g := h.g
		path := potionPreferencesPath()
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		g.setPotionThreshold(false, 77)
		g.savePotionPreferences()
		if !g.potionSettingsDirty || g.settingsSaveError == "" {
			t.Fatal("save failure silently accepted")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		g.savePotionPreferences()
		if g.potionSettingsDirty || g.settingsSaveError != "" {
			t.Fatal("save did not recover")
		}
	})
}

func TestPotionSettingsRealStartupAndCampaignLoad(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	path := potionPreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"health":{"threshold_percent":67,"allowed_types":{"antivenom":true}},"mana":{"threshold_percent":21}}`), 0600); err != nil {
		t.Fatal(err)
	}
	g, _, cfg := bootOpenWorldGame(t, false)
	if cfg.AutoPotionThreshold(false) != 67 || cfg.AutoPotionThreshold(true) != 21 {
		t.Fatal("startup did not load preferences")
	}
	expected := cfg.PlayerPotions
	campaign := filepath.Join(t.TempDir(), "campaign.json")
	if err := g.SaveGameToFile(campaign); err != nil {
		t.Fatal(err)
	}
	if err := g.LoadGameFromFile(campaign); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, cfg.PlayerPotions) {
		t.Fatal("campaign load overwrote player settings")
	}
	g.beginAdventure(newPartyCreateState(cfg))
	if !reflect.DeepEqual(expected, g.config.PlayerPotions) {
		t.Fatal("new adventure overwrote player settings")
	}
}

func TestPotionSettingsLayoutAndTitleTooltip(t *testing.T) {
	for _, size := range [][2]int{{800, 680}, {1024, 768}, {1920, 1080}} {
		l := makeAudioSettingsPanelLayout(size[0], size[1], true)
		region, boxes := audioSettingsLayoutBoxes(size[0], size[1], true)
		filtered := []uiBox{}
		for _, box := range boxes {
			if !strings.HasPrefix(box.Name, "slider-") {
				filtered = append(filtered, box)
			}
		}
		for row := 0; row < 2; row++ {
			p := makePotionSettingsLayout(l.px, l.py, l.panelW, row)
			filtered = append(filtered, namedLayoutBox(fmt.Sprintf("resource-%d", row), p.panel))
			inner := []uiBox{namedLayoutBox("types", p.types), {"slider", p.slider.x1 - 8, p.slider.y1, p.slider.x2 - p.slider.x1 + 16, p.slider.y2 - p.slider.y1}}
			assertNoCollisions(t, "resource", namedLayoutBox("panel", p.panel), inner)
		}
		assertNoCollisions(t, "potion settings", region, filtered)
	}
	h, _ := audioGestureHarness(t, true, 0)
	g := h.g
	g.switchSettingsTab(1)
	fp := installFakePointer(t)
	l := makeAudioSettingsPanelLayout(1024, 768, true)
	r := makePotionSettingsLayout(l.px, l.py, l.panelW, 0).card(0, 0)
	fp.moveTo(r.x+10, r.y+10)
	presentInputScreen(h)
	if len(h.ui.tooltipLines) == 0 {
		t.Fatal("title potion card has no description")
	}
	fp.moveTo(0, 0)
	presentInputScreen(h)
	if h.ui.tooltipLines != nil {
		t.Fatal("title tooltip stuck after leaving card")
	}
}

func TestPotionPolicyAntivenomNeverRevives(t *testing.T) {
	for _, condition := range []character.Condition{character.ConditionUnconscious, character.ConditionDead, character.ConditionEradicated} {
		for _, ally := range []bool{false, true} {
			t.Run(fmt.Sprintf("condition=%v/ally=%v", condition, ally), func(t *testing.T) {
				g, _, ch, _ := sniperFixture(t, false)
				ch.HitPoints = 0
				ch.Conditions = []character.Condition{condition}
				var target *character.MMCharacter
				if ally {
					target = character.CreateCharacter("Patient", character.ClassKnight, g.config)
					target.MaxHitPoints, target.HitPoints = 1000, 1
					g.party.Members = append(g.party.Members, target)
				}
				p := items.CreateItemFromYAML("antivenom")
				p.Quantity = 2
				g.party.Inventory = []items.Item{p}
				used := g.UseConsumableFromInventory(0, 0)
				if used != ally || ch.HitPoints != 0 || !ch.HasCondition(condition) {
					t.Fatal("ordinary antidote healing revived an incapacitated owner")
				}
				if ally && target.HitPoints <= 1 {
					t.Fatal("ordinary healing did not redirect to the conscious ally")
				}
			})
		}
	}
}

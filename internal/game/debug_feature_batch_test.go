//go:build debug

package game

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/storage"

	"github.com/hajimehoshi/ebiten/v2"
)

// Screenshots of the latest feature batch, one per feature, into
// ~/Downloads/RaysAndMagic_features. Run with RAM_DEBUG_SIM=1 -tags debug.

func featureShotDir(t *testing.T) string {
	t.Helper()
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_features")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	return out
}

// captureLaidOutFrame draws one exploration frame at a posed window size.
func captureLaidOutFrame(g *MMGame, winW, winH int) *image.RGBA {
	w, h := g.gameLoop.Layout(winW, winH)
	frame := ebiten.NewImage(w, h)
	defer frame.Deallocate()
	shot := image.NewRGBA(image.Rect(0, 0, w, h))
	runOnDrawFrame(func(_ *ebiten.Image) {
		drawLaidOutFrame(g, frame)
		frame.ReadPixels(shot.Pix)
	})
	return shot
}

// Allies summoned by a hero wear that hero's face; the card ally (the party's)
// has no summoner and a bound former enemy keeps the bind rings.
func TestDebugSim_SummonerBadges(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	g.world.Monsters = nil

	druidSlot := 3
	for _, pool := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve, g.party.Captive} {
		for _, c := range pool {
			if c != nil && c.Class == character.ClassDruid {
				g.party.Members[druidSlot] = c
			}
		}
	}
	druid := g.party.Members[druidSlot]
	druid.Skills[character.SkillAnimalBonding] = &character.Skill{Mastery: character.MasteryMaster}
	def, err := spells.GetSpellDefinitionByID("summon_ice_elemental")
	if err != nil {
		t.Fatal(err)
	}
	if !g.combat.tryCastSummon(def, g.party.Members[1]).handled() {
		t.Fatal("summon spell was not handled")
	}
	if !g.combat.summonAnimalBondingBear(druid) {
		t.Fatal("no bear placed")
	}
	g.combat.spawnPartyAlly("masked_huntress", cardSummonOwnerPrefix+"7")
	bound := monsterPkg.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "goblin", g.config)
	bound.Bound = true
	g.world.Monsters = append(g.world.Monsters, bound)

	ts := float64(g.config.GetTileSize())
	fx, fy := math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
	for i, m := range g.world.Monsters {
		side := (float64(i) - 1.5) * 1.1 * ts
		m.X = g.camera.X + fx*3*ts - fy*side
		m.Y = g.camera.Y + fy*3*ts + fx*side
		m.Direction = g.camera.Angle + math.Pi
	}
	for _, res := range [][2]int{{1280, 720}, {1920, 1080}} {
		shot := captureLaidOutFrame(g, res[0], res[1])
		saveStatusEffectImage(t, filepath.Join(out, fmt.Sprintf("summoner_badges_%dx%d.png", res[0], res[1])), shot)
	}
	t.Logf("summoner badges -> %s", out)
}

// Party creation: the class pitch with highlighted keywords, and a skill card
// opened by resting the pointer on the skill.
func TestDebugSim_PartyCreateClassPitch(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	prevNow, prevCursor := dwellNow, rawCursorPosition
	defer func() { dwellNow, rawCursorPosition = prevNow, prevCursor }()
	clock := time.Unix(1000, 0)
	dwellNow = func() time.Time { return clock }
	var at image.Point
	rawCursorPosition = func() (int, int) { return at.X, at.Y }

	g.appScreen = AppScreenPartyCreate
	for _, res := range [][2]int{{1280, 720}, {1920, 1080}} {
		for _, pose := range []struct {
			class character.CharacterClass
			token string
		}{
			{character.ClassCleric, "party_create_skill:Natural Healer"},
			{character.ClassMonk, "party_create_skill:Spiritual Training"},
			{character.ClassSniper, "party_create_skill:Overwatch"},
		} {
			g.partyCreate = newPartyCreateState(g.config)
			pc := g.partyCreate
			for _, hero := range append(pc.slots[:], pc.pool...) {
				if hero != nil && hero.char.Class == pose.class {
					pc.detail = hero
				}
			}
			w, h := g.gameLoop.Layout(res[0], res[1])
			frame := ebiten.NewImage(w, h)
			draw := func() {
				runOnDrawFrame(func(_ *ebiten.Image) { g.gameLoop.Draw(frame) })
			}
			// Rest the pointer on the skill, scrolling the panel as the
			// wheel would.
			lay := partyCreateLayout(pc, g.config.GetScreenWidth(), g.config.GetScreenHeight())
			var xs []int
			for x := lay.detail.x + 18; x < lay.detail.x+lay.detail.w-16; x += 10 {
				xs = append(xs, x)
			}
			var found bool
			runOnDrawFrame(func(_ *ebiten.Image) {
				_, _, found = scanHeroDetailTokens(g.gameLoop.ui, pc, lay, xs, func(p image.Point) { at = p },
					func(key string) bool { return key == pose.token })
			})
			if !found {
				t.Fatalf("%s: %s not found", pose.class, pose.token)
			}
			draw()
			clock = clock.Add(tooltipDwellDelay)
			draw()
			shot := image.NewRGBA(image.Rect(0, 0, w, h))
			runOnDrawFrame(func(_ *ebiten.Image) {
				g.gameLoop.Draw(frame)
				frame.ReadPixels(shot.Pix)
			})
			frame.Deallocate()
			name := fmt.Sprintf("party_create_%s_%dx%d.png", pose.class.Key(), res[0], res[1])
			saveStatusEffectImage(t, filepath.Join(out, name), shot)
		}
	}
}

// Party creation at 720p as the panel opens: the pitch comes first.
func TestDebugSim_PartyCreatePitchTop(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	prevCursor := rawCursorPosition
	defer func() { rawCursorPosition = prevCursor }()
	rawCursorPosition = func() (int, int) { return 0, 0 }
	g.appScreen = AppScreenPartyCreate
	for _, pose := range []struct {
		name string
		res  [2]int
	}{
		{"Grikka", [2]int{1280, 720}}, {"Auralis", [2]int{1280, 720}}, {"Auberon", [2]int{1280, 720}},
		{"Nyra", [2]int{1920, 1080}}, {"Brinna", [2]int{1920, 1080}}, {"Mara", [2]int{1920, 1080}}, {"Gareth", [2]int{1920, 1080}},
		{"Silvelyn", [2]int{1920, 1080}}, {"Mirelle", [2]int{1920, 1080}}, {"Kael", [2]int{1920, 1080}}, {"Gwen", [2]int{1920, 1080}},
	} {
		g.partyCreate = newPartyCreateState(g.config)
		for _, hero := range append(g.partyCreate.slots[:], g.partyCreate.pool...) {
			if hero != nil && hero.char.Name == pose.name {
				g.partyCreate.detail = hero
			}
		}
		if g.partyCreate.detail.char.Name != pose.name {
			t.Fatalf("no hero %s", pose.name)
		}
		w, h := g.gameLoop.Layout(pose.res[0], pose.res[1])
		frame := ebiten.NewImage(w, h)
		shot := image.NewRGBA(image.Rect(0, 0, w, h))
		runOnDrawFrame(func(_ *ebiten.Image) { g.gameLoop.Draw(frame) })
		runOnDrawFrame(func(_ *ebiten.Image) {
			g.gameLoop.Draw(frame)
			frame.ReadPixels(shot.Pix)
		})
		frame.Deallocate()
		saveStatusEffectImage(t, filepath.Join(out, fmt.Sprintf("party_create_pitch_%s_%dx%d.png", strings.ToLower(pose.name), pose.res[0], pose.res[1])), shot)
	}
}

// The Control Tips page: two columns on a wide screen, scrolled on a small one.
func TestDebugSim_ControlTips(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	g.openMainMenu()
	g.mainMenuMode = MenuControlTips
	for _, pose := range []struct {
		w, h   int
		bottom bool
	}{{1920, 1080, false}, {1280, 720, false}, {1280, 720, true}, {800, 680, false}} {
		g.controlTipsScroll = 0
		if pose.bottom {
			g.controlTipsScroll = 1 << 20 // the draw clamps it to the last row
		}
		shot := captureLaidOutFrame(g, pose.w, pose.h)
		name := fmt.Sprintf("control_tips_%dx%d.png", pose.w, pose.h)
		if pose.bottom {
			name = fmt.Sprintf("control_tips_%dx%d_scrolled.png", pose.w, pose.h)
		}
		saveStatusEffectImage(t, filepath.Join(out, name), shot)
	}
}

// A quick-slot cell opens its card once the pointer rests on it.
func TestDebugSim_QuickSlotTooltip(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	prevNow, prevCursor := dwellNow, rawCursorPosition
	defer func() { dwellNow, rawCursorPosition = prevNow, prevCursor }()
	clock := time.Unix(1000, 0)
	dwellNow = func() time.Time { return clock }
	potion := items.CreateItemFromYAML("health_potion")
	ch := g.party.Members[g.selectedChar]
	ch.QuickSlots[0] = &potion
	if sp, err := spells.CreateSpellItem("firebolt"); err == nil {
		ch.QuickSlots[1] = &sp
	}
	for _, res := range [][2]int{{1280, 720}, {1920, 1080}} {
		for slot, name := range []string{"potion", "spell"} {
			g.gameLoop.Layout(res[0], res[1])
			bar, visible := inGameQuickSlotBarLayout(g)
			if !visible {
				t.Fatal("the quick bar is hidden")
			}
			_, cells := quickSlotRects(bar.x, bar.y, bar.w)
			at := centerOf(cells[slot])
			rawCursorPosition = func() (int, int) {
				return int(float64(at.X) * uiScreenScale), int(float64(at.Y) * uiScreenScale)
			}
			captureLaidOutFrame(g, res[0], res[1])
			clock = clock.Add(quickSlotCardDelay)
			shot := captureLaidOutFrame(g, res[0], res[1])
			saveStatusEffectImage(t, filepath.Join(out, fmt.Sprintf("quick_slot_tooltip_%s_%dx%d.png", name, res[0], res[1])), shot)
		}
	}
}

// F5 quicksaves (the log says so) and the Load menu lists the Quicksave row
// beside the Autosave.
func TestDebugSim_Quicksave(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g.menuOpen = false
	ih := g.gameLoop.inputHandler
	ih.keys = keysPressed(ebiten.KeyF5)
	ih.heldKeys = heldOnly(ebiten.KeyF5)
	ih.HandleInput()
	ih.keys, ih.heldKeys = keytracker.Consumer{}, nil
	if !GetSaveRowSummary(quicksaveRow).Exists {
		t.Fatal("F5 wrote no quicksave")
	}
	g.Autosave()
	saveStatusEffectImage(t, filepath.Join(out, "quicksave_message_1280x720.png"), captureLaidOutFrame(g, 1280, 720))
	g.openMainMenu()
	g.mainMenuMode = MenuLoadSelect
	saveStatusEffectImage(t, filepath.Join(out, "quicksave_load_menu_1280x720.png"), captureLaidOutFrame(g, 1280, 720))
}

// Walking up to a kill's bag picks it up without a key.
func TestDebugSim_LootAutoPickup(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	g.world.Monsters = nil
	ts := float64(g.config.GetTileSize())
	fx, fy := math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
	potion := items.CreateItemFromYAML("health_potion")
	g.addLootBagDrop(g.camera.X+fx*2.6*ts, g.camera.Y+fy*2.6*ts, []items.Item{potion}, 25)
	g.frameCount += 1000 // past the drop hop
	saveStatusEffectImage(t, filepath.Join(out, "loot_auto_pickup_before_1280x720.png"), captureLaidOutFrame(g, 1280, 720))
	ih := g.gameLoop.inputHandler
	for i := 0; i < 400 && len(g.groundContainers) > 0; i++ {
		ih.keys = keytracker.Consumer{}
		ih.heldKeys = heldOnly(ebiten.KeyW)
		ih.HandleInput()
	}
	ih.heldKeys = nil
	if len(g.groundContainers) != 0 {
		t.Fatal("walking up to the bag did not pick it up")
	}
	saveStatusEffectImage(t, filepath.Join(out, "loot_auto_pickup_after_1280x720.png"), captureLaidOutFrame(g, 1280, 720))
}

// The skill, school, trap, technique and weapon cards the skill audit
// reworded, each drawn over the Characters tab as the player sees it.
func TestDebugSim_AuditTooltips(t *testing.T) {
	out := filepath.Join(featureShotDir(t), "tooltips")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ui := g.gameLoop.ui
	prevCursor, prevDetail := rawCursorPosition, tooltipDetailHeld
	defer func() { rawCursorPosition, tooltipDetailHeld = prevCursor, prevDetail }()
	rawCursorPosition = func() (int, int) { return 0, 0 }
	cfg := g.config
	names := map[character.CharacterClass]string{
		character.ClassKnight: "Gareth", character.ClassArcher: "Silvelyn", character.ClassArmsMaster: "Gwen",
		character.ClassSorcerer: "Lysander", character.ClassBattleMage: "Isolde", character.ClassCleric: "Celestine",
		character.ClassThief: "Nyra", character.ClassWayfarer: "Kael", character.ClassSniper: "Mara",
		character.ClassAlchemist: "Elara", character.ClassMonk: "Sora",
	}
	hero := func(_ string, class character.CharacterClass) *character.MMCharacter {
		c := character.CreateCharacter(names[class], class, cfg)
		g.party.Members[0] = c
		g.selectedChar = 0
		return c
	}
	type card struct {
		name string
		pose func() (lines []string, queue func(lines []string, x, y int))
	}
	skill := func(name string, class character.CharacterClass, st character.SkillType) card {
		return card{name, func() ([]string, func([]string, int, int)) {
			hero(name, class)
			return strings.Split(masteryTooltipTextForSkill(st), "\n"), ui.queueTooltip
		}}
	}
	item := func(file string, class character.CharacterClass, make func(c *character.MMCharacter) items.Item, full bool) card {
		return card{file, func() ([]string, func([]string, int, int)) {
			c := hero("Tester", class)
			it := make(c)
			tooltipDetailHeld = func() bool { return full }
			lines := strings.Split(GetItemTooltip(it, c, g.combat, full), "\n")
			return lines, func(l []string, x, y int) { ui.queueItemTooltip(l, it, c, x, y) }
		}}
	}
	cards := []card{
		skill("skill_sword", character.ClassKnight, character.SkillSword),
		skill("skill_blaster", character.ClassArcher, character.SkillBlaster),
		skill("skill_dual_wielding", character.ClassArmsMaster, character.SkillDualWielding),
		skill("skill_meditation", character.ClassSorcerer, character.SkillMeditation),
		skill("skill_strong_magic", character.ClassBattleMage, character.SkillStrongMagic),
		skill("skill_natural_healer", character.ClassCleric, character.SkillNaturalHealer),
		skill("skill_dark_elf_binding", character.ClassThief, character.SkillDarkElfBinding),
		skill("skill_sleight_of_hand", character.ClassThief, character.SkillSleightOfHand),
		skill("skill_flowing_staff", character.ClassWayfarer, character.SkillFlowingStaff),
		skill("skill_designate_target", character.ClassSniper, character.SkillDesignateTarget),
		skill("skill_overwatch", character.ClassSniper, character.SkillOverwatch),
		skill("skill_translocation", character.ClassWayfarer, character.SkillTranslocation),
		skill("skill_pharmacology", character.ClassAlchemist, character.SkillPharmacology),
		{"school_fire", func() ([]string, func([]string, int, int)) {
			hero("Lysander", character.ClassSorcerer)
			return strings.Split(magicMasteryTooltipText(character.MagicSchoolFire), "\n"), ui.queueTooltip
		}},
		item("trap_cleave", character.ClassThief, func(*character.MMCharacter) items.Item {
			it, _ := config.TrapItem("cleave_trap")
			return it
		}, true),
		item("technique_phase_veil", character.ClassWayfarer, func(*character.MMCharacter) items.Item {
			it, _ := config.TechniqueItem("phase_veil")
			return it
		}, false),
		item("technique_quickening", character.ClassWayfarer, func(*character.MMCharacter) items.Item {
			it, _ := config.TechniqueItem("quickening")
			return it
		}, false),
		item("spell_charm", character.ClassCleric, func(*character.MMCharacter) items.Item {
			it, _ := spells.CreateSpellItem("charm")
			return it
		}, false),
		item("weapon_monk_fists_grandmaster", character.ClassMonk, func(c *character.MMCharacter) items.Item {
			c.Skills[character.SkillMartialArts] = &character.Skill{Mastery: character.MasteryGrandMaster}
			return c.Equipment[items.SlotMainHand]
		}, true),
	}
	g.menuOpen, g.currentTab = true, TabCharacters
	for _, c := range cards {
		lines, queue := c.pose()
		tooltipDetailHeld = func() bool { return false }
		w, h := g.gameLoop.Layout(1280, 720)
		frame := ebiten.NewImage(w, h)
		shot := image.NewRGBA(image.Rect(0, 0, w, h))
		runOnDrawFrame(func(_ *ebiten.Image) {
			beginUIFrame(frame, g.uiPixelScale())
			ui.Draw(frame)
			queue(lines, 360, 120)
			ui.drawQueuedTooltips(frame)
			frame.ReadPixels(shot.Pix)
		})
		frame.Deallocate()
		saveStatusEffectImage(t, filepath.Join(out, c.name+".png"), shot)
	}
}

// Party card frames in both clocks: selection, RT cooldowns (single and
// dual-wield) and TB action states (fresh, one hand spent, fully spent).
func TestDebugSim_PartyCardFrames(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	cfg := g.config
	gwen := character.CreateCharacter("Gwen", character.ClassArmsMaster, cfg)
	gwen.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("iron_sword")
	g.party.Members[2] = gwen
	name := os.Getenv("RAM_FRAMES_NAME")
	if name == "" {
		name = "party_frames"
	}
	for _, pose := range []struct {
		mode string
		tb   bool
	}{{"rt", false}, {"tb", true}} {
		g.turnBasedMode = pose.tb
		g.selectedChar = 1
		for i, m := range g.party.Members {
			m.RTCooldown, m.OffHandRTCooldown = 0, 0
			m.ActionsRemaining = 1
			if pose.tb {
				switch i {
				case 0:
					m.ActionsRemaining = 0 // spent
				case 2:
					m.ActionsRemaining = 1 // dual wielder: one hand spent
				}
			} else {
				switch i {
				case 0:
					m.RTCooldown = 60
				case 1:
					m.RTCooldown = 90
				case 2:
					m.RTCooldown, m.OffHandRTCooldown = 60, 30
				}
			}
		}
		for _, res := range [][2]int{{1280, 720}, {1920, 1080}} {
			shot := captureLaidOutFrame(g, res[0], res[1])
			// A cooldown frame fills over time: shoot mid-way by stepping the
			// tracked peaks once.
			if !pose.tb {
				for _, m := range g.party.Members {
					if m.RTCooldown > 0 {
						m.RTCooldown /= 2
					}
					if m.OffHandRTCooldown > 0 {
						m.OffHandRTCooldown /= 2
					}
				}
				shot = captureLaidOutFrame(g, res[0], res[1])
			}
			b := shot.Bounds()
			crop := shot.SubImage(image.Rect(0, b.Dy()-130, b.Dx(), b.Dy()))
			saveStatusEffectImage(t, filepath.Join(out, fmt.Sprintf("%s_%s_%dx%d.png", name, pose.mode, res[0], res[1])), crop)
		}
	}
}

// The mode panel in every shipped font, real-time and turn-based.
func TestDebugSim_TurnModePanelFonts(t *testing.T) {
	out := featureShotDir(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	defer g.applyUIFont()
	var tiles []image.Image
	for _, f := range g.config.Display.Fonts {
		setActiveUIFont(uiFontAtlases[f.Key])
		for _, tb := range []bool{false, true} {
			g.turnBasedMode, g.currentTurn = tb, 0
			shot := captureLaidOutFrame(g, 1280, 720)
			tiles = append(tiles, shot.SubImage(image.Rect(1280-260, 0, 1280, 110)))
		}
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 260*2, 110*len(tiles)/2))
	for i, tile := range tiles {
		b := tile.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				sheet.Set(i%2*260+x, i/2*110+y, tile.At(b.Min.X+x, b.Min.Y+y))
			}
		}
	}
	saveStatusEffectImage(t, filepath.Join(out, "turn_mode_panel_fonts.png"), sheet)
}

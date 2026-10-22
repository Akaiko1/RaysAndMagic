package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pending casts hand the view to their picker without committing. Refusals
// leave the original view intact; Escape from a picker returns to the world.
// Each picker kind (identify, ally, beacon, portal) runs from the book and
// the quick slot in RT and TB; the book's F is Enter's alias; a refusal
// happens before any spell's own branch, so one spell covers both.
func TestDeferredSpellInputHandoff(t *testing.T) {
	type entryKey struct {
		name string
		book bool
		key  ebiten.Key
	}
	bookEnter, bookF, quickF := entryKey{"book Enter", true, ebiten.KeyEnter}, entryKey{"book F", true, ebiten.KeyF}, entryKey{"quick F", false, ebiten.KeyF}
	type cell struct {
		id    spells.SpellID
		tb    bool
		entry entryKey
		state string
	}
	var cells []cell
	kinds := map[string]bool{}
	loadTestConfig(t) // the spell catalog the cells are chosen from
	for _, id := range []spells.SpellID{"identify_monster", "fire_aura", "fate", "vampiric_weapon", "vvoid_beacon", "town_portal"} {
		d, err := spells.GetSpellDefinitionByID(id)
		if err != nil {
			t.Fatal(err)
		}
		// The picker branch executeSpellCast takes, in its own order.
		var kind string
		switch {
		case d.TargetsAlly():
			kind = "ally"
		case d.Mechanic != nil && d.Mechanic.Kind == config.MechanicIdentifyMonster:
			kind = "identify"
		case d.Mechanic != nil && d.Mechanic.Kind == config.MechanicVvoidBeacon:
			kind = "beacon"
		case d.TownPortal:
			kind = "portal"
		default:
			t.Fatalf("%s opens no picker", id)
		}
		if kinds[kind] {
			continue
		}
		kinds[kind] = true
		for _, tb := range []bool{false, true} {
			for _, entry := range []entryKey{bookEnter, quickF} {
				cells = append(cells, cell{id, tb, entry, "ready"})
				if len(kinds) == 1 {
					cells = append(cells, cell{id, tb, entry, "no SP"}, cell{id, tb, entry, "no action"})
				}
			}
		}
	}
	if len(kinds) != 4 {
		t.Fatalf("fixture covers picker kinds %v, want identify, ally, beacon and portal", kinds)
	}
	cells = append(cells, cell{cells[0].id, false, bookF, "ready"})
	for _, cc := range cells {
		id, tb, entry, state := cc.id, cc.tb, cc.entry, cc.state
		t.Run(fmt.Sprintf("%s/TB=%v/%s/%s", id, tb, entry.name, state), func(t *testing.T) {
			g, _, _ := travelFixture(t)
			installFakePointer(t)
			g.turnBasedMode, g.currentTurn, g.selectedChar = tb, 0, 0
			g.menuOpen, g.currentTab = entry.book, TabSpellbook
			g.selectedSchool, g.selectedSpell = 0, 0
			g.visitedTavernMaps = map[string]bool{"other": true}
			c := g.party.Members[0]
			c.Class = character.ClassSorcerer
			d, err := spells.GetSpellDefinitionByID(id)
			if err != nil {
				t.Fatal(err)
			}
			c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolID(d.School): {KnownSpells: []spells.SpellID{id}}}
			c.Equipment[items.SlotQuickAction] = items.Item{Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffect(id)}
			c.SpellPoints, c.MaxSpellPoints, c.ActionsRemaining, c.RTCooldown = 100, 100, 2, 0
			switch state {
			case "no SP":
				c.SpellPoints = 0
			case "no action":
				if tb {
					c.ActionsRemaining = 0
				} else {
					c.RTCooldown = 20
				}
			}
			sp, actions, cooldown := c.SpellPoints, c.ActionsRemaining, c.RTCooldown
			ih := NewInputHandler(g)
			pressKeys(ih, entry.key)
			if c.SpellPoints != sp || c.ActionsRemaining != actions || c.RTCooldown != cooldown {
				t.Fatal("unconfirmed or refused cast spent resources")
			}
			if state != "ready" {
				if g.menuOpen != entry.book || g.townPortalPickerOpen {
					t.Fatal("refused cast changed its original view or opened a picker")
				}
				return
			}
			if g.menuOpen || !g.townPortalPickerOpen || g.townPortalCaster != c || g.townPortalSpell != id || !g.gameplayPausedByOverlay() {
				t.Fatal("pending cast did not replace the book with its paused picker")
			}
			pressKeys(ih, ebiten.KeyEscape)
			if g.menuOpen || g.townPortalPickerOpen || g.gameplayPausedByOverlay() {
				t.Fatal("cancelling targeting did not return to the world")
			}
			if c.SpellPoints != sp || c.ActionsRemaining != actions || c.RTCooldown != cooldown {
				t.Fatal("cancelling targeting spent resources")
			}
		})
	}
}

func TestIdentifyTargetingOwnsPointerAndPayment(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, choice := range []string{"target", "empty", "out_of_range", "wall", "removed", "cancel"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, choice), func(t *testing.T) {
				g, ih, fp, target, _ := mouseCombatHarness(t, tb)
				g.sprites = graphics.NewSpriteManager()
				c := g.party.Members[0]
				c.Class = character.ClassSorcerer
				c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolMind: {KnownSpells: []spells.SpellID{"identify_monster"}}}
				c.Equipment[items.SlotQuickAction] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "identify_monster"}
				c.SpellPoints, c.MaxSpellPoints = 100, 100
				sp, actions, hp := c.SpellPoints, c.ActionsRemaining, target.HitPoints
				g.menuOpen, g.currentTab = true, TabSpellbook
				g.selectedSchool, g.selectedSpell = 0, 0
				pressKeys(ih, ebiten.KeyEnter)
				if !g.identifySelecting || g.menuOpen || !g.gameplayPausedByOverlay() || !g.identifyCursorActive() {
					t.Fatal("cast must close the book and open paused lens targeting")
				}
				if c.SpellPoints != sp || c.ActionsRemaining != actions || c.RTCooldown != 0 {
					t.Fatal("opening target selection spent an action or mana")
				}
				pressKeys(ih) // Release Enter before the later pointer gesture.
				screen := ebiten.NewImage(640, 480)
				t.Cleanup(screen.Deallocate)
				g.gameLoop.ui.Draw(screen)
				clock := g.frameCount
				g.gameLoop.updateExploration()
				if g.frameCount != clock || target.HitPoints != hp {
					t.Fatal("target selection advanced the simulation")
				}
				switch choice {
				case "empty":
					fp.moveTo(2, 2)
				case "out_of_range":
					target.X += g.config.GetTileSize() * 100
				case "wall":
					g.depthBuffer = make([]float64, g.worldWidth())
					for i := range g.depthBuffer {
						g.depthBuffer[i] = g.config.GetTileSize() * .5
					}
				case "removed":
					g.world.Monsters = nil
				case "cancel":
					pressKeys(ih, ebiten.KeyEscape)
				}
				if choice != "cancel" {
					fp.press()
					g.gameLoop.ui.updateMouseState()
					g.gameLoop.ui.dispatchDisplayedInput()
					ih.HandleInput()
				}
				if choice == "target" {
					d, _ := spells.GetSpellDefinitionByID("identify_monster")
					if g.identifiedMonster != target || g.identifySelecting || c.SpellPoints != sp-c.MeditatedSPCost(d.SpellPointsCost) || !g.gameplayPausedByOverlay() {
						t.Fatal("visible target click did not pay once and open a paused report")
					}
					if tb && c.ActionsRemaining != actions-1 || !tb && c.RTCooldown <= 0 {
						t.Fatal("confirmed identification did not spend its action")
					}
					paid := c.SpellPoints
					g.confirmIdentify(target)
					if c.SpellPoints != paid {
						t.Fatal("report accepted a second confirmation")
					}
				} else if g.identifiedMonster != nil || c.SpellPoints != sp || c.ActionsRemaining != actions || c.RTCooldown != 0 {
					t.Fatal("invalid/cancelled identification spent resources or selected a target")
				}
				if target.HitPoints != hp || len(g.slashEffects) > 0 || len(g.magicProjectiles) > 0 {
					t.Fatal("identify click fell through to an attack")
				}
				if choice != "cancel" {
					pressKeys(ih, ebiten.KeyEscape)
				}
				if g.townPortalPickerOpen || g.identifyCursorActive() || g.gameplayPausedByOverlay() {
					t.Fatal("Escape did not release targeting/report pause")
				}
			})
		}
	}
}

func TestAllySpellTargetingOwnsPointerAndPayment(t *testing.T) {
	for _, tc := range []struct {
		id       spells.SpellID
		recovers bool // combat_buff keeps its RT cooldown; a plain buff has none
	}{{"fire_aura", true}, {"fate", true}, {"vampiric_weapon", false}} {
		id := tc.id
		for _, tb := range []bool{false, true} {
			for _, choice := range []string{"other", "self", "empty", "dead", "cancel"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", id, tb, choice), func(t *testing.T) {
					g, ih, fp, target, _ := mouseCombatHarness(t, tb)
					g.sprites = graphics.NewSpriteManager()
					g.showPartyStats = true
					c := g.party.Members[0]
					d, _ := spells.GetSpellDefinitionByID(id)
					c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolID(d.School): {KnownSpells: []spells.SpellID{id}}}
					c.SpellPoints, c.MaxSpellPoints = 100, 100
					c.Equipment[items.SlotQuickAction] = items.Item{Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffect(id)}
					sp, actions := c.SpellPoints, c.ActionsRemaining
					g.menuOpen, g.currentTab = true, TabSpellbook
					g.selectedSchool, g.selectedSpell = 0, 0
					pressKeys(ih, ebiten.KeyF)
					if !g.allySpellCursorActive() || g.menuOpen || !g.gameplayPausedByOverlay() {
						t.Fatal("personal enchantment did not open paused card targeting")
					}
					if c.SpellPoints != sp || c.ActionsRemaining != actions || len(c.SpellState.Effects) > 0 {
						t.Fatal("opening selection spent resources or silently enchanted the caster")
					}
					pressKeys(ih) // Release F before the later pointer gesture.
					clock, hp, mx, my := g.frameCount, c.HitPoints, target.X, target.Y
					for range 12 {
						g.gameLoop.updateExploration()
					}
					if g.frameCount != clock || c.HitPoints != hp || target.X != mx || target.Y != my {
						t.Fatal("ally target selection advanced the world or let a monster act")
					}
					index := 1
					if choice == "self" {
						index = 0
					}
					if choice == "dead" {
						g.party.Members[index].HitPoints = 0
					}
					pw, ph, left, top := partyPortraitLayout(g)
					fp.moveTo(left+index*pw+pw/2, top+ph/2)
					if choice == "empty" {
						fp.moveTo(2, 2)
					}
					screen := ebiten.NewImage(640, 480)
					t.Cleanup(screen.Deallocate)
					g.gameLoop.ui.Draw(screen)
					if choice == "cancel" {
						pressKeys(ih, ebiten.KeyEscape)
					} else {
						fp.press()
						g.gameLoop.ui.updateMouseState()
						g.gameLoop.ui.dispatchDisplayedInput()
						ih.HandleInput()
					}
					if choice == "other" || choice == "self" {
						if len(g.party.Members[index].SpellState.Effects) != 1 || g.allySpellSelecting || g.gameplayPausedByOverlay() || c.SpellPoints != sp-c.MeditatedSPCost(d.SpellPointsCost) {
							t.Fatal("card click did not enchant the chosen hero, pay once and resume")
						}
						wantActions := actions
						if tb {
							wantActions--
						}
						if c.ActionsRemaining != wantActions || (c.RTCooldown != 0) != tc.recovers {
							t.Fatalf("personal buff must cost one TB action; RT recovery=%d, want recovery=%v", c.RTCooldown, tc.recovers)
						}
						card := GetSpellTooltipRows(id, c, g.combat, true).String()
						if !strings.Contains(card, "Target: One Ally") || strings.Contains(card, "Target: Entire Party") || strings.Contains(card, "No RT cooldown") == tc.recovers {
							t.Fatalf("personal buff tooltip misrepresents targeting or recovery: %s", card)
						}
						paid := c.SpellPoints
						g.confirmAllySpell(index)
						if c.SpellPoints != paid {
							t.Fatal("double confirmation spent mana twice")
						}
					} else if c.SpellPoints != sp || c.ActionsRemaining != actions {
						t.Fatal("invalid/cancelled target spent resources")
					}
				})
			}
		}
	}
}

func TestAutocastCheckboxOwnsPhysicalGesture(t *testing.T) {
	for _, id := range []spells.SpellID{"haste", "body_purify", "dispel_magic", "celestial_armor"} {
		t.Run(string(id), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1280, 720)
			g := h.g
			g.menuOpen, g.currentTab, g.selectedSpell = true, TabSpellbook, -1
			c := g.party.Members[0]
			c.Class = character.ClassSorcerer
			d, err := spells.GetSpellDefinitionByID(id)
			if err != nil {
				t.Fatal(err)
			}
			c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolID(d.School): {KnownSpells: []spells.SpellID{id}}}
			menu := computeTabbedMenuLayout(g.config.GetScreenWidth(), gameplayViewportBottom(g))
			bl := computeBookLayout(menu.content, 1)
			x, y := bl.cardPos(0)
			box := spellAutocastRect(x, y, bl.cardW, bl.iconSize)
			fp := installFakePointer(t)
			fp.moveTo(box.x+box.w/2, box.y+box.h/2)
			fp.press()
			h.pointerStep()
			if !c.SpellState.Automatic[string(id)] || g.selectedSpell != -1 || g.dragSrc != dragNone {
				t.Fatalf("checkbox failed ownership: on=%v selection=%d drag=%v", c.SpellState.Automatic[string(id)], g.selectedSpell, g.dragSrc)
			}
			fp.hold()
			fp.moveTo(box.x+dragDeadZone+20, box.y)
			h.pointerStep()
			if g.dragSrc != dragNone {
				t.Fatal("checkbox press became a spell drag")
			}
			fp.release()
			h.pointerStep()
			fp.moveTo(box.x+box.w/2, box.y+box.h/2)
			fp.press()
			h.pointerStep()
			fp.release()
			h.pointerStep()
			if c.SpellState.Automatic[string(id)] || g.selectedSpell != -1 {
				t.Fatal("second checkbox click did not toggle off without selecting the spell")
			}
			// Ordinary spell art retains drag ownership as a positive control.
			fp.moveTo(x+bl.cardW/2, y+bl.iconSize/2)
			fp.press()
			h.pointerStep()
			fp.hold()
			fp.moveTo(x+bl.cardW/2+dragDeadZone+20, y+bl.iconSize/2)
			h.pointerStep()
			if g.dragSrc != dragFromSpell || !g.drag.active {
				t.Fatal("excluding checkbox disabled ordinary spell dragging")
			}
		})
	}
}

func TestAdvancedModalNavigationWithoutMouseClick(t *testing.T) {
	h := newDisplayedModalHarness(t, 1280, 720)
	g, c := h.g, h.g.party.Members[0]
	c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolWater: {Mastery: character.MasteryGrandMaster, KnownSpells: []spells.SpellID{"vvoid_beacon"}}}
	c.SpellPoints, c.ActionsRemaining = 500, 2
	g.turnBasedMode, g.currentTurn = true, 0
	g.townPortalCaster, g.townPortalSpell, g.townPortalPickerOpen, g.beaconPicker = c, "vvoid_beacon", true, true
	h.ui.Draw(h.screen)
	pressKeys(h.loop.inputHandler, ebiten.KeyArrowDown)
	if g.beaconSelection != 1 {
		t.Fatal("keyboard-only Down did not select next beacon")
	}
	pressKeys(h.loop.inputHandler, ebiten.KeyArrowUp)
	if g.beaconSelection != 0 {
		t.Fatal("keyboard-only Up did not select previous beacon")
	}
	pressKeys(h.loop.inputHandler, ebiten.KeyB)
	if len(c.SpellState.Beacons) != 1 || g.townPortalPickerOpen {
		t.Fatal("keyboard-only B did not bind and close")
	}
	g.townPortalCaster, g.townPortalSpell, g.townPortalPickerOpen, g.beaconPicker = c, "vvoid_beacon", true, true
	c.ActionsRemaining, c.RTCooldown = 2, 0
	before := c.SpellPoints
	pressKeys(h.loop.inputHandler, ebiten.KeyEnter)
	if g.townPortalPickerOpen || c.SpellPoints >= before {
		t.Fatal("keyboard-only Enter did not recall")
	}

	g.identifiedMonster = &monster.Monster3D{Name: "Many defenses", Resistances: map[monster.DamageType]int{}}
	for i := 0; i < 32; i++ {
		g.identifiedMonster.Resistances[monster.DamageType(fmt.Sprintf("defense%d", i))] = 10
	}
	g.townPortalPickerOpen = true
	h.ui.Draw(h.screen)
	oldWheel := pointerWheel
	t.Cleanup(func() { pointerWheel = oldWheel })
	pointerWheel = func() (float64, float64) { return 0, -1 }
	h.ui.updateMouseState()
	h.ui.dispatchDisplayedInput()
	if g.identifyScroll <= 0 {
		t.Fatal("wheel-only update did not scroll identification")
	}
	pressKeys(h.loop.inputHandler, ebiten.KeyEscape)
	if g.townPortalPickerOpen || g.identifiedMonster != nil {
		t.Fatal("Escape did not release identification ownership")
	}
}

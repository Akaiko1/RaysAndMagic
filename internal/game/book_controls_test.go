package game

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

func bookControlsHarness(t *testing.T, trap, tb bool) (*displayedModalHarness, *character.MMCharacter, string, layoutRect) {
	t.Helper()
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	class, name := character.ClassSorcerer, "Lysander"
	if trap {
		class, name = character.ClassThief, "Nyra"
	}
	actor := character.CreateCharacter(name, class, g.config)
	actor.SpellPoints, actor.MaxSpellPoints = 100, 100
	g.party.Members[0] = actor
	g.party.Reserve, g.party.Captive = nil, nil
	g.menuOpen, g.currentTab, g.selectedChar, g.turnBasedMode = true, TabSpellbook, 0, tb
	g.world.Monsters = nil
	g.parkSelection = true
	g.world.Width, g.world.Height = 8, 8
	g.world.Tiles = make([][]world.TileType3D, 8)
	for y := range g.world.Tiles {
		g.world.Tiles[y] = make([]world.TileType3D, 8)
	}
	for y := range g.world.Tiles {
		for x := range g.world.Tiles[y] {
			g.world.Tiles[y][x] = world.TileEmpty
		}
	}
	g.camera.X, g.camera.Y, g.camera.Angle = 96, 96, 0
	for i, member := range g.party.Members {
		if i != 0 && member.Name == actor.Name {
			member.Name = fmt.Sprintf("Companion%d", i)
		}
		member.ActionsRemaining = 3
	}
	key, index := "firebolt", -1
	if trap {
		key = "cleave_trap"
		for i, k := range availableTraps(actor) {
			if k == key {
				index, g.selectedTrap = i, i
			}
		}
	} else {
		for si, school := range spellbookSchoolsWithSpells(actor) {
			for i, id := range actor.GetSpellsForSchool(school) {
				if string(id) == key {
					index, g.selectedSchool, g.selectedSpell = i, si, i
				}
			}
		}
	}
	if index < 0 {
		t.Fatal("test ability missing")
	}
	// A different existing quick action must survive direct book use.
	actor.Equipment[items.SlotSpell] = items.Item{Name: "Previous quick action", Type: items.ItemUtilitySpell, SpellEffect: "heal"}
	menu := computeTabbedMenuLayout(g.config.GetScreenWidth(), gameplayViewportBottom(g))
	book := computeBookLayout(menu.content)
	x, y := book.cardPos(index % book.cardsPerSpread())
	// Real displayed controls must resolve art from the same root as the game.
	t.Chdir("../..")
	return h, actor, key, layoutRect{x, y, book.cardW, book.cardH}
}

func TestBookPhysicalClicksOnlyEquip(t *testing.T) {
	for _, trap := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			for _, state := range []string{"ready", "no SP", "down", "cooldown"} {
				t.Run(fmt.Sprintf("trap=%v/TB=%v/%s", trap, tb, state), func(t *testing.T) {
					h, actor, key, card := bookControlsHarness(t, trap, tb)
					g := h.g
					switch state {
					case "no SP":
						actor.SpellPoints = 0
					case "down":
						actor.HitPoints = 0
					case "cooldown":
						actor.RTCooldown = 20
					}
					beforeSP, beforeAction := actor.SpellPoints, actor.ActionsRemaining
					fp := installFakePointer(t)
					fp.moveTo(card.x+card.w/2, card.y+card.h/2)
					fp.press()
					h.pointerStep()
					if string(actor.Equipment[items.SlotSpell].SpellEffect) == key {
						t.Fatal("single click equipped")
					}
					fp.hold()
					for i := 0; i < 3; i++ {
						h.pointerStep()
					}
					if string(actor.Equipment[items.SlotSpell].SpellEffect) == key {
						t.Fatal("held click became double-click")
					}
					fp.release()
					h.pointerStep()
					fp.press()
					h.pointerStep()
					if got := string(actor.Equipment[items.SlotSpell].SpellEffect); got != key {
						t.Fatalf("double-click equipped %q, want %q", got, key)
					}
					if !g.menuOpen || actor.SpellPoints != beforeSP || actor.ActionsRemaining != beforeAction || len(g.traps) != 0 {
						t.Fatal("equipping used a world action")
					}
					fp.release()
					h.pointerStep()
					delete(actor.Equipment, items.SlotSpell)
					fp.press()
					h.pointerStep()
					if _, ok := actor.Equipment[items.SlotSpell]; ok {
						t.Fatal("third click reused the previous pair")
					}
				})
			}
		}
	}
}

func TestBookEnterAndFUseSelection(t *testing.T) {
	for _, trap := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyF} {
				for _, state := range []string{"ready", "no SP", "down", "stunned", "no action", "blocked tile", "owner cap", "locked"} {
					if !trap && (state == "blocked tile" || state == "owner cap" || state == "locked") {
						continue
					}
					t.Run(fmt.Sprintf("trap=%v/TB=%v/key=%v/%s", trap, tb, key, state), func(t *testing.T) {
						h, actor, ability, _ := bookControlsHarness(t, trap, tb)
						g := h.g
						switch state {
						case "no SP":
							actor.SpellPoints = 0
						case "down":
							actor.HitPoints = 0
						case "stunned":
							actor.StunFramesRemaining = 30
						case "no action":
							if tb {
								actor.ActionsRemaining = 0
							} else {
								actor.RTCooldown = 20
							}
						case "blocked tile":
							g.world.Tiles[1][2] = world.TileWall
						case "owner cap":
							for i := 0; i < MaxTrapsPerOwner; i++ {
								g.traps = append(g.traps, PlacedTrap{Key: ability, MapKey: currentMapKey(), TileX: i, TileY: 0, Owner: actor})
							}
						case "locked":
							actor.Level = 0
						}
						beforeSP, beforeActions, beforeCooldown, beforeTraps := actor.SpellPoints, actor.ActionsRemaining, actor.RTCooldown, len(g.traps)
						beforeEquipped := actor.Equipment[items.SlotSpell]
						ih := h.loop.inputHandler
						ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == key })
						ih.handleSpellbookNavigation()
						if state == "ready" {
							if g.menuOpen || actor.SpellPoints >= beforeSP || actor.RTCooldown <= 0 {
								t.Fatal("successful use did not close book/pay SP/arm cooldown")
							}
							if tb && actor.ActionsRemaining != beforeActions-1 {
								t.Fatal("TB use did not spend one action")
							}
							if !tb && actor.ActionsRemaining != beforeActions {
								t.Fatal("RT use spent a TB action")
							}
							if trap && (len(g.traps) != 1 || g.traps[0].Key != ability) {
								t.Fatal("did not place the selected trap")
							}
						} else if !g.menuOpen || actor.SpellPoints != beforeSP || actor.ActionsRemaining != beforeActions || actor.RTCooldown != beforeCooldown || len(g.traps) != beforeTraps {
							t.Fatal("refused use changed book/resources/world")
						}
						if !reflect.DeepEqual(actor.Equipment[items.SlotSpell], beforeEquipped) {
							t.Fatal("direct use changed equipped quick action")
						}
						afterSP, afterTraps := actor.SpellPoints, len(g.traps)
						ih.handleSpellbookNavigation() // the same keyboard edge was already consumed
						if actor.SpellPoints != afterSP || len(g.traps) != afterTraps {
							t.Fatal("one key press was consumed twice")
						}
					})
				}
			}
		}
	}
}

func TestTrapBookLockedEntryCannotEquip(t *testing.T) {
	h, actor, _, card := bookControlsHarness(t, true, false)
	actor.Level = 0
	before := actor.Equipment[items.SlotSpell]
	fp := installFakePointer(t)
	fp.moveTo(card.x+card.w/2, card.y+card.h/2)
	for i := 0; i < 2; i++ {
		fp.press()
		h.pointerStep()
		fp.release()
		h.pointerStep()
	}
	if !reflect.DeepEqual(before, actor.Equipment[items.SlotSpell]) || !h.g.menuOpen {
		t.Fatal("locked trap equipped or closed book")
	}
}

func TestBookUseRetainsTrapSaveContract(t *testing.T) {
	h, actor, key, _ := bookControlsHarness(t, true, true)
	if !equipTrap(actor, key) || !h.loop.inputHandler.useSelectedBookEntryFromHub() {
		t.Fatal("trap book setup failed")
	}
	saves := buildTrapSaves(h.g.traps)
	restored := restoreTraps(saves, h.g.party)
	if len(restored) != 1 || restored[0].Owner != actor || restored[0].Key != key {
		t.Fatal("book trap lost its identity/owner on restore")
	}
	def, ok := config.TrapItem(key)
	if !ok || actor.Equipment[items.SlotSpell].SpellEffect != def.SpellEffect {
		t.Fatal("equipped trap uses a different save representation")
	}
}

func TestBookClickContextDoesNotCarryEquipmentGestures(t *testing.T) {
	for _, trap := range []bool{false, true} {
		for _, change := range []string{"character", "reopen", "modal"} {
			t.Run(fmt.Sprintf("trap=%v/%s", trap, change), func(t *testing.T) {
				h, actor, _, card := bookControlsHarness(t, trap, false)
				g := h.g
				fp := installFakePointer(t)
				fp.moveTo(card.x+card.w/2, card.y+card.h/2)
				fp.press()
				h.pointerStep()
				fp.release()
				h.pointerStep()
				before := actor.Equipment[items.SlotSpell]
				switch change {
				case "character":
					replacement := character.CreateCharacter("Another hero", actor.Class, g.config)
					replacement.Equipment[items.SlotSpell] = before
					g.party.Members[1] = replacement
					g.selectPartyMemberManually(1)
					actor = replacement
				case "reopen":
					g.menuOpen = false
					h.ui.syncCharacterHubClickContext()
					g.menuOpen = true
				case "modal":
					g.mainMenuOpen = true
				}
				fp.press()
				h.pointerStep()
				fp.release()
				h.pointerStep()
				if !reflect.DeepEqual(actor.Equipment[items.SlotSpell], before) || len(g.traps) != 0 {
					t.Fatal("click crossed the book context boundary")
				}
			})
		}
	}
}

// Both books and combat modes must select across cards, then equip only on a
// repeated click on that same card. Each target must be visible and usable.
func TestBookDifferentEntriesDoNotFormDoubleClick(t *testing.T) {
	for _, trap := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("trap=%v/TB=%v", trap, tb), func(t *testing.T) {
				h, actor, _, _ := bookControlsHarness(t, trap, tb)
				g := h.g
				var entries []string
				if trap {
					entries = availableTraps(actor)
				} else {
					schools := spellbookSchoolsWithSpells(actor)
					for _, id := range actor.GetSpellsForSchool(schools[g.selectedSchool]) {
						entries = append(entries, string(id))
					}
				}
				if len(entries) < 2 {
					t.Fatal("fixture requires two different book entries")
				}
				if entries[0] == entries[1] {
					t.Fatal("fixture entries must have different identities")
				}
				if trap {
					for _, key := range entries[:2] {
						def, ok := config.GetTrapDefinition(key)
						if !ok {
							t.Fatalf("missing trap definition for %q", key)
						}
						actor.Level = max(actor.Level, def.Level)
					}
				}
				g.selectedSpell, g.selectedTrap = -1, -1
				menu := computeTabbedMenuLayout(g.config.GetScreenWidth(), gameplayViewportBottom(g))
				book := computeBookLayout(menu.content)
				if book.cardsPerSpread() < 2 {
					t.Fatal("fixture requires both entries on the displayed page")
				}
				beforeSP, beforeActions := actor.SpellPoints, actor.ActionsRemaining
				beforeCooldown := actor.RTCooldown
				wantEquipped := actor.Equipment[items.SlotSpell]
				fp := installFakePointer(t)
				for step, click := range []struct {
					index int
					equip bool
				}{{0, false}, {1, false}, {1, true}, {0, false}, {0, true}} {
					x, y := book.cardPos(click.index)
					fp.moveTo(x+book.cardW/2, y+book.cardH/2)
					fp.press()
					h.pointerStep()
					fp.release()
					h.pointerStep()
					selected := g.selectedSpell
					if trap {
						selected = g.selectedTrap
					}
					if selected != click.index {
						t.Fatalf("step %d: selected entry %d, want %d", step, selected, click.index)
					}
					got := actor.Equipment[items.SlotSpell]
					if click.equip {
						if string(got.SpellEffect) != entries[click.index] {
							t.Fatalf("step %d: double-click equipped %q, want %q", step, got.SpellEffect, entries[click.index])
						}
						wantEquipped = got
					} else if !reflect.DeepEqual(got, wantEquipped) {
						t.Fatalf("step %d: click on a different entry changed equipment", step)
					}
					if !g.menuOpen || actor.SpellPoints != beforeSP || actor.ActionsRemaining != beforeActions || actor.RTCooldown != beforeCooldown || len(g.traps) != 0 {
						t.Fatalf("step %d: book click used a world action", step)
					}
				}
			})
		}
	}
}

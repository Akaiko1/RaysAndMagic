package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

func TestQuickSpellTransferValidatesBothOwners(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		incomingKnown, outgoingKnown, reverseSpell bool
		accepted                                   bool
	}{
		{"known_move", true, true, false, true},
		{"unknown_move", false, true, false, false},
		{"known_swap", true, true, true, true},
		{"invalid_reverse_swap", true, false, true, false},
		{"invalid_forward_swap", false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestCombatSystemWithConfig(t).game
			source, target := g.party.Members[1], g.party.Members[2]
			if tc.incomingKnown {
				target.LearnSpell("firebolt")
			}
			if tc.outgoingKnown {
				source.LearnSpell("mass_heal")
			}
			incoming, _ := spells.CreateSpellItem("firebolt")
			outgoing := items.Item{Name: "Potion", Type: items.ItemConsumable}
			if tc.reverseSpell {
				outgoing, _ = spells.CreateSpellItem("mass_heal")
				target.LearnSpell("mass_heal")
			}
			source.QuickSlots[0], target.QuickSlots[0] = &incoming, &outgoing
			g.dragSrc, g.dragQuickChar, g.dragQuickSlot = dragFromQuickSlot, 1, 0
			g.selectPartyMemberManually(2)
			g.resolveQuickSlotDrop(2, 0)
			if tc.accepted {
				if source.QuickSlots[0] != &outgoing || target.QuickSlots[0] != &incoming {
					t.Fatal("legal shortcut swap failed")
				}
			} else if source.QuickSlots[0] != &incoming || target.QuickSlots[0] != &outgoing {
				t.Fatal("refused transfer lost or displaced an item")
			}
		})
	}
}

func TestPlayerSpellEntriesRequireKnowledge(t *testing.T) {
	for _, entry := range []string{"equipped", "quick_slot", "targeted_heal", "panel_binding", "book_drag", "inventory_drag"} {
		for _, known := range []bool{false, true} {
			name := "unknown"
			if known {
				name = "known"
			}
			t.Run(entry+"/"+name, func(t *testing.T) {
				g := newTestCombatSystemWithConfig(t).game
				caster := g.party.Members[0]
				id := spells.SpellID("firebolt")
				if entry == "targeted_heal" {
					id = "heal_other"
				}
				if known {
					caster.LearnSpell(id)
				}
				item, err := spells.CreateSpellItem(id)
				if err != nil {
					t.Fatal(err)
				}
				caster.SpellPoints = 100
				caster.RTCooldown = 0
				before := caster.SpellPoints
				switch entry {
				case "equipped":
					caster.Equipment[items.SlotQuickAction] = item
					if got := g.combat.CastEquippedSpell(); got != known {
						t.Fatalf("cast=%v, known=%v", got, known)
					}
				case "quick_slot":
					// Also models a stale shortcut restored from an old save.
					caster.QuickSlots[0] = &item
					g.useQuickSlot(0, 0)
				case "targeted_heal":
					caster.Equipment[items.SlotQuickAction] = item
					g.party.Members[1].HitPoints = 1
					if got := g.combat.CastEquippedHeal(); got != known {
						t.Fatalf("heal=%v, known=%v", got, known)
					}
				case "panel_binding":
					delete(caster.Equipment, items.SlotQuickAction)
					g.bindQuickAction(0, item)
					_, got := caster.Equipment[items.SlotQuickAction]
					if got != known {
						t.Fatal("panel binding bypassed ownership")
					}
					return
				case "book_drag":
					g.dragSrc, g.dragSpellID = dragFromSpell, id
					g.resolveQuickSlotDrop(0, 0)
					if (caster.QuickSlots[0] != nil) != known {
						t.Fatal("book drag bypassed ownership")
					}
					return
				case "inventory_drag":
					g.party.Inventory = []items.Item{item}
					g.dragSrc, g.dragInvIndex = dragFromInventory, 0
					g.resolveQuickSlotDrop(0, 0)
					if (caster.QuickSlots[0] != nil) != known || (len(g.party.Inventory) == 0) != known {
						t.Fatal("inventory transfer bypassed ownership or lost item")
					}
					return
				}
				if (caster.SpellPoints < before) != known {
					t.Fatalf("SP %d -> %d, known=%v", before, caster.SpellPoints, known)
				}
			})
		}
	}
}

func TestFreeCardSpellDoesNotRequireKnowledge(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := cs.game.party.Members[0]
	if characterKnowsSpellByID(caster, "firebolt") {
		t.Fatal("fixture already knows firebolt")
	}
	cs.game.selectedChar = 1 // proc attribution follows its actor, not UI selection
	before := caster.SpellPoints
	if !cs.tryCardFireBoltInstead(caster) || len(cs.game.magicProjectiles) != 1 || caster.SpellPoints != before {
		t.Fatal("explicit free card proc was rejected or charged mana")
	}
	if cs.game.magicProjectiles[0].Attacker != caster {
		t.Fatal("projectile lost the actual caster")
	}
}

// A quick-bar spell is the slot owner's action: their readiness gates it and
// their action pays for it, whoever is selected.
func TestQuickBarSpellChargesTheSlotOwner(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[tb], func(t *testing.T) {
			g := newTestCombatSystemWithConfig(t).game
			g.turnBasedMode, g.currentTurn = tb, 0
			selected, owner := g.party.Members[0], g.party.Members[2]
			for _, m := range []*character.MMCharacter{selected, owner} {
				m.ActionsRemaining, m.RTCooldown = 2, 0
			}
			owner.LearnSpell("firebolt")
			owner.SpellPoints = 100
			spell, err := spells.CreateSpellItem("firebolt")
			if err != nil {
				t.Fatal(err)
			}
			owner.QuickSlots[0] = &spell
			g.selectedChar = 0

			g.useQuickSlot(2, 0)

			if owner.SpellPoints == 100 {
				t.Fatal("the quick-bar spell was not cast (positive control)")
			}
			if selected.ActionsRemaining != 2 || selected.RTCooldown != 0 {
				t.Fatalf("selected hero paid: actions %d, cooldown %d", selected.ActionsRemaining, selected.RTCooldown)
			}
			if tb && owner.ActionsRemaining != 1 {
				t.Fatalf("slot owner actions = %d, want 1", owner.ActionsRemaining)
			}
			if owner.RTCooldown <= 0 {
				t.Fatal("slot owner's recovery did not start")
			}
		})
	}
}

package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
)

// The 2026-07-14 roadside props: a campfire (one-time free rest + gold cache),
// stat barrels (50% chance of a permanent green bonus for the SELECTED
// character, sprite swaps closed -> open when spent), and a box pile that
// stays standing after looting.

func TestCampfireFreeRestAndGoldOnce(t *testing.T) {
	g := crateTestGame(t)
	fire := spawnCrate(t, g, "campfire", g.camera.X+64, g.camera.Y)

	hurt := g.party.Members[0]
	hurt.HitPoints = 1
	hurt.SpellPoints = 0
	gold0 := g.party.Gold

	g.useLootCrate(fire)

	if hurt.HitPoints != hurt.MaxHitPoints || hurt.SpellPoints != hurt.MaxSpellPoints {
		t.Fatalf("campfire must fully rest the party (HP %d/%d SP %d/%d)",
			hurt.HitPoints, hurt.MaxHitPoints, hurt.SpellPoints, hurt.MaxSpellPoints)
	}
	if g.party.Gold != gold0+500 {
		t.Fatalf("campfire gold = %d, want +500", g.party.Gold-gold0)
	}
	if !fire.Visited {
		t.Fatal("campfire must be consumed")
	}

	// ONE time: a second use neither rests nor pays.
	hurt.HitPoints = 1
	g.useLootCrate(fire)
	if hurt.HitPoints != 1 || g.party.Gold != gold0+500 {
		t.Fatal("a spent campfire must do nothing")
	}
}

// Statistical run over the authored 50% barrel: both outcomes must occur, an
// empty barrel changes nothing, and every bonus is a PERMANENT effective-stat
// gain for the SELECTED character only (green delta - base stat untouched).
func TestStatBarrelPermanentBonusAndEmptyChance(t *testing.T) {
	bonuses, empties := 0, 0
	for i := 0; i < 200 && (bonuses == 0 || empties == 0); i++ {
		g := crateTestGame(t)
		barrel := spawnCrate(t, g, "barrel_red", g.camera.X+64, g.camera.Y)
		m := g.party.Members[0]
		base := m.Might
		effBefore := m.GetEffectiveMight()

		g.useLootCrate(barrel)

		if m.Might != base {
			t.Fatal("a stat barrel must never write the BASE stat")
		}
		switch m.GetEffectiveMight() {
		case effBefore + 1:
			bonuses++
			// Only the SELECTED character drinks - the rest get nothing.
			for i, mem := range g.party.Members {
				want := 0
				if i == g.selectedChar {
					want = 1
				}
				if mem.PermanentBonuses.Might != want {
					t.Fatalf("member %d (%s) bonus = %d, want %d (selected=%d)", i, mem.Name, mem.PermanentBonuses.Might, want, g.selectedChar)
				}
			}
			found := false
			for _, msg := range g.GetCombatMessages() {
				if strings.Contains(msg, m.Name) && strings.Contains(msg, "permanently") {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("bonus must be announced with the character's name")
			}
		case effBefore:
			empties++
		default:
			t.Fatalf("unexpected effective might %d (was %d)", m.GetEffectiveMight(), effBefore)
		}
		if !barrel.Visited {
			t.Fatal("barrel must be consumed either way")
		}
		// Spent barrel gives nothing more.
		eff := m.GetEffectiveMight()
		g.useLootCrate(barrel)
		if m.GetEffectiveMight() != eff {
			t.Fatal("a spent barrel must not grant again")
		}
	}
	if bonuses == 0 || empties == 0 {
		t.Fatalf("50%% barrel never produced both outcomes in 200 runs (bonus=%d empty=%d)", bonuses, empties)
	}
}

// The intellect barrel must grow MaxSP immediately (max pools derive from
// effective stats).
func TestIntellectBarrelGrowsSpellPool(t *testing.T) {
	for i := 0; i < 200; i++ {
		g := crateTestGame(t)
		barrel := spawnCrate(t, g, "barrel_blue", g.camera.X+64, g.camera.Y)
		caster := g.party.Members[0]
		maxSP0 := caster.MaxSpellPoints
		sp0 := caster.SpellPoints
		g.useLootCrate(barrel)
		if caster.PermanentBonuses.Intellect == 1 {
			if caster.MaxSpellPoints <= maxSP0 {
				t.Fatalf("intellect barrel must re-derive MaxSP (%d -> %d)", maxSP0, caster.MaxSpellPoints)
			}
			// Irreversible gain grants the delta to CURRENT SP as well (the
			// stat-point convention), not just the ceiling.
			if caster.SpellPoints != sp0+(caster.MaxSpellPoints-maxSP0) {
				t.Fatalf("current SP must grow by the max delta (SP %d -> %d, MaxSP %d -> %d)",
					sp0, caster.SpellPoints, maxSP0, caster.MaxSpellPoints)
			}
			return
		}
	}
	t.Fatal("intellect barrel never granted its bonus in 200 runs")
}

// Barrels swap art when spent: closed while full, open once emptied.
func TestBarrelSpriteSwapsWhenSpent(t *testing.T) {
	g := crateTestGame(t)
	barrel := spawnCrate(t, g, "barrel_green", g.camera.X+64, g.camera.Y)
	if got := npcSpriteName(barrel); got != "barrel_green" {
		t.Fatalf("full barrel sprite = %q, want barrel_green", got)
	}
	g.useLootCrate(barrel)
	if got := npcSpriteName(barrel); got != "barrel_green_open" {
		t.Fatalf("spent barrel sprite = %q, want barrel_green_open", got)
	}
}

// The box pile stays standing and interactable-but-inert after looting: not
// hidden (no hide_when_visited), still focusable, second search reports empty.
func TestBoxPileStaysVisibleButInert(t *testing.T) {
	sawLoot, sawNothing := false, false
	for i := 0; i < 200 && (!sawLoot || !sawNothing); i++ {
		g := crateTestGame(t)
		boxes := spawnCrate(t, g, "pile_of_old_boxes", g.camera.X+64, g.camera.Y)
		if boxes.HideWhenVisited {
			t.Fatal("box pile must NOT hide when visited - it stays as scenery")
		}
		inv0 := len(g.party.Inventory)
		g.useLootCrate(boxes)
		if len(g.party.Inventory) > inv0 {
			sawLoot = true
		} else {
			sawNothing = true
		}
		if !boxes.Visited {
			t.Fatal("box pile must be consumed")
		}
		// Still a valid focus target (visible scenery), but a re-search is inert.
		inv1 := len(g.party.Inventory)
		g.useLootCrate(boxes)
		if len(g.party.Inventory) != inv1 {
			t.Fatal("an emptied box pile must stay empty")
		}
	}
	if !sawLoot || !sawNothing {
		t.Fatalf("50/50 box pile never produced both outcomes in 200 runs (loot=%v nothing=%v)", sawLoot, sawNothing)
	}
}

// Permanent bonuses survive save/load and keep feeding effective stats.
func TestPermanentBonusesSurviveSaveLoad(t *testing.T) {
	m := &character.MMCharacter{Name: "Test", Might: 10}
	m.PermanentBonuses.Might = 2
	m.PermanentBonuses.Speed = 1
	eff := m.GetEffectiveMight()

	restored := restoreCharacterSave(buildCharacterSave(m))
	if restored.PermanentBonuses != m.PermanentBonuses {
		t.Fatalf("permanent bonuses lost in save round-trip: %+v", restored.PermanentBonuses)
	}
	if restored.GetEffectiveMight() != eff {
		t.Fatalf("restored effective might = %d, want %d", restored.GetEffectiveMight(), eff)
	}
}


// A crate cannot be opened mid-fight: the Space focus is combat-blocked, but a
// mouse click calls useLootCrate directly - the lockout must fire BEFORE the
// crate is consumed, or the blocked attempt would waste it.
func TestCrateBlockedDuringCombatWithoutWasting(t *testing.T) {
	g := crateTestGame(t)
	fire := spawnCrate(t, g, "campfire", g.camera.X+64, g.camera.Y)
	foe := monster.NewMonster3DFromConfig(g.camera.X+128, g.camera.Y, "goblin", g.config)
	foe.IsEngagingPlayer = true
	g.world.Monsters = []*monster.Monster3D{foe}
	if !g.partyInCombat() {
		t.Fatal("setup: engaging goblin next to the party must mean combat")
	}

	hurt := g.party.Members[0]
	hurt.HitPoints = 1
	gold0 := g.party.Gold
	g.useLootCrate(fire)

	if fire.Visited {
		t.Fatal("a combat-blocked attempt must NOT consume the crate")
	}
	if hurt.HitPoints != 1 || g.party.Gold != gold0 {
		t.Fatal("a combat-blocked attempt must grant nothing")
	}

	// Fight over -> the campfire still works in full.
	foe.HitPoints = 0
	g.useLootCrate(fire)
	if !fire.Visited || g.party.Gold != gold0+500 || hurt.HitPoints != hurt.MaxHitPoints {
		t.Fatal("after combat the untouched campfire must work normally")
	}
}

package game

import (
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// The 2026-07-14 roadside props: a campfire (one-time free rest + gold cache),
// stat barrels (a chance of a permanent green bonus for the SELECTED
// character, sprite swaps closed -> open when spent), and a box pile that
// stays standing after looting.

// campfireGold is what one campfire pays: its single always-winning gold
// source in each of its rolls.
func campfireGold(t *testing.T) int {
	t.Helper()
	crate := config.GetCrateConfig("campfire")
	if crate == nil || len(crate.RollSources) != 1 || crate.RollSources[0].Pool != "gold" || crate.RollSources[0].Weight != 100 {
		t.Fatalf("fixture: campfire must pay one certain gold source: %+v", crate)
	}
	return crate.Rolls * crate.RollSources[0].Amount
}

// barrelBonus returns a stat barrel's authored bonus; the statistical tests
// need a partial chance so both outcomes can occur.
func barrelBonus(t *testing.T, key string) int {
	t.Helper()
	crate := config.GetCrateConfig(key)
	if crate == nil || crate.BonusAmount <= 0 || crate.BonusChancePct <= 0 || crate.BonusChancePct >= 100 {
		t.Fatalf("fixture: %s must grant a bonus by partial chance: %+v", key, crate)
	}
	return crate.BonusAmount
}

func TestCampfireFreeRestAndGoldOnce(t *testing.T) {
	g := crateTestGame(t)
	fire := spawnCrate(t, g, "campfire", g.camera.X+64, g.camera.Y)
	gold := campfireGold(t)

	hurt := g.party.Members[0]
	hurt.HitPoints = 1
	hurt.SpellPoints = 0
	gold0 := g.party.Gold

	g.useLootCrate(fire)

	if hurt.HitPoints != hurt.MaxHitPoints || hurt.SpellPoints != hurt.MaxSpellPoints {
		t.Fatalf("campfire must fully rest the party (HP %d/%d SP %d/%d)",
			hurt.HitPoints, hurt.MaxHitPoints, hurt.SpellPoints, hurt.MaxSpellPoints)
	}
	if g.party.Gold != gold0+gold {
		t.Fatalf("campfire gold = %d, want +%d", g.party.Gold-gold0, gold)
	}
	if !fire.Visited {
		t.Fatal("campfire must be consumed")
	}

	// ONE time: a second use neither rests nor pays.
	hurt.HitPoints = 1
	g.useLootCrate(fire)
	if hurt.HitPoints != 1 || g.party.Gold != gold0+gold {
		t.Fatal("a spent campfire must do nothing")
	}
}

// Statistical run over the authored partial-chance barrel: both outcomes must
// occur, an empty barrel changes nothing, and every bonus is a PERMANENT
// effective-stat gain for the SELECTED character only (green delta - base stat
// untouched).
func TestStatBarrelPermanentBonusAndEmptyChance(t *testing.T) {
	bonuses, empties := 0, 0
	for i := 0; i < 200 && (bonuses == 0 || empties == 0); i++ {
		g := crateTestGame(t)
		bonus := barrelBonus(t, "barrel_red")
		barrel := spawnCrate(t, g, "barrel_red", g.camera.X+64, g.camera.Y)
		m := g.party.Members[0]
		base := m.Might
		effBefore := m.GetEffectiveMight()

		g.useLootCrate(barrel)

		if m.Might != base {
			t.Fatal("a stat barrel must never write the BASE stat")
		}
		switch m.GetEffectiveMight() {
		case effBefore + bonus:
			bonuses++
			// Only the SELECTED character drinks - the rest get nothing.
			for i, mem := range g.party.Members {
				want := 0
				if i == g.selectedChar {
					want = bonus
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
		t.Fatalf("partial-chance barrel never produced both outcomes in 200 runs (bonus=%d empty=%d)", bonuses, empties)
	}
}

// The intellect barrel must grow MaxSP immediately (max pools derive from
// effective stats).
func TestIntellectBarrelGrowsSpellPool(t *testing.T) {
	for i := 0; i < 200; i++ {
		g := crateTestGame(t)
		bonus := barrelBonus(t, "barrel_blue")
		barrel := spawnCrate(t, g, "barrel_blue", g.camera.X+64, g.camera.Y)
		caster := g.party.Members[0]
		maxSP0 := caster.MaxSpellPoints
		sp0 := caster.SpellPoints
		g.useLootCrate(barrel)
		if caster.PermanentBonuses.Intellect == bonus {
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
		t.Fatalf("box pile never produced both outcomes in 200 runs (loot=%v nothing=%v)", sawLoot, sawNothing)
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
	gold := campfireGold(t)
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
	if !fire.Visited || g.party.Gold != gold0+gold || hurt.HitPoints != hurt.MaxHitPoints {
		t.Fatal("after combat the untouched campfire must work normally")
	}
}

// Pure summons ignore the attempted party hit entirely. Bound undead remain
// hittable former enemies, but their generic combat flags must not turn a
// controlled ally into combat for crate interaction.
func TestCrateAllowsBoundAllyAfterPartyHit(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		control func(*MMGame, *monster.Monster3D)
		wantHit bool
	}{
		{
			name: "card ally",
			key:  "masked_huntress",
			control: func(_ *MMGame, ally *monster.Monster3D) {
				markCardAlly(ally)
			},
		},
		{
			name:    "bound undead",
			key:     "skeleton",
			wantHit: true,
			control: func(g *MMGame, ally *monster.Monster3D) {
				g.combat.applyBindUndead(ally, 60, "Bind Undead")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := crateTestGame(t)
			fire := spawnCrate(t, g, "campfire", g.camera.X+64, g.camera.Y)
			ally := monster.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, tt.key, g.config)
			ally.MaxHitPoints, ally.HitPoints = 1000, 1000
			ally.ArmorClass, ally.PerfectDodge = 0, 0
			tt.control(g, ally)
			g.world.Monsters = []*monster.Monster3D{ally}

			// Exercise the real party damage hub rather than setting the flags by hand.
			hpBefore := ally.HitPoints
			g.combat.ApplyDamageToMonster(ally, 1, "Iron Sword", false)
			if gotHit := ally.HitPoints < hpBefore; gotHit != tt.wantHit {
				t.Fatalf("party hit landed=%v, want %v (HP %d -> %d)", gotHit, tt.wantHit, hpBefore, ally.HitPoints)
			}
			if ally.WasAttacked != tt.wantHit {
				t.Fatalf("WasAttacked=%v, want %v", ally.WasAttacked, tt.wantHit)
			}
			if g.partyInCombat() {
				t.Fatal("a bound ally must not count as combat with the party")
			}

			g.useLootCrate(fire)
			if !fire.Visited {
				t.Fatal("a crate must remain usable after hitting a bound ally")
			}
		})
	}
}

// A hostile monster can be actively fighting a summoned ally beside the party.
// IsEngagingPlayer is a legacy generic combat marker in that case, so crate
// lockout must consult the current target policy rather than the raw flag.
func TestCrateAllowsPartyInteractionDuringSummonCrossfire(t *testing.T) {
	g := crateTestGame(t)
	fire := spawnCrate(t, g, "campfire", g.camera.X+64, g.camera.Y)
	enemy := monster.NewMonster3DFromConfig(g.camera.X+128, g.camera.Y, "goblin", g.config)
	ally := monster.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, "masked_huntress", g.config)
	markCardAlly(ally)
	enemy.AIFoe = ally
	enemy.IsEngagingPlayer = true
	g.world.Monsters = []*monster.Monster3D{enemy, ally}

	if g.partyInCombat() {
		t.Fatal("a monster targeting a summon must not lock party interaction")
	}
	g.useLootCrate(fire)
	if !fire.Visited {
		t.Fatal("a crate must remain usable while monsters fight a summon")
	}
}

// Charm is deliberately different from Bind Undead: a party hit breaks the
// charm, returns the monster to hostility, and must still lock the crate.
func TestCrateBlockedAfterPartyBreaksCharm(t *testing.T) {
	g := crateTestGame(t)
	fire := spawnCrate(t, g, "campfire", g.camera.X+64, g.camera.Y)
	charmed := monster.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, "goblin", g.config)
	charmed.MaxHitPoints, charmed.HitPoints = 1000, 1000
	charmed.ArmorClass, charmed.PerfectDodge = 0, 0
	g.world.Monsters = []*monster.Monster3D{charmed}
	g.combat.applyPacify(charmed, 60, "Charm")

	g.combat.ApplyDamageToMonster(charmed, 1, "Iron Sword", false)
	if charmed.Pacified || !g.partyInCombat() {
		t.Fatalf("a hit charmed monster must become a hostile combatant (pacified=%v combat=%v)",
			charmed.Pacified, g.partyInCombat())
	}

	g.useLootCrate(fire)
	if fire.Visited {
		t.Fatal("a crate must stay locked after the party breaks Charm")
	}
}

// These are presentation states of the existing one-shot interaction, not a
// second inventory or reward lifecycle. Loading must settle an in-flight pose.
func TestPropUsePresentationLifecycle(t *testing.T) {
	g := crateTestGame(t)
	for _, key := range []string{"pile_of_old_boxes", "campfire"} {
		t.Run(key, func(t *testing.T) {
			n := spawnCrate(t, g, key, g.camera.X+64, g.camera.Y)
			g.frameCount = 20
			g.useLootCrate(n)
			if !n.Visited || n.PropUseStarted != 21 || g.propUseProgress(n) != 0 {
				t.Fatal("successful use must start the presentation exactly once")
			}
			started := n.PropUseStarted
			g.frameCount += int64(float64(g.config.GetTPS()) * n.PropModel.UseSeconds / 2)
			if p := g.propUseProgress(n); p <= 0 || p >= 1 {
				t.Fatalf("mid-animation progress = %g", p)
			}
			g.useLootCrate(n)
			if n.PropUseStarted != started {
				t.Fatal("empty interaction restarted the animation")
			}
			wm := &world.WorldManager{LoadedMaps: map[string]*world.World3D{"test": g.world}}
			g.restoreSavedNPCs(wm, &GameSave{NPCStates: []NPCSave{{MapKey: "test", Name: n.Name, X: n.X, Y: n.Y, Visited: true}}})
			if !n.Visited || g.propUseProgress(n) != 1 {
				t.Fatal("loaded used prop must show the settled pose")
			}
			n.Visited = false
			if g.propUseProgress(n) != 0 {
				t.Fatal("world reset must restore the closed/active appearance")
			}
		})
	}
}

func TestPropModelPickingRespectsWallClip(t *testing.T) {
	g := crateTestGame(t)
	setTestWorldManager(t, nil)
	g.renderHelper = NewRenderingHelper(g)
	g.camera.Angle = .6
	g.camera.ViewDist = 1024
	g.depthBuffer = make([]float64, g.worldWidth())
	g.wallTopBuffer = make([]int, g.worldWidth())
	for _, key := range []string{"pile_of_old_boxes", "campfire"} {
		n := spawnCrate(t, g, key, g.camera.X+90*math.Cos(.6), g.camera.Y+90*math.Sin(.6))
		for _, used := range []bool{false, true} {
			n.Visited = used
			n.PropUseStarted = 0
			for i := range g.depthBuffer {
				g.depthBuffer[i] = 1024
				g.wallTopBuffer[i] = 0
			}
			hx, hy := -1, -1
			// Find an actual solid surface through the same public NPC picking path
			// used by hover and mouse presses, then independently change its cover.
			for y := g.worldHeight() / 3; y < g.worldHeight() && hx < 0; y += 6 {
				for x := g.worldWidth() / 3; x < g.worldWidth()*2/3; x += 6 {
					if g.npcScreenHitTest(n, n.X, n.Y, x, y) {
						hx, hy = x, y
						break
					}
				}
			}
			if hx < 0 {
				t.Fatalf("%s used=%v has no selectable solid surface", key, used)
			}
			g.depthBuffer[hx] = 32
			if g.npcScreenHitTest(n, n.X, n.Y, hx, hy) {
				t.Fatalf("%s selectable through a foreground wall", key)
			}
			g.wallTopBuffer[hx] = hy + 1
			if !g.npcScreenHitTest(n, n.X, n.Y, hx, hy) {
				t.Fatalf("%s lost its visible surface above a low wall", key)
			}
		}
	}
}

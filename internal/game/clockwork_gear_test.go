package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
)

// clockworkHero is a hero stripped of skills, cards and gear, so a test sees
// one piece of Clock Tower gear at a time.
func clockworkHero(t *testing.T, tb bool) (*CombatSystem, *character.MMCharacter) {
	t.Helper()
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	cs.game.turnBasedMode = tb
	cs.game.cardSlots = [MaxCardSlots]cardSlot{}
	hero := cs.game.party.Members[cs.game.selectedChar]
	isolateTrueDamageMember(hero, 0)
	return cs, hero
}

// wear equips catalog items by key, each in its own slot, bypassing class gates.
func wear(hero *character.MMCharacter, keys ...string) {
	for _, key := range keys {
		item := items.CreateItemFromYAML(key)
		hero.Equipment[item.PreferredSlot(items.SlotArmor)] = item
	}
}

// spawnFoe puts an engaged goblin two tiles from the party.
func spawnFoe(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
	t.Helper()
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	ts := float64(g.config.GetTileSize())
	m := spawnMonsterAtTile(g, "goblin", TileIndex(g.camera.X, ts)+2, TileIndex(g.camera.Y, ts), ts)
	if !g.partyInCombat() {
		t.Fatal("fixture: the party is not fighting")
	}
	return m
}

// startFight spawns a foe and lets the game see the fight start, as its frame
// loop does.
func startFight(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
	t.Helper()
	m := spawnFoe(t, g)
	g.updateFightOpeners()
	return m
}

// passRounds advances the party clocks the way each mode does: frame by frame
// in real time, a party turn per round in turn-based.
func passRounds(g *MMGame, rounds int) {
	for range rounds {
		if g.turnBasedMode {
			g.startPartyTurn()
		} else {
			passFrames(g, g.combatRoundFrames())
		}
	}
}

// combatRoundFrames is one round of RT frames.
func (g *MMGame) combatRoundFrames() int {
	return turnBasedPeriodicEffectFrames(g.config.GetTPS())
}

// passFrames runs the party clocks for frames of real time, one frame a tick.
func passFrames(g *MMGame, frames int) {
	for range frames {
		g.tickPartyClocks(false)
	}
}

// spendFrames runs a clock tick for frames of real time, one frame at a time.
func spendFrames(frames int, tick func(turn bool)) {
	for range frames {
		tick(false)
	}
}

// framesClock is a running clock with frames of real time left.
func framesClock(frames int) status.DualClock {
	return status.FromFrames(frames, config.GlobalConfig.GetTPS())
}

// lastTickClock runs out on the next tick of either mode.
func lastTickClock() status.DualClock { return status.DualClock{Frames: 1, Turns: 1, Rate: 1} }

// roundClock is a running clock of one round.
func roundClock() status.DualClock {
	return framesClock(config.RoundFrames(1, config.GlobalConfig.GetTPS()))
}

// The Mainspring Pike strikes x5 into a target still at 90%+ of its HP, and
// the Minute Hand's twelfth hit on one target strikes the hour for x3 and
// starts the count again; each target keeps its own count. A graze through a
// dodge is no landed hit: it neither strikes the hour nor counts toward it.
func TestClockTowerWeaponsReadTheTargetsState(t *testing.T) {
	cs, hero := clockworkHero(t, false)
	hit := func(weapon string, m *monsterPkg.Monster3D) int {
		before := m.HitPoints
		cs.ApplyDamageToMonster(m, 100, testWeapon(weapon), false)
		return before - m.HitPoints
	}
	for _, tc := range []struct {
		hpPct, want int
	}{{100, 500}, {90, 500}, {89, 100}} {
		m := mkTestMonster("Target", 100000)
		m.HitPoints = m.MaxHitPoints * tc.hpPct / 100
		if got := hit("Mainspring Pike", m); got != tc.want {
			t.Errorf("Mainspring Pike on a target at %d%% HP dealt %d, want %d", tc.hpPct, got, tc.want)
		}
	}

	clock, other := mkTestMonster("Clock", 100000), mkTestMonster("Other", 100000)
	for n := 1; n <= 13; n++ {
		want := 100
		if n == 12 {
			want = 300
		}
		if got := hit("Minute Hand", clock); got != want {
			t.Fatalf("Minute Hand hit %d dealt %d, want %d", n, got, want)
		}
	}
	if got := hit("Minute Hand", other); got != 100 {
		t.Fatalf("another target's first hit dealt %d: marks belong to their target", got)
	}

	hero.Skills[character.SkillDagger] = &character.Skill{Mastery: character.MasteryGrandMaster}
	marked, unmarked := mkTestMonster("Marked", 100000), mkTestMonster("Unmarked", 100000)
	marked.HourMarks = 11
	marked.PerfectDodge, unmarked.PerfectDodge = 100, 100
	if got, base := hit("Minute Hand", marked), hit("Minute Hand", unmarked); base == 0 || got != base || marked.HourMarks != 11 {
		t.Fatalf("a graze at 11 marks dealt %d (unmarked %d) and left %d marks, want the same damage and 11", got, base, marked.HourMarks)
	}
}

// A Chime Maul hit rings again on its target one round later for half its
// damage, in both modes; a target that died meanwhile takes nothing, and a
// map change silences the bell. A boss's disintegrate sear is not echoed.
func TestChimeMaulTollRingsARoundLater(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			cs, _ := clockworkHero(t, tb)
			g := cs.game
			bell, doomed := mkTestMonster("Bell", 10000), mkTestMonster("Doomed", 10000)
			g.world.Monsters = []*monsterPkg.Monster3D{bell, doomed}
			cs.ApplyDamageToMonster(bell, 100, testWeapon("Chime Maul"), false)
			cs.ApplyDamageToMonster(doomed, 100, testWeapon("Chime Maul"), false)
			doomed.HitPoints = 0
			if !tb {
				passFrames(g, g.combatRoundFrames()-1)
				if taken := 10000 - bell.HitPoints; taken != 100 {
					t.Fatalf("the toll rang early: %d taken", taken)
				}
			}
			passRounds(g, 1)
			if taken := 10000 - bell.HitPoints; taken != 150 {
				t.Fatalf("after a round the bell took %d, want 100 + a 50 toll", taken)
			}
			if doomed.HitPoints != 0 {
				t.Fatal("a dead target took the toll")
			}

			cs.ApplyDamageToMonster(bell, 100, testWeapon("Chime Maul"), false)
			g.clearTransientCombatState()
			passRounds(g, 1)
			if taken := 10000 - bell.HitPoints; taken != 250 {
				t.Fatalf("a toll rang across a map change: %d taken, want 250", taken)
			}
		})
	}
	// A designation mark that upgrades the hit to a critical: the toll echoes
	// the critical the hit landed, with no second roll.
	t.Run("designated critical", func(t *testing.T) {
		cs, hero := clockworkHero(t, false)
		g := cs.game
		hero.Skills[character.SkillDesignateTarget] = &character.Skill{}
		g.combat.designationRoll = func(int) int { return 0 }
		bell := mkTestMonster("Bell", 10000)
		bell.ID = "bell" // a mark names its target by ID
		g.world.Monsters = []*monsterPkg.Monster3D{bell}
		g.designateTarget(hero, bell)
		cs.ApplyDamageToMonster(bell, 100, testWeapon("Chime Maul"), false)
		primary := 10000 - bell.HitPoints
		if primary <= 100 {
			t.Fatalf("fixture: the marked hit dealt %d, want a critical over 100", primary)
		}
		passRounds(g, 1)
		if echo := 10000 - bell.HitPoints - primary; echo != primary/2 {
			t.Fatalf("the toll rang %d after a %d critical, want %d", echo, primary, primary/2)
		}
	})
	t.Run("boss sear", func(t *testing.T) {
		cs, _ := clockworkHero(t, false)
		g := cs.game
		def, _ := config.GetWeaponDefinition("chime_maul")
		old := def.DisintegrateChance
		t.Cleanup(func() { def.DisintegrateChance = old })
		def.DisintegrateChance = 1
		boss := mkTestMonster("Boss", 10000)
		boss.Boss = true
		g.world.Monsters = []*monsterPkg.Monster3D{boss}
		cs.ApplyDamageToMonster(boss, 100, testWeapon("Chime Maul"), false)
		sear := 10000 * config.InstakillSearMaxHPPct / 100
		if taken := 10000 - boss.HitPoints; taken != 100+sear {
			t.Fatalf("the seared hit took %d, want 100 + %d", taken, sear)
		}
		passRounds(g, 1)
		if taken := 10000 - boss.HitPoints; taken != 100+sear+50 {
			t.Fatalf("after the toll the boss took %d, want %d: the toll echoes the hit, not the sear", taken, 100+sear+50)
		}
	})
}

// A Cogfang hit jams the target once per attack it makes: real time pushes
// its next attack back by the jam; turn-based takes the same share of a turn's
// swings from its swing bank, so three jams on a one-swing goblin cost one
// attack, which arms its real-time cooldown like any spent attack (the bank's
// save is in the save-load test with the other TB carries).
func TestCogfangJamDelaysTheNextAttack(t *testing.T) {
	cs, _ := clockworkHero(t, false)
	g := cs.game
	jam := lookupWeaponConfig(testWeapon("Cogfang Blade")).JamSeconds * g.config.GetTPS()
	m := mkTestMonster("Gears", 100000)
	cs.ApplyDamageToMonster(m, 10, testWeapon("Cogfang Blade"), false)
	cs.ApplyDamageToMonster(m, 10, testWeapon("Cogfang Blade"), false)
	if m.AttackCDFrames != jam {
		t.Fatalf("two hits in one attack cycle delayed the attack %d frames, want one jam of %d", m.AttackCDFrames, jam)
	}

	g.turnBasedMode = true
	foe := startFight(t, g)
	foe.HitPoints, foe.MaxHitPoints = 100000, 100000
	foe.AttackCooldownMultiplier, foe.PerfectDodge = 1, 0 // one swing an action; every hit lands
	ts := float64(g.config.GetTileSize())
	foe.X = g.camera.X + ts // adjacent: its attack reaches the party
	g.collisionSystem.UpdateEntity(foe.ID, foe.X, foe.Y)
	lost := 0
	for cycle := 1; cycle <= 3; cycle++ {
		cs.ApplyDamageToMonster(foe, 10, testWeapon("Cogfang Blade"), false)
		cs.ApplyDamageToMonster(foe, 10, testWeapon("Cogfang Blade"), false) // the same cycle jams once
		foe.AttackCDFrames = 0
		before := countCombatLog(g, "attack is lost")
		if !cs.commitMonsterAttack(foe, monsterAttackDestination{}, monsterAttackTurn) {
			t.Fatal("fixture: the goblin could not attack")
		}
		if countCombatLog(g, "attack is lost") > before {
			lost++
			if foe.AttackCDFrames == 0 {
				t.Fatal("a lost attack armed no real-time cooldown")
			}
		}
	}
	if lost != 1 {
		t.Fatalf("three jams cost %d attacks of a one-swing goblin, want 1", lost)
	}
}

// The Escapement Staff, in either hand, lengthens every buff its wielder casts
// by half, to whole rounds, and leaves offensive durations alone; the spell's
// card says so.
func TestEscapementStaffLengthensBuffs(t *testing.T) {
	cs, caster := clockworkHero(t, false)
	checked := 0
	for _, key := range slices.Sorted(func(yield func(string) bool) {
		for k := range config.GlobalSpells.Spells {
			if !yield(k) {
				return
			}
		}
	}) {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			continue
		}
		delete(caster.Equipment, items.SlotMainHand)
		base := cs.CalculateSpellDurationSeconds(def.ID, caster)
		if base <= 0 {
			continue
		}
		want := base
		if !def.IsOffensive() {
			rounds := (base*150/100 + config.CombatRoundSeconds - 1) / config.CombatRoundSeconds
			want = rounds * config.CombatRoundSeconds
		}
		// Either hand counts.
		for _, slot := range []items.EquipSlot{items.SlotMainHand, items.SlotOffHand} {
			delete(caster.Equipment, items.SlotMainHand)
			delete(caster.Equipment, items.SlotOffHand)
			caster.Equipment[slot] = items.CreateWeaponFromYAML("escapement_staff")
			if got := cs.CalculateSpellDurationSeconds(def.ID, caster); got != want {
				t.Errorf("%s (offensive=%v, slot %v): %ds with the staff, want %ds from %ds", key, def.IsOffensive(), slot, got, want, base)
			}
			// The caster's card states the duration the cast gives.
			if def.Mechanic == nil && !def.IsOffensive() {
				if card := GetSpellTooltipRows(def.ID, caster, cs, true).String(); !strings.Contains(card, config.DurationText(want)) {
					t.Errorf("%s card with the staff lacks %q:\n%s", key, config.DurationText(want), card)
				}
			}
		}
		delete(caster.Equipment, items.SlotOffHand)
		checked++
	}
	if checked == 0 {
		t.Fatal("no timed spells checked")
	}
}

// The Clockwork Pistol winds a spring per full round it spends unfired, up to
// three; a shot fires one extra bullet per spring and starts winding again. A
// turn-based shot spends its round. Springs, and the rest after a shot,
// survive a save.
func TestClockworkPistolWindsSprings(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			g, ch := rareClassGame(t, character.ClassArcher, tb)
			g.selectedChar = 0
			isolateTrueDamageMember(ch, 0)
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("clockwork_pistol")
			springs := func() int { return ch.Equipment[items.SlotMainHand].Signature.Charges }
			fire := func() int {
				g.arrows = nil
				g.combat.createArrowAttack(100, items.SlotMainHand, "")
				return len(g.arrows)
			}
			passRounds(g, 2)
			if springs() != 2 {
				t.Fatalf("two unfired rounds wound %d springs", springs())
			}
			passRounds(g, 4)
			if springs() != 3 {
				t.Fatalf("springs = %d, want the cap of 3", springs())
			}
			ch = restoreCharacterSave(buildCharacterSave(ch))
			g.party.Members[0] = ch
			if springs() != 3 {
				t.Fatalf("a save kept %d springs", springs())
			}
			if card := GetItemTooltipRows(ch.Equipment[items.SlotMainHand], ch, g.combat, true).String(); !strings.Contains(card, "Springs wound: 3 / 3") {
				t.Fatalf("the pistol's card does not count its springs:\n%s", card)
			}
			if bullets := fire(); bullets != 4 || springs() != 0 {
				t.Fatalf("a wound shot fired %d bullets leaving %d springs, want 4 and 0", bullets, springs())
			}
			ch = restoreCharacterSave(buildCharacterSave(ch))
			g.party.Members[0] = ch
			if !tb {
				passFrames(g, g.combatRoundFrames()-1)
				if springs() != 0 {
					t.Fatal("a spring wound in less than a round")
				}
			}
			passRounds(g, 1)
			want := 1
			if tb {
				want = 0 // the round of the shot winds nothing
			}
			if springs() != want {
				t.Fatalf("springs after the shot's round = %d, want %d", springs(), want)
			}
			if bullets := fire(); bullets != 1+want {
				t.Fatalf("fired %d bullets, want %d", bullets, 1+want)
			}
		})
	}
}

// The Chrono Cape, in a fight, gives back at a round's end the HP lost since
// the round began, then waits out its cooldown. Out of a fight, or for a hero
// who cannot act, nothing is rewound. The cooldown survives a save.
func TestChronoCapeRewindsARound(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			cs, hero := clockworkHero(t, tb)
			g := cs.game
			wear(hero, "chrono_cape")
			hero.MaxHitPoints, hero.HitPoints = 1000, 1000
			passRounds(g, 1)
			hero.HitPoints = 700 // out of a fight
			passRounds(g, 1)
			if hero.HitPoints != 700 {
				t.Fatal("the cape rewound outside a fight")
			}

			startFight(t, g)
			hero.HitPoints = 1000
			passRounds(g, 1)
			hero.HitPoints = 700
			passRounds(g, 1)
			if hero.HitPoints != 1000 {
				t.Fatalf("HP after the rewind = %d, want 1000", hero.HitPoints)
			}
			hero = restoreCharacterSave(buildCharacterSave(hero))
			g.party.Members[g.selectedChar] = hero
			hero.HitPoints = 600
			passRounds(g, 1)
			if hero.HitPoints != 600 {
				t.Fatal("the cape rewound inside its cooldown, or a save reset it")
			}
			passRounds(g, config.CombatRounds(hero.RewindCooldownSeconds()))
			hero.HitPoints = 400
			hero.AddCondition(character.ConditionUnconscious)
			passRounds(g, 1)
			if hero.HitPoints != 400 {
				t.Fatal("the cape rewound a hero who cannot act")
			}
		})
	}
}

// Chainwork: in a fight, a round after the wearer heals, a random offensive
// spell the wearer knows is cast for free. Out of a fight, or with no
// offensive spell known, the heal swings back nothing.
func TestChainworkPendulumCastsAfterAHeal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tb       bool
		fight    bool
		knows    string
		wantCast bool
	}{
		{"in a fight", false, true, "firebolt", true},
		{"in a TB fight", true, true, "firebolt", true},
		{"out of a fight", false, false, "firebolt", false},
		{"nothing offensive known", false, true, "", false},
		{"only an open-sky spell, underground", false, true, "earthquake", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs, hero := clockworkHero(t, tc.tb)
			g := cs.game
			wear(hero, "chainwork_helm", "chainwork_hauberk", "chainwork_pauldrons", "chainwork_gauntlets", "chainwork_greaves")
			if !hero.SetHealFollowUpSpell() {
				t.Fatal("fixture: the Chainwork set is not complete")
			}
			hero.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
			if tc.knows != "" && !hero.LearnSpell(spells.SpellID(tc.knows)) {
				t.Fatalf("fixture: could not learn %s", tc.knows)
			}
			if g.currentMapHasOpenSky() {
				t.Fatal("fixture: the test map has an open sky")
			}
			if tc.fight {
				startFight(t, g)
			}
			heal, _ := spells.GetSpellDefinitionByID("heal")
			hero.SpellPoints = 100
			cs.executeSpellCast(spellCastRequest{ID: heal.ID, Definition: heal, Caster: hero}, func() spellCastOutcome { return castCommitted })
			g.magicProjectiles = nil
			passRounds(g, 1)
			cast := countCombatLog(g, "pendulum swings back") > 0
			if cast != tc.wantCast || hero.SpellPoints != 100 {
				t.Fatalf("free spell cast = %v (want %v), SP %d (want 100)", cast, tc.wantCast, hero.SpellPoints)
			}
		})
	}
}

// Clockplate: an enemy hit on the wearer loses 20% while the party has held
// its ground this round; a move in the round, or damage with no attacker,
// lands whole.
func TestClockplateHoldsGround(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			cs, hero := clockworkHero(t, tb)
			g := cs.game
			wear(hero, "clockplate_helm", "clockplate_cuirass", "clockplate_pauldrons", "clockplate_gauntlets", "clockplate_greaves")
			source := mkTestMonster("Hammer", 100)
			if got := cs.resolveHeroHitDamage(source, hero, 100); got != 80 {
				t.Fatalf("held ground: %d, want 80", got)
			}
			if got := cs.resolveHeroHitDamage(nil, hero, 100); got != 100 {
				t.Fatalf("damage without an attacker was reduced to %d", got)
			}
			ts := g.config.GetTileSize()
			g.camera.X += ts
			g.partyMoved(g.camera.X-ts, g.camera.Y)
			if got := cs.resolveHeroHitDamage(source, hero, 100); got != 100 {
				t.Fatalf("after a move: %d, want 100", got)
			}
			passRounds(g, 1)
			if got := cs.resolveHeroHitDamage(source, hero, 100); got != 80 {
				t.Fatalf("a round later: %d, want 80", got)
			}
		})
	}
}

// Cogleather: when a fight starts the wearer acts first - in real time the
// first weapon attack has no recovery, in turn-based an extra action now
// (mid party turn) or at the next party turn. Leaving the fight drops it;
// stepping straight back in is no new fight, a round out of it is.
func TestCogleatherQuickDraw(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tb          bool
		currentTurn int
	}{{"real time", false, 0}, {"party turn", true, 0}, {"enemy turn", true, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			cs, hero := clockworkHero(t, tc.tb)
			g := cs.game
			wear(hero, "cogleather_helm", "cogleather_jacket", "cogleather_pauldrons", "cogleather_gloves", "cogleather_boots")
			hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
			spawnFoe(t, g)
			control := 0
			if tc.tb {
				g.startPartyTurn() // the same party turn without an opener
				control = hero.ActionsRemaining
			}
			g.currentTurn = tc.currentTurn
			hero.ActionsRemaining = 1
			g.updateFightOpeners()
			switch {
			case !tc.tb:
				ih := &InputHandler{game: g}
				ih.commitRTWeaponAttack(rtActSmart, hero)
				if hero.RTCooldown != 0 {
					t.Fatalf("the first attack recovers %d frames, want none", hero.RTCooldown)
				}
				ih.commitRTWeaponAttack(rtActSmart, hero)
				if hero.RTCooldown == 0 {
					t.Fatal("the second attack has no recovery either")
				}
			case tc.currentTurn == 0:
				if hero.ActionsRemaining != 2 {
					t.Fatalf("actions now = %d, want 2", hero.ActionsRemaining)
				}
			default:
				g.startPartyTurn()
				if hero.ActionsRemaining != control+1 {
					t.Fatalf("actions next turn = %d, want one over the %d without the opener", hero.ActionsRemaining, control)
				}
			}
		})
	}
	cs, hero := clockworkHero(t, false)
	g := cs.game
	wear(hero, "cogleather_helm", "cogleather_jacket", "cogleather_pauldrons", "cogleather_gloves", "cogleather_boots")
	startFight(t, g)
	g.updateFightOpeners()
	g.world.Monsters = nil
	g.updateFightOpeners()
	if hero.Clockwork.QuickDraw {
		t.Fatal("an unspent opener outlived the fight")
	}
	for _, rest := range []int{0, 1} {
		g.world.Monsters = nil
		g.updateFightOpeners()
		passRounds(g, rest)
		startFight(t, g)
		g.updateFightOpeners()
		if hero.Clockwork.QuickDraw != (rest == 1) {
			t.Fatalf("after %d rounds out of the fight the opener is armed: %v", rest, hero.Clockwork.QuickDraw)
		}
	}
}

// A loaded save bags worn gear that no longer belongs in its slot. The old
// Escapement Mace loads as the Escapement Staff: a hero who can wield a staff
// keeps it in hand; one who cannot finds it in the bag, with a line in the
// log. A stat threshold is checked only when gear is put on: Lanista's Scepter
// stays in the hand of a hero whose Personality has since dropped below it.
func TestFormerMaceLoadsAsTheStaff(t *testing.T) {
	newTestCombatSystemWithConfig(t) // loads the weapon catalog
	if def, key, ok := config.GetWeaponDefinitionByName("Escapement Mace"); !ok || key != "escapement_staff" || def.Category != "staff" {
		t.Fatalf("the old name resolves to %q (%v)", key, ok)
	}
	for _, tc := range []struct {
		name         string
		class        character.CharacterClass
		worn, loaded string
		inHand       bool
	}{
		{"staff wielder", character.ClassSorcerer, "Escapement Mace", "Escapement Staff", true},
		{"no staff skill", character.ClassPaladin, "Escapement Mace", "Escapement Staff", false},
		{"presence fell", character.ClassPaladin, "Lanista's Scepter", "Lanista's Scepter", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			hero := character.CreateCharacter("Holder", tc.class, g.config)
			hero.Personality = 1
			hero.Equipment[items.SlotMainHand] = items.Item{Name: tc.worn, Type: items.ItemWeapon}
			g.restoreSavedParty(&GameSave{Party: PartySave{Members: []CharacterSave{buildCharacterSave(hero)}}})
			loaded := g.party.Members[0]
			held, inHand := loaded.Equipment[items.SlotMainHand]
			if inHand != tc.inHand || inHand && held.Name != tc.loaded {
				t.Fatalf("main hand = %+v (%v), want %s in hand: %v", held.Name, inHand, tc.loaded, tc.inHand)
			}
			inBag := slices.ContainsFunc(loaded.Inventory, func(it items.Item) bool { return it.Name == tc.loaded })
			if inBag == tc.inHand {
				t.Fatalf("%s in the bag = %v", tc.loaded, inBag)
			}
			noticed := slices.ContainsFunc(g.loadNotices, func(s string) bool { return strings.Contains(s, tc.loaded) })
			if noticed == tc.inHand {
				t.Fatalf("load notice = %v: %v", noticed, g.loadNotices)
			}
		})
	}
	// A whole load prints the notice in the loaded game's log, once.
	g, wm, _ := travelFixture(t)
	hero := character.CreateCharacter("Holder", character.ClassPaladin, g.config)
	hero.Equipment[items.SlotMainHand] = items.Item{Name: "Escapement Mace", Type: items.ItemWeapon}
	g.party.Members[0] = hero
	raw, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err := json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if n := countCombatLog(g, "Escapement Staff no longer fits"); n != 1 || len(g.loadNotices) != 0 {
		t.Fatalf("logged the notice %d times, %d left pending", n, len(g.loadNotices))
	}
}

// A spell the world refuses here (Earthquake needs the open sky) is never
// Space's pick: Space falls through to the weapon instead of fizzling.
func TestSmartAttackSkipsASpellTheWorldRefuses(t *testing.T) {
	for _, spell := range []spells.SpellID{"firebolt", "earthquake"} {
		t.Run(string(spell), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			member := cs.game.party.Members[0]
			item, err := spells.CreateSpellItem(spell)
			if err != nil {
				t.Fatal(err)
			}
			member.Equipment[items.SlotQuickAction] = item
			member.SpellPoints, member.MaxSpellPoints = 99, 99
			if cs.game.currentMapHasOpenSky() {
				t.Fatal("fixture: the test map has an open sky")
			}
			if _, picked := cs.smartOffensiveSpell(member); picked != (spell == "firebolt") {
				t.Fatalf("Space picks %s underground = %v", spell, picked)
			}
		})
	}
}

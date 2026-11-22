package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
	"ugataima/internal/world"
)

// Case table: each timed family x RT/initial TB/idle TB/resolved round x
// fresh/serialized remainder. Permanent controls and calendar-owned buffs are
// separate cells, rather than exceptions hidden in the generic timer tests.
func clockSaveFixture(t *testing.T) (*MMGame, *GameLoop, *world.WorldManager) {
	t.Helper()
	g, _ := summonTileWorld(t)
	gl := &GameLoop{game: g}
	g.gameLoop = gl
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
	setTestWorldManager(t, wm)
	t.Chdir("../..")
	return g, gl, wm
}

// heroDoTPerSecond is the expected loss of one DoT tick on a hero of maxHP,
// worked out here from the status row: its percent of max HP, at least its
// minimum.
func heroDoTPerSecond(key string, maxHP int) int {
	s := config.Status(key)
	return max(s.PercentPerSecond*maxHP/100, s.MinimumDamage)
}

func TestStatusClockPartyDoTEntryAndRound(t *testing.T) {
	for _, kind := range []string{"poison", "burn", "both"} {
		t.Run(kind, func(t *testing.T) {
			g, gl, wm := clockSaveFixture(t)
			g.turnBasedMode = false
			c := g.party.Members[0]
			c.Conditions = nil
			c.HitPoints, c.MaxHitPoints = 1000, 1000
			tps := g.config.GetTPS()
			if kind != "burn" {
				c.ApplyPoison(7 * tps)
			}
			if kind != "poison" {
				c.ApplyBurn(7 * tps)
			}
			for range tps / 2 {
				g.updatePartyClocks()
			}
			before := buildCharacterSave(c)
			g.ToggleTurnBasedMode()
			for range tps {
				g.updatePartyClocks()
				gl.updateSpecialEffects()
			}
			after := buildCharacterSave(c)
			if before.PoisonFramesRemaining != after.PoisonFramesRemaining || before.BurnFramesRemaining != after.BurnFramesRemaining || before.HitPoints != after.HitPoints || before.PoisonTickTimer != after.PoisonTickTimer || before.BurnTickTimer != after.BurnTickTimer {
				t.Fatal("TB entry/idle spent DoT time or damage")
			}
			save := auditSaveJSON(t, g.buildSave(wm))
			g.restoreSavedParty(&save)
			c = g.party.Members[0]
			g.startPartyTurn()
			perSecond := 0
			if kind != "burn" {
				perSecond += heroDoTPerSecond("poison", c.MaxHitPoints)
			}
			if kind != "poison" {
				perSecond += heroDoTPerSecond("burn", c.MaxHitPoints)
			}
			if c.HitPoints != before.HitPoints-3*perSecond {
				t.Fatalf("round damage=%d", 1000-c.HitPoints)
			}
			poison, burn := c.DoTTickTimers()
			if (kind != "burn" && (c.PoisonFramesRemaining != 7*tps-tps/2-3*tps || poison != before.PoisonTickTimer)) || (kind != "poison" && (c.BurnFramesRemaining != 7*tps-tps/2-3*tps || burn != before.BurnTickTimer)) {
				t.Fatal("round discarded partial cadence")
			}
			g.turnBasedMode = false
			for range tps / 2 {
				g.updatePartyClocks()
			}
			extra := 0
			if kind != "burn" {
				extra = heroDoTPerSecond("poison", c.MaxHitPoints)
			}
			if c.HitPoints != before.HitPoints-3*perSecond-extra {
				t.Fatal("RT did not resume saved sub-second phase")
			}
		})
	}
}

func TestStatusClockControlsAndTraps(t *testing.T) {
	for _, persist := range []bool{false, true} {
		for _, remaining := range []int{1, 17, 720} {
			t.Run(fmt.Sprintf("saved=%v/frames=%d", persist, remaining), func(t *testing.T) {
				g, gl, wm := clockSaveFixture(t)
				g.turnBasedMode = false
				makeMob := func(key string, x float64) *monster.Monster3D {
					return monster.NewMonster3DFromConfig(x, 400, key, g.config)
				}
				charm, bind, forever := makeMob("wolf", 300), makeMob("skeleton", 500), makeMob("skeleton", 600)
				charm.Pacified, charm.PacifiedClock = true, framesClock(remaining+1)
				bind.Bound, bind.BoundClock = true, framesClock(remaining+1)
				forever.Bound = true
				g.world.Monsters = []*monster.Monster3D{charm, bind, forever}
				g.traps = []PlacedTrap{{Key: "bear_trap", MapKey: "forest", TileX: 1, TileY: 1, X: 96, Y: 96, Owner: g.party.Members[0], Life: framesClock(remaining + 1)}, {Key: "bear_trap", MapKey: "mountains", Life: framesClock(remaining + 1), Owner: g.party.Members[0]}}
				gl.updateSpecialEffects()
				if charm.PacifiedClock.Frames != remaining || bind.BoundClock.Frames != remaining || g.traps[0].Life.Frames != remaining {
					t.Fatal("RT did not spend one frame")
				}
				g.ToggleTurnBasedMode()
				for range 40 {
					gl.updateSpecialEffects()
				}
				if charm.PacifiedClock.Frames != remaining || bind.BoundClock.Frames != remaining || g.traps[0].Life.Frames != remaining {
					t.Fatal("TB initial/idle spent gameplay lifetime")
				}
				if persist {
					saved := auditSaveJSON(t, g.buildSave(wm))
					g.restoreSavedMonsters(wm, &saved)
					g.restoreSavedEffects(&saved)
					for _, m := range g.world.Monsters {
						switch m.ID {
						case charm.ID:
							charm = m
						case bind.ID:
							bind = m
						case forever.ID:
							forever = m
						}
					}
				}
				want := max(0, remaining-g.combatRoundFrames())
				g.startPartyTurn()
				if charm.PacifiedClock.Frames != want || bind.BoundClock.Frames != want || charm.Pacified != (want > 0) || bind.Bound != (want > 0) || !forever.Bound {
					t.Fatal("resolved round control lifetime differs from shared clock")
				}
				if want > 0 {
					if len(g.traps) != 2 || g.traps[0].Life.Frames != want || g.traps[1].Life.Frames != want {
						t.Fatal("trap round conversion lost remainder")
					}
					g.turnBasedMode = false
					for range want {
						gl.updateSpecialEffects()
					}
				}
				if charm.Pacified || bind.Bound || !forever.Bound || len(g.traps) != 0 || !charm.IsEngagingPlayer || !bind.IsEngagingPlayer {
					t.Fatal("expiry did not clean up/re-engage")
				}
				beforeCharm, beforeBind := countCombatLog(g, "wears off"), countCombatLog(g, "breaks free")
				if beforeCharm != 1 || beforeBind != 1 {
					t.Fatal("missing expiry announcement")
				}
				for range 5 {
					gl.updateSpecialEffects()
				}
				if countCombatLog(g, "wears off") != beforeCharm || countCombatLog(g, "breaks free") != beforeBind {
					t.Fatal("expiry ran more than once")
				}
			})
		}
	}
}

func TestStatusClockProvidenceCalendar(t *testing.T) {
	for _, id := range []spells.SpellID{"bless", "heroism"} {
		for _, tb := range []bool{false, true} {
			for _, legacy := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tb=%v/legacy=%v", id, tb, legacy), func(t *testing.T) {
					g, gl, wm := clockSaveFixture(t)
					g.turnBasedMode = tb
					g.party.Members[0].Race = "celestial"
					g.party.Members[0].EnsureRacialTraits(nil)
					setProvidencePool(g, id)
					g.dayNightFrames = g.dayNightCycleFrames()/4 - 20
					g.dayNightIsNight = false
					g.refreshCelestialProvidence()
					for range 250 {
						g.startPartyTurn()
					}
					if !g.phaseOwnsBuff(string(id)) {
						t.Fatal("rounds expired Providence before calendar boundary")
					}
					saved := auditSaveJSON(t, g.buildSave(wm))
					if legacy {
						saved.TimedBuffSourceVersion = 0
						for i := range saved.StatBuffs {
							saved.StatBuffs[i].SourceID = ""
						}
						for i := range saved.CombatBuffs {
							saved.CombatBuffs[i].SourceID = ""
						}
					}
					g.restoreSavedEffects(&saved)
					if !g.phaseOwnsBuff(string(id)) {
						t.Fatal("load lost calendar ownership")
					}
					for range 20 {
						g.updateDayNight()
						gl.updateSpecialEffects()
					}
					if g.dayNightIsNight || !g.phaseOwnsBuff(string(id)) {
						t.Fatal("Providence expired early")
					}
					rows, _ := g.buffStatusCardRows(&BuffStatus{ID: string(id), Duration: 1})
					card := rows.Lines()
					tooltip := strings.Join(card, " ")
					if !strings.Contains(tooltip, "Until dusk") || strings.Contains(tooltip, "dispel") || strings.Contains(tooltip, "turn") {
						t.Fatalf("wrong Providence clock tooltip: %s", tooltip)
					}
					// Remove the only Celestial to make the next boundary's expiry unambiguous.
					g.party.Members[0].Race = "human"
					delete(g.party.Members[0].Skills, character.SkillCelestialProvidence)
					g.updateDayNight()
					if !g.dayNightIsNight || g.phaseOwnsBuff(string(id)) {
						t.Fatal("dusk failed to remove old Providence")
					}
					g.party.Members[0].Race = "celestial"
					g.party.Members[0].EnsureRacialTraits(nil)
					g.refreshCelestialProvidence()
					// A normal cast takes ownership; neither natural nor paid phase cleanup
					// may remove it, and it must return to the ordinary combat clock.
					if id == "bless" {
						g.addStatBuff(TimedStatBuff{SpellID: string(id), Clock: framesClock(1000)})
					} else {
						g.addCombatBuff(TimedCombatBuff{ID: string(id), Clock: framesClock(1000)})
					}
					g.advanceDayNightToPhase(false)
					g.finishDayNightSkipImmediately()
					g.tickPartyBuffsTurn()
					left := 0
					if b, ok := g.statBuffByID(string(id)); ok {
						left = b.Clock.Frames
					}
					if b, ok := g.combatBuffByID(string(id)); ok {
						left = b.Clock.Frames
					}
					if left != 1000-g.combatRoundFrames() || g.phaseOwnsBuff(string(id)) {
						t.Fatal("manual cast lost ownership/clock after paid phase skip")
					}
				})
			}
		}
	}
}

// Regeneration periods survive save/load for active and reserve members, TB
// thinking spends neither, and each pays when its own clock runs out. An old
// save's RT frames since a payout become the same part of the period;
// malformed ones are dropped.
func TestStatusClockRegenSaveProgress(t *testing.T) {
	calm := config.RegenerationSeconds * config.GetTargetTPS()
	for _, reserve := range []bool{false, true} {
		for _, tc := range []struct {
			sp, hp   int // frames left
			inCombat bool
		}{
			{1, 1, false},
			{123, 123, false},
			{calm - 1, calm - 1, false},
			{420, 123, true}, // SP saved mid-fight, past the calm period
		} {
			t.Run(fmt.Sprintf("reserve=%v/sp=%d/hp=%d", reserve, tc.sp, tc.hp), func(t *testing.T) {
				g, _, wm := clockSaveFixture(t)
				c := g.party.Members[1]
				c.Conditions = nil
				c.HitPoints, c.MaxHitPoints = 30, 100
				c.SpellPoints, c.MaxSpellPoints = 0, 100
				c.BonusRegenPct = 5
				c.SPRegen, c.HPRegen, c.SPRegenFight = framesClock(tc.sp), framesClock(tc.hp), tc.inCombat
				if reserve {
					g.party.Reserve = append(g.party.Reserve, c)
					g.party.Members = append(g.party.Members[:1], g.party.Members[2:]...)
				}
				saved := auditSaveJSON(t, g.buildSave(wm))
				g.restoreSavedParty(&saved)
				if reserve {
					for _, candidate := range g.party.Reserve {
						if candidate.Name == c.Name {
							c = candidate
							break
						}
					}
				} else {
					c = g.party.Members[1]
				}
				c.HitPoints, c.MaxHitPoints = 30, 100
				c.SpellPoints, c.MaxSpellPoints = 0, 100
				c.BonusRegenPct = 5 // normally re-derived from equipped cards by applySave
				if c.SPRegen != framesClock(tc.sp) || c.HPRegen != framesClock(tc.hp) || c.SPRegenFight != tc.inCombat {
					t.Fatalf("load periods %+v/%+v (fight %v), want %d/%d frames", c.SPRegen, c.HPRegen, c.SPRegenFight, tc.sp, tc.hp)
				}
				for range 10 {
					c.UpdateWithMode(character.PartyClock{TurnBased: true})
				}
				if c.SPRegen != framesClock(tc.sp) || c.HPRegen != framesClock(tc.hp) {
					t.Fatal("TB thinking advanced regeneration")
				}
				startHP, startSP := c.HitPoints, c.SpellPoints
				spPaidAt, hpPaidAt := 0, 0
				for frame := 1; spPaidAt == 0 || hpPaidAt == 0; frame++ {
					if frame > max(tc.sp, tc.hp) {
						t.Fatalf("no payout within %d frames: SP at %d, HP at %d", frame, spPaidAt, hpPaidAt)
					}
					c.UpdateWithMode(character.PartyClock{InCombat: tc.inCombat})
					if spPaidAt == 0 && c.SpellPoints > startSP {
						spPaidAt = frame
					}
					if hpPaidAt == 0 && c.HitPoints > startHP {
						hpPaidAt = frame
					}
				}
				if spPaidAt != tc.sp || hpPaidAt != tc.hp {
					t.Fatalf("paid SP at frame %d, HP at %d; want %d and %d", spPaidAt, hpPaidAt, tc.sp, tc.hp)
				}
			})
		}
	}
	for _, tc := range []struct {
		raw    string
		sp, hp int // frames left, 0 = idle
	}{
		{`{"name":"Legacy"}`, 0, 0},
		{`{"spell_regen_timer":100,"hp_regen_timer":200}`, calm - 100, calm - 200},
		{`{"spell_regen_timer":-20,"hp_regen_timer":99999}`, 0, 1}, // spent: pays on the next tick
	} {
		var saved CharacterSave
		if err := json.Unmarshal([]byte(tc.raw), &saved); err != nil {
			t.Fatal(err)
		}
		c := restoreCharacterSave(saved)
		if c.SPRegen.Frames != tc.sp || c.HPRegen.Frames != tc.hp {
			t.Fatalf("%s: periods %+v/%+v, want %d/%d frames", tc.raw, c.SPRegen, c.HPRegen, tc.sp, tc.hp)
		}
	}
}

func ratedMonsterClock(m *monster.Monster3D, kind string) (frames, turns, rate *int) {
	switch kind {
	case "stun":
		return &m.Stun.Frames, &m.Stun.Turns, &m.Stun.Rate
	case "root":
		return &m.Root.Frames, &m.Root.Turns, &m.Root.Rate
	case "shred":
		return &m.ArmorShred.Frames, &m.ArmorShred.Turns, &m.ArmorShred.Rate
	case "slow":
		return &m.Slow.Frames, &m.Slow.Turns, &m.Slow.Rate
	case "weaken":
		return &m.Weaken.Frames, &m.Weaken.Turns, &m.Weaken.Rate
	case "soak":
		return &m.Soak.Frames, &m.Soak.Turns, &m.Soak.Rate
	case "pounce":
		return &m.PounceCD.Frames, &m.PounceCD.Turns, &m.PounceCD.Rate
	case "volley":
		return &m.TrapVolleyCD.Frames, &m.TrapVolleyCD.Turns, &m.TrapVolleyCD.Rate
	default:
		panic("unknown test clock")
	}
}

func TestStatusClockRatedProductionRemainders(t *testing.T) {
	for _, kind := range []string{"stun", "root", "shred", "slow", "weaken", "soak", "pounce", "volley", "party_root", "hero_stun"} {
		for _, saved := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/save=%v", kind, saved), func(t *testing.T) {
				g, gl, wm := clockSaveFixture(t)
				g.turnBasedMode = false
				tps := g.config.GetTPS()
				m := monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				c := g.party.Members[0]
				c.Conditions = nil
				var f, turns, rate *int
				pointers := func() {
					switch kind {
					case "party_root":
						f, turns, rate = &g.partyRoot.Frames, &g.partyRoot.Turns, &g.partyRoot.Rate
					case "hero_stun":
						f, turns, rate = &c.Stun.Frames, &c.Stun.Turns, &c.Stun.Rate
					default:
						f, turns, rate = ratedMonsterClock(m, kind)
					}
				}
				pointers()
				*f, *turns, *rate = 4*tps, 2, 2*tps
				m.ArmorShredPct, m.SlowPct, m.WeakenPct, m.SoakDamage = 20, 20, 20, 5
				tickRT := func() {
					switch kind {
					case "party_root":
						gl.updateSpecialEffects()
					case "hero_stun":
						g.updatePartyClocks()
					case "pounce":
						m.TickAbilityClocks(false)
					case "volley":
						m.TickAbilityClocks(false)
					default:
						wrapper := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
						wrapper.Update()
					}
				}
				for range tps {
					tickRT()
				}
				if *f != 3*tps || *turns != 2 {
					t.Fatalf("RT remainder=%d/%d", *f, *turns)
				}
				if saved {
					snapshot := auditSaveJSON(t, g.buildSave(wm))
					g.restoreSavedMonsters(wm, &snapshot)
					g.restoreSavedParty(&snapshot)
					g.restoreSavedTurnState(&snapshot)
					m = g.world.Monsters[0]
					c = g.party.Members[0]
					pointers()
				}
				g.turnBasedMode = true
				switch kind {
				case "party_root":
					g.endPartyTurn()
				case "hero_stun":
					g.startPartyTurn()
				case "pounce":
					m.TickAbilityClocks(true)
				case "volley":
					m.TickAbilityClocks(true)
				default:
					g.tickMonsterTurnStatuses(m, true)
				}
				if *f != tps || *turns != 1 || *rate != 2*tps {
					t.Fatalf("one round refunded RT progress: frames/turns/rate=%d/%d/%d, want %d/1/%d", *f, *turns, *rate, tps, 2*tps)
				}
				g.turnBasedMode = false
				for range tps {
					tickRT()
				}
				if *f != 0 || *turns != 0 || *rate != 0 {
					t.Fatalf("exact RT remainder failed to expire: %d/%d/%d", *f, *turns, *rate)
				}
			})
		}
	}
}

// Every monster clock comes back from a reload the same, whether the save
// holds the clock or the frames/turns/rate triple older saves wrote instead.
func TestMonsterClocksSaveAndLegacyTriples(t *testing.T) {
	legacyKeys := map[string][]string{
		"stun_clock":           {"stun_frames_remaining", "stun_turns_remaining", "stun_rate"},
		"root_clock":           {"root_frames_remaining", "root_turns_remaining", "root_rate"},
		"armor_shred_clock":    {"armor_shred_frames", "armor_shred_turns", "armor_shred_rate"},
		"slow_clock":           {"slow_frames", "slow_turns", "slow_rate"},
		"weaken_clock":         {"weaken_frames", "weaken_turns", "weaken_rate"},
		"soak_clock":           {"soak_frames", "soak_turns", "soak_rate"},
		"pounce_clock":         {"pounce_cd_frames", "pounce_cd_turns", "pounce_cd_rate"},
		"phase_shift_clock":    {"phase_shift_cd_frames", "phase_shift_cd_turns", "phase_shift_cd_rate"},
		"trap_volley_clock":    {"trap_volley_cd", "trap_volley_turn_cd", "trap_volley_cd_rate"},
		"stun_dr_memory_clock": {"stun_dr_memory_frames", "stun_dr_memory_turns"},
	}
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%v", legacy), func(t *testing.T) {
			g, _, wm := clockSaveFixture(t)
			m := monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
			clocks := func(m *monster.Monster3D) []*status.DualClock {
				return []*status.DualClock{&m.Stun, &m.Root, &m.ArmorShred, &m.Slow, &m.Weaken, &m.Soak, &m.PounceCD, &m.PhaseShiftCD, &m.TrapVolleyCD}
			}
			for i, c := range clocks(m) {
				*c = status.DualClock{Frames: 100*(i+1) + 7, Turns: i + 1, Rate: 100 + i}
			}
			m.StunDRStacks, m.StunDRMemory = 1, framesClock(600)
			m.ArmorShredPct, m.SlowPct, m.WeakenPct, m.SoakDamage = 20, 20, 20, 5
			g.world.Monsters = []*monster.Monster3D{m}
			raw, err := json.Marshal(g.buildSave(wm))
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				var doc map[string]any
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				// Every record of the monster: the current map's and the loaded maps'.
				records := doc["monsters"].([]any)
				for _, list := range doc["map_monsters"].(map[string]any) {
					records = append(records, list.([]any)...)
				}
				for _, record := range records {
					mon := record.(map[string]any)
					for key, names := range legacyKeys {
						clock := mon[key].(map[string]any)
						for i, field := range []string{"frames", "turns", "rate"}[:len(names)] {
							mon[names[i]] = clock[field]
						}
						delete(mon, key)
					}
				}
				if len(records) < 2 {
					t.Fatalf("fixture: expected the monster in both lists, got %d records", len(records))
				}
				if raw, err = json.Marshal(doc); err != nil {
					t.Fatal(err)
				}
			}
			var save GameSave
			if err := json.Unmarshal(raw, &save); err != nil {
				t.Fatal(err)
			}
			g.restoreSavedMonsters(wm, &save)
			got := g.world.Monsters[0]
			for i, c := range clocks(got) {
				if want := *clocks(m)[i]; *c != want {
					t.Errorf("clock %d = %+v, want %+v", i, *c, want)
				}
			}
			if got.StunDRMemory != m.StunDRMemory {
				t.Errorf("stun DR memory = %+v, want %+v", got.StunDRMemory, m.StunDRMemory)
			}
		})
	}
}

// An old TB save made on the last slowed turn - the slow alive only through
// the turn latch - keeps the count toward its skipped turn.
func TestLegacySlowSkipSurvivesTheLatchedTurn(t *testing.T) {
	g, _, wm := clockSaveFixture(t)
	g.turnBasedMode = true
	m := monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
	m.ApplySlow(30, 1, 1)
	m.TickDebuffs(true) // the clock runs out; the latch holds the turn
	if m.Slow.Active() || m.ActiveSlowPct() <= 0 {
		t.Fatal("fixture: the slow should live only through the latch")
	}
	g.world.Monsters = []*monster.Monster3D{m}
	save := auditSaveJSON(t, g.buildSave(wm))
	legacy := func(ms *MonsterSave) { ms.SlowSkip, ms.SlowTurnCount = nil, config.SlowSkipEveryTurns()-1 }
	legacy(&save.Monsters[0])
	for _, list := range save.MapMonsters {
		for i := range list {
			legacy(&list[i])
		}
	}
	g.restoreSavedMonsters(wm, &save)
	if !g.world.Monsters[0].ConsumeSlowedTurn(config.SlowSkipEveryTurns()) {
		t.Fatal("the legacy count was dropped: the due turn was not skipped")
	}
}

// An old save's party TB regen rounds carry into the active members'
// periods; a save without the HP count paced HP on the SP count.
func TestLegacyTurnBasedRegenRounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		hp     *int
		wantHP int // TB turns left; 0 = no progress, the period starts fresh
	}{
		{"both counts", func() *int { n := 0; return &n }(), 0},
		{"one shared count", nil, config.RegenerationRounds - 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, wm := clockSaveFixture(t)
			save := auditSaveJSON(t, g.buildSave(wm))
			for i := range save.Party.Members {
				save.Party.Members[i].SPRegen, save.Party.Members[i].HPRegen = nil, nil
			}
			save.LegacyTBSPRegenRounds, save.LegacyTBHPRegenRounds = 2, tc.hp
			g.restoreSavedParty(&save)
			c := g.party.Members[0]
			if c.SPRegen.Turns != config.RegenerationRounds-2 || c.HPRegen.Turns != tc.wantHP {
				t.Fatalf("periods SP %+v HP %+v, want %d and %d turns left", c.SPRegen, c.HPRegen, config.RegenerationRounds-2, tc.wantHP)
			}
		})
	}
}

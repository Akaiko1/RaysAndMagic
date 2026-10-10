package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
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
				perSecond++
			}
			if kind != "poison" {
				perSecond += 3
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
				extra = character.PoisonDamagePerTick
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
				charm.Pacified, charm.PacifiedFramesRemaining = true, remaining+1
				bind.Bound, bind.BoundFramesRemaining = true, remaining+1
				forever.Bound = true
				g.world.Monsters = []*monster.Monster3D{charm, bind, forever}
				g.traps = []PlacedTrap{{Key: "bear_trap", MapKey: "forest", TileX: 1, TileY: 1, X: 96, Y: 96, Owner: g.party.Members[0], FramesLeft: remaining + 1}, {Key: "bear_trap", MapKey: "mountains", FramesLeft: remaining + 1, Owner: g.party.Members[0]}}
				gl.updateSpecialEffects()
				if charm.PacifiedFramesRemaining != remaining || bind.BoundFramesRemaining != remaining || g.traps[0].FramesLeft != remaining {
					t.Fatal("RT did not spend one frame")
				}
				g.ToggleTurnBasedMode()
				for range 40 {
					gl.updateSpecialEffects()
				}
				if charm.PacifiedFramesRemaining != remaining || bind.BoundFramesRemaining != remaining || g.traps[0].FramesLeft != remaining {
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
				if charm.PacifiedFramesRemaining != want || bind.BoundFramesRemaining != want || charm.Pacified != (want > 0) || bind.Bound != (want > 0) || !forever.Bound {
					t.Fatal("resolved round control lifetime differs from shared clock")
				}
				if want > 0 {
					if len(g.traps) != 2 || g.traps[0].FramesLeft != want || g.traps[1].FramesLeft != want {
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
					old := celestialProvidenceBuffPool
					celestialProvidenceBuffPool = []spells.SpellID{id}
					t.Cleanup(func() { celestialProvidenceBuffPool = old })
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
					rows, _ := g.buffStatusCardRows(&UtilitySpellStatus{SpellID: id, Duration: 1})
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
						g.addStatBuff(TimedStatBuff{SpellID: string(id), Frames: 1000})
					} else {
						g.addCombatBuff(TimedCombatBuff{SpellID: string(id), Frames: 1000})
					}
					g.advanceDayNightToPhase(false)
					g.finishDayNightSkipImmediately()
					g.tickPartyBuffsTurn()
					left := 0
					if b, ok := g.statBuffByID(string(id)); ok {
						left = b.Frames
					}
					if b, ok := g.combatBuffByID(string(id)); ok {
						left = b.Frames
					}
					if left != 1000-g.combatRoundFrames() || g.phaseOwnsBuff(string(id)) {
						t.Fatal("manual cast lost ownership/clock after paid phase skip")
					}
				})
			}
		}
	}
}

func TestStatusClockRegenSaveProgress(t *testing.T) {
	for _, reserve := range []bool{false, true} {
		for _, phase := range []int{0, 123, character.ManaRegenIntervalFrames - 1} {
			t.Run(fmt.Sprintf("reserve=%v/phase=%d", reserve, phase), func(t *testing.T) {
				g, _, wm := clockSaveFixture(t)
				c := g.party.Members[1]
				c.Conditions = nil
				c.HitPoints, c.MaxHitPoints = 30, 100
				c.SpellPoints, c.MaxSpellPoints = 0, 100
				c.BonusRegenPct = 5
				c.RestoreRealtimeRegenProgress(phase, phase)
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
				sp, hp := c.RealtimeRegenProgress()
				if sp != phase || hp != phase {
					t.Fatalf("load phase %d/%d, want %d", sp, hp, phase)
				}
				for range 10 {
					c.UpdateWithMode(true)
				}
				sp, hp = c.RealtimeRegenProgress()
				if sp != phase || hp != phase {
					t.Fatal("TB thinking advanced RT regeneration")
				}
				startHP, startSP := c.HitPoints, c.SpellPoints
				for range character.ManaRegenIntervalFrames - phase - 1 {
					c.UpdateWithMode(false)
				}
				if c.HitPoints != startHP || c.SpellPoints != startSP {
					t.Fatal("regeneration paid early")
				}
				c.UpdateWithMode(false)
				if c.HitPoints <= startHP || c.SpellPoints <= startSP {
					t.Fatal("load postponed regeneration payout")
				}
				c.RestoreRealtimeRegenProgress(37, 51)
				c.ResetRealtimeRegenCadence()
				sp, hp = c.RealtimeRegenProgress()
				if sp != 0 || hp != 0 {
					t.Fatal("TB payout no longer resets RT cadence")
				}
			})
		}
	}
	for _, raw := range []string{`{"name":"Legacy"}`, `{"spell_regen_timer":-20,"hp_regen_timer":99999}`} {
		var saved CharacterSave
		if err := json.Unmarshal([]byte(raw), &saved); err != nil {
			t.Fatal(err)
		}
		c := restoreCharacterSave(saved)
		sp, hp := c.RealtimeRegenProgress()
		if sp < 0 || hp < 0 || sp >= character.ManaRegenIntervalFrames || hp >= character.ManaRegenIntervalFrames {
			t.Fatal("invalid saved regeneration phase")
		}
	}
}

func ratedMonsterClock(m *monster.Monster3D, kind string) (frames, turns, rate *int) {
	switch kind {
	case "stun":
		return &m.StunFramesRemaining, &m.StunTurnsRemaining, &m.StunRate
	case "root":
		return &m.RootFramesRemaining, &m.RootTurnsRemaining, &m.RootRate
	case "shred":
		return &m.ArmorShredFramesRemaining, &m.ArmorShredTurnsRemaining, &m.ArmorShredRate
	case "slow":
		return &m.SlowFramesRemaining, &m.SlowTurnsRemaining, &m.SlowRate
	case "weaken":
		return &m.WeakenFramesRemaining, &m.WeakenTurnsRemaining, &m.WeakenRate
	case "soak":
		return &m.SoakFrames, &m.SoakTurns, &m.SoakRate
	case "pounce":
		return &m.PounceCDFrames, &m.PounceCDTurns, &m.PounceCDRate
	case "volley":
		return &m.TrapVolleyCDFrames, &m.TrapVolleyTurnCD, &m.TrapVolleyCDRate
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
						f, turns, rate = &c.StunFramesRemaining, &c.StunTurnsRemaining, &c.StunRate
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
						m.TickPounceCooldownFrame()
					case "volley":
						m.TickTrapVolleyCooldownFrame()
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
					m.TickPounceCooldownTurn()
				case "volley":
					m.TickTrapVolleyCooldownTurn()
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

package game

import (
	"fmt"
	"testing"
	"ugataima/internal/status"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestControlSavingThrowsThroughInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, id := range []string{"turn_undead", "mass_fear", "feeblemind"} {
			for _, saves := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/TB=%v/saves=%v", id, tb, saves), func(t *testing.T) {
					g := advancedSupportGame(t, tb)
					equipSpellAndPrepareCaster(t, g.combat, id, 1000, 50)
					c := g.party.Members[0]
					c.Race = "human"
					m := mkTestMonster("Control target", 100)
					m.ID, m.X, m.Y = "control-target", g.camera.X+g.config.GetTileSize(), g.camera.Y
					if id == "turn_undead" {
						m.MonsterType = monster.TypeUndead
					}
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					g.combat.controlSaveRoll = func() float64 {
						if saves {
							return 0
						}
						return 0.99
					}
					def, _ := spells.GetSpellDefinitionByID(spells.SpellID(id))
					sp, actions := c.SpellPoints, c.ActionsRemaining
					ih := NewInputHandler(g)
					pressKeys(ih, ebiten.KeyF)
					if (m.ControlDRActive() || m.Silence.Frames > 0) == saves {
						t.Fatalf("control active=%v, saves=%v", m.ControlDRActive(), saves)
					}
					if c.SpellPoints != sp-c.MeditatedSPCost(def.SpellPointsCost) {
						t.Fatal("resisted/successful cast did not spend mana exactly once")
					}
					if tb && c.ActionsRemaining != actions-1 || !tb && c.RTCooldown <= 0 {
						t.Fatal("cast did not spend its action/cooldown")
					}
					wantStacks := 1
					if saves || id == "feeblemind" {
						wantStacks = 0
					}
					if m.StunDRStacks != wantStacks {
						t.Fatal("save roll incorrectly advanced diminishing returns")
					}
				})
			}
		}
	}
}

func TestOtherSpellControlsKeepFullDurationWithoutDR(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, id := range []string{"slow", "frozen_ground", "feeblemind", "berserk"} {
			t.Run(fmt.Sprintf("%s/TB=%v", id, tb), func(t *testing.T) {
				g := advancedSupportGame(t, tb)
				equipSpellAndPrepareCaster(t, g.combat, id, 1000, 50)
				m := mkTestMonster("Guaranteed control target", 100)
				m.X, m.Y = g.camera.X+64, g.camera.Y
				m.StunDRStacks, m.StunDRMemory = 3, framesClock(960)
				g.world.Monsters = []*monster.Monster3D{m}
				g.combat.controlSaveRoll = func() float64 {
					if id == "feeblemind" {
						return 0.99
					}
					t.Fatal("guaranteed control rolled a save")
					return 0
				}
				ih := NewInputHandler(g)
				pressKeys(ih, ebiten.KeyF)
				frames := map[string]int{"slow": m.Slow.Frames, "frozen_ground": m.Root.Frames, "feeblemind": m.Silence.Frames, "berserk": m.Berserk.Frames}[id]
				def, _ := spells.GetSpellDefinitionByID(spells.SpellID(id))
				c := g.party.Members[0]
				full := def.DurationSeconds(int(c.MagicSchools[c.SpellSchoolFor(def)].Mastery)) * g.config.GetTPS()
				if full <= 0 || frames != full {
					t.Fatalf("guaranteed control lasted %d frames, want the full authored %d", frames, full)
				}
				if m.StunDRStacks != 3 || m.StunDRMemory != framesClock(960) {
					t.Fatal("guaranteed effect changed the DR chain")
				}
				if m.ControlDRActive() {
					t.Fatal("guaranteed control delayed stun/fear recovery")
				}
			})
		}
	}
}

func TestControlSaveUsesAuthoredCurveAndSeparateVictimRolls(t *testing.T) {
	g := advancedSupportGame(t, false)
	cs, c := g.combat, g.party.Members[0]
	equipSpellAndPrepareCaster(t, cs, "mass_fear", 1000, 50)
	def, _ := spells.GetSpellDefinitionByID("mass_fear")
	c.MagicSchools[character.MagicSchoolMind].Mastery = character.MasteryGrandMaster
	a, b := mkTestMonster("A", 100), mkTestMonster("B", 100)
	a.X, a.Y, b.X, b.Y = g.camera.X+64, g.camera.Y, g.camera.X+128, g.camera.Y
	a.Level, b.Level = c.Level+5, c.Level+5
	b.MonsterType = monster.TypeFormless // Formless is not a universal control immunity.
	g.world.Monsters = []*monster.Monster3D{a, b}
	rolls := 0
	cs.controlSaveRoll = func() float64 { rolls++; return float64(rolls)*0.2 + 0.1 } // .3 resists; .5 lands.
	cs.castResolvedSpell(def.ID, def, c, def.SpellPointsCost, false, false)
	if rolls != 2 || a.Fear.Frames != 0 || b.Fear.Frames == 0 {
		t.Fatal("area did not roll separately against the 50% boundary")
	}
	// Change authored balance and verify it reaches the production cast, not a copy.
	rule := &g.config.StatusEffects.ControlSaves
	old := *rule
	t.Cleanup(func() { *rule = old })
	rule.Curve = append([]config.ControlSavePoint(nil), rule.Curve...)
	rule.Curve[2].Percent = 60
	cs.controlSaveRoll = func() float64 { return 0.5 }
	a.Fear, b.Fear = status.DualClock{}, framesClock(0)
	cs.castResolvedSpell(def.ID, def, c, def.SpellPointsCost, false, false)
	if a.Fear.Frames != 0 || b.Fear.Frames != 0 {
		t.Fatal("cast ignored the authored curve change")
	}
	// The same effect reads its own school's resistance, not physical armor.
	*rule = old
	a.Resistances[monster.DamageMind] = 40
	cs.controlSaveRoll = func() float64 { return 0.6 }
	cs.castResolvedSpell(def.ID, def, c, def.SpellPointsCost, false, false)
	if a.Fear.Frames != 0 || b.Fear.Frames == 0 {
		t.Fatal("cast ignored school resistance")
	}
}

func TestControlSaveIneligibleTargetsDoNotRollOrSpendMana(t *testing.T) {
	for _, state := range []string{"undead", "bound", "dead", "sealed", "range", "wall"} {
		t.Run(state, func(t *testing.T) {
			g := advancedSupportGame(t, false)
			g.world = newTestWorldSized(g.config, 8, 8)
			g.collisionSystem = collision.NewCollisionSystem(g.world, g.config.GetTileSize())
			equipSpellAndPrepareCaster(t, g.combat, "mass_fear", 1000, 50)
			c := g.party.Members[0]
			m := mkTestMonster("Ineligible", 100)
			ts := g.config.GetTileSize()
			g.camera.X, g.camera.Y = 1.5*ts, 1.5*ts
			m.X, m.Y = 3.5*ts, g.camera.Y
			switch state {
			case "undead":
				m.MonsterType = monster.TypeUndead
			case "bound":
				m.Bound = true
			case "dead":
				m.HitPoints = 0
			case "sealed":
				m.BossDormant = true
			case "range":
				m.X = g.camera.X + 20*ts
			case "wall":
				g.world.Tiles[1][2] = world.TileWall
			}
			g.world.Monsters = []*monster.Monster3D{m}
			g.combat.controlSaveRoll = func() float64 { t.Fatal("rolled for ineligible target"); return 0 }
			g.combat.CastEquippedSpell()
			if c.SpellPoints != 1000 || m.ControlDRActive() || m.StunDRStacks != 0 {
				t.Fatal("ineligible target spent resources or acquired control")
			}
		})
	}
}

func TestSpellControlsShareStunDiminishingAndRecovery(t *testing.T) {
	g := advancedSupportGame(t, false)
	cs, c := g.combat, g.party.Members[0]
	cs.controlSaveRoll = func() float64 { return 0.99 }
	m := mkTestMonster("Shared control target", 100)
	m.X, m.Y = g.camera.X+64, g.camera.Y
	g.world.Monsters = []*monster.Monster3D{m}
	// A weapon stun and fear share one chain in both directions.
	cs.applyStunDR(m, 2, false)
	m.Stun.Frames, m.Stun.Turns = 0, 0
	equipSpellAndPrepareCaster(t, cs, "mass_fear", 1000, 50)
	def, _ := spells.GetSpellDefinitionByID("mass_fear")
	cs.castResolvedSpell(def.ID, def, c, def.SpellPointsCost, false, false)
	if m.Fear.Frames != 360 {
		t.Fatalf("second control lasted %d frames, want 360", m.Fear.Frames)
	}
	m.Fear = status.DualClock{}
	cs.applyStunDR(m, 2, false)
	// 25% of two rounds rounds up to one whole round, the same span in RT.
	if want := config.RoundFrames(1, g.config.GetTPS()); m.Stun.Frames != want || m.Stun.Turns != 1 {
		t.Fatalf("third control lasted %d frames / %d turns, want %d / 1", m.Stun.Frames, m.Stun.Turns, want)
	}
	m.Stun.Frames, m.Stun.Turns = 0, 0
	cs.castResolvedSpell(def.ID, def, c, def.SpellPointsCost, false, false)
	if m.Fear.Frames != 0 {
		t.Fatal("fourth control bypassed shared immunity")
	}
	m.ApplyFear(360)
	// The RT and TB scheduler paths must not forget DR while a control is active.
	beforeFrames, beforeTurns := m.StunDRMemory.Frames, m.StunDRMemory.Turns
	CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g).Update()
	g.turnBasedMode = true
	g.tickMonsterTurnStatuses(m, true)
	if m.StunDRMemory.Frames != beforeFrames || m.StunDRMemory.Turns != beforeTurns {
		t.Fatal("mode switch recovered DR under active control")
	}
	m.Fear, m.Silence = status.DualClock{}, framesClock(0)
	for range 3 {
		g.tickMonsterTurnStatuses(m, true)
	}
	if m.StunDRStacks != 3 {
		t.Fatal("DR expired before four free turns")
	}
	g.tickMonsterTurnStatuses(m, true)
	if m.StunDRStacks != 0 {
		t.Fatal("DR did not recover after four free turns")
	}
	cs.applyStunDR(m, 1, false)
	m.Stun.Frames, m.Stun.Turns = 0, 0
	g.turnBasedMode = false
	// The RT reset window is the same span as the four free TB turns.
	window := config.RoundFrames(4, g.config.GetTPS())
	for range window - 1 {
		CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g).Update()
	}
	if m.StunDRStacks != 1 {
		t.Fatal("DR expired before the RT reset window")
	}
	CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g).Update()
	if m.StunDRStacks != 0 {
		t.Fatal("DR did not recover after the RT reset window")
	}
}

func TestFeeblemindRecoveryWindow(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g := advancedSupportGame(t, tb)
			equipSpellAndPrepareCaster(t, g.combat, "feeblemind", 1000, 50)
			c := g.party.Members[0]
			m := mkTestMonster("Silence target", 1000)
			m.X, m.Y = g.camera.X+g.config.GetTileSize(), g.camera.Y
			m.ProjectileSpell = "firebolt"
			g.world.Monsters = []*monster.Monster3D{m}
			g.combat.controlSaveRoll = func() float64 { return .99 }
			d, _ := spells.GetSpellDefinitionByID("feeblemind")
			cast := func() {
				t.Helper()
				if !g.combat.castPlayerSpell(d.ID, d, c, false) {
					t.Fatal("eligible silence attempt rejected")
				}
			}
			cast()
			duration := m.Silence.Frames
			recovery := d.Mechanic.ReapplyImmunitySeconds * g.config.GetTPS()
			if duration <= 0 || m.SilenceImmunity.Frames != recovery {
				t.Fatal("silence did not initialize recovery")
			}
			// A tick is a frame in real time and a party turn in turn-based.
			unit := 1
			if tb {
				unit = g.combatRoundFrames()
			}
			ticks := func(n int) {
				for range n {
					g.advanceControlledMonsters(tb)
				}
			}
			ticks(1)
			cast()
			if m.Silence.Frames != duration-unit {
				t.Fatal("repeat cast refreshed silence")
			}
			ticks(duration/unit - 1)
			if !m.HasRangedAttack() || m.SilenceImmunity.Frames != recovery {
				t.Fatal("silence expiry did not restore attacks for the full immunity window")
			}
			cast()
			if m.Silence.Frames != 0 {
				t.Fatal("silence bypassed recovery immunity")
			}
			ticks(recovery/unit - 1)
			cast()
			if m.Silence.Frames != 0 {
				t.Fatal("recovery ended early")
			}
			ticks(1)
			cast()
			if m.Silence.Frames != duration || m.StunDRStacks != 0 {
				t.Fatal("silence did not resume at the exact boundary without DR")
			}
		})
	}
}

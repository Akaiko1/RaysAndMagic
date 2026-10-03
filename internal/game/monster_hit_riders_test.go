package game

import (
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
)

// A landed monster hit rolls its on-hit statuses once and lands them on
// whatever it struck - a hero or another monster - through that target's own
// rules. A champion's striking weapon rides the same fields. Dodged hits and
// dead targets get nothing; a wildlife hunt afflicts but stays out of the log.
func TestMonsterHitRidersReachEveryTarget(t *testing.T) {
	type rider struct {
		name   string
		arm    func(cs *CombatSystem, attacker *monsterPkg.Monster3D)
		onMob  func(m *monsterPkg.Monster3D) bool
		onHero func(c *character.MMCharacter) bool
		lines  []string // the rider's line on a monster and on a hero
	}
	riders := []rider{
		{"poison", func(_ *CombatSystem, a *monsterPkg.Monster3D) { a.PoisonChance, a.PoisonDurationSec = 1, 5 },
			func(m *monsterPkg.Monster3D) bool { return m.PoisonedFramesRemaining > 0 },
			func(c *character.MMCharacter) bool { return c.HasCondition(character.ConditionPoisoned) }, []string{"is poisoned!"}},
		{"ignite", func(_ *CombatSystem, a *monsterPkg.Monster3D) { a.IgniteChance, a.IgniteDurationSec = 1, 5 },
			func(m *monsterPkg.Monster3D) bool { return m.BurnFramesRemaining > 0 },
			func(c *character.MMCharacter) bool { return c.BurnFramesRemaining > 0 }, []string{"catches fire!", "bursts into flames!"}},
		{"stun", func(_ *CombatSystem, a *monsterPkg.Monster3D) {
			a.StunCharChance, a.StunCharSeconds, a.StunCharTurns = 1, 2, 2
		},
			func(m *monsterPkg.Monster3D) bool { return m.StunFramesRemaining > 0 || m.StunTurnsRemaining > 0 },
			func(c *character.MMCharacter) bool { return c.IsStunned() }, []string{"is stunned!"}},
		{"champion weapon stun", func(cs *CombatSystem, a *monsterPkg.Monster3D) {
			mace, ok := config.GetWeaponDefinition("steel_mace")
			if !ok || mace.StunChance <= 0 {
				panic("steel_mace lost its stun rider")
			}
			sure := *mace
			sure.StunChance = 1
			cs.applyChampionHandRiders(a, character.CreateCharacter("Gwen", character.ClassArmsMaster, cs.game.config), &sure)
		},
			func(m *monsterPkg.Monster3D) bool { return m.StunFramesRemaining > 0 || m.StunTurnsRemaining > 0 },
			func(c *character.MMCharacter) bool { return c.IsStunned() }, []string{"is stunned!"}},
	}
	type side struct {
		name     string
		attacker func() *monsterPkg.Monster3D
		target   func() *monsterPkg.Monster3D // nil: a hero
		tone     logTone
		quiet    bool
	}
	foe := func() *monsterPkg.Monster3D {
		return &monsterPkg.Monster3D{ID: "foe", Name: "Ocelot", HitPoints: 500, MaxHitPoints: 500}
	}
	ally := func() *monsterPkg.Monster3D {
		return &monsterPkg.Monster3D{ID: "ally", Name: "Masked Huntress", SummonedBy: spellSummonOwnerPrefix + "x", HitPoints: 500, MaxHitPoints: 500}
	}
	wild := func(id string) func() *monsterPkg.Monster3D {
		return func() *monsterPkg.Monster3D {
			return &monsterPkg.Monster3D{ID: id, Name: "Puma", Disposition: monsterPkg.DispositionWildlife, HitPoints: 500, MaxHitPoints: 500}
		}
	}
	sides := []side{
		{"a hero struck", foe, nil, logToneBad, false},
		{"a foe struck by the party's ally", ally, foe, logToneGood, false},
		{"the party's summon struck by a foe", foe, ally, logToneBad, false},
		{"a wildlife hunt", wild("hunter"), wild("prey"), logToneNone, true},
	}
	for _, r := range riders {
		for _, sd := range sides {
			t.Run(r.name+"/"+sd.name, func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				attacker := sd.attacker()
				r.arm(cs, attacker)
				start := len(g.combatLogHistory)
				afflicted := false
				if sd.target == nil {
					hero := g.party.Members[0]
					hero.Conditions, hero.HitPoints = nil, hero.MaxHitPoints
					cs.monsterHitCharacter(attacker, hero, attacker.Name, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 1}, DamageType: "physical", IgnoresDodge: true})
					afflicted = r.onHero(hero)
				} else {
					target := sd.target()
					cs.strikeMonsterPacketFor(attacker, target, singleMonsterDamagePacket(damagecalc.Parts{True: 1}, "physical", 0), nil, false, true, true, false)
					afflicted = r.onMob(target)
				}
				if !afflicted {
					t.Fatalf("the %s rider never landed", r.name)
				}
				logged := false
				for _, e := range g.combatLogHistory[start:] {
					if slices.ContainsFunc(r.lines, func(l string) bool { return strings.Contains(e.Text, l) }) {
						logged = true
						if e.Tone != sd.tone {
							t.Errorf("%q tone %d, want %d", e.Text, e.Tone, sd.tone)
						}
					}
				}
				if logged == sd.quiet {
					t.Errorf("rider line logged=%v, quiet hunt=%v: %v", logged, sd.quiet, texts(g.combatLogHistory[start:]))
				}
			})
		}
	}

	// Neighbours that must not change.
	t.Run("a dodged hit carries no rider", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		attacker := foe()
		attacker.PoisonChance, attacker.PoisonDurationSec = 1, 5
		target := ally()
		target.PerfectDodge = 100
		cs.strikeMonsterPacketFor(attacker, target, singleMonsterDamagePacket(damagecalc.Parts{Normal: 1}, "physical", 0), nil, false, true, false, true)
		if target.PoisonedFramesRemaining > 0 {
			t.Error("a dodged strike poisoned its target")
		}
	})
	t.Run("a slain target carries no rider", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		attacker := foe()
		attacker.PoisonChance, attacker.PoisonDurationSec = 1, 5
		target := ally()
		target.HitPoints = 1
		cs.strikeMonsterPacketFor(attacker, target, singleMonsterDamagePacket(damagecalc.Parts{True: 50}, "physical", 0), nil, false, true, true, false)
		if target.IsAlive() || target.PoisonedFramesRemaining > 0 {
			t.Errorf("slain target alive=%v poison=%d", target.IsAlive(), target.PoisonedFramesRemaining)
		}
	})
	t.Run("dispel strips party buffs only for a hit on the party", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		g := cs.game
		g.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 600, Bonuses: character.UniformStatBonuses(5)})
		attacker := foe()
		attacker.DispelChance = 1
		cs.strikeMonsterPacketFor(attacker, ally(), singleMonsterDamagePacket(damagecalc.Parts{True: 1}, "physical", 0), nil, false, true, true, false)
		if len(g.statBuffs) != 1 {
			t.Fatal("a foe striking the party's summon dispelled a party buff")
		}
		hero := g.party.Members[0]
		hero.HitPoints = hero.MaxHitPoints
		cs.monsterHitCharacter(attacker, hero, attacker.Name, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 1}, DamageType: "physical", IgnoresDodge: true})
		if len(g.statBuffs) != 0 {
			t.Error("a dispelling hit on a hero left the party buff")
		}
	})
}

package game

import (
	"slices"
	"testing"

	"maps"

	"ugataima/internal/character"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
)

// Every timed status lasts the same span in both modes: whatever lands it, its
// RT frames are exactly its TB rounds of config.CombatRoundSeconds, and those
// rounds are the source's authored seconds over the round length (plus a
// Trapper's bonus rounds). Each row drives one kind of source through the path
// the game uses, over every shipped entry that authors one; a slow also lasts
// whole steps of the TB skip cadence.
func TestStatusSourcesLastTheSameSpanInBothModes(t *testing.T) {
	newFixture := newTestCombatFactory(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	type landed struct{ frames, rounds int }
	expect := func(t *testing.T, cs *CombatSystem, what string, got landed, seconds int) {
		t.Helper()
		rounds := seconds / config.CombatRoundSeconds
		if seconds <= 0 || seconds%config.CombatRoundSeconds != 0 {
			t.Errorf("%s: authored %ds is not whole rounds", what, seconds)
		}
		if got.rounds != rounds || got.frames != config.RoundFrames(rounds, cs.game.config.GetTPS()) {
			t.Errorf("%s: landed %d RT frames / %d TB rounds, want %ds = %d rounds", what, got.frames, got.rounds, seconds, rounds)
		}
	}
	probeHero := func(cs *CombatSystem) *character.MMCharacter {
		hero := character.CreateCharacter("Probe", character.ClassKnight, cs.game.config)
		cs.game.party.Members[0] = hero
		return hero
	}
	sure := func(chance float64) float64 {
		if chance > 0 {
			return 1
		}
		return 0
	}

	t.Run("monster abilities on a hero and on a monster", func(t *testing.T) {
		cs := newFixture(t)
		for _, key := range catalogMonsterKeys() {
			def := monsterPkg.MonsterConfig.Monsters[key]
			a := def.Abilities
			if a.Stun.Chance <= 0 && a.MeltArmor.Chance <= 0 {
				continue
			}
			attacker := &monsterPkg.Monster3D{ID: key, Name: def.Name}
			attacker.Abilities.Stun = monsterPkg.TimedStatusAbility{Chance: sure(a.Stun.Chance), Seconds: a.Stun.Seconds}
			attacker.Abilities.MeltArmor.Chance = sure(a.MeltArmor.Chance)
			hero := probeHero(cs)
			cs.applyMonsterHitRiders(attacker, "", heroHitTarget{cs, hero}, true)
			target := mkTestMonster("Target", 100000)
			cs.applyMonsterHitRiders(attacker, "", monsterHitTarget{cs, attacker, target}, true)
			if a.Stun.Chance > 0 {
				expect(t, cs, key+" stun on a hero", landed{hero.Stun.Frames, hero.Stun.Turns}, a.Stun.Seconds)
				expect(t, cs, key+" stun on a monster", landed{target.Stun.Frames, target.Stun.Turns}, a.Stun.Seconds)
			}
			if a.MeltArmor.Chance > 0 {
				expect(t, cs, key+" melt on a hero", landed{hero.ArmorMelt.Frames, hero.ArmorMelt.Turns}, config.ArmorMeltSeconds())
				expect(t, cs, key+" melt on a monster", landed{target.ArmorMelt.Frames, target.ArmorMelt.Turns}, config.ArmorMeltSeconds())
			}
		}
	})

	t.Run("monster projectile spell stun", func(t *testing.T) {
		cs := newFixture(t)
		for _, key := range catalogMonsterKeys() {
			def := monsterPkg.MonsterConfig.Monsters[key]
			spell, ok := config.GetSpellDefinition(def.ProjectileSpell)
			if !ok || spell.StunChance <= 0 || def.Abilities.Stun.Chance > 0 {
				continue
			}
			forced := *spell
			forced.StunChance = 1
			config.GlobalSpells.Spells[def.ProjectileSpell] = &forced
			t.Cleanup(func() { config.GlobalSpells.Spells[def.ProjectileSpell] = spell })
			hero := probeHero(cs)
			cs.applyMonsterHitRiders(&monsterPkg.Monster3D{ID: key, Name: def.Name}, def.ProjectileSpell, heroHitTarget{cs, hero}, false)
			config.GlobalSpells.Spells[def.ProjectileSpell] = spell
			expect(t, cs, key+" "+def.ProjectileSpell+" stun", landed{hero.Stun.Frames, hero.Stun.Turns}, spell.StunDurationSeconds)
		}
	})

	t.Run("weapon riders on a monster", func(t *testing.T) {
		cs := newFixture(t)
		for _, key := range slices.Sorted(maps.Keys(config.GlobalWeapons.Weapons)) {
			def := *config.GlobalWeapons.Weapons[key]
			def.StunChance, def.RootChance = sure(def.StunChance), sure(def.RootChance)
			def.IgniteChance, def.PoisonChance = 0, 0
			m := mkTestMonster("Target", 100000)
			cs.tryApplyWeaponHitRiders(m, &def)
			for _, r := range []struct {
				what    string
				seconds int
				got     landed
			}{
				{"stun", def.StunSeconds, landed{m.Stun.Frames, m.Stun.Turns}},
				{"root", def.RootSeconds, landed{m.Root.Frames, m.Root.Turns}},
				{"armor shred", def.ArmorShredSeconds, landed{m.ArmorShred.Frames, m.ArmorShred.Turns}},
				{"slow", def.SlowSeconds, landed{m.Slow.Frames, m.Slow.Turns}},
				{"weaken", def.WeakenSeconds, landed{m.Weaken.Frames, m.Weaken.Turns}},
			} {
				if r.seconds > 0 {
					expect(t, cs, key+" "+r.what, r.got, r.seconds)
				}
			}
			if def.SlowSeconds > 0 && m.Slow.Turns%config.SlowSkipEveryTurns() != 0 {
				t.Errorf("%s slow lasts %d TB turns, not whole steps of %d", key, m.Slow.Turns, config.SlowSkipEveryTurns())
			}
		}
	})

	t.Run("spell stuns", func(t *testing.T) {
		cs := newFixture(t)
		for _, id := range slices.Sorted(maps.Keys(config.GlobalSpells.Spells)) {
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
			if err != nil || def.StunDurationSeconds <= 0 {
				continue
			}
			m := mkTestMonster("Target", 100000)
			if def.StunRadiusTiles > 0 {
				m.X, m.Y = cs.game.camera.X+float64(cs.game.config.GetTileSize()), cs.game.camera.Y
				cs.game.world.Monsters = []*monsterPkg.Monster3D{m}
				cs.tryCastAoeStunBy(spells.SpellID(id), def, cs.game.party.Members[0])
			} else {
				cs.applyStun(m, def.StunDurationSeconds, false) // projectile and mortar impacts
			}
			expect(t, cs, id+" stun", landed{m.Stun.Frames, m.Stun.Turns}, def.StunDurationSeconds)
		}
	})

	t.Run("control traps at every Trapper tier", func(t *testing.T) {
		cs := newFixture(t)
		for _, key := range slices.Sorted(maps.Keys(config.GlobalTrapConfig.Traps)) {
			def := config.GlobalTrapConfig.Traps[key]
			if def.StunSeconds <= 0 && def.RootSeconds <= 0 {
				continue
			}
			for tier := character.MasteryNovice; tier <= character.MasteryGrandMaster; tier++ {
				owner := character.CreateCharacter("Trapper", character.ClassThief, cs.game.config)
				owner.Skills[character.SkillTrapper] = &character.Skill{Mastery: tier}
				m := mkTestMonster("Target", 100000)
				cs.fireTrap(&PlacedTrap{Key: key, X: m.X, Y: m.Y, Owner: owner}, m)
				bonus := character.TrapperRoundBonus(int(tier)) * config.CombatRoundSeconds
				if def.StunSeconds > 0 {
					expect(t, cs, key+" stun at "+tier.String(), landed{m.Stun.Frames, m.Stun.Turns}, def.StunSeconds+bonus)
				}
				if def.RootSeconds > 0 {
					expect(t, cs, key+" root at "+tier.String(), landed{m.Root.Frames, m.Root.Turns}, def.RootSeconds+bonus)
				}
			}
		}
	})

	t.Run("telegraph hinders on the party", func(t *testing.T) {
		cs := newFixture(t)
		g := cs.game
		for _, key := range catalogMonsterKeys() {
			a := monsterPkg.MonsterConfig.Monsters[key].Abilities.Telegraph
			if a == nil || (a.Kind != "slow" && a.Kind != "root") {
				continue
			}
			g.partyHinder, g.partyRoot = PartyHinderState{}, status.DualClock{}
			g.applyTelegraphHinder(a)
			if a.Kind == "root" {
				expect(t, cs, key+" root", landed{g.partyRoot.Frames, g.partyRoot.Turns}, a.DurationSeconds)
				continue
			}
			window := g.partyHinder.Slow.Turns
			expect(t, cs, key+" slow", landed{g.partyHinder.Slow.Frames, window}, a.DurationSeconds)
			if window%config.SlowSkipEveryTurns() != 0 {
				t.Errorf("%s slow lasts %d TB turns, not whole steps of %d", key, window, config.SlowSkipEveryTurns())
			}
		}
	})
}

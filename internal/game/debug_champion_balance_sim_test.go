package game

// Headless champion-balance diagnostic - a DEBUG MODULE, not a regression test.
// Builds every champion at every difficulty tier and prints the numbers a duel
// actually runs on: sampled swing damage (through the real character pipeline,
// crits included), armor class, perfect dodge and gear resists - so tier
// ladders can be balanced against each other by fact instead of by eye.
//
// Run with:  RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_ChampionBalance -v

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestDebugSim_ChampionBalance(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	tiers := make([]string, 0, len(config.GlobalChampionConfig.Tiers))
	for name := range config.GlobalChampionConfig.Tiers {
		tiers = append(tiers, name)
	}
	sort.Slice(tiers, func(i, j int) bool {
		return config.GlobalChampionConfig.Tiers[tiers[i]].Level < config.GlobalChampionConfig.Tiers[tiers[j]].Level
	})

	const samples = 2000
	for _, key := range config.ChampionKeys() {
		def := config.GetChampionDefinition(key)
		t.Logf("=== %s (%s) ===", def.Name, def.Class)
		for _, tierName := range tiers {
			ch := cs.game.championTemplate(key, tierName)
			if ch == nil {
				t.Fatalf("%s %s: nil template", key, tierName)
			}
			tier := config.GetChampionTier(tierName)
			weapon := ch.Equipment[items.SlotMainHand]

			m := &monster.Monster3D{Resistances: map[monster.DamageType]int{}}
			sum, lo, hi := 0, 1<<30, 0
			for i := 0; i < samples; i++ {
				_, dmg := cs.championSwingDamage(m, ch, weapon)
				sum += dmg
				if dmg < lo {
					lo = dmg
				}
				if dmg > hi {
					hi = dmg
				}
			}
			wd := lookupWeaponConfigByName(weapon.Name)
			aoe := ""
			if wd != nil && wd.AoeRadiusTiles > 0 {
				aoe = " [AOE: whole party per bolt]"
			}
			resists := ""
			if monster.MonsterConfig != nil {
				for school := range monster.MonsterConfig.DamageTypes {
					if pct := ch.GearResistPct(school); pct != 0 {
						resists += fmt.Sprintf(" %s+%d", school, pct)
					}
				}
			}
			t.Logf("  %-10s L%d HP%-5d %-18s dmg avg %3d (%d-%d)%s | AC %d dodge %d%% int %d pers %d |%s",
				tierName, tier.Level, tier.HP, weapon.Name,
				sum/samples, lo, hi, aoe,
				cs.CalculateTotalArmorClass(ch),
				ch.GetEffectiveLuck()/LuckToDodgeDivisor+cs.armorGMDodgeBonus(ch),
				ch.GetEffectiveIntellect(), ch.GetEffectivePersonality(), resists)

			// The 5% cast pool at this tier: per-spell damage through the same
			// pipeline the cast uses (school mastery included).
			if def.SpellCastChance > 0 {
				line := fmt.Sprintf("    casts %.0f%%:", def.SpellCastChance*100)
				for _, id := range championSpellPool(def) {
					sd, err := spells.GetSpellDefinitionByID(id)
					if err != nil {
						continue
					}
					switch {
					case sd.IncomingDamageReduction > 0:
						line += fmt.Sprintf(" %s[soak %d]", id, scaledIncomingDamageReduction(sd, ch))
					case sd.StunRadiusTiles > 0:
						line += fmt.Sprintf(" %s[stun %ds/%dt]", id, sd.StunDurationSeconds, sd.StunDurationTurns)
					default:
						_, _, dmg := cs.CalculateSpellDamage(id, ch)
						tag := ""
						if sd.AoeRadiusTiles > 0 {
							tag = "*PARTY"
						}
						line += fmt.Sprintf(" %s[%d%s]", id, dmg, tag)
					}
				}
				if def.OpeningSpell != "" {
					osd, err := spells.GetSpellDefinitionByID(spells.SpellID(def.OpeningSpell))
					if err == nil && championOpensWith(def, tierName) {
						line += fmt.Sprintf(" | opener %s[soak %d]", def.OpeningSpell, scaledIncomingDamageReduction(osd, ch))
					}
				}
				t.Log(line)
			}
		}
	}
}

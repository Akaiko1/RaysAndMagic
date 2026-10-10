package game

import (
	"math/rand"
	"slices"
	"ugataima/internal/character"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Extra copies strengthen each trigger, not the authored stack/reduction caps.
// Runtime and collection descriptions share this non-additive rule.
func reactiveCardRules(defs []*config.ItemDefinitionConfig) (pct, limit, step, capPct int) {
	for _, d := range defs {
		if d == nil {
			continue
		}
		pct += d.CardDodgeChargePct
		limit = max(limit, d.CardDodgeChargeLimit)
		step += d.CardRepeatedHitReductionPct
		capPct = max(capPct, d.CardRepeatedHitReductionCap)
	}
	return
}

func (g *MMGame) reactiveCardRules() (pct, limit, step, capPct int) {
	defs := g.cardCollectionDefinitions()
	return reactiveCardRules(defs[:])
}

func (g *MMGame) normalizeReactiveCards() {
	if g.party == nil {
		return
	}
	_, limit, step, _ := g.reactiveCardRules()
	for _, roster := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve, g.party.Captive} {
		for _, ch := range roster {
			if ch == nil {
				continue
			}
			ch.ReactiveCombat.DodgeCharges = min(max(0, ch.ReactiveCombat.DodgeCharges), limit)
			if step == 0 {
				ch.ReactiveCombat.LastAttackerID, ch.ReactiveCombat.RepeatedHits = "", 0
			}
		}
	}
}

func (g *MMGame) gainDodgeCharge(ch *character.MMCharacter) {
	_, limit, _, _ := g.reactiveCardRules()
	ch.ReactiveCombat.DodgeCharges = min(max(0, ch.ReactiveCombat.DodgeCharges)+1, limit)
}

// consumeDodgeCharge touches only a landed primary weapon hit. Cloning leaves
// the immutable splash payload unchanged; spells and secondary riders skip it.
func (cs *CombatSystem) consumeDodgeCharge(attack partyMonsterAttack) partyMonsterAttack {
	if attack.IsSpell || attack.WeaponDef == nil || attack.Attacker == nil {
		return attack
	}
	pct, limit, _, _ := cs.game.reactiveCardRules()
	stacks := min(max(0, attack.Attacker.ReactiveCombat.DodgeCharges), limit)
	attack.Attacker.ReactiveCombat.DodgeCharges = 0
	if stacks == 0 || pct == 0 {
		return attack
	}
	boost := func(p monsterDamagePacket) monsterDamagePacket {
		p.Components = slices.Clone(p.Components)
		for i := range p.Components {
			p.Components[i].Parts.Normal = p.Components[i].Parts.Normal * (100 + pct*stacks) / 100
			p.Components[i].Parts.True = p.Components[i].Parts.True * (100 + pct*stacks) / 100
		}
		return p
	}
	attack.Packet = boost(attack.Packet)
	attack.CriticalPacket = boost(attack.CriticalPacket)
	return attack
}

func (g *MMGame) reactiveHitReduction(source *monsterPkg.Monster3D, target *character.MMCharacter, amount int) int {
	// Terrain, crate traps and friendly splash are damage, not enemy hits.
	// They neither consume the shell nor advance/reset the attacker chain.
	if amount <= 0 || source == nil {
		return amount
	}
	s := &target.ReactiveCombat
	_, _, step, capPct := g.reactiveCardRules()
	if step > 0 && source.ID != "" {
		if s.LastAttackerID != source.ID {
			s.LastAttackerID, s.RepeatedHits = source.ID, 0
		}
		s.RepeatedHits = min(max(0, s.RepeatedHits)+1, (capPct+step-1)/step)
		amount = amount * (100 - min(capPct, s.RepeatedHits*step)) / 100
	} else {
		s.LastAttackerID, s.RepeatedHits = "", 0
	}
	if s.Shell > 0 {
		amount = max(0, amount-s.Shell)
		s.Shell = 0
	}
	return amount
}

// Resolve one landed hit before HP loss, regardless of its attack dispatcher.
func (cs *CombatSystem) resolveHeroHitDamage(source *monsterPkg.Monster3D, target *character.MMCharacter, amount int) int {
	return cs.redirectDamageThroughSacrifice(target, cs.game.reactiveHitReduction(source, target, amount))
}

func (cs *CombatSystem) tryAttackZoneProc(def *config.WeaponDefinitionConfig, owner *character.MMCharacter) {
	if def == nil || def.AttackZoneProc == nil || owner == nil {
		return
	}
	p := def.AttackZoneProc
	if rand.Intn(100) >= p.ChancePct {
		return
	}
	spell, err := spells.GetSpellDefinitionByID(spells.SpellID(p.Spell))
	if err != nil {
		panic(err)
	}
	// A fixed-stat proxy borrows identity, never equipment, buffs or mastery.
	caster := &character.MMCharacter{Name: owner.Name, Intellect: p.Intellect}
	cs.castPersistentDamageZone(spell.ID, spell, caster, true)
}

func (g *MMGame) healingCardCanCleanse(ch *character.MMCharacter) bool {
	return ch.Purifiable() && g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int {
		if d.CardHealingCleanse {
			return 1
		}
		return 0
	}) > 0
}

func (g *MMGame) cleanseHealingRecipient(ch *character.MMCharacter) {
	if g.healingCardCanCleanse(ch) {
		ch.Purify()
	}
}

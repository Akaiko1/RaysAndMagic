package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// cardRule is the active card whose copy of a non-stacking rule applies:
// the highest strength, the first slot on a tie; nil when none authors it.
func (g *MMGame) cardRule(rule config.CardRule) *config.ItemDefinitionConfig {
	defs := g.cardCollectionDefinitions()
	return strongestCard(defs[:], rule)
}

func strongestCard(defs []*config.ItemDefinitionConfig, rule config.CardRule) *config.ItemDefinitionConfig {
	var best *config.ItemDefinitionConfig
	for _, d := range defs {
		if d != nil && rule.Strength(d) > 0 && (best == nil || rule.Strength(d) > rule.Strength(best)) {
			best = d
		}
	}
	return best
}

func relicActorID(ch *character.MMCharacter) uint64 {
	if ch == nil {
		return 0
	}
	if ch.ActorID == 0 {
		ch.ActorID = items.NewInstanceID()
	}
	return ch.ActorID
}

func (g *MMGame) tickRelicMarks(frames int) {
	if g.world == nil {
		return
	}
	for _, m := range g.world.Monsters {
		if m != nil {
			m.Relics.Tick(frames)
		}
	}
}

type wildfireBurst struct {
	x, y   float64
	damage int
	card   *config.ItemDefinitionConfig
}

func (cs *CombatSystem) triggerWildfire(victim *monsterPkg.Monster3D) {
	d := cs.game.cardRule(config.CardWildfire)
	if d == nil || victim == nil || monsterIsPartyAlly(victim) || victim.IsCaravan() ||
		(victim.BurnFramesRemaining <= 0 && !victim.Relics.BurnKilled) {
		return
	}
	cs.wildfireQueue = append(cs.wildfireQueue, wildfireBurst{victim.X, victim.Y, victim.MaxHitPoints * d.CardWildfireHPPct / 100, d})
	if cs.resolvingWildfire {
		return
	}
	cs.resolvingWildfire = true
	defer func() {
		cs.resolvingWildfire = false
		cs.wildfireQueue = cs.wildfireQueue[:0]
	}()
	for index := 0; index < len(cs.wildfireQueue); index++ {
		burst := cs.wildfireQueue[index]
		card, school := burst.card, burst.card.CardWildfireSchool
		attack := cs.newPartyMonsterAttack(burst.damage, 0, school, 0, nil, card.Name, false, false, false)
		cs.game.createSplashImpact(burst.x, burst.y, burst.x, burst.y, card.CardWildfireRadiusTiles*cs.game.config.GetTileSize(), true, false, ElementColors[school])
		cs.forEachAreaVictim(cs.pointBlast(burst.x, burst.y, card.CardWildfireRadiusTiles), partyAreaHurts, func(m *monsterPkg.Monster3D) {
			hit := cs.applyPartyMonsterAttack(m, attack)
			cs.reportIndirectHit(m, hit.Total(), card.Name, school)
			if m.IsAlive() {
				m.ApplyBurn(card.CardWildfireBurnSeconds * cs.game.config.GetTPS())
			} else {
				cs.finishMonsterKillImmediately(m)
			}
		})
	}
}

func (cs *CombatSystem) applyCrumbling(victim *monsterPkg.Monster3D) {
	d := cs.game.cardRule(config.CardCrumbling)
	if d == nil || victim == nil || !victim.IsAlive() || monsterIsPartyAlly(victim) || victim.IsDamageInvulnerable() {
		return
	}
	// The cap follows the victim's current armor (RelicState.Erosion).
	state := &victim.Relics
	state.CrumbleCapPct = d.CardCrumbleCapPct
	state.CrumbleArmor = min(state.CrumbleArmor+d.CardCrumbleArmor, max(0, victim.ArmorClass)*d.CardCrumbleCapPct/100)
}

// stormReversal returns the damage a Perfect Dodge avoided; pierce is the
// dodged hit's own projectile pierce roll, shared with its graze.
func (cs *CombatSystem) stormReversal(source *monsterPkg.Monster3D, target *character.MMCharacter, hit monsterCharacterHit, grazed int, pierce func() bool) {
	d := cs.game.cardRule(config.CardStormReversal)
	// One direct hit on one hero: never a field, nor a blast or arc over the party.
	if d == nil || source == nil || !source.IsAlive() || source.IsDamageInvulnerable() || hit.Hazard || cs.inPartyDamageEvent() ||
		(!hit.Melee && !hit.Projectile) || monsterIsPartyAlly(source) {
		return
	}
	full := cs.mitigateCharacterParts(hit.Parts, hit.DamageType, target, hit.IgnoresArmor, hit.ArmorPiercePct, pierce).Total()
	// A dodge never spends the shell, the chain or Celestial Armor.
	amount := max(0, cs.previewHeroHitDamage(source, target, full)-cs.previewHeroHitDamage(source, target, grazed))
	amount = amount * d.CardStormReversalPct / 100
	if amount <= 0 {
		return
	}
	cs.elementalWeaponBonus(source, amount, d.CardStormReversalSchool, d.Name)
	if !source.IsAlive() {
		cs.finishMonsterKillImmediately(source)
	}
}

type overflowGrant struct {
	recipient *character.MMCharacter
	amount    int
}
type overflowCast struct {
	remaining int
	percent   int
	grants    []overflowGrant
}

func (cs *CombatSystem) beginOverflowCast(cost int) *overflowCast {
	d := cs.game.cardRule(config.CardOverflow)
	if d == nil || cost <= 0 {
		return nil
	}
	return &overflowCast{remaining: cost * d.CardOverflowCapPct / 100, percent: d.CardOverflowPct}
}

func (cs *CombatSystem) queueOverflow(ch *character.MMCharacter, excess int) {
	c := cs.overflow
	if c == nil || ch == nil || excess <= 0 {
		return
	}
	amount := min(c.remaining, min(excess*c.percent/100, max(0, ch.MaxSpellPoints-ch.SpellPoints)))
	if amount > 0 {
		c.remaining -= amount
		c.grants = append(c.grants, overflowGrant{ch, amount})
	}
}

func (c *overflowCast) commit() {
	if c == nil {
		return
	}
	for _, grant := range c.grants {
		grant.recipient.SpellPoints = min(grant.recipient.MaxSpellPoints, grant.recipient.SpellPoints+grant.amount)
	}
}

func (g *MMGame) potionOverflow(ch *character.MMCharacter, excess int) {
	if d := g.cardRule(config.CardOverflow); d != nil && ch != nil && excess > 0 {
		ch.SpellPoints = min(ch.MaxSpellPoints, ch.SpellPoints+excess*d.CardOverflowPct/100)
	}
}

func (g *MMGame) healingPotionHasEffect(ch *character.MMCharacter) bool {
	return ch != nil && (ch.HitPoints < ch.MaxHitPoints || g.healingCardCanCleanse(ch) ||
		(ch.SpellPoints < ch.MaxSpellPoints && g.cardRule(config.CardOverflow) != nil))
}

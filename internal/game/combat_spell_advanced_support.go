package game

import (
	"math"
	"sort"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func (a *partyMonsterAttack) addPersonalSpellPayload(percent int, school monsterPkg.DamageType, vampiric int) {
	a.VampiricPct = vampiric
	if percent <= 0 {
		return
	}
	// Scale each immutable weapon packet before mitigation, including its critical
	// form. The added element is normal damage and uses its own resistance.
	bonus := func(components []monsterDamageComponent) monsterDamageComponent {
		total := 0
		for _, c := range components {
			total += c.Parts.Total()
		}
		return monsterDamageComponent{Parts: damagecalc.Parts{Normal: personalSpellDamageBonus(total, percent)}, School: school}
	}
	a.Packet.Components = append(a.Packet.Components, bonus(a.Packet.Components))
	if len(a.CriticalPacket.Components) > 0 {
		a.CriticalPacket.Components = append(a.CriticalPacket.Components, bonus(a.CriticalPacket.Components))
	}
}

func personalSpellDamageBonus(weaponDamage, percent int) int {
	return weaponDamage * percent / 100
}

func personalSpellPower(c *character.MMCharacter, kind string) int {
	m, tier := personalSpellMechanic(c, kind)
	if m == nil {
		return 0
	}
	return m.Power(tier)
}

func personalSpellDamage(c *character.MMCharacter, kind string) (int, monsterPkg.DamageType) {
	m, tier := personalSpellMechanic(c, kind)
	if m == nil {
		return 0, monsterPkg.DamagePhysical
	}
	return m.Power(tier), convertToMonsterDamageType(m.DamageSchool)
}

func personalSpellMechanic(c *character.MMCharacter, kind string) (*config.SpellMechanicConfig, int) {
	if c == nil {
		return nil, 0
	}
	for _, e := range c.SpellState.Effects {
		if d, ok := config.GetSpellDefinition(e.SpellID); ok && d.Mechanic != nil && d.Mechanic.Kind == kind && (e.Charges > 0 || e.Frames > 0) {
			return d.Mechanic, e.Tier
		}
	}
	return nil, 0
}

// Consume once per committed weapon action, never per victim or projectile leg.
func consumePersonalAttackCharges(c *character.MMCharacter) {
	n := 0
	for _, e := range c.SpellState.Effects {
		if d, ok := config.GetSpellDefinition(e.SpellID); ok && d.Mechanic != nil && len(d.Mechanic.ChargesByMastery) > 0 {
			e.Charges--
			if e.Charges <= 0 {
				continue
			}
		}
		c.SpellState.Effects[n] = e
		n++
	}
	c.SpellState.Effects = c.SpellState.Effects[:n]
}

func (cs *CombatSystem) personalSpellTarget(caster *character.MMCharacter) *character.MMCharacter {
	if cs.spellTarget != nil {
		return cs.spellTarget
	}
	if gl := cs.game.gameLoop; gl != nil && gl.inputHandler != nil {
		x, y := uiCursorPosition()
		if i := gl.inputHandler.getPartyMemberUnderMouse(x, y); i >= 0 {
			return cs.game.party.Members[i]
		}
	}
	return caster
}

func (g *MMGame) advancedBuff(kind string) *TimedCombatBuff {
	for i := range g.combatBuffs {
		b := &g.combatBuffs[i]
		if !b.active() {
			continue
		}
		if d, ok := config.GetSpellDefinition(b.ID); ok && d.Mechanic != nil && d.Mechanic.Kind == kind {
			return b
		}
	}
	return nil
}

// Haste and Quickening share one tempo family; never stack their recovery or
// grant two sets of bonus actions just because different classes cast them.
func (g *MMGame) partyTempoBuff() (TimedCombatBuff, bool) {
	var best TimedCombatBuff
	for _, b := range g.combatBuffs {
		if b.Frames > 0 && (b.RecoveryPct > best.RecoveryPct || b.ExtraActions > best.ExtraActions) {
			best = b
		}
	}
	return best, best.RecoveryPct > 0 || best.ExtraActions > 0
}

func restoreAdvancedBuff(b *TimedCombatBuff, d spells.SpellDefinition) {
	m, tier := d.Mechanic, b.MasteryTier
	if m == nil {
		return
	}
	switch m.Kind {
	case "haste":
		b.RecoveryPct, b.ExtraActions = m.Power(tier), m.ExtraActions(tier)
	case "shield":
		b.RangedWardPct = m.Power(tier)
	case "regeneration":
		b.HPRegenPct = m.Power(tier)
	case "pain_reflection":
		b.ReflectionPct = m.Power(tier)
	}
	if len(m.ChargesByMastery) > 0 {
		b.Charges = min(b.Charges, m.Charges(tier))
	}
	if m.Kind == "celestial_armor" {
		b.Absorb = min(b.Absorb, m.Power(tier))
	}
}

func (cs *CombatSystem) tryCastAdvancedSupport(id spells.SpellID, d spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	m := d.Mechanic
	if m == nil {
		return castNotHandled
	}
	g, tier := cs.game, character.SpellMasteryTier(caster, d)
	frames := d.DurationSeconds(tier) * g.config.GetTPS()
	switch {
	case m.TargetsAlly():
		target := cs.personalSpellTarget(caster)
		if !target.IsLiving() {
			return castRejected
		}
		target.SpellState.Set(character.PersonalSpellEffect{SpellID: string(id), Tier: tier, Charges: m.Charges(tier), Frames: frames})
	case m.IsPartyBuff():
		if m.Kind == "haste" && !g.partyNeedsHaste() {
			g.AddCombatMessage(uitext.Text("technique.refusal_no_benefit"))
			return castRejected
		}
		b := TimedCombatBuff{ID: string(id), Frames: frames, MasteryTier: tier, Charges: m.Charges(tier), DeferFirstTurnTick: g.turnBasedMode && m.Kind == "haste"}
		if m.Kind == "celestial_armor" {
			b.Absorb = m.Power(tier)
		}
		restoreAdvancedBuff(&b, d)
		g.addCombatBuff(b)
		g.refreshCombatBuffStatus(b)
	case m.Kind == "purify" || m.Kind == "dispel_magic":
		if !g.partyNeedsPurify() && (m.Kind != "dispel_magic" || !g.hostileMagicNearby(m.RadiusTiles)) {
			g.AddCombatMessage(uitext.Text("technique.refusal_no_benefit"))
			return castRejected
		}
		g.purifyParty()
		if m.Kind == "dispel_magic" {
			g.dispelHostileProjectiles(m.RadiusTiles)
		}
	case m.Kind == "shared_life":
		if !cs.sharePartyLife(character.SpellHealingBreakdown(d, caster).Total) {
			return castRejected
		}
	case m.Kind == "vvoid_beacon":
		if !g.beaconConfirming {
			return castRejected
		}
	case m.Kind == "identify_monster":
		target := g.identifyConfirming
		if !g.canIdentifyMonster(target, d) {
			return castRejected
		}
		g.identifiedMonster = target
		g.townPortalPickerOpen, g.beaconPicker = true, false
		g.identifySelecting = false
		g.identifyScroll = 0
	default:
		return castNotHandled
	}
	if d.Message != "" {
		g.AddCombatMessage(d.Message)
	}
	cs.playSpellBuffFx(id)
	return castCommitted
}

func (g *MMGame) purifyParty() {
	for _, c := range g.party.Members {
		if c != nil {
			c.Purify()
		}
	}
	g.partyRoot = PartyRootState{}
	g.partyHinder.Slow = 0
}

func (g *MMGame) hostileMagicNearby(radius float64) bool {
	found := false
	g.eachDispellableProjectile(radius, func(*bool, string) bool {
		found = true
		return false
	})
	return found
}

// eachDispellableProjectile visits hostile magic in flight within radius tiles
// (spells and magical arrows) until visit returns false.
func (g *MMGame) eachDispellableProjectile(radius float64, visit func(active *bool, id string) bool) {
	limit := radius * float64(g.config.GetTileSize())
	for i := range g.magicProjectiles {
		p := &g.magicProjectiles[i]
		if g.dispellableProjectile(p.Active, p.LifeTime, p.Owner, true, p.X, p.Y, limit) && !visit(&p.Active, p.ID) {
			return
		}
	}
	for i := range g.arrows {
		p := &g.arrows[i]
		if g.dispellableProjectile(p.Active, p.LifeTime, p.Owner, !isPhysicalDamageType(p.DamageType), p.X, p.Y, limit) && !visit(&p.Active, p.ID) {
			return
		}
	}
}

// A hostile spell is magical even when its damage school is physical. Weapon
// projectiles instead use their authored damage element to distinguish bolts
// from ordinary arrows. Friendly, reflected, and expired shots are untouched.
func (g *MMGame) dispellableProjectile(active bool, lifetime int, owner ProjectileOwner, magical bool, x, y, limit float64) bool {
	return active && lifetime > 0 && magical &&
		(owner == ProjectileOwnerMonster || owner == ProjectileOwnerMonsterAtActor) &&
		math.Hypot(x-g.camera.X, y-g.camera.Y) <= limit
}

func (g *MMGame) dispelHostileProjectiles(radius float64) {
	g.eachDispellableProjectile(radius, func(active *bool, id string) bool {
		*active = false
		g.collisionSystem.UnregisterEntity(id)
		return true
	})
}

// Divide a finite healing pool without wasting shares on full or fallen allies.
func (cs *CombatSystem) distributePartyHealing(pool int) {
	var indices []int
	for i, c := range cs.game.party.Members {
		if c.IsLiving() {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := cs.game.party.Members[indices[i]], cs.game.party.Members[indices[j]]
		return a.MaxHitPoints-a.HitPoints < b.MaxHitPoints-b.HitPoints
	})
	for i, index := range indices {
		c := cs.game.party.Members[index]
		heal := min(max(0, c.MaxHitPoints-c.HitPoints), pool/(len(indices)-i))
		if heal > 0 {
			cs.healMember(index, heal)
			pool -= heal
		}
	}
}

func (cs *CombatSystem) sharePartyLife(bonus int) bool {
	var living []*character.MMCharacter
	pool := bonus
	for _, c := range cs.game.party.Members {
		if c.IsLiving() {
			living = append(living, c)
			pool += c.HitPoints
		}
	}
	if len(living) == 0 {
		return false
	}
	// Water filling caps small heroes without discarding the overflow owed to
	// larger ones. Sorting affects only the local view, never party order.
	sort.SliceStable(living, func(i, j int) bool { return living[i].MaxHitPoints < living[j].MaxHitPoints })
	for i, c := range living {
		hp := min(c.MaxHitPoints, pool/(len(living)-i))
		c.HitPoints = hp
		pool -= hp
	}
	return true
}

// One AoE damage event shares one ward decision across all its victims. Nested
// damage loops inherit that decision; the next event starts a fresh transaction.
func (cs *CombatSystem) partyDamageEvent() func() {
	cs.magicWardDepth++
	if cs.magicWardDepth == 1 {
		cs.magicWardDecision = 0
	}
	return func() {
		cs.magicWardDepth--
		if cs.magicWardDepth == 0 {
			cs.magicWardDecision = 0
		}
	}
}

// magicWardBlocks spends Protection from Magic's shared budget. By design,
// eligible hostile hits include spells (even physical-damage spells) AND
// nonphysical monster hits: elemental melee, elemental weapon projectiles and
// monster trap hits. This is broader than the Spell flag used for absorption;
// do not narrow eligibility to spell casts or exclude melee/hazard delivery.
// Hostile spell and monster-owned elemental eligibility are classified here,
// so direct hits, AoE packets and their control riders cannot diverge.
func (cs *CombatSystem) magicWardBlocks(school string, hostileSpell, monsterOwned bool) bool {
	if !hostileSpell && !(monsterOwned && normalizeDamageTypeStr(school) != "physical") {
		return false
	}
	if cs.magicWardDepth > 0 && cs.magicWardDecision != 0 {
		return cs.magicWardDecision == 1
	}
	blocked := cs.game.spendPartyBuffCharge("protection_from_magic")
	if blocked {
		cs.game.AddCombatMessage(uitext.Text("advanced.magic_blocked"))
	}
	if cs.magicWardDepth > 0 {
		cs.magicWardDecision = -1
		if blocked {
			cs.magicWardDecision = 1
		}
	}
	return blocked
}

func (g *MMGame) preserveFromLethal(c *character.MMCharacter, damage int) int {
	if c.HitPoints <= 0 || damage < c.HitPoints {
		return damage
	}
	if g.spendPreservation(c) {
		return max(0, c.HitPoints-1)
	}
	return damage
}

func (g *MMGame) spendPreservation(c *character.MMCharacter) bool {
	if !g.spendPartyBuffCharge("preservation") {
		return false
	}
	c.CureDamageOverTime()
	g.AddCombatMessage(uitext.Text("advanced.preserved", c.Name))
	return true
}

// spendPartyBuffCharge spends one charge of a party buff; its last charge ends it.
func (g *MMGame) spendPartyBuffCharge(id string) bool {
	b := g.advancedBuff(id)
	if b == nil || b.Charges <= 0 {
		return false
	}
	b.Charges--
	if b.Charges == 0 {
		b.Frames = 0
	}
	return true
}

func (g *MMGame) absorbCelestialDamage(amount int) int {
	if amount <= 0 {
		return amount
	}
	if b := g.advancedBuff("celestial_armor"); b != nil && b.Absorb > 0 {
		absorbed := min(amount, b.Absorb)
		b.Absorb -= absorbed
		amount -= absorbed
		if b.Absorb == 0 {
			b.Frames = 0
		}
	}
	return amount
}

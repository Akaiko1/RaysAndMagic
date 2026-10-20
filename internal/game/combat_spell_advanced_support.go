package game

import (
	"math"
	"slices"
	"sort"
	"ugataima/internal/status"

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

func personalSpellPower(c *character.MMCharacter, kind config.MechanicKind) int {
	m, tier := personalSpellMechanic(c, kind)
	if m == nil {
		return 0
	}
	return m.Power(tier)
}

func personalSpellDamage(c *character.MMCharacter, kind config.MechanicKind) (int, monsterPkg.DamageType) {
	m, tier := personalSpellMechanic(c, kind)
	if m == nil {
		return 0, monsterPkg.DamagePhysical
	}
	return m.Power(tier), convertToMonsterDamageType(m.DamageSchool)
}

// fateGuided reports whether Fate guides the hero's weapon strikes past
// Perfect Dodge.
func fateGuided(c *character.MMCharacter) bool {
	m, _ := personalSpellMechanic(c, config.MechanicFate)
	return m != nil
}

func personalSpellMechanic(c *character.MMCharacter, kind config.MechanicKind) (*config.SpellMechanicConfig, int) {
	if c == nil {
		return nil, 0
	}
	for _, e := range c.SpellState.Effects {
		if d, ok := config.GetSpellDefinition(e.SpellID); ok && d.Mechanic != nil && d.Mechanic.Kind == kind && (e.Charges > 0 || e.Clock.Active()) {
			return d.Mechanic, e.Tier
		}
	}
	return nil, 0
}

// consumePersonalAttackCharges spends one charge of every charged weapon
// empowerment (Fire Aura, Fate), never per victim or projectile leg: a swing
// when it lands, a shot at its release (chargeShots refunds a shot that lands
// on no one). It returns the effects as they stood before.
func consumePersonalAttackCharges(c *character.MMCharacter) []character.PersonalSpellEffect {
	var spent []character.PersonalSpellEffect
	n := 0
	for _, e := range c.SpellState.Effects {
		if d, ok := config.GetSpellDefinition(e.SpellID); ok && d.Mechanic != nil && len(d.Mechanic.ChargesByMastery) > 0 {
			spent = append(spent, e)
			e.Charges--
			if e.Charges <= 0 {
				continue
			}
		}
		c.SpellState.Effects[n] = e
		n++
	}
	c.SpellState.Effects = c.SpellState.Effects[:n]
	return spent
}

// refundAttackCharges gives back the charge of each spent effect, up to the
// full count of the effect now held; a spent-out effect returns with one
// charge.
func refundAttackCharges(state *character.SpellState, spent []character.PersonalSpellEffect) {
	for _, e := range spent {
		i := slices.IndexFunc(state.Effects, func(held character.PersonalSpellEffect) bool { return held.SpellID == e.SpellID })
		if i < 0 {
			e.Charges = 1
			state.Set(e)
			continue
		}
		held := &state.Effects[i]
		full := held.Charges + 1
		if d, ok := config.GetSpellDefinition(held.SpellID); ok && d.Mechanic != nil {
			full = d.Mechanic.Charges(held.Tier)
		}
		held.Charges = min(held.Charges+1, full)
	}
}

// shotCharges is one ranged action's spent attack charges, shared by every
// shot it loosed and every leg they spawn: they stay spent once any of them
// strikes (strikeCarried), and come back when the last leaves play having
// struck no one.
type shotCharges struct {
	hero   *character.MMCharacter
	spent  []character.PersonalSpellEffect
	shots  int
	landed bool
}

// chargeShots spends a ranged action's attack charges at the release and
// hands the account to its shots.
func (cs *CombatSystem) chargeShots(hero *character.MMCharacter, shots []Arrow) {
	spent := consumePersonalAttackCharges(hero)
	if len(spent) == 0 {
		return
	}
	account := &shotCharges{hero: hero, spent: spent}
	for i := range shots {
		shots[i].Charges = account
		account.shots++
	}
}

// settleShotCharges closes the accounts of shots leaving play (all of them,
// or only the spent ones).
func (g *MMGame) settleShotCharges(all bool) {
	for i := range g.arrows {
		ar := &g.arrows[i]
		if ar.Charges == nil || !all && ar.flying() {
			continue
		}
		account := ar.Charges
		ar.Charges = nil
		if account.shots--; account.shots == 0 && !account.landed {
			refundAttackCharges(&account.hero.SpellState, account.spent)
		}
	}
}

// refundShotsInFlight gives a saved hero back the charges of shots still in
// flight that have struck no one: a save keeps no projectiles, so in the
// loaded game those shots never flew.
func (g *MMGame) refundShotsInFlight(hero *character.MMCharacter, state *character.SpellState) {
	seen := map[*shotCharges]bool{}
	for i := range g.arrows {
		account := g.arrows[i].Charges
		if account == nil || account.hero != hero || account.landed || seen[account] {
			continue
		}
		seen[account] = true
		refundAttackCharges(state, account.spent)
	}
}

// strikeCarried is whether a weapon strike counts as landed for its attack
// charges: it got past the target's Perfect Dodge, or a Grandmaster's graze
// carried its damage through it.
func strikeCarried(dodged, through, grazes bool) bool {
	return !dodged || through && grazes
}

// noteStrike marks the shot's charge account when the strike counts.
func (ar *Arrow) noteStrike(dodged, through, grazes bool) {
	if ar.Charges != nil && strikeCarried(dodged, through, grazes) {
		ar.Charges.landed = true
	}
}

func (g *MMGame) advancedBuff(kind config.MechanicKind) *TimedCombatBuff {
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
		if b.Clock.Active() && (b.RecoveryPct > best.RecoveryPct || b.ExtraActions > best.ExtraActions) {
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
	case config.MechanicHaste:
		b.RecoveryPct, b.ExtraActions = m.Power(tier), m.ExtraActions(tier)
	case config.MechanicShield:
		b.RangedWardPct = m.Power(tier)
	case config.MechanicRegeneration:
		b.HPRegenPct = m.Power(tier)
	case config.MechanicPainReflection:
		b.ReflectionPct = m.Power(tier)
	}
	if len(m.ChargesByMastery) > 0 {
		b.Charges = min(b.Charges, m.Charges(tier))
	}
	if m.Kind == config.MechanicCelestialArmor {
		b.Absorb = min(b.Absorb, m.Power(tier))
	}
}

func (cs *CombatSystem) tryCastAdvancedSupport(id spells.SpellID, d spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	m := d.Mechanic
	if m == nil {
		return castNotHandled
	}
	g, tier := cs.game, character.SpellMasteryTier(caster, d)
	frames := cs.CalculateSpellDurationFrames(id, caster)
	switch {
	case m.TargetsAlly():
		// The hero picked in the paused target picker (castSpell opens it
		// before anything is paid).
		target := cs.spellTarget
		if target == nil || !target.IsLiving() {
			return castRejected
		}
		effect := character.PersonalSpellEffect{SpellID: string(id), Tier: tier, Charges: m.Charges(tier)}
		effect.Clock.ArmFrames(frames, g.config.GetTPS())
		target.SpellState.Set(effect)
	case m.IsPartyBuff():
		if m.Kind == config.MechanicHaste && !g.partyNeedsHaste() {
			g.AddCombatMessage(uitext.Text("technique.refusal_no_benefit"))
			return castRejected
		}
		b := TimedCombatBuff{ID: string(id), MasteryTier: tier, Charges: m.Charges(tier), DeferFirstTurnTick: g.turnBasedMode && m.Kind == config.MechanicHaste}
		b.Clock.ArmFrames(frames, g.config.GetTPS())
		if m.Kind == config.MechanicCelestialArmor {
			b.Absorb = m.Power(tier)
		}
		restoreAdvancedBuff(&b, d)
		g.addCombatBuff(b)
	case m.Kind == config.MechanicPurify || m.Kind == config.MechanicDispelMagic:
		if !g.partyNeedsPurify() && (m.Kind != config.MechanicDispelMagic || !g.hostileMagicNearby(m.RadiusTiles)) {
			g.AddCombatMessage(uitext.Text("technique.refusal_no_benefit"))
			return castRejected
		}
		g.purifyParty()
		if m.Kind == config.MechanicDispelMagic {
			g.dispelHostileProjectiles(m.RadiusTiles)
		}
	case m.Kind == config.MechanicSharedLife:
		if !cs.sharePartyLife(character.SpellHealingBreakdown(d, caster).Total) {
			return castRejected
		}
	case m.Kind == config.MechanicVvoidBeacon:
		if !g.beaconConfirming {
			return castRejected
		}
	case m.Kind == config.MechanicIdentifyMonster:
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
	g.partyRoot = status.DualClock{}
	g.partyHinder.Slow = status.DualClock{}
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
	// Redistribution uses all available HP before any healing is wasted. Only
	// the remaining pool is excess healing, shared among the living recipients.
	for i, c := range living {
		excess := max(0, pool) / (len(living) - i)
		cs.queueOverflow(c, excess)
		pool -= excess
	}
	return true
}

// partyDamageEvent opens one damage event over the whole party (a blast, an
// arc, a field): its victims share one Magic Ward decision, and per-hit
// reactions such as Storm Reversal stay out of it (inPartyDamageEvent).
// Nested damage loops join the open event; the next event starts fresh.
func (cs *CombatSystem) partyDamageEvent() func() {
	cs.partyEventDepth++
	if cs.partyEventDepth == 1 {
		cs.magicWardDecision = 0
	}
	return func() {
		cs.partyEventDepth--
		if cs.partyEventDepth == 0 {
			cs.magicWardDecision = 0
		}
	}
}

// inPartyDamageEvent reports a hit landing as part of a party-wide event.
func (cs *CombatSystem) inPartyDamageEvent() bool { return cs.partyEventDepth > 0 }

// magicWardBlockedEvent reports that the ward blocked the party-wide event in
// progress.
func (cs *CombatSystem) magicWardBlockedEvent() bool {
	return cs.inPartyDamageEvent() && cs.magicWardDecision == 1
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
	if cs.inPartyDamageEvent() && cs.magicWardDecision != 0 {
		return cs.magicWardDecision == 1
	}
	blocked := cs.game.spendPartyBuffCharge(config.MechanicProtectionFromMagic)
	if blocked {
		cs.game.AddCombatMessage(uitext.Text("advanced.magic_blocked"))
	}
	if cs.inPartyDamageEvent() {
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
	if !g.spendPartyBuffCharge(config.MechanicPreservation) {
		return false
	}
	c.CureDamageOverTime()
	g.AddCombatMessage(uitext.Text("advanced.preserved", c.Name))
	return true
}

// spendPartyBuffCharge spends one charge of a party buff; its last charge ends it.
func (g *MMGame) spendPartyBuffCharge(kind config.MechanicKind) bool {
	b := g.advancedBuff(kind)
	if b == nil || b.Charges <= 0 {
		return false
	}
	// The last charge ends the buff at once, in any mode.
	if b.Charges--; b.Charges == 0 {
		g.removeCombatBuff(buffRegistryID(*b))
	}
	return true
}

// celestialAbsorb is what Celestial Armor would take from a hit of amount.
func (g *MMGame) celestialAbsorb(amount int) int {
	if b := g.advancedBuff(config.MechanicCelestialArmor); amount > 0 && b != nil && b.Absorb > 0 {
		return min(amount, b.Absorb)
	}
	return 0
}

func (g *MMGame) absorbCelestialDamage(amount int) int {
	absorbed := g.celestialAbsorb(amount)
	if absorbed > 0 {
		b := g.advancedBuff(config.MechanicCelestialArmor)
		if b.Absorb -= absorbed; b.Absorb == 0 {
			g.removeCombatBuff(buffRegistryID(*b))
		}
	}
	return amount - absorbed
}

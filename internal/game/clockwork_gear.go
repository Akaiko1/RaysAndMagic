package game

import (
	"maps"
	"math/rand"
	"slices"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
)

// Clock Tower gear: every piece works on the fight's clock. Each timer is a
// status.DualClock, ticked by tickClockworkGear - a frame per frame in real
// time, a turn at each turn-based party turn - so one rule covers both modes.

// pendingToll is a Chime Maul hit ringing again: the landed attack scaled to
// the echo share, re-resolved against the target's defenses when it rings.
// Transient: a map change or load drops it, like a mortar in flight.
type pendingToll struct {
	Target *monsterPkg.Monster3D
	Attack partyMonsterAttack
	School string
	Clock  status.DualClock
}

// scaleMonsterPacket scales every component of a party damage packet.
func scaleMonsterPacket(p monsterDamagePacket, pct int) monsterDamagePacket {
	p.Components = slices.Clone(p.Components)
	for i := range p.Components {
		p.Components[i].Parts.Normal = p.Components[i].Parts.Normal * pct / 100
		p.Components[i].Parts.True = p.Components[i].Parts.True * pct / 100
	}
	return p
}

// scheduleToll sets a landed weapon hit ringing again on its target one round
// later for the weapon's echo share (Chime Maul).
func (cs *CombatSystem) scheduleToll(m *monsterPkg.Monster3D, landed partyMonsterAttack, def *config.WeaponDefinitionConfig) {
	echo := landed
	echo.Packet = scaleMonsterPacket(landed.Packet, def.TollEchoPct)
	echo.CriticalPacket, echo.DesignationBonuses = monsterDamagePacket{}, nil
	echo.VampiricPct, echo.PrimaryHit = 0, false
	toll := pendingToll{Target: m, Attack: echo, School: weaponDamageTypeStr(def)}
	toll.Clock.Arm(config.CombatRoundSeconds, cs.game.config.GetTPS())
	cs.game.pendingTolls = append(cs.game.pendingTolls, toll)
}

func (g *MMGame) tickPendingTolls(turn bool) {
	if len(g.pendingTolls) == 0 {
		return
	}
	var ripe []pendingToll
	kept := g.pendingTolls[:0]
	for _, t := range g.pendingTolls {
		if t.Clock.Tick(turn) {
			ripe = append(ripe, t)
		} else {
			kept = append(kept, t)
		}
	}
	g.pendingTolls = kept
	for _, t := range ripe {
		g.combat.ringToll(t)
	}
}

func (cs *CombatSystem) ringToll(t pendingToll) {
	m := t.Target
	if m == nil || !m.IsAlive() || !slices.Contains(cs.game.world.Monsters, m) || cs.absorbIfSealed(m) {
		return
	}
	hit := cs.applyPartyMonsterAttack(m, t.Attack)
	cs.markMonsterHit(m)
	name := "Toll"
	if a := t.Attack.Attacker; a != nil {
		name = a.Name + "'s toll"
	}
	cs.game.reportDamage(logToneGood, damageLog{Source: logHeroText(name), Target: monsterLogTarget(m), Amount: hit.Total(), School: t.School})
	if !m.IsAlive() {
		cs.finishWeaponKill(m, nil, t.Attack.Attacker)
	}
}

// windClock is a weapon's Wind-up clock on the one RT/TB clock.
func windClock(state *items.WeaponSignatureState) *status.DualClock {
	return (*status.DualClock)(&state.Wind)
}

// spendWindUp fires a Wind-up weapon's wound springs as extra bullets and
// restarts its winding: each full round the weapon spends unfired winds a
// spring. A turn-based shot spends the round it is fired in, so its winding
// starts a round later.
func spendWindUp(ch *character.MMCharacter, slot items.EquipSlot, def *config.WeaponDefinitionConfig, turnBased bool, tps int) int {
	if ch == nil || !def.HasSignature(config.SignatureWindUp) {
		return 0
	}
	item := ch.Equipment[slot]
	springs := max(0, item.Signature.Charges)
	item.Signature.Charges, item.Signature.Wind = 0, items.RoundClock{}
	rest := config.CombatRoundSeconds
	if turnBased {
		rest *= 2
	}
	windClock(&item.Signature).Arm(rest, tps)
	ch.Equipment[slot] = item
	return springs
}

// windUpWeapons winds every equipped Wind-up weapon a spring per round.
func windUpWeapons(ch *character.MMCharacter, turn bool, tps int) {
	defs := equippedWeaponDefinitions(ch)
	for i, slot := range equippedWeaponSlots {
		def := defs[i]
		if !def.HasSignature(config.SignatureWindUp) {
			continue
		}
		item := ch.Equipment[slot]
		state := &item.Signature
		if state.Charges >= def.Signature.ChargeLimit {
			state.Wind = items.RoundClock{}
		} else if windClock(state).Every(config.CombatRoundSeconds, tps, turn) {
			state.Charges++
		}
		ch.Equipment[slot] = item
	}
}

// tickClockworkGear spends one tick of the Clock Tower clocks - a frame in
// real time, a turn at a turn-based party turn: tolls, the opener cooldown,
// the hold-your-ground clock, wind-up springs, the Chrono Cape rewind and the
// Chainwork pendulum. The fight flag is the one updateFightOpeners keeps.
func (g *MMGame) tickClockworkGear(turn bool) {
	if g.party == nil || g.combat == nil {
		return
	}
	tps := g.config.GetTPS()
	g.tickPendingTolls(turn)
	inFight := g.partyWasInFight
	if !inFight {
		g.openerCooldown.Tick(turn)
	}
	g.partyMoveClock.Tick(turn)
	for _, ch := range g.party.Members {
		if ch == nil {
			continue
		}
		windUpWeapons(ch, turn, tps)
		g.tickRewind(ch, turn, tps, inFight)
		if ch.Clockwork.Pendulum.Tick(turn) && inFight {
			g.combat.castPendulumSpell(ch)
		}
	}
}

// tickRewind is the Chrono Cape: in a fight, at each round's lapse a wearer
// whose cooldown is ready gets back the HP lost during that round; out of a
// fight, or unable to act, the snapshot just follows HP.
func (g *MMGame) tickRewind(ch *character.MMCharacter, turn bool, tps int, inFight bool) {
	st := &ch.Clockwork
	st.RewindCD.Tick(turn)
	cooldown := ch.RewindCooldownSeconds()
	if cooldown == 0 || !inFight || !ch.CanAct() {
		st.RewindHP, st.RewindRound = ch.HitPoints, status.DualClock{}
		return
	}
	if !st.RewindRound.Every(config.CombatRoundSeconds, tps, turn) {
		return
	}
	if restored := min(st.RewindHP, ch.MaxHitPoints) - ch.HitPoints; restored > 0 && !st.RewindCD.Active() {
		ch.HitPoints += restored
		st.RewindCD.Arm(cooldown, tps)
		g.logCombatLine(logToneGood, uitext.Text("combat.s_cape_rewinds_a_round_hp", logSlot, logSlot), logHeroText(ch.Name), logHealed(restored))
	}
	st.RewindHP = ch.HitPoints
}

// armPendulum starts the Chainwork swing-back after a committed heal in a
// fight: one round later a random offensive spell the healer knows casts free.
func (g *MMGame) armPendulum(caster *character.MMCharacter, def spells.SpellDefinition) {
	if caster == nil || !def.IsHeal() || caster.Clockwork.Pendulum.Active() || !caster.SetHealFollowUpSpell() || !g.partyInCombat() {
		return
	}
	caster.Clockwork.Pendulum.Arm(config.CombatRoundSeconds, g.config.GetTPS())
}

// castPendulumSpell casts one random offensive spell the hero knows, free of
// SP, through the shared free-cast path; spells it cannot cast now (reuse,
// the open sky) are skipped.
func (cs *CombatSystem) castPendulumSpell(ch *character.MMCharacter) {
	if !ch.CanUseCombatAction() {
		return
	}
	var pool []spells.SpellDefinition
	for _, school := range slices.Sorted(maps.Keys(ch.MagicSchools)) {
		for _, id := range ch.MagicSchools[school].KnownSpells {
			if def, err := spells.GetSpellDefinitionByID(id); err == nil && def.IsOffensive() && cs.spellCastableNow(ch, def, 0) {
				pool = append(pool, def)
			}
		}
	}
	if len(pool) == 0 {
		return
	}
	def := pool[rand.Intn(len(pool))]
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.s_pendulum_swings_back", logSlot, logSlot), logHeroText(ch.Name), logSchoolWord(def.School, def.Name))
	cs.castResolvedSpell(def.ID, def, ch, 0, false, false)
}

// partyMovedClock records that the party moved, for the hold-your-ground rule.
func (g *MMGame) partyMovedClock() {
	g.partyMoveClock.Arm(config.CombatRoundSeconds, g.config.GetTPS())
}

// partyHeldGround: the party has not moved within the last round.
func (g *MMGame) partyHeldGround() bool { return !g.partyMoveClock.Active() }

// heldGroundReduction is the Clockplate: an enemy hit on a wearer whose party
// held its ground this round loses the set's share.
func (g *MMGame) heldGroundReduction(source *monsterPkg.Monster3D, target *character.MMCharacter, amount int) int {
	if source == nil || amount <= 0 {
		return amount
	}
	pct := target.SetStationaryDamageReductionPct()
	if pct <= 0 || !g.partyHeldGround() {
		return amount
	}
	return amount * (100 - pct) / 100
}

// updateFightOpeners watches the party enter and leave a fight. Entering one
// arms the Cogleather opener for each wearer: mid party turn in turn-based it
// grants its actions at once; otherwise it waits for the next party turn (TB)
// or the wearer's first weapon attack (RT, no recovery). Leaving the fight
// drops an unspent opener and starts a round's cooldown, spent only out of a
// fight: stepping out of reach and straight back is not a new fight.
func (g *MMGame) updateFightOpeners() {
	if g.party == nil {
		return
	}
	in := g.partyInCombat()
	if in == g.partyWasInFight {
		return
	}
	g.partyWasInFight = in
	rested := !g.openerCooldown.Active()
	if !in {
		g.openerCooldown.Arm(config.CombatRoundSeconds, g.config.GetTPS())
	}
	for _, ch := range g.party.Members {
		if ch == nil {
			continue
		}
		ch.Clockwork.QuickDraw = false
		n := ch.SetFightOpenerActions()
		if !in || n <= 0 || !rested {
			continue
		}
		if g.turnBasedMode && g.currentTurn == 0 && ch.CanUseCombatAction() {
			ch.ActionsRemaining += n
			continue
		}
		ch.Clockwork.QuickDraw = true
	}
}

// grantTurnBasedOpener spends a waiting Cogleather opener at a party turn.
func grantTurnBasedOpener(ch *character.MMCharacter) {
	if !ch.Clockwork.QuickDraw || !ch.CanUseCombatAction() {
		return
	}
	ch.Clockwork.QuickDraw = false
	ch.ActionsRemaining += ch.SetFightOpenerActions()
}

// weaponBuffDurationPct is the caster's buff_duration_pct (the Escapement
// Staff), the perk for the buffs they cast.
func weaponBuffDurationPct(char *character.MMCharacter) int {
	if def := strongestWeaponPerk(char, func(d *config.WeaponDefinitionConfig) float64 { return float64(d.BuffDurationPct) }); def != nil {
		return def.BuffDurationPct
	}
	return 0
}

// lengthenBuff applies the caster's buff-duration perk to a non-offensive
// spell's duration, rounded up to whole rounds (durations are whole rounds).
func lengthenBuff(def spells.SpellDefinition, char *character.MMCharacter, seconds int) int {
	pct := weaponBuffDurationPct(char)
	if pct <= 0 || seconds <= 0 || def.IsOffensive() {
		return seconds
	}
	return config.CombatRounds((seconds*(100+pct)+99)/100) * config.CombatRoundSeconds
}

// castersDurations is a spell's duration ladder as this caster casts it, for
// the cards: the same lengthenBuff the cast applies.
func castersDurations(def spells.SpellDefinition, char *character.MMCharacter) []int {
	out := slices.Clone(def.DurationByMastery)
	for i, seconds := range out {
		out[i] = lengthenBuff(def, char, seconds)
	}
	return out
}

// returnMisfitEquipment moves every worn item that no longer belongs in its
// slot (ItemBelongsInSlot - say a weapon that became a staff in an off hand or
// for a hero without the skill) into the hero's own bag, with a log line once
// the loaded game starts. The quick-action slot keeps its own rules. Reports
// whether anything moved.
func (g *MMGame) returnMisfitEquipment(m *character.MMCharacter) bool {
	moved := false
	for _, slot := range items.DisplayEquipSlots {
		item, worn := m.Equipment[slot]
		if !worn || m.ItemBelongsInSlot(item, slot) {
			continue
		}
		delete(m.Equipment, slot)
		m.Inventory = append(m.Inventory, item)
		g.loadNotices = append(g.loadNotices, uitext.Text("save.gear_back_in_bag", m.Name, item.Name))
		moved = true
	}
	return moved
}

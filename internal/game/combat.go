package game

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/mathutil"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
	"ugataima/internal/world"
)

// CombatSystem handles all combat-related functionality
type CombatSystem struct {
	runeSwing            *runeSwing
	wildfireQueue        []wildfireBurst
	resolvingWildfire    bool
	overflow             *overflowCast
	spellTarget          *character.MMCharacter
	castHovered          *character.MMCharacter // the hero under the cursor when a hand cast began (book and quick-bar heals); nil for none or an automatic cast
	strikeLanded         bool                   // the weapon action under way reached a target past its dodge
	partyEventDepth      int
	magicWardDecision    int
	deviceClock          deviceCooldownSweep
	game                 *MMGame
	racialProcRoll       func(int) bool
	elementalAttackRoll  func() float64
	projectilePierceRoll func(int) int // physicalProjectilePierces' dice (rand.Intn) for tests
	designationRoll      func(int) int
	partyAimTarget       *monsterPkg.Monster3D
	phase                phaseScratch // Phase Shift planner buffers
	partyAimAngle        float64
	reactionRoll         func() float64
	controlSaveRoll      func() float64
	monsterAction        *monsterActionObservation
}

// Owner namespaces of the pure party allies. They persist through
// MonsterSave.SummonedBy and identify the no-reward/map-exit lifecycle
// independently of the summoner's display name; each spell also counts only its
// own allies against summon_max.
const (
	animalBondingOwnerPrefix = "animal_bonding:"
	spellSummonOwnerPrefix   = "spell:"
)

// NewCombatSystem creates a new combat system
func NewCombatSystem(game *MMGame) *CombatSystem {
	return &CombatSystem{game: game}
}

func (cs *CombatSystem) rollRacialProc(chancePct int) bool {
	if cs != nil && cs.racialProcRoll != nil {
		return cs.racialProcRoll(chancePct)
	}
	return chancePct > 0 && rand.Intn(100) < chancePct
}

// knockOut handles a member reaching 0 HP from a hit: the Lich Card may cheat
// death (restore half HP + half SP), otherwise the member falls unconscious.
// Single chokepoint so the save applies to every lethal branch alike.
func (cs *CombatSystem) knockOut(target *character.MMCharacter) {
	// Character-owned periodic damage reaches this sink after clamping HP.
	// Preserve its last point of life, without the Lich card's healing burst.
	if cs.game.spendPreservation(target) {
		target.HitPoints = 1
		return
	}
	if pct := cs.game.cardLethalSavePct(); pct > 0 && rand.Intn(100) < pct {
		reviveHalf(target)
		cs.game.logCombatLine(logToneGood, uitext.Text("combat.cheats_death_lich_card", logSlot), logHeroName(target))
		return
	}
	target.AddCondition(character.ConditionUnconscious)
	target.ClearReactiveCombat()
	cs.game.logCombat(logToneBad, "%s %s!", logHeroName(target), logColored("falls unconscious", logDyingHP))
}

// knockOutLethalDoTVictims finds party members poison/burn ticked to 0 HP this
// frame and routes them through the real knockOut (Lich Card save + message) -
// updatePoison/updateBurn only clamp HP, they can't call knockOut themselves
// (character can't import game).
func (cs *CombatSystem) knockOutLethalDoTVictims() {
	// Scale stacks shed only when nothing on the map is engaging the party.
	// The nearby-interaction combat check is too narrow for ranged bosses, so a
	// distant hit must not grant a stack that evaporates on the next frame.
	if !cs.game.partyEngaged() {
		cs.game.clearPartyScaleStacks()
	}
	for _, m := range cs.game.party.Members {
		if m == nil || m.HitPoints > 0 {
			continue
		}
		if m.HasCondition(character.ConditionUnconscious) || m.HasCondition(character.ConditionEradicated) || m.HasCondition(character.ConditionDead) {
			continue
		}
		cs.knockOut(m)
	}
}

// reviveHalf restores a downed member to half max HP and SP (Lich Card save).
func reviveHalf(target *character.MMCharacter) {
	if hp := target.MaxHitPoints / 2; hp > target.HitPoints {
		target.HitPoints = hp
	}
	if sp := target.MaxSpellPoints / 2; sp > target.SpellPoints {
		target.SpellPoints = sp
	}
}

// tryCardHealOnAttack rolls the Ningyo Card's self-heal when the active member
// attacks (chance and amount both stack across copies).
func (cs *CombatSystem) tryCardHealOnAttack() {
	pct := cs.game.cardHealOnAttackPct()
	if pct <= 0 || rand.Intn(100) >= pct {
		return
	}
	amt := cs.game.cardHealAmount()
	idx := cs.game.selectedChar
	if amt <= 0 || idx < 0 || idx >= len(cs.game.party.Members) {
		return
	}
	if m := cs.game.party.Members[idx]; m != nil && m.HitPoints > 0 {
		cs.healMember(idx, amt)
		cs.game.logCombatLine(logToneGood, uitext.Text("combat.s_ningyo_card_mends_hp", logSlot, logSlot), logHeroName(m), logHealed(amt))
	}
}

// tryCardFireBoltInstead casts a free Fire Bolt through the real spell-casting
// path (Pixie Card) - same projectile, damage formula (Intellect-scaled) and
// crit roll as an actual cast, just spellCost 0 so no SP is spent. Returns
// false (and leaves the caller to do a normal swing) if the spell is missing.
func (cs *CombatSystem) tryCardFireBoltInstead(caster *character.MMCharacter) bool {
	spellID := spells.SpellID("firebolt")
	spellDef, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	return cs.castResolvedSpell(spellID, spellDef, caster, 0, false, true)
}

// tryCardMoveBurst rolls the Gorilla Titan Card's on-move shockwave: physical
// true damage to monsters next to the party. Called once per tile stepped into.
func (cs *CombatSystem) tryCardMoveBurst() {
	pct := cs.game.cardMoveAoePct()
	if pct <= 0 || rand.Intn(100) >= pct {
		return
	}
	radiusTiles := cs.game.cardMoveAoeRadiusTiles()
	cs.cardMoveBurstApply(cs.game.cardMoveAoeDmg(), radiusTiles)
	// Play the existing ground FX on every successful roll, even over empty
	// ground. This is only presentation, not an Earthquake spell cast.
	cs.game.spawnQuakeGroundFx(cs.game.camera.X, cs.game.camera.Y, radiusTiles)
}

// cardMoveBurstApply deals `dmg` physical true damage to every living monster
// within the authored radius. Resistance applies; armor and soak do not. The
// slam shakes the ground around the party, so walls do not shield from it.
func (cs *CombatSystem) cardMoveBurstApply(dmg int, radiusTiles float64) bool {
	if dmg <= 0 || radiusTiles <= 0 || cs.game.world == nil {
		return false
	}
	hit := false
	// This automatic movement proc follows the party auto-target policy.
	hurts := func(m *monsterPkg.Monster3D) bool {
		return !isExcludedFromPartyAutoTarget(m) && !m.IsDamageInvulnerable()
	}
	cs.forEachAreaVictim(cs.partyNova(radiusTiles*float64(cs.game.config.GetTileSize())), hurts, func(m *monsterPkg.Monster3D) {
		dealt := cs.applyMonsterDamagePacket(
			m,
			singleMonsterDamagePacket(
				damagecalc.Parts{True: dmg},
				monsterPkg.DamagePhysical.String(),
				0,
			),
			monsterDamageOptions{IgnoreArmor: true},
		)
		cs.markMonsterHit(m)
		cs.game.reportDamage(logToneGood, damageLog{
			Source: logAbility(uitext.Text("combat.damage.card_source")),
			Target: monsterLogTarget(m),
			Amount: dealt.Total(),
			Kind:   damageLogBurst,
		})
		hit = true
		cs.finishIndirectKill(m)
	})
	return hit
}

// countLiveSummonsByOwner is the shared live-cap counter for every summon
// source. SummonedBy is the ownership SSOT for cards, skills, spells and bosses.
func (cs *CombatSystem) countLiveSummonsByOwner(owner string) int {
	w := cs.game.GetCurrentWorld()
	if w == nil || owner == "" {
		return 0
	}
	n := 0
	for _, m := range w.Monsters {
		if m != nil && m.IsAlive() && m.SummonedBy == owner {
			n++
		}
	}
	return n
}

// tryCardSummonOnAction independently rolls every active summon card on a party
// action. Each physical card owns its monster type, live limit and cooldown;
// collection order controls only deterministic roll/spawn order. Called from
// the attack and cast chokepoints.
func (cs *CombatSystem) tryCardSummonOnAction() {
	if cs.game.GetCurrentWorld() == nil {
		return
	}
	for _, source := range cs.game.cardSummonSources() {
		if cs.game.cardSummonCooldown(source.Owner).Active() {
			continue
		}
		// Preserve the old trigger order: a card rolls first, then checks whether
		// its live cap has room. A capped card consumes no cooldown.
		if rand.Intn(100) >= source.Chance {
			continue
		}
		if want := source.Limit - cs.countLiveSummonsByOwner(source.Owner); want > 0 {
			// Arm the cooldown only when allies actually appeared: a whiffed spawn
			// (no free tile around the party) must not waste the proc for 5s.
			if cs.summonCardAllies(source, want) > 0 {
				cs.game.armCardSummonCooldown(source.Owner, source.CooldownSeconds)
			}
		}
	}
}

// tryPartyActionSummons is the shared attack/generic-cast proc gate. Card
// summons are party-wide; Animal Bonding belongs to the character who acted.
func (cs *CombatSystem) tryPartyActionSummons(actor *character.MMCharacter) {
	cs.tryCardSummonOnAction()
	cs.tryAnimalBondingOnAction(actor)
}

func (cs *CombatSystem) tryAnimalBondingOnAction(druid *character.MMCharacter) {
	if !cs.canSummonAnimalBondingBear(druid) {
		return
	}
	tier := druid.SkillTier(character.SkillAnimalBonding)
	if rand.Intn(100) >= character.AnimalBondingProcPct(tier) {
		return
	}
	cs.summonAnimalBondingBear(druid)
}

func animalBondingOwner(druid *character.MMCharacter) string {
	if druid == nil {
		return ""
	}
	return animalBondingOwnerPrefix + druid.Name
}

func (cs *CombatSystem) canSummonAnimalBondingBear(druid *character.MMCharacter) bool {
	return druid != nil &&
		druid.HasSkill(character.SkillAnimalBonding) &&
		cs.game.GetCurrentWorld() != nil &&
		cs.countLiveSummonsByOwner(animalBondingOwner(druid)) < character.AnimalBondingSummonMax
}

func (cs *CombatSystem) summonAnimalBondingBear(druid *character.MMCharacter) bool {
	if !cs.canSummonAnimalBondingBear(druid) {
		return false
	}
	tier := druid.SkillTier(character.SkillAnimalBonding)
	bear := cs.spawnPartyAlly("bear", animalBondingOwner(druid))
	if bear == nil {
		return false
	}
	bear.SummonerName = druid.Name
	statPct := character.AnimalBondingStatPct(tier)
	hpPct := character.AnimalBondingHPPct(tier)
	bear.MaxHitPoints = druid.MaxHitPoints * hpPct / 100
	if bear.MaxHitPoints < 1 {
		bear.MaxHitPoints = 1
	}
	bear.HitPoints = bear.MaxHitPoints
	bear.ArmorClass = cs.CalculateTotalArmorClass(druid) * statPct / 100
	attack := druid.GetEffectiveMight() / WeaponPrimaryStatDivisor
	if weapon, ok := druid.Equipment[items.SlotMainHand]; ok {
		_, _, attack = cs.CalculateWeaponDamage(weapon, druid)
		if def := lookupWeaponConfig(weapon); def != nil {
			trueDamage, _ := cs.weaponMasteryStrike(druid, def)
			attack += trueDamage
		}
	}
	attack = attack * statPct / 100
	if attack < 1 {
		attack = 1
	}
	bear.DamageMin, bear.DamageMax = attack, attack
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.s_calls_a_bear_ally", logSlot, logSlot), logHeroName(druid), logAbility("Animal Bonding"))
	return true
}

// spawnPartyAlly is THE spawn path for every pure party ally (card summons, the
// druid's bear, summon spells): free-tile search near the party, ally tagging and
// world/collision registration. Callers only override per-source stats on the
// returned monster. Returns nil when no tile was free.
func (cs *CombatSystem) spawnPartyAlly(key, owner string) *monsterPkg.Monster3D {
	if cs.game.GetCurrentWorld() == nil {
		return nil
	}
	tile := float64(cs.game.config.GetTileSize())
	angle := rand.Float64() * 2 * math.Pi
	sx, sy, ok := cs.findNearestSummonTile(
		cs.game.camera.X+math.Cos(angle)*2*tile,
		cs.game.camera.Y+math.Sin(angle)*2*tile,
		10,
	)
	if !ok {
		return nil
	}
	add := monsterPkg.NewMonster3DFromConfig(sx, sy, key, cs.game.config)
	markPurePartySummon(add, owner)
	cs.game.registerSpawnedMonster(add)
	cs.game.refreshMonsterCollisionState(add)
	return add
}

// markPurePartySummon tags an ally with its owner NAMESPACE - see
// isPurePartySummon: an unregistered namespace makes the ally a valid target for
// the party's own damage.
func markPurePartySummon(m *monsterPkg.Monster3D, owner string) {
	m.Bound = true
	m.BoundClock = status.DualClock{}
	m.Pacified = false
	m.PacifiedClock = status.DualClock{}
	m.WasAttacked = false
	m.SummonedBy = owner
	m.QuestProgressIgnored = true
}

// summonerOf is the party member who called an ally: the one recorded at the
// summon, or for a bear from a save made before that, the druid its owner tag
// names. nil for card allies (the whole party's), bound former enemies, and a
// summoner no longer in the party.
func (g *MMGame) summonerOf(m *monsterPkg.Monster3D) *character.MMCharacter {
	if m == nil || !isPurePartySummon(m) || g.party == nil {
		return nil
	}
	name := m.SummonerName
	if name == "" {
		name, _ = strings.CutPrefix(m.SummonedBy, animalBondingOwnerPrefix)
		if name == m.SummonedBy {
			return nil
		}
	}
	for _, member := range g.party.Members {
		if member != nil && member.Name == name {
			return member
		}
	}
	return nil
}

// isCardAlly reports whether a monster is a card-collection summon - including
// legacy shared-owner saves and new per-card owners. It is a pure ally, distinct
// from a spell-bound undead (a former ENEMY that still yields its reward).
func isCardAlly(m *monsterPkg.Monster3D) bool {
	return m != nil && (m.SummonedBy == cardSummonOwner || strings.HasPrefix(m.SummonedBy, cardSummonOwnerPrefix))
}

// isPurePartySummon is the shared distinction between creatures created for
// the party and former enemies controlled by Bind Undead. Pure summons yield no
// rewards and are transparent to party attacks; bound undead deliberately get
// neither exclusion. Both crumble on map exit.
//
// A new ally source MUST register its owner namespace here. Missing from this
// list it silently gets the former-enemy treatment: the party's own spells,
// zones, splash and traps hit it, and its authored experience/gold/loot pay out
// on death (the Ice Elemental only looked harmless because those are all 0).
func isPurePartySummon(m *monsterPkg.Monster3D) bool {
	if m == nil {
		return false
	}
	return isCardAlly(m) ||
		strings.HasPrefix(m.SummonedBy, animalBondingOwnerPrefix) ||
		strings.HasPrefix(m.SummonedBy, spellSummonOwnerPrefix)
}

// isExcludedFromPartyAutoTarget is the faction policy for automatic target
// selection: movement bursts, ricochets, trap flank placement and held-pointer
// hover. Summons are never party targets, a charmed monster keeps its Charm and
// the caravan is never picked automatically. Bound former enemies (Bind Undead,
// dark-elf binding), passive monsters and wildlife remain valid targets.
func isExcludedFromPartyAutoTarget(m *monsterPkg.Monster3D) bool {
	return m == nil || isPurePartySummon(m) || m.Pacified || m.IsCaravan()
}

// crumbleBoundAlliesOnDeparture removes the party's bound allies from the world
// being left. A bound undead (a former enemy) grants XP but no loot or gold; a
// pure party summon yields nothing. The removal is immediate because the normal
// end-of-frame death sweep runs only on the newly entered world.
func (g *MMGame) crumbleBoundAlliesOnDeparture(departing *world.World3D) {
	if departing == nil || g.combat == nil {
		return
	}
	completed := map[*monsterPkg.EncounterRewards]bool{}
	kept := departing.Monsters[:0]
	for _, m := range departing.Monsters {
		if m == nil || !m.Bound || !m.IsAlive() {
			kept = append(kept, m)
			continue
		}
		if m.IsEncounterMonster && m.EncounterRewards != nil {
			completed[m.EncounterRewards] = true
		}
		if !isPurePartySummon(m) {
			g.combat.awardExperienceOnly(m)
			g.logCombatLine(logToneNone, uitext.Text("combat.your_bound_crumbles_as_you_leave", logSlot), logMonsterName(m))
		}
		// The departing world's monsters live in the single shared collision system
		// (switchToMap unregisters the old map's monsters one by one); a crumbled
		// ally is dropped from departing.Monsters here, so it would escape that sweep
		// and leave a ghost collision entity on the new map unless we unregister now.
		if g.collisionSystem != nil {
			g.collisionSystem.UnregisterEntity(m.ID)
		}
	}
	departing.Monsters = kept
	gl := &GameLoop{game: g}
	for rewards := range completed {
		if gl.countRemainingEncounterMonsters(departing.Monsters, rewards) == 0 {
			gl.awardEncounterRewards(rewards)
		}
	}
}

// summonCardAllies spawns up to n permanent Bound allies for one physical card.
// an idle BoundClock never expires, so they fight on until slain. Returns
// how many spawned.
func (cs *CombatSystem) summonCardAllies(source cardSummonSource, n int) int {
	spawned := 0
	for attempts := 0; spawned < n && attempts < n*12+12; attempts++ {
		if cs.spawnPartyAlly(source.MonsterKey, source.Owner) != nil {
			spawned++
		}
	}
	if spawned > 0 {
		cs.game.logCombatLine(logToneGood, uitext.Text("combat.the_rallies_ally_to_your_side", logSlot, spawned), logAbility(source.CardName))
	}
	return spawned
}

// SmartAttack is the Space-key "smart attack" (both modes). Priority:
//  1. ANYONE in the party is wounded and the caster has a heal (spellbook or
//     quick slot) -> cast it on the MOST wounded. The quick-slotted heal is
//     preferred; otherwise the first castable heal of the book chain
//     (knownHealChain). Healers can keep a combat spell in the quick slot and
//     still auto-triage.
//  2. No one wounded -> cast the quick-slotted offensive spell when payable
//     (Monk skips this: their quick spell is reserved for Spiritual Training).
//  3. Otherwise swing the equipped weapon.
//
// Acted is false when NOTHING happened (no wounded+heal, no castable quick
// spell, no trap, no weapon) so turn-based slots aren't burned.
func (cs *CombatSystem) SmartAttack() smartAction {
	caster := cs.game.party.Members[cs.game.selectedChar]
	if !caster.CanUseCombatAction() {
		return smartAction{}
	}

	// In RT, a dual-wielder can reach Space with the main hand / cast cooldown
	// still cycling because the off hand is free. That path must swing only the
	// off hand; it cannot sneak in a spell or heal. TB uses action slots instead,
	// so a preserved RT cooldown must not change its smart-action priority.
	if cs.game.turnBasedMode || caster.RTCooldown <= 0 {
		// Can't pay / can't land it -> fall through to attack.
		if healID, def, target, ok := cs.smartHealPlan(caster); ok && cs.castKnownHealOn(healID, def, target) {
			return smartAction{Acted: true, Spell: healID, Recovery: cs.SpellCooldownFrames(caster, healID)}
		}

		if spellID, ok := cs.smartOffensiveSpell(caster); ok && cs.CastEquippedSpell() {
			return smartAction{Acted: true, Spell: spellID, Recovery: cs.SpellCooldownFrames(caster, spellID)}
		}

		// Trap book (thief): Space arms the slotted trap before falling back to
		// the weapon. Silent on refusal (no SP / limit / no room) - quick spells
		// fall through to the weapon just as quietly via their canPay pre-check.
		if trapKey, placed := cs.tryPlaceQuickTrap(caster, false); placed {
			return smartAction{Acted: true, Trap: trapKey, Recovery: cs.TrapCooldownFrames(caster, trapKey)}
		}
	}

	return smartAction{Acted: cs.EquipmentMeleeAttack()}
}

// smartAction is what Space did: a spell, a trap, or else a weapon swing.
// Recovery is the cast or trap's RT cooldown, measured after it landed; a
// swing keeps its own weapon timing.
type smartAction struct {
	Acted    bool
	Spell    spells.SpellID
	Trap     string
	Recovery int
}

// Swing reports a weapon attack.
func (a smartAction) Swing() bool { return a.Acted && a.Spell == "" && a.Trap == "" }

// smartActionAvailable reports whether Space has any branch for this member,
// without acting. Dispatch order: a heal for a wounded ally, a ready slotted
// throw, then SmartAttack's spell, trap and weapon. Target, range and trap
// room are left to the action.
func (cs *CombatSystem) smartActionAvailable(caster *character.MMCharacter) bool {
	if caster == nil || !caster.CanUseCombatAction() {
		return false
	}
	if caster.HasWeaponInEitherHand() {
		return true
	}
	if action := caster.Equipment[items.SlotQuickAction]; action.Type == items.ItemThrowable && cs.game.flaskReady(caster, action.Flask) {
		return true
	}
	if _, _, _, ok := cs.smartHealPlan(caster); ok {
		return true
	}
	if _, ok := cs.smartOffensiveSpell(caster); ok {
		return true
	}
	return cs.quickTrapAvailable(caster)
}

// smartOffensiveSpell returns the slotted offensive spell Space may cast.
// Monks fight with their weapon only.
func (cs *CombatSystem) smartOffensiveSpell(caster *character.MMCharacter) (spells.SpellID, bool) {
	if caster.Class == character.ClassMonk {
		return "", false
	}
	spell, ok := caster.Equipment[items.SlotQuickAction]
	if !ok {
		return "", false
	}
	spellID := spells.SpellID(spell.SpellEffect)
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil || !def.IsOffensive() || !cs.spellCastableNow(caster, def, caster.MeditatedSPCost(def.SpellPointsCost)) {
		return "", false
	}
	return spellID, true
}

// smartHealPlan decides which heal Space should cast and on whom: the
// quick-slotted heal, then the book chain (knownHealChain), each tried in turn
// until one is castable now and has a wounded ally to serve. A slotted
// self-only heal waits like the book's when the caster knows another heal.
// ok=false when no one is wounded enough or no heal can be cast.
func (cs *CombatSystem) smartHealPlan(caster *character.MMCharacter) (spells.SpellID, spells.SpellDefinition, int, bool) {
	chain := knownHealChain(caster)
	candidates := make([]spells.SpellDefinition, 0, len(chain)+1)
	if spell, hasSpell := caster.Equipment[items.SlotQuickAction]; hasSpell {
		if def, err := spells.GetSpellDefinitionByID(spells.SpellID(spell.SpellEffect)); err == nil && def.IsHeal() &&
			(healReachOf(def) != healReachSelf || len(chain) == 0 || healReachOf(chain[0]) == healReachSelf) {
			candidates = append(candidates, def)
		}
	}
	for _, def := range append(candidates, chain...) {
		if !cs.spellCastableNow(caster, def, caster.MeditatedSPCost(def.SpellPointsCost)) {
			continue
		}
		if target := cs.mostWoundedHealTarget(def, SmartHealWoundedPct); target >= 0 {
			return def.ID, def, target, true
		}
	}
	return "", spells.SpellDefinition{}, -1, false
}

// mostWoundedHealTarget returns the party index of the most-wounded ally a
// heal should target (lowest HP fraction, below the `below` fraction), or -1
// if no one is hurt enough. A self-only heal (First Aid) only ever considers
// the caster; an other-target heal considers the whole party. Dead/KO members
// are skipped (heals don't revive).
func (cs *CombatSystem) mostWoundedHealTarget(def spells.SpellDefinition, below float64) int {
	best, bestFrac := -1, below
	for i, m := range cs.game.party.Members {
		if !m.IsLiving() || m.MaxHitPoints <= 0 { // whom a single heal can mend (castKnownHealOn)
			continue
		}
		if def.TargetSelf && i != cs.game.selectedChar {
			continue
		}
		frac := float64(m.HitPoints) / float64(m.MaxHitPoints)
		if frac < bestFrac {
			best, bestFrac = i, frac
		}
	}
	return best
}

// attackSlotFor picks which equipment slot swings on the attacker's next
// melee attack. Non-dual-wielders (or a dual-wielder with no weapon actually
// in the off-hand - e.g. holding a shield instead) always use the main hand.
// If the main hand itself is unequipped - nothing stops that; the unequip
// guard only protects a zero-other-weapon-skill character - the off-hand is
// used regardless of cooldown/cursor, since there's nothing else to swing.
// Otherwise a genuine dual-wielder alternates by NextTBAttackOffHand in both
// modes, while RT still respects per-hand cooldowns: if the cursor hand is
// busy, the other ready hand may swing. Safe to call more than once per attack
// (e.g. once to resolve the swing, again to know which cooldown/cursor to
// update): nothing between those calls changes RTCooldown or the cursor.
func (cs *CombatSystem) attackSlotFor(attacker *character.MMCharacter) items.EquipSlot {
	if attacker == nil || !attacker.IsDualWielding() {
		return items.SlotMainHand
	}
	if _, ok := attacker.Equipment[items.SlotMainHand]; !ok {
		return items.SlotOffHand
	}
	if cs.game.turnBasedMode {
		if attacker.NextTBAttackOffHand {
			return items.SlotOffHand
		}
		return items.SlotMainHand
	}
	mainReady := attacker.RTCooldown <= 0
	offReady := attacker.OffHandRTCooldown <= 0
	switch {
	case mainReady && offReady:
		if attacker.NextTBAttackOffHand {
			return items.SlotOffHand
		}
		return items.SlotMainHand
	case mainReady:
		return items.SlotMainHand
	default:
		return items.SlotOffHand
	}
}

// partyInsideSolidTerrain detects walls and blocking objects at attack height.
// Water and chasms only block walking, so flight permits attacks above them.
// Fighting and casting from inside objects are refused:
// monsters can neither reach nor see the party, so it would be a free-hit
// exploit. Emits a throttled explanation so the refusal reads as a rule.
func (cs *CombatSystem) partyInsideSolidTerrain() bool {
	g := cs.game
	if !g.partyHasTerrainPassage() || g.camera == nil {
		return false
	}
	x, y := cs.logicalCameraXY()
	return cs.attackOriginBlocked(x, y)
}

func (cs *CombatSystem) partyEntombed() bool {
	if !cs.partyInsideSolidTerrain() {
		return false
	}
	g := cs.game
	if g.frameCount-g.entombedMsgFrame > int64(g.config.GetTPS()) {
		g.entombedMsgFrame = g.frameCount
		g.AddCombatMessage(uitext.Text("combat.buried_inside_solid_terrain_the_party"))
	}
	return true
}

// EquipmentMeleeAttack performs a melee attack using equipped weapon
// EquipmentMeleeAttack swings the equipped weapon; reports whether an attack
// actually happened (no weapon / incapacitated -> false, so turn-based action
// slots aren't burned on a no-op).
func (cs *CombatSystem) EquipmentMeleeAttack() bool {
	return cs.equipmentAttackAtAngle(cs.partyAttackAngle(), false)
}

func (cs *CombatSystem) equipmentAttackAtAngle(angle float64, explicitAim bool) bool {
	attacker := cs.game.party.Members[cs.game.selectedChar]
	cs.strikeLanded = false

	// Stunned characters cannot attack either.
	if !attacker.CanUseCombatAction() {
		return false
	}
	// Flying inside solid terrain: no fighting from within the stone (monsters
	// can neither reach nor see the party there - a free-hit exploit).
	if cs.partyEntombed() {
		return false
	}

	// Check if character has a weapon equipped - main hand, or (Dual Wielding)
	// whichever hand attackSlotFor picked for this swing.
	slot := cs.attackSlotFor(attacker)
	if explicitAim {
		slot = items.SlotMainHand
	}
	weapon, hasWeapon := attacker.Equipment[slot]
	if !hasWeapon {
		return false // No weapon equipped
	}

	// Calculate damage using centralized function
	_, _, totalDamage := cs.CalculateWeaponDamage(weapon, attacker)

	weaponDef := lookupWeaponConfig(weapon)
	if weaponDef == nil {
		return false // Weapon not found, skip attack
	}

	// Ranged dispatch by `range` (config.RangedWeaponMinRangeTiles); validation
	// rejects a bow/throwing/blaster category authored with melee reach.
	// For ranged: roll crit and apply doubling inside createArrowAttack only.
	acted := false
	summonRolled := false
	if weaponDef.IsRanged() {
		// Masked Huntress Card: boost ranged weapon damage.
		totalDamage = cs.weaponRangedDamageAtLaunch(totalDamage, true)
		// createArrowAttack returns false at the projectile cap (MaxProjectiles):
		// nothing fired, so no cooldown/action - and no card procs either.
		firstShot := len(cs.game.arrows)
		acted = cs.createArrowAttackAimed(totalDamage, slot, "", angle, explicitAim)
		if acted {
			cs.chargeShots(attacker, cs.game.arrows[firstShot:])
		}
	} else if pct := cs.game.cardSpellProcPct(); pct > 0 && rand.Intn(100) < pct && cs.tryCardFireBoltInstead(attacker) {
		// Pixie Card: the swing becomes a free Fire Bolt cast instead of a melee hit.
		// castResolvedSpell already rolled the summon-card checks for this
		// action (a real cast counts as one) - don't roll it a second time below.
		acted = true
		summonRolled = true
	} else {
		// Bronze Cesti: the pair lands every swing twice at half damage - two
		// full strikes with independent crit rolls (steadier than one big hit).
		strikes := character.WeaponStrikeCount(weaponDef)
		strikeDamage := character.WeaponStrikeDamage(weaponDef, totalDamage)
		baseDamage := strikeDamage
		var isCrit bool
		for s := 0; s < strikes; s++ {
			dmg := strikeDamage
			isCrit, _ = cs.RollWeaponCriticalChance(weapon, attacker)
			dmg = weaponCriticalDamage(dmg, isCrit)
			cs.createMeleeAttack(weapon, dmg, isCrit) // instant swing; a whiff (arc/reach) still spends the cooldown/action, silently
		}
		acted = true
		// Octopus Card: chance to strike again immediately with a fresh swing.
		if pct := cs.game.cardDoubleAttackPct(); pct > 0 && rand.Intn(100) < pct {
			isCrit2, _ := cs.RollWeaponCriticalChance(weapon, attacker)
			dmg2 := baseDamage
			dmg2 = weaponCriticalDamage(dmg2, isCrit2)
			cs.createMeleeAttack(weapon, dmg2, isCrit2)
		}
		// Spiritual Training (Monk): a genuine melee swing can channel a free
		// quick-spell. Kept in THIS branch on purpose - never on a ranged shot
		// (the skill is melee) and never on the Pixie branch above (which
		// already spent the action on its own free cast; firing here too would
		// grant two free spells from one swing).
		cs.trySpiritualTraining(attacker)
	}

	// Card procs only fire on an attack that actually happened (gated above), so a
	// capped ranged weapon can't be spammed for free Ningyo/Orc Warlord procs.
	if acted {
		if cs.strikeLanded {
			// Fate and Fire Aura empower a swing that lands (strikeCarried); a
			// miss and a card spell spend neither. Shots settle in chargeShots.
			consumePersonalAttackCharges(attacker)
		}
		cs.tryAttackZoneProc(weaponDef, attacker)
		if weaponDef.Category == "staff" && !summonRolled {
			attacker.ConsumeFlowingStaffCharge()
		}
		cs.tryCardHealOnAttack() // Ningyo Card: chance to self-heal on attacking
		if !summonRolled {
			cs.tryPartyActionSummons(attacker)
		}
		// Bandit Card: chance to also loose a short bonus bolt (Accuracy/3 dmg).
		// Always the main hand - a generic card proc, not tied to which hand swung.
		// The card itself stays silent; the bolt's chat identity is the authored
		// label of whichever card granted the bonus.
		if pct := cs.game.cardBonusBoltPct(); pct > 0 && rand.Intn(100) < pct {
			cs.createArrowAttackAimed(attacker.GetEffectiveAccuracy()/3, items.SlotMainHand, cs.game.cardBonusBoltLabel(), angle, explicitAim)
		}
	}
	return acted
}

// createArrowAttackAimed creates a projectile arrow attack; reports whether an
// arrow actually left the bow (max-projectiles cap / missing physics -> false,
// so the attempt doesn't cost an action). It fires from the weapon in `slot` -
// SlotMainHand for every normal ranged attack and card procs (Bandit's bonus
// bolt), or whichever hand attackSlotFor picked for a Dual Wielding
// character's primary swing (so a bow in the off-hand fires correctly instead
// of silently reading the main hand's weapon). A non-empty label names the
// projectile in chat instead of the weapon (card-proc bolts on melee wielders
// would otherwise report the hunting-bow physics fallback).
func (cs *CombatSystem) createArrowAttackAimed(damage int, slot items.EquipSlot, label string, angle float64, explicitAim bool) bool {
	// Find the equipped projectile-weapon's YAML key. Range>3 = ranged
	// (matches the dispatch gate in EquipmentMeleeAttack).
	attacker := cs.game.party.Members[cs.game.selectedChar]
	bonusBolt := label != ""
	weapon, hasWeapon := attacker.Equipment[slot]
	bowKey := "hunting_bow"
	var equippedDef *config.WeaponDefinitionConfig
	if hasWeapon && !bonusBolt {
		equippedDef = lookupWeaponConfig(weapon)
		if _, key, ok := config.WeaponDefOf(weapon); ok && equippedDef.IsRanged() {
			bowKey = key
		}
	}

	// Check max projectiles limit for this weapon
	if !bonusBolt && equippedDef != nil && equippedDef.MaxProjectiles > 0 {
		// Count active arrows from this specific bow
		activeArrowsFromBow := 0
		for _, arrow := range cs.game.arrows {
			if arrow.Active && arrow.BowKey == bowKey {
				activeArrowsFromBow++
			}
		}

		// If we've reached the limit, don't create a new arrow
		if activeArrowsFromBow >= equippedDef.MaxProjectiles {
			return false
		}
	}

	weaponDef, exists := config.GetWeaponDefinition(bowKey)
	if !exists || weaponDef == nil || weaponDef.Physics == nil {
		fmt.Printf("[WARN] projectile weapon '%s' is missing physics in weapons.yaml\n", bowKey)
		return false
	}

	tileSize := cs.game.config.GetTileSize()
	arrowSpeed := weaponDef.Physics.GetSpeedPixels(tileSize)
	arrowLifetime := weaponDef.Physics.GetLifetimeFrames()
	if !bonusBolt && character.BallisticsWeapon(equippedDef) {
		rangeTiles, speedTiles := character.EffectiveWeaponFlight(equippedDef, attacker)
		arrowSpeed = speedTiles * float64(tileSize) / float64(cs.game.config.GetTPS())
		if speedTiles > 0 {
			arrowLifetime = int(rangeTiles/speedTiles*float64(cs.game.config.GetTPS()) + 0.5)
		}
	}
	collisionSize := cs.game.config.ProjectileHitboxTiles(weaponDef.Physics) * tileSize

	// Determine damage type from weapon
	damageType := monsterPkg.DamagePhysical.String()
	if !bonusBolt && equippedDef != nil && equippedDef.DamageType != "" {
		damageType = normalizeDamageTypeStr(equippedDef.DamageType)
	}

	// Volley: a weapon may loose several projectiles per shot (e.g. the blowgun
	// fires 2 darts). They fly STRAIGHT along the aim - an angular fan straddled a
	// target dead ahead and missed - spaced back along the line so they read as a
	// quick stream (one behind the other) and all strike what's in front. Each
	// projectile rolls its own crit.
	volley := 1
	if !bonusBolt && equippedDef != nil && equippedDef.Volley > 1 {
		volley = equippedDef.Volley
	}
	// Ashigaru Firelock Card: chance to loose one extra arrow in the volley.
	if !bonusBolt {
		if pct := cs.game.cardVolleyBonusPct(); pct > 0 && rand.Intn(100) < pct {
			volley++
		}
		// Wind-up: every wound spring adds a bullet to this shot.
		volley += spendWindUp(attacker, slot, equippedDef, cs.game.turnBasedMode, cs.game.config.GetTPS())
	}
	trueDamage, grazesDodge := 0, false
	if !bonusBolt {
		trueDamage, grazesDodge = cs.weaponMasteryStrike(attacker, equippedDef)
	}
	disintegrateDef := equippedDef
	if bonusBolt {
		disintegrateDef = nil // Bonus bolts inherit cards, never the held weapon proc.
	}
	disintegrateChance := cs.game.weaponDisintegrateChance(disintegrateDef)
	pierceLeft, ricochetLeft := 0, 0
	if !bonusBolt && equippedDef != nil {
		pierceLeft = equippedDef.PierceCount
		ricochetLeft = equippedDef.RicochetTargets
	}
	launch := cs.partyProjectileLaunch(angle, arrowSpeed*float64(arrowLifetime), explicitAim)
	angle = launch.angle
	dirX, dirY := math.Cos(angle), math.Sin(angle)
	spacing := volleySpacingFrac * float64(tileSize)
	heldCharged := false
	sureCrit := !bonusBolt && spendSkirmishCharge(attacker, slot, equippedDef)
	for i := 0; i < volley; i++ {
		back := spacing * float64(i) // trail later darts behind the first
		isCrit, critChance := false, 0
		if sureCrit {
			isCrit, critChance = true, 100
		} else if !bonusBolt {
			isCrit, critChance = cs.RollWeaponCriticalChance(weapon, attacker)
		}
		dmg := damage
		dmg = weaponCriticalDamage(dmg, isCrit)
		arrow := Arrow{
			Launch:                 launch,
			ElementalAbilityDamage: elementalAbilityDamage(disintegrateDef, attacker),
			ID:                     cs.game.GenerateProjectileID("arrow"),
			Attacker:               cs.activeAttacker(),
			X:                      cs.game.camera.X - dirX*back,
			Y:                      cs.game.camera.Y - dirY*back,
			VelX:                   dirX * arrowSpeed,
			VelY:                   dirY * arrowSpeed,
			Damage:                 dmg,
			TrueDamage:             trueDamage,
			GrazesDodge:            grazesDodge,
			LifeTime:               volleyDartLifetime(arrowLifetime, back, arrowSpeed),
			Active:                 true,
			BowKey:                 bowKey,
			Label:                  label,
			DamageType:             damageType,
			Crit:                   isCrit,
			CritChance:             critChance,
			DisintegrateChance:     disintegrateChance,
			PierceLeft:             pierceLeft,
			RicochetLeft:           ricochetLeft,
			Owner:                  ProjectileOwnerPlayer,
		}
		if !bonusBolt {
			arrow.FireAuraPercent, arrow.FireAuraSchool = personalSpellDamage(attacker, config.MechanicFireAura)
			arrow.Undodgeable = fateGuided(attacker)
			arrow.VampiricPct = personalSpellPower(attacker, config.MechanicVampiricWeapon)
			if i == 0 || equippedDef.HasSignature(config.SignatureReturning) {
				cs.snapshotWeaponSignature(&arrow, attacker, slot, equippedDef)
			}
		}
		if i == 0 {
			heldCharged = arrow.Signature.Flare
		}
		cs.game.arrows = append(cs.game.arrows, arrow)
		arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(arrowEntity)
	}
	if !bonusBolt {
		cs.presentRangedShot(attacker, equippedDef, slot, launch, heldCharged)
	}
	cs.game.playRangedWeaponAttackSound(rangedProjectileSoundDefinition(bonusBolt, equippedDef, weaponDef))
	return true
}

func rangedProjectileSoundDefinition(bonusBolt bool, equippedDef, projectileDef *config.WeaponDefinitionConfig) *config.WeaponDefinitionConfig {
	if bonusBolt {
		return projectileDef
	}
	return equippedDef
}

// createMeleeAttack creates an instant melee attack with proper arc-based hit
// detection.
func (cs *CombatSystem) createMeleeAttack(weapon items.Item, totalDamage int, isCrit bool) {
	// Get weapon definition from YAML
	weaponDef := lookupWeaponConfig(weapon)
	if weaponDef == nil {
		return // Weapon not found, skip attack
	}

	// Check if weapon has melee configuration
	if weaponDef.Melee == nil {
		fmt.Printf("[WARN] weapon '%s' has no melee configuration in weapons.yaml\n", weapon.Name)
		return
	}
	cs.game.playSound(soundMeleeSwing)
	cs.game.endRangedHeld(cs.activeAttacker())

	meleeConfig := weaponDef.Melee
	graphicsConfig := weaponDef.Graphics

	// Create visual slash effect (a per-weapon material flourish; see
	// drawMeleeParticles, driven by Kind).
	if graphicsConfig != nil {
		_, weaponKey, _ := config.WeaponDefOf(weapon)
		// Linger the visual flourish past the (fast) swing so the shaped trail
		// fades slowly - the instant hit already resolved separately. Bespoke
		// legendary styles linger longer: their debris/droplets need the tail.
		linger := MeleeFxLingerFrames
		if graphicsConfig.SlashFx != "" {
			linger = meleeFxStyledLingerFrames
		}
		maxFrames := meleeConfig.AnimationFrames
		if maxFrames < linger {
			maxFrames = linger
		}
		sweepFrames := max(1, int(math.Round(float64(maxFrames)*meleeSweepFrac)))
		maxFrames += max(1, int(math.Round(float64(cs.game.config.GetTPS())*.55)))
		slashEffect := SlashEffect{
			WeaponKey:      weaponKey,
			ID:             cs.game.GenerateProjectileID("slash"),
			X:              cs.game.camera.X,
			Y:              cs.game.camera.Y,
			Width:          graphicsConfig.SlashWidth,
			Length:         graphicsConfig.SlashLength,
			Color:          graphicsConfig.SlashColor,
			AnimationFrame: 0,
			MaxFrames:      maxFrames,
			SweepFrames:    sweepFrames,
			AnchorLift:     graphicsConfig.SlashLiftRatio,
			Active:         true,
			Kind:           meleeFxKind(weaponDef),
			Style:          graphicsConfig.SlashFx,
			Crit:           isCrit,
		}
		cs.game.slashEffects = append(cs.game.slashEffects, slashEffect)
	}

	// Perform instant hit detection in arc
	cs.performMeleeHitDetection(weapon, totalDamage, meleeConfig, isCrit)
}

// Melee swing cone half-angles (radians) per discrete arc type. Front is a thin
// sliver (arc 1), wing reaches the +/-45deg diagonals (arcs 2/3), flank reaches the
// +/-90deg sides (arc 4). The tiny epsilon keeps the exactly-45deg/90deg diagonal and
// side tiles inside the cone despite float rounding.
const (
	meleeArcFront = 22.5 * math.Pi / 180.0
	meleeArcWing  = 45.0*math.Pi/180.0 + 1e-6
	meleeArcFlank = 90.0*math.Pi/180.0 + 1e-6
)

type meleeHitCandidate struct {
	m   *monsterPkg.Monster3D
	ang float64
}

// meleeReachAngle reports whether a target at (tx,ty) is within a swing of
// rangeTiles from (ox,oy) and, if so, its signed angle off the swing facing.
// The reach rule (tile Chebyshev + a (range+0.5)-tile pixel fallback) and the
// angle normalization are the ONE definition shared by the party's PvE swing
// and a champion's swing at summons.
func meleeReachAngle(ox, oy, facing float64, rangeTiles int, tileSize, tx, ty float64) (ang float64, inReach bool) {
	otx, oty := TileIndex(ox, tileSize), TileIndex(oy, tileSize)
	cheb := mathutil.IntAbs(TileIndex(tx, tileSize) - otx)
	if dy := mathutil.IntAbs(TileIndex(ty, tileSize) - oty); dy > cheb {
		cheb = dy
	}
	if cheb > rangeTiles {
		reachPx := (float64(rangeTiles) + 0.5) * tileSize
		if math.Max(math.Abs(tx-ox), math.Abs(ty-oy)) > reachPx {
			return 0, false
		}
	}
	if cheb > 0 {
		ang = math.Atan2(ty-oy, tx-ox) - facing
	}
	for ang > math.Pi {
		ang -= 2 * math.Pi
	}
	for ang < -math.Pi {
		ang += 2 * math.Pi
	}
	return ang, true
}

// meleeArcCandidate is one entity caught by a swing's reach: its angle off the
// facing and a closure that applies the hit. Type-agnostic so a single swing can
// mix targets (a champion's arc catches summons AND party members at once).
type meleeArcCandidate struct {
	ang float64
	hit func()
}

// applyMeleeArc fires the hit closures the weapon's arc catches from a candidate
// set already filtered to reach. THE single arc-shape rule (front sliver / one
// flank / both diagonals / both sides), shared by the party's PvE swing and a
// champion's swing at summons.
func applyMeleeArc(cands []meleeArcCandidate, arcType int) {
	switch arcType {
	case 2:
		// Front always; then ONE diagonal flank - the side that has a target,
		// random when both do.
		var left, right []func()
		for _, c := range cands {
			a := math.Abs(c.ang)
			switch {
			case a <= meleeArcFront:
				c.hit()
			case a > meleeArcWing:
				// out of the swing
			case c.ang < 0:
				left = append(left, c.hit)
			default:
				right = append(right, c.hit)
			}
		}
		side := left
		switch {
		case len(left) > 0 && len(right) > 0:
			if rand.Intn(2) == 0 {
				side = right
			}
		case len(right) > 0:
			side = right
		}
		for _, h := range side {
			h()
		}
	default:
		halfArc := meleeArcFront // arc 1
		switch arcType {
		case 3:
			halfArc = meleeArcWing
		case 4:
			halfArc = meleeArcFlank
		}
		var frontDiagonalAssist []func()
		hitAny := false
		for _, c := range cands {
			a := math.Abs(c.ang)
			if a <= halfArc {
				c.hit()
				hitAny = true
			} else if arcType == 1 && a <= meleeArcWing {
				frontDiagonalAssist = append(frontDiagonalAssist, c.hit)
			}
		}
		if !hitAny && len(frontDiagonalAssist) > 0 {
			frontDiagonalAssist[rand.Intn(len(frontDiagonalAssist))]()
		}
	}
}

// performMeleeHitDetection applies the swing to every monster inside the weapon's
// arc and reports how many it connected with. Reach is TILE-step Chebyshev (a
// diagonal neighbour is one step: range 1 covers all 8 adjacent tiles), with a
// true-distance fallback: a mob straddling tile boundaries can sit ~1 tile away
// yet 2 tile-indices over, so anything within (range+0.5) tiles pixel-Chebyshev
// (the far edge of the covered tile ring) is in reach regardless of where
// inside the tiles both sides stand. Direction follows the camera continuously.
//
// Arc types (counts are for range 1, aligned to an axis):
//
//	1 - straight ahead only (1 foe; a range-2 weapon pierces the line two deep)
//	2 - front + ONE flank (2 foes; the side with a foe, random when both have one)
//	3 - front + both diagonals (3 foes; range 2 sweeps 3+5=8)
//	4 - front + diagonals + both sides (5 foes)
func (cs *CombatSystem) performMeleeHitDetection(weapon items.Item, damage int, meleeConfig *config.MeleeAttackConfig, isCrit bool) int {
	playerX := cs.game.camera.X
	playerY := cs.game.camera.Y
	playerAngle := cs.partyAttackAngle()
	tileSize := float64(cs.game.config.GetTileSize())

	weaponDef := lookupWeaponConfig(weapon)
	rangeTiles := 1
	if weaponDef != nil && weaponDef.Range > 0 {
		rangeTiles = weaponDef.Range
	}

	// Candidates: alive monsters within tile reach, with their signed angle off
	// the player's facing. Stunned monsters are still valid targets (stun only
	// suppresses their own turn).
	var cands []meleeHitCandidate
	for _, monster := range cs.game.world.Monsters {
		if !monster.IsAlive() || isPurePartySummon(monster) {
			continue
		}
		// A combatant merely transiting through another monster's claimed post
		// has no attack position of its own. Arcs read combat posts, not
		// incidental overlap; AoE intentionally does not use this filter and
		// still damages the same mob below/elsewhere.
		if monsterInAttackTransit(monster) {
			continue
		}
		ang, ok := meleeReachAngle(playerX, playerY, playerAngle, rangeTiles, tileSize, monster.X, monster.Y)
		if !ok || !cs.attackLineClear(playerX, playerY, monster.X, monster.Y) {
			continue
		}
		cands = append(cands, meleeHitCandidate{monster, ang})
	}

	// The swing's victims first, then its hits: Rune Discharge picks its
	// primary among the monsters this swing actually strikes.
	var victims []*monsterPkg.Monster3D
	if targets, ok := cs.turnBasedPulledMeleeTargets(cands, meleeConfig.ArcType); ok {
		victims = targets
	} else {
		arcCands := make([]meleeArcCandidate, len(cands))
		for i, c := range cands {
			cm := c.m
			arcCands[i] = meleeArcCandidate{ang: c.ang, hit: func() { victims = append(victims, cm) }}
		}
		applyMeleeArc(arcCands, meleeConfig.ArcType)
	}
	previousRuneSwing := cs.runeSwing
	if weaponDef.HasSignature(config.SignatureRuneDischarge) {
		cs.runeSwing = &runeSwing{primary: cs.runePrimary(victims, cands, playerX, playerY)}
	}
	defer func() { cs.runeSwing = previousRuneSwing }()
	hits := 0
	for _, m := range victims {
		if m.IsAlive() {
			cs.ApplyDamageToMonster(m, damage, weapon, isCrit)
			hits++
		}
	}
	return hits
}

// runePrimary is the swing victim Rune Discharge strikes: the aimed monster
// when the swing hits it, else the most frontal victim, then the nearest.
func (cs *CombatSystem) runePrimary(victims []*monsterPkg.Monster3D, cands []meleeHitCandidate, x, y float64) *monsterPkg.Monster3D {
	var best *monsterPkg.Monster3D
	bestAngle, bestDistance := math.MaxFloat64, math.MaxFloat64
	for _, c := range cands {
		if !slices.Contains(victims, c.m) {
			continue
		}
		if c.m == cs.partyAimTarget {
			return c.m
		}
		angle, distance := math.Abs(c.ang), DistanceSquared(x, y, c.m.X, c.m.Y)
		if angle < bestAngle || angle == bestAngle && distance < bestDistance {
			best, bestAngle, bestDistance = c.m, angle, distance
		}
	}
	return best
}

func (cs *CombatSystem) turnBasedPulledMeleeTargets(cands []meleeHitCandidate, arcType int) ([]*monsterPkg.Monster3D, bool) {
	if cs.partyAimTarget != nil {
		return nil, false // explicit world aim uses the weapon's actual arc
	}
	if arcType != 1 && arcType != 2 {
		return nil, false
	}
	mons := make([]*monsterPkg.Monster3D, len(cands))
	for i, c := range cands {
		mons[i] = c.m
	}
	front, left, right, hasPulledSide := cs.classifyFrontSlots(mons)
	if !hasPulledSide {
		return nil, false
	}

	side := chooseFrontAttackSide(left, right)
	switch arcType {
	case 1:
		if front != nil {
			return []*monsterPkg.Monster3D{front.monster}, true
		}
		if side != nil {
			return []*monsterPkg.Monster3D{side.monster}, true
		}
	case 2:
		targets := make([]*monsterPkg.Monster3D, 0, 2)
		if front != nil {
			targets = append(targets, front.monster)
		}
		if side != nil {
			targets = append(targets, side.monster)
		}
		if len(targets) > 0 {
			return targets, true
		}
	}
	return nil, false
}

type frontAttackSlotChoice struct {
	monster *monsterPkg.Monster3D
	dist2   float64
	vx, vy  float64 // visual (pulled) world position - gates projectile assist by aim direction
}

// classifyFrontSlots buckets monsters into the player's front attack slots
// (dead-ahead / pulled-left / pulled-right), nearest-first per side, via the
// pulledFrontSlot single source of truth. hasPulledSide reports whether any
// DIAGONAL was actually pulled (the trigger for the narrow-arc melee override).
func (cs *CombatSystem) classifyFrontSlots(mons []*monsterPkg.Monster3D) (front, left, right *frontAttackSlotChoice, hasPulledSide bool) {
	for _, m := range mons {
		side, vx, vy, pulled, ok := cs.pulledFrontSlot(m)
		if !ok {
			continue
		}
		choice := &frontAttackSlotChoice{
			monster: m,
			dist2:   DistanceSquared(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y),
			vx:      vx,
			vy:      vy,
		}
		switch side {
		case -1:
			hasPulledSide = hasPulledSide || pulled
			if left == nil || choice.dist2 < left.dist2 {
				left = choice
			}
		case 1:
			hasPulledSide = hasPulledSide || pulled
			if right == nil || choice.dist2 < right.dist2 {
				right = choice
			}
		default:
			if front == nil || choice.dist2 < front.dist2 {
				front = choice
			}
		}
	}
	return front, left, right, hasPulledSide
}

const projectileAssistMaxAngleRad = 35.0 * math.Pi / 180.0

// headingTowardWithin reports whether heading (dirX,dirY) points within maxRad of
// the direction from (ox,oy) to (tx,ty).
func headingTowardWithin(ox, oy, dirX, dirY, tx, ty, maxRad float64) bool {
	if dirX == 0 && dirY == 0 {
		return false
	}
	d := math.Atan2(ty-oy, tx-ox) - math.Atan2(dirY, dirX)
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	return math.Abs(d) <= maxRad
}

func chooseFrontAttackSide(left, right *frontAttackSlotChoice) *frontAttackSlotChoice {
	switch {
	case left != nil && right != nil:
		if right.dist2 < left.dist2 {
			return right
		}
		return left
	case left != nil:
		return left
	default:
		return right
	}
}

// logicalCameraXY is the camera position WITHOUT the cosmetic Draw-time screen
// shake (screenShakeOffset is 0 outside Draw). Render-time geometry that must NOT
// jitter with the shake - the TB front-diagonal pull and its gates - uses this,
// so a pulled monster near a wall doesn't blink when a hit shakes the view.
func (cs *CombatSystem) logicalCameraXY() (float64, float64) {
	return cs.game.camera.X - cs.game.screenShakeOffsetX, cs.game.camera.Y - cs.game.screenShakeOffsetY
}

// pulledFrontSlot is the SINGLE source of truth for the turn-based front-diagonal
// "pull": where a melee monster on a front slot is both DRAWN and ATTACKED FROM,
// so the visual and the hit can never disagree (the renderer and the combat
// resolver both call it). Returns the slot side (-1 left / 0 dead-ahead / +1
// right), the world position to use (pulled ~1 tile ahead + slightly aside for a
// front diagonal; the monster's true spot dead-ahead), whether it was pulled, and
// ok=false when no front slot applies (not TB, not a melee delivery, not
// adjacent, off-axis, behind, or the pulled spot has no line of sight).
func (cs *CombatSystem) pulledFrontSlot(mon *monsterPkg.Monster3D) (side int, x, y float64, pulled, ok bool) {
	if mon == nil || !mon.IsAlive() {
		return 0, 0, 0, false, false
	}
	return cs.monsterFrontSlotGeometry(mon)
}

// Geometry also serves the just-killed actor's corpse and impact effects.
// Target selection keeps its separate alive gate in pulledFrontSlot.
func (cs *CombatSystem) monsterFrontSlotGeometry(mon *monsterPkg.Monster3D) (side int, x, y float64, pulled, ok bool) {
	if cs == nil || cs.game == nil || mon == nil || !cs.game.turnBasedMode {
		return 0, 0, 0, false, false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	if tileSize <= 0 {
		return 0, 0, 0, false, false
	}
	// Logical (un-shaken) camera: the pull decision, its gates, and its LOS must
	// not flip with the per-frame +/- shake jitter (see logicalCameraXY).
	camX, camY := cs.logicalCameraXY()
	ptx, pty := TileIndex(camX, tileSize), TileIndex(camY, tileSize)
	mtx, mty := TileIndex(mon.X, tileSize), TileIndex(mon.Y, tileSize)
	mdx, mdy := mtx-ptx, mty-pty
	fx, fy := cardinalForwardFromAngle(cs.game.camera.Angle)

	// Exactly one tile dead-ahead: a real front target, drawn/attacked where it is.
	if mdx == fx && mdy == fy {
		losOK := cs.game.collisionSystem == nil ||
			cs.game.collisionSystem.CheckLineOfSight(mon.X, mon.Y, camX, camY)
		return 0, mon.X, mon.Y, false, losOK
	}
	// Front DIAGONAL melee neighbour: pull it ~1 tile ahead, slightly to its
	// side. The spatial gate keeps the TRUE-position adjacency/LOS rule - a
	// neighbour behind a wall corner must not be pulled into a targetable
	// front slot; the delivery selector then excludes ranged-only attackers
	// (champions), whose pull would misrepresent their attack.
	if mdx == 0 || mdy == 0 || !cs.monsterMeleeAdjacentToParty(mon) ||
		!cs.monsterUsesMeleeAgainstParty(mon) {
		return 0, 0, 0, false, false
	}
	rx, ry := -fy, fx
	for _, s := range [2]int{-1, 1} {
		if mdx != fx+s*rx || mdy != fy+s*ry {
			continue
		}
		fakeX := camX + float64(fx)*tbFrontDiagonalMonsterForwardTiles*tileSize + float64(s*rx)*tbFrontDiagonalMonsterLateralTiles*tileSize
		fakeY := camY + float64(fy)*tbFrontDiagonalMonsterForwardTiles*tileSize + float64(s*ry)*tbFrontDiagonalMonsterLateralTiles*tileSize
		if cs.game.collisionSystem != nil && !cs.game.collisionSystem.CheckLineOfSight(camX, camY, fakeX, fakeY) {
			return 0, 0, 0, false, false
		}
		return s, fakeX, fakeY, true, true
	}
	return 0, 0, 0, false, false
}

// monsterVisualPos is the single source of truth for where a monster is DRAWN:
// its true spot, shifted by the turn-based front-diagonal pulled slot and by
// the banded-stack fan offset (a band snaps its members onto one tile, then the
// renderer fans them out to read as several). Every impact/splash FX anchors
// here so it lands where the player SEES the monster, not at its real tile -
// the renderer's monsterVisualPosition delegates to this so the two can't drift.
// A dead monster stays where it was drawn as it died.
func (cs *CombatSystem) monsterVisualPos(mon *monsterPkg.Monster3D) (float64, float64) {
	if mon == nil {
		return 0, 0
	}
	if mon.DeathDrawn && !mon.IsAlive() {
		return mon.DeathDrawX, mon.DeathDrawY
	}
	x, y := mon.X, mon.Y
	if _, px, py, pulled, ok := cs.monsterFrontSlotGeometry(mon); ok && pulled {
		x, y = px, py
	}
	if cs.game != nil && cs.game.config != nil {
		ox, oy := cs.game.monsterVisualStackOffset(mon, x, y)
		x, y = x+ox, y+oy
	}
	return x, y
}

// monsterVisualStackOffset is the tile-fan part of where a monster is drawn,
// seen from the logical camera (monster.FanVisualOffset); x, y is the
// presented base point, a TB pulled slot included.
func (g *MMGame) monsterVisualStackOffset(m *monsterPkg.Monster3D, x, y float64) (float64, float64) {
	camX, camY := g.partyFightPos()
	return monsterPkg.FanVisualOffset(m, x, y, camX, camY, g.config.GetTileSize())
}

// spawnMonsterHitBurst bursts generic impact particles where a monster is DRAWN
// (banded-stack / pulled-slot aware, via monsterVisualPos). Shared by AoE splash
// and party-nova victims so both anchor on the sprite, not the raw tile.
func (cs *CombatSystem) spawnMonsterHitBurst(m *monsterPkg.Monster3D, element string) {
	x, y := cs.monsterVisualPos(m)
	cs.game.CreateSpellHitEffect(x, y, element, SpellParticleCount, SpellParticleSize, m)
}

// applyDamageThroughDodge lands what a monster's Perfect Dodge lets through
// (throughDodge): no riders, no splash. False when the dodge took it all.
func (cs *CombatSystem) applyDamageThroughDodge(monster *monsterPkg.Monster3D, attack partyMonsterAttack, attacker *character.MMCharacter, attackerName string, weaponDef *config.WeaponDefinitionConfig) bool {
	// A targeted party attack ends Charm even when the target avoids all of it;
	// otherwise a 100%-dodge charmed mob would stay pacified forever.
	cs.breakPacifyOnHit(monster)
	through := attack.Packet.throughDodge(attack.GrazesDodge)
	if len(through.Components) == 0 {
		return false
	}
	beforeHP := monster.HitPoints
	dealt := cs.applyMonsterDamagePacket(monster, through, cs.partyMonsterDamageOptions(attack, monster))
	actual := dealt.Total()
	settleVampiricHit(attacker, attack.VampiricPct, beforeHP-monster.HitPoints)
	if actual > 0 {
		cs.game.playMonsterSound(soundMonsterHit, monster)
	}
	cs.markMonsterHit(monster)
	cs.game.reportDamage(logToneGood, damageLog{
		Source: logHeroText(attackerName),
		Target: monsterLogTarget(monster),
		Amount: actual,
		School: through.loggedSchool(),
		Shares: dealtShares(dealt),
		Kind:   damageLogDodge,
	})
	if !monster.IsAlive() {
		cs.finishWeaponKill(monster, weaponDef, attacker)
	}
	return true
}

// ApplyDamageToMonster resolves a melee hit and reports its post-hit HP before
// weapon procs and splash resolve their own hits. Armor reduces physical damage.
func (cs *CombatSystem) ApplyDamageToMonster(monster *monsterPkg.Monster3D, damage int, weapon items.Item, isCrit bool) {
	if monster == nil || !monster.IsAlive() {
		return
	}
	if isPurePartySummon(monster) {
		return
	}
	if cs.absorbIfSealed(monster) {
		return
	}
	weaponDef := lookupWeaponConfig(weapon)
	damageTypeStr := weaponDamageTypeStr(weaponDef)
	attacker := cs.activeAttacker() // melee resolves the same frame it swings
	critChance := 0
	if weaponDef != nil && attacker != nil {
		critChance = cs.CalculateWeaponCritChance(attacker.Equipment[cs.attackSlotFor(attacker)], attacker)
	}
	trueDmg, grazesDodge := cs.weaponMasteryStrike(attacker, weaponDef)
	trueDmg += cs.game.cardMeleeTrueDmg()
	attackerName := partyAttackerName(attacker)
	attack := cs.newPartyWeaponAttack(
		damage,
		trueDmg,
		damageTypeStr,
		weaponDef,
		weapon.Name,
		false,
		isCrit,
		critChance,
	)
	attack.Attacker = attacker
	auraDamage, auraSchool := personalSpellDamage(attacker, config.MechanicFireAura)
	attack.addPersonalSpellPayload(auraDamage, auraSchool, personalSpellPower(attacker, config.MechanicVampiricWeapon))
	attack.GrazesDodge = grazesDodge
	attack.Undodgeable = fateGuided(attacker)
	attack.ArmorIgnoreChancePct = cs.game.cardArmorPiercePct()

	// Check monster perfect dodge: the normal hit is avoided, but weapon-mastery
	// TRUE damage still lands and a Grandmaster's hit grazes through.
	if !attack.Undodgeable && cs.monsterPerfectDodges(monster) {
		through := cs.applyDamageThroughDodge(monster, attack, attacker, attackerName, weaponDef)
		if !through {
			cs.game.logCombatLine(logToneNone, uitext.Text("combat.dodges_s_attack", logSlot, logSlot), logMonsterName(monster), logHeroText(attackerName))
		}
		cs.strikeLanded = cs.strikeLanded || strikeCarried(true, through, attack.GrazesDodge)
		return
	}
	cs.strikeLanded = true
	if cs.tryDarkElfBindInstead(attacker, monster) {
		return
	}

	// Weapon and card chances share the projectile policy and immunity gate.
	// A sear belongs to this victim only; the splash keeps the source attack.
	outcome := rollMonsterDisintegrate(monster, cs.game.weaponDisintegrateChance(weaponDef))
	if outcome == instakillKill {
		cs.disintegratePartyTarget(monster, weaponDef, attacker, attack, weaponAoeRadius(weaponDef), logHeroText(attackerName))
		return
	}
	// The landed hit carries the riders; a boss's sear joins its damage only.
	landed := cs.consumeDodgeCharge(attack)
	landed.PrimaryHit = true
	primary := landed
	if outcome == instakillSear {
		primary = cs.searDisintegrated(monster, landed)
	}

	// The immutable packet is reused by every AoE victim. Each target resolves
	// its own armor, bonus-vs and resistances; the packet pays flat soak once.
	hit := cs.applyPartyMonsterAttack(monster, primary)
	finalDamage, isCrit := hit.Total(), hit.Critical
	cs.markMonsterHit(monster)
	cs.game.reportDamage(logToneGood, damageLog{
		Source:   logHeroText(attackerName),
		Target:   monsterLogTarget(monster),
		Amount:   finalDamage,
		School:   damageTypeStr,
		Shares:   dealtShares(hit.DealtDamage),
		Critical: isCrit,
	})
	cs.trySleightOfHand(attacker, monster)
	cs.spawnWeaponHitImpactFX(monster, finalDamage, weaponDef)
	cs.settlePartyHit(monster, weaponDef, attacker, attackerName, landed.resolvedAs(hit), nil, nil)
	if radius := weaponAoeRadius(weaponDef); radius > 0 {
		cs.applyAoeSplash(monster, attack, radius)
	}
}

// settlePartyHit is what every landed party hit does after its damage (and its
// log line): the weapon's riders plus the caller's own (a spell's stun), the
// toll the hit sets ringing, the Maw's execute, and the kill finalization.
// landed is the hit's own attack as it resolved (resolvedAs: a designation
// critical included), without a boss's disintegrate sear.
// executed means the Maw finished the target.
func (cs *CombatSystem) settlePartyHit(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig,
	attacker *character.MMCharacter, attackerName string, landed partyMonsterAttack, shot *Arrow, riders func()) (executed bool) {
	if weaponDef != nil && attacker != nil && weaponDef.HitShellAbsorption > 0 {
		attacker.ReactiveCombat.Shell = weaponDef.HitShellAbsorption
	}
	cs.applyElementalWeaponAbility(monster, weaponDef, attacker, shot)
	cs.applySignatureHit(monster, weaponDef, attacker, shot)
	if weaponDef != nil && attacker != nil {
		cs.applyCrumbling(monster)
	}
	if monster.IsAlive() {
		cs.tryApplyWeaponHitRiders(monster, weaponDef)
		if weaponDef != nil && weaponDef.TollEchoPct > 0 {
			cs.scheduleToll(monster, landed, weaponDef)
		}
		if riders != nil {
			riders()
		}
		executed = cs.tryWeaponExecute(monster, weaponDef, attacker, attackerName)
	}
	if !monster.IsAlive() && !executed {
		cs.finishWeaponKill(monster, weaponDef, attacker)
	}
	return executed
}

// searDisintegrated announces a disintegrate proc that a boss withstands and
// returns its attack carrying the proc's true damage.
func (cs *CombatSystem) searDisintegrated(monster *monsterPkg.Monster3D, attack partyMonsterAttack) partyMonsterAttack {
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.instakill.withstands", logSlot, logSlot), logMonsterName(monster), uitext.Text("combat.instakill.disintegration"))
	return withInstakillSear(attack, monster)
}

// disintegratePartyTarget is the instakill rider of melee and projectile hits:
// the target dies through the shared kill path and the blast still goes off.
func (cs *CombatSystem) disintegratePartyTarget(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig,
	attacker *character.MMCharacter, attack partyMonsterAttack, aoeRadiusTiles float64, who logPart) {
	monster.HitPoints = 0
	cs.markMonsterHit(monster)
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.disintegrates", logSlot, logSlot), who, logMonsterName(monster))
	cs.finishWeaponKill(monster, weaponDef, attacker)
	if aoeRadiusTiles > 0 {
		cs.applyAoeSplash(monster, attack, aoeRadiusTiles)
	}
}

func critPrefix(isCrit bool) string {
	if isCrit {
		return "Critical! "
	}
	return ""
}

func weaponAoeRadius(def *config.WeaponDefinitionConfig) float64 {
	if def == nil {
		return 0
	}
	return def.AoeRadiusTiles
}

// engageTurnBasedSameKindPackOnPartyHit is the one explicit exception to the
// shared sight-agro rule. Only in TB, a party-caused hit may alert nearby
// same-key monsters, but each neighbour must still have direct LoS to the
// party. It is intentionally not an RT mechanic and it never behaves like an
// alarm through walls. Witnessing the hit uses the shared provocation leash.
func (cs *CombatSystem) engageTurnBasedSameKindPackOnPartyHit(hit *monsterPkg.Monster3D) {
	if cs == nil || cs.game == nil || cs.game.world == nil || !cs.game.turnBasedMode || hit == nil {
		return
	}

	tileSize := float64(cs.game.config.GetTileSize())
	radius := tileSize * TurnBasedPackAggroRadiusTiles
	hitKey := hit.Key // pack by exact type (key), not display Name

	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || m.Key != hitKey ||
			Distance(hit.X, hit.Y, m.X, m.Y) > radius {
			continue
		}

		if !m.CanStartPlayerAggro() ||
			!m.HasLineOfSightToPlayer(cs.game.collisionSystem, cs.game.camera.X, cs.game.camera.Y) {
			continue
		}
		cs.game.answerBandAlarm(m, true, true)
	}
}

// tbPersonalActionFloor is a member's guaranteed TB actions before the
// party-wide Speed bonus pool: one by default, dual-wield's floor, or the
// weapon-authored floor - whichever is highest.
func tbPersonalActionFloor(m *character.MMCharacter) int {
	if m == nil {
		return 0
	}
	floor := 1
	if m.IsDualWielding() {
		floor = 2
	}
	if base := weaponTBActionsPerRound(m); base > floor {
		floor = base
	}
	return floor
}

// weaponTBActionsPerRound is the highest tb_actions_per_round across the
// member's equipped weapons; zero means no override.
func weaponTBActionsPerRound(member *character.MMCharacter) int {
	if member == nil {
		return 0
	}
	best := 0
	for _, def := range equippedWeaponDefinitions(member) {
		if def != nil && def.TBActionsPerRound > best {
			best = def.TBActionsPerRound
		}
	}
	return best
}

// HandleMonsterInteractions handles combat between monsters and the player
func (cs *CombatSystem) HandleMonsterInteractions() {
	for _, m := range cs.game.world.Monsters {
		cs.handleMonsterInteraction(m)
	}
}

// Foreground and off-screen encounters share cooldowns, boss actions and strikes.
func (cs *CombatSystem) handleMonsterInteraction(monster *monsterPkg.Monster3D) {

	if !monster.IsAlive() {
		return
	}

	// Tick the persistent attack cooldown every frame, BEFORE any state checks,
	// so it counts down even while the monster is pursuing/alert. This is what
	// stops a kiting player (stepping in and out of range) from resetting the
	// attack cadence: the AI state can churn, but the cooldown can't be skipped.
	if monster.AttackCDFrames > 0 {
		monster.AttackCDFrames--
	}
	if monster.OffHandCDFrames > 0 {
		monster.OffHandCDFrames--
	}

	monster.TickAbilityClocks(false)
	monster.LimitPlayerEngagement(cs.game.camera.X, cs.game.camera.Y)
	behavior := monster.CurrentAIBehavior()
	if !monster.IsInCombat() && behavior != monsterPkg.AIBehaviorEvasive {
		return
	}
	// Movement AI and TB already hold scripted inactive encounter pieces.
	// RT crossfire is a separate action path, so it must enforce the same gate
	// before a precomputed bound-ally foe can trigger an attack.
	if behavior == monsterPkg.AIBehaviorInert {
		return
	}
	// Stunned monsters take no action (the TB path already skips them; the
	// real-time path must too, or a stun frozen at StateTimer==1 would let a
	// monster pounce/strike every frame for the whole stun). Update() decrements
	// the stun counter; here we just suppress the action.
	if monster.Stun.Frames > 0 {
		return
	}

	// Modes that may not attack own no RT attack action: stale StateAttacking,
	// pounce data, or an attack-post claim must never bypass the policy. An
	// evasive quest boss still reaches the boss rider below, which owns its blink.
	if !behavior.Caps().MayAttack && behavior != monsterPkg.AIBehaviorEvasive {
		return
	}
	// Bound (Bind Undead): hunts the nearest enemy monster using its normal
	// per-monster attack cooldown, never the party.
	if behavior == monsterPkg.AIBehaviorBoundAlly {
		if monster.AttackCDFrames == 0 {
			cs.boundAttackNearest(monster)
		}
		return
	}
	// The frame's crossfire target can die when an earlier actor resolves.
	// Reject it before the boss rider too: otherwise a boss can spend that
	// stale frame casting a party special before the crossfire branch gets a
	// chance to wait for the next shared retarget.
	if behavior == monsterPkg.AIBehaviorFightFoe &&
		(monster.AIFoe == nil || !monster.AIFoe.IsAlive()) {
		cs.game.releaseMonsterAttackPost(monster)
		return
	}
	if cs.runTelegraphedAction(monster, monster.AttackCDFrames == 0) {
		return
	}

	// Boss specials ride EVERY fight - party or a summon that out-competed it
	// for aggro. After the stun/charm/bind and bound-ally gates (they still
	// suppress boss actions), BEFORE the crossfire branch that used to swallow
	// the kit. Evasive quest bosses resolve here too: updateBoss owns their
	// blink and always consumes the action.
	if monster.IsBoss() {
		attackTick := cs.bossActionTick(monster)
		if cs.runBossSpecials(monster, attackTick, false) {
			if attackTick && !cs.bossEvasive(monster) {
				cs.armMonsterRTAttackCooldowns(monster)
			}
			return
		}
	}

	// Lured at a bound undead instead of the party: attack it on the monster's
	// normal individual cooldown whenever within reach. The frame snapshot can
	// outlive that foe when an earlier monster kills it; in that case this actor
	// waits for the next shared retarget instead of falling into party combat.
	if behavior == monsterPkg.AIBehaviorFightFoe {
		if cs.commitMonsterAttack(monster, monsterAttackDestination{foe: monster.AIFoe}, monsterAttackRealtime) {
			return
		}
		if monsterInAttackTransit(monster) {
			cs.game.releaseMonsterAttackPost(monster)
		}
		cs.tryPhaseShift(monster, false) // across a gap toward the foe, as toward the party
		return
	}

	// Phase Shift (real-time): on the planner's launch tile, cross the gap
	// instead of waiting at its edge. A movement: no strike this frame.
	if cs.tryPhaseShift(monster, false) {
		return
	}

	// Pounce (real-time): from within its grid reach (pounceGridReach, as in
	// TB), leap to melee contact and strike immediately, then go on cooldown.
	if monster.CanPounce() {
		if monster.PounceCD.Frames == 0 && cs.pounceGridReach(monster, cs.game.camera.X, cs.game.camera.Y) &&
			cs.monsterCanPounceParty(monster) {
			if cs.executePounce(monster, cs.game.camera.X, cs.game.camera.Y) {
				cs.game.logCombatLine(logToneBad, uitext.Text("combat.at_the_party", logSlot, logSlot), logMonsterName(monster), logAbility("pounces"))
				cs.commitMonsterAttack(monster, monsterAttackDestination{}, monsterAttackPounce)
				monster.ArmPounceCooldown(cs.game.config.GetTPS(), TurnBasedPounceCooldownTurns)
				return
			}
		}
	}

	// If monster is in attacking state and within attack range, perform attack.
	// Inclusive (<=) so a mob sitting exactly one tile away (e.g. a puma that
	// just pounced onto an adjacent tile) still lands its hit. Melee monsters
	// also count diagonally-adjacent tiles as point-blank so they can surround
	// the party instead of queueing only on N/S/E/W.
	if monster.State == monsterPkg.StateAttacking && cs.partyInStrikeReach(monster, false) {
		cs.commitMonsterAttack(monster, monsterAttackDestination{}, monsterAttackRealtime)
	}
}

// armMonsterRTAttackCooldowns marks a spent attack action on every real-time
// hand stream. TB uses it when carrying an action back across a mode switch;
// RT specials use it when replacing a normal strike. Taking the max preserves
// a longer cooldown already in flight.
func (cs *CombatSystem) armMonsterRTAttackCooldowns(attacker *monsterPkg.Monster3D) {
	if cs == nil || cs.game == nil || attacker == nil {
		return
	}
	if cooldown := attacker.AttackCooldownFrames(); cooldown > attacker.AttackCDFrames {
		attacker.AttackCDFrames = cooldown
	}
	if !attacker.IsChampion() || attacker.HasRangedAttack() {
		return
	}
	champion := cs.game.championTemplateFor(attacker)
	if champion == nil {
		return
	}
	if _, dual := championOffHandWeapon(champion); !dual {
		return
	}
	if cooldown := cs.championOffHandCooldownFrames(attacker, champion); cooldown > attacker.OffHandCDFrames {
		attacker.OffHandCDFrames = cooldown
	}
}

// monsterCanPounceParty is the shared non-range gate for RT and TB leaps. A
// pounce is a party attack, so it requires both an active party target and a
// direct line of sight; it cannot create aggro or teleport through a wall.
func (cs *CombatSystem) monsterCanPounceParty(m *monsterPkg.Monster3D) bool {
	if cs == nil || cs.game == nil || m == nil || !m.TargetsParty() {
		return false
	}
	return cs.game.collisionSystem == nil ||
		cs.attackLineClear(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y)
}

// pounceLanding is a read-only landing search shared with the threat preview.
func (cs *CombatSystem) pounceLanding(m *monsterPkg.Monster3D, playerX, playerY float64) (float64, float64, bool) {
	if cs.game.collisionSystem == nil {
		return 0, 0, false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	ptx, pty := TileIndex(playerX, tileSize), TileIndex(playerY, tileSize)

	cands := [8][2]int{
		{ptx + 1, pty}, {ptx - 1, pty}, {ptx, pty + 1}, {ptx, pty - 1},
		{ptx + 1, pty + 1}, {ptx + 1, pty - 1}, {ptx - 1, pty + 1}, {ptx - 1, pty - 1},
	}
	bestX, bestY, bestD := m.X, m.Y, math.MaxFloat64
	found := false
	for _, c := range cands {
		cx, cy := TileCenterFromTile(c[0], c[1], tileSize)
		if cs.game.collisionSystem.IsMonsterAttackPostReserved(m.ID, cx, cy) {
			continue
		}
		if !m.CanTraverseTile(cs.game.collisionSystem, monsterPkg.TileCoord{X: c[0], Y: c[1]}) || !cs.attackLineClear(cx, cy, playerX, playerY) {
			continue
		}
		if d := (cx-m.X)*(cx-m.X) + (cy-m.Y)*(cy-m.Y); d < bestD {
			bestD, bestX, bestY, found = d, cx, cy, true
		}
	}
	if !found {
		return 0, 0, false // no free adjacent tile
	}
	return bestX, bestY, true
}

// executePounce leaps a pouncing monster onto the nearest walkable tile
// adjacent to the player - never inside the player's own tile (where the sprite
// would vanish). Diagonal-adjacent tiles are valid melee contact. Callers must
// only resolve the strike when it returns true. Shared by RT and TB pounce hooks.
func (cs *CombatSystem) executePounce(m *monsterPkg.Monster3D, playerX, playerY float64) bool {
	bestX, bestY, found := cs.pounceLanding(m, playerX, playerY)
	if !found {
		return false
	}
	oldX, oldY := m.X, m.Y
	m.X, m.Y = bestX, bestY
	cs.game.collisionSystem.UpdateEntity(m.ID, bestX, bestY)
	if !cs.game.tryClaimMonsterAttackPost(m) {
		m.X, m.Y = oldX, oldY
		cs.game.collisionSystem.UpdateEntity(m.ID, oldX, oldY)
		return false
	}
	m.State = monsterPkg.StateAttacking
	m.StateTimer = 0
	m.ResetPathfinding()
	cs.game.refreshMonsterCollisionState(m)
	cs.game.armMonsterAttackAnimation(m)
	return true
}

// monsterCanStrikePoint is the one RT strike-reach rule for a point target,
// the party or a foe. A monster crossing another attacker's tile is only in
// transit and strikes nothing. A ranged profile supplies the long-distance
// option, not a ban on hand-to-hand: an adjacent attacker may use its melee
// school even when its radial range is shorter than a diagonal.
func (cs *CombatSystem) monsterCanStrikePoint(m *monsterPkg.Monster3D, x, y float64) bool {
	if m == nil || monsterInAttackTransit(m) || !cs.attackLineClear(m.X, m.Y, x, y) {
		return false
	}
	if m.HasRangedAttack() && cs.monsterUsesMeleeAgainstPoint(m, x, y) {
		return true
	}
	if Distance(m.X, m.Y, x, y) <= m.GetAttackRangePixels() {
		return true
	}
	return !m.HasRangedAttack() && cs.monsterMeleeAdjacentToPoint(m, x, y)
}

// monsterMeleeAdjacentToParty is melee contact with the party at the logical
// (un-shaken) camera, so the pull gate cannot flip near a wall.
func (cs *CombatSystem) monsterMeleeAdjacentToParty(monster *monsterPkg.Monster3D) bool {
	camX, camY := cs.logicalCameraXY()
	return cs.monsterMeleeAdjacentToPoint(monster, camX, camY)
}

// partyInStrikeReach is whether the monster can strike the party now: the TB
// grid reach, or the RT strike rule. The logical (un-shaken) camera keeps it
// shake-invariant near walls (see logicalCameraXY).
func (cs *CombatSystem) partyInStrikeReach(m *monsterPkg.Monster3D, turnBased bool) bool {
	x, y := cs.logicalCameraXY()
	if turnBased {
		return !monsterInAttackTransit(m) && cs.monsterAttackPointInReachTB(m, x, y)
	}
	return cs.monsterCanStrikePoint(m, x, y)
}

// monsterUsesMeleeAgainstParty is the single party-target selector for normal
// monster attacks. A projectile-capable monster strikes in melee on any clear
// adjacent tile; otherwise it keeps its authored ranged attack.
func (cs *CombatSystem) monsterUsesMeleeAgainstParty(monster *monsterPkg.Monster3D) bool {
	camX, camY := cs.logicalCameraXY()
	return cs.monsterUsesMeleeAgainstPoint(monster, camX, camY)
}

// monsterUsesMeleeAgainstPoint selects delivery without changing damage data:
// melee resolves its shared profile, while the ranged path keeps the projectile
// spell or weapon's own school.
func (cs *CombatSystem) monsterUsesMeleeAgainstPoint(monster *monsterPkg.Monster3D, targetX, targetY float64) bool {
	if monster == nil || !monster.HasRangedAttack() {
		return monster != nil
	}
	// Arena champions are character builds whose `ranged` flag explicitly owns
	// their main-hand delivery; they do not use the ordinary melee profile.
	if monster.IsChampion() {
		return false
	}
	return cs.monsterMeleeAdjacentToPoint(monster, targetX, targetY)
}

func (cs *CombatSystem) performMonsterAttackAgainstParty(monster *monsterPkg.Monster3D) {
	if cs.monsterUsesMeleeAgainstParty(monster) {
		cs.applyMonsterMeleeDamage(monster)
		return
	}
	cs.spawnMonsterRangedAttack(monster) // lands with its shot
}

func (cs *CombatSystem) performMonsterAttackAgainstMonster(attacker, target *monsterPkg.Monster3D, owner ProjectileOwner) {
	if attacker == nil || target == nil || !target.IsAlive() {
		return
	}
	if cs.tryMonsterAttackSpecial(attacker, monsterAttackDestination{foe: target}) {
		return
	}
	if cs.monsterUsesMeleeAgainstPoint(attacker, target.X, target.Y) {
		if attacker.IsChampion() {
			cs.championAlternatingCrossfireStrike(attacker, target)
		} else {
			cs.monsterStrikeMonster(attacker, target)
		}
		cs.monsterAttackLanded(attacker)
		return
	}
	cs.spawnMonsterRangedAttackAtMonster(attacker, target, owner) // lands with its shot
}

// monsterMeleeAdjacentToPoint is the one tile-adjacency/LoS rule for melee
// delivery against any point target (party or another monster).
func (cs *CombatSystem) monsterMeleeAdjacentToPoint(monster *monsterPkg.Monster3D, targetX, targetY float64) bool {
	if monster == nil || cs == nil || cs.game == nil {
		return false
	}
	return monster.TileNeighbour(targetX, targetY) && cs.attackLineClear(monster.X, monster.Y, targetX, targetY)
}

func (cs *CombatSystem) applyMonsterMeleeDamage(monster *monsterPkg.Monster3D) {
	camX, camY := cs.logicalCameraXY()
	if monster == nil || !cs.attackLineClear(monster.X, monster.Y, camX, camY) {
		return
	}
	// Every melee strike that reaches the party lands here - a plain swing, a
	// special, a pounce in either mode. A shot lands where it hits instead.
	defer cs.monsterAttackLanded(monster)
	if cs.tryMonsterAttackSpecial(monster, monsterAttackDestination{}) {
		return
	}

	// Champion melee resolves through the character pipeline with the weapon's
	// arc width (and the once-per-swing AoE rule) instead of a single target.
	if monster.IsChampion() && cs.championAlternatingStrike(monster) {
		return
	}

	// Melee hits a random living party member (both RT and TB) through the shared
	// monster->character choke point (dodge, KO, blink, poison and melt riders).
	currentChar := cs.randomLivingMember()
	if currentChar == nil {
		return
	}
	cs.game.playMonsterSound(soundMonsterMeleeSwing, monster)
	cs.monsterHitCharacter(
		monster,
		currentChar,
		monster.Name,
		cs.normalMonsterMeleeHitRolled(monster),
	)
	// No knockback: monster attacks are already gated to once per attacking state
	// (StateTimer==1) plus pounce cooldowns, so the old anti-spam pushback is moot.
}

type monsterCharacterHit struct {
	SpellID            string
	ElementalAttack    bool
	Parts              damagecalc.Parts
	DamageType         string
	IgnoresArmor       bool
	ArmorPiercePct     int
	GrazesDodge        bool
	DisintegrateChance float64
	Melee              bool
	// Spell labels the CHANNEL this hit arrived through: spell projectiles,
	// spell-delivered party attacks and champion spell splashes are spells;
	// melee, weapon projectiles, shot party attacks and traps are not. Spell Absorption reads only
	// this flag - never a name or damage-type heuristic.
	Spell bool
	// Hazard marks an authored field (the Brood Mother's brood-fire trap), not
	// an attack: Shield's ranged ward and Pain Reflection ignore it.
	Hazard bool
	// Projectile: a weapon shot or spell bolt, or its splash. A physical one
	// may pierce armor (physicalProjectilePierces).
	Projectile bool
	// NoRiders: an area ability authored riders: false - the monster's
	// on-hit riders stay off this hit.
	NoRiders bool
	// Flame burns the member's card when the hit lands (a fire nova); LogKind
	// tags its damage line.
	Flame   bool
	LogKind damageLogKind
	// Critical: the attacker's roll crit (a champion's weapon or spell, a
	// boss's critical_chance); its line says so like a party crit.
	Critical bool
}

// rangedWeaponShot is a weapon dart: the hit Shield's ranged ward softens.
func (h monsterCharacterHit) rangedWeaponShot() bool { return h.Projectile && !h.Spell }

func hitFromMonster(monster *monsterPkg.Monster3D, normalDamage int, damageType string, ignoresArmor bool, disintegrateChance float64, melee, spell bool) monsterCharacterHit {
	hit := monsterCharacterHit{
		Parts:              damagecalc.Parts{Normal: normalDamage},
		DamageType:         damageType,
		IgnoresArmor:       ignoresArmor,
		DisintegrateChance: disintegrateChance,
		Melee:              melee,
		Spell:              spell,
	}
	if monster != nil {
		hit.Parts.True = monster.TrueDamage
		hit.Parts = monster.OutgoingDamage(hit.Parts)
		hit.GrazesDodge = monster.GrazesDodge
	}
	return hit
}

// monsterHitCharacter is the one choke point for a monster damaging a party
// member: strikes, shots, casts and fields alike, each member of a whole-party
// attack with its own hit and its own dodge roll. The hit snapshots normal/true
// components and its dodge rider before resolution, so an in-flight champion
// projectile cannot inherit a later hand or spell's mutable state.
func (cs *CombatSystem) monsterHitCharacter(monster *monsterPkg.Monster3D, target *character.MMCharacter, sourceName string, hit monsterCharacterHit) {
	cs.recordMonsterPartyAttack(monster)
	defer cs.game.beginProfileMonsterHit(monster, sourceName)()
	if target == nil {
		return
	}
	if sourceName == "" {
		sourceName = "Monster"
	}
	// Ward eligibility deliberately exceeds hit.Spell; see magicWardBlocks.
	// Melee and Hazard describe delivery, not immunity to the ward.
	if cs.magicWardBlocks(hit.DamageType, hit.Spell, true) {
		return
	}
	if hit.rangedWeaponShot() {
		if b := cs.game.advancedBuff(config.MechanicShield); b != nil {
			hit.Parts.Normal = hit.Parts.Normal * (100 - b.RangedWardPct) / 100
			hit.Parts.True = hit.Parts.True * (100 - b.RangedWardPct) / 100
		}
	}
	// Spell Absorption preempts everything - an absorbed spell has nothing left
	// to dodge, disintegrate with, or ride a status on.
	if cs.tryAbsorbSpellHit(target, hit.Parts, hit.Spell, sourceName) {
		return
	}

	// Perfect Dodge: luck/5% to avoid the hit. The dodge evades the mitigable part,
	// but a monster's TRUE damage lands anyway (mirrors party weapon-mastery true,
	// which pierces a monster's dodge), and a champion's Grandmaster hit grazes
	// through (throughDodgeParts) - the rule a GM party member has against
	// monsters. No riders; what gets through meets the normal defenses.
	if dodged, _ := cs.RollPerfectDodge(target); dodged {
		cs.game.gainDodgeCharge(target)
		pierce := cs.projectilePierce(hit.Projectile, hit.DamageType)
		through := throughDodgeParts(hit.Parts, hit.GrazesDodge)
		mitigated := cs.mitigateCharacterParts(through, hit.DamageType, target, hit.IgnoresArmor, hit.ArmorPiercePct, pierce)
		dealt := mitigated.Total()
		cs.stormReversal(monster, target, hit, dealt, pierce)
		if dealt <= 0 {
			cs.game.logCombatLine(logToneNone, uitext.Text("combat.perfect_dodge_evades_s_attack", logSlot, logSlot), logHeroName(target), logMonsterAs(monster, sourceName))
			return
		}
		school := "" // only true damage got through
		if through.Normal > 0 {
			school = normalizeDamageTypeStr(hit.DamageType)
		}
		cs.landHeroDamage(monster, target, sourceName, hit, mitigated, school, damageLogHeroDodge, false)
		return
	}

	if hit.DisintegrateChance > 0 && rand.Float64() < hit.DisintegrateChance {
		targetIndex := cs.findCharacterIndex(target)
		damage := target.HitPoints
		if protected := cs.game.preserveFromLethal(target, damage); protected < damage {
			target.HitPoints -= protected
			cs.game.TriggerDamageHit(targetIndex, protected)
			return
		}
		target.Eradicate()
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.is_by", logSlot, logSlot, logSlot), logHeroName(target), logColored("eradicated", logDyingHP), logMonsterAs(monster, sourceName))
		cs.game.TriggerDamageHit(targetIndex, damage)
		return
	}

	mitigated := cs.mitigateCharacterDamagePartsWithArmorPierce(
		hit.Parts,
		hit.DamageType,
		target,
		hit.IgnoresArmor,
		hit.ArmorPiercePct,
		hit.Projectile,
	)
	cs.landHeroDamage(monster, target, sourceName, hit, mitigated, hit.DamageType, hit.LogKind, true)
}

// landHeroDamage settles a monster hit's mitigated damage on a hero: hit
// triggers (resolveHeroHitDamage), the HP loss and its log line, knockout, the
// damage flash; a struck (undodged) hit adds its FX and the attacker's riders;
// every landing answers with reflection and grows Drakehide scales.
func (cs *CombatSystem) landHeroDamage(monster *monsterPkg.Monster3D, target *character.MMCharacter, sourceName string, hit monsterCharacterHit, mitigated damagecalc.Parts, school string, kind damageLogKind, struck bool) {
	dealt := cs.resolveHeroHitDamage(monster, target, mitigated.Total())
	attacker := sourceName // the hit is the attacking monster's, whatever its ability
	if monster != nil {
		attacker = monster.Name
	}
	cs.takeHeroHP(target, dealt, attacker)
	cs.game.reportDamage(logToneBad, damageLog{
		Source:   logMonsterAs(monster, sourceName),
		Target:   heroLogTarget(target),
		Amount:   dealt,
		School:   school,
		Shares:   heroShares(dealt, mitigated, normalizeDamageTypeStr(hit.DamageType)),
		Kind:     kind,
		Critical: hit.Critical && struck,
	})
	if target.HitPoints == 0 {
		cs.knockOut(target)
	}
	targetIndex := cs.findCharacterIndex(target)
	cs.game.TriggerDamageHit(targetIndex, dealt)
	if struck && dealt > 0 {
		if hit.ElementalAttack {
			cs.game.addElementalAttackFX(0, 0, target, hit.DamageType)
		}
		if hit.Flame {
			cs.game.TriggerPartyFlame(targetIndex)
		}
	}
	if struck && monster != nil && !hit.NoRiders {
		cs.applyMonsterHitRiders(monster, hit.SpellID, heroHitTarget{cs, target}, hit.Melee || hit.Projectile)
	}
	cs.reflectMonsterDamage(monster, target, dealt, hit.Melee, hit.Hazard)
	growScaleStacks(target, dealt) // Drakehide: every damaging hit grows a scale, a dodge's graze too
}

// reflectMonsterDamage answers damage actually received, including a typed true
// component that landed through dodge. Card thorns answer any hit; Parrying
// Dagger answers only melee; Pain Reflection answers attacks, not hazards.
func (cs *CombatSystem) reflectMonsterDamage(monster *monsterPkg.Monster3D, target *character.MMCharacter, received int, melee, hazard bool) {
	if monster == nil || target == nil || received <= 0 || !monster.IsAlive() {
		return
	}
	pct := cs.game.cardThornsPct()
	if b := cs.game.advancedBuff(config.MechanicPainReflection); b != nil && !hazard {
		pct += b.ReflectionPct
	}
	firePct := 0
	if melee {
		pct += weaponThornsPct(target)
		firePct = target.SetFieryRipostePct() // Drakeforged pair answers in fire
	}
	reflected := received * pct / 100
	fireBack := received * firePct / 100
	if reflected <= 0 && fireBack <= 0 {
		return
	}
	// Thorns and the fiery riposte are two packets read as one line.
	var dealt monsterPkg.DealtDamage
	answer := func(amount int, school monsterPkg.DamageType) {
		d := cs.applyMonsterDamagePacket(monster, singleMonsterDamagePacket(damagecalc.Parts{Normal: amount}, school.String(), 0), monsterDamageOptions{IgnoreArmor: true})
		dealt.Normal, dealt.True = dealt.Normal+d.Normal, dealt.True+d.True
		dealt.Shares = append(dealt.Shares, d.Shares...)
	}
	if reflected > 0 {
		answer(reflected, monsterPkg.DamagePhysical)
	}
	if fireBack > 0 && monster.IsAlive() {
		answer(fireBack, monsterPkg.DamageFire)
	}
	if dealt.Total() > 0 {
		cs.game.reportDamage(logToneGood, damageLog{
			Target: monsterLogTarget(monster),
			Amount: dealt.Total(),
			School: monsterPkg.DamagePhysical.String(),
			Shares: dealtShares(dealt),
			Kind:   damageLogReflection,
		})
	}
	if !monster.IsAlive() {
		cs.finishMonsterKill(monster)
	}
}

// partyProjectileReflectPct is the strongest projectile_reflect_pct across the
// conscious party's equipment (Broodscale Aegis - the shield guards the rank).
func (g *MMGame) partyProjectileReflectPct() int {
	if g.party == nil {
		return 0
	}
	best := 0
	for _, member := range g.party.Members {
		if member == nil || !member.CanAct() {
			continue
		}
		if pct := member.ProjectileReflectPct(); pct > best {
			best = pct
		}
	}
	return best
}

// tryReflectMonsterProjectile rolls the Aegis mirror-scale against an incoming
// monster projectile. On success the existing projectile turns toward its
// shooter; damage remains deferred until the return flight actually lands.
func (cs *CombatSystem) tryReflectMonsterProjectile(
	source *monsterPkg.Monster3D,
	x, y float64,
	velX, velY *float64,
	lifetime *int,
	owner *ProjectileOwner,
) bool {
	pct := cs.game.partyProjectileReflectPct()
	if pct <= 0 || source == nil || !source.IsAlive() || velX == nil || velY == nil ||
		lifetime == nil || owner == nil || rand.Intn(100) >= pct {
		return false
	}

	dx, dy := source.X-x, source.Y-y
	distance := math.Hypot(dx, dy)
	speed := math.Hypot(*velX, *velY)
	if distance <= 0 || speed <= 0 {
		return false
	}
	*velX = dx / distance * speed
	*velY = dy / distance * speed
	// A long-range shot may have spent nearly its whole authored lifetime on
	// the incoming leg. Guarantee enough frames for the same object to return.
	returnFrames := int(math.Ceil(distance/speed)) + 2
	if *lifetime < returnFrames {
		*lifetime = returnFrames
	}
	*owner = ProjectileOwnerReflected
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.the_mirror_scales_turn_s_bolt", logSlot), logMonsterName(source))
	return true
}

// One gate covers fresh commands, held repeats and aimed mouse attacks. Either
// Grandmaster Pathfinding or an active Wyrmspine Wing permits party sprint combat.
func (g *MMGame) partyFireWhileRunning() bool {
	if g.party == nil {
		return false
	}
	if g.party.PathfindingTier() >= int(character.MasteryGrandMaster) {
		return true
	}
	for _, member := range g.party.Members {
		if member == nil || !member.CanAct() {
			continue
		}
		for _, def := range equippedWeaponDefinitions(member) {
			if def != nil && def.PartyFireWhileRunning {
				return true
			}
		}
	}
	return false
}

// weaponThornsPct sums thorns_pct across the character's equipped weapons
// (Parrying Dagger riposte - either hand counts).
func weaponThornsPct(target *character.MMCharacter) int {
	if target == nil {
		return 0
	}
	total := 0
	for _, def := range equippedWeaponDefinitions(target) {
		if def != nil {
			total += def.ThornsPct
		}
	}
	return total
}

// sleightChancePct is the pickpocket chance for a melee hit: skill levelx10%
// (Novice/Expert/Master/GM -> 10/20/30/40; SkillTier is 0-based, hence the +1).
// 0 without the skill. The SAME function the skill tooltip quotes.
func sleightChancePct(attacker *character.MMCharacter) int {
	if attacker == nil || !attacker.HasSkill(character.SkillSleightOfHand) {
		return 0
	}
	return (attacker.SkillTier(character.SkillSleightOfHand) + 1) * character.SleightChancePctPerTier
}

// trySleightOfHand rolls the attacker's pickpocket on a melee hit (skill
// levelx10% chance); success marks the monster Pilfered (one pick per victim)
// and rolls its loot table - stolen items go to the inventory, a missed loot
// roll pays consolation gold (level-gated). Constants live in
// character/catalog.go so the skill tooltip quotes the same numbers.
func (cs *CombatSystem) trySleightOfHand(attacker *character.MMCharacter, monster *monsterPkg.Monster3D) {
	if attacker == nil || monster.Pilfered || !monster.IsAlive() {
		return
	}
	chance := sleightChancePct(attacker)
	if chance <= 0 || rand.Intn(100) >= chance {
		return
	}
	monster.Pilfered = true
	cs.game.AddColoredCombatMessage(uitext.Text("combat.tries_to_pick_s_pocket", attacker.Name, monster.Name), combatMessagePurple)
	// A champion's drop is its victory trophy: the pocket holds gold only.
	if stolen := cs.pocketLoot(monster); len(stolen) > 0 {
		for _, it := range stolen {
			cs.game.party.AddItem(it)
			cs.game.AddColoredCombatMessage(uitext.Text("combat.picks_pocket", attacker.Name, monster.Name, it.Name), lootMessageColor([]items.Item{it}))
		}
		cs.game.announceLegendaryDrops(stolen)
		return
	}
	gold := character.SleightGoldLow
	if monster.Level > character.SleightHighLevelThreshold {
		gold = character.SleightGoldHighLevel
	}
	cs.game.awardGold(gold)
	cs.game.AddColoredCombatMessage(uitext.Text("combat.finds_no_item_and_lifts_gold", attacker.Name, gold, monster.Name), combatMessagePurple)
}

// spiritualTrainingChancePct is the Monk's Spiritual Training proc chance on a
// melee hit: skill tier including Novice times the catalog value. 0 without the
// skill. The SAME function the skill tooltip quotes.
func spiritualTrainingChancePct(attacker *character.MMCharacter) int {
	if attacker == nil || !attacker.HasSkill(character.SkillSpiritualTraining) {
		return 0
	}
	return (attacker.SkillTier(character.SkillSpiritualTraining) + 1) * character.SpiritualTrainingProcPctPerTier
}

// trySpiritualTraining rolls a melee hit's chance to also fire the attacker's
// slotted OFFENSIVE quick-spell for free (0 SP) - mirrors the Pixie Card's
// free Fire Bolt proc (tryCardFireBoltInstead), just skill-gated instead of
// card-gated, and additive to the swing rather than replacing it.
func (cs *CombatSystem) trySpiritualTraining(attacker *character.MMCharacter) {
	chance := spiritualTrainingChancePct(attacker)
	if chance <= 0 || rand.Intn(100) >= chance {
		return
	}
	spell, ok := attacker.Equipment[items.SlotQuickAction]
	if !ok || (spell.Type != items.ItemBattleSpell && spell.Type != items.ItemUtilitySpell) {
		return
	}
	spellID := spells.SpellID(spell.SpellEffect)
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return
	}
	// Offensive spells only. IsOffensive() excludes heals, revives, buffs
	// (Bless/Heroism/Stone Skin/Hour of Power) and pure utility, so a melee
	// swing can't free-proc a party buff - only an attack spell like the
	// Monk's Mind Blast, matching the skill's intent and the Pixie precedent.
	if !def.IsOffensive() {
		return
	}
	cs.castResolvedSpell(spellID, def, attacker, 0, false, false)
}

// hitStatusTarget is whoever a landed monster hit struck, as the hit's on-hit
// statuses see it: a hero or another monster, each with its own resistances,
// durations and log line. DoTs take frames, timed statuses whole rounds.
type hitStatusTarget interface {
	poison(frames int)
	ignite(frames int)
	stun(rounds int)
	dispel(attacker *monsterPkg.Monster3D)
	meltArmor(rounds int)
}

// applyMonsterHitRiders rolls a landed monster hit's on-hit statuses once, in
// one order - poison, ignite, stun (a bolt's own spell rider when it has one),
// dispel, armor melt - and lands each on the struck target through its own
// rules. A champion's striking weapon is stamped onto these fields before it
// swings. weaponHit is a melee or projectile hit, the only ones that can melt
// armor.
func (cs *CombatSystem) applyMonsterHitRiders(attacker *monsterPkg.Monster3D, spellID string, target hitStatusTarget, weaponHit bool) {
	if attacker == nil || target == nil {
		return
	}
	tps := cs.game.config.GetTPS()
	if attacker.Abilities.Poison.Chance > 0 && rand.Float64() < attacker.Abilities.Poison.Chance {
		target.poison(tps * attacker.Abilities.Poison.DurationSeconds) // guaranteed by load-time validation
	}
	if attacker.Abilities.Ignite.Chance > 0 && rand.Float64() < attacker.Abilities.Ignite.Chance {
		target.ignite(tps * attacker.Abilities.Ignite.DurationSeconds)
	}
	stun := attacker.Abilities.Stun
	if spellID != "" {
		rider := attacker.ProjectileStun(spellID)
		stun.Chance, stun.Seconds = rider.Chance, rider.Seconds
	}
	if stun.Chance > 0 && rand.Float64() < stun.Chance {
		target.stun(config.CombatRounds(stun.Seconds))
	}
	if attacker.Abilities.Dispel.Chance > 0 && rand.Float64() < attacker.Abilities.Dispel.Chance {
		target.dispel(attacker)
	}
	if weaponHit && attacker.Abilities.MeltArmor.Chance > 0 && rand.Float64() < attacker.Abilities.MeltArmor.Chance {
		target.meltArmor(config.CombatRounds(config.ArmorMeltSeconds()))
	}
}

// heroHitTarget lands monster riders on a party member: cards and gear shorten
// or resist them, stuns walk the hero's diminishing-returns chain.
type heroHitTarget struct {
	cs   *CombatSystem
	hero *character.MMCharacter
}

func (t heroHitTarget) poison(frames int) {
	cs := t.cs
	// Mummy Card: chance to resist the poison outright.
	if resist := cs.game.cardPoisonResistPct(); resist > 0 && rand.Intn(100) < resist {
		return
	}
	// The Still Court aegis (status_duration_pct) shortens it on its wearer.
	t.hero.ApplyPoison(cs.scaledStatusFrames(t.hero, frames))
	cs.game.logCombatLine(logToneBad, uitext.Text("combat.status.poisoned", logSlot, logSlot), logHeroName(t.hero), logColored("poisoned", logPoison))
}

// ignite is a burn DoT 3x as strong as poison that STACKS with it.
func (t heroHitTarget) ignite(frames int) {
	t.hero.ApplyBurn(t.cs.scaledStatusFrames(t.hero, frames))
	t.cs.game.logCombatLine(logToneBad, uitext.Text("combat.bursts_into", logSlot, logSlot), logHeroName(t.hero), logSchoolWord(monsterPkg.DamageFire.String(), "flames"))
}

func (t heroHitTarget) stun(rounds int) {
	if !t.cs.applyScaledCharStun(t.hero, rounds) {
		t.cs.game.logCombatLine(logToneGood, uitext.Text("combat.resists_the", logSlot, logSlot), logHeroName(t.hero), logKeyword("control", "stun"))
		return
	}
	t.cs.game.logCombatLine(logToneBad, uitext.Text("combat.is", logSlot, logSlot, logSlot), logHeroName(t.hero), logKeyword("control", "stunned"), t.cs.game.logStatusLength(t.hero.Stun.Turns, t.hero.Stun.Frames))
}

// meltArmor follows the wearer's status-duration shift (Still Court aegis).
func (t heroHitTarget) meltArmor(rounds int) {
	rounds = config.ScaleDuration(rounds, clampHostileStatusDurationPct(t.hero.ItemStatusDurationPct()))
	fresh := !t.hero.ArmorMelted()
	t.hero.MeltArmor(config.RoundFrames(rounds, t.cs.game.config.GetTPS()), rounds)
	if fresh {
		t.cs.game.logCombatLine(logToneBad, uitext.Text("combat.s_armor", logSlot, logSlot), logHeroName(t.hero), logColored("melts", logDyingHP))
	}
}

// dispel strips one random active party buff (stat or combat). Buffs are
// party-wide, so the struck hero only triggers the roll.
func (t heroHitTarget) dispel(monster *monsterPkg.Monster3D) {
	cs := t.cs
	type dispelTarget struct {
		spellID string
		combat  bool
	}
	var pool []dispelTarget
	for i := range cs.game.statBuffs {
		if cs.game.statBuffs[i].SourceID != "" {
			continue
		}
		pool = append(pool, dispelTarget{cs.game.statBuffs[i].SpellID, false})
	}
	for i := range cs.game.combatBuffs {
		if cs.game.combatBuffs[i].SourceID != "" {
			continue
		}
		pool = append(pool, dispelTarget{cs.game.combatBuffs[i].ID, true})
	}
	if len(pool) == 0 {
		return
	}
	pick := pool[rand.Intn(len(pool))]
	name := buffSourceName(pick.spellID)
	if pick.combat {
		cs.game.removeCombatBuff(pick.spellID)
	} else {
		cs.game.removeStatBuff(pick.spellID)
	}
	cs.game.logCombatLine(logToneBad, uitext.Text("combat.rips_from_the_party", logSlot, logSlot), logMonsterName(monster), logKeyword("buff", name))
}

// monsterHitTarget lands monster riders on another monster: a party summon
// struck by a foe, or a foe struck by the party's ally. Its stuns share the
// monsters' diminishing returns; a wildlife hunt stays out of the log.
type monsterHitTarget struct {
	cs               *CombatSystem
	attacker, target *monsterPkg.Monster3D
}

func (t monsterHitTarget) announce() bool { return !quietActorCombat(t.attacker, t.target) }

func (t monsterHitTarget) poison(frames int) {
	t.cs.afflictMonster(t.target, monsterPoison, frames, monsterStatusTone(t.target), !t.announce())
}

func (t monsterHitTarget) ignite(frames int) {
	t.cs.afflictMonster(t.target, monsterBurn, frames, monsterStatusTone(t.target), !t.announce())
}

// monsterDoT is a damage-over-time status a monster can carry.
type monsterDoT uint8

const (
	monsterPoison monsterDoT = iota
	monsterBurn
)

// afflictMonster lands a DoT on m from any source - a hit's rider, a weapon,
// a card, a flask, a field - and names it in the log when it starts; a
// refresh is silent, as is an immune target. quiet keeps a wildlife hunt out
// of the log.
func (cs *CombatSystem) afflictMonster(m *monsterPkg.Monster3D, dot monsterDoT, frames int, tone logTone, quiet bool) {
	if m == nil || frames <= 0 {
		return
	}
	switch dot {
	case monsterPoison:
		started := m.PoisonedFramesRemaining <= 0
		if m.ApplyPoison(frames) && started && !quiet {
			cs.game.logCombatLine(tone, uitext.Text("combat.status.poisoned", logSlot, logSlot), logMonsterName(m), logColored(uitext.Text("combat.status.poisoned_word"), logPoison))
		}
	case monsterBurn:
		started := m.BurnFramesRemaining <= 0
		m.ApplyBurn(frames)
		if started && !quiet {
			cs.game.logCombatLine(tone, uitext.Text("combat.status.ignited", logSlot, logSlot), logMonsterName(m), logSchoolWord(monsterPkg.DamageFire.String(), uitext.Text("combat.status.fire_word")))
		}
	}
}

func (t monsterHitTarget) stun(rounds int) {
	t.cs.applyStunDR(t.target, rounds, t.announce())
}

// dispel answers a hit on the party only: party buffs are the party's.
func (monsterHitTarget) dispel(*monsterPkg.Monster3D) {}

func (t monsterHitTarget) meltArmor(rounds int) {
	fresh := !t.target.ArmorMelted()
	t.target.MeltArmor(config.RoundFrames(rounds, t.cs.game.config.GetTPS()), rounds)
	if fresh && t.announce() {
		t.cs.game.logCombatLine(monsterStatusTone(t.target), uitext.Text("combat.s_armor", logSlot, logSlot), logMonsterName(t.target), logColored("melts", logDyingHP))
	}
}

// monsterStatusTone is how an affliction on a monster reads for the party:
// good on a foe, bad on the party's own ally.
func monsterStatusTone(m *monsterPkg.Monster3D) logTone {
	if monsterIsPartyAlly(m) {
		return logToneBad
	}
	return logToneGood
}

// applyScaledCharStun stuns a party member for whole rounds, first applying the
// wearer's set stun-duration modifier (Pit Fighter's Quilt: the quilting soaks
// the blow - floor of one round so a landed stun is never a no-op). The one
// place the stun-resist scaling lives, shared by weapon/monster stuns and
// champion casts.
func (cs *CombatSystem) applyScaledCharStun(target *character.MMCharacter, rounds int) bool {
	// Sets (Pit Fighter's Quilt) and per-item shifts (Still Court aegis) stack;
	// the same -90 floor keeps a landed stun from vanishing outright.
	rounds = config.ScaleDuration(rounds, clampHostileStatusDurationPct(target.SetStunDurationPct()+target.ItemStatusDurationPct()))
	// Heroes share the monsters' diminishing returns: repeated stuns shrink to
	// immunity until the hero stays stun-free for the reset window.
	rounds, frames := target.StunDR().Step(rounds, cs.game.config.GetTPS())
	if rounds <= 0 {
		return false
	}
	target.ApplyCharStun(frames, rounds)
	return true
}

func clampHostileStatusDurationPct(pct int) int {
	if pct < config.MinHostileStatusDurationPct {
		return config.MinHostileStatusDurationPct
	}
	return pct
}

// scaledStatusFrames applies the wearer's status-duration shift to a hostile
// DoT duration (poison/burn), floored at one frame. A DoT bills elapsed time in
// both modes, so its frames need not be whole rounds.
func (cs *CombatSystem) scaledStatusFrames(target *character.MMCharacter, frames int) int {
	return config.ScaleDuration(frames, clampHostileStatusDurationPct(target.ItemStatusDurationPct()))
}

// tryAbsorbSpellHit is Spell Absorption's SINGLE gate: a hostile SPELL hit on
// a member holding the skill has a per-tier chance to be eaten whole - it
// deals no damage and its own (pre-mitigation) damage returns as both HP and
// SP. Both monster->character sinks consult it before dealing anything; the
// hostile-spell flag comes from the hit's origin, never from a name heuristic.
func (cs *CombatSystem) tryAbsorbSpellHit(member *character.MMCharacter, parts damagecalc.Parts, hostileSpell bool, sourceName string) bool {
	if !hostileSpell || member == nil || !member.HasSkill(character.SkillSpellAbsorption) {
		return false
	}
	chance := character.SpellAbsorbChancePct(member.SkillTier(character.SkillSpellAbsorption))
	if rand.Intn(100) >= chance {
		return false
	}
	restored := parts.Total()
	hpBefore, spBefore := member.HitPoints, member.SpellPoints
	member.HitPoints = min(member.HitPoints+restored, member.MaxHitPoints)
	member.SpellPoints = min(member.SpellPoints+restored, member.MaxSpellPoints)
	hpRestored := member.HitPoints - hpBefore
	spRestored := member.SpellPoints - spBefore
	if sourceName == "" {
		sourceName = "the hostile"
	} else {
		sourceName += "'s"
	}
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.absorbs_spell_hp_sp", logSlot, sourceName, logSlot, logSlot), logHeroName(member), logHealed(hpRestored), logRecoveredSP(spRestored))
	return true
}

// damagePartyMemberElement applies one normal elemental hit to a single party
// member. Special monster attacks that also carry authored true damage use
// damagePartyMemberParts directly. hostileSpell labels whether the channel is
// an enemy spell eligible for Spell Absorption; friendly self-splash is false.
func (cs *CombatSystem) damagePartyMemberElement(idx int, member *character.MMCharacter, rawDamage int, school string, hostileSpell bool) int {
	return cs.damagePartyMemberParts(idx, member, damagecalc.Parts{Normal: rawDamage}, school, hostileSpell)
}

// damagePartyMemberParts applies an undodgeable damage packet through the shared
// party pipeline and returns the damage actually dealt: mitigate
// (armor%/resist/buffs), subtract, clamp at 0, knock out at 0 (the Lich Card
// cheat-death chokepoint), and flash the damage-blink. Callers supply their
// own flavor line, hostility, and any extra VFX (e.g. party flame).
func (cs *CombatSystem) damagePartyMemberParts(idx int, member *character.MMCharacter, parts damagecalc.Parts, school string, hostileSpell bool) int {
	dealt, _ := cs.damagePartyMemberPartsFromSource(idx, member, parts, school, hostileSpell, nil)
	return dealt
}

// damagePartyMemberPartsFromSource retains ownership for undodgeable monster
// hits. Environmental damage and friendly splash have no hostile source. It
// also returns the loss by element for the caller's log line.
func (cs *CombatSystem) damagePartyMemberPartsFromSource(idx int, member *character.MMCharacter, parts damagecalc.Parts, school string, hostileSpell bool, source *monsterPkg.Monster3D) (int, []damageShare) {
	// Apply the same ward policy as monsterHitCharacter. The source gate keeps
	// environmental damage and friendly splash outside that hostile-hit rule.
	if cs.magicWardBlocks(school, hostileSpell, source != nil) {
		return 0, nil
	}
	if cs.tryAbsorbSpellHit(member, parts, hostileSpell, "") {
		return 0, nil
	}
	mitigated := cs.mitigateCharacterDamageParts(parts, school, member, false)
	dealt := cs.resolveHeroHitDamage(source, member, mitigated.Total())
	hostile := ""
	if source != nil {
		hostile = source.Name
	}
	cs.takeHeroHP(member, dealt, hostile)
	if member.HitPoints == 0 {
		cs.knockOut(member)
	}
	cs.game.TriggerDamageHit(idx, dealt)
	growScaleStacks(member, dealt) // Drakehide: specials grow scales too
	cs.reflectMonsterDamage(source, member, dealt, false, false)
	return dealt, heroShares(dealt, mitigated, normalizeDamageTypeStr(school))
}

func (g *MMGame) clearPartyScaleStacks() {
	if g == nil || g.party == nil {
		return
	}
	for _, member := range g.party.Members {
		if member != nil {
			member.ScaleStacks = 0
		}
	}
}

// growScaleStacks is the ONE Drakehide hook: any hit that actually cost the
// member HP grows a scale (capped by the gauntlets' authored max).
func growScaleStacks(target *character.MMCharacter, dealt int) {
	if target == nil || dealt <= 0 {
		return
	}
	if per, capMax := target.ScaleStackParams(); per > 0 && target.ScaleStacks < capMax {
		target.ScaleStacks++
	}
}

// takeHeroHP takes damage off a hero, clamped at zero. A hostile hit (named
// source) is shown to the hero-hit observer first.
func (cs *CombatSystem) takeHeroHP(hero *character.MMCharacter, damage int, source string) {
	damage = cs.game.preserveFromLethal(hero, damage)
	if obs := cs.game.heroHitObserver; obs != nil && source != "" {
		obs(hero, source, damage)
	}
	hero.HitPoints = max(0, hero.HitPoints-damage)
}

// redirectDamageThroughSacrifice moves a share of an already-mitigated hit
// from the victim to the strongest living Sacrifice user. The transfer is not
// mitigated a second time and never recurses; DoTs bypass this combat-hit sink.
func (cs *CombatSystem) redirectDamageThroughSacrifice(victim *character.MMCharacter, damage int) int {
	if cs == nil || cs.game == nil || cs.game.party == nil || victim == nil || damage <= 0 {
		return damage
	}
	var protector *character.MMCharacter
	bestPct := 0
	for _, member := range cs.game.party.Members {
		if member == nil || member == victim || member.HitPoints <= 0 || !member.HasSkill(character.SkillSacrifice) {
			continue
		}
		pct := character.SacrificeRedirectPct(member.SkillTier(character.SkillSacrifice))
		if pct > bestPct {
			protector, bestPct = member, pct
		}
	}
	redirected := damage * bestPct / 100
	if protector == nil || redirected <= 0 {
		return damage
	}
	cs.takeHeroHP(protector, redirected, "")
	growScaleStacks(protector, redirected)
	if idx := cs.findCharacterIndex(protector); idx >= 0 {
		// One incoming hit produces one impact sound. The primary target's
		// TriggerDamageHit owns it; Sacrifice adds only the protector's card FX.
		cs.game.triggerDamageFx(idx)
	}
	cs.game.reportDamage(logToneBad, damageLog{
		Source: logHeroName(victim),
		Target: heroLogTarget(protector),
		Amount: redirected,
		Kind:   damageLogSacrifice,
	})
	if protector.HitPoints == 0 {
		cs.knockOut(protector)
	}
	return damage - redirected
}

// spawnRangedHitEffect spawns the impact for a ranged weapon projectile: a
// magical weapon (staff/book with a projectile_school) bursts like a spell in its
// school's colour; a plain arrow freezes where it hit and fades.
func (cs *CombatSystem) spawnRangedHitEffect(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, damage int) {
	// Scale a magical burst by damage (arrow freeze ignores count/size).
	count := SpellParticleCount + damage/2
	if count > 48 {
		count = 48
	}
	size := SpellParticleSize + damage/8
	vx, vy := cs.monsterVisualPos(monster) // burst where the monster is drawn (pulled slot in TB)
	cs.game.spawnWeaponBoltImpact(vx, vy, weaponDef, count, size, visualAnchorFor(monster))
}

func (cs *CombatSystem) spawnMonsterRangedAttack(monster *monsterPkg.Monster3D) {
	if cs.tryMonsterAttackSpecial(monster, monsterAttackDestination{}) {
		return
	}
	cs.spawnMonsterRangedAttackAt(monster, cs.game.camera.X, cs.game.camera.Y, ProjectileOwnerMonster)
}

// tryMonsterAttackSpecial selects one replacement action regardless of target.
// Party attacks strike party members only; they do not redirect their victim
// list to a single summon.
func (cs *CombatSystem) tryMonsterAttackSpecial(monster *monsterPkg.Monster3D, target monsterAttackDestination) bool {
	if monster == nil || monster.IsSilenced() || !monster.IsAlive() || cs.attackOriginBlocked(monster.X, monster.Y) {
		return false
	}
	if cs.championTryCastSpell(monster, target) || cs.tryMonsterAllyHeal(monster) {
		return true
	}
	return target.foe == nil && cs.tryMonsterPartyAttack(monster, monsterPkg.PartyAttackOnAttack)
}

func (cs *CombatSystem) tryMonsterAllyHeal(monster *monsterPkg.Monster3D) bool {
	if monster.Abilities.AllyHeal.Chance <= 0 || monster.Abilities.AllyHeal.Amount <= 0 || rand.Float64() >= monster.Abilities.AllyHeal.Chance {
		return false
	}
	target := cs.pickMonsterAllyHealTarget(monster)
	if target == nil {
		return false
	}
	before := target.HitPoints
	target.HitPoints += monster.Abilities.AllyHeal.Amount
	if target.HitPoints > target.MaxHitPoints {
		target.HitPoints = target.MaxHitPoints
	}
	actual := target.HitPoints - before
	if actual <= 0 {
		return false
	}
	if target == monster {
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.mends_itself_for_hp", logSlot, logSlot, logSlot), logMonsterName(monster), logHealed(actual), logHP(target.HitPoints, target.MaxHitPoints))
	} else {
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.mends_for_hp", logSlot, logSlot, logSlot, logSlot), logMonsterName(monster), logMonsterName(target), logHealed(actual), logHP(target.HitPoints, target.MaxHitPoints))
	}
	return true
}

func (cs *CombatSystem) pickMonsterAllyHealTarget(healer *monsterPkg.Monster3D) *monsterPkg.Monster3D {
	if cs.game == nil || cs.game.world == nil {
		return nil
	}
	radius := healer.AllyHealRadiusPixels()
	bestFrac := math.MaxFloat64
	var best *monsterPkg.Monster3D
	for _, candidate := range cs.game.world.Monsters {
		if candidate == nil || !candidate.IsAlive() || candidate.HitPoints >= candidate.MaxHitPoints {
			continue
		}
		if candidate.Bound != healer.Bound || candidate.Disposition != healer.Disposition {
			continue
		}
		if candidate != healer && Distance(healer.X, healer.Y, candidate.X, candidate.Y) > radius {
			continue
		}
		frac := float64(candidate.HitPoints) / float64(candidate.MaxHitPoints)
		if frac < bestFrac {
			bestFrac = frac
			best = candidate
		}
	}
	return best
}

// spawnMonsterRangedAttackAt fires monster's projectile toward a world point with
// the given owner, dispatching to its spell or weapon projectile. Returns true if
// one was spawned. Party attacks are rolled by the caller.
func (cs *CombatSystem) spawnMonsterRangedAttackAt(monster *monsterPkg.Monster3D, targetX, targetY float64, owner ProjectileOwner) bool {
	if monster == nil || monster.IsSilenced() || !cs.attackLineClear(monster.X, monster.Y, targetX, targetY) {
		return false
	}
	if monster.ProjectileSpell != "" {
		cs.spawnMonsterSpellProjectile(monster, spells.SpellID(monster.ProjectileSpell), targetX, targetY, owner)
		return true
	}
	if monster.ProjectileWeapon != "" {
		cs.spawnMonsterWeaponProjectile(monster, monster.ProjectileWeapon, targetX, targetY, owner)
		return true
	}
	return false
}

// spawnMonsterRangedAttackAtMonster aims spawnMonsterRangedAttackAt at another
// monster (PartyAlly: a party ally's shot at an enemy; MonsterAtActor: a mob's
// shot at an ally, a caravan or prey).
func (cs *CombatSystem) spawnMonsterRangedAttackAtMonster(monster, target *monsterPkg.Monster3D, owner ProjectileOwner) bool {
	return cs.spawnMonsterRangedAttackAt(monster, target.X, target.Y, owner)
}

func (cs *CombatSystem) spawnMonsterSpellProjectile(monster *monsterPkg.Monster3D, spellID spells.SpellID, targetX, targetY float64, owner ProjectileOwner) {
	damage, crit := cs.monsterAttackDamage(monster)
	cs.spawnMonsterSpellProjectileDamage(
		monster,
		spellID,
		targetX,
		targetY,
		owner,
		damagecalc.Parts{Normal: damage, True: monster.TrueDamage},
		monster.GrazesDodge,
		crit,
	)
}

// spawnMonsterSpellProjectileDamage is the hit-explicit core: champion
// casts author the damage from the real spell formula (championCastSpell),
// while plain projectile_spell mobs keep their authored attack damage.
func (cs *CombatSystem) spawnMonsterSpellProjectileDamage(monster *monsterPkg.Monster3D, spellID spells.SpellID, targetX, targetY float64, owner ProjectileOwner, parts damagecalc.Parts, grazesDodge, crit bool) {
	// Projectile damage is immutable once fired. Snapshot source-side modifiers
	// here so a Weaken expiring or landing while the bolt is in flight cannot
	// rewrite an already committed attack.
	parts, bossCrit := cs.monsterOutgoingDamage(monster, parts)
	crit = crit || bossCrit
	mp, err := cs.newSpellProjectile(spellID, "monster_", monster.X, monster.Y, math.Atan2(targetY-monster.Y, targetX-monster.X))
	if err != nil {
		return
	}
	mp.Damage, mp.TrueDamage, mp.GrazesDodge, mp.Crit = parts.Normal, parts.True, grazesDodge, crit
	mp.Owner, mp.SourceName, mp.SourceMonster = owner, monster.Name, monster
	if cs.launchSpellProjectile(mp) != nil {
		return
	}
	if owner == ProjectileOwnerMonster {
		cs.recordMonsterPartyAttack(monster)
	} else {
		cs.recordMonsterActorAttack(monster)
	}
	if spellDef, err := spells.GetSpellDefinitionByID(spellID); err == nil {
		cs.game.playMonsterSpellSound(spellDef, monster)
	}
}

func (cs *CombatSystem) spawnMonsterWeaponProjectile(monster *monsterPkg.Monster3D, weaponKey string, targetX, targetY float64, owner ProjectileOwner) {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists || weaponDef == nil || weaponDef.Physics == nil {
		fmt.Printf("[WARN] projectile weapon '%s' is missing physics in weapons.yaml\n", weaponKey)
		return
	}
	if owner == ProjectileOwnerMonster {
		cs.recordMonsterPartyAttack(monster)
	} else {
		cs.recordMonsterActorAttack(monster)
	}

	tileSize := cs.game.config.GetTileSize()
	arrowSpeed := weaponDef.Physics.GetSpeedPixels(tileSize)
	arrowLifetime := weaponDef.Physics.GetLifetimeFrames()
	collisionSize := cs.game.config.ProjectileHitboxTiles(weaponDef.Physics) * tileSize

	damageType := monsterPkg.DamagePhysical.String()
	if weaponDef.DamageType != "" {
		damageType = normalizeDamageTypeStr(weaponDef.DamageType)
	}

	angle := math.Atan2(targetY-monster.Y, targetX-monster.X)
	dirX, dirY := math.Cos(angle), math.Sin(angle)

	// Volley: same rule as the party's bows - the weapon looses several darts
	// per shot, trailed back along the aim line so they read as a quick stream.
	// Each dart rolls its own damage (champions crit per dart).
	volley := 1
	if weaponDef.Volley > 1 {
		volley = weaponDef.Volley
	}
	spacing := volleySpacingFrac * float64(tileSize)
	for i := 0; i < volley; i++ {
		back := spacing * float64(i)
		damage, crit := cs.monsterAttackDamage(monster)
		parts, bossCrit := cs.monsterOutgoingDamage(monster, weaponTrueShare(weaponDef, damagecalc.Parts{Normal: damage, True: monster.TrueDamage}))
		arrow := Arrow{
			ID:                 cs.game.GenerateProjectileID("monster_arrow"),
			SuppressAoE:        i > 0, // an AoE-rider weapon engulfs the party once per VOLLEY, not per dart
			X:                  monster.X - dirX*back,
			Y:                  monster.Y - dirY*back,
			VelX:               dirX * arrowSpeed,
			VelY:               dirY * arrowSpeed,
			Damage:             parts.Normal,
			TrueDamage:         parts.True,
			GrazesDodge:        monster.GrazesDodge,
			LifeTime:           volleyDartLifetime(arrowLifetime, back, arrowSpeed),
			Active:             true,
			BowKey:             weaponKey,
			DamageType:         damageType,
			Crit:               crit || bossCrit,
			DisintegrateChance: weaponDef.DisintegrateChance,
			Owner:              owner,
			SourceName:         monster.Name,
			SourceMonster:      monster,
		}
		cs.game.arrows = append(cs.game.arrows, arrow)
		arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(arrowEntity)
	}
	cs.game.playMonsterRangedWeaponAttackSound(weaponDef, monster)
}

// volleyDartLifetime lets a dart trailed back behind the lead still end on
// the lead's range plane, so every dart of a volley reaches the authored range.
func volleyDartLifetime(lifetime int, back, speed float64) int {
	if speed <= 0 {
		return lifetime
	}
	return lifetime + int(math.Round(back/speed))
}

func (cs *CombatSystem) weaponBonusMultiplier(weaponDef *config.WeaponDefinitionConfig, monster *monsterPkg.Monster3D) float64 {
	if weaponDef == nil || monster == nil || len(weaponDef.BonusVs) == 0 {
		return 1.0
	}

	for bonusKey, mult := range weaponDef.BonusVs {
		if mult > 0 && monsterMatchesBonusTarget(monster, bonusKey) {
			return mult
		}
	}

	return 1.0
}

func (cs *CombatSystem) tryApplyWeaponStun(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) {
	if monster == nil {
		return
	}
	if weaponDef != nil && weaponDef.StunChance > 0 && rand.Float64() < weaponDef.StunChance {
		cs.applyStunDR(monster, config.CombatRounds(weaponDef.StunSeconds), true)
		return
	}
	// Minotaur Card: chance on any hit to stun the target for one round (one
	// stun-roll per hit).
	if pct := cs.game.cardStunOnHitPct(); pct > 0 && rand.Intn(100) < pct {
		cs.applyStunDR(monster, 1, true)
	}
}

// tryApplyWeaponHitRiders applies every on-hit weapon and card rider to a
// surviving target. This is the single rider entry point for melee and
// projectile weapon hits.
func (cs *CombatSystem) tryApplyWeaponHitRiders(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) {
	cs.tryApplyWeaponStun(monster, weaponDef)
	if monster == nil || weaponDef == nil {
		cs.tryCardPoisonProc(monster)
		return
	}
	tps := cs.game.config.GetTPS()
	if tps <= 0 {
		tps = config.GetTargetTPS()
	}
	if weaponDef.ArmorShredPct > 0 && weaponDef.ArmorShredSeconds > 0 {
		frames, rounds := config.EffectClock(weaponDef.ArmorShredSeconds, tps)
		monster.ApplyArmorShred(weaponDef.ArmorShredPct, frames, rounds)
	}
	if weaponDef.RootChance > 0 && weaponDef.RootSeconds > 0 && rand.Float64() < weaponDef.RootChance {
		cs.applyMonsterRoot(monster, config.CombatRounds(weaponDef.RootSeconds))
	}
	// Drakeforged riders: DoTs bill elapsed seconds in both modes.
	if weaponDef.IgniteChance > 0 && weaponDef.IgniteSeconds > 0 && rand.Float64() < weaponDef.IgniteChance {
		cs.afflictMonster(monster, monsterBurn, weaponDef.IgniteSeconds*tps, logToneGood, false)
	}
	if weaponDef.PoisonChance > 0 && weaponDef.PoisonSeconds > 0 && rand.Float64() < weaponDef.PoisonChance {
		cs.afflictMonster(monster, monsterPoison, weaponDef.PoisonSeconds*tps, logToneGood, false)
	}
	if weaponDef.SlowPct > 0 && weaponDef.SlowSeconds > 0 {
		frames, rounds := config.EffectClock(weaponDef.SlowSeconds, tps)
		monster.ApplySlow(weaponDef.SlowPct, frames, rounds)
	}
	if weaponDef.WeakenPct > 0 && weaponDef.WeakenSeconds > 0 {
		frames, rounds := config.EffectClock(weaponDef.WeakenSeconds, tps)
		monster.ApplyWeaken(weaponDef.WeakenPct, frames, rounds)
	}
	// Clock Tower riders: the hour mark (its multiplier read the count
	// before this hit) and the jam.
	if weaponDef.HourStrikeMarks > 0 {
		if monster.HourMarks >= weaponDef.HourStrikeMarks-1 {
			monster.HourMarks = 0
			cs.game.logCombatLine(logToneGood, uitext.Text("combat.strikes_the_hour_on", weaponDef.Name, logSlot), logMonsterName(monster))
		} else {
			monster.HourMarks++
		}
	}
	if weaponDef.JamSeconds > 0 {
		cs.jamMonster(monster, weaponDef.JamSeconds)
	}
	cs.tryCardPoisonProc(monster)
}

// jamMonster delays the struck monster's next attack (Cogfang Blade), once
// per attack it makes: real time adds the seconds to its attack cooldown;
// turn-based banks them as a share of a turn, and a full turn costs its next
// attack (commitMonsterAttack).
func (cs *CombatSystem) jamMonster(m *monsterPkg.Monster3D, seconds int) {
	if m.Jammed || !m.IsAlive() {
		return
	}
	m.Jammed = true
	if cs.game.turnBasedMode {
		m.LoseTurnBasedAttackTime(seconds)
		return
	}
	m.AttackCDFrames += seconds * cs.game.config.GetTPS()
}

// weaponVictimMultiplier is a weapon's multiplier for the victim's state:
// stunned (Gladius), still fresh (Mainspring Pike), or, on the landed primary
// hit whose rider counts the marks, the hour struck (Minute Hand).
func weaponVictimMultiplier(def *config.WeaponDefinitionConfig, m *monsterPkg.Monster3D, primaryHit bool) float64 {
	mult := weaponStunnedBonusMultiplier(def, m)
	if def == nil || m == nil {
		return mult
	}
	if def.FreshTargetMinHPPct > 0 && m.MaxHitPoints > 0 && m.HitPoints*100 >= m.MaxHitPoints*def.FreshTargetMinHPPct {
		mult *= def.FreshTargetMultiplier
	}
	if primaryHit && def.HourStrikeMarks > 0 && m.HourMarks >= def.HourStrikeMarks-1 {
		mult *= def.HourStrikeMultiplier
	}
	return mult
}

// weaponStunnedBonusMultiplier returns the Gladius-style damage multiplier
// against a currently-stunned target (1.0 when not applicable).
func weaponStunnedBonusMultiplier(weaponDef *config.WeaponDefinitionConfig, monster *monsterPkg.Monster3D) float64 {
	if weaponDef == nil || weaponDef.BonusVsStunned <= 0 || weaponDef.BonusVsStunned == 1.0 {
		return 1.0
	}
	if monster.Stun.Active() {
		return weaponDef.BonusVsStunned
	}
	return 1.0
}

// tryCardPoisonProc rolls the Venom-proc cards' (rat/spider/forest_spider)
// on-hit poison chance against a struck monster.
// Undead are immune, matching the genre convention (and this game's own
// mind/body/light resist baseline for the type).
func (cs *CombatSystem) tryCardPoisonProc(monster *monsterPkg.Monster3D) {
	if monster == nil {
		return
	}
	chancePct, durationSec := cs.game.cardPoisonProc()
	if chancePct <= 0 || rand.Intn(100) >= chancePct {
		return
	}
	cs.afflictMonster(monster, monsterPoison, cs.game.config.GetTPS()*durationSec, logToneGood, false)
}

// markMonsterHit applies the side effects every hit shares regardless of source
// (melee, projectile, splash, nova, trap, steam): the damage flash, freeing a
// Charmed monster, and the explicit turn-based same-kind pack response.
func (cs *CombatSystem) markMonsterHit(m *monsterPkg.Monster3D) {
	m.HitTintFrames = MonsterHitFlashFrames
	cs.breakPacifyOnHit(m)
	m.RetaliateAgainstParty(cs.game.camera.X, cs.game.camera.Y)
	cs.game.rallyAuthoredBandHit(m)
	cs.engageTurnBasedSameKindPackOnPartyHit(m)
}

// finishMonsterKill records a slain monster for the end-of-frame removal sweep
// (removeDeadMonstersByID, which also unregisters its collision entity) and
// awards the kill's XP/gold. Returns the XP awarded, for the kill message.
func (cs *CombatSystem) finishMonsterKill(m *monsterPkg.Monster3D) int {
	if m == nil {
		return 0
	}
	for _, id := range cs.game.deadMonsterIDs {
		if id == m.ID {
			return 0
		}
	}
	cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, m.ID)
	m.DeathDrawX, m.DeathDrawY = cs.monsterVisualPos(m)
	m.DeathDrawn = true
	cs.game.recordAdventureBoss(m)
	cs.game.rewardOwner().noteStoryKill(m)
	if m.IsCaravan() {
		cs.game.recordCaravanLoss(m.ID)
	}
	cs.game.beginMonsterDeath(m)
	if !isPurePartySummon(m) {
		cs.game.playMonsterSound(soundEnemyDeath, m)
	}
	cs.scatterBandOnMemberDeath(m)
	cs.announceKill(m, cs.killExperience(m))
	if m.IsChampion() {
		cs.recordChampionVictory(m)
	}
	xp := cs.awardExperienceAndGold(m)
	cs.triggerWildfire(m)
	return xp
}

// announceKill is THE death line of the log: every kill the party is told
// about, with the experience it gives. Strike lines report only their damage.
func (cs *CombatSystem) announceKill(m *monsterPkg.Monster3D, xp int) {
	if quietDeath(m) {
		return
	}
	tone := logToneGood
	if monsterIsPartyAlly(m) {
		tone = logToneBad
	}
	owner := cs.game.rewardOwner()
	cs.game.logCombatLine(tone, uitext.Text("combat.is_slain", logSlot, logSlot), logMonsterName(m), owner.logMonsterKillXP(m, xp))
}

// killExperience is what a kill is worth to the party: nothing for its own
// summons, caravans, wildlife prey or a boss's unbound adds. A boss's adds
// still drop their gold and loot like any kill: intended, not a farming bug.
func (cs *CombatSystem) killExperience(monster *monsterPkg.Monster3D) int {
	if !killCredited(monster) {
		return 0
	}
	if monster.SummonedBy != "" && !monster.CharmedByParty {
		return 0
	}
	return monster.Experience
}

// finishMonsterKillImmediately finalizes a death that must stop colliding in
// the current frame (traps, crossfire projectiles, direct monster-vs-monster
// hits). Rewards still flow through finishMonsterKill, including champions.
func (cs *CombatSystem) finishMonsterKillImmediately(m *monsterPkg.Monster3D) {
	if m == nil {
		return
	}
	if cs.game.collisionSystem != nil {
		cs.game.collisionSystem.UnregisterEntity(m.ID)
	}
	cs.finishMonsterKill(m)
}

// scatterBandOnMemberDeath bursts the victim's band the moment a member is
// slain. The hit-propagation path (TakeDamage -> non-calm member -> next-tick
// scatter) never fires on a one-shot kill: the dead member drops out of the
// band collection, so the survivors would stay calm and stacked - a band could
// be sniped down one by one without ever aggroing.
func (cs *CombatSystem) scatterBandOnMemberDeath(victim *monsterPkg.Monster3D) {
	if victim != nil && victim.LootGuarding {
		if cs.game.gameLoop != nil {
			members := cs.game.gameLoop.lootGuardBandMembers(victim)
			// A death is the same hostile event as a direct hit: surviving guards
			// scatter and join a leashed fight; they can resume guard duty on return.
			cs.game.gameLoop.scatterLootGuardBand(members, true)
		}
		return
	}
	if victim == nil || !victim.Banding || victim.BandID <= 0 ||
		cs.game.gameLoop == nil || cs.game.world == nil || cs.game.collisionSystem == nil {
		return
	}
	var calm, survivors []*monsterPkg.Monster3D
	for _, m := range cs.game.world.Monsters {
		if m == nil || m == victim || !m.IsAlive() || m.BandID != victim.BandID {
			continue
		}
		survivors = append(survivors, m)
		if isCalmBander(m) {
			calm = append(calm, m)
		}
	}
	if len(calm) == 0 {
		return // nobody left to wake - already fighting or band is gone
	}
	cs.game.gameLoop.scatterBand(calm, survivors, float64(cs.game.config.GetTileSize()), true)
}

// awardExperienceAndGold gives experience and gold to the party when a monster is killed.
// Boss summons keep their regular drops/gold/quest behavior, but grant no XP unless
// the party previously charmed them.
func (cs *CombatSystem) awardExperienceAndGold(monster *monsterPkg.Monster3D) int {
	recipient := cs.game.rewardOwner()
	if monster == nil || recipient.party == nil || len(recipient.party.Members) == 0 {
		return 0
	}
	// THE single gate (killCredited), so every death path (melee, projectile,
	// splash) honours it automatically.
	if !killCredited(monster) {
		return 0
	}

	recipient.recordProfileKill(monster)
	xpAwarded := cs.killExperience(monster)
	reportedXP := recipient.adventureKillExperience(monster, xpAwarded).total

	// Each living hero - active, reserve, or captive - gets the per-member share.
	if xpAwarded > 0 {
		recipient.grantMonsterXP(monster, recipient.xpShare(xpAwarded))
	}

	// Check for loot drops
	drops := cs.checkMonsterLootDrop(monster)
	if monster.IsChampion() {
		for _, it := range drops {
			cs.game.AddColoredCombatMessage(uitext.Text("combat.s_drops_on_the_sand", monster.Name, it.Name), combatMessageGold)
		}
	}

	// Update quest progress
	cs.updateQuestProgress(monster)

	// Revenge: a slain patron (DeathRalliesType) sends every live map monster of
	// that type into a relentless map-wide hunt.
	cs.rallyOnPatronDeath(monster)

	// Drop gold/items into a loot bag on the ground (fixed size from config, not
	// scaled by the monster).
	if monster.Gold > 0 || len(drops) > 0 {
		gold := monster.Gold
		if pct := recipient.cardGoldFindPct(); pct != 0 && gold > 0 {
			gold = gold * (100 + pct) / 100 // Jungle Goblin Card
		}
		cs.game.addMonsterLootDrop(monster, drops, gold)
	}

	return reportedXP
}

// rallyOnPatronDeath: when a monster carrying DeathRalliesType dies, every other
// LIVE monster on the map whose Type matches flies into a relentless map-wide
// hunt for the party (the Relentless flag drives pursueRelentlessly, ignoring
// detection range - and it persists across reload). The orc Warlord's death
// turns the masked Amazons (type "human") vengeful; goblins/beasts are untouched.
func (cs *CombatSystem) rallyOnPatronDeath(dead *monsterPkg.Monster3D) {
	if dead == nil || !dead.IsBoss() || dead.DeathRalliesType == "" || cs.game == nil || cs.game.world == nil {
		return
	}
	rallied := 0
	for _, m := range cs.game.world.Monsters {
		if m == nil || m == dead || !m.IsAlive() || m.Relentless || m.MonsterType != dead.DeathRalliesType || !cs.game.monsterIsFrom(cs.game.world, m, cs.game.monsterHomeMap(cs.game.world, dead)) {
			continue
		}
		m.Relentless = true
		m.WasAttacked = true // Saved provocation memory; Relentless grants home-region pursuit.
		m.BeginPlayerEngagement()
		rallied++
	}
	if rallied > 0 {
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.falls_its_retainers_turn_on_you", logSlot), logMonsterName(dead))
	}
}

// killCredited: the party is credited with this death at all - XP, gold, loot.
// A pure party summon was never an enemy; caravans and wildlife prey
// (NoKillRewards) pay nothing.
func killCredited(m *monsterPkg.Monster3D) bool {
	return m != nil && !isPurePartySummon(m) && !m.NoKillRewards && !m.IsCaravan()
}

// killCountsForProgress: a credited death that also advances quests, the story
// and the captain. Runtime summons, ecology actors and fish (marked
// QuestProgressIgnored by their spawners) never do.
func killCountsForProgress(m *monsterPkg.Monster3D) bool {
	return killCredited(m) && !m.QuestProgressIgnored
}

// updateQuestProgress updates quest progress when a monster is killed
func (cs *CombatSystem) updateQuestProgress(monster *monsterPkg.Monster3D) {
	recipient := cs.game.rewardOwner()
	if recipient.questManager == nil {
		return
	}
	if !killCountsForProgress(monster) {
		return
	}

	monsterType := questMonsterTag(monster)
	sourceQuestID := ""
	if monster.IsEncounterMonster && monster.EncounterRewards != nil {
		sourceQuestID = monster.EncounterRewards.QuestID
	}

	completedQuests := recipient.questManager.OnMonsterKilledFromSource(
		monsterType,
		cs.game.questKillMapKey(monster),
		sourceQuestID,
	)

	// Notify player of quest completions. Auto-claimed objective quests do not
	// advertise a journal reward action.
	for _, quest := range completedQuests {
		recipient.announceQuestCompletion(quest)
	}

	// Kill quests also complete when their eligible roster is cleared
	// (counter notwithstanding).
	recipient.completeClearedKillQuestsForTarget(monsterType, true)
}

// checkLevelUp checks if a character should level up and applies level up benefits.
// announce gates the combat-log message: only ACTIVE party members announce, so a
// benched reserve/captive hero leveling "alongside the party" doesn't spam the log
// with "reached level N" for heroes the player can't see (their stat points and
// owed class choices still bank for when they're swapped in).
func (cs *CombatSystem) checkLevelUp(character *character.MMCharacter, announce bool) {
	// Level progression: xpStepCost(currentLevel) experience per level - linear
	// early, quadratic from L13 so high-level farming doesn't run away. Loop
	// handles multiple level-ups from a single XP gain.
	for {
		requiredExp := xpStepCost(character.Level)

		if character.Experience >= requiredExp {
			oldLevel := character.Level
			character.Level++
			character.Experience -= requiredExp // Subtract used experience

			character.FreeStatPoints += StatPointsPerLevel

			// Levels increase capacity, but do not replenish combat resources.
			character.RecalculateMaxStatsKeepingCurrent(cs.game.config)

			if announce {
				cs.game.playSound(soundLevelUp)
				cs.game.logCombatLine(logToneReward, uitext.Text("combat.reached_was_level_stat_points", logSlot, logSlot, oldLevel, StatPointsPerLevel), logHeroName(character), logColored(fmt.Sprintf("level %d", character.Level), combatMessageGold))
			}

			// Offer a class-progression choice every LevelUpChoiceInterval levels
			// (3, 6, 9, 12, ...), or whenever level_up.yaml explicitly defines one
			// for this level (so YAML entries off the interval still fire). The
			// choice is padded to MinLevelUpOptions with random upgrades of skills
			// the character already owns.
			explicit := config.GetLevelUpChoices(character.GetClassKey(), character.Level)
			if character.Level%LevelUpChoiceInterval == 0 || len(explicit) > 0 {
				cs.game.queueLevelUpChoices(character, character.Level, explicit)
			}
		} else {
			break // No more level-ups possible
		}
	}
}

// CalculateWeaponDamage calculates total weapon damage using weapon-specific bonus stat(s)
func (cs *CombatSystem) CalculateWeaponDamage(weapon items.Item, char *character.MMCharacter) (int, int, int) {
	result := character.WeaponDamageAtNight(lookupWeaponConfig(weapon), char, cs != nil && cs.game != nil && cs.game.dayNightIsNight)
	return result.Base + result.ArmsMaster + result.OrcishFury + result.FlowingStaff, result.StatBonus, result.Total
}

// activeAttacker returns the currently selected party member (the attacker for
// melee/ranged hits resolved this frame), or nil if unavailable.
func (cs *CombatSystem) activeAttacker() *character.MMCharacter {
	if cs.game == nil || cs.game.party == nil {
		return nil
	}
	if cs.game.selectedChar < 0 || cs.game.selectedChar >= len(cs.game.party.Members) {
		return nil
	}
	return cs.game.party.Members[cs.game.selectedChar]
}

// weaponMasteryStrike returns the mastery TRUE-damage bonus and dodge-graze
// flag for the given attacker wielding the given weapon. True damage keeps the
// weapon's school and therefore meets resistance, but bypasses armor/flat
// reduction and lands through a Perfect Dodge; a Grandmaster (tier 3) strike
// also grazes through it for WeaponGMDodgeGrazePct of its normal damage
// (throughDodge). A weapon's own true share rides each hit (weaponTrueShare).
func (cs *CombatSystem) weaponMasteryStrike(attacker *character.MMCharacter, weaponDef *config.WeaponDefinitionConfig) (trueDmg int, grazesDodge bool) {
	if weaponDef == nil || attacker == nil {
		return 0, false
	}
	skillType, ok := character.WeaponSkillForCategory(strings.ToLower(weaponDef.Category))
	if !ok {
		return 0, false
	}
	tier := attacker.SkillTier(skillType)
	return tier * MasteryWeaponTrueDamagePerTier, tier >= int(character.MasteryGrandMaster)
}

// tryWeaponExecute closes the Wyrmcleaver's jaws: a SURVIVING target left at
// or under execute_below_pct of its max HP dies outright, bosses included. Returns the XP-awarded kill flag so callers skip their own
// alive-branch messaging.
func (cs *CombatSystem) tryWeaponExecute(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, attacker *character.MMCharacter, attackerName string) bool {
	if monster == nil || weaponDef == nil || weaponDef.ExecuteBelowPct <= 0 {
		return false
	}
	if !monster.IsAlive() || monster.IsDamageInvulnerable() || monster.MaxHitPoints <= 0 {
		return false
	}
	if monster.HitPoints*100 > monster.MaxHitPoints*weaponDef.ExecuteBelowPct {
		return false
	}
	monster.HitPoints = 0
	cs.markMonsterHit(monster)
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.instakill.maw", logSlot, logSlot), logHeroText(attackerName), logMonsterName(monster))
	cs.finishWeaponKill(monster, weaponDef, attacker)
	return true
}

// finishWeaponKill is the single finalization path for a monster killed by a
// party weapon. Kill riders run before generic reward/removal bookkeeping;
// callers pass nil for non-weapon splash packets, preventing recursive bursts.
func (cs *CombatSystem) finishWeaponKill(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, attacker *character.MMCharacter) int {
	cs.tryWeaponDeathBurst(monster, weaponDef, attacker)
	return cs.finishMonsterKill(monster)
}

// tryWeaponDeathBurst pops the Ember Egg: a target KILLED by this weapon
// bursts, dealing a share of its own max HP as fire damage to every other
// monster in the radius (its own attack packet - armor/resists resolve per
// victim; kills credit).
func (cs *CombatSystem) tryWeaponDeathBurst(corpse *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, attacker *character.MMCharacter) {
	if corpse == nil || weaponDef == nil || weaponDef.DeathBurstMaxHPPct <= 0 || weaponDef.DeathBurstRadiusTiles <= 0 {
		return
	}
	if corpse.IsAlive() {
		return
	}
	burst := cs.newPartyMonsterAttack(
		max(1, corpse.MaxHitPoints*weaponDef.DeathBurstMaxHPPct/100),
		0,
		monsterPkg.DamageFire.String(),
		0,
		nil,
		"Clutchburst",
		false,
		false,
		false,
	)
	burst.Attacker = attacker
	cs.spawnMonsterHitBurst(corpse, monsterPkg.DamageFire.String())
	cs.applyAoeSplash(corpse, burst, weaponDef.DeathBurstRadiusTiles)
}

// CriticalChanceBreakdown returns the universal crit components shared by
// weapon and spell attacks. Weapon-specific mastery bonuses stay in
// WeaponCritBreakdown.
func (cs *CombatSystem) CriticalChanceBreakdown(char *character.MMCharacter) (luck, cards, setBonus int) {
	if char != nil {
		luck = char.GetEffectiveLuck() / LuckToCritDivisor
		setBonus = char.SetCritChanceBonus()
	}
	if cs != nil && cs.game != nil && cs.game.isPartyMember(char) {
		cards = cs.game.cardCritBonusPct()
	}
	return luck, cards, setBonus
}

// CalculateCriticalChance calculates the universal critical hit bonus used by
// spells and as one component of weapon critical chance.
func (cs *CombatSystem) CalculateCriticalChance(char *character.MMCharacter) int {
	luck, cards, setBonus := cs.CriticalChanceBreakdown(char)
	return luck + cards + setBonus
}

// totalCriticalChance is the one clamped total used for a real critical roll
// and a spell tooltip. Keeping the clamp here prevents the display and roll
// from diverging when several universal crit bonuses exceed 100%.
func (cs *CombatSystem) totalCriticalChance(baseCrit int, char *character.MMCharacter) int {
	total := baseCrit + cs.CalculateCriticalChance(char)
	if total < 0 {
		return 0
	}
	if total > 100 {
		return 100
	}
	return total
}

// RollCriticalChance returns whether an attack critically hits and the total crit chance used.
// totalCrit = baseCrit + Luck-derived bonus, clamped to [0,100].
func (cs *CombatSystem) RollCriticalChance(baseCrit int, chr *character.MMCharacter) (bool, int) {
	total := cs.totalCriticalChance(baseCrit, chr)
	roll := rand.Intn(100)
	return roll < total, total
}

// RollWeaponCriticalChance rolls a weapon crit using the same total chance shown in tooltips.
func (cs *CombatSystem) RollWeaponCriticalChance(weapon items.Item, chr *character.MMCharacter) (bool, int) {
	total := cs.CalculateWeaponCritChance(weapon, chr)
	roll := rand.Intn(100)
	return roll < total, total
}

// monsterImmuneToDisintegrate reports whether a monster ignores every
// disintegrate effect (spell or weapon proc): undead and dragons by their
// `type`, and an invulnerable boss. Other bosses are not immune but never
// erased - rollMonsterDisintegrate turns their proc into a sear.
func monsterImmuneToDisintegrate(m *monsterPkg.Monster3D) bool {
	if m == nil {
		return false
	}
	// An invulnerable boss (sealed or idol-warded) takes neither kill nor sear.
	return m.MonsterType == monsterPkg.TypeUndead || m.MonsterType == monsterPkg.TypeDragon || m.IsDamageInvulnerable()
}

// absorbIfSealed reports whether the monster is an invulnerable boss and, if so,
// plays the muted "blow absorbed" beat (impact spark + one message). Player damage
// hubs call this and return early; the monster packet sink remains the backstop
// for paths that don't pre-check (AoE splash, mastery, monster-vs-monster).
func (cs *CombatSystem) absorbIfSealed(m *monsterPkg.Monster3D) bool {
	if m == nil {
		return false
	}
	switch {
	case m.BossDormant:
		cs.spawnHitSparks(m)
		cs.game.logCombatLine(logToneNone, uitext.Text("combat.the_seal_holds_is_impervious", logSlot), logMonsterName(m))
		return true
	case m.BossWarded:
		cs.spawnHitSparks(m)
		cs.game.logCombatLine(logToneNone, uitext.Text("combat.the_idols_ward_holds_is_impervious", logSlot), logMonsterName(m))
		return true
	}
	return false
}

// applyStunDR is the single entry point for stunning a monster, for whole
// rounds. It applies DIMINISHING RETURNS: the requested rounds are scaled by the
// stun row's repeat_percents for the target's current DR chain length
// (100/50/25/0%), so repeated stuns shrink to nothing and then the target is
// immune until it goes control-free for the reset window. Refreshes the chain +
// both per-mode reset clocks on every attempt (a TB<->RT switch is
// conservative). announce=false suppresses the per-target line for AoE callers
// that print their own summary. Returns whether it actually stunned.
func (cs *CombatSystem) applyStunDR(m *monsterPkg.Monster3D, rounds int, announce bool) bool {
	if m == nil {
		return false
	}
	wasStunned := m.Stun.Active()
	effTurns, effFrames := m.StunDR().Step(rounds, cs.game.config.GetTPS())

	if effTurns <= 0 { // worn down -> immune this attempt
		if announce && !wasStunned {
			cs.game.logCombatLine(logToneNone, uitext.Text("combat.resists_the", logSlot, logSlot), logMonsterName(m), logKeyword("control", "stun"))
		}
		return false
	}
	m.Stun.Refresh(effFrames, effTurns)
	if announce && !wasStunned {
		cs.game.logCombatLine(monsterStatusTone(m), uitext.Text("combat.is", logSlot, logSlot, logSlot), logMonsterName(m), logKeyword("control", "stunned"), cs.game.logStatusLength(m.Stun.Turns, m.Stun.Frames))
	}
	return true
}

// applyStun stuns a single monster for an authored seconds duration (whole
// rounds), under diminishing returns (see applyStunDR).
func (cs *CombatSystem) applyStun(m *monsterPkg.Monster3D, seconds int, announce bool) {
	cs.applyStunDR(m, config.CombatRounds(seconds), announce)
}

// applyMonsterRoot is the one entry point for roots from traps, weapon riders
// and Frozen Ground, for whole rounds. Root has parallel TB-turn and RT-frame
// clocks so a Tab mode switch cannot release an otherwise active pin; each mode
// clears the counterpart when its own clock expires.
func (cs *CombatSystem) applyMonsterRoot(m *monsterPkg.Monster3D, rounds int) {
	if cs == nil || cs.game == nil || m == nil || rounds <= 0 {
		return
	}
	m.Root.Refresh(config.RoundFrames(rounds, cs.game.config.GetTPS()), rounds)
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.is_in_place", logSlot, logSlot, logSlot), logMonsterName(m), logKeyword("control", "pinned"), cs.game.logStatusLength(m.Root.Turns, m.Root.Frames))
}

// applyBindUndead (Bind Undead) takes control of an UNDEAD target - it hunts
// other monsters for you and ignores the party. No effect on the living. No
// damage is dealt. A separate, mutually exclusive effect from Pacify (Charm).
func (cs *CombatSystem) applyBindUndead(m *monsterPkg.Monster3D, seconds int, spellName string) {
	if m.MonsterType != monsterPkg.TypeUndead {
		cs.game.logCombatLine(logToneNone, uitext.Text("combat.washes_over_only_the_undead_can", logSlot, logSlot), logKeyword("control", spellName), logMonsterName(m))
		return
	}
	m.Bound = true
	m.BoundClock = status.DualClock{}
	if seconds > 0 { // 0 binds for good
		m.BoundClock.Arm(seconds, cs.game.config.GetTPS())
	}
	// Charm and Bind are mutually exclusive. This should only repair malformed
	// old/runtime state (Charm normally rejects undead), but keeps control policy
	// deterministic at every entry point.
	m.Pacified = false
	m.PacifiedClock = status.DualClock{}
	m.WasAttacked = false
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.is_to_your_will", logSlot, logSlot), logMonsterName(m), logKeyword("control", "bound"))
}

const darkElfBindingChancePct = character.DarkElfBindingChancePct

func darkElfBindingEligible(attacker *character.MMCharacter, target *monsterPkg.Monster3D) bool {
	if attacker == nil || !attacker.HasSkill(character.SkillDarkElfBinding) || target == nil || !target.IsAlive() ||
		target.Bound || target.IsBoss() || target.IsDamageInvulnerable() || isPurePartySummon(target) {
		return false
	}
	return target.MonsterType != monsterPkg.TypeUndead && target.MonsterType != monsterPkg.TypeFormless
}

// tryDarkElfBindInstead is the one racial proc boundary for a party weapon or
// spell hit; traps and lingering fields never bind. A successful proc
// replaces the entire hit and all of its on-hit riders; the newly bound
// former enemy remains a normal reward-bearing map creature.
func (cs *CombatSystem) tryDarkElfBindInstead(attacker *character.MMCharacter, target *monsterPkg.Monster3D) bool {
	if !darkElfBindingEligible(attacker, target) || !cs.rollRacialProc(darkElfBindingChancePct) {
		return false
	}
	target.Bound = true
	target.BoundClock = status.DualClock{}
	target.Pacified = false
	target.PacifiedClock = status.DualClock{}
	target.WasAttacked = false
	target.AIFoe = nil
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.s_dark_binding_claims_instead_of", logSlot, logSlot), logHeroName(attacker), logMonsterName(target))
	return true
}

// applyPacify (Charm) pacifies a LIVING target - it stops attacking and breaks
// free on any hit it takes (see breakPacifyOnHit). No effect on undead or
// formless targets, no damage. A separate,
// mutually exclusive effect from Bind Undead.
func (cs *CombatSystem) applyPacify(m *monsterPkg.Monster3D, seconds int, spellName string) {
	if m.MonsterType == monsterPkg.TypeUndead || m.MonsterType == monsterPkg.TypeFormless {
		cs.game.logCombatLine(logToneNone, uitext.Text("combat.has_no_hold_over_the", logSlot, m.MonsterType, logSlot), logKeyword("control", spellName), logMonsterName(m))
		return
	}
	if m.Bound {
		cs.game.logCombatLine(logToneNone, uitext.Text("combat.is_already_bound_to_your_will", logSlot), logMonsterName(m))
		return
	}
	m.Pacified = true
	m.PacifiedClock = status.DualClock{}
	if seconds > 0 { // 0 charms for good
		m.PacifiedClock.Arm(seconds, cs.game.config.GetTPS())
	}
	m.CharmedByParty = true
	m.WasAttacked = false
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.is_and_stops_attacking", logSlot, logSlot), logMonsterName(m), logKeyword("control", "charmed"))
}

// breakPacifyOnHit releases a pacified (Charm) monster the instant it takes any
// hit - it snaps out of the charm and re-aggros. Bound undead are unaffected.
// Called wherever the party deals damage to a monster.
func (cs *CombatSystem) breakPacifyOnHit(m *monsterPkg.Monster3D) {
	if m.Pacified {
		m.Pacified = false
		m.PacifiedClock = status.DualClock{}
		m.WasAttacked = true
		m.BeginPlayerEngagement()
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.breaks_free_of_the", logSlot, logSlot), logMonsterName(m), logKeyword("control", "charm"))
	}
}

// boundAllySeekRadius is the pixel range a bound undead hunts for enemies to
// walk toward (see BoundAllySeekTiles).
func (cs *CombatSystem) boundAllySeekRadius() float64 {
	return BoundAllySeekTiles * float64(cs.game.config.GetTileSize())
}

// monsterCanAttackMonster is the crossfire strike reach (monsterCanStrikePoint),
// except that a foe on the attacker's own tile is out of reach.
// A melee attacker may hit any adjacent tile even when off-centre tokens are
// farther than a radial 1.5 tiles apart. The movement AI already treats that
// tile adjacency as attack contact; a radial-only gate here made melee champions
// walk up to summons and then never swing.
func (cs *CombatSystem) monsterCanAttackMonster(attacker, target *monsterPkg.Monster3D) bool {
	if attacker == nil || target == nil || !target.IsAlive() {
		return false
	}
	if cs != nil && cs.game != nil && cs.game.config != nil {
		tileSize := float64(cs.game.config.GetTileSize())
		if tileSize > 0 && TileIndex(attacker.X, tileSize) == TileIndex(target.X, tileSize) && TileIndex(attacker.Y, tileSize) == TileIndex(target.Y, tileSize) {
			return false
		}
	}
	return cs.monsterCanStrikePoint(attacker, target.X, target.Y)
}

// boundAllyCanDamageMonster is the shared faction/encounter gate for a party
// summon's target selection, direct projectile collision, and AoE collateral.
// Keeping all three on one policy prevents a bolt or splash from silently
// provoking a passive creature that the summon's AI deliberately ignored.
// Fish are party-only catches, including when an allied shot passes through.
func (cs *CombatSystem) boundAllyCanDamageMonster(candidate *monsterPkg.Monster3D) bool {
	return cs != nil && candidate != nil && candidate.IsAlive() &&
		!candidate.IsPartyControlled() && !candidate.IsFish() && !candidate.IsCaravan() &&
		!candidate.IsDamageInvulnerable() &&
		!candidate.IsPassiveUntilProvoked() &&
		!cs.bossEvasive(candidate)
}

// nearestSeenMonster is the closest of candidates to (x, y), nearer than
// best, that accept takes and that sight reaches: the attack line from
// (x, y), or fighter's current foe, kept through cover until that fight ends
// (fighter nil for none). The one rule behind every "nearest enemy" pick -
// bound allies, mobs turning on them, berserkers, ricochets.
func (cs *CombatSystem) nearestSeenMonster(x, y float64, fighter *monsterPkg.Monster3D, candidates []*monsterPkg.Monster3D, best float64, accept func(*monsterPkg.Monster3D) bool) *monsterPkg.Monster3D {
	var nearest *monsterPkg.Monster3D
	for _, other := range candidates {
		if other == nil || other == fighter || !other.IsAlive() || !accept(other) {
			continue
		}
		if (fighter == nil || fighter.AIFoe != other) && !cs.attackLineClear(x, y, other.X, other.Y) {
			continue
		}
		if d := Distance(x, y, other.X, other.Y); d < best {
			best, nearest = d, other
		}
	}
	return nearest
}

// nearestEnemyMonster returns the closest monster a bound ally may damage
// within maxDist (pixels) and can see, or nil.
func (cs *CombatSystem) nearestEnemyMonster(m *monsterPkg.Monster3D, maxDist float64) *monsterPkg.Monster3D {
	return cs.nearestSeenMonster(m.X, m.Y, m, cs.game.world.Monsters, maxDist, cs.boundAllyCanDamageMonster)
}

// monsterAIFoeMonster returns the OTHER monster m should pursue and strike, or
// nil if its foe is the party (or it has none):
//   - bound undead: the nearest enemy monster (within the seek radius).
//   - pacified charm: nil (fully passive - never fights).
//   - normal monster: the nearest bound undead within its alert radius, if one is
//     no farther than the party - so mobs turn on the bound undead in their midst.
func (cs *CombatSystem) monsterAIFoeMonster(m *monsterPkg.Monster3D) *monsterPkg.Monster3D {
	if m == nil {
		return nil
	}
	if m.Berserk.Active() && !m.IsAfraid() {
		return cs.nearestBerserkFoe(m)
	}
	if foe := cs.game.caravanFoe(m); foe != nil {
		return foe
	}
	switch behavior := m.CurrentAIBehavior(); {
	case !behavior.Caps().TakesFoe:
		return nil
	case behavior == monsterPkg.AIBehaviorBoundAlly:
		return cs.nearestEnemyMonster(m, cs.boundAllySeekRadius())
	}
	if m.PartyOutsideHome() {
		return nil
	}
	// Normal monster: only bother if any bound undead exist this frame.
	if len(cs.game.boundAllies) == 0 {
		return nil
	}
	// The nearest bound ally (party summon / bound undead) within the SEEK radius
	// competes with the party for aggro. A mob attacks whichever is closer; ties
	// stay with the party. A mob's own alert_radius is deliberately NOT used here:
	// it is often tiny while a ranged summon peppers it from beyond that radius.
	// Sight IS required to pick one up (nearestSeenMonster) - a summon must
	// not pull mobs through walls the party could never pull them through.
	// All monster-vs-summon pursuit/attack then flows through the shared crossfire
	// path (plain mob or champion alike).
	best := min(Distance(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y), cs.boundAllySeekRadius())
	return cs.nearestSeenMonster(m.X, m.Y, m, cs.game.boundAllies, best, func(*monsterPkg.Monster3D) bool { return true })
}

// refreshMonsterAITarget writes the one per-frame crossfire target and pursuit
// point consumed by both RT workers and the TB scheduler. Keep the foe selection
// and point assignment adjacent: a worker must never observe a target point that
// belongs to a different foe.
func (cs *CombatSystem) refreshMonsterAITarget(m *monsterPkg.Monster3D) {
	if cs == nil || m == nil {
		return
	}
	if cs.game.prepareAmbientTarget(m) {
		return
	}
	m.AIFoe = cs.monsterAIFoeMonster(m)
	m.AITargetX, m.AITargetY = cs.monsterAITargetPoint(m)
}

// monsterAITargetPoint is the world point a monster should pursue/engage, used by
// both the real-time and turn-based movement. It redirects controlled monsters off
// the party: a pacified charm stands still (targets itself), while a bound ally
// seeks its enemy or follows the party when idle. A normal mob chases its undead
// foe if it has one, else the party. Reads the per-frame cached AIFoe (set in
// refreshMonsterAIState) - never recomputes the foe.
func (cs *CombatSystem) monsterAITargetPoint(m *monsterPkg.Monster3D) (float64, float64) {
	switch m.CurrentAIBehavior() {
	case monsterPkg.AIBehaviorInert, monsterPkg.AIBehaviorPacified, monsterPkg.AIBehaviorEvasive:
		return m.X, m.Y // pacified: never chase the party - hold position
	}
	if m.AIFoe != nil {
		return m.AIFoe.X, m.AIFoe.Y
	}
	// Hostiles chase the party; an idle bound ally uses the same point to follow it.
	return cs.game.camera.X, cs.game.camera.Y
}

// monsterStrikeMonster resolves one melee hit from attacker onto target (a
// monster-vs-monster blow). A kill goes through the shared finalization: an
// enemy, or a bound ally a mob cut down (a converted enemy is still the
// party's kill), rewards the party in full; a pure party summon gives nothing.
func (cs *CombatSystem) monsterStrikeMonster(attacker, target *monsterPkg.Monster3D) {
	if attacker == nil || target == nil || !attacker.IsAlive() || !target.IsAlive() || !cs.attackLineClear(attacker.X, attacker.Y, target.X, target.Y) {
		return
	}
	cs.game.playMonsterSound(soundMonsterMeleeSwing, attacker)
	cs.strikeMonsterFor(attacker, target, cs.normalMonsterMeleeHitRolled(attacker), nil, false)
}

// strikeMonsterFor lands a monster-vs-monster blow of an explicit damage packet
// and element, with the shared hit-flash / message / kill / reward path. The packet
// sink for both a plain monster's attack (monsterStrikeMonster rolls it) and a
// champion's weapon sweep at summons (one swing roll applied to every caught
// target - the arc/AoE never re-rolls, matching the vs-party rule).
func (cs *CombatSystem) strikeMonsterFor(
	attacker, target *monsterPkg.Monster3D,
	hit monsterCharacterHit,
	weaponDef *config.WeaponDefinitionConfig,
	isRanged bool,
) {
	packet := singleMonsterDamagePacket(hit.Parts, hit.DamageType, 0)
	if cs.strikeMonsterPacketFor(attacker, target, packet, weaponDef, isRanged, hit.GrazesDodge, true, hit.Critical) && hit.ElementalAttack {
		cs.game.addMonsterElementalAttackFX(target, hit.DamageType)
	}
}

// monsterOnMonsterHit is one monster's hit on another monster.
type monsterOnMonsterHit struct {
	packet       monsterDamagePacket
	source       string // the log's source; "" is the attacker's name
	opts         monsterDamageOptions
	canDodge     bool
	grazesDodge  bool
	disintegrate float64 // instakill chance (rollMonsterDisintegrate)
	spellID      string  // riders read this spell's stun
	kind         damageLogKind
	evades       string // what a dodge evades in its line: the attack noun or ability name
	riders       bool
	critical     bool // the attacker's roll crit: its line says so
}

// monsterHitOutcome is what became of a monster's hit on another monster.
type monsterHitOutcome uint8

const (
	monsterHitMissed        monsterHitOutcome = iota // dodged, or the target was already dead
	monsterHitStruck                                 // landed: damage, riders, maybe the kill
	monsterHitDisintegrated                          // the instakill took the target whole
)

// landMonsterOnMonster is the one way a monster's hit lands on another
// monster - strikes, bolts, telegraph releases and fields alike: the target's
// dodge, disintegration, the damage and its log line, riders and the kill.
func (cs *CombatSystem) landMonsterOnMonster(attacker, target *monsterPkg.Monster3D, hit monsterOnMonsterHit) monsterHitOutcome {
	if target == nil || !target.IsAlive() {
		return monsterHitMissed // already slain this frame - no double damage/reward
	}
	cs.game.notifyCaravanAttack(target)
	source := logMonsterName(attacker)
	if hit.source != "" {
		source = logMonsterAs(attacker, hit.source)
	}
	if hit.canDodge && cs.monsterPerfectDodges(target) {
		through := hit.packet.throughDodge(hit.grazesDodge)
		dealt := cs.applyMonsterDamagePacket(target, through, hit.opts)
		actual := dealt.Total()
		if actual <= 0 {
			cs.game.addActorCombatMessage(attacker, target, styledFormat(uitext.Text("combat.monster.dodges", logSlot, logSlot, logSlot)), logMonsterName(target), source, hit.evades)
			return monsterHitMissed
		}
		cs.game.playMonsterSound(soundMonsterHit, target)
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.reportActorDamage(attacker, target, damageLog{Source: source, Target: monsterLogTarget(target), Amount: actual, School: through.loggedSchool(), Shares: dealtShares(dealt), Kind: damageLogDodge})
		if !target.IsAlive() {
			cs.finishActorKill(attacker, target)
		}
		return monsterHitMissed
	}
	packet, seared := hit.packet, false
	switch rollMonsterDisintegrate(target, hit.disintegrate) {
	case instakillKill:
		target.HitPoints = 0
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.addActorCombatMessage(attacker, target, styledFormat(uitext.Text("combat.monster.disintegrates", logSlot, logSlot, logSlot)), source, hit.evades, logMonsterName(target))
		cs.finishActorKill(attacker, target)
		return monsterHitDisintegrated
	case instakillSear:
		cs.game.addActorCombatMessage(attacker, target, styledFormat(uitext.Text("combat.instakill.withstands", logSlot, logSlot)), logMonsterName(target), uitext.Text("combat.instakill.disintegration"))
		packet, seared = packet.withTrue(instakillSearDamage(target)), true
	}
	if hit.packet.total() > 0 || seared {
		dealt := cs.applyMonsterDamagePacket(target, packet, hit.opts)
		actual := dealt.Total()
		if actual > 0 {
			cs.game.playMonsterSound(soundMonsterHit, target)
		}
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.reportActorDamage(attacker, target, damageLog{Source: source, Target: monsterLogTarget(target), Amount: actual, School: hit.packet.primarySchool(), Shares: dealtShares(dealt), Kind: hit.kind, Critical: hit.critical})
	}
	if target.IsAlive() {
		if hit.riders {
			cs.applyMonsterHitRiders(attacker, hit.spellID, monsterHitTarget{cs, attacker, target}, true)
		}
		return monsterHitStruck
	}
	cs.finishActorKill(attacker, target)
	return monsterHitStruck
}

// strikeMonsterPacketFor lands a monster's weapon strike or dart on another
// monster; it reports a struck hit that dealt damage.
func (cs *CombatSystem) strikeMonsterPacketFor(
	attacker, target *monsterPkg.Monster3D,
	packet monsterDamagePacket,
	weaponDef *config.WeaponDefinitionConfig,
	isRanged, grazesDodge, canDodge, critical bool,
) bool {
	if target == nil || !target.IsAlive() {
		return false
	}
	kind := damageLogStrike
	if attacker.Bound {
		kind = damageLogBoundStrike
	}
	hp := target.HitPoints
	outcome := cs.landMonsterOnMonster(attacker, target, monsterOnMonsterHit{
		packet: packet, opts: cs.monsterWeaponDamageOptions(weaponDef, target, isRanged),
		canDodge: canDodge, grazesDodge: grazesDodge, kind: kind, evades: uitext.Text("combat.noun.attack"), riders: true, critical: critical,
	})
	return outcome == monsterHitStruck && target.HitPoints < hp
}

// boundAttackNearest makes a bound undead attack the nearest enemy monster -
// but ONLY when that enemy is within real attack range (melee: reach; ranged:
// bolt range). It searches a wider seek radius: if the nearest enemy is found but
// still out of attack range it returns false, so the caller walks the undead
// toward it (it hunts instead of striking across the room). Returns true only
// when it actually attacked.
func (cs *CombatSystem) boundAttackNearest(m *monsterPkg.Monster3D) bool {
	if m == nil || m.AIFoe == nil {
		return false
	}
	return cs.commitMonsterAttack(m, monsterAttackDestination{foe: m.AIFoe}, monsterAttackRealtime)
}

// awardExperienceOnly grants the party a monster's XP with NO gold or loot - used
// when a bound (charmed) monster perishes as the party leaves the map.
func (cs *CombatSystem) awardExperienceOnly(monster *monsterPkg.Monster3D) {
	recipient := cs.game.rewardOwner()
	if monster == nil || recipient.party == nil || len(recipient.party.Members) == 0 {
		return
	}
	// The kill's own XP rule and share (awardExperienceAndGold), without the
	// gold and loot.
	if xp := cs.killExperience(monster); xp > 0 {
		recipient.grantMonsterXP(monster, recipient.xpShare(xp))
	}
}

// armorGMDodgeBonus grants ArmorGMDodgeBonus dodge for each Grandmaster-mastered
// armor type the character is wearing at least one piece of (e.g. GM Plate +
// plate equipped -> +5; also GM Shield + shield in the off-hand -> +10).
func (cs *CombatSystem) armorGMDodgeBonus(chr *character.MMCharacter) int {
	if chr == nil {
		return 0
	}
	bonus := 0
	if chr.SkillTier(character.SkillIronBody) >= int(character.MasteryGrandMaster) {
		bonus += character.IronBodyGMDodgeBonus
	}
	armorSlots := []items.EquipSlot{
		items.SlotOffHand, items.SlotArmor, items.SlotHelmet,
		items.SlotBoots, items.SlotCloak, items.SlotGauntlets, items.SlotBelt,
	}
	gmTypes := map[character.SkillType]bool{}
	for _, slot := range armorSlots {
		piece, ok := chr.Equipment[slot]
		if !ok {
			continue
		}
		st, ok := character.ArmorSkillForCategory(strings.ToLower(piece.ArmorCategory))
		if !ok {
			continue
		}
		if chr.SkillTier(st) >= int(character.MasteryGrandMaster) {
			gmTypes[st] = true
		}
	}
	return bonus + len(gmTypes)*ArmorGMDodgeBonus
}

// PerfectDodgeChance returns the clamped chance used by every party dodge roll.
// Keeping the pure calculation separate lets UI previews use the real value
// without consuming random numbers during Draw.
func (cs *CombatSystem) PerfectDodgeChance(chr *character.MMCharacter) int {
	if chr == nil {
		return 0
	}
	// Use effective stats so Bless and equipment affect dodge
	chance := chr.GetEffectiveLuck()/LuckToDodgeDivisor + cs.armorGMDodgeBonus(chr)
	if cs != nil && cs.game != nil && cs.game.isPartyMember(chr) {
		chance += cs.game.cardDodgeBonusPct() + cs.game.combatBuffDodgePct()
	}
	if chance < 0 {
		return 0
	}
	if chance > 100 {
		return 100
	}
	return chance
}

// RollPerfectDodge returns whether the character dodges and the exact chance
// used by the roll.
func (cs *CombatSystem) RollPerfectDodge(chr *character.MMCharacter) (bool, int) {
	chance := cs.PerfectDodgeChance(chr)
	roll := rand.Intn(100)
	return roll < chance, chance
}

// armorMitigationPctFromAC is the SINGLE source of truth for armor's percentage
// mitigation (diminishing returns), shared by the PARTY and MONSTERS:
// physical = min(75%, 100*AC/(AC+K)); elemental is that SAME curve scaled by
// 33/75, so it reaches its 33% cap at the exact AC where physical reaches 75%.
// Returns 0 for AC <= 0.
func armorMitigationPctFromAC(ac int, physical bool) int {
	if ac <= 0 {
		return 0
	}
	phys := 100 * ac / (ac + ArmorMitigationK)
	if phys > ArmorPhysicalMitigationCap {
		phys = ArmorPhysicalMitigationCap
	}
	if physical {
		return phys
	}
	return phys * ArmorElementalMitigationCap / ArmorPhysicalMitigationCap
}

// armorMitigationPct is the PARTY's armor mitigation (over summed equipped AC).
func (cs *CombatSystem) armorMitigationPct(char *character.MMCharacter, physical bool) int {
	return armorMitigationPctFromAC(cs.CalculateTotalArmorClass(char), physicalArmorCurve(physical, char.ArmorMelted()))
}

// mitigateCharacterDamage reduces incoming damage to a party member through the
// fixed pipeline:
//
//  1. Armor   - % mitigation of normal damage (cap 75% physical / 33%
//     elemental); skipped on armor-pierce.
//  2. Resist  - per-school gear resist + party resist buff, applied to normal
//     and typed true damage; capped 100% (100% == immunity -> 0 damage).
//  3. Flat    - additive reductions (Impenetrable Defense, Disarm Trap, Hour of
//     Power / Stone Skin), applied together AFTER the % steps; CAN drive damage
//     to 0.
//
// Armor and Resist are both multiplicative, so their order doesn't change the
// result; the additive flat step is applied last by design.
// schoolResistPct is the SINGLE source of truth for a party member's total
// percentage resistance to `school`: equipped gear (resistances map) + the
// all-damage party buff (Day of the Gods) + per-school spell buffs (Fire Shield)
// + card wards (Dragon Cards, Golden Thief Bug). Capped at 100 (100 = immune).
// Both damage mitigation and the character sheet read this, so the number the
// sheet shows always equals what actually reduces the hit.
func (g *MMGame) schoolResistPct(char *character.MMCharacter, school string) int {
	if char == nil {
		return 0
	}
	school = normalizeDamageTypeStr(school)
	total := char.GearResistPct(school) + g.combatBuffResistPct() +
		g.combatBuffSchoolResistPct(school) + g.cardResistBonusFor(school)
	if total > 100 {
		total = 100
	}
	return total
}

// mitigateCharacterDamageParts is the single party-member mitigation pipeline.
// Both components carry one school and meet its resistance. Armor and flat
// reductions apply only to Normal; True also lands through Perfect Dodge, which
// is handled by monsterHitCharacter before this sink.
func (cs *CombatSystem) mitigateCharacterDamageParts(parts damagecalc.Parts, damageTypeStr string, char *character.MMCharacter, ignoreArmor bool) damagecalc.Parts {
	return cs.mitigateCharacterDamagePartsWithArmorPierce(parts, damageTypeStr, char, ignoreArmor, 0, false)
}

// mitigateCharacterDamagePartsWithArmorPierce adds a weapon's armor pierce and,
// for a projectile, the physical projectile pierce (physicalProjectilePierces).
func (cs *CombatSystem) mitigateCharacterDamagePartsWithArmorPierce(
	parts damagecalc.Parts,
	damageTypeStr string,
	char *character.MMCharacter,
	ignoreArmor bool,
	armorPiercePct int,
	projectile bool,
) damagecalc.Parts {
	return cs.mitigateCharacterParts(parts, damageTypeStr, char, ignoreArmor, armorPiercePct, cs.projectilePierce(projectile, damageTypeStr))
}

// projectilePierce is a hit's physical projectile pierce roll, drawn at most
// once however often the hit is mitigated (a graze and Storm Reversal's full
// hit share it); nil when the hit is no projectile.
func (cs *CombatSystem) projectilePierce(projectile bool, damageTypeStr string) func() bool {
	if !projectile {
		return nil
	}
	rolled, pierced := false, false
	return func() bool {
		if !rolled {
			rolled, pierced = true, cs.physicalProjectilePierces(normalizeDamageTypeStr(damageTypeStr))
		}
		return pierced
	}
}

// mitigateCharacterParts resolves armor, resistance and flat reductions;
// pierce (may be nil) is asked only when armor would mitigate.
func (cs *CombatSystem) mitigateCharacterParts(
	parts damagecalc.Parts,
	damageTypeStr string,
	char *character.MMCharacter,
	ignoreArmor bool,
	armorPiercePct int,
	pierce func() bool,
) damagecalc.Parts {
	if char == nil || (parts.Normal <= 0 && parts.True <= 0) {
		return parts
	}
	hadNormalDamage := parts.Normal > 0
	school := normalizeDamageTypeStr(damageTypeStr)
	physical := school == monsterPkg.DamagePhysical.String()

	// 1) Armor (% mitigation; also blunts elemental on a scaled-down curve).
	if parts.Normal > 0 && !ignoreArmor {
		ac := cs.CalculateTotalArmorClass(char)
		if armorPiercePct > 0 {
			if armorPiercePct > 100 {
				armorPiercePct = 100
			}
			ac = ac * (100 - armorPiercePct) / 100
		}
		if pierce != nil && ac > 0 && pierce() {
			ac = 0 // an armor-piercing shot
		}
		if mit := armorMitigationPctFromAC(ac, physicalArmorCurve(physical, char.ArmorMelted())); mit > 0 {
			parts.Normal = parts.Normal * (100 - mit) / 100
		}
	}
	// 2) Resistance: the single school-resist total (gear + party buff + per-school
	//    buffs + card wards), computed once so the character sheet shows exactly
	//    what reduces the hit. It applies to BOTH normal and typed true damage.
	resist := cs.game.schoolResistPct(char, school)
	parts = parts.ApplyResistance(resist, 0)
	// The % steps alone never fully negate a real hit - keep a 1-damage chip...
	if hadNormalDamage && resist < 100 && parts.Normal < 1 {
		parts.Normal = 1
	}
	// 3) ...then the flat reductions (DisarmTrap + Hour of Power / Stone Skin),
	//    which CAN finish normal damage off to 0. True and DoTs bypass this step.
	parts.Normal -= personalSkillDamageReduction(char)
	parts.Normal -= cs.game.combatBuffInReduce()
	if parts.Normal < 0 {
		parts.Normal = 0
	}
	return parts
}

func personalSkillDamageReduction(char *character.MMCharacter) int {
	if char == nil {
		return 0
	}
	reduction := char.DisarmTrapTier() * DisarmTrapDamageReductionPerTier
	if char.HasSkill(character.SkillImpenetrableDefense) {
		reduction += character.ImpenetrableDefenseReduction(char.SkillTier(character.SkillImpenetrableDefense))
	}
	return reduction
}

// PhysicalMitigation is the breakdown of how an incoming PHYSICAL hit is reduced,
// in the exact order mitigateCharacterDamage applies it. The percentage steps and
// the floor make the result depend on the incoming hit, so the UI renders the
// pipeline, not a single total.
type PhysicalMitigation struct {
	ArmorClass int // total AC across equipped armor
	ArmorPct   int // armor % mitigation vs physical (capped 75)
	ResistPct  int // physical resistance % (gear + party buff, capped 100; 100 = immune)
	SkillFlat  int // combined personal skill reduction, applied AFTER the % steps with FlatBuff
	FlatBuff   int // flat reduction applied after the % steps (Hour of Power / Stone Skin)
}

// PhysicalMitigationBreakdown decomposes physical mitigation for the character
// sheet, reading the SAME pieces mitigateCharacterDamage uses so the UI can't
// drift from combat. Order matches combat: armor % -> resist % -> floor -> (skill flat + flat buff).
func (cs *CombatSystem) PhysicalMitigationBreakdown(char *character.MMCharacter) PhysicalMitigation {
	if cs == nil || char == nil {
		return PhysicalMitigation{}
	}
	return PhysicalMitigation{
		ArmorClass: cs.CalculateTotalArmorClass(char),
		ArmorPct:   cs.armorMitigationPct(char, true),
		SkillFlat:  personalSkillDamageReduction(char),
		ResistPct:  cs.game.schoolResistPct(char, monsterPkg.DamagePhysical.String()),
		FlatBuff:   cs.game.combatBuffInReduce(),
	}
}

func isPhysicalDamageType(damageTypeStr string) bool {
	return normalizeDamageTypeStr(damageTypeStr) == monsterPkg.DamagePhysical.String()
}

// normalizeDamageTypeStr is the game-side data boundary for damage schools.
// Config loaders reject unknown schools; the physical fallback protects old or
// synthetic runtime values without creating a second school catalog here.
func normalizeDamageTypeStr(damageTypeStr string) string {
	return convertToMonsterDamageType(damageTypeStr).String()
}

func weaponDamageTypeStr(weaponDef *config.WeaponDefinitionConfig) string {
	if weaponDef != nil && weaponDef.DamageType != "" {
		return normalizeDamageTypeStr(weaponDef.DamageType)
	}
	return monsterPkg.DamagePhysical.String()
}

func spellDamageTypeStr(spellType string) string {
	if spellDef, err := spells.GetSpellDefinitionByID(spells.SpellID(spellType)); err == nil {
		return spellDef.DamageType()
	}
	return monsterPkg.DamagePhysical.String()
}

func convertToMonsterDamageType(damageTypeStr string) monsterPkg.DamageType {
	damageType, err := monsterPkg.ParseDamageType(damageTypeStr)
	if err != nil {
		return monsterPkg.DamagePhysical
	}
	return damageType
}

// physicalArmorCurve picks the armor curve a hit meets: the physical one for a
// physical hit on sound armor. Melted armor (ArmorMelted) blunts a physical hit
// only as far as an elemental one.
func physicalArmorCurve(physical, melted bool) bool { return physical && !melted }

// physicalProjectilePierces rolls the armor pierce every physical projectile
// carries, whoever fired it and whoever it hits: ArmorPierceRangedChancePct to
// ignore the target's armor entirely.
func (cs *CombatSystem) physicalProjectilePierces(damageTypeStr string) bool {
	if !isPhysicalDamageType(damageTypeStr) {
		return false
	}
	roll := rand.Intn
	if cs != nil && cs.projectilePierceRoll != nil {
		roll = cs.projectilePierceRoll
	}
	return roll(100) < ArmorPierceRangedChancePct
}

// applyMonsterArmor reduces a hit by the monster's armor using the SAME % model
// as the party (armorMitigationPctFromAC): physical capped 75%, elemental scaled
// to 33%. A ranged shot may pierce it (physicalProjectilePierces). Armor alone
// never fully negates a hit (floor 1); resistance (TakeDamageResist, applied
// next) is what can take it to 0.
func (cs *CombatSystem) applyMonsterArmor(damage int, damageTypeStr string, armorClass int, projectile, melted bool) int {
	if damage <= 0 || armorClass <= 0 {
		return damage
	}
	if projectile && cs.physicalProjectilePierces(damageTypeStr) {
		return damage // armor-piercing shot
	}
	mit := armorMitigationPctFromAC(armorClass, physicalArmorCurve(isPhysicalDamageType(damageTypeStr), melted))
	if mit <= 0 {
		return damage
	}
	reduced := damage * (100 - mit) / 100
	if reduced < 1 {
		reduced = 1
	}
	return reduced
}

func (cs *CombatSystem) armorMasteryBonus(char *character.MMCharacter, armor items.Item) int {
	if char == nil {
		return 0
	}
	skillType, ok := armorMasterySkill(armor)
	if !ok {
		return 0
	}
	if skill, exists := char.Skills[skillType]; exists {
		return int(skill.Mastery) * MasteryArmorACPerLevel
	}
	return 0
}

// monsterLootEntries resolves the authored table, including the universal boss
// pool. Consumers exclude kill_only trophies unless resolving a real death.
func (cs *CombatSystem) monsterLootEntries(monster *monsterPkg.Monster3D) []config.LootEntry {
	if monster == nil {
		return nil
	}
	// Resolve loot by the monster's canonical YAML key (always set), NOT by
	// name: several monsters can share a display Name (the four elemental
	// dragons are all "Dragon"), so a name lookup would scramble their loot.
	return config.GetLootTable(monster.Key, monster.IsBoss())
}

func (cs *CombatSystem) rollMonsterLoot(monster *monsterPkg.Monster3D) []items.Item {
	return rollLootEntries(cs.monsterLootEntries(monster))
}

// pocketLoot is what Sleight of Hand lifts: the monster's loot roll, except a
// champion's, whose drop is its victory trophy.
func (cs *CombatSystem) pocketLoot(monster *monsterPkg.Monster3D) []items.Item {
	if monster.IsChampion() {
		return nil
	}
	return rollLootEntries(nonKillLoot(cs.monsterLootEntries(monster)))
}

// rollLootEntries resolves independent loot chances into items. Both normal
// monster loot and the global boss-death pool use this exact entry contract.
// A first_of entry rolls its group in order and keeps the first success.
func rollLootEntries(entries []config.LootEntry) []items.Item {
	drops := make([]items.Item, 0, len(entries))
	for _, e := range entries {
		for range e.RollCount() {
			if e.Type == "first_of" {
				if first := rollLootEntries(firstLootSuccess(e.Of)); len(first) > 0 {
					drops = append(drops, first...)
				}
				continue
			}
			if rand.Float64() < e.Chance {
				landed, err := createLootDrops(e)
				if err != nil {
					fmt.Printf("[WARN] loot drop failed: %v\n", err)
				}
				drops = append(drops, landed...)
			}
		}
	}
	return drops
}

// firstLootSuccess rolls a first_of group in order and returns its first
// successful entry as a certain drop, or nothing.
func firstLootSuccess(group []config.LootEntry) []config.LootEntry {
	for _, e := range group {
		for range e.RollCount() {
			if rand.Float64() < e.Chance {
				won := e
				won.Chance, won.Rolls = 1, 0
				return []config.LootEntry{won}
			}
		}
	}
	return nil
}

// checkMonsterLootDrop handles loot drops when monsters are killed.
func (cs *CombatSystem) checkMonsterLootDrop(monster *monsterPkg.Monster3D) []items.Item {
	return cs.rollMonsterLoot(monster)
}

// randomLivingMember returns a random alive+conscious party member, weighted
// by Halfling Guile (nil if the whole party is down). Used for MELEE targeting
// in both modes.
func (cs *CombatSystem) randomLivingMember() *character.MMCharacter {
	alive := alivePartyIndices(cs.game.party.Members)
	if len(alive) == 0 {
		return nil
	}
	return cs.game.party.Members[cs.weightedPartyTargetIndex(alive)]
}

// randomLivingMembers returns up to n DISTINCT living members in random order -
// the target set of a champion's melee arc (each catches the same swing once).
func (cs *CombatSystem) randomLivingMembers(n int) []*character.MMCharacter {
	alive := alivePartyIndices(cs.game.party.Members)
	if n > len(alive) {
		n = len(alive)
	}
	out := make([]*character.MMCharacter, 0, n)
	for len(out) < n {
		idx := cs.weightedPartyTargetIndex(alive)
		out = append(out, cs.game.party.Members[idx])
		for i, candidate := range alive {
			if candidate == idx {
				alive = append(alive[:i], alive[i+1:]...)
				break
			}
		}
	}
	return out
}

// weightedPartyTargetIndex gives a hero with Halfling Guile one ticket and
// every other hero two. Thus that hero has exactly half another hero's
// relative probability in every random party-target draw, including draws
// without replacement.
func (cs *CombatSystem) weightedPartyTargetIndex(indices []int) int {
	return cs.weightedPartyTargetIndexWithBase(indices, func(int) int { return 1 })
}

func (cs *CombatSystem) partyTargetWeight(idx, baseWeight int) int {
	weight := baseWeight * 2
	if member := cs.game.party.Members[idx]; member != nil && member.HasSkill(character.SkillHalflingGuile) {
		weight /= 2
	}
	return weight
}

// weightedPartyTargetIndexWithBase applies the racial multiplier at the final
// draw, after the caller has expressed positional or attack-specific bias.
func (cs *CombatSystem) weightedPartyTargetIndexWithBase(indices []int, baseWeight func(int) int) int {
	if len(indices) == 0 {
		return -1
	}
	total := 0
	for _, idx := range indices {
		total += cs.partyTargetWeight(idx, baseWeight(idx))
	}
	roll := rand.Intn(total)
	for _, idx := range indices {
		weight := cs.partyTargetWeight(idx, baseWeight(idx))
		if roll < weight {
			return idx
		}
		roll -= weight
	}
	return indices[len(indices)-1]
}

// tankIndex returns the party slot that counts as the "tank": the FRONT slot
// (index 0) while it's alive, else the first living member. -1 if all down.
func (cs *CombatSystem) tankIndex() int {
	m := cs.game.party.Members
	if len(m) > 0 && m[0] != nil && m[0].HitPoints > 0 {
		return 0
	}
	for i, x := range m {
		if x != nil && x.HitPoints > 0 {
			return i
		}
	}
	return -1
}

// rangedTarget preserves the authored all-human tank/off-tank split, then gives
// every halfling half the raw ticket weight of the same slot occupied by any
// other race. The same rule is used in RT and TB.
func (cs *CombatSystem) rangedTarget() *character.MMCharacter {
	ti := cs.tankIndex()
	if ti < 0 {
		return nil
	}
	indices := make([]int, 0, len(cs.game.party.Members))
	for i, member := range cs.game.party.Members {
		if member != nil && member.HitPoints > 0 {
			indices = append(indices, i)
		}
	}
	if len(indices) == 1 {
		return cs.game.party.Members[indices[0]]
	}
	offTankTickets := max(1, int(math.Round(RangedOffTankChance*100)))
	tankTickets := max(1, 100-offTankTickets) * (len(indices) - 1)
	idx := cs.weightedPartyTargetIndexWithBase(indices, func(idx int) int {
		if idx == ti {
			return tankTickets
		}
		return offTankTickets
	})
	return cs.game.party.Members[idx]
}

// findCharacterIndex finds the index of a character in the party
func (cs *CombatSystem) findCharacterIndex(targetChar *character.MMCharacter) int {
	for i, member := range cs.game.party.Members {
		if member == targetChar {
			return i
		}
	}
	// Fallback to selected character if not found
	return cs.game.selectedChar
}

package game

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/mathutil"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// CombatSystem handles all combat-related functionality
type CombatSystem struct {
	game *MMGame
}

// NewCombatSystem creates a new combat system
func NewCombatSystem(game *MMGame) *CombatSystem {
	return &CombatSystem{game: game}
}

// CastEquippedSpell performs a magic attack using equipped spell (unified F key casting).
// Returns true if the spell was successfully cast.
func (cs *CombatSystem) CastEquippedSpell() bool {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot cast
	if caster.IsIncapacitated() {
		return false
	}

	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return false // No spell equipped
	}
	if spell.Type == items.ItemTrap {
		// Thief quick slot: F arms the slotted trap exactly like a quick
		// spell - explicit cast, so refusal messages stay on.
		_, placed := cs.tryPlaceQuickTrap(caster, true)
		return placed
	}
	if spell.Type != items.ItemBattleSpell && spell.Type != items.ItemUtilitySpell {
		return false // SlotSpell should only contain spells
	}

	spellID := spells.SpellID(spell.SpellEffect)
	spellDef, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return false
	}

	// Quick-cast stays quiet on launch; the hit itself reports (anti-spam).
	return cs.castResolvedSpell(spellID, spellDef, caster, cs.effectiveSpellCost(caster, spell.SpellCost), false, true)
}

// healMember is the single source for applying a heal to ONE party member: add
// HP, clamp to max, clear Unconscious when revived above 0, and flash the green
// "+" overlay. Every single-target heal path funnels through here so the clamp /
// revive / VFX behaviour can't drift between them.
func (cs *CombatSystem) healMember(idx, amount int) {
	if idx < 0 || idx >= len(cs.game.party.Members) {
		return
	}
	m := cs.game.party.Members[idx]
	if m == nil {
		return
	}
	m.HitPoints += amount
	if m.HitPoints > m.MaxHitPoints {
		m.HitPoints = m.MaxHitPoints
	}
	if m.HitPoints > 0 {
		m.RemoveCondition(character.ConditionUnconscious)
	}
	cs.game.TriggerPartyHeal(idx) // rising green "+" overlay on the healed card
}

func (cs *CombatSystem) healWholeParty(amount int) int {
	healed := 0
	for i, m := range cs.game.party.Members {
		if m == nil || m.HitPoints <= 0 ||
			m.HasCondition(character.ConditionDead) || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		cs.healMember(i, amount)
		healed++
	}
	return healed
}

// knockOut handles a member reaching 0 HP from a hit: the Lich Card may cheat
// death (restore half HP + half SP), otherwise the member falls unconscious.
// Single chokepoint so the save applies to every lethal branch alike.
func (cs *CombatSystem) knockOut(target *character.MMCharacter) {
	if pct := cs.game.cardLethalSavePct(); pct > 0 && rand.Intn(100) < pct {
		reviveHalf(target)
		cs.game.AddCombatMessage(fmt.Sprintf("%s cheats death! (Lich Card)", target.Name))
		return
	}
	target.AddCondition(character.ConditionUnconscious)
	cs.game.AddCombatMessage(fmt.Sprintf("%s falls unconscious!", target.Name))
}

// knockOutLethalDoTVictims finds party members poison/burn ticked to 0 HP this
// frame and routes them through the real knockOut (Lich Card save + message) -
// updatePoison/updateBurn only clamp HP, they can't call knockOut themselves
// (character can't import game).
func (cs *CombatSystem) knockOutLethalDoTVictims() {
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
		cs.game.AddCombatMessage(fmt.Sprintf("%s's Ningyo Card mends %d HP.", m.Name, amt))
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

// tryCardMoveBurst rolls the Gorilla Titan Card's on-move shockwave: pure damage
// to monsters next to the party. Called once per tile the party steps into.
func (cs *CombatSystem) tryCardMoveBurst() {
	pct := cs.game.cardMoveAoePct()
	if pct <= 0 || rand.Intn(100) >= pct {
		return
	}
	if cs.cardMoveBurstApply(cs.game.cardMoveAoeDmg()) {
		cs.game.AddCombatMessage(fmt.Sprintf("The Gorilla Titan Card erupts for %d pure damage!", cs.game.cardMoveAoeDmg()))
	}
}

// cardMoveBurstApply deals `dmg` pure damage to every living monster within 1.5
// tiles of the party. Returns whether anything was hit. Deterministic core of
// the Gorilla move-burst (the roll lives in tryCardMoveBurst).
func (cs *CombatSystem) cardMoveBurstApply(dmg int) bool {
	if dmg <= 0 || cs.game.world == nil {
		return false
	}
	radius := float64(cs.game.config.GetTileSize()) * 1.5
	px, py := cs.game.camera.X, cs.game.camera.Y
	hit := false
	for _, m := range cs.game.world.Monsters {
		// "Nearby FOES" only: never the party's own bound allies (card summons /
		// bind-undead), charmed (pacified) monsters, or an invulnerable boss (sealed/
		// idol-warded) - the latter would absorb the damage yet still flash + log a hit.
		if m == nil || !m.IsAlive() || m.Bound || m.Pacified || bossInvulnerable(m) ||
			math.Hypot(m.X-px, m.Y-py) > radius {
			continue
		}
		// Pure: bypass armor (TakeDamage skips AC) AND resistance (100% resist-pierce),
		// so physical-resistant/immune mobs still take the full advertised amount.
		m.TakeDamageResist(dmg, monsterPkg.DamagePhysical, 100)
		m.HitTintFrames = MonsterHitFlashFrames
		hit = true
		if !m.IsAlive() {
			cs.finishMonsterKill(m)
		}
	}
	return hit
}

// countCardSummons counts living allies summoned by the card collection.
func (cs *CombatSystem) countCardSummons() int {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return 0
	}
	n := 0
	for _, m := range w.Monsters {
		if m != nil && m.IsAlive() && m.SummonedBy == cardSummonOwner {
			n++
		}
	}
	return n
}

// tryCardSummonOnAction rolls the Orc Warlord Card on a party action: a chance to
// summon allied monsters (Bound - they hunt enemy monsters, ignore the party) up
// to the collection's summon limit. Called from the attack and cast chokepoints.
// A successful summon arms the CARD's own cooldown (card_summon_cd_seconds) -
// it silences only this proc, never the character's actions.
func (cs *CombatSystem) tryCardSummonOnAction() {
	if cs.game.cardSummonCDFrames > 0 {
		return
	}
	chance := cs.game.cardSummonChance()
	limit := cs.game.cardSummonLimit()
	key := cs.game.cardSummonMonsterKey()
	if chance <= 0 || limit <= 0 || key == "" || cs.game.GetCurrentWorld() == nil {
		return
	}
	if rand.Intn(100) >= chance {
		return
	}
	if want := limit - cs.countCardSummons(); want > 0 {
		// Arm the cooldown only when allies actually appeared: a whiffed spawn
		// (no free tile around the party) must not waste the proc for 5s.
		if cs.summonCardAllies(key, want) > 0 {
			cs.game.cardSummonCDFrames = cs.game.cardSummonCDSeconds() * cs.game.config.GetTPS()
		}
	}
}

// markCardAlly turns a spawned monster into a party ally summoned by the card
// collection: Bound (hunts enemy monsters, ignores the party), tagged for the
// summon limit, and excluded from map-clear quest counts.
// BoundFramesRemaining 0 = never expires (the bind tick only counts down > 0).
// A card ally is a PURE summon (never was an enemy): it yields the party no
// XP/gold/loot on death and does not follow across maps - it simply crumbles
// when the party leaves (a fresh set is re-summoned there via the proc).
func markCardAlly(m *monsterPkg.Monster3D) {
	m.Bound = true
	m.BoundFramesRemaining = 0
	m.WasAttacked = false
	m.SummonedBy = cardSummonOwner
	m.QuestProgressIgnored = true
}

// isCardAlly reports whether a monster is a card-collection summon (Orc Warlord
// Card huntresses) - a pure ally, distinct from a spell-bound undead (a former
// ENEMY that still yields its reward). The single source of truth for the
// "yields the party nothing / crumbles on map exit" rules.
func isCardAlly(m *monsterPkg.Monster3D) bool {
	return m != nil && m.SummonedBy == cardSummonOwner
}

// crumbleBoundAlliesOnDeparture removes the party's bound allies from the world
// being left. A bound undead (a former enemy) grants XP but no loot or gold; a
// pure card ally yields nothing. The removal is immediate because the normal
// end-of-frame death sweep runs only on the newly entered world.
func (g *MMGame) crumbleBoundAlliesOnDeparture(departing *world.World3D) {
	if departing == nil || g.combat == nil {
		return
	}
	kept := departing.Monsters[:0]
	for _, m := range departing.Monsters {
		if m == nil || !m.Bound || !m.IsAlive() {
			kept = append(kept, m)
			continue
		}
		if !isCardAlly(m) {
			g.combat.awardExperienceOnly(m)
			g.AddCombatMessage(fmt.Sprintf("Your bound %s crumbles as you leave.", m.Name))
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
}

// summonCardAllies spawns up to n permanent allied (Bound) monsters of `key` near
// the party. BoundFramesRemaining 0 = never expires (the bind tick only counts
// down values > 0), so they fight on until slain. Returns how many spawned.
func (cs *CombatSystem) summonCardAllies(key string, n int) int {
	tile := float64(cs.game.config.GetTileSize())
	px, py := cs.game.camera.X, cs.game.camera.Y
	spawned := 0
	for attempts := 0; spawned < n && attempts < n*12+12; attempts++ {
		angle := rand.Float64() * 2 * math.Pi
		sx, sy, ok := cs.findNearestSummonTile(px+math.Cos(angle)*2*tile, py+math.Sin(angle)*2*tile, 10)
		if !ok {
			continue
		}
		add := monsterPkg.NewMonster3DFromConfig(sx, sy, key, cs.game.config)
		if add == nil {
			continue
		}
		markCardAlly(add)
		cs.game.registerSpawnedMonster(add)
		cs.game.refreshMonsterCollisionSolidity(add)
		spawned++
	}
	if spawned > 0 {
		cs.game.AddCombatMessage(fmt.Sprintf("The Orc Warlord Card rallies %d ally to your side!", spawned))
	}
	return spawned
}

func (cs *CombatSystem) CastEquippedHealOnTarget(targetIndex int) bool {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot cast heals
	if caster.IsIncapacitated() {
		return false
	}

	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return false
	}
	// Allow both heal-type spells for targeting
	if spell.SpellEffect != items.SpellEffectHealSelf && spell.SpellEffect != items.SpellEffectHealOther {
		return false
	}

	spellID := spells.SpellID(spell.SpellEffect)
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	// SP gate, target resolution (self-only heals redirect to the caster),
	// 0-HP refusal, cost and messages all live in the ONE heal cast path.
	return cs.castKnownHealOn(spellID, def, targetIndex)
}

// bestKnownHealSpell returns the most powerful heal spell the caster knows
// across all their magic schools, preferring the highest spell Level (ties
// broken by HealAmount, then by HealParty). Returns false if they know none.
func (cs *CombatSystem) bestKnownHealSpell(caster *character.MMCharacter) (spells.SpellID, bool) {
	var bestID spells.SpellID
	var best spells.SpellDefinition
	found := false
	for _, school := range caster.MagicSchools {
		if school == nil {
			continue
		}
		for _, id := range school.KnownSpells {
			def, err := spells.GetSpellDefinitionByID(id)
			if err != nil || !def.IsHeal() {
				continue
			}
			better := !found ||
				def.Level > best.Level ||
				(def.Level == best.Level && def.HealAmount > best.HealAmount) ||
				(def.Level == best.Level && def.HealAmount == best.HealAmount && def.HealParty && !best.HealParty)
			if better {
				bestID, best, found = id, def, true
			}
		}
	}
	return bestID, found
}

// CastBestHealOnTarget casts the selected character's strongest known heal (by
// level) - bound to the C key. Party heals hit everyone; self-only heals (e.g.
// First Aid) ignore the requested target and heal the caster; other heals use
// targetIndex (resolved from the mouse by the caller). Returns whether a heal
// fired plus the spell used (for the real-time cooldown).
func (cs *CombatSystem) CastBestHealOnTarget(targetIndex int) (bool, spells.SpellID) {
	caster := cs.game.party.Members[cs.game.selectedChar]
	if caster.IsIncapacitated() {
		return false, ""
	}
	spellID, ok := cs.bestKnownHealSpell(caster)
	if !ok {
		// Silent no-op: the C-key cycle only ever targets known healers, so this
		// just guards stray callers - no chat spam.
		return false, ""
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false, ""
	}
	if cs.castKnownHealOn(spellID, def, targetIndex) {
		return true, spellID
	}
	return false, ""
}

// castKnownHealOn pays for and casts a known (book) heal on a resolved party
// target. Party heals restore everyone; self-only heals always land on the
// caster; an out-of-range target falls back to the caster. Callers resolve def
// once and pass it in.
func (cs *CombatSystem) castKnownHealOn(spellID spells.SpellID, def spells.SpellDefinition, targetIndex int) bool {
	caster := cs.game.party.Members[cs.game.selectedChar]
	spellCost := cs.effectiveSpellCost(caster, def.SpellPointsCost)
	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's %s fizzles! (Not enough SP: %d/%d)",
			caster.Name, def.Name, caster.SpellPoints, spellCost))
		return false
	}

	_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)

	// Party heal: restore everyone, ignore the single target.
	if def.HealParty {
		caster.SpellPoints -= spellCost
		n := cs.healWholeParty(healAmount)
		cs.game.AddCombatMessage(fmt.Sprintf("%s casts %s, healing %d allies for %d HP!",
			caster.Name, def.Name, n, healAmount))
		return true
	}

	// Single-target heal. Self-only heals (TargetSelf) always land on the caster.
	if def.TargetSelf {
		targetIndex = cs.game.selectedChar
	}
	if targetIndex < 0 || targetIndex >= len(cs.game.party.Members) {
		targetIndex = cs.game.selectedChar
	}
	target := cs.game.party.Members[targetIndex]
	if target.HitPoints <= 0 || target.HasCondition(character.ConditionDead) || target.HasCondition(character.ConditionEradicated) {
		cs.game.AddCombatMessage(fmt.Sprintf("%s cannot be healed from 0 HP.", target.Name))
		return false
	}

	caster.SpellPoints -= spellCost
	cs.healMember(targetIndex, healAmount)
	if targetIndex == cs.game.selectedChar {
		cs.game.AddCombatMessage(fmt.Sprintf("%s heals themselves for %d HP with %s!", caster.Name, healAmount, def.Name))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s heals %s for %d HP with %s!", caster.Name, target.Name, healAmount, def.Name))
	}
	return true
}

// SmartAttack is the Space-key "smart attack" (both modes). Priority:
//  1. ANYONE in the party is wounded and the caster has a heal (spellbook or
//     quick slot) -> cast it on the MOST wounded. The quick-slotted heal is
//     preferred; otherwise the strongest book heal. Healers can keep a combat
//     spell in the quick slot and still auto-triage.
//  2. No one wounded -> cast the quick-slotted offensive spell when payable
//     (Monk skips this: their quick spell is reserved for Spiritual Training).
//  3. Otherwise swing the equipped weapon.
//
// Returns (acted, castSpellID): acted is false when NOTHING happened (no
// wounded+heal, no castable quick spell, no weapon) so turn-based slots aren't
// burned; castSpellID is non-empty when a spell was cast (RT picks the spell
// cooldown from it, else the weapon cooldown).
func (cs *CombatSystem) SmartAttack() (bool, spells.SpellID) {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// In RT, a dual-wielder can reach Space with the main hand / cast cooldown
	// still cycling because the off hand is free. That path must swing only the
	// off hand; it cannot sneak in a spell or heal. TB uses action slots instead,
	// so a preserved RT cooldown must not change its smart-action priority.
	if cs.game.turnBasedMode || caster.RTCooldown <= 0 {
		if healID, def, target, ok := cs.smartHealPlan(caster); ok {
			if caster.SpellPoints >= cs.effectiveSpellCost(caster, def.SpellPointsCost) &&
				cs.castKnownHealOn(healID, def, target) {
				return true, healID
			}
			// Can't pay / can't land it -> fall through to attack.
		}

		if caster.Class != character.ClassMonk {
			if spell, hasSpell := caster.Equipment[items.SlotSpell]; hasSpell {
				spellID := spells.SpellID(spell.SpellEffect)
				def, err := spells.GetSpellDefinitionByID(spellID)
				canPay := caster.SpellPoints >= cs.effectiveSpellCost(caster, spell.SpellCost)
				if err == nil && def.IsOffensive() && canPay && cs.CastEquippedSpell() {
					return true, spellID
				}
			}
		}

		// Trap book (thief): Space arms the slotted trap before falling back to
		// the weapon. Silent on refusal (no SP / limit / no room) - quick spells
		// fall through to the weapon just as quietly via their canPay pre-check.
		if trapKey, placed := cs.tryPlaceQuickTrap(caster, false); placed {
			return true, spells.SpellID(trapKey)
		}
	}

	return cs.EquipmentMeleeAttack(), ""
}

// smartHealPlan decides which heal Space should cast and on whom: the
// quick-slotted heal if it can serve a wounded ally, else the strongest book
// heal. ok=false when no one is wounded enough or no usable heal exists.
func (cs *CombatSystem) smartHealPlan(caster *character.MMCharacter) (spells.SpellID, spells.SpellDefinition, int, bool) {
	candidates := make([]spells.SpellID, 0, 2)
	if spell, hasSpell := caster.Equipment[items.SlotSpell]; hasSpell {
		candidates = append(candidates, spells.SpellID(spell.SpellEffect))
	}
	if bookID, known := cs.bestKnownHealSpell(caster); known {
		candidates = append(candidates, bookID)
	}
	for _, id := range candidates {
		def, err := spells.GetSpellDefinitionByID(id)
		if err != nil || !def.IsHeal() {
			continue
		}
		if target := cs.mostWoundedHealTarget(def); target >= 0 {
			return id, def, target, true
		}
	}
	return "", spells.SpellDefinition{}, -1, false
}

// mostWoundedHealTarget returns the party index of the most-wounded ally a
// heal should target (lowest HP fraction, below SmartHealWoundedPct), or -1
// if no one is hurt enough. A self-only heal (First Aid) only ever considers
// the caster; an other-target heal considers the whole party. Dead/KO members
// are skipped (heals don't revive).
func (cs *CombatSystem) mostWoundedHealTarget(def spells.SpellDefinition) int {
	best, bestFrac := -1, SmartHealWoundedPct
	for i, m := range cs.game.party.Members {
		if m == nil || !m.CanAct() || m.MaxHitPoints <= 0 {
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

// partyEntombed reports whether the flying party hovers inside terrain that is
// solid without Fly (a wall). Fighting and casting from there are refused:
// monsters can neither reach nor see the party, so it would be a free-hit
// exploit. Emits a throttled explanation so the refusal reads as a rule.
func (cs *CombatSystem) partyEntombed() bool {
	g := cs.game
	if !g.flyActive {
		return false
	}
	w := g.GetCurrentWorld()
	if w == nil {
		return false
	}
	ts := g.config.GetTileSize()
	if !w.IsTileBlockingTerrainAt(int(g.camera.X/ts), int(g.camera.Y/ts)) {
		return false
	}
	if g.frameCount-g.entombedMsgFrame > int64(g.config.GetTPS()) {
		g.entombedMsgFrame = g.frameCount
		g.AddCombatMessage("Buried inside solid terrain, the party cannot fight - fly clear first!")
	}
	return true
}

// EquipmentMeleeAttack performs a melee attack using equipped weapon
// EquipmentMeleeAttack swings the equipped weapon; reports whether an attack
// actually happened (no weapon / incapacitated -> false, so turn-based action
// slots aren't burned on a no-op).
func (cs *CombatSystem) EquipmentMeleeAttack() bool {
	attacker := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot attack
	if attacker.IsIncapacitated() {
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
	weapon, hasWeapon := attacker.Equipment[slot]
	if !hasWeapon {
		return false // No weapon equipped
	}

	// Calculate damage using centralized function
	_, _, totalDamage := cs.CalculateWeaponDamage(weapon, attacker)

	weaponDef := lookupWeaponConfigByName(weapon.Name)
	if weaponDef == nil {
		return false // Weapon not found, skip attack
	}

	// Ranged dispatch by `range` field (in display tiles). Anything > 3
	// goes through the projectile path. Throwing weapons must declare
	// range >= 4 to count as ranged (otherwise they fall into melee).
	// For ranged: roll crit and apply doubling inside createArrowAttack only.
	acted := false
	summonRolled := false
	if weaponDef.Range > 3 {
		// Masked Huntress Card: boost ranged weapon damage.
		if pct := cs.game.cardRangedDmgPct(); pct != 0 {
			totalDamage = totalDamage * (100 + pct) / 100
		}
		// createArrowAttack returns false at the projectile cap (MaxProjectiles):
		// nothing fired, so no cooldown/action - and no card procs either.
		acted = cs.createArrowAttack(totalDamage, slot, "")
	} else if pct := cs.game.cardSpellProcPct(); pct > 0 && rand.Intn(100) < pct && cs.tryCardFireBoltInstead(attacker) {
		// Pixie Card: the swing becomes a free Fire Bolt cast instead of a melee hit.
		// castResolvedSpell already rolled the Orc Warlord summon check for this
		// action (a real cast counts as one) - don't roll it a second time below.
		acted = true
		summonRolled = true
	} else {
		baseDamage := totalDamage
		// Bronze Cesti: the pair lands every swing twice at half damage - two
		// full strikes with independent crit rolls (steadier than one big hit).
		strikes, strikeDamage := 1, totalDamage
		if weaponDef.DoubleStrike {
			strikes, strikeDamage = 2, (totalDamage+1)/2
			baseDamage = strikeDamage
		}
		var isCrit bool
		for s := 0; s < strikes; s++ {
			dmg := strikeDamage
			isCrit, _ = cs.RollWeaponCriticalChance(weapon, attacker)
			if isCrit {
				dmg *= CritDamageMultiplier
			}
			cs.createMeleeAttack(weapon, dmg, isCrit) // instant swing; a whiff (arc/reach) still spends the cooldown/action, silently
		}
		acted = true
		// Octopus Card: chance to strike again immediately with a fresh swing.
		if pct := cs.game.cardDoubleAttackPct(); pct > 0 && rand.Intn(100) < pct {
			isCrit2, _ := cs.RollWeaponCriticalChance(weapon, attacker)
			dmg2 := baseDamage
			if isCrit2 {
				dmg2 *= CritDamageMultiplier
			}
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
		cs.tryCardHealOnAttack() // Ningyo Card: chance to self-heal on attacking
		if !summonRolled {
			cs.tryCardSummonOnAction() // Orc Warlord Card: chance to summon allies
		}
		// Bandit Card: chance to also loose a short bonus bolt (Accuracy/3 dmg).
		// Always the main hand - a generic card proc, not tied to which hand swung.
		// The card itself stays silent; the bolt's chat identity is the authored
		// label of whichever card granted the bonus.
		if pct := cs.game.cardBonusBoltPct(); pct > 0 && rand.Intn(100) < pct {
			cs.createArrowAttack(attacker.GetEffectiveAccuracy()/3, items.SlotMainHand, cs.game.cardBonusBoltLabel())
		}
	}
	return acted
}

// createArrowAttack creates a projectile arrow attack; reports whether an
// arrow actually left the bow (max-projectiles cap / missing physics -> false,
// so the attempt doesn't cost an action).
// createArrowAttack fires from the weapon in `slot` - SlotMainHand for every
// normal ranged attack and card procs (Bandit's bonus bolt), or whichever hand
// attackSlotFor picked for a Dual Wielding character's primary swing (so a
// bow in the off-hand fires correctly instead of silently reading the main
// hand's weapon). A non-empty label names the projectile in chat instead of
// the weapon (card-proc bolts on melee wielders would otherwise report the
// hunting-bow physics fallback).
func (cs *CombatSystem) createArrowAttack(damage int, slot items.EquipSlot, label string) bool {
	// Find the equipped projectile-weapon's YAML key. Range>3 = ranged
	// (matches the dispatch gate in EquipmentMeleeAttack).
	attacker := cs.game.party.Members[cs.game.selectedChar]
	weapon, hasWeapon := attacker.Equipment[slot]
	bowKey := "hunting_bow"
	var equippedDef *config.WeaponDefinitionConfig
	if hasWeapon {
		equippedDef = lookupWeaponConfigByName(weapon.Name)
		if equippedDef != nil && equippedDef.Range > 3 {
			bowKey = items.GetWeaponKeyByName(weapon.Name)
		}
	}

	// Check max projectiles limit for this weapon
	if equippedDef != nil && equippedDef.MaxProjectiles > 0 {
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
	collisionSize := weaponDef.Physics.GetCollisionSizePixels(tileSize)

	// Determine damage type from weapon
	damageType := monsterPkg.DamageSchoolPhysical
	if equippedDef != nil && equippedDef.DamageType != "" {
		damageType = normalizeDamageTypeStr(equippedDef.DamageType)
	}

	// Volley: a weapon may loose several projectiles per shot (e.g. the blowgun
	// fires 2 darts). They fly STRAIGHT along the aim - an angular fan straddled a
	// target dead ahead and missed - spaced back along the line so they read as a
	// quick stream (one behind the other) and all strike what's in front. Each
	// projectile rolls its own crit.
	volley := 1
	if equippedDef != nil && equippedDef.Volley > 1 {
		volley = equippedDef.Volley
	}
	// Ashigaru Firelock Card: chance to loose one extra arrow in the volley.
	if pct := cs.game.cardVolleyBonusPct(); pct > 0 && rand.Intn(100) < pct {
		volley++
	}
	ang := cs.game.camera.Angle
	dirX, dirY := math.Cos(ang), math.Sin(ang)
	spacing := volleySpacingFrac * float64(tileSize)
	for i := 0; i < volley; i++ {
		back := spacing * float64(i) // trail later darts behind the first
		isCrit, _ := cs.RollWeaponCriticalChance(weapon, attacker)
		dmg := damage
		if isCrit {
			dmg *= CritDamageMultiplier
		}
		arrow := Arrow{
			ID:                 cs.game.GenerateProjectileID("arrow"),
			Attacker:           cs.activeAttacker(),
			X:                  cs.game.camera.X - dirX*back,
			Y:                  cs.game.camera.Y - dirY*back,
			VelX:               dirX * arrowSpeed,
			VelY:               dirY * arrowSpeed,
			Damage:             dmg,
			LifeTime:           arrowLifetime,
			Active:             true,
			BowKey:             bowKey,
			Label:              label,
			DamageType:         damageType,
			Crit:               isCrit,
			DisintegrateChance: weaponDef.DisintegrateChance,
			PierceLeft:         weaponDef.PierceCount,
			Owner:              ProjectileOwnerPlayer,
		}
		cs.game.arrows = append(cs.game.arrows, arrow)
		arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(arrowEntity)
	}
	return true
}

// createMeleeAttack creates an instant melee attack with proper arc-based hit
// detection.
func (cs *CombatSystem) createMeleeAttack(weapon items.Item, totalDamage int, isCrit bool) {
	// Get weapon definition from YAML
	weaponDef := lookupWeaponConfigByName(weapon.Name)
	if weaponDef == nil {
		return // Weapon not found, skip attack
	}

	// Check if weapon has melee configuration
	if weaponDef.Melee == nil {
		fmt.Printf("[WARN] weapon '%s' has no melee configuration in weapons.yaml\n", weapon.Name)
		return
	}

	meleeConfig := weaponDef.Melee
	graphicsConfig := weaponDef.Graphics

	// Create visual slash effect (a per-weapon pixel-particle flourish; see
	// drawMeleeParticles, driven by Kind).
	if graphicsConfig != nil {
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
		slashEffect := SlashEffect{
			ID:             cs.game.GenerateProjectileID("slash"),
			X:              cs.game.camera.X,
			Y:              cs.game.camera.Y,
			Width:          graphicsConfig.SlashWidth,
			Length:         graphicsConfig.SlashLength,
			Color:          graphicsConfig.SlashColor,
			AnimationFrame: 0,
			MaxFrames:      maxFrames,
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
	otx, oty := int(ox/tileSize), int(oy/tileSize)
	cheb := mathutil.IntAbs(int(tx/tileSize) - otx)
	if dy := mathutil.IntAbs(int(ty/tileSize) - oty); dy > cheb {
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
	playerAngle := cs.game.camera.Angle
	tileSize := float64(cs.game.config.GetTileSize())

	weaponDef := lookupWeaponConfigByName(weapon.Name)
	rangeTiles := 1
	if weaponDef != nil && weaponDef.Range > 0 {
		rangeTiles = weaponDef.Range
	}

	// Candidates: alive monsters within tile reach, with their signed angle off
	// the player's facing. Stunned monsters are still valid targets (stun only
	// suppresses their own turn).
	var cands []meleeHitCandidate
	for _, monster := range cs.game.world.Monsters {
		if !monster.IsAlive() {
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
		if !ok {
			continue
		}
		cands = append(cands, meleeHitCandidate{monster, ang})
	}

	hits := 0
	hit := func(m *monsterPkg.Monster3D) {
		cs.ApplyDamageToMonster(m, damage, weapon.Name, isCrit)
		hits++
	}

	if targets, ok := cs.turnBasedPulledMeleeTargets(cands, meleeConfig.ArcType); ok {
		for _, m := range targets {
			hit(m)
		}
		return hits
	}

	arcCands := make([]meleeArcCandidate, len(cands))
	for i, c := range cands {
		cm := c.m
		arcCands[i] = meleeArcCandidate{ang: c.ang, hit: func() { hit(cm) }}
	}
	applyMeleeArc(arcCands, meleeConfig.ArcType)
	return hits
}

func (cs *CombatSystem) turnBasedPulledMeleeTargets(cands []meleeHitCandidate, arcType int) ([]*monsterPkg.Monster3D, bool) {
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

// turnBasedProjectileAssistTarget redirects a player projectile that hit nothing
// onto the front attack slot it was AIMED at, so a shot at a pulled front-diagonal
// SPRITE connects with the real monster. It assists only when the shot was heading
// at the slot (within projectileAssistMaxAngleRad of the camera->slot direction)
// AND the projectile has actually FLOWN out to the slot's drawn position - so the
// arrow/bolt visibly travels instead of striking the instant it spawns. A sideways
// or backward miss, or a shot still in the player's lap, never connects.
func (cs *CombatSystem) turnBasedProjectileAssistTarget(px, py, dirX, dirY float64) *monsterPkg.Monster3D {
	if cs == nil || cs.game == nil || !cs.game.turnBasedMode {
		return nil
	}
	front, left, right, _ := cs.classifyFrontSlots(cs.game.world.Monsters)
	best := front
	if best == nil {
		best = chooseFrontAttackSide(left, right)
	}
	if best == nil {
		return nil
	}
	camX, camY := cs.game.camera.X, cs.game.camera.Y
	if !headingTowardWithin(camX, camY, dirX, dirY, best.vx, best.vy, projectileAssistMaxAngleRad) {
		return nil
	}
	// Forward progress of the projectile along the camera->slot ray must reach the
	// slot (minus a tolerance for the sprite's size / fast bolts overshooting a frame).
	slotX, slotY := best.vx-camX, best.vy-camY
	slotDist := math.Hypot(slotX, slotY)
	if slotDist <= 0 {
		return best.monster
	}
	forward := ((px-camX)*slotX + (py-camY)*slotY) / slotDist
	tol := projectileAssistReachToleranceTiles * float64(cs.game.config.GetTileSize())
	if forward < slotDist-tol {
		return nil // still in flight - let it keep travelling
	}
	return best.monster
}

const projectileAssistMaxAngleRad = 35.0 * math.Pi / 180.0

// projectileAssistReachToleranceTiles: how far short of the pulled slot the
// projectile may connect (sprite radius + per-frame overshoot slack).
const projectileAssistReachToleranceTiles = 0.5

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
// ok=false when no front slot applies (not TB, ranged diagonal, not adjacent,
// off-axis, behind, or the pulled spot has no line of sight).
func (cs *CombatSystem) pulledFrontSlot(mon *monsterPkg.Monster3D) (side int, x, y float64, pulled, ok bool) {
	if cs == nil || cs.game == nil || mon == nil || !cs.game.turnBasedMode || !mon.IsAlive() {
		return 0, 0, 0, false, false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	if tileSize <= 0 {
		return 0, 0, 0, false, false
	}
	// Logical (un-shaken) camera: the pull decision, its gates, and its LOS must
	// not flip with the per-frame +/- shake jitter (see logicalCameraXY).
	camX, camY := cs.logicalCameraXY()
	ptx, pty := int(camX/tileSize), int(camY/tileSize)
	mtx, mty := int(mon.X/tileSize), int(mon.Y/tileSize)
	mdx, mdy := mtx-ptx, mty-pty
	fx, fy := cardinalForwardFromAngle(cs.game.camera.Angle)

	// Exactly one tile dead-ahead: a real front target, drawn/attacked where it is.
	if mdx == fx && mdy == fy {
		losOK := cs.game.collisionSystem == nil ||
			cs.game.collisionSystem.CheckLineOfSight(mon.X, mon.Y, camX, camY)
		return 0, mon.X, mon.Y, false, losOK
	}
	// Front DIAGONAL melee neighbour: pull it ~1 tile ahead, slightly to its side.
	if mdx == 0 || mdy == 0 || mon.HasRangedAttack() || !cs.monsterMeleeAdjacentToParty(mon) {
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
func (cs *CombatSystem) monsterVisualPos(mon *monsterPkg.Monster3D) (float64, float64) {
	if mon == nil {
		return 0, 0
	}
	x, y := mon.X, mon.Y
	if _, px, py, pulled, ok := cs.pulledFrontSlot(mon); ok && pulled {
		x, y = px, py
	}
	if cs.game != nil && cs.game.config != nil {
		ox, oy := monsterStackFanOffset(mon, float64(cs.game.config.GetTileSize()))
		x, y = x+ox, y+oy
	}
	return x, y
}

// spawnMonsterHitBurst bursts generic impact particles where a monster is DRAWN
// (banded-stack / pulled-slot aware, via monsterVisualPos). Shared by AoE splash
// and party-nova victims so both anchor on the sprite, not the raw tile.
func (cs *CombatSystem) spawnMonsterHitBurst(m *monsterPkg.Monster3D, element string) {
	x, y := cs.monsterVisualPos(m)
	cs.game.CreateSpellHitEffect(x, y, element, SpellParticleCount, SpellParticleSize)
}

// ApplyDamageToMonster applies damage to a monster and handles combat messages
// This is for melee attacks - AC applies only to physical damage as reduction
// applyTrueDamageThroughDodge deals flat weapon-mastery TRUE damage that landed
// despite the target's Perfect Dodge, with the usual hit bookkeeping (tint, pack
// aggro, death/XP). Caller is responsible for any projectile cleanup.
func (cs *CombatSystem) applyTrueDamageThroughDodge(monster *monsterPkg.Monster3D, trueDmg int, damageType monsterPkg.DamageType, attackerName string) {
	actual := monster.TakeDamage(trueDmg, damageType)
	cs.markMonsterHit(monster)
	if !monster.IsAlive() {
		xpAwarded := cs.finishMonsterKill(monster)
		cs.game.AddCombatMessage(fmt.Sprintf("%s's mastery pierces %s's dodge for %d true damage and kills it!", attackerName, monster.Name, actual))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s dodges, but %s's mastery lands %d true damage! (HP: %d/%d)", monster.Name, attackerName, actual, monster.HitPoints, monster.MaxHitPoints))
	}
}

func (cs *CombatSystem) ApplyDamageToMonster(monster *monsterPkg.Monster3D, damage int, weaponName string, isCrit bool) {
	if cs.absorbIfSealed(monster) {
		return
	}
	weaponDef := lookupWeaponConfigByName(weaponName)
	damageTypeStr := weaponDamageTypeStr(weaponDef)
	damageType := convertToMonsterDamageType(damageTypeStr)

	// Party buffs boost melee exactly like projectiles, filtered by damage type
	// (Heroism applies only to physical; Hour of Power applies to all).
	if damage > 0 {
		damage += cs.game.combatBuffOutBonusForDamageType(damageTypeStr)
	}
	attacker := cs.activeAttacker() // melee resolves the same frame it swings
	trueDmg, ignoreDodge := cs.weaponMasteryStrike(attacker, weaponDef)
	attackerName := "The party"
	if attacker != nil {
		attackerName = attacker.Name
	}

	// Check monster perfect dodge. A Grandmaster ignores it entirely; otherwise
	// the normal hit is avoided but weapon-mastery TRUE damage still lands.
	if monster.PerfectDodge > 0 && !ignoreDodge && rand.Intn(100) < monster.PerfectDodge {
		// A targeted party attack is enough to end Charm, even when the target
		// avoids the damage. Otherwise a 100%-dodge charmed mob could remain
		// pacified forever while absorbing melee swings.
		cs.breakPacifyOnHit(monster)
		if trueDmg > 0 {
			cs.applyTrueDamageThroughDodge(monster, trueDmg, damageType, attackerName)
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges %s's attack!", monster.Name, attackerName))
		}
		return
	}

	// Alien Card: chance any melee hit instantly disintegrates the target (same
	// immunity gate as weapon/spell Disintegrate: undead/dragon/invulnerable boss).
	if pct := cs.game.cardDisintegratePct(); pct > 0 && !monsterImmuneToDisintegrate(monster) && rand.Float64() < float64(pct)/100 {
		monster.HitPoints = 0
		cs.markMonsterHit(monster)
		xpAwarded := cs.finishMonsterKill(monster)
		cs.game.AddCombatMessage(fmt.Sprintf("%s disintegrates %s!", attackerName, monster.Name))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
		return
	}

	// Phys-to-element conversion cards (Archmage=fire, Hexer=dark, Isis=light)
	// divert a share of PHYSICAL damage regardless of source - melee, ranged and
	// traps all go through splitPhysConversions; each share is mitigated as its
	// own element (elemental armor cap, then that element's resistance).
	var convShares []physConvShare
	if damageTypeStr == monsterPkg.DamageSchoolPhysical {
		damage, convShares = cs.game.splitPhysConversions(damage)
	}

	reducedDamage := damage
	// Forest Orc Card: chance to ignore the target's armor entirely.
	if pct := cs.game.cardArmorPiercePct(); pct <= 0 || rand.Intn(100) >= pct {
		reducedDamage = applyMonsterArmor(damage, damageTypeStr, effectiveMonsterArmor(monster, weaponDef), false)
	}
	if mult := cs.weaponBonusMultiplier(weaponDef, monster); mult != 1.0 {
		reducedDamage = int(math.Round(float64(reducedDamage) * mult))
		if reducedDamage < 1 {
			reducedDamage = 1
		}
	}
	// Gladius: the pit blade finishes what a stun begins.
	if mult := weaponStunnedBonusMultiplier(weaponDef, monster); mult != 1.0 {
		reducedDamage = int(math.Round(float64(reducedDamage) * mult))
	}
	// Card bonus_vs (e.g. Elf Archer vs dragons, Skeleton vs formless bosses).
	if mult := cs.game.cardBonusVsMultiplier(monster); mult != 1.0 {
		reducedDamage = int(math.Round(float64(reducedDamage) * mult))
		if reducedDamage < 1 {
			reducedDamage = 1
		}
	}
	reducedDamage += trueDmg                    // weapon-mastery true damage bypasses armor
	reducedDamage += cs.game.cardMeleeTrueDmg() // Samurai Card: flat true melee damage
	if pct := cs.game.cardMeleeDmgPct(); pct != 0 {
		// Masked Serpent Dancer Card: +N% melee weapon damage.
		reducedDamage = reducedDamage * (100 + pct) / 100
	}

	// Apply damage with resistances and distance-aware AI response
	finalDamage := monster.TakeDamage(reducedDamage, damageType)
	finalDamage += cs.applyPhysConversionShares(monster, convShares, false)
	cs.markMonsterHit(monster)
	cs.trySleightOfHand(attacker, monster)
	// Impact feedback: spark burst + light flash at the monster, plus a small
	// damage-scaled view kick (well under a fireball's). The monster stays put
	// and the HitTintFrames timer also drives an in-place sprite shake (see
	// renderer) - no positional knockback. Anchor on the VISUAL position so a
	// pulled front-diagonal monster's sparks land where it's drawn, not its tile.
	vx, vy := cs.monsterVisualPos(monster)
	cs.game.spawnImpactSparks(vx, vy)
	cs.game.addScreenShake(0.05*float64(finalDamage), 2.2)
	if monster.IsAlive() {
		cs.tryApplyWeaponHitRiders(monster, weaponDef)
		cs.tryCardPoisonProc(monster)
	}
	if weaponDef != nil && weaponDef.AoeRadiusTiles > 0 {
		// Splash mirrors the primary's phys-conversion split: the physical
		// remainder AND every converted share reach nearby foes too (previously
		// the already-reduced `damage` was used, so converted shares were
		// silently dropped and splash dealt less than the primary hit).
		cs.applyAoeSplash(monster, damage, damageTypeStr, damageType, weaponName, weaponDef.AoeRadiusTiles, 0)
		cs.splashPhysConversionShares(monster, convShares, weaponName, weaponDef.AoeRadiusTiles)
	}

	// Add combat message
	if monster.IsAlive() {
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hits %s for %d damage! (HP: %d/%d)",
			prefix, cs.game.party.Members[cs.game.selectedChar].Name, monster.Name, finalDamage,
			monster.HitPoints, monster.MaxHitPoints))
	} else {
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hits %s for %d damage and kills it!",
			prefix, cs.game.party.Members[cs.game.selectedChar].Name, monster.Name, finalDamage))

		xpAwarded := cs.finishMonsterKill(monster)

		// Add experience/gold award message
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
	}
}

// engageTurnBasedSameKindPackOnPartyHit is the one explicit exception to the
// shared sight-agro rule. Only in TB, a party-caused hit may alert nearby
// same-key monsters, but each neighbour must still have direct LoS to the
// party. It is intentionally not an RT mechanic and it never behaves like an
// alarm through walls.
func (cs *CombatSystem) engageTurnBasedSameKindPackOnPartyHit(hit *monsterPkg.Monster3D) {
	if cs == nil || cs.game == nil || cs.game.world == nil || !cs.game.turnBasedMode || hit == nil {
		return
	}

	tileSize := float64(cs.game.config.GetTileSize())
	radius := tileSize * TurnBasedPackAggroRadiusTiles
	hitKey := hit.Key // pack by exact type (key), not display Name

	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || m.Key != hitKey ||
			Distance(hit.X, hit.Y, m.X, m.Y) > radius ||
			!m.CanStartPlayerAggro() ||
			!m.HasLineOfSightToPlayer(cs.game.collisionSystem, cs.game.camera.X, cs.game.camera.Y) {
			continue
		}
		m.BeginPlayerEngagement()
	}
}

// CastSelectedSpell casts the currently selected spell from the spellbook.
// It returns the spell ID only when the cast actually spent SP and went off,
// so callers can consume a TB action and retain its RT cooldown on a later
// mode switch.
func (cs *CombatSystem) CastSelectedSpell() (bool, spells.SpellID) {
	currentChar := cs.game.party.Members[cs.game.selectedChar]

	// Prevent casting while down; also avoids utility healing from acting as a revive.
	if currentChar.IsIncapacitated() {
		return false, ""
	}
	// SAME filtered list the spellbook UI numbers (schools with spells only) -
	// indexing the full school list desynced selection when a school was empty.
	schools := spellbookSchoolsWithSpells(currentChar)

	if cs.game.selectedSchool >= len(schools) {
		return false, ""
	}

	selectedSchool := schools[cs.game.selectedSchool]
	availableSpells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell < 0 || cs.game.selectedSpell >= len(availableSpells) {
		return false, ""
	}

	selectedSpellID := availableSpells[cs.game.selectedSpell]
	selectedSpellDef, err := spells.GetSpellDefinitionByID(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return false, ""
	}

	if !cs.castResolvedSpell(selectedSpellID, selectedSpellDef, currentChar,
		cs.effectiveSpellCost(currentChar, selectedSpellDef.SpellPointsCost), true, true) {
		return false, ""
	}
	return true, selectedSpellID
}

// castResolvedSpell is the ONE cast path behind both the equipped quick-slot
// and the spellbook: SP gate, special effects, projectile spawn and utility
// application all live here so the two entry points cannot drift. announce
// controls the "Casting X!" launch message (quick-cast stays quiet).
// countsAsAction gates the Orc Warlord summon roll - false for a cast that
// rides along on an action which already rolled it elsewhere (Spiritual
// Training's free proc on a melee/ranged hit that already summon-rolled at
// swing time; rolling again here would double the odds for one action).
func (cs *CombatSystem) castResolvedSpell(spellID spells.SpellID, spellDef spells.SpellDefinition, caster *character.MMCharacter, spellCost int, announce bool, countsAsAction bool) bool {
	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			caster.Name, caster.SpellPoints, spellCost))
		return false
	}
	// Flying inside solid terrain: no casting from within the stone (the melee
	// gate's twin - see partyEntombed). Checked before the SP spend.
	if cs.partyEntombed() {
		return false
	}
	// Outdoor-only spells (Fly) need an open day/night sky - never in dungeons.
	// Checked before the SP spend so a refused cast costs nothing.
	if spellDef.OutdoorOnly && !cs.game.currentMapHasOpenSky() {
		cs.game.AddCombatMessage(fmt.Sprintf("%s needs the open sky.", spellDef.Name))
		return false
	}
	// Town Portal with no known destination is a no-op - refuse it BEFORE the SP spend
	// and the action-proc roll (Orc Warlord summon), or a failed cast would still
	// pay SP and free-summon before the deep refund runs.
	if spellDef.TownPortal && len(cs.game.sortedTownPortalDestinations()) == 0 {
		cs.game.AddCombatMessage("The portal finds no destination it knows - visit a town or tavern first.")
		return false
	}
	caster.SpellPoints -= spellCost
	spAfterPay := caster.SpellPoints

	// Data-driven effect spells (AoE stun, party buffs, resurrect) - no
	// projectile, no direct damage.
	if cs.tryCastSpecialEffect(spellID, spellDef, caster) {
		// Orc Warlord Card: only a REAL cast is a party action. Empty
		// Resurrect/Awaken/Raise Dead refund the SP and still return handled, so
		// gate the summon roll on the SP staying spent - no free summons on a
		// no-op cast.
		if caster.SpellPoints <= spAfterPay {
			if countsAsAction {
				cs.tryCardSummonOnAction()
			}
			cs.playSpellBuffFx(spellID) // same no-op gate: refunded cast = no animation
		}
		return true
	}

	// A projectile or utility cast that reaches here is a real party action
	// unless it is a free proc riding on an action that already rolled cards.
	if countsAsAction {
		cs.tryCardSummonOnAction()
	}

	castingSystem := spells.NewCastingSystem(cs.game.config)

	// Mortar spells (Stone Blossom): the arc ignores everything in flight and
	// detonates at a fixed distance - never the straight-line projectile path.
	if spellDef.IsProjectile && spellDef.MortarRangeTiles > 0 {
		return cs.castMortarSpell(spellID, spellDef, caster, announce)
	}

	if spellDef.IsProjectile {
		projectile, err := castingSystem.CreateProjectile(spellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return false
		}
		// CreateProjectile carries physics only; damage is authored HERE
		// (effective stats + mastery), once.
		_, _, totalDamage := cs.CalculateSpellDamage(spellID, caster)
		if spellDef.DealsNoDamage {
			totalDamage = 0 // Disintegrate: only the instakill roll matters
		}

		// Resolve spell config before spawning anything so a config error
		// can't leave a projectile without a collision entity.
		spellConfig, err := cs.game.config.GetSpellConfig(string(spellID))
		if err != nil {
			cs.game.AddCombatMessage("Spell config error: " + err.Error())
			return false
		}
		disintegrateChance := 0.0
		if spellDefConfig, exists := config.GetSpellDefinition(string(spellID)); exists && spellDefConfig != nil {
			disintegrateChance = spellDefConfig.DisintegrateChance
		}

		// Luck-based spell crit (no-damage spells never crit - see helper).
		var isCrit bool
		projectile.Damage, isCrit = cs.rollSpellCritDamage(spellID, caster, totalDamage)

		magicProjectile := MagicProjectile{
			ID:                 cs.game.GenerateProjectileID(string(spellID)),
			Attacker:           cs.activeAttacker(),
			X:                  projectile.X,
			Y:                  projectile.Y,
			VelX:               projectile.VelX,
			VelY:               projectile.VelY,
			Damage:             projectile.Damage,
			LifeTime:           projectile.LifeTime,
			Active:             projectile.Active,
			SpellType:          string(spellID),
			Size:               projectile.Size,
			Crit:               isCrit,
			DisintegrateChance: disintegrateChance,
			Owner:              ProjectileOwnerPlayer,
		}
		cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

		tileSize := cs.game.config.GetTileSize()
		collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
		projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(projectileEntity)

		if announce {
			cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", spellDef.Name))
		}
		return true
	}

	if spellDef.IsUtility {
		// ApplyUtilitySpell resolves flags + message only; every NUMBER
		// (heal total, duration, stat bonuses) is computed here, once.
		result, err := castingSystem.ApplyUtilitySpell(spellID)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return false
		}
		if !result.Success {
			return false
		}

		duration := 0
		if spellDef.Duration > 0 {
			duration = cs.CalculateSpellDurationFrames(spellID, caster)
		}

		// Stat-buff spells (Bless, ...) announce the ACTUAL granted bonus -
		// mastery-scaled and caster-dependent - so the chat can never drift from
		// the number really applied (e.g. Bless is +5 base, +10 only at GM).
		isStatBuff := spellDef.StatBonus > 0 || len(spellDef.StatBonuses) > 0
		var statBuff character.StatBonuses
		msg := result.Message
		if isStatBuff {
			statBuff = cs.spellStatBuffBonuses(spellID, caster)
			if suffix := statBuff.Summary(); suffix != "" {
				msg = fmt.Sprintf("%s (%s)", strings.TrimSpace(result.Message), suffix)
			}
		}
		cs.game.AddCombatMessage(msg)

		// Apply healing
		if spellDef.HealAmount > 0 {
			_, _, totalHeal := cs.CalculateSpellHealing(spellID, caster)
			if spellDef.HealParty {
				// Mass Heal: restore every party member.
				cs.healWholeParty(totalHeal)
			} else {
				// Fallback self-heal (mouse-targeted heals go via CastEquippedHealOnTarget).
				cs.healMember(cs.game.selectedChar, totalHeal)
			}
		}

		// Apply vision effects - the RADIUS comes from spells.yaml
		// (vision_radius_tiles), not a hardcoded constant.
		if result.VisionRadiusTiles > 0 {
			switch string(spellID) {
			case "torch_light":
				cs.game.torchLightActive = true
				cs.game.torchLightDuration = duration
				cs.game.torchLightRadius = result.VisionRadiusTiles
			case "wizard_eye":
				cs.game.wizardEyeActive = true
				cs.game.wizardEyeDuration = duration
				cs.game.wizardEyeRadiusTiles = result.VisionRadiusTiles
			}
		}

		// Apply movement effects
		if result.WaterWalk {
			cs.game.walkOnWaterActive = true
			cs.game.walkOnWaterDuration = duration
		}
		if spellDef.Fly {
			cs.game.flyActive = true
			cs.game.flyDuration = duration
		}
		// Town Portal: open the visited-destination picker; the teleport happens
		// on confirm. The no-destination case was already refused before the SP spend.
		if spellDef.TownPortal {
			cs.game.townPortalPickerOpen = true
		}
		if result.WaterBreathing {
			cs.game.waterBreathingActive = true
			cs.game.waterBreathingDuration = duration
			// Store current position and map for return teleportation when effect expires
			cs.game.underwaterReturnX = cs.game.camera.X
			cs.game.underwaterReturnY = cs.game.camera.Y
			if world.GlobalWorldManager != nil {
				cs.game.underwaterReturnMap = world.GlobalWorldManager.CurrentMapKey
			}
		}

		// Stat-buff spells, by DATA (stat_bonus / stat_bonuses), not by ID -
		// any spell authored with a bonus block applies it; different buff
		// spells stack, recasting one refreshes it. Reuses the block already
		// resolved for the announcement above (this cast only, not the aggregate).
		if isStatBuff {
			cs.applyStatBuffSpell(spellID, duration, statBuff)
		}

		cs.game.setUtilityStatus(spellID, duration)
		cs.playSpellBuffFx(spellID)
		return true
	}

	return false
}

// playSpellBuffFx plays the spell's buff overlay animation if it defines one
// (buff_fx_sprite in spells.yaml). Called from every successful cast branch -
// special-effect AND utility - so any spell can be given the animation by data.
func (cs *CombatSystem) playSpellBuffFx(spellID spells.SpellID) {
	if cfgDef, ok := config.GetSpellDefinition(string(spellID)); ok && cfgDef != nil {
		cs.game.playBuffFx(cfgDef.BuffFxSprite)
	}
}

// EquipSelectedSpell equips the selected spell as an item in a battle or utility slot
func (cs *CombatSystem) EquipSelectedSpell() {
	currentChar := cs.game.party.Members[cs.game.selectedChar]
	// Same filtered list the spellbook UI numbers - see CastSelectedSpell.
	schools := spellbookSchoolsWithSpells(currentChar)

	if cs.game.selectedSchool >= len(schools) {
		return
	}

	selectedSchool := schools[cs.game.selectedSchool]
	availableSpells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell < 0 || cs.game.selectedSpell >= len(availableSpells) {
		return
	}

	selectedSpellID := availableSpells[cs.game.selectedSpell]

	// Use centralized spell item creation - no fallbacks, no hardcoded mappings
	item, err := spells.CreateSpellItem(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage("Failed to create spell item: " + err.Error())
		return
	}

	// Equip the spell item in the unified spell slot
	currentChar.Equipment[items.SlotSpell] = item
}

// HandleMonsterInteractions handles combat between monsters and the player
func (cs *CombatSystem) HandleMonsterInteractions() {
	// Check for monsters that are very close and attack the player
	for _, monster := range cs.game.world.Monsters {
		if !monster.IsAlive() {
			continue
		}
		// Movement AI and TB already hold scripted inactive encounter pieces.
		// RT crossfire is a separate action path, so it must enforce the same gate
		// before a precomputed bound-ally foe can trigger an attack.
		if monster.IsInertSetPiece() {
			continue
		}
		// Stunned monsters take no action (the TB path already skips them; the
		// real-time path must too, or a stun frozen at StateTimer==1 would let a
		// monster pounce/strike every frame for the whole stun). Update() decrements
		// the stun counter; here we just suppress the action.
		if monster.StunFramesRemaining > 0 {
			continue
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

		// Pacified (Charm): stands and does nothing, never attacks the party.
		if monster.Pacified {
			continue
		}
		// Bound (Bind Undead): hunts the nearest enemy monster using its normal
		// per-monster attack cooldown, never the party.
		if monster.Bound {
			if monster.AttackCDFrames == 0 && cs.boundAttackNearest(monster) {
				monster.AttackCDFrames = monster.AttackCooldownFrames()
			}
			continue
		}
		// An evasive quest boss is not dormant (it must still blink away from
		// the PARTY), but it is just as inactive toward summons. This stays after
		// the shared stun/charm/bind gates, preserving their suppression of boss
		// actions, while still running before the crossfire branch below.
		if cs.bossEvasive(monster) {
			ready := monster.BossCD == 0
			if monster.BossCD > 0 {
				monster.BossCD--
			}
			cs.updateBoss(monster, ready, false)
			continue
		}

		// Lured at a bound undead instead of the party: attack it on the monster's
		// normal individual cooldown whenever within reach (ranged mobs loose a visible bolt; melee
		// strike directly), independent of the engagement state machine - so a mob
		// jittering at the edge of melee range still connects. Then skip party logic.
		if foe := monster.AIFoe; foe != nil && foe.IsAlive() {
			if monster.IsChampion() {
				if cs.monsterCanAttackMonster(monster, foe) && cs.game.tryClaimMonsterAttackPost(monster) {
					monster.State = monsterPkg.StateAttacking
					cs.championRTCrossfireStrike(monster, foe)
				} else if monsterInAttackTransit(monster) {
					cs.game.releaseMonsterAttackPost(monster)
				}
				continue
			}
			if monster.AttackCDFrames == 0 && cs.monsterCanAttackMonster(monster, foe) && cs.game.tryClaimMonsterAttackPost(monster) {
				monster.State = monsterPkg.StateAttacking
				monster.AttackAnimFrames = MonsterAttackAnimFrames
				if monster.HasRangedAttack() {
					cs.spawnMonsterRangedAttackAtMonster(monster, foe, ProjectileOwnerMonsterAtBound)
				} else {
					cs.monsterStrikeMonster(monster, foe)
				}
				monster.AttackCDFrames = monster.AttackCooldownFrames()
			}
			continue
		}

		attackRange := monster.GetAttackRangePixels()

		dist := Distance(cs.game.camera.X, cs.game.camera.Y, monster.X, monster.Y)

		// Boss behaviour (Golden Thief Bug): evade-until-quest blink, low-HP blink,
		// Inferno casts. Returns true when it has handled the action this tick.
		if cs.isBoss(monster) {
			ready := monster.BossCD == 0
			if monster.BossCD > 0 {
				monster.BossCD--
			}
			// Same reach gate as the normal attack below: a melee boss on an
			// adjacent tile is in real contact at >1 tile of pixel distance.
			attackTick := monster.State == monsterPkg.StateAttacking && monster.StateTimer == 1 &&
				cs.monsterCanAttackParty(monster, dist, attackRange)
			if cs.updateBoss(monster, ready, attackTick) {
				continue
			}
		}

		// Pounce (real-time): from within pounce range but beyond melee, leap
		// to melee contact and strike immediately, then go on cooldown.
		if monster.CanPounce() {
			if monster.PounceCDFrames > 0 {
				monster.PounceCDFrames--
			}
			if monster.PounceCDFrames == 0 && dist > attackRange && dist <= monster.PounceRangePixels &&
				(!monster.PassiveUntilAttacked || monster.WasAttacked || monster.HatesActiveTrait()) {
				if cs.executePounce(monster, cs.game.camera.X, cs.game.camera.Y) {
					cs.game.AddCombatMessage(fmt.Sprintf("%s pounces at the party!", monster.Name))
					cs.applyMonsterMeleeDamage(monster)
					tps := cs.game.config.GetTPS()
					if tps <= 0 {
						tps = 60
					}
					monster.PounceCDFrames = int(monster.PounceCooldownSeconds * float64(tps))
					continue
				}
			}
		}

		// If monster is in attacking state and within attack range, perform attack.
		// Inclusive (<=) so a mob sitting exactly one tile away (e.g. a puma that
		// just pounced onto an adjacent tile) still lands its hit. Melee monsters
		// also count diagonally-adjacent tiles as point-blank so they can surround
		// the party instead of queueing only on N/S/E/W.
		if monster.State == monsterPkg.StateAttacking && cs.monsterCanAttackParty(monster, dist, attackRange) {
			// Melee champions run two independent hand streams (party dual-wield
			// parity); everyone else fires on the single attack tick below.
			if monster.IsChampion() && !monster.HasRangedAttack() &&
				cs.championRTDualStrike(monster, monster.StateTimer == 1) {
				continue
			}
			// Fire on the first frame of the attacking state, but only if the
			// persistent attack cooldown has elapsed - re-entering the attacking
			// state (e.g. after chasing a kiting player back into range) no longer
			// grants a free hit. On a hit, arm the cooldown for the next interval.
			if monster.StateTimer == 1 && monster.AttackCDFrames == 0 {
				monster.AttackCDFrames = monster.AttackCooldownFrames()
				monster.AttackAnimFrames = MonsterAttackAnimFrames
				if monster.HasRangedAttack() {
					cs.spawnMonsterRangedAttack(monster)
				} else {
					cs.applyMonsterMeleeDamage(monster)
				}
			}
		}
	}
}

// executePounce leaps a pouncing monster onto the nearest walkable tile
// adjacent to the player - never inside the player's own tile (where the sprite
// would vanish). Diagonal-adjacent tiles are valid melee contact. Callers must
// only resolve the strike when it returns true. Shared by RT and TB pounce hooks.
func (cs *CombatSystem) executePounce(m *monsterPkg.Monster3D, playerX, playerY float64) bool {
	tileSize := float64(cs.game.config.GetTileSize())
	ptx, pty := int(playerX/tileSize), int(playerY/tileSize)

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
		if !cs.game.collisionSystem.CanMoveToWithHabitat(m.ID, cx, cy, m.HabitatPrefs, m.Flying) {
			continue
		}
		if d := (cx-m.X)*(cx-m.X) + (cy-m.Y)*(cy-m.Y); d < bestD {
			bestD, bestX, bestY, found = d, cx, cy, true
		}
	}
	if !found {
		return false // no free adjacent tile - can't pounce
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
	cs.game.refreshMonsterCollisionSolidity(m)
	m.AttackAnimFrames = MonsterAttackAnimFrames // brief leap/strike animation
	return true
}

func (cs *CombatSystem) monsterCanAttackParty(monster *monsterPkg.Monster3D, dist, attackRange float64) bool {
	if monster == nil {
		return false
	}
	// A monster can physically cross another attacker's tile, but
	// while doing so it is only transit. Keep this gate here as well as in the
	// state/post reconciliation so every RT attack path rejects it even if
	// a caller reaches combat before the next AI state transition.
	if monsterInAttackTransit(monster) {
		return false
	}
	if dist <= attackRange {
		if monster.HasRangedAttack() {
			return cs.game.collisionSystem == nil || cs.game.collisionSystem.CheckLineOfSight(monster.X, monster.Y, cs.game.camera.X, cs.game.camera.Y)
		}
		return true
	}
	if monster.HasRangedAttack() {
		return false
	}
	return cs.monsterMeleeAdjacentToParty(monster)
}

func (cs *CombatSystem) monsterMeleeAdjacentToParty(monster *monsterPkg.Monster3D) bool {
	// Logical (un-shaken) camera so this gate is shake-invariant on the pull path
	// (offset is 0 on the AI path). Otherwise it could flip near a wall - the same
	// blink the pull fix addresses. See logicalCameraXY.
	camX, camY := cs.logicalCameraXY()
	return cs.monsterMeleeAdjacentToPoint(monster, camX, camY)
}

// monsterMeleeAdjacentToPoint is the one tile-adjacency/LoS rule for a melee
// monster attacking any point target (party or another monster).
func (cs *CombatSystem) monsterMeleeAdjacentToPoint(monster *monsterPkg.Monster3D, targetX, targetY float64) bool {
	if monster == nil || cs == nil || cs.game == nil {
		return false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	if tileSize <= 0 {
		return false
	}
	mtx, mty := int(monster.X/tileSize), int(monster.Y/tileSize)
	ptx, pty := int(targetX/tileSize), int(targetY/tileSize)
	dx, dy := mathutil.IntAbs(mtx-ptx), mathutil.IntAbs(mty-pty)
	if dx == 0 && dy == 0 {
		return false
	}
	if dx > 1 || dy > 1 {
		return false
	}
	return cs.game.collisionSystem == nil || cs.game.collisionSystem.CheckLineOfSight(monster.X, monster.Y, targetX, targetY)
}

func (cs *CombatSystem) applyMonsterMeleeDamage(monster *monsterPkg.Monster3D) {
	if cs.tryMonsterSpecialAbility(monster) {
		return
	}
	if cs.tryMonsterAoeAttack(monster) {
		return
	}

	// Champion melee resolves through the character pipeline with the weapon's
	// arc width (and the once-per-swing AoE rule) instead of a single target.
	if monster.IsChampion() && cs.championAlternatingStrike(monster) {
		return
	}

	// Melee hits a random living party member (both RT and TB) through the shared
	// monster->character choke point (dodge, KO, blink, poison rider). Armour-
	// piercing attackers (Golden Thief Bug) bypass the party's armor class;
	// resistances and buff mitigation still apply.
	currentChar := cs.randomLivingMember()
	if currentChar == nil {
		return
	}
	cs.monsterHitCharacter(monster, currentChar, monster.Name, cs.monsterAttackDamage(monster), monsterPkg.DamageSchoolPhysical, monster.IgnoresArmor, 0, true)
	// No knockback: monster attacks are already gated to once per attacking state
	// (StateTimer==1) plus pounce cooldowns, so the old anti-spam pushback is moot.
}

// monsterHitCharacter is the one choke point for "a monster damages a
// character" (melee, piercing shot, projectile): perfect-dodge roll, optional
// disintegrate, mitigation + HP application, KO, and the hit/blink feedback.
// The on-hit poison rider fires only when monster != nil - so melee and piercing
// poison, but a sourceless PROJECTILE does not: monster projectiles carry only a
// SourceName, not a back-reference to the attacker, so a ranged poisonous monster
// (e.g. masked_huntress) can't poison via its projectile. Wiring a source-monster
// ref onto MagicProjectile/Arrow would close that gap if ranged poison is wanted.
// disintegrateChance > 0 enables the eradicate roll (projectiles only).
// monsterHitCharacter resolves one monster-on-party hit. melee=true marks a
// melee blow (vs an arrow / magic bolt / breath), which gates weapon-riposte
// thorns (Parrying Dagger answers a BLOW, not a projectile - see its tooltip).
func (cs *CombatSystem) monsterHitCharacter(monster *monsterPkg.Monster3D, target *character.MMCharacter, sourceName string, damage int, damageType string, ignoresArmor bool, disintegrateChance float64, melee bool) {
	if target == nil {
		return
	}
	if sourceName == "" {
		sourceName = "Monster"
	}
	targetIndex := cs.findCharacterIndex(target)

	// Perfect Dodge: luck/5% to avoid the hit. The dodge evades the mitigable part,
	// but a monster's TRUE damage lands anyway (mirrors party weapon-mastery true,
	// which pierces a monster's dodge) - no riders, just the unmitigable chunk.
	// IgnoresDodge (champion GM weapon mastery) pierces the dodge entirely -
	// the same rule a GM party member enjoys against monsters.
	if dodged, _ := cs.RollPerfectDodge(target); dodged && (monster == nil || !monster.IgnoresDodge) {
		trueDmg := 0
		if monster != nil {
			trueDmg = monster.TrueDamage
		}
		if trueDmg <= 0 {
			cs.game.AddCombatMessage(fmt.Sprintf("Perfect Dodge! %s evades %s's attack!", target.Name, sourceName))
			return
		}
		target.HitPoints -= trueDmg
		if target.HitPoints < 0 {
			target.HitPoints = 0
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s dodges %s but still takes %d! (HP: %d/%d)",
			target.Name, sourceName, trueDmg, target.HitPoints, target.MaxHitPoints))
		if target.HitPoints == 0 {
			cs.knockOut(target)
		}
		cs.game.TriggerDamageBlink(targetIndex)
		return
	}

	if disintegrateChance > 0 && rand.Float64() < disintegrateChance {
		target.HitPoints = 0
		target.Conditions = []character.Condition{character.ConditionEradicated}
		cs.game.AddCombatMessage(fmt.Sprintf("%s is eradicated by %s!", target.Name, sourceName))
		cs.game.TriggerDamageBlink(targetIndex)
		return
	}

	finalDamage := cs.mitigateCharacterDamage(damage, damageType, target, ignoresArmor)
	if monster != nil && monster.TrueDamage > 0 {
		finalDamage += monster.TrueDamage // bypasses all mitigation; folded into the total, no separate line
	}
	target.HitPoints -= finalDamage
	if target.HitPoints < 0 {
		target.HitPoints = 0
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s hits %s for %d damage! (HP: %d/%d)",
		sourceName, target.Name, finalDamage, target.HitPoints, target.MaxHitPoints))
	if target.HitPoints == 0 {
		cs.knockOut(target)
	}
	cs.game.TriggerDamageBlink(targetIndex)

	if monster != nil {
		cs.tryApplyMonsterPoison(monster, target)
		cs.tryApplyMonsterIgnite(monster, target)
		cs.tryApplyMonsterStun(monster, target)
		cs.tryApplyMonsterDispel(monster, target)
		// Vengeful Ningyo Card + Parrying Dagger riposte: reflect a share of the
		// hit back at its source. The card's mystical thorns answer any hit; the
		// weapon riposte answers only a melee BLOW (arrows/bolts/breath don't).
		pct := cs.game.cardThornsPct()
		if melee {
			pct += weaponThornsPct(target)
		}
		if pct > 0 && finalDamage > 0 && monster.IsAlive() {
			if reflected := finalDamage * pct / 100; reflected > 0 {
				dealt := monster.TakeDamage(reflected, monsterPkg.DamagePhysical)
				if !monster.IsAlive() {
					xpAwarded := cs.finishMonsterKill(monster)
					cs.game.AddCombatMessage(fmt.Sprintf("%s's reflected wrath destroys %s!", target.Name, monster.Name))
					cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
				} else if dealt > 0 {
					cs.game.AddCombatMessage(fmt.Sprintf("%s takes %d reflected damage!", monster.Name, dealt))
				}
			}
		}
	}
	return
}

// weaponThornsPct sums thorns_pct across the character's equipped weapons
// (Parrying Dagger riposte - either hand counts).
func weaponThornsPct(target *character.MMCharacter) int {
	if target == nil {
		return 0
	}
	total := 0
	for _, slot := range []items.EquipSlot{items.SlotMainHand, items.SlotOffHand} {
		if w, ok := target.Equipment[slot]; ok && w.Type == items.ItemWeapon {
			if def := lookupWeaponConfigByName(w.Name); def != nil {
				total += def.ThornsPct
			}
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
	cs.game.AddColoredCombatMessage(
		fmt.Sprintf("%s tries to pick %s's pocket!", attacker.Name, monster.Name),
		combatMessagePurple,
	)
	if stolen := cs.checkMonsterLootDrop(monster); len(stolen) > 0 {
		for _, it := range stolen {
			cs.game.party.AddItem(it)
			cs.game.AddColoredCombatMessage(
				fmt.Sprintf("%s picks %s's pocket: %s!", attacker.Name, monster.Name, it.Name),
				lootMessageColor([]items.Item{it}),
			)
		}
		return
	}
	gold := character.SleightGoldLow
	if monster.Level > character.SleightHighLevelThreshold {
		gold = character.SleightGoldHighLevel
	}
	cs.game.awardGold(gold)
	cs.game.AddColoredCombatMessage(
		fmt.Sprintf("%s finds no item and lifts %d gold off %s instead!", attacker.Name, gold, monster.Name),
		combatMessagePurple,
	)
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
	spell, ok := attacker.Equipment[items.SlotSpell]
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

// tryApplyMonsterPoison rolls the attacker's PoisonChance against a character
// that just took a hit. Shared by the RT and TB melee paths.
func (cs *CombatSystem) tryApplyMonsterPoison(monster *monsterPkg.Monster3D, target *character.MMCharacter) {
	if monster.PoisonChance <= 0 || rand.Float64() >= monster.PoisonChance {
		return
	}
	// Mummy Card: chance to resist the poison outright.
	if resist := cs.game.cardPoisonResistPct(); resist > 0 && rand.Intn(100) < resist {
		return
	}
	// poison_duration_seconds is guaranteed by load-time validation.
	poisonFrames := cs.game.config.GetTPS() * monster.PoisonDurationSec
	target.ApplyPoison(poisonFrames)
	cs.game.AddCombatMessage(fmt.Sprintf("%s is poisoned!", target.Name))
}

// tryApplyMonsterIgnite rolls the attacker's IgniteChance and sets the target on
// fire - a burn DoT 3x as strong as poison that STACKS with it (independent tick).
func (cs *CombatSystem) tryApplyMonsterIgnite(monster *monsterPkg.Monster3D, target *character.MMCharacter) {
	if monster.IgniteChance <= 0 || rand.Float64() >= monster.IgniteChance {
		return
	}
	// ignite_duration_seconds is guaranteed by load-time validation.
	burnFrames := cs.game.config.GetTPS() * monster.IgniteDurationSec
	target.ApplyBurn(burnFrames)
	cs.game.AddColoredCombatMessage(fmt.Sprintf("%s bursts into flames!", target.Name), combatMessageOrange)
}

// applyScaledCharStun stuns a party member for frames/turns, first applying the
// wearer's set stun-duration modifier (Pit Fighter's Quilt: the quilting soaks
// the blow - floor of 1 so a landed stun is never a no-op). The one place the
// stun-resist scaling lives, shared by weapon/monster stuns and champion casts.
func (cs *CombatSystem) applyScaledCharStun(target *character.MMCharacter, frames, turns int) {
	if pct := target.SetStunDurationPct(); pct != 0 {
		frames = max(1, frames*(100+pct)/100)
		turns = max(1, turns*(100+pct)/100)
	}
	target.ApplyCharStun(frames, turns)
}

// tryApplyMonsterStun rolls the attacker's StunCharChance and stuns the struck
// character (skips its actions: RT seconds / TB turns).
func (cs *CombatSystem) tryApplyMonsterStun(monster *monsterPkg.Monster3D, target *character.MMCharacter) {
	if monster.StunCharChance <= 0 || rand.Float64() >= monster.StunCharChance {
		return
	}
	cs.applyScaledCharStun(target, cs.game.config.GetTPS()*monster.StunCharSeconds, monster.StunCharTurns)
	cs.game.AddColoredCombatMessage(fmt.Sprintf("%s is stunned!", target.Name), combatMessageYellow)
}

// tryApplyMonsterDispel rolls the attacker's DispelChance and strips one random
// active party buff (stat or combat). Buffs are party-wide, so the struck
// character only triggers the roll.
func (cs *CombatSystem) tryApplyMonsterDispel(monster *monsterPkg.Monster3D, _ *character.MMCharacter) {
	if monster.DispelChance <= 0 || rand.Float64() >= monster.DispelChance {
		return
	}
	type dispelTarget struct {
		spellID string
		combat  bool
	}
	var pool []dispelTarget
	for i := range cs.game.statBuffs {
		pool = append(pool, dispelTarget{cs.game.statBuffs[i].SpellID, false})
	}
	for i := range cs.game.combatBuffs {
		pool = append(pool, dispelTarget{cs.game.combatBuffs[i].SpellID, true})
	}
	if len(pool) == 0 {
		return
	}
	pick := pool[rand.Intn(len(pool))]
	name := pick.spellID
	if def, err := spells.GetSpellDefinitionByID(spells.SpellID(pick.spellID)); err == nil && def.Name != "" {
		name = def.Name
	}
	if pick.combat {
		cs.game.removeCombatBuff(pick.spellID)
	} else {
		cs.game.removeStatBuff(pick.spellID)
	}
	cs.game.AddColoredCombatMessage(fmt.Sprintf("%s rips %s from the party!", monster.Name, name), combatMessagePurple)
}

// damagePartyMemberElement applies one elemental hit to a single party member
// through the shared pipeline and returns the damage actually dealt: mitigate
// (armor%/resist/buffs), subtract, clamp at 0, knock out at 0 (the Lich Card
// cheat-death chokepoint), and flash the damage-blink. The ONE body behind
// every whole-party elemental attack (Fireburst, Inferno, the Inferno nova);
// callers supply their own flavor line and any extra VFX (e.g. party flame).
func (cs *CombatSystem) damagePartyMemberElement(idx int, member *character.MMCharacter, rawDamage int, school string) int {
	dealt := cs.mitigateCharacterDamage(rawDamage, school, member, false)
	member.HitPoints -= dealt
	if member.HitPoints < 0 {
		member.HitPoints = 0
	}
	if member.HitPoints == 0 {
		cs.knockOut(member)
	}
	cs.game.TriggerDamageBlink(idx)
	return dealt
}

func (cs *CombatSystem) applyMonsterFireburst(monster *monsterPkg.Monster3D) {
	cs.game.AddCombatMessage(fmt.Sprintf("%s casts Fireburst!", monster.Name))

	cs.forEachDamageablePartyMember(func(idx int, member *character.MMCharacter) {
		minDamage := monster.FireburstDamageMin
		maxDamage := monster.FireburstDamageMax
		if minDamage <= 0 {
			minDamage = 6
		}
		if maxDamage < minDamage {
			maxDamage = minDamage
		}
		raw := minDamage
		if maxDamage > minDamage {
			raw = minDamage + rand.Intn(maxDamage-minDamage+1)
		}
		dealt := cs.damagePartyMemberElement(idx, member, raw, monsterPkg.DamageSchoolFire)
		cs.game.AddCombatMessage(fmt.Sprintf("Fireburst hits %s for %d damage! (HP: %d/%d)",
			member.Name, dealt, member.HitPoints, member.MaxHitPoints))
	})
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
	cs.game.spawnWeaponBoltImpact(vx, vy, weaponDef, count, size)
}

func (cs *CombatSystem) spawnMonsterRangedAttack(monster *monsterPkg.Monster3D) {
	// Champion spellcasting claims the attack before anything else: the
	// opening spell always takes the duel's first action, then each attack
	// rolls spell_cast_chance. Shared by RT ticks and every TB swing.
	if cs.championTryCastSpell(monster) {
		return
	}
	if cs.tryMonsterSpecialAbility(monster) {
		return
	}
	cs.spawnMonsterRangedAttackNormal(monster)
}

func (cs *CombatSystem) spawnMonsterRangedAttackNormal(monster *monsterPkg.Monster3D) {
	if cs.tryMonsterAoeAttack(monster) {
		return
	}
	cs.spawnMonsterRangedAttackAt(monster, cs.game.camera.X, cs.game.camera.Y, ProjectileOwnerMonster)
}

// tryMonsterAoeAttack runs a monster's whole-party attacks that preempt its
// normal single-target hit - Dragon Breath, then Fireburst, in that order.
// Returns true if one fired (the caller then skips its normal melee/ranged
// attack). Shared by the melee and ranged paths so a new whole-party attack is
// added in ONE place, not copy-pasted into both in the right order.
func (cs *CombatSystem) tryMonsterAoeAttack(monster *monsterPkg.Monster3D) bool {
	if cs.tryMonsterDragonBreath(monster) {
		return true
	}
	if monster.FireburstChance > 0 && rand.Float64() < monster.FireburstChance {
		cs.applyMonsterFireburst(monster)
		return true
	}
	return false
}

func (cs *CombatSystem) tryMonsterDragonBreath(monster *monsterPkg.Monster3D) bool {
	if monster == nil || monster.DragonBreathChance <= 0 || rand.Float64() >= monster.DragonBreathChance {
		return false
	}
	damageType := normalizeDamageTypeStr(monster.DragonBreathDamageType)
	damage := cs.monsterAttackDamage(monster)
	cs.game.AddCombatMessage(fmt.Sprintf("%s breathes %s over the whole party!", monster.Name, damageType))
	cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
		cs.monsterHitCharacter(monster, member, fmt.Sprintf("%s's Dragon Breath", monster.Name), damage, damageType, monster.IgnoresArmor, 0, false)
	})
	return true
}

func (cs *CombatSystem) tryMonsterSpecialAbility(monster *monsterPkg.Monster3D) bool {
	if monster == nil || !monster.IsAlive() {
		return false
	}
	if cs.tryMonsterAllyHeal(monster) {
		return true
	}
	if cs.tryMonsterPiercingShot(monster) {
		return true
	}
	return false
}

func (cs *CombatSystem) tryMonsterPiercingShot(monster *monsterPkg.Monster3D) bool {
	if monster.PiercingShotChance <= 0 || rand.Float64() >= monster.PiercingShotChance {
		return false
	}
	alive := alivePartyIndices(cs.game.party.Members)
	if len(alive) == 0 {
		return false
	}
	targets := monster.PiercingShotTargets
	if targets <= 0 {
		targets = 2
	}
	if targets > len(alive) {
		targets = len(alive)
	}
	rand.Shuffle(len(alive), func(i, j int) { alive[i], alive[j] = alive[j], alive[i] })

	cs.game.AddCombatMessage(fmt.Sprintf("%s fires a Piercing Shot!", monster.Name))
	for _, targetIndex := range alive[:targets] {
		target := cs.game.party.Members[targetIndex]
		// Piercing Shot ignores armor; the shared choke point applies the poison
		// rider (a poisonous monster now poisons via Piercing Shot, like melee).
		cs.monsterHitCharacter(monster, target, "Piercing Shot", cs.monsterAttackDamage(monster), monsterPkg.DamageSchoolPhysical, true, 0, false)
	}
	return true
}

func (cs *CombatSystem) tryMonsterAllyHeal(monster *monsterPkg.Monster3D) bool {
	if monster.AllyHealChance <= 0 || monster.AllyHealAmount <= 0 || rand.Float64() >= monster.AllyHealChance {
		return false
	}
	target := cs.pickMonsterAllyHealTarget(monster)
	if target == nil {
		return false
	}
	before := target.HitPoints
	target.HitPoints += monster.AllyHealAmount
	if target.HitPoints > target.MaxHitPoints {
		target.HitPoints = target.MaxHitPoints
	}
	actual := target.HitPoints - before
	if actual <= 0 {
		return false
	}
	if target == monster {
		cs.game.AddCombatMessage(fmt.Sprintf("%s mends itself for %d HP! (HP: %d/%d)",
			monster.Name, actual, target.HitPoints, target.MaxHitPoints))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s mends %s for %d HP! (HP: %d/%d)",
			monster.Name, target.Name, actual, target.HitPoints, target.MaxHitPoints))
	}
	return true
}

func (cs *CombatSystem) pickMonsterAllyHealTarget(healer *monsterPkg.Monster3D) *monsterPkg.Monster3D {
	if cs.game == nil || cs.game.world == nil {
		return nil
	}
	radius := healer.AllyHealRadiusPixels
	if radius <= 0 {
		radius = 2 * float64(cs.game.config.GetTileSize())
	}
	bestFrac := math.MaxFloat64
	var best *monsterPkg.Monster3D
	for _, candidate := range cs.game.world.Monsters {
		if candidate == nil || !candidate.IsAlive() || candidate.HitPoints >= candidate.MaxHitPoints {
			continue
		}
		if candidate.Bound != healer.Bound {
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
// one was spawned. Fireburst (party-only AoE) is handled by the caller.
func (cs *CombatSystem) spawnMonsterRangedAttackAt(monster *monsterPkg.Monster3D, targetX, targetY float64, owner ProjectileOwner) bool {
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
// monster (BoundUndead: bound undead -> enemy; MonsterAtBound: mob -> bound undead).
func (cs *CombatSystem) spawnMonsterRangedAttackAtMonster(monster, target *monsterPkg.Monster3D, owner ProjectileOwner) bool {
	return cs.spawnMonsterRangedAttackAt(monster, target.X, target.Y, owner)
}

// resolveMonsterProjectileVsMonster applies a monster-fired projectile's hit to
// another monster (bound undead <-> enemy crossfire). Damage is the projectile's
// own; the party is rewarded ONLY when an enemy falls (never for a bound ally).
func (cs *CombatSystem) resolveMonsterProjectileVsMonster(projectile interface{}, pType string, target *monsterPkg.Monster3D, entityID string) {
	var damage int
	var dmgType monsterPkg.DamageType
	var dmgTypeStr, spellFx, sourceName string
	var disintegrateChance, aoeRadiusTiles, stunChance float64
	var stunSeconds, stunTurns int
	var srcMonster *monsterPkg.Monster3D
	var owner ProjectileOwner
	switch pType {
	case "magic_projectile":
		mp := projectile.(*MagicProjectile)
		if !mp.Active || mp.LifeTime <= 0 {
			return
		}
		mp.Active = false
		damage, sourceName, spellFx = mp.Damage, mp.SourceName, mp.SpellType
		disintegrateChance = mp.DisintegrateChance
		srcMonster = mp.SourceMonster
		owner = mp.Owner
		spellDef, _ := spells.GetSpellDefinitionByID(spells.SpellID(mp.SpellType))
		dmgTypeStr = normalizeDamageTypeStr(spellDef.School)
		aoeRadiusTiles = spellDef.AoeRadiusTiles
		stunChance, stunSeconds, stunTurns = spellDef.StunChance, spellDef.StunDurationSeconds, spellDef.StunDurationTurns
	case "arrow":
		ar := projectile.(*Arrow)
		if !ar.Active || ar.LifeTime <= 0 {
			return
		}
		ar.Active = false
		damage, sourceName = ar.Damage, ar.SourceName
		disintegrateChance = ar.DisintegrateChance
		dmgTypeStr = normalizeDamageTypeStr(ar.DamageType)
		srcMonster = ar.SourceMonster
		owner = ar.Owner
		// A weapon projectile carries its own AoE rider (a champion's archmage staff,
		// Bow of Hellfire): read it so the impact splashes nearby monsters, matching
		// the spell path above and the party-side splash.
		if ar.BowKey != "" {
			if wd, ok := config.GetWeaponDefinition(ar.BowKey); ok && wd != nil {
				aoeRadiusTiles = wd.AoeRadiusTiles
			}
		}
	default:
		return
	}
	dmgType = convertToMonsterDamageType(dmgTypeStr)
	cs.game.collisionSystem.UnregisterEntity(entityID)
	// Target already slain this frame (another hit landed first): consume the
	// projectile but don't re-damage or double-reward.
	if !target.IsAlive() {
		return
	}
	if sourceName == "" {
		sourceName = "A bolt"
	}
	if spellFx != "" {
		tx, ty := cs.monsterVisualPos(target) // banded/pulled: burst where the mob is DRAWN
		cs.game.CreateSpellHitEffectFromSpell(tx, ty, spellFx)
	}

	// kill finalizes a slain crossfire target through the same kill choke point as
	// a party hit, so a bound summon can earn champion rewards for the party too.
	kill := func() {
		cs.game.AddCombatMessage(fmt.Sprintf("%s is destroyed!", target.Name))
		cs.finishMonsterKillImmediately(target)
	}

	// Disintegrate rider: the bound mob's projectile keeps its instakill chance.
	if disintegrateChance > 0 && !monsterImmuneToDisintegrate(target) && rand.Float64() < disintegrateChance {
		target.HitPoints = 0
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.AddCombatMessage(fmt.Sprintf("%s's bolt disintegrates %s!", sourceName, target.Name))
		kill()
		return
	}

	if damage > 0 {
		target.TakeDamage(damage, dmgType)
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.AddCombatMessage(fmt.Sprintf("%s's bolt hits %s for %d!", sourceName, target.Name, damage))
	}
	// Stun rider (Psychic Shock etc.) carries over too.
	if target.IsAlive() && stunChance > 0 && rand.Float64() < stunChance {
		cs.applyStun(target, stunSeconds, stunTurns) // announces stun/resist itself
	}
	if !target.IsAlive() {
		kill()
		return
	}
	// AoE rider: crossfire may splash only the direct target's faction. Reusing
	// the player AoE path here would hit the firing champion and its own ordinary
	// allies, then incorrectly credit their deaths to the party.
	if aoeRadiusTiles > 0 {
		cs.applyCrossfireAoeSplash(target, srcMonster, owner, damage, dmgType, aoeRadiusTiles)
		// A CHAMPION's AoE bolt that reaches the party strikes it too (the extra
		// action - the summon-splash's party twin). Plain mob crossfire never hits
		// the party, so this is gated to champions.
		if srcMonster != nil && srcMonster.IsChampion() &&
			Distance(target.X, target.Y, cs.game.camera.X, cs.game.camera.Y) <= aoeRadiusTiles*float64(cs.game.config.GetTileSize()) {
			cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
				cs.monsterHitCharacter(srcMonster, member, srcMonster.Name, damage, dmgTypeStr, srcMonster.IgnoresArmor, 0, false)
			})
		}
	}
}

// applyCrossfireAoeSplash applies an AoE monster projectile to the same faction
// as its direct target. Bound allies splash enemy monsters; enemies splash bound
// allies. The firing monster is never a splash target, so a close-range AoE
// cannot self-kill a champion or damage its own pack.
func (cs *CombatSystem) applyCrossfireAoeSplash(center, source *monsterPkg.Monster3D, owner ProjectileOwner, damage int, damageType monsterPkg.DamageType, radiusTiles float64) {
	if center == nil || source == nil || radiusTiles <= 0 {
		return
	}
	radius := radiusTiles * float64(cs.game.config.GetTileSize())
	for _, candidate := range cs.game.world.Monsters {
		if candidate == nil || candidate == center || candidate == source || !candidate.IsAlive() {
			continue
		}
		if owner == ProjectileOwnerBoundUndead {
			if candidate.Bound || candidate.Pacified {
				continue
			}
		} else if !candidate.Bound {
			continue
		}
		if Distance(center.X, center.Y, candidate.X, candidate.Y) <= radius {
			cs.strikeMonsterFor(source, candidate, damage, damageType)
		}
	}
}

func (cs *CombatSystem) spawnMonsterSpellProjectile(monster *monsterPkg.Monster3D, spellID spells.SpellID, targetX, targetY float64, owner ProjectileOwner) {
	cs.spawnMonsterSpellProjectileDamage(monster, spellID, targetX, targetY, owner, cs.monsterAttackDamage(monster))
}

// spawnMonsterSpellProjectileDamage is the damage-explicit core: champion
// casts author the damage from the real spell formula (championCastSpell),
// while plain projectile_spell mobs keep their authored attack damage.
func (cs *CombatSystem) spawnMonsterSpellProjectileDamage(monster *monsterPkg.Monster3D, spellID spells.SpellID, targetX, targetY float64, owner ProjectileOwner, damage int) {
	castingSystem := spells.NewCastingSystem(cs.game.config)
	angle := math.Atan2(targetY-monster.Y, targetX-monster.X)
	projectile, err := castingSystem.CreateProjectile(spellID, monster.X, monster.Y, angle)
	if err != nil {
		return
	}

	spellConfig, err := cs.game.config.GetSpellConfig(string(spellID))
	if err != nil {
		return
	}
	disintegrateChance := 0.0
	aoe := false
	if spellDefConfig, exists := config.GetSpellDefinition(string(spellID)); exists && spellDefConfig != nil {
		disintegrateChance = spellDefConfig.DisintegrateChance
		aoe = spellDefConfig.AoeRadiusTiles > 0 // e.g. fireball: splash the whole party on hit
	}

	magicProjectile := MagicProjectile{
		ID:                 cs.game.GenerateProjectileID("monster_" + string(spellID)),
		X:                  monster.X,
		Y:                  monster.Y,
		VelX:               projectile.VelX,
		VelY:               projectile.VelY,
		Damage:             damage,
		LifeTime:           projectile.LifeTime,
		Active:             projectile.Active,
		SpellType:          string(spellID),
		Size:               projectile.Size,
		Crit:               false,
		DisintegrateChance: disintegrateChance,
		Owner:              owner,
		SourceName:         monster.Name,
		SourceMonster:      monster,
		AoE:                aoe,
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

	tileSize := cs.game.config.GetTileSize()
	collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
	projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(projectileEntity)
}

func (cs *CombatSystem) spawnMonsterWeaponProjectile(monster *monsterPkg.Monster3D, weaponKey string, targetX, targetY float64, owner ProjectileOwner) {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists || weaponDef == nil || weaponDef.Physics == nil {
		fmt.Printf("[WARN] projectile weapon '%s' is missing physics in weapons.yaml\n", weaponKey)
		return
	}

	tileSize := cs.game.config.GetTileSize()
	arrowSpeed := weaponDef.Physics.GetSpeedPixels(tileSize)
	arrowLifetime := weaponDef.Physics.GetLifetimeFrames()
	collisionSize := weaponDef.Physics.GetCollisionSizePixels(tileSize)

	damageType := monsterPkg.DamageSchoolPhysical
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
		arrow := Arrow{
			ID:                 cs.game.GenerateProjectileID("monster_arrow"),
			SuppressAoE:        i > 0, // an AoE-rider weapon engulfs the party once per VOLLEY, not per dart
			X:                  monster.X - dirX*back,
			Y:                  monster.Y - dirY*back,
			VelX:               dirX * arrowSpeed,
			VelY:               dirY * arrowSpeed,
			Damage:             cs.monsterAttackDamage(monster),
			LifeTime:           arrowLifetime,
			Active:             true,
			BowKey:             weaponKey,
			DamageType:         damageType,
			Crit:               false,
			DisintegrateChance: weaponDef.DisintegrateChance,
			Owner:              owner,
			SourceName:         monster.Name,
			SourceMonster:      monster,
		}
		cs.game.arrows = append(cs.game.arrows, arrow)
		arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(arrowEntity)
	}
}

// CheckProjectileMonsterCollisions checks for collisions between projectiles and monsters
// using perspective-scaled bounding boxes for accurate visual collision detection
func (cs *CombatSystem) CheckProjectileMonsterCollisions() {
	// Collect all active projectiles. Monster-owned ones are excluded (they hit
	// the party, not other monsters); player- and bound/mob-at-bound ones hit monsters.
	type projectileInfo struct {
		entityID string
		data     interface{}
		pType    string
		owner    ProjectileOwner
	}
	var projectiles []projectileInfo

	for i := range cs.game.arrows {
		if cs.game.arrows[i].Active && cs.game.arrows[i].LifeTime > 0 && cs.game.arrows[i].Owner != ProjectileOwnerMonster {
			projectiles = append(projectiles, projectileInfo{cs.game.arrows[i].ID, &cs.game.arrows[i], "arrow", cs.game.arrows[i].Owner})
		}
	}
	for i := range cs.game.magicProjectiles {
		if cs.game.magicProjectiles[i].NoCollide {
			continue // mortar display bolt: the scheduled detonation is the real hit
		}
		if cs.game.magicProjectiles[i].Active && cs.game.magicProjectiles[i].LifeTime > 0 && cs.game.magicProjectiles[i].Owner != ProjectileOwnerMonster {
			projectiles = append(projectiles, projectileInfo{cs.game.magicProjectiles[i].ID, &cs.game.magicProjectiles[i], "magic_projectile", cs.game.magicProjectiles[i].Owner})
		}
	}
	// Check each projectile against each monster using perspective-scaled collision
	for _, proj := range projectiles {
		var hitMonster *monsterPkg.Monster3D
		bestDepth := 0.0
		bestLateral := 0.0

		camCos := math.Cos(cs.game.camera.Angle)
		camSin := math.Sin(cs.game.camera.Angle)

		for _, monster := range cs.game.world.Monsters {
			if !monster.IsAlive() {
				continue
			}
			// A pierce continuation bolt (Arena Arbalest) flies on THROUGH the
			// monster it already hit - never collides with it again.
			if ar, ok := proj.data.(*Arrow); ok && ar.SkipMonster == monster {
				continue
			}
			// Crossfire faction rules: a bound undead's bolt skips controlled allies
			// (hits enemies); a mob's anti-undead bolt hits ONLY the bound undead.
			if proj.owner == ProjectileOwnerBoundUndead && (monster.Bound || monster.Pacified) {
				continue
			}
			if proj.owner == ProjectileOwnerMonsterAtBound && !monster.Bound {
				continue
			}
			if cs.checkPerspectiveScaledCollision(proj.entityID, proj.data, proj.pType, monster) {
				dx := monster.X - cs.game.camera.X
				dy := monster.Y - cs.game.camera.Y
				depth := dx*camCos + dy*camSin
				if depth <= 0 {
					continue
				}
				angle := math.Atan2(dy, dx)
				angleDiff := angle - cs.game.camera.Angle
				for angleDiff > math.Pi {
					angleDiff -= 2 * math.Pi
				}
				for angleDiff < -math.Pi {
					angleDiff += 2 * math.Pi
				}
				if math.Abs(angleDiff) > cs.game.camera.FOV/2 {
					continue
				}
				lateral := math.Abs(-dx*camSin + dy*camCos)
				if hitMonster == nil || depth < bestDepth || (depth == bestDepth && lateral < bestLateral) {
					bestDepth = depth
					bestLateral = lateral
					hitMonster = monster
				}
			}
		}
		if hitMonster == nil && proj.owner == ProjectileOwnerPlayer {
			var px, py, vx, vy float64
			switch d := proj.data.(type) {
			case *Arrow:
				px, py, vx, vy = d.X, d.Y, d.VelX, d.VelY
			case *MagicProjectile:
				px, py, vx, vy = d.X, d.Y, d.VelX, d.VelY
			}
			hitMonster = cs.turnBasedProjectileAssistTarget(px, py, vx, vy)
			// A pierce continuation must not re-hit the monster it went
			// through via the TB assist either.
			if ar, ok := proj.data.(*Arrow); ok && hitMonster != nil && hitMonster == ar.SkipMonster {
				hitMonster = nil
			}
		}
		if hitMonster != nil {
			// Monster-fired crossfire resolves as monster-vs-monster (no party
			// attribution); player projectiles use the full party-damage path.
			if proj.owner == ProjectileOwnerBoundUndead || proj.owner == ProjectileOwnerMonsterAtBound {
				cs.resolveMonsterProjectileVsMonster(proj.data, proj.pType, hitMonster, proj.entityID)
			} else {
				cs.applyProjectileDamage(proj.data, proj.pType, hitMonster, proj.entityID)
			}
		}
	}
}

// CheckProjectilePlayerCollisions checks for collisions between monster projectiles and the player.
func (cs *CombatSystem) CheckProjectilePlayerCollisions() {
	playerEntity := cs.game.collisionSystem.GetEntityByID("player")
	if playerEntity == nil || playerEntity.BoundingBox == nil {
		return
	}

	for i := range cs.game.magicProjectiles {
		mp := &cs.game.magicProjectiles[i]
		if !mp.Active || mp.LifeTime <= 0 || mp.Owner != ProjectileOwnerMonster {
			continue
		}
		if cs.projectileHitsPlayer(mp.ID, playerEntity) {
			damageTypeStr := spellDamageTypeStr(mp.SpellType)
			// Champion spell riders (lightning/psychic-shock stun) resolve from
			// the spell that actually flew - a weapon swing landing mid-flight
			// may have re-stamped the mob's rider fields for a hand.
			cs.stampChampionSpellRiders(mp.SourceMonster, mp.SpellType)
			if mp.AoE {
				cs.applyMonsterProjectileDamageAoE(mp.SourceMonster, mp.SourceName, mp.Damage, damageTypeStr, mp.DisintegrateChance)
			} else {
				cs.applyMonsterProjectileDamage(mp.SourceMonster, mp.SourceName, mp.Damage, damageTypeStr, mp.DisintegrateChance)
			}
			mp.Active = false
			cs.game.collisionSystem.UnregisterEntity(mp.ID)
		}
	}

	for i := range cs.game.arrows {
		ar := &cs.game.arrows[i]
		if !ar.Active || ar.LifeTime <= 0 || ar.Owner != ProjectileOwnerMonster {
			continue
		}
		if cs.projectileHitsPlayer(ar.ID, playerEntity) {
			damageTypeStr := normalizeDamageTypeStr(ar.DamageType)
			// Riders resolve from the weapon that FIRED this dart - a swing landing
			// mid-flight may have re-armed the mob's rider fields for another hand.
			cs.stampChampionProjectileRiders(ar.SourceMonster, ar.BowKey)
			// An AoE-rider weapon (bow_of_hellfire) engulfs the WHOLE party once
			// per volley - the champion rule: arc/AoE never multiply.
			if def, ok := config.GetWeaponDefinition(ar.BowKey); ok && def != nil && def.AoeRadiusTiles > 0 && !ar.SuppressAoE {
				cs.applyMonsterProjectileDamageAoE(ar.SourceMonster, ar.SourceName, ar.Damage, damageTypeStr, ar.DisintegrateChance)
			} else {
				cs.applyMonsterProjectileDamage(ar.SourceMonster, ar.SourceName, ar.Damage, damageTypeStr, ar.DisintegrateChance)
			}
			ar.Active = false
			cs.game.collisionSystem.UnregisterEntity(ar.ID)
		}
	}
}

func (cs *CombatSystem) projectileHitsPlayer(projectileID string, playerEntity *collision.Entity) bool {
	projEntity := cs.game.collisionSystem.GetEntityByID(projectileID)
	if projEntity == nil || projEntity.BoundingBox == nil {
		return false
	}
	return projEntity.BoundingBox.Intersects(playerEntity.BoundingBox)
}

// applyMonsterProjectileDamage applies a single-target monster projectile/arrow.
// Real-time -> the tank (front slot). Turn-based -> mostly the tank, sometimes a
// back-liner (see rangedTBTarget / RangedOffTankChance).
func (cs *CombatSystem) applyMonsterProjectileDamage(src *monsterPkg.Monster3D, sourceName string, damage int, damageTypeStr string, disintegrateChance float64) {
	var target *character.MMCharacter
	if cs.game.turnBasedMode {
		target = cs.rangedTBTarget()
	} else {
		target = cs.tankTarget()
	}
	cs.applyMonsterProjectileDamageToChar(src, target, sourceName, damage, damageTypeStr, disintegrateChance)
}

// applyMonsterProjectileDamageAoE splashes a monster projectile across EVERY
// party member that can still take a hit (AoE spells like a monster's fireball).
func (cs *CombatSystem) applyMonsterProjectileDamageAoE(src *monsterPkg.Monster3D, sourceName string, damage int, damageTypeStr string, disintegrateChance float64) {
	if sourceName == "" {
		sourceName = "Monster"
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s's blast engulfs the whole party!", sourceName))
	cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
		cs.applyMonsterProjectileDamageToChar(src, member, sourceName, damage, damageTypeStr, disintegrateChance)
	})
}

func (cs *CombatSystem) forEachDamageablePartyMember(fn func(idx int, member *character.MMCharacter)) {
	for idx, member := range cs.game.party.Members {
		if member == nil || member.HitPoints <= 0 {
			continue
		}
		fn(idx, member)
	}
}

func (cs *CombatSystem) applyMonsterProjectileDamageToChar(src *monsterPkg.Monster3D, currentChar *character.MMCharacter, sourceName string, damage int, damageTypeStr string, disintegrateChance float64) {
	if currentChar == nil {
		return
	}
	// src is the firing monster (carries true-damage + on-hit riders to impact);
	// the disintegrate roll runs inside the shared choke point.
	cs.monsterHitCharacter(src, currentChar, sourceName, damage, damageTypeStr, false, disintegrateChance, false)
}

// getProjectileGraphicsInfo extracts base size, min size, and max size for a projectile
func (cs *CombatSystem) getProjectileGraphicsInfo(projectile interface{}, projectileType string) (baseSize float64, minSize, maxSize int, ok bool) {
	switch projectileType {
	case "magic_projectile":
		magicProj := projectile.(*MagicProjectile)
		cfg, err := cs.game.config.GetSpellGraphicsConfig(magicProj.SpellType)
		if err != nil {
			return 0, 0, 0, false
		}
		return float64(cfg.BaseSize), cfg.MinSize, cfg.MaxSize, true
	case "arrow":
		arrow := projectile.(*Arrow)
		weaponDef := lookupWeaponConfigByKey(arrow.BowKey)
		if weaponDef == nil || weaponDef.Graphics == nil {
			return 0, 0, 0, false
		}
		return float64(weaponDef.Graphics.BaseSize), weaponDef.Graphics.MinSize, weaponDef.Graphics.MaxSize, true
	}
	return 0, 0, 0, false
}

// getProjectilePosition returns the X, Y position of a projectile
func (cs *CombatSystem) getProjectilePosition(projectile interface{}, projectileType string) (float64, float64) {
	switch projectileType {
	case "magic_projectile":
		p := projectile.(*MagicProjectile)
		return p.X, p.Y
	case "arrow":
		p := projectile.(*Arrow)
		return p.X, p.Y
	}
	return 0, 0
}

// calculatePerspectiveScale calculates the scale factor for perspective-based collision
func (cs *CombatSystem) calculatePerspectiveScale(x, y, baseSize float64, minSize, maxSize int) float64 {
	dist := Distance(cs.game.camera.X, cs.game.camera.Y, x, y)
	if dist == 0 {
		dist = 0.001 // Avoid division by zero
	}

	visualSize := baseSize / dist * float64(cs.game.config.GetTileSize())
	if visualSize > float64(maxSize) {
		visualSize = float64(maxSize)
	}
	if visualSize < float64(minSize) {
		visualSize = float64(minSize)
	}
	scale := visualSize / baseSize
	// Never INFLATE the collision box above its true world size. Near the camera
	// (e.g. the spawn frame, dist~0) this scale would otherwise balloon - a
	// fireball's 2-tile box x ~3.9 ~ 8 tiles - so it "hit" and exploded on a
	// monster several tiles away before the projectile was even drawn. Clamping
	// to 1 keeps collision at the world box up close and only shrinks it far away.
	if scale > 1.0 {
		scale = 1.0
	}
	return scale
}

// spawnProjectileHitFX bursts the impact FX for a projectile hit at the FX
// anchor: spell-typed particles for magic projectiles (school-colored fallback
// otherwise), or the ranged-weapon effect for arrows.
func (cs *CombatSystem) spawnProjectileHitFX(projectile interface{}, fxX, fxY float64, isSpell, isRanged bool, damageTypeStr string, monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, damage int) {
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			cs.game.CreateSpellHitEffectFromSpell(fxX, fxY, mp.SpellType)
		} else {
			cs.game.CreateSpellHitEffect(fxX, fxY, damageTypeStr, SpellParticleCount, SpellParticleSize)
		}
	} else if isRanged {
		cs.spawnRangedHitEffect(monster, weaponDef, damage)
	}
}

// applyProjectileDamage applies damage from a projectile to a monster and generates combat messages
func (cs *CombatSystem) applyProjectileDamage(projectile interface{}, projectileType string, monster *monsterPkg.Monster3D, entityID string) {
	var damage int
	var isCrit bool
	var weaponName string
	var damageType monsterPkg.DamageType
	var damageTypeStr string
	var isSpell bool
	var isRanged bool
	var weaponDef *config.WeaponDefinitionConfig
	var disintegrateChance float64
	var aoeRadiusTiles float64
	var isBindSpell bool
	var bindSeconds int
	var isPacifySpell bool
	var pacifySeconds int
	var dealsNoDamage bool
	var stunChance float64
	var stunSeconds int
	var stunTurns int
	var starburstFx bool

	switch projectileType {
	case "magic_projectile":
		mp := projectile.(*MagicProjectile)
		if !mp.Active || mp.LifeTime <= 0 {
			return
		}
		damage, isCrit = mp.Damage, mp.Crit
		disintegrateChance = mp.DisintegrateChance + float64(cs.game.cardDisintegratePct())/100
		spellID := spells.SpellID(mp.SpellType)
		spellDef, _ := spells.GetSpellDefinitionByID(spellID)
		weaponName = spellDef.Name
		damageTypeStr = normalizeDamageTypeStr(spellDef.School)
		damageType = convertToMonsterDamageType(damageTypeStr)
		aoeRadiusTiles = spellDef.AoeRadiusTiles
		isBindSpell = spellDef.BindUndead
		bindSeconds = spellDef.BindDurationSeconds
		isPacifySpell = spellDef.Pacify
		pacifySeconds = spellDef.PacifyDurationSeconds
		dealsNoDamage = spellDef.DealsNoDamage
		stunChance = spellDef.StunChance
		stunSeconds = spellDef.StunDurationSeconds
		stunTurns = spellDef.StunDurationTurns
		starburstFx = spellDef.StarburstFx
		mp.Active = false
		isSpell = true

	case "arrow":
		ar := projectile.(*Arrow)
		if !ar.Active || ar.LifeTime <= 0 {
			return
		}
		damage, isCrit = ar.Damage, ar.Crit
		disintegrateChance = ar.DisintegrateChance + float64(cs.game.cardDisintegratePct())/100
		weaponName = "Arrow"
		damageTypeStr = normalizeDamageTypeStr(ar.DamageType)
		damageType = convertToMonsterDamageType(damageTypeStr)
		ar.Active = false
		isRanged = true
		if ar.Owner == ProjectileOwnerPlayer && ar.BowKey != "" {
			weaponDef = lookupWeaponConfigByKey(ar.BowKey)
			if weaponDef != nil {
				aoeRadiusTiles = weaponDef.AoeRadiusTiles
				if weaponDef.Name != "" {
					weaponName = weaponDef.Name
				}
			}
		}
		// A labeled bolt (Bandit Card proc) reports under its own name - the
		// BowKey above may be the hunting-bow PHYSICS fallback of a melee wielder.
		if ar.Label != "" {
			weaponName = ar.Label
		}
		// Arena Arbalest: the bolt goes through the first target and flies on as
		// a continuation that can never re-hit the monster it pierced. Spawned
		// before damage resolution so even a killing hit passes through.
		if ar.PierceLeft > 0 && ar.Owner == ProjectileOwnerPlayer {
			cont := *ar
			cont.ID = cs.game.GenerateProjectileID("arrow")
			cont.Active = true
			cont.PierceLeft = ar.PierceLeft - 1
			cont.SkipMonster = monster
			collisionSize := 8.0
			if weaponDef != nil && weaponDef.Physics != nil {
				collisionSize = weaponDef.Physics.GetCollisionSizePixels(float64(cs.game.config.GetTileSize()))
			}
			cs.game.arrows = append(cs.game.arrows, cont)
			contEntity := collision.NewEntity(cont.ID, cont.X, cont.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
			cs.game.collisionSystem.RegisterEntity(contEntity)
		}
	default:
		return
	}

	// A sealed (dormant) boss absorbs the projectile - no damage, control effect,
	// or aggro - until its quest unseals it. The projectile is already consumed by
	// the switch above; drop its collision entity and stop here.
	if cs.absorbIfSealed(monster) {
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Party buffs: flat bonus to party outgoing damage, filtered by damage type
	// (Heroism applies only to physical; Hour of Power applies to all).
	if damage > 0 {
		damage += cs.game.combatBuffOutBonusForDamageType(damageTypeStr)
	}

	// Resolve the attacker the projectile was fired by (stamped at spawn) -
	// selection may have auto-advanced (or the roster swapped) while it flew.
	var attacker *character.MMCharacter
	switch pr := projectile.(type) {
	case *MagicProjectile:
		attacker = pr.Attacker
	case *Arrow:
		attacker = pr.Attacker
	}
	attackerName := "The party"
	if attacker != nil {
		attackerName = attacker.Name
	}

	// Impact FX anchor: the projectile bursts where the monster is DRAWN. For a
	// turn-based pulled front-diagonal target that's the pulled slot (where the
	// assist connects), not its real off-to-the-side tile.
	fxX, fxY := cs.monsterVisualPos(monster)

	// Weapon-mastery TRUE damage / dodge-ignore (physical weapons only; spells
	// leave these zero/false). Spell schools instead pierce resistance at GM.
	trueDmg, ignoreDodge := cs.weaponMasteryStrike(attacker, weaponDef)
	resistPierce := 0
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			resistPierce = cs.spellResistPierce(attacker, mp.SpellType)
		}
	}

	// Check monster perfect dodge (applies to all attack types). A Grandmaster
	// weapon strike ignores it; otherwise the normal hit is dodged but mastery
	// TRUE damage still lands.
	if monster.PerfectDodge > 0 && !ignoreDodge && rand.Intn(100) < monster.PerfectDodge {
		cs.breakPacifyOnHit(monster)
		if trueDmg > 0 {
			cs.applyTrueDamageThroughDodge(monster, trueDmg, damageType, attackerName)
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges the %s!", monster.Name, weaponName))
		}
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Control spells deal no damage - Bind Undead takes control, Charm pacifies.
	if isBindSpell {
		cs.applyBindUndead(monster, bindSeconds, weaponName)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}
	if isPacifySpell {
		cs.applyPacify(monster, pacifySeconds, weaponName)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	if disintegrateChance > 0 && !monsterImmuneToDisintegrate(monster) && rand.Float64() < disintegrateChance {
		cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)

		monster.HitPoints = 0
		cs.markMonsterHit(monster)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		xpAwarded := cs.finishMonsterKill(monster)

		cs.game.AddCombatMessage(fmt.Sprintf("%s's %s disintegrates %s!", attackerName, weaponName, monster.Name))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
		return
	}

	// A no-damage projectile (Disintegrate) that DIDN'T trigger its instakill - or
	// struck an immune target (undead/dragon) - deals nothing but is still a HIT:
	// run the same impact-FX and aggro/pacify/pack bookkeeping a real hit does
	// (TakeDamageResist(0) still sets WasAttacked + engages, so passive mobs aggro)
	// and report "no effect" instead of falling into the damage path, which would
	// spam "hit for 0 damage" with a bogus "Critical!" (the spell can't crit).
	// Bind/Charm are handled earlier; this is the Disintegrate case.
	if dealsNoDamage {
		cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)
		monster.TakeDamageResist(0, damageType, resistPierce)
		cs.markMonsterHit(monster)
		cs.game.AddCombatMessage(fmt.Sprintf("%s has no effect on %s.", weaponName, monster.Name))
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Spawn hit effects at monster position (after dodge check, so only on actual hits)
	cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)

	// Phys-to-element conversion cards apply to every physical source (see
	// splitPhysConversions) - spells aren't physical, so they're excluded here.
	var convShares []physConvShare
	if !isSpell && damageTypeStr == monsterPkg.DamageSchoolPhysical {
		damage, convShares = cs.game.splitPhysConversions(damage)
	}

	// Calculate damage reduction based on damage type
	reducedDamage := applyMonsterArmor(damage, damageTypeStr, effectiveMonsterArmor(monster, weaponDef), isRanged)
	if !isSpell {
		if mult := cs.weaponBonusMultiplier(weaponDef, monster); mult != 1.0 {
			reducedDamage = int(math.Round(float64(reducedDamage) * mult))
			if reducedDamage < 1 {
				reducedDamage = 1
			}
		}
		if mult := weaponStunnedBonusMultiplier(weaponDef, monster); mult != 1.0 {
			reducedDamage = int(math.Round(float64(reducedDamage) * mult))
		}
	}
	// Card bonus_vs (e.g. Elf Archer vs dragons, Skeleton vs formless bosses) is a
	// collection property, not a weapon one - applies to spells too.
	if mult := cs.game.cardBonusVsMultiplier(monster); mult != 1.0 {
		reducedDamage = int(math.Round(float64(reducedDamage) * mult))
		if reducedDamage < 1 {
			reducedDamage = 1
		}
	}
	reducedDamage += trueDmg // weapon-mastery true damage bypasses armor

	// GM spell mastery pierces part of the target's resistance.
	actualDamage := monster.TakeDamageResist(reducedDamage, damageType, resistPierce)
	actualDamage += cs.applyPhysConversionShares(monster, convShares, isRanged)
	cs.markMonsterHit(monster)
	if monster.IsAlive() {
		cs.tryApplyWeaponHitRiders(monster, weaponDef)
		cs.tryCardPoisonProc(monster)
		// Spell stun-on-hit (Psychic Shock): chance to stun the struck monster.
		if stunChance > 0 && rand.Float64() < stunChance {
			cs.applyStun(monster, stunSeconds, stunTurns) // announces stun/resist itself
		}
	}
	cs.game.collisionSystem.UnregisterEntity(entityID)

	if !monster.IsAlive() {
		xpAwarded := cs.finishMonsterKill(monster)
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hits %s for %d damage and kills it!",
			prefix, attackerName, monster.Name, actualDamage))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
	} else {
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hit %s for %d %s damage! (HP: %d/%d)",
			prefix, weaponName, monster.Name, actualDamage, damageTypeStr, monster.HitPoints, monster.MaxHitPoints))
	}

	if aoeRadiusTiles > 0 {
		cs.applyAoeSplash(monster, damage, damageTypeStr, damageType, weaponName, aoeRadiusTiles, resistPierce)
		cs.splashPhysConversionShares(monster, convShares, weaponName, aoeRadiusTiles)
	}
	// Starburst: a star falls into every tile of the blast (purely visual).
	if starburstFx {
		r := aoeRadiusTiles
		if r <= 0 {
			r = 1
		}
		cs.game.spawnStarburstFx(monster.X, monster.Y, r)
	}
}

// applyAoeSplash deals the primary attack's damage to every OTHER alive monster
// within radiusTiles of the primary target. The `damage` passed in is the
// primary's already-resolved hit - so if the primary CRIT, that crit-boosted
// number splashes too (the splash rolls no SEPARATE crit/disintegrate/stun of
// its own). Each splash victim applies its own armor reduction. Drives
// Fireball-style AoE from a single YAML field (`aoe_radius_tiles`), shared
// between spells and weapon projectiles (e.g. Bow of Hellfire).
// applyPhysConversionShares lands each phys-to-element conversion share
// (Archmage=fire, Hexer=dark, Isis=light; see splitPhysConversions) on monster,
// mitigated as its own element - elemental armor cap, then that element's
// resistance. Shared by melee, ranged and trap damage so a conversion card
// applies uniformly to every physical source instead of being wired per-path.
func (cs *CombatSystem) applyPhysConversionShares(monster *monsterPkg.Monster3D, shares []physConvShare, isRanged bool) int {
	total := 0
	for _, s := range shares {
		reduced := applyMonsterArmor(s.amount, s.element, monster.EffectiveArmorClass(), isRanged)
		total += monster.TakeDamage(reduced, convertToMonsterDamageType(s.element))
	}
	return total
}

// splashPhysConversionShares mirrors applyPhysConversionShares for AoE splash:
// every converted share reaches nearby foes too, not just the physical
// remainder (previously the fire/dark/light shares were dropped from splash).
func (cs *CombatSystem) splashPhysConversionShares(center *monsterPkg.Monster3D, shares []physConvShare, weaponName string, radiusTiles float64) {
	for _, s := range shares {
		cs.applyAoeSplash(center, s.amount, s.element, convertToMonsterDamageType(s.element), weaponName, radiusTiles, 0)
	}
}

func (cs *CombatSystem) applyAoeSplash(center *monsterPkg.Monster3D, damage int, damageTypeStr string, damageType monsterPkg.DamageType, weaponName string, radiusTiles float64, resistPierce int) {
	if center == nil || radiusTiles <= 0 {
		return
	}
	tileSize := float64(cs.game.config.GetTileSize())
	radiusPx := radiusTiles * tileSize
	radiusSq := radiusPx * radiusPx
	cx, cy := center.X, center.Y

	for _, m := range cs.game.world.Monsters {
		// An invulnerable boss (sealed or idol-warded) takes no splash and triggers
		// no hit-flash / pack-aggro / message: skip it entirely.
		if m == nil || m == center || !m.IsAlive() || bossInvulnerable(m) {
			continue
		}
		dx := m.X - cx
		dy := m.Y - cy
		if dx*dx+dy*dy > radiusSq {
			continue
		}
		reduced := applyMonsterArmor(damage, damageTypeStr, m.EffectiveArmorClass(), false)
		actual := m.TakeDamageResist(reduced, damageType, resistPierce)
		cs.markMonsterHit(m)
		cs.spawnMonsterHitBurst(m, damageTypeStr)

		if !m.IsAlive() {
			xpAwarded := cs.finishMonsterKill(m)
			cs.game.AddCombatMessage(fmt.Sprintf("%s splash kills %s! (+%d XP)", weaponName, m.Name, xpAwarded))
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s splashes %s for %d %s damage.", weaponName, m.Name, actual, damageTypeStr))
		}
	}
}

func (cs *CombatSystem) weaponBonusMultiplier(weaponDef *config.WeaponDefinitionConfig, monster *monsterPkg.Monster3D) float64 {
	if weaponDef == nil || monster == nil || len(weaponDef.BonusVs) == 0 {
		return 1.0
	}

	// Match bonus_vs against both the display Name (so `bonus_vs: dragon`
	// hits every elemental dragon, all named "Dragon") and the exact key
	// (so a key-specific `bonus_vs: dragon_gold` is also possible).
	candidates := []string{monster.Name}
	if monster.Key != "" {
		candidates = append(candidates, monster.Key)
	}

	for bonusKey, mult := range weaponDef.BonusVs {
		for _, candidate := range candidates {
			if strings.EqualFold(bonusKey, candidate) {
				if mult <= 0 {
					return 1.0
				}
				return mult
			}
		}
	}

	return 1.0
}

func (cs *CombatSystem) tryApplyWeaponStun(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) {
	if monster == nil {
		return
	}
	framesPerTurn := cs.game.config.GetTPS()
	if framesPerTurn <= 0 {
		framesPerTurn = 60
	}
	if weaponDef != nil && weaponDef.StunChance > 0 && rand.Float64() < weaponDef.StunChance {
		turns := weaponDef.StunTurns
		if turns <= 0 {
			turns = 1
		}
		cs.applyStunDR(monster, turns, turns*framesPerTurn, true)
		return
	}
	// Minotaur Card: chance on any hit to stun the target (one stun-roll per hit).
	if pct := cs.game.cardStunOnHitPct(); pct > 0 && rand.Intn(100) < pct {
		cs.applyStunDR(monster, 1, framesPerTurn, true)
	}
}

// tryApplyWeaponHitRiders applies every on-hit weapon rider to a surviving
// target: stun (existing), armor shred (Pit Labrys) and root (Retiarius
// Trident). The single rider entry point for melee and projectile hits.
func (cs *CombatSystem) tryApplyWeaponHitRiders(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) {
	cs.tryApplyWeaponStun(monster, weaponDef)
	if monster == nil || weaponDef == nil {
		return
	}
	framesPerTurn := cs.game.config.GetTPS()
	if framesPerTurn <= 0 {
		framesPerTurn = 60
	}
	if weaponDef.ArmorShredPct > 0 && weaponDef.ArmorShredSeconds > 0 {
		// ~2s per TB turn, matching the stun seconds<->turns convention.
		monster.ApplyArmorShred(weaponDef.ArmorShredPct,
			weaponDef.ArmorShredSeconds*framesPerTurn, (weaponDef.ArmorShredSeconds+1)/2)
	}
	if weaponDef.RootChance > 0 && weaponDef.RootSeconds > 0 && rand.Float64() < weaponDef.RootChance {
		frames := weaponDef.RootSeconds * framesPerTurn
		turns := (weaponDef.RootSeconds + 1) / 2
		// Refresh-never-shortens, like the trap root it mirrors.
		if frames > monster.RootFramesRemaining {
			monster.RootFramesRemaining = frames
		}
		if turns > monster.RootTurnsRemaining {
			monster.RootTurnsRemaining = turns
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s is pinned in place!", monster.Name))
	}
}

// effectiveMonsterArmor is the AC combat math must mitigate against: the
// monster's shred-adjusted armor, further reduced by the weapon's flat
// armor_pierce_pct (Lion-Crest Warhammer).
func effectiveMonsterArmor(monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) int {
	ac := monster.EffectiveArmorClass()
	if weaponDef != nil && weaponDef.ArmorPiercePct > 0 {
		ac = ac * (100 - weaponDef.ArmorPiercePct) / 100
	}
	return ac
}

// weaponStunnedBonusMultiplier returns the Gladius-style damage multiplier
// against a currently-stunned target (1.0 when not applicable).
func weaponStunnedBonusMultiplier(weaponDef *config.WeaponDefinitionConfig, monster *monsterPkg.Monster3D) float64 {
	if weaponDef == nil || weaponDef.BonusVsStunned <= 0 || weaponDef.BonusVsStunned == 1.0 {
		return 1.0
	}
	if monster.StunTurnsRemaining > 0 || monster.StunFramesRemaining > 0 {
		return weaponDef.BonusVsStunned
	}
	return 1.0
}

// tryCardPoisonProc rolls the Venom-proc cards' (rat/spider/forest_spider/
// masked serpent dancer) on-hit poison chance against a struck monster.
// Undead are immune, matching the genre convention (and this game's own
// mind/body/light resist baseline for the type).
func (cs *CombatSystem) tryCardPoisonProc(monster *monsterPkg.Monster3D) {
	if monster == nil || monster.MonsterType == "undead" {
		return
	}
	chancePct, durationSec := cs.game.cardPoisonProc()
	if chancePct <= 0 || rand.Intn(100) >= chancePct {
		return
	}
	frames := cs.game.config.GetTPS() * durationSec
	monster.ApplyPoison(frames)
	cs.game.AddCombatMessage(fmt.Sprintf("%s is poisoned!", monster.Name))
}

// checkPerspectiveScaledCollision checks if a projectile collides with a monster using perspective-scaled bounding boxes
func (cs *CombatSystem) checkPerspectiveScaledCollision(entityID string, projectile interface{}, projectileType string, monster *monsterPkg.Monster3D) bool {
	// Get projectile graphics info for scaling
	baseSize, minSize, maxSize, ok := cs.getProjectileGraphicsInfo(projectile, projectileType)
	if !ok {
		return false
	}

	// Get collision entities
	projEntity := cs.game.collisionSystem.GetEntityByID(entityID)
	monsterCollisionEntity := cs.game.collisionSystem.GetEntityByID(monster.ID)
	if projEntity == nil || monsterCollisionEntity == nil {
		return false
	}

	// Calculate perspective-scaled collision boxes
	projX, projY := cs.getProjectilePosition(projectile, projectileType)
	projScale := cs.calculatePerspectiveScale(projX, projY, baseSize, minSize, maxSize)
	scaledProjW := projEntity.BoundingBox.Width * projScale
	scaledProjH := projEntity.BoundingBox.Height * projScale

	// Monster scaling
	monsterMultiplier := float64(cs.game.config.Graphics.Monster.SizeDistanceMultiplier)
	monsterScale := cs.calculatePerspectiveScale(monster.X, monster.Y, monsterMultiplier,
		cs.game.config.Graphics.Monster.MinSpriteSize, cs.game.config.Graphics.Monster.MaxSpriteSize)
	scaledMonsterW := monsterCollisionEntity.BoundingBox.Width * monsterScale
	scaledMonsterH := monsterCollisionEntity.BoundingBox.Height * monsterScale

	// Check collision with perspective-scaled boxes
	scaledProjBox := collision.NewBoundingBox(projX, projY, scaledProjW, scaledProjH)
	scaledMonsterBox := collision.NewBoundingBox(monster.X, monster.Y, scaledMonsterW, scaledMonsterH)
	return scaledProjBox.Intersects(scaledMonsterBox)
}

// markMonsterHit applies the side effects every hit shares regardless of source
// (melee, projectile, splash, nova, trap, steam): the damage flash, freeing a
// Charmed monster, and the explicit turn-based same-kind pack response.
func (cs *CombatSystem) markMonsterHit(m *monsterPkg.Monster3D) {
	m.HitTintFrames = MonsterHitFlashFrames
	cs.breakPacifyOnHit(m)
	cs.engageTurnBasedSameKindPackOnPartyHit(m)
}

// finishMonsterKill records a slain monster for the end-of-frame removal sweep
// (removeDeadMonstersByID, which also unregisters its collision entity) and
// awards the kill's XP/gold. Returns the XP awarded, for the kill message.
func (cs *CombatSystem) finishMonsterKill(m *monsterPkg.Monster3D) int {
	cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, m.ID)
	cs.scatterBandOnMemberDeath(m)
	if m.IsChampion() {
		cs.recordChampionVictory(m)
	}
	return cs.awardExperienceAndGold(m)
}

// finishMonsterKillImmediately finalizes a death that must stop colliding in
// the current frame (traps, crossfire projectiles, direct monster-vs-monster
// hits). Rewards still flow through finishMonsterKill, including champions.
func (cs *CombatSystem) finishMonsterKillImmediately(m *monsterPkg.Monster3D) {
	if m == nil {
		return
	}
	cs.game.collisionSystem.UnregisterEntity(m.ID)
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
			// scatter, become sticky-hostile, and never resume their post.
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
	if monster == nil || cs.game.party == nil || len(cs.game.party.Members) == 0 {
		return 0
	}
	// A card ally is a pure summon, never an enemy: its death credits the party
	// with nothing (no XP, gold, or loot). THE single gate for that rule, so
	// every death path (melee, projectile, splash) honours it automatically.
	if isCardAlly(monster) {
		return 0
	}

	xpAwarded := monster.Experience
	if monster.SummonedBy != "" && !monster.CharmedByParty {
		xpAwarded = 0
	}

	// Each living hero - active, reserve, or captive - gets the per-member share.
	if xpAwarded > 0 {
		cs.game.grantSharedXP(xpAwarded / len(cs.game.party.Members))
	}

	// Check for loot drops
	drops := cs.checkMonsterLootDrop(monster)

	// Update quest progress
	cs.updateQuestProgress(monster)

	// Revenge: a slain patron (DeathRalliesType) sends every live map monster of
	// that type into a relentless map-wide hunt.
	cs.rallyOnPatronDeath(monster)

	// Drop gold/items into a loot bag on the ground (fixed size from config, not
	// scaled by the monster).
	if monster.Gold > 0 || len(drops) > 0 {
		gold := monster.Gold
		if pct := cs.game.cardGoldFindPct(); pct != 0 && gold > 0 {
			gold = gold * (100 + pct) / 100 // Jungle Goblin Card
		}
		cs.game.addLootBagDrop(monster.X, monster.Y, drops, gold)
	}

	return xpAwarded
}

// rallyOnPatronDeath: when a monster carrying DeathRalliesType dies, every other
// LIVE monster on the map whose Type matches flies into a relentless map-wide
// hunt for the party (the Relentless flag drives pursueRelentlessly, ignoring
// detection range - and it persists across reload). The orc Warlord's death
// turns the masked Amazons (type "human") vengeful; goblins/beasts are untouched.
func (cs *CombatSystem) rallyOnPatronDeath(dead *monsterPkg.Monster3D) {
	if dead == nil || dead.DeathRalliesType == "" || cs.game == nil || cs.game.world == nil {
		return
	}
	rallied := 0
	for _, m := range cs.game.world.Monsters {
		if m == nil || m == dead || !m.IsAlive() || m.Relentless || m.MonsterType != dead.DeathRalliesType {
			continue
		}
		m.Relentless = true
		m.WasAttacked = true // sticky hostility, persisted
		m.BeginPlayerEngagement()
		rallied++
	}
	if rallied > 0 {
		cs.game.AddCombatMessage(fmt.Sprintf("%s falls - its retainers turn on you in a vengeful fury!", dead.Name))
	}
}

// updateQuestProgress updates quest progress when a monster is killed
func (cs *CombatSystem) updateQuestProgress(monster *monsterPkg.Monster3D) {
	if cs.game.questManager == nil {
		return
	}
	if monster.QuestProgressIgnored {
		return
	}

	// Convert monster name to lowercase key format (e.g., "Goblin" -> "goblin", "Dire Wolf" -> "dire_wolf")
	monsterType := strings.ToLower(strings.ReplaceAll(monster.Name, " ", "_"))

	// Only statue-summoned dragons count toward the win quest. They're flagged
	// at summon (IsEncounterMonster + EncounterRewards.QuestID == "dragon_slayer");
	// any other dragon is ignored so it can never trip the victory. The 4 elite
	// dragons are all named "Elder Dragon" -> monsterType "elder_dragon".
	if monsterType == "elder_dragon" {
		summoned := monster.IsEncounterMonster && monster.EncounterRewards != nil &&
			monster.EncounterRewards.QuestID == "dragon_slayer"
		if !summoned {
			return
		}
	}

	completedQuests := cs.game.questManager.OnMonsterKilled(monsterType, currentMapKey())
	cs.game.syncExterminationQuestProgressForTarget(monsterType)

	// Notify player of quest completions
	for _, quest := range completedQuests {
		if quest.ID == "dragon_slayer" {
			cs.game.AddCombatMessage(fmt.Sprintf("Quest '%s' completed!", quest.Definition.Name))
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Open Quests (J) to claim reward.", quest.Definition.Name))
		}
	}

	// Map-scoped kill quests also complete the moment the map is cleared of
	// targets (counter notwithstanding), and completions may change the world
	// (e.g. the wolf-cull bridge).
	cs.game.completeExterminationQuests(monsterType)
	cs.game.applyCompletedQuestTiles()
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

			// Recalculate derived stats (health and mana increase with level)
			character.CalculateDerivedStats(cs.game.config)

			// Restore full health and mana on level up
			character.HitPoints = character.MaxHitPoints
			character.SpellPoints = character.MaxSpellPoints

			if announce {
				message := fmt.Sprintf("%s reached level %d! (was level %d) [+%d stat points]",
					character.Name, character.Level, oldLevel, StatPointsPerLevel)
				cs.game.AddCombatMessage(message)
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
func (cs *CombatSystem) CalculateWeaponDamage(weapon items.Item, character *character.MMCharacter) (int, int, int) {
	weaponDef := lookupWeaponConfigByName(weapon.Name)
	if weaponDef == nil {
		return 0, 0, 0
	}
	baseDamage := weaponDef.Damage
	// Weapon-category mastery no longer adds to this (normal, armor-reduced,
	// dodgeable) damage - it now grants flat TRUE damage applied at the hit site
	// (weaponMasteryStrike), which bypasses armor and lands through dodges.
	// ArmsMaster: general weapon expertise - flat bonus with ANY weapon.
	baseDamage += character.ArmsMasterTier() * ArmsMasterDamagePerTier

	// Stat scaling resolves through the SAME stat-by-name lookup the tooltip
	// uses (getEffectiveStatValue, all seven stats) - a hand-rolled switch here
	// once silently mapped Speed weapons to Might while the tooltip said
	// "Scales with Speed". Stat names are validated at weapons.yaml load.
	primaryStat := weaponDef.BonusStat
	if primaryStat == "" {
		primaryStat = "Might" // default for weapons without bonus stat specified
	}
	primaryStatBonus := getEffectiveStatValue(primaryStat, character) / WeaponPrimaryStatDivisor

	var secondaryStatBonus int
	if weaponDef.BonusStatSecondary != "" {
		secondaryStatBonus = getEffectiveStatValue(weaponDef.BonusStatSecondary, character) / WeaponSecondaryStatDivisor
	}

	totalStatBonus := primaryStatBonus + secondaryStatBonus
	totalDamage := baseDamage + totalStatBonus
	return baseDamage, totalStatBonus, totalDamage
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

// weaponMasteryStrike returns the TRUE-damage bonus and dodge-ignore flag for
// the given attacker wielding the given weapon. True damage bypasses the
// target's armor class and lands even through a Perfect Dodge; a Grandmaster
// (tier 3) makes the WHOLE strike ignore the target's Perfect Dodge.
func (cs *CombatSystem) weaponMasteryStrike(attacker *character.MMCharacter, weaponDef *config.WeaponDefinitionConfig) (trueDmg int, ignoreDodge bool) {
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

// spellResistPierce returns the resistance-pierce percent for the given
// caster's spell: MagicGMResistPiercePct if they are Grandmaster in that
// spell's school, else 0.
func (cs *CombatSystem) spellResistPierce(caster *character.MMCharacter, spellType string) int {
	if caster == nil {
		return 0
	}
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellType))
	if err != nil || def.School == "" {
		return 0
	}
	school := character.MagicSchoolID(def.School)
	if ms, ok := caster.MagicSchools[school]; ok && ms != nil && ms.Mastery >= character.MasteryGrandMaster {
		return MagicGMResistPiercePct
	}
	return 0
}

// effectiveSpellCost applies a Grandmaster meditator's flat percent spell-cost
// reduction. Single source used by every SP check/deduction site.
func (cs *CombatSystem) effectiveSpellCost(caster *character.MMCharacter, baseCost int) int {
	if caster != nil && caster.SkillTier(character.SkillMeditation) >= int(character.MasteryGrandMaster) {
		baseCost = baseCost * (100 - MeditationGMSpellCostReductionPct) / 100
	}
	return baseCost
}

// CalculateElementalSpellDamage calculates damage for fire/air/water/earth spells
func (cs *CombatSystem) CalculateElementalSpellDamage(spellPoints int, char *character.MMCharacter) (int, int, int) {
	baseDamage := spellPoints * spells.SpellDamagePerSP
	intellectBonus := char.GetEffectiveIntellect() / spells.SpellIntellectDivisor
	totalDamage := baseDamage + intellectBonus
	return baseDamage, intellectBonus, totalDamage
}

// CalculateSteamZoneTickDamage is the per-tick damage of a persistent damage zone
// (Hot Steam), scaled by the caster like the elemental spells: the YAML
// zone_tick_damage is the flat base, plus Intellect/divisor and the caster's
// school mastery. Single source of truth for the cast (tryCastSteamZone) and the
// tooltip, so the displayed number always matches the damage dealt.
func (cs *CombatSystem) CalculateSteamZoneTickDamage(def spells.SpellDefinition, char *character.MMCharacter) int {
	tick := def.ZoneTickDamage
	if char != nil {
		tick += char.GetEffectiveIntellect() / spells.SpellIntellectDivisor
		tick += cs.spellMasteryBonus(char, def.ID)
	}
	return tick
}

// spellMasteryBonus returns +5 per mastery level for the spell's school.
func (cs *CombatSystem) spellMasteryBonus(char *character.MMCharacter, spellID spells.SpellID) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil || def.School == "" {
		return 0
	}
	school := character.MagicSchoolID(def.School)
	if skill, exists := char.MagicSchools[school]; exists {
		return int(skill.Mastery) * MasterySpellEffectPerLevel
	}
	return 0
}

// CalculateCriticalChance calculates critical hit bonus from character stats
func (cs *CombatSystem) CalculateCriticalChance(char *character.MMCharacter) int {
	// Use effective Luck so Bless/stat bonuses influence crit chance. Feeds both
	// weapon crit (CalculateWeaponCritChance) and spell crit (RollCriticalChance),
	// so the Ronin Marksman Card's bonus applies to both for free.
	return char.GetEffectiveLuck()/LuckToCritDivisor + cs.game.cardCritBonusPct()
}

// RollCriticalChance returns whether an attack critically hits and the total crit chance used.
// totalCrit = baseCrit + Luck-derived bonus, clamped to [0,100].
func (cs *CombatSystem) RollCriticalChance(baseCrit int, chr *character.MMCharacter) (bool, int) {
	bonus := cs.CalculateCriticalChance(chr)
	total := baseCrit + bonus
	if total < 0 {
		total = 0
	}
	if total > 100 {
		total = 100
	}
	roll := rand.Intn(100)
	return roll < total, total
}

// RollWeaponCriticalChance rolls a weapon crit using the same total chance shown in tooltips.
func (cs *CombatSystem) RollWeaponCriticalChance(weapon items.Item, chr *character.MMCharacter) (bool, int) {
	total := cs.CalculateWeaponCritChance(weapon, chr)
	roll := rand.Intn(100)
	return roll < total, total
}

// monsterImmuneToDisintegrate reports whether a monster cannot be instakilled by
// any disintegrate effect (spell or weapon proc). Driven entirely by the
// monster's `type` (data) - undead and dragons are immune.
// Bosses (incl. quest-gated evasive ones) are deliberately NOT immune: winning
// the 15% Disintegrate lottery against the Golden Thief Bug before the valve
// quest is an accepted jackpot, not a bug.
func monsterImmuneToDisintegrate(m *monsterPkg.Monster3D) bool {
	if m == nil {
		return false
	}
	// An invulnerable boss (sealed or idol-warded) can't be instakilled.
	return m.MonsterType == "undead" || m.MonsterType == "dragon" || bossInvulnerable(m)
}

// bossInvulnerable reports whether a boss is currently immune to ALL damage: a
// sealed (dormant) boss until its quest unseals it, or an idol-warded boss until
// its idols fall. Every indirect-damage path (AoE splash, inferno, zones, traps)
// skips such a monster so it takes no damage and triggers no side effects.
func bossInvulnerable(m *monsterPkg.Monster3D) bool {
	return m != nil && (m.BossDormant || m.BossWarded)
}

// absorbIfSealed reports whether the monster is an invulnerable boss and, if so,
// plays the muted "blow absorbed" beat (impact spark + one message). Player damage
// hubs call this and return early; TakeDamageResist returning 0 is the backstop
// for paths that don't pre-check (AoE splash, mastery, monster-vs-monster).
func (cs *CombatSystem) absorbIfSealed(m *monsterPkg.Monster3D) bool {
	if m == nil {
		return false
	}
	switch {
	case m.BossDormant:
		cs.game.spawnImpactSparks(m.X, m.Y)
		cs.game.AddCombatMessage(fmt.Sprintf("The seal holds - %s is impervious.", m.Name))
		return true
	case m.BossWarded:
		cs.game.spawnImpactSparks(m.X, m.Y)
		cs.game.AddCombatMessage(fmt.Sprintf("The idols' ward holds - %s is impervious. Shatter the idols!", m.Name))
		return true
	}
	return false
}

// tryCastSpecialEffect runs the data-driven "effect spell" dispatchers in order
// (AoE stun -> party buffs -> resurrect). Each returns false unless the spell
// carries its trigger field, so the OR-chain stops at the first that handles
// the cast. Returns true if one did - callers must then skip the
// projectile/utility paths. Single place to register a new effect-spell type.
func (cs *CombatSystem) tryCastSpecialEffect(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	return cs.tryCastAoeStun(spellID, def) ||
		cs.tryCastInferno(def) ||
		cs.tryCastSteamZone(spellID, def, caster) ||
		cs.tryCastPartyBuff(spellID, def, caster) ||
		cs.tryCastRaiseDead(def, caster) ||
		cs.tryCastResurrect(def, caster) ||
		cs.tryCastAwaken(def, caster)
}

// tryCastInferno handles party-centered nova spells (Inferno): every monster AND
// every party member takes the spell's full damage (cost x SpellDamagePerSP).
// MapWide burns the ENTIRE current map - no radius; the party always burns too
// (fire resistance is the intended answer). Gated on either trigger field.
func (cs *CombatSystem) tryCastInferno(def spells.SpellDefinition) bool {
	if def.PartyAoeRadiusTiles <= 0 && !def.MapWide {
		return false
	}
	dmg := def.SpellPointsCost * spells.SpellDamagePerSP
	radius := math.Inf(1) // MapWide: every monster on the map
	if !def.MapWide {
		radius = def.PartyAoeRadiusTiles * float64(cs.game.config.GetTileSize())
	}
	cx, cy := cs.game.camera.X, cs.game.camera.Y
	damageTypeStr := normalizeDamageTypeStr(def.School)
	damageType := convertToMonsterDamageType(damageTypeStr)
	monsterDmg := dmg + cs.game.combatBuffOutBonusForDamageType(damageTypeStr)

	cs.game.AddCombatMessage(fmt.Sprintf("%s erupts around the party!", def.Name))

	// Monsters in range. A sealed (dormant) boss is invulnerable and inert -
	// skip it so the nova neither damages nor wakes it.
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || bossInvulnerable(m) || Distance(cx, cy, m.X, m.Y) > radius {
			continue
		}
		reduced := applyMonsterArmor(monsterDmg, damageTypeStr, m.EffectiveArmorClass(), false)
		m.TakeDamageResist(reduced, damageType, 0)
		cs.markMonsterHit(m)
		cs.spawnMonsterHitBurst(m, damageTypeStr)
		if !m.IsAlive() {
			cs.game.collisionSystem.UnregisterEntity(m.ID)
			xpAwarded := cs.finishMonsterKill(m)
			cs.game.AddCombatMessage(fmt.Sprintf("%s is consumed by %s! (+%d XP)", m.Name, def.Name, xpAwarded))
		}
	}

	// The party is caught in the blast too (each member's resistances apply).
	cs.forEachDamageablePartyMember(func(idx int, member *character.MMCharacter) {
		dealt := cs.damagePartyMemberElement(idx, member, dmg, damageTypeStr)
		cs.game.AddCombatMessage(fmt.Sprintf("%s is scorched for %d! (HP: %d/%d)",
			member.Name, dealt, member.HitPoints, member.MaxHitPoints))
		cs.game.TriggerPartyFlame(idx) // flame-particle overlay on the burned card
	})
	return true
}

// tryCastRaiseDead handles Raise Dead: revives the first fallen ally that is
// Unconscious or Dead (NOT eradicated - that's Resurrect's domain) to
// ReviveHpPct% of max HP, clearing both conditions. Returns true if it handled
// the spell. Gated on ReviveHpPct > 0 so it never collides with Resurrect.
func (cs *CombatSystem) tryCastRaiseDead(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.ReviveHpPct <= 0 {
		return false
	}
	var target *character.MMCharacter
	for _, m := range cs.game.party.Members {
		if m == nil || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		if m.HasCondition(character.ConditionUnconscious) || m.HasCondition(character.ConditionDead) || m.HitPoints <= 0 {
			target = m
			break
		}
	}
	if target == nil {
		// Nothing to raise - refund the SP actually paid (matches Resurrect/Awaken).
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		cs.game.AddCombatMessage("There is no fallen ally to raise.")
		return true
	}
	target.RemoveCondition(character.ConditionUnconscious)
	target.RemoveCondition(character.ConditionDead)
	hp := target.MaxHitPoints * def.ReviveHpPct / 100
	if hp < 1 {
		hp = 1
	}
	target.HitPoints = hp
	cs.game.AddCombatMessage(fmt.Sprintf("%s is raised to %d HP!", target.Name, hp))
	return true
}

// tryCastAwaken handles the Awaken spell: rouses EVERY unconscious party member
// back to 1 HP (does not touch the truly dead/eradicated - that's Resurrect).
// Shared by both cast paths. Returns true if it handled the spell.
func (cs *CombatSystem) tryCastAwaken(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if !def.Awaken {
		return false
	}
	revived := 0
	for _, m := range cs.game.party.Members {
		if m == nil || !m.HasCondition(character.ConditionUnconscious) {
			continue
		}
		m.RemoveCondition(character.ConditionUnconscious)
		if m.HitPoints < 1 {
			m.HitPoints = 1
		}
		revived++
	}
	if revived == 0 {
		// No one to wake - refund the SP actually paid (matches Meditation discount).
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		cs.game.AddCombatMessage("No one is unconscious to awaken.")
		return true
	}
	cs.game.AddCombatMessage(fmt.Sprintf("Awakening rouses %d fallen ally(s) back to 1 HP!", revived))
	return true
}

// tryCastResurrect handles the Resurrect spell: restores the first fallen party
// member (unconscious, dead, or even eradicated) - to full HP if FullHeal.
// Shared by both cast paths. Returns true if it handled the spell.
func (cs *CombatSystem) tryCastResurrect(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if !def.Revive {
		return false
	}
	var target *character.MMCharacter
	for _, m := range cs.game.party.Members {
		if m == nil {
			continue
		}
		if m.HasCondition(character.ConditionUnconscious) ||
			m.HasCondition(character.ConditionDead) ||
			m.HasCondition(character.ConditionEradicated) ||
			m.HitPoints <= 0 {
			target = m
			break
		}
	}
	if target == nil {
		// Nothing to resurrect - refund the spell points actually paid (matches
		// the Meditation-discounted cost so a GM can't farm SP on empty casts).
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		cs.game.AddCombatMessage("There is no fallen ally to resurrect.")
		return true
	}
	target.RemoveCondition(character.ConditionUnconscious)
	target.RemoveCondition(character.ConditionDead)
	target.RemoveCondition(character.ConditionEradicated)
	if def.FullHeal {
		target.HitPoints = target.MaxHitPoints
	} else if target.HitPoints <= 0 {
		target.HitPoints = 1
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s is restored to life!", target.Name))
	return true
}

// ceilStunDRPct scales v by pct% (0-100), rounding UP. turns is usually
// authored as 1 (the smallest nonzero TB unit) - floor division sent it to 0
// at any pct below 100, so the 2nd/3rd stun in a DR chain silently stopped
// skipping a TB turn at all while its much-larger RT-frames twin stayed
// nonzero (stun-star overlay stuck on forever, nothing left to clear it). Only
// pct==0 (the true immune tier) or v<=0 yields exactly 0.
func ceilStunDRPct(v, pct int) int {
	if v <= 0 || pct <= 0 {
		return 0
	}
	return (v*pct + 99) / 100
}

// applyStunDR is the single entry point for stunning a monster. It applies
// DIMINISHING RETURNS: the requested duration is scaled by StunDRFactorsPct for
// the target's current DR chain length (100/50/25/0%), so repeated stuns shrink
// to nothing and then the target is immune until it goes stun-free for the reset
// window. Refreshes the chain + both per-mode reset clocks on every attempt (a
// TB<->RT switch is conservative). announce=false suppresses the per-target line
// for AoE callers that print their own summary. Returns whether it actually stunned.
func (cs *CombatSystem) applyStunDR(m *monsterPkg.Monster3D, turns, frames int, announce bool) bool {
	if m == nil {
		return false
	}
	i := m.StunDRStacks
	if i >= len(StunDRFactorsPct) {
		i = len(StunDRFactorsPct) - 1
	}
	mult := StunDRFactorsPct[i]
	effTurns, effFrames := ceilStunDRPct(turns, mult), ceilStunDRPct(frames, mult)
	wasStunned := m.StunTurnsRemaining > 0 || m.StunFramesRemaining > 0

	// Advance the chain (caps at the immune step) and refresh both reset clocks.
	if m.StunDRStacks < len(StunDRFactorsPct)-1 {
		m.StunDRStacks++
	}
	m.StunDRMemoryTurns = StunDRResetTurns
	m.StunDRMemoryFrames = StunDRResetSeconds * cs.game.config.GetTPS()

	if effTurns <= 0 && effFrames <= 0 { // worn down -> immune this attempt
		if announce && !wasStunned {
			cs.game.AddCombatMessage(fmt.Sprintf("%s resists the stun!", m.Name))
		}
		return false
	}
	if effFrames > m.StunFramesRemaining {
		m.StunFramesRemaining = effFrames
	}
	if effTurns > m.StunTurnsRemaining {
		m.StunTurnsRemaining = effTurns
	}
	if announce && !wasStunned {
		cs.game.AddCombatMessage(fmt.Sprintf("%s is stunned!", m.Name))
	}
	return true
}

// applyStun stuns a single monster for `seconds` real-time and `turns` turn-based
// turns, under diminishing returns (see applyStunDR).
func (cs *CombatSystem) applyStun(m *monsterPkg.Monster3D, seconds, turns int) {
	cs.applyStunDR(m, turns, seconds*cs.game.config.GetTPS(), true)
}

// applyBindUndead (Bind Undead) takes control of an UNDEAD target - it hunts
// other monsters for you and ignores the party. No effect on the living. No
// damage is dealt. A separate, mutually exclusive effect from Pacify (Charm).
func (cs *CombatSystem) applyBindUndead(m *monsterPkg.Monster3D, seconds int, spellName string) {
	if m.MonsterType != "undead" {
		cs.game.AddCombatMessage(fmt.Sprintf("%s washes over %s - only the undead can be bound.", spellName, m.Name))
		return
	}
	m.Bound = true
	m.BoundFramesRemaining = seconds * cs.game.config.GetTPS()
	m.WasAttacked = false
	cs.game.AddCombatMessage(fmt.Sprintf("%s is bound to your will!", m.Name))
}

// applyPacify (Charm) pacifies a LIVING target - it stops attacking and breaks
// free on any hit it takes (see breakPacifyOnHit). No effect on undead, no
// damage. A separate, mutually exclusive effect from Bind Undead.
func (cs *CombatSystem) applyPacify(m *monsterPkg.Monster3D, seconds int, spellName string) {
	if m.MonsterType == "undead" {
		cs.game.AddCombatMessage(fmt.Sprintf("%s has no hold over the undead %s.", spellName, m.Name))
		return
	}
	m.Pacified = true
	m.PacifiedFramesRemaining = seconds * cs.game.config.GetTPS()
	m.CharmedByParty = true
	m.WasAttacked = false
	cs.game.AddCombatMessage(fmt.Sprintf("%s is charmed and stops attacking!", m.Name))
}

// breakPacifyOnHit releases a pacified (Charm) monster the instant it takes any
// hit - it snaps out of the charm and re-aggros. Bound undead are unaffected.
// Called wherever the party deals damage to a monster.
func (cs *CombatSystem) breakPacifyOnHit(m *monsterPkg.Monster3D) {
	if m.Pacified {
		m.Pacified = false
		m.PacifiedFramesRemaining = 0
		m.WasAttacked = true
		m.BeginPlayerEngagement()
		cs.game.AddCombatMessage(fmt.Sprintf("%s breaks free of the charm!", m.Name))
	}
}

// boundAllySeekRadius is the pixel range a bound undead hunts for enemies to
// walk toward (see BoundAllySeekTiles).
func (cs *CombatSystem) boundAllySeekRadius() float64 {
	return BoundAllySeekTiles * float64(cs.game.config.GetTileSize())
}

// monsterCanAttackMonster is the crossfire equivalent of monsterCanAttackParty.
// A melee attacker may hit any adjacent tile even when off-centre tokens are
// farther than a radial 1.5 tiles apart. The movement AI already treats that
// tile adjacency as attack contact; a radial-only gate here made melee champions
// walk up to summons and then never swing.
func (cs *CombatSystem) monsterCanAttackMonster(attacker, target *monsterPkg.Monster3D) bool {
	if attacker == nil || target == nil || !target.IsAlive() {
		return false
	}
	if monsterInAttackTransit(attacker) {
		return false
	}
	if cs != nil && cs.game != nil && cs.game.config != nil {
		tileSize := float64(cs.game.config.GetTileSize())
		if tileSize > 0 && int(attacker.X/tileSize) == int(target.X/tileSize) && int(attacker.Y/tileSize) == int(target.Y/tileSize) {
			return false
		}
	}
	if Distance(attacker.X, attacker.Y, target.X, target.Y) <= attacker.GetAttackRangePixels() {
		return !attacker.HasRangedAttack() || cs.game.collisionSystem == nil ||
			cs.game.collisionSystem.CheckLineOfSight(attacker.X, attacker.Y, target.X, target.Y)
	}
	if attacker.HasRangedAttack() {
		return false
	}
	return cs.monsterMeleeAdjacentToPoint(attacker, target.X, target.Y)
}

// nearestEnemyMonster returns the closest alive ENEMY monster to m within maxDist
// (pixels), or nil. An "enemy" is one the party does not control - i.e. neither
// bound nor pacified. The target of a bound undead and the lure for a normal mob.
func (cs *CombatSystem) nearestEnemyMonster(m *monsterPkg.Monster3D, maxDist float64) *monsterPkg.Monster3D {
	var target *monsterPkg.Monster3D
	best := maxDist
	for _, other := range cs.game.world.Monsters {
		if other == nil || other == m || !other.IsAlive() || other.Bound || other.Pacified ||
			other.BossDormant || other.BossWarded || cs.bossEvasive(other) {
			continue
		}
		if d := Distance(m.X, m.Y, other.X, other.Y); d <= best {
			best, target = d, other
		}
	}
	return target
}

// monsterAIFoeMonster returns the OTHER monster m should pursue and strike, or
// nil if its foe is the party (or it has none):
//   - bound undead: the nearest enemy monster (within the seek radius).
//   - pacified charm: nil (fully passive - never fights).
//   - normal monster: the nearest bound undead within its alert radius, if one is
//     no farther than the party - so mobs turn on the bound undead in their midst.
func (cs *CombatSystem) monsterAIFoeMonster(m *monsterPkg.Monster3D) *monsterPkg.Monster3D {
	if m == nil || m.IsInertSetPiece() || cs.bossEvasive(m) || m.Pacified {
		return nil
	}
	if m.Bound {
		return cs.nearestEnemyMonster(m, cs.boundAllySeekRadius())
	}
	// Normal monster: only bother if any bound undead exist this frame.
	if len(cs.game.boundAllies) == 0 {
		return nil
	}
	// The nearest bound ally (party summon / bound undead) within the SEEK radius
	// competes with the party for aggro. A mob attacks whichever is closer; ties
	// stay with the party. A mob's own alert_radius is deliberately NOT used here:
	// it is often tiny while a ranged summon peppers it from beyond that radius.
	// All monster-vs-summon pursuit/attack then flows through the shared crossfire
	// path (plain mob or champion alike).
	var foe *monsterPkg.Monster3D
	best := Distance(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y)
	if seek := cs.boundAllySeekRadius(); best > seek {
		best = seek
	}
	for _, u := range cs.game.boundAllies {
		if u == nil || !u.IsAlive() {
			continue
		}
		if d := Distance(m.X, m.Y, u.X, u.Y); d < best {
			best, foe = d, u
		}
	}
	return foe
}

// monsterAITargetPoint is the world point a monster should pursue/engage, used by
// both the real-time and turn-based movement. It redirects controlled monsters off
// the party: a pacified charm stands still (targets itself), while a bound ally
// seeks its enemy or follows the party when idle. A normal mob chases its undead
// foe if it has one, else the party. Reads the per-frame cached AIFoe (set in
// refreshBoundAllyCache) - never recomputes the foe.
func (cs *CombatSystem) monsterAITargetPoint(m *monsterPkg.Monster3D) (float64, float64) {
	if m.IsInertSetPiece() {
		return m.X, m.Y
	}
	if m.Pacified {
		return m.X, m.Y // pacified: never chase the party - hold position
	}
	if cs.bossEvasive(m) {
		return m.X, m.Y // evasive boss (quest unfinished): never chases - holds + blinks away
	}
	if m.AIFoe != nil {
		return m.AIFoe.X, m.AIFoe.Y
	}
	if m.Bound {
		return cs.game.camera.X, cs.game.camera.Y // an idle bound ally tags along with the party
	}
	return cs.game.camera.X, cs.game.camera.Y
}

// monsterStrikeMonster resolves one melee hit from attacker onto target (a
// monster-vs-monster blow). On a kill the party is rewarded ONLY if the slain
// monster was an enemy (not a bound ally that a mob just cut down).
func (cs *CombatSystem) monsterStrikeMonster(attacker, target *monsterPkg.Monster3D) {
	cs.strikeMonsterFor(attacker, target, cs.monsterAttackDamage(attacker), monsterPkg.DamagePhysical)
}

// strikeMonsterFor lands a monster-vs-monster blow of an EXPLICIT damage and
// element, with the shared hit-flash / message / kill / reward path. The damage
// sink for both a plain monster's attack (monsterStrikeMonster rolls it) and a
// champion's weapon sweep at summons (one swing roll applied to every caught
// target - the arc/AoE never re-rolls, matching the vs-party rule).
func (cs *CombatSystem) strikeMonsterFor(attacker, target *monsterPkg.Monster3D, dmg int, dtype monsterPkg.DamageType) {
	if !target.IsAlive() {
		return // already slain this frame - no double damage/reward
	}
	actual := target.TakeDamage(dmg, dtype)
	target.HitTintFrames = MonsterHitFlashFrames
	verb := "strikes"
	if attacker.Bound {
		verb = "(bound) strikes"
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s %s %s for %d!", attacker.Name, verb, target.Name, actual))
	if target.IsAlive() {
		return
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s slays %s!", attacker.Name, target.Name))
	cs.finishMonsterKillImmediately(target)
}

// boundAttackNearest makes a bound undead attack the nearest enemy monster -
// but ONLY when that enemy is within real attack range (melee: reach; ranged:
// bolt range). It searches a wider seek radius: if the nearest enemy is found but
// still out of attack range it returns false, so the caller walks the undead
// toward it (it hunts instead of striking across the room). Returns true only
// when it actually attacked.
func (cs *CombatSystem) boundAttackNearest(m *monsterPkg.Monster3D) bool {
	target := m.AIFoe // precomputed this frame (= nearest enemy within seek radius)
	if target == nil || !target.IsAlive() {
		return false
	}
	if !cs.monsterCanAttackMonster(m, target) {
		return false // in sight but out of reach - close the distance first
	}
	if !cs.game.tryClaimMonsterAttackPost(m) {
		return false
	}
	m.State = monsterPkg.StateAttacking
	// Ranged bound undead (e.g. a lich) loose a visible bolt at the enemy; the hit
	// is resolved on impact in CheckProjectileMonsterCollisions. Melee ones strike
	// directly.
	m.AttackAnimFrames = MonsterAttackAnimFrames
	if m.HasRangedAttack() {
		cs.spawnMonsterRangedAttackAtMonster(m, target, ProjectileOwnerBoundUndead)
	} else {
		cs.monsterStrikeMonster(m, target)
	}
	return true
}

// awardExperienceOnly grants the party a monster's XP with NO gold or loot - used
// when a bound (charmed) monster perishes as the party leaves the map.
func (cs *CombatSystem) awardExperienceOnly(monster *monsterPkg.Monster3D) {
	if monster == nil || monster.SummonedBy != "" || cs.game.party == nil || len(cs.game.party.Members) == 0 {
		return
	}
	// Same per-member share as awardExperienceAndGold, but no gold/loot. Routed
	// through grantSharedXP so Learning bonuses and bench training apply uniformly.
	cs.game.grantSharedXP(monster.Experience / len(cs.game.party.Members))
}

// tryCastAoeStun handles AoE-stun effect spells (e.g. Darkness): if the spell
// has StunRadiusTiles > 0, every alive monster within that radius of the caster
// is stunned (RT frames + TB turns), no damage dealt. Shared by both cast
// paths. Returns true if it handled the spell (caller should stop).
func (cs *CombatSystem) tryCastAoeStun(spellID spells.SpellID, def spells.SpellDefinition) bool {
	if def.StunRadiusTiles <= 0 {
		return false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	radius := def.StunRadiusTiles * tileSize
	frames := def.StunDurationSeconds * cs.game.config.GetTPS()
	turns := def.StunDurationTurns
	stunned := 0
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		if Distance(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y) > radius {
			continue
		}
		if cs.applyStunDR(m, turns, frames, false) { // per-target DR; summary printed below
			stunned++
		}
	}
	// Flavor lead comes from the spell's own `message:` (Darkness engulfs, a
	// shockwave rips...); the count suffix is shared.
	lead := def.Message
	if lead == "" {
		lead = def.Name
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s - %d foe(s) stunned!", lead, stunned))
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

func spellMasteryTierForSchool(caster *character.MMCharacter, schoolID string) int {
	if caster == nil || schoolID == "" {
		return 0
	}
	school := character.MagicSchoolID(schoolID)
	if skill, ok := caster.MagicSchools[school]; ok && skill != nil {
		return int(skill.Mastery)
	}
	return 0
}

func scaledSpellMasteryValue(def spells.SpellDefinition, caster *character.MMCharacter, base, max int) int {
	if base <= 0 || max <= base {
		return base
	}
	tier := spellMasteryTierForSchool(caster, def.School)
	gmTier := int(character.MasteryGrandMaster)
	if tier <= 0 || gmTier <= 0 {
		return base
	}
	if tier > gmTier {
		tier = gmTier
	}
	return base + (max-base)*tier/gmTier
}

func scaledIncomingDamageReduction(def spells.SpellDefinition, caster *character.MMCharacter) int {
	return scaledSpellMasteryValue(def, caster, def.IncomingDamageReduction, def.IncomingDamageReductionGrandmaster)
}

// tryCastPartyBuff handles party combat-buff spells (Day of the Gods, Hour of
// Power, Stone Skin). If the spell carries any party-buff field it activates the
// buff for `duration` seconds and returns true. Shared by both cast paths.
func (cs *CombatSystem) tryCastPartyBuff(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.ResistBuffPct <= 0 && def.OutgoingDamageBonus <= 0 && def.IncomingDamageReduction <= 0 && def.ResistBuffSchoolPct <= 0 {
		return false
	}
	// Party-buff magnitudes may opt into mastery scaling with *_grandmaster
	// caps; spells without a cap stay flat at their authored base value.
	frames := cs.CalculateSpellDurationFrames(spellID, caster)
	cs.game.addCombatBuff(TimedCombatBuff{
		SpellID:         string(spellID),
		Frames:          frames,
		OutBonus:        scaledSpellMasteryValue(def, caster, def.OutgoingDamageBonus, def.OutgoingDamageBonusGrandmaster),
		OutDamageType:   def.OutgoingDamageType,
		InReduce:        scaledIncomingDamageReduction(def, caster),
		ResistPct:       scaledSpellMasteryValue(def, caster, def.ResistBuffPct, def.ResistBuffPctGrandmaster),
		ResistSchool:    def.ResistBuffSchool,
		ResistSchoolPct: def.ResistBuffSchoolPct,
	})
	cs.game.AddCombatMessage(fmt.Sprintf("%s empowers the party!", def.Name))
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

// spellStatBuffBonuses resolves the stat-buff block a cast of spellID grants:
// per-stat `stat_bonuses:` maps are authored absolute; uniform `stat_bonus`
// may opt into mastery scaling with stat_bonus_grandmaster.
func (cs *CombatSystem) spellStatBuffBonuses(spellID spells.SpellID, caster *character.MMCharacter) character.StatBonuses {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return character.StatBonuses{}
	}
	if len(def.StatBonuses) > 0 {
		return character.StatBonusesFromMap(def.StatBonuses)
	}
	return character.UniformStatBonuses(cs.CalculateSpellStatBonus(spellID, caster))
}

// applyStatBuffSpell registers a stat-buff spell in the timed registry:
// different spells stack, recasting the same one refreshes it.
func (cs *CombatSystem) applyStatBuffSpell(spellID spells.SpellID, duration int, bonuses character.StatBonuses) {
	cs.game.addStatBuff(TimedStatBuff{SpellID: string(spellID), Frames: duration, Bonuses: bonuses})
}

// RollPerfectDodge returns whether the character performs a perfect dodge and the chance used.
// chance = effective Luck / LuckToDodgeDivisor, clamped to [0,100].
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

func (cs *CombatSystem) RollPerfectDodge(chr *character.MMCharacter) (bool, int) {
	// Use effective stats so Bless and equipment affect dodge
	chance := chr.GetEffectiveLuck()/LuckToDodgeDivisor + cs.armorGMDodgeBonus(chr) + cs.game.cardDodgeBonusPct()
	if chance < 0 {
		chance = 0
	}
	if chance > 100 {
		chance = 100
	}
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
	return armorMitigationPctFromAC(cs.CalculateTotalArmorClass(char), physical)
}

// mitigateCharacterDamage reduces incoming damage to a party member through the
// fixed pipeline:
//
//  1. Armor   - % mitigation (cap 75% physical / 33% elemental); skipped on
//     armor-pierce (ranged crit) and true damage (ignoreArmor).
//  2. Resist  - per-school gear resist + party resist buff, capped 100%
//     (100% == true immunity -> 0 damage).
//  3. Flat    - additive reductions (DisarmTrap placeholder + Hour of Power /
//     Stone Skin), applied together AFTER the % steps; CAN drive damage to 0.
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

func (cs *CombatSystem) mitigateCharacterDamage(damage int, damageTypeStr string, char *character.MMCharacter, ignoreArmor bool) int {
	if damage <= 0 || char == nil {
		return damage
	}
	school := normalizeDamageTypeStr(damageTypeStr)
	physical := school == monsterPkg.DamageSchoolPhysical

	// 1) Armor (% mitigation; also blunts elemental on a scaled-down curve).
	if !ignoreArmor {
		if mit := cs.armorMitigationPct(char, physical); mit > 0 {
			damage = damage * (100 - mit) / 100
		}
	}
	// 2) Resistance: the single school-resist total (gear + party buff + per-school
	//    buffs + card wards), computed once so the character sheet shows exactly
	//    what reduces the hit. 100% = immune.
	resist := cs.game.schoolResistPct(char, school)
	if resist > 0 {
		damage = damage * (100 - resist) / 100
	}
	if resist >= 100 {
		return 0 // true immunity
	}
	// The % steps alone never fully negate a real hit - keep a 1-damage chip...
	if damage < 1 {
		damage = 1
	}
	// 3) ...then the flat reductions (DisarmTrap + Hour of Power / Stone Skin),
	//    which CAN finish a hit off to 0. This deliberately covers only direct
	//    mitigable hits: true damage is added after this function, while poison
	//    and burn tick HP directly, so Disarm Trap does not reduce either.
	damage -= char.DisarmTrapTier() * DisarmTrapDamageReductionPerTier
	damage -= cs.game.combatBuffInReduce()
	if damage < 0 {
		damage = 0
	}
	return damage
}

// PhysicalMitigation is the breakdown of how an incoming PHYSICAL hit is reduced,
// in the exact order mitigateCharacterDamage applies it. The percentage steps and
// the floor make the result depend on the incoming hit, so the UI renders the
// pipeline, not a single total.
type PhysicalMitigation struct {
	ArmorClass int // total AC across equipped armor
	ArmorPct   int // armor % mitigation vs physical (capped 75)
	ResistPct  int // physical resistance % (gear + party buff, capped 100; 100 = immune)
	SkillFlat  int // flat reduction from DisarmTrap tier, applied AFTER the % steps with FlatBuff
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
		SkillFlat:  char.DisarmTrapTier() * DisarmTrapDamageReductionPerTier,
		ResistPct:  cs.game.schoolResistPct(char, monsterPkg.DamageSchoolPhysical),
		FlatBuff:   cs.game.combatBuffInReduce(),
	}
}

func isPhysicalDamageType(damageTypeStr string) bool {
	return normalizeDamageTypeStr(damageTypeStr) == monsterPkg.DamageSchoolPhysical
}

// normalizeDamageTypeStr is the game-side data boundary for damage schools.
// YAML/UI values are strings, while monster damage uses an enum; normalize once
// before either resistance lookup or enum conversion so both stay consistent.
func normalizeDamageTypeStr(damageTypeStr string) string {
	normalized := strings.ToLower(strings.TrimSpace(damageTypeStr))
	if normalized == "" {
		return monsterPkg.DamageSchoolPhysical
	}
	return normalized
}

func weaponDamageTypeStr(weaponDef *config.WeaponDefinitionConfig) string {
	if weaponDef != nil && weaponDef.DamageType != "" {
		return normalizeDamageTypeStr(weaponDef.DamageType)
	}
	return monsterPkg.DamageSchoolPhysical
}

func spellDamageTypeStr(spellType string) string {
	if spellDef, err := spells.GetSpellDefinitionByID(spells.SpellID(spellType)); err == nil {
		return normalizeDamageTypeStr(spellDef.School)
	}
	return monsterPkg.DamageSchoolPhysical
}

func convertToMonsterDamageType(damageTypeStr string) monsterPkg.DamageType {
	damageType := monsterPkg.DamagePhysical
	if monsterPkg.MonsterConfig != nil {
		if ct, err := monsterPkg.MonsterConfig.ConvertDamageType(damageTypeStr); err == nil {
			damageType = ct
		}
	}
	return damageType
}

// applyMonsterArmor reduces a hit by the monster's armor using the SAME % model
// as the party (armorMitigationPctFromAC): physical capped 75%, elemental scaled
// to 33%. A ranged PHYSICAL shot still has ArmorPierceRangedChancePct to bypass
// armor entirely. Armor alone never fully negates a hit (floor 1); resistance
// (TakeDamageResist, applied next) is what can take it to 0.
func applyMonsterArmor(damage int, damageTypeStr string, armorClass int, isRanged bool) int {
	if damage <= 0 || armorClass <= 0 {
		return damage
	}
	physical := isPhysicalDamageType(damageTypeStr)
	if isRanged && physical && rand.Intn(100) < ArmorPierceRangedChancePct {
		return damage // armor-piercing shot
	}
	mit := armorMitigationPctFromAC(armorClass, physical)
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
	skillType, ok := character.ArmorSkillForCategory(strings.ToLower(armor.ArmorCategory))
	if !ok {
		return 0
	}
	if skill, exists := char.Skills[skillType]; exists {
		return int(skill.Mastery) * MasteryArmorACPerLevel
	}
	return 0
}

// checkMonsterLootDrop handles loot drops when monsters are killed
func (cs *CombatSystem) checkMonsterLootDrop(monster *monsterPkg.Monster3D) []items.Item {
	// Resolve loot by the monster's canonical YAML key (always set), NOT by
	// name: several monsters can share a display Name (the four elemental
	// dragons are all "Dragon"), so a name lookup would scramble their loot.
	entries := config.GetLootTable(monster.Key)
	if len(entries) == 0 {
		return nil
	}
	drops := make([]items.Item, 0, len(entries))
	for _, e := range entries {
		if rand.Float64() < e.Chance {
			drop, err := createLootItem(e.Type, e.Key)
			if err != nil {
				fmt.Printf("[WARN] loot drop failed: %v\n", err)
				continue
			}
			drops = append(drops, drop)
		}
	}
	return drops
}

// randomLivingMember returns a uniformly-random alive+conscious party member
// (nil if the whole party is down). Used for MELEE targeting in both modes.
func (cs *CombatSystem) randomLivingMember() *character.MMCharacter {
	alive := alivePartyIndices(cs.game.party.Members)
	if len(alive) == 0 {
		return nil
	}
	return cs.game.party.Members[alive[rand.Intn(len(alive))]]
}

// randomLivingMembers returns up to n DISTINCT living members in random order -
// the target set of a champion's melee arc (each catches the same swing once).
func (cs *CombatSystem) randomLivingMembers(n int) []*character.MMCharacter {
	alive := alivePartyIndices(cs.game.party.Members)
	rand.Shuffle(len(alive), func(i, j int) { alive[i], alive[j] = alive[j], alive[i] })
	if n > len(alive) {
		n = len(alive)
	}
	out := make([]*character.MMCharacter, 0, n)
	for _, idx := range alive[:n] {
		out = append(out, cs.game.party.Members[idx])
	}
	return out
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

// tankTarget is the tank member (front slot, or first survivor). RANGED single
// hits in real time always land here.
func (cs *CombatSystem) tankTarget() *character.MMCharacter {
	if i := cs.tankIndex(); i >= 0 {
		return cs.game.party.Members[i]
	}
	return nil
}

// rangedTBTarget is the turn-based ranged single-target rule: mostly the tank,
// but RangedOffTankChance of the time a random NON-tank living member instead.
func (cs *CombatSystem) rangedTBTarget() *character.MMCharacter {
	ti := cs.tankIndex()
	if ti < 0 {
		return nil
	}
	if rand.Float64() < RangedOffTankChance {
		others := make([]int, 0, len(cs.game.party.Members))
		for i, x := range cs.game.party.Members {
			if i != ti && x != nil && x.HitPoints > 0 {
				others = append(others, i)
			}
		}
		if len(others) > 0 {
			return cs.game.party.Members[others[rand.Intn(len(others))]]
		}
	}
	return cs.game.party.Members[ti]
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

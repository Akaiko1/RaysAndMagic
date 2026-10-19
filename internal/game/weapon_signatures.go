package game

import (
	"math"
	"strings"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/stash"
	"ugataima/internal/status"
)

// The rule and its effective-stat damage travel with the shot. Selection and
// equipment changes cannot change an in-flight hit; Rule points at the loaded
// definition, which a content reload replaces rather than edits, and is nil
// on a shot without a signature.
type weaponSignatureShot struct {
	Name       string
	Rule       *config.WeaponSignatureConfig
	Damage     int
	InstanceID uint64
	Flare      bool
	Returning  bool
	ReturnLeg  bool
	// LegHits are the monsters a returning blade struck on this leg: it
	// pierces each of them once out, and once back.
	LegHits    []string
	HitboxSize float64
	Lifetime   int
	// Runtime-only orientation at the start of the return animation; shots are not saved.
	TurnFromAngle, TurnAtDistance float64
	FlameScale                    float64
}

type runeSwing struct {
	primary *monster.Monster3D
	spent   bool
}

// statScaledDamage resolves a weapon rider's damage against the hero's
// effective stats; without a hero it is the flat part.
func statScaledDamage(s config.StatScaledDamage, ch *character.MMCharacter) int {
	damage := s.Damage
	if ch != nil && s.StatDivisor > 0 {
		damage += max(0, character.EffectiveCombatStats(ch).ValueByName(strings.ToLower(s.ScalingStat))) / s.StatDivisor
	}
	return damage
}

// normalizedSignatureState repairs a saved weapon's signature state: a
// charge-storing signature keeps its charges within the limit, its walked
// distance within one charge step (movement) and a well-formed winding clock
// (Wind-up); any other weapon carries none.
func normalizedSignatureState(def *config.WeaponDefinitionConfig, state items.WeaponSignatureState) items.WeaponSignatureState {
	if !def.StoresCharges() {
		return items.WeaponSignatureState{}
	}
	if def.HasSignature(config.SignatureWindUp) {
		wind := state.Wind
		if wind.Frames < 0 || wind.Turns < 0 || wind.Rate < 0 {
			wind = items.RoundClock{}
		}
		return items.WeaponSignatureState{Charges: min(max(0, state.Charges), def.Signature.ChargeLimit), Wind: wind}
	}
	state.Charges = min(max(0, state.Charges), def.Signature.ChargeLimit)
	distance := state.MoveDistance
	if math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 0 {
		distance = 0
	}
	state.MoveDistance = math.Mod(distance, def.Signature.ChargeMoveTiles)
	return state
}

// chargeSignatureWeapons charges a hero's movement-charged signature weapons
// for movedTiles of party movement, by any means, and for an offensive cast.
func (g *MMGame) chargeSignatureWeapons(ch *character.MMCharacter, movedTiles float64, offensiveCast bool) {
	if ch == nil || !ch.CanUseCombatAction() {
		return
	}
	defs := equippedWeaponDefinitions(ch)
	for i, slot := range equippedWeaponSlots {
		def := defs[i]
		if !def.ChargesByMovement() {
			continue
		}
		item := ch.Equipment[slot]
		rule := def.Signature
		charges := 0
		if offensiveCast && rule.ChargeOnOffensiveCast {
			charges = 1
		}
		if movedTiles > 0 {
			charges += min(rule.ChargeLimit, status.Accrue(&item.Signature.MoveDistance, movedTiles, rule.ChargeMoveTiles))
		}
		if charges > 0 {
			item.Signature.Charges = min(rule.ChargeLimit, max(0, item.Signature.Charges)+charges)
		}
		ch.Equipment[slot] = item
	}
}

// spendSkirmishCharge spends a Skirmish weapon's stored charge on the shot
// being fired, which then always crits; the charge goes on release, hit or miss.
func spendSkirmishCharge(ch *character.MMCharacter, slot items.EquipSlot, def *config.WeaponDefinitionConfig) bool {
	if ch == nil || !def.HasSignature(config.SignatureSkirmish) {
		return false
	}
	item := ch.Equipment[slot]
	if item.Signature.Charges <= 0 {
		return false
	}
	item.Signature.Charges--
	ch.Equipment[slot] = item
	return true
}

func (cs *CombatSystem) snapshotWeaponSignature(shot *Arrow, owner *character.MMCharacter, slot items.EquipSlot, def *config.WeaponDefinitionConfig) {
	if shot == nil || owner == nil || def == nil || def.Signature == nil {
		return
	}
	shot.Signature.Rule = def.Signature
	shot.Signature.FlameScale = def.Graphics.ImpactFlameScale
	shot.Signature.Name = def.Name
	shot.Signature.Damage = statScaledDamage(def.Signature.StatScaledDamage, owner)
	if def.Signature.Kind == config.SignatureReturning {
		shot.Signature.Returning = true
		shot.Signature.HitboxSize = cs.game.config.ProjectileHitboxTiles(def.Physics) * cs.game.config.GetTileSize()
		shot.Signature.Lifetime = shot.LifeTime
		return
	}
	if def.Signature.Kind != config.SignatureSolarFlare {
		return
	}
	item := owner.Equipment[slot]
	if item.Signature.Charges <= 0 {
		return
	}
	items.EnsureInstanceID(&item)
	owner.Equipment[slot] = item
	// One flare in flight reserves the weapon's charge until it lands.
	for i := range cs.game.arrows {
		if a := &cs.game.arrows[i]; a.flying() && a.Signature.Flare && a.Signature.InstanceID == item.InstanceID {
			return
		}
	}
	shot.Signature.InstanceID, shot.Signature.Flare = item.InstanceID, true
}

func (g *MMGame) consumeSignatureCharge(id uint64) bool {
	if id == 0 || g.party == nil {
		return false
	}
	consume := func(it *items.Item) bool {
		if it.InstanceID == id && it.Signature.Charges > 0 {
			it.Signature.Charges = max(0, it.Signature.Charges-1)
			return true
		}
		return false
	}
	if g.eachLiveItem(consume) {
		return true
	}
	if g.stash != nil && g.pendingStashTransferID == "" {
		before := g.stash.Clone()
		spent := false
		g.stash.Each(func(it *items.Item) {
			if !spent {
				spent = consume(it)
			}
		})
		if spent {
			if err := stash.Save(g.stash); err != nil {
				*g.stash = before
				g.AddCombatMessage(uitext.Text("weapon.solar_release_save_failed"))
				return false
			}
			return true
		}
	}
	return false
}

func (cs *CombatSystem) applySignatureHit(target *monster.Monster3D, def *config.WeaponDefinitionConfig, attacker *character.MMCharacter, shot *Arrow) {
	if target == nil || attacker == nil || target.IsDamageInvulnerable() || monsterIsPartyAlly(target) {
		return
	}
	if shot != nil {
		if shot.Signature.Flare {
			cs.releaseSolarFlare(target, shot)
		}
		return
	}
	if !def.HasSignature(config.SignatureRuneDischarge) {
		return
	}
	if cs.runeSwing != nil && (cs.runeSwing.primary != target || cs.runeSwing.spent) {
		return
	}
	// A swing that already killed its target discharges nothing and pays
	// nothing.
	rule := def.Signature
	if !target.IsAlive() || attacker.SpellPoints < rule.SPCost {
		return
	}
	attacker.SpellPoints -= rule.SPCost
	if cs.runeSwing != nil {
		cs.runeSwing.spent = true
	}
	cs.elementalWeaponBonus(target, statScaledDamage(rule.StatScaledDamage, attacker), rule.School, uitext.Text("weapon.rune_discharge_name"))
}

func (cs *CombatSystem) releaseSolarFlare(primary *monster.Monster3D, shot *Arrow) {
	payload := shot.Signature
	shot.Signature.Flare = false
	if !cs.game.consumeSignatureCharge(payload.InstanceID) {
		return
	}
	dx, dy := shot.VelX, shot.VelY
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return
	}
	dx, dy = dx/length, dy/length
	rule := payload.Rule
	cs.game.spellWorldVisuals = append(cs.game.spellWorldVisuals, spellWorldVisual{
		kind: worldSolarFlare, world: cs.game.world, born: cs.game.frameCount, originX: primary.X, originY: primary.Y,
		dx: dx, dy: dy, length: rule.LaneBehindTiles, scale: payload.FlameScale,
	})
	lane := cs.laneBlast(primary.X, primary.Y, dx, dy, rule.LaneBehindTiles, rule.LaneWidthTiles)
	cs.forEachAreaVictim(lane, partyAreaHurts, func(victim *monster.Monster3D) {
		cs.elementalWeaponBonus(victim, payload.Damage, rule.School, uitext.Text("weapon.solar_flare_name"))
		if victim.IsAlive() {
			cs.afflictMonster(victim, monsterBurn, rule.BurnSeconds*cs.game.config.GetTPS(), logToneGood, false)
		} else if victim != primary {
			cs.finishMonsterKillImmediately(victim)
		}
	})
}

// turnSpentBlades turns each outbound returning blade that stopped flying
// (on a wall, at its landing spot or at the end of its range) back toward the
// party from where it stopped; on the way out it pierces every foe
// (applyProjectileDamage), so only these end the outward leg.
func (cs *CombatSystem) turnSpentBlades() {
	for i := range cs.game.arrows {
		if a := &cs.game.arrows[i]; !a.flying() {
			cs.beginReturningBlade(a)
		}
	}
}

func (cs *CombatSystem) beginReturningBlade(shot *Arrow) bool {
	if shot == nil || !shot.Signature.Returning || shot.Signature.ReturnLeg || shot.Owner != ProjectileOwnerPlayer {
		return false
	}
	shot.Signature.ReturnLeg = true
	shot.Signature.LegHits = nil
	shot.Signature.TurnFromAngle = math.Atan2(shot.VelY, shot.VelX)
	shot.Signature.TurnAtDistance = shot.DistanceTraveled
	shot.Continuation = true
	shot.Damage = shot.Damage * shot.Signature.Rule.ReturnDamagePercent / 100
	shot.TrueDamage = shot.TrueDamage * shot.Signature.Rule.ReturnDamagePercent / 100
	shot.PierceLeft, shot.RicochetLeft, shot.DisintegrateChance = 0, 0, 0
	shot.SkipMonster = nil
	shot.Launch = projectileLaunch{}
	shot.Active = true
	shot.LifeTime = 2 * shot.Signature.Lifetime
	size := shot.Signature.HitboxSize
	cs.game.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, shot.X, shot.Y, size, size, collision.CollisionTypeProjectile, false))
	cs.aimReturningBlade(shot)
	return true
}

func (cs *CombatSystem) aimReturningBlade(shot *Arrow) {
	if shot == nil || !shot.Active || !shot.Signature.ReturnLeg {
		return
	}
	x, y := cs.logicalCameraXY()
	dx, dy := x-shot.X, y-shot.Y
	distance, speed := math.Hypot(dx, dy), math.Hypot(shot.VelX, shot.VelY)
	if distance <= speed {
		shot.Active = false
		cs.game.collisionSystem.UnregisterEntity(shot.ID)
		return
	}
	shot.VelX, shot.VelY = dx/distance*speed, dy/distance*speed
}

func (cs *CombatSystem) applyReturningBladeHit(shot *Arrow, target *monster.Monster3D) {
	shot.Signature.LegHits = append(shot.Signature.LegHits, target.ID)
	if cs.absorbIfSealed(target) {
		return
	}
	// The return resolves like the outward shot, without the primary
	// weapon's own effects: the same attack, dodge, school and log line.
	school := normalizeDamageTypeStr(shot.DamageType)
	attack := cs.arrowAttack(shot, school, nil, shot.Signature.Name)
	if dodged, _ := cs.dodgePartyShot(target, attack, partyAttackerName(shot.Attacker), school, shot.Signature.Name, nil); dodged {
		return
	}
	// The racial proc rides every party hit; only the primary weapon's own
	// effects stay off the return.
	if cs.tryDarkElfBindInstead(shot.Attacker, target) {
		return
	}
	hit := cs.applyPartyMonsterAttack(target, attack)
	cs.applyCrumbling(target)
	cs.markMonsterHit(target)
	cs.spawnHitSparks(target)
	cs.reportProjectileHit(target, shot.Signature.Name, school, hit)
	if !target.IsAlive() {
		cs.finishWeaponKill(target, nil, shot.Attacker)
	}
}

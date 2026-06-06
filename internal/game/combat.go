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

	// Check if character has a spell equipped
	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return false // No spell equipped
	}

	// Check spell points (use spell cost for spells)
	var spellCost int
	if spell.Type == items.ItemBattleSpell || spell.Type == items.ItemUtilitySpell {
		spellCost = cs.effectiveSpellCost(caster, spell.SpellCost)
	} else {
		// This shouldn't happen - SlotSpell should only contain spells
		return false
	}

	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			caster.Name, caster.SpellPoints, spellCost))
		return false
	}

	// Cast the equipped spell
	caster.SpellPoints -= spellCost

	spellID := spells.SpellID(spell.SpellEffect)

	// Check if this is a utility spell (non-projectile)
	castingSystem := spells.NewCastingSystem(cs.game.config)
	spellDef, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return false
	}
	// Data-driven effect spells (AoE stun, party buffs, resurrect) — no
	// projectile, no direct damage.
	if cs.tryCastSpecialEffect(spellID, spellDef, caster) {
		return true
	}
	if spellDef.IsUtility {
		// Handle utility spells (like Torch Light)
		result, err := castingSystem.ApplyUtilitySpell(spellID, caster.Intellect)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return false
		}

		// Apply the utility spell effects to the game
		if result.Success {
			// Normalize core spell values through centralized calculators.
			if spellDef.HealAmount > 0 {
				_, _, totalHeal := cs.CalculateSpellHealing(spellID, caster)
				result.HealAmount = totalHeal
			}
			if spellDef.StatBonus > 0 {
				result.StatBonus = cs.CalculateSpellStatBonus(spellID, caster)
			}
			if spellDef.Duration > 0 {
				result.Duration = cs.CalculateSpellDurationFrames(spellID, caster)
			}
			// Add combat message
			cs.game.AddCombatMessage(result.Message)

			// Apply healing effects for heal spells
			if result.HealAmount > 0 {
				if spellDef.HealParty {
					// Mass Heal: restore every party member.
					cs.healWholeParty(result.HealAmount)
				} else {
					// Fallback self-heal (mouse-targeted heals go via CastEquippedHealOnTarget).
					cs.healMember(cs.game.selectedChar, result.HealAmount)
				}
			}

			// Apply utility spell effects dynamically based on spell ID
			switch string(spellID) {
			case "torch_light":
				cs.game.torchLightActive = true
				cs.game.torchLightDuration = result.Duration
				cs.game.torchLightRadius = TorchLightRadiusTiles
			case "wizard_eye":
				cs.game.wizardEyeActive = true
				cs.game.wizardEyeDuration = result.Duration
			case "walk_on_water":
				cs.game.walkOnWaterActive = true
				cs.game.walkOnWaterDuration = result.Duration
			case "water_breathing":
				cs.game.waterBreathingActive = true
				cs.game.waterBreathingDuration = result.Duration
				// Store current position and map for return teleportation when effect expires
				cs.game.underwaterReturnX = cs.game.camera.X
				cs.game.underwaterReturnY = cs.game.camera.Y
				if world.GlobalWorldManager != nil {
					cs.game.underwaterReturnMap = world.GlobalWorldManager.CurrentMapKey
				}
			case "bless":
				cs.applyBlessEffect(result.Duration, result.StatBonus)
			}

			cs.game.setUtilityStatus(spellID, result.Duration)
			return true
		}
		return false
	}

	// For projectile spells, create a projectile using effective intellect (includes Bless bonus)
	effectiveIntellect := caster.GetEffectiveIntellect(cs.game.statBonus)
	projectile, err := castingSystem.CreateProjectile(spellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return false
	}
	// Override damage with centralized calculation (includes mastery bonus).
	if _, _, totalDamage := cs.CalculateSpellDamage(spellID, caster); totalDamage > 0 {
		projectile.Damage = totalDamage
	}
	if spellDef.DealsNoDamage {
		projectile.Damage = 0 // Disintegrate: only the instakill roll matters
	}

	// Get spell-specific config dynamically
	spellConfig, err := cs.game.config.GetSpellConfig(string(spellID))
	if err != nil {
		cs.game.AddCombatMessage("Spell config error: " + err.Error())
		return false
	}
	disintegrateChance := 0.0
	if spellDefConfig, exists := config.GetSpellDefinition(string(spellID)); exists && spellDefConfig != nil {
		disintegrateChance = spellDefConfig.DisintegrateChance
	}

	// Determine critical hit for spells based on Luck only (no base crit for spells)
	isCrit, _ := cs.RollCriticalChance(0, caster)
	if isCrit {
		projectile.Damage *= CritDamageMultiplier
	}

	// Create magic projectile with proper type information
	magicProjectile := MagicProjectile{
		ID:                 cs.game.GenerateProjectileID(string(spellID)),
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

	// Register with collision system
	tileSize := cs.game.config.GetTileSize()
	collisionSize := spellConfig.GetCollisionSizePixels(tileSize) // Use spell-specific collision size in tiles
	projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(projectileEntity)

	// Note: Combat message for spell casting is now only generated when projectile hits a target
	// This prevents spam of "X casts Y!" messages for attacks that miss
	return true
}

// CastEquippedHealOnTarget casts heal using equipped spell on specified party member
// healWholeParty restores `amount` HP to every CONSCIOUS party member (Mass
// Heal). Like single-target heal, it will not revive the dead/eradicated.
// Returns the number of members healed.
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

func (cs *CombatSystem) CastEquippedHealOnTarget(targetIndex int) bool {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot cast heals
	if caster.IsIncapacitated() {
		return false
	}

	// Check if character has a heal spell equipped
	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return false // No spell equipped
	}

	// Allow both heal-type spells for targeting
	if spell.SpellEffect != items.SpellEffectHealSelf && spell.SpellEffect != items.SpellEffectHealOther {
		return false // Not a heal spell
	}

	spellIDStr := string(spell.SpellEffect)
	if spellIDStr == "" {
		return false
	}
	spellID := spells.SpellID(spellIDStr)

	// Check spell points (use spell cost for utility spells)
	spellCost := cs.effectiveSpellCost(caster, spell.SpellCost)
	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			caster.Name, caster.SpellPoints, spellCost))
		return false
	}

	// For First Aid (SpellEffectHealSelf), only allow self-targeting
	if spell.SpellEffect == items.SpellEffectHealSelf && targetIndex != cs.game.selectedChar {
		return false // First Aid can only target self
	}

	// Check if target index is valid
	if targetIndex < 0 || targetIndex >= len(cs.game.party.Members) {
		return false
	}

	target := cs.game.party.Members[targetIndex]

	// Heal must not revive characters at 0 HP / Dead.
	if target.HitPoints <= 0 || target.HasCondition(character.ConditionDead) || target.HasCondition(character.ConditionEradicated) {
		cs.game.AddCombatMessage(fmt.Sprintf("%s cannot be healed from 0 HP.", target.Name))
		return false
	}

	// Cast heal on target
	caster.SpellPoints -= spellCost
	// Calculate heal amount using centralized spell formula
	_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)
	cs.healMember(targetIndex, healAmount)

	// Print feedback message
	if targetIndex == cs.game.selectedChar {
		message := fmt.Sprintf("%s healed themselves for %d HP with %s!", caster.Name, healAmount, spell.Name)
		cs.game.AddCombatMessage(message)
	} else {
		message := fmt.Sprintf("%s healed %s for %d HP with %s!", caster.Name, target.Name, healAmount, spell.Name)
		cs.game.AddCombatMessage(message)
	}
	return true
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
// level) — bound to the C key. Party heals hit everyone; self-only heals (e.g.
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
		cs.game.AddCombatMessage(fmt.Sprintf("%s knows no healing spell.", caster.Name))
		return false, ""
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false, ""
	}
	spellCost := cs.effectiveSpellCost(caster, def.SpellPointsCost)
	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's %s fizzles! (Not enough SP: %d/%d)",
			caster.Name, def.Name, caster.SpellPoints, spellCost))
		return false, ""
	}

	_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)

	// Party heal: restore everyone, ignore the single target.
	if def.HealParty {
		caster.SpellPoints -= spellCost
		n := cs.healWholeParty(healAmount)
		cs.game.AddCombatMessage(fmt.Sprintf("%s casts %s, healing %d allies for %d HP!",
			caster.Name, def.Name, n, healAmount))
		return true, spellID
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
		return false, ""
	}

	caster.SpellPoints -= spellCost
	cs.healMember(targetIndex, healAmount)
	if targetIndex == cs.game.selectedChar {
		cs.game.AddCombatMessage(fmt.Sprintf("%s heals themselves for %d HP with %s!", caster.Name, healAmount, def.Name))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s heals %s for %d HP with %s!", caster.Name, target.Name, healAmount, def.Name))
	}
	return true, spellID
}

// SmartAttack is the Space-key "smart attack" (both modes). Priority:
//  1. A HEAL slotted + a wounded ally present → cast it on the MOST wounded
//     (a healer with First Aid/Heal auto-triages instead of meleeing).
//  2. An OFFENSIVE spell slotted + enough SP → cast it.
//  3. Otherwise swing the equipped weapon.
// Returns whether a spell was cast (and which) so the caller can pick the right
// cooldown; a false return means a weapon attack happened.
func (cs *CombatSystem) SmartAttack() (bool, spells.SpellID) {
	caster := cs.game.party.Members[cs.game.selectedChar]
	if spell, hasSpell := caster.Equipment[items.SlotSpell]; hasSpell {
		spellID := spells.SpellID(spell.SpellEffect)
		def, err := spells.GetSpellDefinitionByID(spellID)
		canPay := caster.SpellPoints >= cs.effectiveSpellCost(caster, spell.SpellCost)
		switch {
		case err == nil && def.IsHeal():
			// A heal is slotted: if an ally is wounded and we can pay, heal the
			// most-wounded instead of attacking. Party heals (Mass Heal) cast on
			// the whole party; single heals target the wounded ally.
			if target := cs.mostWoundedHealTarget(spell); target >= 0 && canPay {
				if def.HealParty {
					if cs.CastEquippedSpell() {
						return true, spellID
					}
				} else if cs.CastEquippedHealOnTarget(target) {
					return true, spellID
				}
			}
			// No one worth healing → fall through to a weapon swing.
		case err == nil && def.IsOffensive():
			if canPay && cs.CastEquippedSpell() {
				return true, spellID
			}
		}
	}
	cs.EquipmentMeleeAttack()
	return false, ""
}

// mostWoundedHealTarget returns the party index of the most-wounded ally a
// slotted heal should target (lowest HP fraction, below SmartHealWoundedPct),
// or -1 if no one is hurt enough. A self-only heal (First Aid) only ever
// considers the caster; an other-target heal considers the whole party. Dead/KO
// members are skipped (heals don't revive).
func (cs *CombatSystem) mostWoundedHealTarget(spell items.Item) int {
	selfOnly := spell.SpellEffect == items.SpellEffectHealSelf
	best, bestFrac := -1, SmartHealWoundedPct
	for i, m := range cs.game.party.Members {
		if m == nil || !m.CanAct() || m.MaxHitPoints <= 0 {
			continue
		}
		if selfOnly && i != cs.game.selectedChar {
			continue
		}
		frac := float64(m.HitPoints) / float64(m.MaxHitPoints)
		if frac < bestFrac {
			best, bestFrac = i, frac
		}
	}
	return best
}

// EquipmentMeleeAttack performs a melee attack using equipped weapon
func (cs *CombatSystem) EquipmentMeleeAttack() {
	attacker := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot attack
	if attacker.IsIncapacitated() {
		return
	}

	// Check if character has a weapon equipped
	weapon, hasWeapon := attacker.Equipment[items.SlotMainHand]
	if !hasWeapon {
		return // No weapon equipped
	}

	// Calculate damage using centralized function
	_, _, totalDamage := cs.CalculateWeaponDamage(weapon, attacker)

	weaponDef := lookupWeaponConfigByName(weapon.Name)
	if weaponDef == nil {
		return // Weapon not found, skip attack
	}

	// Ranged dispatch by `range` field (in display tiles). Anything > 3
	// goes through the projectile path. Throwing weapons must declare
	// range ≥ 4 to count as ranged (otherwise they fall into melee).
	// For ranged: roll crit and apply doubling inside createArrowAttack only.
	if weaponDef.Range > 3 {
		cs.createArrowAttack(totalDamage)
		return
	}
	isCrit, _ := cs.RollWeaponCriticalChance(weapon, attacker)
	if isCrit {
		totalDamage *= CritDamageMultiplier
	}

	// Create instant melee attack for close-range weapons
	cs.createMeleeAttack(weapon, totalDamage, isCrit)
}

// createArrowAttack creates a projectile arrow attack
func (cs *CombatSystem) createArrowAttack(damage int) {
	// Find the equipped projectile-weapon's YAML key. Range>3 = ranged
	// (matches the dispatch gate in EquipmentMeleeAttack).
	attacker := cs.game.party.Members[cs.game.selectedChar]
	weapon, hasWeapon := attacker.Equipment[items.SlotMainHand]
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
			return
		}
	}

	weaponDef, exists := config.GetWeaponDefinition(bowKey)
	if !exists || weaponDef == nil || weaponDef.Physics == nil {
		fmt.Printf("[WARN] projectile weapon '%s' is missing physics in weapons.yaml\n", bowKey)
		return
	}

	tileSize := cs.game.config.GetTileSize()
	arrowSpeed := weaponDef.Physics.GetSpeedPixels(tileSize)
	arrowLifetime := weaponDef.Physics.GetLifetimeFrames()
	collisionSize := weaponDef.Physics.GetCollisionSizePixels(tileSize)

	// Determine damage type from weapon
	damageType := "physical" // Default
	if equippedDef != nil && equippedDef.DamageType != "" {
		damageType = equippedDef.DamageType
	}

	isCrit, _ := cs.RollWeaponCriticalChance(weapon, attacker)
	if isCrit {
		damage *= CritDamageMultiplier
	}

	arrow := Arrow{
		ID:                 cs.game.GenerateProjectileID("arrow"),
		X:                  cs.game.camera.X,
		Y:                  cs.game.camera.Y,
		VelX:               math.Cos(cs.game.camera.Angle) * arrowSpeed,
		VelY:               math.Sin(cs.game.camera.Angle) * arrowSpeed,
		Damage:             damage,
		LifeTime:           arrowLifetime,
		Active:             true,
		BowKey:             bowKey,
		DamageType:         damageType,
		Crit:               isCrit,
		DisintegrateChance: weaponDef.DisintegrateChance,
		Owner:              ProjectileOwnerPlayer,
	}

	cs.game.arrows = append(cs.game.arrows, arrow)

	// Register arrow with collision system using weapon-specific collision size
	arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(arrowEntity)
}

// createMeleeAttack creates an instant melee attack with proper arc-based hit detection
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
		// fades slowly — the instant hit already resolved separately.
		maxFrames := meleeConfig.AnimationFrames
		if maxFrames < MeleeFxLingerFrames {
			maxFrames = MeleeFxLingerFrames
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
		}
		cs.game.slashEffects = append(cs.game.slashEffects, slashEffect)
	}

	// Perform instant hit detection in arc
	cs.performMeleeHitDetection(weapon, totalDamage, meleeConfig, isCrit)
}

// performMeleeHitDetection checks for monsters in the weapon's swing arc and applies damage
func (cs *CombatSystem) performMeleeHitDetection(weapon items.Item, damage int, meleeConfig *config.MeleeAttackConfig, isCrit bool) {
	playerX := cs.game.camera.X
	playerY := cs.game.camera.Y
	playerAngle := cs.game.camera.Angle

	// Convert range from tiles to pixels
	tileSize := float64(cs.game.config.GetTileSize())
	weaponDef := lookupWeaponConfigByName(weapon.Name)
	weaponRange := 1
	if weaponDef != nil {
		weaponRange = weaponDef.Range
	}
	attackRange := float64(weaponRange) * tileSize

	// Convert arc angle from degrees to radians
	arcAngleRad := float64(meleeConfig.ArcAngle) * math.Pi / 180.0
	halfArc := arcAngleRad / 2.0

	// Check all monsters
	for _, monster := range cs.game.world.Monsters {
		if !monster.IsAlive() {
			continue
		}
		if monster.StunFramesRemaining > 0 {
			continue
		}

		// Calculate distance and angle to monster center
		dx := monster.X - playerX
		dy := monster.Y - playerY
		distanceToCenter := Distance(playerX, playerY, monster.X, monster.Y)

		// Get monster collision box size
		monsterWidth, monsterHeight := monster.GetSize()

		// Calculate distance to edge of collision box (closest approach)
		// For rectangular collision boxes, we need to account for the collision radius
		collisionRadius := math.Max(monsterWidth, monsterHeight) / 2.0
		distanceToEdge := distanceToCenter - collisionRadius

		// Check if monster collision box edge is within weapon range
		if distanceToEdge > attackRange {
			continue
		}

		// Calculate angle to monster
		monsterAngle := math.Atan2(dy, dx)

		// Normalize angle difference
		angleDiff := monsterAngle - playerAngle
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// Check if monster is within swing arc
		if math.Abs(angleDiff) <= halfArc {
			// Hit! Apply damage immediately
			cs.ApplyDamageToMonster(monster, damage, weapon.Name, isCrit)
		}
	}
}

// ApplyDamageToMonster applies damage to a monster and handles combat messages
// This is for melee attacks - AC applies only to physical damage as reduction
// applyTrueDamageThroughDodge deals flat weapon-mastery TRUE damage that landed
// despite the target's Perfect Dodge, with the usual hit bookkeeping (tint, pack
// aggro, death/XP). Caller is responsible for any projectile cleanup.
func (cs *CombatSystem) applyTrueDamageThroughDodge(monster *monsterPkg.Monster3D, trueDmg int, damageType monsterPkg.DamageType, attackerName string) {
	actual := monster.TakeDamage(trueDmg, damageType, cs.game.camera.X, cs.game.camera.Y)
	monster.HitTintFrames = MonsterHitFlashFrames
	cs.engageTurnBasedPackOnHit(monster)
	if !monster.IsAlive() {
		cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, monster.ID)
		cs.awardExperienceAndGold(monster)
		cs.game.AddCombatMessage(fmt.Sprintf("%s's mastery pierces %s's dodge for %d true damage and kills it!", attackerName, monster.Name, actual))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", monster.Experience))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s dodges, but %s's mastery lands %d true damage! (HP: %d/%d)", monster.Name, attackerName, actual, monster.HitPoints, monster.MaxHitPoints))
	}
}

func (cs *CombatSystem) ApplyDamageToMonster(monster *monsterPkg.Monster3D, damage int, weaponName string, isCrit bool) {
	weaponDef := lookupWeaponConfigByName(weaponName)
	damageTypeStr := weaponDamageTypeStr(weaponDef)
	damageType := convertToMonsterDamageType(damageTypeStr)
	trueDmg, ignoreDodge := cs.weaponMasteryStrike(weaponDef)
	attackerName := cs.game.party.Members[cs.game.selectedChar].Name

	// Check monster perfect dodge. A Grandmaster ignores it entirely; otherwise
	// the normal hit is avoided but weapon-mastery TRUE damage still lands.
	if monster.PerfectDodge > 0 && !ignoreDodge && rand.Intn(100) < monster.PerfectDodge {
		if trueDmg > 0 {
			cs.applyTrueDamageThroughDodge(monster, trueDmg, damageType, attackerName)
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges %s's attack!", monster.Name, attackerName))
		}
		return
	}

	reducedDamage := applyArmorReductionIfPhysical(damage, damageTypeStr, monster.ArmorClass, false)
	if mult := cs.weaponBonusMultiplier(weaponDef, monster); mult != 1.0 {
		reducedDamage = int(math.Round(float64(reducedDamage) * mult))
		if reducedDamage < 1 {
			reducedDamage = 1
		}
	}
	reducedDamage += trueDmg // weapon-mastery true damage bypasses armor

	// Apply damage with resistances and distance-aware AI response
	finalDamage := monster.TakeDamage(reducedDamage, damageType, cs.game.camera.X, cs.game.camera.Y)
	monster.HitTintFrames = MonsterHitFlashFrames
	// Impact feedback: spark burst at the monster + a small recoil away from the
	// party (the AI walks back in, so it reads as a stagger-and-return).
	cs.game.spawnImpactSparks(monster.X, monster.Y)
	if d := Distance(cs.game.camera.X, cs.game.camera.Y, monster.X, monster.Y); d > 0.001 {
		kb := cs.game.config.MonsterAI.PushbackDistance * 0.7
		monster.X += (monster.X - cs.game.camera.X) / d * kb
		monster.Y += (monster.Y - cs.game.camera.Y) / d * kb
	}
	cs.engageTurnBasedPackOnHit(monster)
	if monster.IsAlive() {
		cs.tryApplyWeaponStun(monster, weaponDef)
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

		// Add to dead monsters list for cleanup (O(1) instead of iterating all monsters)
		cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, monster.ID)

		// Award experience and gold using centralized function
		cs.awardExperienceAndGold(monster)

		// Add experience/gold award message
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", monster.Experience))
	}
}

// engageTurnBasedPackOnHit ensures a hit in turn-based mode pulls in nearby same-type monsters.
func (cs *CombatSystem) engageTurnBasedPackOnHit(hit *monsterPkg.Monster3D) {
	if !cs.game.turnBasedMode || hit == nil {
		return
	}

	tileSize := float64(cs.game.config.GetTileSize())
	radius := tileSize * PackAggroRadiusTiles
	hitKey := hit.Key // pack by exact type (key), not display Name

	for _, m := range cs.game.world.Monsters {
		if !m.IsAlive() {
			continue
		}
		if m.Key != hitKey {
			continue
		}
		if Distance(hit.X, hit.Y, m.X, m.Y) > radius {
			continue
		}
		if m.IsEngagingPlayer {
			continue
		}
		m.IsEngagingPlayer = true
		m.State = monsterPkg.StateAlert
		m.StateTimer = 0
		m.AttackCount = 0
	}
}

// CastSelectedSpell casts the currently selected spell from the spellbook.
// Returns true if SP was actually spent and the spell went off — callers use
// that to consume a turn-based action slot.
func (cs *CombatSystem) CastSelectedSpell() bool {
	currentChar := cs.game.party.Members[cs.game.selectedChar]

	// Prevent casting while down; also avoids utility healing from acting as a revive.
	if currentChar.IsIncapacitated() {
		return false
	}
	schools := currentChar.GetAvailableSchools()

	if cs.game.selectedSchool >= len(schools) {
		return false
	}

	selectedSchool := schools[cs.game.selectedSchool]
	availableSpells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell < 0 || cs.game.selectedSpell >= len(availableSpells) {
		return false
	}

	selectedSpellID := availableSpells[cs.game.selectedSpell]
	selectedSpellDef, err := spells.GetSpellDefinitionByID(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return false
	}

	// Check spell points
	spellCost := cs.effectiveSpellCost(currentChar, selectedSpellDef.SpellPointsCost)
	if currentChar.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			currentChar.Name, currentChar.SpellPoints, spellCost))
		return false
	}

	// Cast the spell
	currentChar.SpellPoints -= spellCost

	// Data-driven effect spells (AoE stun, party buffs, resurrect).
	if cs.tryCastSpecialEffect(selectedSpellID, selectedSpellDef, currentChar) {
		return true
	}

	// Dynamic spell casting - no more hardcoded switches!
	castingSystem := spells.NewCastingSystem(cs.game.config)

	if selectedSpellDef.IsProjectile {
		// Handle projectile spells dynamically using effective intellect (includes Bless bonus)
		effectiveIntellect := currentChar.GetEffectiveIntellect(cs.game.statBonus)
		projectile, err := castingSystem.CreateProjectile(selectedSpellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return false
		}
		// Override damage with centralized calculation (includes mastery bonus).
		if _, _, totalDamage := cs.CalculateSpellDamage(selectedSpellID, currentChar); totalDamage > 0 {
			projectile.Damage = totalDamage
		}
		if selectedSpellDef.DealsNoDamage {
			projectile.Damage = 0 // Disintegrate: only the instakill roll matters
		}

		// Determine critical hit for spells based on Luck only (no base crit for spells)
		isCrit, _ := cs.RollCriticalChance(0, currentChar)
		if isCrit {
			projectile.Damage *= CritDamageMultiplier
		}
		disintegrateChance := 0.0
		if spellDefConfig, exists := config.GetSpellDefinition(string(selectedSpellID)); exists && spellDefConfig != nil {
			disintegrateChance = spellDefConfig.DisintegrateChance
		}

		// Create magic projectile using unified system
		magicProjectile := MagicProjectile{
			ID:                 cs.game.GenerateProjectileID(string(selectedSpellID)),
			X:                  projectile.X,
			Y:                  projectile.Y,
			VelX:               projectile.VelX,
			VelY:               projectile.VelY,
			Damage:             projectile.Damage,
			LifeTime:           projectile.LifeTime,
			Active:             projectile.Active,
			SpellType:          string(selectedSpellID),
			Crit:               isCrit,
			DisintegrateChance: disintegrateChance,
			Owner:              ProjectileOwnerPlayer,
		}
		cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

		// Register projectile with collision system using dynamic config
		spellConfig, err := cs.game.config.GetSpellConfig(string(selectedSpellID))
		if err != nil {
			cs.game.AddCombatMessage("Spell config error: " + err.Error())
			return false
		}
		tileSize := cs.game.config.GetTileSize()
		collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
		projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(projectileEntity)

		// Add message based on spell definition
		cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", selectedSpellDef.Name))

	} else if selectedSpellDef.IsUtility {
		// Handle utility spells using centralized system
		result, err := castingSystem.ApplyUtilitySpell(selectedSpellID, currentChar.Personality)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return false
		}

		if result.Success {
			// Normalize core spell values through centralized calculators.
			if selectedSpellDef.HealAmount > 0 {
				_, _, totalHeal := cs.CalculateSpellHealing(selectedSpellID, currentChar)
				result.HealAmount = totalHeal
			}
			if selectedSpellDef.StatBonus > 0 {
				result.StatBonus = cs.CalculateSpellStatBonus(selectedSpellID, currentChar)
			}
			if selectedSpellDef.Duration > 0 {
				result.Duration = cs.CalculateSpellDurationFrames(selectedSpellID, currentChar)
			}

			// Apply effects based on spell result
			cs.game.AddCombatMessage(result.Message)

			// Apply healing
			if result.HealAmount > 0 {
				if selectedSpellDef.HealParty {
					cs.healWholeParty(result.HealAmount)
				} else {
					cs.healMember(cs.game.selectedChar, result.HealAmount)
				}
			}

			// Apply vision effects
			if result.VisionBonus > 0 {
				switch string(selectedSpellID) {
				case "torch_light":
					cs.game.torchLightActive = true
					cs.game.torchLightDuration = result.Duration
					cs.game.torchLightRadius = TorchLightRadiusTiles
				case "wizard_eye":
					cs.game.wizardEyeActive = true
					cs.game.wizardEyeDuration = result.Duration
				}
			}

			// Apply movement effects
			if result.WaterWalk {
				cs.game.walkOnWaterActive = true
				cs.game.walkOnWaterDuration = result.Duration
			}
			if result.WaterBreathing {
				cs.game.waterBreathingActive = true
				cs.game.waterBreathingDuration = result.Duration
				// Store current position and map for return teleportation when effect expires
				cs.game.underwaterReturnX = cs.game.camera.X
				cs.game.underwaterReturnY = cs.game.camera.Y
				if world.GlobalWorldManager != nil {
					cs.game.underwaterReturnMap = world.GlobalWorldManager.CurrentMapKey
				}
			}

			// Apply stat bonus effects
			if result.StatBonus > 0 {
				cs.applyBlessEffect(result.Duration, result.StatBonus)
			}

			cs.game.setUtilityStatus(selectedSpellID, result.Duration)
		}
	}
	return true
}

// EquipSelectedSpell equips the selected spell as an item in a battle or utility slot
func (cs *CombatSystem) EquipSelectedSpell() {
	currentChar := cs.game.party.Members[cs.game.selectedChar]
	schools := currentChar.GetAvailableSchools()

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

		// Stunned monsters take no action (the TB path already skips them; the
		// real-time path must too, or a stun frozen at StateTimer==1 would let a
		// monster pounce/strike every frame for the whole stun). Update() decrements
		// the stun counter; here we just suppress the action.
		if monster.StunFramesRemaining > 0 {
			continue
		}

		// Charmed: bind_undead attacks the nearest other monster on a ~1s cadence;
		// Charm (pacified) simply does nothing. Neither ever attacks the party.
		if monster.Charmed {
			if !monster.CharmPacified {
				if monster.CharmAttackCD > 0 {
					monster.CharmAttackCD--
				} else if cs.charmedAttackNearest(monster) {
					monster.CharmAttackCD = cs.game.config.GetTPS()
				}
			}
			continue
		}

		dist := Distance(cs.game.camera.X, cs.game.camera.Y, monster.X, monster.Y)
		attackRange := monster.GetAttackRangePixels()

		// Pounce (real-time): from within pounce range but beyond melee, leap
		// to melee contact and strike immediately, then go on cooldown.
		if monster.CanPounce() {
			if monster.PounceCDFrames > 0 {
				monster.PounceCDFrames--
			}
			if monster.PounceCDFrames == 0 && dist > attackRange && dist <= monster.PounceRangePixels &&
				(!monster.PassiveUntilAttacked || monster.WasAttacked || monster.HatesActiveTrait()) {
				if newDist, landed := cs.executePounce(monster, cs.game.camera.X, cs.game.camera.Y); landed {
					cs.game.AddCombatMessage(fmt.Sprintf("%s pounces at the party!", monster.Name))
					cs.applyMonsterMeleeDamage(monster, newDist)
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
		// just pounced onto an adjacent tile) still lands its hit.
		if monster.State == monsterPkg.StateAttacking && dist <= attackRange {
			// Only attack once per attacking state (no separate cooldown needed)
			if monster.StateTimer == 1 { // Attack on first frame of attacking state
				if monster.HasRangedAttack() {
					cs.spawnMonsterRangedAttack(monster)
				} else {
					cs.applyMonsterMeleeDamage(monster, dist)
				}
			}
		}
	}
}

// executePounce leaps a pouncing monster onto the nearest walkable tile
// directly N/S/E/W of the player — never inside the player's own tile (where
// the sprite would vanish) and never diagonally. Returns the new
// center-to-center distance and whether a landing tile was found; callers must
// only resolve the strike when landed is true. Shared by RT and TB pounce hooks.
func (cs *CombatSystem) executePounce(m *monsterPkg.Monster3D, playerX, playerY float64) (float64, bool) {
	tileSize := float64(cs.game.config.GetTileSize())
	ptx, pty := int(playerX/tileSize), int(playerY/tileSize)

	cands := [4][2]int{{ptx + 1, pty}, {ptx - 1, pty}, {ptx, pty + 1}, {ptx, pty - 1}}
	bestX, bestY, bestD := m.X, m.Y, math.MaxFloat64
	found := false
	for _, c := range cands {
		cx, cy := TileCenterFromTile(c[0], c[1], tileSize)
		if !cs.game.collisionSystem.CanMoveToWithHabitat(m.ID, cx, cy, m.HabitatPrefs, m.Flying) {
			continue
		}
		if d := (cx-m.X)*(cx-m.X) + (cy-m.Y)*(cy-m.Y); d < bestD {
			bestD, bestX, bestY, found = d, cx, cy, true
		}
	}
	if !found {
		return Distance(playerX, playerY, m.X, m.Y), false // no free adjacent tile — can't pounce
	}
	m.X, m.Y = bestX, bestY
	cs.game.collisionSystem.UpdateEntity(m.ID, bestX, bestY)
	m.AttackAnimFrames = 8 // brief leap/strike animation
	return Distance(playerX, playerY, bestX, bestY), true
}

func (cs *CombatSystem) applyMonsterMeleeDamage(monster *monsterPkg.Monster3D, dist float64) {
	if monster.FireburstChance > 0 && rand.Float64() < monster.FireburstChance {
		cs.applyMonsterFireburst(monster)
		return
	}

	// Melee hits a random living party member (both RT and TB).
	currentChar := cs.randomLivingMember()
	if currentChar == nil {
		return
	}
	damage := monster.GetAttackDamage()

	finalDamage := cs.mitigateIncoming(cs.applyArmorToCharacterIfPhysical(damage, "physical", currentChar))

	// Perfect Dodge: luck/5% to avoid all damage
	if dodged, _ := cs.RollPerfectDodge(currentChar); !dodged {
		currentChar.HitPoints -= finalDamage
		if currentChar.HitPoints < 0 {
			currentChar.HitPoints = 0
		}

		// Add combat message for monster attack
		cs.game.AddCombatMessage(fmt.Sprintf("%s hits %s for %d damage! (HP: %d/%d)",
			monster.Name, currentChar.Name, finalDamage,
			currentChar.HitPoints, currentChar.MaxHitPoints))

		if currentChar.HitPoints == 0 {
			currentChar.AddCondition(character.ConditionUnconscious)
			cs.game.AddCombatMessage(fmt.Sprintf("%s falls unconscious!", currentChar.Name))
		}

		if monster.PoisonChance > 0 && rand.Float64() < monster.PoisonChance {
			durationSec := monster.PoisonDurationSec
			if durationSec <= 0 {
				durationSec = 20
			}
			poisonFrames := cs.game.config.GetTPS() * durationSec
			currentChar.ApplyPoison(poisonFrames)
			cs.game.AddCombatMessage(fmt.Sprintf("%s is poisoned!", currentChar.Name))
		}
	} else {
		// Announce dodge and skip damage blink below
		cs.game.AddCombatMessage(fmt.Sprintf("Perfect Dodge! %s evades %s's attack!", currentChar.Name, monster.Name))
		return
	}

	// Trigger damage blink effect for the character that was hit
	targetIndex := cs.findCharacterIndex(currentChar)
	cs.game.TriggerDamageBlink(targetIndex)

	// Push monster back slightly to prevent spam
	pushBack := cs.game.config.MonsterAI.PushbackDistance
	if dist == 0 {
		dist = 0.001
	}
	monster.X += (monster.X - cs.game.camera.X) / dist * pushBack
	monster.Y += (monster.Y - cs.game.camera.Y) / dist * pushBack
}

func (cs *CombatSystem) applyMonsterFireburst(monster *monsterPkg.Monster3D) {
	cs.game.AddCombatMessage(fmt.Sprintf("%s casts Fireburst!", monster.Name))

	for idx, member := range cs.game.party.Members {
		if member.HitPoints <= 0 {
			continue
		}

		minDamage := monster.FireburstDamageMin
		maxDamage := monster.FireburstDamageMax
		if minDamage <= 0 {
			minDamage = 6
		}
		if maxDamage < minDamage {
			maxDamage = minDamage
		}
		damage := minDamage
		if maxDamage > minDamage {
			damage = minDamage + rand.Intn(maxDamage-minDamage+1)
		}
		damage = cs.mitigateIncoming(damage)
		member.HitPoints -= damage
		if member.HitPoints < 0 {
			member.HitPoints = 0
		}

		cs.game.AddCombatMessage(fmt.Sprintf("Fireburst hits %s for %d damage! (HP: %d/%d)",
			member.Name, damage, member.HitPoints, member.MaxHitPoints))

		if member.HitPoints == 0 {
			member.AddCondition(character.ConditionUnconscious)
			cs.game.AddCombatMessage(fmt.Sprintf("%s falls unconscious!", member.Name))
		}

		cs.game.TriggerDamageBlink(idx)
	}
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
	cs.game.spawnWeaponBoltImpact(monster.X, monster.Y, weaponDef, count, size)
}

func (cs *CombatSystem) spawnMonsterRangedAttack(monster *monsterPkg.Monster3D) {
	if monster.FireburstChance > 0 && rand.Float64() < monster.FireburstChance {
		cs.applyMonsterFireburst(monster)
		return
	}
	if monster.ProjectileSpell != "" {
		cs.spawnMonsterSpellProjectile(monster, spells.SpellID(monster.ProjectileSpell))
		return
	}
	if monster.ProjectileWeapon != "" {
		cs.spawnMonsterWeaponProjectile(monster, monster.ProjectileWeapon)
	}
}

func (cs *CombatSystem) spawnMonsterSpellProjectile(monster *monsterPkg.Monster3D, spellID spells.SpellID) {
	castingSystem := spells.NewCastingSystem(cs.game.config)
	angle := math.Atan2(cs.game.camera.Y-monster.Y, cs.game.camera.X-monster.X)
	projectile, err := castingSystem.CreateProjectile(spellID, monster.X, monster.Y, angle, 0)
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
		Damage:             monster.GetAttackDamage(),
		LifeTime:           projectile.LifeTime,
		Active:             projectile.Active,
		SpellType:          string(spellID),
		Size:               projectile.Size,
		Crit:               false,
		DisintegrateChance: disintegrateChance,
		Owner:              ProjectileOwnerMonster,
		SourceName:         monster.Name,
		AoE:                aoe,
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

	tileSize := cs.game.config.GetTileSize()
	collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
	projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(projectileEntity)
}

func (cs *CombatSystem) spawnMonsterWeaponProjectile(monster *monsterPkg.Monster3D, weaponKey string) {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists || weaponDef == nil || weaponDef.Physics == nil {
		fmt.Printf("[WARN] projectile weapon '%s' is missing physics in weapons.yaml\n", weaponKey)
		return
	}

	tileSize := cs.game.config.GetTileSize()
	arrowSpeed := weaponDef.Physics.GetSpeedPixels(tileSize)
	arrowLifetime := weaponDef.Physics.GetLifetimeFrames()
	collisionSize := weaponDef.Physics.GetCollisionSizePixels(tileSize)

	damageType := "physical"
	if weaponDef.DamageType != "" {
		damageType = weaponDef.DamageType
	}

	angle := math.Atan2(cs.game.camera.Y-monster.Y, cs.game.camera.X-monster.X)
	arrow := Arrow{
		ID:                 cs.game.GenerateProjectileID("monster_arrow"),
		X:                  monster.X,
		Y:                  monster.Y,
		VelX:               math.Cos(angle) * arrowSpeed,
		VelY:               math.Sin(angle) * arrowSpeed,
		Damage:             monster.GetAttackDamage(),
		LifeTime:           arrowLifetime,
		Active:             true,
		BowKey:             weaponKey,
		DamageType:         damageType,
		Crit:               false,
		DisintegrateChance: weaponDef.DisintegrateChance,
		Owner:              ProjectileOwnerMonster,
		SourceName:         monster.Name,
	}

	cs.game.arrows = append(cs.game.arrows, arrow)

	arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(arrowEntity)
}

// CheckProjectileMonsterCollisions checks for collisions between projectiles and monsters
// using perspective-scaled bounding boxes for accurate visual collision detection
func (cs *CombatSystem) CheckProjectileMonsterCollisions() {
	// Collect all active projectiles
	type projectileInfo struct {
		entityID string
		data     interface{}
		pType    string
	}
	var projectiles []projectileInfo

	for i := range cs.game.arrows {
		if cs.game.arrows[i].Active && cs.game.arrows[i].LifeTime > 0 && cs.game.arrows[i].Owner != ProjectileOwnerMonster {
			projectiles = append(projectiles, projectileInfo{cs.game.arrows[i].ID, &cs.game.arrows[i], "arrow"})
		}
	}
	for i := range cs.game.magicProjectiles {
		if cs.game.magicProjectiles[i].Active && cs.game.magicProjectiles[i].LifeTime > 0 && cs.game.magicProjectiles[i].Owner != ProjectileOwnerMonster {
			projectiles = append(projectiles, projectileInfo{cs.game.magicProjectiles[i].ID, &cs.game.magicProjectiles[i], "magic_projectile"})
		}
	}
	for i := range cs.game.meleeAttacks {
		if cs.game.meleeAttacks[i].Active && cs.game.meleeAttacks[i].LifeTime > 0 {
			projectiles = append(projectiles, projectileInfo{cs.game.meleeAttacks[i].ID, &cs.game.meleeAttacks[i], "melee"})
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
		if hitMonster != nil {
			cs.applyProjectileDamage(proj.data, proj.pType, hitMonster, proj.entityID)
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
			if mp.AoE {
				cs.applyMonsterProjectileDamageAoE(mp.SourceName, mp.Damage, damageTypeStr, mp.DisintegrateChance)
			} else {
				cs.applyMonsterProjectileDamage(mp.SourceName, mp.Damage, damageTypeStr, mp.DisintegrateChance)
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
			cs.applyMonsterProjectileDamage(ar.SourceName, ar.Damage, damageTypeStr, ar.DisintegrateChance)
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
// Real-time → the tank (front slot). Turn-based → mostly the tank, sometimes a
// back-liner (see rangedTBTarget / RangedOffTankChance).
func (cs *CombatSystem) applyMonsterProjectileDamage(sourceName string, damage int, damageTypeStr string, disintegrateChance float64) {
	var target *character.MMCharacter
	if cs.game.turnBasedMode {
		target = cs.rangedTBTarget()
	} else {
		target = cs.tankTarget()
	}
	cs.applyMonsterProjectileDamageToChar(target, sourceName, damage, damageTypeStr, disintegrateChance)
}

// applyMonsterProjectileDamageAoE splashes a monster projectile across EVERY
// party member that can still take a hit (AoE spells like a monster's fireball).
func (cs *CombatSystem) applyMonsterProjectileDamageAoE(sourceName string, damage int, damageTypeStr string, disintegrateChance float64) {
	if sourceName == "" {
		sourceName = "Monster"
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s's blast engulfs the whole party!", sourceName))
	for _, member := range cs.game.party.Members {
		if member == nil || !member.CanAct() {
			continue
		}
		cs.applyMonsterProjectileDamageToChar(member, sourceName, damage, damageTypeStr, disintegrateChance)
	}
}

func (cs *CombatSystem) applyMonsterProjectileDamageToChar(currentChar *character.MMCharacter, sourceName string, damage int, damageTypeStr string, disintegrateChance float64) {
	if currentChar == nil {
		return
	}
	finalDamage := cs.mitigateIncoming(cs.applyArmorToCharacterIfPhysical(damage, damageTypeStr, currentChar))

	if sourceName == "" {
		sourceName = "Monster"
	}

	if dodged, _ := cs.RollPerfectDodge(currentChar); !dodged {
		if disintegrateChance > 0 && rand.Float64() < disintegrateChance {
			currentChar.HitPoints = 0
			currentChar.Conditions = []character.Condition{character.ConditionEradicated}
			cs.game.AddCombatMessage(fmt.Sprintf("%s is eradicated by %s!", currentChar.Name, sourceName))
			targetIndex := cs.findCharacterIndex(currentChar)
			cs.game.TriggerDamageBlink(targetIndex)
			return
		}
		currentChar.HitPoints -= finalDamage
		if currentChar.HitPoints < 0 {
			currentChar.HitPoints = 0
		}

		// Add combat message for monster projectile attack
		cs.game.AddCombatMessage(fmt.Sprintf("%s hits %s for %d damage! (HP: %d/%d)",
			sourceName, currentChar.Name, finalDamage,
			currentChar.HitPoints, currentChar.MaxHitPoints))

		if currentChar.HitPoints == 0 {
			currentChar.AddCondition(character.ConditionUnconscious)
			cs.game.AddCombatMessage(fmt.Sprintf("%s falls unconscious!", currentChar.Name))
		}
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("Perfect Dodge! %s evades %s's attack!", currentChar.Name, sourceName))
		return
	}

	targetIndex := cs.findCharacterIndex(currentChar)
	cs.game.TriggerDamageBlink(targetIndex)
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
	case "melee":
		meleeAttack := projectile.(*MeleeAttack)
		weaponDef := lookupWeaponConfigByName(meleeAttack.WeaponName)
		if weaponDef == nil || weaponDef.Graphics == nil {
			return 0, 0, 0, false
		}
		return float64(weaponDef.Graphics.BaseSize), weaponDef.Graphics.MinSize, weaponDef.Graphics.MaxSize, true
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
	case "melee":
		p := projectile.(*MeleeAttack)
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
	// (e.g. the spawn frame, dist≈0) this scale would otherwise balloon — a
	// fireball's 2-tile box × ~3.9 ≈ 8 tiles — so it "hit" and exploded on a
	// monster several tiles away before the projectile was even drawn. Clamping
	// to 1 keeps collision at the world box up close and only shrinks it far away.
	if scale > 1.0 {
		scale = 1.0
	}
	return scale
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
	var isCharmSpell bool
	var charmSeconds int
	var charmLiving bool
	var charmPacify bool
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
		disintegrateChance = mp.DisintegrateChance
		spellID := spells.SpellID(mp.SpellType)
		spellDef, _ := spells.GetSpellDefinitionByID(spellID)
		weaponName = spellDef.Name
		damageTypeStr = normalizeDamageTypeStr(spellDef.School)
		damageType = convertToMonsterDamageType(damageTypeStr)
		aoeRadiusTiles = spellDef.AoeRadiusTiles
		isCharmSpell = spellDef.Charm
		charmSeconds = spellDef.CharmDurationSeconds
		charmLiving = spellDef.CharmLiving
		charmPacify = spellDef.CharmPacify
		stunChance = spellDef.StunChance
		stunSeconds = spellDef.StunDurationSeconds
		stunTurns = spellDef.StunDurationTurns
		starburstFx = spellDef.StarburstFx
		mp.Active = false
		isSpell = true

	case "melee":
		ma := projectile.(*MeleeAttack)
		if !ma.Active || ma.LifeTime <= 0 {
			return
		}
		damage, isCrit = ma.Damage, ma.Crit
		weaponName = ma.WeaponName
		weaponDef = lookupWeaponConfigByName(weaponName)
		damageTypeStr = weaponDamageTypeStr(weaponDef)
		damageType = convertToMonsterDamageType(damageTypeStr)
		ma.Active = false

	case "arrow":
		ar := projectile.(*Arrow)
		if !ar.Active || ar.LifeTime <= 0 {
			return
		}
		damage, isCrit = ar.Damage, ar.Crit
		disintegrateChance = ar.DisintegrateChance
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
	}

	// Party buffs (Hour of Power, Heroism, …): flat bonus to all party outgoing damage.
	if damage > 0 {
		damage += cs.game.combatBuffOutBonus()
	}

	// Weapon-mastery TRUE damage / dodge-ignore (physical weapons only; spells
	// leave these zero/false). Spell schools instead pierce resistance at GM.
	trueDmg, ignoreDodge := cs.weaponMasteryStrike(weaponDef)
	resistPierce := 0
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			resistPierce = cs.spellResistPierce(mp.SpellType)
		}
	}

	// Check monster perfect dodge (applies to all attack types). A Grandmaster
	// weapon strike ignores it; otherwise the normal hit is dodged but mastery
	// TRUE damage still lands.
	if monster.PerfectDodge > 0 && !ignoreDodge && rand.Intn(100) < monster.PerfectDodge {
		if trueDmg > 0 {
			cs.applyTrueDamageThroughDodge(monster, trueDmg, damageType, cs.game.party.Members[cs.game.selectedChar].Name)
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges the %s!", monster.Name, weaponName))
		}
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Charm (bind_undead / Charm): no damage — binds or pacifies the target.
	if isCharmSpell {
		cs.applyCharm(monster, charmSeconds, charmLiving, charmPacify, weaponName)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	if disintegrateChance > 0 && !monsterImmuneToDisintegrate(monster) && rand.Float64() < disintegrateChance {
		if isSpell {
			if mp, ok := projectile.(*MagicProjectile); ok {
				cs.game.CreateSpellHitEffectFromSpell(monster.X, monster.Y, mp.SpellType)
			} else {
				cs.game.CreateSpellHitEffect(monster.X, monster.Y, damageTypeStr, SpellParticleCount, SpellParticleSize)
			}
		} else if isRanged {
			cs.spawnRangedHitEffect(monster, weaponDef, damage)
		}

		monster.HitPoints = 0
		monster.WasAttacked = true
		monster.HitTintFrames = MonsterHitFlashFrames
		cs.engageTurnBasedPackOnHit(monster)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, monster.ID)
		cs.awardExperienceAndGold(monster)

		attackerName := cs.game.party.Members[cs.game.selectedChar].Name
		cs.game.AddCombatMessage(fmt.Sprintf("%s's %s disintegrates %s!", attackerName, weaponName, monster.Name))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", monster.Experience))
		return
	}

	// Spawn hit effects at monster position (after dodge check, so only on actual hits)
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			cs.game.CreateSpellHitEffectFromSpell(monster.X, monster.Y, mp.SpellType)
		} else {
			cs.game.CreateSpellHitEffect(monster.X, monster.Y, damageTypeStr, SpellParticleCount, SpellParticleSize)
		}
	} else if isRanged {
		cs.spawnRangedHitEffect(monster, weaponDef, damage)
	}

	// Calculate damage reduction based on damage type
	reducedDamage := applyArmorReductionIfPhysical(damage, damageTypeStr, monster.ArmorClass, isRanged)
	if !isSpell {
		if mult := cs.weaponBonusMultiplier(weaponDef, monster); mult != 1.0 {
			reducedDamage = int(math.Round(float64(reducedDamage) * mult))
			if reducedDamage < 1 {
				reducedDamage = 1
			}
		}
	}
	reducedDamage += trueDmg // weapon-mastery true damage bypasses armor

	// GM spell mastery pierces part of the target's resistance.
	actualDamage := monster.TakeDamageResist(reducedDamage, damageType, resistPierce, cs.game.camera.X, cs.game.camera.Y)
	monster.HitTintFrames = MonsterHitFlashFrames
	cs.breakCharmOnHit(monster) // any hit frees a pacified (Charm) monster
	cs.engageTurnBasedPackOnHit(monster)
	if monster.IsAlive() {
		cs.tryApplyWeaponStun(monster, weaponDef)
		// Spell stun-on-hit (Psychic Shock): chance to stun the struck monster.
		if stunChance > 0 && rand.Float64() < stunChance {
			cs.applyStun(monster, stunSeconds, stunTurns)
			cs.game.AddCombatMessage(fmt.Sprintf("%s is stunned by %s!", monster.Name, weaponName))
		}
	}
	cs.game.collisionSystem.UnregisterEntity(entityID)

	if !monster.IsAlive() {
		cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, monster.ID)
		cs.awardExperienceAndGold(monster)
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		attackerName := cs.game.party.Members[cs.game.selectedChar].Name
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hits %s for %d damage and kills it!",
			prefix, attackerName, monster.Name, actualDamage))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", monster.Experience))
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

// applyAoeSplash deals the primary attack's base damage to every OTHER alive
// monster within radiusTiles of the primary target. Each splash victim
// applies its own armor reduction. No crit, no disintegrate, no stun on
// splash — those belong to the primary hit only. Drives Fireball-style AoE
// from a single YAML field (`aoe_radius_tiles`), shared between spells and
// weapon projectiles (e.g. Bow of Hellfire).
func (cs *CombatSystem) applyAoeSplash(center *monsterPkg.Monster3D, damage int, damageTypeStr string, damageType monsterPkg.DamageType, weaponName string, radiusTiles float64, resistPierce int) {
	if center == nil || radiusTiles <= 0 {
		return
	}
	tileSize := float64(cs.game.config.GetTileSize())
	radiusPx := radiusTiles * tileSize
	radiusSq := radiusPx * radiusPx
	cx, cy := center.X, center.Y

	for _, m := range cs.game.world.Monsters {
		if m == nil || m == center || !m.IsAlive() {
			continue
		}
		dx := m.X - cx
		dy := m.Y - cy
		if dx*dx+dy*dy > radiusSq {
			continue
		}
		reduced := applyArmorReductionIfPhysical(damage, damageTypeStr, m.ArmorClass, false)
		actual := m.TakeDamageResist(reduced, damageType, resistPierce, cs.game.camera.X, cs.game.camera.Y)
		m.HitTintFrames = MonsterHitFlashFrames
		cs.breakCharmOnHit(m)
		cs.engageTurnBasedPackOnHit(m)
		cs.game.CreateSpellHitEffect(m.X, m.Y, damageTypeStr, SpellParticleCount, SpellParticleSize)

		if !m.IsAlive() {
			cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, m.ID)
			cs.awardExperienceAndGold(m)
			cs.game.AddCombatMessage(fmt.Sprintf("%s splash kills %s! (+%d XP)", weaponName, m.Name, m.Experience))
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
	if monster == nil || weaponDef == nil {
		return
	}
	if weaponDef.StunChance <= 0 {
		return
	}
	if rand.Float64() >= weaponDef.StunChance {
		return
	}

	turns := weaponDef.StunTurns
	if turns <= 0 {
		turns = 1
	}

	if cs.game.turnBasedMode {
		if monster.StunTurnsRemaining <= 0 {
			cs.game.AddCombatMessage(fmt.Sprintf("%s is stunned!", monster.Name))
		}
		if turns > monster.StunTurnsRemaining {
			monster.StunTurnsRemaining = turns
		}
		return
	}

	framesPerTurn := cs.game.config.GetTPS()
	if framesPerTurn <= 0 {
		framesPerTurn = 60
	}
	frames := turns * framesPerTurn
	if monster.StunFramesRemaining <= 0 {
		cs.game.AddCombatMessage(fmt.Sprintf("%s is stunned!", monster.Name))
	}
	if frames > monster.StunFramesRemaining {
		monster.StunFramesRemaining = frames
	}
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

// awardExperienceAndGold gives experience and gold to the party when a monster is killed
func (cs *CombatSystem) awardExperienceAndGold(monster *monsterPkg.Monster3D) {
	// Each living hero — active, reserve, or captive — gets the per-member share.
	cs.game.grantSharedXP(monster.Experience / len(cs.game.party.Members))

	// Check for loot drops
	drops := cs.checkMonsterLootDrop(monster)

	// Update quest progress
	cs.updateQuestProgress(monster)

	// Drop gold/items into a loot bag on the ground
	if monster.Gold > 0 || len(drops) > 0 {
		sizeMultiplier := monster.GetSizeGameMultiplier() / 2.0
		if sizeMultiplier < 0.1 {
			sizeMultiplier = 0.1
		}
		cs.game.addLootBagDrop(monster.X, monster.Y, drops, monster.Gold, sizeMultiplier)
	}
}

// updateQuestProgress updates quest progress when a monster is killed
func (cs *CombatSystem) updateQuestProgress(monster *monsterPkg.Monster3D) {
	if cs.game.questManager == nil {
		return
	}

	// Convert monster name to lowercase key format (e.g., "Goblin" -> "goblin", "Dire Wolf" -> "dire_wolf")
	monsterType := strings.ToLower(strings.ReplaceAll(monster.Name, " ", "_"))

	// Only statue-summoned dragons count toward the win quest. They're flagged
	// at summon (IsEncounterMonster + EncounterRewards.QuestID == "dragon_slayer");
	// any other dragon is ignored so it can never trip the victory.
	if monsterType == "dragon" {
		summoned := monster.IsEncounterMonster && monster.EncounterRewards != nil &&
			monster.EncounterRewards.QuestID == "dragon_slayer"
		if !summoned {
			return
		}
	}

	completedQuests := cs.game.questManager.OnMonsterKilled(monsterType)

	// Notify player of quest completions
	for _, quest := range completedQuests {
		cs.game.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Open Quests (J) to claim reward.", quest.Definition.Name))
	}
}

// checkLevelUp checks if a character should level up and applies level up benefits
func (cs *CombatSystem) checkLevelUp(character *character.MMCharacter) {
	// Level progression: each level requires currentLevel * XPRequiredPerLevel
	// experience. Loop handles multiple level-ups from a single XP gain.
	for {
		requiredExp := character.Level * XPRequiredPerLevel

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

			message := fmt.Sprintf("%s reached level %d! (was level %d) [+%d stat points]",
				character.Name, character.Level, oldLevel, StatPointsPerLevel)
			cs.game.AddCombatMessage(message)

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
	// dodgeable) damage — it now grants flat TRUE damage applied at the hit site
	// (weaponMasteryStrike), which bypasses armor and lands through dodges.
	// ArmsMaster: general weapon expertise — flat bonus with ANY weapon.
	baseDamage += character.ArmsMasterTier() * ArmsMasterDamagePerTier

	// Get effective stats including any stat bonuses (Bless, Day of Gods, etc.)
	might, intellect, _, _, accuracy, _, _ := character.GetEffectiveStats(cs.game.statBonus)

	// Get the appropriate stat bonus based on weapon's primary bonus stat
	var primaryStatBonus int
	switch weaponDef.BonusStat {
	case "Might":
		primaryStatBonus = might / WeaponPrimaryStatDivisor
	case "Accuracy":
		primaryStatBonus = accuracy / WeaponPrimaryStatDivisor
	case "Intellect":
		primaryStatBonus = intellect / WeaponPrimaryStatDivisor
	default:
		// Fallback to Might for weapons without bonus stat specified
		primaryStatBonus = might / WeaponPrimaryStatDivisor
	}

	// Add secondary stat bonus if weapon has dual scaling
	var secondaryStatBonus int
	if weaponDef.BonusStatSecondary != "" {
		switch weaponDef.BonusStatSecondary {
		case "Might":
			secondaryStatBonus = might / WeaponSecondaryStatDivisor
		case "Accuracy":
			secondaryStatBonus = accuracy / WeaponSecondaryStatDivisor
		case "Intellect":
			secondaryStatBonus = intellect / WeaponSecondaryStatDivisor
		}
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

// weaponMasteryStrike returns the TRUE-damage bonus and dodge-ignore flag for the
// active attacker wielding the given weapon. True damage bypasses the target's
// armor class and lands even through a Perfect Dodge; a Grandmaster (tier 3)
// makes the WHOLE strike ignore the target's Perfect Dodge.
func (cs *CombatSystem) weaponMasteryStrike(weaponDef *config.WeaponDefinitionConfig) (trueDmg int, ignoreDodge bool) {
	if weaponDef == nil {
		return 0, false
	}
	attacker := cs.activeAttacker()
	if attacker == nil {
		return 0, false
	}
	skillType, ok := character.WeaponSkillForCategory(strings.ToLower(weaponDef.Category))
	if !ok {
		return 0, false
	}
	tier := attacker.SkillTier(skillType)
	return tier * MasteryWeaponTrueDamagePerTier, tier >= int(character.MasteryGrandMaster)
}

// spellResistPierce returns the resistance-pierce percent for the active caster's
// spell: MagicGMResistPiercePct if they are Grandmaster in that spell's school,
// else 0.
func (cs *CombatSystem) spellResistPierce(spellType string) int {
	caster := cs.activeAttacker()
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
	intellectBonus := char.GetEffectiveIntellect(cs.game.statBonus) / spells.SpellIntellectDivisor
	totalDamage := baseDamage + intellectBonus
	return baseDamage, intellectBonus, totalDamage
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
	// Use effective Luck so Bless/stat bonuses influence crit chance
	return char.GetEffectiveLuck(cs.game.statBonus) / LuckToCritDivisor
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
// monster's `type` (data) — undead and dragons are immune.
func monsterImmuneToDisintegrate(m *monsterPkg.Monster3D) bool {
	if m == nil {
		return false
	}
	return m.MonsterType == "undead" || m.MonsterType == "dragon"
}

// tryCastSpecialEffect runs the data-driven "effect spell" dispatchers in order
// (AoE stun → party buffs → resurrect). Each returns false unless the spell
// carries its trigger field, so the OR-chain stops at the first that handles
// the cast. Returns true if one did — callers must then skip the
// projectile/utility paths. Single place to register a new effect-spell type.
func (cs *CombatSystem) tryCastSpecialEffect(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	return cs.tryCastAoeStun(spellID, def, caster) ||
		cs.tryCastInferno(spellID, def, caster) ||
		cs.tryCastSteamZone(spellID, def, caster) ||
		cs.tryCastPartyBuff(spellID, def, caster) ||
		cs.tryCastRaiseDead(spellID, def, caster) ||
		cs.tryCastResurrect(spellID, def, caster) ||
		cs.tryCastAwaken(spellID, def, caster)
}

// tryCastInferno handles party-centered nova spells (Inferno): every monster AND
// every party member within PartyAoeRadiusTiles of the party takes the spell's
// full damage (cost × SpellDamagePerSP). Gated on PartyAoeRadiusTiles > 0.
func (cs *CombatSystem) tryCastInferno(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.PartyAoeRadiusTiles <= 0 {
		return false
	}
	dmg := def.SpellPointsCost * spells.SpellDamagePerSP
	radius := def.PartyAoeRadiusTiles * float64(cs.game.config.GetTileSize())
	cx, cy := cs.game.camera.X, cs.game.camera.Y
	damageTypeStr := normalizeDamageTypeStr(def.School)
	damageType := convertToMonsterDamageType(damageTypeStr)

	cs.game.AddCombatMessage(fmt.Sprintf("%s erupts around the party!", def.Name))

	// Monsters in range.
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || Distance(cx, cy, m.X, m.Y) > radius {
			continue
		}
		reduced := applyArmorReductionIfPhysical(dmg, damageTypeStr, m.ArmorClass, false)
		m.TakeDamageResist(reduced, damageType, 0, cx, cy)
		m.HitTintFrames = MonsterHitFlashFrames
		cs.breakCharmOnHit(m)
		cs.engageTurnBasedPackOnHit(m)
		cs.game.CreateSpellHitEffect(m.X, m.Y, damageTypeStr, SpellParticleCount, SpellParticleSize)
		if !m.IsAlive() {
			cs.game.collisionSystem.UnregisterEntity(m.ID)
			cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, m.ID)
			cs.awardExperienceAndGold(m)
			cs.game.AddCombatMessage(fmt.Sprintf("%s is consumed by %s! (+%d XP)", m.Name, def.Name, m.Experience))
		}
	}

	// The party is caught in the blast too — flat, equal damage (no mitigation).
	for idx, member := range cs.game.party.Members {
		if member == nil || member.HitPoints <= 0 {
			continue
		}
		member.HitPoints -= dmg
		if member.HitPoints < 0 {
			member.HitPoints = 0
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s is scorched for %d! (HP: %d/%d)",
			member.Name, dmg, member.HitPoints, member.MaxHitPoints))
		if member.HitPoints == 0 {
			member.AddCondition(character.ConditionUnconscious)
			cs.game.AddCombatMessage(fmt.Sprintf("%s falls unconscious!", member.Name))
		}
		cs.game.TriggerDamageBlink(idx)
		cs.game.TriggerPartyFlame(idx) // flame-particle overlay on the burned card
	}
	return true
}

// tryCastRaiseDead handles Raise Dead: revives the first fallen ally that is
// Unconscious or Dead (NOT eradicated — that's Resurrect's domain) to
// ReviveHpPct% of max HP, clearing both conditions. Returns true if it handled
// the spell. Gated on ReviveHpPct > 0 so it never collides with Resurrect.
func (cs *CombatSystem) tryCastRaiseDead(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
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
		// Nothing to raise — refund the SP actually paid (matches Resurrect/Awaken).
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
// back to 1 HP (does not touch the truly dead/eradicated — that's Resurrect).
// Shared by both cast paths. Returns true if it handled the spell.
func (cs *CombatSystem) tryCastAwaken(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
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
		// No one to wake — refund the SP actually paid (matches Meditation discount).
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		cs.game.AddCombatMessage("No one is unconscious to awaken.")
		return true
	}
	cs.game.AddCombatMessage(fmt.Sprintf("Awakening rouses %d fallen ally(s) back to 1 HP!", revived))
	return true
}

// tryCastResurrect handles the Resurrect spell: restores the first fallen party
// member (unconscious, dead, or even eradicated) — to full HP if FullHeal.
// Shared by both cast paths. Returns true if it handled the spell.
func (cs *CombatSystem) tryCastResurrect(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
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
		// Nothing to resurrect — refund the spell points actually paid (matches
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

// applyStun stuns a single monster for `seconds` real-time (× TPS frames) and
// `turns` turn-based turns, taking the max with any existing stun.
func (cs *CombatSystem) applyStun(m *monsterPkg.Monster3D, seconds, turns int) {
	if frames := seconds * cs.game.config.GetTPS(); frames > m.StunFramesRemaining {
		m.StunFramesRemaining = frames
	}
	if turns > m.StunTurnsRemaining {
		m.StunTurnsRemaining = turns
	}
}

// applyCharm binds or pacifies a monster. bind_undead (living=false) only works
// on undead and makes them fight other monsters. Charm (living=true) only works
// on the LIVING and merely pacifies them — they stop attacking and break free on
// any hit they take (handled in breakCharmOnHit). No damage is dealt either way.
func (cs *CombatSystem) applyCharm(m *monsterPkg.Monster3D, seconds int, living, pacify bool, spellName string) {
	isUndead := m.MonsterType == "undead"
	if !living && !isUndead {
		cs.game.AddCombatMessage(fmt.Sprintf("%s washes over %s — only the undead can be bound.", spellName, m.Name))
		return
	}
	if living && isUndead {
		cs.game.AddCombatMessage(fmt.Sprintf("%s has no hold over the undead %s.", spellName, m.Name))
		return
	}
	m.Charmed = true
	m.CharmPacified = pacify
	m.CharmFramesRemaining = seconds * cs.game.config.GetTPS()
	m.CharmAttackCD = 0
	m.WasAttacked = false
	if pacify {
		cs.game.AddCombatMessage(fmt.Sprintf("%s is charmed and stops attacking!", m.Name))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s is bound to your will!", m.Name))
	}
}

// breakCharmOnHit releases a pacified (Charm) monster the instant it takes any
// hit — it snaps out of the charm and re-aggros. Bound undead (bind_undead) are
// unaffected. Called wherever the party deals damage to a monster.
func (cs *CombatSystem) breakCharmOnHit(m *monsterPkg.Monster3D) {
	if m.Charmed && m.CharmPacified {
		m.Charmed = false
		m.CharmPacified = false
		m.CharmFramesRemaining = 0
		m.WasAttacked = true
		cs.game.AddCombatMessage(fmt.Sprintf("%s breaks free of the charm!", m.Name))
	}
}

// charmedAttackNearest makes a charmed monster strike the nearest OTHER alive,
// non-charmed monster within its alert radius. A kill awards the party normally
// (XP + loot). Returns true if it attacked something.
func (cs *CombatSystem) charmedAttackNearest(m *monsterPkg.Monster3D) bool {
	aggro := m.AlertRadius
	if aggro <= 0 {
		aggro = float64(cs.game.config.GetTileSize()) * 6
	}
	var target *monsterPkg.Monster3D
	best := aggro
	for _, other := range cs.game.world.Monsters {
		if other == nil || other == m || !other.IsAlive() || other.Charmed {
			continue
		}
		if d := Distance(m.X, m.Y, other.X, other.Y); d <= best {
			best, target = d, other
		}
	}
	if target == nil {
		return false
	}
	dmg := m.GetAttackDamage()
	target.TakeDamage(dmg, monsterPkg.DamagePhysical, m.X, m.Y)
	target.HitTintFrames = MonsterHitFlashFrames
	cs.game.AddCombatMessage(fmt.Sprintf("%s (bound) strikes %s for %d!", m.Name, target.Name, dmg))
	if !target.IsAlive() {
		cs.game.AddCombatMessage(fmt.Sprintf("%s slays %s!", m.Name, target.Name))
		cs.game.collisionSystem.UnregisterEntity(target.ID)
		cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, target.ID)
		cs.awardExperienceAndGold(target)
	}
	return true
}

// awardExperienceOnly grants the party a monster's XP with NO gold or loot — used
// when a bound (charmed) monster perishes as the party leaves the map.
func (cs *CombatSystem) awardExperienceOnly(monster *monsterPkg.Monster3D) {
	if len(cs.game.party.Members) == 0 {
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
func (cs *CombatSystem) tryCastAoeStun(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
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
		if frames > m.StunFramesRemaining {
			m.StunFramesRemaining = frames
		}
		if turns > m.StunTurnsRemaining {
			m.StunTurnsRemaining = turns
		}
		stunned++
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s engulfs the area — %d foe(s) stunned!", def.Name, stunned))
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

// tryCastPartyBuff handles party combat-buff spells (Day of the Gods, Hour of
// Power). If the spell carries any party-buff field it activates the buff for
// `duration` seconds and returns true. Shared by both cast paths.
func (cs *CombatSystem) tryCastPartyBuff(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.ResistBuffPct <= 0 && def.OutgoingDamageBonus <= 0 && def.IncomingDamageReduction <= 0 {
		return false
	}
	frames := def.Duration * cs.game.config.GetTPS()
	cs.game.addCombatBuff(TimedCombatBuff{
		SpellID:   string(spellID),
		Frames:    frames,
		OutBonus:  def.OutgoingDamageBonus,
		InReduce:  def.IncomingDamageReduction,
		ResistPct: def.ResistBuffPct,
	})
	cs.game.AddCombatMessage(fmt.Sprintf("%s empowers the party!", def.Name))
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

// mitigateIncoming reduces a single incoming-damage value by the party's active
// defensive buffs: Day of the Gods (% reduction) then Hour of Power (flat),
// floored at 0. Applied at every party damage-application site.
func (cs *CombatSystem) mitigateIncoming(dmg int) int {
	if dmg <= 0 {
		return dmg
	}
	if pct := cs.game.combatBuffResistPct(); pct > 0 {
		dmg = dmg * (100 - pct) / 100
	}
	if red := cs.game.combatBuffInReduce(); red > 0 {
		dmg -= red
	}
	if dmg < 0 {
		dmg = 0
	}
	return dmg
}

// applyBlessEffect applies the Bless spell effect consistently across all casting methods
func (cs *CombatSystem) applyBlessEffect(duration, statBonus int) {
	// If Bless is already active, remove old bonus before applying new one
	if cs.game.blessActive {
		cs.game.statBonus -= cs.game.blessStatBonus
	}
	cs.game.blessActive = true
	cs.game.blessDuration = duration
	cs.game.blessStatBonus = statBonus // Store the bonus for proper removal later
	cs.game.statBonus += statBonus     // ADD to total stat bonus
}

// RollPerfectDodge returns whether the character performs a perfect dodge and the chance used.
// chance = effective Luck / LuckToDodgeDivisor, clamped to [0,100].
// armorGMDodgeBonus grants ArmorGMDodgeBonus dodge for each Grandmaster-mastered
// armor type the character is wearing at least one piece of (e.g. GM Plate +
// plate equipped → +5; also GM Shield + shield in the off-hand → +10).
func (cs *CombatSystem) armorGMDodgeBonus(chr *character.MMCharacter) int {
	if chr == nil {
		return 0
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
	return len(gmTypes) * ArmorGMDodgeBonus
}

func (cs *CombatSystem) RollPerfectDodge(chr *character.MMCharacter) (bool, int) {
	// Use effective stats so Bless and equipment affect dodge
	chance := chr.GetEffectiveLuck(cs.game.statBonus)/LuckToDodgeDivisor + cs.armorGMDodgeBonus(chr)
	if chance < 0 {
		chance = 0
	}
	if chance > 100 {
		chance = 100
	}
	roll := rand.Intn(100)
	return roll < chance, chance
}

// ApplyArmorDamageReduction calculates final damage after armor reduction (YAML-driven items)
func (cs *CombatSystem) ApplyArmorDamageReduction(damage int, char *character.MMCharacter) int {
	totalArmorClass := cs.CalculateTotalArmorClass(char)

	damageReduction := totalArmorClass / ArmorPhysicalReductionDivisor

	// Apply damage reduction
	finalDamage := damage - damageReduction

	// DisarmTrap PLACEHOLDER effect: until trap tiles exist, the skill simply
	// shaves a flat amount off incoming damage per mastery tier.
	// TODO: implement the real trap-disarming mechanic — trap special-tiles on
	// maps that trigger damage/effects unless a party member's DisarmTrap check
	// defuses them — and remove this stand-in damage reduction.
	finalDamage -= char.DisarmTrapTier() * DisarmTrapDamageReductionPerTier

	if finalDamage < 1 {
		finalDamage = 1 // Minimum 1 damage (armor can't completely negate damage)
	}

	return finalDamage
}

func (cs *CombatSystem) applyArmorToCharacterIfPhysical(damage int, damageTypeStr string, char *character.MMCharacter) int {
	if isPhysicalDamageType(damageTypeStr) {
		return cs.ApplyArmorDamageReduction(damage, char)
	}
	return damage
}

func isPhysicalDamageType(damageTypeStr string) bool {
	return strings.EqualFold(strings.TrimSpace(damageTypeStr), "physical")
}

func normalizeDamageTypeStr(damageTypeStr string) string {
	normalized := strings.TrimSpace(damageTypeStr)
	if normalized == "" {
		return "physical"
	}
	return normalized
}

func weaponDamageTypeStr(weaponDef *config.WeaponDefinitionConfig) string {
	if weaponDef != nil && weaponDef.DamageType != "" {
		return weaponDef.DamageType
	}
	return "physical"
}

func spellDamageTypeStr(spellType string) string {
	if spellDef, err := spells.GetSpellDefinitionByID(spells.SpellID(spellType)); err == nil {
		return normalizeDamageTypeStr(spellDef.School)
	}
	return "physical"
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

func applyArmorReductionIfPhysical(damage int, damageTypeStr string, armorClass int, isRanged bool) int {
	reducedDamage := damage
	if isPhysicalDamageType(damageTypeStr) {
		applyArmor := true
		if isRanged && rand.Intn(100) < ArmorPierceRangedChancePct {
			applyArmor = false // Armor piercing shot!
		}
		if applyArmor {
			armorReduction := armorClass / ArmorPhysicalReductionDivisor
			reducedDamage = damage - armorReduction
			if reducedDamage < 1 {
				reducedDamage = 1 // Minimum 1 damage
			}
		}
	}
	return reducedDamage
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
			var drop items.Item
			var err error
			switch e.Type {
			case "weapon":
				drop, err = items.TryCreateWeaponFromYAML(e.Key)
			case "item":
				drop, err = items.TryCreateItemFromYAML(e.Key)
			default:
				continue
			}
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

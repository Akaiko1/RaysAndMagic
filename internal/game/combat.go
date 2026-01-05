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

// getWeaponConfig safely retrieves weapon definition and reports missing configs without panicking.
// Returns nil if weapon not found or missing required config section.
func (cs *CombatSystem) getWeaponConfig(weaponName string) *config.WeaponDefinitionConfig {
	weaponKey := items.GetWeaponKeyByName(weaponName)
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		fmt.Printf("[WARN] weapon '%s' (key: %s) not found in weapons.yaml\n", weaponName, weaponKey)
		return nil
	}
	return weaponDef
}

// getWeaponConfigByKey safely retrieves weapon definition by key without panicking.
func (cs *CombatSystem) getWeaponConfigByKey(weaponKey string) *config.WeaponDefinitionConfig {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		fmt.Printf("[WARN] weapon key '%s' not found in weapons.yaml\n", weaponKey)
		return nil
	}
	return weaponDef
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
		spellCost = spell.SpellCost
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

	// Dynamic spell effect to spell ID mapping (YAML-based)
	spellID := spells.SpellID(items.SpellEffectToSpellID(spell.SpellEffect))

	// Check if this is a utility spell (non-projectile)
	castingSystem := spells.NewCastingSystem(cs.game.config)
	spellDef, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return false
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
				// Both heal and heal_other now use mouse targeting (handled by CastEquippedSpellWithTarget)
				// Fallback to self-heal if no target specified
				caster.HitPoints += result.HealAmount
				if caster.HitPoints > caster.MaxHitPoints {
					caster.HitPoints = caster.MaxHitPoints
				}
				if caster.HitPoints > 0 {
					caster.RemoveCondition(character.ConditionUnconscious)
				}
			}

			// Apply utility spell effects dynamically based on spell ID
			switch string(spellID) {
			case "torch_light":
				cs.game.torchLightActive = true
				cs.game.torchLightDuration = result.Duration
				cs.game.torchLightRadius = 4.0 // 4-tile radius
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
			cs.recordSpellCast(caster, spellID)
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
		projectile.Damage *= 2
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
	cs.recordSpellCast(caster, spellID)
	return true
}

// EquipmentHeal casts heal using equipped spell (special targeting for heal spells)
func (cs *CombatSystem) EquipmentHeal() {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot cast heals
	if caster.IsIncapacitated() {
		return
	}

	// Check if character has a spell equipped
	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return // No spell equipped
	}

	// Only allow heal-type spells for this function
	if spell.SpellEffect != items.SpellEffectHealSelf && spell.SpellEffect != items.SpellEffectHealOther {
		return // Not a heal spell, use F key for other spells
	}

	// Check spell points (use spell cost for utility spells)
	spellCost := spell.SpellCost
	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			caster.Name, caster.SpellPoints, spellCost))
		return
	}

	// Map item spell effects to spell IDs dynamically
	spellIDStr := items.SpellEffectToSpellID(spell.SpellEffect)
	if spellIDStr == "" {
		// Unknown utility spell, exit
		return
	}

	// Cast the utility spell
	caster.SpellPoints -= spellCost

	// Use the spell casting system
	castingSystem := spells.NewCastingSystem(cs.game.config)
	spellID := spells.SpellID(spellIDStr)
	result, err := castingSystem.ApplyUtilitySpell(spellID, caster.Personality)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return
	}

	if result.Success {
		if _, _, totalHeal := cs.CalculateSpellHealing(spellID, caster); totalHeal > 0 {
			result.HealAmount = totalHeal
		}
		// Apply spell effects based on result
		if result.HealAmount > 0 {
			if result.TargetSelf {
				// Heal self
				caster.HitPoints += result.HealAmount
				if caster.HitPoints > caster.MaxHitPoints {
					caster.HitPoints = caster.MaxHitPoints
				}
			} else {
				// For heal other, use the currently selected target or self if none
				targetIndex := cs.game.selectedChar // Default to self
				// TODO: Add target selection mechanism for heal other
				target := cs.game.party.Members[targetIndex]
				target.HitPoints += result.HealAmount
				if target.HitPoints > target.MaxHitPoints {
					target.HitPoints = target.MaxHitPoints
				}
			}
		}

		// TODO: Apply other effects like vision bonus, stat bonus, duration effects
		// For now, just show the message
		cs.game.AddCombatMessage(result.Message)
		cs.recordSpellCast(caster, spellID)
	} else {
		cs.game.AddCombatMessage(result.Message)
	}
}

// CastEquippedHealOnTarget casts heal using equipped spell on specified party member
func (cs *CombatSystem) CastEquippedHealOnTarget(targetIndex int) {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Unconscious characters cannot cast heals
	if caster.IsIncapacitated() {
		return
	}

	// Check if character has a heal spell equipped
	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return // No spell equipped
	}

	// Allow both heal-type spells for targeting
	if spell.SpellEffect != items.SpellEffectHealSelf && spell.SpellEffect != items.SpellEffectHealOther {
		return // Not a heal spell
	}

	// Map item spell effects to spell IDs dynamically
	spellIDStr := items.SpellEffectToSpellID(spell.SpellEffect)
	if spellIDStr == "" {
		return
	}
	spellID := spells.SpellID(spellIDStr)

	// Check spell points (use spell cost for utility spells)
	spellCost := spell.SpellCost
	if caster.SpellPoints < spellCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			caster.Name, caster.SpellPoints, spellCost))
		return
	}

	// For First Aid (SpellEffectHealSelf), only allow self-targeting
	if spell.SpellEffect == items.SpellEffectHealSelf && targetIndex != cs.game.selectedChar {
		return // First Aid can only target self
	}

	// Check if target index is valid
	if targetIndex < 0 || targetIndex >= len(cs.game.party.Members) {
		return
	}

	target := cs.game.party.Members[targetIndex]

	// Heal must not revive characters at 0 HP / Dead.
	if target.HitPoints <= 0 || target.HasCondition(character.ConditionDead) || target.HasCondition(character.ConditionEradicated) {
		cs.game.AddCombatMessage(fmt.Sprintf("%s cannot be healed from 0 HP.", target.Name))
		return
	}

	// Cast heal on target
	caster.SpellPoints -= spellCost
	// Calculate heal amount using centralized spell formula
	_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)
	target.HitPoints += healAmount
	if target.HitPoints > target.MaxHitPoints {
		target.HitPoints = target.MaxHitPoints
	}
	if target.HitPoints > 0 {
		target.RemoveCondition(character.ConditionUnconscious)
	}

	// Print feedback message
	if targetIndex == cs.game.selectedChar {
		message := fmt.Sprintf("%s healed themselves for %d HP with %s!", caster.Name, healAmount, spell.Name)
		cs.game.AddCombatMessage(message)
	} else {
		message := fmt.Sprintf("%s healed %s for %d HP with %s!", caster.Name, target.Name, healAmount, spell.Name)
		cs.game.AddCombatMessage(message)
	}
	cs.recordSpellCast(caster, spellID)
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

	// Check if weapon is a bow (range > 3 tiles indicates ranged weapon)
	// For ranged: roll crit and apply doubling inside createArrowAttack only.
	if weapon.Range > 3 {
		cs.createArrowAttack(totalDamage)
		return
	}

	// Melee: determine critical hit based on weapon base crit chance + Luck bonus
	weaponDef := cs.getWeaponConfig(weapon.Name)
	if weaponDef == nil {
		return // Weapon not found, skip attack
	}
	baseCrit := 0
	if weaponDef.CritChance > 0 {
		baseCrit = weaponDef.CritChance
	}
	isCrit, _ := cs.RollCriticalChance(baseCrit, attacker)
	if isCrit {
		totalDamage *= 2
	}

	// Create instant melee attack for close-range weapons
	cs.createMeleeAttack(weapon, totalDamage, isCrit)
}

// createArrowAttack creates a projectile arrow attack
func (cs *CombatSystem) createArrowAttack(damage int) {
	// Find the equipped bow's YAML key
	attacker := cs.game.party.Members[cs.game.selectedChar]
	weapon, hasWeapon := attacker.Equipment[items.SlotMainHand]
	bowKey := "hunting_bow"
	if hasWeapon && weapon.Range > 3 {
		bowKey = items.GetWeaponKeyByName(weapon.Name)
	}

	// Check max projectiles limit for this weapon
	if hasWeapon && weapon.MaxProjectiles > 0 {
		// Count active arrows from this specific bow
		activeArrowsFromBow := 0
		for _, arrow := range cs.game.arrows {
			if arrow.Active && arrow.BowKey == bowKey {
				activeArrowsFromBow++
			}
		}

		// If we've reached the limit, don't create a new arrow
		if activeArrowsFromBow >= weapon.MaxProjectiles {
			return
		}
	}

	// Get weapon-specific properties from YAML
	weaponDef, exists := config.GetWeaponDefinition(bowKey)
	var arrowSpeed float64
	var arrowLifetime int
	var collisionSize float64
	disintegrateChance := 0.0
	if exists {
		disintegrateChance = weaponDef.DisintegrateChance
	}

	tileSize := cs.game.config.GetTileSize()
	if exists && weaponDef.Physics != nil {
		// Use weapon-specific physics properties (tile-based)
		arrowSpeed = weaponDef.Physics.GetSpeedPixels(tileSize)
		arrowLifetime = weaponDef.Physics.GetLifetimeFrames()
		collisionSize = weaponDef.Physics.GetCollisionSizePixels(tileSize)
	} else {
		// Fallback to default config values
		arrowSpeed = cs.game.config.GetArrowSpeed()
		arrowLifetime = cs.game.config.GetArrowLifetime()
		collisionSize = float64(cs.game.config.GetArrowCollisionSize())
	}

	// Determine damage type from weapon
	damageType := "physical" // Default
	if hasWeapon && weapon.DamageType != "" {
		damageType = weapon.DamageType
	}

	// Roll for critical hit: base weapon crit + Luck bonus
	baseCrit := 0
	if exists {
		baseCrit = weaponDef.CritChance
	}
	isCrit, _ := cs.RollCriticalChance(baseCrit, attacker)
	if isCrit {
		damage *= 2
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
		DisintegrateChance: disintegrateChance,
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
	weaponDef := cs.getWeaponConfig(weapon.Name)
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

	// Create visual slash effect
	if graphicsConfig != nil {
		style := meleeEffectStyle(weaponDef, meleeConfig)
		sweep := 0.0
		if style == SlashEffectStyleSlash {
			sweep = float64(meleeConfig.ArcAngle) * math.Pi / 180.0
		}
		slashEffect := SlashEffect{
			ID:             cs.game.GenerateProjectileID("slash"),
			X:              cs.game.camera.X,
			Y:              cs.game.camera.Y,
			Angle:          cs.game.camera.Angle,
			SweepAngle:     sweep,
			Width:          graphicsConfig.SlashWidth,
			Length:         graphicsConfig.SlashLength,
			Color:          graphicsConfig.SlashColor,
			AnimationFrame: 0,
			MaxFrames:      meleeConfig.AnimationFrames,
			Active:         true,
			Style:          style,
		}
		cs.game.slashEffects = append(cs.game.slashEffects, slashEffect)
	}

	// Perform instant hit detection in arc
	cs.performMeleeHitDetection(weapon, totalDamage, meleeConfig, isCrit)
}

func meleeEffectStyle(weaponDef *config.WeaponDefinitionConfig, meleeConfig *config.MeleeAttackConfig) SlashEffectStyle {
	if weaponDef == nil {
		return SlashEffectStyleSlash
	}
	category := strings.ToLower(weaponDef.Category)
	if strings.Contains(category, "spear") ||
		strings.Contains(category, "dagger") ||
		strings.Contains(category, "knife") ||
		strings.Contains(category, "rapier") {
		return SlashEffectStyleThrust
	}
	if meleeConfig != nil && meleeConfig.ArcAngle > 0 && meleeConfig.ArcAngle <= 40 {
		return SlashEffectStyleThrust
	}
	return SlashEffectStyleSlash
}

// performMeleeHitDetection checks for monsters in the weapon's swing arc and applies damage
func (cs *CombatSystem) performMeleeHitDetection(weapon items.Item, damage int, meleeConfig *config.MeleeAttackConfig, isCrit bool) {
	playerX := cs.game.camera.X
	playerY := cs.game.camera.Y
	playerAngle := cs.game.camera.Angle

	// Convert range from tiles to pixels
	tileSize := float64(cs.game.config.GetTileSize())
	attackRange := float64(weapon.Range) * tileSize

	// Convert arc angle from degrees to radians
	arcAngleRad := float64(meleeConfig.ArcAngle) * math.Pi / 180.0
	halfArc := arcAngleRad / 2.0

	// Check all monsters
	for _, monster := range cs.game.world.Monsters {
		if !monster.IsAlive() {
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
// This is for melee attacks - AC applies as damage reduction
func (cs *CombatSystem) ApplyDamageToMonster(monster *monsterPkg.Monster3D, damage int, weaponName string, isCrit bool) {
	// Check monster perfect dodge first
	if monster.PerfectDodge > 0 && rand.Intn(100) < monster.PerfectDodge {
		cs.game.AddCombatMessage(fmt.Sprintf("%s dodges %s's attack!", monster.Name, cs.game.party.Members[cs.game.selectedChar].Name))
		return
	}

	// Apply monster armor class as damage reduction (same formula as player armor)
	armorReduction := monster.ArmorClass / 2
	reducedDamage := damage - armorReduction
	if reducedDamage < 1 {
		reducedDamage = 1 // Minimum 1 damage
	}

	weaponDef := cs.getWeaponConfig(weaponName)
	if mult := cs.weaponBonusMultiplier(weaponDef, monster); mult != 1.0 {
		reducedDamage = int(math.Round(float64(reducedDamage) * mult))
		if reducedDamage < 1 {
			reducedDamage = 1
		}
	}

	// Apply damage with resistances and distance-aware AI response
	finalDamage := monster.TakeDamage(reducedDamage, monsterPkg.DamagePhysical, cs.game.camera.X, cs.game.camera.Y)
	monster.HitTintFrames = cs.game.config.UI.DamageBlinkFrames
	cs.engageTurnBasedPackOnHit(monster)

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
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience and %d gold.",
			monster.Experience, monster.Gold))
	}
}

// engageTurnBasedPackOnHit ensures a hit in turn-based mode pulls in nearby same-type monsters.
func (cs *CombatSystem) engageTurnBasedPackOnHit(hit *monsterPkg.Monster3D) {
	if !cs.game.turnBasedMode || hit == nil {
		return
	}

	tileSize := float64(cs.game.config.GetTileSize())
	radius := tileSize * 8.0
	hitName := hit.Name

	for _, m := range cs.game.world.Monsters {
		if !m.IsAlive() {
			continue
		}
		if m.Name != hitName {
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

// CastSelectedSpell casts the currently selected spell from the spellbook
func (cs *CombatSystem) CastSelectedSpell() {
	currentChar := cs.game.party.Members[cs.game.selectedChar]

	// Prevent casting while down; also avoids utility healing from acting as a revive.
	if currentChar.IsIncapacitated() {
		return
	}
	schools := currentChar.GetAvailableSchools()

	if cs.game.selectedSchool >= len(schools) {
		return
	}

	selectedSchool := schools[cs.game.selectedSchool]
	availableSpells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell >= len(availableSpells) {
		return
	}

	selectedSpellID := availableSpells[cs.game.selectedSpell]
	selectedSpellDef, err := spells.GetSpellDefinitionByID(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return
	}

	// Check spell points
	if currentChar.SpellPoints < selectedSpellDef.SpellPointsCost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)",
			currentChar.Name, currentChar.SpellPoints, selectedSpellDef.SpellPointsCost))
		return
	}

	// Cast the spell
	currentChar.SpellPoints -= selectedSpellDef.SpellPointsCost

	// Dynamic spell casting - no more hardcoded switches!
	castingSystem := spells.NewCastingSystem(cs.game.config)

	if selectedSpellDef.IsProjectile {
		// Handle projectile spells dynamically using effective intellect (includes Bless bonus)
		effectiveIntellect := currentChar.GetEffectiveIntellect(cs.game.statBonus)
		projectile, err := castingSystem.CreateProjectile(selectedSpellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return
		}
		// Override damage with centralized calculation (includes mastery bonus).
		if _, _, totalDamage := cs.CalculateSpellDamage(selectedSpellID, currentChar); totalDamage > 0 {
			projectile.Damage = totalDamage
		}

		// Determine critical hit for spells based on Luck only (no base crit for spells)
		isCrit, _ := cs.RollCriticalChance(0, currentChar)
		if isCrit {
			projectile.Damage *= 2
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
			return
		}
		tileSize := cs.game.config.GetTileSize()
		collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
		projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(projectileEntity)

		// Add message based on spell definition
		cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", selectedSpellDef.Name))
		cs.recordSpellCast(currentChar, selectedSpellID)

	} else if selectedSpellDef.IsUtility {
		// Handle utility spells using centralized system
		result, err := castingSystem.ApplyUtilitySpell(selectedSpellID, currentChar.Personality)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return
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
				currentChar.HitPoints += result.HealAmount
				if currentChar.HitPoints > currentChar.MaxHitPoints {
					currentChar.HitPoints = currentChar.MaxHitPoints
				}
			}

			// Apply vision effects
			if result.VisionBonus > 0 {
				switch string(selectedSpellID) {
				case "torch_light":
					cs.game.torchLightActive = true
					cs.game.torchLightDuration = result.Duration
					cs.game.torchLightRadius = 4.0
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

			// Apply awakening effects
			if result.Awaken {
				// TODO: Remove sleep/paralysis conditions from party
			}

			cs.game.setUtilityStatus(selectedSpellID, result.Duration)
			cs.recordSpellCast(currentChar, selectedSpellID)
		}
	}
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

	if cs.game.selectedSpell >= len(availableSpells) {
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

		dist := Distance(cs.game.camera.X, cs.game.camera.Y, monster.X, monster.Y)

		// If monster is in attacking state and within attack range, perform attack
		attackRange := monster.GetAttackRangePixels()
		if monster.State == monsterPkg.StateAttacking && dist < attackRange {
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

func (cs *CombatSystem) applyMonsterMeleeDamage(monster *monsterPkg.Monster3D, dist float64) {
	if monster.FireburstChance > 0 && rand.Float64() < monster.FireburstChance {
		cs.applyMonsterFireburst(monster)
		return
	}

	// Monster attacks character with highest endurance (and HP > 0)
	currentChar := cs.findHighestEnduranceTarget()
	damage := monster.GetAttackDamage()

	// Apply armor damage reduction
	finalDamage := cs.ApplyArmorDamageReduction(damage, currentChar)

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
	if spellDefConfig, exists := config.GetSpellDefinition(string(spellID)); exists && spellDefConfig != nil {
		disintegrateChance = spellDefConfig.DisintegrateChance
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
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

	tileSize := cs.game.config.GetTileSize()
	collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
	projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(projectileEntity)
}

func (cs *CombatSystem) spawnMonsterWeaponProjectile(monster *monsterPkg.Monster3D, weaponKey string) {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	disintegrateChance := 0.0
	if exists {
		disintegrateChance = weaponDef.DisintegrateChance
	}

	tileSize := cs.game.config.GetTileSize()
	arrowSpeed := cs.game.config.GetArrowSpeed()
	arrowLifetime := cs.game.config.GetArrowLifetime()
	collisionSize := float64(cs.game.config.GetArrowCollisionSize())
	if exists && weaponDef.Physics != nil {
		arrowSpeed = weaponDef.Physics.GetSpeedPixels(tileSize)
		arrowLifetime = weaponDef.Physics.GetLifetimeFrames()
		collisionSize = weaponDef.Physics.GetCollisionSizePixels(tileSize)
	}

	damageType := "physical"
	if exists && weaponDef.DamageType != "" {
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
		DisintegrateChance: disintegrateChance,
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
			cs.applyMonsterProjectileDamage(mp.SourceName, mp.Damage, mp.DisintegrateChance)
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
			cs.applyMonsterProjectileDamage(ar.SourceName, ar.Damage, ar.DisintegrateChance)
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

func (cs *CombatSystem) applyMonsterProjectileDamage(sourceName string, damage int, disintegrateChance float64) {
	currentChar := cs.findHighestEnduranceTarget()
	finalDamage := cs.ApplyArmorDamageReduction(damage, currentChar)

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
		weaponDef := cs.getWeaponConfig(meleeAttack.WeaponName)
		if weaponDef == nil || weaponDef.Graphics == nil {
			return 0, 0, 0, false
		}
		return float64(weaponDef.Graphics.BaseSize), weaponDef.Graphics.MinSize, weaponDef.Graphics.MaxSize, true
	case "arrow":
		arrow := projectile.(*Arrow)
		weaponDef := cs.getWeaponConfigByKey(arrow.BowKey)
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
	return visualSize / baseSize
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
	var arrowVelX, arrowVelY float64
	var disintegrateChance float64

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
		damageTypeStr = spellDef.School
		damageType = monsterPkg.DamageFire // Default
		if monsterPkg.MonsterConfig != nil {
			if ct, err := monsterPkg.MonsterConfig.ConvertDamageType(damageTypeStr); err == nil {
				damageType = ct
			}
		}
		mp.Active = false
		isSpell = true

	case "melee":
		ma := projectile.(*MeleeAttack)
		if !ma.Active || ma.LifeTime <= 0 {
			return
		}
		damage, isCrit = ma.Damage, ma.Crit
		weaponName = ma.WeaponName
		damageType = monsterPkg.DamagePhysical
		damageTypeStr = "physical"
		ma.Active = false

	case "arrow":
		ar := projectile.(*Arrow)
		if !ar.Active || ar.LifeTime <= 0 {
			return
		}
		damage, isCrit = ar.Damage, ar.Crit
		disintegrateChance = ar.DisintegrateChance
		weaponName = "Arrow"
		damageTypeStr = ar.DamageType
		arrowVelX, arrowVelY = ar.VelX, ar.VelY
		damageType = monsterPkg.DamagePhysical // Default
		if monsterPkg.MonsterConfig != nil {
			if ct, err := monsterPkg.MonsterConfig.ConvertDamageType(ar.DamageType); err == nil {
				damageType = ct
			}
		}
		ar.Active = false
		isRanged = true
		if ar.Owner == ProjectileOwnerPlayer && ar.BowKey != "" {
			weaponDef = cs.getWeaponConfigByKey(ar.BowKey)
		}
	}

	// Check monster perfect dodge (applies to all attack types)
	if monster.PerfectDodge > 0 && rand.Intn(100) < monster.PerfectDodge {
		cs.game.AddCombatMessage(fmt.Sprintf("%s dodges the %s!", monster.Name, weaponName))
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	if disintegrateChance > 0 && rand.Float64() < disintegrateChance {
		if isSpell {
			if mp, ok := projectile.(*MagicProjectile); ok {
				cs.game.CreateSpellHitEffectFromSpell(monster.X, monster.Y, mp.SpellType)
			} else {
				cs.game.CreateSpellHitEffect(monster.X, monster.Y, damageTypeStr, SpellParticleCount, SpellParticleSize)
			}
		} else if isRanged {
			cs.game.CreateArrowHitEffect(monster.X, monster.Y, arrowVelX, arrowVelY)
		}

		monster.HitPoints = 0
		monster.WasAttacked = true
		monster.HitTintFrames = cs.game.config.UI.DamageBlinkFrames
		cs.engageTurnBasedPackOnHit(monster)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, monster.ID)
		cs.awardExperienceAndGold(monster)

		attackerName := cs.game.party.Members[cs.game.selectedChar].Name
		cs.game.AddCombatMessage(fmt.Sprintf("%s's %s disintegrates %s!", attackerName, weaponName, monster.Name))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience and %d gold.",
			monster.Experience, monster.Gold))
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
		cs.game.CreateArrowHitEffect(monster.X, monster.Y, arrowVelX, arrowVelY)
	}

	// Calculate damage reduction based on attack type
	reducedDamage := damage
	if !isSpell {
		// Physical attacks: AC reduces damage, but ranged has 33% chance to pierce
		applyArmor := true
		if isRanged && rand.Intn(100) < 33 {
			applyArmor = false // Armor piercing shot!
		}

		if applyArmor {
			armorReduction := monster.ArmorClass / 2
			reducedDamage = damage - armorReduction
			if reducedDamage < 1 {
				reducedDamage = 1 // Minimum 1 damage
			}
		}
	}
	// Spells ignore AC completely - reducedDamage stays as original damage
	if !isSpell {
		if mult := cs.weaponBonusMultiplier(weaponDef, monster); mult != 1.0 {
			reducedDamage = int(math.Round(float64(reducedDamage) * mult))
			if reducedDamage < 1 {
				reducedDamage = 1
			}
		}
	}

	actualDamage := monster.TakeDamage(reducedDamage, damageType, cs.game.camera.X, cs.game.camera.Y)
	monster.HitTintFrames = cs.game.config.UI.DamageBlinkFrames
	cs.engageTurnBasedPackOnHit(monster)
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
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience and %d gold.",
			monster.Experience, monster.Gold))
	} else {
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hit %s for %d %s damage! (HP: %d/%d)",
			prefix, weaponName, monster.Name, actualDamage, damageTypeStr, monster.HitPoints, monster.MaxHitPoints))
	}
}

func (cs *CombatSystem) weaponBonusMultiplier(weaponDef *config.WeaponDefinitionConfig, monster *monsterPkg.Monster3D) float64 {
	if weaponDef == nil || monster == nil || len(weaponDef.BonusVs) == 0 {
		return 1.0
	}

	candidates := []string{monster.Name}
	if monsterPkg.MonsterConfig != nil {
		if key, ok := monsterPkg.MonsterConfig.GetMonsterKeyByName(monster.Name); ok {
			candidates = append(candidates, key)
		}
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
	// Distribute experience among all living party members
	experiencePerMember := monster.Experience / len(cs.game.party.Members)
	for _, member := range cs.game.party.Members {
		if member.HitPoints > 0 { // Only living members get experience
			member.Experience += experiencePerMember

			// Check for level up (simple level up system)
			cs.checkLevelUp(member)
		}
	}

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
		cs.game.addLootBag(monster.X, monster.Y, drops, monster.Gold, sizeMultiplier)
	}
}

// updateQuestProgress updates quest progress when a monster is killed
func (cs *CombatSystem) updateQuestProgress(monster *monsterPkg.Monster3D) {
	if cs.game.questManager == nil {
		return
	}

	// Convert monster name to lowercase key format (e.g., "Goblin" -> "goblin", "Dire Wolf" -> "dire_wolf")
	monsterType := strings.ToLower(strings.ReplaceAll(monster.Name, " ", "_"))

	completedQuests := cs.game.questManager.OnMonsterKilled(monsterType)

	// Notify player of quest completions
	for _, quest := range completedQuests {
		cs.game.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Open Quests (J) to claim reward.", quest.Definition.Name))
	}
}

// checkLevelUp checks if a character should level up and applies level up benefits
func (cs *CombatSystem) checkLevelUp(character *character.MMCharacter) {
	// Simple level progression: each level requires level * 100 experience
	// Loop to handle multiple level-ups from single experience gain
	for {
		requiredExp := character.Level * 100

		if character.Experience >= requiredExp {
			oldLevel := character.Level
			character.Level++
			character.Experience -= requiredExp // Subtract used experience

			// Grant 5 free stat points per level
			character.FreeStatPoints += 5

			// Recalculate derived stats (health and mana increase with level)
			character.CalculateDerivedStats(cs.game.config)

			// Restore full health and mana on level up
			character.HitPoints = character.MaxHitPoints
			character.SpellPoints = character.MaxSpellPoints

			message := fmt.Sprintf("%s reached level %d! (was level %d) [+5 stat points]",
				character.Name, character.Level, oldLevel)
			cs.game.AddCombatMessage(message)

			if choices := config.GetLevelUpChoices(character.GetClassKey(), character.Level); len(choices) > 0 {
				cs.game.queueLevelUpChoices(character, character.Level, choices)
			}
		} else {
			break // No more level-ups possible
		}
	}
}

// CalculateWeaponDamage calculates total weapon damage using weapon-specific bonus stat(s)
func (cs *CombatSystem) CalculateWeaponDamage(weapon items.Item, character *character.MMCharacter) (int, int, int) {
	baseDamage := weapon.Damage
	masteryBonus := cs.weaponMasteryBonus(weapon, character)
	if masteryBonus > 0 {
		baseDamage += masteryBonus
	}

	// Get effective stats including any stat bonuses (Bless, Day of Gods, etc.)
	might, intellect, _, _, accuracy, _, _ := character.GetEffectiveStats(cs.game.statBonus)

	// Get the appropriate stat bonus based on weapon's primary bonus stat
	var primaryStatBonus int
	switch weapon.BonusStat {
	case "Might":
		primaryStatBonus = might / 3
	case "Accuracy":
		primaryStatBonus = accuracy / 3
	case "Intellect":
		primaryStatBonus = intellect / 3
	default:
		// Fallback to Might for weapons without bonus stat specified
		primaryStatBonus = might / 3
	}

	// Add secondary stat bonus if weapon has dual scaling
	var secondaryStatBonus int
	if weapon.BonusStatSecondary != "" {
		switch weapon.BonusStatSecondary {
		case "Might":
			secondaryStatBonus = might / 4 // Secondary stats give less bonus
		case "Accuracy":
			secondaryStatBonus = accuracy / 4
		case "Intellect":
			secondaryStatBonus = intellect / 4
		}
	}

	totalStatBonus := primaryStatBonus + secondaryStatBonus
	totalDamage := baseDamage + totalStatBonus
	return baseDamage, totalStatBonus, totalDamage
}

func (cs *CombatSystem) weaponMasteryBonus(weapon items.Item, character *character.MMCharacter) int {
	if character == nil {
		return 0
	}
	weaponDef := cs.getWeaponConfig(weapon.Name)
	if weaponDef == nil {
		return 0
	}
	skillType, ok := weaponSkillFromCategory(weaponDef.Category)
	if !ok {
		return 0
	}
	if skill, exists := character.Skills[skillType]; exists {
		return int(skill.Mastery) * 2
	}
	return 0
}

func weaponSkillFromCategory(category string) (character.SkillType, bool) {
	switch strings.ToLower(category) {
	case "sword":
		return character.SkillSword, true
	case "dagger":
		return character.SkillDagger, true
	case "throwing":
		return character.SkillDagger, true
	case "axe":
		return character.SkillAxe, true
	case "spear":
		return character.SkillSpear, true
	case "bow":
		return character.SkillBow, true
	case "mace":
		return character.SkillMace, true
	case "staff":
		return character.SkillStaff, true
	default:
		return 0, false
	}
}

// CalculateElementalSpellDamage calculates damage for fire/air/water/earth spells
func (cs *CombatSystem) CalculateElementalSpellDamage(spellPoints int, char *character.MMCharacter) (int, int, int) {
	baseDamage := spellPoints * 3
	intellectBonus := char.GetEffectiveIntellect(cs.game.statBonus) / 2
	totalDamage := baseDamage + intellectBonus
	return baseDamage, intellectBonus, totalDamage
}

// spellMasteryBonus returns +5 per mastery level for the spell's school.
func (cs *CombatSystem) spellMasteryBonus(char *character.MMCharacter, spellID spells.SpellID) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil || def.School == "" {
		return 0
	}
	school := character.MagicSchoolIDToLegacy(character.MagicSchoolID(def.School))
	if skill, exists := char.MagicSchools[school]; exists {
		return int(skill.Mastery) * 5
	}
	return 0
}

// recordSpellCast increments cast counters and auto-advances mastery every 30 casts per school.
func (cs *CombatSystem) recordSpellCast(char *character.MMCharacter, spellID spells.SpellID) {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil || def.School == "" {
		return
	}
	school := character.MagicSchoolIDToLegacy(character.MagicSchoolID(def.School))
	skill, exists := char.MagicSchools[school]
	if !exists {
		return
	}
	skill.CastCount++
	desired := character.SkillMastery(skill.CastCount / 30)
	if desired > character.MasteryGrandMaster {
		desired = character.MasteryGrandMaster
	}
	if desired > skill.Mastery {
		skill.Mastery = desired
	}
}

// CalculateAccuracyBonus calculates accuracy bonus from character stats
func (cs *CombatSystem) CalculateAccuracyBonus(character *character.MMCharacter) int {
	// Accuracy bonus is half of character's Accuracy stat
	return character.Accuracy / 2
}

// CalculateCriticalChance calculates critical hit bonus from character stats
func (cs *CombatSystem) CalculateCriticalChance(char *character.MMCharacter) int {
	// Use effective Luck so Bless/stat bonuses influence crit chance
	return char.GetEffectiveLuck(cs.game.statBonus) / 4
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
// chance = effective Luck / 5, clamped to [0,100].
func (cs *CombatSystem) RollPerfectDodge(chr *character.MMCharacter) (bool, int) {
	// Use effective stats so Bless and equipment affect dodge
	chance := chr.GetEffectiveLuck(cs.game.statBonus) / 5
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

	// Damage reduction (same formula as tooltip)
	damageReduction := totalArmorClass / 2

	// Apply damage reduction
	finalDamage := damage - damageReduction
	if finalDamage < 1 {
		finalDamage = 1 // Minimum 1 damage (armor can't completely negate damage)
	}

	return finalDamage
}

func (cs *CombatSystem) armorMasteryBonus(char *character.MMCharacter, armor items.Item) int {
	if char == nil {
		return 0
	}
	skillType, ok := armorSkillFromCategory(armor.ArmorCategory)
	if !ok {
		return 0
	}
	if skill, exists := char.Skills[skillType]; exists {
		return int(skill.Mastery)
	}
	return 0
}

func armorSkillFromCategory(category string) (character.SkillType, bool) {
	switch strings.ToLower(category) {
	case "leather":
		return character.SkillLeather, true
	case "chain":
		return character.SkillChain, true
	case "plate":
		return character.SkillPlate, true
	case "shield":
		return character.SkillShield, true
	default:
		return 0, false
	}
}

// checkMonsterLootDrop handles loot drops when monsters are killed
func (cs *CombatSystem) checkMonsterLootDrop(monster *monsterPkg.Monster3D) []items.Item {
	// Use YAML-configured loot tables keyed by monster
	if monsterPkg.MonsterConfig == nil {
		return nil
	}
	monsterKey, ok := monsterPkg.MonsterConfig.GetMonsterKeyByName(monster.Name)
	if !ok {
		return nil
	}
	entries := config.GetLootTable(monsterKey)
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

// findHighestEnduranceTarget finds the party member with the highest endurance who has HP > 0
func (cs *CombatSystem) findHighestEnduranceTarget() *character.MMCharacter {
	var bestTarget *character.MMCharacter
	highestEndurance := -1

	for _, member := range cs.game.party.Members {
		// Only consider living members
		if member.HitPoints <= 0 {
			continue
		}

		// Get effective endurance (includes equipment bonuses)
		effectiveEndurance := member.GetEffectiveEndurance(cs.game.statBonus)

		// Check if this member has higher endurance than current best
		if effectiveEndurance > highestEndurance {
			highestEndurance = effectiveEndurance
			bestTarget = member
		}
	}

	// Fallback to selected character if no living members found (shouldn't happen)
	if bestTarget == nil {
		bestTarget = cs.game.party.Members[cs.game.selectedChar]
	}

	return bestTarget
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

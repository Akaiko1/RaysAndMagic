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

// CastEquippedSpell performs a magic attack using equipped spell (unified F key casting)
func (cs *CombatSystem) CastEquippedSpell() {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Check if character has a spell equipped
	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return // No spell equipped
	}

	// Check spell points (use spell cost for spells)
	var spellCost int
	if spell.Type == items.ItemBattleSpell || spell.Type == items.ItemUtilitySpell {
		spellCost = spell.SpellCost
	} else {
		// This shouldn't happen - SlotSpell should only contain spells
		return
	}

	if caster.SpellPoints < spellCost {
		return
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
		return
	}

	if spellDef.IsUtility {
		// Handle utility spells (like Torch Light)
		result, err := castingSystem.ApplyUtilitySpell(spellID, caster.Intellect)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return
		}

		// Apply the utility spell effects to the game
		if result.Success {
			// Add combat message
			cs.game.AddCombatMessage(result.Message)

			// Apply healing effects for heal spells
			if result.HealAmount > 0 {
				if result.TargetSelf || string(spellID) == "heal" {
					// Heal self
					caster.HitPoints += result.HealAmount
					if caster.HitPoints > caster.MaxHitPoints {
						caster.HitPoints = caster.MaxHitPoints
					}
				} else if string(spellID) == "heal_other" {
					// For F key heal other, just heal self since there's no target selection
					// Use H key with mouse for proper targeting
					caster.HitPoints += result.HealAmount
					if caster.HitPoints > caster.MaxHitPoints {
						caster.HitPoints = caster.MaxHitPoints
					}
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
		}
		return
	}

	// For projectile spells, create a projectile using effective intellect (includes Bless bonus)
	_, effectiveIntellect, _, _, _, _, _ := caster.GetEffectiveStats(cs.game.statBonus)
	projectile, err := castingSystem.CreateProjectile(spellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return
	}

	// Get spell-specific config dynamically
	spellConfig, err := cs.game.config.GetSpellConfig(string(spellID))
	if err != nil {
		cs.game.AddCombatMessage("Spell config error: " + err.Error())
		return
	}

	// Create magic projectile with proper type information
	magicProjectile := MagicProjectile{
		ID:        cs.game.GenerateProjectileID(string(spellID)),
		X:         projectile.X,
		Y:         projectile.Y,
		VelX:      projectile.VelX,
		VelY:      projectile.VelY,
		Damage:    projectile.Damage,
		LifeTime:  projectile.LifeTime,
		Active:    projectile.Active,
		SpellType: string(spellID),
		Size:      projectile.Size,
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

	// Register with collision system
	collisionSize := float64(spellConfig.CollisionSize) // Use spell-specific collision size
	projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(projectileEntity)

	// Note: Combat message for spell casting is now only generated when projectile hits a target
	// This prevents spam of "X casts Y!" messages for attacks that miss
}

// EquipmentHeal casts heal using equipped spell (special targeting for heal spells)
func (cs *CombatSystem) EquipmentHeal() {
	caster := cs.game.party.Members[cs.game.selectedChar]

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
	} else {
		cs.game.AddCombatMessage(result.Message)
	}
}

// CastEquippedHealOnTarget casts heal using equipped spell on specified party member
func (cs *CombatSystem) CastEquippedHealOnTarget(targetIndex int) {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Check if character has a heal spell equipped
	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return // No spell equipped
	}

	// Allow both heal-type spells for targeting
	if spell.SpellEffect != items.SpellEffectHealSelf && spell.SpellEffect != items.SpellEffectHealOther {
		return // Not a heal spell
	}

	// Check spell points (use spell cost for utility spells)
	spellCost := spell.SpellCost
	if caster.SpellPoints < spellCost {
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

	// Cast heal on target
	caster.SpellPoints -= spellCost
	// Calculate heal amount using centralized function
	_, _, healAmount := cs.CalculateEquippedHealAmount(spellCost, caster)
	target.HitPoints += healAmount
	if target.HitPoints > target.MaxHitPoints {
		target.HitPoints = target.MaxHitPoints
	}

	// Print feedback message
	if targetIndex == cs.game.selectedChar {
		message := fmt.Sprintf("%s healed themselves for %d HP with %s!", caster.Name, healAmount, spell.Name)
		cs.game.AddCombatMessage(message)
	} else {
		message := fmt.Sprintf("%s healed %s for %d HP with %s!", caster.Name, target.Name, healAmount, spell.Name)
		cs.game.AddCombatMessage(message)
	}
}

// EquipmentMeleeAttack performs a melee attack using equipped weapon
func (cs *CombatSystem) EquipmentMeleeAttack() {
	attacker := cs.game.party.Members[cs.game.selectedChar]

	// Check if character has a weapon equipped
	weapon, hasWeapon := attacker.Equipment[items.SlotMainHand]
	if !hasWeapon {
		return // No weapon equipped
	}

	// Calculate damage using centralized function
	_, _, totalDamage := cs.CalculateWeaponDamage(weapon, attacker)

	// Check if weapon is a bow (range > 3 tiles indicates ranged weapon)
	if weapon.Range > 3 {
		// Create projectile arrow for ranged weapons
		cs.createArrowAttack(totalDamage)
	} else {
		// Create instant melee attack for close-range weapons
		cs.createMeleeAttack(weapon, totalDamage)
	}
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

	if exists && weaponDef.Physics != nil {
		// Use weapon-specific physics properties
		arrowSpeed = weaponDef.Physics.Speed
		arrowLifetime = weaponDef.Physics.Lifetime
		collisionSize = float64(weaponDef.Physics.CollisionSize)
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

	arrow := Arrow{
		ID:         cs.game.GenerateProjectileID("arrow"),
		X:          cs.game.camera.X,
		Y:          cs.game.camera.Y,
		VelX:       math.Cos(cs.game.camera.Angle) * arrowSpeed,
		VelY:       math.Sin(cs.game.camera.Angle) * arrowSpeed,
		Damage:     damage,
		LifeTime:   arrowLifetime,
		Active:     true,
		BowKey:     bowKey,
		DamageType: damageType,
	}

	cs.game.arrows = append(cs.game.arrows, arrow)

	// Register arrow with collision system using weapon-specific collision size
	arrowEntity := collision.NewEntity(arrow.ID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(arrowEntity)
}

// createMeleeAttack creates an instant melee attack with proper arc-based hit detection
func (cs *CombatSystem) createMeleeAttack(weapon items.Item, totalDamage int) {
	// Get weapon key and definition from YAML
	weaponKey := items.GetWeaponKeyByName(weapon.Name)
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		panic("weapon '" + weapon.Name + "' not found in weapons.yaml - system misconfigured")
	}

	// Check if weapon has melee configuration
	if weaponDef.Melee == nil {
		panic("weapon '" + weapon.Name + "' has no melee configuration in weapons.yaml")
	}

	meleeConfig := weaponDef.Melee
	graphicsConfig := weaponDef.Graphics

	// Create visual slash effect
	if graphicsConfig != nil {
		slashEffect := SlashEffect{
			ID:             cs.game.GenerateProjectileID("slash"),
			X:              cs.game.camera.X,
			Y:              cs.game.camera.Y,
			Angle:          cs.game.camera.Angle,
			Width:          graphicsConfig.SlashWidth,
			Length:         graphicsConfig.SlashLength,
			Color:          graphicsConfig.SlashColor,
			AnimationFrame: 0,
			MaxFrames:      meleeConfig.AnimationFrames,
			Active:         true,
		}
		cs.game.slashEffects = append(cs.game.slashEffects, slashEffect)
	}

	// Perform instant hit detection in arc
	cs.performMeleeHitDetection(weapon, totalDamage, meleeConfig)
}

// performMeleeHitDetection checks for monsters in the weapon's swing arc and applies damage
func (cs *CombatSystem) performMeleeHitDetection(weapon items.Item, damage int, meleeConfig *config.MeleeAttackConfig) {
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
		distanceToCenter := math.Sqrt(dx*dx + dy*dy)

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
			cs.ApplyDamageToMonster(monster, damage, weapon.Name)
		}
	}
}

// ApplyDamageToMonster applies damage to a monster and handles combat messages
func (cs *CombatSystem) ApplyDamageToMonster(monster *monsterPkg.Monster3D, damage int, weaponName string) {
	// Apply damage with resistances and distance-aware AI response
	finalDamage := monster.TakeDamage(damage, monsterPkg.DamagePhysical, cs.game.camera.X, cs.game.camera.Y)

	// Add combat message
	if monster.IsAlive() {
		cs.game.AddCombatMessage(fmt.Sprintf("%s hits %s for %d damage!",
			cs.game.party.Members[cs.game.selectedChar].Name, monster.Name, finalDamage))
	} else {
		cs.game.AddCombatMessage(fmt.Sprintf("%s kills %s!",
			cs.game.party.Members[cs.game.selectedChar].Name, monster.Name))

		// Award experience and gold using centralized function
		cs.awardExperienceAndGold(monster)

		// Add experience/gold award message
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience and %d gold.",
			monster.Experience, monster.Gold))
	}
}

// CastSelectedSpell casts the currently selected spell from the spellbook
func (cs *CombatSystem) CastSelectedSpell() {
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
	selectedSpellDef, err := spells.GetSpellDefinitionByID(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
		return
	}

	// Check spell points
	if currentChar.SpellPoints < selectedSpellDef.SpellPointsCost {
		return
	}

	// Cast the spell
	currentChar.SpellPoints -= selectedSpellDef.SpellPointsCost

	// Dynamic spell casting - no more hardcoded switches!
	castingSystem := spells.NewCastingSystem(cs.game.config)

	if selectedSpellDef.IsProjectile {
		// Handle projectile spells dynamically using effective intellect (includes Bless bonus)
		_, effectiveIntellect, _, _, _, _, _ := currentChar.GetEffectiveStats(cs.game.statBonus)
		projectile, err := castingSystem.CreateProjectile(selectedSpellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return
		}

		// Create magic projectile using unified system
		magicProjectile := MagicProjectile{
			ID:       cs.game.GenerateProjectileID(string(selectedSpellID)),
			X:        projectile.X,
			Y:        projectile.Y,
			VelX:     projectile.VelX,
			VelY:     projectile.VelY,
			Damage:   projectile.Damage,
			LifeTime: projectile.LifeTime,
			Active:   projectile.Active,
		}
		cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

		// Register projectile with collision system using dynamic config
		spellConfig, err := cs.game.config.GetSpellConfig(string(selectedSpellID))
		if err != nil {
			cs.game.AddCombatMessage("Spell config error: " + err.Error())
			return
		}
		projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, float64(spellConfig.CollisionSize), float64(spellConfig.CollisionSize), collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(projectileEntity)

		// Add message based on spell definition
		cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", selectedSpellDef.Name))

	} else if selectedSpellDef.IsUtility {
		// Handle utility spells using centralized system
		result, err := castingSystem.ApplyUtilitySpell(selectedSpellID, currentChar.Personality)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return
		}

		if result.Success {
			// Apply skill level multiplier to duration effects
			duration := result.Duration
			if magicSkill, exists := currentChar.MagicSchools[selectedSchool]; exists && duration > 0 {
				skillMultiplier := 1.0 + (float64(magicSkill.Level) * 0.1)
				duration = int(float64(duration) * skillMultiplier)
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
					cs.game.torchLightDuration = duration
					cs.game.torchLightRadius = 4.0
				case "wizard_eye":
					cs.game.wizardEyeActive = true
					cs.game.wizardEyeDuration = duration
				}
			}

			// Apply movement effects
			if result.WaterWalk {
				cs.game.walkOnWaterActive = true
				cs.game.walkOnWaterDuration = duration
			}

			// Apply stat bonus effects
			if result.StatBonus > 0 {
				cs.applyBlessEffect(duration, result.StatBonus)
			}

			// Apply awakening effects
			if result.Awaken {
				// TODO: Remove sleep/paralysis conditions from party
			}
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

		dx := monster.X - cs.game.camera.X
		dy := monster.Y - cs.game.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		// If monster is in attacking state and within attack radius, deal damage
		if monster.State == monsterPkg.StateAttacking && distance < monster.AttackRadius {
			// Only attack once per attacking state (no separate cooldown needed)
			if monster.StateTimer == 1 { // Attack on first frame of attacking state
				// Monster attacks character with highest endurance (and HP > 0)
				currentChar := cs.findHighestEnduranceTarget()
				damage := monster.GetAttackDamage()

				// Apply armor damage reduction
				finalDamage := cs.ApplyArmorDamageReduction(damage, currentChar)
				currentChar.HitPoints -= finalDamage
				if currentChar.HitPoints < 0 {
					currentChar.HitPoints = 0
				}

				// Trigger damage blink effect for the character that was hit
				targetIndex := cs.findCharacterIndex(currentChar)
				cs.game.TriggerDamageBlink(targetIndex)

				// Push monster back slightly to prevent spam
				pushBack := cs.game.config.MonsterAI.PushbackDistance
				monster.X += (monster.X - cs.game.camera.X) / distance * pushBack
				monster.Y += (monster.Y - cs.game.camera.Y) / distance * pushBack
			}
		}
	}
}

// CheckProjectileMonsterCollisions checks for collisions between projectiles and monsters using bounding boxes
func (cs *CombatSystem) CheckProjectileMonsterCollisions() {
	// Get all collisions from the collision system
	collisions := cs.game.collisionSystem.GetCollisions()

	for _, collisionPair := range collisions {
		// Check if this is a projectile-monster collision
		var projectileEntity, monsterEntity *collision.Entity

		if collisionPair.Entity1.CollisionType == collision.CollisionTypeProjectile && collisionPair.Entity2.CollisionType == collision.CollisionTypeMonster {
			projectileEntity = collisionPair.Entity1
			monsterEntity = collisionPair.Entity2
		} else if collisionPair.Entity2.CollisionType == collision.CollisionTypeProjectile && collisionPair.Entity1.CollisionType == collision.CollisionTypeMonster {
			projectileEntity = collisionPair.Entity2
			monsterEntity = collisionPair.Entity1
		} else {
			continue // Not a projectile-monster collision
		}

		// Handle the collision based on projectile type
		cs.handleProjectileMonsterCollision(projectileEntity, monsterEntity)
	}
}

// handleProjectileMonsterCollision handles collision between a specific projectile and monster
func (cs *CombatSystem) handleProjectileMonsterCollision(projectileEntity, monsterEntity *collision.Entity) {
	// Find the actual projectile and monster objects
	var projectile interface{}
	var projectileType string

	// Determine projectile type and find projectile by unique ID
	if strings.HasPrefix(projectileEntity.ID, "arrow_") {
		// Find arrow by unique ID
		for i := range cs.game.arrows {
			if cs.game.arrows[i].ID == projectileEntity.ID && cs.game.arrows[i].Active && cs.game.arrows[i].LifeTime > 0 {
				projectile = &cs.game.arrows[i]
				projectileType = "arrow"
				break
			}
		}
	} else if strings.HasPrefix(projectileEntity.ID, "melee_") {
		// Find melee attack by unique ID
		for i := range cs.game.meleeAttacks {
			if cs.game.meleeAttacks[i].ID == projectileEntity.ID && cs.game.meleeAttacks[i].Active && cs.game.meleeAttacks[i].LifeTime > 0 {
				projectile = &cs.game.meleeAttacks[i]
				projectileType = "melee"
				break
			}
		}
	} else {
		// Find magic projectile by unique ID (any spell type)
		for i := range cs.game.magicProjectiles {
			if cs.game.magicProjectiles[i].ID == projectileEntity.ID && cs.game.magicProjectiles[i].Active && cs.game.magicProjectiles[i].LifeTime > 0 {
				projectile = &cs.game.magicProjectiles[i]
				projectileType = "magic_projectile"
				break
			}
		}
	}

	if projectile == nil {
		return
	}

	// Find the monster by ID from entity ID
	if len(monsterEntity.ID) <= 8 || monsterEntity.ID[:8] != "monster_" {
		return
	}

	// Find the monster with the matching ID
	var monster *monsterPkg.Monster3D
	for _, m := range cs.game.world.Monsters {
		if m.ID == monsterEntity.ID {
			monster = m
			break
		}
	}

	if monster == nil || !monster.IsAlive() {
		return
	}

	// Apply perspective-scaled collision detection (same logic as visual collision boxes)
	// Get projectile and monster positions
	var projectileX, projectileY float64
	switch projectileType {
	case "magic_projectile":
		magicProj := projectile.(*MagicProjectile)
		projectileX, projectileY = magicProj.X, magicProj.Y
	case "melee":
		meleeAttack := projectile.(*MeleeAttack)
		projectileX, projectileY = meleeAttack.X, meleeAttack.Y
	case "arrow":
		arrow := projectile.(*Arrow)
		projectileX, projectileY = arrow.X, arrow.Y
	}

	// Calculate distances from camera for perspective scaling
	projDx := projectileX - cs.game.camera.X
	projDy := projectileY - cs.game.camera.Y
	projectileDistance := math.Sqrt(projDx*projDx + projDy*projDy)

	monsterDx := monster.X - cs.game.camera.X
	monsterDy := monster.Y - cs.game.camera.Y
	monsterDistance := math.Sqrt(monsterDx*monsterDx + monsterDy*monsterDy)

	// Get original collision box sizes
	projEntity := cs.game.collisionSystem.GetEntityByID(projectileEntity.ID)
	monsterEntityCollision := cs.game.collisionSystem.GetEntityByID(monsterEntity.ID)
	if projEntity == nil || monsterEntityCollision == nil {
		return
	}

	// Apply the EXACT same perspective scaling used in visual rendering

	// For projectiles: use the same formula as renderer (baseSize / distance * tileSize)
	var baseSize float64
	switch projectileType {
	case "magic_projectile":
		magicProj := projectile.(*MagicProjectile)
		spellGraphicsConfig, err := cs.game.config.GetSpellGraphicsConfig(magicProj.SpellType)
		if err != nil {
			return // Skip if no graphics config
		}
		baseSize = float64(spellGraphicsConfig.BaseSize)
	case "melee":
		meleeAttack := projectile.(*MeleeAttack)
		// Get weapon config for base size from YAML - NO FALLBACKS
		weaponKey := items.GetWeaponKeyByName(meleeAttack.WeaponName)
		weaponDef, exists := config.GetWeaponDefinition(weaponKey)
		if !exists {
			panic("weapon '" + meleeAttack.WeaponName + "' not found in weapons.yaml - system misconfigured")
		}
		if weaponDef.Graphics == nil {
			panic("weapon '" + meleeAttack.WeaponName + "' has no graphics configuration in weapons.yaml")
		}
		baseSize = float64(weaponDef.Graphics.BaseSize)
	case "arrow":
		// Use the bow that fired this arrow
		arrow := projectile.(*Arrow)
		weaponDef, exists := config.GetWeaponDefinition(arrow.BowKey)
		if !exists {
			panic("bow '" + arrow.BowKey + "' not found in weapons.yaml - system misconfigured")
		}
		if weaponDef.Graphics == nil {
			panic("bow '" + arrow.BowKey + "' has no graphics configuration in weapons.yaml")
		}
		baseSize = float64(weaponDef.Graphics.BaseSize)
	}

	// Calculate projectile visual size (same as renderer) with size limits
	projVisualSize := baseSize / projectileDistance * float64(cs.game.config.GetTileSize())

	// Apply the same size limits as the renderer
	var maxSize, minSize int
	switch projectileType {
	case "magic_projectile":
		magicProj := projectile.(*MagicProjectile)
		spellGraphicsConfig, err := cs.game.config.GetSpellGraphicsConfig(magicProj.SpellType)
		if err != nil {
			return // Skip if no graphics config
		}
		maxSize, minSize = spellGraphicsConfig.MaxSize, spellGraphicsConfig.MinSize
	case "melee":
		meleeAttack := projectile.(*MeleeAttack)
		// Get weapon graphics config from YAML - NO FALLBACKS
		weaponKey := items.GetWeaponKeyByName(meleeAttack.WeaponName)
		weaponDef, exists := config.GetWeaponDefinition(weaponKey)
		if !exists {
			panic("weapon '" + meleeAttack.WeaponName + "' not found in weapons.yaml - system misconfigured")
		}
		if weaponDef.Graphics == nil {
			panic("weapon '" + meleeAttack.WeaponName + "' has no graphics configuration in weapons.yaml")
		}
		maxSize, minSize = weaponDef.Graphics.MaxSize, weaponDef.Graphics.MinSize
	case "arrow":
		// Use the bow that fired this arrow
		arrow := projectile.(*Arrow)
		weaponDef, exists := config.GetWeaponDefinition(arrow.BowKey)
		if !exists {
			panic("bow '" + arrow.BowKey + "' not found in weapons.yaml - system misconfigured")
		}
		if weaponDef.Graphics == nil {
			panic("bow '" + arrow.BowKey + "' has no graphics configuration in weapons.yaml")
		}
		maxSize, minSize = weaponDef.Graphics.MaxSize, weaponDef.Graphics.MinSize
	}

	if projVisualSize > float64(maxSize) {
		projVisualSize = float64(maxSize)
	}
	if projVisualSize < float64(minSize) {
		projVisualSize = float64(minSize)
	}

	projScale := projVisualSize / baseSize // Scale factor relative to base size

	scaledProjW := projEntity.BoundingBox.Width * projScale
	scaledProjH := projEntity.BoundingBox.Height * projScale

	// For monsters: calculate sprite size the EXACT same way renderer does
	monsterMultiplier := float64(cs.game.config.Graphics.Monster.SizeDistanceMultiplier)
	monsterVisualSize := float64(cs.game.config.GetTileSize()) / monsterDistance * monsterMultiplier

	// Apply the same size limits as renderer
	maxMonsterSize := float64(cs.game.config.Graphics.Monster.MaxSpriteSize)
	minMonsterSize := float64(cs.game.config.Graphics.Monster.MinSpriteSize)
	if monsterVisualSize > maxMonsterSize {
		monsterVisualSize = maxMonsterSize
	}
	if monsterVisualSize < minMonsterSize {
		monsterVisualSize = minMonsterSize
	}

	// Calculate scale factor using the configurable base size (multiplier, not hardcoded 64)
	monsterScale := monsterVisualSize / monsterMultiplier

	scaledMonsterW := monsterEntityCollision.BoundingBox.Width * monsterScale
	scaledMonsterH := monsterEntityCollision.BoundingBox.Height * monsterScale

	// Create temporary scaled bounding boxes for perspective collision check
	scaledProjBox := collision.NewBoundingBox(projectileX, projectileY, scaledProjW, scaledProjH)
	scaledMonsterBox := collision.NewBoundingBox(monster.X, monster.Y, scaledMonsterW, scaledMonsterH)

	// Check collision using perspective-scaled boxes (same as visual collision boxes)
	if !scaledProjBox.Intersects(scaledMonsterBox) {
		return // No collision with perspective-scaled boxes
	}

	// Handle the collision based on projectile type
	switch projectileType {
	case "magic_projectile":
		magicProjectile := projectile.(*MagicProjectile)
		if !magicProjectile.Active || magicProjectile.LifeTime <= 0 {
			return
		}

		// Determine correct damage type from spell school using the original spell ID
		spellID := spells.SpellID(magicProjectile.SpellType)
		spellDef, _ := spells.GetSpellDefinitionByID(spellID)
		damageTypeStr := spellDef.School
		spellName := spellDef.Name
		// fmt.Println("Spell damage type:", damageTypeStr)
		damageType := monsterPkg.DamageFire // Default fallback, TODO: Add debug fallback damage type
		if monsterPkg.MonsterConfig != nil {
			if convertedType, err := monsterPkg.MonsterConfig.ConvertDamageType(damageTypeStr); err == nil {
				damageType = convertedType
			}
		}
		actualDamage := monster.TakeDamage(magicProjectile.Damage, damageType, cs.game.camera.X, cs.game.camera.Y)
		magicProjectile.Active = false

		// Unregister the magic projectile from collision system
		cs.game.collisionSystem.UnregisterEntity(projectileEntity.ID)

		// Check if monster died and give rewards
		if !monster.IsAlive() {
			cs.awardExperienceAndGold(monster)
			message := fmt.Sprintf("%s killed %s! Awarded %d experience and %d gold.",
				spellName, monster.Name, monster.Experience, monster.Gold)
			cs.game.AddCombatMessage(message)
		} else {
			// Use correct damage type in message
			message := fmt.Sprintf("%s hit %s for %d %s damage! (HP: %d/%d)",
				spellName, monster.Name, actualDamage, damageTypeStr, monster.HitPoints, monster.MaxHitPoints)
			cs.game.AddCombatMessage(message)
		}

	case "melee":
		attack := projectile.(*MeleeAttack)
		if !attack.Active || attack.LifeTime <= 0 {
			return
		}

		// Monster takes physical damage with distance-aware AI response
		actualDamage := monster.TakeDamage(attack.Damage, monsterPkg.DamagePhysical, cs.game.camera.X, cs.game.camera.Y)
		attack.Active = false

		// Unregister the sword attack from collision system
		cs.game.collisionSystem.UnregisterEntity(projectileEntity.ID)

		// Check if monster died and give rewards
		if !monster.IsAlive() {
			cs.awardExperienceAndGold(monster)
			message := fmt.Sprintf("%s killed %s! Awarded %d experience and %d gold.",
				attack.WeaponName, monster.Name, monster.Experience, monster.Gold)
			cs.game.AddCombatMessage(message)
		} else {
			message := fmt.Sprintf("%s hit %s for %d physical damage! (HP: %d/%d)",
				attack.WeaponName, monster.Name, actualDamage, monster.HitPoints, monster.MaxHitPoints)
			cs.game.AddCombatMessage(message)
		}

	case "arrow":
		arrow := projectile.(*Arrow)
		if !arrow.Active || arrow.LifeTime <= 0 {
			return
		}

		// Convert string damage type to monster damage type enum
		damageType := monsterPkg.DamagePhysical // Default fallback
		if monsterPkg.MonsterConfig != nil {
			if convertedType, err := monsterPkg.MonsterConfig.ConvertDamageType(arrow.DamageType); err == nil {
				damageType = convertedType
			}
		}

		// Monster takes damage with appropriate damage type and distance-aware AI response
		actualDamage := monster.TakeDamage(arrow.Damage, damageType, cs.game.camera.X, cs.game.camera.Y)
		arrow.Active = false

		// Unregister the arrow from collision system
		cs.game.collisionSystem.UnregisterEntity(projectileEntity.ID)

		// Check if monster died and give rewards
		if !monster.IsAlive() {
			cs.awardExperienceAndGold(monster)
			message := fmt.Sprintf("Arrow killed %s! Awarded %d experience and %d gold.",
				monster.Name, monster.Experience, monster.Gold)
			cs.game.AddCombatMessage(message)
		} else {
			message := fmt.Sprintf("Arrow hit %s for %d physical damage! (HP: %d/%d)",
				monster.Name, actualDamage, monster.HitPoints, monster.MaxHitPoints)
			cs.game.AddCombatMessage(message)
		}
	}
}

// awardExperienceAndGold gives experience and gold to the party when a monster is killed
func (cs *CombatSystem) awardExperienceAndGold(monster *monsterPkg.Monster3D) {
	// Award gold to party
	cs.game.party.Gold += monster.Gold

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
	cs.checkMonsterLootDrop(monster)
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
		} else {
			break // No more level-ups possible
		}
	}
}

// CalculateWeaponDamage calculates total weapon damage using weapon-specific bonus stat(s)
func (cs *CombatSystem) CalculateWeaponDamage(weapon items.Item, character *character.MMCharacter) (int, int, int) {
	baseDamage := weapon.Damage

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

// CalculateElementalSpellDamage calculates damage for fire/air/water/earth spells
func (cs *CombatSystem) CalculateElementalSpellDamage(spellPoints int, character *character.MMCharacter) (int, int, int) {
	baseDamage := spellPoints * 3
	// Get effective intellect including any stat bonuses
	_, intellect, _, _, _, _, _ := character.GetEffectiveStats(cs.game.statBonus)
	intellectBonus := intellect / 2
	totalDamage := baseDamage + intellectBonus
	return baseDamage, intellectBonus, totalDamage
}

// CalculateHealingSpellAmount calculates healing for body magic spells from spellbook
func (cs *CombatSystem) CalculateHealingSpellAmount(spellPoints int, character *character.MMCharacter) (int, int, int) {
	baseHealing := spellPoints * 5
	// Get effective personality including any stat bonuses
	_, _, personality, _, _, _, _ := character.GetEffectiveStats(cs.game.statBonus)
	personalityBonus := personality / 2
	totalHealing := baseHealing + personalityBonus
	return baseHealing, personalityBonus, totalHealing
}

// CalculateEquippedHealAmount calculates healing for equipped heal spells (targeted healing)
func (cs *CombatSystem) CalculateEquippedHealAmount(spellCost int, character *character.MMCharacter) (int, int, int) {
	baseHealing := spellCost * 3
	// Get effective personality including any stat bonuses
	_, _, personality, _, _, _, _ := character.GetEffectiveStats(cs.game.statBonus)
	personalityBonus := personality / 2
	totalHealing := baseHealing + personalityBonus
	return baseHealing, personalityBonus, totalHealing
}

// CalculateAccuracyBonus calculates accuracy bonus from character stats
func (cs *CombatSystem) CalculateAccuracyBonus(character *character.MMCharacter) int {
	// Accuracy bonus is half of character's Accuracy stat
	return character.Accuracy / 2
}

// CalculateCriticalChance calculates critical hit bonus from character stats
func (cs *CombatSystem) CalculateCriticalChance(character *character.MMCharacter) int {
	// Critical chance bonus based on Luck stat
	return character.Luck / 4
}

// applyBlessEffect applies the Bless spell effect consistently across all casting methods
func (cs *CombatSystem) applyBlessEffect(duration, statBonus int) {
	cs.game.blessActive = true
	cs.game.blessDuration = duration
	cs.game.statBonus += statBonus // ADD to total stat bonus
}

// ApplyArmorDamageReduction calculates final damage after armor reduction
func (cs *CombatSystem) ApplyArmorDamageReduction(damage int, character *character.MMCharacter) int {
	// Get effective endurance (includes equipment bonuses like Leather Armor)
	_, _, _, effectiveEndurance, _, _, _ := character.GetEffectiveStats(cs.game.statBonus)

	// Calculate armor class (same formula as tooltip)
	baseArmor := 0
	// Check if wearing armor
	if armor, hasArmor := character.Equipment[items.SlotArmor]; hasArmor {
		if armor.Name == "Leather Armor" {
			baseArmor = 2
		}
		// Future: Add more armor types
	}

	enduranceBonus := effectiveEndurance / 5
	totalArmorClass := baseArmor + enduranceBonus

	// Damage reduction (same formula as tooltip)
	damageReduction := totalArmorClass / 2

	// Apply damage reduction
	finalDamage := damage - damageReduction
	if finalDamage < 1 {
		finalDamage = 1 // Minimum 1 damage (armor can't completely negate damage)
	}

	return finalDamage
}

// checkMonsterLootDrop handles loot drops when monsters are killed
func (cs *CombatSystem) checkMonsterLootDrop(monster *monsterPkg.Monster3D) {
	// Check if this is a Pixie (referred to as "Sprite" by user)
	if monster.Name == "Pixie" {
		// 5% chance to drop Magic Ring
		if rand.Float64() < 0.05 {
			// Create Magic Ring item
			magicRing := items.Item{
				Name:        "Magic Ring",
				Type:        items.ItemAccessory,
				Description: "A magical ring that enhances spell power and mana",
				Attributes:  make(map[string]int),
			}

			// Add to party inventory
			cs.game.party.AddItem(magicRing)

			// Add combat message about the loot drop
			cs.game.AddCombatMessage("Pixie dropped a Magic Ring! It has been added to your inventory.")
		}
	}

	// Check if this is a Dragon
	if monster.Name == "Dragon" {
		// 5% chance to drop Bow of Hellfire
		if rand.Float64() < 0.05 {
			// Create Bow of Hellfire using YAML definition
			bowOfHellfire := items.CreateWeaponFromYAML("bow_of_hellfire")

			// Add to party inventory
			cs.game.party.AddItem(bowOfHellfire)

			// Add combat message about the legendary loot drop
			cs.game.AddCombatMessage("Dragon dropped a Bow of Hellfire! A legendary weapon has been added to your inventory.")
		}
	}
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
		_, _, _, effectiveEndurance, _, _, _ := member.GetEffectiveStats(cs.game.statBonus)

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

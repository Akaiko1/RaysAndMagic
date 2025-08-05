package game

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
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

	// Dynamic spell effect to spell type mapping (no more hardcoded switches!)
	spellID := items.SpellEffectToSpellID(spell.SpellEffect)
	spellType := spells.SpellIDToType(spells.SpellID(spellID))

	// Check if this is a utility spell (non-projectile)
	castingSystem := spells.NewCastingSystem(cs.game.config)
	spellDef := spells.GetSpellDefinition(spellType)

	if spellDef.IsUtility {
		// Handle utility spells (like Torch Light)
		result := castingSystem.ApplyUtilitySpell(spellType, caster.Intellect)

		// Apply the utility spell effects to the game
		if result.Success {
			// Add combat message
			cs.game.AddCombatMessage(result.Message)

			// Apply healing effects for heal spells
			if result.HealAmount > 0 {
				spellID := spells.SpellTypeToID(spellType)
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
			spellID := spells.SpellTypeToID(spellType)
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
			case "bless":
				cs.applyBlessEffect(result.Duration, result.StatBonus)
			}
		}
		return
	}

	// For projectile spells, create a projectile using effective intellect (includes Bless bonus)
	_, effectiveIntellect, _, _, _, _, _ := caster.GetEffectiveStats(cs.game.statBonus)
	projectile := castingSystem.CreateProjectile(spellType, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)

	// Get spell-specific config dynamically (no more hardcoded switches!)
	projectileSpellID := spells.SpellTypeToID(spellType)
	spellConfig := cs.game.config.GetSpellConfig(string(projectileSpellID))

	// Create magic projectile with proper type information
	magicProjectile := MagicProjectile{
		X:         projectile.X,
		Y:         projectile.Y,
		VelX:      projectile.VelX,
		VelY:      projectile.VelY,
		Damage:    projectile.Damage,
		LifeTime:  projectile.LifeTime,
		Active:    projectile.Active,
		SpellType: spellType.String(),
		Size:      projectile.Size,
	}
	cs.game.fireballs = append(cs.game.fireballs, magicProjectile)

	// Register with collision system
	projectileID := fmt.Sprintf("%s_%d", spellType.String(), len(cs.game.fireballs)-1)
	collisionSize := float64(spellConfig.CollisionSize) // Use spell-specific collision size
	projectileEntity := collision.NewEntity(projectileID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
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

	// Map item spell effects to spell types dynamically
	var spellType spells.SpellType
	switch spell.SpellEffect {
	case items.SpellEffectHealSelf:
		spellType = spells.SpellTypeHeal
	case items.SpellEffectHealOther:
		spellType = spells.SpellTypeHealOther
	case items.SpellEffectTorchLight:
		spellType = spells.SpellTypeTorchLight
	case items.SpellEffectBless:
		spellType = spells.SpellTypeBless
	case items.SpellEffectWizardEye:
		spellType = spells.SpellTypeWizardEye
	case items.SpellEffectAwaken:
		spellType = spells.SpellTypeAwaken
	default:
		// Unknown utility spell, exit
		return
	}

	// Cast the utility spell
	caster.SpellPoints -= spellCost

	// Use the spell casting system
	castingSystem := spells.NewCastingSystem(cs.game.config)
	result := castingSystem.ApplyUtilitySpell(spellType, caster.Personality)

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
	// Create arrow projectile
	arrow := Arrow{
		X:        cs.game.camera.X,
		Y:        cs.game.camera.Y,
		VelX:     math.Cos(cs.game.camera.Angle) * cs.game.config.GetArrowSpeed(),
		VelY:     math.Sin(cs.game.camera.Angle) * cs.game.config.GetArrowSpeed(),
		Damage:   damage,
		LifeTime: cs.game.config.GetArrowLifetime(),
		Active:   true,
	}

	cs.game.arrows = append(cs.game.arrows, arrow)

	// Register arrow with collision system
	arrowID := "arrow_" + strconv.Itoa(len(cs.game.arrows)-1)
	collisionSize := cs.game.config.GetArrowCollisionSize()
	arrowEntity := collision.NewEntity(arrowID, arrow.X, arrow.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(arrowEntity)
}

// createMeleeAttack creates an instant melee attack
func (cs *CombatSystem) createMeleeAttack(weapon items.Item, totalDamage int) {
	// Get weapon definition to determine category
	weaponType := items.GetWeaponTypeByName(weapon.Name)
	weaponDef := items.GetWeaponDefinition(weaponType)

	// Get weapon-specific config based on category
	weaponConfig := cs.game.config.GetWeaponConfig(weaponDef.Category)

	// Calculate target position based on camera direction and weapon range
	attackRange := float64(weapon.Range * 32) // Convert tiles to pixels
	targetX := cs.game.camera.X + math.Cos(cs.game.camera.Angle)*attackRange
	targetY := cs.game.camera.Y + math.Sin(cs.game.camera.Angle)*attackRange

	// Create attack effect at target location
	attack := SwordAttack{
		X:          targetX,
		Y:          targetY,
		VelX:       0, // No movement for range-based attacks
		VelY:       0,
		Damage:     totalDamage,
		LifeTime:   weaponConfig.Lifetime, // Use weapon-specific config
		Active:     true,
		WeaponName: weapon.Name, // Store weapon name for combat messages
	}

	cs.game.swordAttacks = append(cs.game.swordAttacks, attack)

	// Register attack with collision system
	attackID := "melee_" + strconv.Itoa(len(cs.game.swordAttacks)-1)
	collisionSize := float64(weaponConfig.CollisionSize) // Use weapon-specific collision size
	attackEntity := collision.NewEntity(attackID, attack.X, attack.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(attackEntity)
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
	selectedSpellDef := spells.GetSpellDefinitionByID(selectedSpellID)

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
		selectedSpellType := spells.SpellIDToType(selectedSpellID)
		_, effectiveIntellect, _, _, _, _, _ := currentChar.GetEffectiveStats(cs.game.statBonus)
		projectile := castingSystem.CreateProjectile(selectedSpellType, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, effectiveIntellect)

		// Create fireball using unified system
		fireball := Fireball{
			X:        projectile.X,
			Y:        projectile.Y,
			VelX:     projectile.VelX,
			VelY:     projectile.VelY,
			Damage:   projectile.Damage,
			LifeTime: projectile.LifeTime,
			Active:   projectile.Active,
		}
		cs.game.fireballs = append(cs.game.fireballs, fireball)

		// Register projectile with collision system using dynamic config
		spellConfig := cs.game.config.GetSpellConfig(string(selectedSpellID))
		fireballID := "fireball_" + strconv.Itoa(len(cs.game.fireballs)-1)
		fireballEntity := collision.NewEntity(fireballID, fireball.X, fireball.Y, float64(spellConfig.CollisionSize), float64(spellConfig.CollisionSize), collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(fireballEntity)

		// Add message based on spell definition
		cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", selectedSpellDef.Name))

	} else if selectedSpellDef.IsUtility {
		// Handle utility spells using centralized system
		selectedSpellType := spells.SpellIDToType(selectedSpellID)
		result := castingSystem.ApplyUtilitySpell(selectedSpellType, currentChar.Personality)

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
	selectedSpellType := spells.SpellIDToType(selectedSpellID)
	item := spells.CreateSpellItem(selectedSpellType)

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

		// If monster is very close, it attacks the player
		if distance < cs.game.config.MonsterAI.AttackDistance {
			// Monster attacks current character
			currentChar := cs.game.party.Members[cs.game.selectedChar]
			damage := monster.GetAttackDamage()
			
			// Apply armor damage reduction
			finalDamage := cs.ApplyArmorDamageReduction(damage, currentChar)
			currentChar.HitPoints -= finalDamage
			if currentChar.HitPoints < 0 {
				currentChar.HitPoints = 0
			}

			// Trigger damage blink effect for the character that was hit
			cs.game.TriggerDamageBlink(cs.game.selectedChar)

			// Push monster back slightly to prevent spam
			pushBack := cs.game.config.MonsterAI.PushbackDistance
			monster.X += (monster.X - cs.game.camera.X) / distance * pushBack
			monster.Y += (monster.Y - cs.game.camera.Y) / distance * pushBack
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

	// Determine projectile type based on entity ID
	// Check specific projectile types first to avoid conflicts
	if len(projectileEntity.ID) > 6 && projectileEntity.ID[:6] == "arrow_" {
		// Parse index from ID and find the arrow
		if idx, err := strconv.Atoi(projectileEntity.ID[6:]); err == nil {
			// Find the arrow by checking if it's still active and at the expected index
			if idx < len(cs.game.arrows) && cs.game.arrows[idx].Active && cs.game.arrows[idx].LifeTime > 0 {
				projectile = &cs.game.arrows[idx]
				projectileType = "arrow"
			}
		}
	} else if len(projectileEntity.ID) > 6 && projectileEntity.ID[:6] == "melee_" {
		// Parse index from ID
		if idx, err := strconv.Atoi(projectileEntity.ID[6:]); err == nil && idx < len(cs.game.swordAttacks) {
			projectile = &cs.game.swordAttacks[idx]
			projectileType = "melee"
		}
	} else if len(projectileEntity.ID) > 9 && projectileEntity.ID[:9] == "fireball_" {
		// Legacy format for backward compatibility
		if idx, err := strconv.Atoi(projectileEntity.ID[9:]); err == nil && idx < len(cs.game.fireballs) {
			projectile = &cs.game.fireballs[idx]
			projectileType = "fireball"
		}
	} else {
		// Handle spell projectile format (spellname_index) - only for actual spells
		parts := strings.Split(projectileEntity.ID, "_")
		if len(parts) == 2 {
			// Check if this is a spell projectile (e.g., "firebolt_0", "fireball_1")
			spellName := parts[0]
			if idx, err := strconv.Atoi(parts[1]); err == nil && idx < len(cs.game.fireballs) {
				// Verify this is actually a spell projectile by checking the spell type
				if cs.game.fireballs[idx].SpellType == spellName {
					projectile = &cs.game.fireballs[idx]
					projectileType = "fireball" // All magic projectiles use "fireball" type internally
				}
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

	// Handle the collision based on projectile type
	switch projectileType {
	case "fireball":
		fireball := projectile.(*Fireball)
		if !fireball.Active || fireball.LifeTime <= 0 {
			return
		}

		// Get the actual spell name from the projectile
		spellName := fireball.SpellType
		if spellDef := spells.GetSpellDefinitionByID(spells.SpellID(spellName)); spellDef.Name != "" {
			spellName = spellDef.Name
		}

		// Monster takes fire damage
		actualDamage := monster.TakeDamage(fireball.Damage, monsterPkg.DamageFire)
		fireball.Active = false

		// Unregister the fireball from collision system
		cs.game.collisionSystem.UnregisterEntity(projectileEntity.ID)

		// Check if monster died and give rewards
		if !monster.IsAlive() {
			cs.awardExperienceAndGold(monster)
			message := fmt.Sprintf("%s killed %s! Awarded %d experience and %d gold.",
				spellName, monster.Name, monster.Experience, monster.Gold)
			cs.game.AddCombatMessage(message)
		} else {
			message := fmt.Sprintf("%s hit %s for %d fire damage! (HP: %d/%d)",
				spellName, monster.Name, actualDamage, monster.HitPoints, monster.MaxHitPoints)
			cs.game.AddCombatMessage(message)
		}

	case "melee":
		attack := projectile.(*SwordAttack)
		if !attack.Active || attack.LifeTime <= 0 {
			return
		}

		// Monster takes physical damage
		actualDamage := monster.TakeDamage(attack.Damage, monsterPkg.DamagePhysical)
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

		// Monster takes physical damage from arrow
		actualDamage := monster.TakeDamage(arrow.Damage, monsterPkg.DamagePhysical)
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
	}
}

// CalculateWeaponDamage calculates total weapon damage using weapon-specific bonus stat
func (cs *CombatSystem) CalculateWeaponDamage(weapon items.Item, character *character.MMCharacter) (int, int, int) {
	baseDamage := weapon.Damage

	// Get effective stats including any stat bonuses (Bless, Day of Gods, etc.)
	might, intellect, _, _, accuracy, _, _ := character.GetEffectiveStats(cs.game.statBonus)

	// Get the appropriate stat bonus based on weapon's bonus stat
	var statBonus int
	switch weapon.BonusStat {
	case "Might":
		statBonus = might / 3
	case "Accuracy":
		statBonus = accuracy / 3
	case "Intellect":
		statBonus = intellect / 3
	default:
		// Fallback to Might for weapons without bonus stat specified
		statBonus = might / 3
	}

	totalDamage := baseDamage + statBonus
	return baseDamage, statBonus, totalDamage
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
	cs.game.statBonus += statBonus  // ADD to total stat bonus
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

	// Future: Add more monster loot drops here
	// if monster.Name == "Dragon" {
	//     // Rare loot drops for dragons
	// }
}

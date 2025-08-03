package game

import (
	"fmt"
	"math"
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

	// Map item spell effects to spell types
	var spellType spells.SpellType
	switch spell.SpellEffect {
	case items.SpellEffectFireball:
		spellType = spells.SpellTypeFireball
	case items.SpellEffectLightning:
		spellType = spells.SpellTypeLightning
	case items.SpellEffectFireBolt:
		spellType = spells.SpellTypeFireBolt
	case items.SpellEffectTorchLight:
		spellType = spells.SpellTypeTorchLight
	case items.SpellEffectWizardEye:
		spellType = spells.SpellTypeWizardEye
	case items.SpellEffectHealSelf:
		spellType = spells.SpellTypeHeal
	case items.SpellEffectHealOther:
		spellType = spells.SpellTypeHealOther
	case items.SpellEffectBless:
		spellType = spells.SpellTypeBless
	case items.SpellEffectAwaken:
		spellType = spells.SpellTypeAwaken
	case items.SpellEffectWalkOnWater:
		spellType = spells.SpellTypeWalkOnWater
	default:
		// Default to fireball for unknown spells
		spellType = spells.SpellTypeFireball
	}

	// Check if this is a utility spell (non-projectile)
	castingSystem := spells.NewCastingSystem(cs.game.config)
	spellEffect := spells.GetSpellEffect(spellType)

	if spellEffect.IsUtility {
		// Handle utility spells (like Torch Light)
		result := castingSystem.ApplyUtilitySpell(spellType, caster.Intellect)

		// Apply the utility spell effects to the game
		if result.Success {
			// Add combat message
			cs.game.AddCombatMessage(result.Message)

			// Apply healing effects for heal spells
			if result.HealAmount > 0 {
				if result.TargetSelf || spellType == spells.SpellTypeHeal {
					// Heal self
					caster.HitPoints += result.HealAmount
					if caster.HitPoints > caster.MaxHitPoints {
						caster.HitPoints = caster.MaxHitPoints
					}
				} else if spellType == spells.SpellTypeHealOther {
					// For F key heal other, just heal self since there's no target selection
					// Use H key with mouse for proper targeting
					caster.HitPoints += result.HealAmount
					if caster.HitPoints > caster.MaxHitPoints {
						caster.HitPoints = caster.MaxHitPoints
					}
				}
			}

			// Apply torch light effect if it's torch light spell
			if spellType == spells.SpellTypeTorchLight {
				cs.game.torchLightActive = true
				cs.game.torchLightDuration = result.Duration
				cs.game.torchLightRadius = 4.0 // 4-tile radius
			}

			// Apply wizard eye effect if it's wizard eye spell
			if spellType == spells.SpellTypeWizardEye {
				cs.game.wizardEyeActive = true
				cs.game.wizardEyeDuration = result.Duration
			}

			// Apply walk on water effect if it's walk on water spell
			if spellType == spells.SpellTypeWalkOnWater {
				cs.game.walkOnWaterActive = true
				cs.game.walkOnWaterDuration = result.Duration
			}
		}
		return
	}

	// For projectile spells, create a projectile
	projectile := castingSystem.CreateProjectile(spellType, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle, caster.Intellect)

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
	projectileEntity := collision.NewEntity(projectileID, magicProjectile.X, magicProjectile.Y, float64(magicProjectile.Size), float64(magicProjectile.Size), collision.CollisionTypeProjectile, false)
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

	// Map item spell effects to spell types
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
	// Calculate heal amount based on spell cost and caster's personality
	healAmount := spellCost*3 + caster.Personality/2
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

	// Calculate damage based on weapon damage and character stats
	baseDamage := weapon.Damage
	mightBonus := attacker.Might / 3
	totalDamage := baseDamage + mightBonus

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
		VelX:     math.Cos(cs.game.camera.Angle) * cs.game.config.GetFireballSpeed() * 1.2, // Slightly faster than fireballs
		VelY:     math.Sin(cs.game.camera.Angle) * cs.game.config.GetFireballSpeed() * 1.2,
		Damage:   damage,
		LifeTime: 54, // 8 tiles range (8 * 64 / 9.6 ≈ 54 frames)
		Active:   true,
	}

	cs.game.arrows = append(cs.game.arrows, arrow)

	// Register arrow with collision system
	arrowID := "arrow_" + strconv.Itoa(len(cs.game.arrows)-1)
	arrowEntity := collision.NewEntity(arrowID, arrow.X, arrow.Y, 12, 12, collision.CollisionTypeProjectile, false)
	cs.game.collisionSystem.RegisterEntity(arrowEntity)
}

// createMeleeAttack creates an instant melee attack
func (cs *CombatSystem) createMeleeAttack(weapon items.Item, totalDamage int) {
	// Calculate target position based on camera direction and weapon range
	attackRange := float64(weapon.Range * 32) // Convert tiles to pixels
	targetX := cs.game.camera.X + math.Cos(cs.game.camera.Angle)*attackRange
	targetY := cs.game.camera.Y + math.Sin(cs.game.camera.Angle)*attackRange

	// Create attack effect at target location
	attack := SwordAttack{
		X:        targetX,
		Y:        targetY,
		VelX:     0, // No movement for range-based attacks
		VelY:     0,
		Damage:   totalDamage,
		LifeTime: 10, // Short duration for instant hit
		Active:   true,
	}

	cs.game.swordAttacks = append(cs.game.swordAttacks, attack)

	// Register attack with collision system
	attackID := "melee_" + strconv.Itoa(len(cs.game.swordAttacks)-1)
	attackEntity := collision.NewEntity(attackID, attack.X, attack.Y, 24, 24, collision.CollisionTypeProjectile, false)
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
	spells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell >= len(spells) {
		return
	}

	selectedSpell := spells[cs.game.selectedSpell]

	// Check spell points
	if currentChar.SpellPoints < selectedSpell.SpellPoints {
		return
	}

	// Cast the spell
	currentChar.SpellPoints -= selectedSpell.SpellPoints

	// Handle different spell effects
	switch selectedSchool {
	case character.MagicFire:
		if selectedSpell.Name == "Fireball" || selectedSpell.Name == "Fire Bolt" {
			// Create fireball projectile
			fireball := Fireball{
				X:        cs.game.camera.X,
				Y:        cs.game.camera.Y,
				VelX:     math.Cos(cs.game.camera.Angle) * cs.game.config.GetFireballSpeed(),
				VelY:     math.Sin(cs.game.camera.Angle) * cs.game.config.GetFireballSpeed(),
				Damage:   selectedSpell.SpellPoints*3 + currentChar.Intellect/2,
				LifeTime: cs.game.config.GetFireballLifetime(),
				Active:   true,
			}
			cs.game.fireballs = append(cs.game.fireballs, fireball)

			// Register fireball with collision system
			fireballID := "fireball_" + strconv.Itoa(len(cs.game.fireballs)-1)
			fireballEntity := collision.NewEntity(fireballID, fireball.X, fireball.Y, 16, 16, collision.CollisionTypeProjectile, false)
			cs.game.collisionSystem.RegisterEntity(fireballEntity)
		}
	case character.MagicBody:
		if selectedSpell.Name == "Heal" || selectedSpell.Name == "First Aid" {
			// Heal current character
			healAmount := selectedSpell.SpellPoints*5 + currentChar.Personality/2
			currentChar.HitPoints += healAmount
			if currentChar.HitPoints > currentChar.MaxHitPoints {
				currentChar.HitPoints = currentChar.MaxHitPoints
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
	spells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell >= len(spells) {
		return
	}

	selectedSpell := spells[cs.game.selectedSpell]

	// Convert the character's known spell to a proper spell item
	var item items.Item

	// Map spell names to proper spell effects and create items using helper functions
	switch selectedSpell.Name {
	case "Fire Bolt":
		item = items.CreateBattleSpell("Fire Bolt", items.SpellEffectFireBolt, "fire", selectedSpell.SpellPoints, "Launches a fast bolt of fire")
	case "Fireball":
		item = items.CreateBattleSpell("Fireball", items.SpellEffectFireball, "fire", selectedSpell.SpellPoints, "Hurls a powerful ball of fire")
	case "Lightning Bolt", "Lightning":
		item = items.CreateBattleSpell("Lightning Bolt", items.SpellEffectLightning, "air", selectedSpell.SpellPoints, "Strikes with a bolt of lightning")
	case "Torch Light":
		item = items.CreateUtilitySpell("Torch Light", items.SpellEffectTorchLight, "fire", selectedSpell.SpellPoints, "Creates magical illumination")
	case "Heal":
		item = items.CreateUtilitySpell("Heal", items.SpellEffectHealOther, "body", selectedSpell.SpellPoints, "Restores health to self or others")
	case "First Aid":
		item = items.CreateUtilitySpell("First Aid", items.SpellEffectHealSelf, "body", selectedSpell.SpellPoints, "Basic healing for the caster")
	case "Heal Other":
		item = items.CreateUtilitySpell("Heal Other", items.SpellEffectHealOther, "body", selectedSpell.SpellPoints, "Restores health to a target party member")
	case "Bless":
		item = items.CreateUtilitySpell("Bless", items.SpellEffectBless, "spirit", selectedSpell.SpellPoints, "Increases party's combat effectiveness")
	case "Wizard Eye":
		item = items.CreateUtilitySpell("Wizard Eye", items.SpellEffectWizardEye, "air", selectedSpell.SpellPoints, "Reveals the surrounding area")
	case "Awaken":
		item = items.CreateUtilitySpell("Awaken", items.SpellEffectAwaken, "water", selectedSpell.SpellPoints, "Awakens fallen party members")
	case "Walk on Water":
		item = items.CreateUtilitySpell("Walk on Water", items.SpellEffectWalkOnWater, "water", selectedSpell.SpellPoints, "Allows the party to walk across water surfaces")
	default:
		// For unknown spells, determine type based on magic school
		currentChar := cs.game.party.Members[cs.game.selectedChar]
		schoolName := currentChar.GetMagicSchoolName(selectedSchool)
		if selectedSchool == character.MagicFire || selectedSchool == character.MagicAir || selectedSchool == character.MagicWater || selectedSchool == character.MagicEarth {
			// Elemental magic - battle spell
			item = items.CreateBattleSpell(selectedSpell.Name, items.SpellEffectFireball, schoolName, selectedSpell.SpellPoints, fmt.Sprintf("A %s spell", schoolName))
		} else {
			// Self magic (Body, Mind, Spirit) - utility spell
			item = items.CreateUtilitySpell(selectedSpell.Name, items.SpellEffectPartyBuff, schoolName, selectedSpell.SpellPoints, fmt.Sprintf("A %s spell", schoolName))
		}
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

		// If monster is very close, it attacks the player
		if distance < cs.game.config.MonsterAI.AttackDistance {
			// Monster attacks current character
			currentChar := cs.game.party.Members[cs.game.selectedChar]
			damage := monster.GetAttackDamage()
			currentChar.HitPoints -= damage
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
	// Handle new spell type format (SpellTypeXXX_index) and legacy formats
	if strings.Contains(projectileEntity.ID, "SpellType") {
		// New spell system: SpellTypeFireball_0, SpellTypeFireBolt_1, etc.
		parts := strings.Split(projectileEntity.ID, "_")
		if len(parts) == 2 {
			if idx, err := strconv.Atoi(parts[1]); err == nil && idx < len(cs.game.fireballs) {
				projectile = &cs.game.fireballs[idx]
				projectileType = "fireball" // All magic projectiles use "fireball" type internally
			}
		}
	} else if len(projectileEntity.ID) > 9 && projectileEntity.ID[:9] == "fireball_" {
		// Legacy format for backward compatibility
		if idx, err := strconv.Atoi(projectileEntity.ID[9:]); err == nil && idx < len(cs.game.fireballs) {
			projectile = &cs.game.fireballs[idx]
			projectileType = "fireball"
		}
	} else if len(projectileEntity.ID) > 6 && projectileEntity.ID[:6] == "melee_" {
		// Parse index from ID
		if idx, err := strconv.Atoi(projectileEntity.ID[6:]); err == nil && idx < len(cs.game.swordAttacks) {
			projectile = &cs.game.swordAttacks[idx]
			projectileType = "melee"
		}
	} else if len(projectileEntity.ID) > 6 && projectileEntity.ID[:6] == "arrow_" {
		// Parse index from ID
		if idx, err := strconv.Atoi(projectileEntity.ID[6:]); err == nil && idx < len(cs.game.arrows) {
			projectile = &cs.game.arrows[idx]
			projectileType = "arrow"
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

		// Monster takes fire damage
		actualDamage := monster.TakeDamage(fireball.Damage, monsterPkg.DamageFire)
		fireball.Active = false

		// Unregister the fireball from collision system
		cs.game.collisionSystem.UnregisterEntity(projectileEntity.ID)

		// Check if monster died and give rewards
		if !monster.IsAlive() {
			cs.awardExperienceAndGold(monster)
			message := fmt.Sprintf("Fireball killed %s! Awarded %d experience and %d gold.",
				monster.Name, monster.Experience, monster.Gold)
			cs.game.AddCombatMessage(message)
		} else {
			message := fmt.Sprintf("Fireball hit %s for %d fire damage! (HP: %d/%d)",
				monster.Name, actualDamage, monster.HitPoints, monster.MaxHitPoints)
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
			message := fmt.Sprintf("Sword attack killed %s! Awarded %d experience and %d gold.",
				monster.Name, monster.Experience, monster.Gold)
			cs.game.AddCombatMessage(message)
		} else {
			message := fmt.Sprintf("Sword attack hit %s for %d physical damage! (HP: %d/%d)",
				monster.Name, actualDamage, monster.HitPoints, monster.MaxHitPoints)
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
}

// checkLevelUp checks if a character should level up and applies level up benefits
func (cs *CombatSystem) checkLevelUp(character *character.MMCharacter) {
	// Simple level progression: each level requires level * 100 experience
	requiredExp := character.Level * 100

	if character.Experience >= requiredExp {
		oldLevel := character.Level
		character.Level++
		character.Experience -= requiredExp // Subtract used experience

		// Recalculate derived stats (health and mana increase with level)
		character.CalculateDerivedStats(cs.game.config)

		// Restore full health and mana on level up
		character.HitPoints = character.MaxHitPoints
		character.SpellPoints = character.MaxSpellPoints

		message := fmt.Sprintf("%s reached level %d! (was level %d)",
			character.Name, character.Level, oldLevel)
		cs.game.AddCombatMessage(message)
	}
}


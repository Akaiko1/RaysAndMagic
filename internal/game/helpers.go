package game

import (
	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

// Helper functions to reduce code duplication in wrapper creation and common operations

// CreateMagicProjectileWrapper creates a wrapper for magic projectile entities
func CreateMagicProjectileWrapper(magicProjectile *MagicProjectile, collisionSystem *collision.CollisionSystem, projectileID string) entities.ProjectileUpdateInterface {
	return &MagicProjectileWrapper{
		MagicProjectile: magicProjectile,
		collisionSystem: collisionSystem,
		projectileID:    projectileID,
	}
}

// CreateMeleeAttackWrapper creates a wrapper for melee attack entities
func CreateMeleeAttackWrapper(attack *MeleeAttack, collisionSystem *collision.CollisionSystem, projectileID string) entities.ProjectileUpdateInterface {
	return &MeleeAttackWrapper{
		MeleeAttack:     attack,
		collisionSystem: collisionSystem,
		projectileID:    projectileID,
	}
}

// CreateArrowWrapper creates a wrapper for arrow entities
func CreateArrowWrapper(arrow *Arrow, collisionSystem *collision.CollisionSystem, projectileID string) entities.ProjectileUpdateInterface {
	return &ArrowWrapper{
		Arrow:           arrow,
		collisionSystem: collisionSystem,
		projectileID:    projectileID,
	}
}

// CreateMonsterWrapper creates a wrapper for monster entities
func CreateMonsterWrapper(monster *monster.Monster3D, collisionSystem *collision.CollisionSystem, monsterID string, game *MMGame) entities.MonsterUpdateInterface {
	return &MonsterWrapper{
		Monster:         monster,
		collisionSystem: collisionSystem,
		monsterID:       monsterID,
		game:            game,
	}
}

// ConvertProjectilesToWrappers converts projectile slices to wrapper interfaces
func (g *MMGame) ConvertProjectilesToWrappers() []entities.ProjectileUpdateInterface {
	var allProjectiles []entities.ProjectileUpdateInterface

	// Convert magic projectiles
	for i := range g.magicProjectiles {
		// Use the actual unique ID from the projectile
		allProjectiles = append(allProjectiles, CreateMagicProjectileWrapper(&g.magicProjectiles[i], g.collisionSystem, g.magicProjectiles[i].ID))
	}

	// Convert melee attacks
	for i := range g.meleeAttacks {
		// Use the actual unique ID from the attack
		allProjectiles = append(allProjectiles, CreateMeleeAttackWrapper(&g.meleeAttacks[i], g.collisionSystem, g.meleeAttacks[i].ID))
	}

	// Convert arrows
	for i := range g.arrows {
		// Use the actual unique ID from the arrow
		allProjectiles = append(allProjectiles, CreateArrowWrapper(&g.arrows[i], g.collisionSystem, g.arrows[i].ID))
	}

	return allProjectiles
}

// ConvertMonstersToWrappers converts monster slice to wrapper interfaces
func (g *MMGame) ConvertMonstersToWrappers() []entities.MonsterUpdateInterface {
	monsters := make([]entities.MonsterUpdateInterface, 0, len(g.world.Monsters))
	for _, monster := range g.world.Monsters {
		// Use the monster's unique ID instead of array index and pass game instance for player position access
		monsters = append(monsters, CreateMonsterWrapper(monster, g.collisionSystem, monster.ID, g))
	}
	return monsters
}

// RemoveInactiveEntities removes inactive projectiles from slices
// This consolidates the duplicate removal logic
func (g *MMGame) RemoveInactiveEntities() {
	// Remove inactive magic projectiles and unregister from collision system
	activeMagicProjectiles := make([]MagicProjectile, 0, len(g.magicProjectiles))
	for _, magicProjectile := range g.magicProjectiles {
		if magicProjectile.Active && magicProjectile.LifeTime > 0 {
			activeMagicProjectiles = append(activeMagicProjectiles, magicProjectile)
		} else {
			// Unregister inactive magic projectile from collision system using unique ID
			g.collisionSystem.UnregisterEntity(magicProjectile.ID)
		}
	}
	g.magicProjectiles = activeMagicProjectiles

	// Remove inactive melee attacks and unregister from collision system
	activeMeleeAttacks := make([]MeleeAttack, 0, len(g.meleeAttacks))
	for _, attack := range g.meleeAttacks {
		if attack.Active && attack.LifeTime > 0 {
			activeMeleeAttacks = append(activeMeleeAttacks, attack)
		} else {
			// Unregister inactive attack from collision system using unique ID
			g.collisionSystem.UnregisterEntity(attack.ID)
		}
	}
	g.meleeAttacks = activeMeleeAttacks

	// Remove inactive arrows and unregister from collision system
	activeArrows := make([]Arrow, 0, len(g.arrows))
	for _, arrow := range g.arrows {
		if arrow.Active && arrow.LifeTime > 0 {
			activeArrows = append(activeArrows, arrow)
		} else {
			// Unregister inactive arrow from collision system using unique ID
			g.collisionSystem.UnregisterEntity(arrow.ID)
		}
	}
	g.arrows = activeArrows
}

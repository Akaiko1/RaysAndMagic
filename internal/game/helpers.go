package game

import (
	"fmt"
	"strconv"
	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

// Helper functions to reduce code duplication in wrapper creation and common operations

// CreateFireballWrapper creates a wrapper for fireball entities
func CreateFireballWrapper(fireball *Fireball, collisionSystem *collision.CollisionSystem, projectileID string) entities.ProjectileUpdateInterface {
	return &FireballWrapper{
		Fireball:        fireball,
		collisionSystem: collisionSystem,
		projectileID:    projectileID,
	}
}

// CreateSwordAttackWrapper creates a wrapper for sword attack entities
func CreateSwordAttackWrapper(attack *SwordAttack, collisionSystem *collision.CollisionSystem, projectileID string) entities.ProjectileUpdateInterface {
	return &SwordAttackWrapper{
		SwordAttack:     attack,
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
func CreateMonsterWrapper(monster *monster.Monster3D, collisionSystem *collision.CollisionSystem, monsterID string) entities.MonsterUpdateInterface {
	return &MonsterWrapper{
		Monster:         monster,
		collisionSystem: collisionSystem,
		monsterID:       monsterID,
	}
}

// ConvertProjectilesToWrappers converts projectile slices to wrapper interfaces
func (g *MMGame) ConvertProjectilesToWrappers() []entities.ProjectileUpdateInterface {
	var allProjectiles []entities.ProjectileUpdateInterface

	// Convert fireballs (magic projectiles)
	for i := range g.fireballs {
		// Use the spell type from the projectile to create the correct collision ID
		// This matches the ID format created during projectile registration
		projectileID := fmt.Sprintf("%s_%d", g.fireballs[i].SpellType, i)
		allProjectiles = append(allProjectiles, CreateFireballWrapper(&g.fireballs[i], g.collisionSystem, projectileID))
	}

	// Convert sword attacks
	for i := range g.swordAttacks {
		projectileID := "sword_" + strconv.Itoa(i)
		allProjectiles = append(allProjectiles, CreateSwordAttackWrapper(&g.swordAttacks[i], g.collisionSystem, projectileID))
	}

	// Convert arrows
	for i := range g.arrows {
		projectileID := "arrow_" + strconv.Itoa(i)
		allProjectiles = append(allProjectiles, CreateArrowWrapper(&g.arrows[i], g.collisionSystem, projectileID))
	}

	return allProjectiles
}

// ConvertMonstersToWrappers converts monster slice to wrapper interfaces
func (g *MMGame) ConvertMonstersToWrappers() []entities.MonsterUpdateInterface {
	monsters := make([]entities.MonsterUpdateInterface, 0, len(g.world.Monsters))
	for _, monster := range g.world.Monsters {
		// Use the monster's unique ID instead of array index
		monsters = append(monsters, CreateMonsterWrapper(monster, g.collisionSystem, monster.ID))
	}
	return monsters
}

// RemoveInactiveEntities removes inactive projectiles from slices
// This consolidates the duplicate removal logic
func (g *MMGame) RemoveInactiveEntities() {
	// Remove inactive fireballs
	activeFireballs := make([]Fireball, 0, len(g.fireballs))
	for _, fireball := range g.fireballs {
		if fireball.Active && fireball.LifeTime > 0 {
			activeFireballs = append(activeFireballs, fireball)
		}
	}
	g.fireballs = activeFireballs

	// Remove inactive sword attacks
	activeSwordAttacks := make([]SwordAttack, 0, len(g.swordAttacks))
	for _, attack := range g.swordAttacks {
		if attack.Active && attack.LifeTime > 0 {
			activeSwordAttacks = append(activeSwordAttacks, attack)
		}
	}
	g.swordAttacks = activeSwordAttacks

	// Remove inactive arrows
	activeArrows := make([]Arrow, 0, len(g.arrows))
	for _, arrow := range g.arrows {
		if arrow.Active && arrow.LifeTime > 0 {
			activeArrows = append(activeArrows, arrow)
		}
	}
	g.arrows = activeArrows
}

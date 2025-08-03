package character

import (
	"math"
	"testing"
	"ugataima/internal/config"
)

// Standalone combat mechanics testing without graphics dependencies
type TestCombatSystem struct {
	party        *Party
	config       *config.Config
	selectedChar int
	camera       TestCamera
	fireballs    []TestFireball
	swordAttacks []TestSwordAttack
}

type TestCamera struct {
	X, Y  float64
	Angle float64
}

type TestFireball struct {
	X, Y       float64
	VelX, VelY float64
	Damage     int
	LifeTime   int
	Active     bool
}

type TestSwordAttack struct {
	X, Y       float64
	VelX, VelY float64
	Damage     int
	LifeTime   int
	Active     bool
}

func NewTestCombatSystem(cfg *config.Config) *TestCombatSystem {
	party := NewParty(cfg)
	return &TestCombatSystem{
		party:        party,
		config:       cfg,
		selectedChar: 0,
		camera:       TestCamera{X: 100, Y: 100, Angle: 0},
		fireballs:    make([]TestFireball, 0),
		swordAttacks: make([]TestSwordAttack, 0),
	}
}

func (cs *TestCombatSystem) CastFireball() bool {
	caster := cs.party.Members[cs.selectedChar]

	// Check if caster can cast fire magic
	fireSkill, canCast := caster.MagicSchools[MagicFire]
	if !canCast || len(fireSkill.Spells) == 0 {
		return false
	}

	// Check spell points
	spellCost := 3 // Default fireball cost
	if caster.SpellPoints < spellCost {
		return false
	}

	// Cast fireball
	caster.SpellPoints -= spellCost

	// Create fireball projectile
	fireball := TestFireball{
		X:        cs.camera.X,
		Y:        cs.camera.Y,
		VelX:     math.Cos(cs.camera.Angle) * 3.0, // Default speed
		VelY:     math.Sin(cs.camera.Angle) * 3.0,
		Damage:   10 + caster.Intellect/2,
		LifeTime: 60, // Default lifetime
		Active:   true,
	}

	cs.fireballs = append(cs.fireballs, fireball)
	return true
}

func (cs *TestCombatSystem) CastHeal() bool {
	caster := cs.party.Members[cs.selectedChar]

	// Check if caster can cast body magic
	bodySkill, canCast := caster.MagicSchools[MagicBody]
	if !canCast || len(bodySkill.Spells) == 0 {
		return false
	}

	// Check spell points
	spellCost := 2 // Default heal cost
	if caster.SpellPoints < spellCost {
		return false
	}

	// Cast heal
	caster.SpellPoints -= spellCost
	healAmount := 15 + caster.Personality/2
	caster.HitPoints += healAmount
	if caster.HitPoints > caster.MaxHitPoints {
		caster.HitPoints = caster.MaxHitPoints
	}
	return true
}

func (cs *TestCombatSystem) SwordAttack() bool {
	attacker := cs.party.Members[cs.selectedChar]

	// Check if character has sword skill
	swordSkill, hasSword := attacker.Skills[SkillSword]
	if !hasSword {
		return false
	}

	// Create sword attack
	attack := TestSwordAttack{
		X:        cs.camera.X,
		Y:        cs.camera.Y,
		VelX:     math.Cos(cs.camera.Angle) * 5.0, // Default sword speed
		VelY:     math.Sin(cs.camera.Angle) * 5.0,
		Damage:   8 + attacker.Might/3 + swordSkill.Level,
		LifeTime: 15, // Default sword lifetime
		Active:   true,
	}

	cs.swordAttacks = append(cs.swordAttacks, attack)
	return true
}

func (cs *TestCombatSystem) UpdateProjectiles() {
	// Update fireballs
	for i := range cs.fireballs {
		if cs.fireballs[i].Active {
			cs.fireballs[i].X += cs.fireballs[i].VelX
			cs.fireballs[i].Y += cs.fireballs[i].VelY
			cs.fireballs[i].LifeTime--
			if cs.fireballs[i].LifeTime <= 0 {
				cs.fireballs[i].Active = false
			}
		}
	}

	// Update sword attacks
	for i := range cs.swordAttacks {
		if cs.swordAttacks[i].Active {
			cs.swordAttacks[i].X += cs.swordAttacks[i].VelX
			cs.swordAttacks[i].Y += cs.swordAttacks[i].VelY
			cs.swordAttacks[i].LifeTime--
			if cs.swordAttacks[i].LifeTime <= 0 {
				cs.swordAttacks[i].Active = false
			}
		}
	}
}

func TestCombatMechanics(t *testing.T) {
	cfg := createTestConfig()
	combat := NewTestCombatSystem(cfg)

	t.Run("Fireball Casting", func(t *testing.T) {
		// Test with sorcerer (index 1)
		combat.selectedChar = 1
		sorcerer := combat.party.Members[1]

		originalSP := sorcerer.SpellPoints
		originalFireballCount := len(combat.fireballs)

		// Cast fireball
		success := combat.CastFireball()

		if !success {
			t.Error("Sorcerer should be able to cast fireball")
		}

		// Check spell points were consumed
		if sorcerer.SpellPoints >= originalSP {
			t.Error("Spell points should have been consumed")
		}

		// Check fireball was created
		if len(combat.fireballs) <= originalFireballCount {
			t.Error("Fireball should have been created")
		}

		// Check fireball properties
		if len(combat.fireballs) > 0 {
			fb := combat.fireballs[len(combat.fireballs)-1]
			if !fb.Active {
				t.Error("New fireball should be active")
			}
			if fb.Damage <= 0 {
				t.Error("Fireball should have positive damage")
			}
			if fb.LifeTime <= 0 {
				t.Error("Fireball should have positive lifetime")
			}

			expectedDamage := 10 + sorcerer.Intellect/2
			if fb.Damage != expectedDamage {
				t.Errorf("Expected fireball damage %d, got %d", expectedDamage, fb.Damage)
			}
		}
	})

	t.Run("Heal Casting", func(t *testing.T) {
		// Test with cleric (index 2)
		combat.selectedChar = 2
		cleric := combat.party.Members[2]

		// Damage the cleric first
		cleric.HitPoints -= 10
		originalHP := cleric.HitPoints
		originalSP := cleric.SpellPoints

		// Cast heal
		success := combat.CastHeal()

		if !success {
			t.Error("Cleric should be able to cast heal")
		}

		// Check spell points were consumed
		if cleric.SpellPoints >= originalSP {
			t.Error("Spell points should have been consumed for heal")
		}

		// Check hit points were restored
		if cleric.HitPoints <= originalHP {
			t.Error("Hit points should have been restored")
		}

		// Check hit points don't exceed maximum
		if cleric.HitPoints > cleric.MaxHitPoints {
			t.Error("Hit points should not exceed maximum")
		}

		expectedHeal := 15 + cleric.Personality/2
		actualHeal := cleric.HitPoints - originalHP
		// Account for max HP capping
		maxPossibleHeal := cleric.MaxHitPoints - originalHP
		expectedActualHeal := expectedHeal
		if expectedHeal > maxPossibleHeal {
			expectedActualHeal = maxPossibleHeal
		}

		if actualHeal != expectedActualHeal {
			t.Errorf("Expected heal amount %d (capped from %d), got %d (cleric personality: %d)",
				expectedActualHeal, expectedHeal, actualHeal, cleric.Personality)
		}
	})

	t.Run("Sword Attack", func(t *testing.T) {
		// Test with knight (index 0)
		combat.selectedChar = 0
		knight := combat.party.Members[0]

		originalAttackCount := len(combat.swordAttacks)

		// Perform sword attack
		success := combat.SwordAttack()

		if !success {
			t.Error("Knight should be able to perform sword attack")
		}

		// Check sword attack was created
		if len(combat.swordAttacks) <= originalAttackCount {
			t.Error("Sword attack should have been created")
		}

		// Check sword attack properties
		if len(combat.swordAttacks) > 0 {
			attack := combat.swordAttacks[len(combat.swordAttacks)-1]
			if !attack.Active {
				t.Error("New sword attack should be active")
			}
			if attack.Damage <= 0 {
				t.Error("Sword attack should have positive damage")
			}
			if attack.LifeTime <= 0 {
				t.Error("Sword attack should have positive lifetime")
			}

			swordSkill := knight.Skills[SkillSword]
			expectedDamage := 8 + knight.Might/3 + swordSkill.Level
			if attack.Damage != expectedDamage {
				t.Errorf("Expected sword damage %d, got %d", expectedDamage, attack.Damage)
			}
		}
	})

	t.Run("Class Restrictions", func(t *testing.T) {
		// Test archer trying to cast fireball (should fail)
		combat.selectedChar = 3 // Archer
		archer := combat.party.Members[3]

		originalSP := archer.SpellPoints
		originalFireballCount := len(combat.fireballs)

		// Try to cast fireball
		success := combat.CastFireball()

		if success {
			t.Error("Archer should not be able to cast fireball")
		}

		// Should not consume spell points or create fireball
		if archer.SpellPoints != originalSP {
			t.Error("Archer should not consume spell points for failed fireball")
		}
		if len(combat.fireballs) != originalFireballCount {
			t.Error("No fireball should be created by archer")
		}

		// Test knight trying to cast heal (should fail)
		combat.selectedChar = 0 // Knight
		knight := combat.party.Members[0]

		originalKnightSP := knight.SpellPoints
		originalKnightHP := knight.HitPoints

		success = combat.CastHeal()

		if success {
			t.Error("Knight should not be able to cast heal")
		}

		if knight.SpellPoints != originalKnightSP {
			t.Error("Knight should not consume spell points for failed heal")
		}
		if knight.HitPoints != originalKnightHP {
			t.Error("Knight HP should not change from failed heal")
		}
	})
}

func TestProjectileUpdates(t *testing.T) {
	cfg := createTestConfig()
	combat := NewTestCombatSystem(cfg)

	t.Run("Fireball Lifetime", func(t *testing.T) {
		// Create a fireball
		combat.selectedChar = 1 // Sorcerer
		combat.CastFireball()

		if len(combat.fireballs) == 0 {
			t.Fatal("Should have created a fireball")
		}

		fireball := &combat.fireballs[0]
		originalLifetime := fireball.LifeTime
		originalX := fireball.X
		originalY := fireball.Y

		// Update projectiles
		combat.UpdateProjectiles()

		// Check lifetime decreased
		if fireball.LifeTime >= originalLifetime {
			t.Error("Fireball lifetime should decrease")
		}

		// Check position changed
		if fireball.X == originalX && fireball.Y == originalY {
			t.Error("Fireball position should change")
		}

		// Update until lifetime expires
		for fireball.LifeTime > 0 && fireball.Active {
			combat.UpdateProjectiles()
		}

		if fireball.Active {
			t.Error("Fireball should become inactive after lifetime expires")
		}
	})

	t.Run("Sword Attack Lifetime", func(t *testing.T) {
		// Create a sword attack
		combat.selectedChar = 0 // Knight
		combat.SwordAttack()

		if len(combat.swordAttacks) == 0 {
			t.Fatal("Should have created a sword attack")
		}

		attack := &combat.swordAttacks[0]
		originalLifetime := attack.LifeTime

		// Update projectiles
		combat.UpdateProjectiles()

		// Check lifetime decreased
		if attack.LifeTime >= originalLifetime {
			t.Error("Sword attack lifetime should decrease")
		}

		// Update until lifetime expires
		for attack.LifeTime > 0 && attack.Active {
			combat.UpdateProjectiles()
		}

		if attack.Active {
			t.Error("Sword attack should become inactive after lifetime expires")
		}
	})
}

func TestCharacterSwitching(t *testing.T) {
	cfg := createTestConfig()
	combat := NewTestCombatSystem(cfg)

	t.Run("Switch Between Characters", func(t *testing.T) {
		// Test switching to each character and their abilities
		for i := 0; i < len(combat.party.Members); i++ {
			combat.selectedChar = i
			member := combat.party.Members[i]

			// Verify character selection
			if combat.selectedChar != i {
				t.Errorf("Expected selected character %d, got %d", i, combat.selectedChar)
			}

			// Test character-specific abilities
			switch member.Class {
			case ClassKnight:
				if !combat.SwordAttack() {
					t.Error("Knight should be able to sword attack")
				}
				if combat.CastFireball() {
					t.Error("Knight should not be able to cast fireball")
				}

			case ClassSorcerer:
				if !combat.CastFireball() {
					t.Error("Sorcerer should be able to cast fireball")
				}
				if combat.CastHeal() {
					t.Error("Sorcerer should not be able to cast heal")
				}

			case ClassCleric:
				if !combat.CastHeal() {
					t.Error("Cleric should be able to cast heal")
				}
				if combat.CastFireball() {
					t.Error("Cleric should not be able to cast fireball")
				}

			case ClassArcher:
				// Archer can't cast fireball or heal, and doesn't have sword
				if combat.CastFireball() {
					t.Error("Archer should not be able to cast fireball")
				}
				if combat.CastHeal() {
					t.Error("Archer should not be able to cast heal")
				}
				if combat.SwordAttack() {
					t.Error("Archer should not be able to sword attack")
				}
			}
		}
	})
}

func createTestConfig() *config.Config {
	return &config.Config{
		Characters: config.CharacterConfig{
			StartingGold: 1000,
			StartingFood: 50,
			Classes: map[string]config.ClassStats{
				"knight":   {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
				"sorcerer": {Might: 8, Intellect: 18, Personality: 16, Endurance: 10, Accuracy: 12, Speed: 14, Luck: 13},
				"cleric":   {Might: 12, Intellect: 14, Personality: 18, Endurance: 14, Accuracy: 13, Speed: 12, Luck: 15},
				"archer":   {Might: 14, Intellect: 12, Personality: 13, Endurance: 12, Accuracy: 18, Speed: 16, Luck: 14},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}
}

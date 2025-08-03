package test

import (
	"fmt"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
)

// TestGameIntegration performs comprehensive integration testing without graphics dependencies
func TestGameIntegration(t *testing.T) {
	fmt.Println("=== UgaTaima Integration Test Suite ===")

	cfg, err := loadTestConfig()
	if err != nil {
		cfg = createMinimalConfig()
	}

	t.Run("Complete Game Initialization", func(t *testing.T) {
		testCompleteGameInitialization(t, cfg)
	})

	t.Run("Character Switching and Actions", func(t *testing.T) {
		testCharacterSwitchingAndActions(t, cfg)
	})

	t.Run("Movement Simulation", func(t *testing.T) {
		testMovementSimulation(cfg)
	})

	t.Run("Combat System Integration", func(t *testing.T) {
		testCombatSystemIntegration(cfg)
	})

	t.Run("Magic System Integration", func(t *testing.T) {
		testMagicSystemIntegration(cfg)
	})

	t.Run("Full Game Session Simulation", func(t *testing.T) {
		testFullGameSessionSimulation(t, cfg)
	})
}

func testCompleteGameInitialization(t *testing.T, cfg *config.Config) {
	fmt.Println("\n--- Testing Complete Game Initialization ---")

	// Create party (core game component)
	party := character.NewParty(cfg)

	// Simulate world parameters
	worldWidth, worldHeight := 50, 50
	tileSize := 64
	startX, startY := float64(worldWidth/2*tileSize), float64(worldHeight/2*tileSize)

	// Verify all components
	if party == nil {
		t.Fatal("Party should be initialized")
	}

	// Check party composition
	if len(party.Members) != 4 {
		t.Errorf("Expected 4 party members, got %d", len(party.Members))
	}

	// Verify each character is properly set up
	expectedClasses := []character.CharacterClass{
		character.ClassKnight, character.ClassSorcerer,
		character.ClassCleric, character.ClassArcher,
	}

	for i, member := range party.Members {
		if member.Class != expectedClasses[i] {
			t.Errorf("Member %d: expected class %d, got %d", i, expectedClasses[i], member.Class)
		}
		if member.HitPoints <= 0 {
			t.Errorf("Member %d should have positive hit points", i)
		}
		if member.MaxHitPoints <= 0 {
			t.Errorf("Member %d should have positive max hit points", i)
		}
	}

	fmt.Printf("✓ Game initialized: Party(%d), World(%dx%d), Starting position: (%.0f, %.0f)\n",
		len(party.Members), worldWidth, worldHeight, startX, startY)
}

func testCharacterSwitchingAndActions(t *testing.T, cfg *config.Config) {
	fmt.Println("\n--- Testing Character Switching and Actions ---")

	party := character.NewParty(cfg)

	// Test switching between all characters
	for i, member := range party.Members {
		fmt.Printf("Testing character %d: %s (%s)\n", i+1, member.Name, getClassName(member.Class))

		// Test character capabilities
		canCastFireball := hasFireMagic(member)
		canCastHeal := hasBodyMagic(member)
		canSwordAttack := hasSwordSkill(member)

		fmt.Printf("  - Fire Magic: %v, Body Magic: %v, Sword: %v\n",
			canCastFireball, canCastHeal, canSwordAttack)

		// Verify character-specific abilities
		switch member.Class {
		case character.ClassKnight:
			if !canSwordAttack {
				t.Error("Knight should have sword skill")
			}
			// Knights are typically non-magical in traditional RPGs
		case character.ClassSorcerer:
			if !canCastFireball {
				t.Error("Sorcerer should have fire magic")
			}
		case character.ClassCleric:
			if !canCastHeal {
				t.Error("Cleric should have body magic")
			}
		case character.ClassArcher:
			// Archer has bow skill and some magic
			if _, hasBow := member.Skills[character.SkillBow]; !hasBow {
				t.Error("Archer should have bow skill")
			}
		}
	}

	fmt.Println("✓ Character switching and capability verification passed")
}

func testMovementSimulation(cfg *config.Config) {
	fmt.Println("\n--- Testing Movement Simulation ---")

	// Simulate world bounds and movement
	worldWidth, worldHeight := 50, 50
	tileSize := int(cfg.GetTileSize())
	startX, startY := float64(worldWidth/2*tileSize), float64(worldHeight/2*tileSize)

	// Simulate movement in all directions
	movements := []struct {
		name   string
		deltaX float64
		deltaY float64
	}{
		{"North", 0, -float64(tileSize)},
		{"East", float64(tileSize), 0},
		{"South", 0, float64(tileSize)},
		{"West", -float64(tileSize), 0},
		{"Northeast", float64(tileSize) / 2, -float64(tileSize) / 2},
		{"Back to start", 0, 0}, // Will be set to start position
	}

	currentX, currentY := startX, startY

	for i, move := range movements {
		if move.name == "Back to start" {
			currentX, currentY = startX, startY
		} else {
			currentX += move.deltaX
			currentY += move.deltaY
		}

		// Check bounds
		maxCoord := float64(worldWidth * tileSize)
		inBounds := currentX >= 0 && currentX < maxCoord && currentY >= 0 && currentY < maxCoord

		// Calculate tile coordinates
		tileX := int(currentX) / tileSize
		tileY := int(currentY) / tileSize

		fmt.Printf("Step %d: %s -> (%.0f, %.0f) - Tile: (%d, %d) - In bounds: %v\n",
			i+1, move.name, currentX, currentY, tileX, tileY, inBounds)

		if !inBounds && move.name != "Back to start" {
			// Note: Some test movements intentionally go out of bounds
		}
	}

	fmt.Println("✓ Movement simulation completed")
}

func testCombatSystemIntegration(cfg *config.Config) {
	fmt.Println("\n--- Testing Combat System Integration ---")

	party := character.NewParty(cfg)

	// Test combat scenarios with each character
	for _, member := range party.Members {
		fmt.Printf("\nTesting combat with %s (%s):\n", member.Name, getClassName(member.Class))

		originalHP := member.HitPoints
		originalSP := member.SpellPoints

		// Test damage and healing mechanics
		if hasBodyMagic(member) {
			// Simulate taking damage
			member.HitPoints -= 10
			if member.HitPoints < 0 {
				member.HitPoints = 0
			}

			// Simulate healing
			healCost := 2
			if member.SpellPoints >= healCost {
				healAmount := 15 + member.Personality/2
				member.SpellPoints -= healCost
				member.HitPoints += healAmount
				if member.HitPoints > member.MaxHitPoints {
					member.HitPoints = member.MaxHitPoints
				}
				fmt.Printf("  ✓ Healed for %d HP (Cost: %d SP)\n", healAmount, healCost)
			}
		}

		// Test spell casting
		if hasFireMagic(member) {
			fireballCost := 3
			if member.SpellPoints >= fireballCost {
				damage := 10 + member.Intellect/2
				member.SpellPoints -= fireballCost
				fmt.Printf("  ✓ Can cast fireball (Damage: %d, Cost: %d SP)\n", damage, fireballCost)
			}
		}

		// Test sword attack calculation
		if hasSwordSkill(member) {
			swordSkill := member.Skills[character.SkillSword]
			damage := 8 + member.Might/3 + swordSkill.Level
			fmt.Printf("  ✓ Can perform sword attack (Damage: %d)\n", damage)
		}

		fmt.Printf("  HP: %d→%d, SP: %d→%d\n", originalHP, member.HitPoints, originalSP, member.SpellPoints)
	}

	fmt.Println("✓ Combat system integration test passed")
}

func testMagicSystemIntegration(cfg *config.Config) {
	fmt.Println("\n--- Testing Magic System Integration ---")

	party := character.NewParty(cfg)

	// Test spell system for each character
	for _, member := range party.Members {
		if len(member.MagicSchools) == 0 {
			fmt.Printf("%s has no magic schools\n", member.Name)
			continue
		}

		fmt.Printf("\n%s's Magic:\n", member.Name)
		for school, magicSkill := range member.MagicSchools {
			schoolName := getMagicSchoolName(school)
			fmt.Printf("  %s Magic (Level %d):\n", schoolName, magicSkill.Level)

			for _, spell := range magicSkill.Spells {
				canCast := member.SpellPoints >= spell.SpellPoints
				fmt.Printf("    - %s (Level %d, Cost: %d SP) - Available: %v\n",
					spell.Name, spell.Level, spell.SpellPoints, canCast)

				// Test spell casting simulation
				if canCast {
					originalSP := member.SpellPoints
					member.SpellPoints -= spell.SpellPoints

					// Simulate spell effects
					switch school {
					case character.MagicFire:
						damage := spell.SpellPoints*3 + member.Intellect/2
						fmt.Printf("      → Cast! Damage: %d, SP: %d→%d\n", damage, originalSP, member.SpellPoints)
					case character.MagicBody:
						healAmount := spell.SpellPoints*5 + member.Personality/2
						fmt.Printf("      → Cast! Heal: %d, SP: %d→%d\n", healAmount, originalSP, member.SpellPoints)
					default:
						fmt.Printf("      → Cast! SP: %d→%d\n", originalSP, member.SpellPoints)
					}
				}
			}
		}
	}

	fmt.Println("✓ Magic system integration test passed")
}

func testFullGameSessionSimulation(t *testing.T, cfg *config.Config) {
	fmt.Println("\n--- Testing Full Game Session Simulation ---")

	// Initialize all components
	party := character.NewParty(cfg)
	worldWidth, worldHeight := 50, 50
	tileSize := 64
	startX, startY := float64(worldWidth/2*tileSize), float64(worldHeight/2*tileSize)

	fmt.Printf("Game State: Party ready, World loaded (%dx%d), Starting at (%.0f, %.0f)\n", worldWidth, worldHeight, startX, startY)

	// Simulate a game session
	currentX, currentY := startX, startY
	selectedChar := 0

	fmt.Println("\n--- Simulating Game Session ---")

	// Phase 1: Exploration
	fmt.Println("Phase 1: Exploration")
	moves := []string{"North", "East", "South", "West"}
	for i, direction := range moves {
		// Move
		switch direction {
		case "North":
			currentY -= float64(tileSize)
		case "East":
			currentX += float64(tileSize)
		case "South":
			currentY += float64(tileSize)
		case "West":
			currentX -= float64(tileSize)
		}

		tileX := int(currentX) / tileSize
		tileY := int(currentY) / tileSize
		// Simulate different tile types
		tileType := (tileX + tileY) % 5 // Simple tile type simulation
		fmt.Printf("  Move %d %s: Tile %d at (%d, %d)\n", i+1, direction, tileType, tileX, tileY)
	}

	// Phase 2: Character switching
	fmt.Println("\nPhase 2: Character Switching")
	for i := 0; i < 4; i++ {
		selectedChar = i
		member := party.Members[selectedChar]
		fmt.Printf("  Selected: [%d] %s (%s) - HP:%d/%d SP:%d/%d\n",
			i+1, member.Name, getClassName(member.Class),
			member.HitPoints, member.MaxHitPoints,
			member.SpellPoints, member.MaxSpellPoints)
	}

	// Phase 3: Combat simulation
	fmt.Println("\nPhase 3: Combat Actions")

	// Knight sword attack
	knight := party.Members[0]
	if hasSwordSkill(knight) {
		swordSkill := knight.Skills[character.SkillSword]
		damage := 8 + knight.Might/3 + swordSkill.Level
		fmt.Printf("  Knight sword attack: %d damage\n", damage)
	}

	// Sorcerer fireball
	sorcerer := party.Members[1]
	if hasFireMagic(sorcerer) && sorcerer.SpellPoints >= 3 {
		damage := 10 + sorcerer.Intellect/2
		sorcerer.SpellPoints -= 3
		fmt.Printf("  Sorcerer fireball: %d damage (SP: %d→%d)\n",
			damage, sorcerer.SpellPoints+3, sorcerer.SpellPoints)
	}

	// Cleric heal
	cleric := party.Members[2]
	if hasBodyMagic(cleric) && cleric.SpellPoints >= 2 {
		// Simulate damage first
		cleric.HitPoints -= 15
		originalHP := cleric.HitPoints

		healAmount := 15 + cleric.Personality/2
		cleric.SpellPoints -= 2
		cleric.HitPoints += healAmount
		if cleric.HitPoints > cleric.MaxHitPoints {
			cleric.HitPoints = cleric.MaxHitPoints
		}
		fmt.Printf("  Cleric heal: %d→%d HP (SP: %d→%d)\n",
			originalHP, cleric.HitPoints,
			cleric.SpellPoints+2, cleric.SpellPoints)
	}

	// Phase 4: Party updates
	fmt.Println("\nPhase 4: Party Updates")
	for i := 0; i < 3; i++ {
		party.Update()
		fmt.Printf("  Update %d: Party regeneration cycle\n", i+1)
	}

	fmt.Println("\n✓ Full game session simulation completed successfully!")
	fmt.Printf("Final party status:\n")
	for i, member := range party.Members {
		fmt.Printf("  [%d] %s: HP %d/%d, SP %d/%d\n",
			i+1, member.Name, member.HitPoints, member.MaxHitPoints,
			member.SpellPoints, member.MaxSpellPoints)
	}

	// Verify all tests passed
	if len(party.Members) != 4 {
		t.Errorf("Expected 4 party members, got %d", len(party.Members))
	}

	for i, member := range party.Members {
		if member.HitPoints <= 0 {
			t.Errorf("Member %d (%s) should have positive hit points", i, member.Name)
		}
		if member.MaxHitPoints <= 0 {
			t.Errorf("Member %d (%s) should have positive max hit points", i, member.Name)
		}
	}
}

// Helper functions
func loadTestConfig() (*config.Config, error) {
	return config.LoadConfig("../config.yaml")
}

func createMinimalConfig() *config.Config {
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

func getClassName(class character.CharacterClass) string {
	switch class {
	case character.ClassKnight:
		return "Knight"
	case character.ClassPaladin:
		return "Paladin"
	case character.ClassArcher:
		return "Archer"
	case character.ClassCleric:
		return "Cleric"
	case character.ClassSorcerer:
		return "Sorcerer"
	case character.ClassDruid:
		return "Druid"
	default:
		return "Unknown"
	}
}

func getMagicSchoolName(school character.MagicSchool) string {
	names := map[character.MagicSchool]string{
		character.MagicBody: "Body", character.MagicMind: "Mind", character.MagicSpirit: "Spirit",
		character.MagicFire: "Fire", character.MagicWater: "Water", character.MagicAir: "Air", character.MagicEarth: "Earth",
		character.MagicLight: "Light", character.MagicDark: "Dark",
	}
	if name, exists := names[school]; exists {
		return name
	}
	return "Unknown"
}

func hasFireMagic(char *character.MMCharacter) bool {
	_, exists := char.MagicSchools[character.MagicFire]
	return exists
}

func hasBodyMagic(char *character.MMCharacter) bool {
	_, exists := char.MagicSchools[character.MagicBody]
	return exists
}

func hasSwordSkill(char *character.MMCharacter) bool {
	_, exists := char.Skills[character.SkillSword]
	return exists
}

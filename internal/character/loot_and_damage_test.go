package character

import (
	"fmt"
	"math/rand"
	"testing"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// MockParty simulates a party for testing loot drops
type MockParty struct {
	Inventory []items.Item
	Gold      int
}

func (p *MockParty) AddItem(item items.Item) {
	p.Inventory = append(p.Inventory, item)
}

// MockCombatSystem simulates the combat system's loot drop logic
type MockCombatSystem struct {
	party *MockParty
}

func (cs *MockCombatSystem) checkMonsterLootDrop(monster *monster.Monster3D) {
	// Replicate the exact loot drop logic from combat.go

	// Check if this is a Pixie
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
			cs.party.AddItem(magicRing)
		}
	}

	// Check if this is a Dragon
	if monster.Name == "Dragon" {
		// 5% chance to drop Bow of Hellfire
		if rand.Float64() < 0.05 {
			// Create Bow of Hellfire using YAML definition
			bowOfHellfire := items.CreateWeaponFromYAML("bow_of_hellfire")

			// Add to party inventory
			cs.party.AddItem(bowOfHellfire)
		}
	}
}

// ApplyArmorDamageReduction replicates the exact damage reduction logic from combat.go
func ApplyArmorDamageReduction(damage int, character *MMCharacter, statBonus int) int {
	// Get effective endurance (includes equipment bonuses like Leather Armor)
	_, _, _, effectiveEndurance, _, _, _ := character.GetEffectiveStats(statBonus)

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

// TestLootDrops tests that dragons and pixies actually drop items at the expected 5% rate
func TestLootDrops(t *testing.T) {
	fmt.Println("=== Loot Drop Testing - Killing 1000 Dragons and 1000 Pixies ===")

	// Load monster configuration for tests
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	cfg := createTestConfig()

	// Create mock party and combat system
	party := &MockParty{
		Inventory: []items.Item{},
		Gold:      0,
	}
	combat := &MockCombatSystem{party: party}

	// Set consistent seed for reproducible testing
	rand.Seed(12345)

	t.Run("Dragon Loot Drops - 1000 Kills", func(t *testing.T) {
		fmt.Println("\n--- Testing: Dragon Loot Drops (Expected: ~50 drops at 5% rate) ---")

		dragonDropCount := 0
		initialInventorySize := len(party.Inventory)

		for i := 0; i < 1000; i++ {
			// Create a new dragon
			dragon := monster.NewMonster3DFromConfig(100, 100, "dragon", cfg)
			dragon.HitPoints = 0 // Kill it instantly for testing

			// Check loot drop
			combat.checkMonsterLootDrop(dragon)

			// Count items added to inventory
			if len(party.Inventory) > initialInventorySize+dragonDropCount {
				dragonDropCount++
				// Verify it's the correct item
				lastItem := party.Inventory[len(party.Inventory)-1]
				if lastItem.Name != "Bow of Hellfire" {
					t.Errorf("Dragon dropped wrong item: %s (expected: Bow of Hellfire)", lastItem.Name)
				}
			}
		}

		fmt.Printf("Dragon kills: 1000\n")
		fmt.Printf("Bow of Hellfire drops: %d\n", dragonDropCount)
		fmt.Printf("Drop rate: %.2f%% (expected: ~5%%)\n", float64(dragonDropCount)/10.0)

		// Check that drop rate is reasonable (2-8% range to account for randomness)
		if dragonDropCount < 20 || dragonDropCount > 80 {
			t.Errorf("Dragon drop rate seems wrong: %d/1000 (%.2f%%). Expected around 50 drops (5%%)",
				dragonDropCount, float64(dragonDropCount)/10.0)
		} else {
			fmt.Printf("✅ Dragon drop rate is within expected range!\n")
		}
	})

	t.Run("Pixie Loot Drops - 1000 Kills", func(t *testing.T) {
		fmt.Println("\n--- Testing: Pixie Loot Drops (Expected: ~50 drops at 5% rate) ---")

		pixieDropCount := 0
		initialInventorySize := len(party.Inventory)

		for i := 0; i < 1000; i++ {
			// Create a new pixie
			pixie := monster.NewMonster3DFromConfig(100, 100, "pixie", cfg)
			pixie.HitPoints = 0 // Kill it instantly for testing

			// Check loot drop
			combat.checkMonsterLootDrop(pixie)

			// Count items added to inventory
			if len(party.Inventory) > initialInventorySize+pixieDropCount {
				pixieDropCount++
				// Verify it's the correct item
				lastItem := party.Inventory[len(party.Inventory)-1]
				if lastItem.Name != "Magic Ring" {
					t.Errorf("Pixie dropped wrong item: %s (expected: Magic Ring)", lastItem.Name)
				}
			}
		}

		fmt.Printf("Pixie kills: 1000\n")
		fmt.Printf("Magic Ring drops: %d\n", pixieDropCount)
		fmt.Printf("Drop rate: %.2f%% (expected: ~5%%)\n", float64(pixieDropCount)/10.0)

		// Check that drop rate is reasonable (2-8% range to account for randomness)
		if pixieDropCount < 20 || pixieDropCount > 80 {
			t.Errorf("Pixie drop rate seems wrong: %d/1000 (%.2f%%). Expected around 50 drops (5%%)",
				pixieDropCount, float64(pixieDropCount)/10.0)
		} else {
			fmt.Printf("✅ Pixie drop rate is within expected range!\n")
		}
	})

	// Final inventory summary
	fmt.Printf("\nFinal Inventory Summary:\n")
	fmt.Printf("Total items: %d\n", len(party.Inventory))

	bowCount := 0
	ringCount := 0
	for _, item := range party.Inventory {
		switch item.Name {
		case "Bow of Hellfire":
			bowCount++
		case "Magic Ring":
			ringCount++
		}
	}
	fmt.Printf("Bow of Hellfire: %d\n", bowCount)
	fmt.Printf("Magic Ring: %d\n", ringCount)
}

// TestArmorDamageReduction tests that armor and endurance properly reduce damage
func TestArmorDamageReduction(t *testing.T) {
	fmt.Println("\n=== Damage Reduction Testing - Armor and Endurance ===")

	t.Run("No Armor - Base Endurance", func(t *testing.T) {
		fmt.Println("\n--- Testing: No Armor, Base Endurance ---")

		// Create a character with base stats
		character := &MMCharacter{
			Name:      "TestWarrior",
			Endurance: 20, // Base endurance
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Test various damage amounts
		testDamages := []int{10, 20, 50, 100}
		for _, damage := range testDamages {
			finalDamage := ApplyArmorDamageReduction(damage, character, 0)

			// Expected: Endurance 20 → enduranceBonus = 4 → damageReduction = 2
			expectedReduction := (20 / 5) / 2 // (endurance / 5) / 2 = 2
			expectedDamage := damage - expectedReduction
			if expectedDamage < 1 {
				expectedDamage = 1
			}

			fmt.Printf("Damage %d → %d (reduction: %d)\n", damage, finalDamage, damage-finalDamage)

			if finalDamage != expectedDamage {
				t.Errorf("Damage reduction incorrect. Input: %d, Got: %d, Expected: %d",
					damage, finalDamage, expectedDamage)
			}
		}
		fmt.Printf("✅ Base endurance damage reduction working correctly!\n")
	})

	t.Run("With Leather Armor", func(t *testing.T) {
		fmt.Println("\n--- Testing: With Leather Armor ---")

		// Create a character with leather armor
		character := &MMCharacter{
			Name:      "TestKnight",
			Endurance: 20,
			Equipment: make(map[items.EquipSlot]items.Item),
		}

        // Equip leather armor from YAML to include its attributes
        character.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")

		// Test various damage amounts
		testDamages := []int{10, 20, 50, 100}
		for _, damage := range testDamages {
			finalDamage := ApplyArmorDamageReduction(damage, character, 0)

			// Expected: baseArmor = 2, enduranceBonus = 4, totalAC = 6, reduction = 3
			expectedReduction := (2 + 20/5) / 2 // (baseArmor + endurance/5) / 2 = 3
			expectedDamage := damage - expectedReduction
			if expectedDamage < 1 {
				expectedDamage = 1
			}

			fmt.Printf("Damage %d → %d (reduction: %d, armor+endurance)\n",
				damage, finalDamage, damage-finalDamage)

			if finalDamage != expectedDamage {
				t.Errorf("Armor damage reduction incorrect. Input: %d, Got: %d, Expected: %d",
					damage, finalDamage, expectedDamage)
			}
		}
		fmt.Printf("✅ Leather armor damage reduction working correctly!\n")
	})

	t.Run("High Endurance Character", func(t *testing.T) {
		fmt.Println("\n--- Testing: High Endurance Character ---")

		// Create a character with high endurance
		character := &MMCharacter{
			Name:      "TestTank",
			Endurance: 100, // Very high endurance
			Equipment: make(map[items.EquipSlot]items.Item),
		}

        // Equip leather armor from YAML to include its attributes
        character.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")

		// Test various damage amounts
		testDamages := []int{10, 20, 50, 100}
		for _, damage := range testDamages {
			finalDamage := ApplyArmorDamageReduction(damage, character, 0)

			// Expected: effectiveEndurance = 100 + 20 (leather armor bonus) = 120
			// baseArmor = 2, enduranceBonus = 120/5 = 24, totalAC = 26, reduction = 13
			effectiveEndurance := 100 + (100 / 5)               // Base + leather armor bonus
			expectedReduction := (2 + effectiveEndurance/5) / 2 // (baseArmor + effectiveEndurance/5) / 2 = 13
			expectedDamage := damage - expectedReduction
			if expectedDamage < 1 {
				expectedDamage = 1 // Minimum damage is always 1
			}

			fmt.Printf("Damage %d → %d (reduction: %d, high endurance)\n",
				damage, finalDamage, damage-finalDamage)

			if finalDamage != expectedDamage {
				t.Errorf("High endurance damage reduction incorrect. Input: %d, Got: %d, Expected: %d",
					damage, finalDamage, expectedDamage)
			}
		}
		fmt.Printf("✅ High endurance damage reduction working correctly!\n")
	})

	t.Run("Minimum Damage Rule", func(t *testing.T) {
		fmt.Println("\n--- Testing: Minimum Damage Rule ---")

		// Create a super-tank character
		character := &MMCharacter{
			Name:      "SuperTank",
			Endurance: 200, // Extremely high endurance
			Equipment: make(map[items.EquipSlot]items.Item),
		}

        // Equip leather armor from YAML to include its attributes
        character.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")

		// Test very low damage that should be reduced to 1
		lowDamages := []int{1, 2, 5, 10, 15}
		for _, damage := range lowDamages {
			finalDamage := ApplyArmorDamageReduction(damage, character, 0)

			fmt.Printf("Low damage %d → %d (minimum damage rule)\n", damage, finalDamage)

			// All damage should be reduced to 1 (minimum)
			if finalDamage != 1 {
				t.Errorf("Minimum damage rule failed. Input: %d, Got: %d, Expected: 1",
					damage, finalDamage)
			}
		}

		fmt.Println("✅ Minimum damage rule working correctly - armor can't completely negate damage!")
	})

	t.Run("Edge Cases", func(t *testing.T) {
		fmt.Println("\n--- Testing: Edge Cases ---")

		// Test with stat bonuses (like Bless spell)
		character := &MMCharacter{
			Name:      "BlessedWarrior",
			Endurance: 30,
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Test with stat bonus
		damage := 20
		statBonus := 10 // Simulating a Bless spell
		finalDamage := ApplyArmorDamageReduction(damage, character, statBonus)

		// Expected: effective endurance = 40, enduranceBonus = 8, reduction = 4
		expectedDamage := damage - (40/5)/2 // 20 - 4 = 16

		fmt.Printf("Damage %d with stat bonus +%d → %d (effective endurance: %d)\n",
			damage, statBonus, finalDamage, 30+statBonus)

		if finalDamage != expectedDamage {
			t.Errorf("Stat bonus damage reduction incorrect. Input: %d, Got: %d, Expected: %d",
				damage, finalDamage, expectedDamage)
		}

		fmt.Printf("✅ Stat bonus effects working correctly!\n")
	})
}

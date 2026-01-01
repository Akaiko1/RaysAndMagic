package character

import (
	"fmt"
	"math/rand"
	"testing"
	"ugataima/internal/config"
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
		// 5% chance to drop Wizard Robes
		if rand.Float64() < 0.05 {
			wizardRobes := items.Item{
				Name:          "Wizard Robes",
				Type:          items.ItemArmor,
				ArmorCategory: "cloth",
				Description:   "Silken robes embroidered with arcane runes",
				Attributes: map[string]int{
					"armor_class_base":  1,
					"bonus_intellect":   10,
					"bonus_personality": 10,
					"equip_slot":        int(items.SlotArmor),
				},
			}
			cs.party.AddItem(wizardRobes)
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
	// Get effective endurance (includes equipment bonuses)
	effectiveEndurance := character.GetEffectiveEndurance(statBonus)

	// Calculate armor class from all armor slots (matches updated combat.go)
	baseArmor := 0
	totalEnduranceBonus := 0

	armorSlots := []items.EquipSlot{
		items.SlotArmor,
		items.SlotHelmet,
		items.SlotBoots,
		items.SlotCloak,
		items.SlotGauntlets,
		items.SlotBelt,
	}

	for _, slot := range armorSlots {
		if armorPiece, hasArmor := character.Equipment[slot]; hasArmor {
			if v, ok := armorPiece.Attributes["armor_class_base"]; ok {
				baseArmor += v
			}
			if enduranceDiv, ok := armorPiece.Attributes["endurance_scaling_divisor"]; ok && enduranceDiv > 0 {
				totalEnduranceBonus += effectiveEndurance / enduranceDiv
			}
		}
	}

	totalArmorClass := baseArmor + totalEnduranceBonus

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
		fmt.Println("\n--- Testing: Pixie Loot Drops (Expected: ~50 drops each at 5% rate) ---")

		magicRingDrops := 0
		wizardRobeDrops := 0
		prevInventorySize := len(party.Inventory)

		for i := 0; i < 1000; i++ {
			// Create a new pixie
			pixie := monster.NewMonster3DFromConfig(100, 100, "pixie", cfg)
			pixie.HitPoints = 0 // Kill it instantly for testing

			// Check loot drop
			combat.checkMonsterLootDrop(pixie)

			// Count items added to inventory (pixie can drop multiple items)
			if len(party.Inventory) > prevInventorySize {
				for _, item := range party.Inventory[prevInventorySize:] {
					switch item.Name {
					case "Magic Ring":
						magicRingDrops++
					case "Wizard Robes":
						wizardRobeDrops++
					default:
						t.Errorf("Pixie dropped wrong item: %s (expected: Magic Ring or Wizard Robes)", item.Name)
					}
				}
				prevInventorySize = len(party.Inventory)
			}
		}

		fmt.Printf("Pixie kills: 1000\n")
		fmt.Printf("Magic Ring drops: %d\n", magicRingDrops)
		fmt.Printf("Wizard Robes drops: %d\n", wizardRobeDrops)
		fmt.Printf("Magic Ring drop rate: %.2f%% (expected: ~5%%)\n", float64(magicRingDrops)/10.0)
		fmt.Printf("Wizard Robes drop rate: %.2f%% (expected: ~5%%)\n", float64(wizardRobeDrops)/10.0)

		// Check that drop rate is reasonable (2-8% range to account for randomness)
		if magicRingDrops < 20 || magicRingDrops > 80 {
			t.Errorf("Magic Ring drop rate seems wrong: %d/1000 (%.2f%%). Expected around 50 drops (5%%)",
				magicRingDrops, float64(magicRingDrops)/10.0)
		} else {
			fmt.Printf("✅ Magic Ring drop rate is within expected range!\n")
		}
		if wizardRobeDrops < 20 || wizardRobeDrops > 80 {
			t.Errorf("Wizard Robes drop rate seems wrong: %d/1000 (%.2f%%). Expected around 50 drops (5%%)",
				wizardRobeDrops, float64(wizardRobeDrops)/10.0)
		} else {
			fmt.Printf("✅ Wizard Robes drop rate is within expected range!\n")
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

			// Expected: No armor equipped = no damage reduction, all damage goes through
			expectedDamage := damage

			fmt.Printf("Damage %d → %d (reduction: %d)\n", damage, finalDamage, damage-finalDamage)

			if finalDamage != expectedDamage {
				t.Errorf("Damage reduction incorrect. Input: %d, Got: %d, Expected: %d",
					damage, finalDamage, expectedDamage)
			}
		}
		fmt.Printf("✅ Base endurance (no armor) working correctly!\n")
	})

	t.Run("With Leather Armor", func(t *testing.T) {
		fmt.Println("\n--- Testing: With Leather Armor ---")

		// Create a character with leather armor
		character := &MMCharacter{
			Name:      "TestKnight",
			Endurance: 20,
			Skills: map[SkillType]*Skill{
				SkillLeather: {Level: 1, Mastery: MasteryNovice},
				SkillPlate:   {Level: 1, Mastery: MasteryNovice},
			},
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

		// Test with stat bonus but no armor
		damage := 20
		statBonus := 10 // Simulating a Bless spell
		finalDamage := ApplyArmorDamageReduction(damage, character, statBonus)

		// Expected: No armor = no damage reduction regardless of endurance
		expectedDamage := damage

		fmt.Printf("Damage %d with stat bonus +%d → %d (no armor equipped)\n",
			damage, statBonus, finalDamage)

		if finalDamage != expectedDamage {
			t.Errorf("Stat bonus damage reduction incorrect. Input: %d, Got: %d, Expected: %d",
				damage, finalDamage, expectedDamage)
		}

		fmt.Printf("✅ Stat bonus effects working correctly!\n")
	})
}

// TestEquipmentSlots tests the new YAML-driven equipment slot system
func TestEquipmentSlots(t *testing.T) {
	fmt.Println("\n=== Equipment Slot System Testing - YAML-Driven Slots ===")

	// Setup test environment with required accessors
	setupTestAccessors()

	t.Run("Equipment Slot Assignment", func(t *testing.T) {
		fmt.Println("\n--- Testing: Equipment Slot Assignment ---")

		character := &MMCharacter{
			Name:      "TestKnight",
			Endurance: 20,
			Skills: map[SkillType]*Skill{
				SkillLeather: {Level: 1, Mastery: MasteryNovice},
				SkillPlate:   {Level: 1, Mastery: MasteryNovice},
			},
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Test different item types go to correct slots
		testCases := []struct {
			itemKey      string
			expectedSlot items.EquipSlot
			description  string
		}{
			{"leather_armor", items.SlotArmor, "Leather Armor → Armor Slot"},
			{"leather_helmet", items.SlotHelmet, "Leather Helmet → Helmet Slot"},
			{"leather_pants", items.SlotBoots, "Leather Pants → Boots Slot"},
			{"magic_ring", items.SlotRing1, "Magic Ring → Ring Slot"},
			{"iron_armor", items.SlotArmor, "Iron Armor → Armor Slot"},
		}

		for _, tc := range testCases {
			// Create item from YAML
			item := items.CreateItemFromYAML(tc.itemKey)

			// Equip the item
			_, _, success := character.EquipItem(item)

			if !success {
				t.Errorf("Failed to equip %s", item.Name)
				continue
			}

			// Verify it went to the correct slot
			equippedItem, exists := character.Equipment[tc.expectedSlot]
			if !exists {
				t.Errorf("%s: Item not found in expected slot %d", tc.description, tc.expectedSlot)
				continue
			}

			if equippedItem.Name != item.Name {
				t.Errorf("%s: Wrong item in slot. Expected: %s, Got: %s",
					tc.description, item.Name, equippedItem.Name)
				continue
			}

			fmt.Printf("✅ %s\n", tc.description)

			// Clean up for next test
			character.UnequipItem(tc.expectedSlot)
		}
	})

	t.Run("Multiple Armor Pieces - Cumulative Armor Class", func(t *testing.T) {
		fmt.Println("\n--- Testing: Multiple Armor Pieces ---")

		character := &MMCharacter{
			Name:      "TestKnight",
			Endurance: 20,
			Skills: map[SkillType]*Skill{
				SkillLeather: {Level: 1, Mastery: MasteryNovice},
			},
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Equip multiple armor pieces
		leatherArmor := items.CreateItemFromYAML("leather_armor")   // AC: 2
		leatherHelmet := items.CreateItemFromYAML("leather_helmet") // AC: 1
		leatherPants := items.CreateItemFromYAML("leather_pants")   // AC: 1

		character.EquipItem(leatherArmor)
		character.EquipItem(leatherHelmet)
		character.EquipItem(leatherPants)

		// Test damage reduction with multiple armor pieces
		damage := 50
		finalDamage := ApplyArmorDamageReduction(damage, character, 0)

		// Calculate expected values step by step
		// Get the character's effective endurance after equipment bonuses
		effectiveEndurance := character.GetEffectiveEndurance(0)

		// Calculate expected armor values
		expectedBaseArmor := 2 + 1 + 1 // leather_armor + leather_helmet + leather_pants
		expectedEnduranceBonus := (effectiveEndurance / 5) + (effectiveEndurance / 6) + (effectiveEndurance / 6)
		expectedTotalAC := expectedBaseArmor + expectedEnduranceBonus
		expectedReduction := expectedTotalAC / 2
		expectedDamage := damage - expectedReduction

		fmt.Printf("Effective endurance: %d\n", effectiveEndurance)
		fmt.Printf("Expected base armor: %d\n", expectedBaseArmor)
		fmt.Printf("Expected endurance bonus: %d\n", expectedEnduranceBonus)

		fmt.Printf("Equipped: Leather Armor (AC:2), Helmet (AC:1), Pants (AC:1)\n")
		fmt.Printf("Expected total AC: %d, Expected reduction: %d\n", expectedTotalAC, expectedReduction)
		fmt.Printf("Damage %d → %d (reduction: %d)\n", damage, finalDamage, damage-finalDamage)

		if finalDamage != expectedDamage {
			t.Errorf("Multiple armor pieces damage reduction incorrect. Expected: %d, Got: %d",
				expectedDamage, finalDamage)
		} else {
			fmt.Printf("✅ Multiple armor pieces working correctly!\n")
		}
	})

	t.Run("Equipment Stat Bonuses", func(t *testing.T) {
		fmt.Println("\n--- Testing: Equipment Stat Bonuses ---")

		character := &MMCharacter{
			Name:        "TestMage",
			Intellect:   30,
			Personality: 25,
			Endurance:   20,
			Equipment:   make(map[items.EquipSlot]items.Item),
		}

		// Equip magic ring for stat bonuses
		magicRing := items.CreateItemFromYAML("magic_ring")
		character.EquipItem(magicRing)

		// Test effective stats calculation
		_, intellect, personality, endurance, _, _, _ := character.GetEffectiveStats(0)

		// Expected bonuses from magic ring:
		// - Intellect: 30/6 = 5
		// - Personality: 25/8 = 3
		expectedIntellect := 30 + (30 / 6)   // 35
		expectedPersonality := 25 + (25 / 8) // 28
		expectedEndurance := 20              // No endurance bonus from ring

		fmt.Printf("Base stats: INT:%d, PER:%d, END:%d\n", 30, 25, 20)
		fmt.Printf("With Magic Ring: INT:%d, PER:%d, END:%d\n", intellect, personality, endurance)
		fmt.Printf("Expected: INT:%d, PER:%d, END:%d\n", expectedIntellect, expectedPersonality, expectedEndurance)

		if intellect != expectedIntellect {
			t.Errorf("Intellect bonus incorrect. Expected: %d, Got: %d", expectedIntellect, intellect)
		}
		if personality != expectedPersonality {
			t.Errorf("Personality bonus incorrect. Expected: %d, Got: %d", expectedPersonality, personality)
		}
		if endurance != expectedEndurance {
			t.Errorf("Endurance should be unchanged. Expected: %d, Got: %d", expectedEndurance, endurance)
		}

		if intellect == expectedIntellect && personality == expectedPersonality && endurance == expectedEndurance {
			fmt.Printf("✅ Equipment stat bonuses working correctly!\n")
		}
	})

	t.Run("Slot Conflicts and Replacement", func(t *testing.T) {
		fmt.Println("\n--- Testing: Slot Conflicts and Replacement ---")

		character := &MMCharacter{
			Name: "TestKnight",
			Skills: map[SkillType]*Skill{
				SkillLeather: {Level: 1, Mastery: MasteryNovice},
				SkillPlate:   {Level: 1, Mastery: MasteryNovice},
			},
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Equip leather armor first
		leatherArmor := items.CreateItemFromYAML("leather_armor")
		previousItem, hadPrevious, success := character.EquipItem(leatherArmor)

		if !success || hadPrevious {
			t.Errorf("Initial armor equip failed or detected false previous item")
		}

		// Now equip iron armor (should replace leather armor)
		ironArmor := items.CreateItemFromYAML("iron_armor")
		previousItem, hadPrevious, success = character.EquipItem(ironArmor)

		if !success {
			t.Errorf("Failed to equip iron armor")
		}
		if !hadPrevious {
			t.Errorf("Should have detected previous armor")
		}
		if previousItem.Name != "Leather Armor" {
			t.Errorf("Wrong previous item returned. Expected: Leather Armor, Got: %s", previousItem.Name)
		}

		// Verify iron armor is now equipped
		equippedArmor, exists := character.Equipment[items.SlotArmor]
		if !exists || equippedArmor.Name != "Iron Armor" {
			t.Errorf("Iron armor not properly equipped")
		}

		fmt.Printf("✅ Equipment replacement working correctly!\n")
	})

	t.Run("Unequip Items", func(t *testing.T) {
		fmt.Println("\n--- Testing: Unequip Items ---")

		character := &MMCharacter{
			Name: "TestKnight",
			Skills: map[SkillType]*Skill{
				SkillLeather: {Level: 1, Mastery: MasteryNovice},
			},
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Equip some items
		helmet := items.CreateItemFromYAML("leather_helmet")
		ring := items.CreateItemFromYAML("magic_ring")

		character.EquipItem(helmet)
		character.EquipItem(ring)

		// Verify items are equipped
		if len(character.Equipment) != 2 {
			t.Errorf("Expected 2 equipped items, got %d", len(character.Equipment))
		}

		// Unequip helmet
		unequippedItem, success := character.UnequipItem(items.SlotHelmet)
		if !success {
			t.Errorf("Failed to unequip helmet")
		}
		if unequippedItem.Name != "Leather Helmet" {
			t.Errorf("Wrong item unequipped. Expected: Leather Helmet, Got: %s", unequippedItem.Name)
		}

		// Verify helmet slot is empty
		if _, exists := character.Equipment[items.SlotHelmet]; exists {
			t.Errorf("Helmet slot should be empty after unequip")
		}

		// Ring should still be equipped
		if _, exists := character.Equipment[items.SlotRing1]; !exists {
			t.Errorf("Ring should still be equipped")
		}

		fmt.Printf("✅ Unequip functionality working correctly!\n")
	})

	t.Run("Invalid Equipment Attempts", func(t *testing.T) {
		fmt.Println("\n--- Testing: Invalid Equipment Attempts ---")

		character := &MMCharacter{
			Name:      "TestKnight",
			Equipment: make(map[items.EquipSlot]items.Item),
		}

		// Try to equip a consumable (should fail)
		healthPotion := items.CreateItemFromYAML("health_potion")
		_, _, success := character.EquipItem(healthPotion)

		if success {
			t.Errorf("Should not be able to equip consumable items")
		} else {
			fmt.Printf("✅ Correctly rejected consumable item!\n")
		}

		// Try to unequip from empty slot
		_, success = character.UnequipItem(items.SlotHelmet)
		if success {
			t.Errorf("Should not be able to unequip from empty slot")
		} else {
			fmt.Printf("✅ Correctly rejected unequip from empty slot!\n")
		}
	})
}

// setupTestAccessors configures the required global accessors for testing
func setupTestAccessors() {
	// Setup item accessor for YAML items
	if items.GlobalItemAccessor == nil {
		// Load item config for tests
		config.MustLoadItemConfig("../../assets/items.yaml")

		// Setup bridge
		items.GlobalItemAccessor = func(itemKey string) (*items.ItemDefinitionFromYAML, bool) {
			def, exists := config.GetItemDefinition(itemKey)
			if !exists || def == nil {
				return nil, false
			}
			adapted := &items.ItemDefinitionFromYAML{
				Name:                      def.Name,
				Description:               def.Description,
				Flavor:                    def.Flavor,
				Type:                      def.Type,
				ArmorType:                 def.ArmorType,
				ArmorClassBase:            def.ArmorClassBase,
				EnduranceScalingDivisor:   def.EnduranceScalingDivisor,
				IntellectScalingDivisor:   def.IntellectScalingDivisor,
				PersonalityScalingDivisor: def.PersonalityScalingDivisor,
				HealBase:                  def.HealBase,
				HealEnduranceDivisor:      def.HealEnduranceDivisor,
				SummonDistanceTiles:       def.SummonDistanceTiles,
				EquipSlot:                 def.EquipSlot,
			}
			return adapted, true
		}
	}
}

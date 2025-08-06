package character

import (
	"fmt"
	"testing"
	"ugataima/internal/monster"
)

// TestMonsterCombat demonstrates actually killing monsters in combat
func TestMonsterCombat(t *testing.T) {
	fmt.Println("=== Monster Combat Test - Can You Kill Mobs? ===")

	// Load monster configuration for tests
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")

	cfg := createTestConfig()
	party := NewParty(cfg)

	t.Run("Kill Monster with Fireball", func(t *testing.T) {
		fmt.Println("\n--- Testing: Kill Monster with Fireball ---")

		// Create a weak goblin using new YAML system
		goblin := monster.NewMonster3DFromConfig(150, 150, "goblin", cfg)
		fmt.Printf("Spawned: %s (HP: %d/%d)\n", goblin.Name, goblin.HitPoints, goblin.MaxHitPoints)

		// Use sorcerer
		sorcerer := party.Members[1] // Lysander
		fmt.Printf("Sorcerer %s ready (SP: %d/%d)\n", sorcerer.Name, sorcerer.SpellPoints, sorcerer.MaxSpellPoints)

		// Cast fireballs until monster dies
		fireballCount := 0
		for goblin.IsAlive() && sorcerer.SpellPoints >= 3 {
			fireballCount++

			// Calculate fireball damage
			damage := 10 + sorcerer.Intellect/2

			// Monster takes damage with resistances
			actualDamage := goblin.TakeDamage(damage, monster.DamageFire, 100, 100)
			sorcerer.SpellPoints -= 3

			fmt.Printf("Fireball %d: %d fire damage → %s HP: %d/%d (SP: %d)\n",
				fireballCount, actualDamage, goblin.Name, goblin.HitPoints, goblin.MaxHitPoints, sorcerer.SpellPoints)

			if !goblin.IsAlive() {
				fmt.Printf("💀 %s KILLED! Awarded %d experience and %d gold!\n", goblin.Name, goblin.Experience, goblin.Gold)

				// Award experience and gold to party
				party.Gold += goblin.Gold
				expPerMember := goblin.Experience / len(party.Members)
				for _, member := range party.Members {
					member.Experience += expPerMember
				}
				fmt.Printf("Party gained %d gold, %d exp per member\n", goblin.Gold, expPerMember)
				break
			}
		}

		if goblin.IsAlive() {
			t.Errorf("Goblin should be dead after %d fireballs", fireballCount)
		}
	})

	t.Run("Kill Monster with Sword Attacks", func(t *testing.T) {
		fmt.Println("\n--- Testing: Kill Monster with Sword Attacks ---")

		// Create another weak monster using new YAML system
		orc := monster.NewMonster3DFromConfig(200, 200, "orc", cfg)
		fmt.Printf("Spawned: %s (HP: %d/%d)\n", orc.Name, orc.HitPoints, orc.MaxHitPoints)

		// Use knight
		knight := party.Members[0] // Gareth
		fmt.Printf("Knight %s ready (Might: %d)\n", knight.Name, knight.Might)

		// Sword attacks until monster dies
		attackCount := 0
		for orc.IsAlive() && attackCount < 20 { // Safety limit
			attackCount++

			// Calculate sword damage
			swordSkill := knight.Skills[SkillSword]
			damage := 8 + knight.Might/3 + swordSkill.Level

			// Monster takes physical damage
			actualDamage := orc.TakeDamage(damage, monster.DamagePhysical, 150, 150)

			fmt.Printf("Sword attack %d: %d physical damage → %s HP: %d/%d\n",
				attackCount, actualDamage, orc.Name, orc.HitPoints, orc.MaxHitPoints)

			if !orc.IsAlive() {
				fmt.Printf("⚔️ %s SLAIN! Awarded %d experience and %d gold!\n", orc.Name, orc.Experience, orc.Gold)

				// Award experience and gold to party
				party.Gold += orc.Gold
				expPerMember := orc.Experience / len(party.Members)
				for _, member := range party.Members {
					member.Experience += expPerMember
				}
				fmt.Printf("Party gained %d gold, %d exp per member\n", orc.Gold, expPerMember)
				break
			}
		}

		if orc.IsAlive() {
			t.Errorf("Orc should be dead after %d sword attacks", attackCount)
		}
	})

	t.Run("Tough Monster Requires Multiple Characters", func(t *testing.T) {
		fmt.Println("\n--- Testing: Tough Monster - Team Combat ---")

		// Create a stronger monster using new YAML system
		troll := monster.NewMonster3DFromConfig(300, 300, "troll", cfg)
		fmt.Printf("Spawned: %s (HP: %d/%d) - This will be tough!\n", troll.Name, troll.HitPoints, troll.MaxHitPoints)

		totalDamageDealt := 0
		roundCount := 0

		// Team combat - everyone attacks
		for troll.IsAlive() && roundCount < 10 { // Safety limit
			roundCount++
			fmt.Printf("\n=== Combat Round %d ===\n", roundCount)

			// Knight sword attack
			knight := party.Members[0]
			if knight.HitPoints > 0 {
				swordSkill := knight.Skills[SkillSword]
				damage := 8 + knight.Might/3 + swordSkill.Level
				actualDamage := troll.TakeDamage(damage, monster.DamagePhysical, 250, 250)
				totalDamageDealt += actualDamage
				fmt.Printf("  Knight %s: %d sword damage\n", knight.Name, actualDamage)
			}

			if !troll.IsAlive() {
				break
			}

			// Sorcerer fireball
			sorcerer := party.Members[1]
			if sorcerer.SpellPoints >= 3 {
				damage := 10 + sorcerer.Intellect/2
				actualDamage := troll.TakeDamage(damage, monster.DamageFire, 250, 250)
				totalDamageDealt += actualDamage
				sorcerer.SpellPoints -= 3
				fmt.Printf("  Sorcerer %s: %d fire damage (SP: %d)\n", sorcerer.Name, actualDamage, sorcerer.SpellPoints)
			}

			if !troll.IsAlive() {
				break
			}

			// Cleric can heal if needed, or attack
			cleric := party.Members[2]
			if knight.HitPoints < knight.MaxHitPoints/2 && cleric.SpellPoints >= 2 {
				// Heal the knight
				healAmount := 15 + cleric.Personality/2
				knight.HitPoints += healAmount
				if knight.HitPoints > knight.MaxHitPoints {
					knight.HitPoints = knight.MaxHitPoints
				}
				cleric.SpellPoints -= 2
				fmt.Printf("  Cleric %s: healed knight for %d HP\n", cleric.Name, healAmount)
			}

			fmt.Printf("  → %s HP: %d/%d (Total damage dealt: %d)\n",
				troll.Name, troll.HitPoints, troll.MaxHitPoints, totalDamageDealt)
		}

		if !troll.IsAlive() {
			fmt.Printf("\n🏆 VICTORY! %s defeated after %d rounds!\n", troll.Name, roundCount)
			fmt.Printf("Team effort: %d total damage dealt\n", totalDamageDealt)
			fmt.Printf("Massive rewards: %d experience, %d gold!\n", troll.Experience, troll.Gold)

			// Award big rewards
			party.Gold += troll.Gold
			expPerMember := troll.Experience / len(party.Members)
			for _, member := range party.Members {
				member.Experience += expPerMember
			}
		} else {
			t.Error("Troll should eventually be defeated by team combat")
		}

		// Show final party status
		fmt.Println("\nFinal Party Status:")
		for i, member := range party.Members {
			fmt.Printf("  [%d] %s: HP %d/%d, SP %d/%d, Exp: %d\n",
				i+1, member.Name, member.HitPoints, member.MaxHitPoints,
				member.SpellPoints, member.MaxSpellPoints, member.Experience)
		}
		fmt.Printf("Party Gold: %d\n", party.Gold)
	})

	t.Run("Monster Resistances", func(t *testing.T) {
		fmt.Println("\n--- Testing: Monster Resistances ---")

		// Create a troll (has regeneration and resistances) using new YAML system
		trollForResistance := monster.NewMonster3DFromConfig(400, 400, "troll", cfg)
		fmt.Printf("Spawned: %s (HP: %d/%d)\n", trollForResistance.Name, trollForResistance.HitPoints, trollForResistance.MaxHitPoints)

		// Test fire damage
		fmt.Println("Testing fire damage:")
		fireDamage := 20
		actualFireDamage := trollForResistance.TakeDamage(fireDamage, monster.DamageFire, 350, 350)
		fmt.Printf("  Fire damage: %d → %d\n", fireDamage, actualFireDamage)

		// Reset HP for next test
		trollForResistance.HitPoints = trollForResistance.MaxHitPoints

		// Test physical damage
		fmt.Println("Testing physical damage:")
		physicalDamage := 20
		actualPhysicalDamage := trollForResistance.TakeDamage(physicalDamage, monster.DamagePhysical, 350, 350)
		fmt.Printf("  Physical damage: %d → %d\n", physicalDamage, actualPhysicalDamage)

		fmt.Printf("Fire vs Physical damage comparison: %d vs %d\n", actualFireDamage, actualPhysicalDamage)
	})
}

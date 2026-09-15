package character

import (
	"fmt"
	"strings"
)

// TrainingCost offers only the next tier of a skill the character already has.
// The caller owns the skill-existence and quest gates; this rule owns tiers/prices.
func (n *NPC) TrainingCost(current SkillMastery) (int, bool) {
	if n == nil || n.Type != NPCTypeSkillTrainer || current < MasteryNovice || current >= MasteryGrandMaster {
		return 0, false
	}
	cost := n.Training[strings.ToLower((current + 1).String())]
	return cost, cost > 0
}

// TrainingOfferLines describes the same authored tiers/prices in the editor.
func TrainingOfferLines(training map[string]int) []string {
	var lines []string
	for next := MasteryExpert; next <= MasteryGrandMaster; next++ {
		if cost := training[strings.ToLower(next.String())]; cost > 0 {
			lines = append(lines, fmt.Sprintf("%s -> %s: %d gold", next-1, next, cost))
		}
	}
	return lines
}

func validateNPCTraining(cfg *NPCConfig) error {
	for key, n := range cfg.NPCs {
		if n == nil {
			continue
		}
		if n.Type != NPCTypeSkillTrainer {
			if len(n.Training) != 0 {
				return fmt.Errorf("NPC %q: training requires skill_trainer type", key)
			}
			continue
		}
		if len(n.Training) == 0 {
			return fmt.Errorf("NPC %q: skill_trainer requires training offers", key)
		}
		for tier, cost := range n.Training {
			mastery, ok := masteryFromKey(tier)
			if !ok || mastery == MasteryNovice || cost <= 0 {
				return fmt.Errorf("NPC %q: training %q must name an upgrade tier with positive cost", key, tier)
			}
		}
	}
	return nil
}

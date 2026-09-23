package game

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
)

func (ui *UISystem) queueOverwatchTooltip(member *character.MMCharacter, x, y int) {
	chance := character.OverwatchChancePct(member.SkillTier(character.SkillOverwatch))
	ui.queueTooltip([]string{
		uitext.Text("ui.overwatch_ready"),
		uitext.Text("ui.overwatch_chances", chance, float64(chance)*character.OverwatchAttackChanceScale),
	}, x, y)
}

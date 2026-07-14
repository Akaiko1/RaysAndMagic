package game

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

type npcDialogVars struct {
	Name  string
	Spell string
	Cost  int
}

type trainerOption struct {
	Label     string
	Current   character.SkillMastery
	Next      character.SkillMastery
	Cost      int
	IsMagic   bool
	SkillType character.SkillType
	School    character.MagicSchoolID
}

func npcHasSpellTrading(npc *character.NPC) bool {
	return npc != nil && len(npc.SpellData) > 0
}

func npcHasMerchant(npc *character.NPC) bool {
	return npc != nil && (npc.SellAvailable || len(npc.MerchantStock) > 0)
}

func npcHasSkillTraining(npc *character.NPC) bool {
	return npc != nil && npc.Type == "skill_trainer"
}

// npcHasChoiceDialog reports whether the NPC presents a choice prompt - either
// an encounter (combat / quest pickup) or a pure dialogue with selectable
// options. Both flow through the same encounter-style UI and input handler.
func npcHasChoiceDialog(npc *character.NPC) bool {
	return npc != nil && (npc.EncounterData != nil || (npc.DialogueData != nil && len(npc.DialogueData.Choices) > 0))
}

func npcSpellKeys(npc *character.NPC) []string {
	if npc == nil || npc.SpellData == nil {
		return []string{}
	}
	keys := make([]string, 0, len(npc.SpellData))
	for key := range npc.SpellData {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func trainerOptions(char *character.MMCharacter) []trainerOption {
	if char == nil {
		return nil
	}
	options := make([]trainerOption, 0, len(char.Skills)+len(char.MagicSchools))
	for skillType, skill := range char.Skills {
		if skill == nil || skill.Mastery >= character.MasteryGrandMaster {
			continue
		}
		next := skill.Mastery + 1
		options = append(options, trainerOption{
			Label:     skillType.String(),
			Current:   skill.Mastery,
			Next:      next,
			Cost:      character.TrainingCostForMastery(next),
			SkillType: skillType,
		})
	}
	for school, skill := range char.MagicSchools {
		if skill == nil || skill.Mastery >= character.MasteryGrandMaster {
			continue
		}
		next := skill.Mastery + 1
		options = append(options, trainerOption{
			Label:   school.DisplayName() + " Magic",
			Current: skill.Mastery,
			Next:    next,
			Cost:    character.TrainingCostForMastery(next),
			IsMagic: true,
			School:  school,
		})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].IsMagic != options[j].IsMagic {
			return !options[i].IsMagic
		}
		return options[i].Label < options[j].Label
	})
	return options
}

func characterKnowsSpellByName(char *character.MMCharacter, spellName string) bool {
	if char == nil || spellName == "" {
		return false
	}
	for _, magicSkill := range char.MagicSchools {
		for _, spellID := range magicSkill.KnownSpells {
			if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.Name == spellName {
				return true
			}
		}
	}
	return false
}

func canCharacterLearnNPCSpell(char *character.MMCharacter, spellData *character.NPCSpell) bool {
	if char == nil || spellData == nil {
		return false
	}
	school, ok := schoolIDFromString(spellData.School)
	if !ok {
		return false
	}
	skill := char.MagicSchools[school]
	if skill == nil {
		return false
	}

	if spellData.Requirements != nil {
		req := spellData.Requirements
		if req.MinLevel > 0 && char.Level < req.MinLevel {
			return false
		}
		for _, schoolReq := range req.Schools {
			if strings.TrimSpace(schoolReq.School) == "" {
				continue
			}
			reqSchool, ok := schoolIDFromString(schoolReq.School)
			if !ok {
				return false
			}
			reqSkill := char.MagicSchools[reqSchool]
			if reqSkill == nil {
				return false
			}
			if schoolReq.MinLevel > 0 && reqSkill.Level() < schoolReq.MinLevel {
				return false
			}
		}
	}

	return true
}

// schoolIDFromString returns the typed school ID for a YAML/dialog string. The
// bool reports whether the value matches a known school.
func schoolIDFromString(raw string) (character.MagicSchoolID, bool) {
	school := character.MagicSchoolID(strings.ToLower(strings.TrimSpace(raw)))
	switch school {
	case character.MagicSchoolBody,
		character.MagicSchoolMind,
		character.MagicSchoolSpirit,
		character.MagicSchoolFire,
		character.MagicSchoolWater,
		character.MagicSchoolAir,
		character.MagicSchoolEarth,
		character.MagicSchoolLight,
		character.MagicSchoolDark:
		return school, true
	default:
		return "", false
	}
}

func formatNPCDialogue(template string, vars npcDialogVars) string {
	if template == "" {
		return ""
	}

	replaced := strings.ReplaceAll(template, "{name}", vars.Name)
	replaced = strings.ReplaceAll(replaced, "{spell}", vars.Spell)
	replaced = strings.ReplaceAll(replaced, "{cost}", strconv.Itoa(vars.Cost))

	if !strings.Contains(replaced, "%") {
		return replaced
	}

	args := make([]interface{}, 0)
	usedName := false
	usedSpell := false
	for i := 0; i < len(replaced); i++ {
		if replaced[i] != '%' || i+1 >= len(replaced) {
			continue
		}
		verb := replaced[i+1]
		if verb == '%' {
			i++
			continue
		}
		switch verb {
		case 's':
			if !usedName {
				args = append(args, vars.Name)
				usedName = true
			} else if !usedSpell {
				args = append(args, vars.Spell)
				usedSpell = true
			} else {
				args = append(args, "")
			}
		case 'd':
			args = append(args, vars.Cost)
		}
		i++
	}
	if len(args) == 0 {
		return replaced
	}
	return fmt.Sprintf(replaced, args...)
}

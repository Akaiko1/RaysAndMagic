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

func npcHasSpellTrading(npc *character.NPC) bool {
	return npc != nil && len(npc.SpellData) > 0
}

func npcHasMerchant(npc *character.NPC) bool {
	return npc != nil && (npc.SellAvailable || len(npc.MerchantStock) > 0)
}

func npcHasEncounter(npc *character.NPC) bool {
	return npc != nil && npc.EncounterData != nil
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
	school, ok := legacySchoolFromString(spellData.School)
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
			reqSchool, ok := legacySchoolFromString(schoolReq.School)
			if !ok {
				return false
			}
			reqSkill := char.MagicSchools[reqSchool]
			if reqSkill == nil {
				return false
			}
			if schoolReq.MinLevel > 0 && reqSkill.Level < schoolReq.MinLevel {
				return false
			}
		}
	}

	return true
}

func legacySchoolFromString(raw string) (character.MagicSchool, bool) {
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
		return character.MagicSchoolIDToLegacy(school), true
	default:
		return 0, false
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

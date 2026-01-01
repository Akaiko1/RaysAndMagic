package game

import (
	"fmt"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

type levelUpChoiceOption struct {
	choice         config.LevelUpChoice
	label          string
	masteryPrefix  string
	masteryCurrent string
	masteryNext    string
	hasMastery     bool
	skillType      character.SkillType
	school         character.MagicSchool
	spellID        spells.SpellID
}

type levelUpChoiceRequest struct {
	charIndex int
	level     int
	options   []levelUpChoiceOption
	selection int
}

func (g *MMGame) queueLevelUpChoices(char *character.MMCharacter, level int, choices []config.LevelUpChoice) {
	if char == nil || len(choices) == 0 {
		return
	}
	charIndex := -1
	for i, member := range g.party.Members {
		if member == char {
			charIndex = i
			break
		}
	}
	if charIndex == -1 {
		return
	}

	options := buildLevelUpChoiceOptions(char, choices)
	if len(options) == 0 {
		return
	}
	g.levelUpChoiceQueue = append(g.levelUpChoiceQueue, levelUpChoiceRequest{
		charIndex: charIndex,
		level:     level,
		options:   options,
		selection: 0,
	})
}

func (g *MMGame) currentLevelUpChoice() *levelUpChoiceRequest {
	if !g.levelUpChoiceOpen || len(g.levelUpChoiceQueue) == 0 {
		return nil
	}
	if g.levelUpChoiceIdx < 0 || g.levelUpChoiceIdx >= len(g.levelUpChoiceQueue) {
		g.levelUpChoiceOpen = false
		return nil
	}
	return &g.levelUpChoiceQueue[g.levelUpChoiceIdx]
}

func (g *MMGame) hasLevelUpChoiceForChar(charIndex int) bool {
	for _, req := range g.levelUpChoiceQueue {
		if req.charIndex == charIndex {
			return true
		}
	}
	return false
}

func (g *MMGame) openLevelUpChoiceForChar(charIndex int) {
	if g.levelUpChoiceOpen {
		return
	}
	for i, req := range g.levelUpChoiceQueue {
		if req.charIndex == charIndex {
			g.levelUpChoiceIdx = i
			g.levelUpChoiceOpen = true
			g.levelUpChoiceQueue[i].selection = 0
			return
		}
	}
}

func (g *MMGame) closeLevelUpChoice() {
	g.levelUpChoiceOpen = false
	g.levelUpChoiceIdx = 0
}

func (g *MMGame) consumeLevelUpChoice(choiceIdx int) {
	req := g.currentLevelUpChoice()
	if req == nil {
		return
	}
	if choiceIdx < 0 || choiceIdx >= len(req.options) {
		return
	}
	charIndex := req.charIndex
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	char := g.party.Members[charIndex]
	option := req.options[choiceIdx]

	switch option.choice.Type {
	case "spell":
		if addSpellByID(char, option.spellID) {
			g.AddCombatMessage(fmt.Sprintf("%s learned %s!", char.Name, spellDisplayName(option.spellID)))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s already knows %s.", char.Name, spellDisplayName(option.spellID)))
		}
	case "weapon_mastery", "armor_mastery":
		if upgradeSkillMastery(char, option.skillType) {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Mastery increased!", char.Name, skillDisplayName(option.skillType)))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Mastery is already at maximum.", char.Name, skillDisplayName(option.skillType)))
		}
	case "magic_mastery":
		if upgradeMagicMastery(char, option.school) {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Magic Mastery increased!", char.Name, magicSchoolDisplayName(option.school)))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Magic Mastery is already at maximum.", char.Name, magicSchoolDisplayName(option.school)))
		}
	}

	// Pop the current request
	idx := g.levelUpChoiceIdx
	if idx < 0 || idx >= len(g.levelUpChoiceQueue) {
		g.closeLevelUpChoice()
		return
	}
	if len(g.levelUpChoiceQueue) == 1 {
		g.levelUpChoiceQueue = g.levelUpChoiceQueue[:0]
	} else {
		g.levelUpChoiceQueue = append(g.levelUpChoiceQueue[:idx], g.levelUpChoiceQueue[idx+1:]...)
	}
	g.closeLevelUpChoice()
}

func buildLevelUpChoiceOptions(char *character.MMCharacter, choices []config.LevelUpChoice) []levelUpChoiceOption {
	var options []levelUpChoiceOption
	for _, choice := range choices {
		switch strings.ToLower(choice.Type) {
		case "spell":
			spellID := spells.SpellID(choice.Spell)
			if choice.Spell == "" {
				continue
			}
			def, err := spells.GetSpellDefinitionByID(spellID)
			if err != nil {
				continue
			}
			label := fmt.Sprintf("Learn Spell: %s", def.Name)
			if characterKnowsSpellByID(char, spellID) {
				label = fmt.Sprintf("Learn Spell: %s (Already Known)", def.Name)
			}
			options = append(options, levelUpChoiceOption{
				choice:  choice,
				label:   label,
				spellID: spellID,
			})
		case "weapon_mastery", "armor_mastery":
			skillType, ok := skillTypeFromKey(choice.Skill)
			if !ok {
				continue
			}
			label, current, next, ok := masteryOptionLabel(char, skillType)
			option := levelUpChoiceOption{
				choice:    choice,
				label:     label,
				skillType: skillType,
			}
			if ok {
				option.masteryPrefix = fmt.Sprintf("%s Mastery: ", skillDisplayName(skillType))
				option.masteryCurrent = current
				option.masteryNext = next
				option.hasMastery = true
			}
			options = append(options, option)
		case "magic_mastery":
			if strings.ToLower(choice.School) == "any" {
				for school := range char.MagicSchools {
					label, current, next, ok := magicMasteryOptionLabel(char, school)
					option := levelUpChoiceOption{
						choice: config.LevelUpChoice{
							Type:   choice.Type,
							School: schoolKey(school),
						},
						label:  label,
						school: school,
					}
					if ok {
						option.masteryPrefix = fmt.Sprintf("%s Magic Mastery: ", magicSchoolDisplayName(school))
						option.masteryCurrent = current
						option.masteryNext = next
						option.hasMastery = true
					}
					options = append(options, option)
				}
			} else {
				school := character.MagicSchoolIDToLegacy(character.MagicSchoolID(choice.School))
				if _, ok := char.MagicSchools[school]; !ok {
					continue
				}
				label, current, next, ok := magicMasteryOptionLabel(char, school)
				option := levelUpChoiceOption{
					choice: choice,
					label:  label,
					school: school,
				}
				if ok {
					option.masteryPrefix = fmt.Sprintf("%s Magic Mastery: ", magicSchoolDisplayName(school))
					option.masteryCurrent = current
					option.masteryNext = next
					option.hasMastery = true
				}
				options = append(options, option)
			}
		}
	}
	return options
}

func masteryOptionLabel(char *character.MMCharacter, skillType character.SkillType) (string, string, string, bool) {
	name := skillDisplayName(skillType)
	current := character.MasteryNovice
	if skill, ok := char.Skills[skillType]; ok {
		current = skill.Mastery
	}
	if current >= character.MasteryGrandMaster {
		return fmt.Sprintf("%s Mastery: %s (Max)", name, masteryName(current)), "", "", false
	}
	next := current + 1
	return fmt.Sprintf("%s Mastery: %s -> %s", name, masteryName(current), masteryName(next)), masteryName(current), masteryName(next), true
}

func magicMasteryOptionLabel(char *character.MMCharacter, school character.MagicSchool) (string, string, string, bool) {
	name := magicSchoolDisplayName(school)
	if skill, ok := char.MagicSchools[school]; ok {
		if skill.Mastery >= character.MasteryGrandMaster {
			return fmt.Sprintf("%s Magic Mastery: %s (Max)", name, masteryName(skill.Mastery)), "", "", false
		}
		return fmt.Sprintf("%s Magic Mastery: %s -> %s", name, masteryName(skill.Mastery), masteryName(skill.Mastery+1)),
			masteryName(skill.Mastery), masteryName(skill.Mastery + 1), true
	}
	return fmt.Sprintf("%s Magic Mastery: Novice -> Expert", name), masteryName(character.MasteryNovice), masteryName(character.MasteryExpert), true
}

func skillTypeFromKey(key string) (character.SkillType, bool) {
	switch strings.ToLower(key) {
	case "sword":
		return character.SkillSword, true
	case "dagger":
		return character.SkillDagger, true
	case "axe":
		return character.SkillAxe, true
	case "spear":
		return character.SkillSpear, true
	case "bow":
		return character.SkillBow, true
	case "mace":
		return character.SkillMace, true
	case "staff":
		return character.SkillStaff, true
	case "leather":
		return character.SkillLeather, true
	case "chain":
		return character.SkillChain, true
	case "plate":
		return character.SkillPlate, true
	case "shield":
		return character.SkillShield, true
	default:
		return 0, false
	}
}

func skillDisplayName(skillType character.SkillType) string {
	switch skillType {
	case character.SkillSword:
		return "Sword"
	case character.SkillDagger:
		return "Dagger"
	case character.SkillAxe:
		return "Axe"
	case character.SkillSpear:
		return "Spear"
	case character.SkillBow:
		return "Bow"
	case character.SkillMace:
		return "Mace"
	case character.SkillStaff:
		return "Staff"
	case character.SkillLeather:
		return "Leather"
	case character.SkillChain:
		return "Chain"
	case character.SkillPlate:
		return "Plate"
	case character.SkillShield:
		return "Shield"
	default:
		return "Unknown"
	}
}

func magicSchoolDisplayName(school character.MagicSchool) string {
	switch school {
	case character.MagicBody:
		return "Body"
	case character.MagicMind:
		return "Mind"
	case character.MagicSpirit:
		return "Spirit"
	case character.MagicFire:
		return "Fire"
	case character.MagicWater:
		return "Water"
	case character.MagicAir:
		return "Air"
	case character.MagicEarth:
		return "Earth"
	case character.MagicLight:
		return "Light"
	case character.MagicDark:
		return "Dark"
	default:
		return "Unknown"
	}
}

func schoolKey(school character.MagicSchool) string {
	return strings.ToLower(magicSchoolDisplayName(school))
}

func masteryName(mastery character.SkillMastery) string {
	switch mastery {
	case character.MasteryNovice:
		return "Novice"
	case character.MasteryExpert:
		return "Expert"
	case character.MasteryMaster:
		return "Master"
	case character.MasteryGrandMaster:
		return "Grandmaster"
	default:
		return "Unknown"
	}
}

func addSpellByID(char *character.MMCharacter, spellID spells.SpellID) bool {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	school := character.MagicSchoolIDToLegacy(character.MagicSchoolID(def.School))
	if char.MagicSchools[school] == nil {
		char.MagicSchools[school] = &character.MagicSkill{
			Level:       1,
			Mastery:     character.MasteryNovice,
			KnownSpells: make([]spells.SpellID, 0),
		}
	}
	for _, existing := range char.MagicSchools[school].KnownSpells {
		if existing == spellID {
			return false
		}
	}
	char.MagicSchools[school].KnownSpells = append(char.MagicSchools[school].KnownSpells, spellID)
	return true
}

func characterKnowsSpellByID(char *character.MMCharacter, spellID spells.SpellID) bool {
	for _, magicSkill := range char.MagicSchools {
		for _, known := range magicSkill.KnownSpells {
			if known == spellID {
				return true
			}
		}
	}
	return false
}

func spellDisplayName(spellID spells.SpellID) string {
	if def, err := spells.GetSpellDefinitionByID(spellID); err == nil {
		return def.Name
	}
	return string(spellID)
}

func upgradeSkillMastery(char *character.MMCharacter, skillType character.SkillType) bool {
	skill, ok := char.Skills[skillType]
	if !ok {
		skill = &character.Skill{Level: 1, Mastery: character.MasteryNovice}
		char.Skills[skillType] = skill
	}
	if skill.Mastery >= character.MasteryGrandMaster {
		return false
	}
	skill.Mastery++
	return true
}

func upgradeMagicMastery(char *character.MMCharacter, school character.MagicSchool) bool {
	skill, ok := char.MagicSchools[school]
	if !ok {
		return false
	}
	if skill.Mastery >= character.MasteryGrandMaster {
		return false
	}
	skill.Mastery++
	return true
}

func levelUpChoiceLayout(req *levelUpChoiceRequest, screenW, screenH int) (popupX, popupY, popupW, popupH, startY, rowH int) {
	popupW = 480
	rowH = 26
	baseH := 140
	if req != nil {
		popupH = baseH + len(req.options)*rowH
	} else {
		popupH = baseH
	}
	if popupH < 180 {
		popupH = 180
	}
	popupX = (screenW - popupW) / 2
	popupY = (screenH - popupH) / 2
	startY = popupY + 70
	return
}

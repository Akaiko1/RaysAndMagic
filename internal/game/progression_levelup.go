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
	school         character.MagicSchoolID
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
			g.AddCombatMessage(fmt.Sprintf("%s's %s Mastery increased!", char.Name, option.skillType.String()))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Mastery is already at maximum.", char.Name, option.skillType.String()))
		}
	case "magic_mastery":
		if upgradeMagicMastery(char, option.school) {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Magic Mastery increased!", char.Name, option.school.DisplayName()))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s's %s Magic Mastery is already at maximum.", char.Name, option.school.DisplayName()))
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
				option.masteryPrefix = fmt.Sprintf("%s Mastery: ", skillType.String())
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
							School: string(school),
						},
						label:  label,
						school: school,
					}
					if ok {
						option.masteryPrefix = fmt.Sprintf("%s Magic Mastery: ", school.DisplayName())
						option.masteryCurrent = current
						option.masteryNext = next
						option.hasMastery = true
					}
					options = append(options, option)
				}
			} else {
				school := character.MagicSchoolID(choice.School)
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
					option.masteryPrefix = fmt.Sprintf("%s Magic Mastery: ", school.DisplayName())
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
	name := skillType.String()
	current := character.MasteryNovice
	if skill, ok := char.Skills[skillType]; ok {
		skill.SyncLevelAndMastery()
		current = skill.Mastery
	}
	if current >= character.MasteryGrandMaster {
		return fmt.Sprintf("%s Mastery: %s (Max)", name, current.String()), "", "", false
	}
	next := current + 1
	return fmt.Sprintf("%s Mastery: %s -> %s", name, current.String(), next.String()), current.String(), next.String(), true
}

func magicMasteryOptionLabel(char *character.MMCharacter, school character.MagicSchoolID) (string, string, string, bool) {
	name := school.DisplayName()
	if skill, ok := char.MagicSchools[school]; ok {
		skill.SyncLevelAndMastery()
		if skill.Mastery >= character.MasteryGrandMaster {
			return fmt.Sprintf("%s Magic Mastery: %s (Max)", name, skill.Mastery.String()), "", "", false
		}
		next := skill.Mastery + 1
		return fmt.Sprintf("%s Magic Mastery: %s -> %s", name, skill.Mastery, next),
			skill.Mastery.String(), next.String(), true
	}
	return fmt.Sprintf("%s Magic Mastery: Novice -> Expert", name), character.MasteryNovice.String(), character.MasteryExpert.String(), true
}

// skillTypeFromKey resolves a level-up choice key (weapon or armor category)
// to its SkillType. "throwing" and "blaster" aren't level-up choices and are
// intentionally not accepted here.
func skillTypeFromKey(key string) (character.SkillType, bool) {
	key = strings.ToLower(key)
	if skill, ok := character.WeaponSkillForCategory(key); ok && key != "throwing" {
		return skill, true
	}
	return character.ArmorSkillForCategory(key)
}

func addSpellByID(char *character.MMCharacter, spellID spells.SpellID) bool {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	school := character.MagicSchoolID(def.School)
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
	return skill.IncreaseLevel()
}

func upgradeMagicMastery(char *character.MMCharacter, school character.MagicSchoolID) bool {
	skill, ok := char.MagicSchools[school]
	if !ok {
		return false
	}
	return skill.IncreaseLevel()
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

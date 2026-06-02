package game

import (
	"fmt"
	"math/rand"
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
	// Multi-select support. maxSelections == 1 (or 0) is the classic single-pick
	// level-up flow. maxSelections > 1 turns the popup into a "pick K of N"
	// picker (used by Archmage/Lich promotions): rows toggle into `selected`, and
	// a Confirm row applies them all at once, then runs onComplete.
	maxSelections int
	selected      []bool
	title         string
	onComplete    func()
}

// isMultiSelect reports whether this request is a "pick K of N" picker.
func (r *levelUpChoiceRequest) isMultiSelect() bool { return r.maxSelections > 1 }

// selectedCount returns how many options are currently toggled on.
func (r *levelUpChoiceRequest) selectedCount() int {
	n := 0
	for _, s := range r.selected {
		if s {
			n++
		}
	}
	return n
}

// confirmRowIndex is the cursor index of the Confirm row (only for multi-select),
// drawn just below the last option.
func (r *levelUpChoiceRequest) confirmRowIndex() int { return len(r.options) }

func (g *MMGame) queueLevelUpChoices(char *character.MMCharacter, level int, choices []config.LevelUpChoice) {
	if char == nil || g.party == nil {
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
		// Benched (non-active) hero: bank the owed choice on the character; it
		// surfaces when they're swapped into the active party.
		bankOwedLevelChoice(char, level)
		return
	}

	options := buildLevelUpChoiceOptions(char, choices)
	// Always offer at least MinLevelUpOptions: when level_up.yaml specifies fewer
	// (or none, e.g. levels 6/9/12), pad with random upgrades of skills the
	// character already owns so the player still gets a meaningful choice.
	options = padLevelUpOptions(char, options)
	if len(options) == 0 {
		return
	}
	g.levelUpChoiceQueue = append(g.levelUpChoiceQueue, levelUpChoiceRequest{
		charIndex:     charIndex,
		level:         level,
		options:       options,
		selection:     0,
		maxSelections: 1,
		selected:      make([]bool, len(options)),
	})
}

// grantSharedXP gives `amount` experience to every LIVING hero — active party,
// tavern reserve, and imprisoned captives — and applies any level-ups. Benched
// heroes thus "train alongside the party" (their stat points / L3 choice bank
// unspent). The living-only rule is uniform, so a downed hero (even benched)
// gains nothing. Single source for all XP-award sites. No-op without combat.
func (g *MMGame) grantSharedXP(amount int) {
	if amount <= 0 || g.combat == nil {
		return
	}
	award := func(m *character.MMCharacter) {
		if m != nil && m.HitPoints > 0 {
			// Learning grants a percentage XP bonus per mastery tier.
			gain := amount + amount*m.SkillTier(character.SkillLearning)*LearningXPPctPerTier/100
			m.Experience += gain
			g.combat.checkLevelUp(m)
		}
	}
	for _, m := range g.party.Members {
		award(m)
	}
	for _, m := range g.party.Reserve {
		award(m)
	}
	for _, m := range g.party.Captive {
		award(m)
	}
}

// bankOwedLevelChoice records a level-up choice owed to a benched character
// (dedup), to be surfaced when they next enter the active party.
func bankOwedLevelChoice(char *character.MMCharacter, level int) {
	for _, l := range char.OwedLevelChoices {
		if l == level {
			return
		}
	}
	char.OwedLevelChoices = append(char.OwedLevelChoices, level)
}

// swapRosterMember exchanges an active party member with a reserve member and
// reconciles the level-up choice queue: the outgoing member's un-consumed
// queued choices are banked back onto them, and the incoming member's banked
// choices are queued for their new slot.
func (g *MMGame) swapRosterMember(activeIdx, reserveIdx int) bool {
	if g.party == nil || activeIdx < 0 || activeIdx >= len(g.party.Members) ||
		reserveIdx < 0 || reserveIdx >= len(g.party.Reserve) {
		return false
	}
	outgoing := g.party.Members[activeIdx]
	g.benchQueuedChoices(activeIdx, outgoing)
	if !g.party.SwapActiveReserve(activeIdx, reserveIdx) {
		return false
	}
	g.drainOwedChoices(activeIdx)
	return true
}

// benchQueuedChoices removes any queued level-up choices for the given active
// slot and banks their levels back onto the character (they're leaving the party).
func (g *MMGame) benchQueuedChoices(charIndex int, char *character.MMCharacter) {
	if char == nil {
		return
	}
	kept := g.levelUpChoiceQueue[:0]
	for _, req := range g.levelUpChoiceQueue {
		if req.charIndex == charIndex {
			bankOwedLevelChoice(char, req.level)
			continue
		}
		kept = append(kept, req)
	}
	g.levelUpChoiceQueue = kept
	g.levelUpChoiceOpen = false
	g.levelUpChoiceIdx = 0
}

// drainOwedChoices queues every choice owed to the character now occupying the
// given active slot, then clears their owed list.
func (g *MMGame) drainOwedChoices(charIndex int) {
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	char := g.party.Members[charIndex]
	if char == nil || len(char.OwedLevelChoices) == 0 {
		return
	}
	owed := char.OwedLevelChoices
	char.OwedLevelChoices = nil
	for _, lvl := range owed {
		// Queue unconditionally: even levels with no explicit level_up.yaml entry
		// (6/9/12/...) still get padded to MinLevelUpOptions random upgrades.
		g.queueLevelUpChoices(char, lvl, config.GetLevelUpChoices(char.GetClassKey(), lvl))
	}
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

// applyLevelUpOption applies a single chosen option to the character.
func (g *MMGame) applyLevelUpOption(char *character.MMCharacter, option levelUpChoiceOption) {
	switch option.choice.Type {
	case "spell":
		if addSpellByID(char, option.spellID) {
			g.AddCombatMessage(fmt.Sprintf("%s learned %s!", char.Name, spellDisplayName(option.spellID)))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s already knows %s.", char.Name, spellDisplayName(option.spellID)))
		}
	case "weapon_mastery", "armor_mastery":
		if g.trainSkill(char, option.skillType) {
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
}

// popLevelUpChoice removes the active request from the queue and closes the popup.
func (g *MMGame) popLevelUpChoice() {
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

// consumeLevelUpChoice applies a single-select choice and pops the request.
// Used by the classic level-up flow (maxSelections == 1).
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
	g.applyLevelUpOption(g.party.Members[charIndex], req.options[choiceIdx])
	g.popLevelUpChoice()
}

// toggleLevelUpSelection flips option `idx` on/off in a multi-select picker,
// enforcing the maxSelections cap (a new toggle past the cap is ignored).
func (g *MMGame) toggleLevelUpSelection(idx int) {
	req := g.currentLevelUpChoice()
	if req == nil || idx < 0 || idx >= len(req.options) || idx >= len(req.selected) {
		return
	}
	if req.selected[idx] {
		req.selected[idx] = false
		return
	}
	if req.selectedCount() >= req.maxSelections {
		return // already picked the maximum
	}
	req.selected[idx] = true
}

// confirmLevelUpSelections applies every toggled option in a multi-select
// picker, runs onComplete, and pops the request. No-op until exactly
// maxSelections are chosen.
func (g *MMGame) confirmLevelUpSelections() {
	req := g.currentLevelUpChoice()
	if req == nil || !req.isMultiSelect() {
		return
	}
	if req.selectedCount() != req.maxSelections {
		return
	}
	charIndex := req.charIndex
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	char := g.party.Members[charIndex]
	for i, on := range req.selected {
		if on && i < len(req.options) {
			g.applyLevelUpOption(char, req.options[i])
		}
	}
	onComplete := req.onComplete
	g.popLevelUpChoice()
	if onComplete != nil {
		onComplete()
	}
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

// padLevelUpOptions ensures a level-up choice presents at least MinLevelUpOptions
// entries. It appends random "upgrade an existing skill/school" options for
// skills the character already owns (and hasn't maxed), skipping any already in
// the list, until the minimum is met or candidates run out.
func padLevelUpOptions(char *character.MMCharacter, options []levelUpChoiceOption) []levelUpChoiceOption {
	if char == nil || len(options) >= MinLevelUpOptions {
		return options
	}

	// Skills/schools already represented (don't re-offer the same upgrade).
	usedSkills := make(map[character.SkillType]bool)
	usedSchools := make(map[character.MagicSchoolID]bool)
	for _, opt := range options {
		switch strings.ToLower(opt.choice.Type) {
		case "weapon_mastery", "armor_mastery":
			usedSkills[opt.skillType] = true
		case "magic_mastery":
			usedSchools[opt.school] = true
		}
	}

	var candidates []levelUpChoiceOption
	for skillType := range char.Skills {
		if usedSkills[skillType] {
			continue
		}
		label, current, next, ok := masteryOptionLabel(char, skillType)
		if !ok { // already Grandmaster
			continue
		}
		candidates = append(candidates, levelUpChoiceOption{
			choice:         config.LevelUpChoice{Type: "weapon_mastery"},
			label:          label,
			masteryPrefix:  fmt.Sprintf("%s Mastery: ", skillType.String()),
			masteryCurrent: current,
			masteryNext:    next,
			hasMastery:     true,
			skillType:      skillType,
		})
	}
	for school := range char.MagicSchools {
		if usedSchools[school] {
			continue
		}
		label, current, next, ok := magicMasteryOptionLabel(char, school)
		if !ok { // already Grandmaster
			continue
		}
		candidates = append(candidates, levelUpChoiceOption{
			choice:         config.LevelUpChoice{Type: "magic_mastery", School: string(school)},
			label:          label,
			masteryPrefix:  fmt.Sprintf("%s Magic Mastery: ", school.DisplayName()),
			masteryCurrent: current,
			masteryNext:    next,
			hasMastery:     true,
			school:         school,
		})
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	for _, cand := range candidates {
		if len(options) >= MinLevelUpOptions {
			break
		}
		options = append(options, cand)
	}
	return options
}

func masteryOptionLabel(char *character.MMCharacter, skillType character.SkillType) (string, string, string, bool) {
	name := skillType.String()
	current := character.MasteryNovice
	if skill, ok := char.Skills[skillType]; ok {
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
		skill = &character.Skill{Mastery: character.MasteryNovice}
		char.Skills[skillType] = skill
	}
	return skill.IncreaseMastery()
}

// trainSkill raises a skill's mastery and refreshes derived stats so any
// stat-feeding skill (e.g. Bodybuilding → Max HP) updates immediately, just
// like spending an Endurance point. Single entry point for both the level-up
// choice and the NPC trainer. Returns false if already at max mastery.
func (g *MMGame) trainSkill(char *character.MMCharacter, skillType character.SkillType) bool {
	if !upgradeSkillMastery(char, skillType) {
		return false
	}
	char.RecalculateMaxStatsKeepingCurrent(g.config)
	return true
}

func upgradeMagicMastery(char *character.MMCharacter, school character.MagicSchoolID) bool {
	skill, ok := char.MagicSchools[school]
	if !ok {
		return false
	}
	return skill.IncreaseMastery()
}

func levelUpChoiceLayout(req *levelUpChoiceRequest, screenW, screenH int) (popupX, popupY, popupW, popupH, startY, rowH int) {
	popupW = 480
	rowH = 26
	baseH := 140
	if req != nil {
		rows := len(req.options)
		if req.isMultiSelect() {
			rows++ // Confirm row drawn below the options
		}
		popupH = baseH + rows*rowH
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

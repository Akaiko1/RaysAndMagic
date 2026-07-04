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
	// A Grandmaster of Learning teaches the whole party: a flat % to everyone,
	// on top of each hero's own per-tier bonus.
	teacherPct := g.learningTeacherBonusPct()
	award := func(m *character.MMCharacter, announce bool) {
		if m != nil && m.HitPoints > 0 {
			pct := m.SkillTier(character.SkillLearning)*LearningXPPctPerTier + teacherPct
			gain := amount + amount*pct/100
			m.Experience += gain
			g.combat.checkLevelUp(m, announce) // only the visible active party announces level-ups
		}
	}
	for _, m := range g.party.Members {
		award(m, true)
	}
	for _, m := range g.party.Reserve {
		award(m, false)
	}
	for _, m := range g.party.Captive {
		award(m, false)
	}
}

// learningTeacherBonusPct returns the party-wide XP percentage contributed by a
// living Grandmaster of Learning ("teacher"). Counted once for the whole party.
func (g *MMGame) learningTeacherBonusPct() int {
	if g.party == nil {
		return 0
	}
	rosters := [][]*character.MMCharacter{g.party.Members, g.party.Reserve, g.party.Captive}
	for _, roster := range rosters {
		for _, m := range roster {
			if m != nil && m.HitPoints > 0 && m.SkillTier(character.SkillLearning) >= int(character.MasteryGrandMaster) {
				return LearningGMPartyXPPct
			}
		}
	}
	return 0
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
	// Buffs (Bless) belong to the ACTIVE party: the incoming hero picks up the
	// current bonuses, the benched one sheds them — otherwise a swap freezes a
	// buff on the bench forever (or the newcomer fights unbuffed). Route through
	// applyPartyStatBonuses (not a partial hand-roll) so the incoming member also
	// picks up card-granted BonusMaxHP/BonusRegenPct — a member benched before the
	// party found a Jungle Idol/Troll Card would otherwise swap back in with
	// stale (zero) values until some unrelated change happened to recompute them.
	outgoing.BuffBonuses = character.StatBonuses{}
	outgoing.BonusMaxHP = 0
	outgoing.BonusRegenPct = 0
	outgoing.RecalculateMaxStatsKeepingCurrent(g.config)
	g.applyPartyStatBonuses()
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
			// Recompute labels: an earlier stacked popup may have already raised
			// this skill's mastery, so "Novice -> Expert" must become "Expert -> Master".
			g.refreshLevelUpOptionLabels(&g.levelUpChoiceQueue[i])
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

// setLevelUpOptionDisplay (re)computes an option's display fields (label and
// mastery prefix/current/next) from the character's CURRENT state. Single source
// for building, padding, and refreshing options so a stacked second popup can't
// show a stale "Novice -> Expert" after the first popup already raised mastery.
// hasMastery ends false for a maxed (Grandmaster) skill/school.
func setLevelUpOptionDisplay(char *character.MMCharacter, opt *levelUpChoiceOption) {
	switch strings.ToLower(opt.choice.Type) {
	case "spell":
		def, err := spells.GetSpellDefinitionByID(opt.spellID)
		if err != nil {
			return
		}
		if characterKnowsSpellByID(char, opt.spellID) {
			opt.label = fmt.Sprintf("Learn Spell: %s (Already Known)", def.Name)
		} else {
			opt.label = fmt.Sprintf("Learn Spell: %s", def.Name)
		}
	case "weapon_mastery", "armor_mastery":
		label, current, next, ok := masteryOptionLabel(char, opt.skillType)
		opt.label, opt.masteryCurrent, opt.masteryNext, opt.hasMastery = label, current, next, ok
		if ok {
			opt.masteryPrefix = fmt.Sprintf("%s Mastery: ", opt.skillType.String())
		} else {
			opt.masteryPrefix = ""
		}
	case "magic_mastery":
		label, current, next, ok := magicMasteryOptionLabel(char, opt.school)
		opt.label, opt.masteryCurrent, opt.masteryNext, opt.hasMastery = label, current, next, ok
		if ok {
			opt.masteryPrefix = fmt.Sprintf("%s Magic Mastery: ", opt.school.DisplayName())
		} else {
			opt.masteryPrefix = ""
		}
	}
}

func buildLevelUpChoiceOptions(char *character.MMCharacter, choices []config.LevelUpChoice) []levelUpChoiceOption {
	var options []levelUpChoiceOption
	add := func(opt levelUpChoiceOption) {
		setLevelUpOptionDisplay(char, &opt)
		options = append(options, opt)
	}
	for _, choice := range choices {
		switch strings.ToLower(choice.Type) {
		case "spell":
			if choice.Spell == "" {
				continue
			}
			spellID := spells.SpellID(choice.Spell)
			if _, err := spells.GetSpellDefinitionByID(spellID); err != nil {
				continue
			}
			add(levelUpChoiceOption{choice: choice, spellID: spellID})
		case "weapon_mastery", "armor_mastery":
			skillType, ok := skillTypeFromKey(choice.Skill)
			if !ok {
				continue
			}
			add(levelUpChoiceOption{choice: choice, skillType: skillType})
		case "magic_mastery":
			if strings.ToLower(choice.School) == "any" {
				for school := range char.MagicSchools {
					add(levelUpChoiceOption{
						choice: config.LevelUpChoice{Type: choice.Type, School: string(school)},
						school: school,
					})
				}
			} else {
				school := character.MagicSchoolID(choice.School)
				if _, ok := char.MagicSchools[school]; !ok {
					continue
				}
				add(levelUpChoiceOption{choice: choice, school: school})
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
	addCandidate := func(opt levelUpChoiceOption) {
		setLevelUpOptionDisplay(char, &opt)
		if !opt.hasMastery { // already Grandmaster → not a real upgrade
			return
		}
		candidates = append(candidates, opt)
	}
	for skillType := range char.Skills {
		if !usedSkills[skillType] {
			addCandidate(levelUpChoiceOption{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: skillType})
		}
	}
	for school := range char.MagicSchools {
		if !usedSchools[school] {
			addCandidate(levelUpChoiceOption{choice: config.LevelUpChoice{Type: "magic_mastery", School: string(school)}, school: school})
		}
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

// refreshLevelUpOptionLabels recomputes every option's display text against the
// character's current mastery. Called when a (possibly stacked) request becomes
// the active popup so labels reflect upgrades applied by earlier popups.
func (g *MMGame) refreshLevelUpOptionLabels(req *levelUpChoiceRequest) {
	if req == nil || req.charIndex < 0 || req.charIndex >= len(g.party.Members) {
		return
	}
	char := g.party.Members[req.charIndex]
	if char == nil {
		return
	}
	for i := range req.options {
		setLevelUpOptionDisplay(char, &req.options[i])
	}
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
	return char.LearnSpell(spellID)
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
	char.RecalculateMaxStatsGrantingGain(g.config)
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

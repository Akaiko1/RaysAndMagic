package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// lastLevelUpRequest returns the most recently queued level-up choice request.
func lastLevelUpRequest(t *testing.T, g *MMGame) levelUpChoiceRequest {
	t.Helper()
	if len(g.levelUpChoiceQueue) == 0 {
		t.Fatal("expected a queued level-up choice")
	}
	return g.levelUpChoiceQueue[len(g.levelUpChoiceQueue)-1]
}

// TestLevelUpChoice_AlwaysAtLeastFourOptions: an explicit L3 entry (which lists
// only 2 options) is padded up to MinLevelUpOptions with upgrades of skills the
// character already owns.
func TestLevelUpChoice_AlwaysAtLeastFourOptions(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t) // loads level_up.yaml
	g := newTestGame(cfg, newTestWorld(cfg))

	member := g.party.Members[0]
	explicit := config.GetLevelUpChoices(member.GetClassKey(), 3)
	g.queueLevelUpChoices(member, 3, explicit)

	req := lastLevelUpRequest(t, g)
	if len(req.options) < MinLevelUpOptions {
		t.Errorf("L3 choice offered %d options, want >= %d", len(req.options), MinLevelUpOptions)
	}
}

// TestLevelUpChoice_SixNineTwelveOffered: levels with no explicit level_up.yaml
// entry still present MinLevelUpOptions random upgrades of owned skills.
func TestLevelUpChoice_SixNineTwelveOffered(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0]

	for _, lvl := range []int{6, 9, 12} {
		g.levelUpChoiceQueue = nil
		g.queueLevelUpChoices(member, lvl, config.GetLevelUpChoices(member.GetClassKey(), lvl))
		req := lastLevelUpRequest(t, g)
		if len(req.options) < MinLevelUpOptions {
			t.Errorf("level %d offered %d options, want >= %d", lvl, len(req.options), MinLevelUpOptions)
		}
		// Every padded option must be an upgrade of a skill/school the member owns.
		for _, opt := range req.options {
			switch opt.choice.Type {
			case "weapon_mastery", "armor_mastery":
				if _, ok := member.Skills[opt.skillType]; !ok {
					t.Errorf("level %d offered upgrade for unowned skill %v", lvl, opt.skillType)
				}
			case "magic_mastery":
				if _, ok := member.MagicSchools[opt.school]; !ok {
					t.Errorf("level %d offered upgrade for unowned school %v", lvl, opt.school)
				}
			}
		}
	}
}

// TestApplyLevelUpOption_BodybuildingRefreshesMaxHP: choosing the Bodybuilding
// mastery upgrade must raise Max HP right away (like spending Endurance), not
// only at the next level-up/equip recalc.
func TestApplyLevelUpOption_BodybuildingRefreshesMaxHP(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0]
	member.Skills[character.SkillBodybuilding] = &character.Skill{Mastery: character.MasteryNovice}
	member.CalculateDerivedStats(cfg)

	before := member.MaxHitPoints
	g.applyLevelUpOption(member, levelUpChoiceOption{
		choice:    config.LevelUpChoice{Type: "weapon_mastery"},
		skillType: character.SkillBodybuilding,
	})
	if got := member.MaxHitPoints - before; got != character.BodybuildingHPPerTier {
		t.Errorf("Max HP delta after Bodybuilding upgrade = %d, want %d", got, character.BodybuildingHPPerTier)
	}
}

// TestStackedLevelUp_SecondPopupRefreshesMastery: two stacked level-ups that
// both offer the same skill must not both read "Novice -> Expert". After the
// first popup raises mastery, the second must show "Expert -> Master".
func TestStackedLevelUp_SecondPopupRefreshesMastery(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0]
	member.Skills[character.SkillSword] = &character.Skill{Mastery: character.MasteryNovice}

	mkReq := func() levelUpChoiceRequest {
		opt := levelUpChoiceOption{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: character.SkillSword}
		setLevelUpOptionDisplay(member, &opt)
		return levelUpChoiceRequest{
			charIndex: 0, level: 3,
			options:  []levelUpChoiceOption{opt},
			selected: make([]bool, 1), maxSelections: 1,
		}
	}
	g.levelUpChoiceQueue = []levelUpChoiceRequest{mkReq(), mkReq()}

	// First popup: Novice -> Expert, then apply.
	g.openLevelUpChoiceForChar(0)
	if got := g.currentLevelUpChoice().options[0].masteryNext; got != character.MasteryExpert.String() {
		t.Fatalf("first popup next = %q, want %q", got, character.MasteryExpert.String())
	}
	g.consumeLevelUpChoice(0)
	if member.Skills[character.SkillSword].Mastery != character.MasteryExpert {
		t.Fatalf("Sword should be Expert after first pick, got %v", member.Skills[character.SkillSword].Mastery)
	}

	// Second (stacked) popup must reflect the new mastery, not the stale one.
	g.openLevelUpChoiceForChar(0)
	opt := g.currentLevelUpChoice().options[0]
	if opt.masteryCurrent != character.MasteryExpert.String() || opt.masteryNext != character.MasteryMaster.String() {
		t.Errorf("second popup shows %q -> %q, want %q -> %q",
			opt.masteryCurrent, opt.masteryNext, character.MasteryExpert.String(), character.MasteryMaster.String())
	}
	g.consumeLevelUpChoice(0)
	if member.Skills[character.SkillSword].Mastery != character.MasteryMaster {
		t.Errorf("Sword should be Master after second pick, got %v", member.Skills[character.SkillSword].Mastery)
	}
}

// TestPadLevelUpOptions_NoDuplicateSkill: padding never re-offers a skill that
// an explicit option already covers.
func TestPadLevelUpOptions_NoDuplicateSkill(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0]

	var pick character.SkillType
	for st := range member.Skills {
		pick = st
		break
	}
	explicit := []levelUpChoiceOption{{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: pick}}
	padded := padLevelUpOptions(member, explicit)

	seen := 0
	for _, opt := range padded {
		if (opt.choice.Type == "weapon_mastery" || opt.choice.Type == "armor_mastery") && opt.skillType == pick {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("skill %v appears %d times after padding, want exactly 1", pick, seen)
	}
}

func TestLevelUpChoice_FiltersMaxedExplicitMasteriesAndShowsRemainingUpgrades(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0] // knight: explicit L3 choices are sword/spear.

	for _, skill := range member.Skills {
		skill.Mastery = character.MasteryGrandMaster
	}
	member.Skills[character.SkillLeather].Mastery = character.MasteryMaster
	member.Skills[character.SkillChain].Mastery = character.MasteryExpert

	g.queueLevelUpChoices(member, 3, config.GetLevelUpChoices(member.GetClassKey(), 3))
	req := lastLevelUpRequest(t, g)
	if len(req.options) != 2 {
		t.Fatalf("options = %d, want exactly the two remaining non-maxed masteries: %#v", len(req.options), req.options)
	}

	seen := map[character.SkillType]bool{}
	for _, opt := range req.options {
		if !opt.hasMastery {
			t.Fatalf("maxed or non-upgrade option leaked into popup: %#v", opt)
		}
		seen[opt.skillType] = true
		if opt.skillType == character.SkillSword || opt.skillType == character.SkillSpear {
			t.Fatalf("maxed explicit option leaked into popup: %s", opt.skillType)
		}
	}
	for _, want := range []character.SkillType{character.SkillLeather, character.SkillChain} {
		if !seen[want] {
			t.Fatalf("missing remaining upgrade %s; got %#v", want, req.options)
		}
	}
}

// twoOwnedSkills returns two distinct skills the member already has.
func twoOwnedSkills(t *testing.T, member *character.MMCharacter) (a, b character.SkillType) {
	t.Helper()
	got := make([]character.SkillType, 0, 2)
	for st := range member.Skills {
		got = append(got, st)
		if len(got) == 2 {
			return got[0], got[1]
		}
	}
	t.Fatalf("test member needs at least two skills, has %d", len(got))
	return
}

// Options that went stale between queueing and opening (an earlier stacked
// popup maxed the skill) are pruned from the popup instead of being shown as
// "(Max)"; a request whose every option went stale dissolves without opening.
func TestLevelUpChoice_MaxedOptionsPrunedOnOpen(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0]

	maxed, valid := twoOwnedSkills(t, member)
	member.Skills[maxed].Mastery = character.MasteryGrandMaster
	member.Skills[valid].Mastery = character.MasteryNovice

	g.levelUpChoiceQueue = []levelUpChoiceRequest{{
		charIndex: 0,
		options: []levelUpChoiceOption{
			{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: maxed},
			{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: valid},
		},
	}}
	g.openLevelUpChoiceForChar(0)

	req := g.currentLevelUpChoice()
	if req == nil {
		t.Fatal("popup with one valid option must open")
	}
	if len(req.options) != 1 || req.options[0].skillType != valid {
		t.Fatalf("maxed option must be pruned, got %+v", req.options)
	}

	g.consumeLevelUpChoice(0)
	if len(g.levelUpChoiceQueue) != 0 {
		t.Fatal("the remaining valid option must consume the choice")
	}
	if member.Skills[valid].Mastery != character.MasteryExpert {
		t.Fatalf("skill mastery = %v, want Expert", member.Skills[valid].Mastery)
	}
	if member.Skills[maxed].Mastery != character.MasteryGrandMaster {
		t.Fatal("maxed skill must be untouched")
	}
}

// A request whose every option is stale never opens - and never costs anything.
func TestLevelUpChoice_AllMaxedRequestDissolves(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	member := g.party.Members[0]

	maxed, _ := twoOwnedSkills(t, member)
	member.Skills[maxed].Mastery = character.MasteryGrandMaster

	g.levelUpChoiceQueue = []levelUpChoiceRequest{{
		charIndex: 0,
		options: []levelUpChoiceOption{
			{choice: config.LevelUpChoice{Type: "weapon_mastery"}, skillType: maxed},
		},
	}}
	g.openLevelUpChoiceForChar(0)

	if g.currentLevelUpChoice() != nil || len(g.levelUpChoiceQueue) != 0 {
		t.Fatal("an all-stale request must dissolve instead of opening")
	}
}

// level_up.yaml offers the Archer the SAME spell at level 3 and again at level 9.
// Taking it the first time must not shrink the second menu: the now-dead option
// is REPLACED by a random mastery upgrade, never left as a gap (user-reported:
// "3 options instead of 4").
func TestLevelUpChoice_RepeatedSpellIsReplacedNotDropped(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)
	g := newTestGame(cfg, newTestWorld(cfg))

	idx := -1
	var archer *character.MMCharacter
	for i, m := range g.party.Members {
		if m != nil && m.GetClassKey() == "archer" {
			idx, archer = i, m
			break
		}
	}
	if archer == nil {
		t.Skip("shipped party has no archer")
	}

	l3 := config.GetLevelUpChoices("archer", 3)
	l9 := config.GetLevelUpChoices("archer", 9)
	repeated := ""
	for _, early := range l3 {
		if !strings.EqualFold(early.Type, "spell") {
			continue
		}
		for _, late := range l9 {
			if strings.EqualFold(late.Type, "spell") && late.Spell == early.Spell {
				repeated = early.Spell
			}
		}
	}
	if repeated == "" {
		t.Skip("no spell is offered to the archer twice any more")
	}
	repeatedID := spells.SpellID(repeated)

	// Take it at level 3 through the real popup path.
	g.queueLevelUpChoices(archer, 3, l3)
	g.openLevelUpChoiceForChar(idx)
	req := g.currentLevelUpChoice()
	if req == nil {
		t.Fatal("level 3 choice must open")
	}
	pick := -1
	for i, opt := range req.options {
		if opt.spellID == repeatedID {
			pick = i
		}
	}
	if pick < 0 {
		t.Fatalf("level 3 menu must offer %s, got %d options", repeated, len(req.options))
	}
	g.consumeLevelUpChoice(pick)
	if !characterKnowsSpellByID(archer, repeatedID) {
		t.Fatalf("%s should be known after picking it", repeated)
	}

	// Level 9 repeats the same spell - the menu must still be full and dead-entry free.
	g.queueLevelUpChoices(archer, 9, l9)
	g.openLevelUpChoiceForChar(idx)
	req = g.currentLevelUpChoice()
	if req == nil {
		t.Fatal("level 9 choice must open")
	}
	if len(req.options) != MinLevelUpOptions {
		t.Errorf("level 9 offered %d options, want %d", len(req.options), MinLevelUpOptions)
	}
	for _, opt := range req.options {
		if opt.spellID == repeatedID {
			t.Errorf("already-known %s must not be offered again", repeated)
		}
		if !strings.EqualFold(opt.choice.Type, "spell") && !opt.hasMastery {
			t.Errorf("substituted option %q is not a real upgrade", opt.label)
		}
	}
}

// The spell can also become known AFTER the request was built but BEFORE its
// popup opens (stacked level-ups, a trader purchase, a lectern between the two).
// The menu must still be topped back up when it opens.
func TestLevelUpChoice_SpellLearnedAfterQueueingIsReplaced(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)
	g := newTestGame(cfg, newTestWorld(cfg))

	idx := -1
	var archer *character.MMCharacter
	for i, m := range g.party.Members {
		if m != nil && m.GetClassKey() == "archer" {
			idx, archer = i, m
			break
		}
	}
	if archer == nil {
		t.Skip("shipped party has no archer")
	}

	l9 := config.GetLevelUpChoices("archer", 9)
	var offered spells.SpellID
	for _, c := range l9 {
		if strings.EqualFold(c.Type, "spell") {
			offered = spells.SpellID(c.Spell)
		}
	}
	if offered == "" {
		t.Skip("archer level 9 offers no spell")
	}

	// Queue while the spell is still unknown: it takes a slot in the request.
	g.queueLevelUpChoices(archer, 9, l9)
	req := lastLevelUpRequest(t, g)
	if len(req.options) != MinLevelUpOptions {
		t.Fatalf("queued %d options, want %d", len(req.options), MinLevelUpOptions)
	}

	// Learn it elsewhere before the popup shows (trader/lectern/stacked popup).
	if !archer.LearnSpell(offered) {
		t.Fatalf("could not pre-learn %s", offered)
	}

	g.openLevelUpChoiceForChar(idx)
	open := g.currentLevelUpChoice()
	if open == nil {
		t.Fatal("choice must open")
	}
	if len(open.options) != MinLevelUpOptions {
		t.Errorf("after pruning the known spell the menu offered %d options, want %d",
			len(open.options), MinLevelUpOptions)
	}
	for _, opt := range open.options {
		if opt.spellID == offered {
			t.Errorf("already-known %s must not remain in the menu", offered)
		}
	}
}

// Class-agnostic guard: for EVERY class and EVERY authored level, the menu is
// full and free of dead entries - both fresh and after the character already
// learned everything that level offers. Today only the Archer repeats a spell
// (Lightning at 3 and 9), so this also protects future level_up.yaml edits.
func TestLevelUpChoice_EveryClassAndLevelStaysFull(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)

	for _, class := range character.PlayableClasses {
		classKey := class.Key()
		// Levels are authored sparsely (3, 9, ...); sweep the whole early game.
		for level := 2; level <= 20; level++ {
			choices := config.GetLevelUpChoices(classKey, level)
			if len(choices) == 0 {
				continue
			}
			for _, preLearned := range []bool{false, true} {
				name := fmt.Sprintf("%s/L%d/prelearned=%v", classKey, level, preLearned)
				t.Run(name, func(t *testing.T) {
					g := newTestGame(cfg, newTestWorld(cfg))
					hero := character.CreateCharacter("Probe", class, cfg)
					g.party.Members[0] = hero

					if preLearned {
						for _, c := range choices {
							if strings.EqualFold(c.Type, "spell") {
								hero.LearnSpell(spells.SpellID(c.Spell))
							}
						}
					}

					g.queueLevelUpChoices(hero, level, choices)
					g.openLevelUpChoiceForChar(0)
					req := g.currentLevelUpChoice()
					if req == nil {
						t.Fatalf("%s: no popup opened", name)
					}
					if len(req.options) != MinLevelUpOptions {
						t.Errorf("%s: offered %d options, want %d", name, len(req.options), MinLevelUpOptions)
					}
					for _, opt := range req.options {
						if strings.EqualFold(opt.choice.Type, "spell") {
							if characterKnowsSpellByID(hero, opt.spellID) {
								t.Errorf("%s: offers already-known spell %s", name, opt.spellID)
							}
							continue
						}
						if !opt.hasMastery {
							t.Errorf("%s: option %q is not a real upgrade", name, opt.label)
						}
					}
				})
			}
		}
	}
}

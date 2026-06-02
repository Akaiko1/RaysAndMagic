package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
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

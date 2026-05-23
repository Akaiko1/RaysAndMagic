package character

import "testing"

func TestSkillLevelAndMasterySync(t *testing.T) {
	skill := &Skill{Level: 1, Mastery: MasteryExpert}
	skill.SyncLevelAndMastery()
	if skill.Level != 2 || skill.Mastery != MasteryExpert {
		t.Fatalf("expected inconsistent skill to keep higher mastery as level 2/expert, got level=%d mastery=%s", skill.Level, skill.Mastery)
	}

	if !skill.IncreaseLevel() {
		t.Fatalf("expected skill to increase")
	}
	if skill.Level != 3 || skill.Mastery != MasteryMaster {
		t.Fatalf("expected level 3/master after increase, got level=%d mastery=%s", skill.Level, skill.Mastery)
	}

	skill.SetLevel(99)
	if skill.Level != MaxSkillLevel || skill.Mastery != MasteryGrandMaster {
		t.Fatalf("expected clamped grandmaster, got level=%d mastery=%s", skill.Level, skill.Mastery)
	}
}

func TestMagicSkillLevelAndMasterySync(t *testing.T) {
	skill := &MagicSkill{Level: 1, Mastery: MasteryMaster}
	skill.SyncLevelAndMastery()
	if skill.Level != 3 || skill.Mastery != MasteryMaster {
		t.Fatalf("expected inconsistent magic skill to keep higher mastery as level 3/master, got level=%d mastery=%s", skill.Level, skill.Mastery)
	}
}

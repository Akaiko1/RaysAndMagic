package character

import "testing"

func TestSkillLevelIsDerivedFromMastery(t *testing.T) {
	for mastery := MasteryNovice; mastery <= MasteryGrandMaster; mastery++ {
		skill := &Skill{Mastery: mastery}
		if got, want := skill.Level(), int(mastery)+1; got != want {
			t.Fatalf("expected skill level %d for mastery %s, got %d", want, mastery, got)
		}
	}
}

func TestMagicSkillLevelIsDerivedFromMastery(t *testing.T) {
	for mastery := MasteryNovice; mastery <= MasteryGrandMaster; mastery++ {
		skill := &MagicSkill{Mastery: mastery}
		if got, want := skill.Level(), int(mastery)+1; got != want {
			t.Fatalf("expected magic skill level %d for mastery %s, got %d", want, mastery, got)
		}
	}
}

func TestIncreaseMasteryStopsAtGrandMaster(t *testing.T) {
	skill := &Skill{Mastery: MasteryMaster}
	if !skill.IncreaseMastery() {
		t.Fatalf("expected skill mastery to increase")
	}
	if skill.Mastery != MasteryGrandMaster || skill.Level() != MaxSkillLevel {
		t.Fatalf("expected grandmaster level %d, got level=%d mastery=%s", MaxSkillLevel, skill.Level(), skill.Mastery)
	}
	if skill.IncreaseMastery() {
		t.Fatalf("expected grandmaster skill to stay capped")
	}
}

package character

import (
	"fmt"
	"strings"
	"testing"
	"ugataima/internal/config"
)

func TestSkillCatalogProgressionsWithoutGlobalConfig(t *testing.T) {
	previous := config.GlobalConfig
	config.GlobalConfig = nil
	t.Cleanup(func() { config.GlobalConfig = previous })
	for _, tc := range []struct {
		skill SkillType
		value func(int) int
		want  [4]int
	}{
		{SkillOverwatch, OverwatchChancePct, [4]int{20, 30, 40, 50}},
		{SkillBallistics, BallisticsSpeedPct, [4]int{15, 25, 35, 50}},
		{SkillBallistics, BallisticsRangeTiles, [4]int{0, 0, 1, 2}},
		{SkillBallistics, BallisticsCritPct, [4]int{2, 4, 6, 8}},
		{SkillFieldMedicine, FieldMedicineRestorePct, [4]int{15, 25, 40, 60}},
		{SkillFieldMedicine, FieldMedicinePoisonReductionPct, [4]int{10, 20, 30, 40}},
		{SkillDesignateTarget, DesignationCritPct, [4]int{5, 8, 12, 15}},
		{SkillDesignateTarget, DesignationSeconds, [4]int{6, 9, 12, 15}},
	} {
		t.Run(fmt.Sprintf("%s/%v", tc.skill.String(), tc.want), func(t *testing.T) {
			for tier, want := range tc.want {
				if got := tc.value(tier); got != want {
					t.Fatalf("tier=%d got=%d want=%d", tier, got, want)
				}
			}
			if tc.value(-1) != 0 || tc.value(99) != tc.want[3] {
				t.Fatal("mastery boundary rule diverged from the common catalog")
			}
			progression := fmt.Sprintf("%d/%d/%d/%d", tc.want[0], tc.want[1], tc.want[2], tc.want[3])
			if text := tc.skill.Description(); !strings.Contains(text, progression) || strings.Contains(text, "%!") {
				t.Fatalf("shared description lost progression: %s", text)
			}
		})
	}
}

func TestFieldMedicineAndBallisticsRequireLearnedSkill(t *testing.T) {
	def := &config.WeaponDefinitionConfig{Category: "bow", Range: 5}
	for _, kind := range []string{"nil", "untrained", "trained"} {
		t.Run(kind, func(t *testing.T) {
			var ch *MMCharacter
			if kind != "nil" {
				ch = &MMCharacter{Skills: map[SkillType]*Skill{}}
			}
			if kind == "trained" {
				ch.Skills[SkillBallistics] = &Skill{Mastery: MasteryGrandMaster}
				ch.Skills[SkillFieldMedicine] = &Skill{Mastery: MasteryGrandMaster}
			}
			reach, _ := EffectiveWeaponFlight(def, ch)
			wantReach, wantHeal := 5.0, 100
			if kind == "trained" {
				wantReach, wantHeal = 7, 160
			}
			if reach != wantReach || ConsumableRestore(ch, 100, 0, false) != wantHeal || ConsumableRestore(ch, 100, 0, true) != wantHeal {
				t.Fatal("skill effect ignored training or nil bearer")
			}
		})
	}
}

// A skill's label names its mastery only when the skill has grades: racial
// traits like Celestial Providence read as the bare name.
func TestMasteryLabelNamesGradesOnlyForGradedSkills(t *testing.T) {
	for _, s := range AllSkills {
		got := s.MasteryLabel(MasteryExpert)
		want := s.String()
		if s.UsesMastery() {
			want += " (Expert)"
		}
		if got != want {
			t.Errorf("%s label = %q, want %q", s.String(), got, want)
		}
	}
	for _, s := range []SkillType{SkillCelestialProvidence, SkillHalflingGuile, SkillDarkElfBinding} {
		if s.UsesMastery() || strings.Contains(s.MasteryLabel(MasteryNovice), "(") {
			t.Errorf("%s shows a grade it does not have", s.String())
		}
	}
}

// Race-owned skills are exactly the ones a race grants: EnsureRacialTraits
// gives each to its race, and no class kit lists one.
func TestRaceOwnedSkillsComeFromRaces(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	granted := map[SkillType]bool{}
	for race := range cfg.Characters.Races {
		for _, class := range PlayableClasses {
			c := CreateCharacter("Race", class, cfg)
			if race != "human" {
				c.ApplyRace(race, cfg)
			}
			for st := range c.Skills {
				if st.RaceOwned() {
					granted[st] = true
				}
			}
		}
	}
	for _, st := range AllSkills {
		if st.RaceOwned() != granted[st] {
			t.Errorf("%s: race-owned=%v but a race grants it=%v", st.String(), st.RaceOwned(), granted[st])
		}
	}
	for key, class := range cfg.Characters.Classes {
		for _, sk := range class.Skills {
			if st, ok := SkillTypeFromKey(sk); ok && st.RaceOwned() {
				t.Errorf("class %s kit lists race-owned %s", key, sk)
			}
		}
	}
}

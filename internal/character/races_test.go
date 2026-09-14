package character

import (
	"testing"
	"ugataima/internal/config"
)

func racesTestConfig() *config.Config {
	return &config.Config{
		Characters: config.CharacterConfig{
			Classes: map[string]config.ClassStats{
				"knight": {Might: 15, Intellect: 10, Personality: 10, Endurance: 15, Accuracy: 12, Speed: 10, Luck: 10},
				"archer": {Might: 12, Intellect: 10, Personality: 10, Endurance: 10, Accuracy: 15, Speed: 12, Luck: 10},
			},
			Races: map[string]config.RaceStats{
				"human":    {},
				"half_orc": {Might: 3, Endurance: 2, Intellect: -2, Personality: -2, Speed: -1},
				"halfling": {Accuracy: 2, Speed: 2, Luck: 2, Might: -3, Endurance: -1},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 2, LevelMultiplier: 3},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}
}

func TestApplyRace_AdditiveModifiers(t *testing.T) {
	cfg := racesTestConfig()
	c := CreateCharacter("Grikka", ClassKnight, cfg)
	c.ApplyRace("half_orc", cfg)

	if c.Might != 18 || c.Endurance != 17 || c.Intellect != 8 || c.Personality != 8 || c.Speed != 9 {
		t.Errorf("half_orc knight stats = M%d I%d P%d E%d S%d, want M18 I8 P8 E17 S9",
			c.Might, c.Intellect, c.Personality, c.Endurance, c.Speed)
	}
	// Untouched modifiers stay at class base.
	if c.Accuracy != 12 || c.Luck != 10 {
		t.Errorf("accuracy/luck shifted without a modifier: A%d L%d", c.Accuracy, c.Luck)
	}
}

func TestApplyRace_HumanBaseline(t *testing.T) {
	cfg := racesTestConfig()
	base := CreateCharacter("Gareth", ClassKnight, cfg)
	human := CreateCharacter("Gareth2", ClassKnight, cfg)
	human.ApplyRace("human", cfg)
	if human.Might != base.Might || human.Endurance != base.Endurance || human.Luck != base.Luck {
		t.Error("human race must be a no-op baseline")
	}
}

func TestCreateRosterCharacter_RaceRederivesHP(t *testing.T) {
	cfg := racesTestConfig()
	human := createRosterCharacter(config.RosterEntry{Name: "Gareth", Class: "knight"}, cfg)
	orc := createRosterCharacter(config.RosterEntry{Name: "Grikka", Class: "knight", Race: "half_orc"}, cfg)
	if orc.MaxHitPoints != human.MaxHitPoints+2*cfg.Characters.HitPoints.EnduranceMultiplier {
		t.Errorf("half_orc MaxHP = %d, want %d (endurance +2 rederived)",
			orc.MaxHitPoints, human.MaxHitPoints+2*cfg.Characters.HitPoints.EnduranceMultiplier)
	}
	if orc.HitPoints != orc.MaxHitPoints {
		t.Error("recruit should start at full HP after race application")
	}
}

func TestNewParty_TavernRecruitsStartInReserve(t *testing.T) {
	cfg := racesTestConfig()
	cfg.Characters.StartingParty = []config.RosterEntry{{Name: "Gareth", Class: "knight"}}
	cfg.Characters.Captives = []config.RosterEntry{{Name: "Auberon", Class: "knight"}}
	cfg.Characters.TavernRecruits = []config.RosterEntry{
		{Name: "Grikka", Class: "knight", Race: "half_orc"},
		{Name: "Brinna", Class: "archer", Race: "halfling"},
	}

	party := NewParty(cfg)
	if len(party.Reserve) != 2 {
		t.Fatalf("reserve = %d heroes, want the 2 tavern recruits", len(party.Reserve))
	}
	grikka, brinna := party.Reserve[0], party.Reserve[1]
	if grikka.Name != "Grikka" || grikka.Class != ClassKnight || grikka.Might != 18 {
		t.Errorf("Grikka = %s/%v might %d, want knight with half_orc might 18", grikka.Name, grikka.Class, grikka.Might)
	}
	if brinna.Name != "Brinna" || brinna.Class != ClassArcher || brinna.Accuracy != 17 || brinna.Might != 9 {
		t.Errorf("Brinna = %s/%v acc %d might %d, want archer with halfling acc 17 might 9",
			brinna.Name, brinna.Class, brinna.Accuracy, brinna.Might)
	}
}

func TestEnsureRacialTraitsCaseTable(t *testing.T) {
	cfg := racesTestConfig()
	cfg.Characters.Races["celestial"] = config.RaceStats{}
	cfg.Characters.Races["dark_elf"] = config.RaceStats{}
	tests := []struct {
		name        string
		race        string
		class       CharacterClass
		oldDefense  SkillMastery
		wantSkill   SkillType
		wantMastery SkillMastery
		wantDefense bool
	}{
		{"celestial fixed passive", "celestial", ClassKnight, MasteryNovice, SkillCelestialProvidence, MasteryNovice, true},
		{"halfling fixed passive", "halfling", ClassArcher, MasteryNovice, SkillHalflingGuile, MasteryNovice, false},
		{"dark elf fixed passive", "dark_elf", ClassArcher, MasteryNovice, SkillDarkElfBinding, MasteryNovice, false},
		{"half-orc Knight migrates defense mastery", "half_orc", ClassKnight, MasteryMaster, SkillOrcishFury, MasteryMaster, false},
		{"half-orc non-Knight gets no replacement", "half_orc", ClassArcher, MasteryNovice, SkillOrcishFury, MasteryNovice, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &MMCharacter{
				Name:   tt.name,
				Class:  tt.class,
				Race:   tt.race,
				Skills: map[SkillType]*Skill{},
			}
			if tt.class == ClassKnight {
				c.Skills[SkillImpenetrableDefense] = &Skill{Mastery: tt.oldDefense}
			}
			c.EnsureRacialTraits(cfg)
			skill := c.Skills[tt.wantSkill]
			if tt.race == "half_orc" && tt.class != ClassKnight {
				if skill != nil {
					t.Fatalf("non-Knight half-orc received Orcish Fury")
				}
				return
			}
			if skill == nil || skill.Mastery != tt.wantMastery {
				t.Fatalf("racial skill = %+v, want %s at %s", skill, tt.wantSkill, tt.wantMastery)
			}
			_, hasDefense := c.Skills[SkillImpenetrableDefense]
			if hasDefense != tt.wantDefense {
				t.Fatalf("defense present = %v, want %v", hasDefense, tt.wantDefense)
			}
		})
	}
}

func TestEnsureRacialTraitsInfersLegacyRosterRace(t *testing.T) {
	cfg := racesTestConfig()
	cfg.Characters.TavernRecruits = []config.RosterEntry{{Name: "Grikka", Class: "knight", Race: "half_orc"}}
	c := &MMCharacter{
		Name:   "Grikka",
		Class:  ClassKnight,
		Skills: map[SkillType]*Skill{SkillImpenetrableDefense: {Mastery: MasteryExpert}},
	}
	if !c.EnsureRacialTraits(cfg) {
		t.Fatal("legacy roster character was not migrated")
	}
	if c.Race != "half_orc" || c.Skills[SkillOrcishFury] == nil || c.Skills[SkillOrcishFury].Mastery != MasteryExpert {
		t.Fatalf("legacy race migration = race %q, fury %+v", c.Race, c.Skills[SkillOrcishFury])
	}
}

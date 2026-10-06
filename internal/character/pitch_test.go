package character

import (
	"strings"
	"testing"

	"ugataima/internal/config"
)

func pitchTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// highlighted is every keyword a pitch highlights.
func highlighted(t *testing.T, paragraphs []string) []string {
	t.Helper()
	var out []string
	for _, p := range paragraphs {
		spans, err := config.ParseKeywordMarkup(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range spans {
			if s.Kind != "" {
				out = append(out, s.Text)
			}
		}
	}
	return out
}

// A hero's pitch describes exactly the kit that hero has: for every class in
// every race, no highlighted keyword names a skill the hero lacks, every
// racial trait it has is named, and a non-human race lists its stat shifts.
func TestHeroPitchMatchesTheKit(t *testing.T) {
	cfg := pitchTestConfig(t)
	racial := []SkillType{SkillCelestialProvidence, SkillHalflingGuile, SkillDarkElfBinding, SkillOrcishFury}
	skillByName := map[string]SkillType{}
	for _, st := range AllSkills {
		skillByName[st.String()] = st
	}
	check := func(t *testing.T, c *MMCharacter) {
		pitch := HeroPitch(c, cfg)
		words := highlighted(t, pitch)
		for _, w := range words {
			if st, ok := skillByName[w]; ok && !c.HasSkill(st) {
				t.Errorf("%s %s: pitch names %s, which the hero lacks", c.Race, c.Class.Key(), w)
			}
		}
		for _, st := range racial {
			if c.HasSkill(st) && !strings.Contains(strings.Join(words, "|"), st.String()) {
				t.Errorf("%s %s: pitch never names its %s", c.Race, c.Class.Key(), st.String())
			}
		}
		if race := cfg.Characters.Races[c.Race]; c.Race != "" && c.Race != "human" {
			if len(pitch) < 2 || !strings.HasPrefix(pitch[len(pitch)-1], race.Name+": ") {
				t.Errorf("%s %s: no race paragraph in %q", c.Race, c.Class.Key(), pitch)
			} else if race.Might != 0 && !strings.Contains(pitch[len(pitch)-1], "Might") {
				t.Errorf("%s %s: race paragraph misses its Might shift", c.Race, c.Class.Key())
			}
		}
		if len(pitch) == 0 || len(words) == 0 {
			t.Errorf("%s %s: empty pitch", c.Race, c.Class.Key())
		}
	}
	for _, class := range PlayableClasses {
		for race := range cfg.Characters.Races {
			t.Run(race+"/"+class.Key(), func(t *testing.T) {
				c := CreateCharacter("Pitch", class, cfg)
				if race != "human" {
					c.ApplyRace(race, cfg)
				}
				check(t, c)
			})
		}
	}
	// The shipped heroes, as the party-creation screen builds them.
	for _, roster := range [][]config.RosterEntry{cfg.Characters.StartingParty, cfg.Characters.Captives, cfg.Characters.TavernRecruits} {
		for _, e := range roster {
			t.Run("roster/"+e.Name, func(t *testing.T) { check(t, CreateRosterCharacter(e, cfg)) })
		}
	}
}

// raceStatShifts lists gains before losses, each group in sheet order.
func TestRaceStatShiftsOrder(t *testing.T) {
	got := raceStatShifts(config.RaceStats{Might: 3, Intellect: -2, Personality: -2, Endurance: 2, Speed: -1, Luck: 1})
	if want := "+3 Might, +2 Endurance, +1 Luck, -2 Intellect, -2 Personality, -1 Speed"; got != want {
		t.Errorf("raceStatShifts = %q, want %q", got, want)
	}
	if got := raceStatShifts(config.RaceStats{Name: "Human"}); got != "" {
		t.Errorf("a race without modifiers lists %q, want nothing", got)
	}
}

// The reported heroes: the half-orc knight gets Orcish Fury instead of the
// Impenetrable Defense point, the celestial cleric her dawn/dusk buff, and a
// human knight keeps Impenetrable Defense. Every authored point tied to a
// skill the hero has is in the pitch, every point tied to a skill it lacks is
// not, and a race with stat shifts opens its paragraph with them.
func TestHeroPitchRacialCases(t *testing.T) {
	cfg := pitchTestConfig(t)
	for _, tc := range []struct {
		race, class string
		has, lacks  []SkillType
	}{
		{"half_orc", "knight", []SkillType{SkillOrcishFury}, []SkillType{SkillImpenetrableDefense}},
		{"human", "knight", []SkillType{SkillImpenetrableDefense}, []SkillType{SkillOrcishFury}},
		{"celestial", "cleric", []SkillType{SkillCelestialProvidence}, nil},
		{"dark_elf", "thief", []SkillType{SkillDarkElfBinding, SkillTrapper}, nil},
		{"halfling", "archer", []SkillType{SkillHalflingGuile}, nil},
	} {
		t.Run(tc.race+"/"+tc.class, func(t *testing.T) {
			hero := CreateRosterCharacter(config.RosterEntry{Name: "Pitch", Class: tc.class, Race: tc.race}, cfg)
			if hero == nil {
				t.Fatalf("no hero for class %q", tc.class)
			}
			race, ok := cfg.Characters.Races[tc.race]
			if !ok {
				t.Fatalf("race %q is not authored", tc.race)
			}
			text := strings.Join(HeroPitch(hero, cfg), "\n")
			points := append(append([]config.DescriptionPoint{}, cfg.Characters.Classes[tc.class].Description...), race.Description...)
			pointsFor := func(st SkillType) []string {
				var out []string
				for _, p := range points {
					if p.Skill == skillKey(st) {
						out = append(out, p.Text)
					}
				}
				return out
			}
			for _, st := range tc.has {
				if !hero.HasSkill(st) {
					t.Fatalf("hero lacks %s", st)
				}
				texts := pointsFor(st)
				if len(texts) == 0 {
					t.Fatalf("no pitch point is tied to %s", st)
				}
				for _, w := range texts {
					if !strings.Contains(text, w) {
						t.Errorf("pitch lacks %q:\n%s", w, text)
					}
				}
			}
			for _, st := range tc.lacks {
				if hero.HasSkill(st) {
					t.Fatalf("hero has %s", st)
				}
				for _, w := range pointsFor(st) {
					if strings.Contains(text, w) {
						t.Errorf("pitch still has %q:\n%s", w, text)
					}
				}
			}
			header := race.Name + ": "
			if shifts := raceStatShifts(race); shifts != "" {
				if !strings.Contains(text, header+shifts+".") {
					t.Errorf("pitch lacks %q:\n%s", header+shifts+".", text)
				}
			} else if strings.Contains(text, header) {
				t.Errorf("race without shifts still opens a %q paragraph:\n%s", header, text)
			}
		})
	}
}

// The shipped pitches pass the load check; a keyword naming a skill must sit
// in a point tied to that skill.
func TestValidateHeroPitches(t *testing.T) {
	cfg := pitchTestConfig(t)
	if err := ValidateHeroPitches(cfg); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		point config.DescriptionPoint
		want  string
	}{
		{config.DescriptionPoint{Text: "{defense:Impenetrable Defense} blocks."}, "needs skill: impenetrable_defense"},
		{config.DescriptionPoint{Skill: "sword", Text: "{defense:Impenetrable Defense} blocks."}, "needs skill: impenetrable_defense"},
		{config.DescriptionPoint{Skill: "no_such_skill", Text: "Plain."}, "unknown skill"},
		{config.DescriptionPoint{Skill: "impenetrable_defense", Text: "{defense:Impenetrable Defense} blocks."}, ""},
		{config.DescriptionPoint{Text: "A {defense:tank} with a {damage:sword or spear}."}, ""},
	} {
		c := *cfg
		c.Characters.Classes = map[string]config.ClassStats{"knight": {Description: []config.DescriptionPoint{tc.point}}}
		c.Characters.Races = nil
		err := ValidateHeroPitches(&c)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%+v: error %v, want %q", tc.point, err, tc.want)
		}
	}
}

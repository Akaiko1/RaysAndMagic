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

// The reported heroes: the half-orc knight gets Orcish Fury instead of the
// Impenetrable Defense sentence, the celestial cleric her dawn/dusk buff, and
// a human knight keeps Impenetrable Defense.
func TestHeroPitchRacialCases(t *testing.T) {
	cfg := pitchTestConfig(t)
	find := func(name string) *MMCharacter {
		for _, e := range cfg.Characters.TavernRecruits {
			if e.Name == name {
				return CreateRosterCharacter(e, cfg)
			}
		}
		for _, e := range cfg.Characters.StartingParty {
			if e.Name == name {
				return CreateRosterCharacter(e, cfg)
			}
		}
		t.Fatalf("no roster hero %s", name)
		return nil
	}
	for _, tc := range []struct {
		hero      string
		want, not []string
	}{
		{"Grikka", []string{"Orcish Fury", "Half-Orc: +3 Might, +2 Endurance, -2 Intellect"}, []string{"{defense:Impenetrable Defense}"}},
		{"Gareth", []string{"{defense:Impenetrable Defense}"}, []string{"Orcish Fury", "Human:"}},
		{"Auralis", []string{"Celestial Providence", "random Master-tier buff", "Celestial: +1 Intellect, +3 Personality, +1 Luck, -2 Might"}, nil},
		{"Nyra", []string{"Dark Elf Binding", "{control:Trapper}"}, nil},
		{"Brinna", []string{"Halfling Guile", "Halfling: "}, nil},
	} {
		text := strings.Join(HeroPitch(find(tc.hero), cfg), "\n")
		for _, w := range tc.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s: pitch lacks %q:\n%s", tc.hero, w, text)
			}
		}
		for _, w := range tc.not {
			if strings.Contains(text, w) {
				t.Errorf("%s: pitch still has %q:\n%s", tc.hero, w, text)
			}
		}
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

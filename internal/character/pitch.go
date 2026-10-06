package character

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
)

// HeroPitch is the party-creation pitch of this hero as keyword-markup
// paragraphs: the class points its kit has, then its race's stat shifts and
// the race points it has. A point tied to a skill the hero lacks (a racial
// replacement) is left out.
func HeroPitch(c *MMCharacter, cfg *config.Config) []string {
	if c == nil || cfg == nil {
		return nil
	}
	var out []string
	if class := c.pitchPoints(cfg.Characters.Classes[c.Class.Key()].Description); class != "" {
		out = append(out, class)
	}
	race, ok := cfg.Characters.Races[c.Race]
	if !ok {
		return out
	}
	var parts []string
	if shifts := raceStatShifts(race); shifts != "" {
		parts = append(parts, race.Name+": "+shifts+".")
	}
	if points := c.pitchPoints(race.Description); points != "" {
		parts = append(parts, points)
	}
	if len(parts) > 0 {
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func (c *MMCharacter) pitchPoints(points []config.DescriptionPoint) string {
	var kept []string
	for _, p := range points {
		if p.Skill != "" {
			if st, ok := SkillTypeFromKey(p.Skill); !ok || !c.HasSkill(st) {
				continue
			}
		}
		kept = append(kept, p.Text)
	}
	return strings.Join(kept, " ")
}

// raceStatShifts lists a race's stat modifiers, gains first, each group in
// sheet order ("+3 Might, +2 Endurance, -2 Intellect").
func raceStatShifts(r config.RaceStats) string {
	stats := []struct {
		name  string
		value int
	}{
		{"Might", r.Might}, {"Intellect", r.Intellect}, {"Personality", r.Personality}, {"Endurance", r.Endurance},
		{"Accuracy", r.Accuracy}, {"Speed", r.Speed}, {"Luck", r.Luck},
	}
	var gains, losses []string
	for _, s := range stats {
		switch {
		case s.value > 0:
			gains = append(gains, fmt.Sprintf("%+d %s", s.value, s.name))
		case s.value < 0:
			losses = append(losses, fmt.Sprintf("%+d %s", s.value, s.name))
		}
	}
	return strings.Join(append(gains, losses...), ", ")
}

// ValidateHeroPitches checks every class and race pitch point: its skill key
// is known, and a keyword naming a skill names the point's own skill, so a
// pitch can never promise a skill the hero's kit lacks.
func ValidateHeroPitches(cfg *config.Config) error {
	byName := map[string]SkillType{}
	for _, st := range AllSkills {
		byName[strings.ToLower(st.String())] = st
	}
	check := func(path string, points []config.DescriptionPoint) error {
		for i, p := range points {
			var gate SkillType
			gated := false
			if p.Skill != "" {
				st, ok := SkillTypeFromKey(p.Skill)
				if !ok {
					return fmt.Errorf("%s.description[%d]: unknown skill %q", path, i, p.Skill)
				}
				gate, gated = st, true
			}
			spans, err := config.ParseKeywordMarkup(p.Text)
			if err != nil {
				return fmt.Errorf("%s.description[%d]: %v", path, i, err)
			}
			for _, s := range spans {
				st, names := byName[strings.ToLower(s.Text)]
				if s.Kind == "" || !names {
					continue
				}
				if !gated || st != gate {
					return fmt.Errorf("%s.description[%d]: keyword %q names a skill, so the point needs skill: %s", path, i, s.Text, skillKey(st))
				}
			}
		}
		return nil
	}
	for key, class := range cfg.Characters.Classes {
		if err := check("characters.classes."+key, class.Description); err != nil {
			return err
		}
	}
	for key, race := range cfg.Characters.Races {
		if err := check("characters.races."+key, race.Description); err != nil {
			return err
		}
	}
	return nil
}

func skillKey(st SkillType) string {
	for key, t := range skillTypeByKey {
		if t == st {
			return key
		}
	}
	return st.String()
}

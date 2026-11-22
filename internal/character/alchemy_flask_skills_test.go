package character

import "testing"

// A flask is thrown only with every skill it names: Bomb Throwing for a
// crafted flask, Bomb Throwing and Alchemy for the reusable Retort. Nobody
// (a nil hero) throws nothing.
func TestCanThrowFlaskNeedsEverySkill(t *testing.T) {
	hero := func(skills ...SkillType) *MMCharacter {
		c := &MMCharacter{Skills: map[SkillType]*Skill{}}
		for _, s := range skills {
			c.Skills[s] = &Skill{Mastery: MasteryNovice}
		}
		return c
	}
	for _, tc := range []struct {
		name  string
		c     *MMCharacter
		flask string
		want  bool
	}{
		{"no hero", nil, "fire_flask", false},
		{"crafted flask, bomb throwing", hero(SkillBombThrowing), "fire_flask", true},
		{"crafted flask, untrained", hero(), "fire_flask", false},
		{"retort, both skills", hero(SkillBombThrowing, SkillAlchemy), "solstice_everflow_retort", true},
		{"retort, no alchemy", hero(SkillBombThrowing), "solstice_everflow_retort", false},
		{"retort, no bomb throwing", hero(SkillAlchemy), "solstice_everflow_retort", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.CanThrowFlask(tc.flask); got != tc.want {
				t.Fatalf("CanThrowFlask = %v, want %v", got, tc.want)
			}
		})
	}
}

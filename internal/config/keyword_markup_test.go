package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseKeywordMarkup(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    []KeywordSpan
		wantErr string
	}{
		{in: "plain prose", want: []KeywordSpan{{Text: "plain prose"}}},
		{in: "{heal:Heal} mends", want: []KeywordSpan{{"heal", "Heal"}, {Text: " mends"}}},
		{in: "a {auto:bear ally}, b", want: []KeywordSpan{{Text: "a "}, {"auto", "bear ally"}, {Text: ", b"}}},
		{in: "{damage:Harm}{damage:Lash}", want: []KeywordSpan{{"damage", "Harm"}, {"damage", "Lash"}}},
		{in: "{glory:Big}", wantErr: "unknown kind"},
		{in: "{heal}", wantErr: "{kind:text}"},
		{in: "{heal:}", wantErr: "{kind:text}"},
		{in: "{heal:open", wantErr: "unclosed"},
		{in: "{heal:{buff:x}}", wantErr: "unclosed"},
		{in: "stray } brace", wantErr: "unmatched"},
	} {
		got, err := ParseKeywordMarkup(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%q: error %v, want %q", tc.in, err, tc.wantErr)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	if got := PlainKeywordText("Knows {heal:Heal} and {damage:Harm}."); got != "Knows Heal and Harm." {
		t.Errorf("plain text = %q", got)
	}
}

// Every class carries a valid pitch and every race a name; a class without
// one, or a point with bad markup, fails the load.
func TestEveryClassHasADescription(t *testing.T) {
	cfg, err := LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for key, class := range cfg.Characters.Classes {
		if len(class.Description) == 0 {
			t.Errorf("class %s has no description", key)
		}
	}
	for key, race := range cfg.Characters.Races {
		if race.Name == "" {
			t.Errorf("race %s has no name", key)
		}
	}
	for _, tc := range []struct {
		points   []DescriptionPoint
		required bool
		want     string
	}{
		{nil, true, "description is required"},
		{nil, false, ""},
		{[]DescriptionPoint{{Text: "  "}}, false, "text is required"},
		{[]DescriptionPoint{{Text: "A {armor:tank}."}}, true, "unknown kind"},
		{[]DescriptionPoint{{Text: "A {defense:tank}."}, {Skill: "sword", Text: "Swings."}}, true, ""},
	} {
		err := validateDescription("characters.classes.knight", tc.points, tc.required)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%+v: error %v, want %q", tc.points, err, tc.want)
		}
	}
}

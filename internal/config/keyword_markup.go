package config

import (
	"fmt"
	"slices"
	"strings"
)

// Authored help prose may mark keywords as {kind:text}. The kind names what
// the keyword does for the party; the game draws each kind in its own color.
var KeywordKinds = []string{"heal", "buff", "damage", "defense", "auto", "control", "utility"}

// KeywordSpan is a run of prose; Kind is "" for plain text.
type KeywordSpan struct {
	Kind string
	Text string
}

// ParseKeywordMarkup splits prose into plain and marked spans.
func ParseKeywordMarkup(s string) ([]KeywordSpan, error) {
	var spans []KeywordSpan
	for s != "" {
		open := strings.IndexAny(s, "{}")
		if open < 0 {
			spans = append(spans, KeywordSpan{Text: s})
			break
		}
		if s[open] == '}' {
			return nil, fmt.Errorf("unmatched '}' near %q", s[open:])
		}
		if open > 0 {
			spans = append(spans, KeywordSpan{Text: s[:open]})
		}
		body, rest, closed := strings.Cut(s[open+1:], "}")
		if !closed || strings.Contains(body, "{") {
			return nil, fmt.Errorf("unclosed keyword near %q", s[open:])
		}
		kind, text, ok := strings.Cut(body, ":")
		if !ok || text == "" {
			return nil, fmt.Errorf("keyword %q must be {kind:text}", body)
		}
		if !slices.Contains(KeywordKinds, kind) {
			return nil, fmt.Errorf("keyword %q: unknown kind %q (want one of %s)", body, kind, strings.Join(KeywordKinds, ", "))
		}
		spans = append(spans, KeywordSpan{Kind: kind, Text: text})
		s = rest
	}
	return spans, nil
}

// PlainKeywordText is the prose with its markup removed, for plain-text
// readers. It expects markup that passed validation.
func PlainKeywordText(s string) string {
	spans, err := ParseKeywordMarkup(s)
	if err != nil {
		return s
	}
	var b strings.Builder
	for _, span := range spans {
		b.WriteString(span.Text)
	}
	return b.String()
}

// DescriptionPoint is one sentence of a class or race pitch. Skill (a skill
// key) shows it only to a hero whose kit has that skill.
type DescriptionPoint struct {
	Text  string `yaml:"text"`
	Skill string `yaml:"skill,omitempty"`
}

// validateDescription checks every point's markup; required demands at least
// one point. Skill keys are checked by the character package, which owns them.
func validateDescription(path string, points []DescriptionPoint, required bool) error {
	if required && len(points) == 0 {
		return fmt.Errorf("%s.description is required", path)
	}
	for i, p := range points {
		if strings.TrimSpace(p.Text) == "" {
			return fmt.Errorf("%s.description[%d]: text is required", path, i)
		}
		if _, err := ParseKeywordMarkup(p.Text); err != nil {
			return fmt.Errorf("%s.description[%d]: %v", path, i, err)
		}
	}
	return nil
}

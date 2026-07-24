package damage

import (
	"fmt"
	"strings"
)

// Type is the canonical damage-school key at both YAML and runtime boundaries.
type Type string

const (
	Physical Type = "physical"
	Fire     Type = "fire"
	Water    Type = "water"
	Air      Type = "air"
	Earth    Type = "earth"
	Spirit   Type = "spirit"
	Mind     Type = "mind"
	Body     Type = "body"
	Light    Type = "light"
	Dark     Type = "dark"
)

var types = [...]Type{
	Physical,
	Fire,
	Water,
	Air,
	Earth,
	Spirit,
	Mind,
	Body,
	Light,
	Dark,
}

func (t Type) String() string {
	return string(t)
}

// Types returns a copy of the closed damage-school catalog.
func Types() []Type {
	return append([]Type(nil), types[:]...)
}

// ParseType validates and normalizes an external school key.
func ParseType(school string) (Type, error) {
	normalized := strings.ToLower(strings.TrimSpace(school))
	for _, damageType := range types {
		if normalized == damageType.String() {
			return damageType, nil
		}
	}
	return Physical, fmt.Errorf("unknown damage type: %s", school)
}

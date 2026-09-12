package stats

import "strings"

// ScalingTerm is one authored stat contribution, shared by calculations and
// formula displays. Stat uses the display name (for example, "Intellect").
type ScalingTerm struct {
	Stat    string
	Divisor int
}

type Contribution struct {
	ScalingTerm
	Value int
	Bonus int
}

func (term ScalingTerm) Evaluate(value int) Contribution {
	out := Contribution{ScalingTerm: term, Value: value}
	if term.Divisor > 0 {
		out.Bonus = value / term.Divisor
	}
	return out
}

// Formula describes the terms that actually participate in a calculation.
// A mastery ladder replaces all other terms; it is not an additional bonus.
type Formula struct {
	Base           int
	Terms          []ScalingTerm
	MasteryPerTier int
	MasteryLadder  []int
}

type Breakdown struct {
	Base      int
	Terms     []Contribution
	StatBonus int
	Mastery   int
	Total     int
}

func (f Formula) Evaluate(values StatBonuses, tier int) Breakdown {
	tier = max(0, min(3, tier))
	if len(f.MasteryLadder) == 4 {
		return Breakdown{Base: f.MasteryLadder[tier], Total: f.MasteryLadder[tier]}
	}
	out := Breakdown{Base: f.Base, Mastery: tier * f.MasteryPerTier}
	for _, term := range f.Terms {
		part := term.Evaluate(values.ValueByName(strings.ToLower(term.Stat)))
		out.Terms = append(out.Terms, part)
		out.StatBonus += part.Bonus
	}
	out.Total = out.Base + out.StatBonus + out.Mastery
	return out
}

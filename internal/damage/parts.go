// Package damage defines the target-independent parts of one combat hit.
package damage

// Parts keeps mitigable and true damage separate until the target applies its
// defenses. Both components carry the attack's school: resistance affects both,
// while armor and flat reductions affect Normal only.
type Parts struct {
	Normal int
	True   int
}

// Total returns the damage dealt after the target has mitigated each component.
func (p Parts) Total() int {
	return p.Normal + p.True
}

// ApplyResistance applies one school's resistance to a damage amount.
// Positive resistance may be pierced; negative resistance remains a
// vulnerability and intentionally amplifies both normal and true damage.
func ApplyResistance(amount, resistance, piercePct int) int {
	if amount <= 0 {
		return 0
	}
	if piercePct < 0 {
		piercePct = 0
	} else if piercePct > 100 {
		piercePct = 100
	}
	if resistance > 0 && piercePct > 0 {
		resistance = resistance * (100 - piercePct) / 100
	}
	amount = amount * (100 - resistance) / 100
	if amount < 0 {
		return 0
	}
	return amount
}

// ApplyResistance applies the same attack school to both hit components.
func (p Parts) ApplyResistance(resistance, piercePct int) Parts {
	return Parts{
		Normal: ApplyResistance(p.Normal, resistance, piercePct),
		True:   ApplyResistance(p.True, resistance, piercePct),
	}
}

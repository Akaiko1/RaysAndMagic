package status

import "math"

// Accrue adds amount to a fractional carry and returns the whole units the
// total completes, keeping the remainder in the carry: the ONE accumulator for
// every fractional quantity (TB swings and steps, field damage, distance
// charges, pulse phases). unit is one whole unit: 1 for counts and damage, a
// tile for distance. A total short of a unit by float noise still counts.
func Accrue(carry *float64, amount, unit float64) int {
	if unit <= 0 {
		return 0
	}
	total := *carry + amount
	n := int(math.Floor(total/unit + 1e-9))
	*carry = max(0, total-float64(n)*unit)
	return n
}

// AccrueNearest is Accrue rounded to the nearest unit: the carry stays within
// [-0.5, 0.5), a positive carry a fraction already earned, a negative one a
// fraction already paid in advance.
func AccrueNearest(carry *float64, amount float64) int {
	shifted := *carry + 0.5
	n := Accrue(&shifted, amount, 1)
	*carry = shifted - 0.5
	return n
}

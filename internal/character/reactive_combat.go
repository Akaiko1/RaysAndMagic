package character

// ReactiveCombatState belongs to one hero, including while benched or saved.
// No timers: charges change only on the corresponding combat events.
type ReactiveCombatState struct {
	DodgeCharges   int    `json:"dodge_charges,omitempty"`
	Shell          int    `json:"shell,omitempty"`
	LastAttackerID string `json:"last_attacker_id,omitempty"`
	RepeatedHits   int    `json:"repeated_hits,omitempty"`
}

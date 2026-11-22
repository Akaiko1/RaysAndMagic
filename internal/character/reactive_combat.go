package character

// ReactiveCombatState belongs to one hero, including while benched or saved.
// ActorID is stable through death and revival; combat charges have no timers.
type ReactiveCombatState struct {
	DodgeCharges   int    `json:"dodge_charges,omitempty"`
	Shell          int    `json:"shell,omitempty"`
	LastAttackerID string `json:"last_attacker_id,omitempty"`
	RepeatedHits   int    `json:"repeated_hits,omitempty"`
}

func (c *MMCharacter) ClearReactiveCombat() {
	c.ReactiveCombat = ReactiveCombatState{}
}

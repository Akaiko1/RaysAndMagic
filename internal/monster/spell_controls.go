package monster

// IsAfraid reports Fear's forced retreat.
func (m *Monster3D) IsAfraid() bool { return m != nil && m.FearFramesRemaining > 0 }

// IsSilenced reports the Feeblemind lock on spells, ranged bolts and boss specials.
func (m *Monster3D) IsSilenced() bool { return m != nil && m.SilenceFramesRemaining > 0 }

// ControlDRActive holds the shared stun/control DR memory until the target is
// free of stun and fear. Both scheduler clocks use this predicate before recovery.
func (m *Monster3D) ControlDRActive() bool {
	return m.StunFramesRemaining > 0 || m.StunTurnsRemaining > 0 || m.IsAfraid()
}

// AdvanceSpellControls shares the game's elapsed combat clock in RT and TB.
// No frame-local AI callback consumes these timers, so pausing and idle TB
// frames cannot spend a control effect.
func (m *Monster3D) AdvanceSpellControls(elapsed int) {
	if elapsed <= 0 {
		return
	}
	wasAfraid := m.IsAfraid()
	m.FearFramesRemaining = max(0, m.FearFramesRemaining-elapsed)
	unsilencedElapsed := max(0, elapsed-m.SilenceFramesRemaining)
	m.SilenceFramesRemaining = max(0, m.SilenceFramesRemaining-elapsed)
	m.SilenceImmunityFramesRemaining = max(0, m.SilenceImmunityFramesRemaining-unsilencedElapsed)
	m.BerserkFramesRemaining = max(0, m.BerserkFramesRemaining-elapsed)
	if m.IsAlive() && wasAfraid && !m.IsAfraid() {
		m.State = StateAlert
		m.StateTimer = 0
		m.ResetPathfinding()
	}
	if m.IsAlive() && m.IsAfraid() {
		m.State = StateFleeing
	}
}

// ApplyFear overrides ordinary pursuit without changing faction or loot.
func (m *Monster3D) ApplyFear(frames int) {
	m.FearFramesRemaining = max(m.FearFramesRemaining, frames)
	m.State, m.StateTimer = StateFleeing, 0
	m.AIFoe = nil
	m.ResetPathfinding()
}

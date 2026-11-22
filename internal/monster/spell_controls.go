package monster

// IsAfraid reports Fear's forced retreat.
func (m *Monster3D) IsAfraid() bool { return m != nil && m.Fear.Active() }

// IsSilenced reports the Feeblemind lock on spells, ranged bolts and boss specials.
func (m *Monster3D) IsSilenced() bool { return m != nil && m.Silence.Active() }

// ControlDRActive holds the shared stun/control DR memory until the target is
// free of stun and fear. Both scheduler clocks use this predicate before recovery.
func (m *Monster3D) ControlDRActive() bool {
	return m.Stun.Active() || m.IsAfraid()
}

// AdvanceSpellControls spends one tick of the control clocks: a frame in real
// time, a turn at a turn-based party turn. No frame-local AI callback consumes
// them, so pausing and idle TB frames cannot spend a control effect. The
// feeblemind immunity runs only while the monster is not silenced.
func (m *Monster3D) AdvanceSpellControls(turn bool) {
	wasAfraid := m.IsAfraid()
	m.Fear.Tick(turn)
	if m.Silence.Active() {
		m.Silence.Tick(turn)
	} else {
		m.SilenceImmunity.Tick(turn)
	}
	m.Berserk.Tick(turn)
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
	m.Fear.ArmFrames(frames, m.tps())
	m.State, m.StateTimer = StateFleeing, 0
	m.AIFoe = nil
	m.ResetPathfinding()
}

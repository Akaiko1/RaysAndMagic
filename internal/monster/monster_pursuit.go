package monster

import (
	"math"
	"ugataima/internal/status"
)

// PartyAggroContext is bound to an actor's physical world. Queries read current
// logical party coordinates, including movement before the next AI update.
type PartyAggroContext interface {
	PartyScope() (present bool, region string)
}

type RetaliationState struct {
	Frames      int     `json:"frames,omitempty"`
	Turns       int     `json:"turns,omitempty"`
	Rate        int     `json:"rate,omitempty"`
	RadiusTiles float64 `json:"radius_tiles,omitempty"`
}

func (m *Monster3D) partyPresent() bool {
	if m.AggroContext == nil {
		return true
	}
	present, _ := m.AggroContext.PartyScope()
	return present
}

func (m *Monster3D) partyInHome() bool {
	if m.AggroContext == nil {
		return true
	}
	present, region := m.AggroContext.PartyScope()
	return present && (m.HomeMap == "" || region == "" || m.HomeMap == region)
}

func (m *Monster3D) PartyOutsideHome() bool { return !m.partyInHome() }

func (m *Monster3D) partyScopeAllowsPursuit() bool {
	return m.partyInHome() || (m.Retaliation.Frames > 0 && m.partyPresent())
}

func (m *Monster3D) PursuitLimitTiles() float64 {
	// Standalone, unconfigured simulation actors have no authored distance rule.
	// Loaded games validate this required field before constructing monsters.
	if m.config == nil {
		return math.Inf(1)
	}
	return m.config.MonsterAI.Pursuit.MaxRadiusTiles
}

// RetaliateAgainstParty is an incoming party attack event, never a sight check
// or an autonomous poison tick. It grants temporary reach from that hit's range.
func (m *Monster3D) RetaliateAgainstParty(x, y float64) {
	if m.config == nil || !m.partyPresent() || m.IsPartyControlled() || m.IsAmbient() || m.IsInertSetPiece() || m.BossEvasive {
		return
	}
	c := m.config.MonsterAI.Pursuit
	m.WasAttacked = true
	r := &m.Retaliation
	r.RadiusTiles = math.Max(m.PursuitLimitTiles(), math.Hypot(m.X-x, m.Y-y)/m.tileSize()+c.RetaliationMarginTiles)
	status.RefreshDualRated(&r.Frames, &r.Turns, &r.Rate, int(math.Ceil(c.RetaliationSeconds*float64(m.config.GetTPS()))), c.RetaliationTurns)
	if !m.IsEngagingPlayer {
		m.BeginPlayerEngagement()
	}
}

func (m *Monster3D) TickRetaliation(turnBased bool) {
	r := &m.Retaliation
	if turnBased {
		status.TickTurnRated(&r.Turns, &r.Frames, &r.Rate)
	} else {
		status.TickFrameRated(&r.Frames, &r.Turns, &r.Rate)
	}
	if r.Frames <= 0 {
		*r = RetaliationState{}
	}
}

func (m *Monster3D) RestoreRetaliation(saved RetaliationState) {
	m.Retaliation = RetaliationState{}
	if m.config == nil || saved.Frames <= 0 || math.IsNaN(saved.RadiusTiles) || math.IsInf(saved.RadiusTiles, 0) || saved.RadiusTiles <= 0 {
		return
	}
	c := m.config.MonsterAI.Pursuit
	frames := int(math.Ceil(c.RetaliationSeconds * float64(m.config.GetTPS())))
	rate := status.DualRate(frames, c.RetaliationTurns)
	if rate <= 0 {
		return
	}
	saved.Frames = min(saved.Frames, frames)
	saved.Rate = rate
	saved.Turns = (saved.Frames + rate - 1) / rate
	m.Retaliation = saved
}

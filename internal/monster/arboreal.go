package monster

import (
	"fmt"
	"math"
	"math/rand"

	"ugataima/internal/config"
)

// ArborealConfig permits only authored trees, never arbitrary walls or water.
type ArborealConfig struct {
	TreeTiles      []string `yaml:"tree_tiles"`
	HeightTiles    float64  `yaml:"height_tiles"`
	JumpRangeTiles float64  `yaml:"jump_range_tiles"`
	ClimbSeconds   float64  `yaml:"climb_seconds"`
	JumpSeconds    float64  `yaml:"jump_seconds"`
	RestSeconds    float64  `yaml:"rest_seconds"`
}

func (a *ArborealConfig) validate() error {
	if len(a.TreeTiles) == 0 {
		return fmt.Errorf("arboreal movement needs tree_tiles")
	}
	for _, v := range []float64{a.HeightTiles, a.JumpRangeTiles, a.ClimbSeconds, a.JumpSeconds, a.RestSeconds} {
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("arboreal movement values must be finite and positive")
		}
	}
	if a.JumpRangeTiles > 8 || a.HeightTiles > 4 {
		return fmt.Errorf("arboreal height/range exceeds supported tree bounds")
	}
	return nil
}

// ArborealState is actor-owned and saved with coordinates in the actor's map.
// Ground is a reachable landing tile beside the current destination tree.
type ArborealState struct {
	Phase            string  `json:"phase,omitempty"`
	Progress         float64 `json:"progress,omitempty"`
	Height           float64 `json:"height,omitempty"`
	FromHeight       float64 `json:"from_height,omitempty"`
	FromX, FromY     float64
	ToX, ToY         float64
	GroundX, GroundY float64
	HoldSeconds      float64 `json:"hold_seconds,omitempty"`
	Hops             int     `json:"hops,omitempty"`
}

func (s ArborealState) MapPositions(f func(float64, float64) (float64, float64)) ArborealState {
	if s.Phase != "" {
		s.FromX, s.FromY = f(s.FromX, s.FromY)
		s.ToX, s.ToY = f(s.ToX, s.ToY)
		s.GroundX, s.GroundY = f(s.GroundX, s.GroundY)
	}
	return s
}

func (m *Monster3D) ArborealAnimation() (string, float64) {
	if m.Arboreal == nil {
		return "", 0
	}
	return m.Arbor.Phase, m.Arbor.Progress
}

func (m *Monster3D) arborealInBounds(x, y float64) bool {
	b := m.AmbientBounds
	t := m.tileSize()
	return b == nil || (x >= float64(b[0])*t && y >= float64(b[1])*t && x < float64(b[2])*t && y < float64(b[3])*t)
}

func (m *Monster3D) climbable(checker CollisionChecker, x, y float64) bool {
	return m.arborealInBounds(x, y) &&
		!checker.CanOccupyTilesWithTileOverrides(m.ID, x, y, nil, false) &&
		checker.CanOccupyTilesWithTileOverrides(m.ID, x, y, m.Arboreal.TreeTiles, false)
}

func (m *Monster3D) arborealRouteClear(c CollisionChecker, x, y float64) bool {
	n := max(1, int(math.Ceil(math.Hypot(x-m.X, y-m.Y)/(m.tileSize()/8))))
	for i := 1; i <= n; i++ {
		p := float64(i) / float64(n)
		px, py := m.X+(x-m.X)*p, m.Y+(y-m.Y)*p
		if !m.arborealInBounds(px, py) || !c.CanOccupyTilesWithTileOverrides(m.ID, px, py, m.Arboreal.TreeTiles, false) {
			return false
		}
	}
	return true
}

func (m *Monster3D) arborealLanding(c CollisionChecker, x, y float64) (float64, float64, bool) {
	t := m.tileSize()
	for _, d := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		gx, gy := x+d[0]*t, y+d[1]*t
		if m.arborealInBounds(gx, gy) && c.CanMoveToWithTileOverrides(m.ID, gx, gy, nil, false) && m.arborealRouteClear(c, gx, gy) {
			return gx, gy, true
		}
	}
	return 0, 0, false
}

func (m *Monster3D) chooseTree(c CollisionChecker, reach, tx, ty float64) (x, y, gx, gy float64, found bool) {
	t := m.tileSize()
	cx, cy := int(m.X/t), int(m.Y/t)
	radius := int(math.Ceil(reach))
	best := math.Inf(-1)
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			nx, ny := (float64(cx+dx)+.5)*t, (float64(cy+dy)+.5)*t
			d := math.Hypot(nx-m.X, ny-m.Y)
			if d < .5*t || d > reach*t || !m.climbable(c, nx, ny) {
				continue
			}
			lx, ly, ok := m.arborealLanding(c, nx, ny)
			if !ok || !m.arborealRouteClear(c, nx, ny) {
				continue
			}
			score := rand.Float64()
			if m.AmbientFlee {
				score = math.Hypot(nx-tx, ny-ty) - math.Hypot(m.X-tx, m.Y-ty)
				if score <= 0 {
					continue
				}
			}
			if score > best {
				best, x, y, gx, gy, found = score, nx, ny, lx, ly, true
			}
		}
	}
	return
}

func (m *Monster3D) startArboreal(phase string, x, y float64) {
	s := &m.Arbor
	s.Phase, s.Progress = phase, 0
	s.FromX, s.FromY, s.FromHeight = m.X, m.Y, s.Height
	s.ToX, s.ToY = x, y
	m.Direction = math.Atan2(y-m.Y, x-m.X)
	m.clearMoveTarget()
	m.PathTiles = nil
}

// updateArboreal shares one progression clock across RT and tile-based turns.
// A stopped or stunned animal never advances height independently of its body.
func (m *Monster3D) updateArboreal(c CollisionChecker, tx, ty float64, turn bool) bool {
	a := m.Arboreal
	if a == nil {
		return false
	}
	s := &m.Arbor
	// A saved canopy position can become a wall after a map edit. Ordinary
	// route checks cannot escape that starting tile; reuse bounded recovery.
	if s.Phase != "" && (!m.arborealInBounds(m.X, m.Y) || !c.CanOccupyTilesWithTileOverrides(m.ID, m.X, m.Y, a.TreeTiles, false)) {
		if m.unstuckFromObstaclesWithin(c, m.arborealInBounds) {
			*s = ArborealState{HoldSeconds: a.RestSeconds}
			m.ResetPathfinding()
		}
		return true
	}
	tps := config.GetTargetTPS()
	if m.config != nil {
		tps = m.config.GetTPS()
	}
	dt := 1 / float64(max(1, tps))
	if turn {
		dt = m.tileSize() / (60 * math.Max(.01, m.Speed))
		// Turn move credit already applies Slow before this method is called.
	} else {
		dt *= m.EffectiveSpeed() / math.Max(.01, m.Speed)
	}
	if s.Phase == "" {
		s.HoldSeconds = math.Max(0, s.HoldSeconds-dt)
		if s.HoldSeconds > 0 {
			return false
		}
		x, y, gx, gy, ok := m.chooseTree(c, 1.5, tx, ty)
		if !ok {
			s.HoldSeconds = .5
			return false
		}
		s.GroundX, s.GroundY = gx, gy
		m.startArboreal("climbing", x, y)
	}
	// A removed tree or changed route cannot leave a permanent hovering actor.
	if s.Phase != "descending" && (!m.climbable(c, s.ToX, s.ToY) || !m.arborealRouteClear(c, s.ToX, s.ToY)) {
		m.startArboreal("descending", s.GroundX, s.GroundY)
	}
	if s.Phase == "perched" {
		m.State = StateIdle
		s.HoldSeconds -= dt
		if s.HoldSeconds > 0 && !m.AmbientFlee {
			return true
		}
		if m.AmbientFlee || s.Hops < 2 {
			if x, y, gx, gy, ok := m.chooseTree(c, a.JumpRangeTiles, tx, ty); ok {
				s.GroundX, s.GroundY = gx, gy
				s.Hops++
				m.startArboreal("jumping", x, y)
				return true
			}
		}
		m.startArboreal("descending", s.GroundX, s.GroundY)
	}
	if s.Phase == "descending" && (!m.arborealInBounds(s.ToX, s.ToY) || !c.CanMoveToWithTileOverrides(m.ID, s.ToX, s.ToY, nil, false) || !m.arborealRouteClear(c, s.ToX, s.ToY)) {
		if x, y, ok := m.arborealLanding(c, m.X, m.Y); ok {
			m.startArboreal("descending", x, y)
		} else {
			return true
		}
	}
	m.State = StatePatrolling
	if m.AmbientFlee {
		m.State = StateFleeing
	}
	duration := a.ClimbSeconds
	if s.Phase == "jumping" {
		duration = a.JumpSeconds
	}
	p := math.Min(1, s.Progress+dt/duration)
	nx, ny := s.FromX+(s.ToX-s.FromX)*p, s.FromY+(s.ToY-s.FromY)*p
	if !m.arborealRouteClear(c, nx, ny) {
		return true
	}
	s.Progress, m.X, m.Y = p, nx, ny
	switch s.Phase {
	case "climbing":
		s.Height = a.HeightTiles * p
	case "jumping":
		s.Height = a.HeightTiles + 4*.35*p*(1-p)
	case "descending":
		s.Height = s.FromHeight * (1 - p)
	}
	if p >= 1 {
		if s.Phase == "descending" {
			*s = ArborealState{HoldSeconds: a.RestSeconds}
		} else {
			s.Phase, s.Progress, s.Height = "perched", 0, a.HeightTiles
			s.HoldSeconds = a.RestSeconds
		}
	}
	return true
}

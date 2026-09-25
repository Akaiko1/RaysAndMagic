package game

import (
	"math"
	"math/rand"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

type tacticalState struct {
	initialized       bool
	x, y              float64
	world             *world.World3D
	stationarySeconds float64
	movedTB           bool
	approach          map[*monster.Monster3D]float64
}

func (g *MMGame) resetOverwatch() {
	g.tactics = tacticalState{movedTB: true}
}

func (g *MMGame) updateTacticalClocks() {
	if g.camera == nil {
		return
	}
	s := &g.tactics
	if !s.initialized || s.world != g.world {
		*s = tacticalState{initialized: true, x: g.camera.X, y: g.camera.Y, world: g.world, movedTB: true, approach: make(map[*monster.Monster3D]float64)}
	}
	if s.x != g.camera.X || s.y != g.camera.Y {
		s.stationarySeconds = 0
		s.movedTB = true
		clear(s.approach)
	} else if !g.turnBasedMode {
		s.stationarySeconds += 1 / float64(g.config.GetTPS())
	}
	s.x, s.y = g.camera.X, g.camera.Y
	for m := range s.approach {
		if !m.IsAlive() || !m.IsEngagingPlayer {
			delete(s.approach, m)
		}
	}
	if !g.turnBasedMode {
		for _, ch := range g.party.Members {
			if ch != nil && ch.DesignationFrames > 0 {
				ch.DesignationFrames--
			}
		}
	}
}

// overwatchReady is the shared firing and HUD indicator predicate.
func (g *MMGame) overwatchReady(ch *character.MMCharacter) bool {
	if g == nil || ch == nil || !ch.CanUseCombatAction() || !ch.HasSkill(character.SkillOverwatch) || !g.tactics.initialized || g.tactics.world != g.world {
		return false
	}
	if g.camera == nil || g.combat == nil {
		return false
	}
	x, y := g.combat.logicalCameraXY()
	if math.Abs(x-g.tactics.x) > 1e-7 || math.Abs(y-g.tactics.y) > 1e-7 {
		return false
	}
	if g.turnBasedMode {
		if g.tactics.movedTB {
			return false
		}
	} else if g.tactics.stationarySeconds+1e-9 < character.OverwatchReadySeconds {
		return false
	}
	weapon, ok := ch.Equipment[items.SlotMainHand]
	return ok && character.BallisticsWeapon(lookupWeaponConfigByName(weapon.Name)) && g.combat != nil && !g.combat.partyInsideSolidTerrain()
}

// observeOverwatchMovement runs only after serial publication of an AI walk.
// Count actual approach distance, not grid-boundary jitter or party movement.
func (g *MMGame) observeOverwatchMovement(m *monster.Monster3D, oldX, oldY float64) {
	if m == nil || !m.IsAlive() || !m.IsEngagingPlayer || m.Bound || m.Pacified || m.AIFoe != nil || m.IsDamageInvulnerable() {
		return
	}
	ready := false
	for _, ch := range g.party.Members {
		if g.overwatchReady(ch) {
			ready = true
			break
		}
	}
	if !ready {
		delete(g.tactics.approach, m)
		return
	}
	tile := float64(g.config.GetTileSize())
	distance := math.Max(math.Abs(m.X-oldX), math.Abs(m.Y-oldY))
	if distance == 0 {
		return
	}
	if distance > tile*1.5 || math.Hypot(m.X-g.camera.X, m.Y-g.camera.Y) >= math.Hypot(oldX-g.camera.X, oldY-g.camera.Y) {
		delete(g.tactics.approach, m)
		return
	}
	if g.tactics.approach == nil {
		g.tactics.approach = make(map[*monster.Monster3D]float64)
	}
	g.tactics.approach[m] += distance
	for g.tactics.approach[m]+1e-7 >= tile {
		g.tactics.approach[m] -= tile
		g.tryOverwatchReaction(m, 1)
	}
}

// observeOverwatchAttack runs once after an enemy's committed attack action,
// not on animation ticks, failed attempts, or every projectile in a volley.
func (g *MMGame) observeOverwatchAttack(m *monster.Monster3D) {
	g.tryOverwatchReaction(m, character.OverwatchAttackChanceScale)
}

func (g *MMGame) tryOverwatchReaction(m *monster.Monster3D, chanceScale float64) {
	if m == nil || !m.IsAlive() || !m.IsEngagingPlayer || m.IsPartyControlled() || m.AIFoe != nil || m.IsDamageInvulnerable() {
		return
	}
	for index, ch := range g.party.Members {
		if !g.overwatchReady(ch) {
			continue
		}
		def := lookupWeaponConfigByName(ch.Equipment[items.SlotMainHand].Name)
		reach, _ := character.EffectiveWeaponFlight(def, ch)
		x, y := g.combat.logicalCameraXY()
		if Distance(x, y, m.X, m.Y) > reach*float64(g.config.GetTileSize()) || !g.combat.attackLineClear(x, y, m.X, m.Y) {
			continue
		}
		chance := float64(character.OverwatchChancePct(ch.SkillTier(character.SkillOverwatch))) * chanceScale / 100
		roll := rand.Float64
		if g.combat.reactionRoll != nil {
			roll = g.combat.reactionRoll
		}
		if chance <= 0 || roll() >= chance {
			continue
		}
		g.fireOverwatch(index, m)
	}
}

func (g *MMGame) fireOverwatch(index int, target *monster.Monster3D) bool {
	// The normal attack pipeline still owns all damage, volley and card effects.
	// Selection is restored before any UI or other actor runs.
	previous := g.selectedChar
	g.selectedChar = index
	defer func() { g.selectedChar = previous }()
	first := len(g.arrows)
	if !g.combat.equipmentAttackAtAngle(math.Atan2(target.Y-g.camera.Y, target.X-g.camera.X), true) {
		return false
	}
	for i := first; i < len(g.arrows); i++ {
		g.arrows[i].Overwatch = true
	}
	g.AddCombatMessage(uitext.Text("combat.overwatch_fires", g.party.Members[index].Name, target.Name))
	return true
}

func (g *MMGame) designateTarget(ch *character.MMCharacter, target *monster.Monster3D) {
	if ch == nil || !ch.HasSkill(character.SkillDesignateTarget) || !g.isPartyMember(ch) || !target.IsAlive() {
		return
	}
	ch.DesignatedTargetID = target.ID
	ch.DesignationFrames = character.DesignationSeconds(ch.SkillTier(character.SkillDesignateTarget)) * g.config.GetTPS()
}

// designationBonus is shared by weapon damage and the visible target marker.
func (g *MMGame) designationBonus(target *monster.Monster3D) int {
	if target == nil || !target.IsAlive() || target.ID == "" || g.party == nil {
		return 0
	}
	bonus := 0
	for _, ch := range g.party.Members {
		if ch != nil && ch.DesignatedTargetID == target.ID {
			bonus = max(bonus, activeDesignationBonus(ch))
		}
	}
	return bonus
}

func activeDesignationBonus(ch *character.MMCharacter) int {
	if ch == nil || !ch.HasSkill(character.SkillDesignateTarget) || !ch.CanUseCombatAction() || ch.DesignationFrames <= 0 || ch.DesignatedTargetID == "" {
		return 0
	}
	return character.DesignationCritPct(ch.SkillTier(character.SkillDesignateTarget))
}

// Snapshot once per impact: a ranged hit can replace its owner's mark before
// splash resolves, but that new mark must only benefit subsequent attacks.
func (g *MMGame) designationBonuses() map[string]int {
	var bonuses map[string]int
	if g.party == nil {
		return nil
	}
	for _, ch := range g.party.Members {
		if bonus := activeDesignationBonus(ch); bonus > 0 {
			if bonuses == nil {
				bonuses = make(map[string]int)
			}
			bonuses[ch.DesignatedTargetID] = max(bonuses[ch.DesignatedTargetID], bonus)
		}
	}
	return bonuses
}

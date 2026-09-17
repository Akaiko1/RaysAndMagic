package game

import (
	"math"
	"math/rand"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
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
	} else if g.tactics.stationarySeconds+1e-9 < g.config.Characters.Tactics.OverwatchReadySeconds {
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
		for index, ch := range g.party.Members {
			if !g.overwatchReady(ch) {
				continue
			}
			def := lookupWeaponConfigByName(ch.Equipment[items.SlotMainHand].Name)
			rangeTiles, _ := character.EffectiveWeaponFlight(def, ch)
			if Distance(g.camera.X, g.camera.Y, m.X, m.Y) > rangeTiles*tile || !g.combat.attackLineClear(g.camera.X, g.camera.Y, m.X, m.Y) {
				continue
			}
			if rand.Intn(100) >= ch.TacticalSkillValue(character.SkillOverwatch, g.config.Characters.Tactics.OverwatchChance) {
				continue
			}
			g.fireOverwatch(index, m)
		}
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
	ch.DesignationFrames = ch.TacticalSkillValue(character.SkillDesignateTarget, config.TacticalSkills().DesignationSeconds) * g.config.GetTPS()
}

// designationBonus is shared by weapon damage and the visible target marker.
func (g *MMGame) designationBonus(target *monster.Monster3D) int {
	if target == nil || !target.IsAlive() || target.ID == "" || g.party == nil {
		return 0
	}
	bonus := 0
	for _, ch := range g.party.Members {
		if ch != nil && ch.CanUseCombatAction() && ch.DesignationFrames > 0 && ch.DesignatedTargetID == target.ID {
			bonus = max(bonus, ch.TacticalSkillValue(character.SkillDesignateTarget, config.TacticalSkills().DesignationCritPct))
		}
	}
	return bonus
}

// A second conditional roll adds percentage points to a launch-time crit roll
// without re-rolling it or granting marks to spells and secondary proc bolts.
func (cs *CombatSystem) designatedCritical(target *monster.Monster3D, damage int, crit bool, baseChance int) (int, bool) {
	if crit {
		return damage, crit
	}
	bonus := cs.game.designationBonus(target)
	if bonus > 0 && rand.Intn(max(1, 100-baseChance)) < bonus {
		return weaponCriticalDamage(damage, true), true
	}
	return damage, false
}

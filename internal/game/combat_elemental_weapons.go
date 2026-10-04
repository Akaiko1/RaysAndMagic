package game

import (
	"math"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
)

func elementalAbilityDamage(def *config.WeaponDefinitionConfig, attacker *character.MMCharacter) int {
	if def == nil || def.ElementalAbility == nil {
		return 0
	}
	a := def.ElementalAbility
	damage := a.Damage
	if attacker != nil && a.ScalingStat != "" {
		_, intellect, _, _, _, speed, _ := attacker.GetEffectiveStats()
		stat := intellect
		if a.ScalingStat == "Speed" {
			stat = speed
		}
		damage += max(0, stat) / a.StatDivisor
	}
	return damage
}

// Direct, landed party weapon hits alone can charge marks. Projectile scaling
// is captured at launch; a secondary charge never re-enters this hook.
func (cs *CombatSystem) applyElementalWeaponAbility(target *monster.Monster3D, def *config.WeaponDefinitionConfig, attacker *character.MMCharacter, shot *Arrow) {
	if def == nil || def.ElementalAbility == nil || target == nil || target.Bound || isPurePartySummon(target) || target.IsDamageInvulnerable() {
		return
	}
	a, g := def.ElementalAbility, cs.game
	damage := elementalAbilityDamage(def, attacker)
	if shot != nil {
		damage = shot.ElementalAbilityDamage
	}
	if a.Kind == "backwash" {
		if shot == nil {
			return
		}
		if target.IsAlive() {
			cs.elementalWeaponBonus(target, damage, def.DamageType, "Backwash")
		}
		cs.spawnBackwashCharge(target, def, shot, map[string]int{target.ID: 1})
		return
	}
	if !target.IsAlive() {
		return
	}
	frames, turns := weaponStatusClocks(a.DurationSeconds, g.config.GetTPS())
	mark := target.ElementalMarks[a.Kind]
	if mark.Frames <= 0 || mark.Turns <= 0 {
		mark.Count = 0
	}
	mark.Count++
	if a.Kind == "anchor" {
		mark.Count = min(mark.Count, a.MaxStacks)
		target.SetElementalMark(a.Kind, mark, frames, turns)
		cs.elementalWeaponBonus(target, damage*mark.Count, def.DamageType, "Stored Load")
		return
	}
	if mark.Count < a.Hits {
		target.SetElementalMark(a.Kind, mark, frames, turns)
		return
	}
	delete(target.ElementalMarks, a.Kind)
	label := "Pressure Vent"
	if a.Kind == "crosswind" {
		label = "Transfer"
		cs.pushElementalTarget(target, a.RangeTiles)
	}
	cs.elementalWeaponBonus(target, damage, def.DamageType, label)
}

// A chain owns one hit ledger. Only its current, single projectile carries it;
// subsequent legs use the normal projectile movement and wall collision path.
type backwashCharge struct {
	Hits             map[string]int
	Target           *monster.Monster3D
	OriginX, OriginY float64
}

// Retarget serially before projectile workers run. Losing range, sight or the
// target ends the charge; it cannot fly through a newly closed door.
func (cs *CombatSystem) prepareBackwashCharge(shot *Arrow) {
	if shot.Backwash == nil || !shot.Active {
		return
	}
	c := shot.Backwash
	def := lookupWeaponConfigByKey(shot.BowKey)
	if !cs.backwashTargetEligible(c.OriginX, c.OriginY, c.Target, def.ElementalAbility, c.Hits) {
		shot.Active = false
		cs.game.collisionSystem.UnregisterEntity(shot.ID)
		return
	}
	angle := math.Atan2(c.Target.Y-shot.Y, c.Target.X-shot.X)
	speed := math.Min(def.Physics.GetSpeedPixels(cs.game.config.GetTileSize()), Distance(shot.X, shot.Y, c.Target.X, c.Target.Y))
	shot.VelX, shot.VelY = math.Cos(angle)*speed, math.Sin(angle)*speed
}

func (cs *CombatSystem) backwashTargetEligible(fromX, fromY float64, target *monster.Monster3D, a *config.ElementalWeaponAbility, hits map[string]int) bool {
	return target != nil && target.IsAlive() && !target.Bound && !isExcludedFromPartyAutoTarget(target) &&
		!target.IsDamageInvulnerable() && hits[target.ID] < a.HitsPerTarget &&
		Distance(fromX, fromY, target.X, target.Y) < a.RangeTiles*float64(cs.game.config.GetTileSize()) &&
		cs.attackLineClear(fromX, fromY, target.X, target.Y)
}

func (cs *CombatSystem) spawnBackwashCharge(from *monster.Monster3D, def *config.WeaponDefinitionConfig, shot *Arrow, hits map[string]int) {
	g := cs.game
	if g.world == nil || g.collisionSystem == nil {
		return
	}
	var next *monster.Monster3D
	best := math.Inf(1)
	for _, candidate := range g.world.Monsters {
		if candidate == from || !cs.backwashTargetEligible(from.X, from.Y, candidate, def.ElementalAbility, hits) {
			continue
		}
		d := Distance(from.X, from.Y, candidate.X, candidate.Y)
		if d < best || (d == best && next != nil && candidate.ID < next.ID) {
			next, best = candidate, d
		}
	}
	if next == nil {
		return
	}
	speed := def.Physics.GetSpeedPixels(g.config.GetTileSize())
	angle := math.Atan2(next.Y-from.Y, next.X-from.X)
	cs.spawnArrowContinuation(Arrow{
		X: from.X, Y: from.Y, VelX: math.Cos(angle) * speed, VelY: math.Sin(angle) * speed,
		LifeTime: int(math.Ceil(def.ElementalAbility.RangeTiles * float64(g.config.GetTileSize()) / speed)),
		BowKey:   shot.BowKey, Label: "Backwash", DamageType: def.DamageType,
		ElementalAbilityDamage: shot.ElementalAbilityDamage, Attacker: shot.Attacker,
		Owner: ProjectileOwnerPlayer, WorldAim: true, SkipMonster: from,
		Backwash: &backwashCharge{Hits: hits, Target: next, OriginX: from.X, OriginY: from.Y},
	}, def)
}

// Resolve only the charge damage, never weapon procs, criticals or splash.
func (cs *CombatSystem) resolveBackwashCharge(shot *Arrow, target *monster.Monster3D) {
	if !shot.Active || shot.LifeTime <= 0 {
		return
	}
	shot.Active = false
	cs.game.collisionSystem.UnregisterEntity(shot.ID)
	def := lookupWeaponConfigByKey(shot.BowKey)
	c := shot.Backwash
	if target != c.Target || !cs.backwashTargetEligible(c.OriginX, c.OriginY, target, def.ElementalAbility, c.Hits) {
		return
	}
	c.Hits[target.ID]++
	cs.elementalWeaponBonus(target, shot.ElementalAbilityDamage, def.DamageType, "Backwash")
	cs.spawnBackwashCharge(target, def, shot, c.Hits)
	if !target.IsAlive() {
		cs.finishWeaponKill(target, nil, shot.Attacker)
	}
}

// Sweep in short increments so a three-tile push cannot skip a wall, arena
// boundary or reserved attack post. RT and TB share the same physical rules.
func (cs *CombatSystem) pushElementalTarget(m *monster.Monster3D, tiles float64) {
	g := cs.game
	if g.collisionSystem == nil {
		return
	}
	px, py := cs.logicalCameraXY()
	angle := math.Atan2(m.Y-py, m.X-px)
	tile := float64(g.config.GetTileSize())
	distance := tiles * tile
	x, y := m.X, m.Y
	for moved := 0.; moved < distance; {
		step := math.Min(tile/16, distance-moved)
		nx, ny := x+math.Cos(angle)*step, y+math.Sin(angle)*step
		if g.collisionSystem.IsMonsterAttackPostReserved(m.ID, nx, ny) ||
			!g.collisionSystem.CanMoveToWithTileOverrides(m.ID, nx, ny, m.WalkableTileOverrides, m.Flying) {
			break
		}
		x, y, moved = nx, ny, moved+step
	}
	if x != m.X || y != m.Y {
		g.releaseMonsterAttackPost(m)
		m.X, m.Y = x, y
		m.ResetPathfinding()
		g.collisionSystem.UpdateEntity(m.ID, x, y)
		g.refreshMonsterCollisionState(m)
	}
}

// A bonus is a separate resisted elemental impact, without recursive riders,
// crit rolls, or a second kill settlement for the primary target.
func (cs *CombatSystem) elementalWeaponBonus(target *monster.Monster3D, amount int, school, label string) {
	parts := cs.game.elementalDamageBuff(damagecalc.Parts{Normal: amount}, school)
	dealt := cs.applyMonsterDamagePacket(target, singleMonsterDamagePacket(parts, school, 0), monsterDamageOptions{}).Total()
	cs.markMonsterHit(target)
	x, y := cs.monsterVisualPos(target)
	cs.game.createElementalAbilityImpact(x, y, school)
	cs.game.logCombat(logToneGood, "%s hits %s for %s damage!", logSchoolWord(school, label), logMonsterName(target), logDamage(dealt, school))
}

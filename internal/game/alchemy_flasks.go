package game

import (
	"fmt"
	"math"
	"slices"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

func flaskDefinition(item items.Item) (string, *config.ItemDefinitionConfig) {
	d, k, ok := config.GetItemDefinitionByName(item.Name)
	if ok && d.Flask != nil {
		return k, d
	}
	return "", nil
}

type flaskSupply struct {
	count int
	bag   character.InventoryBag
	index int
}

// Count, availability and payment use the same acting hero's stock: personal
// bag first, then shared. Another hero's personal bag is never a fallback.
func (g *MMGame) availableFlasks(actor *character.MMCharacter, key string) flaskSupply {
	supply := flaskSupply{index: -1}
	d, ok := config.GetItemDefinition(key)
	if !ok || d.Flask == nil || g.party == nil || actor == nil || !slices.Contains(g.party.Members, actor) {
		return supply
	}
	for _, bag := range [2]character.InventoryBag{g.party.Bag(actor), g.party.Bag()} {
		for i, it := range bag.Items() {
			if it.Type != items.ItemConsumable || it.Name != d.Name {
				continue
			}
			supply.count += it.Count()
			if supply.index < 0 {
				supply.bag, supply.index = bag, i
			}
		}
	}
	return supply
}

func (g *MMGame) flaskStock(actor *character.MMCharacter, key string) int {
	return g.availableFlasks(actor, key).count
}
func (g *MMGame) equipFlask(idx int, key string) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	c := g.party.Members[idx]
	if !c.HasSkill(character.SkillBombThrowing) {
		g.AddCombatMessage("Bomb Throwing is required.")
		return false
	}
	it, ok := config.FlaskItem(key)
	if !ok {
		return false
	}
	c.Equipment[items.SlotSpell] = it
	g.AddCombatMessage(fmt.Sprintf("%s readies %s (%d in bag).", c.Name, it.Name, g.flaskStock(c, key)))
	return true
}
func flaskDamage(c *character.MMCharacter, d *config.FlaskDefinition) int {
	return config.TierValue(d.Damage, c.SkillTier(character.SkillBombThrowing)) + c.GetEffectiveIntellect()/character.BombThrowingIntellectDivisor
}

// flaskAim fixes the arc at launch. Keyboard throws acquire a visible forward
// foe using the shared auto-target and front-slot rules; clicks keep their aim.
// Fly to the real position, so the splash still resolves in world coordinates.
func (cs *CombatSystem) flaskAim(maxRange float64) (angle, distance float64) {
	g := cs.game
	angle, distance = cs.partyAttackAngle(), maxRange
	if target := cs.partyAimTarget; target != nil {
		return angle, min(maxRange, math.Hypot(target.X-g.camera.X, target.Y-g.camera.Y))
	}
	var best *monster.Monster3D
	bestDistance := maxRange
	dirX, dirY := math.Cos(angle), math.Sin(angle)
	cone := min(g.camera.FOV/2, projectileAssistMaxAngleRad)
	for _, target := range g.world.Monsters {
		if isExcludedFromPartyAutoTarget(target) || !target.IsAlive() {
			continue
		}
		d := math.Hypot(target.X-g.camera.X, target.Y-g.camera.Y)
		if d <= 0 || d > bestDistance || !cs.attackLineClear(g.camera.X, g.camera.Y, target.X, target.Y) {
			continue
		}
		x, y := target.X, target.Y
		if _, px, py, pulled, ok := cs.pulledFrontSlot(target); ok && pulled {
			x, y = px, py
		}
		if !headingTowardWithin(g.camera.X, g.camera.Y, dirX, dirY, x, y, cone) {
			continue
		}
		if best == nil || d < bestDistance || (d == bestDistance && target.ID < best.ID) {
			best, bestDistance = target, d
		}
	}
	if best != nil {
		angle = math.Atan2(best.Y-g.camera.Y, best.X-g.camera.X)
		distance = bestDistance
	}
	return angle, distance
}

func (g *MMGame) throwFlask(idx int, key string, announce bool) bool {
	refuse := func(message string) bool {
		if announce {
			g.AddCombatMessage(message)
		}
		return false
	}
	d, ok := config.GetItemDefinition(key)
	if !ok || d.Flask == nil {
		return false
	}
	if idx < 0 || idx >= len(g.party.Members) || !g.canSpendCombatAction(idx) || (!g.turnBasedMode && !g.combatActorAllowed(idx)) {
		return refuse("This hero is not ready.")
	}
	c := g.party.Members[idx]
	if !c.HasSkill(character.SkillBombThrowing) {
		return refuse("Bomb Throwing is required.")
	}
	if g.combat.partyInsideSolidTerrain() || g.collisionSystem == nil {
		return refuse("No room to throw.")
	}
	supply := g.availableFlasks(c, key)
	if supply.index < 0 {
		return refuse("No " + d.Name + " in this hero's bag or the shared bag.")
	}
	f := d.Flask
	tier := c.SkillTier(character.SkillBombThrowing)
	tps := g.config.GetTPS()
	tile := float64(g.config.GetTileSize())
	speed := 8 * tile / float64(tps)
	a, distance := g.combat.flaskAim(float64(f.RangeTiles) * tile)
	p := MagicProjectile{
		ID:                g.GenerateProjectileID("flask"),
		FlaskKey:          key,
		FlaskPoisonFrames: config.TierValue(f.PoisonSeconds, tier) * tps,
		FlaskBurnFrames:   config.TierValue(f.BurnSeconds, tier) * tps,
		FlaskRadius:       float64(f.RadiusTiles) * tile,
		FlaskRemaining:    distance,
		FlaskFlightRange:  distance,
		WorldAim:          true,
		Attacker:          c,
		X:                 g.camera.X,
		Y:                 g.camera.Y,
		VelX:              math.Cos(a) * speed,
		VelY:              math.Sin(a) * speed,
		Damage:            flaskDamage(c, f),
		LifeTime:          10 * tps,
		Active:            true,
		SpellType:         f.Element,
		Size:              12,
		Owner:             ProjectileOwnerPlayer,
	}
	if !supply.bag.Consume(supply.index, 1) {
		return false
	}
	g.magicProjectiles = append(g.magicProjectiles, p)
	g.collisionSystem.RegisterEntity(collision.NewEntity(p.ID, p.X, p.Y, 10, 10, collision.CollisionTypeProjectile, false))
	g.consumeCharacterActionWithRTCooldown(idx, g.combat.SpellCooldownFrames(c, spells.SpellID(key)))
	g.spellInputCooldown = rtActionStagger
	if announce {
		g.AddCombatMessage(c.Name + " throws " + d.Name + ".")
	}
	return true
}

// detonateFlask uses the same immutable damage packet independently per victim.
// It intentionally has no spell, weapon, crit, designation or ricochet dispatcher.
func (cs *CombatSystem) detonateFlask(p *MagicProjectile, x, y float64) {
	if p == nil || p.LifeTime <= 0 {
		return
	}
	d, ok := config.GetItemDefinition(p.FlaskKey)
	if !ok || d.Flask == nil {
		return
	}
	p.Active = false
	p.LifeTime = 0
	if cs.game.collisionSystem != nil {
		cs.game.collisionSystem.UnregisterEntity(p.ID)
	}
	attack := cs.newPartyMonsterAttack(p.Damage, 0, d.Flask.Element, 0, nil, d.Name, false, false, false)
	attack.Packet = cs.newPartyMonsterDamagePacket(p.Damage, 0, d.Flask.Element, 0, false)
	attack.Packet = cs.elementalBuffPacket(attack.Packet)
	attack.Attacker = p.Attacker
	cs.applyAoeSplashAt(x, y, attack, p.FlaskRadius/float64(cs.game.config.GetTileSize()), nil, func(m *monster.Monster3D, actual int) {
		cs.reportIndirectHit(m, actual, d.Name)
		if m.IsAlive() {
			if p.FlaskPoisonFrames > 0 {
				m.ApplyPoison(p.FlaskPoisonFrames)
			}
			if p.FlaskBurnFrames > 0 {
				m.ApplyBurn(p.FlaskBurnFrames)
			}
		}
		cs.finishIndirectKill(m)
	})
	cs.game.createFlaskImpact(p, x, y, d.BrewColor)
}
func (g *MMGame) useSlottedRareAction(idx int, kind rtActionKind, announce bool) (bool, bool) {
	if idx < 0 || idx >= len(g.party.Members) {
		return false, false
	}
	it := g.party.Members[idx].Equipment[items.SlotSpell]
	switch it.Type {
	case items.ItemTechnique:
		d := config.Technique(string(it.SpellEffect))
		if kind != rtActCast || d == nil || d.FreeStep {
			return false, false
		}
		return true, g.useTechnique(idx, string(it.SpellEffect), false, announce)
	case items.ItemThrowable:
		return true, g.throwFlask(idx, string(it.SpellEffect), announce)
	}
	return false, false
}

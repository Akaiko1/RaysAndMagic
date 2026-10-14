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
	c.Equipment[items.SlotQuickAction] = it
	g.AddCombatMessage(fmt.Sprintf("%s readies %s (%d in bag).", c.Name, it.Name, g.flaskStock(c, key)))
	return true
}
func flaskDamage(c *character.MMCharacter, d *config.FlaskDefinition) int {
	return config.TierValue(d.Damage, c.SkillTier(character.SkillBombThrowing)) + c.GetEffectiveIntellect()/character.BombThrowingIntellectDivisor
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
	launch := g.combat.partyProjectileLaunch(g.combat.partyAttackAngle(), float64(f.RangeTiles)*tile, false)
	launch.lobApex = flaskApexTiles * tile
	a, distance := launch.angle, launch.distance
	p := FlaskProjectile{
		Launch:       launch,
		ID:           g.GenerateProjectileID("flask"),
		Key:          key,
		PoisonFrames: config.TierValue(f.PoisonSeconds, tier) * tps,
		BurnFrames:   config.TierValue(f.BurnSeconds, tier) * tps,
		Radius:       float64(f.RadiusTiles) * tile,
		Remaining:    distance,
		Attacker:     c,
		X:            g.camera.X,
		Y:            g.camera.Y,
		VelX:         math.Cos(a) * speed,
		VelY:         math.Sin(a) * speed,
		Damage:       flaskDamage(c, f),
		LifeTime:     10 * tps,
		Active:       true,
	}
	if !supply.bag.Consume(supply.index, 1) {
		return false
	}
	g.flasks = append(g.flasks, p)
	g.collisionSystem.RegisterEntity(collision.NewEntity(p.ID, p.X, p.Y, 10, 10, collision.CollisionTypeProjectile, false))
	g.consumeCharacterActionWithRTCooldown(idx, g.combat.FlaskCooldownFrames(c, key))
	g.actionInputStagger = rtActionStagger
	if announce {
		g.AddCombatMessage(c.Name + " throws " + d.Name + ".")
	}
	return true
}

// detonateFlask uses the same immutable damage packet independently per victim.
// It intentionally has no spell, weapon, crit, designation or ricochet dispatcher.
func (cs *CombatSystem) detonateFlask(p *FlaskProjectile, x, y float64) {
	if p == nil || p.LifeTime <= 0 {
		return
	}
	d, ok := config.GetItemDefinition(p.Key)
	if !ok || d.Flask == nil {
		return
	}
	p.Active = false
	p.LifeTime = 0
	if cs.game.collisionSystem != nil {
		cs.game.collisionSystem.UnregisterEntity(p.ID)
	}
	attack := cs.newPartyMonsterAttack(p.Damage, 0, d.Flask.Element, 0, nil, d.Name, false, false, false)
	attack.Projectile = true // a thrown flask is a projectile: its splash meets the physical pierce rule
	attack.Packet = cs.newPartyMonsterDamagePacket(p.Damage, 0, d.Flask.Element, 0, false)
	attack.Packet = cs.elementalBuffPacket(attack.Packet)
	attack.Attacker = p.Attacker
	cs.applyAoeSplashAt(x, y, attack, p.Radius/float64(cs.game.config.GetTileSize()), nil, func(m *monster.Monster3D, actual int) {
		cs.reportIndirectHit(m, actual, d.Name)
		if m.IsAlive() {
			if p.PoisonFrames > 0 {
				m.ApplyPoison(p.PoisonFrames)
			}
			if p.BurnFrames > 0 {
				m.ApplyBurn(p.BurnFrames)
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
	it := g.party.Members[idx].Equipment[items.SlotQuickAction]
	switch it.Type {
	case items.ItemTechnique:
		d := config.Technique(it.Technique)
		if kind != rtActSlotted || d == nil || d.IsStep() {
			return false, false
		}
		return true, g.useTechnique(idx, it.Technique, false, announce)
	case items.ItemThrowable:
		return true, g.throwFlask(idx, it.Flask, announce)
	}
	return false, false
}

package game

import (
	"math"
	"slices"

	uitext "ugataima/assets/text"
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
	count  int
	bag    character.InventoryBag
	index  int
	device *items.Item
}

// Count, availability and payment use the same acting hero's stock: personal
// bag first, then shared. Another hero's personal bag is never a fallback.
func (g *MMGame) availableFlasks(actor *character.MMCharacter, key string) flaskSupply {
	supply := flaskSupply{index: -1}
	d, ok := config.GetItemDefinition(key)
	if !ok || d.Flask == nil || g.party == nil || actor == nil || !slices.Contains(g.party.Members, actor) {
		return supply
	}
	consider := func(it *items.Item, bag character.InventoryBag, index int) {
		if (it.Type != items.ItemConsumable && !(d.Flask.Reusable && it.Type == items.ItemDevice)) || it.Name != d.Name {
			return
		}
		supply.count += it.Count()
		if supply.index < 0 && supply.device == nil || d.Flask.Reusable && supply.device.DeviceCooldownFrames > 0 && it.DeviceCooldownFrames <= 0 {
			supply.bag, supply.index = bag, index
			if d.Flask.Reusable {
				supply.device = it
			}
		}
	}
	visitBag := func(bag character.InventoryBag) {
		stock := bag.Items()
		for i := range stock {
			consider(&stock[i], bag, i)
		}
	}
	visitBag(g.party.Bag(actor))
	visitBag(g.party.Bag())
	return supply
}

func (g *MMGame) flaskStock(actor *character.MMCharacter, key string) int {
	return g.availableFlasks(actor, key).count
}

func (g *MMGame) flaskReady(actor *character.MMCharacter, key string) bool {
	d, ok := config.GetItemDefinition(key)
	if !ok || d.Flask == nil || actor == nil {
		return false
	}
	if _, missing := actor.FlaskSkillMissing(d.Flask); missing {
		return false
	}
	supply := g.availableFlasks(actor, key)
	return supply.index >= 0 && !d.Flask.Reusable || supply.device != nil && supply.device.DeviceCooldownFrames <= 0
}
func (g *MMGame) equipFlask(idx int, key string) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	c := g.party.Members[idx]
	d, ok := config.GetItemDefinition(key)
	if !ok || d.Flask == nil {
		return false
	}
	if skill, missing := c.FlaskSkillMissing(d.Flask); missing {
		g.AddCombatMessage(uitext.Text("item.flask_skill_required", skill))
		return false
	}
	it, ok := config.FlaskItem(key)
	if !ok {
		return false
	}
	c.Equipment[items.SlotQuickAction] = it
	g.AddCombatMessage(uitext.Text("item.flask_readied", c.Name, it.Name, g.flaskStock(c, key)))
	return true
}
func flaskDamage(c *character.MMCharacter, d *config.FlaskDefinition) int {
	return config.TierValue(d.Damage, c.SkillTier(character.FlaskMasterySkill(d))) + c.GetEffectiveIntellect()/character.BombThrowingIntellectDivisor
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
		return refuse(uitext.Text("item.device_cannot_act"))
	}
	c := g.party.Members[idx]
	if skill, missing := c.FlaskSkillMissing(d.Flask); missing {
		return refuse(uitext.Text("item.flask_skill_required", skill))
	}
	if g.combat.partyInsideSolidTerrain() || g.collisionSystem == nil {
		return refuse(uitext.Text("item.flask_no_room"))
	}
	supply := g.availableFlasks(c, key)
	if supply.index < 0 && supply.device == nil {
		return refuse(uitext.Text("item.flask_none", d.Name))
	}
	if d.Flask.Reusable && supply.device.DeviceCooldownFrames > 0 {
		return refuse(uitext.Text("item.device_recharging", d.Name))
	}
	f := d.Flask
	tier := c.SkillTier(character.FlaskMasterySkill(f))
	tps := g.config.GetTPS()
	tile := float64(g.config.GetTileSize())
	speed := 8 * tile / float64(tps)
	launch := g.combat.partyProjectileLaunch(g.combat.partyAttackAngle(), float64(f.RangeTiles)*tile, false)
	launch.lobApex = flaskApexTiles * tile
	a, distance := launch.angle, launch.distance
	p := FlaskProjectile{
		ActorID:          relicActorID(c),
		Reusable:         f.Reusable,
		CatalysisPercent: config.TierValue(f.CatalysisPercent, tier),
		CatalysisFrames:  f.CatalysisSeconds * tps,
		Launch:           launch,
		ID:               g.GenerateProjectileID("flask"),
		Key:              key,
		PoisonFrames:     config.TierValue(f.PoisonSeconds, tier) * tps,
		BurnFrames:       config.TierValue(f.BurnSeconds, tier) * tps,
		Radius:           float64(f.RadiusTiles) * tile,
		Remaining:        distance,
		Attacker:         c,
		X:                g.camera.X,
		Y:                g.camera.Y,
		VelX:             math.Cos(a) * speed,
		VelY:             math.Sin(a) * speed,
		Damage:           flaskDamage(c, f),
		LifeTime:         10 * tps,
		Active:           true,
	}
	if f.Reusable {
		g.armDeviceRecharge(supply.device)
	} else {
		if !supply.bag.Consume(supply.index, 1) {
			return false
		}
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
	// A flask is a consumable, not a weapon, spell or trap hit: the Dark Elf
	// binding never replaces it (skill.dark_elf_binding), and each victim's
	// packet takes its own catalysis.
	cs.forEachSplashVictim(x, y, p.Radius/float64(cs.game.config.GetTileSize()), nil, func(m *monster.Monster3D) {
		victimAttack := attack
		victimAttack.Packet = catalyzedFlaskPacket(m, p, attack.Packet)
		actual := cs.applyPartyMonsterAttack(m, victimAttack).Total()
		cs.reportIndirectHit(m, actual, d.Name, normalizeDamageTypeStr(d.Flask.Element))
		if m.IsAlive() {
			if p.CatalysisPercent > 0 && p.CatalysisFrames > 0 && p.ActorID != 0 {
				if m.Relics.Catalysis == nil {
					m.Relics.Catalysis = make(map[uint64]monster.CatalysisMark)
				}
				m.Relics.Catalysis[p.ActorID] = monster.CatalysisMark{Percent: p.CatalysisPercent, Frames: p.CatalysisFrames}
			}
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

func catalyzedFlaskPacket(victim *monster.Monster3D, shot *FlaskProjectile, packet monsterDamagePacket) monsterDamagePacket {
	if shot.Reusable || shot.ActorID == 0 || victim == nil {
		return packet
	}
	mark, ok := victim.Relics.Catalysis[shot.ActorID]
	if !ok || mark.Frames <= 0 || mark.Percent <= 0 {
		return packet
	}
	delete(victim.Relics.Catalysis, shot.ActorID)
	packet.Components = slices.Clone(packet.Components)
	for i := range packet.Components {
		packet.Components[i].Parts.Normal = packet.Components[i].Parts.Normal * (100 + mark.Percent) / 100
	}
	return packet
}

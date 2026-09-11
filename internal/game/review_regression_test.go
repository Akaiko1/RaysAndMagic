package game

import (
	"fmt"
	"math"
	"testing"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestRegressionArrowSliceGrowth(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	m := mkTestMonster("Target", 1000)
	m.ID = "review-target"
	m.X = 64
	m.Y = 0
	g.world.Monsters = []*monsterPkg.Monster3D{m}
	g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 32, 32, collision.CollisionTypeMonster, false))
	g.arrows = make([]Arrow, 2)
	for i := range g.arrows {
		id := g.GenerateProjectileID("arrow")
		g.arrows[i] = Arrow{ID: id, Active: true, X: m.X, Y: m.Y, VelX: 8, Damage: 10, LifeTime: 50, BowKey: "arbalest", DamageType: "physical", Owner: ProjectileOwnerPlayer}
		g.collisionSystem.RegisterEntity(collision.NewEntity(id, m.X, m.Y, 8, 8, collision.CollisionTypeProjectile, false))
	}
	g.arrows[0].PierceLeft = 1
	g.turnBasedMode = true
	cs.CheckProjectileMonsterCollisions()
	t.Logf("arrows len=%d cap=%d HP=%d second active=%v collisionExists=%v", len(g.arrows), cap(g.arrows), m.HitPoints, g.arrows[1].Active, g.collisionSystem.GetEntityByID(g.arrows[1].ID) != nil)
	g.turnBasedMode = true
	before := m.HitPoints
	cs.CheckProjectileMonsterCollisions()
	t.Logf("TB next collision pass HP %d -> %d", before, m.HitPoints)
	if before == 1000 {
		t.Fatal("fixture did not collide")
	}
	if before != m.HitPoints || g.arrows[1].Active {
		t.Fatal("second impacted arrow remains active after first impact reallocates slice")
	}
}
func TestRegressionLoadUnknownMap(t *testing.T) {
	g, ts := summonTileWorld(t)
	cfg := g.config
	w := g.world
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps["forest"] = w
	setTestWorldManager(t, wm)
	save := g.buildSave(wm)
	save.MapKey = "removed_or_failed_map"
	save.PlayerX = 7.5 * ts
	save.PlayerY = 10.5 * ts
	err := g.applySave(wm, &save)
	t.Logf("err=%v currentMap=%s player=(%.0f,%.0f)", err, wm.CurrentMapKey, g.camera.X, g.camera.Y)
	if err == nil {
		t.Fatal("load accepted missing map and applied its coordinates to current world")
	}
}

func TestRegressionLastStunTurnGetsBonusAction(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	m := g.party.Members[0]
	m.Speed = 100
	m.ApplyCharStun(120, 1)
	g.startPartyTurn()
	t.Logf("stun turns=%d frames=%d actions=%d floor=%d", m.StunTurnsRemaining, m.StunFramesRemaining, m.ActionsRemaining, m.TBRoundActionFloor)
	if m.ActionsRemaining != 0 {
		t.Fatal("stunned actor got a speed bonus action in the very turn it should skip")
	}
}

func TestRegressionBlockedFirewallEchoRefund(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.world = newTestWorldSized(g.config, 20, 20)
	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y, g.camera.Angle = 1.5*ts, 1.5*ts, math.Pi
	caster := g.party.Members[0]
	weapon, err := items.TryCreateWeaponFromYAML("verdant_eye_scepter")
	if err != nil {
		t.Fatal(err)
	}
	caster.Equipment[items.SlotMainHand] = weapon
	wd, _ := config.GetWeaponDefinition("verdant_eye_scepter")
	old := wd.SpellEchoPct
	wd.SpellEchoPct = 100
	t.Cleanup(func() { wd.SpellEchoPct = old })
	def, err := spells.GetSpellDefinitionByID("firewall")
	if err != nil {
		t.Fatal(err)
	}
	caster.SpellPoints = 100
	before := caster.SpellPoints
	cost := cs.effectiveSpellCost(caster, def.SpellPointsCost)
	if !cs.castResolvedSpell("firewall", def, caster, cost, false, false) {
		t.Fatal("cast refused")
	}
	t.Logf("blocked cast cost=%d SP=%d -> %d zones=%d", cost, before, caster.SpellPoints, len(g.persistentDamageZones))
	if caster.SpellPoints != before {
		t.Fatal("free echo refunded mana that was never charged")
	}
}
func TestRegressionZoneBillsBeyondLifetime(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	setTestWorldManager(t, nil)
	m := mkTestMonster("Target", 1000)
	m.ID = "review-zone-target"
	m.X, m.Y = 64, 64
	g.world.Monsters = []*monsterPkg.Monster3D{m}
	g.persistentDamageZones = []PersistentDamageZone{{SpellID: "firewall", X: 64, Y: 64, Radius: 64, FramesLeft: 1, tickCounter: g.config.GetTPS() - 1, IntervalFrames: g.config.GetTPS(), TickDamage: 10}}
	gl := &GameLoop{game: g}
	gl.tickPersistentDamageZonesTB()
	t.Logf("zone with 1 frame left dealt %d damage", 1000-m.HitPoints)
	if m.HitPoints != 990 {
		t.Fatal("zone charged more than the one tick owed before expiry")
	}
}

func TestRegressionPendingGeneratedLevelChoiceLost(t *testing.T) {
	g, _ := summonTileWorld(t)
	cs := g.combat
	if _, err := config.LoadLevelUpConfig("../../assets/level_up.yaml"); err != nil {
		t.Fatal(err)
	}
	m := g.party.Members[0]
	m.Level = 5
	m.Experience = xpStepCost(5)
	cs.checkLevelUp(m, false)
	if len(g.levelUpChoiceQueue) != 1 {
		t.Fatalf("setup queue=%d", len(g.levelUpChoiceQueue))
	}
	wm := world.NewWorldManager(g.config)
	wm.LoadedMaps["forest"] = g.world
	setTestWorldManager(t, wm)
	save := g.buildSave(wm)
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	t.Logf("level=%d saved choices=%d restored choices=%d", g.party.Members[0].Level, len(save.PendingLevelUpChoices), len(g.levelUpChoiceQueue))
	if len(g.levelUpChoiceQueue) != 1 {
		t.Fatal("earned level 6 choice lost after load")
	}
}

func TestRegressionMeleeCrossfireThroughWall(t *testing.T) {
	g, ts := summonTileWorld(t)
	cfg := g.config
	placePlayerAtTile(g, 1, 18, ts)
	a := monsterPkg.NewMonster3DFromConfig(5.5*ts, 5.5*ts, "dragon_green", cfg)
	b := monsterPkg.NewMonster3DFromConfig(7.5*ts, 5.5*ts, "skeleton", cfg)
	g.world.Monsters = []*monsterPkg.Monster3D{a, b}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.combat.applyBindUndead(b, 60, "Bind Undead")
	a.AIFoe = b
	a.AITargetX, a.AITargetY = b.X, b.Y
	g.world.Tiles[5][6] = world.TileWall
	if g.collisionSystem.CheckLineOfSight(a.X, a.Y, b.X, b.Y) {
		t.Fatal("fixture has LOS")
	}
	for _, m := range g.world.Monsters {
		if !g.collisionSystem.CanMoveToWithHabitat(m.ID, m.X, m.Y, m.HabitatPrefs, m.Flying) {
			t.Fatalf("fixture stands in obstacle: %s", m.Key)
		}
	}
	before := b.HitPoints
	gl := &GameLoop{game: g}
	attacked := gl.tryMonsterAttackFoeTurnBased(a, b)
	t.Logf("wall: attacked=%v targetHP=%d -> %d reach=%.1f distance=%.1f", attacked, before, b.HitPoints, a.GetAttackRangePixels(), Distance(a.X, a.Y, b.X, b.Y))
	if attacked {
		t.Fatal("melee crossfire attacked through wall without line of sight")
	}
}
func TestRegressionTransferredUnknownQuickSpellCasts(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	source, target := g.party.Members[1], g.party.Members[2]
	if !characterKnowsSpellByID(source, "firebolt") || characterKnowsSpellByID(target, "firebolt") {
		t.Fatal("fixture knowledge wrong")
	}
	it, err := spells.CreateSpellItem("firebolt")
	if err != nil {
		t.Fatal(err)
	}
	source.QuickSlots[0] = &it
	g.dragSrc = dragFromQuickSlot
	g.dragQuickChar = 1
	g.dragQuickSlot = 0
	g.selectPartyMemberManually(2)
	g.resolveQuickSlotDrop(2, 0)
	before := target.SpellPoints
	g.useQuickSlot(2, 0)
	t.Logf("non-fire caster %s SP=%d -> %d projectiles=%d", target.Name, before, target.SpellPoints, len(g.magicProjectiles))
	if len(g.magicProjectiles) > 0 {
		t.Fatal("quick-slot transfer bypasses spell knowledge")
	}
}

func TestRegressionPartySpearThroughWall(t *testing.T) {
	g, ts := summonTileWorld(t)
	placePlayerAtTile(g, 5, 5, ts)
	g.camera.Angle = 0
	m := monsterPkg.NewMonster3DFromConfig(7.5*ts, 5.5*ts, "skeleton", g.config)
	g.world.Monsters = []*monsterPkg.Monster3D{m}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.world.Tiles[5][6] = world.TileWall
	if g.collisionSystem.CheckLineOfSight(g.camera.X, g.camera.Y, m.X, m.Y) {
		t.Fatal("fixture has LOS")
	}
	spear, err := items.TryCreateWeaponFromYAML("iron_spear")
	if err != nil {
		t.Fatal(err)
	}
	g.party.Members[0].Equipment[items.SlotMainHand] = spear
	g.selectedChar = 0
	before := m.HitPoints
	attacked := g.combat.EquipmentMeleeAttack()
	t.Logf("party spear through wall: attacked=%v HP=%d -> %d", attacked, before, m.HitPoints)
	if m.HitPoints != before {
		t.Fatal("party spear hit monster through solid wall")
	}
}

func TestRegressionLethalCrossfireLosesSplash(t *testing.T) {
	for _, hp := range []int{1000, 1} {
		t.Run(fmt.Sprint(hp), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			source := mkTestMonster("source", 1000)
			source.ID = "review-source"
			source.Bound = true
			target := mkTestMonster("direct", hp)
			target.ID = "review-direct"
			target.X = 64
			target.Y = 64
			nearby := mkTestMonster("nearby", 1000)
			nearby.ID = "review-nearby"
			nearby.X = 100
			nearby.Y = 64
			g.world.Monsters = []*monsterPkg.Monster3D{source, target, nearby}
			for _, m := range g.world.Monsters {
				g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, false))
			}
			g.magicProjectiles = []MagicProjectile{{ID: "review-fireball", X: 64, Y: 64, Damage: 10, Active: true, LifeTime: 50, SpellType: "fireball", Owner: ProjectileOwnerBoundUndead, SourceMonster: source}}
			g.collisionSystem.RegisterEntity(collision.NewEntity("review-fireball", 64, 64, 8, 8, collision.CollisionTypeProjectile, false))
			cs.CheckProjectileMonsterCollisions()
			t.Logf("direct starting HP=%d ending HP=%d; nearby HP=%d", hp, target.HitPoints, nearby.HitPoints)
			if nearby.HitPoints == 1000 {
				t.Fatal("lethal direct hit suppressed fireball splash")
			}
		})
	}
}

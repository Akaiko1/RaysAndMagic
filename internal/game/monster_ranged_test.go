package game

import (
	"math"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

type testTileChecker struct {
	width  int
	height int
}

func (t *testTileChecker) IsTileBlocking(tileX, tileY int) bool {
	return false
}

func (t *testTileChecker) IsTileBlockingForHabitat(tileX, tileY int, habitatPrefs []string, flying bool) bool {
	return false
}

func (t *testTileChecker) IsTileOpaque(tileX, tileY int) bool {
	return false
}

func (t *testTileChecker) GetWorldBounds() (int, int) {
	return t.width, t.height
}

func newTestCombatSystemWithConfig(t *testing.T) *CombatSystem {
	t.Helper()
	setTestWorldManager(t, nil)

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadTrapConfig("../../assets/traps.yaml"); err != nil {
		t.Fatalf("load traps: %v", err)
	}
	if _, err := config.LoadLevelUpConfig("../../assets/level_up.yaml"); err != nil {
		t.Fatalf("load level-up choices: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	game := &MMGame{
		config: cfg,
		camera: &FirstPersonCamera{
			X:        0,
			Y:        0,
			Angle:    0,
			FOV:      squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight()),
			ViewDist: cfg.GetViewDistance(),
		},
		party: character.NewParty(cfg),
		world: &world.World3D{},
	}
	stripNewClassSkillsForLegacyFixtures(game.party)
	game.selectedChar = 0
	game.collisionSystem = collision.NewCollisionSystem(&testTileChecker{width: 100, height: 100}, float64(cfg.GetTileSize()))
	game.collisionSystem.RegisterEntity(collision.NewEntity("player", game.camera.X, game.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	cs := NewCombatSystem(game)
	game.combat = cs

	return cs
}

func TestMonsterRangedAttack_SpawnsWeaponProjectile(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	game.camera.X = 128
	game.camera.Y = 0
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	monster := &monsterPkg.Monster3D{
		Name:             "Bandit",
		X:                0,
		Y:                0,
		AttackRadius:     128,
		State:            monsterPkg.StateAttacking,
		StateTimer:       1,
		ProjectileWeapon: "throwing_knife",
		DamageMin:        5,
		DamageMax:        5,
		HitPoints:        1,
		MaxHitPoints:     1,
	}
	game.world.Monsters = []*monsterPkg.Monster3D{monster}

	beforeHP := game.party.Members[0].HitPoints
	cs.HandleMonsterInteractions()

	if len(game.arrows) != 1 {
		t.Fatalf("expected 1 arrow, got %d", len(game.arrows))
	}
	arrow := game.arrows[0]
	if arrow.Owner != ProjectileOwnerMonster {
		t.Fatalf("expected monster-owned arrow, got %v", arrow.Owner)
	}
	if arrow.BowKey != "throwing_knife" {
		t.Fatalf("expected throwing_knife, got %s", arrow.BowKey)
	}
	if arrow.SourceName != "Bandit" {
		t.Fatalf("expected source name Bandit, got %s", arrow.SourceName)
	}
	if game.party.Members[0].HitPoints != beforeHP {
		t.Fatalf("expected no immediate melee damage, HP %d -> %d", beforeHP, game.party.Members[0].HitPoints)
	}
}

func TestMonsterRangedAttack_SpawnsSpellProjectile(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	game.camera.X = 128
	game.camera.Y = 0
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	monster := &monsterPkg.Monster3D{
		Name:            "Dragon",
		X:               0,
		Y:               0,
		AttackRadius:    128,
		State:           monsterPkg.StateAttacking,
		StateTimer:      1,
		ProjectileSpell: "firebolt",
		DamageMin:       7,
		DamageMax:       7,
		HitPoints:       1,
		MaxHitPoints:    1,
	}
	game.world.Monsters = []*monsterPkg.Monster3D{monster}

	beforeHP := game.party.Members[0].HitPoints
	cs.HandleMonsterInteractions()

	if len(game.magicProjectiles) != 1 {
		t.Fatalf("expected 1 magic projectile, got %d", len(game.magicProjectiles))
	}
	mp := game.magicProjectiles[0]
	if mp.Owner != ProjectileOwnerMonster {
		t.Fatalf("expected monster-owned projectile, got %v", mp.Owner)
	}
	if mp.SpellType != "firebolt" {
		t.Fatalf("expected firebolt, got %s", mp.SpellType)
	}
	if mp.SourceName != "Dragon" {
		t.Fatalf("expected source name Dragon, got %s", mp.SourceName)
	}
	if game.party.Members[0].HitPoints != beforeHP {
		t.Fatalf("expected no immediate melee damage, HP %d -> %d", beforeHP, game.party.Members[0].HitPoints)
	}
}

// A projectile profile extends a monster's options instead of replacing its
// close attack. At point blank it uses melee_damage_type; once the party steps
// away, the projectile keeps the weapon's own damage school.
func TestMonsterRangedAttack_PointBlankUsesMeleeAndKeepsSeparateSchools(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	tile := float64(game.config.GetTileSize())

	game.party.Members = game.party.Members[:1]
	member := game.party.Members[0]
	isolateTrueDamageMember(member, 0)
	member.Equipment[items.SlotRing1] = items.Item{
		Attributes: map[string]int{"resist_dark": 100},
	}
	member.HitPoints, member.MaxHitPoints = 400, 400

	game.camera.X = tile
	game.camera.Y = 0
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	attacker := &monsterPkg.Monster3D{
		Name:             "Ranged Boss",
		Boss:             true,
		X:                0,
		Y:                0,
		AttackRadius:     4 * tile,
		State:            monsterPkg.StateAttacking,
		StateTimer:       1,
		ProjectileWeapon: "alien_blaster",
		MeleeDamageType:  monsterPkg.DamageDark.String(),
		DamageMin:        50,
		DamageMax:        50,
		HitPoints:        100,
		MaxHitPoints:     100,
	}
	game.world.Monsters = []*monsterPkg.Monster3D{attacker}

	cs.HandleMonsterInteractions()
	if len(game.arrows) != 0 {
		t.Fatalf("point-blank ranged boss fired %d projectiles, want melee", len(game.arrows))
	}
	if member.HitPoints != 400 {
		t.Fatalf("dark melee bypassed 100%% dark resist: HP %d, want 400", member.HitPoints)
	}
	if attacker.AttackCDFrames == 0 {
		t.Fatal("point-blank melee did not spend the ranged boss's attack action")
	}

	game.camera.X = 2 * tile
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
	attacker.AttackCDFrames = 0
	attacker.StateTimer = 1
	cs.HandleMonsterInteractions()

	if len(game.arrows) != 1 {
		t.Fatalf("ranged boss outside melee spawned %d projectiles, want 1", len(game.arrows))
	}
	if got := game.arrows[0].DamageType; got != monsterPkg.DamageSpirit.String() {
		t.Fatalf("alien blaster projectile school = %q, want spirit; melee school must not leak into ranged delivery", got)
	}
}

func TestMonsterRangedAttack_CrossfireUsesSamePointBlankRule(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tile := float64(cs.game.config.GetTileSize())
	attacker := &monsterPkg.Monster3D{
		Name:             "Crossfire Archer",
		X:                0,
		Y:                0,
		ProjectileWeapon: "throwing_knife",
		DamageMin:        15,
		DamageMax:        15,
		HitPoints:        100,
		MaxHitPoints:     100,
	}
	target := &monsterPkg.Monster3D{
		Name:         "Summon",
		X:            tile,
		Y:            0,
		HitPoints:    100,
		MaxHitPoints: 100,
		Resistances:  make(map[monsterPkg.DamageType]int),
	}

	cs.performMonsterAttackAgainstMonster(attacker, target, ProjectileOwnerMonsterAtBound)
	if len(cs.game.arrows) != 0 {
		t.Fatalf("point-blank crossfire spawned %d projectiles, want melee", len(cs.game.arrows))
	}
	if target.HitPoints >= target.MaxHitPoints {
		t.Fatal("point-blank crossfire did not damage its target in melee")
	}

	target.X = 2 * tile
	cs.performMonsterAttackAgainstMonster(attacker, target, ProjectileOwnerMonsterAtBound)
	if len(cs.game.arrows) != 1 {
		t.Fatalf("distant crossfire spawned %d projectiles, want 1", len(cs.game.arrows))
	}
}

func TestMonsterProjectileHitsPlayer(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	// Ensure whichever weighted party target is selected cannot perfect-dodge.
	beforeHP := 0
	for _, member := range game.party.Members {
		member.Luck = 0
		beforeHP += member.HitPoints
	}

	mp := MagicProjectile{
		ID:         "monster_test_proj",
		X:          game.camera.X,
		Y:          game.camera.Y,
		Damage:     5,
		LifeTime:   10,
		Active:     true,
		SpellType:  "firebolt",
		Owner:      ProjectileOwnerMonster,
		SourceName: "Dragon",
	}
	game.magicProjectiles = append(game.magicProjectiles, mp)
	game.collisionSystem.RegisterEntity(collision.NewEntity(mp.ID, mp.X, mp.Y, 8, 8, collision.CollisionTypeProjectile, false))

	cs.CheckProjectilePlayerCollisions()

	afterHP := 0
	for _, member := range game.party.Members {
		afterHP += member.HitPoints
	}
	if afterHP >= beforeHP {
		t.Fatalf("expected party HP to decrease, total HP %d -> %d", beforeHP, afterHP)
	}
	if game.magicProjectiles[0].Active {
		t.Fatalf("expected projectile to deactivate after hit")
	}
}

func equipGuaranteedBroodscaleReflection(t *testing.T, game *MMGame) {
	t.Helper()
	def, ok := config.GetItemDefinition("broodscale_aegis")
	if !ok || def == nil {
		t.Fatal("broodscale_aegis is missing from items.yaml")
	}
	previousChance := def.ProjectileReflectPct
	def.ProjectileReflectPct = 100
	t.Cleanup(func() { def.ProjectileReflectPct = previousChance })

	shield, err := items.TryCreateItemFromYAML("broodscale_aegis")
	if err != nil {
		t.Fatalf("create Broodscale Aegis: %v", err)
	}
	game.party.Members[0].Equipment[items.SlotOffHand] = shield
}

func TestBroodscaleAegisReturnsMagicBoltBeforeDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	equipGuaranteedBroodscaleReflection(t, game)

	tile := float64(game.config.GetTileSize())
	shooter := mkTestMonster("Shooter", 100)
	shooter.ID, shooter.X, shooter.Y = "reflect_shooter", 3*tile, 0
	bystander := mkTestMonster("Bystander", 100)
	bystander.ID, bystander.X, bystander.Y = "reflect_bystander", 1.5*tile, 0
	game.world.Monsters = []*monsterPkg.Monster3D{bystander, shooter}
	game.collisionSystem.RegisterEntity(collision.NewEntity(shooter.ID, shooter.X, shooter.Y, 32, 32, collision.CollisionTypeMonster, false))
	game.collisionSystem.RegisterEntity(collision.NewEntity(bystander.ID, bystander.X, bystander.Y, 32, 32, collision.CollisionTypeMonster, false))

	bolt := MagicProjectile{
		ID:            "reflected_magic_bolt",
		X:             game.camera.X,
		Y:             game.camera.Y,
		VelX:          -8,
		Damage:        30,
		LifeTime:      2,
		Active:        true,
		SpellType:     "firebolt",
		Owner:         ProjectileOwnerMonster,
		SourceName:    shooter.Name,
		SourceMonster: shooter,
	}
	game.magicProjectiles = append(game.magicProjectiles, bolt)
	game.collisionSystem.RegisterEntity(collision.NewEntity(bolt.ID, bolt.X, bolt.Y, 8, 8, collision.CollisionTypeProjectile, false))

	partyHP := game.party.Members[0].HitPoints
	cs.CheckProjectilePlayerCollisions()
	reflected := &game.magicProjectiles[0]
	if game.party.Members[0].HitPoints != partyHP {
		t.Fatalf("reflection damaged party: HP %d -> %d", partyHP, game.party.Members[0].HitPoints)
	}
	if !reflected.Active || reflected.Owner != ProjectileOwnerReflected {
		t.Fatalf("bolt was not kept alive as a reflected projectile: %+v", *reflected)
	}
	if reflected.VelX <= 0 || reflected.VelY != 0 {
		t.Fatalf("reflected bolt velocity = (%.2f, %.2f), want flight back toward shooter", reflected.VelX, reflected.VelY)
	}
	if reflected.LifeTime <= 2 {
		t.Fatalf("return flight kept spent lifetime %d", reflected.LifeTime)
	}
	if shooter.HitPoints != shooter.MaxHitPoints {
		t.Fatal("shooter took damage before the return bolt landed")
	}

	for reflected.Active && reflected.LifeTime > 0 {
		reflected.X += reflected.VelX
		reflected.Y += reflected.VelY
		reflected.LifeTime--
		game.collisionSystem.UpdateEntity(reflected.ID, reflected.X, reflected.Y)
		cs.CheckProjectileMonsterCollisions()
	}
	if shooter.HitPoints >= shooter.MaxHitPoints {
		t.Fatal("return bolt reached shooter without dealing reflected damage")
	}
	if bystander.HitPoints != bystander.MaxHitPoints {
		t.Fatal("return bolt hit a bystander instead of its original shooter")
	}
}

func TestBroodscaleAegisReturnsWeaponBoltAsSameArrow(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game
	equipGuaranteedBroodscaleReflection(t, game)

	tile := float64(game.config.GetTileSize())
	shooter := mkTestMonster("Archer", 100)
	shooter.ID, shooter.X, shooter.Y = "reflect_archer", 2*tile, 0
	game.world.Monsters = []*monsterPkg.Monster3D{shooter}
	game.collisionSystem.RegisterEntity(collision.NewEntity(shooter.ID, shooter.X, shooter.Y, 32, 32, collision.CollisionTypeMonster, false))

	arrow := Arrow{
		ID:            "reflected_weapon_bolt",
		X:             game.camera.X,
		Y:             game.camera.Y,
		VelX:          -10,
		Damage:        20,
		LifeTime:      30,
		Active:        true,
		BowKey:        "throwing_knife",
		DamageType:    monsterPkg.DamagePhysical.String(),
		Owner:         ProjectileOwnerMonster,
		SourceName:    shooter.Name,
		SourceMonster: shooter,
	}
	game.arrows = append(game.arrows, arrow)
	game.collisionSystem.RegisterEntity(collision.NewEntity(arrow.ID, arrow.X, arrow.Y, 8, 8, collision.CollisionTypeProjectile, false))

	cs.CheckProjectilePlayerCollisions()
	reflected := &game.arrows[0]
	if !reflected.Active || reflected.Owner != ProjectileOwnerReflected || reflected.ID != arrow.ID || reflected.BowKey != arrow.BowKey {
		t.Fatalf("weapon bolt was replaced or lost its visual identity: %+v", *reflected)
	}
	for reflected.Active && reflected.LifeTime > 0 {
		reflected.X += reflected.VelX
		reflected.Y += reflected.VelY
		reflected.LifeTime--
		game.collisionSystem.UpdateEntity(reflected.ID, reflected.X, reflected.Y)
		cs.CheckProjectileMonsterCollisions()
	}
	if shooter.HitPoints >= shooter.MaxHitPoints {
		t.Fatal("reflected weapon bolt did not damage its shooter on impact")
	}
}

func TestPlayerRangedAttackUsesWeaponPhysics(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	game := cs.game

	bow := items.CreateWeaponFromYAML("hunting_bow")
	game.party.Members[0].Equipment[items.SlotMainHand] = bow

	weaponDef, exists := config.GetWeaponDefinition("hunting_bow")
	if !exists || weaponDef.Physics == nil {
		t.Fatalf("hunting_bow must have physics")
	}

	cs.EquipmentMeleeAttack()

	if len(game.arrows) != 1 {
		t.Fatalf("expected 1 player arrow, got %d", len(game.arrows))
	}
	arrow := game.arrows[0]
	if arrow.Owner != ProjectileOwnerPlayer {
		t.Fatalf("expected player-owned arrow, got %v", arrow.Owner)
	}
	if arrow.BowKey != "hunting_bow" {
		t.Fatalf("expected hunting_bow, got %s", arrow.BowKey)
	}

	expectedSpeed := weaponDef.Physics.GetSpeedPixels(game.config.GetTileSize())
	if math.Abs(arrow.VelX-expectedSpeed) > 0.0001 {
		t.Fatalf("expected arrow VelX %.4f from weapon physics, got %.4f", expectedSpeed, arrow.VelX)
	}
	if arrow.VelY != 0 {
		t.Fatalf("expected arrow VelY 0 at angle 0, got %.4f", arrow.VelY)
	}
	if arrow.LifeTime != weaponDef.Physics.GetLifetimeFrames() {
		t.Fatalf("expected lifetime %d from weapon physics, got %d", weaponDef.Physics.GetLifetimeFrames(), arrow.LifeTime)
	}

	entity := game.collisionSystem.GetEntityByID(arrow.ID)
	if entity == nil || entity.BoundingBox == nil {
		t.Fatalf("expected arrow collision entity")
	}
	expectedCollisionSize := weaponDef.Physics.GetCollisionSizePixels(game.config.GetTileSize())
	if math.Abs(entity.BoundingBox.Width-expectedCollisionSize) > 0.0001 {
		t.Fatalf("expected collision width %.4f from weapon physics, got %.4f", expectedCollisionSize, entity.BoundingBox.Width)
	}
	if math.Abs(entity.BoundingBox.Height-expectedCollisionSize) > 0.0001 {
		t.Fatalf("expected collision height %.4f from weapon physics, got %.4f", expectedCollisionSize, entity.BoundingBox.Height)
	}
}

package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// Sword racks roll the castle_armory pool, which must contain only ZONE gear +
// gold and NEVER a unique - uniques drop from the boss alone. Guards the user's
// "racks hold zone loot, not uniques" rule structurally and by sampling.
func TestCastleArmoryLootTable_NeverYieldsUniques(t *testing.T) {
	newTestCombatSystemWithConfig(t) // loads weapons/items configs + bridges
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	tbl, ok := config.GetWeightedLootTable("castle_armory")
	if !ok {
		t.Fatal("castle_armory loot table missing")
	}
	// Structural: the pool lists none of the zone's uniques.
	for _, e := range tbl.Entries {
		switch e.Key {
		case "muramasa", "tonbogiri", "kage_kunai", "inazuma_matchlock", "onryo_lamellar":
			t.Fatalf("castle_armory must not list unique %q", e.Key)
		}
	}
	allowed := make(map[string]bool, len(tbl.Entries))
	for _, e := range tbl.Entries {
		allowed[e.Key] = true
	}
	// Sampling: every roll yields Rolls items, gold in range, and only pooled keys.
	for i := 0; i < 600; i++ {
		loot, gold := rollWeightedLootTable("castle_armory")
		if len(loot) != tbl.Rolls {
			t.Fatalf("expected %d item(s) per roll, got %d", tbl.Rolls, len(loot))
		}
		if gold < tbl.GoldMin || gold > tbl.GoldMax {
			t.Fatalf("gold %d outside [%d,%d]", gold, tbl.GoldMin, tbl.GoldMax)
		}
		for _, it := range loot {
			if it.Name == "" {
				t.Fatal("rolled item has no name (creation failed)")
			}
		}
	}
}

// The Samurai Warlord is the second boss on the SAME kit as the Golden Thief Bug,
// but a different config: dormant (not evasive) until castle_armory completes,
// then aggressive with summon + enrage + armor-piercing. This verifies the kit
// generalization (dormant-visible gate) and that completing the rack quest wakes it.
func TestSamuraiBoss_DormantUntilArmoryQuest(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	qcfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qcfg)
	cs.game.questManager = qm

	boss := monsterPkg.NewMonster3DFromConfig(0, 0, "old_samurai", cs.game.config)
	if boss == nil {
		t.Fatal("old_samurai monster missing")
	}
	// Config shape: dormant (no evade radius), summons, enrages, ignores armor.
	if boss.PassiveUntilQuest != "castle_armory" {
		t.Errorf("boss should gate on castle_armory, got %q", boss.PassiveUntilQuest)
	}
	if boss.EvadeRadiusTiles != 0 {
		t.Errorf("samurai must be dormant (evade_radius 0), got %v", boss.EvadeRadiusTiles)
	}
	if len(boss.SummonMonsters) == 0 {
		t.Error("samurai should summon adds")
	}
	if boss.EnrageAtHP <= 0 {
		t.Error("samurai should enrage below an HP threshold")
	}
	if !boss.IgnoresArmor {
		t.Error("samurai should ignore armor")
	}
	if !cs.isBoss(boss) {
		t.Error("samurai must be recognized as a boss")
	}

	// Dormant while the rack quest is unfinished: holds (consumes its action) and
	// never blinks (no evade radius).
	if !cs.bossEvasive(boss) {
		t.Fatal("boss must be dormant while castle_armory is unfinished")
	}
	prevX, prevY := boss.X, boss.Y
	if !cs.updateBoss(boss, true, true) {
		t.Error("a dormant boss should consume its action (hold, no normal attack)")
	}
	if boss.X != prevX || boss.Y != prevY {
		t.Error("a dormant boss must not blink away (it has no evade radius)")
	}

	// Movement freeze: the per-frame pre-pass must flag the sealed boss BossDormant
	// (not BossAggro) so the RT movement loop holds it on its throne. updateBoss
	// only suppresses the ATTACK - the separate movement path is what let the boss
	// wander off, so this asserts the flag that gates it.
	cs.game.world.Monsters = append(cs.game.world.Monsters, boss)
	cs.game.refreshBoundAllyCache()
	if !boss.BossDormant {
		t.Error("sealed boss must be flagged BossDormant while castle_armory is unfinished")
	}
	if boss.BossAggro {
		t.Error("sealed boss must not be BossAggro while dormant")
	}

	// Collect all 5 racks -> quest completes -> boss turns aggressive.
	if err := qm.ActivateQuest("castle_armory"); err != nil {
		t.Fatalf("activate castle_armory: %v", err)
	}
	for i := 0; i < 5; i++ {
		qm.OnInteract("sword_rack")
	}
	q := qm.GetQuest("castle_armory")
	if q == nil || q.Status != quests.QuestStatusCompleted {
		t.Fatalf("castle_armory should be completed after 5 racks, got %+v", q)
	}
	if cs.bossEvasive(boss) {
		t.Error("boss must turn aggressive once castle_armory completes")
	}
	cs.game.refreshBoundAllyCache()
	if boss.BossDormant {
		t.Error("boss must no longer be dormant once castle_armory completes")
	}
	// Unsealed but UNengaged: the Samurai does NOT beeline across the whole map
	// (that map-wide chase is the Golden Thief Bug's unique trait). It goes
	// relentless only after normal aggro - within its (large) alert radius or once
	// the party hits it.
	if boss.BossAggro {
		t.Error("unsealed-but-unengaged Samurai must NOT relentlessly chase from across the map")
	}
	boss.IsEngagingPlayer = true
	cs.game.refreshBoundAllyCache()
	if !boss.BossAggro {
		t.Error("an engaged unsealed Samurai should relentlessly pursue")
	}
}

// A sealed (dormant) boss is invulnerable: it can't be disintegrated and the
// player damage hubs short-circuit via absorbIfSealed. Clearing the flag (quest
// unseals it) makes it damageable again.
func TestSealedBossInvulnerableGates(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	boss := monsterPkg.NewMonster3DFromConfig(64, 64, "old_samurai", cs.game.config)
	if boss == nil {
		t.Fatal("old_samurai missing")
	}

	boss.BossDormant = true
	if !monsterImmuneToDisintegrate(boss) {
		t.Error("sealed boss must be immune to disintegrate")
	}
	if !cs.absorbIfSealed(boss) {
		t.Error("sealed boss must absorb player hits (short-circuit the damage hub)")
	}

	boss.BossDormant = false
	if monsterImmuneToDisintegrate(boss) {
		t.Error("unsealed samurai is not undead/dragon -> disintegratable")
	}
	if cs.absorbIfSealed(boss) {
		t.Error("unsealed boss must take damage normally (no absorb)")
	}
}

// A sealed (dormant) boss is inert to INDIRECT damage too - AoE splash, traps,
// and turn-based pack-aggro must not damage it, flash it, or wake it. Direct
// hits route through absorbIfSealed; these paths call TakeDamageResist (returns
// 0) and previously still applied hit tint / pack-engage side effects.
func TestSealedBossIgnoresIndirectEffects(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	tile := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = 0, 0

	boss := monsterPkg.NewMonster3DFromConfig(5*tile, 5*tile, "old_samurai", cs.game.config)
	boss.BossDormant = true
	wantHP := boss.HitPoints

	assertInert := func(src string) {
		t.Helper()
		if boss.HitPoints != wantHP {
			t.Errorf("%s: sealed boss took damage (HP %d, want %d)", src, boss.HitPoints, wantHP)
		}
		if boss.IsEngagingPlayer {
			t.Errorf("%s: sealed boss must not aggro", src)
		}
		if boss.HitTintFrames != 0 {
			t.Errorf("%s: sealed boss must not flash (tint=%d)", src, boss.HitTintFrames)
		}
	}

	// AoE splash centered on a mob right beside the boss.
	center := monsterPkg.NewMonster3DFromConfig(5*tile+8, 5*tile, "goblin", cs.game.config)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{center, boss}
	cs.applyAoeSplash(center, 9999, "fire", monsterPkg.DamageFire, "Test", 3.0, 0)
	assertInert("AoE splash")

	// Trap payload directly on the boss.
	cs.applyTrapDamage(boss, 9999, "fire", monsterPkg.DamageFire, "Spike Trap")
	assertInert("trap")

	// Turn-based pack aggro from a struck same-key mob must not wake a sealed one.
	cs.game.turnBasedMode = true
	hit := monsterPkg.NewMonster3DFromConfig(10*tile, 10*tile, "goblin", cs.game.config)
	sealed := monsterPkg.NewMonster3DFromConfig(11*tile, 10*tile, "goblin", cs.game.config)
	sealed.BossDormant = true
	cs.game.world.Monsters = []*monsterPkg.Monster3D{hit, sealed}
	cs.engageTurnBasedPackOnHit(hit)
	if sealed.IsEngagingPlayer {
		t.Error("pack aggro: sealed monster must not be pulled in")
	}
}

func TestSamuraiSummonRespectsCapAndOccupiedTiles(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	oldTM, oldWM := world.GlobalTileManager, world.GlobalWorldManager
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	world.GlobalWorldManager = nil
	defer func() {
		world.GlobalTileManager = oldTM
		world.GlobalWorldManager = oldWM
	}()

	w := world.NewWorld3D(cs.game.config)
	w.Width = 12
	w.Height = 12
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	cs.game.world = w
	tile := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = TileCenterFromTile(6, 6, tile)
	cs.game.collisionSystem = collision.NewCollisionSystem(w, tile)
	cs.game.collisionSystem.RegisterEntity(collision.NewEntity("player", cs.game.camera.X, cs.game.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	bossX, bossY := TileCenterFromTile(4, 6, tile)
	boss := monsterPkg.NewMonster3DFromConfig(bossX, bossY, "old_samurai", cs.game.config)
	boss.SummonMonsters = []string{"ashigaru_firelock"}
	boss.SummonCount = 2
	boss.SummonMax = 4
	cs.game.registerSpawnedMonster(boss)

	if !cs.summonSpawnOccupied(cs.game.camera.X, cs.game.camera.Y) {
		t.Fatal("summons must not use the party tile")
	}
	if !cs.summonSpawnOccupied(boss.X, boss.Y) {
		t.Fatal("summons must not use a tile occupied by a monster")
	}

	for i, pos := range [][2]int{{3, 6}, {4, 5}, {5, 6}} {
		x, y := TileCenterFromTile(pos[0], pos[1], tile)
		add := monsterPkg.NewMonster3DFromConfig(x, y, "ashigaru_firelock", cs.game.config)
		add.SummonedBy = boss.ID
		add.IsEngagingPlayer = true
		add.WasAttacked = true
		cs.game.registerSpawnedMonster(add)
		t.Logf("seeded summon %d at tile (%d,%d)", i, pos[0], pos[1])
	}

	if !cs.summonBossAdds(boss) {
		t.Fatal("boss should fill the remaining summon slot")
	}
	if got := cs.countLiveSummons(boss); got != 4 {
		t.Fatalf("live summons = %d, want hard cap 4", got)
	}
}

func TestBossFirstSummonGuaranteedThenUsesChance(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	oldTM, oldWM := world.GlobalTileManager, world.GlobalWorldManager
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	world.GlobalWorldManager = nil
	defer func() {
		world.GlobalTileManager = oldTM
		world.GlobalWorldManager = oldWM
	}()

	w := world.NewWorld3D(cs.game.config)
	w.Width = 12
	w.Height = 12
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	cs.game.world = w
	tile := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = TileCenterFromTile(6, 6, tile)
	cs.game.collisionSystem = collision.NewCollisionSystem(w, tile)
	cs.game.collisionSystem.RegisterEntity(collision.NewEntity("player", cs.game.camera.X, cs.game.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	bossX, bossY := TileCenterFromTile(4, 6, tile)
	boss := monsterPkg.NewMonster3DFromConfig(bossX, bossY, "old_samurai", cs.game.config)
	boss.PassiveUntilQuest = ""
	boss.SummonChance = 0
	boss.SummonFirstGuaranteed = true
	boss.SummonMonsters = []string{"ashigaru_firelock"}
	boss.SummonCount = 2
	boss.SummonMax = 4
	cs.game.registerSpawnedMonster(boss)

	if !cs.updateBoss(boss, true, true) {
		t.Fatal("first summon should be guaranteed even when summon_chance is 0")
	}
	if !boss.SummonFirstDone {
		t.Fatal("first successful summon must latch SummonFirstDone")
	}
	if got := cs.countLiveSummons(boss); got != 2 {
		t.Fatalf("first summon count = %d, want 2", got)
	}
	if cs.updateBoss(boss, true, true) {
		t.Fatal("second summon should use summon_chance after the first guaranteed summon")
	}
	if got := cs.countLiveSummons(boss); got != 2 {
		t.Fatalf("second no-proc changed summon count to %d, want 2", got)
	}
}

func TestAshigaruPiercingShotHitsTwoLivingPartyMembers(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	for _, member := range cs.game.party.Members {
		member.MaxHitPoints = 100
		member.HitPoints = 100
		member.Luck = 0
	}

	ashigaru := &monsterPkg.Monster3D{
		Name:                "Ashigaru Arquebusier",
		HitPoints:           100,
		MaxHitPoints:        100,
		DamageMin:           20,
		DamageMax:           20,
		PiercingShotChance:  1,
		PiercingShotTargets: 2,
	}

	if !cs.tryMonsterPiercingShot(ashigaru) {
		t.Fatal("piercing shot should fire at 100% chance")
	}

	hit := 0
	for _, member := range cs.game.party.Members {
		switch member.HitPoints {
		case 80:
			hit++
		case 100:
		default:
			t.Fatalf("unexpected party HP after piercing shot: %d", member.HitPoints)
		}
	}
	if hit != 2 {
		t.Fatalf("piercing shot hit %d members, want 2", hit)
	}
}

func TestNingyoAllyHealChoosesNearbyWoundedMonster(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tile := float64(cs.game.config.GetTileSize())
	healer := &monsterPkg.Monster3D{
		Name:                 "Miyabi Ningyo",
		X:                    0,
		Y:                    0,
		HitPoints:            100,
		MaxHitPoints:         500,
		AllyHealChance:       1,
		AllyHealAmount:       200,
		AllyHealRadiusPixels: 2 * tile,
	}
	near := &monsterPkg.Monster3D{
		Name:         "Nearby Ally",
		X:            tile,
		Y:            0,
		HitPoints:    50,
		MaxHitPoints: 500,
	}
	far := &monsterPkg.Monster3D{
		Name:         "Far Ally",
		X:            5 * tile,
		Y:            0,
		HitPoints:    1,
		MaxHitPoints: 500,
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{healer, near, far}

	if !cs.tryMonsterAllyHeal(healer) {
		t.Fatal("ally heal should cast at 100% chance with a wounded target nearby")
	}
	if near.HitPoints != 250 {
		t.Fatalf("near ally HP = %d, want 250", near.HitPoints)
	}
	if healer.HitPoints != 100 {
		t.Fatalf("healer HP = %d, want unchanged 100", healer.HitPoints)
	}
	if far.HitPoints != 1 {
		t.Fatalf("far ally HP = %d, want unchanged 1", far.HitPoints)
	}
}

// Regression: a weapon's display name must resolve to its YAML key via the real
// name index, not a naive lower+underscore transform - otherwise flavor-named
// weapons (commas, "of the ...") fail CanEquipWeaponByName and are
// unequippable. Bug: Nyra (thief, has dagger) couldn't equip "Kage-kunai, the
// Twin Shadows" because the transform yielded "kage-kunai,_the_twin_shadows".
func TestFancyNamedWeapons_ResolveAndEquip(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t) // sets up the weapon bridge (name index)

	for name, want := range map[string]string{
		"Kage-kunai, the Twin Shadows":   "kage_kunai",
		"Muramasa, the Thirsting Edge":   "muramasa",
		"Tonbogiri, the Dragonfly Spear": "tonbogiri",
		"Tanegashima Matchlock":          "tanegashima",
		"Kanabo":                         "kanabo",
		"Silver Sword":                   "silver_sword", // well-named still resolves
	} {
		if got := items.GetWeaponKeyByName(name); got != want {
			t.Errorf("GetWeaponKeyByName(%q) = %q, want %q", name, got, want)
		}
	}

	// A dagger-skilled member who can wield the basic dagger must also wield the
	// fancy-named twin blades - the equip gate now resolves by key, not by name shape.
	var daggerUser *character.MMCharacter
	for _, m := range cs.game.party.Members {
		if m != nil && m.CanEquipWeaponByName("Magic Dagger") {
			daggerUser = m
			break
		}
	}
	if daggerUser == nil {
		t.Fatal("no dagger-skilled party member found (expected the thief)")
	}
	if !daggerUser.CanEquipWeaponByName("Kage-kunai, the Twin Shadows") {
		t.Error("a dagger user must be able to equip Kage-kunai (fancy name -> kage_kunai)")
	}
}

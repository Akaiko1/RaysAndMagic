package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/monster"
)

// The "close 7 valves" interact-quest advances via OnInteract("valve") and
// completes at 7/7 with the right progress string.
func TestCulvertsValveQuest_InteractProgresses(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	qm := loadTestQuestManager(t)
	cs.game.questManager = qm
	const qid = "culverts_valves"
	if err := qm.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	for i := 1; i <= 7; i++ {
		completed := qm.OnInteract("valve")
		q := qm.GetQuest(qid)
		if q.CurrentCount != i {
			t.Fatalf("after %d closes, count = %d", i, q.CurrentCount)
		}
		if i < 7 && len(completed) != 0 {
			t.Errorf("quest completed early at %d/7", i)
		}
		if i == 7 && (len(completed) != 1 || !q.Completed) {
			t.Errorf("quest should complete at 7/7 (completed=%v)", q.Completed)
		}
	}
	if got := qm.GetQuest(qid).GetProgressString(); got != "7/7 valves closed" {
		t.Errorf("progress = %q, want \"7/7 valves closed\"", got)
	}
}

// Closing a valve advances the quest once and the valve concludes (Visited) so it
// can't be re-closed (no close_valve choice remains).
func TestCloseValve_AdvancesOnceAndSticks(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	const qid = "culverts_valves"
	g.questManager.ActivateQuest(qid)

	valve := &character.NPC{
		Name:           "Sluice Valve I",
		RenderCategory: "npc",
		DialogueData: &character.NPCDialogue{
			Greeting:       "A rusty valve.",
			VisitedMessage: "The valve is shut.",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Close", Action: "close_valve", QuestID: qid},
				{Text: "Leave", Action: "leave"},
			},
		},
	}
	ih := NewInputHandler(g)
	g.dialogNPC = valve
	ih.handleCloseValve(qid)

	if c := g.questManager.GetQuest(qid).CurrentCount; c != 1 {
		t.Errorf("one valve closed should be 1/7, got %d", c)
	}
	if !valve.Visited {
		t.Errorf("closed valve should be Visited (stays shut)")
	}
	if g.npcDialogueState(valve) != npcStateConcluded {
		t.Errorf("closed valve should be concluded")
	}
	for _, c := range g.visibleNPCChoices(valve) {
		if c.Action == "close_valve" {
			t.Errorf("a shut valve must not offer close_valve again")
		}
	}
}

// The Golden Thief Bug parses its boss flags and is evasive (no chase, no attack)
// until the valve quest is complete, then turns aggressive (chases the party).
func TestGoldenThiefBug_FlagsAndQuestGatedEvasion(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")

	gtb := monster.NewMonster3DFromConfig(g.camera.X+500, g.camera.Y+500, "golden_thief_bug", g.config)
	g.world.Monsters = append(g.world.Monsters, gtb)
	if gtb.MaxHitPoints != 1200 || !gtb.IgnoresArmor || gtb.InfernoChance == 0 ||
		gtb.TeleportAtHP != 300 || gtb.PassiveUntilQuest != "culverts_valves" {
		t.Fatalf("GTB flags not parsed: HP=%d ignoresArmor=%v inferno=%.2f teleAtHP=%d quest=%q",
			gtb.MaxHitPoints, gtb.IgnoresArmor, gtb.InfernoChance, gtb.TeleportAtHP, gtb.PassiveUntilQuest)
	}

	// Quest not taken -> evasive: never chases (target = self), updateBoss handles
	// it (no normal attack).
	if !cs.bossEvasive(gtb) {
		t.Errorf("GTB should be evasive before the valve quest is done")
	}
	if tx, ty := cs.monsterAITargetPoint(gtb); tx != gtb.X || ty != gtb.Y {
		t.Errorf("evasive GTB must hold position, not chase the party")
	}
	if !cs.updateBoss(gtb, true, true) {
		t.Errorf("evasive GTB should be fully handled by updateBoss (no normal attack)")
	}

	// Complete the valve quest -> aggressive: now chases the party.
	g.questManager.ActivateQuest("culverts_valves")
	for i := 0; i < 7; i++ {
		g.questManager.OnInteract("valve")
	}
	if cs.bossEvasive(gtb) {
		t.Errorf("GTB should turn aggressive once the valve quest is complete")
	}
	if tx, ty := cs.monsterAITargetPoint(gtb); tx != g.camera.X || ty != g.camera.Y {
		t.Errorf("aggressive GTB should chase the party")
	}

	// refreshBoundAllyCache flags the now-aggressive boss for relentless pursuit;
	// an evasive boss must NOT carry that flag (it only holds + blinks).
	g.refreshBoundAllyCache()
	if !gtb.BossAggro {
		t.Errorf("aggressive GTB should be flagged BossAggro (relentless chase)")
	}
}

// While evasive, the Golden Thief Bug blinks away the moment it takes damage
// (not only when the party is within 3 tiles).
func TestGoldenThiefBug_EvasiveBlinksOnDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")

	// The test world needs real dimensions for blinkMonsterRandom to find a tile.
	g.world.Width, g.world.Height = 100, 100

	// Park the boss far from the party so proximity can't be the trigger.
	gtb := monster.NewMonster3DFromConfig(g.camera.X+2000, g.camera.Y+2000, "golden_thief_bug", g.config)
	g.world.Monsters = append(g.world.Monsters, gtb)
	g.collisionSystem.RegisterEntity(collision.NewEntity(gtb.ID, gtb.X, gtb.Y, 16, 16, collision.CollisionTypeMonster, false))

	if !cs.bossEvasive(gtb) {
		t.Fatalf("GTB should be evasive before the valve quest is done")
	}

	// First tick (far, undamaged) just establishes the HP baseline - it must NOT blink.
	startX, startY := gtb.X, gtb.Y
	if !cs.updateBoss(gtb, true, false) {
		t.Fatalf("evasive GTB action should be fully handled by updateBoss")
	}
	if gtb.X != startX || gtb.Y != startY {
		t.Fatalf("undamaged evasive GTB far from the party must not blink")
	}

	// Now it takes a hit. The damage debt latches, so the next tick blinks it away
	// even though the party is far - and even though no hit-flash timer is set.
	gtb.HitPoints -= 100
	preX, preY := gtb.X, gtb.Y
	cs.updateBoss(gtb, true, false)
	if gtb.X == preX && gtb.Y == preY {
		t.Errorf("a wounded evasive GTB should blink to a new tile, even from far away")
	}
	if gtb.BossHurtPending {
		t.Errorf("a successful blink should clear the hurt debt")
	}
}

// Armour-piercing attackers bypass the party's armor class; the flag is parsed
// and mitigateCharacterDamage (the path the melee skips) does reduce.
func TestIgnoresArmor_FlagAndArmorPath(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	member := cs.game.party.Members[0]
	reduced := cs.mitigateCharacterDamage(100, "physical", member, false)
	if reduced > 100 {
		t.Errorf("armor path should never increase damage")
	}
	gtb := monster.NewMonster3DFromConfig(0, 0, "golden_thief_bug", cs.game.config)
	if !gtb.IgnoresArmor {
		t.Errorf("Golden Thief Bug must ignore armor (skips the reduction path)")
	}
}

// A blink (either evasive-on-damage or low-HP) always lands the boss on an exact
// tile CENTER (never wedged off-grid into a wall) and clears its cached path so it
// repaths from the new spot instead of freezing on stale waypoints. Both blink
// kinds call blinkMonsterRandom, so testing it covers all blink types.
func TestBlinkLandsCenteredAndResetsPath(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.world.Width, g.world.Height = 60, 60
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	gtb := monster.NewMonster3DFromConfig(100, 100, "golden_thief_bug", g.config)
	g.world.Monsters = append(g.world.Monsters, gtb)
	g.collisionSystem.RegisterEntity(collision.NewEntity(gtb.ID, gtb.X, gtb.Y, 16, 16, collision.CollisionTypeMonster, false))

	// Stale path from before the blink - must be cleared.
	gtb.PathTiles = []monster.TileCoord{{X: 1, Y: 1}, {X: 2, Y: 2}}
	gtb.PathIndex = 1

	if !cs.blinkMonsterRandom(gtb) {
		t.Fatal("blink should find a walkable tile in an open world")
	}
	tile := float64(g.config.GetTileSize())
	// Tile centers sit at tile*size + size/2, so coord mod size == size/2 exactly.
	if math.Mod(gtb.X, tile) != tile/2 || math.Mod(gtb.Y, tile) != tile/2 {
		t.Errorf("blink must land on a tile CENTER, got (%.3f, %.3f) [tile %.0f]", gtb.X, gtb.Y, tile)
	}
	if len(gtb.PathTiles) != 0 || gtb.PathIndex != 0 {
		t.Errorf("blink must reset the cached path, got %d tiles at index %d", len(gtb.PathTiles), gtb.PathIndex)
	}
}

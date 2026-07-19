package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// makeDoorGame gives a crate-fixture game (items + npcs + loots loaded) plus a
// closed locked door registered on the world so the unlock flow can run.
func makeDoorGame(t *testing.T, npc *character.NPC) *MMGame {
	t.Helper()
	g := crateTestGame(t)
	g.lockedDoorEntityIDs = map[string]bool{}
	npc.X, npc.Y = g.camera.X+64, g.camera.Y
	g.world.NPCs = append(g.world.NPCs, npc)
	g.registerLockedDoors()
	return g
}

func woodenDoorNPC() *character.NPC {
	return &character.NPC{
		Name: "Wooden Door", Type: character.NPCTypeDoor, RenderCategory: "door",
		DoorBehavior: character.NPCDoorBehaviorLocked,
		LockLabel:    "wooden", DoorKeyItemKeys: []string{"ordinary_key"},
		DoorStatReqs: []character.NPCDoorStatReq{{Stat: "Might", Value: 55}},
		DialogueData: &character.NPCDialogue{Greeting: "barred"},
	}
}

func TestKeyItemsCarryDoorAttributes(t *testing.T) {
	crateTestGame(t) // loads item config + bridge
	for name, attr := range map[string]string{
		"ordinary_key": "door_key", "inlaid_key": "door_key", "skeleton_key": "master_key",
	} {
		it := items.CreateItemFromYAML(name)
		if it.Attributes[attr] <= 0 {
			t.Errorf("%s missing %s attribute: %+v", name, attr, it.Attributes)
		}
	}
	if !items.CreateItemFromYAML("skeleton_key").Stackable() {
		t.Error("keys are trinkets and must stack")
	}
}

func TestKeyTooltipsShareYAMLMechanicsWithEditor(t *testing.T) {
	g := crateTestGame(t)
	cases := []struct {
		key     string
		effects []string
		usage   []string
	}{
		{"ordinary_key", []string{"Opens wooden doors"}, []string{"Choose at a locked door", "Consumed when it opens a door"}},
		{"inlaid_key", []string{"Opens reinforced and steel doors"}, []string{"Choose at a locked door", "Consumed when it opens a door"}},
		{"skeleton_key", []string{"Opens any locked door"}, []string{"Choose at a locked door", "Never consumed"}},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			def, ok := config.GetItemDefinition(tc.key)
			if !ok || def == nil {
				t.Fatalf("missing item definition for %q", tc.key)
			}
			if got, want := strings.Join(def.TooltipEffects, "\n"), strings.Join(tc.effects, "\n"); got != want {
				t.Errorf("tooltip_effects = %q, want %q", got, want)
			}
			if got, want := strings.Join(def.TooltipUsage, "\n"), strings.Join(tc.usage, "\n"); got != want {
				t.Errorf("tooltip_usage = %q, want %q", got, want)
			}

			tooltip := GetItemTooltip(items.CreateItemFromYAML(tc.key), nil, g.combat, false)
			editorCard := strings.Join(character.RenderCardLines(character.ItemCardSections(def), true), "\n")
			for _, line := range append(append([]string{}, def.TooltipEffects...), def.TooltipUsageLines()...) {
				if !strings.Contains(tooltip, line) {
					t.Errorf("game tooltip missing YAML line %q:\n%s", line, tooltip)
				}
				if !strings.Contains(editorCard, line) {
					t.Errorf("editor card missing YAML line %q:\n%s", line, editorCard)
				}
			}
			if strings.Contains(tooltip, "Collectible; sell to merchants") {
				t.Errorf("key tooltip must not use generic trinket usage:\n%s", tooltip)
			}
		})
	}
}

func TestAvailableDoorUnlocks_KeyStatAndMaster(t *testing.T) {
	npc := woodenDoorNPC()
	g := makeDoorGame(t, npc)
	g.party.Members[0].Might = 10 // no one can force it by default

	// Nothing in the bag, no strong member -> no options -> sealed message.
	if opts := g.availableDoorUnlocks(npc); len(opts) != 0 {
		t.Fatalf("expected no unlocks, got %+v", opts)
	}
	if !npcHasChoiceDialog(npc) {
		t.Fatal("a locked door must use the choice-dialog path even with no unlock")
	}
	if choices := g.visibleNPCChoices(npc); len(choices) != 1 || choices[0].Action != "leave" {
		t.Fatalf("sealed door choices = %+v, want only Leave", choices)
	}
	if len(npc.DialogueData.Choices) != 0 {
		t.Fatalf("derived door choices mutated authored dialogue: %+v", npc.DialogueData.Choices)
	}
	if g.npcDialogueText(npc) == npc.DialogueData.Greeting {
		t.Error("a sealed door must show the sealed notice, not the authored greeting")
	}

	// A matching key adds one consumable option.
	g.party.AddItem(items.CreateItemFromYAML("ordinary_key"))
	if opts := g.availableDoorUnlocks(npc); len(opts) != 1 || opts[0].keyItemKey != "ordinary_key" {
		t.Fatalf("ordinary key should give one key unlock, got %+v", opts)
	}
	if choices := g.visibleNPCChoices(npc); len(choices) != 2 || choices[0].Action != "open_door" || choices[1].Action != "leave" {
		t.Fatalf("door choices with a key = %+v, want Open + Leave", choices)
	}

	// A strong member adds the force option (order: keys then forcings).
	g.party.Members[0].Might = 60
	opts := g.availableDoorUnlocks(npc)
	if len(opts) != 2 || !opts[1].force {
		t.Fatalf("might 60 should add a force option, got %+v", opts)
	}

	// The Skeleton Key leads and is a master unlock.
	g.party.AddItem(items.CreateItemFromYAML("skeleton_key"))
	opts = g.availableDoorUnlocks(npc)
	if len(opts) == 0 || opts[0].masterKeyName != "Skeleton Key" {
		t.Fatalf("skeleton key should lead the option list, got %+v", opts)
	}
}

func TestOpenLockedDoor_ConsumesKeyButNotSkeleton(t *testing.T) {
	t.Run("ordinary key is consumed", func(t *testing.T) {
		npc := woodenDoorNPC()
		g := makeDoorGame(t, npc)
		g.party.AddItem(items.CreateItemFromYAML("ordinary_key"))
		g.dialogNPC = npc
		g.openLockedDoor(npc, 0)
		if !npc.Visited {
			t.Fatal("door did not open")
		}
		if g.party.CountItemsByName("Ordinary Key") != 0 {
			t.Error("ordinary key must be consumed opening a door")
		}
		if g.lockedDoorEntityIDs[lockedDoorEntityID(npc)] {
			t.Error("opened door must drop its collision block")
		}
	})
	t.Run("skeleton key is never spent", func(t *testing.T) {
		npc := woodenDoorNPC()
		g := makeDoorGame(t, npc)
		g.party.AddItem(items.CreateItemFromYAML("skeleton_key"))
		g.dialogNPC = npc
		g.openLockedDoor(npc, 0) // master key leads
		if !npc.Visited {
			t.Fatal("skeleton key did not open the door")
		}
		if g.party.CountItemsByName("Skeleton Key") != 1 {
			t.Error("skeleton key must NOT be consumed - it opens every door forever")
		}
	})
}

func TestValidateDoorNPCs(t *testing.T) {
	good := map[string]*character.NPCData{
		"d": {Type: character.NPCTypeDoor, RenderCategory: "door",
			DoorBehavior:    character.NPCDoorBehaviorLocked,
			DoorKeyItemKeys: []string{"ordinary_key"},
			Dialogue:        &character.NPCDialogue{}},
	}
	crateTestGame(t) // item config for the key-name existence check
	if err := ValidateDoorNPCs(good); err != nil {
		t.Fatalf("valid door rejected: %v", err)
	}
	bad := []map[string]*character.NPCData{
		{"typeonly": {Type: character.NPCTypeDoor, RenderCategory: "scenery", DoorBehavior: character.NPCDoorBehaviorLocked, DoorKeyItemKeys: []string{"ordinary_key"}}},
		{"renderonly": {Type: character.NPCTypeEncounter, RenderCategory: "door"}},
		{"badstat": {Type: character.NPCTypeDoor, RenderCategory: "door", DoorBehavior: character.NPCDoorBehaviorLocked, DoorStatReqs: []character.NPCDoorStatReq{{Stat: "Luck", Value: 10}}, Dialogue: &character.NPCDialogue{}}},
		{"noreqs": {Type: character.NPCTypeDoor, RenderCategory: "door", DoorBehavior: character.NPCDoorBehaviorLocked, Dialogue: &character.NPCDialogue{}}},
		{"badkey": {Type: character.NPCTypeDoor, RenderCategory: "door", DoorBehavior: character.NPCDoorBehaviorLocked, DoorKeyItemKeys: []string{"no_such_key"}, Dialogue: &character.NPCDialogue{}}},
		{"notakey": {Type: character.NPCTypeDoor, RenderCategory: "door", DoorBehavior: character.NPCDoorBehaviorLocked, DoorKeyItemKeys: []string{"ruby"}, Dialogue: &character.NPCDialogue{}}},
		{"badbehavior": {Type: character.NPCTypeDoor, RenderCategory: "door", DoorBehavior: "mystery"}},
	}
	for _, m := range bad {
		if err := ValidateDoorNPCs(m); err == nil {
			t.Errorf("mis-authored door accepted: %+v", m)
		}
	}
}

func TestShippedDoorNPCsValidate(t *testing.T) {
	crateTestGame(t) // loads npcs + items
	if err := ValidateDoorNPCs(character.NPCConfigInstance.NPCs); err != nil {
		t.Fatalf("shipped door NPCs failed validation: %v", err)
	}
}

func TestSaveLoad_OpenedLockedDoorDoesNotReblock(t *testing.T) {
	cfg := loadTestConfig(t)
	const mapKey = "door_save_test"
	tile := float64(cfg.GetTileSize())
	newWorld := func(visited bool) *world.World3D {
		w := newTestWorldSized(cfg, 12, 12)
		door := woodenDoorNPC()
		door.X, door.Y = TileCenterFromTile(5, 5, tile)
		door.Visited = visited
		w.NPCs = append(w.NPCs, door)
		return w
	}
	newManager := func(w *world.World3D) *world.WorldManager {
		wm := world.NewWorldManager(cfg)
		wm.CurrentMapKey = mapKey
		wm.LoadedMaps = map[string]*world.World3D{mapKey: w}
		return wm
	}

	wmSave := newManager(newWorld(true))
	save := newTestGame(cfg, wmSave.GetCurrentWorld()).buildSave(wmSave)
	wmLoad := newManager(newWorld(false))
	loaded := newTestGame(cfg, wmLoad.GetCurrentWorld())
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	door := wmLoad.GetCurrentWorld().NPCs[0]
	if !door.Visited {
		t.Fatal("opened door lost its visited state")
	}
	if loaded.collisionSystem.CanMoveTo("player", door.X, door.Y) == false {
		t.Fatal("opened door restored as an invisible collision block")
	}
}

func TestDoorKeyLootAndSkeletonKeyPolicy(t *testing.T) {
	g := crateTestGame(t)
	cs := g.combat

	wantOwners := map[string]map[string]bool{
		"ordinary_key": {"bandit": true, "forest_orc": true, "thief_bug": true},
		"inlaid_key": {
			"ashigaru_firelock": true, "ronin_marksman": true, "possessed_tome": true, "alarm_clock": true, "grandfather_clock": true,
			"elf_archer": true, "elf_swordsman": true, "archmage": true, "lich": true, "lich_king": true,
		},
	}
	actualOwners := map[string]map[string]bool{"ordinary_key": {}, "inlaid_key": {}}
	bossLoot := config.GetBossLoot()
	if len(bossLoot) != 1 || bossLoot[0].Type != "item" || bossLoot[0].Key != "skeleton_key" || bossLoot[0].Chance != 0.05 {
		t.Fatalf("boss_loot = %+v, want one 5%% Skeleton Key entry", bossLoot)
	}
	for _, monsterKey := range monsterPkg.MonsterConfig.GetAllMonsterKeys() {
		monster := monsterPkg.NewMonster3DFromConfig(0, 0, monsterKey, g.config)
		for _, entry := range cs.monsterLootEntries(monster) {
			if owners, tracked := actualOwners[entry.Key]; tracked {
				if monster.IsBoss() {
					t.Errorf("boss %s authors %s; ordinary and inlaid keys belong only to normal mobs", monsterKey, entry.Key)
				}
				owners[monsterKey] = true
			}
		}
		hasSkeletonKey := false
		for _, entry := range cs.monsterLootEntries(monster) {
			if entry.Key == "skeleton_key" {
				hasSkeletonKey = true
			}
		}
		if hasSkeletonKey != monster.IsBoss() {
			t.Errorf("normal loot for %s includes Skeleton Key = %v, want %v", monsterKey, hasSkeletonKey, monster.IsBoss())
		}
	}
	for key, want := range wantOwners {
		if got := actualOwners[key]; len(got) != len(want) {
			t.Errorf("%s owners = %v, want %v", key, got, want)
			continue
		}
		for monsterKey := range want {
			if !actualOwners[key][monsterKey] {
				t.Errorf("%s missing from normal monster %s", key, monsterKey)
			}
		}
	}
}

func TestBossLootUsesTheSameTableForDeathAndSleightOfHand(t *testing.T) {
	g, thief := newThiefTestGame(t)
	previousLoots := config.GlobalLoots
	t.Cleanup(func() { config.GlobalLoots = previousLoots })
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	// Force the table entry to land so this test exercises the two real loot
	// paths rather than relying on a 5%% sample.
	config.GlobalLoots.BossLoot = []config.LootEntry{{Type: "item", Key: "skeleton_key", Chance: 1}}

	boss := monsterPkg.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, "golden_thief_bug", g.config)
	if boss == nil || !boss.IsBoss() {
		t.Fatal("golden_thief_bug must be a configured boss")
	}

	containsSkeletonKey := func(drops []items.Item) bool {
		for _, drop := range drops {
			if drop.Name == "Skeleton Key" {
				return true
			}
		}
		return false
	}
	if !containsSkeletonKey(g.combat.checkMonsterLootDrop(boss)) {
		t.Fatal("boss death loot did not include Skeleton Key from the normal table")
	}

	thief.Skills[character.SkillSleightOfHand].Mastery = character.MasteryGrandMaster
	for attempts := 0; attempts < 500 && !boss.Pilfered; attempts++ {
		g.combat.trySleightOfHand(thief, boss)
	}
	if !boss.Pilfered {
		t.Fatal("Sleight of Hand did not land in 500 attempts")
	}
	if !containsSkeletonKey(g.party.Inventory) {
		t.Fatal("Sleight of Hand could not steal Skeleton Key from a boss")
	}
}

func TestArenaGateIsPlacedAndClassifiedAsADoor(t *testing.T) {
	g := crateTestGame(t)
	wm, _ := loadRealWorldForTest(t, g.config, "")
	w := wm.LoadedMaps["arena"]
	if w == nil {
		t.Fatal("arena map missing")
	}
	if err := wm.SwitchToMap("arena"); err != nil {
		t.Fatalf("switch to arena: %v", err)
	}
	var gate *character.NPC
	for _, npc := range w.NPCs {
		if npc != nil && npc.Key == "arena_gate" {
			gate = npc
			break
		}
	}
	if gate == nil {
		t.Fatal("arena_gate is not placed on arena")
	}
	if gate.RenderCategory != "door" || !character.IsDoor(gate) {
		t.Fatalf("arena_gate is not a classified door: %+v", gate)
	}
	mapGame := newTestGame(g.config, w)
	if _, _, _, ok := mapGame.doorPose(gate.X, gate.Y); !ok {
		t.Error("arena_gate is not placed in a doorway")
	}
	if gate.DoorBehavior != character.NPCDoorBehaviorChampionPortcullis {
		t.Fatalf("arena_gate door behavior = %q, want champion_portcullis", gate.DoorBehavior)
	}
}

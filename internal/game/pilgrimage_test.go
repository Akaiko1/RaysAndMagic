package game

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

func TestPilgrimageItemCategoriesAndSavedCopies(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, cfg := bootOpenWorldGame(t, false)
	// Each row covers fresh creation and legacy copies in every persistent
	// equipment location. Cloth needs no armor skill; class gates still apply.
	cases := []struct {
		key      string
		itemType items.ItemType
		category string
		slot     items.EquipSlot
	}{
		{"unbroken_headband", items.ItemArmor, "cloth", items.SlotHelmet},
		{"unbroken_handwraps", items.ItemArmor, "cloth", items.SlotGauntlets},
		{"unbroken_sandals", items.ItemArmor, "cloth", items.SlotBoots},
		{"unbroken_sash", items.ItemAccessory, "", items.SlotBelt},
		{"unbroken_mantle", items.ItemAccessory, "", items.SlotCloak},
		{"unbroken_robe", items.ItemArmor, "cloth", items.SlotArmor},
	}
	active := character.CreateCharacter("Active Pilgrim", character.ClassMonk, cfg)
	reserve := character.CreateCharacter("Reserve Pilgrim", character.ClassCleric, cfg)
	captive := character.CreateCharacter("Captive Pilgrim", character.ClassMonk, cfg)
	g.party = &character.Party{Members: []*character.MMCharacter{active}, Reserve: []*character.MMCharacter{reserve}, Captive: []*character.MMCharacter{captive}}
	chest := &stash.Stash{}
	bag := GroundContainer{Kind: ContainerKindLootBag, ID: "pilgrim-category-test", MapKey: wm.CurrentMapKey, X: g.camera.X, Y: g.camera.Y}
	for i, tc := range cases {
		item, err := items.TryCreateItemFromYAML(tc.key)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(tc.key+"/fresh", func(t *testing.T) {
			if item.Type != tc.itemType || item.ArmorCategory != tc.category || item.PreferredSlot(items.SlotArmor) != tc.slot {
				t.Fatalf("fresh category/slot mismatch: %+v", item)
			}
		})
		// Reproduce the original five accessory definitions in an older save.
		if tc.key != "unbroken_robe" {
			item.Type, item.ArmorCategory = items.ItemAccessory, ""
		}
		copyItem := func() items.Item {
			copy := item
			copy.InstanceID = 0
			items.EnsureInstanceID(&copy)
			return copy
		}
		g.party.Inventory = append(g.party.Inventory, copyItem())
		for _, member := range []*character.MMCharacter{active, reserve, captive} {
			member.Equipment[tc.slot] = copyItem()
		}
		bag.Items = append(bag.Items, copyItem())
		chest.Slots[i] = copyItem()
	}
	g.groundContainers = []GroundContainer{bag}
	if err := stash.Save(chest); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err := json.Unmarshal(data, &save); err != nil {
		t.Fatal(err)
	}
	g.stash = nil
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		for location, item := range map[string]items.Item{
			"inventory": g.party.Inventory[i],
			"active":    g.party.Members[0].Equipment[tc.slot],
			"reserve":   g.party.Reserve[0].Equipment[tc.slot],
			"captive":   g.party.Captive[0].Equipment[tc.slot],
			"loot":      g.groundContainers[0].Items[i],
			"stash":     g.stash.Slots[i],
		} {
			t.Run(tc.key+"/saved_"+location, func(t *testing.T) {
				if item.Type != tc.itemType || item.ArmorCategory != tc.category || item.PreferredSlot(items.SlotArmor) != tc.slot {
					t.Fatalf("saved category/slot mismatch: %+v", item)
				}
				if !g.party.Members[0].ItemFitsSlot(item, tc.slot) {
					t.Fatal("classification change made the item ineligible for the Monk")
				}
			})
		}
	}
}

func TestPilgrimageSetEligibilityAndCap(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	cs := NewCombatSystem(g)
	set := config.GetItemSet("unbroken_road")
	for key := range cfg.Characters.Classes {
		class, ok := character.ClassFromKey(key)
		if !ok {
			t.Fatal(key)
		}
		t.Run(key, func(t *testing.T) {
			c := character.CreateCharacter("Pilgrim", class, cfg)
			c.Equipment = map[items.EquipSlot]items.Item{}
			c.Skills = map[character.SkillType]*character.Skill{}
			allowed := class == character.ClassMonk || class == character.ClassCleric
			for i, key := range set.RequiredPieces {
				item, err := items.TryCreateItemFromYAML(key)
				if err != nil {
					t.Fatal(err)
				}
				// JSON rehydration must not lose class eligibility.
				b, _ := json.Marshal(item)
				var restored items.Item
				json.Unmarshal(b, &restored)
				slot := restored.PreferredSlot(items.SlotArmor)
				if c.ItemFitsSlot(restored, slot) != allowed {
					t.Fatalf("%s eligibility", key)
				}
				if _, _, ok := c.EquipItemToSlot(restored, slot); ok != allowed {
					t.Fatalf("%s equip", key)
				}
				if i < 5 && c.SetArmorClassBonus() != 0 {
					t.Fatal("incomplete set grants armor")
				}
			}
			if !allowed {
				return
			}
			if c.GetEffectiveAccuracy() != c.Accuracy || c.GetEffectiveMight() != c.Might+16 || c.GetEffectiveLuck() != c.Luck+4 {
				t.Fatal("set must improve monk melee and luck without a ranged Accuracy bonus")
			}
			if got := cs.CalculateTotalArmorClass(c); got != 95 {
				t.Fatalf("set AC=%d want95", got)
			}
			if c.GetEffectivePersonality() != c.Personality+18 {
				t.Fatal("set personality bonus missing")
			}
			if class == character.ClassMonk {
				for tier := character.MasteryNovice; tier <= character.MasteryGrandMaster; tier++ {
					c.Skills[character.SkillIronBody] = &character.Skill{Mastery: tier}
					if got := cs.CalculateTotalArmorClass(c); got != 95+(int(tier)+1)*10 {
						t.Fatalf("tier%d AC%d", tier, got)
					}
				}
			}
		})
	}
}

func TestPilgrimageShippedChain(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, unified := range []bool{false, true} {
		t.Run(map[bool]string{false: "split", true: "stitched"}[unified], func(t *testing.T) {
			g, wm, _ := bootOpenWorldGame(t, unified)
			if err := g.activateQuest("unbroken_mountain"); err == nil {
				t.Fatal("L1 accepted L15 trial")
			}
			if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "pilgrimage"); err != nil {
				t.Fatal(err)
			}
			tx, ty := projectTileToCurrentWorld("highlands", 21, 42)
			px, py := TileCenterFromTile(tx, ty, float64(g.config.GetTileSize()))
			if g.camera.X != px || g.camera.Y != py {
				t.Fatal("scenario camera position")
			}
			for _, m := range g.world.Monsters {
				if math.Hypot(m.X-px, m.Y-py) <= 5*float64(g.config.GetTileSize()) {
					t.Fatal("unsafe scenario arrival")
				}
			}
			if len(g.party.Members) != 4 || g.party.Members[0].Class != character.ClassMonk {
				t.Fatal("scenario party")
			}
			for _, m := range g.party.Members {
				if m.Level != 15 {
					t.Fatalf("level%d", m.Level)
				}
			}
			if g.party.Members[0].SkillTier(character.SkillIronBody) != 1 {
				t.Fatal("scenario silently granted GM")
			}
			if err := g.activateQuest("unbroken_mountain"); err != nil {
				t.Fatal(err)
			}
			g.flushPendingQuestSpawns()
			if err := g.activateQuest("unbroken_mountain"); err == nil {
				t.Fatal("duplicate activation")
			}
			count := 0
			for _, m := range wm.WorldByKey("highlands").Monsters {
				if m.Name == "Bronze Gatekeeper" || m.Name == "Gale Novice" {
					count++
					m.HitPoints = 0
					g.combat.finishMonsterKill(m)
				}
			}
			if count != 2 {
				t.Fatalf("unique enemies %d", count)
			}
			g.gameLoop.removeDeadMonstersByID()
			before := g.party.GetTotalItems()
			var mira *character.NPC
			for _, n := range g.allLoadedNPCs() {
				if n.Key == "sister_mira" {
					mira = n
					break
				}
			}
			if mira == nil {
				t.Fatal("Mira missing")
			}
			g.dialogNPC = mira
			(&InputHandler{game: g}).handleTurnInQuest("unbroken_mountain")
			if mira.Visited || g.claimQuestReward("unbroken_mountain") {
				t.Fatal("Mira concluded early or duplicate claim")
			}
			if g.party.GetTotalItems() != before+2 {
				t.Fatal("first rewards not exactly two")
			}
			q := g.questManager.GetQuest("unbroken_desert")
			if q == nil {
				t.Fatal("next chapter missing")
			}
			interact := func(key string) {
				var npc *character.NPC
				for _, n := range g.allLoadedNPCs() {
					if n.Key == key {
						npc = n
						break
					}
				}
				if npc == nil {
					t.Fatal("missing NPC", key)
				}
				id, p := activityProp(npc)
				g.dialogNPC = npc
				(&InputHandler{game: g}).handleQuestPropInteract(id, p)
			}
			interact("pilgrim_sluice_spring")
			interact("pilgrim_sluice_monastery")
			if q.Activity.SequenceIndex != 0 || q.Completed {
				t.Fatal("wrong order did not reset")
			}
			interact("pilgrim_sluice_spring")
			save := g.buildSave(wm)
			bytes, _ := json.Marshal(save)
			var restored GameSave
			json.Unmarshal(bytes, &restored)
			q.Activity.SequenceIndex = 0
			if err := g.applySave(wm, &restored); err != nil {
				t.Fatal(err)
			}
			q = g.questManager.GetQuest("unbroken_desert")
			if q.Activity.SequenceIndex != 1 {
				t.Fatal("puzzle progress lost")
			}
			interact("pilgrim_sluice_travelers")
			interact("pilgrim_sluice_monastery")
			if !g.claimQuestReward("unbroken_desert") {
				t.Fatal("desert incomplete")
			}
			q = g.questManager.GetQuest("unbroken_jungle")
			selected := append([]string(nil), q.Activity.Selected...)
			if len(selected) != 5 {
				t.Fatal("flower count")
			}
			for _, npc := range g.allLoadedNPCs() {
				id, prop := activityProp(npc)
				if id != "unbroken_jungle" {
					continue
				}
				if g.npcMapMarkerVisible(npc) {
					t.Fatal("search flower leaked onto map")
				}
				if g.npcAbsent(npc) == slices.Contains(selected, prop.Token) {
					t.Fatal("wrong flower visibility")
				}
				g.dayNightIsNight = q.Definition.Activity.TokenPhase(prop.Token) != "night"
				if g.activityNPCSprite(npc) != prop.DormantSprite {
					t.Fatal("wrong-phase flower did not close")
				}
			}
			if !g.npcMapMarkerVisible(mira) {
				t.Fatal("quest giver lost map marker")
			}
			day, night := 0, 0
			for _, token := range selected {
				if q.Definition.Activity.TokenPhase(token) == "day" {
					day++
				} else {
					night++
				}
			}
			if day != 3 || night != 2 {
				t.Fatalf("day%d night%d", day, night)
			}
			save = g.buildSave(wm)
			bytes, _ = json.Marshal(save)
			json.Unmarshal(bytes, &restored)
			q.Activity.Selected = nil
			if err := g.applySave(wm, &restored); err != nil {
				t.Fatal(err)
			}
			q = g.questManager.GetQuest("unbroken_jungle")
			if !reflect.DeepEqual(q.Activity.Selected, selected) {
				t.Fatal("flowers rerolled on load")
			}
			for i, token := range selected {
				isNight := q.Definition.Activity.TokenPhase(token) == "night"
				g.dayNightIsNight = !isNight
				old := q.CurrentCount
				interact("pilgrim_" + token)
				if q.CurrentCount != old {
					t.Fatal("wrong phase collected")
				}
				g.dayNightIsNight = isNight
				interact("pilgrim_" + token)
				for _, npc := range g.allLoadedNPCs() {
					if npc.Key == "pilgrim_"+token {
						t.Fatal("collected flower remains in the physical roster")
					}
				}
				g.questManager.InteractActivity(q.ID, "pilgrim_flower", token, isNight)
				if q.CurrentCount != old+1 {
					t.Fatal("flower not counted once")
				}
				if i == 0 {
					snapshot := g.buildSave(wm)
					raw, err := json.Marshal(snapshot)
					if err != nil {
						t.Fatal(err)
					}
					var loaded GameSave
					if err := json.Unmarshal(raw, &loaded); err != nil {
						t.Fatal(err)
					}
					if err := g.applySave(wm, &loaded); err != nil {
						t.Fatal(err)
					}
					q = g.questManager.GetQuest("unbroken_jungle")
					if len(q.Activity.Collected) != 1 || q.Activity.Collected[0] != token {
						t.Fatal("collected flower returned after load")
					}
				}
			}
			if !g.claimQuestReward("unbroken_jungle") {
				t.Fatal("jungle incomplete")
			}
			if g.party.GetTotalItems() != before+6 {
				t.Fatal("set not exactly six rewards")
			}
			// Saving/loading and reconciliation must never summon extra guardians.
			save = g.buildSave(wm)
			g.restoreSavedQuests(&save)
			g.flushPendingQuestSpawns()
			count = 0
			for _, m := range wm.WorldByKey("highlands").Monsters {
				if m.Name == "Bronze Gatekeeper" || m.Name == "Gale Novice" {
					count++
				}
			}
			if count != 0 {
				t.Fatalf("respawned guardians %d", count)
			}
		})
	}
}

func TestPilgrimagePropsHaveReachableDistinctPlacements(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	for _, mapKey := range []string{"highlands", "desert", "deep_jungle"} {
		w := wm.WorldByKey(mapKey)
		// Use authored spawn/arrival connectivity, without Fly or water passage.
		var start [2]int
		switch mapKey {
		case "highlands":
			start = [2]int{20, 45}
		case "desert":
			start = [2]int{31, 20}
		case "deep_jungle":
			start = [2]int{4, 47}
		}
		seen := map[[2]int]bool{start: true}
		queue := [][2]int{start}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := [2]int{p[0] + d[0], p[1] + d[1]}
				if seen[n] || w.IsTileBlockingTerrainAt(n[0], n[1]) {
					continue
				}
				seen[n] = true
				queue = append(queue, n)
			}
		}
		count := 0
		tokens := map[string]bool{}
		candidates := append([]*character.NPC(nil), w.NPCs...)
		for _, def := range g.questManager.Definitions() {
			for _, layout := range def.PropLayouts() {
				for _, p := range layout.Props {
					if p.Map != mapKey {
						continue
					}
					x, y := TileCenterFromTile(p.X, p.Y, g.config.GetTileSize())
					npc, err := character.CreateNPCFromConfig(p.NPC, x, y)
					if err != nil {
						t.Fatal(err)
					}
					candidates = append(candidates, npc)
				}
			}
		}
		for _, npc := range candidates {
			_, prop := activityProp(npc)
			if prop == nil && npc.Key != "sister_mira" {
				continue
			}
			pos := [2]int{TileIndex(npc.X, g.config.GetTileSize()), TileIndex(npc.Y, g.config.GetTileSize())}
			if !seen[pos] {
				t.Errorf("%s at %v unreachable from %v", npc.Key, pos, start)
			}
			if prop != nil {
				identity := fmt.Sprintf("%s:%v", prop.Token, pos)
				if tokens[identity] {
					t.Error("duplicate placed token", prop.Token)
				}
				tokens[identity] = true
			}
			count++
		}
		want := map[string]int{"highlands": 1, "desert": 9, "deep_jungle": 15}[mapKey]
		if count != want {
			t.Errorf("%s props%d want%d", mapKey, count, want)
		}
	}
}

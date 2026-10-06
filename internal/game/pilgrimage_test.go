package game

import (
	"encoding/json"
	"fmt"
	"maps"
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
	// Expected stats are the sums of every authored piece and the set bonus.
	var might, luck, personality, accuracy, armor int
	var allowedClasses []string
	for i, key := range set.RequiredPieces {
		d, ok := config.GetItemDefinition(key)
		if !ok {
			t.Fatalf("set piece %s missing", key)
		}
		if i == 0 {
			allowedClasses = d.AllowedClasses
		} else if !slices.Equal(allowedClasses, d.AllowedClasses) {
			t.Fatalf("%s allows %v, unlike the rest of the set %v", key, d.AllowedClasses, allowedClasses)
		}
		might += d.BonusMight
		luck += d.BonusLuck
		personality += d.BonusPersonality
		accuracy += d.BonusAccuracy
		armor += d.ArmorClassBase
	}
	might += set.BonusMight
	luck += set.BonusLuck
	personality += set.BonusPersonality
	accuracy += set.BonusAccuracy
	armor += set.BonusArmorClass
	if len(allowedClasses) == 0 || len(allowedClasses) == len(cfg.Characters.Classes) {
		t.Fatalf("the set must be class-gated to exercise both outcomes, allows %v", allowedClasses)
	}
	for key := range cfg.Characters.Classes {
		class, ok := character.ClassFromKey(key)
		if !ok {
			t.Fatal(key)
		}
		t.Run(key, func(t *testing.T) {
			c := character.CreateCharacter("Pilgrim", class, cfg)
			c.Equipment = map[items.EquipSlot]items.Item{}
			c.Skills = map[character.SkillType]*character.Skill{}
			allowed := slices.Contains(allowedClasses, key)
			bareAC := cs.CalculateTotalArmorClass(c)
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
				if i < set.RequiredPieceCount()-1 && c.SetArmorClassBonus() != 0 {
					t.Fatal("incomplete set grants armor")
				}
			}
			if !allowed {
				return
			}
			if c.GetEffectiveAccuracy() != c.Accuracy+accuracy || c.GetEffectiveMight() != c.Might+might || c.GetEffectiveLuck() != c.Luck+luck {
				t.Fatal("set Might/Luck/Accuracy differ from the authored pieces and bonus")
			}
			wantAC := bareAC + armor
			if got := cs.CalculateTotalArmorClass(c); got != wantAC {
				t.Fatalf("set AC=%d want %d", got, wantAC)
			}
			if c.GetEffectivePersonality() != c.Personality+personality {
				t.Fatal("set personality bonus missing")
			}
			if class == character.ClassMonk {
				for tier := character.MasteryNovice; tier <= character.MasteryGrandMaster; tier++ {
					c.Skills[character.SkillIronBody] = &character.Skill{Mastery: tier}
					if got := cs.CalculateTotalArmorClass(c); got != wantAC+(int(tier)+1)*character.IronBodyACPerTier {
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
			scenarios, err := LoadTestScenarios("assets/test_scenarios.yaml")
			if err != nil {
				t.Fatal(err)
			}
			sc := scenarios["pilgrimage"]
			tx, ty := projectTileToCurrentWorld(sc.Map, sc.X, sc.Y)
			px, py := TileCenterFromTile(tx, ty, float64(g.config.GetTileSize()))
			if g.camera.X != px || g.camera.Y != py {
				t.Fatal("scenario camera position")
			}
			for _, m := range g.world.Monsters {
				if math.Hypot(m.X-px, m.Y-py) <= sc.SafeRadius*float64(g.config.GetTileSize()) {
					t.Fatal("unsafe scenario arrival")
				}
			}
			if len(g.party.Members) != len(sc.Party) {
				t.Fatal("scenario party")
			}
			for i, member := range sc.Party {
				m := g.party.Members[i]
				if class, _ := character.ClassFromKey(member.Class); m.Class != class || m.Level != sc.Level {
					t.Fatalf("member %d: class %v level %d", i, m.Class, m.Level)
				}
				for key, tier := range member.Skills {
					skill, _ := character.SkillTypeFromKey(key)
					if mastery, _ := character.MasteryFromKey(tier); m.SkillTier(skill) != int(mastery) {
						t.Fatalf("member %d %s at tier %d, scenario authors %s", i, key, m.SkillTier(skill), tier)
					}
				}
			}
			defs := g.questManager.Definitions()
			chain := []string{"unbroken_mountain", "unbroken_desert", "unbroken_jungle"}
			guardians, guardianMaps := map[string]bool{}, map[string]bool{}
			for _, spawn := range defs[chain[0]].OnAcceptSpawns {
				guardians[spawn.Monster], guardianMaps[spawn.Map] = true, true
			}
			if len(guardians) == 0 {
				t.Fatal("the first chapter must summon its guardians")
			}
			// countGuardians counts (and optionally kills) the summoned guardians.
			countGuardians := func(kill bool) int {
				n := 0
				for mapKey := range guardianMaps {
					for _, m := range wm.WorldByKey(mapKey).Monsters {
						if guardians[m.Key] {
							n++
							if kill {
								m.HitPoints = 0
								g.combat.finishMonsterKill(m)
							}
						}
					}
				}
				return n
			}
			rewardsThrough := func(last int) int {
				n := 0
				for _, id := range chain[:last+1] {
					n += len(defs[id].Rewards.Items)
				}
				return n
			}
			if err := g.activateQuest("unbroken_mountain"); err != nil {
				t.Fatal(err)
			}
			g.flushPendingQuestSpawns()
			if err := g.activateQuest("unbroken_mountain"); err == nil {
				t.Fatal("duplicate activation")
			}
			if count := countGuardians(true); count != len(defs[chain[0]].OnAcceptSpawns) {
				t.Fatalf("unique enemies %d, want %d", count, len(defs[chain[0]].OnAcceptSpawns))
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
			if g.party.GetTotalItems() != before+rewardsThrough(0) {
				t.Fatalf("first rewards not exactly %d", rewardsThrough(0))
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
			wantPhase := map[string]int{}
			for _, group := range q.Definition.Activity.Forage {
				wantPhase[group.Phase] += group.Count
			}
			if len(selected) != wantPhase["day"]+wantPhase["night"] {
				t.Fatalf("flower count %d, want %v", len(selected), wantPhase)
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
			if day != wantPhase["day"] || night != wantPhase["night"] {
				t.Fatalf("day%d night%d, want %v", day, night, wantPhase)
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
			if g.party.GetTotalItems() != before+rewardsThrough(len(chain)-1) {
				t.Fatalf("set not exactly %d rewards", rewardsThrough(len(chain)-1))
			}
			// Saving/loading and reconciliation must never summon extra guardians.
			save = g.buildSave(wm)
			g.restoreSavedQuests(&save)
			g.flushPendingQuestSpawns()
			if count := countGuardians(false); count != 0 {
				t.Fatalf("respawned guardians %d", count)
			}
		})
	}
}

// Every pilgrimage prop layout and the chain's quest giver stand on ground
// reachable from the map's authored arrival points (start tile, teleporters),
// without Fly or water passage, and no token repeats a placement.
func TestPilgrimagePropsHaveReachableDistinctPlacements(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	defs := g.questManager.Definitions()
	chain := map[string]bool{}
	for id := "unbroken_mountain"; id != "" && !chain[id]; id = defs[id].NextQuest {
		chain[id] = true
	}
	offersChain := func(npc *character.NPC) bool {
		found := false
		_ = npc.DialogueData.WalkChoices(func(choice *character.NPCDialogueChoice) error {
			found = found || (choice.Action == "give_quest" && chain[choice.QuestID])
			return nil
		})
		return found
	}
	// Maps to check: every map a chain layout uses, plus the giver's map.
	mapSet := map[string]bool{}
	for id := range chain {
		for _, layout := range defs[id].PropLayouts() {
			for _, p := range layout.Props {
				mapSet[p.Map] = true
			}
		}
	}
	giverFound := false
	for mapKey, w := range wm.LoadedMaps {
		for _, npc := range w.NPCs {
			if offersChain(npc) {
				mapSet[mapKey], giverFound = true, true
			}
		}
	}
	if !giverFound || len(mapSet) < 2 {
		t.Fatalf("pilgrimage giver present=%v, maps %v", giverFound, mapSet)
	}
	tile := g.config.GetTileSize()
	for _, mapKey := range slices.Sorted(maps.Keys(mapSet)) {
		w := wm.WorldByKey(mapKey)
		var arrivals [][2]int
		if w.StartX >= 0 && w.StartY >= 0 {
			arrivals = append(arrivals, [2]int{w.StartX, w.StartY})
		}
		for _, tp := range wm.GlobalTeleporterRegistry.Teleporters {
			if tp.MapKey == mapKey {
				arrivals = append(arrivals, [2]int{tp.X, tp.Y})
			}
		}
		if len(arrivals) == 0 {
			t.Fatalf("%s has no authored arrival point", mapKey)
		}
		seen := map[[2]int]bool{}
		queue := append([][2]int(nil), arrivals...)
		for _, a := range arrivals {
			seen[a] = true
		}
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
		// The giver and every activity prop already standing on the map ...
		var candidates []*character.NPC
		for _, npc := range w.NPCs {
			if _, prop := activityProp(npc); prop != nil || offersChain(npc) {
				candidates = append(candidates, npc)
			}
		}
		// ... plus every authored layout prop, each of which must be an activity prop.
		for _, def := range defs {
			for _, layout := range def.PropLayouts() {
				for _, p := range layout.Props {
					if p.Map != mapKey {
						continue
					}
					x, y := TileCenterFromTile(p.X, p.Y, tile)
					npc, err := character.CreateNPCFromConfig(p.NPC, x, y)
					if err != nil {
						t.Fatal(err)
					}
					if _, prop := activityProp(npc); prop == nil {
						t.Errorf("layout %s prop %s is not an activity prop", layout.ID, p.NPC)
					}
					candidates = append(candidates, npc)
				}
			}
		}
		tokens := map[string]bool{}
		for _, npc := range candidates {
			pos := [2]int{TileIndex(npc.X, tile), TileIndex(npc.Y, tile)}
			if !seen[pos] {
				t.Errorf("%s at %v unreachable from %s arrivals %v", npc.Key, pos, mapKey, arrivals)
			}
			if _, prop := activityProp(npc); prop != nil {
				identity := fmt.Sprintf("%s:%v", prop.Token, pos)
				if tokens[identity] {
					t.Error("duplicate placed token", prop.Token)
				}
				tokens[identity] = true
			}
		}
	}
}

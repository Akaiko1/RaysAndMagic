package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func rareClassGame(t *testing.T, class character.CharacterClass, tb bool) (*MMGame, *character.MMCharacter) {
	t.Helper()
	g, _ := summonTileWorld(t)
	setTestWorldManager(t, nil)
	g.turnBasedMode = tb
	g.currentTurn = 0
	g.camera.Angle = 0
	c := character.CreateCharacter("Rare", class, g.config)
	c.Level = 10
	c.SpellPoints = 100
	c.MaxSpellPoints = 100
	c.ActionsRemaining = 2
	c.RTCooldown = 0
	g.party.Members[0] = c
	for _, m := range g.party.Members[1:] {
		m.ActionsRemaining = 2
		m.RTCooldown = 0
	}
	g.selectedChar = 0
	g.combat = NewCombatSystem(g)
	return g, c
}

func TestRarePurifyLifeStates(t *testing.T) {
	for _, life := range []character.Condition{character.ConditionNormal, character.ConditionUnconscious, character.ConditionDead, character.ConditionEradicated} {
		t.Run(life.String(), func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, true)
			victim := g.party.Members[1]
			conditions := []character.Condition{life, character.ConditionPoisoned, character.ConditionDiseased, character.ConditionCursed, character.ConditionAsleep, character.ConditionFear, character.ConditionParalyzed, character.ConditionStone, character.ConditionBurning, character.ConditionStunned}
			victim.Conditions = append([]character.Condition(nil), conditions...)
			victim.HitPoints = 0
			victim.SpellPoints = 3
			victim.PoisonFramesRemaining = 100
			victim.BurnFramesRemaining = 100
			victim.StunFramesRemaining = 100
			victim.StunTurnsRemaining = 2
			valid := life != character.ConditionDead && life != character.ConditionEradicated
			if got := g.useTechnique(0, "purify", false, false); got != valid {
				t.Fatalf("cast=%v want %v", got, valid)
			}
			if victim.HitPoints != 0 || victim.SpellPoints != 3 {
				t.Fatal("cleanse healed or revived")
			}
			if valid {
				if victim.Purifiable() || victim.PoisonFramesRemaining != 0 || victim.BurnFramesRemaining != 0 || victim.StunTurnsRemaining != 0 {
					t.Fatal("affliction timer survived")
				}
				if life == character.ConditionUnconscious && !victim.HasCondition(life) {
					t.Fatal("unconscious ally was awakened")
				}
				if c.SpellPoints != 88 || c.ActionsRemaining != 1 {
					t.Fatalf("wrong cost: SP=%d AP=%d", c.SpellPoints, c.ActionsRemaining)
				}
				if g.useTechnique(0, "purify", false, false) {
					t.Fatal("empty cleanse committed")
				}
			} else if !reflect.DeepEqual(victim.Conditions, conditions) || c.SpellPoints != 100 || c.ActionsRemaining != 2 {
				t.Fatal("invalid life state changed")
			}
		})
	}
}

func TestRareCraftAllRecipesAndAtomicFailure(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	for _, recipe := range config.GlobalAlchemy.Recipes {
		for _, tier := range []character.SkillMastery{character.MasteryNovice, character.MasteryGrandMaster} {
			t.Run(fmt.Sprintf("%s/%d", recipe.Key, tier), func(t *testing.T) {
				c.Skills[character.SkillAlchemy] = &character.Skill{Mastery: tier}
				g.party.Inventory = nil
				choices := make([]int, len(recipe.Ingredients))
				for _, group := range recipe.Ingredients {
					a := group.Alternatives[0]
					it, err := items.TryCreateItemFromYAML(a.Items[0])
					if err != nil {
						t.Fatal(err)
					}
					it.Quantity = a.Count * 2
					g.party.AddItem(it)
				}
				before := append([]items.Item(nil), g.party.Inventory...)
				if _, _, err := g.party.Brew(c, &recipe, choices, 3); err == nil {
					t.Fatal("short batch succeeded")
				}
				if !reflect.DeepEqual(before, g.party.Inventory) {
					t.Fatal("failed batch consumed ingredients")
				}
				count, _, err := g.party.Brew(c, &recipe, choices, 2)
				if err != nil {
					t.Fatal(err)
				}
				want := 2 * character.AlchemyYield(int(tier), recipe.Family)
				if count != want || len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != want {
					t.Fatalf("bad yield/inventory: %d %+v", count, g.party.Inventory)
				}
				output, _ := config.GetItemDefinition(recipe.Output)
				if g.party.Inventory[0].Name != output.Name || output.Value != 1 || !output.CraftedOnly {
					t.Fatal("crafted identity lost")
				}
				if err := config.ValidateOrdinaryItemGrant(recipe.Output); err == nil {
					t.Fatal("crafted stock allowed into ordinary source")
				}
			})
		}
	}
}

func TestRareCraftMixedFishAndChosenAlternative(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	g.party.Inventory = nil
	for key, count := range map[string]int{"dawnleaf": 1, "carp_scale": 2, "koi_scale": 1, "rainbow_salmon_scale": 1, "rabbit_pelt": 1} {
		it, _ := items.TryCreateItemFromYAML(key)
		it.Quantity = count
		g.party.AddItem(it)
	}
	r := config.AlchemyRecipeByKey("health_potion")
	if _, _, err := g.party.Brew(c, r, []int{0, 0}, 1); err != nil {
		t.Fatal(err)
	}
	if g.party.CountItemsByName("Rabbit Pelt") != 1 {
		t.Fatal("unselected alternative consumed")
	}
	for _, key := range []string{"carp_scale", "koi_scale", "rainbow_salmon_scale"} {
		d, _ := config.GetItemDefinition(key)
		if g.party.CountItemsByName(d.Name) != 0 {
			t.Fatal("mixed fish not consumed")
		}
	}
}

func TestRarePharmacologyCoherentSourceAndTooltip(t *testing.T) {
	g, a := rareClassGame(t, character.ClassAlchemist, false)
	b := character.CreateCharacter("Master", character.ClassAlchemist, g.config)
	g.party.Members[1] = b
	a.Name = "Scholar"
	a.Intellect = 120
	a.Skills[character.SkillPharmacology] = &character.Skill{Mastery: character.MasteryNovice}
	b.Intellect = 3
	b.Skills[character.SkillPharmacology] = &character.Skill{Mastery: character.MasteryGrandMaster}
	recipient := g.party.Members[2]
	recipient.Skills[character.SkillFieldMedicine] = &character.Skill{Mastery: character.MasteryNovice}
	for _, base := range []int{10, 200} {
		hp := g.party.PotionSupport(recipient, base, 0, false)
		sp := g.party.PotionSupport(recipient, base, 0, true)
		expected := a.Name
		if base == 200 {
			expected = b.Name
		}
		if hp.Source != expected || sp.Source != b.Name {
			t.Fatalf("wrong coherent sources: %+v %+v", hp, sp)
		}
		actual := character.ConsumableRestore(recipient, base, 0, false, hp)
		want := base*(100+character.FieldMedicineRestorePct(0)+hp.HPPct)/100 + hp.HPFlat
		if actual != want {
			t.Fatalf("recovery=%d want=%d", actual, want)
		}
	}
	a.Conditions = []character.Condition{character.ConditionStunned}
	a.StunFramesRemaining = 100
	if got := g.party.PotionSupport(recipient, 10, 0, false); got.Source != b.Name {
		t.Fatal("incapacitated source contributed")
	}
	it, _ := items.TryCreateItemFromYAML("brewed_health_potion")
	d, _ := config.GetItemDefinition("brewed_health_potion")
	bonus := g.party.PotionSupport(recipient, d.HealBase, d.HealEnduranceDivisor, false)
	want := character.ConsumableRestore(recipient, d.HealBase, d.HealEnduranceDivisor, false, bonus)
	tooltip := buildSimpleItemTooltipWithParty(it, true, recipient, g.party)
	if !strings.Contains(tooltip, fmt.Sprint(want)) || !strings.Contains(tooltip, "Master") {
		t.Fatalf("tooltip diverged: %s", tooltip)
	}
}

func TestRareTechniqueAutocastTransaction(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, blocked := range []string{"none", "checkbox", "SP", "AP", "overlay", "guard"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, blocked), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				g.selectedChar = 1
				c.RareClass.Automatic = map[string]bool{"purify": true, "phase_veil": true, "quickening": true}
				g.party.Members[1].Conditions = []character.Condition{character.ConditionPoisoned}
				switch blocked {
				case "checkbox":
					c.RareClass.Automatic = nil
				case "SP":
					c.SpellPoints = 0
				case "AP":
					c.ActionsRemaining = 0
					c.RTCooldown = 100
				case "overlay":
					g.menuOpen = true
				case "guard":
					c.RareClass.AutoFrames = 100
				}
				ap, sp := c.ActionsRemaining, c.SpellPoints
				otherAP := g.party.Members[1].ActionsRemaining
				(&GameLoop{game: g}).updateAutomaticTechniques()
				if blocked == "none" {
					if g.party.Members[1].Purifiable() || c.SpellPoints != sp-12 || c.RareClass.AutoFrames != 3*g.config.GetTPS() {
						t.Fatal("automatic cleanse not committed")
					}
					wantAP := ap
					if tb {
						wantAP--
					}
					if c.ActionsRemaining != wantAP || c.RTCooldown <= 0 {
						t.Fatal("incorrect actor action cost")
					}
					if g.selectedChar != 1 || g.party.Members[1].ActionsRemaining != otherAP {
						t.Fatal("autocast stole selection or another hero's action")
					}
					if len(g.combatBuffs) != 0 {
						t.Fatal("more than one autocast fired")
					}
				} else if c.SpellPoints != sp || c.ActionsRemaining != ap || !g.party.Members[1].Purifiable() {
					t.Fatal("blocked autocast changed state")
				}
			})
		}
	}
}

func TestRareSpatialStepsFreeButTransactional(t *testing.T) {
	for _, blocked := range []string{"none", "wall", "endpoint", "SP", "reuse", "enemyphase"} {
		t.Run(blocked, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, true)
			c.ActionsRemaining = 0
			ts := float64(g.config.GetTileSize())
			x, y := g.camera.X, g.camera.Y
			switch blocked {
			case "wall":
				g.world.Tiles[10][9] = world.TileWall
			case "endpoint":
				m := zoneVictim(t, g)
				m.X = x + 3*ts
				m.Y = y
				g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
				g.refreshMonsterCollisionState(m)
			case "SP":
				c.SpellPoints = 0
			case "reuse":
				g.spatialReuseFrames = 1
			case "enemyphase":
				g.currentTurn = 1
			}
			sp := c.SpellPoints
			got := g.useTechnique(0, "fold_step", false, false)
			if got != (blocked == "none") {
				t.Fatalf("cast=%v reason=%s", got, g.techniqueRefusal(0, "fold_step"))
			}
			if !got {
				if c.SpellPoints != sp || g.camera.X != x || g.camera.Y != y || c.RareClass.Anchor.Frames != 0 {
					t.Fatal("rejected step changed state")
				}
				return
			}
			distance := 3 * ts
			if c.ActionsRemaining != 0 || c.RTCooldown != 0 || c.SpellPoints != sp-8 || math.Abs(g.camera.X-x-distance) > .01 {
				t.Fatal("spatial step paid AP or moved incorrectly")
			}
			if c.FlowingStaffCharges() == 0 || c.RareClass.Anchor.X != x || g.spatialReuseFrames != 3*g.config.GetTPS() {
				t.Fatal("step state missing")
			}
			if g.useTechnique(0, "return_step", false, false) {
				t.Fatal("shared lock bypassed")
			}
			g.startPartyTurn()
			if !g.useTechnique(0, "return_step", false, false) || math.Abs(g.camera.X-x) > .01 || c.RareClass.Anchor.Frames != 0 {
				t.Fatal("return failed")
			}
		})
	}
}

func TestRareCombatClocksAndPersistence(t *testing.T) {
	g, c := rareClassGame(t, character.ClassWayfarer, true)
	if !g.useTechnique(0, "quickening", false, false) {
		t.Fatal("cast failed")
	}
	b, _ := g.combatBuffByID("quickening")
	frames := b.Frames
	for i := 0; i < 120; i++ {
		g.tickCombatBuffs()
	}
	b, _ = g.combatBuffByID("quickening")
	if b.Frames != frames {
		t.Fatal("thinking time consumed new buff")
	}
	saved := buildCombatBuffSaves(g.combatBuffs)
	g.combatBuffs = restoreCombatBuffs(saved)
	b, _ = g.combatBuffByID("quickening")
	if b.RecoveryPct != 20 || b.ExtraActions != 1 || !b.DeferFirstTurnTick {
		t.Fatalf("buff restore lost values: %+v", b)
	}
	g.tickCombatBuffsTurn(3 * g.config.GetTPS())
	b, _ = g.combatBuffByID("quickening")
	if b.Frames != frames {
		t.Fatal("lost duration before first bonus turn")
	}
	g.tickCombatBuffsTurn(3 * g.config.GetTPS())
	b, _ = g.combatBuffByID("quickening")
	if b.Frames != frames-3*g.config.GetTPS() {
		t.Fatal("TB clock did not advance")
	}
	g.turnBasedMode = false
	g.tickCombatBuffs()
	b, _ = g.combatBuffByID("quickening")
	if b.DeferFirstTurnTick {
		t.Fatal("RT kept deferred tick")
	}
	c.RareClass.Automatic = map[string]bool{"purify": true}
	c.RareClass.AutoFrames = 10
	c.RareClass.Anchor = character.SpatialAnchor{MapKey: "forest", X: 12, Y: 34, Frames: 80}
	state := c.RareClass.Clone()
	state.Automatic["purify"] = false
	if !c.RareClass.Automatic["purify"] {
		t.Fatal("save clone aliases switches")
	}
	encoded, err := json.Marshal(c.RareClass)
	if err != nil {
		t.Fatal(err)
	}
	var restored character.RareClassState
	if err = json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(restored, c.RareClass) {
		t.Fatal("rare state persistence failed")
	}
}

func TestRareFlaskStockShortcutAndAreaPacket(t *testing.T) {
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		t.Run(key, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassAlchemist, true)
			g.party.Inventory = nil
			stock, _ := items.TryCreateItemFromYAML(key)
			stock.Quantity = 2
			g.party.AddItem(stock)
			if !g.equipFlask(0, key) || g.flaskStock(key) != 2 {
				t.Fatal("equipping took stock")
			}
			if !g.throwFlask(0, key, false) || g.flaskStock(key) != 1 || c.ActionsRemaining != 1 || len(g.magicProjectiles) != 1 {
				t.Fatal("throw did not commit exactly one stock/action")
			}
			p := &g.magicProjectiles[0]
			victims := []int{}
			for i := 0; i < 3; i++ {
				m := zoneVictim(t, g)
				m.ArmorClass = 0
				m.X = g.camera.X + float64(i)*30
				m.Y = g.camera.Y
				victims = append(victims, len(g.world.Monsters)-1)
			}
			g.combat.detonateFlask(p, g.camera.X, g.camera.Y)
			for _, i := range victims {
				m := g.world.Monsters[i]
				if dealt := m.MaxHitPoints - m.HitPoints; dealt != p.Damage {
					t.Fatalf("AoE packet=%d want full %d", dealt, p.Damage)
				}
				if key == "venom_flask" && m.PoisonedFramesRemaining == 0 {
					t.Fatal("poison missing")
				}
				if key == "fire_flask" && m.BurnFramesRemaining == 0 {
					t.Fatal("burn missing")
				}
			}
			g.returnQuickItemToInventory(c.Equipment[items.SlotSpell])
			if g.flaskStock(key) != 1 {
				t.Fatal("virtual shortcut duplicated stock")
			}
		})
	}
}

func TestRareFirewallBandsAndSharedCadence(t *testing.T) {
	for _, offset := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}, {2, 0}} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			g, ts := summonTileWorld(t)
			setTestWorldManager(t, nil)
			x, y := TileCenterFromTile(10, 10, ts)
			g.persistentDamageZones = []PersistentDamageZone{{SpellID: "firewall", FieldID: 1, X: x, Y: y, AxisY: 1, Radius: .55 * ts, TickDamage: 15, FramesLeft: 600, IntervalFrames: 60}}
			m := zoneVictim(t, g)
			m.X = x + float64(offset[0])*ts
			m.Y = y + float64(offset[1])*ts
			g.refreshMonsterCollisionState(m)
			tickZoneSpellOnce(g.combat, "firewall")
			want := 7
			if offset == [2]int{0, 0} {
				want = 15
			}
			if offset == [2]int{2, 0} {
				want = 0
			}
			if got := m.MaxHitPoints - m.HitPoints; got != want {
				t.Fatalf("band hit=%d want=%d", got, want)
			}
			if (m.BurnFramesRemaining > 0) != (want > 0) {
				t.Fatal("band burn mismatch")
			}
			hp := m.HitPoints
			g.combat.applyZoneEntrySpell("firewall")
			if m.HitPoints != hp {
				t.Fatal("entry double billed tick")
			}
		})
	}
}

func TestRareHarvestDawnSaveAndRoster(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	prev := config.GlobalAlchemySpawns
	t.Cleanup(func() { config.GlobalAlchemySpawns = prev })
	if _, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"forest", "highlands", "dragon_cliffs", "desert", "sakura_garden", "deep_jungle", "japanese_castle", "water"} {
		t.Run(region, func(t *testing.T) {
			wm := world.NewWorldManager(g.config)
			wm.CurrentMapKey = region
			wm.LoadedMaps = map[string]*world.World3D{region: g.world}
			world.GlobalWorldManager = wm
			g.alchemy = AlchemyState{}
			g.harvestRuntime = harvestRuntime{}
			g.world.NPCs = nil
			g.calendarDay = 1
			g.party.Members[0] = c
			finishRareHarvest(t, g)
			expected := 9
			if region == "japanese_castle" || region == "water" {
				expected = 0
			}
			if len(g.world.NPCs) != expected {
				t.Fatalf("nodes=%d want=%d", len(g.world.NPCs), expected)
			}
			if expected == 0 {
				return
			}
			seen := map[[2]int]bool{}
			for _, p := range g.alchemy.Populations {
				for _, n := range p.Nodes {
					if seen[[2]int{n.X, n.Y}] || !g.harvestGround(n.X, n.Y) {
						t.Fatal("duplicate or blocked harvest node")
					}
					seen[[2]int{n.X, n.Y}] = true
				}
			}
			target := g.world.NPCs[0]
			ts := float64(g.config.GetTileSize())
			placePlayerAtTile(g, TileIndex(target.X, ts)-1, TileIndex(target.Y, ts), ts)
			if !g.gatherAlchemyReagent(target) {
				t.Fatal("reachable node could not be gathered")
			}
			finishRareHarvest(t, g)
			if len(g.world.NPCs) != 8 {
				t.Fatal("same-day respawn")
			}
			data, err := json.Marshal(g.alchemy)
			if err != nil {
				t.Fatal(err)
			}
			var saved AlchemyState
			if err = json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			g.alchemy = saved
			g.harvestRuntime = harvestRuntime{}
			g.world.NPCs = nil
			finishRareHarvest(t, g)
			if len(g.world.NPCs) != 8 {
				t.Fatal("reload rerolled depleted population")
			}
			g.party.Members[0] = character.CreateCharacter("Other", character.ClassKnight, g.config)
			if !g.npcAbsent(g.world.NPCs[0]) {
				t.Fatal("reagent visible without alchemist")
			}
			g.party.Members[0] = c
			finishRareHarvest(t, g)
			if len(g.world.NPCs) != 8 {
				t.Fatal("class swap replenished stock")
			}
			survivor := g.world.NPCs[0].Key
			g.advanceCalendarAtDawn()
			finishRareHarvest(t, g)
			if len(g.world.NPCs) != 9 {
				t.Fatal("dawn did not replenish deficit")
			}
			found := false
			for _, n := range g.world.NPCs {
				if n.Key == survivor {
					found = true
				}
			}
			if !found {
				t.Fatal("dawn rerolled survivor")
			}
		})
	}
}

func TestRareSaveRealCharacterAndTurnState(t *testing.T) {
	for _, class := range []character.CharacterClass{character.ClassAlchemist, character.ClassWayfarer} {
		g, c := rareClassGame(t, class, true)
		c.RareClass = character.RareClassState{KitGranted: true, Automatic: map[string]bool{"purify": true}, AutoFrames: 88, PurifyFrames: 123, FlowCharges: 3, FlowDistance: 13, Anchor: character.SpatialAnchor{MapKey: "forest", X: 50, Y: 90, Frames: 260}}
		key := "purify"
		it, _ := config.TechniqueItem(key)
		if class == character.ClassAlchemist {
			key = "harm_flask"
			it, _ = config.FlaskItem(key)
		}
		c.Equipment[items.SlotSpell] = it
		c.QuickSlots[0] = &it
		raw, err := json.Marshal(buildCharacterSave(c))
		if err != nil {
			t.Fatal(err)
		}
		var saved CharacterSave
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		restored := restoreCharacterSave(saved)
		if restored.Class != class || !reflect.DeepEqual(restored.RareClass, c.RareClass) || restored.QuickSlots[0] == nil || restored.QuickSlots[0].Type != it.Type || restored.Equipment[items.SlotSpell].Type != it.Type {
			t.Fatal("real save/restore lost rare class state or shortcut type")
		}
		g.restoreSavedTurnState(&GameSave{TurnBased: true, SpatialReuseFrames: 150, SpatialStepThisTurn: true, PartyActionsUsed: 1})
		if g.spatialReuseFrames != 150 || !g.spatialStepThisTurn || g.partyActionsUsed != 1 {
			t.Fatal("turn restore lost spatial guard")
		}
	}
}

func TestRareFlaskRangeAndWallImpactOnce(t *testing.T) {
	for _, impact := range []string{"range", "wall"} {
		t.Run(impact, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			g.party.Inventory = nil
			it, _ := items.TryCreateItemFromYAML("harm_flask")
			g.party.AddItem(it)
			if !g.throwFlask(0, "harm_flask", false) {
				t.Fatal("launch failed")
			}
			p := &g.magicProjectiles[0]
			x, y := p.X, p.Y
			rangePx := p.FlaskRemaining
			m := zoneVictim(t, g)
			m.ArmorClass = 0
			m.X = x + rangePx
			m.Y = y
			wrapper := &MagicProjectileWrapper{MagicProjectile: p, collisionSystem: g.collisionSystem, projectileID: p.ID, game: g}
			if impact == "wall" {
				wrapper.OnCollision(x+rangePx, y)
			} else {
				wrapper.SetPosition(x+rangePx+100, y)
				if math.Abs(p.X-x-rangePx) > .01 {
					t.Fatal("range overshoot")
				}
			}
			hp := m.HitPoints
			wrapper.ApplyCollisionEffects()
			if hp-m.HitPoints != p.Damage {
				t.Fatal("impact missed or diluted full packet")
			}
			hp = m.HitPoints
			wrapper.ApplyCollisionEffects()
			if hp != m.HitPoints {
				t.Fatal("impact applied twice")
			}
			if p.Active || g.flaskStock("harm_flask") != 0 {
				t.Fatal("finished flask survived")
			}
		})
	}
}

func TestRareMonsterBurnClockAndBossRule(t *testing.T) {
	for _, boss := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("boss=%v/tb=%v", boss, tb), func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassAlchemist, tb)
				m := zoneVictim(t, g)
				m.Boss = boss
				m.MaxHitPoints = 1000
				m.HitPoints = 1000
				m.ApplyBurn(3 * g.config.GetTPS())
				if tb {
					m.TickBurnTurn(3 * g.config.GetTPS())
				} else {
					for i := 0; i < 3*g.config.GetTPS(); i++ {
						m.TickBurn()
					}
				}
				want := 90
				if boss {
					want = 30
				}
				if m.MaxHitPoints-m.HitPoints != want {
					t.Fatalf("burn=%d want=%d", m.MaxHitPoints-m.HitPoints, want)
				}
			})
		}
	}
}

func TestRareSpatialNoExtraTurnBothOrdersAndMeditation(t *testing.T) {
	for _, stepFirst := range []bool{false, true} {
		g, c := rareClassGame(t, character.ClassWayfarer, true)
		c.Skills[character.SkillMeditation] = &character.Skill{Mastery: character.MasteryGrandMaster}
		step := func() {
			if !g.useTechnique(0, "fold_step", false, false) {
				t.Fatal("step failed")
			}
		}
		paid := func() {
			if !g.useTechnique(0, "phase_veil", false, false) {
				t.Fatal("paid technique failed")
			}
		}
		if stepFirst {
			step()
			paid()
		} else {
			paid()
			step()
		}
		if g.turnBasedExtraMonsterAction || c.SpellPoints != 88 {
			t.Fatalf("unexpected monster turn or wrong Meditation discount: extra=%v SP=%d", g.turnBasedExtraMonsterAction, c.SpellPoints)
		}
	}
}

func TestRareBrewedBuffRefreshesOrdinaryEffect(t *testing.T) {
	for _, key := range []string{"flame_ward_draught", "storm_ward_draught", "gloom_ward_draught", "stoneskin_draught", "mirage_salt"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", key, reverse), func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassAlchemist, false)
				keys := []string{key, "brewed_" + key}
				if reverse {
					keys[0], keys[1] = keys[1], keys[0]
				}
				g.party.Inventory = nil
				for _, k := range keys {
					it, err := items.TryCreateItemFromYAML(k)
					if err != nil {
						t.Fatal(err)
					}
					g.party.AddItem(it)
					if !g.UseConsumableFromInventory(0, 0) {
						t.Fatalf("cannot drink %s", k)
					}
				}
				if len(g.combatBuffs) != 1 || g.combatBuffs[0].SpellID != key || len(g.party.Inventory) != 0 {
					t.Fatalf("duplicate effect or unspent potion: %+v", g.combatBuffs)
				}
				restored := restoreCombatBuffs(buildCombatBuffSaves(g.combatBuffs))
				if !reflect.DeepEqual(restored, g.combatBuffs) {
					t.Fatal("canonical effect changed on save/load")
				}
			})
		}
	}
}

func TestRareCraftedSourceProductionGuards(t *testing.T) {
	g, _ := rareClassGame(t, character.ClassAlchemist, false)
	oldNPC, oldLoot := character.NPCConfigInstance, config.GlobalLoots
	oldMonsters := monster.MonsterConfig
	t.Cleanup(func() {
		character.NPCConfigInstance = oldNPC
		config.GlobalLoots = oldLoot
		monster.MonsterConfig = oldMonsters
	})
	for _, key := range []string{"health_potion", "brewed_health_potion", "harm_flask"} {
		d, _ := config.GetItemDefinition(key)
		for _, source := range []string{"merchant", "monster", "boss", "weighted", "quest", "quest_pool"} {
			t.Run(source+"/"+key, func(t *testing.T) {
				var err error
				switch source {
				case "merchant":
					p := filepath.Join(t.TempDir(), "npcs.yaml")
					body := fmt.Sprintf("npcs:\n  test:\n    name: Trader\n    type: merchant\n    render_category: standee\n    inventory:\n      - {name: %q, type: consumable, cost: 1, quantity: 1}\n", d.Name)
					if err = os.WriteFile(p, []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
					err = character.LoadNPCConfig(p)
				case "monster", "boss", "weighted":
					entry := fmt.Sprintf("{type: item, key: %s, chance: 1}", key)
					body := "loots:\n  test:\n    - " + entry + "\n"
					if source == "boss" {
						body = "boss_loot:\n  - " + entry + "\n"
					}
					if source == "weighted" {
						body = fmt.Sprintf("loot_tables:\n  test:\n    rolls: 1\n    entries:\n      - {type: item, key: %s, weight: 1}\n", key)
					}
					p := filepath.Join(t.TempDir(), "loots.yaml")
					if err = os.WriteFile(p, []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
					_, err = config.LoadLootTables(p)
				default:
					character.NPCConfigInstance = nil
					monster.MonsterConfig = nil
					reward := quests.QuestRewards{Items: []string{key}}
					if source == "quest_pool" {
						reward = quests.QuestRewards{ItemPool: []string{key}}
					}
					qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"test": {Rewards: reward}}})
					err = g.validateQuestWorldReferences(qm)
				}
				if d.CraftedOnly {
					if err == nil || !strings.Contains(err.Error(), "crafted-only") {
						t.Fatalf("crafted item guard: %v", err)
					}
				} else if err != nil {
					t.Fatalf("ordinary source refused: %v", err)
				}
			})
		}
	}
}

func TestRareDisplayedAutocastCheckboxes(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			c := character.CreateCharacter("Kael", character.ClassWayfarer, g.config)
			c.Level = 10
			g.party.Members[0] = c
			g.selectedChar = 0
			g.menuOpen = true
			g.currentTab = TabSpellbook
			content := computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content
			l := computeRareBookLayout(content, false)
			for i, d := range config.GlobalTechniques.Techniques {
				if d.Automatic {
					for _, want := range []bool{true, false, true} {
						r := l.auto(i)
						h.clicks(false, r.x+10, r.y+10, 1)
						if c.RareClass.Automatic[d.Key] != want {
							t.Fatalf("displayed %s checkbox=%v want %v", d.Key, c.RareClass.Automatic[d.Key], want)
						}
						if c.Equipment[items.SlotSpell].Type == items.ItemTechnique {
							t.Fatal("checkbox unexpectedly equipped a technique")
						}
					}
				}
			}
		})
	}
}

func TestRareTechniqueQuickSlotAndBookCosts(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, route := range []string{"book", "quickslot"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, route), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				it, _ := config.TechniqueItem("purify")
				g.party.Members[1].Conditions = []character.Condition{character.ConditionPoisoned}
				if route == "book" {
					g.menuOpen = true
					g.currentTab = TabSpellbook
					if !g.useTechniqueFromBook("purify") {
						t.Fatal("book refused")
					}
				} else {
					c.QuickSlots[0] = &it
					g.useQuickSlot(0, 0)
				}
				wantAP := 2
				if tb {
					wantAP = 1
				}
				if g.party.Members[1].Purifiable() || c.SpellPoints != 88 || c.ActionsRemaining != wantAP {
					t.Fatalf("route lost action: SP %d AP %d", c.SpellPoints, c.ActionsRemaining)
				}
				if route == "quickslot" && c.QuickSlots[0].Type != items.ItemTechnique {
					t.Fatal("virtual technique consumed")
				}
			})
		}
	}
}

func TestRareAutomaticSupportTriggers(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"peace", "distant", "surrounded", "low_hp", "already_active", "incapacitated"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, state), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.RareClass.Automatic = map[string]bool{"phase_veil": true, "quickening": true}
				if state != "peace" {
					m := zoneVictim(t, g)
					m.IsEngagingPlayer = true
					m.X += 10 * float64(g.config.GetTileSize())
				}
				if state == "surrounded" {
					for i := 0; i < 2; i++ {
						m := zoneVictim(t, g)
						m.IsEngagingPlayer = true
					}
				}
				if state == "low_hp" {
					g.party.Members[1].HitPoints = 1
				}
				if state == "already_active" {
					g.addCombatBuff(TimedCombatBuff{SpellID: "quickening", Frames: 100})
				}
				if state == "incapacitated" {
					c.ApplyCharStun(120, 2)
				}
				(&GameLoop{game: g}).updateAutomaticTechniques()
				want := ""
				if state == "distant" {
					want = "quickening"
				}
				if state == "surrounded" || state == "low_hp" {
					want = "phase_veil"
				}
				if want != "" {
					if b, ok := g.combatBuffByID(want); !ok || !b.CombatClock || c.SpellPoints >= 100 {
						t.Fatalf("missing automatic %s", want)
					}
				} else if c.SpellPoints != 100 {
					t.Fatal("inactive trigger spent SP")
				}
				if state != "already_active" && len(g.combatBuffs) > 1 {
					t.Fatal("two automatic techniques in one update")
				}
			})
		}
	}
}

func TestRareFlowChargeActualAttack(t *testing.T) {
	for _, action := range []string{"staff", "other_weapon", "pixie_replacement", "stunned", "projectile_cap"} {
		t.Run(action, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, false)
			g.notifyPilgrimDisplacement(g.camera.X, g.camera.Y)
			if c.FlowingStaffCharges() != 0 {
				t.Fatal("standing still charged staff")
			}
			g.notifyPilgrimDisplacement(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
			if c.FlowingStaffCharges() != 1 {
				t.Fatal("one tile did not charge staff")
			}
			c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("oak_staff")
			if action == "other_weapon" {
				c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
			}
			if action == "pixie_replacement" {
				wd := lookupWeaponConfigByName("Oak Staff")
				oldRange := wd.Range
				wd.Range = 3
				t.Cleanup(func() { wd.Range = oldRange })
				d, _ := config.GetItemDefinition("pixie_card")
				before := d.CardSpellProcPct
				d.CardSpellProcPct = 100
				t.Cleanup(func() { d.CardSpellProcPct = before })
				g.cardSlots[0].key = "pixie_card"
			}
			if action == "stunned" {
				c.ApplyCharStun(120, 2)
			}
			if action == "projectile_cap" {
				wd := lookupWeaponConfigByName("Oak Staff")
				oldCap := wd.MaxProjectiles
				wd.MaxProjectiles = 1
				t.Cleanup(func() { wd.MaxProjectiles = oldCap })
				if !g.combat.equipmentAttackAtAngle(g.camera.Angle, false) {
					t.Fatal("first staff bolt failed")
				}
				c.RareClass.FlowCharges = 1
			}
			acted := g.combat.equipmentAttackAtAngle(g.camera.Angle, false)
			if acted != (action != "stunned" && action != "projectile_cap") {
				t.Fatalf("acted=%v", acted)
			}
			if (c.FlowingStaffCharges() == 0) != (action == "staff") {
				t.Fatal("charge consumed outside a real staff attack")
			}
			if action == "pixie_replacement" && len(g.magicProjectiles) == 0 {
				t.Fatal("Pixie replacement was not exercised")
			}
		})
	}
}

func finishRareHarvest(t *testing.T, g *MMGame) {
	t.Helper()
	for i := 0; i < g.world.Width*g.world.Height+1; i++ {
		g.updateAlchemyHarvest()
		if g.harvestRuntime.search == nil {
			return
		}
	}
	t.Fatal("harvest search did not finish")
}

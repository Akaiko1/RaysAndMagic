package game

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestRareRulesCheapestMaterial(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	for ri, r := range config.GlobalAlchemy.Recipes {
		for choice, source := range r.Ingredients[1].Alternatives {
			for _, batches := range []int{1, 2} {
				t.Run(fmt.Sprintf("%s/%d/batches%d", r.Key, choice, batches), func(t *testing.T) {
					stock := make([]items.Item, 0, len(source.Items))
					for _, key := range source.Items {
						it, _ := items.TryCreateItemFromYAML(key)
						it.Quantity = source.Count * batches
						stock = append(stock, it)
					}
					sort.SliceStable(stock, func(i, j int) bool { return stock[i].Attributes["value"] > stock[j].Attributes["value"] })
					cheapest := stock[0]
					for _, it := range stock {
						if it.Attributes["value"] < cheapest.Attributes["value"] {
							cheapest = it
						}
					}
					g.party.Inventory = append([]items.Item(nil), stock...)
					base := r.Ingredients[0].Alternatives[0]
					herb, _ := items.TryCreateItemFromYAML(base.Items[0])
					herb.Quantity = base.Count * batches
					g.party.AddItem(herb)
					before := append([]items.Item(nil), g.party.Inventory...)
					g.selectedRare, g.alchemyBatches, g.brewAnimation = ri, batches, nil
					g.alchemy.Choices = map[string][]int{r.Key: {0, choice}}
					g.alchemy.Selections = nil // Each case loads a different legacy source.
					if g.party.MaxAlchemyBatches(&r, []int{0, choice}) != batches {
						t.Fatal("preview disagrees with stocked base")
					}
					if !g.brewSelectedRecipe() {
						t.Fatal(g.rareBookMessage)
					}
					assertAlchemyReceipt(t, before, g.party.Inventory, g.brewAnimation.Ingredients)
					for _, it := range stock {
						want := it.Count()
						if it.Name == cheapest.Name {
							want = 0
						}
						if got := g.party.CountItemsByName(it.Name); got != want {
							t.Fatalf("%s remaining=%d want=%d; cheapest=%s", it.Name, got, want, cheapest.Name)
						}
					}
					// Scarce-only stock remains a legitimate explicitly selected pool.
					g.party.Inventory = nil
					costly := stock[0]
					g.party.AddItem(costly)
					g.party.AddItem(herb)
					if _, _, err := g.party.Brew(c, &r, []int{0, choice}, batches); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestRareRulesExplicitStepOwnsRefusal(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"fold_step", "return_step"} {
			for _, reason := range []string{"reuse", "SP", "destination", "unconscious", "stagger"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, key, reason), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, tb)
					other := character.CreateCharacter("Caster", character.ClassSorcerer, g.config)
					other.SpellPoints = 100
					other.ActionsRemaining = 2
					g.party.Members = []*character.MMCharacter{c, other}
					it, _ := config.TechniqueItem(key)
					c.Equipment[items.SlotSpell] = it
					switch reason {
					case "reuse":
						g.spatialReuseFrames = 1
						g.spatialStepThisTurn = tb
					case "SP":
						c.SpellPoints = 0
					case "destination":
						g.world.Tiles[10][9] = world.TileWall
					case "unconscious":
						c.AddCondition(character.ConditionUnconscious)
						g.parkSelection = true
					case "stagger":
						g.spellInputCooldown = 1
					}
					x, y, sp, ap := g.camera.X, g.camera.Y, c.SpellPoints, c.ActionsRemaining
					rareInputCast(g, NewInputHandler(g), tb)
					if g.selectedChar != 0 || other.SpellPoints != 100 || other.ActionsRemaining != 2 || c.SpellPoints != sp || c.ActionsRemaining != ap || g.camera.X != x || g.camera.Y != y {
						t.Fatal("rejected explicit step fell through or spent resources")
					}
				})
			}
		}
	}
}

func TestRareRulesQuickeningOwnershipAndSharedRecovery(t *testing.T) {
	g, c := rareClassGame(t, character.ClassWayfarer, false)
	c.Speed = 16
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("archmage_staff")
	outside := character.CreateCharacter("Outside", character.ClassWayfarer, g.config)
	outside.Speed = c.Speed
	outside.Equipment[items.SlotMainHand] = c.Equipment[items.SlotMainHand]
	for _, actor := range []*character.MMCharacter{c, outside} {
		for _, action := range []string{"weapon", "offhand", "fireball", "bear_trap", "purify", "harm_flask", "phase_veil", "fold_step"} {
			t.Run(actor.Name+"/"+action, func(t *testing.T) {
				g.combatBuffs = nil
				frames := func() int {
					switch action {
					case "weapon":
						return g.combat.WeaponCooldownFrames(actor)
					case "offhand":
						return g.combat.OffHandWeaponCooldownFrames(actor)
					}
					return g.combat.SpellCooldownFrames(actor, spells.SpellID(action))
				}
				base := frames()
				g.addCombatBuff(TimedCombatBuff{SpellID: "quickening", Frames: 100, RecoveryPct: 35})
				want := base
				if actor == c && base > 0 {
					want = clampRTCooldown(base * 65 / 100)
				}
				if got := frames(); got != want {
					t.Fatalf("recovery=%d want=%d", got, want)
				}
			})
		}
	}
	g.combatBuffs = nil
	primeTestChampions(t, g)
	for _, key := range []string{"hobbit_archer", "weapon_master"} {
		g.combatBuffs = nil
		first := monsterPkg.NewMonster3DFromConfig(0, 0, key, g.config)
		g.mirrorChampionStats(first)
		g.addCombatBuff(TimedCombatBuff{SpellID: "quickening", Frames: 100, RecoveryPct: 35})
		second := monsterPkg.NewMonster3DFromConfig(0, 0, key, g.config)
		g.mirrorChampionStats(second)
		if second.AttackCooldownMultiplier != first.AttackCooldownMultiplier {
			t.Fatal("party Quickening changed enemy champion cadence")
		}
	}
	// Exercise the actual commits, including the staff modifier shared with spells.
	for _, action := range []string{"purify", "harm_flask"} {
		t.Run("commit/"+action, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassAlchemist, false)
			if action == "purify" {
				c.Class = character.ClassWayfarer
				c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.MasteryNovice}
				g.party.Members[1].AddCondition(character.ConditionPoisoned)
			}
			c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("archmage_staff")
			c.Speed = 16
			g.addCombatBuff(TimedCombatBuff{SpellID: "quickening", Frames: 100, RecoveryPct: 35})
			seconds, _ := baseCastCooldownSeconds(spells.SpellID(action))
			want := clampRTCooldown(clampRTCooldown(int(math.Round(seconds*float64(g.config.GetTPS())*spellCooldownSpeedFactor(c.GetEffectiveSpeed())*.8))) * 65 / 100)
			if action == "purify" {
				if !g.useTechnique(0, action, false, false) {
					t.Fatal("purify rejected")
				}
			} else {
				it, _ := items.TryCreateItemFromYAML(action)
				g.party.AddItem(it)
				if !g.throwFlask(0, action, false) {
					t.Fatal("flask rejected")
				}
			}
			if c.RTCooldown != want {
				t.Fatalf("commit recovery=%d want shared staff+Speed+Quickening=%d", c.RTCooldown, want)
			}
		})
	}
}

func TestRareRulesTechniqueMovementAndPersistence(t *testing.T) {
	for _, key := range []string{"purify", "phase_veil", "quickening"} {
		for _, automatic := range []bool{false, true} {
			for _, attack := range []string{"none", "before", "after"} {
				t.Run(fmt.Sprintf("%s/auto=%v/attack=%s", key, automatic, attack), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, true)
					g.selectedChar = 1
					c.ActionsRemaining = 2
					if key == "purify" {
						g.party.Members[1].AddCondition(character.ConditionPoisoned)
					}
					m := zoneVictim(t, g)
					m.X = g.camera.X + 32
					m.Y = g.camera.Y
					m.BeginPlayerEngagement()
					g.party.Members[1].HitPoints = 1
					if attack == "before" {
						g.consumeCharacterActionWithRTCooldown(1, 0)
					}
					if automatic {
						c.RareClass.Automatic = map[string]bool{key: true}
						(&GameLoop{game: g}).updateAutomaticTechniques()
					} else if !g.useTechnique(0, key, false, false) {
						t.Fatal("manual technique failed")
					}
					if c.ActionsRemaining != 1 || g.partyTechniqueActionsUsed != 1 {
						t.Fatal("technique action was not recorded independently")
					}
					if attack == "after" {
						g.consumeCharacterActionWithRTCooldown(1, 0)
					}
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = currentMapKey()
					wm.LoadedMaps = map[string]*world.World3D{wm.CurrentMapKey: g.world}
					raw, err := json.Marshal(g.buildSave(wm))
					if err != nil {
						t.Fatal(err)
					}
					var save GameSave
					if err = json.Unmarshal(raw, &save); err != nil {
						t.Fatal(err)
					}
					g.partyTechniqueActionsUsed = 0
					g.restoreSavedTurnState(&save)
					g.endPartyTurnAfterMovement()
					if g.turnBasedExtraMonsterAction != (attack != "none") {
						t.Fatal("technique created or erased an attack's movement penalty after save/load")
					}
					(&GameLoop{game: g}).endMonsterTurn()
					if g.partyTechniqueActionsUsed != 0 {
						t.Fatal("technique accounting leaked into next turn")
					}
				})
			}
		}
	}
}

func TestRareRulesQuietEntombedTechniques(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, tb)
			g.flyActive = true
			g.flyDuration = 999
			ts := float64(g.config.GetTileSize())
			g.world.Tiles[TileIndex(g.camera.Y, ts)][TileIndex(g.camera.X, ts)] = world.TileWall
			c.RareClass.Automatic = map[string]bool{"purify": true}
			g.party.Members[1].AddCondition(character.ConditionPoisoned)
			before := len(g.combatLogHistory)
			for i := 0; i < 4; i++ {
				g.frameCount += int64(2 * g.config.GetTPS())
				(&GameLoop{game: g}).updateAutomaticTechniques()
				_ = g.techniqueRefusal(0, "purify")
			}
			if len(g.combatLogHistory) != before || c.SpellPoints != 100 || c.ActionsRemaining != 2 {
				t.Fatal("passive refusal wrote chat or spent resources")
			}
			if g.useTechnique(0, "purify", false, true) || len(g.combatLogHistory) != before+1 {
				t.Fatal("explicit refusal did not report exactly once")
			}
		})
	}
}

func TestRareRulesBuildingFoldAndHarvest(t *testing.T) {
	for _, dir := range []string{"e", "w", "s", "n", "landmark"} {
		for _, tb := range []bool{false, true} {
			for _, entry := range []string{"fold", "harvest"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", dir, tb, entry), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, tb)
					ts := float64(g.config.GetTileSize())
					n := &character.NPC{Key: "building", GridSpanTiles: 4, GridSpanDir: dir}
					tx, ty := 13, 10
					switch dir {
					case "w":
						tx = 16
					case "s":
						ty = 9
					case "n":
						ty = 12
					case "landmark":
						n.GridSpanTiles = 0
						n.RenderCategory = "landmark"
					}
					if entry == "fold" && dir != "landmark" {
						// Leave a legal endpoint beyond every facade: destination
						// collision alone must not make the path check pass this test.
						n.GridSpanTiles = 2
						if dir == "w" {
							tx = 14
						}
						if dir == "n" {
							ty = 11
						}
					}
					n.X, n.Y = TileCenterFromTile(tx, ty, ts)
					g.world.NPCs = append(g.world.NPCs, n)
					g.registerBuildingFootprints()
					if entry == "fold" {
						c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.MasteryGrandMaster}
						x, y := g.camera.X, g.camera.Y
						if !g.useTechnique(0, "fold_step", false, false) || math.Abs(g.camera.X-x-4*ts) > .01 {
							t.Fatal("Fold crossed a solid building instead of stopping before it")
						}
						g.setPartyPosition(x+9*ts, y)
						g.spatialReuseFrames = 0
						g.spatialStepThisTurn = false
						if !g.useTechnique(0, "return_step", false, false) || g.camera.X != x {
							t.Fatal("Return incorrectly inherited Fold's path restriction")
						}
						return
					}
					g.party.Members[0] = character.CreateCharacter("Alchemist", character.ClassAlchemist, g.config)
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
					setTestWorldManager(t, wm)
					previous := config.GlobalAlchemySpawns
					t.Cleanup(func() { config.GlobalAlchemySpawns = previous })
					cfg, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
					if err != nil {
						t.Fatal(err)
					}
					cfg.Populations = []config.HarvestPopulation{{Map: "forest", Key: "ordinary", Count: g.world.Width * g.world.Height, Yield: 1, Weights: map[string]int{"dawnleaf": 1}}}
					// A persisted hidden node must be relocated, not preserved forever.
					g.alchemy.Populations = map[string]HarvestPopulationState{"forest:ordinary": {Day: g.currentCalendarDay(), Nodes: []HarvestNode{{Region: "forest", Key: "dawnleaf", X: 13, Y: 10, ID: "legacy_hidden", Quantity: 1}}}}
					for _, day := range []int{1, 2} {
						if day == 2 {
							p := g.alchemy.Populations["forest:ordinary"]
							p.Day--
							p.Nodes = nil
							g.alchemy.Populations["forest:ordinary"] = p
							g.harvestRuntime.dirty = true
						}
						finishRareHarvest(t, g)
						pop := g.alchemy.Populations["forest:ordinary"]
						if len(pop.Nodes) == 0 {
							t.Fatal("harvest fixture produced no nodes")
						}
						occupied := map[[2]int]bool{{13, 10}: true}
						switch dir {
						case "e", "w":
							for xx := 13; xx <= 16; xx++ {
								occupied[[2]int{xx, 10}] = true
							}
						case "s", "n":
							for yy := 9; yy <= 12; yy++ {
								occupied[[2]int{13, yy}] = true
							}
						}
						for _, node := range pop.Nodes {
							if occupied[[2]int{node.X, node.Y}] {
								t.Fatal("harvest node inside building footprint")
							}
						}
					}
				})
			}
		}
	}
}

func TestRareRulesZoneAuthoringAndExterior(t *testing.T) {
	g, ts := summonTileWorld(t)
	setTestWorldManager(t, nil)
	g.camera.Angle, g.camera.FOV, g.camera.ViewDist = 0, math.Pi/2, 20*ts
	original, _ := config.GetSpellDefinition("firewall")
	custom := *original
	custom.ZoneEdgeDamagePercent = 25
	custom.ZoneBurnSeconds = 7
	config.GlobalSpells.Spells["test_heat"] = &custom
	t.Cleanup(func() { delete(config.GlobalSpells.Spells, "test_heat") })
	x, y := TileCenterFromTile(12, 10, ts)
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g.turnBasedMode = tb
			g.world.Monsters = nil
			g.persistentDamageZones = []PersistentDamageZone{{SpellID: "test_heat", FieldID: 1, X: x, Y: y, AxisY: 1, Radius: .55 * ts, TickDamage: 40, FramesLeft: 600, IntervalFrames: 60}, {SpellID: "test_heat", FieldID: 1, X: x, Y: y + ts, AxisY: 1, Radius: .55 * ts, TickDamage: 40, FramesLeft: 600, IntervalFrames: 60}}
			direct := zoneVictim(t, g)
			direct.X, direct.Y = x, y
			edge := zoneVictim(t, g)
			edge.X, edge.Y = x+ts, y
			tickZoneSpellOnce(g.combat, "test_heat")
			if direct.MaxHitPoints-direct.HitPoints != 40 || edge.MaxHitPoints-edge.HitPoints != 10 || edge.BurnFramesRemaining != 7*g.config.GetTPS() {
				t.Fatal("zone damage/burn ignored authored fields or direct-over-edge priority")
			}
			saved := buildPersistentDamageZoneSaves(g.persistentDamageZones)
			g.persistentDamageZones = restorePersistentDamageZones(saved, currentMapKey())
			r := &Renderer{game: g}
			tiles := r.visibleZoneHeatTiles()
			if len(tiles) == 0 {
				t.Fatal("visible wall has no exterior")
			}
			seen := map[[2]int]bool{}
			for _, tile := range tiles {
				if tile == [2]int{12, 10} || tile == [2]int{12, 11} || seen[tile] {
					t.Fatal("heat overlays a flame cell or duplicates another edge")
				}
				seen[tile] = true
			}
			g.world.Tiles[10][13] = world.TileWall
			for _, tile := range r.visibleZoneHeatTiles() {
				if tile == [2]int{13, 10} {
					t.Fatal("heat appears inside a blocking wall")
				}
			}
			g.world.Tiles[10][13] = world.TileEmpty
			for _, view := range []string{"behind", "distant"} {
				angle, dist := g.camera.Angle, g.camera.ViewDist
				if view == "behind" {
					g.camera.Angle = math.Pi
				} else {
					g.camera.ViewDist = 1
				}
				combat := g.combat
				g.combat = nil // Any LOS/coverage work for this hidden field is a bug.
				hidden := r.visibleZoneHeatTiles()
				g.combat = combat
				g.camera.Angle, g.camera.ViewDist = angle, dist
				if len(hidden) != 0 {
					t.Fatal("offscreen heat was not culled before coverage checks")
				}
			}
		})
	}
}

func TestRareRulesCatalogEligibility(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	original := config.GlobalItems
	t.Cleanup(func() { config.GlobalItems = original })
	for _, key := range []string{"dawnleaf", "grave_orchid", "harm_flask", "brewed_health_potion", "carp_scale"} {
		def := original.Items[key]
		t.Run(key, func(t *testing.T) {
			config.GlobalItems = &config.ItemSystemConfig{Items: map[string]*config.ItemDefinitionConfig{key: def}}
			_, ok := rollCatalogItem(def.Type, def.Rarity, "", "")
			want := key == "carp_scale"
			if ok != want || config.CatalogItemMatchesFilter(key, def.Type, def.Rarity, "", "") != want {
				t.Fatal("runtime and validation eligibility disagree")
			}
			if !want {
				for i := 0; i < 100; i++ {
					it, ok := rollCatalogItemByRarity(def.Rarity)
					if ok && it.Name == def.Name {
						t.Fatal("forbidden material escaped rarity pool")
					}
				}
			}
		})
	}
}

func TestRareRulesSharedSplashVictims(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, source := range []string{"splash", "harm_flask", "venom_flask", "fire_flask"} {
			for _, state := range []string{"center", "edge", "outside", "dead", "card summon", "druid summon", "spell summon", "bound", "dormant", "warded", "lethal"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, source, state), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassAlchemist, tb)
					ts := float64(g.config.GetTileSize())
					center := zoneVictim(t, g)
					center.ArmorClass = 0
					target := zoneVictim(t, g)
					target.ArmorClass = 0
					target.X += ts
					want := 40
					switch state {
					case "center":
						target.X = center.X
					case "edge":
						target.X += ts
					case "outside":
						target.X += ts + 1
						want = 0
					case "dead":
						target.HitPoints, want = 0, 0
					case "card summon", "druid summon", "spell summon":
						if !markPartySummonKind(g, target, state) {
							t.Fatal("invalid summon fixture")
						}
						want = 0
					case "bound":
						target.Bound = true
						target.BoundFramesRemaining = 300
					case "dormant":
						target.BossDormant, want = true, 0
					case "warded":
						target.BossWarded, want = true, 0
					case "lethal":
						target.HitPoints, want = 10, 10
					}
					before, primary := target.HitPoints, center.HitPoints
					g.world.Monsters = append(g.world.Monsters, nil)
					var p MagicProjectile
					if source == "splash" {
						attack := g.combat.newPartyMonsterAttack(40, 0, "fire", 0, nil, "Test Splash", false, true, false)
						g.combat.applyAoeSplash(center, attack, 2)
					} else {
						p = MagicProjectile{Active: true, LifeTime: 100, FlaskKey: source, FlaskRadius: 2 * ts, Damage: 40, Attacker: c}
						if source == "venom_flask" {
							p.FlaskPoisonFrames = 120
						} else if source == "fire_flask" {
							p.FlaskBurnFrames = 120
						}
						g.combat.detonateFlask(&p, center.X, center.Y)
					}
					if before-target.HitPoints != want {
						t.Fatalf("victim damage=%d want=%d", before-target.HitPoints, want)
					}
					primaryWant := 40
					if source == "splash" {
						primaryWant = 0
					}
					if primary-center.HitPoints != primaryWant {
						t.Fatal("ground impact and primary-excluding splash disagree on center ownership")
					}
					rider := want > 0 && target.IsAlive()
					if (target.PoisonedFramesRemaining > 0) != (rider && source == "venom_flask") || (target.BurnFramesRemaining > 0) != (rider && source == "fire_flask") {
						t.Fatal("flask status rider reached an excluded or dead victim")
					}
					if state == "lethal" && !slices.Contains(g.deadMonsterIDs, target.ID) {
						t.Fatal("splash kill bypassed the shared death/reward pipeline")
					}
					if state == "lethal" && source != "splash" && g.collisionSystem.GetEntityByID(target.ID) != nil {
						t.Fatal("flask kill lost immediate collision cleanup")
					}
					if source != "splash" {
						hp, xp := target.HitPoints, c.Experience
						g.combat.detonateFlask(&p, center.X, center.Y)
						if target.HitPoints != hp || c.Experience != xp {
							t.Fatal("repeat impact dealt damage or awarded XP twice")
						}
					}
				})
			}
		}
	}
}

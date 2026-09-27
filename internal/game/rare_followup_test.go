package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

// Every flask and collision route shares the tile scatter, native particle
// lifecycle and damage-once gate in both combat modes. No FX state is saved.
func TestRareFlaskImpactLifecycle(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
			for _, hit := range []string{"body", "wall", "range"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", key, tb, hit), func(t *testing.T) {
					g, _ := rareClassGame(t, character.ClassAlchemist, tb)
					it, _ := items.TryCreateItemFromYAML(key)
					g.party.AddItem(it)
					if !g.throwFlask(0, key, false) {
						t.Fatal("throw failed")
					}
					p := &g.magicProjectiles[0]
					w := MagicProjectileWrapper{MagicProjectile: p, game: g, collisionSystem: g.collisionSystem, projectileID: p.ID}
					distance := p.FlaskFlightRange / 2
					if hit == "range" {
						distance = p.FlaskFlightRange
					}
					x, y := g.camera.X+distance, g.camera.Y
					m := zoneVictim(t, g)
					m.X, m.Y, m.ArmorClass = x, y, 0
					hp := m.HitPoints
					w.SetPosition(x, y)
					if hit == "body" {
						g.combat.applyProjectileDamage(p, "magic_projectile", m, p.ID)
					} else {
						if hit == "wall" {
							w.OnCollision(x, y)
						}
						w.ApplyCollisionEffects()
					}
					assertFlaskTileFootprint(t, g, x, y, p.FlaskRadius)
					if hp-m.HitPoints != p.Damage {
						t.Fatalf("damage=%d want=%d", hp-m.HitPoints, p.Damage)
					}
					effectCount := len(g.spellHitEffects)
					g.combat.detonateFlask(p, x, y)
					w.ApplyCollisionEffects()
					if len(g.spellHitEffects) != effectCount || hp-m.HitPoints != p.Damage {
						t.Fatal("duplicate detonation")
					}
					g.RemoveInactiveEntities()
					for n := 0; n < 2*g.config.GetTPS(); n++ {
						(&GameLoop{game: g}).updateProjectilesAndImpacts()
					}
					if len(g.spellHitEffects) != 0 {
						t.Fatal("native impact particles never expired")
					}
					d, _ := config.GetItemDefinition(key)
					g.createFlaskImpact(p, x, y, d.BrewColor)
					g.clearTransientCombatState()
					if len(g.spellHitEffects) != 0 {
						t.Fatal("world reset retained flask particles")
					}
				})
			}
		}
	}
}

func assertFlaskTileFootprint(t *testing.T, g *MMGame, x, y, radius float64) {
	t.Helper()
	tile := float64(g.config.GetTileSize())
	want := map[[2]int]bool{}
	for ty := TileIndex(y-radius, tile); ty <= TileIndex(y+radius, tile); ty++ {
		for tx := TileIndex(x-radius, tile); tx <= TileIndex(x+radius, tile); tx++ {
			if math.Hypot((float64(tx)+.5)*tile-x, (float64(ty)+.5)*tile-y) <= radius {
				want[[2]int{tx, ty}] = true
			}
		}
	}
	for _, effect := range g.spellHitEffects {
		if len(effect.Particles) == 0 || !effect.Particles[0].DepthTest {
			continue // A struck victim can also emit its ordinary hit sparks.
		}
		first := effect.Particles[0]
		cell := [2]int{TileIndex(first.X, tile), TileIndex(first.Y, tile)}
		if !want[cell] {
			t.Fatalf("unexpected or duplicate ground patch at %v", cell)
		}
		delete(want, cell)
		for _, p := range effect.Particles {
			if p.X != first.X || p.Y != first.Y || !p.Active || p.LifeTime <= 0 || p.LifeTime != p.MaxLife ||
				!p.DepthTest || p.OffsetY > g.groundOffsetY() || p.OffsetY < g.groundOffsetY()-.1*g.offsetYPerTileHeight() {
				t.Fatal("scatter did not emit live native particles anchored to the tile floor")
			}
		}
	}
	if len(want) > 0 {
		t.Fatalf("missing ground effects on %d affected tiles", len(want))
	}
}

func TestRareFlaskRetunedGroundFootprint(t *testing.T) {
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		for _, radius := range []int{1, 2, 4} {
			for _, offset := range []float64{0, .27, .5} {
				t.Run(fmt.Sprintf("%s/radius%d/offset%g", key, radius, offset), func(t *testing.T) {
					g, _ := rareClassGame(t, character.ClassAlchemist, false)
					d, _ := config.GetItemDefinition(key)
					old := d.Flask.RadiusTiles
					t.Cleanup(func() { d.Flask.RadiusTiles = old })
					d.Flask.RadiusTiles = radius
					it, _ := items.TryCreateItemFromYAML(key)
					g.party.AddItem(it)
					if !g.throwFlask(0, key, false) {
						t.Fatal("throw failed")
					}
					p := &g.magicProjectiles[0]
					tile := float64(g.config.GetTileSize())
					x, y := g.camera.X+(3+offset)*tile, g.camera.Y+offset*tile
					g.combat.detonateFlask(p, x, y)
					assertFlaskTileFootprint(t, g, x, y, float64(radius)*tile)
				})
			}
		}
	}
}

func TestRareFlaskGroundVisibility(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		t.Run(key, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			g.camera.FOV, g.camera.ViewDist = math.Pi/2, 1000
			g.renderHelper = NewRenderingHelper(g)
			d, _ := config.GetItemDefinition(key)
			tile := float64(g.config.GetTileSize())
			x, y := g.camera.X+4*tile, g.camera.Y
			p := &MagicProjectile{FlaskRadius: 2 * tile, FlaskPoisonFrames: d.Flask.PoisonSeconds[0], FlaskBurnFrames: d.Flask.BurnSeconds}
			g.createFlaskImpact(p, x, y, d.BrewColor)
			initial := g.spellHitEffects[0]
			initial.Particles = append([]SpellHitParticle(nil), initial.Particles...)
			r := NewRenderer(g)
			screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
			defer screen.Deallocate()
			for _, state := range []string{"clear", "wall_surface", "occluded", "behind_camera", "inactive", "expired"} {
				g.spellHitEffects = []SpellHitEffect{{Active: state != "inactive", Particles: append([]SpellHitParticle(nil), initial.Particles...)}}
				g.depthBuffer = nil
				if state == "wall_surface" || state == "occluded" {
					depth := initial.Particles[0].X - g.camera.X
					if state == "occluded" {
						depth /= 2
					}
					g.depthBuffer = make([]float64, g.config.GetScreenWidth())
					for i := range g.depthBuffer {
						g.depthBuffer[i] = depth
					}
				}
				if state == "behind_camera" {
					for i := range g.spellHitEffects[0].Particles {
						g.spellHitEffects[0].Particles[i].X = g.camera.X - tile
					}
				}
				if state == "expired" {
					for n := 0; n < 2*g.config.GetTPS(); n++ {
						g.UpdateHitEffects()
					}
				}
				runOnDrawFrame(func(_ *ebiten.Image) {
					screen.Clear()
					r.drawHitEffects(screen)
					pixels := make([]byte, 4*screen.Bounds().Dx()*screen.Bounds().Dy())
					screen.ReadPixels(pixels)
					visible := 0
					for i := 3; i < len(pixels); i += 4 {
						if pixels[i] > 0 {
							visible++
						}
					}
					want := state == "clear" || state == "wall_surface"
					if (want && visible < 25) || (!want && visible != 0) {
						t.Errorf("%s visible pixels=%d", state, visible)
					}
				})
			}
		})
	}
}

func TestRareReturnAcrossFoldChain(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier, tiles := range []int{3, 4, 6, 8} {
			for _, state := range []string{"clear", "intervening_wall", "save_load", "occupied", "landing_wall", "expired", "different_map"} {
				t.Run(fmt.Sprintf("TB=%v/tier=%d/%s", tb, tier, state), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, tb)
					c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.SkillMastery(tier)}
					x, y := g.camera.X, g.camera.Y
					ts := float64(g.config.GetTileSize())
					settle := func() {
						if tb {
							g.startPartyTurn()
						} else {
							g.tickRareClassClocks(3 * g.config.GetTPS())
						}
						g.spellInputCooldown = 0
					}
					for hop := 0; hop < 3; hop++ {
						if hop > 0 {
							settle()
						}
						c.ActionsRemaining = 0
						c.RTCooldown = 123
						it, _ := config.TechniqueItem("fold_step")
						c.Equipment[items.SlotSpell] = it
						rareInputCast(g, NewInputHandler(g), tb)
						if math.Abs(g.camera.X-x-float64((hop+1)*tiles)*ts) > .01 || c.ActionsRemaining != 0 || c.RTCooldown != 123 {
							t.Fatal("equipped F step did not use mastery range for free")
						}
						if c.RareClass.Anchor.X != x || c.RareClass.Anchor.Y != y || c.RareClass.Anchor.Frames != config.TierValue(config.Technique("fold_step").Duration, tier)*g.config.GetTPS() {
							t.Fatal("later Fold replaced first anchor or failed to refresh it")
						}
					}
					settle()
					switch state {
					case "intervening_wall":
						g.world.Tiles[10][10] = world.TileWall
					case "save_load":
						raw, err := json.Marshal(buildCharacterSave(c))
						if err != nil {
							t.Fatal(err)
						}
						var saved CharacterSave
						if err = json.Unmarshal(raw, &saved); err != nil {
							t.Fatal(err)
						}
						c = restoreCharacterSave(saved)
						g.party.Members[0] = c
					case "occupied":
						m := zoneVictim(t, g)
						m.X, m.Y = x, y
					case "landing_wall":
						g.world.Tiles[10][8] = world.TileWall
					case "expired":
						c.RareClass.Anchor.Frames = 0
					case "different_map":
						c.RareClass.Anchor.MapKey = "another_map"
					}
					want := state == "clear" || state == "intervening_wall" || state == "save_load"
					beforeX, sp, anchor := g.camera.X, c.SpellPoints, c.RareClass.Anchor
					got := g.useTechnique(0, "return_step", false, false)
					if got != want {
						t.Fatalf("Return=%v want=%v", got, want)
					}
					if got && (math.Abs(g.camera.X-x) > .01 || g.camera.Y != y || c.RareClass.Anchor.Frames != 0) {
						t.Fatal("Return missed first anchor")
					}
					if !got && (g.camera.X != beforeX || c.SpellPoints != sp || c.RareClass.Anchor != anchor || g.spatialReuseFrames != 0) {
						t.Fatal("failed Return spent state")
					}
				})
			}
		}
	}
}

func TestRareFlowingStaffPersistentCharges(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier, pct := range []int{25, 50, 75, 100} {
			t.Run(fmt.Sprintf("TB=%v/tier=%d", tb, tier), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.Skills[character.SkillFlowingStaff] = &character.Skill{Mastery: character.SkillMastery(tier)}
				c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("oak_staff")
				d := lookupWeaponConfigByName("Oak Staff")
				base := character.WeaponDamageBreakdown(d, c).Total
				if !g.useTechnique(0, "fold_step", false, false) {
					t.Fatal("Fold failed")
				}
				if c.FlowingStaffCharges() != tier+1 {
					t.Fatal("movement did not grant mastery capacity")
				}
				g.tickRareClassClocks(1000000)
				g.clearTransientCombatState()
				c = restoreCharacterSave(buildCharacterSave(c))
				g.party.Members[0] = c
				if c.FlowingStaffCharges() != tier+1 {
					t.Fatal("time, world reset or save load discarded charges")
				}
				for left := tier + 1; left > 0; left-- {
					b := character.WeaponDamageBreakdown(d, c)
					if b.FlowingStaff != base*pct/100 || b.Total != base+base*pct/100 {
						t.Fatal("wrong charged damage")
					}
					g.magicProjectiles = nil
					g.arrows = nil
					c.RTCooldown = 0
					c.ActionsRemaining = 2
					if !g.combat.equipmentAttackAtAngle(g.camera.Angle, false) || c.FlowingStaffCharges() != left-1 {
						t.Fatal("staff attack did not spend exactly one charge")
					}
				}
				if character.WeaponDamageBreakdown(d, c).FlowingStaff != 0 {
					t.Fatal("empty charges still boost damage")
				}
				g.notifyPilgrimDisplacement(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
				g.notifyPilgrimDisplacement(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
				if c.FlowingStaffCharges() != tier+1 {
					t.Fatal("refill exceeded mastery cap")
				}
			})
		}
	}
	g, c := rareClassGame(t, character.ClassWayfarer, false)
	c.RareClass.FlowFrames = 1
	c.RareClass.FlowCharges = 0
	c = restoreCharacterSave(buildCharacterSave(c))
	g.party.Members[0] = c
	g.tickRareClassClocks(100000)
	if c.FlowingStaffCharges() != 1 || c.RareClass.FlowFrames != 0 {
		t.Fatal("legacy timer did not migrate to one permanent charge")
	}
}

func TestRarePathfindingPartyAndRunning(t *testing.T) {
	for tier, pct := range []int{5, 10, 15, 20} {
		for _, source := range []string{"active", "unconscious", "reserve", "second_guide", "legacy"} {
			t.Run(fmt.Sprintf("tier=%d/%s", tier, source), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, false)
				c.Skills[character.SkillPathfinding] = &character.Skill{Mastery: character.SkillMastery(tier)}
				want := pct
				switch source {
				case "unconscious":
					c.HitPoints = 0
				case "reserve":
					g.party.Members = g.party.Members[1:]
				case "second_guide":
					g.party.Members[1] = character.CreateCharacter("Guide", character.ClassWayfarer, g.config)
				case "legacy":
					delete(c.Skills, character.SkillPathfinding)
					c.EnsureClassKitSkills(g.config)
					want = 5
				}
				if source == "unconscious" || source == "reserve" {
					want = 0
				}
				ih := NewInputHandler(g)
				for _, running := range []bool{false, true} {
					ih.heldKeys = func(k ebiten.Key) bool { return running && k == ebiten.KeyShiftLeft }
					base := g.config.GetMoveSpeed() * ih.movementScale()
					if running {
						base *= g.config.GetRunMultiplier()
					}
					if math.Abs(ih.moveSpeed()-base*(1+float64(want)/100)) > .00001 {
						t.Fatal("party movement bonus is missing, duplicated or applies from an inactive guide")
					}
				}
				if g.partyFireWhileRunning() != (want == 20) {
					t.Fatal("Grandmaster running gate differs from party aura")
				}
			})
		}
	}
	for _, entry := range []string{"fresh_attack", "held_attack", "fresh_cast", "held_cast", "mouse"} {
		for _, gm := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/GM=%v", entry, gm), func(t *testing.T) {
				g, guide := rareClassGame(t, character.ClassWayfarer, false)
				guide.Skills[character.SkillPathfinding] = &character.Skill{Mastery: character.MasteryMaster}
				if gm {
					guide.Skills[character.SkillPathfinding].Mastery = character.MasteryGrandMaster
				}
				caster := character.CreateCharacter("Caster", character.ClassSorcerer, g.config)
				caster.SpellPoints = 100
				caster.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
				g.party.Members = []*character.MMCharacter{caster, guide}
				g.selectedChar = 0
				ih := NewInputHandler(g)
				ih.heldKeys = func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft }
				switch entry {
				case "fresh_attack", "fresh_cast":
					key := ebiten.KeyR
					if entry == "fresh_cast" {
						key = ebiten.KeyF
					}
					ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == key })
					ih.keys.BeginFrame()
					ih.handleCombatInput()
				case "held_attack":
					ih.pendingRepeat = rtActWeapon
					ih.performPendingRepeat()
				case "held_cast":
					ih.pendingRepeat = rtActCast
					ih.performPendingRepeat()
				case "mouse":
					m := zoneVictim(t, g)
					m.X = g.camera.X + float64(g.config.GetTileSize())
					m.Y = g.camera.Y
					ih.performMouseSmartAttack(m)
				}
				fired := len(g.arrows) > 0 || len(g.magicProjectiles) > 0
				if fired != gm {
					t.Fatalf("running action fired=%v want=%v", fired, gm)
				}
			})
		}
	}
}

func TestRareAlchemyMaterialVariety(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	for _, r := range config.GlobalAlchemy.Recipes {
		keys := map[string]bool{}
		for _, a := range r.Ingredients[1].Alternatives {
			if len(a.Items) > 4 {
				t.Errorf("%s source icons exceed the measured row", r.Key)
			}
			for _, k := range a.Items {
				keys[k] = true
			}
		}
		if len(keys) < 4 || r.Family == "common" && (len(keys) < 8 || len(keys) > 12) {
			t.Errorf("%s has %d material alternatives", r.Key, len(keys))
		}
	}
}

func TestRarePathfindingAutocastAndProgression(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, source := range []string{"master", "grandmaster", "wyrmspine"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, source), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.Skills[character.SkillPathfinding] = &character.Skill{Mastery: character.MasteryMaster}
				if source == "grandmaster" {
					c.Skills[character.SkillPathfinding].Mastery = character.MasteryGrandMaster
				}
				if source == "wyrmspine" {
					g.party.Members[1].Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("wyrmspine_bow")
				}
				c.RareClass.Automatic = map[string]bool{"purify": true}
				g.party.Members[1].Conditions = []character.Condition{character.ConditionPoisoned}
				ih := NewInputHandler(g)
				ih.heldKeys = func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft }
				(&GameLoop{game: g, inputHandler: ih}).updateAutomaticTechniques()
				cast := !g.party.Members[1].Purifiable()
				if cast != (tb || source != "master") {
					t.Fatalf("automatic cast while running=%v", cast)
				}
			})
		}
	}
	g, c := rareClassGame(t, character.ClassWayfarer, false)
	if _, err := config.LoadLevelUpConfig("../../assets/level_up.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, level := range []int{3, 6, 9} {
		g.levelUpChoiceQueue = nil
		g.queueLevelUpChoices(c, level, config.GetLevelUpChoices("wayfarer", level))
		if len(g.levelUpChoiceQueue) != 1 {
			t.Fatal("no mastery choice")
		}
		found := false
		for _, opt := range g.levelUpChoiceQueue[0].options {
			if opt.skillType == character.SkillPathfinding {
				found = true
				if !opt.hasMastery {
					t.Fatal("Pathfinding is not a mastery option")
				}
			}
		}
		if !found || !g.trainSkill(c, character.SkillPathfinding) {
			t.Fatal("Pathfinding cannot be trained")
		}
		c = restoreCharacterSave(buildCharacterSave(c))
		g.party.Members[0] = c
	}
	if c.SkillTier(character.SkillPathfinding) != 3 || !g.partyFireWhileRunning() {
		t.Fatal("trained mastery did not survive save/load")
	}
}

func TestRareFlowingStaffHUDGeometry(t *testing.T) {
	for n := 0; n <= 4; n++ {
		boxes := flowingStaffChargeRects(panelPortraitX, panelPortraitY, panelPortraitW, n)
		if len(boxes) != n {
			t.Fatalf("%d charges have %d glyphs", n, len(boxes))
		}
		badges := makePartyProgressionBadgeLayout(panelPortraitX, panelPortraitY, panelPortraitW, panelPortraitH, true, true)
		for i, b := range boxes {
			if b.x < panelPortraitX+panelPortraitW || b.right() >= partyPanelContentLeft || b.y < 0 || b.bottom() > badges.stat.y {
				t.Fatal("charge icon overlaps portrait, stats or progression badges")
			}
			if i > 0 && boxes[i-1].bottom() > b.y {
				t.Fatal("charges overlap")
			}
		}
	}
}

func TestRareAuthoredMasteryChoices(t *testing.T) {
	g, c := rareClassGame(t, character.ClassWayfarer, false)
	cfg, err := config.LoadLevelUpConfig("../../assets/level_up.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for class, data := range cfg.LevelUps {
		for _, level := range data.Levels {
			for _, choice := range level.Choices {
				if choice.Type != "weapon_mastery" && choice.Type != "armor_mastery" {
					continue
				}
				t.Run(fmt.Sprintf("%s/%d/%s", class, level.Level, choice.Skill), func(t *testing.T) {
					skill, ok := character.SkillTypeFromKey(choice.Skill)
					if !ok || !skill.UsesMastery() {
						t.Fatal("invalid authored mastery")
					}
					for tier := character.MasteryNovice; tier < character.MasteryGrandMaster; tier++ {
						c.Skills[skill] = &character.Skill{Mastery: tier}
						opts := buildLevelUpChoiceOptions(c, []config.LevelUpChoice{choice})
						if len(opts) != 1 || opts[0].skillType != skill || !opts[0].hasMastery {
							t.Fatal("authored mastery silently skipped or replaced")
						}
						g.applyLevelUpOption(c, opts[0])
						if c.Skills[skill].Mastery != tier+1 {
							t.Fatal("choice did not upgrade its skill")
						}
					}
					if len(buildLevelUpChoiceOptions(c, []config.LevelUpChoice{choice})) != 0 {
						t.Fatal("max mastery still offered")
					}
				})
			}
		}
	}
	for _, key := range []string{"throwing", "unknown", "celestial_providence", "halfling_guile", "dark_elf_binding"} {
		if _, ok := skillTypeFromKey(key); ok {
			t.Fatalf("untrainable key %s offered", key)
		}
	}
}

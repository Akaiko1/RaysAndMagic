package game

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"
	"ugataima/internal/world"
)

func TestDestroyUndeadExecutionAndFallbackThroughInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"execute", "immune execution", "damage", "resisted damage", "living", "behind living", "sealed", "warded", "bound", "out of range", "wall", "no mana"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, state), func(t *testing.T) {
				g := advancedSupportGame(t, tb)
				g.world = newTestWorldSized(g.config, 16, 16)
				ts := g.config.GetTileSize()
				g.camera.X, g.camera.Y = 3.5*ts, 3.5*ts
				g.collisionSystem = collision.NewCollisionSystem(g.world, ts)
				equipSpellAndPrepareCaster(t, g.combat, "destroy_undead", 1000, 60)
				c := g.party.Members[0]
				c.Race = "human"
				d, _ := config.GetSpellDefinition("destroy_undead")
				previous := d.Mechanic
				mechanic := *previous
				d.Mechanic = &mechanic
				t.Cleanup(func() { d.Mechanic = previous })
				mechanic.InstantKillChance = 1
				m := mkTestMonster("Execution target", 10000)
				m.ID, m.MonsterType, m.X, m.Y = "execution-target", monster.TypeUndead, g.camera.X+2*ts, g.camera.Y
				m.Experience, m.Gold = 40, 19
				wantLoss, wantDead, wantPaid := 0, false, false
				var decoy *monster.Monster3D
				switch state {
				case "behind living":
					// A nearer living monster must not shadow the undead target.
					decoy = mkTestMonster("Living decoy", 10000)
					decoy.ID, decoy.X, decoy.Y = "living-decoy", g.camera.X+ts, g.camera.Y
					m.X = g.camera.X + 3*ts
					wantLoss, wantDead, wantPaid = 10000, true, true
				case "execute", "immune execution":
					wantLoss, wantDead, wantPaid = 10000, true, true
					if state == "immune execution" {
						m.Resistances[monster.DamageLight] = 100
					}
				case "damage", "resisted damage":
					mechanic.InstantKillChance = 0
					wantLoss, wantPaid = 80, true // 60 base + 60/3 Intellect at Novice.
					if state == "resisted damage" {
						m.Resistances[monster.DamageLight] = 50
						wantLoss = 40
					}
				case "living":
					m.MonsterType = "human"
				case "sealed":
					m.BossDormant = true
				case "warded":
					m.BossWarded = true
				case "bound":
					m.Bound = true
				case "out of range":
					m.X = g.camera.X + (mechanic.RadiusTiles+1)*ts
				case "wall":
					g.world.Tiles[3][4] = world.TileWall
				case "no mana":
					c.SpellPoints = 0
				}
				g.world.Monsters = []*monster.Monster3D{m}
				g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 32, 32, collision.CollisionTypeMonster, false))
				if decoy != nil {
					g.world.Monsters = append(g.world.Monsters, decoy)
					g.collisionSystem.RegisterEntity(collision.NewEntity(decoy.ID, decoy.X, decoy.Y, 32, 32, collision.CollisionTypeMonster, false))
					t.Cleanup(func() {
						if decoy.HitPoints != decoy.MaxHitPoints {
							t.Error("Destroy Undead struck the living decoy")
						}
					})
				}
				sp, xp, actions := c.SpellPoints, c.Experience, c.ActionsRemaining
				ih := NewInputHandler(g)
				pressKeys(ih, ebiten.KeyF)
				if got := 10000 - m.HitPoints; got != wantLoss {
					t.Fatalf("HP loss=%d, want %d", got, wantLoss)
				}
				wantSP := sp
				if wantPaid {
					wantSP -= c.MeditatedSPCost(d.SpellPointsCost)
				}
				if c.SpellPoints != wantSP {
					t.Fatalf("SP=%d, want %d", c.SpellPoints, wantSP)
				}
				if wantPaid && (tb && c.ActionsRemaining != actions-1 || !tb && c.RTCooldown <= 0) {
					t.Fatal("successful cast did not charge its action/cooldown once")
				}
				if wantDead {
					if len(g.deadMonsterIDs) != 1 || c.Experience != xp+10 || g.collisionSystem.GetEntityByID(m.ID) != nil || len(g.groundContainers) != 1 || g.groundContainers[0].Gold != 19 {
						t.Fatalf("execution bypassed death/reward cleanup: dead=%v xp=%d loot=%v", g.deadMonsterIDs, c.Experience-xp, g.groundContainers)
					}
					for _, entry := range g.combatLogHistory {
						if strings.Contains(entry.Text, "damage!") {
							t.Fatal("execution also applied fallback damage")
						}
					}
					g.combat.CastEquippedSpell()
					if c.SpellPoints != wantSP || c.Experience != xp+10 || len(g.groundContainers) != 1 {
						t.Fatal("dead target paid rewards or charged mana twice")
					}
				} else if c.Experience != xp || len(g.groundContainers) != 0 {
					t.Fatal("nonlethal cast awarded a kill")
				}
			})
		}
	}
}

func TestShrapmetalConeThroughInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier := 0; tier < 4; tier++ {
			t.Run(fmt.Sprintf("TB=%v/tier=%d", tb, tier), func(t *testing.T) {
				g := advancedSupportGame(t, tb)
				equipSpellAndPrepareCaster(t, g.combat, "shrapmetal", 1000, 60)
				g.cardSlots[0].key = "alien_card"
				d, _ := spells.GetSpellDefinitionByID("shrapmetal")
				c := g.party.Members[0]
				c.MagicSchools[c.SpellSchoolFor(d)].Mastery = character.SkillMastery(tier)
				g.camera.Angle = 1.2
				// Empty space is a legal shot; there is no enemy seeking or target requirement.
				ih := NewInputHandler(g)
				pressKeys(ih, ebiten.KeyF)
				if len(g.magicProjectiles) != d.Mechanic.Projectiles(tier) {
					t.Fatalf("wrong fragment count: %d", len(g.magicProjectiles))
				}
				if c.SpellPoints != 1000-c.MeditatedSPCost(d.SpellPointsCost) || tb && c.ActionsRemaining != 1 || !tb && c.RTCooldown <= 0 {
					t.Fatal("volley did not spend exactly one spell action")
				}
				// Every fragment carries the full single-shot payload: authored base
				// + INT 60 / 3 + 5 per mastery tier (crits are off in this fixture).
				want := d.DamageBase + 60/3 + tier*5
				ids := map[string]bool{}
				for _, p := range g.magicProjectiles {
					if p.DisintegrateChance != float64(cardDef("alien_card").CardDisintegratePct)/100 {
						t.Fatal("fragment omitted party disintegration")
					}
					ids[p.ID] = true
					angle := math.Atan2(p.VelY, p.VelX)
					if math.Abs(angle-g.camera.Angle) > d.Mechanic.ConeDegrees*math.Pi/360 || p.X != g.camera.X || p.Y != g.camera.Y || p.Launch.target != nil || p.NoCollide || g.collisionSystem.GetEntityByID(p.ID) == nil {
						t.Fatal("fragment lost its physical cone, common origin or collision entity")
					}
					if got := p.Damage + p.TrueDamage; got != want {
						t.Fatalf("fragment payload=%d, want %d", got, want)
					}
				}
				if len(ids) != len(g.magicProjectiles) {
					t.Fatal("fragments share an ID and collide as one body")
				}
			})
		}
	}
}

func TestShrapmetalSharedHitLimitThroughCollision(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, cap := range []int{1, 3} {
			for _, dodge := range []string{"none", "normal", "true damage"} {
				t.Run(fmt.Sprintf("TB=%v/cap=%d/dodge=%s", tb, cap, dodge), func(t *testing.T) {
					g := advancedSupportGame(t, tb)
					equipSpellAndPrepareCaster(t, g.combat, "shrapmetal", 1000, 60)
					c := g.party.Members[0]
					if dodge == "true damage" {
						def, _ := spells.GetSpellDefinitionByID("shrapmetal")
						c.MagicSchools[c.SpellSchoolFor(def)].Mastery = character.MasteryGrandMaster
					}
					d, _ := config.GetSpellDefinition("shrapmetal")
					previous := d.Mechanic
					mechanic := *previous
					mechanic.MaxHitsPerTarget = cap
					mechanic.ProjectilesByMastery = []int{8, 8, 8, 8}
					d.Mechanic = &mechanic
					t.Cleanup(func() { d.Mechanic = previous })
					// Reauthor count and cap: runtime must read both, not literal 7/3.
					m, other := mkTestMonster("first", 10000), mkTestMonster("second", 10000)
					m.ID, m.X = "first", 64
					other.ID, other.X = "second", 192
					g.world.Monsters = []*monster.Monster3D{m, other}
					for _, target := range g.world.Monsters {
						g.collisionSystem.RegisterEntity(collision.NewEntity(target.ID, target.X, target.Y, 16, 16, collision.CollisionTypeMonster, false))
					}
					if !g.combat.CastEquippedSpell() || !g.combat.CastEquippedSpell() {
						t.Fatal("overlapping casts refused")
					}
					if len(g.magicProjectiles) != 16 {
						t.Fatal("authored fragment ladder not used")
					}
					for i := range g.magicProjectiles {
						p := &g.magicProjectiles[i]
						target := m
						if i%8 >= 4 {
							target = other
						}
						// A complete dodge spends no hit; later hits from the same
						// volley can still fill the cap. Each next cast starts fresh.
						target.PerfectDodge = 0
						if dodge == "normal" && i%4 == 0 || dodge == "true damage" {
							target.PerfectDodge = 100
						}
						p.X, p.Y = target.X, target.Y
						g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
						g.combat.CheckProjectileMonsterCollisions()
						if g.magicProjectiles[i].Active || g.collisionSystem.GetEntityByID(p.ID) != nil {
							t.Fatal("impact fragment kept flying")
						}
					}
					perHit := 32
					if dodge == "true damage" {
						perHit = 15
					}
					for _, target := range g.world.Monsters {
						if got, want := 10000-target.HitPoints, 2*cap*perHit; got != want {
							t.Fatalf("%s lost %d HP, want %d", target.ID, got, want)
						}
					}
					if c.SpellPoints != 1000-2*c.MeditatedSPCost(d.SpellPointsCost) {
						t.Fatal("hits charged extra mana")
					}
				})
			}
		}
	}
}

func TestShrapmetalFlightStopsAtBodiesWallsAndRange(t *testing.T) {
	for _, mode := range []string{"RT", "TB", "switch"} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/wall=%v", mode, blocked), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 24, 24)
				g.threading = &threading.ThreadingComponents{EntityUpdater: entities.NewEntityUpdaterWithWorkers(1)}
				t.Cleanup(g.threading.Shutdown)
				g.turnBasedMode = mode == "TB"
				placePlayerAtTile(g, 10, 10, tile)
				// Face the front body (+x): the fixture's default heading points the
				// random cone away from it, so the hit count depended on the spread.
				g.camera.Angle = 0
				c := g.party.Members[0]
				isolateTrueDamageMember(c, 0)
				equipSpellAndPrepareCaster(t, g.combat, "shrapmetal", 1000, 60)
				d, _ := spells.GetSpellDefinitionByID("shrapmetal")
				c.MagicSchools[c.SpellSchoolFor(d)].Mastery = character.MasteryGrandMaster
				// A broad front body catches the whole cone; outside/behind actors
				// prove this is travel collision, not the previous targeted fan.
				front, behind, far := mkTestMonster("front", 10000), mkTestMonster("behind", 10000), mkTestMonster("far", 10000)
				front.ID, front.X, front.Y = "front", g.camera.X+tile, g.camera.Y
				behind.ID, behind.X, behind.Y = "behind", g.camera.X-tile, g.camera.Y
				far.ID, far.X, far.Y = "far", g.camera.X+8*tile, g.camera.Y
				g.world.Monsters = []*monster.Monster3D{front, behind, far}
				for _, m := range g.world.Monsters {
					g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, tile/2, tile*3, collision.CollisionTypeMonster, false))
				}
				if blocked {
					for row := 8; row <= 12; row++ {
						g.world.Tiles[row][11] = world.TileWall
					}
				}
				if !g.combat.CastEquippedSpell() {
					t.Fatal("volley refused")
				}
				ids := make([]string, len(g.magicProjectiles))
				frames := 0
				for i, p := range g.magicProjectiles {
					ids[i] = p.ID
					frames = max(frames, p.LifeTime)
				}
				for frame := 0; frame <= frames; frame++ {
					if mode == "switch" && frame == 5 {
						g.turnBasedMode = !g.turnBasedMode
					}
					gl.updateProjectilesAndImpacts()
				}
				want := d.Mechanic.MaxHitsPerTarget * 47 // 12 + 60/3 + GM 15.
				if blocked {
					want = 0
				}
				if 10000-front.HitPoints != want || behind.HitPoints != 10000 || far.HitPoints != 10000 {
					t.Fatalf("wrong physical victims: front=%d behind=%d far=%d, want front loss=%d", front.HitPoints, behind.HitPoints, far.HitPoints, want)
				}
				if len(g.magicProjectiles) != 0 {
					t.Fatal("fragments survived range/lifetime")
				}
				for _, id := range ids {
					if g.collisionSystem.GetEntityByID(id) != nil {
						t.Fatal("expired fragment left collision entity")
					}
				}
			})
		}
	}
}

func TestAdvancedDamageScalesThroughProductionCast(t *testing.T) {
	for _, id := range []spells.SpellID{"acid_burst", "flying_fist", "destroy_undead", "prismatic_light", "shrapmetal", "souldrinker"} {
		for tier := 0; tier < 4; tier++ {
			for _, intellect := range []int{0, 61} {
				t.Run(fmt.Sprintf("%s/tier=%d/INT=%d", id, tier, intellect), func(t *testing.T) {
					g := advancedSupportGame(t, false)
					c := g.party.Members[0]
					c.Intellect, c.Personality = intellect, 32
					d, err := spells.GetSpellDefinitionByID(id)
					if err != nil {
						t.Fatal(err)
					}
					if id == "destroy_undead" {
						// This table measures the fallback damage, not the execution roll.
						mechanic := *d.Mechanic
						mechanic.InstantKillChance = 0
						d.Mechanic = &mechanic
					}
					c.LearnSpell(id)
					c.MagicSchools[c.SpellSchoolFor(d)].Mastery = character.SkillMastery(tier)
					m := mkTestMonster("scaling target", 10000)
					m.ID, m.X, m.MonsterType = "scaling target", g.config.GetTileSize(), monster.TypeUndead
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					want := d.SpellPointsCost*3 + intellect/3 + tier*5
					if d.DamageBase > 0 {
						want = d.DamageBase + intellect/3 + tier*5
					}
					if id == "flying_fist" {
						want = d.SpellPointsCost*3 + 32/3 + tier*5
					}
					if id == "prismatic_light" {
						ray, _ := spells.GetSpellDefinitionByID("ray_of_light")
						want = (ray.SpellPointsCost*3*ray.DamageCostMultiplier + intellect/3 + 32/3 + tier*5) / 2
					}
					if !g.combat.castResolvedSpell(id, d, c, 0, false, false) {
						t.Fatal("cast rejected")
					}
					if d.IsProjectile {
						wantCount := 1
						if id == "shrapmetal" {
							wantCount = d.Mechanic.Projectiles(tier)
						}
						if len(g.magicProjectiles) != wantCount {
							t.Fatalf("got %d projectiles, want %d", len(g.magicProjectiles), wantCount)
						}
						p := &g.magicProjectiles[0]
						if p.Damage+p.TrueDamage != want {
							t.Fatalf("projectile payload=%d, want %d", p.Damage+p.TrueDamage, want)
						}
						p.X, p.Y = m.X, m.Y
						g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
						g.combat.CheckProjectileMonsterCollisions()
					}
					if got := 10000 - m.HitPoints; got != want {
						t.Fatalf("actual HP loss=%d, want %d", got, want)
					}
					if id == "prismatic_light" && tier == 3 {
						parts := g.combat.spellMasteryDamageParts(id, c, want)
						if parts.True != 7 {
							t.Fatalf("half-scaled mastery leaked %d true damage", parts.True)
						}
					}
				})
			}
		}
	}
}

func TestAdvancedDrainDistributesOnePoolWithoutWastingFullHeroShares(t *testing.T) {
	g := advancedSupportGame(t, false)
	for i, hp := range []int{1000, 999, 10, 0} {
		g.party.Members[i].HitPoints = hp
	}
	c := g.party.Members[0]
	c.Intellect = 300
	m := mkTestMonster("drain victim", 101)
	m.ID, m.X = "drain victim", g.config.GetTileSize()
	g.world.Monsters = []*monster.Monster3D{m}
	castAdvancedSupport(t, g, "souldrinker", 3, nil)
	for i, want := range []int{1000, 1000, 59, 0} {
		if got := g.party.Members[i].HitPoints; got != want {
			t.Fatalf("hero %d HP=%d, want %d", i, got, want)
		}
	}
}

func TestAreaSpellVisualFlightsDoNotDealASecondHit(t *testing.T) {
	for _, id := range []string{"prismatic_light", "souldrinker"} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB=%v", id, tb), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 24, 24)
				g.threading = &threading.ThreadingComponents{EntityUpdater: entities.NewEntityUpdaterWithWorkers(1)}
				t.Cleanup(g.threading.Shutdown)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 10, 10, tile)
				near := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "goblin", g.config)
				other := monster.NewMonster3DFromConfig(11.5*tile, 12.5*tile, "goblin", g.config)
				far := monster.NewMonster3DFromConfig(22.5*tile, 22.5*tile, "goblin", g.config)
				for _, m := range []*monster.Monster3D{near, other, far} {
					m.HitPoints, m.MaxHitPoints = 5000, 5000
				}
				g.world.Monsters = []*monster.Monster3D{near, other, far}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
				if err != nil {
					t.Fatal(err)
				}
				if !g.combat.castResolvedSpell(def.ID, def, g.party.Members[0], 0, false, false) {
					t.Fatal("area cast failed")
				}
				if near.HitPoints >= 5000 || other.HitPoints >= 5000 || far.HitPoints != 5000 {
					t.Fatal("instant area target set changed")
				}
				if len(g.magicProjectiles) != 2 {
					t.Fatalf("want one flight per affected enemy, got %d", len(g.magicProjectiles))
				}
				remaining := 0
				for _, p := range g.magicProjectiles {
					if !p.NoCollide || p.Damage != 0 || p.TrueDamage != 0 || g.collisionSystem.GetEntityByID(p.ID) != nil {
						t.Fatal("visual flight became a damaging/colliding projectile")
					}
					remaining = max(remaining, p.LifeTime)
				}
				hp := []int{near.HitPoints, other.HitPoints, far.HitPoints}
				for frame := 0; frame <= remaining; frame++ {
					gl.updateProjectilesAndImpacts()
				}
				if len(g.magicProjectiles) != 0 {
					t.Fatal("visual flights did not expire")
				}
				for i, m := range g.world.Monsters {
					if m.HitPoints != hp[i] {
						t.Fatal("visual flight applied damage twice")
					}
				}
			})
		}
	}
}

func TestTargetedSpellFansRespectTargetsAndAuthoredCap(t *testing.T) {
	for _, id := range []spells.SpellID{"prismatic_light"} {
		for _, tb := range []bool{false, true} {
			for _, count := range []int{0, 2, 9} {
				t.Run(fmt.Sprintf("%s/TB=%v/enemies=%d", id, tb, count), func(t *testing.T) {
					g, _, tile := tbBehaviorGame(t, 24, 24)
					g.turnBasedMode = tb
					placePlayerAtTile(g, 10, 10, tile)
					def, err := spells.GetSpellDefinitionByID(id)
					if err != nil {
						t.Fatal(err)
					}
					// Alter the authored cap, not a production constant: this also
					// proves the targeting path reads the mechanic data.
					mechanic := *def.Mechanic
					mechanic.MaxTargets = 3
					def.Mechanic = &mechanic
					for i := count - 1; i >= 0; i-- {
						a := float64(i) * .6
						d := tile * (1.2 + float64(i)*.20)
						m := monster.NewMonster3DFromConfig(g.camera.X+math.Cos(a)*d, g.camera.Y+math.Sin(a)*d, "goblin", g.config)
						m.ID = fmt.Sprintf("fan-%d", i)
						m.HitPoints, m.MaxHitPoints = 5000, 5000
						g.world.Monsters = append(g.world.Monsters, m)
					}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					result := g.combat.castSpell(spellCastRequest{ID: id, Definition: def, Caster: g.party.Members[0]})
					if (result == castCommitted) != (count > 0) {
						t.Fatalf("cast result %v with %d enemies", result, count)
					}
					want := min(count, mechanic.MaxTargets)
					if len(g.magicProjectiles) != want {
						t.Fatalf("got %d flights, want %d", len(g.magicProjectiles), want)
					}
					for i, p := range g.magicProjectiles {
						target := g.world.Monsters[count-1-i]
						if p.Launch.x != g.camera.X || p.Launch.y != g.camera.Y || p.Launch.aimX != target.X || p.Launch.aimY != target.Y {
							t.Fatal("fan lost its common origin or nearest distinct target")
						}
					}
					if id == "prismatic_light" {
						for i, m := range g.world.Monsters {
							if (m.HitPoints < 5000) != (i >= count-want) {
								t.Fatal("damage and visual fan selected different victims")
							}
						}
					}
				})
			}
		}
	}
}

func TestAdvancedFearRetreatsWithoutAttackingInBothModes(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, rooted := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/root=%v", tb, rooted), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 24, 24)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 10, 10, tile)
				m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "puma", g.config)
				m.ApplyFear(10 * g.config.GetTPS())
				if rooted {
					g.combat.applyMonsterRoot(m, 1)
				}
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				before, hp := Distance(m.X, m.Y, g.camera.X, g.camera.Y), partyHPSum(g)
				g.refreshMonsterAIState()
				if tb {
					runOneMonsterTurn(g, gl)
				} else {
					wr := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
					wr.Update()
					wr.ApplyCollisionUpdate()
					g.combat.HandleMonsterInteractions()
				}
				after := Distance(m.X, m.Y, g.camera.X, g.camera.Y)
				if m.CurrentAIBehavior() != monster.AIBehaviorFleeing || partyHPSum(g) != hp {
					t.Fatal("fear allowed attack or lost retreat ownership")
				}
				if rooted && after != before || !rooted && after <= before {
					t.Fatalf("retreat distance %g -> %g, rooted=%v", before, after, rooted)
				}
			})
		}
	}
}

func TestAdvancedControlsAndIceSurviveSaveRestore(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 8, 8)
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"
	setTestWorldManager(t, wm)
	g := newTestGame(cfg, w)
	m := monster.NewMonster3DFromConfig(128, 128, "bandit", cfg)
	m.ID = "advanced-control-save"
	m.StunDR().Step(5, cfg.GetTPS())
	m.ApplyFear(181)
	m.Silence, m.Berserk = framesClock(241), framesClock(301)
	m.SilenceImmunity = framesClock(720)
	w.Monsters = []*monster.Monster3D{m}
	g.frozenWater = []FrozenWaterTile{{MapKey: "forest", X: 3, Y: 3, Clock: framesClock(1440)}, {MapKey: "desert", X: 4, Y: 5, LegacyFrames: 1800}}
	snapshot := g.buildSave(wm)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err = json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	loadedWorld := newTestWorldSized(cfg, 8, 8)
	wm.LoadedMaps = map[string]*world.World3D{"forest": loadedWorld}
	loaded := newTestGame(cfg, loadedWorld)
	if err = loaded.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if len(loadedWorld.Monsters) != 1 {
		t.Fatal("saved monster missing")
	}
	restored := loadedWorld.Monsters[0]
	if restored.StunDRStacks != m.StunDRStacks || restored.StunDRMemory.Frames != m.StunDRMemory.Frames || restored.StunDRMemory.Turns != m.StunDRMemory.Turns {
		t.Fatal("saving and loading reset the shared control protection")
	}
	if restored.Fear.Frames != 181 || restored.Silence.Frames != 241 || restored.SilenceImmunity.Frames != 720 || restored.Berserk.Frames != 301 {
		t.Fatal("monster spell controls were not restored")
	}
	if len(loaded.frozenWater) != 2 || loaded.frozenWater[0].Clock.Frames != 1440 || loaded.frozenWater[1].MapKey != "desert" || loaded.frozenWater[1].Clock.Frames != 1800 {
		t.Fatal("map-scoped ice was not restored")
	}
	spendFrames(60, loaded.advanceControlledMonsters)
	if restored.Fear.Frames != 121 || restored.Silence.Frames != 181 {
		t.Fatal("restored controls do not use shared elapsed clock")
	}
}

func TestAdvancedControlSpellsThroughCast(t *testing.T) {
	for _, id := range []string{"slow", "frozen_ground", "turn_undead", "mass_fear", "feeblemind", "berserk"} {
		t.Run(id, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			cs.controlSaveRoll = func() float64 { return 0.99 } // This table checks successful target eligibility.
			g := cs.game
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
			if err != nil {
				t.Fatal(err)
			}
			ts := g.config.GetTileSize()
			living, dead, far := mkTestMonster("living", 100), mkTestMonster("undead", 100), mkTestMonster("far", 100)
			living.ID, living.X = "living", ts
			dead.ID, dead.X, dead.MonsterType = "undead", 2*ts, monster.TypeUndead
			far.ID, far.X = "far", 20*ts
			if id == "berserk" {
				// The nearer undead is ineligible; Berserk must reach past it.
				living.X, dead.X = dead.X, living.X
			}
			g.world.Monsters = []*monster.Monster3D{living, dead, far}
			if !cs.castResolvedSpell(def.ID, def, g.party.Members[0], 0, false, false) {
				t.Fatal("cast not handled")
			}
			switch id {
			case "slow":
				if living.SlowPct == 0 || dead.SlowPct == 0 || far.SlowPct != 0 {
					t.Fatal("mass slow target set incorrect")
				}
			case "frozen_ground":
				if living.Root.Frames == 0 || dead.Root.Frames == 0 || far.Root.Frames != 0 {
					t.Fatal("root target set incorrect")
				}
			case "turn_undead":
				if dead.Fear.Frames == 0 || living.Fear.Frames != 0 {
					t.Fatal("undead fear eligibility incorrect")
				}
			case "mass_fear":
				if living.Fear.Frames == 0 || dead.Fear.Frames != 0 {
					t.Fatal("living fear eligibility incorrect")
				}
			case "feeblemind":
				if living.Silence.Frames == 0 || dead.Silence.Frames == 0 || far.Silence.Frames != 0 {
					t.Fatal("silence target set incorrect")
				}
			case "berserk":
				if living.Berserk.Frames == 0 || dead.Berserk.Frames != 0 {
					t.Fatal("berserk target selection incorrect")
				}
			}
		})
	}
}

func TestAdvancedFearAndSilenceLifecycle(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	m := mkTestMonster("controlled", 100)
	m.X, m.ID, m.ProjectileSpell = 64, "controlled", "firebolt"
	g.world.Monsters = []*monster.Monster3D{m}
	m.ApplyFear(200)
	cs.markMonsterHit(m)
	if m.CurrentAIBehavior() != monster.AIBehaviorFleeing || m.CurrentAIBehavior().Caps().MayAttack {
		t.Fatal("damage cancelled fear")
	}
	m.Silence = framesClock(200)
	if m.HasRangedAttack() || cs.spawnMonsterRangedAttackAt(m, 0, 0, ProjectileOwnerMonster) {
		t.Fatal("silenced monster fired")
	}
	g.turnBasedMode = true
	(&GameLoop{game: g}).updateControlledMonsters() // a TB thinking frame
	g.turnBasedMode = false
	if m.Fear.Frames != 200 {
		t.Fatal("idle clock spent fear")
	}
	spendFrames(199, g.advanceControlledMonsters)
	if m.Fear.Frames != 1 || m.Silence.Frames != 1 {
		t.Fatal("shared clock did not preserve remainder")
	}
	g.advanceControlledMonsters(false)
	if m.Fear.Frames != 0 || !m.HasRangedAttack() {
		t.Fatal("control did not expire")
	}
}

func TestFrozenWaterUsesConeAndExpires(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	g := cs.game
	previousTiles := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previousTiles })
	world.GlobalTileManager = world.NewTileManager(g.config.Graphics.SizeClasses)
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	g.world = newTestWorldSized(g.config, 9, 9)
	ts := g.config.GetTileSize()
	g.camera.X, g.camera.Y = 3.5*ts, 4.5*ts
	g.world.Tiles[4][4], g.world.Tiles[4][2], g.world.Tiles[2][3] = world.TileWater, world.TileWater, world.TileWater
	g.world.Tiles[4][5] = world.TileForestStream
	g.collisionSystem = collision.NewCollisionSystem(g.world, ts)
	def, err := spells.GetSpellDefinitionByID("frozen_ground")
	if err != nil {
		t.Fatal(err)
	}
	if !cs.castResolvedSpell(def.ID, def, g.party.Members[0], 0, false, false) {
		t.Fatal("freeze cast failed")
	}
	if !g.isFrozenWaterTile(g.world, 5, 4) || !g.isFrozenWaterTile(g.world, 4, 4) || g.isFrozenWaterTile(g.world, 2, 4) || g.isFrozenWaterTile(g.world, 3, 2) {
		t.Fatal("frozen water is not a forward cone")
	}
	if g.world.IsTileBlockingTerrainAt(4, 4) {
		t.Fatal("frozen water still blocks actual world movement")
	}
	// Party and monster collision (live and worker snapshot) must agree. Party
	// water-walking is intentionally not inherited by grounded monsters.
	walker := spawnTestMonsterAt(g, 3, 3)
	wx, wy := 4.5*ts, 4.5*ts
	if g.world.IsTileBlockingForMonster(4, 4, nil, false) ||
		!g.collisionSystem.CanMoveToWithTileOverrides(walker.ID, wx, wy, nil, false) ||
		!g.collisionSystem.Snapshot().CanMoveToWithTileOverrides(walker.ID, wx, wy, nil, false) {
		t.Fatal("ice blocks monster pathfinding or committed movement")
	}
	if !g.world.IsTileBlockingForMonster(2, 4, nil, false) {
		t.Fatal("unfrozen water became walkable for grounded monsters")
	}
	savedIce := append([]FrozenWaterTile(nil), g.frozenWater...)
	g.replaceFrozenWater(nil)
	if !g.world.IsTileBlockingForMonster(4, 4, nil, false) {
		t.Fatal("cleared ice remained walkable")
	}
	g.restoreSavedEffects(&GameSave{FrozenWater: savedIce})
	if g.world.IsTileBlockingForMonster(4, 4, nil, false) {
		t.Fatal("restored ice blocks monsters")
	}

	// Arrival checks (beacon, portal) on a world the party has left read its live ice.
	lake := g.world
	wm := world.NewWorldManager(g.config)
	wm.LoadedMaps = map[string]*world.World3D{"lake": lake, "elsewhere": newTestWorldSized(g.config, 9, 9)}
	wm.CurrentMapKey = "elsewhere"
	setTestWorldManager(t, wm)
	for i := range g.frozenWater {
		g.frozenWater[i].MapKey = "lake"
	}
	g.world = wm.LoadedMaps["elsewhere"]
	if err := g.arrivalTileError("lake", [2]int{4, 4}); err != nil {
		t.Fatalf("live ice on the left world refused: %v", err)
	}
	spendFrames(def.Mechanic.WaterFreezeSeconds*g.config.GetTPS(), g.advanceFrozenWater)
	if !lake.IsTileBlockingForMonster(4, 4, nil, false) {
		t.Fatal("monsters still walk on expired ice in another world")
	}
	if g.isFrozenWaterTile(lake, 4, 4) || g.arrivalTileError("lake", [2]int{4, 4}) == nil {
		t.Fatal("ice did not restore original water collision")
	}
}

// Thawed ice never strands an actor in water: neither on the current world at
// the thaw, nor on a world re-entered after its ice melted while the party was away.
func TestThawSettlesStrandedActors(t *testing.T) {
	for _, away := range []bool{false, true} {
		t.Run(fmt.Sprintf("away=%v", away), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			previousTiles := world.GlobalTileManager
			t.Cleanup(func() { world.GlobalTileManager = previousTiles })
			world.GlobalTileManager = world.NewTileManager(g.config.Graphics.SizeClasses)
			if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
				t.Fatal(err)
			}
			ts := g.config.GetTileSize()
			lake := newTestWorldSized(g.config, 9, 9)
			lake.Tiles[4][4] = world.TileWater
			wm := world.NewWorldManager(g.config)
			wm.LoadedMaps = map[string]*world.World3D{"lake": lake, "elsewhere": newTestWorldSized(g.config, 9, 9)}
			wm.CurrentMapKey = "lake"
			setTestWorldManager(t, wm)
			g.world = lake
			g.collisionSystem = collision.NewCollisionSystem(lake, ts)
			g.skyImg, g.groundImg = ebiten.NewImage(1, 1), ebiten.NewImage(1, 1) // a map switch repaints both
			t.Cleanup(g.skyImg.Deallocate)
			t.Cleanup(g.groundImg.Deallocate)
			g.frozenWater = []FrozenWaterTile{{MapKey: "lake", X: 4, Y: 4, Clock: framesClock(1)}}
			g.syncFrozenWaterCollision()
			wader, dry := mkTestMonster("wader", 100), mkTestMonster("dry", 100)
			wader.ID, dry.ID = "wader", "dry"
			wader.X, wader.Y = TileCenterFromTile(4, 4, ts)
			dry.X, dry.Y = TileCenterFromTile(6, 6, ts)
			lake.Monsters = []*monster.Monster3D{wader, dry}
			lake.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			// A solid NPC stands on the nearest shore tile: the walker must
			// land beside it, never inside it.
			npcX, npcY := TileCenterFromTile(4, 3, ts)
			g.collisionSystem.RegisterEntity(collision.NewEntity("shore_npc", npcX, npcY, 32, 32, collision.CollisionTypeNPC, true))
			if !g.collisionSystem.CanOccupyTilesWithTileOverrides(wader.ID, wader.X, wader.Y, nil, false) {
				t.Fatal("live ice does not hold a walker")
			}
			dryX, dryY := dry.X, dry.Y
			if away {
				if err := g.switchToMap("elsewhere"); err != nil {
					t.Fatal(err)
				}
				g.advanceFrozenWater(false)
				if err := g.switchToMap("lake"); err != nil {
					t.Fatal(err)
				}
			} else {
				g.advanceFrozenWater(false)
			}
			iceX, iceY := TileCenterFromTile(4, 4, ts)
			if Distance(wader.X, wader.Y, iceX, iceY) != ts || !g.collisionSystem.CanMoveToWithTileOverrides(wader.ID, wader.X, wader.Y, nil, false) {
				t.Fatalf("thawed walker at (%v,%v), want the nearest dry tile clear of the NPC", wader.X, wader.Y)
			}
			if dry.X != dryX || dry.Y != dryY {
				t.Fatal("a monster on dry ground was moved")
			}
		})
	}
}

func TestAdvancedSpellProjectileRiders(t *testing.T) {
	for _, id := range []string{"acid_burst", "flying_fist"} {
		t.Run(id, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
			if err != nil {
				t.Fatal(err)
			}
			ts := g.config.GetTileSize()
			m := mkTestMonster("victim", 10000)
			m.Speed = 1
			g.camera.X, g.camera.Y = ts, ts
			g.collisionSystem.UpdateEntity("player", ts, ts)
			m.ID, m.X, m.Y = "victim", 4*ts, ts
			g.world.Monsters = []*monster.Monster3D{m}
			g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, false))
			if !cs.castResolvedSpell(def.ID, def, g.party.Members[0], 0, false, false) {
				t.Fatal("cast failed")
			}
			if len(g.magicProjectiles) != 1 {
				t.Fatal("normal spell did not launch one projectile")
			}
			p := &g.magicProjectiles[0]
			p.X, p.Y = m.X, m.Y
			g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
			beforeX := m.X
			cs.CheckProjectileMonsterCollisions()
			if m.HitPoints >= 10000 {
				t.Fatal("projectile impact dealt no damage")
			}
			if id == "acid_burst" && m.ArmorShredPct == 0 {
				t.Fatal("acid hit did not corrode armor")
			}
			if id == "flying_fist" && m.X <= beforeX {
				t.Fatal("fist hit did not displace its target")
			}
		})
	}
}

func TestBerserkSelectsNearestActorIncludingParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ts := g.config.GetTileSize()
	m, other := mkTestMonster("berserk", 100), mkTestMonster("other", 100)
	m.ID, m.X, m.Berserk = "berserk", 3*ts, framesClock(100)
	other.ID, other.X = "other", 4*ts
	g.world.Monsters = []*monster.Monster3D{m, other}
	if cs.monsterAIFoeMonster(m) != other {
		t.Fatal("berserk ignores nearer hostile")
	}
	g.camera.X = 2.5 * ts
	if cs.monsterAIFoeMonster(m) != nil {
		t.Fatal("berserk ignores nearer party")
	}
	spendFrames(100, m.AdvanceSpellControls)
	if cs.monsterAIFoeMonster(m) != nil {
		t.Fatal("expired berserk retains hostile faction target")
	}
}

func TestFrozenWaterMaterialFollowsCastAndThawState(t *testing.T) {
	r, wm := terrainTestRenderer(t, "forest")
	g := r.game
	g.gameLoop = &GameLoop{game: g, renderer: r}
	g.world.Tiles[3][3] = world.TileWater
	g.frozenWater = []FrozenWaterTile{{MapKey: wm.CurrentMapKey, X: 3, Y: 3, Clock: framesClock(1)}}
	r.buildFloorColorMap(g.world.Width, g.world.Height)
	i := (3*g.world.Width+3)*4 + 2
	base := r.floorTextureIndexPixels[i] &^ floorFrozenWater
	if r.floorTextureIndexPixels[i]&floorFrozenWater == 0 {
		t.Fatal("loaded frozen water has no floor material")
	}
	g.advanceFrozenWater(false)
	if r.floorTextureIndexPixels[i] != base {
		t.Fatal("thaw did not restore the exact underlying floor profile")
	}
	// A load swaps the ice list after the floor was rebuilt; a reset drops it.
	g.restoreSavedEffects(&GameSave{FrozenWater: []FrozenWaterTile{{MapKey: wm.CurrentMapKey, X: 3, Y: 3, Clock: framesClock(60)}}})
	if r.floorTextureIndexPixels[i]&floorFrozenWater == 0 || g.world.IsTileBlockingTerrainAt(3, 3) {
		t.Fatal("restored ice is not both drawn and walkable")
	}
	g.resetTimedEffects()
	if r.floorTextureIndexPixels[i] != base || !g.world.IsTileBlockingTerrainAt(3, 3) {
		t.Fatal("reset left ice drawn or walkable")
	}
}

func TestAdvancedProjectileDamageTypes(t *testing.T) {
	for _, tc := range []struct {
		id           spells.SpellID
		typeOfDamage monster.DamageType
		otherType    monster.DamageType
	}{
		{"flying_fist", monster.DamageBody, monster.DamagePhysical},
		{"shrapmetal", monster.DamagePhysical, monster.DamageDark},
	} {
		for _, immune := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/immune=%v", tc.id, immune), func(t *testing.T) {
				g := advancedSupportGame(t, false)
				c := g.party.Members[0]
				c.Luck = 0
				d, _ := spells.GetSpellDefinitionByID(tc.id)
				if got := spellDamageTypeStr(string(tc.id)); got != tc.typeOfDamage.String() {
					t.Fatalf("shared projectile type=%s, want %s", got, tc.typeOfDamage)
				}
				c.LearnSpell(tc.id)
				m := mkTestMonster("Damage type target", 10000)
				m.ID, m.X = "damage-type-target", g.config.GetTileSize()
				resist := tc.otherType
				if immune {
					resist = tc.typeOfDamage
				}
				m.Resistances[resist] = 100
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				if !g.combat.castResolvedSpell(tc.id, d, c, 0, false, false) || len(g.magicProjectiles) == 0 {
					t.Fatal("projectile did not launch")
				}
				p := &g.magicProjectiles[0]
				want := p.Damage + p.TrueDamage
				if immune {
					want = 0
				}
				p.X, p.Y = m.X, m.Y
				g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
				g.combat.CheckProjectileMonsterCollisions()
				if got := 10000 - m.HitPoints; got != want {
					t.Fatalf("HP loss=%d, want %d for %s against %s resistance", got, want, tc.typeOfDamage, resist)
				}
			})
		}
	}
}

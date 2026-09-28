package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func flaskTargetFixture(t *testing.T, key string, tb bool) (*MMGame, *InputHandler) {
	t.Helper()
	g, c := rareClassGame(t, character.ClassAlchemist, tb)
	g.appScreen = AppScreenInGame
	g.party.Members = []*character.MMCharacter{c}
	g.party.Inventory = nil
	g.camera.FOV = math.Pi / 2
	it, err := items.TryCreateItemFromYAML(key)
	if err != nil {
		t.Fatal(err)
	}
	it.Quantity = 3
	g.party.AddItem(it)
	if !g.equipFlask(0, key) {
		t.Fatal("equip failed")
	}
	return g, NewInputHandler(g)
}

func addFlaskTarget(g *MMGame, dx, dy float64) *monster.Monster3D {
	ts := float64(g.config.GetTileSize())
	m := monster.NewMonster3DFromConfig(g.camera.X+dx*ts, g.camera.Y+dy*ts, "goblin", g.config)
	m.HitPoints, m.MaxHitPoints, m.PerfectDodge, m.ArmorClass = 10000, 10000, 0, 0
	g.world.Monsters = append(g.world.Monsters, m)
	return m
}

func sendFlaskInput(g *MMGame, ih *InputHandler, input string, target *monster.Monster3D) {
	if input == "click" || input == "intercepted" {
		ih.performMouseSmartAttack(target)
		return
	}
	if g.turnBasedMode {
		key := ebiten.KeySpace
		if input == "F" {
			key = ebiten.KeyF
		}
		ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == key })
		ih.keys.BeginFrame()
		ih.handleTurnBasedInput()
	} else if input == "repeat" {
		ih.pendingRepeat = rtActSmart
		ih.performPendingRepeat()
	} else if input == "F" {
		ih.performRTCombatAction(rtActCast, true)
	} else {
		ih.performRTCombatAction(rtActSmart, false)
	}
}

func assertFlaskAim(t *testing.T, g *MMGame, key string, target *monster.Monster3D) {
	t.Helper()
	if len(g.magicProjectiles) != 1 || g.flaskStock(g.party.Members[0], key) != 2 {
		t.Fatalf("one launch must consume one bottle: shots=%d stock=%d", len(g.magicProjectiles), g.flaskStock(g.party.Members[0], key))
	}
	p := g.magicProjectiles[0]
	wantAngle := g.camera.Angle
	d, _ := config.GetItemDefinition(key)
	wantDistance := float64(d.Flask.RangeTiles) * g.config.GetTileSize()
	if target != nil {
		wantAngle = math.Atan2(target.Y-g.camera.Y, target.X-g.camera.X)
		wantDistance = min(wantDistance, math.Hypot(target.X-g.camera.X, target.Y-g.camera.Y))
	}
	if math.Abs(math.Atan2(p.VelY, p.VelX)-wantAngle) > 1e-6 || math.Abs(p.FlaskFlightRange-wantDistance) > 1e-6 || !p.WorldAim {
		t.Fatalf("wrong launch: angle=%g distance=%g, want %g/%g", math.Atan2(p.VelY, p.VelX), p.FlaskFlightRange, wantAngle, wantDistance)
	}
	c := g.party.Members[0]
	if g.turnBasedMode && c.ActionsRemaining != 1 || !g.turnBasedMode && c.RTCooldown <= 0 {
		t.Fatal("throw did not spend exactly one action/cooldown")
	}
}

func TestAlchemyFlaskForwardAcquisition(t *testing.T) {
	cases := []string{"near_left", "near_right", "dead", "charmed", "card summon", "druid summon", "spell summon", "caravan", "bound", "passive", "wildlife", "behind", "outside_cone", "beyond_range", "wall", "empty", "diagonal"}
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
			for _, state := range cases {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, key, state), func(t *testing.T) {
					g, ih := flaskTargetFixture(t, key, tb)
					far := addFlaskTarget(g, 5, 0)
					near := addFlaskTarget(g, 3, .8)
					want := near
					ts := float64(g.config.GetTileSize())
					switch state {
					case "near_left":
						near.Y = g.camera.Y - .8*ts
					case "dead":
						near.HitPoints, want = 0, far
					case "charmed":
						near.Pacified, want = true, far
					case "card summon", "druid summon", "spell summon":
						if !markPartySummonKind(g, near, state) {
							t.Fatal("missing summon source")
						}
						want = far
					case "caravan":
						near.Disposition, want = monster.DispositionCaravan, far
					case "bound":
						near.Bound = true
					case "passive":
						near.PassiveUntilAttacked = true
					case "wildlife":
						near.Disposition = monster.DispositionWildlife
					case "behind":
						near.X, want = g.camera.X-ts, far
					case "outside_cone":
						near.X, near.Y, want = g.camera.X+2*ts, g.camera.Y+3*ts, far
					case "beyond_range":
						near.X, want = g.camera.X+7*ts, far
					case "wall":
						g.world.Monsters = []*monster.Monster3D{near}
						g.world.Tiles[10][9] = world.TileWall
						want = nil
					case "empty":
						g.world.Monsters, want = nil, nil
					case "diagonal":
						near.X, near.Y = g.camera.X+ts, g.camera.Y+ts
						if !tb {
							want = far
						}
					}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					sendFlaskInput(g, ih, "Space", nil)
					assertFlaskAim(t, g, key, want)
				})
			}
		}
	}
}

func TestAlchemyFlaskInputAndImpact(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
			for _, input := range []string{"Space", "F", "repeat", "click", "intercepted"} {
				if tb && input == "repeat" {
					continue // TB consumes key edges, not the RT repeat queue.
				}
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, key, input), func(t *testing.T) {
					g, ih := flaskTargetFixture(t, key, tb)
					target := addFlaskTarget(g, 3, -.8)
					victim := target
					if input == "click" {
						addFlaskTarget(g, 1.5, .8) // Nearer, but outside the bottle's physical path.
					} else if input == "intercepted" {
						victim = addFlaskTarget(g, 1, 0) // Aiming cannot bypass an intervening body.
					}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					sendFlaskInput(g, ih, input, target)
					assertFlaskAim(t, g, key, target)
					if g.camera.Angle != 0 || g.combat.partyAimTarget != nil {
						t.Fatal("throw changed the camera or leaked target state")
					}
					g.camera.Angle = math.Pi // Rotation cannot redirect an airborne bottle.
					p := &g.magicProjectiles[0]
					wrapper := MagicProjectileWrapper{MagicProjectile: p, game: g, collisionSystem: g.collisionSystem, projectileID: p.ID}
					for n := 0; n < 2*g.config.GetTPS() && p.Active; n++ {
						wrapper.SetPosition(p.X+p.VelX, p.Y+p.VelY)
						wrapper.ApplyCollisionEffects()
						g.combat.CheckProjectileMonsterCollisions()
					}
					if p.Active || p.LifeTime != 0 || victim.HitPoints != victim.MaxHitPoints-p.Damage {
						t.Fatalf("aimed flask failed to hit once: HP=%d, damage=%d, active=%v", victim.HitPoints, p.Damage, p.Active)
					}
					if key == "venom_flask" && victim.PoisonedFramesRemaining == 0 || key == "fire_flask" && victim.BurnFramesRemaining == 0 {
						t.Fatal("aimed impact lost its status payload")
					}
					hp := victim.HitPoints
					wrapper.ApplyCollisionEffects()
					g.combat.CheckProjectileMonsterCollisions()
					if victim.HitPoints != hp || g.flaskStock(g.party.Members[0], key) != 2 {
						t.Fatal("impact spent another bottle or applied damage twice")
					}
				})
			}
		}
	}
}

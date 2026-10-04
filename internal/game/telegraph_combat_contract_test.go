package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
	"ugataima/internal/threading"
	"ugataima/internal/world"
)

// Case table: RT/TB x party/actor x impact/field x normal/weakened x
// reflection off/on (reflection is party-only). All cells use runMonsterFrame.
// Lifecycle: lethal poison/burn, final stun turn, slow skip, and a live control.
// Displacement: push/transfer x fresh/restored deferred state. Extra TB passes
// must not tick clocks twice. Persistence also covers partial owned field ticks.
func telegraphContractGame(t *testing.T, tb bool) (*MMGame, *GameLoop, *monster.Monster3D) {
	t.Helper()
	g, gl := newSpecialsTestGame(t)
	g.gameLoop, g.turnBasedMode = gl, tb
	g.threading = threading.NewThreadingComponents(g.config)
	t.Cleanup(g.threading.Shutdown)
	placePlayerAtTile(g, 6, 6, 64)
	g.party.Members = g.party.Members[:1]
	hero := g.party.Members[0]
	isolateTrueDamageMember(hero, 0)
	hero.HitPoints, hero.MaxHitPoints = 10000, 10000
	m := spawnSpecialsMonster(g, "solstice_kiln_automaton", 8, 6)
	m.DamageMin, m.DamageMax = 100, 100
	m.HitPoints, m.MaxHitPoints = 10000, 10000
	m.Speed = 0
	m.Resistances = nil
	m.ArmorClass = 0
	m.State, m.StateTimer = monster.StateAttacking, 1
	return g, gl, m
}

func contractMonsterFrame(g *MMGame, gl *GameLoop) {
	g.frameCount++
	if g.turnBasedMode {
		g.currentTurn, g.monsterTurnResolved = 1, false
		g.monsterTurnState.resetPasses()
	}
	gl.runMonsterFrame()
}

func contractTelegraphDefinition(t *testing.T, m *monster.Monster3D) {
	t.Helper()
	def := monster.MonsterConfig.Monsters[m.Key]
	original := def
	attack := *def.Telegraph
	attack.DamagePercent = 100
	attack.SustainDamage = 100
	attack.DurationSeconds, attack.DurationRounds = 6, 2
	def.Telegraph = &attack
	monster.MonsterConfig.Monsters[m.Key] = def
	t.Cleanup(func() { monster.MonsterConfig.Monsters[m.Key] = original })
}

func TestTelegraphOwnedDamageContract(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, actor := range []bool{false, true} {
			for _, field := range []bool{false, true} {
				for _, weaken := range []bool{false, true} {
					for _, reflect := range []bool{false, true} {
						if actor && reflect {
							continue
						}
						t.Run(fmt.Sprintf("TB%v/actor%v/field%v/weaken%v/reflect%v", tb, actor, field, weaken, reflect), func(t *testing.T) {
							g, gl, m := telegraphContractGame(t, tb)
							contractTelegraphDefinition(t, m)
							m.TrueDamage = 20
							if !field {
								telegraphDefinition(m).SustainDamage = 0
							}
							var foe *monster.Monster3D
							if actor {
								placePlayerAtTile(g, 2, 2, 64)
								foe = spawnSpecialsMonster(g, "goblin", 6, 6)
								foe.Bound, foe.BoundFramesRemaining = true, 100000
								foe.HitPoints, foe.MaxHitPoints = 10000, 10000
								foe.Speed, foe.AttackCDFrames = 0, 100000
								foe.Resistances = nil
								foe.ArmorClass = 0
							}
							if reflect {
								vengefulNingyoThornsPct(t, g)
							}
							contractMonsterFrame(g, gl)
							if m.Telegraph.Warning <= 0 {
								t.Fatal("scheduler did not announce special")
							}
							if weaken {
								m.ApplyWeaken(50, 1000, 5)
							}
							duration := 1
							if field {
								m.Telegraph.Warning, m.Telegraph.Remaining = 0, 1
								if !tb {
									duration = g.config.GetTPS()
								}
							} else {
								m.Telegraph.Warning = 1e-6
							}
							// Keep ordinary attacks out of a lingering-field measurement.
							m.AttackCDFrames = 100000
							sourceHits := 0
							g.heroHitObserver = func(_ *character.MMCharacter, source string, damage int) {
								if source != m.Name {
									t.Errorf("source=%q want %q", source, m.Name)
								}
								sourceHits += damage
							}
							for range duration {
								contractMonsterFrame(g, gl)
							}
							want := 100
							if !field {
								want += m.TrueDamage
							}
							if field && tb {
								want *= 3
							}
							if weaken {
								want /= 2
							}
							got := 10000 - g.party.Members[0].HitPoints
							if actor {
								got = 10000 - foe.HitPoints
							}
							if got != want {
								t.Fatalf("damage=%d want %d", got, want)
							}
							if !actor && sourceHits != want {
								t.Fatalf("attributed damage=%d want %d", sourceHits, want)
							}
							if reflect {
								if got := 10000 - m.HitPoints; got != want*g.cardThornsPct()/100 {
									t.Fatalf("reflected=%d", got)
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestTelegraphStatusPrecedesRelease(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, condition := range []string{"alive", "burn", "poison", "stun", "slow-skip"} {
			if !tb && condition == "slow-skip" {
				continue
			}
			t.Run(fmt.Sprintf("TB%v/%s", tb, condition), func(t *testing.T) {
				g, gl, m := telegraphContractGame(t, tb)
				contractMonsterFrame(g, gl)
				if m.Telegraph.Warning <= 0 {
					t.Fatal("no warning")
				}
				m.Telegraph.Warning = 1e-6
				switch condition {
				case "burn":
					m.HitPoints = 1
					m.ApplyBurn(1000)
					m.RestoreDoTTickTimers(0, g.config.GetTPS()-1)
				case "poison":
					m.HitPoints = 1
					m.PoisonedFramesRemaining = 1000
					m.RestoreDoTTickTimers(g.config.GetTPS()-1, 0)
				case "stun":
					m.StunFramesRemaining, m.StunTurnsRemaining = 2, 1
				case "slow-skip":
					m.ApplySlow(25, 1000, 5)
					m.SlowTurnCount = config.SlowSkipEveryTurns() - 1
				}
				hits := 0
				g.heroHitObserver = func(_ *character.MMCharacter, _ string, damage int) { hits += damage }
				contractMonsterFrame(g, gl)
				if got := hits > 0; got != (condition == "alive") {
					t.Fatalf("special hit=%v under %s", got, condition)
				}
				if (condition == "burn" || condition == "poison") && m.IsAlive() {
					t.Fatal("lethal status did not tick")
				}
			})
		}
	}
}

func TestTelegraphTurnPositionAndPasses(t *testing.T) {
	for _, transfer := range []bool{false, true} {
		for _, restored := range []bool{false, true} {
			t.Run(fmt.Sprintf("transfer%v/restored%v", transfer, restored), func(t *testing.T) {
				g, gl, m := telegraphContractGame(t, true)
				m.Key = "solstice_gale_elemental"
				contractMonsterFrame(g, gl)
				if transfer {
					m.Telegraph.Warning = 0
					a := &config.AdventureConfig{Effects: []config.EnvironmentEffect{{ID: "transfer", Name: "Transfer", Kind: "transfer", School: "air", Rect: [4]int{6, 6, 6, 6}, Destination: [2]int{5, 6}, WarningSeconds: 1, WarningRounds: 1}}}
					world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest", MapConfigs: map[string]*config.MapConfig{"forest": {Adventure: a}}}
					g.adventureVisit("forest").effectState("transfer").Warning = 1
				}
				if restored {
					raw, err := json.Marshal(m.Telegraph)
					if err != nil {
						t.Fatal(err)
					}
					if err = json.Unmarshal(raw, &m.Telegraph); err != nil {
						t.Fatal(err)
					}
					g.adventure = cloneAdventureState(g.adventure)
				}
				goblin := spawnSpecialsMonster(g, "goblin", 7, 6)
				oldX, oldY := goblin.X, goblin.Y
				g.turnBasedExtraMonsterAction = true
				contractMonsterFrame(g, gl)
				if x, y := g.GetPlayerTilePosition(); x != 5 || y != 6 {
					t.Fatalf("party at %d,%d", x, y)
				}
				if goblin.X == oldX && goblin.Y == oldY {
					t.Fatal("following actor used stale party position")
				}
				cooldown := m.Telegraph.Cooldown
				g.turnBasedMonsterPassDelay = 0
				gl.runMonsterFrame()
				if m.Telegraph.Cooldown != cooldown {
					t.Fatal("extra pass ticked special clock twice")
				}
			})
		}
	}
}

func TestTelegraphFieldOwnershipSave(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	if err := g.transitionToMap(mapTransition{mapKey: "solstice_fire", arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	var m *monster.Monster3D
	for _, candidate := range g.world.Monsters {
		if candidate.Key == "solstice_ember_elemental" {
			m = candidate
			break
		}
	}
	if m == nil {
		t.Fatal("no caster")
	}
	g.world.Monsters = []*monster.Monster3D{m}
	g.turnBasedMode = false
	m.BeginPlayerEngagement()
	m.WasAttacked = true
	placePlayerAtTile(g, 4, 4, 64)
	m.Telegraph = monster.TelegraphState{Cooldown: 1, Remaining: 1, Tiles: [][2]int{{4, 4}}}
	g.refreshMonsterAIState()
	g.tickEnvironment(.25, false)
	state := g.adventureVisit("solstice_fire").effectState("continuous:fire:")
	if state.SourceDamageCarry[m.ID] <= 0 {
		t.Fatalf("missing partial source contribution: state=%+v key=%s allowed=%v party=%v mode=%v", state, currentMapKey(), g.telegraphAllowed(m), m.TargetsParty(), m.CurrentAIBehavior())
	}
	raw, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err = json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	if err = g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	restored := g.adventureVisit("solstice_fire").effectState("continuous:fire:")
	if restored.Carry != state.Carry || restored.SourceDamageCarry[m.ID] != state.SourceDamageCarry[m.ID] {
		t.Fatal("save lost ownership or cadence")
	}
	// Finish the owned pulse after restoration and check the shared hit observer.
	hits := 0
	g.heroHitObserver = func(_ *character.MMCharacter, source string, damage int) {
		if source == m.Name {
			hits += damage
		}
	}
	g.tickEnvironment(.75, false)
	if hits <= 0 {
		t.Fatal("restored field became unowned environmental damage")
	}
}

func TestTelegraphFieldOverlapAttribution(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"monster strongest", "terrain strongest", "two monsters", "owner changes"} {
			if tb && kind == "owner changes" {
				continue
			} // Changes within a second are an RT-only interval.
			t.Run(fmt.Sprintf("TB%v/%s", tb, kind), func(t *testing.T) {
				g, _, m := telegraphContractGame(t, tb)
				contractTelegraphDefinition(t, m)
				vengefulNingyoThornsPct(t, g)
				m.Telegraph = monster.TelegraphState{Cooldown: 1, Remaining: 1, Tiles: [][2]int{{6, 6}}}
				a := &config.AdventureConfig{}
				if kind == "monster strongest" || kind == "terrain strongest" {
					rate := 50
					if kind == "terrain strongest" {
						m.ApplyWeaken(50, 1000, 5)
						rate = 75
					}
					a.Effects = []config.EnvironmentEffect{{ID: "floor", Kind: "occupation", Name: "Floor", School: "fire", Damage: rate, Rect: [4]int{6, 6, 6, 6}}}
				}
				world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest", MapConfigs: map[string]*config.MapConfig{"forest": {Adventure: a}}}
				var other *monster.Monster3D
				if kind == "two monsters" || kind == "owner changes" {
					other = spawnSpecialsMonster(g, m.Key, 8, 7)
					other.HitPoints, other.MaxHitPoints = 10000, 10000
					other.Resistances = nil
					other.Telegraph = m.Telegraph.Clone()
					if kind == "owner changes" {
						other.Telegraph.Remaining = 0
					}
				}
				g.refreshMonsterAIState()
				attributed := map[string]int{}
				g.heroHitObserver = func(_ *character.MMCharacter, name string, damage int) { attributed[name] += damage }
				if kind == "owner changes" {
					g.tickEnvironment(.5, false)
					g.adventure = cloneAdventureState(g.adventure)
					m.Telegraph.Remaining = 0
					other.Telegraph.Remaining = 1
					g.tickEnvironment(.5, false)
				} else if tb {
					g.tickEnvironment(3, true)
				} else {
					g.tickEnvironment(1, false)
				}
				want := 100
				if kind == "terrain strongest" {
					want = 75
				}
				if tb {
					want *= 3
				}
				if loss := 10000 - g.party.Members[0].HitPoints; loss != want {
					t.Fatalf("overlap damage=%d want %d", loss, want)
				}
				if kind == "terrain strongest" {
					if len(attributed) != 0 || m.HitPoints != 10000 {
						t.Fatal("terrain inherited monster ownership")
					}
				} else if kind == "owner changes" {
					wantReflect := 50 * g.cardThornsPct() / 100
					if 10000-m.HitPoints != wantReflect || 10000-other.HitPoints != wantReflect {
						t.Fatal("interval damage assigned to the wrong owner")
					}
				} else {
					if 10000-m.HitPoints != want*g.cardThornsPct()/100 {
						t.Fatal("dominant source lost reflection")
					}
					if other != nil && other.HitPoints != 10000 {
						t.Fatal("suppressed source received reflection")
					}
				}
			})
		}
	}
}

func TestTelegraphFieldRetainsSchoolAtZeroRate(t *testing.T) {
	g, _, m := telegraphContractGame(t, false)
	contractTelegraphDefinition(t, m)
	m.Telegraph = monster.TelegraphState{Cooldown: 1, Remaining: 1, Tiles: [][2]int{{6, 6}}}
	g.combatBuffs = []TimedCombatBuff{{ResistSchool: "fire", ResistSchoolPct: 100}}
	g.refreshMonsterAIState()
	g.tickEnvironment(.5, false)
	m.ApplyWeaken(100, 1000, 5)
	g.tickEnvironment(.5, false)
	if g.party.Members[0].HitPoints != 10000 {
		t.Fatal("partial fire exposure lost its school when the current rate became zero")
	}
}

func TestTelegraphChargeObservesMovementOnce(t *testing.T) {
	g, gl, ch, tile := sniperFixture(t, false)
	g.gameLoop = gl
	g.threading = threading.NewThreadingComponents(g.config)
	t.Cleanup(g.threading.Shutdown)
	ch.MaxHitPoints, ch.HitPoints = 10000, 10000
	m := spawnMonsterAtTile(g, "solstice_cinder_hound", 7, 10, tile)
	m.BeginPlayerEngagement()
	m.Speed = 0
	m.State, m.StateTimer = monster.StateAttacking, 1
	contractMonsterFrame(g, gl)
	if m.Telegraph.Warning <= 0 {
		t.Fatal("charge did not start")
	}
	m.Telegraph.Warning = 1e-6
	m.Speed = 1
	rolls := 0
	g.combat.reactionRoll = func() float64 { rolls++; return 1 }
	contractMonsterFrame(g, gl)
	if rolls != 2 {
		t.Fatalf("reaction attempts=%d; want one movement and one attack observation", rolls)
	}
}

package game

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func lemurTestGame(t *testing.T, key string) (*MMGame, *monster.Monster3D, float64) {
	t.Helper()
	g, _, tile := ecologyTestGame(t)
	placePlayerAtTile(g, 9, 10, tile)
	tree, ok := world.GlobalTileManager.GetTileTypeFromKey("jungle_palm")
	if !ok {
		t.Fatal("missing jungle palm")
	}
	g.world.Tiles[10][12], g.world.Tiles[10][15] = tree, tree
	m := monster.NewMonster3DFromConfig(11.5*tile, 10.5*tile, key, g.config)
	g.world.Monsters = []*monster.Monster3D{m}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	return g, m, tile
}

func stepLemur(g *MMGame, m *monster.Monster3D, turn bool) {
	g.frameCount++
	g.prepareAmbientTarget(m)
	if turn {
		m.UpdateAmbient(g.collisionSystem, m.AITargetX, m.AITargetY, true)
		g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	} else {
		worker := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
		worker.Update()
		worker.ApplyCollisionUpdate()
	}
}

func TestLemurTraversalBothSpeciesAndModes(t *testing.T) {
	for _, key := range []string{"ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, turn := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB=%v", key, turn), func(t *testing.T) {
				g, m, tile := lemurTestGame(t, key)
				g.turnBasedMode = turn
				seen := map[string]bool{}
				maxHeight := 0.0
				landed := false
				for i := 0; i < 2400; i++ {
					stepLemur(g, m, turn)
					seen[m.Arbor.Phase] = true
					maxHeight = math.Max(maxHeight, m.Arbor.Height)
					if m.Arbor.Phase == "" && seen["descending"] {
						landed = true
						break
					}
				}
				for _, phase := range []string{"climbing", "perched", "jumping", "descending"} {
					if !seen[phase] {
						t.Errorf("did not traverse %s: %+v", phase, m.Arbor)
					}
				}
				if !landed || m.Arbor.Height != 0 || maxHeight <= m.Arboreal.HeightTiles || m.X < 15*tile {
					t.Fatalf("bad jump/landing: x=%.2f height=%.2f max=%.2f state=%+v", m.X/tile, m.Arbor.Height, maxHeight, m.Arbor)
				}
				if !g.collisionSystem.CanMoveTo(m.ID, m.X, m.Y) || m.IsEngagingPlayer || m.AIFoe != nil || m.Experience != 0 || m.Gold != 0 {
					t.Fatal("lemur landed blocked, became hostile, or has farming rewards")
				}
			})
		}
	}
}

func TestLemurMovementHoldsAndPersistence(t *testing.T) {
	for _, phase := range []string{"climbing", "perched", "jumping", "descending"} {
		t.Run(phase, func(t *testing.T) {
			g, m, _ := lemurTestGame(t, "ring_tailed_lemur")
			for i := 0; i < 1800 && m.Arbor.Phase != phase; i++ {
				stepLemur(g, m, false)
			}
			if m.Arbor.Phase != phase {
				t.Fatal("phase not reached")
			}
			before, x, y := m.Arbor, m.X, m.Y
			m.RootFramesRemaining = 30
			stepLemur(g, m, false)
			if m.Arbor != before || m.X != x || m.Y != y {
				t.Fatal("root advanced tree motion")
			}
			m.RootFramesRemaining = 0
			wm := world.GlobalWorldManager
			save := g.buildSave(wm)
			data, err := json.Marshal(save)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip GameSave
			if err = json.Unmarshal(data, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if err = g.applySave(wm, &roundTrip); err != nil {
				t.Fatal(err)
			}
			got := g.world.Monsters[0]
			if got.Arbor != before || got.X != x || got.Y != y {
				t.Fatalf("tree state lost on load: %+v -> %+v", before, got.Arbor)
			}
		})
	}
}

func TestLemurNoTreesAndPopulation(t *testing.T) {
	g, wm, tile := ecologyTestGame(t)
	wm.LoadedMaps["deep_jungle"] = g.world
	wm.CurrentMapKey = "deep_jungle"
	g.replenishWildlife()
	counts := map[string]int{}
	for _, m := range g.world.Monsters {
		counts[m.Key]++
	}
	if counts["ring_tailed_lemur"] != 10 || counts["red_ruffed_lemur"] != 10 {
		t.Fatal(counts)
	}
	m := monster.NewMonster3DFromConfig(11.5*tile, 10.5*tile, "ring_tailed_lemur", g.config)
	g.world.Monsters = []*monster.Monster3D{m}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	placePlayerAtTile(g, 9, 10, tile)
	x := m.X
	for i := 0; i < 30; i++ {
		stepLemur(g, m, false)
	}
	if m.X <= x || m.Arbor.Height != 0 || m.Arbor.Phase != "" {
		t.Fatalf("ground fleeing failed: %+v", m.Arbor)
	}
}

func TestLemurPrewarmAndDropContract(t *testing.T) {
	g, m, _ := lemurTestGame(t, "ring_tailed_lemur")
	resource := mapMonsterPrewarmResource{key: m.Key, spriteName: m.GetSpriteType()}
	requests := mapRenderSourceRequests(mapRenderPrewarmPlan{monsterDecode: []mapMonsterPrewarmResource{resource}})
	set := map[graphics.SpriteResourceRequest]bool{}
	for _, req := range requests {
		set[req] = true
	}
	for _, action := range arborealAnimations {
		if !set[graphics.SpriteResourceRequest{Name: m.GetSpriteType(), AnimationType: action + "_r"}] {
			t.Errorf("prewarm omitted %s", action)
		}
	}
	for _, key := range []string{"ring_tailed_lemur", "red_ruffed_lemur"} {
		drops := config.GlobalLoots.Loots[key]
		if len(drops) != 1 || drops[0].Key != "lemur_fur" || drops[0].Chance != .2 {
			t.Errorf("unexpected minimal drops: %+v", drops)
		}
	}
	c := &monsterCorpse{arborealHeight: 1.2, sizeTiles: .35, started: g.frameCount}
	ground, size := float64(g.config.GetScreenHeight())/2+50, 35.0
	if got := g.corpseBottom(c, ground, size); math.Abs(got-(ground-120)) > .01 {
		t.Fatalf("canopy death starts on floor: %.2f", got)
	}
	g.frameCount += int64(g.config.GetTPS()) * 3
	if got := g.corpseBottom(c, ground, size); got != ground {
		t.Fatalf("canopy corpse did not fall: %.2f", got)
	}
}

func TestLemurTerrainAndStatusRestrictions(t *testing.T) {
	for _, scenario := range []string{"wall", "blocked_jump", "region_bound", "removed_tree", "root_RT", "root_TB", "slow_RT", "slow_TB"} {
		t.Run(scenario, func(t *testing.T) {
			g, m, tile := lemurTestGame(t, "ring_tailed_lemur")
			if scenario == "wall" {
				g.world.Tiles[10][12] = world.TileWall
				stepLemur(g, m, false)
				if m.Arbor.Phase != "" {
					t.Fatal("climbed an ordinary wall")
				}
				return
			}
			for i := 0; i < 300 && m.Arbor.Phase != "perched"; i++ {
				stepLemur(g, m, false)
			}
			if m.Arbor.Phase != "perched" {
				t.Fatal("not perched")
			}
			switch scenario {
			case "blocked_jump":
				g.world.Tiles[10][14] = world.TileWall
			case "region_bound":
				m.AmbientBounds = &[4]int{0, 0, 14, 30}
			case "removed_tree":
				g.world.Tiles[10][12] = g.world.Tiles[9][12]
			case "root_RT", "root_TB":
				m.RootFramesRemaining, m.RootTurnsRemaining = 60, 2
			case "slow_RT", "slow_TB":
				m.ApplySlow(100, 60, 2)
			}
			before, x, y := m.Arbor, m.X, m.Y
			turn := scenario == "root_TB" || scenario == "slow_TB"
			stepLemur(g, m, turn)
			if scenario == "blocked_jump" || scenario == "region_bound" || scenario == "removed_tree" {
				if m.Arbor.Phase != "descending" || m.X >= 14*tile {
					t.Fatalf("unsafe canopy route: %+v", m.Arbor)
				}
			} else if m.Arbor != before || m.X != x || m.Y != y {
				t.Fatal("status hold advanced canopy motion")
			}
		})
	}
}

func TestLemurInterruptedJumpFindsReachableLanding(t *testing.T) {
	g, m, tile := lemurTestGame(t, "red_ruffed_lemur")
	for i := 0; i < 400 && m.Arbor.Phase != "jumping"; i++ {
		stepLemur(g, m, false)
	}
	if m.Arbor.Phase != "jumping" {
		t.Fatal("jump not reached")
	}
	g.world.Tiles[10][14] = world.TileWall
	for i := 0; i < 400 && m.Arbor.Phase != ""; i++ {
		stepLemur(g, m, false)
		if m.X >= 14*tile {
			t.Fatal("crossed the new obstacle")
		}
	}
	if m.Arbor.Phase != "" || m.Arbor.Height != 0 || !g.collisionSystem.CanMoveTo(m.ID, m.X, m.Y) {
		t.Fatalf("failed to land beside the interrupted jump: %+v", m.Arbor)
	}
}

func TestLemurPartialSlowProgression(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", turn), func(t *testing.T) {
			g, m, _ := lemurTestGame(t, "ring_tailed_lemur")
			m.Speed = config.GlobalEcology.TurnStepSpeed
			stepLemur(g, m, false)
			before, x, y := m.Arbor, m.X, m.Y
			m.AmbientMoveCredit = 0
			stepLemur(g, m, turn)
			normal := m.Arbor.Progress - before.Progress
			m.Arbor, m.X, m.Y, m.AmbientMoveCredit = before, x, y, 0
			m.ApplySlow(50, 100, 100)
			stepLemur(g, m, turn)
			stepLemur(g, m, turn)
			if got := m.Arbor.Progress - before.Progress; math.Abs(got-normal) > 1e-9 {
				t.Fatalf("50 percent Slow should take two updates: normal=%f slow=%f", normal, got)
			}
		})
	}
}

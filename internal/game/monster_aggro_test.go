package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// Drive both live schedulers; changing mode cannot extend the same pursuit.
func TestPursuitLimitsAcrossCombatModes(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, turn := range []bool{false, true} {
		for _, tc := range []struct {
			name                                    string
			radius, distance                        float64
			hit, boss, guard, hunter, stunned, want bool
		}{
			{name: "sight edge", radius: 4, distance: 8, want: true},
			{name: "sight lost", radius: 4, distance: 9},
			{name: "hit edge", radius: 4, distance: 16, hit: true, want: true},
			{name: "hit lost", radius: 4, distance: 17, hit: true},
			{name: "stunned hit lost", radius: 4, distance: 17, hit: true, stunned: true},
			{name: "boss sight edge", radius: 14, distance: 16, boss: true, want: true},
			{name: "boss sight lost", radius: 14, distance: 17, boss: true},
			{name: "boss hit lost", radius: 14, distance: 17, boss: true, hit: true},
			{name: "chamber sight edge", radius: 3.2, distance: 7, boss: true, want: true},
			{name: "chamber sight lost", radius: 3.2, distance: 8, boss: true},
			{name: "guard edge", radius: 4, distance: 7, guard: true, want: true},
			{name: "guard lost", radius: 4, distance: 8, guard: true},
			{name: "provoked guard", radius: 4, distance: 12, guard: true, hit: true, want: true},
			{name: "remote map hunter", radius: 4, distance: 30, boss: true, hunter: true, want: true},
		} {
			t.Run(fmt.Sprintf("TB=%v/%s", turn, tc.name), func(t *testing.T) {
				g := newTestGame(cfg, newTestWorldSized(cfg, 42, 24))
				g.combat = NewCombatSystem(g)
				gl := &GameLoop{game: g}
				ts := float64(cfg.GetTileSize())
				placePlayerAtTile(g, 3, 10, ts)
				m := spawnMonsterAtTile(g, "goblin", 3+int(tc.distance), 10, ts)
				m.AlertRadius, m.WasAttacked = tc.radius*ts, false
				m.Boss, m.AggroWholeMap, m.LootGuardAlerted = tc.boss, tc.hunter, tc.guard
				m.SpawnX += 2 * ts
				m.State = monster.StateAlert
				if tc.hit {
					m.TakeDamageParts(damagecalc.Parts{True: 1}, "true", 0)
				}
				if tc.stunned {
					m.StunTurnsRemaining, m.StunFramesRemaining = 5, 600
				}
				hp, startX := m.HitPoints, m.X
				step := func(tb bool) {
					g.turnBasedMode = tb
					g.refreshMonsterAIState()
					if tb {
						g.currentTurn, g.monsterTurnResolved = 1, false
						gl.updateMonstersTurnBased()
					} else {
						runLootGuardRealTimeStep(g, gl)
					}
				}
				step(turn)
				if m.IsEngagingPlayer != tc.want || m.TargetsParty() != tc.want {
					t.Fatalf("engaged=%v target=%v want=%v", m.IsEngagingPlayer, m.TargetsParty(), tc.want)
				}
				if m.WasAttacked != tc.hit || m.HitPoints != hp {
					t.Fatal("disengagement changed memory or HP")
				}
				if !tc.want && !tc.stunned && m.X <= startX {
					t.Fatal("disengaged monster did not walk toward home")
				}
				step(!turn)
				if m.IsEngagingPlayer != tc.want {
					t.Fatal("mode switch changed pursuit scope")
				}
				if tc.hunter && g.combatAuraHostile(m) {
					t.Fatal("remote hunter expanded the local overlay")
				}
			})
		}
	}
}

// A fresh party hit extends reach, not permanent hostility. Exercise actual hit
// bookkeeping and both schedulers, including edited YAML values and mode swaps.
func TestPursuitTemporaryRetaliation(t *testing.T) {
	cfg := loadTestConfig(t)
	cfg.MonsterAI.Pursuit.MaxRadiusTiles = 9
	cfg.MonsterAI.Pursuit.RetaliationSeconds = .1
	cfg.MonsterAI.Pursuit.RetaliationTurns = 3
	for _, tb := range []bool{false, true} {
		for _, boss := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/boss=%v", tb, boss), func(t *testing.T) {
				g := newTestGame(cfg, newTestWorldSized(cfg, 32, 20))
				g.combat = NewCombatSystem(g)
				gl := &GameLoop{game: g}
				g.gameLoop = gl
				ts := cfg.GetTileSize()
				placePlayerAtTile(g, 3, 10, ts)
				m := monster.NewMonster3DFromConfig(23.5*ts, 10.5*ts, "goblin", cfg)
				m.Boss = boss
				m.SpawnX += 2 * ts
				g.registerSpawnedMonster(m)
				g.turnBasedMode = tb
				g.combat.ApplyDamageToMonster(m, 1, "Test", false)
				if !m.TargetsParty() || m.Retaliation.Frames != int(.1*float64(cfg.GetTPS())) || m.Retaliation.RadiusTiles != 20+cfg.MonsterAI.Pursuit.RetaliationMarginTiles {
					t.Fatal("party hit did not grant authored temporary retaliation from its actual range")
				}
				hp, startX := m.HitPoints, m.X
				step := func(turn bool) {
					g.turnBasedMode = turn
					g.refreshMonsterAIState()
					if turn {
						g.currentTurn, g.monsterTurnResolved = 1, false
						gl.updateMonstersTurnBased()
					} else {
						runLootGuardRealTimeStep(g, gl)
					}
				}
				step(tb)
				if !tb {
					step(false)
				} // The RT alert phase precedes the first movement step.
				if m.X >= startX {
					t.Fatal("retaliating monster did not advance from twenty tiles")
				}
				m.Speed = 0 // Pin distance while checking exact timer expiry.
				if !m.TargetsParty() {
					t.Fatal("fresh retaliation was cut off by the normal leash")
				}
				remaining := m.Retaliation.Frames
				step(!tb)
				if m.Retaliation.Frames >= remaining {
					t.Fatal("mode switch refunded retaliation time")
				}
				for i := 0; i < cfg.GetTPS() && m.Retaliation.Frames > 0; i++ {
					step(!tb)
				}
				if m.TargetsParty() || m.IsEngagingPlayer || !m.ReturningHome || !m.WasAttacked || m.HitPoints != hp || m.PursuitLeashPixels() != 9*ts {
					t.Fatalf("expiry: target=%v engaged=%v returning=%v memory=%v HP=%d/%d leash=%v", m.TargetsParty(), m.IsEngagingPlayer, m.ReturningHome, m.WasAttacked, m.HitPoints, hp, m.PursuitLeashPixels()/ts)
				}
				m.AttackCDFrames, m.OffHandCDFrames, m.PounceCDFrames, m.PounceCDTurns, m.PounceCDRate = 2, 3, 4, 2, 2
				m.BossCD, m.InfernoCDFrames, m.TrapVolleyCDFrames, m.TrapVolleyTurnCD, m.TrapVolleyCDRate = 2, 2, 4, 2, 2
				g.combat.HandleMonsterInteractions()
				if m.AttackCDFrames != 1 || m.OffHandCDFrames != 2 || m.PounceCDFrames != 3 {
					t.Fatal("calm monster froze its attack recovery")
				}
				if boss && (m.BossCD != 1 || m.InfernoCDFrames != 1 || m.TrapVolleyCDFrames != 3 || len(g.bossFireTraps) != 0) {
					t.Fatal("calm boss recovery must tick without releasing an action")
				}
			})
		}
	}
}

// Use the real stitcher so crossing a biome exercises rotated/local map identity,
// not a stand-in boolean. Returning to the region wakes only map hunters at range.
func TestPursuitRegionExitAndReentry(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	oldTM, oldWM := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager, world.GlobalWorldManager = oldTM, oldWM })
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatal(err)
	}
	wm.MapConfigs = map[string]*config.MapConfig{"forest": wm.MapConfigs["forest"], "highlands": wm.MapConfigs["highlands"]}
	wm.SetOpenWorldConfig(&config.OpenWorldConfig{VoidTile: "oob_cliff", Placements: map[string]config.OpenWorldPlacement{
		"forest": {X: 0, Y: 0}, "highlands": {X: 60, Y: 0, Orient: "rot270"},
	}})
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatal(err)
	}
	world.GlobalWorldManager = wm
	g := newTestGame(cfg, wm.OpenWorld)
	g.combat = NewCombatSystem(g)
	ts := float64(cfg.GetTileSize())
	x, y := wm.ProjectWorldPos("forest", 10.5*ts, 10.5*ts)
	hunter := monster.NewMonster3DFromConfig(x, y, "goblin", cfg)
	hunter.HomeMap, hunter.Relentless = "forest", true
	normal := monster.NewMonster3DFromConfig(x, y, "goblin", cfg)
	normal.HomeMap, normal.WasAttacked = "forest", true
	g.world.Monsters = []*monster.Monster3D{hunter, normal}
	g.camera.X, g.camera.Y = x+ts, y
	wm.CurrentMapKey = "forest"
	hunter.BeginPlayerEngagement()
	normal.BeginPlayerEngagement()
	hunter.Telegraph.Warning = 2
	g.camera.X, g.camera.Y = wm.ProjectWorldPos("highlands", 10.5*ts, 10.5*ts)
	g.syncOpenWorldRegion()
	for _, m := range g.world.Monsters {
		if m.TargetsParty() || m.IsEngagingPlayer || !m.ReturningHome {
			t.Fatal("region exit retained pursuit")
		}
	}
	if hunter.Telegraph.Warning != 0 || !hunter.Relentless || !normal.WasAttacked {
		t.Fatal("exit failed to cancel windup or erased hostility memory")
	}
	g.camera.X, g.camera.Y = wm.ProjectWorldPos("forest", 40.5*ts, 10.5*ts)
	g.syncOpenWorldRegion()
	g.refreshMonsterAIState()
	if !hunter.TargetsParty() || normal.TargetsParty() || g.combatAuraHostile(hunter) {
		t.Fatal("reentry did not separate map hunt from local combat")
	}
	// A seam crossing and attack can share one input frame, before region sync.
	g.camera.X, g.camera.Y = wm.ProjectWorldPos("highlands", 10.5*ts, 10.5*ts)
	g.syncOpenWorldRegion()
	g.camera.X, g.camera.Y = x+10*ts, y
	g.combat.applyMonsterDamagePacket(normal, singleMonsterDamagePacket(damagecalc.Parts{True: 1}, "true", 0), monsterDamageOptions{})
	if !normal.TargetsParty() || normal.PartyOutsideHome() {
		t.Fatal("hit on reentry used stale region scope")
	}
	// Move the party into another biome without running region/AI synchronization.
	// This nearby, strayed forest monster still belongs to its original biome.
	g.camera.X, g.camera.Y = wm.ProjectWorldPos("highlands", 10.5*ts, 10.5*ts)
	normal.X, normal.Y = g.camera.X-ts, g.camera.Y
	normal.EndPlayerEngagement()
	if !normal.PartyOutsideHome() {
		t.Fatal("scope query retained a cached party biome")
	}
	g.combat.ApplyDamageToMonster(normal, 1, "Test", false)
	if !normal.TargetsParty() {
		t.Fatal("cross-biome party attack received no retaliation")
	}
	for i := 0; i < cfg.MonsterAI.Pursuit.RetaliationTurns; i++ {
		g.tickMonsterTurnStatuses(normal, true)
	}
	normal.LimitPlayerEngagement(g.camera.X, g.camera.Y)
	if normal.TargetsParty() || normal.IsEngagingPlayer {
		t.Fatal("cross-biome retaliation outlived its timer")
	}
	// A separate physical world never inherits the temporary region exception.
	g.combat.ApplyDamageToMonster(normal, 1, "Test", false)
	originalWorld := g.world
	g.world = newTestWorldSized(cfg, 8, 8)
	if normal.TargetsParty() {
		t.Fatal("retaliation followed the party into another physical world")
	}
	endWorldPursuit(originalWorld)
	g.world = originalWorld
	if normal.Retaliation.Frames != 0 {
		t.Fatal("map departure kept temporary retaliation")
	}
	g.camera.X, g.camera.Y = x+ts, y
	// Revenge is scoped to the deceased boss's home, not the stitched world.
	boss := monster.NewMonster3DFromConfig(x, y, "orc_hero_boss", cfg)
	boss.HomeMap = "forest"
	local := monster.NewMonster3DFromConfig(x, y, "masked_huntress", cfg)
	local.HomeMap = "forest"
	remote := monster.NewMonster3DFromConfig(x, y, "masked_huntress", cfg)
	remote.HomeMap = "highlands"
	g.world.Monsters = []*monster.Monster3D{local, remote}
	g.combat.rallyOnPatronDeath(boss)
	if !local.Relentless || remote.Relentless {
		t.Fatal("revenge escaped the boss's home region")
	}
}

func TestPursuitSaveMemoryAndLegacyMigration(t *testing.T) {
	base := loadTestConfig(t)
	oldWM := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = oldWM })
	for _, state := range []string{"returning", "retaliating", "shortened"} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy=%v", state, legacy), func(t *testing.T) {
				cfgCopy := *base
				cfg := &cfgCopy
				wm := world.NewWorldManager(cfg)
				w := newTestWorldSized(cfg, 40, 24)
				wm.LoadedMaps, wm.CurrentMapKey = map[string]*world.World3D{"forest": w}, "forest"
				world.GlobalWorldManager = wm
				g := newTestGame(cfg, w)
				g.combat = NewCombatSystem(g)
				ts := cfg.GetTileSize()
				placePlayerAtTile(g, 3, 10, ts)
				m := spawnMonsterAtTile(g, "goblin", 30, 10, ts)
				g.stampMonsterHome(w, m)
				m.HomeMap = "forest"
				m.SpawnX += 2 * ts
				m.EndPlayerEngagement()
				if state != "returning" {
					g.combat.ApplyDamageToMonster(m, 1, "Test", false)
					m.TickRetaliation(false)
				}
				frames, hp, pending := m.Retaliation.Frames, m.HitPoints, m.BandHitPending
				s := g.buildSave(wm)
				if legacy {
					s.MapMonsters["forest"][0].Aggro = nil
				}
				data, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				var restored GameSave
				if err := json.Unmarshal(data, &restored); err != nil {
					t.Fatal(err)
				}
				if state == "shortened" {
					cfg.MonsterAI.Pursuit.RetaliationSeconds = 1
					frames = cfg.GetTPS()
				}
				if err := g.applySave(wm, &restored); err != nil {
					t.Fatal(err)
				}
				m = g.world.Monsters[0]
				active := !legacy && state != "returning"
				if m.TargetsParty() != active || m.IsEngagingPlayer != active || !m.WasAttacked || m.ReturningHome == active || m.HitPoints != hp {
					t.Fatal("save failed to distinguish active retaliation from provocation memory or lost wounds")
				}
				if active && (m.Retaliation.Frames != frames || m.BandHitPending != pending) {
					t.Fatal("save lost/clamped retaliation or the unconsumed fresh hit incorrectly")
				}
				if g.combatAuraHostile(m) {
					t.Fatal("restored remote monster expanded overlay")
				}
			})
		}
	}
}

// Reconciliation runs before the first input after load. It must not wake an
// evasive map hunter before its quest is complete, or forget an unlocked hunt.
func TestPursuitMapHunterQuestRestore(t *testing.T) {
	cfg := loadTestConfig(t)
	oldWM := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = oldWM })
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprintf("completed=%v", completed), func(t *testing.T) {
			wm := world.NewWorldManager(cfg)
			w := newTestWorldSized(cfg, 40, 24)
			wm.LoadedMaps, wm.CurrentMapKey = map[string]*world.World3D{"culverts": w}, "culverts"
			world.GlobalWorldManager = wm
			g := newTestGame(cfg, w)
			g.combat = NewCombatSystem(g)
			ts := float64(cfg.GetTileSize())
			placePlayerAtTile(g, 3, 10, ts)
			m := spawnMonsterAtTile(g, "golden_thief_bug", 30, 10, ts)
			m.HomeMap = "culverts"
			s := g.buildSave(wm)
			status := quests.QuestStatusActive
			if completed {
				status = quests.QuestStatusCompleted
			}
			s.Quests = []QuestSave{{ID: m.PassiveUntilQuest, Status: string(status)}}
			if err := g.applySave(wm, &s); err != nil {
				t.Fatal(err)
			}
			m = g.world.Monsters[0]
			if m.BossEvasive == completed || m.BossDormant || m.TargetsParty() != completed {
				t.Fatal("first-frame pursuit ignored the hunter's saved quest gate")
			}
			if g.combatAuraHostile(m) {
				t.Fatal("distant map hunter expanded the local overlay after load")
			}
		})
	}
}

package game

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/collision"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestCombatOverlaySettings(t *testing.T) {
	forEachUIFont(t, func(t *testing.T) {
		for _, size := range [][2]int{{907, 680}, {1024, 768}, {1920, 1080}} {
			for _, entry := range []bool{false, true} {
				r, b := settingsLayoutBoxes(size[0], size[1], entry, settingsTabCombat)
				assertNoCollisions(t, fmt.Sprintf("combat/%dx%d/entry=%v", size[0], size[1], entry), r, b)
				layout := makeAudioSettingsPanelLayout(size[0], size[1], entry)
				for _, wide := range []bool{false, true} {
					card := viewChoiceRect(layout.px, layout.py, layout.panelW, wide)
					var textBoxes []uiBox
					for i, text := range viewChoiceTextLayout(card, wide) {
						textBoxes = append(textBoxes, textLineBox(fmt.Sprint(i), text.label, text.box.x, text.box.y))
					}
					assertNoCollisions(t, "view choice", namedLayoutBox("view choice", card), textBoxes)
				}
			}
		}
	})
	for _, entry := range []bool{false, true} {
		t.Run(fmt.Sprintf("entry=%v", entry), func(t *testing.T) {
			h, _ := audioGestureHarness(t, entry, 0)
			g, fp := h.g, installFakePointer(t)
			g.loadCombatPreferences()
			if g.combatPreferences.TurnBasedOverlay {
				t.Fatal("missing preference must default to off")
			}
			layout := makeAudioSettingsPanelLayout(1024, 768, entry)
			click := func(r layoutRect) {
				presentInputScreen(h)
				fp.moveTo(r.x+r.w/2, r.y+r.h/2)
				fp.press()
				updateInputScreen(h)
				fp.release()
				updateInputScreen(h)
			}
			check := func(want bool) {
				t.Helper()
				restored := &MMGame{}
				restored.loadCombatPreferences()
				if g.combatPreferences.TurnBasedOverlay != want || restored.combatPreferences.TurnBasedOverlay != want || g.combatSettingsDirty {
					t.Fatalf("overlay preference runtime=%v saved=%v dirty=%v, want %v", g.combatPreferences, restored.combatPreferences, g.combatSettingsDirty, want)
				}
			}
			click(settingsTabRect(layout.px, layout.py, layout.panelW, settingsTabCombat))
			click(combatOverlayChoiceRect(layout.px, layout.py, layout.panelW, true))
			check(true)
			for _, key := range []ebiten.Key{ebiten.KeyLeft, ebiten.KeyRight} {
				presentInputScreen(h)
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == key })
				updateInputScreen(h)
				check(key == ebiten.KeyRight)
			}
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(ebiten.Key) bool { return false })
			click(combatOverlayChoiceRect(layout.px, layout.py, layout.panelW, false))
			check(false)
			click(viewChoiceRect(layout.px, layout.py, layout.panelW, true))
			restored := &MMGame{}
			restored.loadCombatPreferences()
			if !g.combatPreferences.WideView || !restored.combatPreferences.WideView {
				t.Fatal("wide view choice was not saved")
			}
			if g.audioSettingsSelection != gameplayRowView {
				t.Fatal("clicking a view option did not select its keyboard row")
			}
			presentInputScreen(h)
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyLeft })
			updateInputScreen(h)
			restored.loadCombatPreferences()
			if g.combatPreferences.WideView || restored.combatPreferences.WideView {
				t.Fatal("keyboard did not select and save Classic")
			}
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(ebiten.Key) bool { return false })
			if err := os.WriteFile(combatPreferencesPath(), []byte(`{"turn_based_overlay":`), 0600); err != nil {
				t.Fatal(err)
			}
			g.combatPreferences.TurnBasedOverlay = true
			g.loadCombatPreferences()
			if g.combatPreferences.TurnBasedOverlay {
				t.Fatal("corrupt preference must fall back to off")
			}
		})
	}
}

// This checks gameplay information, rather than cosmetic pixels: mode gating,
// threat topology, movement constraints and warning priority through collection.
func TestCombatOverlayInformation(t *testing.T) {
	g, _, ts := tbBehaviorGame(t, 24, 24)
	placePlayerAtTile(g, 3, 8, ts)
	g.camera.Angle, g.camera.FOV, g.camera.ViewDist = 0, math.Pi/2, 20*ts
	g.renderHelper = NewRenderingHelper(g)
	g.world.StartX, g.world.StartY = -1, -1
	m := spawnMonsterAtTile(g, "bandit", 8, 8, ts)
	m.RangedAttackRange = 4 * ts
	r := &Renderer{game: g}
	collect := func() { r.collectTileCurtains(nil) }
	collect()
	if len(r.auraCurtainEdges) != 0 {
		t.Fatal("default-off overlay drew curtains")
	}
	g.combatPreferences.TurnBasedOverlay = true
	collect()
	if len(r.auraCurtainEdges) == 0 {
		t.Fatal("engaged TB battle has no overlay")
	}
	for _, edge := range r.auraCurtainEdges {
		if math.Abs(edge.riseFraction-auraRiseFraction/3) > 1e-9 {
			t.Fatal("combat curtain is not one-third normal height")
		}
	}
	if r.combatAura.edges[combatAuraEdge{10, 8, false}] != combatAuraAttack {
		t.Fatal("ranged firing lane is not outlined red")
	}
	if r.combatAura.edges[combatAuraEdge{10, 10, false}] == combatAuraAttack {
		t.Fatal("distant diagonal incorrectly shown as a firing lane")
	}
	if r.combatAura.edges[combatAuraEdge{9, 8, true}] == combatAuraAttack {
		t.Fatal("attack interior has a red grid edge")
	}
	if r.combatAura.edges[combatAuraEdge{5, 6, false}] != combatAuraGrid || r.combatAura.edges[combatAuraEdge{3, 16, false}] != 0 {
		t.Fatal("battle grid does not follow party-to-enemy distance plus movement reach")
	}
	// A normal attack cannot shoot through a wall; movement cannot pass it.
	oldTile := g.world.Tiles[8][9]
	g.world.Tiles[8][9] = world.TileWall
	r.precomputeFloorColorCache()
	collect()
	if r.combatAura.edges[combatAuraEdge{10, 8, false}] == combatAuraAttack {
		t.Fatal("ranged outline passes through a wall")
	}
	g.combatAuraMovement(&r.combatAura, m, 2, ts)
	if r.combatAura.tiles[[2]int{9, 8}] || r.combatAura.tiles[[2]int{10, 8}] {
		t.Fatal("movement preview passes through a wall")
	}
	if !r.combatAura.tiles[[2]int{8, 6}] {
		t.Fatal("extra action did not extend movement to two steps")
	}
	m.RootTurnsRemaining = 1
	collect()
	for _, style := range r.combatAura.edges {
		if style == combatAuraMove {
			t.Fatal("rooted monster has a movement outline")
		}
	}
	m.RootTurnsRemaining = 0
	g.world.Tiles[8][9] = oldTile
	r.precomputeFloorColorCache()
	// Reusing the cached footprint must still observe a blocker being added,
	// moved or removed, and a change of the attacker's live reach.
	checkLane := func(want bool) {
		t.Helper()
		collect()
		if got := r.combatAura.edges[combatAuraEdge{10, 8, false}] == combatAuraAttack; got != want {
			t.Fatalf("cached firing lane=%v want %v", got, want)
		}
	}
	checkLane(true)
	g.collisionSystem.RegisterEntity(collision.NewSightBlockingEntity("preview-door", 9.5*ts, 8.5*ts, ts, ts, collision.CollisionTypeNPC, true))
	checkLane(false)
	g.collisionSystem.UpdateEntity("preview-door", 9.5*ts, 11.5*ts)
	checkLane(true)
	g.collisionSystem.UpdateEntity("preview-door", 9.5*ts, 8.5*ts)
	checkLane(false)
	g.collisionSystem.UnregisterEntity("preview-door")
	checkLane(true)
	m.RangedAttackRange = ts
	checkLane(false)
	m.RangedAttackRange = 4 * ts
	checkLane(true)
	// Non-aurora effects on the same tile must keep the tactical edges.
	x, y := TileCenterFromTile(5, 8, ts)
	g.persistentDamageZones = []PersistentDamageZone{
		{SpellID: "firewall", X: x, Y: y, Radius: ts * .55, FramesLeft: 100, AxisY: 1},
		{SpellID: "hot_steam", X: x, Y: y, Radius: ts * .55, FramesLeft: 100},
	}
	g.bossFireTraps = []bossFireTrap{{TX: 5, TY: 8}}
	g.traps = []PlacedTrap{{Key: "bear_trap", MapKey: currentMapKey(), TileX: 5, TileY: 8, FramesLeft: 100}}
	r.teleporterTiles = []teleporterTileFx{{tx: 5, ty: 8}}
	sprites := r.collectPersistentDamageZoneEffects(nil)
	sprites = r.collectBossFireTrapBorders(sprites)
	r.collectTileCurtains(sprites)
	for _, edge := range combatTileEdges(5, 8) {
		if r.combatAura.edges[edge] == 0 {
			t.Fatal("non-aurora effect suppressed the combat grid")
		}
	}
	g.persistentDamageZones, g.bossFireTraps, g.traps = nil, nil, nil
	r.teleporterTiles, r.zoneVisuals = nil, nil
	// Existing warnings must replace tactical edges, not blend with them.
	m.Telegraph = monster.TelegraphState{Warning: 1, Tiles: [][2]int{{5, 8}}}
	collect()
	for _, edge := range combatTileEdges(5, 8) {
		if r.combatAura.priorityStrength(edge, .5) != 1 {
			t.Fatal("combat grid obscures an existing telegraph")
		}
	}
	m.Telegraph = monster.TelegraphState{}
	for _, state := range []string{"real_time", "dead", "pacified", "calm", "sealed", "bound"} {
		t.Run(state, func(t *testing.T) {
			g.turnBasedMode = true
			m.HitPoints = 100
			m.Pacified, m.BossDormant, m.Bound = false, false, false
			m.IsEngagingPlayer, m.WasAttacked = true, true
			switch state {
			case "real_time":
				g.turnBasedMode = false
			case "dead":
				m.HitPoints = 0
			case "pacified":
				m.Pacified = true
			case "calm":
				m.IsEngagingPlayer, m.WasAttacked = false, false
				m.State = monster.StateIdle
			case "sealed":
				m.BossDormant = true
			case "bound":
				m.Bound = true
			}
			collect()
			for _, edge := range r.auraCurtainEdges {
				if edge.riseFraction == combatAuraRiseFraction {
					t.Fatal("tactical overlay remained without a hostile TB battle")
				}
			}
		})
	}
}

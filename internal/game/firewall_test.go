package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Firewall is a 3x1 wall of zone cells laid ACROSS the party's facing, two tiles
// ahead, ticking three times per turn for its mastery-authored damage. This pins
// the authored contract (user spec) and the placement geometry.
func TestFirewall_LaysThreeCellsAcrossTheFacing(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.world = newTestWorldSized(g.config, 20, 20)
	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = 5.5*ts, 5.5*ts
	g.camera.Angle = 0 // facing +X

	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall definition: %v", err)
	}
	if def.ZoneWidthTiles != 3 || def.ZoneAheadTiles != 2 {
		t.Fatalf("authored placement changed: width=%d ahead=%.1f", def.ZoneWidthTiles, def.ZoneAheadTiles)
	}
	if got := def.DamageByMastery; len(got) != 4 || got[0] != 15 || got[3] != 60 {
		t.Fatalf("authored damage ladder changed: %v", got)
	}

	g.steamZones = g.steamZones[:0]
	if !cs.tryCastSteamZone(spells.SpellID("firewall"), def, g.party.Members[0]) {
		t.Fatal("firewall must be handled by the zone path")
	}
	if len(g.steamZones) != 3 {
		t.Fatalf("expected 3 wall cells, got %d", len(g.steamZones))
	}
	tps := g.config.GetTPS()
	for i := range g.steamZones {
		z := &g.steamZones[i]
		// Every cell sits 2 tiles downrange (x), spread along y (across the facing).
		if dx := z.X - g.camera.X; math.Abs(dx-2*ts) > 1 {
			t.Errorf("cell %d is %.0fpx downrange, want %.0f", i, dx, 2*ts)
		}
		// One tick per second: three ticks inside one three-second TB turn.
		if z.IntervalFrames != tps {
			t.Errorf("cell %d tick interval = %d frames, want %d (1s)", i, z.IntervalFrames, tps)
		}
		if want := 30 * tps; z.FramesLeft != want {
			t.Errorf("cell %d lifetime = %d frames, want %d (30s = 10 turns)", i, z.FramesLeft, want)
		}
		if z.TickDamage != 15 {
			t.Errorf("cell %d tick damage = %d, want 15 at Novice", i, z.TickDamage)
		}
	}
	// Cells must be distinct positions across the facing, not stacked.
	if g.steamZones[0].Y == g.steamZones[1].Y || g.steamZones[1].Y == g.steamZones[2].Y {
		t.Error("wall cells are stacked instead of spread across the facing")
	}
}

// Jump lands the party JumpTiles ahead, and refuses (without moving them) when
// the landing tile is blocked.
func TestJump_MovesPartyForwardOrRefuses(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.world = newTestWorldSized(g.config, 20, 20)
	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = 5.5*ts, 5.5*ts
	g.camera.Angle = 0

	def, err := spells.GetSpellDefinitionByID(spells.SpellID("jump"))
	if err != nil {
		t.Fatalf("jump definition: %v", err)
	}
	if def.JumpTiles != 2 {
		t.Fatalf("authored jump distance changed: %.1f", def.JumpTiles)
	}
	caster := g.party.Members[0]
	startX := g.camera.X
	if !cs.tryCastJump(def, caster) {
		t.Fatal("jump must be handled")
	}
	if math.Abs(g.camera.X-(startX+2*ts)) > 1 {
		t.Errorf("party at %.0f, want %.0f (two tiles ahead)", g.camera.X, startX+2*ts)
	}

	// Blocked landing: the party holds position AND gets its SP back - the cast
	// did nothing, so it must not cost mana.
	for ty := 0; ty < 20; ty++ {
		for tx := 0; tx < 20; tx++ {
			g.world.Tiles[ty][tx] = world.TileWall
		}
	}
	g.collisionSystem.UpdateTileChecker(g.world)
	heldX, heldY := g.camera.X, g.camera.Y
	spBefore := caster.SpellPoints
	if !cs.tryCastJump(def, caster) {
		t.Fatal("a refused jump still consumes the cast")
	}
	if g.camera.X != heldX || g.camera.Y != heldY {
		t.Errorf("party moved into rock: (%.0f,%.0f) -> (%.0f,%.0f)", heldX, heldY, g.camera.X, g.camera.Y)
	}
	if got := caster.SpellPoints - spBefore; got != cs.effectiveSpellCost(caster, def.SpellPointsCost) {
		t.Errorf("blocked jump refunded %d SP, want %d", got, cs.effectiveSpellCost(caster, def.SpellPointsCost))
	}
}

// zoneTiles lists the tiles the live zones of one spell cover.
func zoneTiles(g *MMGame, spellID string) map[[2]int]bool {
	ts := float64(g.config.GetTileSize())
	out := map[[2]int]bool{}
	for i := range g.steamZones {
		if z := &g.steamZones[i]; z.SpellID == spellID {
			out[[2]int{int(z.X / ts), int(z.Y / ts)}] = true
		}
	}
	return out
}

// Zone casts merge by TILE (user spec): re-casting on the same ground refreshes
// that ground instead of stacking a second field, a shifted cast only adds its
// NEW tiles, a cast on fresh ground is a separate field, and an overlapping cast
// REFRESHES the lifetime rather than extending it.
func TestZoneCast_MergesByTile(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall definition: %v", err)
	}
	caster := g.party.Members[0]
	g.camera.Angle = 0 // facing +X, so the wall spreads along Y

	cast := func(tileX, tileY int) {
		placePlayerAtTile(g, tileX, tileY, ts)
		if !cs.tryCastSteamZone(spells.SpellID("firewall"), def, caster) {
			t.Fatal("firewall was not handled by the zone path")
		}
	}

	g.steamZones = g.steamZones[:0]
	cast(5, 5)
	first := len(g.steamZones)
	if first != def.ZoneWidthTiles {
		t.Fatalf("first wall = %d cells, want %d", first, def.ZoneWidthTiles)
	}

	// Same spot again: one wall, not two.
	cast(5, 5)
	if got := len(g.steamZones); got != first {
		t.Errorf("re-cast on the same ground = %d cells, want %d", got, first)
	}

	// One tile sideways: two cells land on covered ground, one is new.
	cast(5, 6)
	if got := len(g.steamZones); got != first+1 {
		t.Errorf("wall shifted by one tile = %d cells, want %d", got, first+1)
	}
	if got := len(zoneTiles(g, "firewall")); got != first+1 {
		t.Errorf("covered tiles = %d, want %d", got, first+1)
	}

	// Far behind: no shared tile, so a second wall stands on its own.
	cast(1, 5)
	if got := len(g.steamZones); got != first+1+def.ZoneWidthTiles {
		t.Errorf("second wall on fresh ground = %d cells, want %d", got, first+1+def.ZoneWidthTiles)
	}

	// Lifetime is REFRESHED, never extended - no stacking a two-hour firewall.
	full := g.steamZones[0].FramesLeft
	for i := range g.steamZones {
		g.steamZones[i].FramesLeft /= 2
	}
	cast(5, 5)
	for i := range g.steamZones {
		if z := &g.steamZones[i]; z.FramesLeft > full {
			t.Fatalf("cell %d lifetime %d exceeds one cast's %d", i, z.FramesLeft, full)
		}
	}
	if g.steamZones[0].FramesLeft != full {
		t.Errorf("re-cast cell lifetime = %d, want the full %d", g.steamZones[0].FramesLeft, full)
	}
}

// A shifted re-cast refreshes only the tiles it actually lays: the old wall's
// edge tile keeps its own remaining lifetime.
func TestZoneCast_ShiftedWallLeavesOldEdgeTileTimer(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall definition: %v", err)
	}
	caster := g.party.Members[0]
	g.camera.Angle = 0 // facing +X, wall spreads along Y

	g.steamZones = g.steamZones[:0]
	placePlayerAtTile(g, 5, 5, ts)
	if !cs.tryCastSteamZone(spells.SpellID("firewall"), def, caster) {
		t.Fatal("first firewall cast failed")
	}
	full := g.steamZones[0].FramesLeft
	half := full / 2
	for i := range g.steamZones {
		g.steamZones[i].FramesLeft = half
	}

	placePlayerAtTile(g, 5, 6, ts)
	if !cs.tryCastSteamZone(spells.SpellID("firewall"), def, caster) {
		t.Fatal("shifted firewall cast failed")
	}
	edges, relaid := 0, 0
	for i := range g.steamZones {
		switch z := &g.steamZones[i]; z.FramesLeft {
		case half:
			edges++
		case full:
			relaid++
		default:
			t.Errorf("cell %d has lifetime %d, want %d (old edge) or %d (relaid)", i, z.FramesLeft, half, full)
		}
	}
	if edges != 1 || relaid != def.ZoneWidthTiles {
		t.Errorf("shifted cast left %d old-timer edges and %d relaid cells, want 1 and %d", edges, relaid, def.ZoneWidthTiles)
	}
}

// Hot Steam follows the same tile rule: cast twice from one tile and there is one
// field, not two overlapping ones.
func TestZoneCast_RadialRefreshesOnSameTile(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("hot_steam"))
	if err != nil {
		t.Fatalf("hot_steam definition: %v", err)
	}
	caster := g.party.Members[0]

	g.steamZones = g.steamZones[:0]
	placePlayerAtTile(g, 8, 8, ts)
	if !cs.tryCastSteamZone(spells.SpellID("hot_steam"), def, caster) {
		t.Fatal("hot_steam was not handled by the zone path")
	}
	g.steamZones[0].FramesLeft /= 2
	if !cs.tryCastSteamZone(spells.SpellID("hot_steam"), def, caster) {
		t.Fatal("second hot_steam cast was not handled")
	}
	if got := len(g.steamZones); got != 1 {
		t.Fatalf("same-tile re-cast = %d zones, want 1", got)
	}
	if want := cs.CalculateSpellDurationFrames(spells.SpellID("hot_steam"), caster); g.steamZones[0].FramesLeft != want {
		t.Errorf("refreshed lifetime = %d, want %d", g.steamZones[0].FramesLeft, want)
	}
}

// A wall keeps its full width and stays contiguous at ANY facing: it is laid
// along the grid axis nearest the party's right vector, so no two cells share a
// tile and none sit corner-to-corner with a gap between their radii.
func TestFirewall_GridAlignedAtEveryAngle(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall definition: %v", err)
	}
	caster := g.party.Members[0]

	for _, deg := range []float64{0, 15, 30, 45, 60, 90, 135, 180, 225, 270, 315} {
		t.Run(fmt.Sprintf("%.0fdeg", deg), func(t *testing.T) {
			g.steamZones = g.steamZones[:0]
			placePlayerAtTile(g, 10, 10, ts)
			g.camera.Angle = deg * math.Pi / 180
			if !cs.tryCastSteamZone(spells.SpellID("firewall"), def, caster) {
				t.Fatal("firewall was not handled by the zone path")
			}
			if got := len(g.steamZones); got != def.ZoneWidthTiles {
				t.Fatalf("wall = %d cells, want %d", got, def.ZoneWidthTiles)
			}

			seen := map[[2]int]bool{}
			for i := range g.steamZones {
				z := &g.steamZones[i]
				tileXY := [2]int{int(z.X / ts), int(z.Y / ts)}
				if seen[tileXY] {
					t.Fatalf("two cells share tile %v", tileXY)
				}
				seen[tileXY] = true
				if z.AxisX*z.AxisY != 0 || z.AxisX+z.AxisY == 0 {
					t.Errorf("cell axis (%.2f,%.2f) is not a single grid axis", z.AxisX, z.AxisY)
				}
				if i == 0 {
					continue
				}
				p := &g.steamZones[i-1]
				dx, dy := math.Abs(z.X-p.X), math.Abs(z.Y-p.Y)
				if (dx > 1 && dy > 1) || math.Max(dx, dy) > ts+1 {
					t.Errorf("cells %d and %d are not adjacent on one axis (dx=%.0f dy=%.0f)", i-1, i, dx, dy)
				}
				if gap := Distance(p.X, p.Y, z.X, z.Y) - 2*z.Radius; gap > 0 {
					t.Errorf("cells %d and %d leave a %.1fpx gap", i-1, i, gap)
				}
			}

			// The wall crosses the facing: its centre cell sits zone_ahead_tiles out.
			mid := g.steamZones[len(g.steamZones)/2]
			if d := Distance(g.camera.X, g.camera.Y, mid.X, mid.Y); math.Abs(d-def.ZoneAheadTiles*ts) > ts {
				t.Errorf("centre cell is %.0fpx from the party, want about %.0f", d, def.ZoneAheadTiles*ts)
			}
		})
	}
}

// Toppled props are filtered out of the sprite caches in place - the expensive
// full rebuild (world rescan, processed-sprite drop, prewarm re-arm) is only for
// changes that ADD props.
func TestFilterOutPropTiles_DropsOnlyTheGivenTiles(t *testing.T) {
	cache := []TransparentSpriteData{
		{tileX: 1, tileY: 1, spriteName: "tree_a"},
		{tileX: 2, tileY: 1, spriteName: "rock"},
		{tileX: 3, tileY: 7, spriteName: "tree_b"},
	}
	kept := filterOutPropTiles(cache, map[[2]int]bool{{2, 1}: true, {9, 9}: true})
	if len(kept) != 2 {
		t.Fatalf("kept %d entries, want 2: %+v", len(kept), kept)
	}
	for _, s := range kept {
		if s.tileX == 2 && s.tileY == 1 {
			t.Error("toppled tile survived in the cache")
		}
	}
}

// The live card must state the new authored fields: Jump's distance, and for a
// party-sparing nova the Monsters-only target line plus the topple chance.
func TestNewSpellCardsStateTheirAuthoredFields(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	cardText := func(id string) string {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s definition: %v", id, err)
		}
		return buildSpellTooltipUnified(def, char, cs, true)
	}

	jump := cardText("jump")
	if !strings.Contains(jump, "2 tiles straight ahead") {
		t.Errorf("Jump card hides its distance:\n%s", jump)
	}

	quake := cardText("earthquake")
	if !strings.Contains(quake, "Targets: Monsters only") {
		t.Errorf("Earthquake spares the party but the card says otherwise:\n%s", quake)
	}
	if !strings.Contains(quake, "Topples") {
		t.Errorf("Earthquake card hides the topple chance:\n%s", quake)
	}
}

// A Jump is movement: in turn-based mode it ends the party turn exactly like a
// step, so it cannot reposition after an attack while the others keep acting.
func TestJump_EndsTheTurnLikeMovementInTB(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.world = newTestWorldSized(g.config, 20, 20)
	g.collisionSystem.UpdateTileChecker(g.world)
	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = 5.5*ts, 5.5*ts
	g.camera.Angle = 0
	g.turnBasedMode = true
	g.currentTurn = 0
	g.partyActionsUsed = 1 // the party already attacked this round
	g.turnBasedSpRegenCount = 0
	for _, m := range g.party.Members {
		m.ActionsRemaining = 1
	}

	def, err := spells.GetSpellDefinitionByID(spells.SpellID("jump"))
	if err != nil {
		t.Fatalf("jump definition: %v", err)
	}
	if !cs.tryCastJump(def, g.party.Members[0]) {
		t.Fatal("jump must be handled")
	}

	for i, m := range g.party.Members {
		if m.ActionsRemaining != 0 {
			t.Errorf("member %d kept %d actions after the party jumped", i, m.ActionsRemaining)
		}
	}
	if !g.turnBasedExtraMonsterAction {
		t.Error("jumping after an attack must grant monsters the anti-kiting pass")
	}
	// Every production cast path commits the action after castResolvedSpell
	// returns. Jump already ended the party phase, so that generic commit must
	// not end the round a second time and accelerate SP/card regeneration.
	g.consumeSelectedCharActionWithRTCooldown(cs.SpellCooldownFrames(g.party.Members[0], spells.SpellID("jump")))
	if g.turnBasedSpRegenCount != 1 {
		t.Errorf("Jump advanced the TB regen counter to %d, want exactly 1", g.turnBasedSpRegenCount)
	}
}

// Ground FX (zone flames, tile auras) draw after the sprites, so they consult the
// actor depth buffer: a creature stamps the columns it covers with its distance,
// keeping the nearest value.
func TestStampActorDepth_MarksTheCentralBandWithTheNearest(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	wantWidth := g.config.GetScreenWidth()
	// A same-size Layout pass must repair a missing auxiliary buffer instead of
	// returning solely because the primary depth buffer already matches.
	g.depthBuffer = make([]float64, wantWidth)
	g.wallTopBuffer = make([]int, wantWidth)
	g.actorDepthBuffer = nil
	g.handleResize(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	if got := len(g.actorDepthBuffer); got != wantWidth {
		t.Fatalf("same-size resize left actor depth buffer at %d columns, want %d", got, wantWidth)
	}

	g.actorDepthBuffer = make([]float64, 200)
	for i := range g.actorDepthBuffer {
		g.actorDepthBuffer[i] = 1000
	}
	r := &Renderer{game: g}

	r.stampActorDepth(UnifiedSpriteRenderData{screenXF: 100, sizeF: 40, depthPerp: 300})
	if g.actorDepthBuffer[100] != 300 {
		t.Errorf("centre column = %.0f, want 300", g.actorDepthBuffer[100])
	}
	if g.actorDepthBuffer[60] != 1000 {
		t.Errorf("column outside the sprite = %.0f, want untouched 1000", g.actorDepthBuffer[60])
	}
	// The 7% margin keeps transparent edges from occluding.
	if g.actorDepthBuffer[81] != 1000 {
		t.Errorf("edge column 81 = %.0f, want untouched (margin)", g.actorDepthBuffer[81])
	}

	// A nearer creature wins; a farther one must not overwrite.
	r.stampActorDepth(UnifiedSpriteRenderData{screenXF: 100, sizeF: 40, depthPerp: 500})
	if g.actorDepthBuffer[100] != 300 {
		t.Errorf("farther creature overwrote the column: %.0f", g.actorDepthBuffer[100])
	}
	r.stampActorDepth(UnifiedSpriteRenderData{screenXF: 100, sizeF: 40, depthPerp: 120})
	if g.actorDepthBuffer[100] != 120 {
		t.Errorf("nearer creature did not win: %.0f", g.actorDepthBuffer[100])
	}
}

// Every per-column buffer must exist straight out of the real constructor: a nil
// one silently disables the occlusion check it feeds instead of failing.
func TestNewGame_AllocatesEveryColumnBuffer(t *testing.T) {
	t.Chdir("../..")
	g, _, cfg := bootOpenWorldGame(t, false)

	width := cfg.GetScreenWidth()
	for _, buf := range []struct {
		name string
		got  int
	}{
		{"depthBuffer", len(g.depthBuffer)},
		{"actorDepthBuffer", len(g.actorDepthBuffer)},
		{"wallTopBuffer", len(g.wallTopBuffer)},
	} {
		if buf.got != width {
			t.Errorf("%s has %d columns, want %d", buf.name, buf.got, width)
		}
	}
}

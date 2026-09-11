package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// zoneVictim spawns a plain, tough monster with no resistances. Callers move it
// into the zone AFTER the cast: a wall spell lays its cells two tiles ahead of
// the party, not on it.
func zoneVictim(t *testing.T, g *MMGame) *monsterPkg.Monster3D {
	t.Helper()
	m := monsterPkg.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "goblin", g.config)
	if m == nil {
		t.Fatal("goblin config missing")
	}
	m.MaxHitPoints, m.HitPoints = 5000, 5000
	m.Resistances = map[monsterPkg.DamageType]int{}
	g.registerSpawnedMonster(m)
	g.refreshMonsterCollisionState(m)
	return m
}

// placeInZone stands each monster on a zone cell's centre.
func placeInZone(t *testing.T, g *MMGame, z *PersistentDamageZone, victims ...*monsterPkg.Monster3D) {
	t.Helper()
	for _, m := range victims {
		m.X, m.Y = z.X, z.Y
		g.refreshMonsterCollisionState(m)
	}
}

// A zone burns with its SPELL'S element. Firewall used to deal WATER damage, so
// fire resistance did nothing and water resistance shielded a mob from flames.
func TestZoneDamageUsesTheSpellSchool(t *testing.T) {
	for _, tc := range []struct {
		spell   string
		resists monsterPkg.DamageType
		immune  monsterPkg.DamageType
	}{
		{spell: "firewall", resists: monsterPkg.DamageFire, immune: monsterPkg.DamageWater},
		{spell: "hot_steam", resists: monsterPkg.DamageWater, immune: monsterPkg.DamageFire},
	} {
		t.Run(tc.spell, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			cs := g.combat
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.spell))
			if err != nil {
				t.Fatalf("%s definition: %v", tc.spell, err)
			}

			// Immune to the OTHER element: this mob must still burn.
			wrong := zoneVictim(t, g)
			wrong.Resistances[tc.immune] = 100
			// Immune to the spell's OWN element: this mob must be untouched.
			right := zoneVictim(t, g)
			right.Resistances[tc.resists] = 100

			g.persistentDamageZones = g.persistentDamageZones[:0]
			if !cs.tryCastPersistentDamageZone(spells.SpellID(tc.spell), def, g.party.Members[0]).handled() {
				t.Fatalf("%s was not handled by the zone path", tc.spell)
			}
			placeInZone(t, g, &g.persistentDamageZones[0], wrong, right)
			tickZoneSpellOnce(cs, tc.spell)

			if wrong.HitPoints >= wrong.MaxHitPoints {
				t.Errorf("%s did nothing to a mob resistant to %v - wrong element applied",
					tc.spell, tc.immune)
			}
			if right.HitPoints != right.MaxHitPoints {
				t.Errorf("%s hurt a mob immune to %v for %d",
					tc.spell, tc.resists, right.MaxHitPoints-right.HitPoints)
			}
		})
	}
}

// A zone tick must read like an ordinary hit: flash, and a log line carrying the
// damage and the victim's remaining HP.
func TestZoneTickReportsLikeANormalHit(t *testing.T) {
	g, _ := summonTileWorld(t)
	cs := g.combat
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall definition: %v", err)
	}
	victim := zoneVictim(t, g)

	g.persistentDamageZones = g.persistentDamageZones[:0]
	if !cs.tryCastPersistentDamageZone(spells.SpellID("firewall"), def, g.party.Members[0]).handled() {
		t.Fatal("firewall was not handled by the zone path")
	}
	placeInZone(t, g, &g.persistentDamageZones[0], victim)
	before := len(g.combatLogHistory)
	tickZoneSpellOnce(cs, "firewall")

	if victim.HitTintFrames == 0 {
		t.Error("a burned monster must flash like any struck monster")
	}
	dealt := victim.MaxHitPoints - victim.HitPoints
	if dealt <= 0 {
		t.Fatal("victim took no damage")
	}
	hit := ""
	for _, entry := range g.combatLogHistory[before:] {
		if strings.Contains(entry.Text, victim.Name) && strings.Contains(entry.Text, "damage") {
			hit = entry.Text
		}
	}
	if hit == "" {
		t.Fatalf("no hit line logged, got %v", g.combatLogHistory[before:])
	}
	if !strings.Contains(hit, def.Name) || !strings.Contains(hit, "HP:") {
		t.Errorf("hit line %q must name the source and show remaining HP", hit)
	}
}

// One TB round deals exactly the authored number of ticks, and the entry pass
// runs after the mobs have moved: a mob standing in a Firewall must not collect
// an extra hit, while a mob that walks in during the round is burned that round.
func TestZone_TurnBasedTickCountAndEntryOrder(t *testing.T) {
	g, _ := summonTileWorld(t)
	g.turnBasedMode = true
	gl := &GameLoop{game: g}
	cs := g.combat
	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall definition: %v", err)
	}

	standing := zoneVictim(t, g)
	walksIn := zoneVictim(t, g)
	g.persistentDamageZones = g.persistentDamageZones[:0]
	if !cs.tryCastPersistentDamageZone(spells.SpellID("firewall"), def, g.party.Members[0]).handled() {
		t.Fatal("firewall was not handled by the zone path")
	}
	zone := &g.persistentDamageZones[0]
	placeInZone(t, g, zone, standing)
	// The second mob waits outside, far from every cell.
	walksIn.X, walksIn.Y = g.camera.X-8*float64(g.config.GetTileSize()), g.camera.Y
	g.refreshMonsterCollisionState(walksIn)

	ticksPerTurn := int(character.TurnBasedTurnSeconds / def.ZoneTickSeconds)
	perTick := zone.TickDamage
	before := standing.HitPoints

	runOneMonsterTurn(g, gl)

	if got, want := before-standing.HitPoints, ticksPerTurn*perTick; got != want {
		t.Errorf("standing mob took %d over one turn, want %d (%d ticks x %d)",
			got, want, ticksPerTurn, perTick)
	}

	// A mob that steps in after this round's ticks is burned by the entry pass
	// that closes the round - exactly one tick, not zero and not two.
	hp := walksIn.HitPoints
	placeInZone(t, g, zone, walksIn)
	gl.applyZoneEntryDamageAll()
	if got := hp - walksIn.HitPoints; got != perTick {
		t.Errorf("mob that walked in took %d, want one tick of %d", got, perTick)
	}
}

// Two fields of the same spell keep their OWN cadence: a tick of one must never
// bill the monsters standing in the other, or offset casts double each other's
// rate. And an expiring cell still owes its last tick - to its own victims.
func TestZone_IndependentFieldsBillSeparately(t *testing.T) {
	g, ts := summonTileWorld(t)
	gl := &GameLoop{game: g}
	g.turnBasedMode = false

	tps := g.config.GetTPS()
	perTick := 20
	mkZone := func(tileX int, counter, framesLeft int) PersistentDamageZone {
		x, y := TileCenterFromTile(tileX, 10, ts)
		return PersistentDamageZone{
			SpellID: "hot_steam", X: x, Y: y, Radius: 0.55 * ts,
			TickDamage: perTick, FramesLeft: framesLeft,
			IntervalFrames: tps, tickCounter: counter,
		}
	}
	// Field A ticks on this frame; field B is half an interval behind.
	g.persistentDamageZones = append(g.persistentDamageZones[:0], mkZone(4, tps-1, 10*tps), mkZone(20, tps/2, 10*tps))
	victimA, victimB := zoneVictim(t, g), zoneVictim(t, g)
	victimA.X, victimA.Y = g.persistentDamageZones[0].X, g.persistentDamageZones[0].Y
	victimB.X, victimB.Y = g.persistentDamageZones[1].X, g.persistentDamageZones[1].Y
	g.refreshMonsterCollisionState(victimA)
	g.refreshMonsterCollisionState(victimB)

	hpA, hpB := victimA.HitPoints, victimB.HitPoints
	gl.advancePersistentDamageZones(1)
	if got := hpA - victimA.HitPoints; got != perTick {
		t.Errorf("field A victim took %d, want one tick of %d", got, perTick)
	}
	if got := hpB - victimB.HitPoints; got != 0 {
		t.Errorf("field B victim took %d from ANOTHER field's tick, want 0", got)
	}

	// An expiring cell pays its final tick to the mob standing in IT.
	g.persistentDamageZones = append(g.persistentDamageZones[:0], mkZone(4, tps-1, 1), mkZone(20, 0, 10*tps))
	hpA, hpB = victimA.HitPoints, victimB.HitPoints
	gl.advancePersistentDamageZones(1)
	if got := hpA - victimA.HitPoints; got != perTick {
		t.Errorf("expiring cell paid %d to its own victim, want %d", got, perTick)
	}
	if got := hpB - victimB.HitPoints; got != 0 {
		t.Errorf("expiring cell's tick leaked %d onto the surviving field", got)
	}
	for i := range g.persistentDamageZones {
		if g.persistentDamageZones[i].X == mkZone(4, 0, 0).X {
			t.Error("expired cell survived the pass")
		}
	}
}

// A fresh cast bills entry damage again even for a monster the previous field had
// already burned: the stamps must not outlive the field.
func TestZone_BurnStampsDieWithTheField(t *testing.T) {
	g, ts := summonTileWorld(t)
	gl := &GameLoop{game: g}
	g.turnBasedMode = false
	cs := g.combat
	tps := g.config.GetTPS()

	victim := zoneVictim(t, g)
	x, y := TileCenterFromTile(6, 10, ts)
	victim.X, victim.Y = x, y
	g.refreshMonsterCollisionState(victim)

	g.persistentDamageZones = append(g.persistentDamageZones[:0], PersistentDamageZone{
		SpellID: "hot_steam", X: x, Y: y, Radius: 0.55 * ts,
		TickDamage: 30, FramesLeft: 1, IntervalFrames: tps, tickCounter: tps - 1,
	})
	gl.advancePersistentDamageZones(1) // fires the final tick, then the cell expires
	if len(g.persistentDamageZones) != 0 {
		t.Fatalf("field should be gone, %d cells left", len(g.persistentDamageZones))
	}
	// The stamps lived in that cell, so they died with it - nothing to leak.

	// New cast on the same spot: the entry pass must bill this monster again.
	g.persistentDamageZones = append(g.persistentDamageZones[:0], PersistentDamageZone{
		SpellID: "hot_steam", X: x, Y: y, Radius: 0.55 * ts,
		TickDamage: 30, FramesLeft: 10 * tps, IntervalFrames: tps,
	})
	hp := victim.HitPoints
	cs.applyZoneEntrySpell("hot_steam")
	if got := hp - victim.HitPoints; got != 30 {
		t.Errorf("fresh field billed %d on entry, want 30", got)
	}
}

// The frame AFTER a tick is where a shared burn stamp shows up: field A's tick
// must not un-mark field B's victim, or the entry pass bills B again next frame.
func TestZone_TickOfOneFieldDoesNotReArmTheOther(t *testing.T) {
	g, ts := summonTileWorld(t)
	gl := &GameLoop{game: g}
	g.turnBasedMode = false

	tps := g.config.GetTPS()
	perTick := 20
	mkZone := func(tileX int, counter int) PersistentDamageZone {
		x, y := TileCenterFromTile(tileX, 10, ts)
		return PersistentDamageZone{
			SpellID: "hot_steam", X: x, Y: y, Radius: 0.55 * ts,
			TickDamage: perTick, FramesLeft: 10 * tps,
			IntervalFrames: tps, tickCounter: counter,
		}
	}
	// A fires on the first frame; B is half an interval behind.
	g.persistentDamageZones = append(g.persistentDamageZones[:0], mkZone(4, tps-1), mkZone(20, tps/2))
	victimA, victimB := zoneVictim(t, g), zoneVictim(t, g)
	victimA.X, victimA.Y = g.persistentDamageZones[0].X, g.persistentDamageZones[0].Y
	victimB.X, victimB.Y = g.persistentDamageZones[1].X, g.persistentDamageZones[1].Y
	g.refreshMonsterCollisionState(victimA)
	g.refreshMonsterCollisionState(victimB)

	gl.updatePersistentDamageZonesRT() // entry pass bills both; field A also ticks
	hpA, hpB := victimA.HitPoints, victimB.HitPoints

	// Neither field reaches its interval in this window, and both victims are
	// standing still - so nothing may be billed. Field A's tick on the previous
	// frame must not have re-armed field B's victim.
	for i := 0; i < 2; i++ {
		gl.updatePersistentDamageZonesRT()
	}
	if got := hpA - victimA.HitPoints; got != 0 {
		t.Errorf("field A victim took %d while standing still between ticks, want 0", got)
	}
	if got := hpB - victimB.HitPoints; got != 0 {
		t.Errorf("field B victim took %d after ANOTHER field ticked, want 0", got)
	}

	// Cadence still runs: over a full interval each field bills its own victim once.
	hpA, hpB = victimA.HitPoints, victimB.HitPoints
	for i := 0; i < tps; i++ {
		gl.updatePersistentDamageZonesRT()
	}
	if got := hpA - victimA.HitPoints; got != perTick {
		t.Errorf("field A billed %d over one interval, want %d", got, perTick)
	}
	if got := hpB - victimB.HitPoints; got != perTick {
		t.Errorf("field B billed %d over one interval, want %d", got, perTick)
	}
}

func TestZone_EntryStampIsPerLogicalField(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	perTick := 20
	mkZone := func(tileX int, fieldID uint64) PersistentDamageZone {
		x, y := TileCenterFromTile(tileX, 10, ts)
		return PersistentDamageZone{
			SpellID: "hot_steam", FieldID: fieldID, X: x, Y: y,
			Radius: 0.55 * ts, TickDamage: perTick,
			FramesLeft: 10 * g.config.GetTPS(), IntervalFrames: g.config.GetTPS(),
		}
	}
	// Field B has two cells like a wall. Its cells share one entry stamp, while
	// the distant field A must not suppress B's first entry hit.
	g.persistentDamageZones = []PersistentDamageZone{mkZone(4, 11), mkZone(20, 22), mkZone(21, 22)}
	victim := zoneVictim(t, g)

	placeInZone(t, g, &g.persistentDamageZones[0], victim)
	hp := victim.HitPoints
	cs.applyZoneEntrySpell("hot_steam")
	if got := hp - victim.HitPoints; got != perTick {
		t.Fatalf("entry into field A billed %d, want %d", got, perTick)
	}

	placeInZone(t, g, &g.persistentDamageZones[1], victim)
	hp = victim.HitPoints
	cs.applyZoneEntrySpell("hot_steam")
	if got := hp - victim.HitPoints; got != perTick {
		t.Fatalf("field A stamp suppressed entry into field B: billed %d, want %d", got, perTick)
	}

	// Idle RT frames must not erode the field-wide stamp cell by cell.
	for i := 0; i < 3; i++ {
		cs.applyZoneEntrySpell("hot_steam")
	}
	placeInZone(t, g, &g.persistentDamageZones[2], victim)
	hp = victim.HitPoints
	cs.applyZoneEntrySpell("hot_steam")
	if got := hp - victim.HitPoints; got != 0 {
		t.Errorf("moving between cells of field B billed %d, want 0", got)
	}
}

// Two casts of one spell never share a tile (mergeZoneCast replaces), so
// overlapping fields means walls side by side: a mob pays the field of its own
// TILE, that field's snapshot, exactly once. No seam double-hit, no corner hole.
func TestZone_WallCellBillsItsOwnTileExactlyOnce(t *testing.T) {
	for _, mode := range []struct {
		name string
		hit  func(*CombatSystem)
	}{
		{name: "periodic tick", hit: func(cs *CombatSystem) { tickZoneSpellOnce(cs, "firewall") }},
		{name: "entry hit", hit: func(cs *CombatSystem) { cs.applyZoneEntrySpell("firewall") }},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				offsetX float64 // tiles from field A's centre
				offsetY float64
				want    int
			}{
				{name: "field A centre", want: 10},
				{name: "field A corner", offsetX: 0.45, offsetY: 0.45, want: 10},
				{name: "field B centre", offsetX: 1, want: 30},
				{name: "seam between the two casts", offsetX: 0.5, want: 30}, // the boundary belongs to tile B
			} {
				t.Run(tc.name, func(t *testing.T) {
					g, ts := summonTileWorld(t)
					wall := func(tileX int, fieldID uint64, dmg int) PersistentDamageZone {
						x, y := TileCenterFromTile(tileX, 10, ts)
						return PersistentDamageZone{
							SpellID: "firewall", FieldID: fieldID, X: x, Y: y,
							Radius: 0.55 * ts, TickDamage: dmg,
							FramesLeft: 600, IntervalFrames: 60, AxisY: 1,
						}
					}
					g.persistentDamageZones = []PersistentDamageZone{wall(10, 101, 10), wall(11, 202, 30)}
					victim := zoneVictim(t, g)
					victim.X = g.persistentDamageZones[0].X + tc.offsetX*ts
					victim.Y = g.persistentDamageZones[0].Y + tc.offsetY*ts
					g.refreshMonsterCollisionState(victim)

					before := victim.HitPoints
					mode.hit(g.combat)
					if got := before - victim.HitPoints; got != tc.want {
						t.Fatalf("dealt %d damage, want %d", got, tc.want)
					}
				})
			}
		})
	}
}

// Two radial casts of one spell overlap mid-air: the mob in the intersection
// pays the spell ONCE per pass - one entry hit, one hit per tick pass, and the
// per-cast cadence never adds up to double damage.
func TestZone_IntersectionOfTwoCastsBillsOnce(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	mk := func(tileX int, fieldID uint64, dmg int) PersistentDamageZone {
		x, y := TileCenterFromTile(tileX, 10, ts)
		return PersistentDamageZone{
			SpellID: "hot_steam", FieldID: fieldID, X: x, Y: y,
			Radius: 1.0 * ts, TickDamage: dmg,
			FramesLeft: 600, IntervalFrames: 60,
		}
	}
	g.persistentDamageZones = []PersistentDamageZone{mk(10, 101, 10), mk(11, 202, 30)}
	victim := zoneVictim(t, g)
	victim.X = g.persistentDamageZones[0].X + 0.5*ts // covered by both circles
	victim.Y = g.persistentDamageZones[0].Y
	g.refreshMonsterCollisionState(victim)

	fire := func(zoneIdx ...int) int {
		before := victim.HitPoints
		var firing []firingZoneCell
		for _, i := range zoneIdx {
			firing = append(firing, firingZoneCell{cell: g.persistentDamageZones[i], ticks: 1})
		}
		cs.billZoneTicks(firing)
		return before - victim.HitPoints
	}
	entry := func() int {
		before := victim.HitPoints
		cs.applyZoneEntrySpell("hot_steam")
		return before - victim.HitPoints
	}

	if got := entry(); got != 10 {
		t.Fatalf("entry into the intersection dealt %d, want one hit of 10", got)
	}
	if got := entry(); got != 0 {
		t.Fatalf("standing still repeated %d entry damage", got)
	}
	if got := fire(0, 1); got != 10 {
		t.Fatalf("synced tick of both casts dealt %d, want one hit of 10", got)
	}
	// Offset cadences: the cast that did NOT bill last skips (the mob is still
	// stamped), then the stamping cast's own tick bills - once per interval.
	if got := fire(0); got != 0 {
		t.Fatalf("offset tick of cast A dealt %d on a mob cast B still stamps, want 0", got)
	}
	if got := fire(1); got != 30 {
		t.Fatalf("cast B's own tick dealt %d, want 30", got)
	}
}

// Stamps expire with coverage: a mob that walks OUT of a cast and back in pays
// entry again, instead of holding a lifetime pass to that field.
func TestZone_ReEnteringAfterLeavingBillsAgain(t *testing.T) {
	g, ts := summonTileWorld(t)
	cs := g.combat
	x, y := TileCenterFromTile(10, 10, ts)
	g.persistentDamageZones = []PersistentDamageZone{{
		SpellID: "hot_steam", FieldID: 101, X: x, Y: y,
		Radius: 1.0 * ts, TickDamage: 10,
		FramesLeft: 600, IntervalFrames: 60,
	}}
	victim := zoneVictim(t, g)
	placeInZone(t, g, &g.persistentDamageZones[0], victim)

	entry := func() int {
		before := victim.HitPoints
		cs.applyZoneEntrySpell("hot_steam")
		return before - victim.HitPoints
	}
	if got := entry(); got != 10 {
		t.Fatalf("first entry dealt %d, want 10", got)
	}
	victim.X = x + 5*ts // walk far out; the pass while outside drops the stamp
	if got := entry(); got != 0 {
		t.Fatalf("a mob outside the zone took %d, want 0", got)
	}
	victim.X = x
	if got := entry(); got != 10 {
		t.Fatalf("re-entry dealt %d, want a fresh hit of 10", got)
	}
}

func TestZone_SyncStampsUsesFieldIdentity(t *testing.T) {
	g, ts := summonTileWorld(t)
	x, y := TileCenterFromTile(8, 8, ts)
	g.persistentDamageZones = []PersistentDamageZone{
		{SpellID: "hot_steam", FieldID: 101, MapKey: "map_a", X: x, Y: y, entered: map[string]bool{"a": true}},
		{SpellID: "hot_steam", FieldID: 202, MapKey: "map_b", X: x, Y: y, entered: map[string]bool{"old": true}},
	}
	firing := []firingZoneCell{{cell: g.persistentDamageZones[1], ticks: 1}}
	firing[0].cell.entered = map[string]bool{"b": true}

	g.combat.syncZoneStamps(firing)

	if !g.persistentDamageZones[0].entered["a"] || g.persistentDamageZones[0].entered["b"] {
		t.Errorf("map A stamps were overwritten: %v", g.persistentDamageZones[0].entered)
	}
	if !g.persistentDamageZones[1].entered["b"] || g.persistentDamageZones[1].entered["old"] {
		t.Errorf("map B did not receive its firing copy: %v", g.persistentDamageZones[1].entered)
	}
}

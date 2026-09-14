package game

import (
	"fmt"
	"math"
	"math/rand"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
	"ugataima/internal/world"
)

// PersistentDamageZone is one fixed-position field cast, shared by radial zones
// such as Hot Steam and wall-cell zones such as Firewall. CasterName is the
// stable roster identity used to restore source-dependent effects after load.
type PersistentDamageZone struct {
	SpellID        string
	CasterName     string
	FieldID        uint64  // logical cast; all cells of one wall share it
	MapKey         string  // map the zone was cast on - it never follows the party
	X, Y           float64 // world center (fixed at cast)
	Radius         float64 // pixels
	FramesLeft     int     // total lifetime remaining (frames)
	TickDamage     int
	TrueTickDamage int
	ResistPierce   int // snapshotted from the caster's school mastery
	IntervalFrames int // RT damage cadence
	tickCounter    int // frames since last RT tick
	// AxisX/AxisY is the unit vector a WALL zone runs along (Firewall). Zero for
	// a radial zone (Hot Steam). Fire renders its flames along this axis, so the
	// curtain stays flat instead of filling the tile.
	AxisX, AxisY float64
	// entered stamps monsters billed since this cell's last tick. Entry checks
	// combine stamps across cells sharing FieldID, so one wall acts as one field
	// while separate casts keep independent cadence. Transient: reload re-arms it.
	entered map[string]bool
}

func (g *MMGame) allocatePersistentDamageZoneFieldID() uint64 {
	g.nextPersistentDamageZoneFieldID++
	if g.nextPersistentDamageZoneFieldID == 0 {
		g.nextPersistentDamageZoneFieldID = 1
	}
	return g.nextPersistentDamageZoneFieldID
}

// ensurePersistentDamageZoneFieldIDs migrates legacy/test cells that predate field IDs.
// A zero-ID cell becomes its own field; released saves never contained a
// multi-cell wall, so this preserves the old single-zone contract.
func (g *MMGame) ensurePersistentDamageZoneFieldIDs() {
	for i := range g.persistentDamageZones {
		if id := g.persistentDamageZones[i].FieldID; id > g.nextPersistentDamageZoneFieldID {
			g.nextPersistentDamageZoneFieldID = id
		}
	}
	for i := range g.persistentDamageZones {
		if g.persistentDamageZones[i].FieldID == 0 {
			g.persistentDamageZones[i].FieldID = g.allocatePersistentDamageZoneFieldID()
		}
	}
}

func (g *MMGame) reseedPersistentDamageZoneFieldIDs() {
	g.nextPersistentDamageZoneFieldID = 0
	g.ensurePersistentDamageZoneFieldIDs()
}

// tryCastPersistentDamageZone handles every persistent-zone spell. It creates
// one radial cell or a fixed wall of cells and snapshots the caster identity.
func (cs *CombatSystem) tryCastPersistentDamageZone(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	if def.ZoneRadiusTiles <= 0 {
		return castNotHandled
	}
	tps := cs.game.config.GetTPS()
	tile := float64(cs.game.config.GetTileSize())
	interval := int(def.ZoneTickSeconds * float64(tps))
	if interval < 1 {
		interval = tps // default: once per second
	}
	// Duration scales with mastery (CalculateSpellDurationFrames), matching the
	// in-game tooltip - same source of truth as every other timed spell.
	frames := cs.CalculateSpellDurationFrames(spellID, caster)
	tickParts := cs.spellDamageParts(spellID, caster, cs.CalculatePersistentDamageZoneTickDamage(def, caster))
	newZone := PersistentDamageZone{
		SpellID:        string(spellID),
		CasterName:     caster.Name,
		FieldID:        cs.game.allocatePersistentDamageZoneFieldID(),
		MapKey:         currentMapKey(),
		X:              cs.game.camera.X,
		Y:              cs.game.camera.Y,
		Radius:         def.ZoneRadiusTiles * tile,
		FramesLeft:     frames,
		TickDamage:     tickParts.Normal,
		TrueTickDamage: tickParts.True,
		ResistPierce:   cs.spellResistPierce(caster, string(spellID)),
		IntervalFrames: interval,
	}

	cells := cs.zoneCastCells(newZone, def, tile)
	if len(cells) == 0 {
		cs.game.AddCombatMessage(fmt.Sprintf("There is no open ground for %s.", def.Name))
		return castNoEffect
	}
	cs.mergeZoneCast(cells)
	cs.game.AddCombatMessage(spellCastMessage(def))
	cs.game.setUtilityStatus(spellID, frames)
	return castCommitted
}

// zoneCastCells builds the cells one cast occupies: a wall spell
// (zone_width_tiles) lays one per tile across the party's facing,
// zone_ahead_tiles ahead; any other zone is a single cell on the party. Cells
// snap to tile centres - a zone covers TILES, and tiles decide what a re-cast
// refreshes.
func (cs *CombatSystem) zoneCastCells(proto PersistentDamageZone, def spells.SpellDefinition, tile float64) []PersistentDamageZone {
	g := cs.game
	snap := func(cell PersistentDamageZone) PersistentDamageZone {
		cell.X, cell.Y = TileCenterFromTile(TileIndex(cell.X, tile), TileIndex(cell.Y, tile), tile)
		return cell
	}
	blocked := func(cell PersistentDamageZone) bool {
		return g.world != nil && g.world.IsTileBlockingTerrainAt(TileIndex(cell.X, tile), TileIndex(cell.Y, tile))
	}
	if def.ZoneWidthTiles <= 1 {
		cell := snap(proto)
		if blocked(cell) {
			return nil
		}
		return []PersistentDamageZone{cell}
	}

	ahead := def.ZoneAheadTiles
	if ahead <= 0 {
		ahead = 1
	}
	fx, fy := math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
	rx, ry := -fy, fx // camera right, in world space
	// The wall runs along the GRID AXIS nearest the facing's right vector. Laying
	// it on the raw diagonal instead leaves cells sharing a tile (a 3-wide wall
	// becoming 2 cells) or sitting corner-to-corner with a gap between them.
	stepX, stepY := 0, 1
	if math.Abs(rx) > math.Abs(ry) {
		stepX, stepY = 1, 0
	}
	if (stepX == 1 && rx < 0) || (stepY == 1 && ry < 0) {
		stepX, stepY = -stepX, -stepY
	}
	centerTX := TileIndex((g.camera.X + fx*ahead*tile), tile)
	centerTY := TileIndex((g.camera.Y + fy*ahead*tile), tile)
	half := (def.ZoneWidthTiles - 1) / 2
	cells := make([]PersistentDamageZone, 0, def.ZoneWidthTiles)
	for i := 0; i < def.ZoneWidthTiles; i++ {
		off := i - half
		cell := proto
		cell.X, cell.Y = TileCenterFromTile(centerTX+stepX*off, centerTY+stepY*off, tile)
		cell.AxisX, cell.AxisY = float64(stepX), float64(stepY)
		if blocked(cell) {
			continue
		}
		cells = append(cells, cell)
	}
	return cells
}

// mergeZoneCast lays a cast's cells in by TILE. A cell on ground the same spell
// already covers replaces that cell (refresh, never extend); a cell on fresh
// ground is appended. Tiles the cast does NOT lay are untouched - a shifted wall
// must not refresh the old edge tile. Cells compare by WORLD, since two region
// keys share one map on the unified world.
func (cs *CombatSystem) mergeZoneCast(cells []PersistentDamageZone) {
	if len(cells) == 0 {
		return
	}
	tile := float64(cs.game.config.GetTileSize())
	sameWorldZone := func(a, b string) bool {
		if wm := world.GlobalWorldManager; wm != nil {
			return wm.SameWorldKey(a, b)
		}
		return a == b
	}
	sameTile := func(a, b PersistentDamageZone) bool {
		return TileIndex(a.X, tile) == TileIndex(b.X, tile) && TileIndex(a.Y, tile) == TileIndex(b.Y, tile)
	}

	live := cs.game.persistentDamageZones
	for _, cell := range cells {
		matched := false
		for i := range live {
			z := &live[i]
			if z.SpellID != cell.SpellID || !sameWorldZone(z.MapKey, cell.MapKey) {
				continue
			}
			if sameTile(*z, cell) {
				cell.tickCounter, cell.entered = 0, nil // relaid: fresh cadence
				*z = cell
				matched = true
			}
		}
		if !matched {
			live = append(live, cell)
		}
	}
	cs.game.persistentDamageZones = live
}

// zoneSourceName is the zone's display name for combat messages - the spell's
// authored name, falling back to the raw id for a stub/legacy zone.
func zoneSourceName(spellID string) string {
	if def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellID)); err == nil && def.Name != "" {
		return def.Name
	}
	return spellID
}

// persistentDamageZoneCaster resolves the saved source identity across every
// roster. A benched caster still owns a field they laid while active.
func (g *MMGame) persistentDamageZoneCaster(zone *PersistentDamageZone) *character.MMCharacter {
	if g == nil || g.party == nil || zone == nil || zone.CasterName == "" {
		return nil
	}
	for _, roster := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve, g.party.Captive} {
		for _, member := range roster {
			if member != nil && member.Name == zone.CasterName {
				return member
			}
		}
	}
	return nil
}

// firingZoneCell is one cell that reached its tick interval, plus how many ticks
// it owes this pass.
type firingZoneCell struct {
	cell  PersistentDamageZone
	ticks int
}

// zoneStampedAny: the monster is stamped somewhere in the spell's live cells,
// i.e. already paid this spell and is still inside the stamping field.
func zoneStampedAny(view []*PersistentDamageZone, monsterID string) bool {
	for _, z := range view {
		if z.entered[monsterID] {
			return true
		}
	}
	return false
}

// dropStaleZoneStamps forgets monsters a FIELD no longer covers: a stamp means
// "paid AND still inside that field". Per field, not per cell - standing on one
// tile of a wall keeps the whole wall paid.
func (cs *CombatSystem) dropStaleZoneStamps(cells []*PersistentDamageZone) {
	tile := float64(cs.game.config.GetTileSize())
	pos := map[string][2]float64{}
	for _, m := range cs.game.world.Monsters {
		if m != nil && m.IsAlive() {
			pos[m.ID] = [2]float64{m.X, m.Y}
		}
	}
	byField := map[uint64][]*PersistentDamageZone{}
	for _, z := range cells {
		byField[z.FieldID] = append(byField[z.FieldID], z)
	}
	for _, field := range byField {
		stamped := map[string]bool{}
		for _, z := range field {
			for id := range z.entered {
				stamped[id] = true
			}
		}
		for id := range stamped {
			covered := false
			if p, ok := pos[id]; ok {
				for _, z := range field {
					if z.coversMonster(p[0], p[1], tile) {
						covered = true
						break
					}
				}
			}
			if !covered {
				for _, z := range field {
					delete(z.entered, id)
				}
			}
		}
	}
}

// isWallCell: only a wall zone (Firewall) carries a run axis.
func (z *PersistentDamageZone) isWallCell() bool { return z.AxisX != 0 || z.AxisY != 0 }

// coversMonster is the zone hit test. A wall cell covers exactly its TILE (it is
// laid and merged in tile terms; its 0.55 radius is render spread only); a radial
// field covers its circle.
func (z *PersistentDamageZone) coversMonster(mx, my, tile float64) bool {
	if z.isWallCell() {
		return TileIndex(mx, tile) == TileIndex(z.X, tile) && TileIndex(my, tile) == TileIndex(z.Y, tile)
	}
	dx, dy := z.X-mx, z.Y-my
	return dx*dx+dy*dy <= z.Radius*z.Radius // squared: runs cells x monsters every frame
}

// zoneCoveredMonsters maps each monster to every supplied cell covering it. The
// caller bills the monster once, then stamps every logical field it occupies.
func (cs *CombatSystem) zoneCoveredMonsters(cells []*PersistentDamageZone) map[*monsterPkg.Monster3D][]*PersistentDamageZone {
	covered := map[*monsterPkg.Monster3D][]*PersistentDamageZone{}
	tile := float64(cs.game.config.GetTileSize())
	for _, z := range cells {
		if z.TickDamage <= 0 && z.TrueTickDamage <= 0 {
			continue
		}
		// A zone lives on the map it was cast on: same coordinates on another map
		// would scald that map's monsters out of thin air.
		if z.MapKey != "" && !mapKeyOnCurrentWorld(z.MapKey) {
			continue
		}
		for _, m := range cs.game.world.Monsters {
			// An invulnerable boss (sealed or idol-warded) is unscathed by the zone.
			if m == nil || !m.IsAlive() || isPurePartySummon(m) || m.IsDamageInvulnerable() {
				continue
			}
			if !z.coversMonster(m.X, m.Y, tile) {
				continue
			}
			covered[m] = append(covered[m], z)
		}
	}
	return covered
}

// billZoneTicks charges the ticks owed by the cells that just fired, per tick
// index and per spell. A firing cell clears its own stamps first, so its
// occupants are re-billed; a monster still stamped by ANOTHER cell of the spell
// (an overlapping cast that billed it last) is skipped - overlap never stacks.
func (cs *CombatSystem) billZoneTicks(firing []firingZoneCell) {
	maxTicks := 0
	for i := range firing {
		if firing[i].ticks > maxTicks {
			maxTicks = firing[i].ticks
		}
	}
	for t := 0; t < maxTicks; t++ {
		bySpell := map[string][]*PersistentDamageZone{}
		var order []string
		for i := range firing {
			if firing[i].ticks <= t {
				continue
			}
			cell := &firing[i].cell
			cell.entered = nil // this cell's fresh tick bills everyone inside it again
			if bySpell[cell.SpellID] == nil {
				order = append(order, cell.SpellID)
			}
			bySpell[cell.SpellID] = append(bySpell[cell.SpellID], cell)
		}
		for _, id := range order {
			cs.damageZoneMonsters(id, bySpell[id], cs.zoneTickView(id, bySpell[id]))
		}
		// The firing copies own the stamps; hand them back to the live cells.
		cs.syncZoneStamps(firing)
	}
}

// zoneTickView is the stamp authority during a tick pass: the firing copies plus
// the spell's live cells that are not firing (a firing cell's live counterpart
// holds stale stamps until syncZoneStamps).
func (cs *CombatSystem) zoneTickView(spellID string, firingCells []*PersistentDamageZone) []*PersistentDamageZone {
	tile := float64(cs.game.config.GetTileSize())
	type cellKey struct {
		field  uint64
		tx, ty int
	}
	fired := map[cellKey]bool{}
	view := append([]*PersistentDamageZone(nil), firingCells...)
	for _, z := range firingCells {
		fired[cellKey{z.FieldID, TileIndex(z.X, tile), TileIndex(z.Y, tile)}] = true
	}
	for i := range cs.game.persistentDamageZones {
		z := &cs.game.persistentDamageZones[i]
		if z.SpellID != spellID || fired[cellKey{z.FieldID, TileIndex(z.X, tile), TileIndex(z.Y, tile)}] {
			continue
		}
		view = append(view, z)
	}
	return view
}

// syncZoneStamps copies burn stamps from firing cell copies back to their live
// cells. FieldID separates identical tile coordinates on different maps.
func (cs *CombatSystem) syncZoneStamps(firing []firingZoneCell) {
	tile := float64(cs.game.config.GetTileSize())
	for i := range firing {
		src := &firing[i].cell
		for j := range cs.game.persistentDamageZones {
			live := &cs.game.persistentDamageZones[j]
			if live.SpellID != src.SpellID || live.FieldID != src.FieldID {
				continue
			}
			if TileIndex(live.X, tile) == TileIndex(src.X, tile) && TileIndex(live.Y, tile) == TileIndex(src.Y, tile) {
				live.entered = src.entered
				break
			}
		}
	}
}

// applyZoneEntrySpell bills whatever has ENTERED the spell since its last tick.
// Runs every RT frame and once at the end of a TB monster round, so crossing a
// zone always costs at least one tick's damage. Stale stamps are dropped first,
// so walking out of one cast and into another pays a fresh entry hit.
func (cs *CombatSystem) applyZoneEntrySpell(spellID string) {
	cs.game.ensurePersistentDamageZoneFieldIDs()
	var cells []*PersistentDamageZone
	for i := range cs.game.persistentDamageZones {
		if z := &cs.game.persistentDamageZones[i]; z.SpellID == spellID && z.FramesLeft > 0 {
			cells = append(cells, z)
		}
	}
	cs.dropStaleZoneStamps(cells)
	cs.damageZoneMonsters(spellID, cells, cells)
}

// stampCoveredZoneFields marks every cell belonging to a field the monster
// currently occupies. This is what lets a mob cross a multi-cell wall without
// paying a second entry hit.
func stampCoveredZoneFields(cells, covering []*PersistentDamageZone, monsterID string) {
	fields := map[uint64]bool{}
	for _, z := range covering {
		fields[z.FieldID] = true
	}
	for _, z := range cells {
		if !fields[z.FieldID] {
			continue
		}
		if z.entered == nil {
			z.entered = map[string]bool{}
		}
		z.entered[monsterID] = true
	}
}

// damageZoneMonsters bills every monster covered by a coverage cell and not
// stamped anywhere in view - ONE hit per spell per pass, however many cells or
// casts overlap it. The snapshot comes from the first covering cell (stable
// slice order); the hit stamps every covering field in view.
func (cs *CombatSystem) damageZoneMonsters(spellID string, coverage, view []*PersistentDamageZone) {
	if len(coverage) == 0 {
		return
	}
	// Element = the spell's authored school, so resistance matches the flames.
	damageTypeStr := spellDamageTypeStr(spellID)
	covered := cs.zoneCoveredMonsters(coverage)
	for _, m := range cs.game.world.Monsters {
		covering := covered[m]
		if len(covering) == 0 {
			continue
		}
		if zoneStampedAny(view, m.ID) {
			continue
		}
		stampCoveredZoneFields(view, covering, m.ID)
		z := covering[0]
		if cs.tryDarkElfBindInstead(cs.game.persistentDamageZoneCaster(z), m) {
			continue
		}
		parts, _ := cs.spellPartsWithOutgoingBuff(
			damagecalc.Parts{Normal: z.TickDamage, True: z.TrueTickDamage},
			damageTypeStr,
		)
		actual := cs.applyMonsterDamagePacket(
			m,
			singleMonsterDamagePacket(parts, damageTypeStr, z.ResistPierce),
			monsterDamageOptions{IgnoreArmor: true},
		).Total()
		cs.reportIndirectHit(m, actual, zoneSourceName(spellID))
		if damageTypeStr == damagecalc.Water.String() {
			cs.game.spawnSteamPuff(m.X, m.Y) // scalding steam keeps its own puff
		}
		cs.finishIndirectKill(m)
	}
}

// updatePersistentDamageZonesRT advances zones only in real time. TB owns the same clock at
// the monster-round boundary in tickPersistentDamageZonesTB.
func (gl *GameLoop) updatePersistentDamageZonesRT() {
	if gl.game.turnBasedMode {
		return
	}
	gl.applyZoneEntryDamageAll()
	gl.advancePersistentDamageZones(1)
}

// applyZoneEntryDamageAll gives every live spell field its entry pass.
func (gl *GameLoop) applyZoneEntryDamageAll() {
	for _, spellID := range gl.game.liveZoneSpellIDs() {
		gl.game.combat.applyZoneEntrySpell(spellID)
	}
}

// liveZoneSpellIDs lists the spells with at least one live cell, in a stable
// order so damage never depends on map iteration.
func (g *MMGame) liveZoneSpellIDs() []string {
	var ids []string
	seen := map[string]bool{}
	for i := range g.persistentDamageZones {
		if z := &g.persistentDamageZones[i]; z.FramesLeft > 0 && !seen[z.SpellID] {
			seen[z.SpellID] = true
			ids = append(ids, z.SpellID)
		}
	}
	return ids
}

// advancePersistentDamageZones advances lifetime and cadence by elapsedFrames. It is the
// single RT/TB implementation: one RT frame passes 1; one TB monster round
// passes the configured three-second equivalent. Damage is resolved before
// final-turn expiry, matching poison/burn's final active tick.
func (gl *GameLoop) advancePersistentDamageZones(elapsedFrames int) {
	gl.game.ensurePersistentDamageZoneFieldIDs()
	zones := gl.game.persistentDamageZones
	if len(zones) == 0 || elapsedFrames <= 0 {
		return
	}
	// Several zones can share one spell id (recasts at different spots), but
	// the HUD has ONE status per id - aggregate to the LONGEST-lived survivor,
	// and clear an id only when its last zone expired (per-zone updates let a
	// short zone wipe the icon of a longer one, order-dependent).
	maxLeft := map[string]int{}
	expired := map[string]bool{}
	// Cells that fire this pass, captured BY VALUE: an expiring cell still owes its
	// last tick, and only the cells that actually reached their interval may bill.
	var firing []firingZoneCell
	w := 0
	for i := range zones {
		z := &zones[i]
		if z.FramesLeft <= 0 {
			expired[z.SpellID] = true
			continue
		}

		interval := z.IntervalFrames
		if interval <= 0 {
			interval = turnBasedPeriodicEffectFrames(gl.game.config.GetTPS())
		}
		ticks, _ := status.TickDoT(&z.FramesLeft, &z.tickCounter, elapsedFrames, interval)
		if ticks > 0 {
			firing = append(firing, firingZoneCell{cell: *z, ticks: ticks})
		}

		if z.FramesLeft <= 0 {
			z.FramesLeft = 0
			expired[z.SpellID] = true
			continue
		}
		if z.FramesLeft > maxLeft[z.SpellID] {
			maxLeft[z.SpellID] = z.FramesLeft
		}
		// Ambient steam is now a per-tile procedural bubble field drawn each
		// frame (Renderer.drawPersistentDamageZoneEffects) - no sparse particle spawns here.
		zones[w] = *z
		w++
	}
	gl.game.persistentDamageZones = zones[:w]
	gl.game.combat.billZoneTicks(firing)
	for id, left := range maxLeft {
		gl.game.updateUtilityStatus(spells.SpellID(id), left, true)
	}
	for id := range expired {
		if maxLeft[id] == 0 {
			gl.game.updateUtilityStatus(spells.SpellID(id), 0, false)
		}
	}
}

// tickPersistentDamageZonesTB fires a round's periodic ticks. The entry pass belongs at the
// END of the monster turn (updateMonstersTurnBased), once the mobs have moved.
//
// tickPersistentDamageZonesTB advances Hot Steam by the same three seconds one TB round
// represents for poison/burn. With its authored three-second interval this is
// exactly one damage tick, and no time passes while the player deliberates.
func (gl *GameLoop) tickPersistentDamageZonesTB() {
	gl.advancePersistentDamageZones(turnBasedPeriodicEffectFrames(gl.game.config.GetTPS()))
}

// syncPersistentDamageZoneStatuses rebuilds the one-HUD-icon-per-spell view without
// advancing zone time. Load uses it because a restored TB game may deliberate
// indefinitely before the next monster round updates the zone.
func (g *MMGame) syncPersistentDamageZoneStatuses() {
	maxLeft := map[string]int{}
	for i := range g.persistentDamageZones {
		z := &g.persistentDamageZones[i]
		if z.FramesLeft > maxLeft[z.SpellID] {
			maxLeft[z.SpellID] = z.FramesLeft
		}
	}
	for id, left := range maxLeft {
		g.updateUtilityStatus(spells.SpellID(id), left, left > 0)
	}
}

// spawnSteamPuff emits a small cluster of rising, fading whitish-gray particles
// at a point - the look of scalding steam.
func (g *MMGame) spawnSteamPuff(x, y float64) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()
	g.appendSteamPuffLocked(x, y, 5)
}

// appendSteamPuffLocked builds one rising steam puff. Caller holds hitEffectsMu.
func (g *MMGame) appendSteamPuffLocked(x, y float64, count int) {
	base := [3]int{220, 225, 230} // pale gray-white steam
	particles := make([]SpellHitParticle, 0, count)
	for i := 0; i < count; i++ {
		tint := mixColor(base, [3]int{255, 255, 255}, rand.Float64()*0.5)
		particles = append(particles, SpellHitParticle{
			X:        x,
			Y:        y,
			OffsetX:  (rand.Float64() - 0.5) * 10,
			OffsetY:  (rand.Float64() - 0.5) * 6,
			VelX:     (rand.Float64() - 0.5) * 0.5,
			VelY:     -(0.8 + rand.Float64()*1.0), // rise like steam
			Gravity:  -0.02,
			Color:    tint,
			LifeTime: SpellParticleLife + rand.Intn(10),
			MaxLife:  SpellParticleLife,
			Size:     SpellParticleSize,
			Active:   true,
		})
	}
	g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Particles: particles, Active: true})
}

// PersistentDamageZoneSave is the JSON form of a PersistentDamageZone for save files.
type PersistentDamageZoneSave struct {
	SpellID        string  `json:"spell_id"`
	CasterName     string  `json:"caster_name,omitempty"`
	FieldID        uint64  `json:"field_id,omitempty"`
	MapKey         string  `json:"map_key,omitempty"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Radius         float64 `json:"radius"`
	FramesLeft     int     `json:"frames_left"`
	TickDamage     int     `json:"tick_damage"`
	TrueTickDamage int     `json:"true_tick_damage,omitempty"`
	ResistPierce   int     `json:"resist_pierce,omitempty"`
	IntervalFrames int     `json:"interval_frames"`
	TickCounter    int     `json:"tick_counter,omitempty"`
	AxisX          float64 `json:"axis_x,omitempty"`
	AxisY          float64 `json:"axis_y,omitempty"`
}

func buildPersistentDamageZoneSaves(zones []PersistentDamageZone) []PersistentDamageZoneSave {
	if len(zones) == 0 {
		return nil
	}
	out := make([]PersistentDamageZoneSave, len(zones))
	for i, z := range zones {
		out[i] = PersistentDamageZoneSave{
			SpellID: z.SpellID, CasterName: z.CasterName, FieldID: z.FieldID, MapKey: z.MapKey, X: z.X, Y: z.Y, Radius: z.Radius,
			FramesLeft: z.FramesLeft, TickDamage: z.TickDamage, TrueTickDamage: z.TrueTickDamage, ResistPierce: z.ResistPierce,
			IntervalFrames: z.IntervalFrames, TickCounter: z.tickCounter,
			AxisX: z.AxisX, AxisY: z.AxisY,
		}
	}
	return out
}

// restorePersistentDamageZones rebuilds zones from a save; legacy entries without a map
// are pinned to the map the save was made on (same migration as loot bags).
func restorePersistentDamageZones(saves []PersistentDamageZoneSave, saveMapKey string) []PersistentDamageZone {
	if len(saves) == 0 {
		return nil
	}
	out := make([]PersistentDamageZone, len(saves))
	for i, s := range saves {
		mapKey := s.MapKey
		if mapKey == "" {
			mapKey = saveMapKey
		}
		out[i] = PersistentDamageZone{
			SpellID: s.SpellID, CasterName: s.CasterName, FieldID: s.FieldID, MapKey: mapKey, X: s.X, Y: s.Y, Radius: s.Radius,
			FramesLeft: s.FramesLeft, TickDamage: s.TickDamage, TrueTickDamage: s.TrueTickDamage, ResistPierce: s.ResistPierce,
			IntervalFrames: s.IntervalFrames, tickCounter: s.TickCounter,
			AxisX: s.AxisX, AxisY: s.AxisY,
		}
	}
	return out
}

// spellCastMessage is the authored cast line, falling back to a generic one.
func spellCastMessage(def spells.SpellDefinition) string {
	if def.Message != "" {
		return def.Message
	}
	return fmt.Sprintf("%s takes hold!", def.Name)
}

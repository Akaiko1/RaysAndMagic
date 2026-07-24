package game

import (
	"math/rand"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// SteamZone is a fixed-position persistent damage field (Hot Steam). It is
// spawned at the party's location on cast and, each tick, sears every monster
// within Radius. RT advances it every frame; one TB monster round advances it by
// TurnBasedPeriodicEffectSeconds, so deliberating over the party turn can never
// deal free damage or consume duration.
type SteamZone struct {
	SpellID        string
	MapKey         string  // map the zone was cast on - it never follows the party
	X, Y           float64 // world center (fixed at cast)
	Radius         float64 // pixels
	FramesLeft     int     // total lifetime remaining (frames)
	TickDamage     int
	ResistPierce   int // snapshotted from the caster's school mastery
	IntervalFrames int // RT damage cadence
	tickCounter    int // frames since last RT tick
}

// tryCastSteamZone handles persistent-zone spells (Hot Steam): spawns a fixed
// damage zone centered on the party. Gated on ZoneRadiusTiles > 0.
func (cs *CombatSystem) tryCastSteamZone(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.ZoneRadiusTiles <= 0 {
		return false
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
	newZone := SteamZone{
		SpellID:        string(spellID),
		MapKey:         currentMapKey(),
		X:              cs.game.camera.X,
		Y:              cs.game.camera.Y,
		Radius:         def.ZoneRadiusTiles * tile,
		FramesLeft:     frames,
		TickDamage:     cs.CalculateSteamZoneTickDamage(def, caster),
		ResistPierce:   cs.spellResistPierce(caster, string(spellID)),
		IntervalFrames: interval,
	}

	// Separate zones are allowed, but overlapping copies of the same spell
	// replace one another so a single area cannot multiply its damage. Zones
	// compare by WORLD, not raw key: on the unified world two region keys
	// share one map, and an overlap across a seam must still replace.
	sameWorldZone := func(a, b string) bool {
		if wm := world.GlobalWorldManager; wm != nil {
			return wm.SameWorldKey(a, b)
		}
		return a == b
	}
	zones := cs.game.steamZones
	w := 0
	for i := range zones {
		z := zones[i]
		sameArea := z.SpellID == newZone.SpellID &&
			sameWorldZone(z.MapKey, newZone.MapKey) &&
			Distance(z.X, z.Y, newZone.X, newZone.Y) < z.Radius+newZone.Radius
		if sameArea {
			continue
		}
		zones[w] = z
		w++
	}
	cs.game.steamZones = append(zones[:w], newZone)
	cs.game.AddCombatMessage(def.Message)
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

// damageSteamZoneOnce applies one damage tick from a zone to every monster inside
// it, with a small steam puff on each victim.
func (cs *CombatSystem) damageSteamZoneOnce(z *SteamZone) {
	if z.TickDamage <= 0 {
		return
	}
	// A zone lives on the map it was cast on: same coordinates on another map
	// would scald that map's monsters out of thin air.
	if z.MapKey != "" && !mapKeyOnCurrentWorld(z.MapKey) {
		return
	}
	damageTypeStr := damagecalc.Water.String()
	tickDamage := z.TickDamage + cs.game.combatBuffOutBonusForDamageType(damageTypeStr)
	for _, m := range cs.game.world.Monsters {
		// An invulnerable boss (sealed or idol-warded) is unscathed by the zone.
		if m == nil || !m.IsAlive() || m.IsDamageInvulnerable() {
			continue
		}
		if Distance(z.X, z.Y, m.X, m.Y) > z.Radius {
			continue
		}
		cs.applyMonsterDamagePacket(
			m,
			singleMonsterDamagePacket(damagecalc.Parts{Normal: tickDamage}, damageTypeStr, z.ResistPierce),
			monsterDamageOptions{IgnoreArmor: true},
		)
		cs.markMonsterHit(m)
		cs.game.spawnSteamPuff(m.X, m.Y)
		cs.finishIndirectKill(m)
	}
}

// updateSteamZonesRT advances zones only in real time. TB owns the same clock at
// the monster-round boundary in tickSteamZonesTB.
func (gl *GameLoop) updateSteamZonesRT() {
	if gl.game.turnBasedMode {
		return
	}
	gl.advanceSteamZones(1)
}

// advanceSteamZones advances lifetime and cadence by elapsedFrames. It is the
// single RT/TB implementation: one RT frame passes 1; one TB monster round
// passes the configured three-second equivalent. Damage is resolved before
// final-turn expiry, matching poison/burn's final active tick.
func (gl *GameLoop) advanceSteamZones(elapsedFrames int) {
	zones := gl.game.steamZones
	if len(zones) == 0 || elapsedFrames <= 0 {
		return
	}
	// Several zones can share one spell id (recasts at different spots), but
	// the HUD has ONE status per id - aggregate to the LONGEST-lived survivor,
	// and clear an id only when its last zone expired (per-zone updates let a
	// short zone wipe the icon of a longer one, order-dependent).
	maxLeft := map[string]int{}
	expired := map[string]bool{}
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
		z.tickCounter += elapsedFrames
		for z.tickCounter >= interval {
			z.tickCounter -= interval
			gl.game.combat.damageSteamZoneOnce(z)
		}

		z.FramesLeft -= elapsedFrames
		if z.FramesLeft <= 0 {
			z.FramesLeft = 0
			expired[z.SpellID] = true
			continue
		}
		if z.FramesLeft > maxLeft[z.SpellID] {
			maxLeft[z.SpellID] = z.FramesLeft
		}
		// Ambient steam is now a per-tile procedural bubble field drawn each
		// frame (Renderer.drawSteamZoneBubbles) - no sparse particle spawns here.
		zones[w] = *z
		w++
	}
	gl.game.steamZones = zones[:w]
	for id, left := range maxLeft {
		gl.game.updateUtilityStatus(spells.SpellID(id), left, true)
	}
	for id := range expired {
		if maxLeft[id] == 0 {
			gl.game.updateUtilityStatus(spells.SpellID(id), 0, false)
		}
	}
}

// tickSteamZonesTB advances Hot Steam by the same three seconds one TB round
// represents for poison/burn. With its authored three-second interval this is
// exactly one damage tick, and no time passes while the player deliberates.
func (gl *GameLoop) tickSteamZonesTB() {
	gl.advanceSteamZones(turnBasedPeriodicEffectFrames(gl.game.config.GetTPS()))
}

// syncSteamZoneStatuses rebuilds the one-HUD-icon-per-spell view without
// advancing zone time. Load uses it because a restored TB game may deliberate
// indefinitely before the next monster round updates the zone.
func (g *MMGame) syncSteamZoneStatuses() {
	maxLeft := map[string]int{}
	for i := range g.steamZones {
		z := &g.steamZones[i]
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

// SteamZoneSave is the JSON form of a SteamZone for save files.
type SteamZoneSave struct {
	SpellID        string  `json:"spell_id"`
	MapKey         string  `json:"map_key,omitempty"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Radius         float64 `json:"radius"`
	FramesLeft     int     `json:"frames_left"`
	TickDamage     int     `json:"tick_damage"`
	ResistPierce   int     `json:"resist_pierce,omitempty"`
	IntervalFrames int     `json:"interval_frames"`
	TickCounter    int     `json:"tick_counter,omitempty"`
}

func buildSteamZoneSaves(zones []SteamZone) []SteamZoneSave {
	if len(zones) == 0 {
		return nil
	}
	out := make([]SteamZoneSave, len(zones))
	for i, z := range zones {
		out[i] = SteamZoneSave{
			SpellID: z.SpellID, MapKey: z.MapKey, X: z.X, Y: z.Y, Radius: z.Radius,
			FramesLeft: z.FramesLeft, TickDamage: z.TickDamage, ResistPierce: z.ResistPierce,
			IntervalFrames: z.IntervalFrames, TickCounter: z.tickCounter,
		}
	}
	return out
}

// restoreSteamZones rebuilds zones from a save; legacy entries without a map
// are pinned to the map the save was made on (same migration as loot bags).
func restoreSteamZones(saves []SteamZoneSave, saveMapKey string) []SteamZone {
	if len(saves) == 0 {
		return nil
	}
	out := make([]SteamZone, len(saves))
	for i, s := range saves {
		mapKey := s.MapKey
		if mapKey == "" {
			mapKey = saveMapKey
		}
		out[i] = SteamZone{
			SpellID: s.SpellID, MapKey: mapKey, X: s.X, Y: s.Y, Radius: s.Radius,
			FramesLeft: s.FramesLeft, TickDamage: s.TickDamage, ResistPierce: s.ResistPierce,
			IntervalFrames: s.IntervalFrames, tickCounter: s.TickCounter,
		}
	}
	return out
}

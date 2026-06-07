package game

import (
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// SteamZone is a fixed-position persistent damage field (Hot Steam). It is
// spawned at the party's location on cast and, each tick, sears every monster
// within Radius. Lifetime (FramesLeft) counts down in real frames in BOTH modes
// (like the other timed buffs); damage ticks every IntervalFrames in real-time
// and once per monster turn in turn-based.
type SteamZone struct {
	SpellID        string
	X, Y           float64 // world center (fixed at cast)
	Radius         float64 // pixels
	FramesLeft     int     // total lifetime remaining (frames)
	TickDamage     int
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
	// in-game tooltip — same source of truth as every other timed spell.
	frames := cs.CalculateSpellDurationFrames(spellID, caster)
	cs.game.steamZones = append(cs.game.steamZones, SteamZone{
		SpellID:        string(spellID),
		X:              cs.game.camera.X,
		Y:              cs.game.camera.Y,
		Radius:         def.ZoneRadiusTiles * tile,
		FramesLeft:     frames,
		TickDamage:     cs.CalculateSteamZoneTickDamage(def, caster),
		IntervalFrames: interval,
	})
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
	dmgType := convertToMonsterDamageType("water")
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		if Distance(z.X, z.Y, m.X, m.Y) > z.Radius {
			continue
		}
		m.TakeDamageResist(z.TickDamage, dmgType, 0, cs.game.camera.X, cs.game.camera.Y)
		m.HitTintFrames = MonsterHitFlashFrames
		cs.breakPacifyOnHit(m)
		cs.engageTurnBasedPackOnHit(m)
		cs.game.spawnSteamPuff(m.X, m.Y)
		if !m.IsAlive() {
			cs.game.collisionSystem.UnregisterEntity(m.ID)
			cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, m.ID)
			cs.awardExperienceAndGold(m)
		}
	}
}

// updateSteamZonesRT ticks every zone once per frame: counts down its lifetime
// (both modes) and, in real-time, applies damage on its cadence plus ambient
// steam VFX. Expired zones are dropped and their HUD status cleared.
func (gl *GameLoop) updateSteamZonesRT() {
	zones := gl.game.steamZones
	if len(zones) == 0 {
		return
	}
	w := 0
	for i := range zones {
		z := &zones[i]
		z.FramesLeft--
		if z.FramesLeft <= 0 {
			gl.game.updateUtilityStatus(spells.SpellID(z.SpellID), 0, false)
			continue
		}
		gl.game.updateUtilityStatus(spells.SpellID(z.SpellID), z.FramesLeft, true)

		if !gl.game.turnBasedMode {
			z.tickCounter++
			if z.tickCounter >= z.IntervalFrames {
				z.tickCounter = 0
				gl.combat.damageSteamZoneOnce(z)
			}
		}
		// Ambient steam is now a per-tile procedural bubble field drawn each
		// frame (Renderer.drawSteamZoneBubbles) — no sparse particle spawns here.
		zones[w] = *z
		w++
	}
	gl.game.steamZones = zones[:w]
}

// tickSteamZonesTB applies one steam damage tick per zone — called once per
// monster turn in turn-based combat.
func (gl *GameLoop) tickSteamZonesTB() {
	for i := range gl.game.steamZones {
		gl.combat.damageSteamZoneOnce(&gl.game.steamZones[i])
	}
}

// spawnSteamPuff emits a small cluster of rising, fading whitish-gray particles
// at a point — the look of scalding steam.
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
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Radius         float64 `json:"radius"`
	FramesLeft     int     `json:"frames_left"`
	TickDamage     int     `json:"tick_damage"`
	IntervalFrames int     `json:"interval_frames"`
}

func buildSteamZoneSaves(zones []SteamZone) []SteamZoneSave {
	if len(zones) == 0 {
		return nil
	}
	out := make([]SteamZoneSave, len(zones))
	for i, z := range zones {
		out[i] = SteamZoneSave{z.SpellID, z.X, z.Y, z.Radius, z.FramesLeft, z.TickDamage, z.IntervalFrames}
	}
	return out
}

func restoreSteamZones(saves []SteamZoneSave) []SteamZone {
	if len(saves) == 0 {
		return nil
	}
	out := make([]SteamZone, len(saves))
	for i, s := range saves {
		out[i] = SteamZone{
			SpellID: s.SpellID, X: s.X, Y: s.Y, Radius: s.Radius,
			FramesLeft: s.FramesLeft, TickDamage: s.TickDamage, IntervalFrames: s.IntervalFrames,
		}
	}
	return out
}

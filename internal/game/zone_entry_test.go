package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// A monster that WALKS INTO a damage zone must be burned on the spot - waiting
// for the next periodic tick let fast mobs cross a firewall for free
// (user-reported). Standing inside must still cost exactly one tick per interval.
func TestZone_DamagesOnEntryNotOnlyOnTick(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	prevMC := monsterPkg.MonsterConfig
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	defer func() { monsterPkg.MonsterConfig = prevMC }()
	g.world = newTestWorldSized(g.config, 20, 20)
	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = 5.5*ts, 5.5*ts
	g.camera.Angle = 0

	def, err := spells.GetSpellDefinitionByID(spells.SpellID("firewall"))
	if err != nil {
		t.Fatalf("firewall: %v", err)
	}
	g.persistentDamageZones = g.persistentDamageZones[:0]
	if !cs.tryCastPersistentDamageZone(spells.SpellID("firewall"), def, g.party.Members[0]) {
		t.Fatal("cast not handled")
	}
	z := &g.persistentDamageZones[1] // the middle cell, dead ahead
	gl := &GameLoop{game: g}

	// A monster far away takes nothing from the entry pass.
	mob := monsterPkg.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "goblin", g.config)
	if mob == nil {
		t.Skip("goblin fixture unavailable")
	}
	g.world.Monsters = append(g.world.Monsters, mob)
	full := mob.HitPoints
	gl.applyZoneEntryDamageAll()
	if mob.HitPoints != full {
		t.Fatalf("a monster outside the zone lost %d HP", full-mob.HitPoints)
	}

	// Step it into the cell: the very next frame must hurt, with no tick elapsed.
	mob.X, mob.Y = z.X, z.Y
	gl.applyZoneEntryDamageAll()
	afterEntry := mob.HitPoints
	if afterEntry >= full {
		t.Fatalf("entering the zone dealt no damage (HP %d -> %d)", full, afterEntry)
	}

	// Standing still does NOT re-burn it every frame.
	gl.applyZoneEntryDamageAll()
	if mob.HitPoints != afterEntry {
		t.Fatalf("standing inside burned again within the same tick window (%d -> %d)", afterEntry, mob.HitPoints)
	}

	// Once the interval elapses, the periodic tick burns it again.
	gl.advancePersistentDamageZones(z.IntervalFrames)
	if mob.HitPoints >= afterEntry {
		t.Fatalf("the periodic tick dealt no damage (HP stayed %d)", mob.HitPoints)
	}
}

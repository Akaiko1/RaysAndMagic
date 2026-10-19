package game

import (
	"fmt"
	"math"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func (g *MMGame) beaconSlots() int {
	d, err := spells.GetSpellDefinitionByID(g.townPortalSpell)
	if err != nil || d.Mechanic == nil || g.townPortalCaster == nil {
		return 0
	}
	return d.Mechanic.Charges(character.SpellMasteryTier(g.townPortalCaster, d))
}

func (g *MMGame) currentBeacon() (character.SpellBeacon, bool) {
	if world.GlobalWorldManager == nil || g.camera == nil || g.adventureArenaBounds().Enabled {
		return character.SpellBeacon{}, false
	}
	key, x, y := g.canonicalPosition(g.camera.X, g.camera.Y)
	px, py := world.GlobalWorldManager.ProjectWorldPos(key, x, y)
	// Region localization clamps seam corridors; a mark must preserve the actual pose.
	if math.Abs(px-g.camera.X) > 0.001 || math.Abs(py-g.camera.Y) > 0.001 {
		return character.SpellBeacon{}, false
	}
	b := character.SpellBeacon{MapKey: key, X: x, Y: y, Angle: world.GlobalWorldManager.LocalizeAngle(key, g.camera.Angle)}
	if visit := g.adventure.Visits[key]; visit != nil {
		b.Generation = visit.Generation
	}
	_, err := g.beaconRequest(b)
	return b, err == nil
}

// beaconRequest resolves the saved local pose without mutating worlds or paying.
// A beacon never enters a fresh chamber: only its still-valid generation.
func (g *MMGame) beaconRequest(b character.SpellBeacon) (mapTransition, error) {
	bad := func() (mapTransition, error) {
		return mapTransition{}, fmt.Errorf("%s", uitext.Text("advanced.beacon_unavailable"))
	}
	wm := world.GlobalWorldManager
	if wm == nil || b.MapKey == "" || g.adventureArenaBounds().Enabled || (b.MapKey == underwaterMapKey && !g.waterBreathingActive) {
		return bad()
	}
	for _, value := range []float64{b.X, b.Y, b.Angle} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return bad()
		}
	}
	w := g.worldByKey(b.MapKey)
	if w == nil {
		return bad()
	}
	if policy := g.adventureConfig(b.MapKey); policy != nil && policy.OpeningOwned {
		v := g.adventure.Visits[b.MapKey]
		if v == nil || v.Generation != b.Generation || v.Opening == "" {
			return bad()
		}
		if policy.ResetDays > 0 {
			if v.BossGranted && g.adventureAvailable(b.MapKey, g.currentQuestDay()) {
				return bad()
			}
		} else if v.Opening != g.openingID() {
			return bad()
		}
	}
	ts := float64(g.config.GetTileSize())
	if !wm.SourceTile(b.MapKey, TileIndex(b.X, ts), TileIndex(b.Y, ts)) {
		return bad()
	}
	x, y := wm.ProjectWorldPos(b.MapKey, b.X, b.Y)
	tx, ty := TileIndex(x, ts), TileIndex(y, ts)
	if tx < 0 || ty < 0 || tx >= w.Width || ty >= w.Height || w.IsTileBlockingTerrainAt(tx, ty) {
		return bad()
	}
	if livingMonsterOnTile(w, tx, ty, ts, nil) != nil {
		return bad()
	}
	for _, n := range w.NPCs {
		if n != nil && g.npcIsDoor(n) && !g.npcDoorOpen(n) && TileIndex(n.X, ts) == tx && TileIndex(n.Y, ts) == ty {
			return bad()
		}
	}
	return mapTransition{mapKey: b.MapKey, arrival: mapArrivalPose, pose: MapPose{X: x, Y: y, Angle: wm.ProjectAngle(b.MapKey, b.Angle)}}, nil
}

func (g *MMGame) confirmBeacon(slot int, bind bool) {
	if !g.beaconPicker || slot < 0 || slot >= g.beaconSlots() {
		return
	}
	caster, id := g.townPortalCaster, g.townPortalSpell
	idx := g.combat.findCharacterIndex(caster)
	if caster == nil || !g.canSpendCombatAction(idx) {
		return
	}
	var mark character.SpellBeacon
	var request mapTransition
	if bind {
		var valid bool
		mark, valid = g.currentBeacon()
		if !valid {
			g.AddCombatMessage(uitext.Text("advanced.beacon_unavailable"))
			return
		}
	} else {
		if slot >= len(caster.SpellState.Beacons) {
			return
		}
		var err error
		request, err = g.beaconRequest(caster.SpellState.Beacons[slot])
		if err != nil {
			g.AddCombatMessage(err.Error())
			return
		}
	}
	def, err := spells.GetSpellDefinitionByID(id)
	if err != nil {
		return
	}
	g.beaconConfirming = true
	cast := g.commitPickedSpell(caster, id, def)
	g.beaconConfirming = false
	if !cast {
		return
	}
	if bind {
		for len(caster.SpellState.Beacons) <= slot {
			caster.SpellState.Beacons = append(caster.SpellState.Beacons, character.SpellBeacon{})
		}
		caster.SpellState.Beacons[slot] = mark
	}
	g.cancelTownPortalPicker()
	g.menuOpen = false
	if !bind {
		if err := g.transitionToMap(request); err != nil {
			g.AddCombatMessage(err.Error())
		}
	}
}

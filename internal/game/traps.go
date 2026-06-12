package game

import (
	"fmt"
	"math"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// Trap mechanics (thief trap book). A placed trap is a one-shot tile device:
// it arms on its tile, shows a particle swirl + a colored tile-edge glow, and
// fires when a monster occupies the tile — damage traps scale with the OWNER's
// Intellect+Accuracy and Trapper mastery at trigger time, control traps
// stun/root for a mastery-extended duration. Traps are map-scoped (MapKey) and
// persist in saves; the owner is a live pointer (like Arrow.Attacker) so
// roster swaps mid-arm don't re-point it.

const (
	// Canonical values live in config/traps.go (the editor cards quote them).
	MaxTrapsPerOwner    = config.MaxTrapsPerOwner
	TrapPlaceRangeTiles = config.TrapPlaceRangeTiles
	// trapSwirlPeriodTicks is the cadence of the ambient swirl particle spawn.
	trapSwirlPeriodTicks = 9
)

// PlacedTrap is one armed trap on a map tile.
type PlacedTrap struct {
	Key          string // traps.yaml key
	MapKey       string
	TileX, TileY int
	X, Y         float64                // tile center (render/VFX anchor)
	Owner        *character.MMCharacter // scaling + per-owner limit; nil after failed save resolve
	FramesLeft   int                    // armed lifetime; the trap despawns at 0
	swirlTick    int
}

// trapAt returns the index of the trap occupying a tile on the current map, or -1.
func (g *MMGame) trapAt(tileX, tileY int) int {
	mapKey := currentMapKey()
	for i := range g.traps {
		if g.traps[i].MapKey == mapKey && g.traps[i].TileX == tileX && g.traps[i].TileY == tileY {
			return i
		}
	}
	return -1
}

// ownerTrapCount counts the character's armed traps on the current map.
func (g *MMGame) ownerTrapCount(owner *character.MMCharacter) int {
	mapKey := currentMapKey()
	n := 0
	for i := range g.traps {
		if g.traps[i].MapKey == mapKey && g.traps[i].Owner == owner {
			n++
		}
	}
	return n
}

// trapDamage computes a damage trap's payload for an owner at TRIGGER time:
// flat base + (Intellect+Accuracy)/divisor + Trapper mastery. The SAME
// function feeds the trap-book tooltip, so combat and UI can't drift.
func trapDamage(def *config.TrapDefinitionConfig, owner *character.MMCharacter) int {
	dmg := def.DamageBase
	if dmg <= 0 {
		return 0
	}
	if owner != nil {
		dmg += (owner.GetEffectiveIntellect() + owner.GetEffectiveAccuracy()) / character.TrapStatScalingDivisor
		dmg += owner.SkillTier(character.SkillTrapper) * character.TrapperDamagePerTier
	}
	return dmg
}

// trapControlDuration returns the mastery-extended control duration of a trap
// in TB turns and RT seconds (whichever pair the trap carries — stun or root).
func trapControlDuration(baseTurns, baseSeconds int, owner *character.MMCharacter) (turns, seconds int) {
	turns, seconds = baseTurns, baseSeconds
	if owner != nil {
		tier := owner.SkillTier(character.SkillTrapper)
		turns += tier * character.TrapperTurnsPerTier
		seconds += tier * character.TrapperSecondsPerTier
	}
	return turns, seconds
}

// equipTrap puts a trap into the character's quick slot. Refuses unknown keys
// and level-locked traps (the book shows LOCKED — equipping one would only
// fail later at placement).
func equipTrap(char *character.MMCharacter, key string) bool {
	def, ok := config.GetTrapDefinition(key)
	if !ok || char.Level < def.Level {
		return false
	}
	it, _ := config.TrapItem(key)
	char.Equipment[items.SlotSpell] = it
	return true
}

// equippedTrapKey returns the trap key armed in the quick slot, if any.
func equippedTrapKey(char *character.MMCharacter) (string, bool) {
	it, ok := char.Equipment[items.SlotSpell]
	if !ok || it.Type != items.ItemTrap {
		return "", false
	}
	return string(it.SpellEffect), true
}

// availableTraps returns the trap keys the character can use, in book order.
// Level-gated entries are included (the UI shows them locked); placement
// re-checks the gate.
func availableTraps(char *character.MMCharacter) []string {
	if char == nil || !char.HasSkill(character.SkillTrapper) {
		return nil
	}
	return config.TrapKeysOrdered()
}

// hasTrapBook reports whether the character uses the trap book in place of a
// magic spellbook (data-driven: carries the Trapper skill).
func hasTrapBook(char *character.MMCharacter) bool {
	return char != nil && char.HasSkill(character.SkillTrapper)
}

// tryPlaceQuickTrap places the caster's selected trap. Target tile: step from
// the party tile along the facing direction up to TrapPlaceRangeTiles — the
// first tile holding a monster wins ("right under its feet"), a wall stops the
// throw at the previous tile, otherwise it lands at max range. Returns the
// trap key on success (RT cooldown resolves from it).
//
// announce gates the FAILURE messages: Space (SmartAttack) probes the trap
// silently and falls through to the weapon — matching quick spells, whose
// canPay pre-check is equally quiet; the explicit F cast keeps the messages.
func (cs *CombatSystem) tryPlaceQuickTrap(caster *character.MMCharacter, announce bool) (string, bool) {
	trapKey, armed := equippedTrapKey(caster)
	if !armed {
		return "", false
	}
	return cs.placeTrapByKey(caster, trapKey, announce)
}

// placeTrapByKey arms a SPECIFIC trap (the trap book's double-click casts the
// clicked entry, slotted or not) — gates and placement shared with the quick
// slot path.
func (cs *CombatSystem) placeTrapByKey(caster *character.MMCharacter, trapKey string, announce bool) (string, bool) {
	if caster == nil || !hasTrapBook(caster) {
		return "", false
	}
	def, ok := config.GetTrapDefinition(trapKey)
	if !ok {
		return "", false
	}
	refuse := func(msg string) (string, bool) {
		if announce {
			cs.game.AddCombatMessage(msg)
		}
		return "", false
	}
	if caster.Level < def.Level {
		return refuse(fmt.Sprintf("%s needs level %d for %s.", caster.Name, def.Level, def.Name))
	}
	spCost := cs.effectiveSpellCost(caster, def.SPCost)
	if caster.SpellPoints < spCost {
		return refuse(fmt.Sprintf("%s's %s fizzles! (Not enough SP: %d/%d)",
			caster.Name, def.Name, caster.SpellPoints, spCost))
	}
	if cs.game.ownerTrapCount(caster) >= MaxTrapsPerOwner {
		return refuse(fmt.Sprintf("%s already has %d traps armed.", caster.Name, MaxTrapsPerOwner))
	}

	tileX, tileY, ok := cs.pickTrapTile()
	if !ok {
		return refuse("No room to place a trap there.")
	}
	if cs.game.trapAt(tileX, tileY) >= 0 {
		return refuse("There is already a trap on that tile.")
	}

	caster.SpellPoints -= spCost
	ts := float64(cs.game.config.GetTileSize())
	cx, cy := TileCenterFromTile(tileX, tileY, ts)
	cs.game.traps = append(cs.game.traps, PlacedTrap{
		Key: trapKey, MapKey: currentMapKey(),
		TileX: tileX, TileY: tileY, X: cx, Y: cy, Owner: caster,
		FramesLeft: def.LifetimeSeconds * cs.game.config.GetTPS(),
	})
	cs.game.AddCombatMessage(fmt.Sprintf("%s arms a %s!", caster.Name, def.Name))
	cs.game.spawnTrapSwirl(cx, cy, def.Element)
	// A trap thrown under a monster's feet fires immediately (TB has no
	// per-frame sweep; in RT the next frame's sweep would catch it anyway).
	cs.sweepTrapTriggers()
	return trapKey, true
}

// pickTrapTile walks tile-by-tile from the party along the camera facing.
// First tile with a living monster wins; a blocking tile stops the walk at the
// previous tile (which may be the party's own — refused); otherwise max range.
func (cs *CombatSystem) pickTrapTile() (int, int, bool) {
	ts := float64(cs.game.config.GetTileSize())
	dirX, dirY := math.Cos(cs.game.camera.Angle), math.Sin(cs.game.camera.Angle)
	curX, curY := int(cs.game.camera.X/ts), int(cs.game.camera.Y/ts)
	lastX, lastY := curX, curY

	for step := 1; step <= TrapPlaceRangeTiles; step++ {
		tx := int((cs.game.camera.X + dirX*float64(step)*ts) / ts)
		ty := int((cs.game.camera.Y + dirY*float64(step)*ts) / ts)
		if tx == lastX && ty == lastY {
			continue
		}
		w := cs.game.world
		if w == nil || tx < 0 || ty < 0 || tx >= w.Width || ty >= w.Height || w.IsTileBlocking(tx, ty) {
			break // wall/out of bounds: settle on the previous tile
		}
		lastX, lastY = tx, ty
		if cs.monsterOnTile(tx, ty) != nil {
			return tx, ty, true // right under its feet
		}
	}
	if lastX == curX && lastY == curY {
		return 0, 0, false // facing straight into a wall
	}
	return lastX, lastY, true
}

// monsterOnTile returns a living monster occupying the tile, or nil.
func (cs *CombatSystem) monsterOnTile(tileX, tileY int) *monsterPkg.Monster3D {
	ts := float64(cs.game.config.GetTileSize())
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		if int(m.X/ts) == tileX && int(m.Y/ts) == tileY {
			return m
		}
	}
	return nil
}

// sweepTrapTriggers fires every trap on the current map that a living monster
// is standing on. RT runs it each frame; TB runs it after monster moves and
// right after placement.
func (cs *CombatSystem) sweepTrapTriggers() {
	if len(cs.game.traps) == 0 {
		return
	}
	mapKey := currentMapKey()
	w := 0
	for i := range cs.game.traps {
		t := cs.game.traps[i]
		if t.MapKey == mapKey {
			if victim := cs.monsterOnTile(t.TileX, t.TileY); victim != nil {
				cs.fireTrap(&t, victim)
				continue // one-shot: drop the trap
			}
		}
		cs.game.traps[w] = t
		w++
	}
	cs.game.traps = cs.game.traps[:w]
}

// fireTrap applies a trap's payload to the victim (and, for AoE, everything
// in radius), with messages and burst VFX.
func (cs *CombatSystem) fireTrap(t *PlacedTrap, victim *monsterPkg.Monster3D) {
	def, ok := config.GetTrapDefinition(t.Key)
	if !ok {
		return
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s springs under %s!", def.Name, victim.Name))
	cs.game.CreateSpellHitEffect(t.X, t.Y, def.Element, 0, 0)

	if dmg := trapDamage(def, t.Owner); dmg > 0 {
		dmgType := convertToMonsterDamageType(def.Element)
		if def.AoeRadiusTiles > 0 {
			radius := def.AoeRadiusTiles * float64(cs.game.config.GetTileSize())
			for _, m := range cs.game.world.Monsters {
				if m == nil || !m.IsAlive() || Distance(t.X, t.Y, m.X, m.Y) > radius {
					continue
				}
				cs.applyTrapDamage(m, dmg, def.Element, dmgType, def.Name)
			}
		} else {
			cs.applyTrapDamage(victim, dmg, def.Element, dmgType, def.Name)
		}
	}

	turnsStun, secsStun := trapControlDuration(def.StunTurns, def.StunSeconds, t.Owner)
	if def.StunTurns > 0 {
		if cs.game.turnBasedMode {
			if turnsStun > victim.StunTurnsRemaining {
				victim.StunTurnsRemaining = turnsStun
			}
		} else if frames := secsStun * cs.game.config.GetTPS(); frames > victim.StunFramesRemaining {
			victim.StunFramesRemaining = frames
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s is stunned!", victim.Name))
	}

	turnsRoot, secsRoot := trapControlDuration(def.RootTurns, def.RootSeconds, t.Owner)
	if def.RootTurns > 0 {
		if cs.game.turnBasedMode {
			if turnsRoot > victim.RootTurnsRemaining {
				victim.RootTurnsRemaining = turnsRoot
			}
		} else if frames := secsRoot * cs.game.config.GetTPS(); frames > victim.RootFramesRemaining {
			victim.RootFramesRemaining = frames
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s is pinned in place!", victim.Name))
	}
}

// applyTrapDamage lands trap damage on one monster with the shared indirect-
// damage bookkeeping (hit flash, charm break, pack aggro, kill credit).
func (cs *CombatSystem) applyTrapDamage(m *monsterPkg.Monster3D, dmg int, element string, dmgType monsterPkg.DamageType, sourceName string) {
	// Physical traps respect monster armor like any other physical hit;
	// elemental payloads go through resistances only.
	dmg = applyArmorReductionIfPhysical(dmg, element, m.ArmorClass, false)
	actual := m.TakeDamageResist(dmg, dmgType, 0, cs.game.camera.X, cs.game.camera.Y)
	m.HitTintFrames = MonsterHitFlashFrames
	cs.breakPacifyOnHit(m)
	cs.engageTurnBasedPackOnHit(m)
	cs.game.AddCombatMessage(fmt.Sprintf("%s takes %d damage from %s!", m.Name, actual, sourceName))
	cs.finishIndirectKill(m)
}

// finishIndirectKill handles a monster death from an autonomous source (trap,
// steam zone): collision cleanup, death sweep registration, XP and gold.
func (cs *CombatSystem) finishIndirectKill(m *monsterPkg.Monster3D) {
	if m.IsAlive() {
		return
	}
	cs.game.collisionSystem.UnregisterEntity(m.ID)
	cs.game.deadMonsterIDs = append(cs.game.deadMonsterIDs, m.ID)
	cs.awardExperienceAndGold(m)
}

// updateTraps runs once per frame from the game loop: ambient swirl VFX in
// both modes, trigger sweep in real-time (TB sweeps after monster moves).
func (gl *GameLoop) updateTraps() {
	g := gl.game
	if len(g.traps) == 0 {
		return
	}
	mapKey := currentMapKey()
	w := 0
	for i := range g.traps {
		t := g.traps[i]
		// Lifetime ticks on every map (armed steel doesn't care where you are).
		t.FramesLeft--
		if t.FramesLeft <= 0 {
			if t.MapKey == mapKey {
				if def, ok := config.GetTrapDefinition(t.Key); ok {
					g.spawnTrapSwirl(t.X, t.Y, def.Element) // fizzle puff
				}
			}
			continue // expired: drop
		}
		if t.MapKey == mapKey {
			t.swirlTick++
			if t.swirlTick >= trapSwirlPeriodTicks {
				t.swirlTick = 0
				if def, ok := config.GetTrapDefinition(t.Key); ok {
					g.spawnTrapSwirl(t.X, t.Y, def.Element)
				}
			}
		}
		g.traps[w] = t
		w++
	}
	g.traps = g.traps[:w]
	if !g.turnBasedMode {
		gl.combat.sweepTrapTriggers()
	}
}

// spawnTrapSwirl emits the armed-trap "vortex": particles anchored at WORLD
// positions on a ring around the tile center (screen offsets would drag the
// swirl with the camera). Rotation comes from advancing the spawn phase with
// the frame counter; short lifetimes keep the ring visibly turning. No impact
// light (it runs every few ticks; a light would strobe the floor).
func (g *MMGame) spawnTrapSwirl(x, y float64, element string) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()

	baseColor, ok := ElementColors[element]
	if !ok {
		baseColor = ElementColors["physical"]
	}
	const n = 3
	const ringRadius = 13.0 // world units around the tile center
	phase := float64(g.frameCount) * 0.11
	particles := make([]SpellHitParticle, 0, n)
	for i := 0; i < n; i++ {
		ang := phase + float64(i)/n*2*math.Pi
		life := 22 + i*2
		particles = append(particles, SpellHitParticle{
			// World anchor ON the ring: the particle is pinned to the map.
			X:        x + math.Cos(ang)*ringRadius,
			Y:        y + math.Sin(ang)*ringRadius,
			OffsetY:  -2,
			VelY:     -0.35, // gentle rise above the floor
			Gravity:  -0.01,
			Color:    baseColor,
			LifeTime: life, MaxLife: life,
			Size: 3, Active: true,
		})
	}
	g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Particles: particles, Active: true})
}

// TrapSave is the JSON form of a PlacedTrap. The owner is stored by NAME and
// re-pointed at load (party names are unique); an unresolvable owner leaves
// nil — the trap still fires at base values.
type TrapSave struct {
	Key        string  `json:"key"`
	MapKey     string  `json:"map_key"`
	TileX      int     `json:"tile_x"`
	TileY      int     `json:"tile_y"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Owner      string  `json:"owner,omitempty"`
	FramesLeft int     `json:"frames_left,omitempty"`
}

func buildTrapSaves(traps []PlacedTrap) []TrapSave {
	if len(traps) == 0 {
		return nil
	}
	out := make([]TrapSave, len(traps))
	for i, t := range traps {
		ts := TrapSave{Key: t.Key, MapKey: t.MapKey, TileX: t.TileX, TileY: t.TileY, X: t.X, Y: t.Y, FramesLeft: t.FramesLeft}
		if t.Owner != nil {
			ts.Owner = t.Owner.Name
		}
		out[i] = ts
	}
	return out
}

func restoreTraps(saves []TrapSave, party *character.Party) []PlacedTrap {
	if len(saves) == 0 {
		return nil
	}
	byName := map[string]*character.MMCharacter{}
	if party != nil {
		for _, m := range party.Members {
			if m != nil {
				byName[m.Name] = m
			}
		}
		for _, m := range party.Reserve {
			if m != nil {
				byName[m.Name] = m
			}
		}
	}
	out := make([]PlacedTrap, len(saves))
	for i, s := range saves {
		left := s.FramesLeft
		if left <= 0 { // legacy save without lifetime: re-arm fresh
			if def, ok := config.GetTrapDefinition(s.Key); ok && config.GlobalConfig != nil {
				left = def.LifetimeSeconds * config.GlobalConfig.GetTPS()
			}
		}
		out[i] = PlacedTrap{
			Key: s.Key, MapKey: s.MapKey, TileX: s.TileX, TileY: s.TileY,
			X: s.X, Y: s.Y, Owner: byName[s.Owner], FramesLeft: left,
		}
	}
	return out
}

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
// it arms on its tile, shows a grounded shader model, and
// fires when a monster occupies the tile - damage traps scale with the OWNER's
// Intellect+Accuracy and Trapper mastery at trigger time, control traps
// stun/root for a mastery-extended duration. Traps are map-scoped (MapKey) and
// persist in saves; the owner is a live pointer (like Arrow.Attacker) so
// roster swaps mid-arm don't re-point it.

const (
	// Canonical values live in config/traps.go (the editor cards quote them).
	MaxTrapsPerOwner    = config.MaxTrapsPerOwner
	TrapPlaceRangeTiles = config.TrapPlaceRangeTiles
)

// PlacedTrap is one armed trap on a map tile.
type PlacedTrap struct {
	Key          string // traps.yaml key
	MapKey       string
	TileX, TileY int
	X, Y         float64                // tile center (render/VFX anchor)
	Owner        *character.MMCharacter // scaling + per-owner limit; nil after failed save resolve
	FramesLeft   int                    // armed lifetime; the trap despawns at 0
}

// trapAt returns the index of the trap occupying a tile on the current world, or -1.
func (g *MMGame) trapAt(tileX, tileY int) int {
	for i := range g.traps {
		if mapKeyOnCurrentWorld(g.traps[i].MapKey) && g.traps[i].TileX == tileX && g.traps[i].TileY == tileY {
			return i
		}
	}
	return -1
}

// ownerTrapCount counts the character's armed traps on the current world.
func (g *MMGame) ownerTrapCount(owner *character.MMCharacter) int {
	n := 0
	for i := range g.traps {
		if mapKeyOnCurrentWorld(g.traps[i].MapKey) && g.traps[i].Owner == owner {
			n++
		}
	}
	return n
}

// trapDamage computes a damage trap's payload for an owner at TRIGGER time:
// flat base + (Intellect+Accuracy)/divisor + Trapper mastery. The SAME
// function feeds the trap-book tooltip, so combat and UI can't drift.
func trapDamage(def *config.TrapDefinitionConfig, owner *character.MMCharacter) int {
	if def.DamageBase <= 0 || owner == nil {
		return trapDamageAtTier(def, 0)
	}
	stat := (owner.GetEffectiveIntellect() + owner.GetEffectiveAccuracy()) / character.TrapStatScalingDivisor
	return trapDamageAtTier(def, owner.SkillTier(character.SkillTrapper)) + stat
}

// trapDamageAtTier is a trap's damage for a statless owner at a Trapper tier.
func trapDamageAtTier(def *config.TrapDefinitionConfig, tier int) int {
	if def.DamageBase <= 0 {
		return 0
	}
	return def.DamageBase + tier*character.TrapperDamagePerTier
}

// trapControlDuration returns the mastery-extended control duration of a trap
// in TB turns and RT seconds (whichever pair the trap carries - stun or root).
func trapControlDuration(baseTurns, baseSeconds int, owner *character.MMCharacter) (turns, seconds int) {
	if owner == nil {
		return baseTurns, baseSeconds
	}
	return trapControlDurationAtTier(baseTurns, baseSeconds, owner.SkillTier(character.SkillTrapper))
}

func trapControlDurationAtTier(baseTurns, baseSeconds, tier int) (turns, seconds int) {
	return baseTurns + character.TrapperTurnBonus(tier), baseSeconds + tier*character.TrapperSecondsPerTier
}

// equipTrap puts a trap into the character's quick slot. Refuses unknown keys
// and level-locked traps (the book shows LOCKED - equipping one would only
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
// the party tile along the facing direction up to TrapPlaceRangeTiles - the
// first tile holding a monster wins ("right under its feet"), a wall stops the
// throw at the previous tile, otherwise it lands at max range. Returns the
// trap key on success (RT cooldown resolves from it).
//
// announce gates the FAILURE messages: Space (SmartAttack) probes the trap
// silently and falls through to the weapon - matching quick spells, whose
// canPay pre-check is equally quiet; the explicit F cast keeps the messages.
func (cs *CombatSystem) tryPlaceQuickTrap(caster *character.MMCharacter, announce bool) (string, bool) {
	trapKey, armed := equippedTrapKey(caster)
	if !armed {
		return "", false
	}
	return cs.placeTrapByKey(caster, trapKey, announce)
}

// quickTrapAvailable reports whether the slotted trap passes every gate except
// the tile, which depends on where the party faces.
func (cs *CombatSystem) quickTrapAvailable(caster *character.MMCharacter) bool {
	trapKey, armed := equippedTrapKey(caster)
	if !armed || !caster.CanUseCombatAction() || !hasTrapBook(caster) {
		return false
	}
	def, ok := config.GetTrapDefinition(trapKey)
	return ok && cs.trapRefusal(caster, def) == ""
}

// trapRefusal is the caster-side placement gate; "" means the trap may be armed.
func (cs *CombatSystem) trapRefusal(caster *character.MMCharacter, def *config.TrapDefinitionConfig) string {
	if caster.Level < def.Level {
		return fmt.Sprintf("%s needs level %d for %s.", caster.Name, def.Level, def.Name)
	}
	if spCost := cs.effectiveSpellCost(caster, def.SPCost); caster.SpellPoints < spCost {
		return fmt.Sprintf("%s's %s fizzles! (Not enough SP: %d/%d)", caster.Name, def.Name, caster.SpellPoints, spCost)
	}
	if cs.game.ownerTrapCount(caster) >= MaxTrapsPerOwner {
		return fmt.Sprintf("%s already has %d traps armed.", caster.Name, MaxTrapsPerOwner)
	}
	return ""
}

// placeTrapByKey arms a specific trap (Enter/F in the book uses the selected
// entry, slotted or not). Gates and placement are shared with the quick slot.
func (cs *CombatSystem) placeTrapByKey(caster *character.MMCharacter, trapKey string, announce bool) (string, bool) {
	if !caster.CanUseCombatAction() || !hasTrapBook(caster) {
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
	if msg := cs.trapRefusal(caster, def); msg != "" {
		return refuse(msg)
	}
	spCost := cs.effectiveSpellCost(caster, def.SPCost)

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
	cs.game.logCombat(logToneGood, "%s arms a %s!", logHeroName(caster), logAbility(def.Name))
	// A trap thrown under a monster's feet fires immediately (TB has no
	// per-frame sweep; in RT the next frame's sweep would catch it anyway).
	cs.sweepTrapTriggers()
	return trapKey, true
}

// pickTrapTile walks tile-by-tile from the party along the camera facing.
// First tile with a living monster wins; a blocking tile stops the walk at the
// previous tile (which may be the party's own - refused); otherwise max range.
func (cs *CombatSystem) pickTrapTile() (int, int, bool) {
	ts := float64(cs.game.config.GetTileSize())
	dirX, dirY := math.Cos(cs.partyAttackAngle()), math.Sin(cs.partyAttackAngle())
	curX, curY := TileIndex(cs.game.camera.X, ts), TileIndex(cs.game.camera.Y, ts)
	lastX, lastY := curX, curY

	for step := 1; step <= TrapPlaceRangeTiles; step++ {
		tx := TileIndex((cs.game.camera.X + dirX*float64(step)*ts), ts)
		ty := TileIndex((cs.game.camera.Y + dirY*float64(step)*ts), ts)
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
	// Nothing dead-ahead: a front-diagonal monster pulled to screen-center is a
	// valid melee target, so drop the trap under its REAL tile (traps are
	// world-space; the sweep fires on the monster's true position). Closest
	// pulled flank wins - mirrors the melee front->side priority.
	if mon := cs.nearestPulledFlankMonster(); mon != nil {
		return TileIndex(mon.X, ts), TileIndex(mon.Y, ts), true
	}
	if lastX == curX && lastY == curY {
		return 0, 0, false // facing straight into a wall
	}
	return lastX, lastY, true
}

// nearestPulledFlankMonster returns the closest monster currently pulled onto a
// turn-based front DIAGONAL slot (drawn at screen-center), or nil. Uses the
// pulledFrontSlot SSoT so trap auto-targeting matches what the player sees, and
// the party auto-target policy so a trap never lands under an ally.
func (cs *CombatSystem) nearestPulledFlankMonster() *monsterPkg.Monster3D {
	if cs.partyAimTarget != nil {
		return nil // explicit aim must not acquire a different pulled flank
	}
	var best *monsterPkg.Monster3D
	var bestD float64
	for _, m := range cs.game.world.Monsters {
		if isExcludedFromPartyAutoTarget(m) || !m.IsAlive() {
			continue
		}
		side, _, _, pulled, ok := cs.pulledFrontSlot(m)
		if !ok || !pulled || side == 0 {
			continue // only genuinely pulled diagonals (dead-ahead handled above)
		}
		d := DistanceSquared(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y)
		if best == nil || d < bestD {
			best, bestD = m, d
		}
	}
	return best
}

// monsterOnTile returns a living monster occupying the tile, or nil.
func (cs *CombatSystem) monsterOnTile(tileX, tileY int) *monsterPkg.Monster3D {
	ts := float64(cs.game.config.GetTileSize())
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || isPurePartySummon(m) {
			continue
		}
		if TileIndex(m.X, ts) == tileX && TileIndex(m.Y, ts) == tileY {
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
	w := 0
	for i := range cs.game.traps {
		t := cs.game.traps[i]
		if mapKeyOnCurrentWorld(t.MapKey) {
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
	if isPurePartySummon(victim) {
		return
	}
	def, ok := config.GetTrapDefinition(t.Key)
	if !ok {
		return
	}
	cs.game.logCombat(logToneGood, "%s springs under %s!", logAbility(def.Name), logMonsterName(victim))
	cs.game.startTrapBurst(*t)

	boundVictim := false
	if dmg := trapDamage(def, t.Owner); dmg > 0 {
		if def.AoeRadiusTiles > 0 {
			hurts := func(m *monsterPkg.Monster3D) bool { return !isPurePartySummon(m) }
			cs.forEachAreaVictim(cs.pointBlast(t.X, t.Y, def.AoeRadiusTiles), hurts, func(m *monsterPkg.Monster3D) {
				if cs.tryDarkElfBindInstead(t.Owner, m) {
					boundVictim = boundVictim || m == victim
					return
				}
				cs.applyTrapDamage(m, dmg, def.Element, def.Name)
			})
		} else {
			if cs.tryDarkElfBindInstead(t.Owner, victim) {
				boundVictim = true
			} else {
				cs.applyTrapDamage(victim, dmg, def.Element, def.Name)
			}
		}
	}

	if boundVictim {
		return
	}
	if def.DamageBase <= 0 && cs.tryDarkElfBindInstead(t.Owner, victim) {
		return
	}
	// A sealed / idol-warded boss is immune to indirect damage (gated inside
	// applyTrapDamage) - and to its control riders too. Skip stun/root for it.
	if victim.IsDamageInvulnerable() {
		return
	}

	turnsStun, secsStun := trapControlDuration(def.StunTurns, def.StunSeconds, t.Owner)
	if def.StunTurns > 0 {
		cs.applyStunDR(victim, turnsStun, secsStun*cs.game.config.GetTPS(), true) // announces stun/resist
	}

	turnsRoot, secsRoot := trapControlDuration(def.RootTurns, def.RootSeconds, t.Owner)
	if def.RootTurns > 0 {
		cs.applyMonsterRoot(victim, turnsRoot, secsRoot*cs.game.config.GetTPS())
	}
}

// applyTrapDamage lands trap damage on one monster with the shared indirect-
// damage bookkeeping (hit flash, charm break, pack aggro, kill credit).
func (cs *CombatSystem) applyTrapDamage(m *monsterPkg.Monster3D, dmg int, element string, sourceName string) {
	if isPurePartySummon(m) || m.IsDamageInvulnerable() {
		return // transparent summon or invulnerable boss: no damage, FX, or aggro
	}
	// Traps use the shared party packet builder so physical conversion stays one
	// hit and soak is paid once. Weapon/attack-only target modifiers do not apply;
	// this preserves the pre-refactor trap formula. Trap control stays undodgeable.
	packet := cs.newPartyMonsterDamagePacket(dmg, 0, element, 0, true)
	packet = cs.elementalBuffPacket(packet)
	actual := cs.applyMonsterDamagePacket(m, packet, monsterDamageOptions{}).Total()
	cs.reportIndirectHit(m, actual, sourceName)
	cs.finishIndirectKill(m)
}

// reportIndirectHit shows a trap/zone hit exactly like a weapon hit minus the
// view kick: flash, sparks, and a log line with damage and remaining HP. Kill
// credit stays with finishIndirectKill, which the caller runs next.
func (cs *CombatSystem) reportIndirectHit(m *monsterPkg.Monster3D, dealt int, sourceName string) {
	cs.markMonsterHit(m)
	cs.spawnHitSparks(m)
	cs.game.logCombat(logToneGood, "%s takes %s damage from %s! %s",
		logMonsterName(m), logDamage(dealt, ""), logAbility(sourceName), logHP(m.HitPoints, m.MaxHitPoints))
}

// finishIndirectKill handles a monster death from an autonomous source (trap,
// steam zone): collision cleanup, death sweep registration, XP and gold.
func (cs *CombatSystem) finishIndirectKill(m *monsterPkg.Monster3D) {
	if m.IsAlive() {
		return
	}
	cs.finishMonsterKillImmediately(m)
}

// updateTraps expires presentation bursts on the frame clock in either mode.
// Gameplay lifetimes and trigger sweeps retain their TB/RT clock rules.
func (gl *GameLoop) updateTraps() {
	g := gl.game
	g.advanceTrapLifetimes(g.combatFrameElapsed())
	w := 0
	for _, burst := range g.trapBursts {
		if float64(g.frameCount-burst.born) >= trapBurstSeconds*float64(g.config.GetTPS()) {
			continue
		}
		g.trapBursts[w] = burst
		w++
	}
	g.trapBursts = g.trapBursts[:w]
	if len(g.traps) > 0 && !g.turnBasedMode {
		g.combat.sweepTrapTriggers()
	}
}

// Lifetime is gameplay time on every map. Shader animation uses presentation
// time independently, including while the party is thinking in TB mode.
func (g *MMGame) advanceTrapLifetimes(elapsed int) {
	if elapsed <= 0 {
		return
	}
	w := 0
	for _, t := range g.traps {
		t.FramesLeft = max(0, t.FramesLeft-elapsed)
		if t.FramesLeft == 0 {
			continue
		}
		g.traps[w] = t
		w++
	}
	g.traps = g.traps[:w]
}

// TrapSave is the JSON form of a PlacedTrap. The owner is stored by NAME and
// re-pointed at load (party names are unique); an unresolvable owner leaves
// nil - the trap still fires at base values.
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

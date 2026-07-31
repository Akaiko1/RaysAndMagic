package game

import (
	"fmt"
	"math"
	"math/rand"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// The Brood Mother's fire-trap field. One boss owns one field: every volley
// interval she re-sows trap_volley_count fire traps on random walkable tiles
// within trap_volley_radius_tiles of herself, and the NEW volley replaces the
// old field (user spec). A party member's tile igniting consumes that trap and
// burns the whole party. Positions are tile coords on the current world.

// bossFireTrap is one armed tile of the field. Runtime coordinates belong to
// the current world; build/restore helpers convert them to/from map-local save
// coordinates through WorldManager's canonical tile transforms.
type bossFireTrap struct {
	MapKey string `json:"map_key,omitempty"`
	TX     int    `json:"tx"`
	TY     int    `json:"ty"`
}

type bossFireTrapTileTransform func(mapKey string, tx, ty int) (int, int)

func transformBossFireTraps(
	traps []bossFireTrap,
	fallbackMapKey string,
	transformUnkeyed bool,
	transform bossFireTrapTileTransform,
) []bossFireTrap {
	if len(traps) == 0 {
		return nil
	}
	out := make([]bossFireTrap, len(traps))
	for i, trap := range traps {
		hasMapKey := trap.MapKey != ""
		if !hasMapKey {
			trap.MapKey = fallbackMapKey
		}
		if transform != nil && (hasMapKey || transformUnkeyed) {
			trap.TX, trap.TY = transform(trap.MapKey, trap.TX, trap.TY)
		}
		out[i] = trap
	}
	return out
}

func buildBossFireTrapSaves(traps []bossFireTrap, fallbackMapKey string, wm *world.WorldManager) []bossFireTrap {
	if wm == nil {
		return transformBossFireTraps(traps, fallbackMapKey, true, nil)
	}
	return transformBossFireTraps(traps, fallbackMapKey, true, wm.LocalizeTile)
}

func restoreBossFireTraps(saves []bossFireTrap, fallbackMapKey string, wm *world.WorldManager) []bossFireTrap {
	if wm == nil {
		return transformBossFireTraps(saves, fallbackMapKey, false, nil)
	}
	// Entries without MapKey predate local-canon saves and already contain
	// runtime coordinates. Tag them for ownership without projecting twice.
	return transformBossFireTraps(saves, fallbackMapKey, false, wm.ProjectTile)
}

// tryBossTrapVolley is called from the boss action paths and owns both cadence
// counters: RT ticks frames, while TB ticks once per monster pass.
func (cs *CombatSystem) tryBossTrapVolley(m *monsterPkg.Monster3D, turnBased bool) {
	if m == nil || m.TrapVolleyCount <= 0 || !m.IsAlive() || cs.bossDisabled(m) || cs.bossEvasive(m) {
		return
	}
	// A provoked boss owns the field; a passive nesting mother sows nothing until
	// struck. Fighting a SUMMON counts - IsEngagingPlayer alone read that as calm.
	if !m.IsEngagingPlayer && !m.IsInCombat() {
		return
	}
	if turnBased {
		// Decrement THEN test, mirroring the RT frames counter: a volley fires
		// every interval-th pass, not every interval+1 (off-by-one otherwise).
		if m.TrapVolleyTurnCD > 0 {
			m.TickTrapVolleyCooldownTurn()
		}
		if m.TrapVolleyTurnCD > 0 {
			return
		}
	} else {
		if m.TrapVolleyCDFrames > 0 {
			m.TickTrapVolleyCooldownFrame()
		}
		if m.TrapVolleyCDFrames > 0 {
			return
		}
	}
	m.ArmTrapVolleyCooldown(cs.game.config.GetTPS())
	cs.game.sowBossTrapField(m)
}

// sowBossTrapField replaces the field with a fresh volley around the boss.
func (g *MMGame) sowBossTrapField(m *monsterPkg.Monster3D) {
	w := g.GetCurrentWorld()
	if w == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	cx, cy := TileIndex(m.X, ts), TileIndex(m.Y, ts)
	radius := m.TrapVolleyRadiusTiles
	extent := int(math.Ceil(radius))
	seen := make(map[[2]int]bool, m.TrapVolleyCount)
	field := make([]bossFireTrap, 0, m.TrapVolleyCount)
	// Random rejection sampling with a generous attempt budget; a cramped
	// arena simply gets a thinner field.
	for attempts := 0; attempts < m.TrapVolleyCount*10 && len(field) < m.TrapVolleyCount; attempts++ {
		dx := rand.Intn(2*extent+1) - extent
		dy := rand.Intn(2*extent+1) - extent
		// Euclidean radius, matching the game-wide convention - a square would
		// reach ~1.4x the authored distance at the corners.
		if float64(dx*dx+dy*dy) > radius*radius {
			continue
		}
		tx, ty := cx+dx, cy+dy
		key := [2]int{tx, ty}
		if seen[key] || (tx == cx && ty == cy) {
			continue
		}
		if w.IsTileBlockingTerrainAt(tx, ty) {
			continue
		}
		seen[key] = true
		field = append(field, bossFireTrap{MapKey: g.mapKeyAtTile(tx, ty), TX: tx, TY: ty})
	}
	g.bossFireTraps = field
	g.bossFireTrapsOwner = m.ID
	g.AddColoredCombatMessage(fmt.Sprintf("%s seeds the ground with smouldering eggs!", m.Name), combatMessageOrange)
}

// checkBossFireTraps runs every frame in both modes: clears an orphaned field
// (owner dead or gone) and detonates the trap under the party's feet.
func (g *MMGame) checkBossFireTraps() {
	if len(g.bossFireTraps) == 0 {
		return
	}
	owner := g.monsterByID(g.bossFireTrapsOwner)
	if owner == nil || !owner.IsAlive() {
		g.bossFireTraps = nil
		g.bossFireTrapsOwner = ""
		return
	}
	ts := float64(g.config.GetTileSize())
	ptx, pty := TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts)
	for i, t := range g.bossFireTraps {
		if t.TX != ptx || t.TY != pty {
			continue
		}
		g.bossFireTraps = append(g.bossFireTraps[:i], g.bossFireTraps[i+1:]...)
		g.detonateBossFireTrap(owner)
		return
	}
}

// detonateBossFireTrap burns every living member (fire school, normal party
// mitigation; the shared choke point handles KO/blink/messages).
func (g *MMGame) detonateBossFireTrap(owner *monsterPkg.Monster3D) {
	cs := g.combat
	if cs == nil {
		return
	}
	g.AddColoredCombatMessage("The ground erupts in brood-fire!", combatMessageOrange)
	g.CreateSpellHitEffect(g.camera.X, g.camera.Y, "fire", 26, 9)
	g.addScreenShake(3, 5)
	// The field carries authored trap damage only, not the owner's true_damage.
	hit := monsterCharacterHit{
		Parts:      owner.OutgoingDamage(damagecalc.Parts{Normal: owner.TrapVolleyDamage}),
		DamageType: monsterPkg.DamageFire.String(),
	}
	cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
		cs.monsterHitCharacter(owner, member, "Brood-fire trap", hit)
	})
}

// monsterByID resolves a live monster on the current world by collision ID.
func (g *MMGame) monsterByID(id string) *monsterPkg.Monster3D {
	if id == "" || g.world == nil {
		return nil
	}
	for _, m := range g.world.Monsters {
		if m != nil && m.ID == id {
			return m
		}
	}
	return nil
}

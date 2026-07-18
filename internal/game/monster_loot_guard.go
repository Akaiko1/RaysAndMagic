package game

import (
	"sort"

	"ugataima/internal/character"
	"ugataima/internal/monster"
)

const maxLootGuardBandSize = 2

// The 12 positions form a clockwise route around the guarded prop. Every
// point is within a Euclidean radius of two tiles and the prop's own tile is
// intentionally absent. LootGuardSide and LootGuardPatrolAlt are the compact,
// persisted route cursor: side selects a pair, alt selects its second point.
var lootGuardPatrolOffsets = [6][2][2]int{
	{{1, 0}, {2, 0}},    // east inner / east outer
	{{1, 1}, {0, 2}},    // south-east / south outer
	{{0, 1}, {-1, 1}},   // south inner / south-west
	{{-2, 0}, {-1, 0}},  // west outer / west inner
	{{-1, -1}, {0, -2}}, // north-west / north outer
	{{0, -1}, {1, -1}},  // north inner / north-east
}

type lootGuardTargetID struct {
	key   string
	tileX int
	tileY int
}

type lootGuardPostID struct {
	tileX int
	tileY int
}

type lootGuardTarget struct {
	id  lootGuardTargetID
	npc *character.NPC
}

func isLootGuardTarget(npc *character.NPC) bool {
	return npc != nil && !npc.Visited &&
		(npc.Type == character.NPCTypeLootCrate || npc.Type == character.NPCTypeSpellLectern)
}

func (gl *GameLoop) activeLootGuardTargets() (map[lootGuardTargetID]lootGuardTarget, []lootGuardTarget) {
	if gl == nil || gl.game == nil || gl.game.world == nil || gl.game.config == nil {
		return nil, nil
	}
	tile := float64(gl.game.config.GetTileSize())
	byID := make(map[lootGuardTargetID]lootGuardTarget)
	var targets []lootGuardTarget
	for _, npc := range gl.game.world.NPCs {
		if !isLootGuardTarget(npc) {
			continue
		}
		id := lootGuardTargetID{key: npc.Key, tileX: int(npc.X / tile), tileY: int(npc.Y / tile)}
		target := lootGuardTarget{id: id, npc: npc}
		byID[id] = target
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].id.tileY != targets[j].id.tileY {
			return targets[i].id.tileY < targets[j].id.tileY
		}
		if targets[i].id.tileX != targets[j].id.tileX {
			return targets[i].id.tileX < targets[j].id.tileX
		}
		return targets[i].id.key < targets[j].id.key
	})
	return byID, targets
}

func lootGuardTargetOf(m *monster.Monster3D) lootGuardTargetID {
	return lootGuardTargetID{
		key:   m.LootGuardTargetKey,
		tileX: m.LootGuardTargetTileX,
		tileY: m.LootGuardTargetTileY,
	}
}

// lootGuardEligible is deliberately stricter than generic calm wandering:
// scripted fights, bosses, champions, and party-controlled/dynamic creatures
// keep their authored behaviour instead of being pulled into ambient prop AI.
func (gl *GameLoop) lootGuardEligible(m *monster.Monster3D) bool {
	if m == nil || !m.IsAlive() || m.Bound || m.Pacified || m.SummonedBy != "" ||
		m.IsEncounterMonster || m.IsChampion() || m.PassiveUntilAttacked ||
		m.IsInertSetPiece() || m.IsEngagingPlayer || m.WasAttacked ||
		m.Relentless || m.BossAggro || m.AIFoe != nil {
		return false
	}
	if m.IsBoss() {
		return false
	}
	return m.State == monster.StateIdle || m.State == monster.StatePatrolling
}

func (gl *GameLoop) canStartLootGuard(m *monster.Monster3D) bool {
	return gl.lootGuardEligible(m) && !m.LootGuarding && m.BandID == 0 && m.BandStackCount <= 1
}

func clearLootGuard(m *monster.Monster3D) {
	if m == nil {
		return
	}
	m.LootGuarding = false
	m.LootGuardTargetKey = ""
	m.LootGuardTargetTileX, m.LootGuardTargetTileY = 0, 0
	m.LootGuardSide = 0
	m.LootGuardPatrolAlt = false
	m.LootGuardMoveTileX, m.LootGuardMoveTileY = 0, 0
	m.LootGuardPatrolUntil = 0
	m.ResetPathfinding()
	leaveBand(m)
}

func (gl *GameLoop) lootGuardGroups(targets map[lootGuardTargetID]lootGuardTarget) map[lootGuardTargetID][]*monster.Monster3D {
	groups := make(map[lootGuardTargetID][]*monster.Monster3D)
	if gl == nil || gl.game == nil || gl.game.world == nil {
		return groups
	}
	for _, m := range gl.game.world.Monsters {
		if m == nil || !m.LootGuarding {
			continue
		}
		if !m.IsAlive() {
			clearLootGuard(m)
			continue
		}
		id := lootGuardTargetOf(m)
		if _, ok := targets[id]; !ok {
			clearLootGuard(m)
			continue
		}
		groups[id] = append(groups[id], m)
	}
	return groups
}

func (gl *GameLoop) lootGuardBandWantsParty(members []*monster.Monster3D) bool {
	for _, m := range members {
		if m != nil && (m.IsEngagingPlayer || m.WasAttacked) {
			return true
		}
	}
	return false
}

func (gl *GameLoop) lootGuardBandCanContinue(members []*monster.Monster3D) bool {
	for _, m := range members {
		if !gl.lootGuardEligible(m) {
			return false
		}
	}
	return true
}

func (gl *GameLoop) dissolveLootGuardBand(members []*monster.Monster3D) {
	for _, m := range members {
		clearLootGuard(m)
	}
}

// scatterLootGuardBand preserves ordinary band response semantics: a direct
// hit makes both guards sticky-hostile, while sight only engages guards with
// their own direct LoS. It is separate from normal banding because guard pairs
// may mix monster keys and may contain monsters whose YAML banding flag is
// false.
func (gl *GameLoop) scatterLootGuardBand(members []*monster.Monster3D, forceHit bool) {
	if gl == nil || gl.game == nil || gl.game.config == nil {
		return
	}
	stacked := lootGuardBandIsStacked(members)
	wasHit := forceHit
	live := make([]*monster.Monster3D, 0, len(members))
	for _, m := range members {
		if m == nil || !m.IsAlive() {
			continue
		}
		wasHit = wasHit || m.WasAttacked
		live = append(live, m)
	}
	// Check sight before clearing LootGuarding: that flag supplies the guard's
	// seven-tile direct-sight minimum. Pending pair members can be on different
	// sides of a wall, so the result must be kept per guard rather than inferred
	// from the member that triggered the scatter.
	sawParty := make(map[*monster.Monster3D]bool, len(live))
	if !wasHit {
		for _, m := range live {
			sawParty[m] = gl.bandMemberSeesParty(m)
		}
	}
	if !wasHit {
		// scatterBand evaluates after LootGuarding has been cleared. Preserve the
		// already-computed guard-specific sight result for a stacked pair.
		for _, m := range live {
			if sawParty[m] {
				m.BeginPlayerEngagement()
			}
		}
	}
	gl.dissolveLootGuardBand(members)
	if len(live) == 0 {
		return
	}
	if stacked && gl.game.collisionSystem != nil {
		// Like ordinary bands, a parallel tick may have alerted both guards
		// before this serial pass. A stacked pair must still physically burst
		// apart rather than merely lose its guard/BandID metadata.
		gl.scatterBand(live, members, float64(gl.game.config.GetTileSize()), wasHit)
		return
	}
	// A pair may still be walking independently to its post. It has reserved the
	// same crate, but has not physically stacked yet, so dissolve it without
	// pulling either mob across the map merely to perform a visual band scatter.
	for _, m := range live {
		engageBandMemberOnScatter(m, wasHit, sawParty[m])
		m.ResetPathfinding()
	}
}

func (gl *GameLoop) lootGuardBandMembers(member *monster.Monster3D) []*monster.Monster3D {
	if gl == nil || gl.game == nil || gl.game.world == nil || member == nil || !member.LootGuarding {
		return nil
	}
	target := lootGuardTargetOf(member)
	var members []*monster.Monster3D
	for _, m := range gl.game.world.Monsters {
		if m != nil && m.LootGuarding && lootGuardTargetOf(m) == target {
			members = append(members, m)
		}
	}
	return members
}

func lootGuardSideSeed(id string, target lootGuardTargetID) int {
	h := uint32(target.tileX*31 + target.tileY*131)
	for i := 0; i < len(target.key); i++ {
		h = h*33 + uint32(target.key[i])
	}
	for i := 0; i < len(id); i++ {
		h = h*33 + uint32(id[i])
	}
	return int(h % uint32(len(lootGuardPatrolOffsets)))
}

func lootGuardPatrolRouteIndex(side int, alt bool) int {
	if side < 0 || side >= len(lootGuardPatrolOffsets) {
		side = 0
	}
	index := side * 2
	if alt {
		index++
	}
	return index
}

func lootGuardPatrolRouteState(index int) (side int, alt bool) {
	count := len(lootGuardPatrolOffsets) * 2
	index %= count
	if index < 0 {
		index += count
	}
	return index / 2, index%2 != 0
}

func (gl *GameLoop) lootGuardTileFits(members []*monster.Monster3D, tileX, tileY int) bool {
	if gl == nil || gl.game == nil || gl.game.collisionSystem == nil || gl.game.config == nil {
		return true
	}
	tile := float64(gl.game.config.GetTileSize())
	x, y := TileCenterFromTile(tileX, tileY, tile)
	for _, m := range members {
		if m == nil || !gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, x, y, m.HabitatPrefs, m.Flying) {
			return false
		}
	}
	return true
}

// lootGuardPatrolTile picks the next genuinely walkable post from the
// radius-two route. The same post must fit every member: mixed guard pairs are
// allowed, but are never snapped onto terrain or a solid prop only one of them
// can occupy.
func (gl *GameLoop) lootGuardPatrolTile(target lootGuardTarget, members []*monster.Monster3D, side int, alt bool) (nextSide int, nextAlt bool, tileX, tileY int, ok bool) {
	return gl.lootGuardPatrolTileAvoiding(target, members, side, alt, nil)
}

// lootGuardPatrolTileAvoiding also excludes posts already reserved for another
// prop. Calm monsters ordinarily pass through each other, so collision alone
// cannot prevent two nearby crates from visually producing a four-mob pile.
func (gl *GameLoop) lootGuardPatrolTileAvoiding(target lootGuardTarget, members []*monster.Monster3D, side int, alt bool, reserved map[lootGuardPostID]lootGuardTargetID) (nextSide int, nextAlt bool, tileX, tileY int, ok bool) {
	start := lootGuardPatrolRouteIndex(side, alt)
	for step := 0; step < len(lootGuardPatrolOffsets)*2; step++ {
		index := (start + step) % (len(lootGuardPatrolOffsets) * 2)
		pair := lootGuardPatrolOffsets[index/2]
		offset := pair[index%2]
		tileX, tileY = target.id.tileX+offset[0], target.id.tileY+offset[1]
		if owner, taken := reserved[lootGuardPostID{tileX: tileX, tileY: tileY}]; taken && owner != target.id {
			continue
		}
		if !gl.lootGuardTileFits(members, tileX, tileY) {
			continue
		}
		nextSide, nextAlt = lootGuardPatrolRouteState(index)
		return nextSide, nextAlt, tileX, tileY, true
	}
	return 0, false, 0, 0, false
}

func stableLootGuardLeader(members []*monster.Monster3D) *monster.Monster3D {
	if len(members) == 0 {
		return nil
	}
	for _, m := range members {
		if m != nil && m.BandID > 0 && m.BandLeaderID == m.ID {
			return m
		}
	}
	leader := members[0]
	for _, m := range members[1:] {
		if m != nil && (leader == nil || m.ID < leader.ID) {
			leader = m
		}
	}
	return leader
}

func (gl *GameLoop) lootGuardPatrolPauseFrames() int64 {
	if gl == nil || gl.game == nil || gl.game.config == nil {
		return 1
	}
	frames := gl.game.config.GetTPS()
	if frames < 1 {
		frames = 1
	}
	return int64(frames)
}

func (gl *GameLoop) nextLootGuardBandID() int {
	maxID := 0
	if gl == nil || gl.game == nil || gl.game.world == nil {
		return 1
	}
	for _, m := range gl.game.world.Monsters {
		if m != nil && m.BandID > maxID {
			maxID = m.BandID
		}
	}
	return maxID + 1
}

func (gl *GameLoop) existingLootGuardBandID(members []*monster.Monster3D) int {
	if !lootGuardBandIsStacked(members) {
		return 0
	}
	id := members[0].BandID
	return id
}

// lootGuardBandIsStacked reports whether this reservation is already one real
// positional band. Pending guards intentionally have no BandID yet: they walk
// to the crate themselves instead of snapping across the map to a leader.
func lootGuardBandIsStacked(members []*monster.Monster3D) bool {
	if len(members) != maxLootGuardBandSize || members[0] == nil || members[0].BandID <= 0 {
		return false
	}
	id := members[0].BandID
	leaderID := members[0].BandLeaderID
	hasLeader := false
	for _, m := range members[1:] {
		if m == nil || m.BandID != id || m.BandLeaderID != leaderID || m.BandStackCount != len(members) {
			return false
		}
	}
	for _, m := range members {
		if m != nil && m.ID == leaderID {
			hasLeader = true
			break
		}
	}
	return leaderID != "" && members[0].BandStackCount == len(members) && hasLeader
}

func (gl *GameLoop) lootGuardMembersAtTile(members []*monster.Monster3D, tileX, tileY int) bool {
	if gl == nil || gl.game == nil || gl.game.config == nil {
		return false
	}
	tile := float64(gl.game.config.GetTileSize())
	for _, m := range members {
		if m == nil || int(m.X/tile) != tileX || int(m.Y/tile) != tileY {
			return false
		}
	}
	return true
}

func advanceLootGuardPatrolRoute(m *monster.Monster3D) {
	if m == nil {
		return
	}
	next := lootGuardPatrolRouteIndex(m.LootGuardSide, m.LootGuardPatrolAlt) + 1
	m.LootGuardSide, m.LootGuardPatrolAlt = lootGuardPatrolRouteState(next)
	m.LootGuardPatrolUntil = 0
}

func lootGuardTargetReservations(targets map[lootGuardTargetID]lootGuardTarget) map[lootGuardPostID]lootGuardTargetID {
	reservations := make(map[lootGuardPostID]lootGuardTargetID, len(targets))
	for id := range targets {
		reservations[lootGuardPostID{tileX: id.tileX, tileY: id.tileY}] = id
	}
	return reservations
}

func (gl *GameLoop) lootGuardPostReservations(targets map[lootGuardTargetID]lootGuardTarget, except lootGuardTargetID) map[lootGuardPostID]lootGuardTargetID {
	reservations := lootGuardTargetReservations(targets)
	for id, members := range gl.lootGuardGroups(targets) {
		if id == except || len(members) == 0 {
			continue
		}
		leader := stableLootGuardLeader(members)
		if leader == nil {
			continue
		}
		reservations[lootGuardPostID{tileX: leader.LootGuardMoveTileX, tileY: leader.LootGuardMoveTileY}] = id
	}
	return reservations
}

// syncLootGuardBand updates a valid target reservation. A two-mob reservation
// only becomes a normal positional band after both mobs arrive at the same post;
// until then each walks there independently, avoiding a long-distance snap.
func (gl *GameLoop) syncLootGuardBand(target lootGuardTarget, members []*monster.Monster3D, advancePatrol bool, reserved map[lootGuardPostID]lootGuardTargetID) {
	if len(members) == 0 {
		return
	}
	sortMonstersByID(members)
	leader := stableLootGuardLeader(members)
	if leader == nil {
		gl.dissolveLootGuardBand(members)
		return
	}
	for i, m := range members {
		if m == leader {
			if i != 0 {
				copy(members[1:i+1], members[0:i])
				members[0] = leader
			}
			break
		}
	}
	leader = members[0]
	moveX, moveY := 0, 0
	selectPost := func() bool {
		side, alt, tileX, tileY, ok := gl.lootGuardPatrolTileAvoiding(target, members, leader.LootGuardSide, leader.LootGuardPatrolAlt, reserved)
		if !ok {
			return false
		}
		leader.LootGuardSide, leader.LootGuardPatrolAlt = side, alt
		moveX, moveY = tileX, tileY
		return true
	}
	if !selectPost() {
		gl.dissolveLootGuardBand(members)
		return
	}
	stacked := lootGuardBandIsStacked(members)
	patrolling := len(members) == 1 || stacked
	atPost := int(leader.X/float64(gl.game.config.GetTileSize())) == moveX &&
		int(leader.Y/float64(gl.game.config.GetTileSize())) == moveY
	if patrolling && atPost && !gl.game.turnBasedMode {
		if leader.LootGuardPatrolUntil == 0 {
			leader.LootGuardPatrolUntil = gl.game.frameCount + gl.lootGuardPatrolPauseFrames()
		} else if advancePatrol && gl.game.frameCount >= leader.LootGuardPatrolUntil {
			advanceLootGuardPatrolRoute(leader)
			if !selectPost() {
				gl.dissolveLootGuardBand(members)
				return
			}
		}
	}
	for _, m := range members {
		m.LootGuarding = true
		m.LootGuardTargetKey = target.id.key
		m.LootGuardTargetTileX, m.LootGuardTargetTileY = target.id.tileX, target.id.tileY
		m.LootGuardSide = leader.LootGuardSide
		m.LootGuardPatrolAlt = leader.LootGuardPatrolAlt
		m.LootGuardMoveTileX, m.LootGuardMoveTileY = moveX, moveY
		m.LootGuardPatrolUntil = leader.LootGuardPatrolUntil
	}
	if reserved != nil {
		reserved[lootGuardPostID{tileX: moveX, tileY: moveY}] = target.id
	}
	if len(members) == 1 {
		leaveBand(members[0])
		return
	}
	if !stacked && !gl.lootGuardMembersAtTile(members, moveX, moveY) {
		for _, m := range members {
			leaveBand(m)
		}
		return
	}
	id := gl.existingLootGuardBandID(members)
	if id == 0 {
		id = gl.nextLootGuardBandID()
	}
	gl.stackMonsterBand(id, members)
}

func (gl *GameLoop) assignLootGuard(m *monster.Monster3D, target lootGuardTarget) {
	m.LootGuarding = true
	m.LootGuardTargetKey = target.id.key
	m.LootGuardTargetTileX, m.LootGuardTargetTileY = target.id.tileX, target.id.tileY
	m.LootGuardSide = lootGuardSideSeed(m.ID, target.id)
	m.LootGuardPatrolAlt = false
	m.LootGuardPatrolUntil = 0
	m.State = monster.StatePatrolling
	m.StateTimer = 0
	m.ResetPathfinding()
}

// prepareLootPropGuards runs before movement. It preserves valid existing
// assignments, releases spent props, then lets only solo calm mobs reserve the
// remaining slots. Reservation order is stable by monster ID, not map-slice or
// goroutine timing.
func (gl *GameLoop) prepareLootPropGuards() {
	if gl == nil || gl.game == nil || gl.game.world == nil {
		return
	}
	targetByID, targets := gl.activeLootGuardTargets()
	if len(targets) == 0 {
		for _, m := range gl.game.world.Monsters {
			if m != nil && m.LootGuarding {
				clearLootGuard(m)
			}
		}
		return
	}

	groups := gl.lootGuardGroups(targetByID)
	for _, target := range targets {
		members := groups[target.id]
		if len(members) == 0 {
			continue
		}
		sortMonstersByID(members)
		if len(members) > maxLootGuardBandSize {
			for _, m := range members[maxLootGuardBandSize:] {
				clearLootGuard(m)
			}
			members = members[:maxLootGuardBandSize]
			groups[target.id] = members
		}
		if gl.lootGuardBandWantsParty(members) {
			gl.scatterLootGuardBand(members, false)
			delete(groups, target.id)
			continue
		}
		if !gl.lootGuardBandCanContinue(members) {
			gl.dissolveLootGuardBand(members)
			delete(groups, target.id)
		}
	}

	candidates := make([]*monster.Monster3D, 0, len(gl.game.world.Monsters))
	for _, m := range gl.game.world.Monsters {
		if gl.canStartLootGuard(m) {
			candidates = append(candidates, m)
		}
	}
	sortMonstersByID(candidates)
	tile := float64(gl.game.config.GetTileSize())
	// Assignment and first-sight alert share one authored seven-tile guard
	// minimum. This is a prop-objective exception, not a TB visibility cap.
	maxDist := monster.LootGuardMinAlertRadiusTiles * tile
	for _, m := range candidates {
		var chosen *lootGuardTarget
		bestDist := maxDist
		for i := range targets {
			target := &targets[i]
			members := groups[target.id]
			if len(members) >= maxLootGuardBandSize {
				continue
			}
			d := Distance(m.X, m.Y, target.npc.X, target.npc.Y)
			if d > bestDist || (gl.game.collisionSystem != nil && !gl.game.collisionSystem.CheckLineOfSight(m.X, m.Y, target.npc.X, target.npc.Y)) {
				continue
			}
			combined := append(append([]*monster.Monster3D(nil), members...), m)
			leader := stableLootGuardLeader(combined)
			preferred := lootGuardSideSeed(m.ID, target.id)
			if leader != nil {
				preferred = leader.LootGuardSide
			}
			if _, _, _, _, ok := gl.lootGuardPatrolTile(*target, combined, preferred, false); !ok {
				continue
			}
			bestDist = d
			chosen = target
		}
		if chosen == nil {
			continue
		}
		gl.assignLootGuard(m, *chosen)
		groups[chosen.id] = append(groups[chosen.id], m)
	}
	postReservations := lootGuardTargetReservations(targetByID)
	for _, target := range targets {
		if members := groups[target.id]; len(members) > 0 {
			gl.syncLootGuardBand(target, members, true, postReservations)
		}
	}
}

// reconcileLootPropGuardBands runs after RT/TB movement. It sees a guard that
// noticed or was hit this frame and scatters the whole pair before combat uses
// the new state; it otherwise re-stacks followers after their private movement.
func (gl *GameLoop) reconcileLootPropGuardBands() {
	if gl == nil || gl.game == nil || gl.game.world == nil {
		return
	}
	targetByID, targets := gl.activeLootGuardTargets()
	groups := gl.lootGuardGroups(targetByID)
	if len(targets) == 0 {
		return
	}
	postReservations := lootGuardTargetReservations(targetByID)
	for _, target := range targets {
		members := groups[target.id]
		if len(members) == 0 {
			continue
		}
		sortMonstersByID(members)
		if gl.lootGuardBandWantsParty(members) {
			gl.scatterLootGuardBand(members, false)
			continue
		}
		if !gl.lootGuardBandCanContinue(members) {
			gl.dissolveLootGuardBand(members)
			continue
		}
		gl.syncLootGuardBand(target, members, false, postReservations)
	}
}

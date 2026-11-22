package game

import (
	"fmt"
	"maps"
	"math"
	"slices"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/status"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *MMGame) techniqueRefusal(idx int, key string) string {
	return g.techniqueCastRefusal(idx, key, false)
}

func (g *MMGame) techniqueCastRefusal(idx int, key string, automatic bool) string {
	d := config.Technique(key)
	if d == nil || g.party == nil || idx < 0 || idx >= len(g.party.Members) {
		return uitext.Text("technique.refusal_unknown")
	}
	c := g.party.Members[idx]
	if c == nil || c.Class != character.ClassWayfarer || !c.HasSkill(character.SkillTranslocation) || !c.CanUseCombatAction() {
		return uitext.Text("technique.refusal_no_pilgrim")
	}
	if automatic && (!d.Automatic || !g.canAutocast(idx)) {
		return uitext.Text("technique.refusal_not_ready")
	}
	if !g.turnBasedMode && !g.combatActorAllowed(idx) {
		return uitext.Text("technique.refusal_unfocused")
	}
	if c.Level < d.Level {
		return uitext.Text("technique.refusal_level", d.Name, d.Level)
	}
	if g.combat != nil && g.combat.partyInsideSolidTerrain() {
		return uitext.Text("technique.refusal_no_room")
	}
	if d.IsStep() {
		// A step moves the party, and a rooted party cannot move (as with Jump).
		if g.partyRooted() {
			return uitext.Text("technique.refusal_rooted")
		}
		if g.turnBasedMode && g.currentTurn != 0 {
			return uitext.Text("technique.refusal_party_phase")
		}
		if g.spatialReuse.Active() {
			return uitext.Text("technique.refusal_space")
		}
	} else if !automatic && !g.canSpendCombatAction(idx) {
		return uitext.Text("technique.refusal_not_ready")
	}
	if c.SpellPoints < g.techniqueSPCost(c, d) {
		return uitext.Text("technique.refusal_sp")
	}
	if !d.IsStep() && !g.techniqueUseful(d) {
		return uitext.Text("technique.refusal_no_benefit")
	}
	return ""
}

// techniqueUseful reports whether a non-step technique would change anything.
func (g *MMGame) techniqueUseful(d *config.TechniqueDefinition) bool {
	switch d.Kind {
	case config.TechniquePurify:
		return g.partyNeedsPurify()
	case config.TechniqueDodge:
		_, active := g.combatBuffByID(config.TechniqueID(d.Key))
		return !active
	case config.TechniqueHaste:
		return g.partyNeedsHaste()
	}
	return false
}

// partyNeedsPurify: a living ally has an affliction, or the party is rooted or
// slowed. Shared by the Purify technique and the Purify/Dispel Magic spells.
func (g *MMGame) partyNeedsPurify() bool {
	if g.partyRooted() || g.partyHinder.Slow.Active() {
		return true
	}
	for _, c := range g.party.Members {
		if c.Purifiable() {
			return true
		}
	}
	return false
}

// partyNeedsHaste: no haste effect is running and one would add actions.
// Shared by the Quickening technique and the Haste spell.
func (g *MMGame) partyNeedsHaste() bool {
	if _, active := g.partyTempoBuff(); active {
		return false
	}
	if !g.turnBasedMode {
		for _, c := range g.party.Members {
			if c != nil && c.CanUseCombatAction() {
				return true
			}
		}
		return false
	}
	eligible := 0
	pool := g.cardBonusActions()
	speed := 0
	for _, c := range g.party.Members {
		if c != nil && c.CanUseCombatAction() {
			eligible++
			speed = max(speed, c.SpeedBonusActionTier())
		}
	}
	return eligible > pool+speed
}
func (g *MMGame) techniqueSPCost(c *character.MMCharacter, d *config.TechniqueDefinition) int {
	return c.MeditatedSPCost(config.TierValue(d.SPCost, c.SkillTier(character.SkillTranslocation)))
}
func (g *MMGame) techniqueCooldown(c *character.MMCharacter, d *config.TechniqueDefinition) int {
	if d.IsStep() || d.CooldownSeconds == 0 {
		return 0
	}
	return g.combat.actionCooldownBreakdown(c, d.CooldownSeconds).TotalFrames
}

// useTechnique is the single manual/book/quick/autocast transaction. Rejection
// spends nothing; an indexed commit never borrows the selected hero's action.
func (g *MMGame) useTechnique(idx int, key string, automatic, announce bool) bool {
	if reason := g.techniqueCastRefusal(idx, key, automatic); reason != "" {
		if announce {
			g.AddCombatMessage(reason)
		}
		return false
	}
	d := config.Technique(key)
	c := g.party.Members[idx]
	tier := c.SkillTier(character.SkillTranslocation)
	tps := g.config.GetTPS()
	cooldown := g.techniqueCooldown(c, d)
	fxStart := len(g.buffFxAnims)
	if d.IsStep() {
		if !g.performSpatialStep(c, d, tier) {
			if announce {
				g.AddCombatMessage(uitext.Text("combat.no_legal_spatial_destination"))
			}
			return false
		}
	} else {
		switch d.Kind {
		case config.TechniquePurify:
			g.purifyParty()
		case config.TechniqueDodge, config.TechniqueHaste:
			b := TimedCombatBuff{ID: config.TechniqueID(d.Key), CombatClock: true, MasteryTier: tier}
			b.Clock.Arm(config.TierValue(d.Duration, tier), tps)
			applyTechniqueBuff(&b, d, tier)
			b.DeferFirstTurnTick = d.Kind == config.TechniqueHaste
			g.addCombatBuff(b)
		}
	}
	cost := g.techniqueSPCost(c, d)
	c.SpellPoints -= cost
	if automatic {
		g.finishAutocast(c, fxStart)
	}
	mode := ""
	if automatic {
		mode = " automatically"
	}
	g.AddCombatMessage(uitext.Text("combat.uses_sp", c.Name, d.Name, mode, cost))
	if !automatic && !d.IsStep() {
		// Manual techniques spend the caster's action but never add a movement penalty.
		if g.turnBasedMode && g.currentTurn == 0 && c.ActionsRemaining > 0 {
			g.partyPenaltyFreeActionsUsed++
		}
		g.commitHeroAction(idx, cooldown)
	}
	if !automatic {
		g.actionInputStagger = g.rtActionStaggerFrames()
	}
	return true
}

// commitHeroAction ends hero idx's action: its real-time recovery starts
// (never shortened) and, in TB, one action slot is spent - when it ran out,
// selection moves on, and with no one left the party turn ends.
func (g *MMGame) commitHeroAction(idx, cooldown int) {
	if g.party == nil || idx < 0 || idx >= len(g.party.Members) {
		return
	}
	c := g.party.Members[idx]
	if c == nil {
		return
	}
	c.RTCooldown = max(c.RTCooldown, cooldown)
	if !g.turnBasedMode || g.currentTurn != 0 {
		return
	}
	if c.ActionsRemaining > 0 {
		c.ActionsRemaining--
		g.partyActionsUsed++
	}
	if g.partyAllExhausted() {
		g.endPartyTurn()
		return
	}
	if idx == g.selectedChar && c.ActionsRemaining == 0 {
		g.advanceToNextEligibleChar()
	}
}
func (g *MMGame) useTechniqueFromBook(key string) bool {
	return g.dispatchCharacterHubWorldAction(func() bool { return g.useTechnique(g.selectedChar, key, false, true) })
}
func (g *MMGame) quickenRecovery(c *character.MMCharacter, frames int) int {
	pct := g.quickenRecoveryPct(c)
	if g.isPartyMember(c) {
		frames = g.hinderRecovery(frames)
	}
	if frames <= 0 || pct == 0 {
		return frames
	}
	return g.clampRTCooldown(frames * (100 - pct) / 100)
}

// quickenRecoveryPct is the recovery cut Quickening gives c right now (0 when
// absent). Champion templates reuse these formulas but do not own party buffs.
func (g *MMGame) quickenRecoveryPct(c *character.MMCharacter) int {
	if !g.isPartyMember(c) {
		return 0
	}
	b, ok := g.partyTempoBuff()
	if !ok {
		return 0
	}
	return b.RecoveryPct
}

// tickPartyClocks spends one tick of the party's clocks (status.DualClock) -
// a frame per RT frame, a turn at each TB party turn: personal spell reuse and
// effects, the autocast and auto-drink budgets, target marks, the Fold Step
// anchor and reuse, monster catalysis marks and the Clock Tower gear.
func (g *MMGame) tickPartyClocks(turn bool) {
	g.tickRelicMarks(turn)
	g.tickClockworkGear(turn)
	g.spatialReuse.Tick(turn)
	g.tickCardSummonCooldowns(turn)
	g.ecology.Stop.Tick(turn)
	g.tactics.still.Tick(turn)
	if wm := world.GlobalWorldManager; wm != nil && wm.GlobalTeleporterRegistry != nil {
		wm.GlobalTeleporterRegistry.TickGroups(turn)
	}
	for _, c := range g.party.Members {
		if c == nil {
			continue
		}
		c.RareClass.Tick(turn)
		c.SpellState.Tick(turn)
		c.AutocastCD.Tick(turn)
		c.AutoDrinkCD.Tick(turn)
		c.Designation.Tick(turn)
		if c.HasCondition(character.ConditionDead) || c.HasCondition(character.ConditionEradicated) {
			c.RareClass.Anchor = character.SpatialAnchor{}
		}
	}
}

// partyMoved is the one hook for the party moving from (x, y) to the camera,
// by any means (a step, a jump, a teleport, a push, a current): movement-
// charged gear (Flowing Staff, Solar Flare weapons) sees the distance.
func (g *MMGame) partyMoved(x, y float64) {
	if g.party == nil || g.camera == nil {
		return
	}
	distance := math.Hypot(g.camera.X-x, g.camera.Y-y)
	if distance > 0 {
		g.partyMovedClock()
	}
	for _, c := range g.party.Members {
		g.chargeSignatureWeapons(c, distance/g.config.GetTileSize(), false)
	}
	for _, c := range g.party.Members {
		if c == nil || !c.CanUseCombatAction() || !c.HasSkill(character.SkillFlowingStaff) {
			continue
		}
		if status.Accrue(&c.RareClass.FlowDistance, distance, float64(g.config.GetTileSize())) > 0 {
			c.RareClass.FlowCharges = character.FlowingStaffCapacity(c.SkillTier(character.SkillFlowingStaff))
		}
	}
}
func (g *MMGame) canonicalPosition(x, y float64) (string, float64, float64) {
	if wm := world.GlobalWorldManager; g.openWorldActive() {
		if key, lx, ly, ok := wm.LocalizeWorldPos(x, y); ok {
			return key, lx, ly
		}
	}
	return currentMapKey(), x, y
}

type spatialTerrain struct {
	*world.World3D
	g           *MMGame
	region      string
	buildings   map[[2]int]bool
	passOpenAir bool // Transit only; never permits landing on an unwalkable floor.
}

func (t spatialTerrain) IsTileBlocking(x, y int) bool {
	g := t.g
	if t.buildings[[2]int{x, y}] {
		return true
	}
	if g.world.IsTileBlockingTerrainAt(x, y) {
		if !t.passOpenAir || x < 0 || y < 0 || x >= g.world.Width || y >= g.world.Height || world.GlobalTileManager == nil {
			return true
		}
		d := world.GlobalTileManager.GetTileData(g.world.Tiles[y][x])
		if d == nil || !d.FlyOver {
			return true
		}
	}
	ts := float64(g.config.GetTileSize())
	wx, wy := (float64(x)+0.5)*ts, (float64(y)+0.5)*ts
	key, _, _ := g.canonicalPosition(wx, wy)
	if key != t.region {
		return true
	}
	if wm := world.GlobalWorldManager; wm != nil && wm.GlobalTeleporterRegistry != nil {
		if _, ok := wm.GlobalTeleporterRegistry.FindTeleporter(key, x, y); ok {
			return true
		}
	}
	if tm := world.GlobalTileManager; tm != nil {
		if def := tm.GetTileData(g.world.Tiles[y][x]); def != nil {
			if _, ok := def.Properties["teleporter_group"]; ok || def.Type == "teleporter" {
				return true
			}
		}
	}
	return false
}
func (g *MMGame) performSpatialStep(c *character.MMCharacter, d *config.TechniqueDefinition, tier int) bool {
	if g.collisionSystem == nil || g.camera == nil || g.world == nil {
		return false
	}
	ts := float64(g.config.GetTileSize())
	oldX, oldY := g.camera.X, g.camera.Y
	region, lx, ly := g.canonicalPosition(oldX, oldY)
	maxTiles := config.TierValue(d.Range, tier)
	terrain := spatialTerrain{World3D: g.world, g: g, region: region, buildings: g.buildingOccupiedTiles()}
	pathTerrain := terrain
	pathTerrain.passOpenAir = true
	legalLanding := func(x, y float64) bool {
		if terrain.IsTileBlocking(TileIndex(x, ts), TileIndex(y, ts)) {
			return false
		}
		// Intermediate combat bodies may be crossed, but landing must be clear.
		if livingMonsterOnTile(g.world, TileIndex(x, ts), TileIndex(y, ts), ts, nil) != nil {
			return false
		}
		return g.canMovePartyTo(x, y)
	}
	var x, y float64
	if d.Kind == config.TechniqueReturnStep {
		a := c.RareClass.Anchor
		if !a.Clock.Active() || a.MapKey != region {
			return false
		}
		x, y = a.X, a.Y
		if wm := world.GlobalWorldManager; wm != nil {
			x, y = wm.ProjectWorldPos(a.MapKey, x, y)
		}
		// Intentional design: Return Step has NO distance or path/LOS limit.
		// It returns to the first Fold in a chain anywhere on this same map,
		// even across intervening walls. Only the exact landing must be legal.
		if !legalLanding(x, y) {
			return false
		}
	} else {
		angle := g.camera.Angle
		dx, dy := math.Cos(angle), math.Sin(angle)
		originX, originY := oldX, oldY
		if g.turnBasedMode {
			gridX, gridY := (&InputHandler{game: g}).getDirectionFromAngle(angle)
			dx, dy = float64(gridX), float64(gridY)
			originX, originY = (float64(TileIndex(oldX, ts))+.5)*ts, (float64(TileIndex(oldY, ts))+.5)*ts
		}
		found := false
		for tiles := maxTiles; tiles >= d.MinRange; tiles-- {
			x, y = originX+dx*float64(tiles)*ts, originY+dy*float64(tiles)*ts
			if math.Hypot(x-oldX, y-oldY)+0.01 >= float64(d.MinRange)*ts && legalLanding(x, y) &&
				collision.CheckMovementLine(pathTerrain, ts, oldX, oldY, x, y) && g.collisionSystem.CheckLineOfSight(oldX, oldY, x, y) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	g.displaceParty(x, y, "spatial")
	if d.Kind == config.TechniqueFoldStep {
		// Keep the FIRST departure point across successive Folds; refreshing
		// its lifetime must not replace it with the last hop's origin.
		if !c.RareClass.Anchor.Clock.Active() || c.RareClass.Anchor.MapKey != region {
			c.RareClass.Anchor = character.SpatialAnchor{MapKey: region, X: lx, Y: ly}
		}
		c.RareClass.Anchor.Clock = status.DualClock{}
		c.RareClass.Anchor.Clock.Arm(config.TierValue(d.Duration, tier), g.config.GetTPS())
	} else {
		c.RareClass.Anchor = character.SpatialAnchor{}
	}
	// The shared step reuse is whole rounds: a TB step locks the next until
	// the next party turn at least.
	g.spatialReuse = status.DualClock{}
	g.spatialReuse.Arm(d.ReuseSeconds, g.config.GetTPS())
	if g.gameLoop != nil && g.gameLoop.inputHandler != nil {
		g.gameLoop.inputHandler.applyLandingTileEffects()
	}
	return true
}

// canAutocast is independent of action points and recovery. Both spellbook
// spells and Pilgrim techniques share one personal budget, AutocastCD.
func (g *MMGame) canAutocast(index int) bool {
	if g.party == nil || index < 0 || index >= len(g.party.Members) {
		return false
	}
	c := g.party.Members[index]
	if c == nil || !c.CanUseCombatAction() || c.AutocastCD.Active() {
		return false
	}
	if g.turnBasedMode {
		return g.currentTurn == 0
	}
	return g.combatActorAllowed(index)
}

func (g *MMGame) finishAutocast(c *character.MMCharacter, fxStart int) {
	c.AutocastCD = status.DualClock{}
	c.AutocastCD.Arm(g.config.AutocastIntervalRounds*config.CombatRoundSeconds, g.config.GetTPS())
	g.queueAutocastFx(fxStart)
}

// Spellbook autocasts use the same personal budget as class techniques.
// They pay mana without spending an action or triggering action-only procs.
func (g *MMGame) tryAutomaticSpell(index int) bool {
	if !g.canAutocast(index) {
		return false
	}
	c := g.party.Members[index]
	var enabled []string
	for id, on := range c.SpellState.Automatic {
		if on {
			enabled = append(enabled, id)
		}
	}
	slices.Sort(enabled)
	for _, id := range enabled {
		d, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil || !d.HasAutocast() {
			continue
		}
		if !characterKnowsSpellByID(c, d.ID) || c.SpellPoints < c.MeditatedSPCost(d.SpellPointsCost) {
			continue
		}
		if !g.spellAutocastFires(d) {
			continue
		}
		fxStart := len(g.buffFxAnims)
		outcome := g.combat.castSpell(spellCastRequest{
			ID: d.ID, Definition: d, Caster: c, Cost: c.MeditatedSPCost(d.SpellPointsCost),
			PlayerInitiated: true, Automatic: true,
		})
		if outcome != castCommitted {
			continue
		}
		g.finishAutocast(c, fxStart)
		return true
	}
	return false
}

// Automatic casting answers shared needs: a spell's mechanic kind and a
// technique's kind each map onto the same predicates. These tables are the one
// trigger per kind; validateAutocastTriggers keeps them in step with the kinds
// config lets autocast.
var spellAutocastTriggers = map[config.MechanicKind]func(*MMGame, spells.SpellDefinition) bool{
	config.MechanicDispelMagic: func(g *MMGame, d spells.SpellDefinition) bool {
		return g.partyNeedsPurify() || g.hostileMagicNearby(d.Mechanic.RadiusTiles)
	},
	config.MechanicCelestialArmor: func(g *MMGame, d spells.SpellDefinition) bool {
		return g.advancedBuff(d.Mechanic.Kind) == nil && g.partyEngaged()
	},
	config.MechanicPurify: func(g *MMGame, _ spells.SpellDefinition) bool { return g.partyNeedsPurify() },
	config.MechanicHaste:  func(g *MMGame, _ spells.SpellDefinition) bool { return g.partyUnderAttack() && g.partyNeedsHaste() },
}

var techniqueAutocastTriggers = map[string]func(*MMGame, *config.TechniqueDefinition) bool{
	config.TechniquePurify: func(g *MMGame, _ *config.TechniqueDefinition) bool { return g.partyNeedsPurify() },
	config.TechniqueHaste: func(g *MMGame, _ *config.TechniqueDefinition) bool {
		return g.partyUnderAttack() && g.partyNeedsHaste()
	},
	config.TechniqueDodge: func(g *MMGame, d *config.TechniqueDefinition) bool {
		return g.partyUnderAttack() && g.techniqueUseful(d) && g.partyHardPressed()
	},
}

// kindNames lists closed-set kinds as their authored names.
// validateAutocastTriggers fails the boot on a kind config lets autocast with
// no trigger (its AUTO toggle would never cast) or a trigger for a kind that
// may not autocast.
func validateAutocastTriggers() error {
	for _, c := range []struct {
		what     string
		kinds    []string
		triggers []string
	}{
		{"spell mechanic", config.KindNames(config.AutocastMechanicKinds()), config.KindNames(slices.Sorted(maps.Keys(spellAutocastTriggers)))},
		{"technique", config.AutocastTechniqueKinds(), slices.Sorted(maps.Keys(techniqueAutocastTriggers))},
	} {
		if !slices.Equal(c.kinds, c.triggers) {
			return fmt.Errorf("%s autocast kinds %v but triggers for %v", c.what, c.kinds, c.triggers)
		}
	}
	return nil
}

func (g *MMGame) spellAutocastFires(d spells.SpellDefinition) bool {
	trigger, ok := spellAutocastTriggers[d.Mechanic.Kind]
	return ok && trigger(g, d)
}

func (g *MMGame) techniqueAutocastFires(d *config.TechniqueDefinition) bool {
	trigger, ok := techniqueAutocastTriggers[d.Kind]
	return ok && trigger(g, d)
}

// partyHardPressed: two attackers in contact, or an ally near defeat.
func (g *MMGame) partyHardPressed() bool {
	ts := float64(g.config.GetTileSize())
	px, py := g.partyFightPos()
	near := 0
	for m := range g.combatants() {
		if g.monsterPressesParty(m) && math.Hypot(m.X-px, m.Y-py) <= 2*ts {
			near++
		}
	}
	if near >= 2 {
		return true
	}
	for _, m := range g.party.Members {
		if m != nil && m.CanAct() && m.HitPoints*100 < m.MaxHitPoints*35 {
			return true
		}
	}
	return false
}
func (gl *GameLoop) updateAutomaticTechniques() {
	g := gl.game
	if g.party == nil || g.world == nil || g.gameplayPausedByOverlay() || g.automaticInventoryBlocked() || g.actionInputStagger > 0 {
		return
	}
	if gl.inputHandler != nil && gl.inputHandler.isRunning() && !g.partyFireWhileRunning() {
		return
	}
	for i, c := range g.party.Members {
		if !g.canAutocast(i) {
			continue
		}
		for d := range config.AutomaticTechniques() {
			if c.RareClass.Automatic[d.Key] && g.techniqueAutocastFires(d) && g.useTechnique(i, d.Key, true, false) {
				return
			}
		}
		if g.tryAutomaticSpell(i) {
			return
		}
	}
}
func (ih *InputHandler) handleSpatialStepInput() bool {
	if !ih.keys.Consume(ebiten.KeyV) {
		return false
	}
	kind := config.TechniqueFoldStep
	if ih.keyHeld(ebiten.KeyShiftLeft) || ih.keyHeld(ebiten.KeyShiftRight) {
		kind = config.TechniqueReturnStep
	}
	key := "" // no such technique: useTechnique refuses with a message
	if d := config.TechniqueOfKind(kind); d != nil {
		key = d.Key
	}
	if ih.keyHeld(ebiten.KeyW) || ih.keyHeld(ebiten.KeyS) || ih.keyHeld(ebiten.KeyA) || ih.keyHeld(ebiten.KeyD) {
		if ih.isRunning() && !ih.game.partyFireWhileRunning() {
			return true
		}
	}
	ih.game.useTechnique(ih.game.selectedChar, key, false, true)
	return true
}
func (g *MMGame) slottedTechnique(idx int) (string, bool) {
	if idx < 0 || idx >= len(g.party.Members) {
		return "", false
	}
	it := g.party.Members[idx].Equipment[items.SlotQuickAction]
	return it.Technique, it.Type == items.ItemTechnique
}

func (t spatialTerrain) GetWorldBounds() (int, int) { return t.g.world.GetWorldBounds() }

// Only a fresh F press invokes a slotted spatial step. SmartAttack and repeat
// dispatch exclude it, so targeting an enemy never moves the party.
func (ih *InputHandler) tryExplicitSlottedStep() bool {
	g := ih.game
	key, ok := g.slottedTechnique(g.selectedChar)
	d := config.Technique(key)
	if !ok || d == nil || !d.IsStep() {
		return false
	}
	// Handled is independent of success: an explicit step may never fall through
	// to another hero's spell, including during the global input stagger.
	if g.actionInputStagger == 0 {
		g.useTechnique(g.selectedChar, key, false, true)
	}
	return true
}

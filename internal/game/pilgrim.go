package game

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"maps"
	"math"
	"slices"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"
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
		if g.spatialReuseFrames > 0 || (g.turnBasedMode && g.spatialStepThisTurn) {
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
	if g.partyRooted() || g.partyHinder.Slow > 0 {
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
	return g.combat.effectiveSpellCost(c, config.TierValue(d.SPCost, c.SkillTier(character.SkillTranslocation)))
}
func (g *MMGame) techniqueCooldown(c *character.MMCharacter, d *config.TechniqueDefinition) int {
	if d.IsStep() || d.CooldownSeconds == 0 {
		return 0
	}
	return g.combat.castCooldownFrames(c, d.CooldownSeconds)
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
				g.AddCombatMessage("No legal spatial destination.")
			}
			return false
		}
	} else {
		switch d.Kind {
		case config.TechniquePurify:
			g.purifyParty()
		case config.TechniqueDodge, config.TechniqueHaste:
			b := TimedCombatBuff{SpellID: config.TechniqueID(d.Key), Frames: config.TierValue(d.Duration, tier) * tps, CombatClock: true, MasteryTier: tier}
			applyTechniqueBuff(&b, d, tier)
			b.DeferFirstTurnTick = d.Kind == config.TechniqueHaste
			g.addCombatBuff(b)
			g.refreshCombatBuffStatus(b)
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
	g.AddCombatMessage(fmt.Sprintf("%s uses %s%s (%d SP).", c.Name, d.Name, mode, cost))
	if !automatic && !d.IsStep() {
		// Manual techniques spend the caster's action but never add a movement penalty.
		if g.turnBasedMode && g.currentTurn == 0 && c.ActionsRemaining > 0 {
			g.partyPenaltyFreeActionsUsed++
		}
		g.consumeCharacterActionWithRTCooldown(idx, cooldown)
	}
	if !automatic {
		g.spellInputCooldown = rtActionStagger
	}
	return true
}
func (g *MMGame) consumeCharacterActionWithRTCooldown(idx, cooldown int) {
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
	return clampRTCooldown(frames * (100 - pct) / 100)
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
func (g *MMGame) tickRareClassClocks(frames int) {
	g.spatialReuseFrames = max(0, g.spatialReuseFrames-frames)
	for _, c := range g.party.Members {
		if c == nil {
			continue
		}
		c.RareClass.Tick(frames)
		c.SpellState.Tick(frames)
		if c.HasCondition(character.ConditionDead) || c.HasCondition(character.ConditionEradicated) {
			c.RareClass.Anchor = character.SpatialAnchor{}
		}
	}
}
func (g *MMGame) notifyPilgrimDisplacement(x, y float64) {
	if g.party == nil || g.camera == nil {
		return
	}
	distance := math.Hypot(g.camera.X-x, g.camera.Y-y)
	for _, c := range g.party.Members {
		if c == nil || !c.CanUseCombatAction() || !c.HasSkill(character.SkillFlowingStaff) {
			continue
		}
		c.RareClass.FlowDistance += distance
		if c.RareClass.FlowDistance >= float64(g.config.GetTileSize()) {
			c.RareClass.FlowDistance = math.Mod(c.RareClass.FlowDistance, float64(g.config.GetTileSize()))
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
		for _, m := range g.world.Monsters {
			if m != nil && m.IsAlive() && TileIndex(m.X, ts) == TileIndex(x, ts) && TileIndex(m.Y, ts) == TileIndex(y, ts) {
				return false
			}
		}
		return g.canMovePartyTo(x, y)
	}
	var x, y float64
	if d.Kind == config.TechniqueReturnStep {
		a := c.RareClass.Anchor
		if a.Frames <= 0 || a.MapKey != region {
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
	g.setPartyPosition(x, y)
	g.creditAdventureMovement("spatial", oldX, oldY, x, y)
	g.notifyPilgrimDisplacement(oldX, oldY)
	if d.Kind == config.TechniqueFoldStep {
		// Keep the FIRST departure point across successive Folds; refreshing
		// its lifetime must not replace it with the last hop's origin.
		if c.RareClass.Anchor.Frames <= 0 || c.RareClass.Anchor.MapKey != region {
			c.RareClass.Anchor = character.SpatialAnchor{MapKey: region, X: lx, Y: ly}
		}
		c.RareClass.Anchor.Frames = config.TierValue(d.Duration, tier) * g.config.GetTPS()
	} else {
		c.RareClass.Anchor = character.SpatialAnchor{}
	}
	g.spatialReuseFrames = d.ReuseSeconds * g.config.GetTPS()
	if g.turnBasedMode {
		g.spatialStepThisTurn = true
	}
	if g.gameLoop != nil && g.gameLoop.inputHandler != nil {
		g.gameLoop.inputHandler.applyLandingTileEffects()
	}
	return true
}
func (g *MMGame) automaticCastDelayFrames() int {
	return g.config.AutocastIntervalRounds * g.combatRoundFrames()
}

// canAutocast is independent of action points and recovery. Both spellbook
// spells and Pilgrim techniques share one personal budget; old technique
// budgets migrate into it when a save is restored.
func (g *MMGame) canAutocast(index int) bool {
	if g.party == nil || index < 0 || index >= len(g.party.Members) {
		return false
	}
	c := g.party.Members[index]
	if c == nil || !c.CanUseCombatAction() || c.SpellState.AutoFrames > 0 {
		return false
	}
	if g.turnBasedMode {
		return g.currentTurn == 0
	}
	return g.combatActorAllowed(index)
}

func (g *MMGame) finishAutocast(c *character.MMCharacter, fxStart int) {
	c.SpellState.AutoFrames = g.automaticCastDelayFrames()
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
		if !characterKnowsSpellByID(c, d.ID) || c.SpellPoints < g.combat.effectiveSpellCost(c, d.SpellPointsCost) {
			continue
		}
		if !g.spellAutocastFires(d) {
			continue
		}
		fxStart := len(g.buffFxAnims)
		outcome := g.combat.castSpell(spellCastRequest{
			ID: d.ID, Definition: d, Caster: c, Cost: g.combat.effectiveSpellCost(c, d.SpellPointsCost),
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
var spellAutocastTriggers = map[string]func(*MMGame, spells.SpellDefinition) bool{
	"dispel_magic": func(g *MMGame, d spells.SpellDefinition) bool {
		return g.partyNeedsPurify() || g.hostileMagicNearby(d.Mechanic.RadiusTiles)
	},
	"celestial_armor": func(g *MMGame, d spells.SpellDefinition) bool {
		return g.advancedBuff(d.Mechanic.Kind) == nil && g.partyEngaged()
	},
	"purify": func(g *MMGame, _ spells.SpellDefinition) bool { return g.partyNeedsPurify() },
	"haste":  func(g *MMGame, _ spells.SpellDefinition) bool { return g.partyUnderAttack() && g.partyNeedsHaste() },
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

// validateAutocastTriggers fails the boot on a kind config lets autocast with
// no trigger (its AUTO toggle would never cast) or a trigger for a kind that
// may not autocast.
func validateAutocastTriggers() error {
	for _, c := range []struct {
		what     string
		kinds    []string
		triggers []string
	}{
		{"spell mechanic", config.AutocastMechanicKinds(), slices.Sorted(maps.Keys(spellAutocastTriggers))},
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
	if g.party == nil || g.world == nil || g.gameplayPausedByOverlay() || g.automaticInventoryBlocked() || g.spellInputCooldown > 0 {
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
	it := g.party.Members[idx].Equipment[items.SlotSpell]
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
	if g.spellInputCooldown == 0 {
		g.useTechnique(g.selectedChar, key, false, true)
	}
	return true
}

package game

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func (g *MMGame) techniqueRefusal(idx int, key string) string {
	d := config.Technique(key)
	if d == nil || g.party == nil || idx < 0 || idx >= len(g.party.Members) {
		return "Unknown technique."
	}
	c := g.party.Members[idx]
	if c == nil || c.Class != character.ClassWayfarer || !c.HasSkill(character.SkillTranslocation) || !c.CanUseCombatAction() {
		return "A capable Pilgrim is required."
	}
	if !g.turnBasedMode && !g.combatActorAllowed(idx) {
		return "This Pilgrim is outside the focused party."
	}
	if c.Level < d.Level {
		return fmt.Sprintf("%s requires level %d.", d.Name, d.Level)
	}
	if g.combat != nil && g.combat.partyInsideSolidTerrain() {
		return "There is no room to use a technique here."
	}
	if d.FreeStep {
		// A step moves the party, and a rooted party cannot move (as with Jump).
		if g.partyRooted() {
			return "The party is rooted in place."
		}
		if g.turnBasedMode && g.currentTurn != 0 {
			return "Wait for the party phase."
		}
		if g.spatialReuseFrames > 0 || (g.turnBasedMode && g.spatialStepThisTurn) {
			return "Space has not settled yet."
		}
	} else if !g.canSpendCombatAction(idx) {
		return "This Pilgrim is not ready."
	}
	if c.SpellPoints < g.techniqueSPCost(c, d) {
		return "Not enough SP."
	}
	if key == "purify" && c.RareClass.PurifyFrames > 0 {
		return "Purify is recovering."
	}
	if !d.FreeStep && !g.techniqueUseful(key) {
		return "The party would not benefit."
	}
	return ""
}
func (g *MMGame) techniqueUseful(key string) bool {
	switch key {
	case "purify":
		if g.partyRooted() {
			return true
		}
		for _, c := range g.party.Members {
			if c.Purifiable() {
				return true
			}
		}
	case "phase_veil":
		_, active := g.combatBuffByID(key)
		return !active
	case "quickening":
		if _, active := g.combatBuffByID(key); active {
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
	return false
}
func (g *MMGame) techniqueSPCost(c *character.MMCharacter, d *config.TechniqueDefinition) int {
	return g.combat.effectiveSpellCost(c, config.TierValue(d.SPCost, c.SkillTier(character.SkillTranslocation)))
}
func (g *MMGame) techniqueCooldown(c *character.MMCharacter, d *config.TechniqueDefinition) int {
	if d.FreeStep || d.CooldownSeconds == 0 {
		return 0
	}
	return g.combat.SpellCooldownFrames(c, spells.SpellID(d.Key))
}

// useTechnique is the single manual/book/quick/autocast transaction. Rejection
// spends nothing; an indexed commit never borrows the selected hero's action.
func (g *MMGame) useTechnique(idx int, key string, automatic, announce bool) bool {
	if reason := g.techniqueRefusal(idx, key); reason != "" {
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
	if d.FreeStep {
		if !g.performSpatialStep(c, key, tier) {
			if announce {
				g.AddCombatMessage("No legal spatial destination.")
			}
			return false
		}
	} else {
		switch key {
		case "purify":
			for _, m := range g.party.Members {
				m.Purify()
			}
			g.partyRoot = PartyRootState{}
			c.RareClass.PurifyFrames = 6 * tps
		case "phase_veil", "quickening":
			b := TimedCombatBuff{SpellID: key, Frames: config.TierValue(d.Duration, tier) * tps, CombatClock: true, TechniqueTier: tier}
			if key == "phase_veil" {
				b.DodgePct = config.TierValue(d.Power, tier)
			} else {
				b.RecoveryPct = config.TierValue(d.Power, tier)
				b.ExtraActions = config.TierValue(d.TBPower, tier)
				b.DeferFirstTurnTick = g.turnBasedMode
			}
			g.addCombatBuff(b)
			g.updateUtilityStatus(spells.SpellID(key), b.Frames, true)
		}
	}
	cost := g.techniqueSPCost(c, d)
	c.SpellPoints -= cost
	if automatic {
		c.RareClass.AutoFrames = 3 * tps
	}
	mode := ""
	if automatic {
		mode = " automatically"
	}
	g.AddCombatMessage(fmt.Sprintf("%s uses %s%s (%d SP).", c.Name, d.Name, mode, cost))
	if !d.FreeStep {
		// Techniques spend the caster's action but never add a movement penalty.
		if g.turnBasedMode && g.currentTurn == 0 && c.ActionsRemaining > 0 {
			g.partyTechniqueActionsUsed++
		}
		g.consumeCharacterActionWithRTCooldown(idx, cooldown)
	}
	g.spellInputCooldown = rtActionStagger
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
	b, ok := g.combatBuffByID("quickening")
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
func canonicalPosition(x, y float64) (string, float64, float64) {
	if wm := world.GlobalWorldManager; wm != nil {
		if key, lx, ly, ok := wm.LocalizeWorldPos(x, y); ok {
			return key, lx, ly
		}
	}
	return currentMapKey(), x, y
}

type spatialTerrain struct {
	*world.World3D
	g         *MMGame
	region    string
	buildings map[[2]int]bool
}

func (t spatialTerrain) IsTileBlocking(x, y int) bool {
	g := t.g
	if g.world.IsTileBlockingTerrainAt(x, y) || t.buildings[[2]int{x, y}] {
		return true
	}
	ts := float64(g.config.GetTileSize())
	wx, wy := (float64(x)+0.5)*ts, (float64(y)+0.5)*ts
	key, _, _ := canonicalPosition(wx, wy)
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
func (g *MMGame) performSpatialStep(c *character.MMCharacter, key string, tier int) bool {
	if g.collisionSystem == nil || g.camera == nil || g.world == nil {
		return false
	}
	fold := config.Technique("fold_step")
	if fold == nil {
		return false
	}
	ts := float64(g.config.GetTileSize())
	oldX, oldY := g.camera.X, g.camera.Y
	region, lx, ly := canonicalPosition(oldX, oldY)
	maxTiles := config.TierValue(fold.Range, tier)
	terrain := spatialTerrain{World3D: g.world, g: g, region: region, buildings: g.buildingOccupiedTiles()}
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
		return g.collisionSystem.CanMoveTo("player", x, y)
	}
	var x, y float64
	if key == "return_step" {
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
		for tiles := maxTiles; tiles >= fold.MinRange; tiles-- {
			x, y = originX+dx*float64(tiles)*ts, originY+dy*float64(tiles)*ts
			if math.Hypot(x-oldX, y-oldY)+0.01 >= float64(fold.MinRange)*ts && legalLanding(x, y) &&
				collision.CheckMovementLine(terrain, ts, oldX, oldY, x, y) && g.collisionSystem.CheckLineOfSight(oldX, oldY, x, y) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	g.setPartyPosition(x, y)
	g.notifyPilgrimDisplacement(oldX, oldY)
	if key == "fold_step" {
		// Keep the FIRST departure point across successive Folds; refreshing
		// its lifetime must not replace it with the last hop's origin.
		if c.RareClass.Anchor.Frames <= 0 || c.RareClass.Anchor.MapKey != region {
			c.RareClass.Anchor = character.SpatialAnchor{MapKey: region, X: lx, Y: ly}
		}
		c.RareClass.Anchor.Frames = config.TierValue(fold.Duration, tier) * g.config.GetTPS()
	} else {
		c.RareClass.Anchor = character.SpatialAnchor{}
	}
	g.spatialReuseFrames = 3 * g.config.GetTPS()
	if g.turnBasedMode {
		g.spatialStepThisTurn = true
	}
	if g.gameLoop != nil && g.gameLoop.inputHandler != nil {
		g.gameLoop.inputHandler.applyLandingTileEffects()
	}
	return true
}
func (g *MMGame) automaticTechniqueTrigger(key string) bool {
	if key == "purify" {
		return g.techniqueUseful(key)
	}
	engaged, near := false, 0
	ts := float64(g.config.GetTileSize())
	for _, m := range g.world.Monsters {
		if m != nil && m.IsAlive() && !m.Bound && m.TargetsParty() {
			engaged = true
			if math.Hypot(m.X-g.camera.X, m.Y-g.camera.Y) <= 2*ts {
				near++
			}
		}
	}
	if !engaged || !g.techniqueUseful(key) {
		return false
	}
	if key == "quickening" {
		return true
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
		if c == nil || c.RareClass.AutoFrames > 0 {
			continue
		}
		for _, key := range []string{"purify", "phase_veil", "quickening"} {
			if c.RareClass.Automatic[key] && g.automaticTechniqueTrigger(key) && g.useTechnique(i, key, true, false) {
				return
			}
		}
	}
}
func (ih *InputHandler) handleSpatialStepInput() bool {
	if !ih.keys.Consume(ebiten.KeyV) {
		return false
	}
	key := "fold_step"
	if ih.keyHeld(ebiten.KeyShiftLeft) || ih.keyHeld(ebiten.KeyShiftRight) {
		key = "return_step"
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
	return string(it.SpellEffect), it.Type == items.ItemTechnique
}

func (t spatialTerrain) GetWorldBounds() (int, int) { return t.g.world.GetWorldBounds() }

// Only a fresh F press invokes a slotted spatial step. SmartAttack and repeat
// dispatch exclude it, so targeting an enemy never moves the party.
func (ih *InputHandler) tryExplicitSlottedStep() bool {
	g := ih.game
	key, ok := g.slottedTechnique(g.selectedChar)
	d := config.Technique(key)
	if !ok || d == nil || !d.FreeStep {
		return false
	}
	// Handled is independent of success: an explicit step may never fall through
	// to another hero's spell, including during the global input stagger.
	if g.spellInputCooldown == 0 {
		g.useTechnique(g.selectedChar, key, false, true)
	}
	return true
}

package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// FxPreview is the map editor's window into the game's special effects: a tiny
// sandbox MMGame whose REAL combat/render code casts, shoots and draws into an
// offscreen scene. Single source of truth by construction - the editor never
// re-implements an effect, it plays the game's own.
type FxPreview struct {
	g         *MMGame
	scene     *ebiten.Image
	sel       FxItem
	tick      int
	arena     *world.World3D
	tileItems []FxItem // exhibits placed into the arena at build time
	homeX     float64  // default camera pose (spell/weapon stage)
	homeY     float64
	homeA     float64
	stageX    float64 // where projectiles land / bursts spawn
	stageY    float64
}

// FxKind groups the preview catalog.
type FxKind int

const (
	FxSpell FxKind = iota
	FxWeapon
	FxTrap
	FxTile
	FxCard
	FxStatus
)

// FxItem is one selectable effect in the editor's FX tab.
type FxItem struct {
	Kind  FxKind
	Key   string
	Label string
	// Tile exhibits: camera pose to view them.
	camX, camY, camA float64
	// Status exhibits: the motif, staged on the hero card or a stage monster.
	status statusVisuals
	onHero bool
}

const fxStageMapKey = "fx_stage"

// fxRespawnTicks is the minimum interval between preview casts. Finite attack
// visuals and persistent fields finish before the next cast can begin.
const fxRespawnTicks = 75 // ~0.62s at 120 TPS

// fxHeroIdx is the sandbox hero whose party card the card stage represents.
const fxHeroIdx = 0

// fxStatusDummyKey is the preferred stage actor for monster status motifs.
const fxStatusDummyKey = "orc"

// fxCardExhibits is the one list of party-card effects: catalog entry, what
// starts it on the sandbox hero, and the HUD draw call that paints it.
var fxCardExhibits = []struct {
	key, label string
	start      func(g *MMGame, hero *character.MMCharacter)
	draw       func(ui *UISystem, dst *ebiten.Image, x, y, w, h int)
}{
	{"ignite", "Card: burning (ignite DoT)",
		func(_ *MMGame, h *character.MMCharacter) { h.AddCondition(character.ConditionBurning) },
		func(ui *UISystem, dst *ebiten.Image, x, y, w, h int) { ui.drawCardIgnite(dst, x, y, w, h, fxHeroIdx) }},
	{"poison", "Card: poisoned (bubbles)",
		func(_ *MMGame, h *character.MMCharacter) { h.AddCondition(character.ConditionPoisoned) },
		func(ui *UISystem, dst *ebiten.Image, x, y, w, h int) { ui.drawCardPoisonBubbles(dst, x, y, w, h) }},
	{"stun", "Card: stunned (stars)",
		func(_ *MMGame, h *character.MMCharacter) { h.AddCondition(character.ConditionStunned) },
		func(ui *UISystem, dst *ebiten.Image, x, y, w, h int) { ui.drawCardStunStars(dst, x, y, w, h) }},
	{"flame", "Card: inferno flames",
		func(g *MMGame, _ *character.MMCharacter) { g.TriggerPartyFlame(fxHeroIdx) },
		func(ui *UISystem, dst *ebiten.Image, x, y, w, h int) { ui.drawCardFlames(dst, x, y, w, h, fxHeroIdx) }},
	{"spark", "Card: damage taken (flash + sparks)",
		func(g *MMGame, _ *character.MMCharacter) { g.triggerDamageFx(fxHeroIdx) },
		func(ui *UISystem, dst *ebiten.Image, x, y, w, h int) { ui.drawCardSparks(dst, x, y, w, h, fxHeroIdx) }},
	{"heal", "Card: heal (rising +)",
		func(g *MMGame, _ *character.MMCharacter) { g.TriggerPartyHeal(fxHeroIdx) },
		func(ui *UISystem, dst *ebiten.Image, x, y, w, h int) { ui.drawCardHealPlus(dst, x, y, w, h, fxHeroIdx) }},
}

// NewFxPreview builds the sandbox: a small flat arena world registered under
// the global world manager (created if the host app never set one), a real
// MMGame on top of it, and a caster/attacker standing at the stage edge.
func NewFxPreview(cfg *config.Config) (*FxPreview, error) {
	if world.GlobalTileManager == nil || config.GlobalSpells == nil {
		return nil, fmt.Errorf("fx preview: game data not loaded (boot.LoadGameData first)")
	}

	p := &FxPreview{}
	p.arena = p.buildArena(cfg)
	// Exhibits must land in the world BEFORE the game/renderer exist: the
	// renderer snapshots tile-driven caches (floor colors, teleporter glow
	// anchors) at construction time.
	p.tileItems = p.fxTileExhibits(cfg)
	if world.GlobalWorldManager == nil {
		world.GlobalWorldManager = world.NewWorldManager(cfg)
	}
	world.GlobalWorldManager.LoadedMaps[fxStageMapKey] = p.arena
	world.GlobalWorldManager.CurrentMapKey = fxStageMapKey

	p.g = newMMGame(cfg, true)
	p.g.turnBasedMode = false
	p.g.selectedChar = 0

	ts := float64(cfg.GetTileSize())
	p.homeX, p.homeY, p.homeA = 3.5*ts, 8.5*ts, 0 // stand west, look east down the arena
	p.stageX, p.stageY = 8.5*ts, 8.5*ts
	p.resetCamera()
	return p, nil
}

func (p *FxPreview) resetCamera() {
	p.g.camera.X, p.g.camera.Y = p.homeX, p.homeY
	p.g.snapFacing(p.homeA)
	if e := p.g.collisionSystem.GetEntityByID("player"); e != nil {
		p.g.collisionSystem.UpdateEntity("player", p.g.camera.X, p.g.camera.Y)
	}
}

// buildArena creates a flat 17x17 world with tile exhibits along the north row.
func (p *FxPreview) buildArena(cfg *config.Config) *world.World3D {
	w := buildFlatArena(cfg, 17)
	// Spawn-tile border FX anchors here - kept BEHIND the default camera (which
	// stands at x=3.5 facing east) so exhibits never photobomb the spell stage.
	w.StartX, w.StartY = 1, 12
	return w
}

// buildFlatArena creates an all-floor square world - the empty stage every
// editor preview sandbox (FX, mobs) builds on.
func buildFlatArena(cfg *config.Config, size int) *world.World3D {
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = size, size
	empty, ok := world.GlobalTileManager.GetTileTypeFromKey("empty")
	if !ok {
		// Any walkable non-solid tile works as the floor; scan for one.
		for key, td := range world.GlobalTileManager.ListTiles() {
			if td != nil && td.Walkable && !td.Solid {
				empty, _ = world.GlobalTileManager.GetTileTypeFromKey(key)
				break
			}
		}
	}
	w.Tiles = make([][]world.TileType3D, size)
	for y := 0; y < size; y++ {
		w.Tiles[y] = make([]world.TileType3D, size)
		for x := 0; x < size; x++ {
			w.Tiles[y][x] = empty
		}
	}
	return w
}

// fxTileExhibits places one instance of each tile-driven effect into the arena
// and returns the catalog entries pointing a camera at each. Data-driven: only
// tiles that actually exist in tiles.yaml become exhibits.
func (p *FxPreview) fxTileExhibits(cfg *config.Config) []FxItem {
	ts := float64(cfg.GetTileSize())
	items := []FxItem{}
	// All exhibits live along the WEST edge - behind the default east-facing
	// camera - and are viewed by their own west-facing poses.
	place := func(label string, tx, ty int, candidates ...string) {
		for _, key := range candidates {
			tt, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
			if !ok {
				continue
			}
			p.arena.Tiles[ty][tx] = tt
			items = append(items, FxItem{
				Kind: FxTile, Key: key, Label: label,
				camX: (float64(tx) + 3.5) * ts, camY: (float64(ty) + 0.5) * ts, camA: math.Pi,
			})
			return
		}
	}
	// Match the renderer's authored opt-in rule; rocks no longer imply an aura.
	var auraKeys []string
	for key, td := range world.GlobalTileManager.ListTiles() {
		tt, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
		if ok && tileShowsImpassableAura(td) && !world.GlobalTileManager.IsWalkable(tt) {
			auraKeys = append(auraKeys, key)
		}
	}
	sort.Strings(auraKeys)
	place("Impassable aura", 1, 4, auraKeys...)
	// Teleporter glow + inherit-floor tint.
	place("Teleporter glow", 1, 8, "vteleporter", "rteleporter")
	// Spawn-tile border sits at StartX/StartY - camera-only entry.
	items = append(items, FxItem{
		Kind: FxTile, Key: "spawn", Label: "Spawn tile border",
		camX: (1.0 + 3.5) * ts, camY: 12.5 * ts, camA: math.Pi,
	})
	return items
}

// Items enumerates the full preview catalog from the loaded YAML data.
func (p *FxPreview) Items() []FxItem {
	var out []FxItem

	spellKeys := make([]string, 0, len(config.GlobalSpells.Spells))
	for k := range config.GlobalSpells.Spells {
		// Only spells with a visible world effect: a flying/bursting projectile,
		// a lingering zone, a starburst, a nova ground effect, or a buff overlay
		// animation. The rest have nothing to show on the 3D stage and would be
		// an empty preview.
		def := config.GlobalSpells.Spells[k]
		if !def.IsProjectile && def.ZoneRadiusTiles <= 0 && !def.StarburstFx && def.BuffFxSprite == "" &&
			(def.Graphics == nil || def.Graphics.NovaFx == "") {
			continue
		}
		spellKeys = append(spellKeys, k)
	}
	sort.Strings(spellKeys)
	for _, k := range spellKeys {
		out = append(out, FxItem{Kind: FxSpell, Key: k, Label: config.GlobalSpells.Spells[k].Name})
	}

	weaponKeys := make([]string, 0, len(config.GlobalWeapons.Weapons))
	for k := range config.GlobalWeapons.Weapons {
		weaponKeys = append(weaponKeys, k)
	}
	sort.Strings(weaponKeys)
	for _, k := range weaponKeys {
		out = append(out, FxItem{Kind: FxWeapon, Key: k, Label: config.GlobalWeapons.Weapons[k].Name})
	}

	for _, k := range config.TrapKeysOrdered() {
		if def, ok := config.GetTrapDefinition(k); ok {
			out = append(out, FxItem{Kind: FxTrap, Key: k, Label: def.Name})
		}
	}

	out = append(out, p.tileItems...)

	for _, c := range fxCardExhibits {
		out = append(out, FxItem{Kind: FxCard, Key: c.key, Label: c.label})
	}
	for _, e := range statusVisualCatalog {
		if e.monster != nil && fxStatusDummy() != "" {
			out = append(out, FxItem{Kind: FxStatus, Key: "monster_" + e.key, Label: "Monster: " + e.label, status: e.flag})
		}
		if e.hero != nil {
			out = append(out, FxItem{Kind: FxStatus, Key: "hero_" + e.key, Label: "Hero card: " + e.label, status: e.flag, onHero: true})
		}
	}
	return out
}

// fxStatusDummy picks the stage actor for monster motifs: the preferred key,
// else the first ordinary combatant in the catalog.
func fxStatusDummy() string {
	if monster.MonsterConfig == nil {
		return ""
	}
	if _, ok := monster.MonsterConfig.Monsters[fxStatusDummyKey]; ok {
		return fxStatusDummyKey
	}
	keys := make([]string, 0, len(monster.MonsterConfig.Monsters))
	for k, def := range monster.MonsterConfig.Monsters {
		if def.HasAttackStats() && !def.Boss && def.Disposition == "" && def.Champion == "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// usesCardStage reports whether the selection paints on the party-card box.
func (it FxItem) usesCardStage() bool {
	return it.Kind == FxCard || (it.Kind == FxStatus && it.onHero)
}

// Select switches the previewed effect and fires it immediately.
func (p *FxPreview) Select(item FxItem) {
	p.sel = item
	p.tick = 0
	p.clearTransient()
	if item.Kind == FxTile {
		p.g.camera.X, p.g.camera.Y = item.camX, item.camY
		p.g.snapFacing(item.camA)
	} else {
		p.resetCamera()
	}
	p.spawn()
}

// clearTransient wipes leftover projectiles/effects so previews don't overlap.
func (p *FxPreview) clearTransient() {
	g := p.g
	// A previous utility preview (notably Fly) must not change which edges
	// qualify for the next exhibit. Reuse the shared effect reset, without
	// gameplay expiry callbacks such as return teleports.
	g.resetTimedEffects()
	g.world.SetTerrainPassageActive(false)
	g.world.SetWalkOnWaterActive(false)
	g.world.SetWaterBreathingActive(false)
	for i := range g.magicProjectiles {
		g.collisionSystem.UnregisterEntity(g.magicProjectiles[i].ID)
	}
	for i := range g.arrows {
		g.collisionSystem.UnregisterEntity(g.arrows[i].ID)
	}
	g.magicProjectiles = g.magicProjectiles[:0]
	g.arrows = g.arrows[:0]
	g.slashEffects = g.slashEffects[:0]
	g.spellHitEffects = g.spellHitEffects[:0]
	g.impactLights = g.impactLights[:0]
	g.persistentDamageZones = g.persistentDamageZones[:0]
	g.pendingMortars = g.pendingMortars[:0]
	g.traps = g.traps[:0]
	g.trapBursts = g.trapBursts[:0]
	g.buffFxAnims = g.buffFxAnims[:0]
	g.elementalAttackEffects = g.elementalAttackEffects[:0]
	g.screenShake = 0
	p.clearStage()
}

// clearStage removes staged status actors and every condition an exhibit put
// on the sandbox hero, so one exhibit never leaks into the next.
func (p *FxPreview) clearStage() {
	g := p.g
	for _, m := range g.world.Monsters {
		g.collisionSystem.UnregisterEntity(m.ID)
	}
	g.world.Monsters = g.world.Monsters[:0]
	g.partyRoot = PartyRootState{}
	g.partyHinder = PartyHinderState{}
	if len(g.party.Members) > fxHeroIdx {
		h := g.party.Members[fxHeroIdx]
		h.Conditions = h.Conditions[:0]
	}
}

// caster returns the sandbox hero, topped up so any cast/attack succeeds.
func (p *FxPreview) caster() *character.MMCharacter {
	if len(p.g.party.Members) <= fxHeroIdx {
		return nil
	}
	m := p.g.party.Members[fxHeroIdx]
	m.SpellPoints = 9999
	m.HitPoints = m.MaxHitPoints
	m.RTCooldown = 0
	return m
}

// spawn (re)fires the selected effect through the game's own combat paths.
func (p *FxPreview) spawn() {
	g := p.g
	m := p.caster()
	if m == nil {
		return
	}
	switch p.sel.Kind {
	case FxSpell:
		id := spells.SpellID(p.sel.Key)
		def, err := spells.GetSpellDefinitionByID(id)
		if err != nil {
			return
		}
		buffAnimsBefore := len(g.buffFxAnims)
		hitFxBefore := len(g.spellHitEffects)
		g.combat.castResolvedSpell(id, def, m, 0, false, false)
		// A sandbox cast can no-op (buff already active from the previous loop,
		// hero lacks the school) and refund - the gate then skips the overlay.
		// The tab's job is showing the art, so force-play it in that case.
		if cfgDef, ok := config.GetSpellDefinition(p.sel.Key); ok && cfgDef != nil &&
			cfgDef.BuffFxSprite != "" && len(g.buffFxAnims) == buffAnimsBefore {
			g.playBuffFx(cfgDef.BuffFxSprite)
		}
		// Same for a nova's ground effect: the stage has no open sky, so an
		// outdoor_only quake refunds itself and paints nothing. Play it anyway -
		// the tab's job is showing the art.
		if cfgDef, ok := config.GetSpellDefinition(p.sel.Key); ok && cfgDef != nil &&
			cfgDef.Graphics != nil && cfgDef.Graphics.NovaFx != "" && len(g.spellHitEffects) == hitFxBefore {
			radius := def.PartyAoeRadiusTiles
			if def.MapWide || radius <= 0 {
				radius = mapWideNovaFxRadiusTiles
			}
			g.spawnNovaFx(p.sel.Key, g.camera.X, g.camera.Y, radius)
		}
	case FxWeapon:
		if def, ok := config.GetWeaponDefinition(p.sel.Key); ok && def != nil {
			// Straight into the slot: sandbox hero wields anything, class gates
			// don't apply to a preview.
			m.Equipment[items.SlotMainHand] = items.Item{Name: def.Name, Type: items.ItemWeapon}
			g.combat.EquipmentMeleeAttack()
		}
	case FxTrap:
		if def, ok := config.GetTrapDefinition(p.sel.Key); ok && def != nil {
			g.traps = g.traps[:0]
			ts := float64(g.config.GetTileSize())
			tx, ty := TileIndex(p.stageX, ts), TileIndex(p.stageY, ts)
			g.traps = append(g.traps, PlacedTrap{
				Key: p.sel.Key, MapKey: fxStageMapKey,
				TileX: tx, TileY: ty,
				X: (float64(tx) + 0.5) * ts, Y: (float64(ty) + 0.5) * ts,
				Owner: m, FramesLeft: 3 * g.config.GetTPS(),
			})
		}
	case FxTile:
		// Static world FX - nothing to spawn; the camera already points at it.
	case FxCard:
		for _, c := range fxCardExhibits {
			if c.key == p.sel.Key {
				c.start(g, m)
			}
		}
	case FxStatus:
		p.stageStatus(m)
	}
}

// refreshStagedStatus re-applies the selected motif's state to the staged
// actor; every catalog setter refreshes rather than stacks.
func (p *FxPreview) refreshStagedStatus() {
	e, ok := statusVisualEntryFor(p.sel.status)
	if !ok {
		return
	}
	if len(p.g.world.Monsters) != 1 {
		p.stageStatus(p.caster())
		return
	}
	e.monster(p.g.world.Monsters[0])
}

// stageStatus reproduces the motif from real game state: on the hero for card
// motifs, on a fresh stage monster facing the camera for world motifs.
func (p *FxPreview) stageStatus(hero *character.MMCharacter) {
	e, ok := statusVisualEntryFor(p.sel.status)
	if !ok {
		return
	}
	p.clearStage()
	g := p.g
	if p.sel.onHero {
		e.hero(g, hero)
		return
	}
	key := fxStatusDummy()
	def, err := monster.MonsterConfig.GetMonsterByKey(key)
	if err != nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	m := monster.NewMonster3DFromConfig(p.homeX+previewStageDistanceTiles(def)*ts, p.homeY, key, g.config)
	if m == nil {
		return
	}
	m.Direction = math.Atan2(p.homeY-m.Y, p.homeX-m.X)
	e.monster(m)
	g.registerSpawnedMonster(m)
}

// resolveStageImpacts gives the empty preview stage a target plane. A burst
// belongs to the flying projectile and starts only when it reaches that plane.
// Use the game's impact handlers so ranged weapons and spell variants keep
// their normal impact style; utility and zone spells retain their cast visuals.
func (p *FxPreview) resolveStageImpacts() {
	g := p.g
	dx, dy := math.Cos(p.homeA), math.Sin(p.homeA)
	contact := func(x, y, vx, vy, rangeTiles float64) (float64, float64, bool) {
		distance := (p.stageX-p.homeX)*dx + (p.stageY-p.homeY)*dy
		// Short-range shots must reach the exhibit before their lifetime ends.
		if rangeTiles > 0 {
			distance = math.Min(distance, rangeTiles*float64(g.config.GetTileSize())*.8)
		}
		beyond := (x-p.homeX)*dx + (y-p.homeY)*dy - distance
		speed := vx*dx + vy*dy
		if beyond < 0 || speed <= 0 {
			return 0, 0, false
		}
		return x - vx*beyond/speed, y - vy*beyond/speed, true
	}
	for i := range g.magicProjectiles {
		shot := &g.magicProjectiles[i]
		if !shot.Active || shot.NoCollide {
			continue
		}
		def, ok := config.GetSpellDefinition(shot.SpellType)
		if !ok || def == nil || def.DealsNoDamage || def.ZoneRadiusTiles > 0 {
			continue
		}
		physics, err := g.config.GetSpellConfig(shot.SpellType)
		if err != nil || physics == nil {
			continue
		}
		x, y, hit := contact(shot.X, shot.Y, shot.VelX, shot.VelY, physics.RangeTiles)
		if !hit {
			continue
		}
		shot.X, shot.Y = x, y
		wrapper := MagicProjectileWrapper{MagicProjectile: shot, game: g}
		wrapper.OnCollision(x, y)
		wrapper.ApplyCollisionEffects()
		g.collisionSystem.UnregisterEntity(shot.ID)
	}
	for i := range g.arrows {
		shot := &g.arrows[i]
		if !shot.Active {
			continue
		}
		def, ok := config.GetWeaponDefinition(shot.BowKey)
		if !ok || def == nil || def.Physics == nil {
			continue
		}
		x, y, hit := contact(shot.X, shot.Y, shot.VelX, shot.VelY, def.Physics.RangeTiles)
		if !hit {
			continue
		}
		shot.X, shot.Y = x, y
		wrapper := ArrowWrapper{Arrow: shot, game: g}
		wrapper.OnCollision(x, y)
		wrapper.ApplyCollisionEffects()
		g.collisionSystem.UnregisterEntity(shot.ID)
	}
}

// Step advances the sandbox one tick - the same sub-updates the game loop runs
// for effects, minus input/monsters.
func (p *FxPreview) Step() {
	// Editor preview sandboxes share the global world manager; re-pin our stage
	// in case another preview tab switched the current map.
	world.GlobalWorldManager.CurrentMapKey = fxStageMapKey
	g := p.g
	gl := g.gameLoop
	g.updateInterfacePresentation()
	g.frameCount++
	gl.updateSpecialEffects()
	gl.updateProjectilesAndImpacts()
	p.resolveStageImpacts()

	p.tick++
	interval := fxRespawnTicks
	if p.sel.Kind == FxTrap {
		// Show the armed device, then its actual activation and complete fade.
		armedTicks := g.config.GetTPS() * 3 / 2
		if p.tick == armedTicks {
			for _, trap := range g.traps {
				g.startTrapBurst(trap)
			}
			g.traps = g.traps[:0]
		}
		interval = armedTicks + int(math.Ceil(trapBurstSeconds*float64(g.config.GetTPS()))) + 15
	}
	if p.tick >= interval {
		if (p.sel.Kind == FxSpell || p.sel.Kind == FxWeapon) && p.attackVisualsActive() {
			return
		}
		p.tick = 0
		// Refresh a staged monster in place: the effects pass ticks control
		// timers (Charm, Bind), and a new actor would reseed its animation.
		if p.sel.Kind == FxStatus && !p.sel.onHero {
			p.refreshStagedStatus()
			return
		}
		if p.sel.usesCardStage() {
			p.clearStage()
		}
		p.spawn()
	}
}

// attackVisualsActive keeps a full flight, contact burst and fade in one loop.
// Fields keep their authored lifetime instead of stacking a new cast each loop.
func (p *FxPreview) attackVisualsActive() bool {
	g := p.g
	for _, shot := range g.magicProjectiles {
		if shot.Active {
			return true
		}
	}
	for _, shot := range g.arrows {
		if shot.Active {
			return true
		}
	}
	for _, slash := range g.slashEffects {
		if slash.Active {
			return true
		}
	}
	for _, hit := range g.spellHitEffects {
		if hit.Active {
			return true
		}
	}
	return len(g.buffFxAnims) > 0 || len(g.elementalAttackEffects) > 0 ||
		len(g.persistentDamageZones) > 0 || len(g.pendingMortars) > 0
}

// Scene renders the sandbox through the real renderer into an offscreen image
// sized to the game's world resolution (the renderer's projection math reads
// that size, not the target's bounds). The editor scales it into its panel.
func (p *FxPreview) Scene() *ebiten.Image {
	cw, ch := p.g.worldWidth(), p.g.worldHeight()
	if p.scene == nil || p.scene.Bounds().Dx() != cw || p.scene.Bounds().Dy() != ch {
		p.scene = ebiten.NewImage(cw, ch)
	}
	p.scene.Clear()
	p.g.gameLoop.renderer.RenderFirstPersonView(p.scene)
	if p.sel.usesCardStage() {
		p.drawCardStage(p.scene)
	}
	return p.scene
}

// drawCardStage draws one oversized party-card box centre-screen and plays the
// selected card FX over it - the same UISystem draw calls the HUD uses.
func (p *FxPreview) drawCardStage(screen *ebiten.Image) {
	cw, ch := p.g.worldWidth(), p.g.worldHeight()
	w, h := 220, 300
	x, y := (cw-w)/2, (ch-h)/2
	drawFilledRect(screen, x, y, w, h, color.RGBA{30, 30, 50, 235})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{150, 150, 190, 255})
	ui := p.g.gameLoop.ui
	if p.sel.Kind == FxStatus {
		// Same painter and state derivation as the HUD's party card.
		hero := p.g.party.Members[fxHeroIdx]
		statusCanvas{dst: screen, x: float64(x), y: float64(y), w: float64(w), h: float64(h),
			clock: ui.partyStatusClock(fxHeroIdx)}.draw(p.g.partyStatusVisuals(hero))
		return
	}
	for _, c := range fxCardExhibits {
		if c.key == p.sel.Key {
			c.draw(ui, screen, x, y, w, h)
		}
	}
}

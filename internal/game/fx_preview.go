package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// FxPreview is the map editor's window into the game's special effects: a tiny
// sandbox MMGame whose REAL combat/render code casts, shoots and draws into an
// offscreen scene. Single source of truth by construction — the editor never
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
)

// FxItem is one selectable effect in the editor's FX tab.
type FxItem struct {
	Kind  FxKind
	Key   string
	Label string
	// Tile exhibits: camera pose to view them.
	camX, camY, camA float64
}

const fxStageMapKey = "fx_stage"

// fxRespawnTicks is how often the selected effect re-fires so it loops.
const fxRespawnTicks = 75

// tile exhibit slots (tile coords in the arena).
type fxExhibit struct {
	label   string
	tileKey string   // primary tile placed at the exhibit spot
	extra   []string // fallback candidates if tileKey is absent in tiles.yaml
}

// NewFxPreview builds the sandbox: a small flat arena world registered under
// the global world manager (created if the host app never set one), a real
// MMGame on top of it, and a caster/attacker standing at the stage edge.
func NewFxPreview(cfg *config.Config) (*FxPreview, error) {
	if world.GlobalTileManager == nil || config.GlobalSpells == nil {
		return nil, fmt.Errorf("fx preview: game data not loaded (boot.LoadGameData first)")
	}

	p := &FxPreview{}
	arena, err := p.buildArena(cfg)
	if err != nil {
		return nil, err
	}
	p.arena = arena
	// Exhibits must land in the world BEFORE the game/renderer exist: the
	// renderer snapshots tile-driven caches (floor colors, teleporter glow
	// anchors) at construction time.
	p.tileItems = p.fxTileExhibits(cfg)
	if world.GlobalWorldManager == nil {
		world.GlobalWorldManager = world.NewWorldManager(cfg)
	}
	world.GlobalWorldManager.LoadedMaps[fxStageMapKey] = arena
	world.GlobalWorldManager.CurrentMapKey = fxStageMapKey

	p.g = NewMMGame(cfg)
	p.g.turnBasedMode = false
	p.g.selectedChar = 0

	ts := float64(cfg.GetTileSize())
	p.homeX, p.homeY, p.homeA = 3.5*ts, 8.5*ts, 0 // stand west, look east down the arena
	p.stageX, p.stageY = 8.5*ts, 8.5*ts
	p.resetCamera()
	return p, nil
}

func (p *FxPreview) resetCamera() {
	p.g.camera.X, p.g.camera.Y, p.g.camera.Angle = p.homeX, p.homeY, p.homeA
	if e := p.g.collisionSystem.GetEntityByID("player"); e != nil {
		p.g.collisionSystem.UpdateEntity("player", p.g.camera.X, p.g.camera.Y)
	}
}

// buildArena creates a flat 17x17 world with tile exhibits along the north row.
func (p *FxPreview) buildArena(cfg *config.Config) (*world.World3D, error) {
	w := buildFlatArena(cfg, 17)
	// Spawn-tile border FX anchors here — kept BEHIND the default camera (which
	// stands at x=3.5 facing east) so exhibits never photobomb the spell stage.
	w.StartX, w.StartY = 1, 12
	return w, nil
}

// buildFlatArena creates an all-floor square world — the empty stage every
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
	// All exhibits live along the WEST edge — behind the default east-facing
	// camera — and are viewed by their own west-facing poses.
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
	// Impassable-aura billboard (rock/cliff bubble outline).
	place("Impassable aura", 1, 4, "moss_rock", "rock", "cliff")
	// Teleporter glow + inherit-floor tint.
	place("Teleporter glow", 1, 8, "vteleporter", "rteleporter")
	// Spawn-tile border sits at StartX/StartY — camera-only entry.
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
		// a lingering zone, a starburst, or a buff overlay animation. The rest
		// have nothing to show on the 3D stage and would be an empty preview.
		def := config.GlobalSpells.Spells[k]
		if !def.IsProjectile && def.ZoneRadiusTiles <= 0 && !def.StarburstFx && def.BuffFxSprite == "" {
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

	for _, c := range []struct{ key, label string }{
		{"ignite", "Card: burning (ignite DoT)"},
		{"poison", "Card: poisoned (bubbles)"},
		{"stun", "Card: stunned (stars)"},
		{"flame", "Card: inferno flames"},
		{"spark", "Card: damage taken (flash + sparks)"},
		{"heal", "Card: heal (rising +)"},
	} {
		out = append(out, FxItem{Kind: FxCard, Key: c.key, Label: c.label})
	}
	return out
}

// Select switches the previewed effect and fires it immediately.
func (p *FxPreview) Select(item FxItem) {
	p.sel = item
	p.tick = 0
	p.clearTransient()
	if item.Kind == FxTile {
		p.g.camera.X, p.g.camera.Y, p.g.camera.Angle = item.camX, item.camY, item.camA
	} else {
		p.resetCamera()
	}
	p.spawn()
}

// clearTransient wipes leftover projectiles/effects so previews don't overlap.
func (p *FxPreview) clearTransient() {
	g := p.g
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
	g.steamZones = g.steamZones[:0]
	g.traps = g.traps[:0]
	g.screenShake = 0
}

// caster returns the sandbox hero, topped up so any cast/attack succeeds.
func (p *FxPreview) caster() *character.MMCharacter {
	if len(p.g.party.Members) == 0 {
		return nil
	}
	m := p.g.party.Members[0]
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
		g.combat.castResolvedSpell(id, def, m, 0, false)
		// A sandbox cast can no-op (buff already active from the previous loop,
		// hero lacks the school) and refund — the gate then skips the overlay.
		// The tab's job is showing the art, so force-play it in that case.
		if cfgDef, ok := config.GetSpellDefinition(p.sel.Key); ok && cfgDef != nil &&
			cfgDef.BuffFxSprite != "" && len(g.buffFxAnims) == buffAnimsBefore {
			g.playBuffFx(cfgDef.BuffFxSprite)
		}
		// Impact burst at the stage point — ONLY for damage-dealing projectile
		// spells (in the game this burst fires when the bolt lands on a target;
		// the sandbox has no targets). Utility/buff/zone spells show exactly
		// what the game shows for them: their cast visuals, no explosion.
		if cfgDef, ok := config.GetSpellDefinition(p.sel.Key); ok && cfgDef != nil &&
			cfgDef.IsProjectile && !def.DealsNoDamage && cfgDef.ZoneRadiusTiles == 0 {
			g.CreateSpellHitEffectFromSpell(p.stageX, p.stageY, p.sel.Key)
			// In the game the falling stars trigger on the target hit; the
			// sandbox has no targets, so drop them at the stage point too.
			if cfgDef.StarburstFx {
				radius := def.AoeRadiusTiles
				if radius <= 0 {
					radius = 1
				}
				g.spawnStarburstFx(p.stageX, p.stageY, radius)
			}
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
			ts := float64(g.config.GetTileSize())
			tx, ty := int(p.stageX/ts), int(p.stageY/ts)
			g.traps = append(g.traps, PlacedTrap{
				Key: p.sel.Key, MapKey: fxStageMapKey,
				TileX: tx, TileY: ty,
				X: (float64(tx) + 0.5) * ts, Y: (float64(ty) + 0.5) * ts,
				Owner: m, FramesLeft: fxRespawnTicks + 30,
			})
		}
	case FxTile:
		// Static world FX — nothing to spawn; the camera already points at it.
	case FxCard:
		idx := 0
		switch p.sel.Key {
		case "ignite":
			m.AddCondition(character.ConditionBurning)
		case "poison":
			m.AddCondition(character.ConditionPoisoned)
		case "stun":
			m.AddCondition(character.ConditionStunned)
		case "flame":
			g.TriggerPartyFlame(idx)
		case "spark":
			g.TriggerDamageBlink(idx)
		case "heal":
			g.TriggerPartyHeal(idx)
		}
	}
}

// Step advances the sandbox one tick — the same sub-updates the game loop runs
// for effects, minus input/monsters.
func (p *FxPreview) Step() {
	// Editor preview sandboxes share the global world manager; re-pin our stage
	// in case another preview tab switched the current map.
	world.GlobalWorldManager.CurrentMapKey = fxStageMapKey
	g := p.g
	gl := g.gameLoop
	g.frameCount++
	if gl.hasActiveProjectiles() {
		gl.updateProjectilesParallel()
	}
	if len(g.slashEffects) > 0 {
		gl.updateSlashEffects()
	}
	if len(g.spellHitEffects) > 0 {
		g.UpdateHitEffects()
	}
	gl.updateSteamZonesRT()
	gl.updateSpecialEffects()
	g.UpdateDamageBlinkTimers()

	p.tick++
	if p.tick >= fxRespawnTicks {
		p.tick = 0
		if p.sel.Kind == FxCard {
			p.clearCardConditions()
		}
		p.spawn()
	}
}

func (p *FxPreview) clearCardConditions() {
	if len(p.g.party.Members) == 0 {
		return
	}
	m := p.g.party.Members[0]
	m.RemoveCondition(character.ConditionBurning)
	m.RemoveCondition(character.ConditionPoisoned)
	m.RemoveCondition(character.ConditionStunned)
}

// Scene renders the sandbox through the real renderer into an offscreen image
// sized to the game's configured resolution (the renderer's projection math
// reads config dimensions, not the target's bounds). The editor scales it into
// its panel.
func (p *FxPreview) Scene() *ebiten.Image {
	cw, ch := p.g.config.GetScreenWidth(), p.g.config.GetScreenHeight()
	if p.scene == nil || p.scene.Bounds().Dx() != cw || p.scene.Bounds().Dy() != ch {
		p.scene = ebiten.NewImage(cw, ch)
	}
	p.scene.Clear()
	p.g.gameLoop.renderer.RenderFirstPersonView(p.scene)
	if p.sel.Kind == FxCard {
		p.drawCardStage(p.scene)
	}
	return p.scene
}

// drawCardStage draws one oversized party-card box centre-screen and plays the
// selected card FX over it — the same UISystem draw calls the HUD uses.
func (p *FxPreview) drawCardStage(screen *ebiten.Image) {
	cw, ch := p.g.config.GetScreenWidth(), p.g.config.GetScreenHeight()
	w, h := 220, 300
	x, y := (cw-w)/2, (ch-h)/2
	drawFilledRect(screen, x, y, w, h, color.RGBA{30, 30, 50, 235})
	drawRectBorder(screen, x, y, w, h, 2, color.RGBA{150, 150, 190, 255})
	ui := p.g.gameLoop.ui
	idx := 0
	switch p.sel.Key {
	case "ignite":
		ui.drawCardIgnite(screen, x, y, w, h, idx)
	case "poison":
		ui.drawCardPoisonBubbles(screen, x, y, w, h)
	case "stun":
		ui.drawCardStunStars(screen, x, y, w, h)
	case "flame":
		ui.drawCardFlames(screen, x, y, w, h, idx)
	case "spark":
		ui.drawCardSparks(screen, x, y, w, h, idx)
	case "heal":
		ui.drawCardHealPlus(screen, x, y, w, h, idx)
	}
}

package game

import (
	"math"
	"math/rand"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// monsterCorpse is presentation only. Dead actors leave AI, collision, combat
// targeting and save rosters immediately; rewards are never tied to this timer.
type monsterCorpse struct {
	key, spriteName, animation string
	x, y, sizeTiles, yaw       float64
	mirror                     bool
	flying                     bool
	arborealHeight             float64
	tintR, tintG, tintB        float32
	started                    int64
	frameCount                 int
}

type lootHop struct {
	fromX, fromY float64
	started      int64
	duration     int64
	heightTiles  float64
}

func (h lootHop) progress(tick int64) float64 {
	if h.duration <= 0 {
		return 1
	}
	return math.Max(0, math.Min(1, float64(tick-h.started)/float64(h.duration)))
}
func (h lootHop) active(tick int64) bool  { return h.duration > 0 && h.progress(tick) < 1 }
func (h lootHop) waiting(tick int64) bool { return h.duration > 0 && tick < h.started }

func (g *MMGame) monsterDeathSettings() config.MonsterDeathRenderConfig {
	if g.config != nil && g.config.Graphics.Monster.Death.FPS > 0 {
		return g.config.Graphics.Monster.Death
	}
	return config.DefaultMonsterDeathRenderConfig()
}

func (g *MMGame) monsterDeathAnimation(m *monster.Monster3D) (string, int) {
	if g == nil || g.sprites == nil || m == nil {
		return "", 0
	}
	for _, name := range []string{"dying_r", "dying_l"} {
		if count := g.sprites.AnimationFrameCount(m.GetSpriteType(), name); count > 0 {
			return name, count
		}
	}
	return "", 0
}

func (g *MMGame) beginMonsterDeath(m *monster.Monster3D) {
	animation, frames := g.monsterDeathAnimation(m)
	if animation == "" {
		return
	}
	x, y := m.X, m.Y
	if g.combat != nil {
		x, y = g.combat.monsterVisualPos(m)
	}
	yaw := m.StandeeYaw
	if m.StandeeYawTick == 0 {
		yaw = math.Atan2(y-g.camera.Y, x-g.camera.X) + math.Pi/2
	}
	mirror := m.StandeeMirror
	// A frozen frontal heading has no decisive mirror. Preserve its previous
	// facing while accounting for a death sheet authored in the opposite direction.
	liveLeft := false
	kinds := []string{"walking"}
	if m.AttackAnimFrames > 0 {
		kinds = []string{"attacking", "walking"}
	}
	for _, kind := range kinds {
		if g.sprites.AnimationFrameCount(m.GetSpriteType(), kind+"_r") > 0 {
			break
		}
		if g.sprites.AnimationFrameCount(m.GetSpriteType(), kind+"_l") > 0 {
			liveLeft = true
			break
		}
	}
	mirror = mirror != (liveLeft != (animation == "dying_l"))
	// Resolve facing from the death sheet itself, including left-only assets.
	if resolved, decisive := standeeMirrorFor(g.camera.Angle, yaw, m.Direction, animation == "dying_l"); decisive {
		mirror = resolved
	}
	g.monsterCorpses = append(g.monsterCorpses, monsterCorpse{
		key: m.Key, spriteName: m.GetSpriteType(), animation: animation,
		x: x, y: y, sizeTiles: m.GetSizeGameMultiplier(), yaw: yaw,
		mirror: mirror, flying: m.Flying, tintR: m.TintR, tintG: m.TintG, tintB: m.TintB,
		arborealHeight: m.Arbor.Height,
		started:        g.frameCount, frameCount: frames,
	})
}

func (g *MMGame) corpseFrameAndOpacity(c *monsterCorpse) (int, float32) {
	settings := g.monsterDeathSettings()
	age := math.Max(0, float64(g.frameCount-c.started)/float64(g.config.GetTPS()))
	last := max(0, c.frameCount-1)
	frame := min(last, int(age*float64(settings.FPS)))
	fadeStart := float64(last) / float64(settings.FPS)
	if c.flying || c.arborealHeight > 0 {
		fadeStart = math.Max(fadeStart, settings.FallSeconds)
	}
	fadeAge := math.Max(0, age-fadeStart)
	return frame, float32(math.Max(0, 1-fadeAge/settings.FadeSeconds))
}

// World ticks advance in both RT and TB, but stop behind loading and modals.
func (g *MMGame) updateMonsterDeaths() {
	kept := g.monsterCorpses[:0]
	for _, corpse := range g.monsterCorpses {
		if _, opacity := g.corpseFrameAndOpacity(&corpse); opacity > 0 {
			kept = append(kept, corpse)
		}
	}
	clear(g.monsterCorpses[len(kept):])
	g.monsterCorpses = kept
}

func (g *MMGame) monsterLootLanding(m *monster.Monster3D) (float64, float64) {
	if m.Arbor.Phase != "" && g.world != nil && g.world.CanMoveTo(m.Arbor.GroundX, m.Arbor.GroundY) {
		return m.Arbor.GroundX, m.Arbor.GroundY
	}
	w := g.GetCurrentWorld()
	if w == nil {
		return m.X, m.Y
	}
	ts := g.config.GetTileSize()
	tx, ty := TileIndex(m.X, ts), TileIndex(m.Y, ts)
	// Cardinal neighbors avoid throwing loot diagonally through wall corners.
	offsets := [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
	start := rand.Intn(len(offsets))
	for i := range offsets {
		off := offsets[(start+i)%len(offsets)]
		nx, ny := tx+off[0], ty+off[1]
		if nx < 0 || ny < 0 || nx >= w.Width || ny >= w.Height || !w.CanMoveTo((float64(nx)+0.5)*ts, (float64(ny)+0.5)*ts) {
			continue
		}
		x, y := TileCenterFromTile(nx, ny, ts)
		// Use the party's collision rule as well, so closed doors and authored
		// solid props cannot receive a bag. Terrain above ignores active Fly.
		if g.collisionSystem != nil && g.collisionSystem.GetEntityByID("player") != nil && !g.collisionSystem.CanMoveTo("player", x, y) {
			continue
		}
		return x, y
	}
	return m.X, m.Y
}

// Both ordinary rewards and champion trophies use this path. The persistent
// container is placed at its destination immediately, even during the hop.
func (g *MMGame) addMonsterLootDrop(m *monster.Monster3D, drops []items.Item, gold int) {
	if m == nil || (len(drops) == 0 && gold <= 0) {
		return
	}
	x, y := m.X, m.Y
	hop := lootHop{}
	if animation, _ := g.monsterDeathAnimation(m); animation != "" {
		x, y = g.monsterLootLanding(m)
		sx, sy := m.X, m.Y
		if g.combat != nil {
			sx, sy = g.combat.monsterVisualPos(m)
		}
		settings := g.monsterDeathSettings()
		hop = lootHop{fromX: sx, fromY: sy, started: g.frameCount,
			duration: max(1, int64(settings.LootHopSeconds*float64(g.config.GetTPS()))), heightTiles: settings.LootHopHeightTiles}
		if m.Flying || m.Arbor.Height > 0 {
			hop.started += int64(math.Ceil(settings.FallSeconds * float64(g.config.GetTPS())))
		}
	}
	before := len(g.groundContainers)
	g.addLootBagDrop(x, y, drops, gold)
	if len(g.groundContainers) > before {
		g.groundContainers[before].hop = hop
	}
}

func (g *MMGame) lootHopHeight(c *GroundContainer) float64 {
	t := c.hop.progress(g.frameCount)
	return 4 * t * (1 - t) * c.hop.heightTiles
}

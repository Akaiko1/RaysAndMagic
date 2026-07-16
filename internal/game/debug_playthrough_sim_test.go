//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Headless PLAYTHROUGH sim - a DEBUG MODULE, not a regression test. An agent
// actually PLAYS the game through the real runtime: the full boot sequence
// (main.go's config set), a real NewMMGame on the real starting map, and the
// real GameLoop.updateExploration ticking input, monsters, projectiles, procs
// and day/night. No rendering. The bot drives the same entry points the
// keyboard does: InputHandler.moveForward for walking, EquipmentMeleeAttack /
// CastEquippedSpell / CastEquippedHealOnTarget for actions, ground-container
// pickup for loot, and the level-up choice flow for progression. The
// adventure log (every combat message with a sim-time stamp) is the output.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_Playthrough -v

import (
	"fmt"
	"math"
	"os"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// playBot is the minimal "player": pick the nearest hostile, close in, swing
// every ready hand, heal whoever is about to drop, loot the corpses.
type playBot struct {
	g  *MMGame
	cs *CombatSystem
	ih *InputHandler

	tick     int
	lastMsgs int
	pickups  int

	// Unstick state: straight-line pursuit jams on tree lines; when the party
	// stops making progress the bot sidesteps for a moment like a player would.
	lastX, lastY float64
	stuckTicks   int
	wiggleUntil  int
	wiggleSide   float64

	// Target patience: give up on a mob the party can't close on (a river or a
	// tree wall between them) and hunt something else for a while.
	curTarget    *monster.Monster3D
	bestDist     float64
	progressTick int
	shunned      map[*monster.Monster3D]int // monster -> tick when forgivable

	// Wander leg: after shunning a target, walk somewhere else for a few
	// seconds instead of oscillating between two unreachable mobs at a wall.
	wanderUntil int
	wanderAngle float64

	// Navigation: a throwaway Relentless "scout" monster runs the SAME A* the
	// monster AI uses, from the party's position toward the hunt target - the
	// party walks tile-to-tile around rivers and tree walls like the mobs do.
	scout      *monster.Monster3D
	navTileX   int
	navTileY   int
	navValid   bool
	navRecheck int
	// initial is the authored roster at map load: clearing THEM is winning
	// (night packs respawn every dusk and don't count).
	initial map[*monster.Monster3D]bool
}

// botMove walks one tick in the camera's current heading at RUN speed - a
// player holds Shift everywhere; the idle-keyboard InputHandler never would.
func (b *playBot) botMove() {
	speed := b.g.config.GetMoveSpeed() * b.g.config.GetRunMultiplier()
	b.ih.movePlayer(b.g.camera.GetForwardX()*speed, b.g.camera.GetForwardY()*speed)
}

func (b *playBot) simTime() string {
	sec := b.tick / b.g.config.GetTPS()
	return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
}

// drainLog prints every combat message the game emitted since the last tick -
// the game narrates the run itself (hits, loot, learns, level-ups). New-message
// count comes from combatLogVersion (bumped once per message; log clears reset
// the baseline), because combatLogHistory is a capped ring whose LENGTH stops
// moving once full.
func (b *playBot) drainLog() {
	newCount := b.g.combatLogVersion - b.lastMsgs
	if newCount <= 0 || len(b.g.combatLogHistory) == 0 {
		b.lastMsgs = b.g.combatLogVersion
		return
	}
	if newCount > len(b.g.combatLogHistory) {
		newCount = len(b.g.combatLogHistory)
	}
	for _, e := range b.g.combatLogHistory[len(b.g.combatLogHistory)-newCount:] {
		fmt.Printf("[%s] %s\n", b.simTime(), e.Text)
	}
	b.lastMsgs = b.g.combatLogVersion
}

// nearestHostile is the closest living non-ally monster (inert set-pieces -
// sealed bosses, ward idols - are unkillable furniture and get skipped).
func (b *playBot) nearestHostile() *monster.Monster3D {
	var best *monster.Monster3D
	bestD := math.MaxFloat64
	for _, m := range b.g.world.Monsters {
		if m == nil || !m.IsAlive() || m.Bound || m.Pacified || m.IsInertSetPiece() {
			continue
		}
		if until, bad := b.shunned[m]; bad && b.tick < until {
			continue
		}
		if b.initial != nil && !b.initial[m] {
			continue // night-pack respawns are not the goal - clear the AUTHORED roster
		}
		// Nearest-first: crossing the map toward "easy" targets aggros every
		// pack on the way (tried weakest-first - five straight wipes).
		d := math.Hypot(m.X-b.g.camera.X, m.Y-b.g.camera.Y)
		if d < bestD {
			bestD, best = d, m
		}
	}
	return best
}

// pickTarget keeps the current hunt while it makes progress and shuns a mob
// the party hasn't closed on for 10 seconds (unreachable across water/trees).
func (b *playBot) pickTarget() *monster.Monster3D {
	tps := b.g.config.GetTPS()
	if b.curTarget != nil && b.curTarget.IsAlive() {
		d := math.Hypot(b.curTarget.X-b.g.camera.X, b.curTarget.Y-b.g.camera.Y)
		if d < b.bestDist-1 {
			b.bestDist, b.progressTick = d, b.tick
		}
		if b.tick-b.progressTick > 10*tps {
			if b.shunned == nil {
				b.shunned = map[*monster.Monster3D]int{}
			}
			b.shunned[b.curTarget] = b.tick + 30*tps
			b.curTarget = nil
			// Walk away at a skewed angle for a few seconds - breaks the
			// pace-along-the-wall oscillation between unreachable mobs.
			b.wanderUntil = b.tick + 4*tps
			b.wanderAngle = math.Atan2(b.g.camera.Y-b.lastY, b.g.camera.X-b.lastX) + math.Pi/2 + float64(b.tick%3-1)*math.Pi/3
		}
	} else {
		b.curTarget = nil
	}
	if b.curTarget == nil {
		if b.curTarget = b.nearestHostile(); b.curTarget != nil {
			b.bestDist = math.Hypot(b.curTarget.X-b.g.camera.X, b.curTarget.Y-b.g.camera.Y)
			b.progressTick = b.tick
			b.navValid, b.navRecheck = false, 0
		}
	}
	return b.curTarget
}

// navHeading resolves the walk direction toward the target: the scout's A*
// next-step tile when a path exists (re-queried every half second or when the
// step tile is reached), else the straight line. ok=false means A* proved the
// target unreachable right now.
func (b *playBot) navHeading(target *monster.Monster3D) (float64, bool) {
	tile := float64(b.g.config.GetTileSize())
	cam := b.g.camera
	tx, ty := int(cam.X/tile), int(cam.Y/tile)
	if b.navValid && tx == b.navTileX && ty == b.navTileY {
		b.navValid = false // reached the step tile - ask for the next one
	}
	if !b.navValid || b.tick >= b.navRecheck {
		b.scout.X, b.scout.Y = cam.X, cam.Y
		nx, ny, ok := b.scout.NextPathStepTile(b.g.collisionSystem, target.X, target.Y)
		b.navRecheck = b.tick + b.g.config.GetTPS()/2
		if ok {
			b.navTileX, b.navTileY, b.navValid = nx, ny, true
		} else {
			b.navValid = false
			// Adjacent is fine (melee range); farther with no path = unreachable.
			if math.Hypot(target.X-cam.X, target.Y-cam.Y) > tile*2 {
				return 0, false
			}
		}
	}
	if b.navValid {
		cx := (float64(b.navTileX) + 0.5) * tile
		cy := (float64(b.navTileY) + 0.5) * tile
		return math.Atan2(cy-cam.Y, cx-cam.X), true
	}
	return math.Atan2(target.Y-cam.Y, target.X-cam.X), true
}

// resolveLevelUps opens and spends every queued level-up choice (first
// pickable option; multi-selects fill from the top) - the bot never leaves a
// character owed a pick.
func (b *playBot) resolveLevelUps() {
	for i := range b.g.party.Members {
		b.g.drainOwedChoices(i)
	}
	for guard := 0; guard < 64; guard++ {
		req := b.g.currentLevelUpChoice()
		if req == nil {
			opened := false
			for i := range b.g.party.Members {
				if b.g.hasLevelUpChoiceForChar(i) {
					b.g.openLevelUpChoiceForChar(i)
					opened = true
					break
				}
			}
			if !opened {
				return
			}
			continue
		}
		if req.isMultiSelect() {
			for i := 0; i < len(req.options) && req.selectedCount() < req.maxSelections; i++ {
				b.g.toggleLevelUpSelection(i)
			}
			b.g.confirmLevelUpSelections()
			continue
		}
		picked := false
		for i := range req.options {
			char := b.g.party.Members[req.charIndex]
			setLevelUpOptionDisplay(char, &req.options[i])
			if levelUpOptionPickable(char, &req.options[i]) {
				b.g.consumeLevelUpChoice(i)
				picked = true
				break
			}
		}
		if !picked { // everything stale - dissolve by closing
			b.g.popLevelUpChoice()
		}
	}
}

// act is one player-decision tick: face the target, walk into reach, act with
// every ready character, loot whatever is underfoot.
func (b *playBot) act() {
	tile := float64(b.g.config.GetTileSize())

	// Emergency healing: mid-fight only true emergencies (<35%) to save SP;
	// out of combat top everyone up to 60%.
	healBelow := 60
	if b.g.partyInCombat() {
		healBelow = 35
	}
	for hi, healer := range b.g.party.Members {
		if healer == nil || healer.IsIncapacitated() || !healer.AnyWeaponHandReady() {
			continue
		}
		if sp, ok := healer.Equipment[items.SlotSpell]; !ok || sp.Name == "" || healer.SpellPoints < 5 {
			continue
		}
		for ti, m := range b.g.party.Members {
			if m == nil || m.HitPoints <= 0 || m.HitPoints*100 >= m.MaxHitPoints*healBelow {
				continue
			}
			b.g.selectedChar = hi
			if b.cs.CastEquippedHealOnTarget(ti) {
				break
			}
		}
	}

	// Survival instinct: with a member critical (<25%) and the healer's SP dry,
	// disengage - run FROM the nearest hostile for a couple of seconds and let
	// regen/heals catch up before re-engaging.
	critical := false
	for _, m := range b.g.party.Members {
		if m != nil && m.HitPoints > 0 && m.HitPoints*100 < m.MaxHitPoints*25 {
			critical = true
			break
		}
	}
	if critical && b.tick >= b.wanderUntil {
		if foe := b.nearestHostile(); foe != nil {
			d := math.Hypot(foe.X-b.g.camera.X, foe.Y-b.g.camera.Y)
			// Already in its face - running just donates free hits; stand and
			// fight. Retreat only from approaching-but-not-adjacent danger.
			if d > tile*1.8 && d < tile*6 {
				b.wanderUntil = b.tick + 3*b.g.config.GetTPS()
				b.wanderAngle = math.Atan2(b.g.camera.Y-foe.Y, b.g.camera.X-foe.X)
			}
		}
	}

	// Mid-wander: keep walking the chosen way; fight only what stands in reach.
	if b.tick < b.wanderUntil {
		b.g.camera.Angle = b.wanderAngle
		b.botMove()
		if m := b.nearestHostile(); m != nil && math.Hypot(m.X-b.g.camera.X, m.Y-b.g.camera.Y) <= tile*1.6 {
			b.g.camera.Angle = math.Atan2(m.Y-b.g.camera.Y, m.X-b.g.camera.X)
			for i, mem := range b.g.party.Members {
				if mem != nil && !mem.IsIncapacitated() && mem.AnyWeaponHandReady() {
					b.g.selectedChar = i
					b.cs.EquipmentMeleeAttack()
				}
			}
		}
		b.lastX, b.lastY = b.g.camera.X, b.g.camera.Y
		return
	}

	target := b.pickTarget()
	if target == nil {
		return
	}
	dx, dy := target.X-b.g.camera.X, target.Y-b.g.camera.Y
	dist := math.Hypot(dx, dy)
	heading := math.Atan2(dy, dx)
	walkHeading, reachable := b.navHeading(target)
	if !reachable {
		// A* says there is no path right now - shun and hunt something else.
		if b.shunned == nil {
			b.shunned = map[*monster.Monster3D]int{}
		}
		b.shunned[target] = b.tick + 30*b.g.config.GetTPS()
		b.curTarget = nil
		return
	}
	b.g.camera.Angle = heading // square up like a player mousing the target
	hasLOS := b.g.collisionSystem == nil || b.g.collisionSystem.CheckLineOfSight(b.g.camera.X, b.g.camera.Y, target.X, target.Y)

	// Close the gap while out of melee reach (ranged members act on the way).
	// Straight pursuit jams on tree lines: when progress stalls, sidestep at
	// 90 degrees for a moment (alternating sides), like a player strafing
	// around an obstacle. Facing snaps back to the target for the attacks.
	if dist > tile*1.4 {
		b.g.camera.Angle = walkHeading
		if b.tick < b.wiggleUntil {
			b.g.camera.Angle = walkHeading + b.wiggleSide*math.Pi/2
		}
		b.botMove()
		if math.Hypot(b.g.camera.X-b.lastX, b.g.camera.Y-b.lastY) < 0.5 {
			b.stuckTicks++
			if b.stuckTicks > 15 {
				b.stuckTicks = 0
				b.wiggleSide = -b.wiggleSide
				if b.wiggleSide == 0 {
					b.wiggleSide = 1
				}
				b.wiggleUntil = b.tick + 45
			}
		} else {
			b.stuckTicks = 0
		}
		b.g.camera.Angle = heading
	}
	b.lastX, b.lastY = b.g.camera.X, b.g.camera.Y

	for i, m := range b.g.party.Members {
		if m == nil || m.IsIncapacitated() || !m.AnyWeaponHandReady() {
			continue
		}
		b.g.selectedChar = i
		reach := tile * 1.6
		if w, ok := m.Equipment[items.SlotMainHand]; ok {
			if def, _, found := config.GetWeaponDefinitionByName(w.Name); found && def != nil && def.Range > 1 {
				reach = tile * float64(def.Range)
			}
		}
		switch {
		case dist <= reach && (hasLOS || reach <= tile*1.6):
			// No player shoots a bow into a tree wall: ranged reach needs LOS.
			b.cs.EquipmentMeleeAttack()
		case dist <= tile*8 && hasLOS:
			// Out of weapon reach: casters throw their equipped spell - but only
			// a DAMAGE spell (Celestine's slot holds Heal; hurling it at goblins
			// just drains her SP).
			if sp, ok := m.Equipment[items.SlotSpell]; ok && sp.Name != "" && m.SpellPoints > 8 {
				if def, err := spells.GetSpellDefinitionByID(spells.SpellID(sp.SpellEffect)); err == nil && def.IsProjectile && !def.IsUtility {
					b.cs.CastEquippedSpell()
				}
			}
		}
	}

	// Loot anything in arm's reach (the game logs what was picked up).
	if b.g.tryPickupNearestGroundContainer(b.g.groundContainerPickupRange()) {
		b.pickups++
	}
}

func TestDebugSim_Playthrough(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	// The full main.go boot, in dependency order.
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("assets/weapons.yaml"); err != nil {
		t.Fatalf("weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("assets/items.yaml"); err != nil {
		t.Fatalf("items: %v", err)
	}
	if _, err := config.LoadLootTables("assets/loots.yaml"); err != nil {
		t.Fatalf("loots: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	prevTM, prevWM, prevQM := world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager
	defer func() {
		world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager = prevTM, prevWM, prevQM
	}()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	if _, err := config.LoadTrapConfig("assets/traps.yaml"); err != nil {
		t.Fatalf("traps: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}
	if _, err := config.LoadChampionConfig("assets/champions.yaml"); err != nil {
		t.Fatalf("champions: %v", err)
	}
	if err := PrimeChampions(cfg); err != nil {
		t.Fatalf("prime champions: %v", err)
	}
	if _, err := config.LoadLevelUpConfig("assets/level_up.yaml"); err != nil {
		t.Fatalf("level-up: %v", err)
	}
	monster.MustLoadHatesConfig("assets/hates.yaml")
	questConfig, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		t.Fatalf("quests: %v", err)
	}
	simMinutes := 5
	if v := os.Getenv("RAM_PLAY_MINUTES"); v != "" {
		fmt.Sscanf(v, "%d", &simMinutes)
	}

	// runAttempt is one LIFE: fresh world manager (monsters respawn like a
	// reload), fresh game, play until the authored roster is cleared, the
	// party wipes, or the clock runs out.
	runAttempt := func(attempt int) (cleared bool) {
		quests.GlobalQuestManager = quests.NewQuestManager(questConfig)
		quests.GlobalQuestManager.InitializeStartingQuests()
		wm := world.NewWorldManager(cfg)
		if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
			t.Fatalf("map configs: %v", err)
		}
		if err := wm.LoadAllMaps(); err != nil {
			t.Fatalf("maps: %v", err)
		}
		world.GlobalWorldManager = wm

		g := NewMMGame(cfg)
		defer g.Shutdown()
		g.appScreen = AppScreenInGame

		bot := &playBot{g: g, cs: g.combat, ih: &InputHandler{game: g}}
		// The nav scout is a Relentless ground walker: same A*, map-wide window,
		// no habitat restriction - a stand-in for the party's own legs.
		bot.scout = monster.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "goblin", cfg)
		bot.scout.Relentless = true // map-wide A* window
		bot.scout.HabitatPrefs = nil
		bot.scout.Flying = false
		// The A* neighbor checks run under the scout's entity ID - an UNREGISTERED
		// id fails CanMoveToWithHabitat outright (no path, ever). Walk as the
		// party itself: the registered "player" entity's own collision box.
		bot.scout.ID = "player"
		bot.initial = make(map[*monster.Monster3D]bool, len(g.world.Monsters))
		for _, m := range g.world.Monsters {
			if m != nil && m.IsAlive() {
				bot.initial[m] = true
			}
		}
		gl := g.gameLoop
		tps := cfg.GetTPS()

		t.Logf("=== ATTEMPT %d: map %q, %d monsters, party of %d, %d TPS ===",
			attempt, wm.CurrentMapKey, len(g.world.Monsters), len(g.party.Members), tps)
		for _, m := range g.party.Members {
			t.Logf("  %-10s %-10s HP %d/%d SP %d/%d", m.Name, m.Class.String(),
				m.HitPoints, m.MaxHitPoints, m.SpellPoints, m.MaxSpellPoints)
		}
		xpAtStart := g.party.Members[0].Experience

		lastStatus := 0
		for bot.tick = 0; bot.tick < simMinutes*60*tps; bot.tick++ {
			g.frameCount++
			bot.resolveLevelUps() // a queued choice pauses the gameplay update - spend it first
			gl.updateExploration()
			bot.act() // move every tick like a held W key; attacks self-gate on cooldowns
			bot.drainLog()

			// Victory: the authored roster is cleared.
			if bot.tick%60 == 0 {
				left := 0
				for m := range bot.initial {
					if m.IsAlive() {
						left++
					}
				}
				if left == 0 {
					t.Logf("=== MAP CLEARED at %s - every authored monster is down! ===", bot.simTime())
					cleared = true
					break
				}
			}

			// Party wipe = this life ends.
			up := 0
			for _, m := range g.party.Members {
				if m != nil && !m.IsIncapacitated() {
					up++
				}
			}
			if up == 0 {
				t.Logf("=== PARTY WIPED at %s - reloading... ===", bot.simTime())
				break
			}

			// Periodic status line.
			if sec := bot.tick / tps; sec >= lastStatus+30 {
				lastStatus = sec
				alive := 0
				for m := range bot.initial {
					if m.IsAlive() {
						alive++
					}
				}
				hp := ""
				for _, m := range g.party.Members {
					hp += fmt.Sprintf(" %s:%d/%d", m.Name[:2], m.HitPoints, m.MaxHitPoints)
				}
				t.Logf("[%s] pos=(%.0f,%.0f) gold=%d authoredLeft=%d%s",
					bot.simTime(), g.camera.X, g.camera.Y, g.party.Gold, alive, hp)
			}
		}

		t.Logf("=== attempt %d over at %s: %d XP earned per member, %d pickups, gold %d ===",
			attempt, bot.simTime(), g.party.Members[0].Experience-xpAtStart, bot.pickups, g.party.Gold)
		for _, m := range g.party.Members {
			t.Logf("  %-10s Lv.%d  HP %d/%d  SP %d/%d  XP %d", m.Name, m.Level,
				m.HitPoints, m.MaxHitPoints, m.SpellPoints, m.MaxSpellPoints, m.Experience)
		}
		return cleared
	}

	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if runAttempt(attempt) {
			t.Logf("=== VICTORY on attempt %d ===", attempt)
			return
		}
	}
	t.Logf("=== no clear in %d attempts ===", maxAttempts)
}

package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	perfLowFpsThreshold = 70.0
	perfLowFpsDuration  = 3 * time.Second
	perfLogInterval     = 3 * time.Second
)

func (gl *GameLoop) maybeLogPerfDrop() {
	if !gl.game.perfDebugEnabled {
		return
	}

	fps := ebiten.ActualFPS()
	if fps >= perfLowFpsThreshold {
		gl.game.perfLowFpsSince = time.Time{}
		gl.game.perfLastPerfLog = time.Time{}
		return
	}

	now := time.Now()
	if gl.game.perfLowFpsSince.IsZero() {
		gl.game.perfLowFpsSince = now
		return
	}

	if now.Sub(gl.game.perfLowFpsSince) < perfLowFpsDuration {
		return
	}

	if !gl.game.perfLastPerfLog.IsZero() && now.Sub(gl.game.perfLastPerfLog) < perfLogInterval {
		return
	}

	gl.game.perfLastPerfLog = now
	gl.logPerfSnapshot(fps)
}

func (gl *GameLoop) logPerfSnapshot(fps float64) {
	tps := ebiten.ActualTPS()
	stats := gl.game.threading.PerformanceMonitor.GetDetailedStats()
	lastFrameMs := getPerfFloat(stats, "last_frame_time_ms")
	lastRaycastMs := getPerfFloat(stats, "last_raycast_time_ms")
	lastSpriteMs := getPerfFloat(stats, "last_sprite_render_time_ms")
	lastEntityMs := getPerfFloat(stats, "last_entity_update_time_ms")
	queuedJobs := getPerfInt(stats, "queued_jobs")
	activeWorkers := getPerfInt(stats, "active_workers")
	goroutines := getPerfInt(stats, "goroutines")
	memAllocMB := getPerfUint(stats, "memory_alloc_mb")
	memSysMB := getPerfUint(stats, "memory_sys_mb")
	gcCycles := getPerfUint(stats, "gc_cycles")

	world := gl.game.world
	worldW, worldH := 0, 0
	monsters, npcs, items, teachers := 0, 0, 0, 0
	if world != nil {
		worldW = world.Width
		worldH = world.Height
		monsters = len(world.Monsters)
		npcs = len(world.NPCs)
		items = len(world.Items)
		teachers = len(world.Teachers)
	}

	projectiles := len(gl.game.magicProjectiles) + len(gl.game.meleeAttacks) + len(gl.game.arrows)
	effects := len(gl.game.slashEffects) + len(gl.game.arrowHitEffects) + len(gl.game.spellHitEffects)

	activeUtility := 0
	for _, status := range gl.game.utilitySpellStatuses {
		if status != nil && status.Duration > 0 {
			activeUtility++
		}
	}

	causes := make([]string, 0, 6)
	if monsters > 60 {
		causes = append(causes, fmt.Sprintf("high monsters (%d)", monsters))
	}
	if projectiles > 40 {
		causes = append(causes, fmt.Sprintf("projectiles (%d)", projectiles))
	}
	if effects > 40 {
		causes = append(causes, fmt.Sprintf("effects (%d)", effects))
	}
	if items > 200 {
		causes = append(causes, fmt.Sprintf("world items (%d)", items))
	}
	if gl.game.showCollisionBoxes {
		causes = append(causes, "collision boxes")
	}
	if gl.game.mapOverlayOpen {
		causes = append(causes, "map overlay")
	}
	if gl.game.menuOpen || gl.game.mainMenuOpen {
		causes = append(causes, "menu open")
	}
	if gl.game.dialogActive {
		causes = append(causes, "dialog active")
	}

	causeText := "none obvious"
	if len(causes) > 0 {
		causeText = strings.Join(causes, ", ")
	}

	fmt.Printf(
		"[PERF] FPS<%.0f for >=%s | fps=%.1f tps=%.1f causes=%s\n",
		perfLowFpsThreshold,
		perfLowFpsDuration,
		fps,
		tps,
		causeText,
	)
	fmt.Printf(
		"[PERF] world=%dx%d monsters=%d npcs=%d items=%d teachers=%d projectiles=%d effects=%d utility=%d turnBased=%v mapOverlay=%v\n",
		worldW,
		worldH,
		monsters,
		npcs,
		items,
		teachers,
		projectiles,
		effects,
		activeUtility,
		gl.game.turnBasedMode,
		gl.game.mapOverlayOpen,
	)
	fmt.Printf(
		"[PERF] update=%.2fms draw=%.2fms busy=%.2fms budget=%.2fms idle=%.2fms update_frame=%.2fms raycast=%.2fms sprites=%.2fms entity=%.2fms workers=%d queued=%d goroutines=%d vsync=%v target_tps=%d\n",
		float64(gl.lastUpdateDuration.Microseconds())/1000.0,
		float64(gl.lastDrawDuration.Microseconds())/1000.0,
		(float64(gl.lastUpdateDuration.Microseconds())+float64(gl.lastDrawDuration.Microseconds()))/1000.0,
		frameBudgetMs(fps),
		idleBudgetMs(fps, gl.lastUpdateDuration, gl.lastDrawDuration),
		lastFrameMs,
		lastRaycastMs,
		lastSpriteMs,
		lastEntityMs,
		activeWorkers,
		queuedJobs,
		goroutines,
		ebiten.IsVsyncEnabled(),
		gl.game.config.GetTPS(),
	)
	fmt.Printf(
		"[PERF] mem_alloc=%dMB mem_sys=%dMB gc_cycles=%d\n",
		memAllocMB,
		memSysMB,
		gcCycles,
	)
	fmt.Printf(
		"[PERF] dead_monsters=%d depth_buf=%d reusable_monsters=%d/%d reusable_projectiles=%d/%d rewards_map=%d\n",
		len(gl.game.deadMonsterIDs),
		len(gl.game.depthBuffer),
		len(gl.game.reusableMonsterWrappers),
		cap(gl.game.reusableMonsterWrappers),
		len(gl.game.reusableProjectileWrappers),
		cap(gl.game.reusableProjectileWrappers),
		len(gl.game.reusableEncounterRewardsMap),
	)

	if gl.renderer != nil {
		fmt.Printf(
			"[PERF] caches floor_colors=%d circles=%d transparent=%d rendered_sprites=%d ray_dirs=%d tile_sprites=%d floor_pixels=%d visible_buf=%d/%d\n",
			len(gl.renderer.floorColorCache),
			len(gl.renderer.circleCache),
			len(gl.renderer.transparentSpritesCache),
			len(gl.renderer.renderedSpritesThisFrame),
			len(gl.renderer.rayDirectionsX),
			len(gl.renderer.tileTypeSpriteCache),
			len(gl.renderer.floorPixels),
			len(gl.renderer.visibleSpritesBuffer),
			cap(gl.renderer.visibleSpritesBuffer),
		)
	}
}

func frameBudgetMs(fps float64) float64 {
	if fps <= 0 {
		return 0
	}
	return 1000.0 / fps
}

func idleBudgetMs(fps float64, updateDur, drawDur time.Duration) float64 {
	budget := frameBudgetMs(fps)
	busy := float64(updateDur.Microseconds()+drawDur.Microseconds()) / 1000.0
	idle := budget - busy
	if idle < 0 {
		return 0
	}
	return idle
}

func getPerfFloat(stats map[string]interface{}, key string) float64 {
	if val, ok := stats[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int32:
			return float64(v)
		case int64:
			return float64(v)
		case uint64:
			return float64(v)
		}
	}
	return 0
}

func getPerfInt(stats map[string]interface{}, key string) int {
	if val, ok := stats[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case uint64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

func getPerfUint(stats map[string]interface{}, key string) uint64 {
	if val, ok := stats[key]; ok {
		switch v := val.(type) {
		case uint64:
			return v
		case int64:
			return uint64(v)
		case int:
			return uint64(v)
		case float64:
			return uint64(v)
		}
	}
	return 0
}

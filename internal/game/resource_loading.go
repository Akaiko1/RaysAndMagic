package game

import (
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

const loadingBannerDelay = 200 * time.Millisecond

// Icons and small portraits should not turn a shop page into a loading screen.
// Cap both each source and the full Draw; panels and world assets still stream.
const smallUIResourceBytes = 256 << 10
const smallUIFrameBytes = 2 << 20

// A render miss unwinds only a speculative Draw. It must never interrupt an
// Update transaction: update lookups return unavailable and queue the source.
type loadingRenderMiss struct{}

type gameLoadingState struct {
	worldPass     bool
	worldRequests map[graphics.SpriteResourceRequest]bool

	stream        *graphics.ResourceStream
	generation    uint64
	started       time.Time
	finished      time.Time
	awaitingFrame bool
	rendering     bool
	front, back   *ebiten.Image
	uploads       []*ebiten.Image
	pattern       <-chan struct{}
	inlineUIBytes int
}

func (gl *GameLoop) ensureResourceLoading() bool {
	if gl == nil || gl.renderer == nil || gl.game.sprites == nil || gl.game.appScreen != AppScreenInGame {
		return false
	}
	if gl.loading == nil {
		gl.loading = &gameLoadingState{}
		gl.renderer.loadCurrentMapFloorTextures()
	}
	l := gl.loading
	if l.stream == nil || l.generation != gl.renderer.mapRenderGeneration {
		l.stream.Close()
		l.stream = graphics.NewResourceStream(gl.game.sprites)
		l.worldRequests = make(map[graphics.SpriteResourceRequest]bool)
		l.generation = gl.renderer.mapRenderGeneration
		l.uploads = nil
	}
	return true
}

func (gl *GameLoop) closeResourceLoading() {
	if gl == nil || gl.loading == nil {
		return
	}
	l := gl.loading
	l.stream.Close()
	if gl.renderer != nil {
		gl.renderer.cancelFloorPreparation()
		gl.renderer.cancelMapRenderPrewarmOutside(nil)
	}
	if l.front != nil {
		l.front.Deallocate()
	}
	if l.back != nil {
		l.back.Deallocate()
	}
	gl.loading = nil
}

func (gl *GameLoop) deferGameplayResource(request graphics.SpriteResourceRequest) bool {
	l := gl.loading
	if l.rendering && !l.worldPass {
		limit := min(smallUIResourceBytes, smallUIFrameBytes-l.inlineUIBytes)
		if bytes := gl.game.sprites.ResourcePixelBytesWithin(request, limit); bytes > 0 {
			l.inlineUIBytes += bytes
			return false
		}
	}
	l.stream.Request(request)
	if l.worldPass || !l.rendering {
		l.worldRequests[request] = true
	}
	l.begin(time.Now())
	if l.rendering {
		panic(loadingRenderMiss{})
	}
	return true
}

func (l *gameLoadingState) begin(now time.Time) {
	if l.started.IsZero() {
		l.started = now
	}
	l.finished = time.Time{}
	l.awaitingFrame = true
}

func (r *Renderer) requiredLoadingRegions() []string {
	keys := []string{currentMapKey()}
	if r.game.openWorldActive() {
		keys = append(keys, visibleOpenWorldMapKeys(world.GlobalWorldManager, r.game.camera,
			float64(r.game.config.GetTileSize()), mapRenderLoadFOVMargin, mapRenderLoadMarginInTiles*float64(r.game.config.GetTileSize()))...)
	}
	return keys
}

func (r *Renderer) loadingRegionsReady() bool {
	if r.game.openWorldActive() {
		r.syncVisibleMapRenderResidency()
	}
	required := r.requiredLoadingRegions()
	r.prioritizeLoadingRegions(required)
	ready := true
	for _, key := range required {
		if _, resident := r.mapRenderResourcesByMap[key]; !resident {
			// A renderer without an authored tile manager has no region work.
			if world.GlobalTileManager == nil && !r.mapRenderResourcePrewarmPending {
				continue
			}
			r.scheduleMapRenderResourcePrewarm(key)
			ready = false
		}
		for _, upload := range r.mapRenderUploadQueue {
			if upload.task != nil && upload.task.mapKey == key {
				ready = false
			}
		}
		for _, task := range r.mapRenderShaderWarmTasks {
			if task != nil && task.mapKey == key {
				ready = false
			}
		}
	}
	return ready
}

func (gl *GameLoop) loadingWorkReady() bool {
	l := gl.loading
	if l == nil {
		return true
	}
	if l.pattern != nil {
		select {
		case <-l.pattern:
			l.pattern = nil
		default:
			return false
		}
	}
	return !l.stream.Pending() && len(l.uploads) == 0 && gl.renderer.floorPreparation == nil && gl.renderer.loadingRegionsReady()
}

func (gl *GameLoop) loadingBarrier() bool {
	if gl.loading == nil || gl.game.appScreen != AppScreenInGame {
		return false
	}
	gl.ensureResourceLoading()
	if !gl.loadingWorkReady() {
		gl.loading.begin(time.Now())
	} else {
		gl.prepareLiveCombatResources()
	}
	return gl.loading.awaitingFrame
}

func (gl *GameLoop) discardLoadingInput() {
	if gl.ui != nil {
		gl.ui.dropQueuedClicks()
		gl.ui.cancelScreenPointerGestures()
		gl.ui.displayedInput.ready = false
	}
	if gl.inputHandler != nil {
		gl.inputHandler.keys.BeginFrame()
	}
	// Retire ordinary drag gestures so their release cannot act on the first
	// complete frame. Picked-up split fragments retain their inventory owner.
	gl.game.prevWorldClickAllowed = false
	gl.game.dragDropAt = 0
	gl.game.stashDragDrop = false
	if !gl.game.dragPickedUp {
		gl.game.clearDrag()
	}
	if !gl.game.stashDragPickedUp {
		gl.game.clearStashDrag()
	}
}

func (gl *GameLoop) tickLoadingPause() {
	gl.discardLoadingInput()
	gl.game.advanceInterfaceClock()
	gl.game.UpdateDamageBlinkTimers()
}

func (gl *GameLoop) advanceResourceLoading() {
	if gl.renderer == nil {
		return
	}
	if gl.loading == nil {
		gl.renderer.prewarmPendingMapRenderResources()
		return
	}
	gl.ensureResourceLoading()
	l := gl.loading
	gl.renderer.advanceFloorPreparation(mapRenderSpriteCommitFrameBytes)
	request, images := l.stream.Advance(mapRenderSpriteCommitFrameBytes)
	if len(images) != 0 {
		if l.worldRequests[request] {
			gl.renderer.observeLazySpriteLoad(request, images)
		}
		for img := range images {
			l.uploads = append(l.uploads, img)
		}
	}
	delete(l.worldRequests, request)
	// A loading pause can spend a little more of the frame on preparation, but
	// never drains the whole region or waits for a worker result.
	deadline := time.Now().Add(2 * time.Millisecond)
	for step := 0; step < 8 && gl.renderer.floorPreparation == nil; step++ {
		gl.renderer.prewarmPendingMapRenderResources()
		if !l.awaitingFrame || time.Now().After(deadline) {
			break
		}
	}
	if panorama, exists := gl.game.skyPanoramaCache[gl.game.currentSkyTexture]; exists {
		gl.game.skyPanorama = panorama
	}
}

func (gl *GameLoop) tryLoadingFrame(dst *ebiten.Image) (complete bool) {
	l := gl.loading
	l.inlineUIBytes = 0
	l.rendering = true
	gl.game.sprites.SetDeferredResourceHandler(gl.deferGameplayResource)
	defer func() {
		l.rendering = false
		gl.game.sprites.SetDeferredResourceHandler(nil)
		if caught := recover(); caught != nil {
			if _, missing := caught.(loadingRenderMiss); !missing {
				panic(caught)
			}
			gl.ui.displayedInput.building = false
			gl.discardLoadingInput()
		}
	}()
	dst.Clear()
	gl.drawExplorationFrame(dst)
	return true
}

func (gl *GameLoop) drawResourceLoadingFrame(screen *ebiten.Image) {
	l := gl.loading
	if l.back == nil || l.back.Bounds().Size() != screen.Bounds().Size() {
		if l.back != nil {
			l.back.Deallocate()
		}
		l.back = ebiten.NewImage(screen.Bounds().Dx(), screen.Bounds().Dy())
	}
	if gl.loadingWorkReady() {
		if gl.tryLoadingFrame(l.back) {
			l.front, l.back = l.back, l.front
			if l.awaitingFrame {
				l.finished = time.Now()
			}
			l.awaitingFrame = false
		}
	} else {
		l.begin(time.Now())
	}
	screen.Fill(color.RGBA{12, 15, 18, 255})
	if l.front != nil {
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(float64(screen.Bounds().Dx())/float64(l.front.Bounds().Dx()), float64(screen.Bounds().Dy())/float64(l.front.Bounds().Dy()))
		screen.DrawImage(l.front, opts)
	}
	// Demand uploads also get a draw submission before the loading gate opens.
	if len(l.uploads) > 0 {
		img := l.uploads[0]
		opts := &ebiten.DrawImageOptions{}
		opts.ColorScale.ScaleAlpha(0)
		opts.GeoM.Scale(1/float64(img.Bounds().Dx()), 1/float64(img.Bounds().Dy()))
		screen.DrawImage(img, opts)
		l.uploads[0] = nil
		l.uploads = l.uploads[1:]
	}
	now := time.Now()
	alpha := l.bannerAlpha(now)
	if alpha > 0 {
		gl.ui.drawLoadingBanner(screen, now.Sub(l.started), alpha)
	} else {
		gl.ui.drawScreenBanner(screen)
	}
}

func (ui *UISystem) loadingBannerLabel() string {
	// Use the same layer priority as input so a hidden hub tab cannot name a
	// loading message for a dialog or another overlay above it.
	if ui.topModalLayer() != modalLayerNone {
		return "Loading..."
	}
	if ui.game.menuOpen {
		switch ui.game.currentTab {
		case TabInventory:
			return "Loading inventory..."
		case TabSpellbook:
			return "Loading spellbook..."
		default:
			return "Loading..."
		}
	}
	return "Loading area..."
}

func (ui *UISystem) drawLoadingBanner(screen *ebiten.Image, elapsed time.Duration, alpha float64) {
	label := ui.loadingBannerLabel()
	offset := -12 * (1 - alpha)
	ui.drawScreenBannerContent(screen, label, bannerQuestProgress, alpha, offset)
	geo := screenBannerLayout(ui.game.config.GetScreenWidth(), label, offset)
	x, y, width := geo.plateX+12, geo.plateY+geo.plateH-7, geo.plateW-24
	drawFilledRect(screen, x, y, width, 3, fadeVectorColor(color.RGBA{48, 42, 20, 255}, alpha))
	// An indeterminate sweep, not a completion percentage. Wall time keeps it
	// moving even when simulation is paused or Draw runs without an Update.
	phase := math.Mod(max(0, (elapsed-loadingBannerDelay).Seconds()), 1.2) / 1.2
	position := (1 - math.Cos(2*math.Pi*phase)) / 2
	segment := max(12, width/5)
	x += int(math.Round(position * float64(width-segment)))
	drawFilledRect(screen, x, y, segment, 3, fadeVectorColor(color.RGBA{232, 195, 94, 255}, alpha))
}

func (l *gameLoadingState) bannerAlpha(now time.Time) float64 {
	if l.started.IsZero() {
		return 0
	}
	if !l.finished.IsZero() {
		if l.finished.Sub(l.started) < loadingBannerDelay {
			l.started = time.Time{}
			return 0
		}
		alpha := 1 - float64(now.Sub(l.finished))/float64(120*time.Millisecond)
		if alpha <= 0 {
			l.started = time.Time{}
			return 0
		}
		return alpha
	}
	return min(1, max(0, float64(now.Sub(l.started)-loadingBannerDelay)/float64(120*time.Millisecond)))
}

// Metadata analysis is CPU-only and shares the ordinary pattern cache. Abort
// the speculative frame before a synchronous PNG analysis can begin.
func (ui *UISystem) loadingPatternFrame(name string, slice int) *patternFrame {
	gl := ui.game.gameLoop
	if gl == nil || gl.loading == nil || !gl.loading.rendering {
		return patternFrameFor(name, slice)
	}
	key := patternFrameCacheKey{name: name, slice: slice}
	patternFrameMu.Lock()
	pf, ready := patternFrameCache[key]
	patternFrameMu.Unlock()
	if ready {
		return pf
	}
	done := make(chan struct{})
	gl.loading.pattern = done
	gl.loading.begin(time.Now())
	go func() { defer close(done); patternFrameFor(name, slice) }()
	panic(loadingRenderMiss{})
}

// Dynamic spawns must have authored attack timing before their next AI tick.
// This uses the same animation names and source of truth as combat itself.
func (gl *GameLoop) prepareLiveCombatResources() {
	if gl.game.world == nil {
		return
	}
	scopes := make([]mapRenderPrewarmScope, 0)
	for _, key := range gl.renderer.requiredLoadingRegions() {
		scopes = append(scopes, gl.renderer.mapRenderPrewarmScope(key))
	}
	for _, mon := range gl.game.world.Monsters {
		if mon == nil || !mon.IsAlive() {
			continue
		}
		behavior := mon.CurrentAIBehavior()
		needed := mon.IsEngagingPlayer || mon.WasAttacked || behavior == monster.AIBehaviorBoundAlly || behavior == monster.AIBehaviorFightFoe
		for _, scope := range scopes {
			needed = needed || scope.containsWorld(mon.X, mon.Y)
		}
		if needed {
			gl.game.authoredMonsterAttackFrameCount(mon)
		}
	}
}

// Keep required work ahead of speculative neighbours without discarding an
// active neighbour's already committed resources on every camera turn.
func (r *Renderer) prioritizeLoadingRegions(required []string) {
	keys := r.mapRenderResourcePrewarmMapKeys
	next := make([]string, 0, len(keys))
	wanted := make(map[string]bool, len(required))
	for _, key := range required {
		wanted[key] = true
	}
	for _, key := range keys {
		if wanted[key] {
			next = append(next, key)
		}
	}
	for _, key := range keys {
		if !wanted[key] {
			next = append(next, key)
		}
	}
	r.mapRenderResourcePrewarmMapKeys = next
}

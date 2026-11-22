package game

import (
	"time"

	"ugataima/internal/graphics"
)

// mapRenderUploadLimit bounds the texture data one frame hands to the GPU.
type mapRenderUploadLimit struct {
	bytes  int64
	images int
}

// full reports that the frame takes no further image. An empty frame accepts
// one image of any size, so a large texture cannot stall the queue.
func (l mapRenderUploadLimit) full(imageCount int, frameBytes, nextImageBytes int64) bool {
	return imageCount > 0 && (imageCount >= l.images || frameBytes+nextImageBytes > l.bytes)
}

// loadingPace is how fast region loading may run. Play has frames to keep
// smooth, so its quotas spread the work thinly. A held frame - the loading
// screen - has nothing to keep smooth: the same work runs as fast as the
// machine allows, and the game loop waits for its workers instead of leaving.
type loadingPace struct {
	// steps is the number of scheduler passes in one Update; 0 = until the
	// deadline.
	steps    int
	duration time.Duration
	// waitForWorkers blocks the Update on a worker result until the deadline.
	waitForWorkers bool
	// decoders and standeeWorkers are the CPU preparations running at once.
	decoders, standeeWorkers int
	// queueBytes bounds decoded pixels waiting for the game loop.
	queueBytes int64
	uploads    mapRenderUploadLimit
}

// The play quotas. Their loading-screen counterparts are authored in
// graphics.streaming.loading_screen and never fall below these.
const (
	mapRenderPlayDecoders       = 2
	mapRenderPlayStandeeWorkers = 1
	mapRenderPlayQueueBytes     = 32 << 20
	// A held Update leaves the rest of the tick to Draw and the banner.
	mapRenderHeldUpdateBudget = 4 * time.Millisecond
)

var mapRenderPlayUploads = mapRenderUploadLimit{bytes: 8 << 20, images: 32}

func (r *Renderer) loadingPace(held bool) loadingPace {
	streaming := r.game.config.Graphics.Streaming
	pace := loadingPace{
		steps:          max(1, streaming.BackgroundStepsPerTick),
		duration:       mapRenderDerivedFrameBudget,
		decoders:       mapRenderPlayDecoders,
		standeeWorkers: mapRenderPlayStandeeWorkers,
		queueBytes:     mapRenderPlayQueueBytes,
		uploads:        mapRenderPlayUploads,
	}
	if !held {
		return pace
	}
	screen := streaming.LoadingScreen
	workers := screen.WorkerCount()
	return loadingPace{
		duration:       mapRenderHeldUpdateBudget,
		waitForWorkers: true,
		decoders:       max(pace.decoders, workers),
		standeeWorkers: max(pace.standeeWorkers, workers),
		queueBytes:     max(pace.queueBytes, int64(screen.QueueMegabytes)<<20),
		uploads: mapRenderUploadLimit{
			bytes:  max(pace.uploads.bytes, int64(screen.UploadMegabytesPerFrame)<<20),
			images: max(pace.uploads.images, screen.UploadImagesPerFrame),
		},
	}
}

// applyLoadingPace switches every worker-side limit to the frame's mode. A
// region that began loading in play speeds up the moment the frame is held.
func (r *Renderer) applyLoadingPace(held bool) loadingPace {
	pace := r.loadingPace(held)
	r.ensureLoadingGates()
	r.decodeGate.SetLimit(pace.decoders)
	r.standeeGate.SetLimit(pace.standeeWorkers)
	if task := r.mapRenderResourcePrewarmActive; task != nil {
		task.queueBudget.SetLimit(pace.queueBytes)
	}
	return pace
}

// ensureLoadingGates starts as many workers as the loading screen may use;
// the pace decides how many of them run.
func (r *Renderer) ensureLoadingGates() {
	if r.decodeGate != nil {
		return
	}
	held := r.loadingPace(true)
	r.decodeGate = graphics.NewWorkerGate(held.decoders, mapRenderPlayDecoders)
	r.standeeGate = graphics.NewWorkerGate(held.standeeWorkers, mapRenderPlayStandeeWorkers)
	if r.game.sprites != nil {
		r.game.sprites.SetDecodeGate(r.decodeGate)
	}
}

// uploadLimit is the quota of the frame being drawn, read when it submits:
// the Draw that releases a held frame is already a play frame.
func (r *Renderer) uploadLimit() mapRenderUploadLimit {
	if r.game == nil || r.game.config == nil {
		return mapRenderPlayUploads
	}
	return r.loadingPace(r.game.loadingStall()).uploads
}

// releasesPerUpdate is how many released textures one Update disposes
// (graphics.streaming.release_images_per_tick), at least one.
func (r *Renderer) releasesPerUpdate() int {
	if r.game == nil || r.game.config == nil {
		return 1
	}
	return max(1, r.game.config.Graphics.Streaming.ReleaseImagesPerTick)
}

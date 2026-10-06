package game

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

type renderLoadDiagnostics struct {
	steps                    uint64
	last, peak               time.Duration
	peakResource             string
	readbacks, readbackBytes uint64
}

func (r *Renderer) readRenderPixels(src *ebiten.Image, dst []byte) {
	if cpu := r.retainedSpriteCPU(src); cpu != nil && cpu.Bounds().Size() == src.Bounds().Size() {
		bounds := cpu.Bounds()
		rowBytes := bounds.Dx() * 4
		for y := 0; y < bounds.Dy(); y++ {
			start := cpu.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			copy(dst[y*rowBytes:(y+1)*rowBytes], cpu.Pix[start:start+rowBytes])
		}
		return
	}
	r.deferRenderReadback(src)
	r.loadDiagnostics.readbacks++
	r.loadDiagnostics.readbackBytes += uint64(len(dst))
	src.ReadPixels(dst)
}

func (r *Renderer) recordRenderLoadStep(start time.Time, task *mapRenderPrewarmTask) {
	d := &r.loadDiagnostics
	d.steps++
	d.last = time.Since(start)
	if d.last > d.peak {
		d.peak = d.last
		d.peakResource = task.mapKey
		if task.prewarmer != nil {
			d.peakResource += ":" + task.prewarmer.lastResource
		}
	}
}

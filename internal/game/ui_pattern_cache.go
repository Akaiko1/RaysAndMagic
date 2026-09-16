package game

import (
	"image"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
)

// Plans and SubImage views use CPU memory only. They never bake a panel-sized
// texture. Bound both bytes and entries so dragging a window cannot retain
// every intermediate size. Views borrow the sprite; never deallocate them.
const patternPlanCacheBytes = 8 << 20
const patternPlanCacheEntries = 16

type patternBlit struct {
	part   *ebiten.Image
	dx, dy int
	dw, dh int
}

type patternPlan struct {
	name        string
	source      *ebiten.Image
	pattern     *patternFrame
	w, h, scale int
	ops         []patternBlit
	ok          bool
	used        uint64
	bytes       int
}

type patternPlanCache struct {
	entries []*patternPlan
	bytes   int
	clock   uint64
}

func (c *patternPlanCache) remove(i int) {
	c.bytes -= c.entries[i].bytes
	copy(c.entries[i:], c.entries[i+1:])
	c.entries[len(c.entries)-1] = nil
	c.entries = c.entries[:len(c.entries)-1]
}

func (c *patternPlanCache) get(name string, src *ebiten.Image, pf *patternFrame, w, h, scale int) *patternPlan {
	c.clock++
	for i := 0; i < len(c.entries); {
		p := c.entries[i]
		if p.name == name && (p.source != src || p.pattern != pf) {
			c.remove(i)
			continue
		}
		if p.source == src && p.pattern == pf && p.w == w && p.h == h && p.scale == scale {
			p.used = c.clock
			return p
		}
		i++
	}
	ops, ok := planPatternFrame(pf, w, h, scale)
	p := &patternPlan{name: name, source: src, pattern: pf, w: w, h: h, scale: scale, ok: ok, used: c.clock}
	if ok {
		p.ops = make([]patternBlit, len(ops))
		// Thousands of repeated tiles share only a handful of source rectangles.
		views := make(map[image.Rectangle]*ebiten.Image)
		for i, op := range ops {
			rect := image.Rect(op.sx, op.sy, op.sx+op.sw, op.sy+op.sh).Add(src.Bounds().Min)
			part := views[rect]
			if part == nil {
				part = src.SubImage(rect).(*ebiten.Image)
				views[rect] = part
			}
			p.ops[i] = patternBlit{part: part, dx: op.dx, dy: op.dy, dw: op.dw, dh: op.dh}
		}
		p.bytes = len(p.ops)*int(unsafe.Sizeof(patternBlit{})) + len(views)*int(unsafe.Sizeof(ebiten.Image{}))
	}
	p.bytes += int(unsafe.Sizeof(patternPlan{}))
	if p.bytes > patternPlanCacheBytes {
		return p
	}
	for len(c.entries) >= patternPlanCacheEntries || c.bytes+p.bytes > patternPlanCacheBytes {
		oldest := 0
		for i, entry := range c.entries {
			if entry.used < c.entries[oldest].used {
				oldest = i
			}
		}
		c.remove(oldest)
	}
	c.entries = append(c.entries, p)
	c.bytes += p.bytes
	return p
}

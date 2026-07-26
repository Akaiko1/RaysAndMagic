package game

import (
	"image"
	"image/draw"
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/graphics"
)

// Pattern-frame nine-slice: panel art like menu_panel_frame is a static 96x96
// painting, not seamless tiles - its edge strips carry corner joints and its
// centre carries inner-corner ornaments. Naive tiling repeats those. This path
// measures, once per sprite, the 1:1 caps/margins and the periodic core of
// every strip, then draws caps 1:1 and tiles only the core. Art that does not
// prove periodic (character_scroll_panel) falls back to the stretch nine-slice.

// stripPattern: capA/capB draw 1:1 at the run's ends, the middle tiles with
// period px of the source run.
type stripPattern struct{ capA, capB, period int }

type patternFrame struct {
	w, h, slice int
	top, bottom stripPattern
	left, right stripPattern
	centreH     stripPattern // margins/period across the centre patch (X axis)
	centreV     stripPattern // margins/period down the centre patch (Y axis)
}

// frameOp is one 1:1 blit from the source sprite into the panel.
type frameOp struct{ sx, sy, sw, sh, dx, dy int }

var (
	patternFrameMu    sync.Mutex
	patternFrameCache = map[string]*patternFrame{}
)

// patternFrameFor analyzes the sprite's PNG once and caches the result;
// nil means "not tileable, stretch it".
func patternFrameFor(name string, slice int) *patternFrame {
	key := name
	patternFrameMu.Lock()
	defer patternFrameMu.Unlock()
	if pf, ok := patternFrameCache[key]; ok {
		return pf
	}
	pf := analyzePatternFrame(name, slice)
	patternFrameCache[key] = pf
	return pf
}

func analyzePatternFrame(name string, slice int) *patternFrame {
	path, ok := graphics.ResolveSpritePath(name)
	if !ok {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	decoded, _, err := image.Decode(file)
	if err != nil {
		return nil
	}
	b := decoded.Bounds()
	img := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), decoded, b.Min, draw.Src)

	w, h := b.Dx(), b.Dy()
	if w <= slice*2 || h <= slice*2 {
		return nil
	}
	pf := &patternFrame{w: w, h: h, slice: slice}
	strips := []struct {
		out  *stripPattern
		r    image.Rectangle
		horz bool
	}{
		{&pf.top, image.Rect(slice, 0, w-slice, slice), true},
		{&pf.bottom, image.Rect(slice, h-slice, w-slice, h), true},
		{&pf.left, image.Rect(0, slice, slice, h-slice), false},
		{&pf.right, image.Rect(w-slice, slice, w, h-slice), false},
		{&pf.centreH, image.Rect(slice, slice, w-slice, h-slice), true},
		{&pf.centreV, image.Rect(slice, slice, w-slice, h-slice), false},
	}
	for _, s := range strips {
		p, ok := analyzeStrip(img, s.r, s.horz)
		if !ok {
			return nil
		}
		*s.out = p
	}
	return pf
}

// analyzeStrip finds the smallest period along the run axis (exact pixel
// equality) and grows the periodic core outward; what is left are the caps.
func analyzeStrip(img *image.NRGBA, r image.Rectangle, horiz bool) (stripPattern, bool) {
	length := r.Dx()
	if !horiz {
		length = r.Dy()
	}
	// One byte-slice per position across the run.
	lines := make([]string, length)
	for i := 0; i < length; i++ {
		var buf []byte
		if horiz {
			for y := r.Min.Y; y < r.Max.Y; y++ {
				o := img.PixOffset(r.Min.X+i, y)
				buf = append(buf, img.Pix[o:o+4]...)
			}
		} else {
			o := img.PixOffset(r.Min.X, r.Min.Y+i)
			buf = append(buf, img.Pix[o:o+4*r.Dx()]...)
		}
		lines[i] = string(buf)
	}
	eq := func(a, b int) bool { return lines[a] == lines[b] }
	for p := 2; p < length/2; p++ {
		c := length / 2
		periodic := true
		for x := c - p; x < c; x++ {
			if x < 0 || x+p >= length || !eq(x, x+p) {
				periodic = false
				break
			}
		}
		if !periodic {
			continue
		}
		a := 0
		for a < length-p && !eq(a, a+p) {
			a++
		}
		b := a + p
		for b < length && eq(b-p, b) {
			b++
		}
		return stripPattern{capA: a, capB: length - b, period: p}, true
	}
	return stripPattern{}, false
}

// planCappedRun emits caps 1:1 and tiles the core; the partial tile lands
// against the far cap. srcX/srcY is the run's source origin, thick its cross
// size. Returns false when the destination cannot hold both caps.
func planCappedRun(ops []frameOp, sp stripPattern, srcX, srcY, srcLen, thick int, horiz bool, dstX, dstY, dstLen int) ([]frameOp, bool) {
	if dstLen < sp.capA+sp.capB+1 {
		return ops, false
	}
	emit := func(off, srcOff, size int) []frameOp {
		if horiz {
			return append(ops, frameOp{srcX + srcOff, srcY, size, thick, dstX + off, dstY})
		}
		return append(ops, frameOp{srcX, srcY + srcOff, thick, size, dstX, dstY + off})
	}
	if sp.capA > 0 {
		ops = emit(0, 0, sp.capA)
	}
	if sp.capB > 0 {
		ops = emit(dstLen-sp.capB, srcLen-sp.capB, sp.capB)
	}
	room := dstLen - sp.capA - sp.capB
	for off := 0; off < room; off += sp.period {
		ops = emit(sp.capA+off, sp.capA, min(sp.period, room-off))
	}
	return ops, true
}

// planPatternFrame lays the whole panel out of 1:1 blits: corners, capped
// edges, then the centre as a nested frame (margin corners 1:1, margin bands
// and core tiled).
func planPatternFrame(pf *patternFrame, w, h int) ([]frameOp, bool) {
	s := pf.slice
	if w <= s*2 || h <= s*2 {
		return nil, false
	}
	var ops []frameOp
	ops = append(ops,
		frameOp{0, 0, s, s, 0, 0},
		frameOp{pf.w - s, 0, s, s, w - s, 0},
		frameOp{0, pf.h - s, s, s, 0, h - s},
		frameOp{pf.w - s, pf.h - s, s, s, w - s, h - s},
	)
	srcRun, srcDrop := pf.w-2*s, pf.h-2*s
	var ok bool
	if ops, ok = planCappedRun(ops, pf.top, s, 0, srcRun, s, true, s, 0, w-2*s); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, pf.bottom, s, pf.h-s, srcRun, s, true, s, h-s, w-2*s); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, pf.left, 0, s, srcDrop, s, false, 0, s, h-2*s); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, pf.right, pf.w-s, s, srcDrop, s, false, w-s, s, h-2*s); !ok {
		return nil, false
	}

	// Centre: margins from both axes' caps, weave core tiled 2D.
	ml, mr := pf.centreH.capA, pf.centreH.capB
	mt, mb := pf.centreV.capA, pf.centreV.capB
	px, py := pf.centreH.period, pf.centreV.period
	cw, ch := w-2*s, h-2*s
	if cw < ml+mr+1 || ch < mt+mb+1 {
		return nil, false
	}
	cx, cy := s, s // centre patch origin, same in src and dst
	ops = append(ops,
		frameOp{cx, cy, ml, mt, cx, cy},
		frameOp{cx + srcRun - mr, cy, mr, mt, cx + cw - mr, cy},
		frameOp{cx, cy + srcDrop - mb, ml, mb, cx, cy + ch - mb},
		frameOp{cx + srcRun - mr, cy + srcDrop - mb, mr, mb, cx + cw - mr, cy + ch - mb},
	)
	band := func(sp stripPattern) stripPattern { return stripPattern{period: sp.period} }
	if ops, ok = planCappedRun(ops, band(pf.centreH), cx+ml, cy, srcRun-ml-mr, mt, true, cx+ml, cy, cw-ml-mr); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, band(pf.centreH), cx+ml, cy+srcDrop-mb, srcRun-ml-mr, mb, true, cx+ml, cy+ch-mb, cw-ml-mr); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, band(pf.centreV), cx, cy+mt, srcDrop-mt-mb, ml, false, cx, cy+mt, ch-mt-mb); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, band(pf.centreV), cx+srcRun-mr, cy+mt, srcDrop-mt-mb, mr, false, cx+cw-mr, cy+mt, ch-mt-mb); !ok {
		return nil, false
	}
	for oy := 0; oy < ch-mt-mb; oy += py {
		th := min(py, ch-mt-mb-oy)
		for ox := 0; ox < cw-ml-mr; ox += px {
			tw := min(px, cw-ml-mr-ox)
			ops = append(ops, frameOp{cx + ml, cy + mt, tw, th, cx + ml + ox, cy + mt + oy})
		}
	}
	return ops, true
}

// drawPatternFrame renders a nine-slice panel, tiling pattern art without
// smearing or repeating its ornaments; non-periodic art stretches as before.
func (ui *UISystem) drawPatternFrame(screen *ebiten.Image, name string, x, y, w, h, slice int) {
	src := ui.game.sprites.GetSprite(name)
	pf := patternFrameFor(name, slice)
	if pf == nil {
		drawNineSlice(screen, src, x, y, w, h, slice)
		return
	}
	ops, ok := planPatternFrame(pf, w, h)
	if !ok {
		drawNineSlice(screen, src, x, y, w, h, slice)
		return
	}
	for _, op := range ops {
		part := src.SubImage(image.Rect(op.sx, op.sy, op.sx+op.sw, op.sy+op.sh)).(*ebiten.Image)
		drawImageScaled(screen, part, x+op.dx, y+op.dy, op.sw, op.sh)
	}
}

package game

import (
	"image"
	"image/draw"
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/graphics"
)

// Pattern-frame nine-slice measures the fixed caps and periodic core of every
// strip, then draws the caps once and tiles only the core. High-resolution
// generated frames use four source pixels per destination pixel so their
// 512x512 sources keep the same on-screen border weight as the original UI.

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

// frameOp is one source blit into the panel. dw/dh differ from sw/sh for
// high-resolution frame sources.
type frameOp struct{ sx, sy, sw, sh, dx, dy, dw, dh int }

type patternFrameCacheKey struct {
	name  string
	slice int
}

var (
	patternFrameMu    sync.Mutex
	patternFrameCache = map[patternFrameCacheKey]*patternFrame{}
)

const (
	generatedPatternFrameSlice          = 96
	generatedSlotFrameSourceScale       = 8
	generatedCompactFrameSourceScale    = 6
	generatedTallPanelFrameSourceScale  = 5
	generatedLargePanelFrameSourceScale = 3
)

func generatedPatternFrame(name string) bool {
	switch name {
	case "menu_panel_frame",
		"menu_panel_slot",
		"menu_panel_tall",
		"menu_panel_wide",
		"menu_panel_slatted",
		"menu_panel_parchment",
		"character_scroll_panel":
		return true
	default:
		return false
	}
}

func patternFrameGeometry(name string, src *ebiten.Image, fallbackSlice int) (slice, sourceScale int) {
	if src == nil || src.Bounds().Dx() != 512 || src.Bounds().Dy() != 512 || !generatedPatternFrame(name) {
		return fallbackSlice, 1
	}
	switch name {
	case "menu_panel_frame", "character_scroll_panel":
		return generatedPatternFrameSlice, generatedCompactFrameSourceScale
	case "menu_panel_slot":
		return generatedPatternFrameSlice, generatedSlotFrameSourceScale
	case "menu_panel_tall":
		return generatedPatternFrameSlice, generatedTallPanelFrameSourceScale
	default:
		return generatedPatternFrameSlice, generatedLargePanelFrameSourceScale
	}
}

// patternFrameFor analyzes the sprite's PNG once and caches the result;
// nil means "not tileable, stretch it".
func patternFrameFor(name string, slice int) *patternFrame {
	key := patternFrameCacheKey{name: name, slice: slice}
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

func scaledFrameLength(sourceLength, sourceScale int) int {
	if sourceLength <= 0 {
		return 0
	}
	return max(1, (sourceLength+sourceScale/2)/sourceScale)
}

// planCappedRun emits fixed caps and tiles the core; the partial tile lands
// against the far cap. sourceScale is source pixels per destination pixel.
func planCappedRun(ops []frameOp, sp stripPattern, srcX, srcY, srcLen, thick int, horiz bool, dstX, dstY, dstLen, sourceScale int) ([]frameOp, bool) {
	capA := scaledFrameLength(sp.capA, sourceScale)
	capB := scaledFrameLength(sp.capB, sourceScale)
	period := scaledFrameLength(sp.period, sourceScale)
	dstThick := scaledFrameLength(thick, sourceScale)
	if dstLen < capA+capB+1 {
		return ops, false
	}
	emit := func(off, srcOff, srcSize, dstSize int) []frameOp {
		if horiz {
			return append(ops, frameOp{srcX + srcOff, srcY, srcSize, thick, dstX + off, dstY, dstSize, dstThick})
		}
		return append(ops, frameOp{srcX, srcY + srcOff, thick, srcSize, dstX, dstY + off, dstThick, dstSize})
	}
	if sp.capA > 0 {
		ops = emit(0, 0, sp.capA, capA)
	}
	if sp.capB > 0 {
		ops = emit(dstLen-capB, srcLen-sp.capB, sp.capB, capB)
	}
	room := dstLen - capA - capB
	for off := 0; off < room; off += period {
		dstSize := min(period, room-off)
		srcSize := sp.period
		if dstSize < period {
			srcSize = max(1, (sp.period*dstSize+period-1)/period)
		}
		ops = emit(capA+off, sp.capA, srcSize, dstSize)
	}
	return ops, true
}

// planPatternFrame lays the whole panel out of corners, capped edges, then the
// centre as a nested frame. sourceScale is source pixels per destination pixel.
func planPatternFrame(pf *patternFrame, w, h, sourceScale int) ([]frameOp, bool) {
	s := pf.slice
	ds := scaledFrameLength(s, sourceScale)
	if w <= ds*2 || h <= ds*2 {
		return nil, false
	}
	var ops []frameOp
	ops = append(ops,
		frameOp{0, 0, s, s, 0, 0, ds, ds},
		frameOp{pf.w - s, 0, s, s, w - ds, 0, ds, ds},
		frameOp{0, pf.h - s, s, s, 0, h - ds, ds, ds},
		frameOp{pf.w - s, pf.h - s, s, s, w - ds, h - ds, ds, ds},
	)
	srcRun, srcDrop := pf.w-2*s, pf.h-2*s
	var ok bool
	if ops, ok = planCappedRun(ops, pf.top, s, 0, srcRun, s, true, ds, 0, w-2*ds, sourceScale); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, pf.bottom, s, pf.h-s, srcRun, s, true, ds, h-ds, w-2*ds, sourceScale); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, pf.left, 0, s, srcDrop, s, false, 0, ds, h-2*ds, sourceScale); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, pf.right, pf.w-s, s, srcDrop, s, false, w-ds, ds, h-2*ds, sourceScale); !ok {
		return nil, false
	}

	// Centre: margins from both axes' caps, weave core tiled 2D.
	ml, mr := pf.centreH.capA, pf.centreH.capB
	mt, mb := pf.centreV.capA, pf.centreV.capB
	px, py := pf.centreH.period, pf.centreV.period
	dml, dmr := scaledFrameLength(ml, sourceScale), scaledFrameLength(mr, sourceScale)
	dmt, dmb := scaledFrameLength(mt, sourceScale), scaledFrameLength(mb, sourceScale)
	dpx, dpy := scaledFrameLength(px, sourceScale), scaledFrameLength(py, sourceScale)
	cw, ch := w-2*ds, h-2*ds
	if cw < dml+dmr+1 || ch < dmt+dmb+1 {
		return nil, false
	}
	cx, cy := s, s
	dcx, dcy := ds, ds
	ops = append(ops,
		frameOp{cx, cy, ml, mt, dcx, dcy, dml, dmt},
		frameOp{cx + srcRun - mr, cy, mr, mt, dcx + cw - dmr, dcy, dmr, dmt},
		frameOp{cx, cy + srcDrop - mb, ml, mb, dcx, dcy + ch - dmb, dml, dmb},
		frameOp{cx + srcRun - mr, cy + srcDrop - mb, mr, mb, dcx + cw - dmr, dcy + ch - dmb, dmr, dmb},
	)
	band := func(sp stripPattern) stripPattern { return stripPattern{period: sp.period} }
	if ops, ok = planCappedRun(ops, band(pf.centreH), cx+ml, cy, srcRun-ml-mr, mt, true, dcx+dml, dcy, cw-dml-dmr, sourceScale); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, band(pf.centreH), cx+ml, cy+srcDrop-mb, srcRun-ml-mr, mb, true, dcx+dml, dcy+ch-dmb, cw-dml-dmr, sourceScale); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, band(pf.centreV), cx, cy+mt, srcDrop-mt-mb, ml, false, dcx, dcy+dmt, ch-dmt-dmb, sourceScale); !ok {
		return nil, false
	}
	if ops, ok = planCappedRun(ops, band(pf.centreV), cx+srcRun-mr, cy+mt, srcDrop-mt-mb, mr, false, dcx+cw-dmr, dcy+dmt, ch-dmt-dmb, sourceScale); !ok {
		return nil, false
	}
	for oy := 0; oy < ch-dmt-dmb; oy += dpy {
		dh := min(dpy, ch-dmt-dmb-oy)
		sh := py
		if dh < dpy {
			sh = max(1, (py*dh+dpy-1)/dpy)
		}
		for ox := 0; ox < cw-dml-dmr; ox += dpx {
			dw := min(dpx, cw-dml-dmr-ox)
			sw := px
			if dw < dpx {
				sw = max(1, (px*dw+dpx-1)/dpx)
			}
			ops = append(ops, frameOp{cx + ml, cy + mt, sw, sh, dcx + dml + ox, dcy + dmt + oy, dw, dh})
		}
	}
	return ops, true
}

// drawPatternFrame renders a nine-slice panel, tiling pattern art without
// smearing or repeating its ornaments; non-periodic art stretches as before.
func (ui *UISystem) drawPatternFrame(screen *ebiten.Image, name string, x, y, w, h, slice int) {
	src := ui.game.sprites.GetSprite(name)
	slice, sourceScale := patternFrameGeometry(name, src, slice)
	pf := patternFrameFor(name, slice)
	if pf == nil {
		if sourceScale == 1 {
			drawNineSlice(screen, src, x, y, w, h, slice)
		} else {
			drawImageScaled(screen, src, x, y, w, h)
		}
		return
	}
	ops, ok := planPatternFrame(pf, w, h, sourceScale)
	if !ok {
		if sourceScale == 1 {
			drawNineSlice(screen, src, x, y, w, h, slice)
		} else {
			drawImageScaled(screen, src, x, y, w, h)
		}
		return
	}
	for _, op := range ops {
		part := src.SubImage(image.Rect(op.sx, op.sy, op.sx+op.sw, op.sy+op.sh)).(*ebiten.Image)
		drawImageScaled(screen, part, x+op.dx, y+op.dy, op.dw, op.dh)
	}
}
